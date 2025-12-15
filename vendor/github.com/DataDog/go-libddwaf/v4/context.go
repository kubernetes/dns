// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libddwaf

import (
	"fmt"
	"maps"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
	"github.com/DataDog/go-libddwaf/v4/internal/pin"
	"github.com/DataDog/go-libddwaf/v4/timer"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
)

// Context is a WAF execution context. It allows running the WAF incrementally when calling it
// multiple times to run its rules every time new addresses become available. Each request must have
// its own [Context]. New [Context] instances can be created by calling
// [Handle.NewContext].
type Context struct {
	// Timer registers the time spent in the WAF and go-libddwaf. It is created alongside the Context using the options
	// passed in to NewContext. Once its time budget is exhausted, each new call to Context.Run will return a timeout error.
	Timer timer.NodeTimer

	handle *Handle // Instance of the WAF

	cContext bindings.WAFContext // The C ddwaf_context pointer

	// mutex protecting the use of cContext which is not thread-safe and truncations
	mutex sync.Mutex

	// truncations provides details about truncations that occurred while encoding address data for the WAF execution.
	truncations map[TruncationReason][]int

	// pinner is used to retain Go data that is being passed to the WAF as part of
	// [RunAddressData.Persistent] until the [Context.Close] method results in the context being
	// destroyed.
	pinner pin.ConcurrentPinner
}

// RunAddressData provides address data to the [Context.Run] method. If a given key is present in
// both `Persistent` and `Ephemeral`, the value from `Persistent` will take precedence.
// When encoding Go structs to the WAF-compatible format, fields with the `ddwaf:"ignore"` tag are
// ignored and will not be visible to the WAF.
type RunAddressData struct {
	// Persistent address data is scoped to the lifetime of a given Context, and subsquent calls to
	// Context.Run with the same address name will be silently ignored.
	Persistent map[string]any
	// Ephemeral address data is scoped to a given Context.Run call and is not persisted across
	// calls. This is used for protocols such as gRPC client/server streaming or GraphQL, where a
	// single request can incur multiple subrequests.
	Ephemeral map[string]any

	// TimerKey is the key used to track the time spent in the WAF for this run.
	// If left empty, a new timer with unlimited budget is started.
	TimerKey timer.Key
}

func (d RunAddressData) isEmpty() bool {
	return len(d.Persistent) == 0 && len(d.Ephemeral) == 0
}

// newTimer creates a new timer for this run. If the TimerKey is empty, a new timer without taking the parent into account is created.
func (d RunAddressData) newTimer(parent timer.NodeTimer) (timer.NodeTimer, error) {
	if d.TimerKey == "" {
		return timer.NewTreeTimer(
			timer.WithComponents(
				EncodeTimeKey,
				DurationTimeKey,
				DecodeTimeKey,
			),
			timer.WithBudget(parent.SumRemaining()),
		)
	}

	return parent.NewNode(d.TimerKey,
		timer.WithComponents(
			EncodeTimeKey,
			DurationTimeKey,
			DecodeTimeKey,
		),
		timer.WithInheritedSumBudget(),
	)
}

