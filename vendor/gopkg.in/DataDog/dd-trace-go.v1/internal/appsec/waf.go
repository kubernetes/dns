// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

//go:build appsec
// +build appsec

package appsec

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo/instrumentation/grpcsec"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo/instrumentation/httpsec"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo/instrumentation/sharedsec"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/samplernames"

	waf "github.com/DataDog/go-libddwaf"
	"go.uber.org/atomic"
)

const (
	eventRulesVersionTag = "_dd.appsec.event_rules.version"
	eventRulesErrorsTag  = "_dd.appsec.event_rules.errors"
	eventRulesLoadedTag  = "_dd.appsec.event_rules.loaded"
	eventRulesFailedTag  = "_dd.appsec.event_rules.error_count"
	wafDurationTag       = "_dd.appsec.waf.duration"
	wafDurationExtTag    = "_dd.appsec.waf.duration_ext"
	wafTimeoutTag        = "_dd.appsec.waf.timeouts"
	wafVersionTag        = "_dd.appsec.waf.version"
)

type wafHandle struct {
	*waf.Handle
	// Actions are tightly link to a ruleset, which is linked to a waf handle
	actions map[string]*sharedsec.Action
}

func (a *appsec) swapWAF(rules rulesFragment) (err error) {
	// Instantiate a new WAF handle and verify its state
	newHandle, err := newWAFHandle(rules, a.cfg)
	if err != nil {
		return err
	}

	// Close the WAF handle in case of an error in what's following
	defer func() {
		if err != nil {
			newHandle.Close()
		}
	}()

	listeners, err := newWAFEventListeners(newHandle, a.cfg, a.limiter)
	if err != nil {
		return err
	}

	// Register the event listeners now that we know that the new handle is valid
	newRoot := dyngo.NewRootOperation()
	for _, l := range listeners {
		newRoot.On(l)
	}

	// Hot-swap dyngo's root operation
	dyngo.SwapRootOperation(newRoot)

	// Close old handle.
	// Note that concurrent requests are still using it, and it will be released
	// only when no more requests use it.
	// TODO: implement in dyngo ref-counting of the root operation so we can
	//   rely on a Finish event listener on the root operation instead?
	//   Avoiding saving the current WAF handle would guarantee no one is
	//   accessing a.wafHandle while we swap
	oldHandle := a.wafHandle
	a.wafHandle = newHandle
	if oldHandle != nil {
		oldHandle.Close()
	}

	return nil
}

func actionFromEntry(e *actionEntry) *sharedsec.Action {
	switch e.Type {
	case "block_request":
		grpcCode := 10 // use the grpc.Codes value for "Aborted" by default
		if e.Parameters.GRPCStatusCode != nil {
			grpcCode = *e.Parameters.GRPCStatusCode
		}
		return sharedsec.NewBlockRequestAction(e.Parameters.StatusCode, grpcCode, e.Parameters.Type)
	case "redirect_request":
		return sharedsec.NewRedirectRequestAction(e.Parameters.StatusCode, e.Parameters.Location)
	default:
		log.Debug("appsec: unknown action type `%s`", e.Type)
		return nil
	}
}

func newWAFHandle(rules rulesFragment, cfg *Config) (*wafHandle, error) {
	handle, err := waf.NewHandle(rules, cfg.obfuscator.KeyRegex, cfg.obfuscator.ValueRegex)
	actions := map[string]*sharedsec.Action{
		// Default built-in block action
		"block": sharedsec.NewBlockRequestAction(403, 10, "auto"),
	}

	for _, entry := range rules.Actions {
		a := actionFromEntry(&entry)
		if a != nil {
			actions[entry.ID] = a
		}
	}
	return &wafHandle{
		Handle:  handle,
		actions: actions,
	}, err
}

