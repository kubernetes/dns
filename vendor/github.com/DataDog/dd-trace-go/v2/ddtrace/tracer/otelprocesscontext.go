// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.
package tracer

// OtelProcessContext represents the OTEL context for the process.
//
//go:generate go run github.com/tinylib/msgp -unexported -marshal=true -o=otelprocesscontext_msgp.go -tests=false
type otelProcessContext struct {
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/deployment/#deployment-environment-name
	DeploymentEnvironmentName string `msg:"deployment.environment.name"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/host/#host-name
	HostName string `msg:"host.name"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/service/#service-instance-id
	ServiceInstanceID string `msg:"service.instance.id"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/service/#service-name
	ServiceName string `msg:"service.name"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/service/#service-version
	ServiceVersion string `msg:"service.version"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/telemetry/#telemetry-sdk-language
	TelemetrySDKLanguage string `msg:"telemetry.sdk.language"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/telemetry/#telemetry-sdk-version
	TelemetrySDKVersion string `msg:"telemetry.sdk.version"`
	// https://opentelemetry.io/docs/specs/semconv/registry/attributes/telemetry/#telemetry-sdk-name
	TelemetrySdkName string `msg:"telemetry.sdk.name"`
}
