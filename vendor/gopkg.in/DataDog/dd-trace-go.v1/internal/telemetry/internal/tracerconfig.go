// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

// TracerConfig is the configuration for the tracer for the telemetry client.
type TracerConfig struct {
	// Service is the name of the service being traced.
	Service string
	// Env is the environment the service is running in.
	Env string
	// Version is the version of the service.
	Version string
}
