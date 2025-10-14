// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package transport

import (
	"encoding/json"
	"fmt"
)

// Application is identifying information about the app itself
type Application struct {
	ServiceName     string `json:"service_name"`
	Env             string `json:"env"`
	ServiceVersion  string `json:"service_version"`
	TracerVersion   string `json:"tracer_version"`
	LanguageName    string `json:"language_name"`
	LanguageVersion string `json:"language_version"`
	ProcessTags     string `json:"process_tags,omitempty"`
}

// Host is identifying information about the host on which the app
// is running
type Host struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	OSVersion     string `json:"os_version,omitempty"`
	Architecture  string `json:"architecture"`
	KernelName    string `json:"kernel_name"`
	KernelRelease string `json:"kernel_release"`
	KernelVersion string `json:"kernel_version"`
}

// Body is the common high-level structure encapsulating a telemetry request body
// Described here: https://github.com/DataDog/instrumentation-telemetry-api-docs/blob/main/GeneratedDocumentation/ApiDocs/v2/SchemaDocumentation/Request%20Bodies/telemetry_body.md
type Body struct {
	APIVersion  string      `json:"api_version"`
	RequestType RequestType `json:"request_type"`
	TracerTime  int64       `json:"tracer_time"`
	RuntimeID   string      `json:"runtime_id"`
	SeqID       int64       `json:"seq_id"`
	Debug       bool        `json:"debug,omitempty"`
	Payload     Payload     `json:"payload"`
	Application Application `json:"application"`
	Host        Host        `json:"host"`
}

// UnmarshalJSON is used to test the telemetry client end to end
func (b *Body) UnmarshalJSON(bytes []byte) error {
	var anyMap map[string]json.RawMessage
	var err error
	if err = json.Unmarshal(bytes, &anyMap); err != nil {
		return err
	}

	if err = json.Unmarshal(anyMap["api_version"], &b.APIVersion); err != nil {
		return err
	}

	if err = json.Unmarshal(anyMap["request_type"], &b.RequestType); err != nil {
		return err
	}

	if err = json.Unmarshal(anyMap["tracer_time"], &b.TracerTime); err != nil {
		return err
	}

	if err = json.Unmarshal(anyMap["runtime_id"], &b.RuntimeID); err != nil {
		return err
	}

	if err = json.Unmarshal(anyMap["seq_id"], &b.SeqID); err != nil {
		return err
	}

	if _, ok := anyMap["debug"]; ok {
		if err = json.Unmarshal(anyMap["debug"], &b.Debug); err != nil {
			return err
		}
	}

	if err = json.Unmarshal(anyMap["application"], &b.Application); err != nil {
		return err
	}

	if err = json.Unmarshal(anyMap["host"], &b.Host); err != nil {
		return err
	}

	if b.RequestType == RequestTypeMessageBatch {
		var messageBatch []struct {
			RequestType RequestType     `json:"request_type"`
			Payload     json.RawMessage `json:"payload"`
		}

		if err = json.Unmarshal(anyMap["payload"], &messageBatch); err != nil {
			return err
		}

		batch := make([]Message, len(messageBatch))
		for i, message := range messageBatch {
			payload, err := unmarshalPayload(message.Payload, message.RequestType)
			if err != nil {
				return err
			}
			batch[i] = Message{RequestType: message.RequestType, Payload: payload}
		}
		b.Payload = MessageBatch(batch)
		return nil
	}

	b.Payload, err = unmarshalPayload(anyMap["payload"], b.RequestType)
	return err
}

func unmarshalPayload(bytes json.RawMessage, requestType RequestType) (Payload, error) {
	var payload Payload
	switch requestType {
	case RequestTypeAppClientConfigurationChange:
		payload = new(AppClientConfigurationChange)
	case RequestTypeAppProductChange:
		payload = new(AppProductChange)
	case RequestTypeAppIntegrationsChange:
		payload = new(AppIntegrationChange)
	case RequestTypeAppHeartbeat:
		payload = new(AppHeartbeat)
	case RequestTypeAppStarted:
		payload = new(AppStarted)
	case RequestTypeAppClosing:
		payload = new(AppClosing)
	case RequestTypeAppExtendedHeartBeat:
		payload = new(AppExtendedHeartbeat)
	case RequestTypeAppDependenciesLoaded:
		payload = new(AppDependenciesLoaded)
	case RequestTypeDistributions:
		payload = new(Distributions)
	case RequestTypeGenerateMetrics:
		payload = new(GenerateMetrics)
	case RequestTypeLogs:
		payload = new(Logs)
	}

	if err := json.Unmarshal(bytes, payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %s", err.Error())
	}

	return payload, nil
}