func newWAFEventListeners(waf *wafHandle, cfg *Config, l Limiter) (listeners []dyngo.EventListener, err error) {
	// Check if there are addresses in the rule
	ruleAddresses := waf.Addresses()
	if len(ruleAddresses) == 0 {
		return nil, errors.New("no addresses found in the rule")
	}

	// Check there are supported addresses in the rule
	httpAddresses, grpcAddresses, notSupported := supportedAddresses(ruleAddresses)
	if len(httpAddresses) == 0 && len(grpcAddresses) == 0 {
		return nil, fmt.Errorf("the addresses present in the rules are not supported: %v", notSupported)
	}

	if len(notSupported) > 0 {
		log.Debug("appsec: the addresses present in the rules are partially supported: not supported=%v", notSupported)
	}

	// Register the WAF event listeners
	if len(httpAddresses) > 0 {
		log.Debug("appsec: creating http waf event listener of the rules addresses %v", httpAddresses)
		listeners = append(listeners, newHTTPWAFEventListener(waf, httpAddresses, cfg.wafTimeout, l))
	}

	if len(grpcAddresses) > 0 {
		log.Debug("appsec: creating the grpc waf event listener of the rules addresses %v", grpcAddresses)
		listeners = append(listeners, newGRPCWAFEventListener(waf, grpcAddresses, cfg.wafTimeout, l))
	}

	return listeners, nil
}

// newWAFEventListener returns the WAF event listener to register in order to enable it.
func newHTTPWAFEventListener(handle *wafHandle, addresses map[string]struct{}, timeout time.Duration, limiter Limiter) dyngo.EventListener {
	var monitorRulesOnce sync.Once // per instantiation

	return httpsec.OnHandlerOperationStart(func(op *httpsec.Operation, args httpsec.HandlerOperationArgs) {
		wafCtx := waf.NewContext(handle.Handle)
		if wafCtx == nil {
			// The WAF event listener got concurrently released
			return
		}

		if _, ok := addresses[userIDAddr]; ok {
			// OnUserIDOperationStart happens when appsec.SetUser() is called. We run the WAF and apply actions to
			// see if the associated user should be blocked. Since we don't control the execution flow in this case
			// (SetUser is SDK), we delegate the responsibility of interrupting the handler to the user.
			op.On(sharedsec.OnUserIDOperationStart(func(operation *sharedsec.UserIDOperation, args sharedsec.UserIDOperationArgs) {
				matches, actionIds := runWAF(wafCtx, map[string]interface{}{userIDAddr: args.UserID}, timeout)
				if len(matches) > 0 {
					processHTTPSDKAction(operation, handle.actions, actionIds)
					addSecurityEvents(op, limiter, matches)
					log.Debug("appsec: WAF detected a suspicious user: %s", args.UserID)
				}
			}))
		}

		values := map[string]interface{}{}
		for addr := range addresses {
			switch addr {
			case httpClientIPAddr:
				if args.ClientIP.IsValid() {
					values[httpClientIPAddr] = args.ClientIP.String()
				}
			case serverRequestMethodAddr:
				values[serverRequestMethodAddr] = args.Method
			case serverRequestRawURIAddr:
				values[serverRequestRawURIAddr] = args.RequestURI
			case serverRequestHeadersNoCookiesAddr:
				if headers := args.Headers; headers != nil {
					values[serverRequestHeadersNoCookiesAddr] = headers
				}
			case serverRequestCookiesAddr:
				if cookies := args.Cookies; cookies != nil {
					values[serverRequestCookiesAddr] = cookies
				}
			case serverRequestQueryAddr:
				if query := args.Query; query != nil {
					values[serverRequestQueryAddr] = query
				}
			case serverRequestPathParamsAddr:
				if pathParams := args.PathParams; pathParams != nil {
					values[serverRequestPathParamsAddr] = pathParams
				}
			}
		}

		matches, actionIds := runWAF(wafCtx, values, timeout)
		if len(matches) > 0 {
			interrupt := processActions(op, handle.actions, actionIds)
			addSecurityEvents(op, limiter, matches)
			log.Debug("appsec: WAF detected an attack before executing the request")
			if interrupt {
				wafCtx.Close()
				return
			}
		}

		if _, ok := addresses[serverRequestBodyAddr]; ok {
			op.On(httpsec.OnSDKBodyOperationStart(func(sdkBodyOp *httpsec.SDKBodyOperation, args httpsec.SDKBodyOperationArgs) {
				matches, actionIds := runWAF(wafCtx, map[string]interface{}{serverRequestBodyAddr: args.Body}, timeout)
				if len(matches) > 0 {
					processHTTPSDKAction(sdkBodyOp, handle.actions, actionIds)
					addSecurityEvents(op, limiter, matches)
					log.Debug("appsec: WAF detected a suspicious request body")
				}
			}))
		}

		op.On(httpsec.OnHandlerOperationFinish(func(op *httpsec.Operation, res httpsec.HandlerOperationRes) {
			defer wafCtx.Close()

			values := make(map[string]interface{}, 1)
			if _, ok := addresses[serverResponseStatusAddr]; ok {
				values[serverResponseStatusAddr] = res.Status
			}

			// Run the WAF, ignoring the returned actions - if any - since blocking after the request handler's
			// response is not supported at the moment.
			matches, _ := runWAF(wafCtx, values, timeout)

			// Add WAF metrics.
			rInfo := handle.RulesetInfo()
			overallRuntimeNs, internalRuntimeNs := wafCtx.TotalRuntime()
			addWAFMonitoringTags(op, rInfo.Version, overallRuntimeNs, internalRuntimeNs, wafCtx.TotalTimeouts())

			// Add the following metrics once per instantiation of a WAF handle
			monitorRulesOnce.Do(func() {
				addRulesMonitoringTags(op, rInfo)
				op.AddTag(ext.ManualKeep, samplernames.AppSec)
			})

			// Log the attacks if any
			if len(matches) == 0 {
				return
			}
			log.Debug("appsec: attack detected by the waf")
			addSecurityEvents(op, limiter, matches)
		}))
	})
}

