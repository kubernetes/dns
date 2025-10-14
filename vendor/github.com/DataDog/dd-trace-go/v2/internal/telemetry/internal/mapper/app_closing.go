// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package mapper

import (
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// NewAppClosingMapper returns a new Mapper that appends an AppClosing payload to the given payloads and calls the underlying Mapper with it.
func NewAppClosingMapper(next Mapper) Mapper {
	return &appClosingEnricher{next: next}
}

type appClosingEnricher struct {
	next Mapper
}

func (t *appClosingEnricher) Transform(payloads []transport.Payload) ([]transport.Payload, Mapper) {
	return t.next.Transform(append(payloads, transport.AppClosing{}))
}
