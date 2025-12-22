// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v3"

	"github.com/DataDog/dd-trace-go/v2/internal/stacktrace"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

const (
	stackTraceKey      = "stacktrace"
	telemetryStackSkip = 4 // Skip: CaptureWithRedaction, capture, loggerBackend.add, loggerBackend.Add
)

type loggerKey struct {
	tags    string
	message string
	level   LogLevel
}

type loggerValue struct {
	count  atomic.Uint32
	record Record

	captureStacktrace bool
	rawStack          stacktrace.RawStackTrace
}

type formatter struct {
	buffer  *bytes.Buffer
	handler slog.Handler
}

type loggerBackend struct {
	store *xsync.MapOf[loggerKey, *loggerValue]

	distinctLogs       atomic.Int32
	maxDistinctLogs    int32
	onceMaxLogsReached sync.Once

	formatters *sync.Pool
}

func newLoggerBackend(maxDistinctLogs int32) *loggerBackend {
	return &loggerBackend{
		store:           xsync.NewMapOf[loggerKey, *loggerValue](),
		maxDistinctLogs: maxDistinctLogs,

		formatters: &sync.Pool{
			New: func() any {
				buf := &bytes.Buffer{}
				return &formatter{
					buffer: buf,
					handler: slog.NewTextHandler(buf, &slog.HandlerOptions{
						ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
							// Remove time, level, source attributes, stacktrace and empty message attributes
							if a.Key == slog.TimeKey ||
								a.Key == slog.LevelKey ||
								a.Key == slog.SourceKey ||
								a.Key == stackTraceKey {
								return slog.Attr{}
							}
							if a.Key == slog.MessageKey && a.Value.String() == "" {
								return slog.Attr{}
							}
							return a
						},
					}),
				}
			},
		},
	}
}

func (logger *loggerBackend) Add(record Record, opts ...LogOption) {
	if logger.distinctLogs.Load() >= logger.maxDistinctLogs {
		logger.onceMaxLogsReached.Do(func() {
			logger.add(NewRecord(LogError, "telemetry: log count exceeded maximum, dropping log"), WithStacktrace())
		})
		return
	}

	logger.add(record, opts...)
}

func (logger *loggerBackend) add(record Record, opts ...LogOption) {
	key := loggerKey{
		level:   slogLevelToLogLevel(record.Level),
		message: record.Message,
	}

	for _, opt := range opts {
		opt(&key, nil)
	}

	value, _ := logger.store.LoadOrCompute(key, func() *loggerValue {
		// Create the record at capture time, not send time
		value := &loggerValue{
			record: record,
		}
		for _, opt := range opts {
			opt(nil, value)
		}
		if value.captureStacktrace {
			value.rawStack = stacktrace.CaptureRaw(telemetryStackSkip)
		}
		logger.distinctLogs.Add(1)
		return value
	})

	value.count.Add(1)
}

func (logger *loggerBackend) Payload() transport.Payload {
	logs := make([]transport.LogMessage, 0, logger.store.Size()+1)
	logger.store.Range(func(key loggerKey, value *loggerValue) bool {
		logger.store.Delete(key)
		logger.distinctLogs.Add(-1)
		msg := transport.LogMessage{
			Message:    logger.formatMessage(value.record),
			Level:      key.level,
			Tags:       key.tags,
			Count:      value.count.Load(),
			TracerTime: value.record.Time.Unix(),
		}
		if value.captureStacktrace {
			msg.StackTrace = stacktrace.Format(value.rawStack.SymbolicateWithRedaction())
		}
		logs = append(logs, msg)
		return true
	})

	if len(logs) == 0 {
		return nil
	}

	return transport.Logs{Logs: logs}
}

func (logger *loggerBackend) formatMessage(record Record) string {
	if logger.formatters == nil {
		return record.Message
	}

	hasAttrs := false
	record.Attrs(func(attr slog.Attr) bool {
		hasAttrs = true
		return false
	})

	if !hasAttrs {
		return record.Message
	}

	// Capture the message before clearing it.
	message := record.Message

	formatter := logger.formatters.Get().(*formatter)
	defer func() {
		formatter.buffer.Reset()
		logger.formatters.Put(formatter)
	}()

	// Clear the message so TextHandler only formats attributes.
	record.Message = ""
	formatter.handler.Handle(context.Background(), record.Record)
	formattedAttrs := strings.TrimSpace(formatter.buffer.String())

	if formattedAttrs == "" {
		return message
	}

	return message + ": " + formattedAttrs
}
