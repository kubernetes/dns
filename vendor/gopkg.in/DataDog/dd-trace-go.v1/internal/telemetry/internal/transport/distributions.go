// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// All objects in this file are used to define the payload of the requests sent
// to the telemetry API.
// https://github.com/DataDog/instrumentation-telemetry-api-docs/tree/dad49961203d74ec8236b68ce4b54bbb7ed8716f/GeneratedDocumentation/ApiDocs/v2/SchemaDocumentation/Schemas

type Distributions struct {
	Namespace     Namespace            `json:"namespace"`
	Series        []DistributionSeries `json:"series"`
	SkipAllowlist bool                 `json:"skip_allowlist,omitempty"`
}

func (Distributions) RequestType() RequestType {
	return RequestTypeDistributions
}

// DistributionSeries is a sequence of observations for a distribution metric.
// Unlike `MetricData`, DistributionSeries does not store timestamps in `Points`
type DistributionSeries struct {
	Metric string    `json:"metric"`
	Points []float64 `json:"points"`
	Tags   []string  `json:"tags,omitempty"`
	// Common distinguishes metrics which are cross-language vs.
	// language-specific.
	//
	// NOTE: If this field isn't present in the request, the API assumes
	// the metric is common. So we can't "omitempty" even though the
	// field is technically optional.
	Common    bool      `json:"common,omitempty"`
	Namespace Namespace `json:"namespace,omitempty"`
}
