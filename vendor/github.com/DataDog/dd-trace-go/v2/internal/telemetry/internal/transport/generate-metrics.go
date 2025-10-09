// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// All objects in this file are used to define the payload of the requests sent
// to the telemetry API.
// https://github.com/DataDog/instrumentation-telemetry-api-docs/tree/dad49961203d74ec8236b68ce4b54bbb7ed8716f/GeneratedDocumentation/ApiDocs/v2/SchemaDocumentation/Schemas

type GenerateMetrics struct {
	Namespace     Namespace    `json:"namespace,omitempty"`
	Series        []MetricData `json:"series"`
	SkipAllowlist bool         `json:"skip_allowlist,omitempty"`
}

func (GenerateMetrics) RequestType() RequestType {
	return RequestTypeGenerateMetrics
}

// MetricType is the type of metric being sent, either count, gauge, or rate
// distribution is another payload altogether
type MetricType string

const (
	RateMetric  MetricType = "rate"
	CountMetric MetricType = "count"
	GaugeMetric MetricType = "gauge"
	DistMetric  MetricType = "distribution"
)

// MetricData is a sequence of observations for a single named metric.
type MetricData struct {
	Metric string `json:"metric"`
	// Points stores pairs of timestamps and values
	// This first value should be an int64 timestamp and the second should be a float64 value
	Points [][2]any `json:"points"`
	// Interval is required only for gauge and rate metrics
	Interval int64 `json:"interval,omitempty"`
	// Type cannot be of type distribution because there is a different payload for it
	Type MetricType `json:"type,omitempty"`
	Tags []string   `json:"tags,omitempty"`

	// Common distinguishes metrics which are cross-language vs.
	// language-specific.
	//
	// NOTE: If this field isn't present in the request, the API assumes
	// the metric is common. So we can't "omitempty" even though the
	// field is technically optional.
	Common    bool      `json:"common"`
	Namespace Namespace `json:"namespace,omitempty"`
}
