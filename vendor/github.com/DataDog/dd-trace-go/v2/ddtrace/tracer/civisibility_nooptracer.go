// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package tracer

import (
	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/constants"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

var _ Tracer = (*ciVisibilityNoopTracer)(nil)

// ciVisibilityNoopTracer is an implementation of Tracer that is no-op for non CiVisibility spans
// the usage of this tracer is limited to scenarios where the actual noop Tracer is used in the tests.
// For those cases we don't want to change the behaviour, so we need to act like a noop one.
// This scenario should be opt-in because with this we loose context propagation and children spans.
type ciVisibilityNoopTracer struct {
	Tracer
}

// wrapWithCiVisibilityNoopTracer creates a wrapped version of the Tracer that only accepts CiVisibility spans
func wrapWithCiVisibilityNoopTracer(tracer Tracer) *ciVisibilityNoopTracer {
	return &ciVisibilityNoopTracer{
		Tracer: tracer,
	}
}

// StartSpan implements Tracer.
func (t *ciVisibilityNoopTracer) StartSpan(operationName string, opts ...StartSpanOption) *Span {
	if opts != nil {
		cfg := NewStartSpanConfig(opts...)
		if cfg != nil && cfg.Tags != nil {
			// Let's check if the span is a CIVisibility span.
			// If yes, we create the span.
			// If not, we just behave like a noop tracer.
			if v, ok := cfg.Tags[ext.SpanType]; ok {
				if v == constants.SpanTypeTest ||
					v == constants.SpanTypeTestSuite ||
					v == constants.SpanTypeTestModule ||
					v == constants.SpanTypeTestSession {
					return t.Tracer.StartSpan(operationName, []StartSpanOption{useConfig(cfg)}...)
				}
			}
		}
	}
	log.Debug("CI Visibility tracer is behaving like a noop tracer, so the span will be skipped.")
	return nil
}

// SetServiceInfo implements Tracer.
func (t *ciVisibilityNoopTracer) SetServiceInfo(_, _, _ string) {}

// Extract implements Tracer.
func (t *ciVisibilityNoopTracer) Extract(_ interface{}) (*SpanContext, error) {
	return nil, nil
}

// Inject implements Tracer.
func (t *ciVisibilityNoopTracer) Inject(_ *SpanContext, _ interface{}) error { return nil }

// Stop implements Tracer.
func (t *ciVisibilityNoopTracer) Stop() {
	t.Tracer.Stop()
}

func (t *ciVisibilityNoopTracer) TracerConf() TracerConf {
	return t.Tracer.TracerConf()
}

func (t *ciVisibilityNoopTracer) Flush() {
	t.Tracer.Flush()
}

func useConfig(config *StartSpanConfig) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		if config == nil {
			return
		}

		cfg.Parent = config.Parent
		cfg.StartTime = config.StartTime
		cfg.Tags = config.Tags
		cfg.SpanID = config.SpanID
		cfg.Context = config.Context
		cfg.SpanLinks = config.SpanLinks
	}
}
