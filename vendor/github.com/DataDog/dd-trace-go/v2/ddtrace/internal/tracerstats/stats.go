// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracerstats

import "sync/atomic"

// Events are things that happen in the tracer such as a trace being dropped or
// a span being started. These are counted and submitted as metrics.
type Event int

const (
	SpanStarted Event = iota
	SpansFinished
	TracesDropped
	DroppedP0Traces
	DroppedP0Spans
	PartialTraces

	// Read-only. We duplicate some of the stats so that we can send them to the
	// agent in headers as well as counting them with statsd.
	AgentDroppedP0Traces
	AgentDroppedP0Spans
)

// These integers track metrics about spans and traces as they are started,
// finished, and dropped
var spansStarted, spansFinished, tracesDropped uint32

// Records the number of dropped P0 traces and spans.
var droppedP0Traces, droppedP0Spans uint32

// partialTrace the number of partially dropped traces.
var partialTraces uint32

// Copies of the stats to be sent to the agent.
var agentDroppedP0Traces, agentDroppedP0Spans uint32

func Signal(e Event, count uint32) {
	switch e {
	case SpanStarted:
		atomic.AddUint32(&spansStarted, count)
	case SpansFinished:
		atomic.AddUint32(&spansFinished, count)
	case TracesDropped:
		atomic.AddUint32(&tracesDropped, count)
	case DroppedP0Traces:
		atomic.AddUint32(&droppedP0Traces, count)
		atomic.AddUint32(&agentDroppedP0Traces, count)
	case DroppedP0Spans:
		atomic.AddUint32(&droppedP0Spans, count)
		atomic.AddUint32(&agentDroppedP0Spans, count)
	case PartialTraces:
		atomic.AddUint32(&partialTraces, count)
	}
}

func Count(e Event) uint32 {
	switch e {
	case SpanStarted:
		return atomic.SwapUint32(&spansStarted, 0)
	case SpansFinished:
		return atomic.SwapUint32(&spansFinished, 0)
	case TracesDropped:
		return atomic.SwapUint32(&tracesDropped, 0)
	case DroppedP0Traces:
		return atomic.SwapUint32(&droppedP0Traces, 0)
	case DroppedP0Spans:
		return atomic.SwapUint32(&droppedP0Spans, 0)
	case PartialTraces:
		return atomic.SwapUint32(&partialTraces, 0)
	case AgentDroppedP0Traces:
		return atomic.SwapUint32(&agentDroppedP0Traces, 0)
	case AgentDroppedP0Spans:
		return atomic.SwapUint32(&agentDroppedP0Spans, 0)
	}
	return 0
}

func Reset() {
	atomic.StoreUint32(&spansStarted, 0)
	atomic.StoreUint32(&spansFinished, 0)
	atomic.StoreUint32(&tracesDropped, 0)
	atomic.StoreUint32(&droppedP0Traces, 0)
	atomic.StoreUint32(&droppedP0Spans, 0)
	atomic.StoreUint32(&partialTraces, 0)
	atomic.StoreUint32(&agentDroppedP0Traces, 0)
	atomic.StoreUint32(&agentDroppedP0Spans, 0)
}
