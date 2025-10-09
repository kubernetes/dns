// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import "github.com/DataDog/dd-trace-go/v2/internal/log"

var _ Tracer = (*NoopTracer)(nil)

// NoopTracer is an implementation of Tracer that is a no-op.
type NoopTracer struct{}

// StartSpan implements Tracer.
func (NoopTracer) StartSpan(_ string, _ ...StartSpanOption) *Span {
	log.Debug("Tracer must be started before starting a span; Review the docs for more information: https://docs.datadoghq.com/tracing/trace_collection/library_config/go/")
	return nil
}

// SetServiceInfo implements Tracer.
func (NoopTracer) SetServiceInfo(_, _, _ string) {}

// Extract implements Tracer.
func (NoopTracer) Extract(_ interface{}) (*SpanContext, error) {
	return nil, nil
}

// Inject implements Tracer.
func (NoopTracer) Inject(_ *SpanContext, _ interface{}) error { return nil }

// Stop implements Tracer.
func (NoopTracer) Stop() {}

func (NoopTracer) TracerConf() TracerConf {
	return TracerConf{}
}

func (NoopTracer) Flush() {}
