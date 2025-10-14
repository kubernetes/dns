// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"sync"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

type integrations struct {
	mu           sync.Mutex
	integrations []transport.Integration
}

func (i *integrations) Add(integration Integration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.integrations = append(i.integrations, transport.Integration{
		Name:    integration.Name,
		Version: integration.Version,
		Enabled: integration.Error == "", // no error means the integration was enabled successfully
		Error:   integration.Error,
	})
}

func (i *integrations) Payload() transport.Payload {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.integrations) == 0 {
		return nil
	}
	integrations := i.integrations
	i.integrations = nil
	return transport.AppIntegrationChange{
		Integrations: integrations,
	}
}
