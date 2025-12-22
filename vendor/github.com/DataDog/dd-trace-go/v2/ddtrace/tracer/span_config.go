// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"context"
	"time"
)

// StartSpanOption is a configuration option that can be used with a Tracer's StartSpan method.
type StartSpanOption func(cfg *StartSpanConfig)

// StartSpanConfig holds the configuration for starting a new span. It is usually passed
// around by reference to one or more StartSpanOption functions which shape it into its
// final form.
type StartSpanConfig struct {
	// Parent holds the SpanContext that should be used as a parent for the
	// new span. If nil, implementations should return a root span.
	Parent *SpanContext

	// StartTime holds the time that should be used as the start time of the span.
	// Implementations should use the current time when StartTime.IsZero().
	StartTime time.Time

	// Tags holds a set of key/value pairs that should be set as metadata on the
	// new span.
	Tags map[string]interface{}

	// SpanID will be the SpanID of the Span, overriding the random number that would
	// be generated. If no Parent SpanContext is present, then this will also set the
	// TraceID to the same value.
	SpanID uint64

	// Context is the parent context where the span should be stored.
	Context context.Context

	// SpanLink represents a causal relationship between two spans. A span can have multiple links.
	SpanLinks []SpanLink
}

// NewStartSpanConfig allows to build a base config struct. It accepts the same options as StartSpan.
// It's useful to reduce the number of operations in any hot path and update it for request/operation specifics.
func NewStartSpanConfig(opts ...StartSpanOption) *StartSpanConfig {
	cfg := new(StartSpanConfig)
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

// FinishOption is a configuration option that can be used with a Span's Finish method.
type FinishOption func(cfg *FinishConfig)

// FinishConfig holds the configuration for finishing a span. It is usually passed around by
// reference to one or more FinishOption functions which shape it into its final form.
type FinishConfig struct {
	// FinishTime represents the time that should be set as finishing time for the
	// span. Implementations should use the current time when FinishTime.IsZero().
	FinishTime time.Time

	// Error holds an optional error that should be set on the span before
	// finishing.
	Error error

	// NoDebugStack will prevent any set errors from generating an attached stack trace tag.
	NoDebugStack bool

	// StackFrames specifies the number of stack frames to be attached in spans that finish with errors.
	StackFrames uint

	// SkipStackFrames specifies the offset at which to start reporting stack frames from the stack.
	SkipStackFrames uint
}

// NewFinishConfig allows building a base finish config struct. It accepts the same options as Finish.
// It's useful to reduce the number of operations in any hot path and update it for request/operation specifics.
func NewFinishConfig(opts ...FinishOption) *FinishConfig {
	cfg := new(FinishConfig)
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

// FinishTime sets the given time as the finishing time for the span. By default,
// the current time is used.
func FinishTime(t time.Time) FinishOption {
	return func(cfg *FinishConfig) {
		cfg.FinishTime = t
	}
}

// WithError marks the span as having had an error. It uses the information from
// err to set tags such as the error message, error type and stack trace. It has
// no effect if the error is nil.
func WithError(err error) FinishOption {
	return func(cfg *FinishConfig) {
		cfg.Error = err
	}
}

// NoDebugStack prevents any error presented using the WithError finishing option
// from generating a stack trace. This is useful in situations where errors are frequent
// and performance is critical.
func NoDebugStack() FinishOption {
	return func(cfg *FinishConfig) {
		cfg.NoDebugStack = true
	}
}

// StackFrames limits the number of stack frames included into erroneous spans to n, starting from skip.
func StackFrames(n, skip uint) FinishOption {
	if n == 0 {
		return NoDebugStack()
	}
	return func(cfg *FinishConfig) {
		cfg.StackFrames = n
		cfg.SkipStackFrames = skip
	}
}

// WithFinishConfig merges the given FinishConfig into the one used to finish the span.
// It is useful when you want to set a common base finish config, reducing the number of function calls in hot loops.
func WithFinishConfig(cfg *FinishConfig) FinishOption {
	return func(fc *FinishConfig) {
		fc.Error = cfg.Error
		if fc.FinishTime.IsZero() {
			fc.FinishTime = cfg.FinishTime
		}
		if !fc.NoDebugStack {
			fc.NoDebugStack = cfg.NoDebugStack
		}
		if fc.SkipStackFrames == 0 {
			fc.SkipStackFrames = cfg.SkipStackFrames
		}
		if fc.StackFrames == 0 {
			fc.StackFrames = cfg.StackFrames
		}
	}
}
