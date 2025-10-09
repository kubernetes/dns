// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// ConfKeyValue is a library-specific configuration value
type ConfKeyValue struct {
	Name   string `json:"name"`
	Value  any    `json:"value"` // can be any type of integer, float, string, or boolean
	Origin Origin `json:"origin"`
	ID     string `json:"config_id,omitempty"`
	Error  Error  `json:"error,omitempty"`

	// SeqID is used to track the total number of configuration key value pairs applied across the tracer
	SeqID uint64 `json:"seq_id,omitempty"`
}
