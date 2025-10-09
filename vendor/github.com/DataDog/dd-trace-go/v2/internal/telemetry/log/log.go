// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package log

import (
	"fmt"

	internallog "github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

func divideArgs(args []any) ([]telemetry.LogOption, []any) {
	if len(args) == 0 {
		return nil, nil
	}

	var options []telemetry.LogOption
	var fmtArgs []any
	for _, arg := range args {
		if opt, ok := arg.(telemetry.LogOption); ok {
			options = append(options, opt)
		} else {
			fmtArgs = append(fmtArgs, arg)
		}
	}
	return options, fmtArgs
}

// Debug sends a telemetry payload with a debug log message to the backend.
func Debug(format string, args ...any) {
	log(telemetry.LogDebug, format, args)
}

// Warn sends a telemetry payload with a warning log message to the backend and the console as a debug log.
func Warn(format string, args ...any) {
	log(telemetry.LogWarn, format, args)
}

// Error sends a telemetry payload with an error log message to the backend and the console as a debug log.
func Error(format string, args ...any) {
	log(telemetry.LogError, format, args)
}

func log(lvl telemetry.LogLevel, format string, args []any) {
	opts, fmtArgs := divideArgs(args)
	telemetry.Log(lvl, fmt.Sprintf(format, fmtArgs...), opts...)

	if lvl != telemetry.LogDebug {
		internallog.Debug(format, fmtArgs...) //nolint:gocritic // Telemetry log plumbing needs to pass through variable format strings
	}
}
