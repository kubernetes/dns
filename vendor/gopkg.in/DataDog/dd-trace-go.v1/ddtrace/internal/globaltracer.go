// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package internal // import "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/internal"

import (
	"sync/atomic"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
)

var (
	// globalTracer stores the current tracer as *ddtrace.Tracer (pointer to interface). The
	// atomic.Value type requires types to be consistent, which requires using *ddtrace.Tracer.
	globalTracer atomic.Value
)

func init() {
	var tracer ddtrace.Tracer = &NoopTracer{}
	globalTracer.Store(&tracer)
}

// SetGlobalTracer sets the global tracer to t.
func SetGlobalTracer(t ddtrace.Tracer) {
	old := *globalTracer.Swap(&t).(*ddtrace.Tracer)
	if !Testing {
		old.Stop()
	}
}

// GetGlobalTracer returns the currently active tracer.
func GetGlobalTracer() ddtrace.Tracer {
	return *globalTracer.Load().(*ddtrace.Tracer)
}

// Testing is set to true when the mock tracer is active. It usually signifies that we are in a test
// environment. This value is used by tracer.Start to prevent overriding the GlobalTracer in tests.
var Testing = false

var _ ddtrace.Tracer = (*NoopTracer)(nil)

// NoopTracer is an implementation of ddtrace.Tracer that is a no-op.
type NoopTracer struct{}

// StartSpan implements ddtrace.Tracer.
func (NoopTracer) StartSpan(_ string, _ ...ddtrace.StartSpanOption) ddtrace.Span {
	return NoopSpan{}
}

// SetServiceInfo implements ddtrace.Tracer.
func (NoopTracer) SetServiceInfo(_, _, _ string) {}

// Extract implements ddtrace.Tracer.
func (NoopTracer) Extract(_ interface{}) (ddtrace.SpanContext, error) {
	return NoopSpanContext{}, nil
}

// Inject implements ddtrace.Tracer.
func (NoopTracer) Inject(_ ddtrace.SpanContext, _ interface{}) error { return nil }

// Stop implements ddtrace.Tracer.
func (NoopTracer) Stop() {}

var _ ddtrace.Span = (*NoopSpan)(nil)

// NoopSpan is an implementation of ddtrace.Span that is a no-op.
type NoopSpan struct{}

// SetTag implements ddtrace.Span.
func (NoopSpan) SetTag(_ string, _ interface{}) {}

// SetOperationName implements ddtrace.Span.
func (NoopSpan) SetOperationName(_ string) {}

// BaggageItem implements ddtrace.Span.
func (NoopSpan) BaggageItem(_ string) string { return "" }

// SetBaggageItem implements ddtrace.Span.
func (NoopSpan) SetBaggageItem(_, _ string) {}

// Finish implements ddtrace.Span.
func (NoopSpan) Finish(_ ...ddtrace.FinishOption) {}

// Tracer implements ddtrace.Span.
func (NoopSpan) Tracer() ddtrace.Tracer { return NoopTracer{} }

// Context implements ddtrace.Span.
func (NoopSpan) Context() ddtrace.SpanContext { return NoopSpanContext{} }

var _ ddtrace.SpanContext = (*NoopSpanContext)(nil)

// NoopSpanContext is an implementation of ddtrace.SpanContext that is a no-op.
type NoopSpanContext struct{}

// SpanID implements ddtrace.SpanContext.
func (NoopSpanContext) SpanID() uint64 { return 0 }

// TraceID implements ddtrace.SpanContext.
func (NoopSpanContext) TraceID() uint64 { return 0 }

// ForeachBaggageItem implements ddtrace.SpanContext.
func (NoopSpanContext) ForeachBaggageItem(_ func(k, v string) bool) {}
