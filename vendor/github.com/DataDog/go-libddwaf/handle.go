// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

import (
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/go-libddwaf/internal/noopfree"
	"go.uber.org/atomic"
)

// Handle represents an instance of the WAF for a given ruleset.
type Handle struct {
	// Instance of the WAF
	cHandle wafHandle

	// Lock-less reference counter avoiding blocking calls to the Close() method
	// while WAF contexts are still using the WAF handle. Instead, we let the
	// release actually happen only when the reference counter reaches 0.
	// This can happen either from a request handler calling its WAF context's
	// Close() method, or either from the appsec instance calling the WAF
	// handle's Close() method when creating a new WAF handle with new rules.
	// Note that this means several instances of the WAF can exist at the same
	// time with their own set of rules. This choice was done to be able to
	// efficiently update the security rules concurrently, without having to
	// block the request handlers for the time of the security rules update.
	refCounter *atomic.Int32

	// RWMutex protecting the R/W accesses to the internal rules data (stored
	// in the handle).
	mutex sync.RWMutex

	// rulesetInfo holds information about rules initialization
	rulesetInfo RulesetInfo
}

// NewHandle creates and returns a new instance of the WAF with the given security rules and configuration
// of the sensitive data obfuscator. The returned handle is nil in case of an error.
// Rules-related metrics, including errors, are accessible with the `RulesetInfo()` method.
func NewHandle(rules any, keyObfuscatorRegex string, valueObfuscatorRegex string) (*Handle, error) {
	// The order of action is the following:
	// - Open the ddwaf C library
	// - Encode the security rules as a ddwaf_object
	// - Create a ddwaf_config object and fill the values
	// - Run ddwaf_init to create a new handle based on the given rules and config
	// - Check for errors and streamline the ddwaf_ruleset_info returned

	if ok, err := Load(); !ok {
		return nil, err
		// The case where ok == true && err != nil is ignored on purpose, as
		// this is out of the scope of NewHandle which only requires a properly
		// loaded libddwaf in order to use it
	}

	encoder := newMaxEncoder()
	obj, err := encoder.Encode(rules)
	if err != nil {
		return nil, fmt.Errorf("could not encode the WAF ruleset into a WAF object: %w", err)
	}

	config := newConfig(&encoder.cgoRefs, keyObfuscatorRegex, valueObfuscatorRegex)
	cRulesetInfo := new(wafRulesetInfo)

	cHandle := wafLib.wafInit(obj, config, cRulesetInfo)
	keepAlive(encoder.cgoRefs)
	// Note that the encoded obj was copied by libddwaf, so we don't need to keep them alive
	// for the lifetime of the handle (ddwaf API guarantee).
	if cHandle == 0 {
		return nil, errors.New("could not instantiate the WAF")
	}

	defer wafLib.wafRulesetInfoFree(cRulesetInfo)

	errorsMap, err := decodeErrors(&cRulesetInfo.errors)
	if err != nil { // Something is very wrong
		return nil, fmt.Errorf("could not decode the WAF ruleset errors: %w", err)
	}

	return &Handle{
		cHandle:    cHandle,
		refCounter: atomic.NewInt32(1), // We count the handle itself in the counter
		rulesetInfo: RulesetInfo{
			Loaded:  cRulesetInfo.loaded,
			Failed:  cRulesetInfo.failed,
			Errors:  errorsMap,
			Version: gostring(cast[byte](cRulesetInfo.version)),
		},
	}, nil
}

// RulesetInfo returns the rules initialization metrics for the current WAF handle
func (handle *Handle) RulesetInfo() RulesetInfo {
	return handle.rulesetInfo
}

// Addresses returns the list of addresses the WAF rule is expecting.
func (handle *Handle) Addresses() []string {
	return wafLib.wafRequiredAddresses(handle.cHandle)
}

// closeContext calls ddwaf_context_destroy and eventually ddwaf_destroy on the handle
func (handle *Handle) closeContext(context *Context) {
	wafLib.wafContextDestroy(context.cContext)
	if handle.addRefCounter(-1) == 0 {
		wafLib.wafDestroy(handle.cHandle)
	}
}

// Close puts the handle in termination state, when all the contexts are closed the handle will be destroyed
func (handle *Handle) Close() {
	if handle.addRefCounter(-1) > 0 {
		// There are still Contexts that are not closed
		return
	}

	wafLib.wafDestroy(handle.cHandle)
}

// addRefCounter add x to Handle.refCounter.
// It relies on a CAS spin-loop implementation in order to avoid changing the
// counter when 0 has been reached.
func (handle *Handle) addRefCounter(x int32) int32 {
	for {
		current := handle.refCounter.Load()
		if current == 0 {
			// The object was released
			return 0
		}
		if swapped := handle.refCounter.CompareAndSwap(current, current+x); swapped {
			return current + x
		}
	}
}

func newConfig(cgoRefs *cgoRefPool, keyObfuscatorRegex string, valueObfuscatorRegex string) *wafConfig {
	config := new(wafConfig)
	*config = wafConfig{
		limits: wafConfigLimits{
			maxContainerDepth: wafMaxContainerDepth,
			maxContainerSize:  wafMaxContainerSize,
			maxStringLength:   wafMaxStringLength,
		},
		obfuscator: wafConfigObfuscator{
			keyRegex:   cgoRefs.AllocCString(keyObfuscatorRegex),
			valueRegex: cgoRefs.AllocCString(valueObfuscatorRegex),
		},
		// Prevent libddwaf from freeing our Go-memory-allocated ddwaf_objects
		freeFn: noopfree.NoopFreeFn,
	}
	return config
}

func goRunError(rc wafReturnCode) error {
	switch rc {
	case wafErrInternal:
		return ErrInternal
	case wafErrInvalidObject:
		return ErrInvalidObject
	case wafErrInvalidArgument:
		return ErrInvalidArgument
	default:
		return fmt.Errorf("unknown waf return code %d", int(rc))
	}
}
