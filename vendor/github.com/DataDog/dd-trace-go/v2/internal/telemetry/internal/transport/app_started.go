// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// All objects in this file are used to define the payload of the requests sent
// to the telemetry API.
// https://github.com/DataDog/instrumentation-telemetry-api-docs/tree/dad49961203d74ec8236b68ce4b54bbb7ed8716f/GeneratedDocumentation/ApiDocs/v2/SchemaDocumentation/Schemas

type AppStarted struct {
	Products          Products            `json:"products,omitempty"`
	Configuration     []ConfKeyValue      `json:"configuration,omitempty"`
	Error             Error               `json:"error,omitempty"`
	InstallSignature  InstallSignature    `json:"install_signature,omitempty"`
	AdditionalPayload []AdditionalPayload `json:"additional_payload,omitempty"`
}

func (AppStarted) RequestType() RequestType {
	return RequestTypeAppStarted
}

// InstallSignature is a structure to send the install signature with the
// AppStarted payload.
type InstallSignature struct {
	InstallID   string `json:"install_id,omitempty"`
	InstallType string `json:"install_type,omitempty"`
	InstallTime string `json:"install_time,omitempty"`
}

// AdditionalPayload is a generic structure to send additional data with the
// AppStarted payload.
type AdditionalPayload struct {
	Name  RequestType `json:"name"`
	Value Payload     `json:"value"`
}
