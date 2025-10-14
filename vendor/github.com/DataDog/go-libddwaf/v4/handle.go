// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libddwaf

import (
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
	"github.com/DataDog/go-libddwaf/v4/internal/unsafe"
	"github.com/DataDog/go-libddwaf/v4/timer"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
)

// Handle represents an instance of the WAF for a given ruleset. It is obtained
// from [Builder.Build]; and must be disposed of by calling [Handle.Close]
// once no longer in use.
type Handle struct {
	// Lock-less reference counter avoiding blocking calls to the [Handle.Close]
	// method while WAF [Context]s are still using the WAF handle. Instead, we let
	// the release actually happen only when the reference counter reaches 0.
	// This can happen either from a request handler calling its WAF context's
	// [Context.Close] method, or either from the appsec instance calling the WAF
	// [Handle.Close] method when creating a new WAF handle with new rules.
	// Note that this means several instances of the WAF can exist at the same
	// time with their own set of rules. This choice was done to be able to
	// efficiently update the security rules concurrently, without having to
	// block the request handlers for the time of the security rules update.
	refCounter atomic.Int32

	// Instance of the WAF
	cHandle bindings.WAFHandle
}

// wrapHandle wraps the provided C handle into a [Handle]. The caller is
// responsible to ensure the cHandle value is not 0 (NULL). The returned
// [Handle] has a reference count of 1, so callers need not call [Handle.retain]
// on it.
func wrapHandle(cHandle bindings.WAFHandle) *Handle {
	handle := &Handle{cHandle: cHandle}
	handle.refCounter.Store(1) // We count the handle itself in the counter
	return handle
}

// NewContext returns a new WAF context for the given WAF handle.
// An error is returned when the WAF handle was released or when the WAF context
// couldn't be created.
func (handle *Handle) NewContext(timerOptions ...timer.Option) (*Context, error) {
	// Handle has been released
	if !handle.retain() {
		return nil, fmt.Errorf("handle was released")
	}

	cContext := wafLib.ContextInit(handle.cHandle)
	if cContext == 0 {
		handle.Close() // We couldn't get a context, so we no longer have an implicit reference to the Handle in it...
		return nil, fmt.Errorf("could not get C context")
	}

	rootTimer, err := timer.NewTreeTimer(timerOptions...)
	if err != nil {
		return nil, err
	}

	return &Context{
		handle:      handle,
		cContext:    cContext,
		Timer:       rootTimer,
		truncations: make(map[TruncationReason][]int, 3),
	}, nil
}

// Addresses returns the list of addresses the WAF has been configured to monitor based on the input
// ruleset.
func (handle *Handle) Addresses() []string {
	return wafLib.KnownAddresses(handle.cHandle)
}

// Actions returns the list of actions the WAF has been configured to monitor based on the input
// ruleset.
func (handle *Handle) Actions() []string {
	return wafLib.KnownActions(handle.cHandle)
}

// Close decrements the reference counter of this [Handle], possibly allowing it to be destroyed
// and all the resources associated with it to be released.
func (handle *Handle) Close() {
	if handle.addRefCounter(-1) != 0 {
		// Either the counter is still positive (this Handle is still referenced), or it had previously
		// reached 0 and some other call has done the cleanup already.
		return
	}

	wafLib.Destroy(handle.cHandle)
	handle.cHandle = 0 // Makes it easy to spot use-after-free/double-free issues
}

// retain increments the reference counter of this [Handle]. Returns true if the
// [Handle] is still valid, false if it is no longer usable. Calls to
// [Handle.retain] must be balanced with calls to [Handle.Close] in order to
// avoid leaking [Handle]s.
func (handle *Handle) retain() bool {
	return handle.addRefCounter(1) > 0
}

// addRefCounter adds x to Handle.refCounter. The return valid indicates whether the refCounter
// reached 0 as part of this call or not, which can be used to perform "only-once" activities:
//
// * result > 0    => the Handle is still usable
// * result == 0   => the handle is no longer usable, ref counter reached 0 as part of this call
// * result == -1  => the handle is no longer usable, ref counter was already 0 previously
func (handle *Handle) addRefCounter(x int32) int32 {
	// We use a CAS loop to avoid setting the refCounter to a negative value.
	for {
		current := handle.refCounter.Load()
		if current <= 0 {
			// The object had already been released
			return -1
		}

		next := current + x
		if swapped := handle.refCounter.CompareAndSwap(current, next); swapped {
			if next < 0 {
				// TODO(romain.marcadier): somehow signal unexpected behavior to the
				// caller (panic? error?). We currently clamp to 0 in order to avoid
				// causing a customer program crash, but this is the symptom of a bug
				// and should be investigated (however this clamping hides the issue).
				return 0
			}
			return next
		}
	}
}

func newConfig(pinner *runtime.Pinner, keyObfuscatorRegex string, valueObfuscatorRegex string) *bindings.WAFConfig {
	return &bindings.WAFConfig{
		Limits: bindings.WAFConfigLimits{
			MaxContainerDepth: bindings.MaxContainerDepth,
			MaxContainerSize:  bindings.MaxContainerSize,
			MaxStringLength:   bindings.MaxStringLength,
		},
		Obfuscator: bindings.WAFConfigObfuscator{
			KeyRegex:   unsafe.PtrToUintptr(unsafe.Cstring(pinner, keyObfuscatorRegex)),
			ValueRegex: unsafe.PtrToUintptr(unsafe.Cstring(pinner, valueObfuscatorRegex)),
		},
		// Prevent libddwaf from freeing our Go-memory-allocated ddwaf_objects
		FreeFn: 0,
	}
}

func goRunError(rc bindings.WAFReturnCode) error {
	switch rc {
	case bindings.WAFErrInternal:
		return waferrors.ErrInternal
	case bindings.WAFErrInvalidObject:
		return waferrors.ErrInvalidObject
	case bindings.WAFErrInvalidArgument:
		return waferrors.ErrInvalidArgument
	case bindings.WAFOK, bindings.WAFMatch:
		// No error...
		return nil
	default:
		return fmt.Errorf("unknown waf return code %d", int(rc))
	}
}
