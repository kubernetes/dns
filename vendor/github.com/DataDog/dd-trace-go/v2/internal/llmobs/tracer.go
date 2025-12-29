// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package llmobs

import (
	"context"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/llmobs/transport"
)

// Tracer represents the interface for the underlying APM tracer.
type Tracer interface {
	// StartSpan starts a new APM span with the given name and configuration.
	StartSpan(ctx context.Context, name string, cfg StartAPMSpanConfig) (APMSpan, context.Context)
}

// StartAPMSpanConfig contains configuration options for starting an APM span.
type StartAPMSpanConfig struct {
	// SpanType is the type of the APM span.
	SpanType string
	// StartTime is the start time for the span.
	StartTime time.Time
}

// FinishAPMSpanConfig contains configuration options for finishing an APM span.
type FinishAPMSpanConfig struct {
	// FinishTime is the finish time for the span.
	FinishTime time.Time
	// Error is an error to set on the span when finishing.
	Error error
}

// APMSpan represents the interface for an APM span.
type APMSpan interface {
	// Finish finishes the span with the given configuration.
	Finish(cfg FinishAPMSpanConfig)
	// AddLink adds a span link to this span.
	AddLink(link SpanLink)
	// SpanID returns the span ID.
	SpanID() string
	// TraceID returns the trace ID.
	TraceID() string
	// SetBaggageItem sets a baggage item on the span.
	SetBaggageItem(key string, value string)
}

// SpanLink represents a link between spans, aliased from the transport package.
type SpanLink = transport.SpanLink
