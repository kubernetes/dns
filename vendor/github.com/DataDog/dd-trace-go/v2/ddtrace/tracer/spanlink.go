// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

// SpanLink represents a reference to a span that exists outside of the trace.
//
//go:generate go run github.com/tinylib/msgp -unexported -marshal=false -o=span_link_msgp.go -tests=false

type SpanLink struct {
	// TraceID represents the low 64 bits of the linked span's trace id. This field is required.
	TraceID uint64 `msg:"trace_id" json:"trace_id"`
	// TraceIDHigh represents the high 64 bits of the linked span's trace id. This field is only set if the linked span's trace id is 128 bits.
	TraceIDHigh uint64 `msg:"trace_id_high,omitempty" json:"trace_id_high"`
	// SpanID represents the linked span's span id.
	SpanID uint64 `msg:"span_id" json:"span_id"`
	// Attributes is a mapping of keys to string values. These values are used to add additional context to the span link.
	Attributes map[string]string `msg:"attributes,omitempty" json:"attributes"`
	// Tracestate is the tracestate of the linked span. This field is optional.
	Tracestate string `msg:"tracestate,omitempty" json:"tracestate"`
	// Flags represents the W3C trace flags of the linked span. This field is optional.
	Flags uint32 `msg:"flags,omitempty" json:"flags"`
}