// newGRPCWAFEventListener returns the WAF event listener to register in order
// to enable it.
func newGRPCWAFEventListener(handle *wafHandle, addresses map[string]struct{}, timeout time.Duration, limiter Limiter) dyngo.EventListener {
	var monitorRulesOnce sync.Once // per instantiation

	return grpcsec.OnHandlerOperationStart(func(op *grpcsec.HandlerOperation, handlerArgs grpcsec.HandlerOperationArgs) {
		// Limit the maximum number of security events, as a streaming RPC could
		// receive unlimited number of messages where we could find security events
		const maxWAFEventsPerRequest = 10
		var (
			nbEvents          atomic.Uint32
			logOnce           sync.Once // per request
			overallRuntimeNs  atomic.Uint64
			internalRuntimeNs atomic.Uint64
			nbTimeouts        atomic.Uint64

			events []json.RawMessage
			mu     sync.Mutex // events mutex
		)

		wafCtx := waf.NewContext(handle.Handle)
		if wafCtx == nil {
			// The WAF event listener got concurrently released
			return
		}

		// OnUserIDOperationStart happens when appsec.SetUser() is called. We run the WAF and apply actions to
		// see if the associated user should be blocked. Since we don't control the execution flow in this case
		// (SetUser is SDK), we delegate the responsibility of interrupting the handler to the user.
		op.On(sharedsec.OnUserIDOperationStart(func(userIDOp *sharedsec.UserIDOperation, args sharedsec.UserIDOperationArgs) {
			values := map[string]interface{}{}
			for addr := range addresses {
				if addr == userIDAddr {
					values[userIDAddr] = args.UserID
				}
			}
			matches, actionIds := runWAF(wafCtx, values, timeout)
			if len(matches) > 0 {
				for _, id := range actionIds {
					if a, ok := handle.actions[id]; ok && a.Blocking() {
						code, err := a.GRPC()(map[string][]string{})
						userIDOp.EmitData(grpcsec.NewMonitoringError(err.Error(), code))
					}
				}
				addSecurityEvents(op, limiter, matches)
				log.Debug("appsec: WAF detected an authenticated user attack: %s", args.UserID)
			}
		}))

		// The same address is used for gRPC and http when it comes to client ip
		values := map[string]interface{}{}
		for addr := range addresses {
			if addr == httpClientIPAddr && handlerArgs.ClientIP.IsValid() {
				values[httpClientIPAddr] = handlerArgs.ClientIP.String()
			}
		}

		matches, actionIds := runWAF(wafCtx, values, timeout)
		if len(matches) > 0 {
			interrupt := processActions(op, handle.actions, actionIds)
			addSecurityEvents(op, limiter, matches)
			log.Debug("appsec: WAF detected an attack before executing the request")
			if interrupt {
				wafCtx.Close()
				return
			}
		}

		op.On(grpcsec.OnReceiveOperationFinish(func(_ grpcsec.ReceiveOperation, res grpcsec.ReceiveOperationRes) {
			if nbEvents.Load() == maxWAFEventsPerRequest {
				logOnce.Do(func() {
					log.Debug("appsec: ignoring the rpc message due to the maximum number of security events per grpc call reached")
				})
				return
			}
			// The current workaround of the WAF context limitations is to
			// simply instantiate and release the WAF context for the operation
			// lifetime so that:
			//   1. We avoid growing the memory usage of the context every time
			//      a grpc.server.request.message value is added to it during
			//      the RPC lifetime.
			//   2. We avoid the limitation of 1 event per attack type.
			// TODO(Julio-Guerra): a future libddwaf API should solve this out.
			wafCtx := waf.NewContext(handle.Handle)
			if wafCtx == nil {
				// The WAF event listener got concurrently released
				return
			}
			defer wafCtx.Close()
			// Run the WAF on the rule addresses available in the args
			// Note that we don't check if the address is present in the rules
			// as we only support one at the moment, so this callback cannot be
			// set when the address is not present.
			values := map[string]interface{}{grpcServerRequestMessage: res.Message}
			if md := handlerArgs.Metadata; len(md) > 0 {
				values[grpcServerRequestMetadata] = md
			}
			// Run the WAF, ignoring the returned actions - if any - since blocking after the request handler's
			// response is not supported at the moment.
			event, _ := runWAF(wafCtx, values, timeout)

			// WAF run durations are WAF context bound. As of now we need to keep track of those externally since
			// we use a new WAF context for each callback. When we are able to re-use the same WAF context across
			// callbacks, we can get rid of these variables and simply use the WAF bindings in OnHandlerOperationFinish.
			overall, internal := wafCtx.TotalRuntime()
			overallRuntimeNs.Add(overall)
			internalRuntimeNs.Add(internal)
			nbTimeouts.Add(wafCtx.TotalTimeouts())

			if len(event) == 0 {
				return
			}
			log.Debug("appsec: attack detected by the grpc waf")
			nbEvents.Inc()
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}))

		op.On(grpcsec.OnHandlerOperationFinish(func(op *grpcsec.HandlerOperation, _ grpcsec.HandlerOperationRes) {
			defer wafCtx.Close()
			rInfo := handle.RulesetInfo()
			addWAFMonitoringTags(op, rInfo.Version, overallRuntimeNs.Load(), internalRuntimeNs.Load(), nbTimeouts.Load())

			// Log the following metrics once per instantiation of a WAF handle
			monitorRulesOnce.Do(func() {
				addRulesMonitoringTags(op, rInfo)
				op.AddTag(ext.ManualKeep, samplernames.AppSec)
			})

			addSecurityEvents(op, limiter, events...)
		}))
	})
}

