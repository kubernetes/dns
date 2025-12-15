// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/dd-trace-go/v2/internal/version"
)

type SpanLink struct {
	TraceID     uint64            `json:"trace_id"`
	TraceIDHigh uint64            `json:"trace_id_high,omitempty"`
	SpanID      uint64            `json:"span_id"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Tracestate  string            `json:"tracestate,omitempty"`
	Flags       uint32            `json:"flags,omitempty"`
}

type LLMObsSpanEvent struct {
	SpanID           string             `json:"span_id,omitempty"`
	TraceID          string             `json:"trace_id,omitempty"`
	ParentID         string             `json:"parent_id,omitempty"`
	SessionID        string             `json:"session_id,omitempty"`
	Tags             []string           `json:"tags,omitempty"`
	Name             string             `json:"name,omitempty"`
	StartNS          int64              `json:"start_ns,omitempty"`
	Duration         int64              `json:"duration,omitempty"`
	Status           string             `json:"status,omitempty"`
	StatusMessage    string             `json:"status_message,omitempty"`
	Meta             map[string]any     `json:"meta,omitempty"`
	Metrics          map[string]float64 `json:"metrics,omitempty"`
	CollectionErrors []string           `json:"collection_errors,omitempty"`
	SpanLinks        []SpanLink         `json:"span_links,omitempty"`
	Scope            string             `json:"-"`
}

type PushSpanEventsRequest struct {
	Stage         string             `json:"_dd.stage,omitempty"`
	TracerVersion string             `json:"_dd.tracer_version,omitempty"`
	Scope         string             `json:"_dd.scope,omitempty"`
	EventType     string             `json:"event_type,omitempty"`
	Spans         []*LLMObsSpanEvent `json:"spans,omitempty"`
}

func (c *Transport) PushSpanEvents(
	ctx context.Context,
	events []*LLMObsSpanEvent,
) error {
	if len(events) == 0 {
		return nil
	}
	path := endpointLLMSpan
	method := http.MethodPost
	body := make([]*PushSpanEventsRequest, 0, len(events))
	for _, ev := range events {
		req := &PushSpanEventsRequest{
			Stage:         "raw",
			TracerVersion: version.Tag,
			EventType:     "span",
			Spans:         []*LLMObsSpanEvent{ev},
		}
		if ev.Scope != "" {
			req.Scope = ev.Scope
		}
		body = append(body, req)
	}

	result, err := c.jsonRequest(ctx, method, path, subdomainLLMSpan, body, defaultTimeout)
	if err != nil {
		return err
	}
	if result.statusCode != http.StatusOK && result.statusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}
	return nil
}
