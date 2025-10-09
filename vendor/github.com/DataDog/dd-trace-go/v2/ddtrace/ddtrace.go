// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package ddtrace contains the interfaces that specify the implementations of Datadog's
// tracing library, as well as a set of sub-packages containing various implementations:
// our native implementation ("tracer") and a mock tracer to be used for testing ("mocktracer").
// Additionally, package "ext" provides a set of tag names and values specific to Datadog's APM product.
//
// To get started, visit the documentation for any of the packages you'd like to begin
// with by accessing the subdirectories of this package: https://pkg.go.dev/github.com/DataDog/dd-trace-go/v2/ddtrace#pkg-subdirectories.
package ddtrace // import "github.com/DataDog/dd-trace-go/v2/ddtrace"

// SpanContext represents a span state that can propagate to descendant spans
// and across process boundaries. It contains all the information needed to
// spawn a direct descendant of the span that it belongs to. It can be used
// to create distributed tracing by propagating it using the provided interfaces.
type SpanContext interface {
	// SpanID returns the span ID that this context is carrying.
	SpanID() uint64

	// TraceID returns the trace ID that this context is carrying.
	TraceID() string

	// TraceID128 returns the raw bytes of the 128-bit trace ID that this context is carrying.
	TraceIDBytes() [16]byte

	// TraceIDLower returns the lower part of the trace ID that this context is carrying.
	TraceIDLower() uint64

	// ForeachBaggageItem provides an iterator over the key/value pairs set as
	// baggage within this context. Iteration stops when the handler returns
	// false.
	ForeachBaggageItem(handler func(k, v string) bool)
}
