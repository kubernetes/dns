// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package mapper

import (
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// Mapper is an interface for transforming payloads to comply with different types of lifecycle events in the application.
type Mapper interface {
	// Transform transforms the given payloads and returns the transformed payloads and the Mapper to use for the next
	// transformation at the next flush a minute later
	Transform([]transport.Payload) ([]transport.Payload, Mapper)
}
