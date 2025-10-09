// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"runtime/debug"
	"strings"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

type dependencies struct {
	DependencyLoader func() (*debug.BuildInfo, bool)

	once sync.Once

	mu       sync.Mutex
	payloads []transport.Payload
}

func (d *dependencies) Payload() transport.Payload {
	d.once.Do(func() {
		deps := d.loadDeps()
		// Requirement described here:
		// https://github.com/DataDog/instrumentation-telemetry-api-docs/blob/main/GeneratedDocumentation/ApiDocs/v2/producing-telemetry.md#app-dependencies-loaded
		const maxPerPayload = 2000
		if len(deps) > maxPerPayload {
			log.Debug("telemetry: too many (%d) dependencies to send, sending over multiple bodies", len(deps))
		}

		for i := 0; i < len(deps); i += maxPerPayload {
			end := min(i+maxPerPayload, len(deps))

			d.payloads = append(d.payloads, transport.AppDependenciesLoaded{
				Dependencies: deps[i:end],
			})
		}
	})

	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.payloads) == 0 {
		return nil
	}

	// return payloads one by one
	payloadZero := d.payloads[0]
	if len(d.payloads) == 1 {
		d.payloads = nil
	}

	if len(d.payloads) > 1 {
		d.payloads = d.payloads[1:]
	}

	return payloadZero
}

// loadDeps returns the dependencies from the DependencyLoader, formatted for telemetry intake.
func (d *dependencies) loadDeps() []transport.Dependency {
	if d.DependencyLoader == nil {
		return nil
	}

	deps, ok := d.DependencyLoader()
	if !ok {
		log.Debug("telemetry: could not read build info, no dependencies will be reported")
		return nil
	}

	transportDeps := make([]transport.Dependency, 0, len(deps.Deps))
	for _, dep := range deps.Deps {
		if dep == nil {
			continue
		}

		if dep.Replace != nil && dep.Replace.Version != "" {
			dep = dep.Replace
		}

		transportDeps = append(transportDeps, transport.Dependency{
			Name:    dep.Path,
			Version: strings.TrimPrefix(dep.Version, "v"),
		})
	}

	return transportDeps
}