// Run encodes the given [RunAddressData] values and runs them against the WAF rules.
// Callers must check the returned [Result] object even when an error is returned, as the WAF might
// have been able to match some rules and generate events or actions before the error was reached;
// especially when the error is [waferrors.ErrTimeout].
func (context *Context) Run(addressData RunAddressData) (res Result, err error) {
	if addressData.isEmpty() {
		return Result{}, nil
	}

	// If the context has already timed out, we don't need to run the WAF again
	if context.Timer.SumExhausted() {
		return Result{}, waferrors.ErrTimeout
	}

	runTimer, err := addressData.newTimer(context.Timer)
	if err != nil {
		return Result{}, err
	}

	defer func() {
		res.TimerStats = runTimer.Stats()
	}()

	runTimer.Start()
	defer runTimer.Stop()

	wafEncodeTimer := runTimer.MustLeaf(EncodeTimeKey)
	wafEncodeTimer.Start()
	defer wafEncodeTimer.Stop()

	persistentData, err := context.encodeOneAddressType(&context.pinner, addressData.Persistent, wafEncodeTimer)
	if err != nil {
		return Result{}, err
	}

	// The WAF releases ephemeral address data at the max of each run call, so we need not keep the Go
	// values live beyond that in the same way we need for persistent data. We hence use a separate
	// encoder.
	var ephemeralPinner runtime.Pinner
	defer ephemeralPinner.Unpin()
	ephemeralData, err := context.encodeOneAddressType(&ephemeralPinner, addressData.Ephemeral, wafEncodeTimer)
	if err != nil {
		return Result{}, err
	}

	wafEncodeTimer.Stop()

	// ddwaf_run cannot run concurrently, so we need to lock the context
	context.mutex.Lock()
	defer context.mutex.Unlock()

	if context.cContext == 0 {
		// Context has been closed, returning an empty result...
		return Result{}, waferrors.ErrContextClosed
	}

	if runTimer.SumExhausted() {
		return Result{}, waferrors.ErrTimeout
	}

	return context.run(persistentData, ephemeralData, runTimer)
}

// merge merges two maps of slices into a single map of slices. The resulting map will contain all
// keys from both a and b, with the corresponding value from a and b concatenated (in this order) in
// a single slice. The implementation tries to minimize reallocations.
func merge[K comparable, V any](a, b map[K][]V) (merged map[K][]V) {
	count := len(a) + len(b)
	if count == 0 {
		return
	}

	keys := make(map[K]struct{}, count)
	nothing := struct{}{}
	totalCount := 0
	for _, m := range [2]map[K][]V{a, b} {
		for k, v := range m {
			keys[k] = nothing
			totalCount += len(v)
		}
	}

	merged = make(map[K][]V, count)
	values := make([]V, 0, totalCount)

	for k := range keys {
		idxS := len(values) // Start index
		values = append(values, a[k]...)
		values = append(values, b[k]...)
		idxE := len(values) // End index

		merged[k] = values[idxS:idxE]
	}

	return
}

// encodeOneAddressType encodes the given addressData values and returns the corresponding WAF
// object and its refs. If the addressData is empty, it returns nil for the WAF object and an empty
// ref pool.
// At this point, if the encoder does not timeout, the only error we can get is an error in case the
// top level object is a nil map, but this  behaviour is expected since either persistent or
// ephemeral addresses are allowed to be null one at a time. In this case, Encode will return nil,
// which is what we need to send to ddwaf_run to signal that the address data is empty.
func (context *Context) encodeOneAddressType(pinner pin.Pinner, addressData map[string]any, timer timer.Timer) (*bindings.WAFObject, error) {
	if addressData == nil {
		return nil, nil
	}

	encoder, err := newEncoder(newEncoderConfig(pinner, timer))
	if err != nil {
		return nil, fmt.Errorf("could not create encoder: %w", err)
	}

	data, _ := encoder.Encode(addressData)
	if len(encoder.truncations) > 0 {
		context.mutex.Lock()
		defer context.mutex.Unlock()

		context.truncations = merge(context.truncations, encoder.truncations)
	}

	if timer.Exhausted() {
		return nil, waferrors.ErrTimeout
	}

	return data, nil
}

// run executes the ddwaf_run call with the provided data on this context. The caller is responsible for locking the
// context appropriately around this call.
func (context *Context) run(persistentData, ephemeralData *bindings.WAFObject, runTimer timer.NodeTimer) (Result, error) {
	var pinner runtime.Pinner
	defer pinner.Unpin()

	var result bindings.WAFObject
	pinner.Pin(&result)
	defer bindings.Lib.ObjectFree(&result)

	// The value of the timeout cannot exceed 2^55
	// cf. https://en.cppreference.com/w/cpp/chrono/duration
	timeout := uint64(runTimer.SumRemaining().Microseconds()) & 0x008FFFFFFFFFFFFF
	ret := bindings.Lib.Run(context.cContext, persistentData, ephemeralData, &result, timeout)

	decodeTimer := runTimer.MustLeaf(DecodeTimeKey)
	decodeTimer.Start()
	defer decodeTimer.Stop()

	res, duration, err := unwrapWafResult(ret, &result)
	runTimer.AddTime(DurationTimeKey, duration)
	return res, err
}

