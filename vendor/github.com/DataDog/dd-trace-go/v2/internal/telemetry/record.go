// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"log/slog"
	"runtime"
	"time"
)

type Record struct {
	slog.Record
}

func logLevelToSlogLevel(level LogLevel) slog.Level {
	switch level {
	case LogDebug:
		return slog.LevelDebug
	case LogWarn:
		return slog.LevelWarn
	case LogError:
		return slog.LevelError
	default:
		return slog.LevelError
	}
}

func slogLevelToLogLevel(level slog.Level) LogLevel {
	switch level {
	case slog.LevelDebug:
		return LogDebug
	case slog.LevelWarn:
		return LogWarn
	case slog.LevelError:
		return LogError
	default:
		return LogError
	}
}

func NewRecord(level LogLevel, message string) Record {
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	return Record{
		Record: slog.Record{
			Time:    time.Now(),
			Level:   logLevelToSlogLevel(level),
			Message: message,
			PC:      pcs[0],
		},
	}
}
