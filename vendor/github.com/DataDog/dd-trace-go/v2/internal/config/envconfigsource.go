// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import (
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

type envConfigSource struct{}

func (e *envConfigSource) get(key string) string {
	return env.Get(normalizeKey(key))
}

func (e *envConfigSource) origin() telemetry.Origin {
	return telemetry.OriginEnvVar
}
