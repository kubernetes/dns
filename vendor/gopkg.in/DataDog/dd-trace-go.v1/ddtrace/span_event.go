// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package ddtrace

import (
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

// SpanWithEvents represents a Span that can include span events.
type SpanWithEvents interface {
	Span

	// AddEvent adds the given event to the span.
	AddEvent(name string, opts ...SpanEventOption)
}

// SpanEventConfig represent the configuration of a span event.
type SpanEventConfig struct {
	// Time is the time when the event happened.
	Time time.Time

	// Attributes is a map of string to attribute.
	// Only the following types are supported:
	//   string, integer (any), boolean, float (any), []string, []integer (any), []boolean, []float (any)
	Attributes map[string]any
}

// AddSpanEvent attaches a new event to the given span.
func AddSpanEvent(span Span, name string, opts ...SpanEventOption) {
	withEvents, ok := span.(SpanWithEvents)
	if !ok {
		log.Debug("failed to add span event to the given span (unsupported span type: %T)", span)
		return
	}
	withEvents.AddEvent(name, opts...)
}

// SpanEventOption can be used to customize an event created with NewSpanEvent.
type SpanEventOption func(cfg *SpanEventConfig)

// WithSpanEventTimestamp sets the time when the span event occurred.
func WithSpanEventTimestamp(tStamp time.Time) SpanEventOption {
	return func(cfg *SpanEventConfig) {
		cfg.Time = tStamp
	}
}

// WithSpanEventAttributes sets the given attributes for the span event.
func WithSpanEventAttributes(attributes map[string]any) SpanEventOption {
	return func(cfg *SpanEventConfig) {
		cfg.Attributes = attributes
	}
}
