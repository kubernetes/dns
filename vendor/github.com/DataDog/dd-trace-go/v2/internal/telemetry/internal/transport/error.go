// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// Error stores error information about various tracer events
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
