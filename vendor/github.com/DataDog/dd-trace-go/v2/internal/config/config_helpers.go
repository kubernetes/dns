// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import "github.com/DataDog/dd-trace-go/v2/internal/log"

const (
	// DefaultRateLimit specifies the default rate limit per second for traces.
	// TODO: Maybe delete this. We will have defaults in supported_configuration.json anyway.
	DefaultRateLimit = 100.0
	// traceMaxSize is the maximum number of spans we keep in memory for a
	// single trace. This is to avoid memory leaks. If more spans than this
	// are added to a trace, then the trace is dropped and the spans are
	// discarded. Adding additional spans after a trace is dropped does
	// nothing.
	TraceMaxSize = int(1e5)
)

func validateSampleRate(rate float64) bool {
	if rate < 0.0 || rate > 1.0 {
		log.Warn("ignoring DD_TRACE_SAMPLE_RATE: out of range %f", rate)
		return false
	}
	return true
}

func validateRateLimit(rate float64) bool {
	if rate < 0.0 {
		log.Warn("ignoring DD_TRACE_RATE_LIMIT: negative value %f", rate)
		return false
	}
	return true
}

func validatePartialFlushMinSpans(minSpans int) bool {
	if minSpans <= 0 {
		log.Warn("ignoring DD_TRACE_PARTIAL_FLUSH_MIN_SPANS: negative value %d", minSpans)
		return false
	}
	if minSpans >= TraceMaxSize {
		log.Warn("ignoring DD_TRACE_PARTIAL_FLUSH_MIN_SPANS: value %d is greater than the max number of spans that can be kept in memory for a single trace (%d spans)", minSpans, TraceMaxSize)
		return false
	}
	return true
}
