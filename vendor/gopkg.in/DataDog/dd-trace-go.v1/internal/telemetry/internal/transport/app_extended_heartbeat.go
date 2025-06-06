// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

type AppExtendedHeartbeat struct {
	Configuration []ConfKeyValue `json:"configuration,omitempty"`
	Dependencies  []Dependency   `json:"dependencies,omitempty"`
	Integrations  []Integration  `json:"integrations,omitempty"`
}

func (AppExtendedHeartbeat) RequestType() RequestType {
	return RequestTypeAppExtendedHeartBeat
}
