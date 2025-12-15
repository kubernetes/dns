// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package llmobs

import "context"

type (
	ctxKeyActiveLLMSpan     struct{}
	ctxKeyPropagatedLLMSpan struct{}
)

// PropagatedLLMSpan represents LLMObs span context that can be propagated across process boundaries.
type PropagatedLLMSpan struct {
	// MLApp is the ML application name.
	MLApp string
	// TraceID is the LLMObs trace ID.
	TraceID string
	// SpanID is the span ID.
	SpanID string
}

// PropagatedLLMSpanFromContext retrieves a PropagatedLLMSpan from the context.
// Returns the span and true if found, nil and false otherwise.
func PropagatedLLMSpanFromContext(ctx context.Context) (*PropagatedLLMSpan, bool) {
	if val, ok := ctx.Value(ctxKeyPropagatedLLMSpan{}).(*PropagatedLLMSpan); ok {
		return val, true
	}
	return nil, false
}

// ContextWithPropagatedLLMSpan returns a new context with the given PropagatedLLMSpan attached.
func ContextWithPropagatedLLMSpan(ctx context.Context, span *PropagatedLLMSpan) context.Context {
	return context.WithValue(ctx, ctxKeyPropagatedLLMSpan{}, span)
}

// ActiveLLMSpanFromContext retrieves the active LLMObs span from the context.
// Returns the span and true if found, nil and false otherwise.
func ActiveLLMSpanFromContext(ctx context.Context) (*Span, bool) {
	if span, ok := ctx.Value(ctxKeyActiveLLMSpan{}).(*Span); ok {
		return span, true
	}
	return nil, false
}

func contextWithActiveLLMSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, ctxKeyActiveLLMSpan{}, span)
}
