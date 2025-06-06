// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// Payload is the interface implemented by all telemetry top-level structures, AKA payloads.
// All structs are a strict representation of what is described in Instrumentation Telemetry v2 documentation schemas:
// https://github.com/DataDog/instrumentation-telemetry-api-docs/tree/dad49961203d74ec8236b68ce4b54bbb7ed8716f/GeneratedDocumentation/ApiDocs/v2/SchemaDocumentation/Schemas
type Payload interface {
	RequestType() RequestType
}
