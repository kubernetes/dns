// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package mapper

import (
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// NewDefaultMapper returns a Mapper that transforms payloads into a MessageBatch and adds a heartbeat message.
// The heartbeat message is added every heartbeatInterval.
func NewDefaultMapper(heartbeatInterval, extendedHeartBeatInterval time.Duration) Mapper {
	mapper := &defaultMapper{
		heartbeatEnricher: heartbeatEnricher{
			heartbeatRL:         rate.NewLimiter(rate.Every(heartbeatInterval), 1),
			extendedHeartbeatRL: rate.NewLimiter(rate.Every(extendedHeartBeatInterval), 1),
		},
	}

	// The rate limiter is initialized with a token, but we want the first heartbeat to be sent in one minute, so we consume the token
	mapper.heartbeatEnricher.heartbeatRL.Allow()
	mapper.heartbeatEnricher.extendedHeartbeatRL.Allow()
	return mapper
}

type defaultMapper struct {
	heartbeatEnricher
	messageBatchReducer
}

func (t *defaultMapper) Transform(payloads []transport.Payload) ([]transport.Payload, Mapper) {
	payloads, _ = t.heartbeatEnricher.Transform(payloads)
	payloads, _ = t.messageBatchReducer.Transform(payloads)
	return payloads, t
}

type messageBatchReducer struct{}

func (t *messageBatchReducer) Transform(payloads []transport.Payload) ([]transport.Payload, Mapper) {
	if len(payloads) <= 1 {
		return payloads, t
	}

	messages := make([]transport.Message, len(payloads))
	for i, payload := range payloads {
		messages[i] = transport.Message{
			RequestType: payload.RequestType(),
			Payload:     payload,
		}
	}

	return []transport.Payload{transport.MessageBatch(messages)}, t
}

type heartbeatEnricher struct {
	heartbeatRL         *rate.Limiter
	extendedHeartbeatRL *rate.Limiter

	extendedHeartbeat transport.AppExtendedHeartbeat
	heartbeat         transport.AppHeartbeat
}

func (t *heartbeatEnricher) Transform(payloads []transport.Payload) ([]transport.Payload, Mapper) {
	// Built the extended heartbeat using other payloads
	// Composition described here:
	// https://github.com/DataDog/instrumentation-telemetry-api-docs/blob/main/GeneratedDocumentation/ApiDocs/v2/producing-telemetry.md#app-extended-heartbeat
	for _, payload := range payloads {
		switch payload := payload.(type) {
		case transport.AppStarted:
			// Should be sent only once anyway
			t.extendedHeartbeat.Configuration = payload.Configuration
		case transport.AppDependenciesLoaded:
			if t.extendedHeartbeat.Dependencies == nil {
				t.extendedHeartbeat.Dependencies = payload.Dependencies
			}
		case transport.AppIntegrationChange:
			// The number of integrations should be small enough so we can just append to the list
			t.extendedHeartbeat.Integrations = append(t.extendedHeartbeat.Integrations, payload.Integrations...)
		}
	}

	if t.extendedHeartbeatRL.Allow() {
		return append(payloads, t.extendedHeartbeat), t
	}

	if t.heartbeatRL.Allow() {
		return append(payloads, t.heartbeat), t
	}

	// We don't send anything
	return payloads, t
}
