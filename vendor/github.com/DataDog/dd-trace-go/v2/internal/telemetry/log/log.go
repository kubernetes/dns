// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

// Package log provides secure telemetry logging with strict security controls.
//
// SECURITY MODEL:
//
// This package implements strict security controls for telemetry logging to prevent
// PII and sensitive information from being sent to external telemetry services.
//
// REQUIREMENTS:
//   - Messages MUST be constant templates only - no dynamic parameter replacement
//   - Stack traces MUST be redacted to show only Datadog, runtime, and known 3rd party frames
//   - Errors MUST use SafeError type with message redaction
//   - slog.Any() only allowed with LogValuer implementations
//
// BENEFITS:
//   - Constant messages enable deduplication to reduce redundant log transmission
//
// SECURE USAGE PATTERNS:
//
//	// ✅ Correct - constant message with structured data
//	telemetrylog.Error("operation failed", slog.String("operation", "startup"))
//	telemetrylog.Error("validation error", slog.Any("error", SafeError(err)))
//	telemetrylog.Error("operation failed", slog.Any("error", SafeError(err)), WithStacktrace())
//
//	// ❌ Forbidden - dynamic messages
//	telemetrylog.Error(err.Error()) // Raw error message
//	telemetrylog.Error("failed: " + details) // String concatenation
//	telemetrylog.Error(fmt.Sprintf("error: %s", err)) // Format strings
//
//	// ❌ Forbidden - raw error exposure
//	telemetrylog.Error("failed", slog.Any("error", err)) // Raw error object
//	telemetrylog.Error("failed", slog.String("err", err.Error())) // Raw error message
package log

import (
	"log/slog"
	"slices"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

type Logger struct {
	opts []telemetry.LogOption
}

var (
	// defaultLogger is the global logger instance with no pre-configured options
	defaultLogger atomic.Pointer[Logger]
	sendLog       func(r telemetry.Record, opts ...telemetry.LogOption) = telemetry.Log
)

func init() {
	defaultLogger.Store(&Logger{})
}

func SetDefaultLogger(logger *Logger) {
	defaultLogger.CompareAndSwap(defaultLogger.Load(), logger)
}

func With(opts ...telemetry.LogOption) *Logger {
	return &Logger{
		opts: opts,
	}
}

func (l *Logger) With(opts ...telemetry.LogOption) *Logger {
	return &Logger{
		opts: slices.Concat(l.opts, opts),
	}
}

func Debug(message string, attrs ...slog.Attr) {
	defaultLogger.Load().Debug(message, attrs...)
}

func Warn(message string, attrs ...slog.Attr) {
	defaultLogger.Load().Warn(message, attrs...)
}

func Error(message string, attrs ...slog.Attr) {
	defaultLogger.Load().Error(message, attrs...)
}

func (l *Logger) Debug(message string, attrs ...slog.Attr) {
	record := telemetry.NewRecord(telemetry.LogDebug, message)
	record.AddAttrs(attrs...)
	sendLog(record, l.opts...)
}

func (l *Logger) Warn(message string, attrs ...slog.Attr) {
	record := telemetry.NewRecord(telemetry.LogWarn, message)
	record.AddAttrs(attrs...)
	sendLog(record, l.opts...)
}

func (l *Logger) Error(message string, attrs ...slog.Attr) {
	record := telemetry.NewRecord(telemetry.LogError, message)
	record.AddAttrs(attrs...)
	sendLog(record, l.opts...)
}
