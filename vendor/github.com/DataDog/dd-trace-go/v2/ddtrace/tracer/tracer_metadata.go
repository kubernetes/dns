// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.
package tracer

// Metadata represents the configuration of the tracer.
//
//go:generate go run github.com/tinylib/msgp -unexported -marshal=true -o=tracer_metadata_msgp.go -tests=false
type Metadata struct {
	// Version of the schema.
	SchemaVersion uint8 `msg:"schema_version"`
	// Runtime UUID.
	RuntimeID string `msg:"runtime_id"`
	// Programming language of the tracer.
	Language string `msg:"tracer_language"`
	// Version of the tracer
	Version string `msg:"tracer_version"`
	// Identfier of the machine running the process.
	Hostname string `msg:"hostname"`
	// Name of the service being instrumented.
	ServiceName string `msg:"service_name"`
	// Environment of the service being instrumented.
	ServiceEnvironment string `msg:"service_env"`
	// Version of the service being instrumented.
	ServiceVersion string `msg:"service_version"`
	// ProcessTags describe the process
	ProcessTags string `msg:"process_tags"`
	// ContainerID identified by the process.
	ContainerID string `msg:"container_id"`
}
