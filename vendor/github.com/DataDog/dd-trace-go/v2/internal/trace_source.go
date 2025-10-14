// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import (
	"fmt"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// TraceSource represents the 8-bit bitmask for the _dd.p.ts tag
type TraceSource uint8

const (
	APMTraceSource TraceSource = 0x01
	ASMTraceSource TraceSource = 0x02
	DSMTraceSource TraceSource = 0x04
	DJMTraceSource TraceSource = 0x08
	DBMTraceSource TraceSource = 0x10
)

// String converts the bitmask to a two-character hexadecimal string
func (ts TraceSource) String() string {
	return fmt.Sprintf("%02X", uint8(ts))
}

// ParseTraceSource parses a hexadecimal string into a TraceSource bitmask
func ParseTraceSource(hexStr string) (TraceSource, error) {
	// Ensure at least 2 chars, allowing up to 8 for forward compatibility (32-bit)
	if len(hexStr) < 2 || len(hexStr) > 8 {
		return 0, fmt.Errorf("invalid length for TraceSource mask, expected 2 to 8 characters")
	}

	// Parse the full mask as a 32-bit unsigned integer
	value, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid hexadecimal format: %w", err)
	}

	// Extract only the least significant 8 bits (ensuring compliance with 8-bit mask)
	return TraceSource(value & 0xFF), nil
}

func VerifyTraceSourceEnabled(hexStr string, target TraceSource) bool {
	ts, err := ParseTraceSource(hexStr)
	if err != nil {
		if len(hexStr) != 0 { // Empty trace source should not trigger an error log.
			log.Error("invalid trace-source hex string given for source verification: %s", err.Error())
		}
		return false
	}

	return ts.IsSet(target)
}

// Set enables specific TraceSource (bit) in the bitmask
func (ts *TraceSource) Set(src TraceSource) {
	*ts |= src
}

// Unset disables specific TraceSource (bit) in the bitmask
func (ts *TraceSource) Unset(src TraceSource) {
	*ts &^= src
}

// IsSet checks if a specific TraceSource (bit) is enabled
func (ts TraceSource) IsSet(src TraceSource) bool {
	return ts&src != 0
}