func runWAF(wafCtx *waf.Context, values map[string]interface{}, timeout time.Duration) ([]byte, []string) {
	matches, actions, err := wafCtx.Run(values, timeout)
	if err != nil {
		if err == waf.ErrTimeout {
			log.Debug("appsec: waf timeout value of %s reached", timeout)
		} else {
			log.Error("appsec: unexpected waf error: %v", err)
			return nil, nil
		}
	}
	return matches, actions
}

// HTTP rule addresses currently supported by the WAF
const (
	serverRequestMethodAddr           = "server.request.method"
	serverRequestRawURIAddr           = "server.request.uri.raw"
	serverRequestHeadersNoCookiesAddr = "server.request.headers.no_cookies"
	serverRequestCookiesAddr          = "server.request.cookies"
	serverRequestQueryAddr            = "server.request.query"
	serverRequestPathParamsAddr       = "server.request.path_params"
	serverRequestBodyAddr             = "server.request.body"
	serverResponseStatusAddr          = "server.response.status"
	httpClientIPAddr                  = "http.client_ip"
	userIDAddr                        = "usr.id"
)

// List of HTTP rule addresses currently supported by the WAF
var httpAddresses = []string{
	serverRequestMethodAddr,
	serverRequestRawURIAddr,
	serverRequestHeadersNoCookiesAddr,
	serverRequestCookiesAddr,
	serverRequestQueryAddr,
	serverRequestPathParamsAddr,
	serverRequestBodyAddr,
	serverResponseStatusAddr,
	httpClientIPAddr,
	userIDAddr,
}

// gRPC rule addresses currently supported by the WAF
const (
	grpcServerRequestMessage  = "grpc.server.request.message"
	grpcServerRequestMetadata = "grpc.server.request.metadata"
)

