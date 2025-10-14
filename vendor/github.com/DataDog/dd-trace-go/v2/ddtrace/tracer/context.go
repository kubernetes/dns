// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"context"

	"github.com/DataDog/dd-trace-go/v2/instrumentation/options"
	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/orchestrion"
)

// ContextWithSpan returns a copy of the given context which includes the span s.
func ContextWithSpan(ctx context.Context, s *Span) context.Context {
	return orchestrion.CtxWithValue(ctx, internal.ActiveSpanKey, s)
}

// SpanFromContext returns the span contained in the given context. A second return
// value indicates if a span was found in the context. If no span is found, a no-op
// span is returned.
func SpanFromContext(ctx context.Context) (*Span, bool) {
	if ctx == nil {
		return nil, false
	}
	v := orchestrion.WrapContext(ctx).Value(internal.ActiveSpanKey)
	if s, ok := v.(*Span); ok {
		// We may have a nil *Span wrapped in an interface in the GLS context stack,
		// in which case we need to act a if there was nothing (for else we'll
		// forcefully un-do a [ChildOf] option if one was passed).
		return s, s != nil
	}
	return nil, false
}

// StartSpanFromContext returns a new span with the given operation name and options. If a span
// is found in the context, it will be used as the parent of the resulting span. If the ChildOf
// option is passed, it will only be used as the parent if there is no span found in `ctx`.
func StartSpanFromContext(ctx context.Context, operationName string, opts ...StartSpanOption) (*Span, context.Context) {
	// copy opts in case the caller reuses the slice in parallel
	// we will add at least 1, at most 2 items
	optsLocal := options.Expand(opts, 0, 2)
	if ctx == nil {
		// default to context.Background() to avoid panics on Go >= 1.15
		ctx = context.Background()
	} else if s, ok := SpanFromContext(ctx); ok {
		optsLocal = append(optsLocal, ChildOf(s.Context()))
	}
	optsLocal = append(optsLocal, withContext(ctx))
	s := StartSpan(operationName, optsLocal...)
	if s != nil && s.pprofCtxActive != nil {
		ctx = s.pprofCtxActive
	}
	return s, ContextWithSpan(ctx, s)
}
