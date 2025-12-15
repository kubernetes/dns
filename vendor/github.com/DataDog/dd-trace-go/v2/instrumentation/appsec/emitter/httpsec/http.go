// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package httpsec defines is the HTTP instrumentation API and contract for
// AppSec. It defines an abstract representation of HTTP handlers, along with
// helper functions to wrap (aka. instrument) standard net/http handlers.
// HTTP integrations must use this package to enable AppSec features for HTTP,
// which listens to this package's operation events.
package httpsec

import (
	"context"
	// Blank import needed to use embed for the default blocked response payloads
	_ "embed"
	"net/http"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/appsec/events"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/actions"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/addresses"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/trace"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/emitter/waf"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
)

// HandlerOperation type representing an HTTP operation. It must be created with
// StartOperation() and finished with its Finish().
type (
	HandlerOperation struct {
		dyngo.Operation
		*waf.ContextOperation

		// wafContextOwner indicates if the waf.ContextOperation was started by us or not and if we need to close it.
		wafContextOwner bool

		// framework is the name of the framework or library that started the operation.
		framework string
		// method is the HTTP method for the current handler operation.
		method string
		// route is the HTTP route for the current handler operation (or the URL if no route is available).
		route string

		// downstreamRequestBodyAnalysis is the number of times a call to a downstream request body monitoring function was made.
		downstreamRequestBodyAnalysis atomic.Int32
	}

	// HandlerOperationArgs is the HTTP handler operation arguments.
	HandlerOperationArgs struct {
		Framework    string // Optional: name of the framework or library being used
		Method       string
		RequestURI   string
		RequestRoute string
		Host         string
		RemoteAddr   string
		Headers      map[string][]string
		Cookies      map[string][]string
		QueryParams  map[string][]string
		PathParams   map[string]string
	}

	// HandlerOperationRes is the HTTP handler operation results.
	HandlerOperationRes struct {
		Headers    map[string][]string
		StatusCode int
	}

	// EarlyBlock is used to trigger an early block before the handler is executed.
	EarlyBlock struct{}
)

func (HandlerOperationArgs) IsArgOf(*HandlerOperation)   {}
func (HandlerOperationRes) IsResultOf(*HandlerOperation) {}

func StartOperation(ctx context.Context, args HandlerOperationArgs, span trace.TagSetter) (*HandlerOperation, *atomic.Pointer[actions.BlockHTTP], context.Context) {
	wafOp, found := dyngo.FindOperation[waf.ContextOperation](ctx)
	if !found {
		wafOp, ctx = waf.StartContextOperation(ctx, span)
	}

	op := &HandlerOperation{
		Operation:        dyngo.NewOperation(wafOp),
		ContextOperation: wafOp,
		wafContextOwner:  !found, // If we started the parent operation, we finish it, otherwise we don't
		framework:        args.Framework,
		method:           args.Method,
		route:            args.RequestRoute,
	}

	// We need to use an atomic pointer to store the action because the action may be created asynchronously in the future
	var action atomic.Pointer[actions.BlockHTTP]
	dyngo.OnData(op, func(a *actions.BlockHTTP) {
		action.Store(a)
	})

	return op, &action, dyngo.StartAndRegisterOperation(ctx, op, args)
}

// Framework returns the name of the framework or library that started the operation.
func (op *HandlerOperation) Framework() string {
	return op.framework
}

// Method returns the HTTP method for the current handler operation.
func (op *HandlerOperation) Method() string {
	return op.method
}

// Route returns the HTTP route for the current handler operation.
func (op *HandlerOperation) Route() string {
	return op.route
}

// DownstreamRequestBodyAnalysis returns the number of times a call to a downstream request body monitoring function was made.
func (op *HandlerOperation) DownstreamRequestBodyAnalysis() int {
	return int(op.downstreamRequestBodyAnalysis.Load())
}

// IncrementDownstreamRequestBodyAnalysis increments the number of times a call to a downstream request body monitoring function was made.
func (op *HandlerOperation) IncrementDownstreamRequestBodyAnalysis() {
	op.downstreamRequestBodyAnalysis.Add(1)
}

// Finish the HTTP handler operation and its children operations and write everything to the service entry span.
func (op *HandlerOperation) Finish(res HandlerOperationRes) {
	dyngo.FinishOperation(op, res)
	if op.wafContextOwner {
		op.ContextOperation.Finish()
	}
}

const (
	monitorParsedBodyErrorLog = `
"appsec: parsed http body monitoring ignored: could not find the http handler instrumentation metadata in the request context:
	the request handler is not being monitored by a middleware function or the provided context is not the expected request context
`
	monitorResponseBodyErrorLog = `
"appsec: http response body monitoring ignored: could not find the http handler instrumentation metadata in the request context:
	the request handler is not being monitored by a middleware function or the provided context is not the expected request context
`
)

// MonitorParsedBody starts and finishes the SDK body operation.
// This function should not be called when AppSec is disabled in order to
// get more accurate error logs.
func MonitorParsedBody(ctx context.Context, body any) error {
	return waf.RunSimple(ctx,
		addresses.NewAddressesBuilder().
			WithRequestBody(body).
			Build(),
		monitorParsedBodyErrorLog,
	)
}

