// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package tracer

import (
	"context"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/internal/llmobs"
)

var (
	_ llmobs.Tracer  = (*llmobsTracerAdapter)(nil)
	_ llmobs.APMSpan = (*llmobsSpanAdapter)(nil)
)

// llmobsTracerAdapter adapts the public ddtrace/tracer API to the internal/llmobs.Tracer interface.
// This allows the internal llmobs package to start APM spans without directly depending
// on the tracer package, avoiding circular dependencies.
type llmobsTracerAdapter struct{}

func (l *llmobsTracerAdapter) StartSpan(ctx context.Context, name string, cfg llmobs.StartAPMSpanConfig) (llmobs.APMSpan, context.Context) {
	opts := make([]StartSpanOption, 0)
	if !cfg.StartTime.IsZero() {
		opts = append(opts, StartTime(cfg.StartTime))
	}
	if cfg.SpanType != "" {
		opts = append(opts, SpanType(cfg.SpanType))
	}
	span, ctx := StartSpanFromContext(ctx, name, opts...)
	return &llmobsSpanAdapter{span}, ctx
}

// llmobsSpanAdapter adapts a public ddtrace/tracer.Span to the internal/llmobs.APMSpan interface.
type llmobsSpanAdapter struct {
	span *Span
}

func (l *llmobsSpanAdapter) Finish(cfg llmobs.FinishAPMSpanConfig) {
	opts := make([]FinishOption, 0)
	if !cfg.FinishTime.IsZero() {
		opts = append(opts, FinishTime(cfg.FinishTime))
	}
	if cfg.Error != nil {
		opts = append(opts, WithError(cfg.Error))
	}
	l.span.Finish(opts...)
}

func (l *llmobsSpanAdapter) AddLink(link llmobs.SpanLink) {
	l.span.AddLink(SpanLink{
		TraceID:     link.TraceID,
		TraceIDHigh: link.TraceIDHigh,
		SpanID:      link.SpanID,
		Attributes:  link.Attributes,
		Tracestate:  link.Tracestate,
		Flags:       link.Flags,
	})
}

func (l *llmobsSpanAdapter) SpanID() string {
	return strconv.FormatUint(l.span.Context().SpanID(), 10)
}

func (l *llmobsSpanAdapter) TraceID() string {
	return l.span.Context().TraceID()
}

func (l *llmobsSpanAdapter) SetBaggageItem(key string, value string) {
	l.span.SetBaggageItem(key, value)
}
