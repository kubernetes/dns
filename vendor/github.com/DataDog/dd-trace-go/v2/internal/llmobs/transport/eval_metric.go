// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

import (
	"context"
	"fmt"
	"net/http"
)

// EvaluationJoinOn represents how to join evaluation metrics to spans.
// Exactly one of Span or Tag should be provided.
type EvaluationJoinOn struct {
	// Span contains span and trace IDs for direct span joining.
	Span *EvaluationSpanJoin `json:"span,omitempty"`
	// Tag contains tag key-value for tag-based joining.
	Tag *EvaluationTagJoin `json:"tag,omitempty"`
}

// EvaluationSpanJoin represents joining by span and trace ID.
type EvaluationSpanJoin struct {
	// SpanID is the span ID to join on.
	SpanID string `json:"span_id"`
	// TraceID is the trace ID to join on.
	TraceID string `json:"trace_id"`
}

// EvaluationTagJoin represents joining by tag key-value pairs.
type EvaluationTagJoin struct {
	// Key is the tag key to search for.
	Key string `json:"key"`
	// Value is the tag value to match.
	Value string `json:"value"`
}

// LLMObsMetric represents an evaluation metric for LLMObs spans.
type LLMObsMetric struct {
	JoinOn           EvaluationJoinOn `json:"join_on"`
	MetricType       string           `json:"metric_type,omitempty"`
	Label            string           `json:"label,omitempty"`
	CategoricalValue *string          `json:"categorical_value,omitempty"`
	ScoreValue       *float64         `json:"score_value,omitempty"`
	BooleanValue     *bool            `json:"boolean_value,omitempty"`
	MLApp            string           `json:"ml_app,omitempty"`
	TimestampMS      int64            `json:"timestamp_ms,omitempty"`
	Tags             []string         `json:"tags,omitempty"`
}

type PushMetricsRequest struct {
	Data PushMetricsRequestData `json:"data"`
}

type PushMetricsRequestData struct {
	Type       string                           `json:"type"`
	Attributes PushMetricsRequestDataAttributes `json:"attributes"`
}

type PushMetricsRequestDataAttributes struct {
	Metrics []*LLMObsMetric `json:"metrics"`
}

func (c *Transport) PushEvalMetrics(
	ctx context.Context,
	metrics []*LLMObsMetric,
) error {
	if len(metrics) == 0 {
		return nil
	}
	path := endpointEvalMetric
	method := http.MethodPost
	body := &PushMetricsRequest{
		Data: PushMetricsRequestData{
			Type: "evaluation_metric",
			Attributes: PushMetricsRequestDataAttributes{
				Metrics: metrics,
			},
		},
	}

	result, err := c.jsonRequest(ctx, method, path, subdomainEvalMetric, body, defaultTimeout)
	if err != nil {
		return err
	}
	if result.statusCode != http.StatusOK && result.statusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status %d: %s", result.statusCode, string(result.body))
	}
	return nil
}