// MonitorResponseBody gets the response body through the in-app WAF.
// This function should not be called when AppSec is disabled in order to get
// more accurate error logs.
func MonitorResponseBody(ctx context.Context, body any) error {
	return waf.RunSimple(ctx,
		addresses.NewAddressesBuilder().
			WithResponseBody(body).
			Build(),
		monitorResponseBodyErrorLog,
	)
}

// Return the map of parsed cookies if any and following the specification of
// the rule address `server.request.cookies`.
func makeCookies(parsed []*http.Cookie) map[string][]string {
	if len(parsed) == 0 {
		return nil
	}
	cookies := make(map[string][]string, len(parsed))
	for _, c := range parsed {
		cookies[c.Name] = append(cookies[c.Name], c.Value)
	}
	return cookies
}

// RouteMatched can be called if BeforeHandle is started too early in the http request lifecycle like
// before the router has matched the request to a route. This can happen when the HTTP handler is wrapped
// using http.NewServeMux instead of http.WrapHandler. In this case the route is empty and so are the path parameters.
// In this case the route and path parameters will be filled in later by calling RouteMatched with the actual route.
// If RouteMatched returns an error, the request should be considered blocked and the error should be reported.
func RouteMatched(ctx context.Context, route string, routeParams map[string]string) error {
	op, ok := dyngo.FindOperation[HandlerOperation](ctx)
	if !ok {
		log.Debug("appsec: RouteMatched called without an active HandlerOperation in the context, ignoring")
		telemetrylog.With(telemetry.WithTags([]string{"product:appsec"})).
			Warn("appsec: RouteMatched called without an active HandlerOperation in the context, ignoring")
		return nil
	}

	// Overwrite the previous route that was created using a quantization algorithm
	op.route = route

	var err error
	dyngo.OnData(op, func(e *events.BlockingSecurityEvent) {
		err = e
	})

	// Call the WAF with this new data
	op.Run(op, addresses.NewAddressesBuilder().
		WithPathParams(routeParams).
		Build(),
	)

	return err
}

// BeforeHandle contains the appsec functionality that should be executed before a http.Handler runs.
// It returns the modified http.ResponseWriter and http.Request, an additional afterHandle function
// that should be executed after the Handler runs, and a handled bool that instructs if the request has been handled
// or not - in case it was handled, the original handler should not run.
func BeforeHandle(
	w http.ResponseWriter,
	r *http.Request,
	span trace.TagSetter,
	opts *Config,
) (http.ResponseWriter, *http.Request, func(), bool) {
	if opts == nil {
		opts = defaultWrapHandlerConfig
	}
	if opts.ResponseHeaderCopier == nil {
		opts.ResponseHeaderCopier = defaultWrapHandlerConfig.ResponseHeaderCopier
	}

	op, blockAtomic, ctx := StartOperation(r.Context(), HandlerOperationArgs{
		Framework:    opts.Framework,
		Method:       r.Method,
		RequestURI:   r.RequestURI,
		RequestRoute: opts.Route,
		Host:         r.Host,
		RemoteAddr:   r.RemoteAddr,
		Headers:      r.Header,
		Cookies:      makeCookies(r.Cookies()),
		QueryParams:  r.URL.Query(),
		PathParams:   opts.RouteParams,
	}, span)
	tr := r.WithContext(ctx)

	afterHandle := func() {
		var statusCode int
		if res, ok := w.(interface{ Status() int }); ok {
			statusCode = res.Status()
		}
		op.Finish(HandlerOperationRes{
			Headers:    opts.ResponseHeaderCopier(w),
			StatusCode: statusCode,
		})

		// Execute the onBlock functions to make sure blocking works properly
		// in case we are instrumenting the Gin framework
		if blockPtr := blockAtomic.Load(); blockPtr != nil {
			for _, f := range opts.OnBlock {
				f()
			}

			if blockPtr.Handler != nil {
				blockPtr.Handler.ServeHTTP(w, tr)
			}
		}
	}

	handled := false
	if blockPtr := blockAtomic.Load(); blockPtr != nil && blockPtr.Handler != nil {
		// handler is replaced
		blockPtr.Handler.ServeHTTP(w, tr)
		blockPtr.Handler = nil
		handled = true
	}

	// We register a handler for cases that would require us to write the blocking response before any more code
	// from a specific framework (like Gin) is executed that would write another (wrong) response here.
	dyngo.OnData(op, func(e EarlyBlock) {
		if blockPtr := blockAtomic.Load(); blockPtr != nil && blockPtr.Handler != nil {
			blockPtr.Handler.ServeHTTP(w, tr)
			blockPtr.Handler = nil
		}
	})

	return w, tr, afterHandle, handled
}

// WrapHandler wraps the given HTTP handler with the abstract HTTP operation defined by HandlerOperationArgs and
// HandlerOperationRes.
// The onBlock params are used to cleanup the context when needed.
// It is a specific patch meant for Gin, for which we must abort the
// context since it uses a queue of handlers and it's the only way to make
// sure other queued handlers don't get executed.
// TODO: this patch must be removed/improved when we rework our actions/operations system
func WrapHandler(handler http.Handler, span trace.TagSetter, opts *Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw, tr, afterHandle, handled := BeforeHandle(w, r, span, opts)
		defer afterHandle()
		if handled {
			return
		}

		handler.ServeHTTP(tw, tr)
	})
}