func unwrapWafResult(ret bindings.WAFReturnCode, result *bindings.WAFObject) (Result, time.Duration, error) {
	if !result.IsMap() {
		return Result{}, 0, fmt.Errorf("invalid result (expected map, got %s)", result.Type)
	}

	entries, err := result.Values()
	if err != nil {
		return Result{}, 0, err
	}

	var (
		res      Result
		duration time.Duration
	)
	for _, entry := range entries {
		switch key := entry.MapKey(); key {
		case "timeout":
			timeout, err := entry.BoolValue()
			if err != nil {
				return Result{}, 0, fmt.Errorf("failed to decode timeout: %w", err)
			}
			if timeout {
				err = waferrors.ErrTimeout
			}
		case "keep":
			keep, err := entry.BoolValue()
			if err != nil {
				return Result{}, 0, fmt.Errorf("failed to decode keep: %w", err)
			}
			res.Keep = keep
		case "duration":
			dur, err := entry.UIntValue()
			if err != nil {
				return Result{}, 0, fmt.Errorf("failed to decode duration: %w", err)
			}
			duration = time.Duration(dur) * time.Nanosecond
		case "events":
			if !entry.IsArray() {
				return Result{}, 0, fmt.Errorf("invalid events (expected array, got %s)", entry.Type)
			}
			if entry.NbEntries != 0 {
				events, err := entry.ArrayValue()
				if err != nil {
					return Result{}, 0, fmt.Errorf("failed to decode events: %w", err)
				}
				res.Events = events
			}
		case "actions":
			if !entry.IsMap() {
				return Result{}, 0, fmt.Errorf("invalid actions (expected map, got %s)", entry.Type)
			}
			if entry.NbEntries != 0 {
				actions, err := entry.MapValue()
				if err != nil {
					return Result{}, 0, fmt.Errorf("failed to decode actions: %w", err)
				}
				res.Actions = actions
			}
		case "attributes":
			if !entry.IsMap() {
				return Result{}, 0, fmt.Errorf("invalid attributes (expected map, got %s)", entry.Type)
			}
			if entry.NbEntries != 0 {
				derivatives, err := entry.MapValue()
				if err != nil {
					return Result{}, 0, fmt.Errorf("failed to decode attributes: %w", err)
				}
				res.Derivatives = derivatives
			}
		}
	}

	return res, duration, goRunError(ret)
}

// Close disposes of the underlying `ddwaf_context` and releases the associated
// internal data. It also decreases the reference count of the [Handle] which
// created this [Context], possibly releasing it completely (if this was the
// last [Context] created from it, and it is no longer in use by its creator).
func (context *Context) Close() {
	context.mutex.Lock()
	defer context.mutex.Unlock()

	bindings.Lib.ContextDestroy(context.cContext)
	defer context.handle.Close() // Reduce the reference counter of the Handle.
	context.cContext = 0         // Makes it easy to spot use-after-free/double-free issues

	context.pinner.Unpin() // The pinned data is no longer needed, explicitly release
}

// Truncations returns the truncations that occurred while encoding address data for WAF execution.
// The key is the truncation reason: either because the object was too deep, the arrays where to large or the strings were too long.
// The value is a slice of integers, each integer being the original size of the object that was truncated.
// In case of the [ObjectTooDeep] reason, the original size can only be approximated because of recursive objects.
func (context *Context) Truncations() map[TruncationReason][]int {
	context.mutex.Lock()
	defer context.mutex.Unlock()

	return maps.Clone(context.truncations)
}
