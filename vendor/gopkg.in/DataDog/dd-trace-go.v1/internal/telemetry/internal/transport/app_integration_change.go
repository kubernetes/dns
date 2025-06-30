// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

type AppIntegrationChange struct {
	Integrations []Integration `json:"integrations"`
}

func (AppIntegrationChange) RequestType() RequestType {
	return RequestTypeAppIntegrationsChange
}

// Integration is an integration that is configured to be traced automatically.
type Integration struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Version     string `json:"version,omitempty"`
	AutoEnabled bool   `json:"auto_enabled,omitempty"`
	Compatible  bool   `json:"compatible,omitempty"`
	Error       string `json:"error,omitempty"`
}
