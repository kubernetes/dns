// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tracer

import (
	"sync"
	"sync/atomic"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/internal"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

var _ ddtrace.SpanContext = (*spanContext)(nil)

// SpanContext represents a span state that can propagate to descendant spans
// and across process boundaries. It contains all the information needed to
// spawn a direct descendant of the span that it belongs to. It can be used
// to create distributed tracing by propagating it using the provided interfaces.
type spanContext struct {
	// the below group should propagate only locally

	trace *trace // reference to the trace that this span belongs too
	span  *span  // reference to the span that hosts this context
	drop  bool   // when true, the span will not be sent to the agent

	// the below group should propagate cross-process

	traceID uint64
	spanID  uint64

	mu         sync.RWMutex // guards below fields
	baggage    map[string]string
	hasBaggage int32  // atomic int for quick checking presence of baggage. 0 indicates no baggage, otherwise baggage exists.
	origin     string // e.g. "synthetics"
}

// newSpanContext creates a new SpanContext to serve as context for the given
// span. If the provided parent is not nil, the context will inherit the trace,
// baggage and other values from it. This method also pushes the span into the
// new context's trace and as a result, it should not be called multiple times
// for the same span.
func newSpanContext(span *span, parent *spanContext) *spanContext {
	context := &spanContext{
		traceID: span.TraceID,
		spanID:  span.SpanID,
		span:    span,
	}
	if parent != nil {
		context.trace = parent.trace
		context.drop = parent.drop
		context.origin = parent.origin
		parent.ForeachBaggageItem(func(k, v string) bool {
			context.setBaggageItem(k, v)
			return true
		})
	}
	if context.trace == nil {
		context.trace = newTrace()
	}
	if context.trace.root == nil {
		// first span in the trace can safely be assumed to be the root
		context.trace.root = span
	}
	// put span in context's trace
	context.trace.push(span)
	return context
}

// SpanID implements ddtrace.SpanContext.
func (c *spanContext) SpanID() uint64 { return c.spanID }

// TraceID implements ddtrace.SpanContext.
func (c *spanContext) TraceID() uint64 { return c.traceID }

// ForeachBaggageItem implements ddtrace.SpanContext.
func (c *spanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	if atomic.LoadInt32(&c.hasBaggage) == 0 {
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k, v := range c.baggage {
		if !handler(k, v) {
			break
		}
	}
}

func (c *spanContext) setSamplingPriority(p int) {
	if c.trace == nil {
		c.trace = newTrace()
	}
	c.trace.setSamplingPriority(float64(p))
}

func (c *spanContext) samplingPriority() (p int, ok bool) {
	if c.trace == nil {
		return 0, false
	}
	return c.trace.samplingPriority()
}

func (c *spanContext) setBaggageItem(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baggage == nil {
		atomic.StoreInt32(&c.hasBaggage, 1)
		c.baggage = make(map[string]string, 1)
	}
	c.baggage[key] = val
}

func (c *spanContext) baggageItem(key string) string {
	if atomic.LoadInt32(&c.hasBaggage) == 0 {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baggage[key]
}

// finish marks this span as finished in the trace.
func (c *spanContext) finish() { c.trace.finishedOne(c.span) }

// trace contains shared context information about a trace, such as sampling
// priority, the root reference and a buffer of the spans which are part of the
// trace, if these exist.
type trace struct {
	mu       sync.RWMutex // guards below fields
	spans    []*span      // all the spans that are part of this trace
	finished int          // the number of finished spans
	full     bool         // signifies that the span buffer is full
	priority *float64     // sampling priority
	locked   bool         // specifies if the sampling priority can be altered

	// root specifies the root of the trace, if known; it is nil when a span
	// context is extracted from a carrier, at which point there are no spans in
	// the trace yet.
	root *span
}

var (
	// traceStartSize is the initial size of our trace buffer,
	// by default we allocate for a handful of spans within the trace,
	// reasonable as span is actually way bigger, and avoids re-allocating
	// over and over. Could be fine-tuned at runtime.
	traceStartSize = 10
	// traceMaxSize is the maximum number of spans we keep in memory.
	// This is to avoid memory leaks, if above that value, spans are randomly
	// dropped and ignore, resulting in corrupted tracing data, but ensuring
	// original program continues to work as expected.
	traceMaxSize = int(1e5)
)

// newTrace creates a new trace using the given callback which will be called
// upon completion of the trace.
func newTrace() *trace {
	return &trace{spans: make([]*span, 0, traceStartSize)}
}

func (t *trace) samplingPriority() (p int, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.priority == nil {
		return 0, false
	}
	return int(*t.priority), true
}

func (t *trace) setSamplingPriority(p float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setSamplingPriorityLocked(p)
}

func (t *trace) setSamplingPriorityLocked(p float64) {
	if t.locked {
		return
	}
	if t.root == nil {
		// this trace is distributed (no local root); modifications
		// to the sampling priority are not allowed.
		t.locked = true
	}
	if t.priority == nil {
		t.priority = new(float64)
	}
	*t.priority = p
}

// push pushes a new span into the trace. If the buffer is full, it returns
// a errBufferFull error.
func (t *trace) push(sp *span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.full {
		return
	}
	tr, haveTracer := internal.GetGlobalTracer().(*tracer)
	if len(t.spans) >= traceMaxSize {
		// capacity is reached, we will not be able to complete this trace.
		t.full = true
		t.spans = nil // GC
		log.Error("trace buffer full (%d), dropping trace", traceMaxSize)
		if haveTracer {
			atomic.AddInt64(&tr.tracesDropped, 1)
		}
		return
	}
	if v, ok := sp.Metrics[keySamplingPriority]; ok {
		t.setSamplingPriorityLocked(v)
	}
	t.spans = append(t.spans, sp)
	if haveTracer {
		atomic.AddInt64(&tr.spansStarted, 1)
	}
}

// finishedOne aknowledges that another span in the trace has finished, and checks
// if the trace is complete, in which case it calls the onFinish function. It uses
// the given priority, if non-nil, to mark the root span.
func (t *trace) finishedOne(s *span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.full {
		// capacity has been reached, the buffer is no longer tracking
		// all the spans in the trace, so the below conditions will not
		// be accurate and would trigger a pre-mature flush, exposing us
		// to a race condition where spans can be modified while flushing.
		return
	}
	t.finished++
	if s == t.root && t.priority != nil {
		// after the root has finished we lock down the priority;
		// we won't be able to make changes to a span after finishing
		// without causing a race condition.
		t.root.setMetric(keySamplingPriority, *t.priority)
		t.locked = true
	}
	if len(t.spans) != t.finished {
		return
	}
	if tr, ok := internal.GetGlobalTracer().(*tracer); ok {
		// we have a tracer that can receive completed traces.
		tr.pushTrace(t.spans)
		atomic.AddInt64(&tr.spansFinished, int64(len(t.spans)))
	}
	t.spans = nil
	t.finished = 0 // important, because a buffer can be used for several flushes
}