// List of gRPC rule addresses currently supported by the WAF
var grpcAddresses = []string{
	grpcServerRequestMessage,
	grpcServerRequestMetadata,
	httpClientIPAddr,
	userIDAddr,
}

func init() {
	// sort the address lists to avoid mistakes and use sort.SearchStrings()
	sort.Strings(httpAddresses)
	sort.Strings(grpcAddresses)
}

// supportedAddresses returns the list of addresses we actually support from the
// given rule addresses.
func supportedAddresses(ruleAddresses []string) (supportedHTTP, supportedGRPC map[string]struct{}, notSupported []string) {
	// Filter the supported addresses only
	supportedHTTP = map[string]struct{}{}
	supportedGRPC = map[string]struct{}{}
	for _, addr := range ruleAddresses {
		supported := false
		if i := sort.SearchStrings(httpAddresses, addr); i < len(httpAddresses) && httpAddresses[i] == addr {
			supportedHTTP[addr] = struct{}{}
			supported = true
		}
		if i := sort.SearchStrings(grpcAddresses, addr); i < len(grpcAddresses) && grpcAddresses[i] == addr {
			supportedGRPC[addr] = struct{}{}
			supported = true
		}

		if !supported {
			notSupported = append(notSupported, addr)
		}
	}

	return supportedHTTP, supportedGRPC, notSupported
}

type tagsHolder interface {
	AddTag(string, interface{})
}

// Add the tags related to security rules monitoring
func addRulesMonitoringTags(th tagsHolder, rInfo waf.RulesetInfo) {
	if len(rInfo.Errors) == 0 {
		rInfo.Errors = nil
	}
	rulesetErrors, err := json.Marshal(rInfo.Errors)
	if err != nil {
		log.Error("appsec: could not marshal the waf ruleset info errors to json")
	}
	th.AddTag(eventRulesErrorsTag, string(rulesetErrors)) // avoid the tracer's call to fmt.Sprintf on the value
	th.AddTag(eventRulesLoadedTag, float64(rInfo.Loaded))
	th.AddTag(eventRulesFailedTag, float64(rInfo.Failed))
	th.AddTag(wafVersionTag, waf.Version())
}

// Add the tags related to the monitoring of the WAF
func addWAFMonitoringTags(th tagsHolder, rulesVersion string, overallRuntimeNs, internalRuntimeNs, timeouts uint64) {
	// Rules version is set for every request to help the backend associate WAF duration metrics with rule version
	th.AddTag(eventRulesVersionTag, rulesVersion)
	th.AddTag(wafTimeoutTag, float64(timeouts))
	th.AddTag(wafDurationTag, float64(internalRuntimeNs)/1e3)   // ns to us
	th.AddTag(wafDurationExtTag, float64(overallRuntimeNs)/1e3) // ns to us
}

type securityEventsAdder interface {
	AddSecurityEvents(events ...json.RawMessage)
}

// Helper function to add sec events to an operation taking into account the rate limiter.
func addSecurityEvents(op securityEventsAdder, limiter Limiter, matches ...json.RawMessage) {
	if len(matches) > 0 && limiter.Allow() {
		op.AddSecurityEvents(matches...)
	}
}

// processActions sends the relevant actions to the operation's data listener.
// It returns true if at least one of those actions require interrupting the request handler
func processActions(op dyngo.Operation, actions map[string]*sharedsec.Action, actionIds []string) (interrupt bool) {
	for _, id := range actionIds {
		if a, ok := actions[id]; ok {
			op.EmitData(actions[id])
			interrupt = interrupt || a.Blocking()
		}
	}
	return interrupt
}

// processHTTPSDKAction does two things:
//   - send actions to the parent operation's data listener, for their handlers to be executed after the user handler
//   - send an error to the current operation's data listener (created by an SDK call), to signal users to interrupt
//     their handler.
func processHTTPSDKAction(op dyngo.Operation, actions map[string]*sharedsec.Action, actionIds []string) {
	for _, id := range actionIds {
		if action, ok := actions[id]; ok {
			if op.Parent() != nil {
				op.Parent().EmitData(action) // Send the action so that the handler gets executed
			}
			if action.Blocking() { // Send the error to be returned by the SDK
				op.EmitData(httpsec.NewMonitoringError("Request blocked")) // Send error
			}
		}
	}
}
