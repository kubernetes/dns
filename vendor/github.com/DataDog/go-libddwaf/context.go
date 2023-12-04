// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

import (
	"sync"
	"time"

	"go.uber.org/atomic"
)

// Context is a WAF execution context. It allows running the WAF incrementally
// when calling it multiple times to run its rules every time new addresses
// become available. Each request must have its own Context.
type Context struct {
	// Instance of the WAF
	handle   *Handle
	cContext wafContext
	// cgoRefs is used to retain go references to WafObjects until the context is destroyed.
	// As per libddwaf documentation, WAF Objects must be alive during all the context lifetime
	cgoRefs cgoRefPool
	// Mutex protecting the use of cContext which is not thread-safe and cgoRefs.
	mutex sync.Mutex

	// Stats
	// Cumulated internal WAF run time - in nanoseconds - for this context.
	totalRuntimeNs atomic.Uint64
	// Cumulated overall run time - in nanoseconds - for this context.
	totalOverallRuntimeNs atomic.Uint64
	// Cumulated timeout count for this context.
	timeoutCount atomic.Uint64
}

// NewContext returns a new WAF context of to the given WAF handle.
// A nil value is returned when the WAF handle was released or when the
// WAF context couldn't be created.
// handle. A nil value is returned when the WAF handle can no longer be used
// or the WAF context couldn't be created.
func NewContext(handle *Handle) *Context {
	// Handle has been released
	if handle.addRefCounter(1) == 0 {
		return nil
	}

	cContext := wafLib.wafContextInit(handle.cHandle)
	if cContext == 0 {
		handle.addRefCounter(-1)
		return nil
	}

	return &Context{handle: handle, cContext: cContext}
}

// Run encodes the given addressesToData values and runs them against the WAF rules within the given
// timeout value. It returns the matches as a JSON string (usually opaquely used) along with the corresponding
// actions in any. In case of an error, matches and actions can still be returned, for instance in the case of a
// timeout error. Errors can be tested against the RunError type.
func (context *Context) Run(addressesToData map[string]any, timeout time.Duration) (matches []byte, actions []string, err error) {
	if len(addressesToData) == 0 {
		return
	}

	now := time.Now()
	defer func() {
		dt := time.Since(now)
		context.totalOverallRuntimeNs.Add(uint64(dt.Nanoseconds()))
	}()

	encoder := encoder{
		stringMaxSize:    wafMaxStringLength,
		containerMaxSize: wafMaxContainerSize,
		objectMaxDepth:   wafMaxContainerDepth,
	}
	obj, err := encoder.Encode(addressesToData)
	if err != nil {
		return nil, nil, err
	}

	// ddwaf_run cannot run concurrently and the next append write on the context state so we need a mutex
	context.mutex.Lock()
	defer context.mutex.Unlock()

	// Save the Go pointer references to addressesToData that were referenced by the encoder
	// into C ddwaf_objects. libddwaf's API requires to keep this data for the lifetime of the ddwaf_context.
	defer context.cgoRefs.append(encoder.cgoRefs)

	return context.run(obj, timeout, &encoder.cgoRefs)
}

func (context *Context) run(obj *wafObject, timeout time.Duration, cgoRefs *cgoRefPool) ([]byte, []string, error) {
	// RLock the handle to safely get read access to the WAF handle and prevent concurrent changes of it
	// such as a rules-data update.
	context.handle.mutex.RLock()
	defer context.handle.mutex.RUnlock()

	result := new(wafResult)
	defer wafLib.wafResultFree(result)

	ret := wafLib.wafRun(context.cContext, obj, result, uint64(timeout/time.Microsecond))

	context.totalRuntimeNs.Add(result.total_runtime)
	matches, actions, err := unwrapWafResult(ret, result)
	if err == ErrTimeout {
		context.timeoutCount.Inc()
	}

	return matches, actions, err
}

func unwrapWafResult(ret wafReturnCode, result *wafResult) (matches []byte, actions []string, err error) {
	if result.timeout > 0 {
		err = ErrTimeout
	}

	if ret == wafOK {
		return nil, nil, err
	}

	if ret != wafMatch {
		return nil, nil, goRunError(ret)
	}

	if result.data != 0 {
		matches = []byte(gostring(cast[byte](result.data)))
	}

	if size := result.actions.size; size > 0 {
		actions = decodeActions(result.actions.array, uint64(size))
	}

	return matches, actions, err
}

// Close calls handle.closeContext which calls ddwaf_context_destroy and maybe also close the handle if it in termination state.
func (context *Context) Close() {
	defer context.handle.closeContext(context)
	// Keep the Go pointer references until the end of the context
	keepAlive(context.cgoRefs)
	// The context is no longer used so we can try releasing the Go pointer references asap by nulling them
	context.cgoRefs = cgoRefPool{}
}

// TotalRuntime returns the cumulated WAF runtime across various run calls within the same WAF context.
// Returned time is in nanoseconds.
func (context *Context) TotalRuntime() (overallRuntimeNs, internalRuntimeNs uint64) {
	return context.totalOverallRuntimeNs.Load(), context.totalRuntimeNs.Load()
}

// TotalTimeouts returns the cumulated amount of WAF timeouts across various run calls within the same WAF context.
func (context *Context) TotalTimeouts() uint64 {
	return context.timeoutCount.Load()
}
