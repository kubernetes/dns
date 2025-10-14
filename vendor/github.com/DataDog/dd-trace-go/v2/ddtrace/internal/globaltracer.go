// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import "sync/atomic"

var (
	// globalTracer stores the current tracer as *ddtrace/tracer.Tracer (pointer to interface). The
	// atomic.Value type requires types to be consistent, which requires using the same type for the
	// stored value.
	globalTracer atomic.Value
)

// tracerLike is an interface to restrict the types that can be stored in `globalTracer`.
// This interface doesn't leak to the users. We are leveraging the type system to generate
// the functions below for `tracer.Tracer` without creating an import cycle.
type tracerLike interface {
	Flush()
	Stop()
}

// SetGlobalTracer sets the global tracer to t.
// It is the responsibility of the caller to ensure that the value is `tracer.Tracer`.
func SetGlobalTracer[T tracerLike](t T) {
	if (tracerLike)(t) == nil {
		panic("ddtrace/internal: SetGlobalTracer called with nil")
	}
	old := globalTracer.Swap(&t)
	if old == nil {
		return
	}
	oldTracer := *old.(*T)
	oldTracer.Stop()
}

// GetGlobalTracer returns the current global tracer.
// It is the responsability of the caller to ensure that calling code uses `tracer.Tracer`
// as generic type.
func GetGlobalTracer[T tracerLike]() T {
	return *globalTracer.Load().(*T)
}

// mockTracerLike is an interface to restrict the types that can be stored in `globalTracer`.
// This represents the mock tracer type used in tests. And prevent calling the StoreGlobalTracer
// function with a normal tracer.Tracer.
type mockTracerLike interface {
	tracerLike
	Reset()
}

// StoreGlobalTracer is a helper function to set the global tracer internally without stopping the old one.
// WARNING: this is used by the civisibilitymocktracer working as a wrapper around the global tracer, hence we don't stop the tracer.
// DO NOT USE THIS FUNCTION ON NORMAL tracer.Tracer.
func StoreGlobalTracer[M mockTracerLike, T tracerLike](m M) {
	if (mockTracerLike)(m) == nil {
		panic("ddtrace/internal: StoreGlobalTracer called with nil")
	}
	// convert the mock tracer like to the actual tracer like type (avoid panic on storing different types in the atomic.Value)
	t := (tracerLike)(m).(T)
	globalTracer.Store(&t)
}
