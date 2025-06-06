// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// All objects in this file are used to define the payload of the requests sent
// to the telemetry API.
// https://github.com/DataDog/instrumentation-telemetry-api-docs/tree/dad49961203d74ec8236b68ce4b54bbb7ed8716f/GeneratedDocumentation/ApiDocs/v2/SchemaDocumentation/Schemas

type AppHeartbeat struct{}

func (AppHeartbeat) RequestType() RequestType {
	return RequestTypeAppHeartbeat
}
