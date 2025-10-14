// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v3"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// WithTags returns a LogOption that sets the tags for the telemetry log message. Tags are key-value pairs that are then
// serialized into a simple "key:value,key2:value2" format. No quoting or escaping is performed.
func WithTags(tags []string) LogOption {
	compiledTags := strings.Join(tags, ",")
	return func(key *loggerKey, _ *loggerValue) {
		if key == nil {
			return
		}
		key.tags = compiledTags
	}
}

// WithStacktrace returns a LogOption that sets the stacktrace for the telemetry log message. The stacktrace is a string
// that is generated inside the WithStacktrace function. Logs demultiplication does not take the stacktrace into account.
// This means that a log that has been demultiplicated will only show of the first log.
func WithStacktrace() LogOption {
	buf := make([]byte, 4_096)
	buf = buf[:runtime.Stack(buf, false)]
	return func(_ *loggerKey, value *loggerValue) {
		if value == nil {
			return
		}
		value.stacktrace = string(buf)
	}
}

type loggerKey struct {
	tags    string
	message string
	level   LogLevel
}

type loggerValue struct {
	count      atomic.Uint32
	stacktrace string
	time       int64 // Unix timestamp
}

type logger struct {
	store *xsync.MapOf[loggerKey, *loggerValue]

	distinctLogs       atomic.Int32
	maxDistinctLogs    int32
	onceMaxLogsReached sync.Once
}

func (logger *logger) Add(level LogLevel, text string, opts ...LogOption) {
	if logger.distinctLogs.Load() >= logger.maxDistinctLogs {
		logger.onceMaxLogsReached.Do(func() {
			logger.add(LogError, "telemetry: log count exceeded maximum, dropping log", WithStacktrace())
		})
		return
	}

	logger.add(level, text, opts...)
}

func (logger *logger) add(level LogLevel, text string, opts ...LogOption) {
	key := loggerKey{
		message: text,
		level:   level,
	}

	for _, opt := range opts {
		opt(&key, nil)
	}

	value, _ := logger.store.LoadOrCompute(key, func() *loggerValue {
		value := &loggerValue{
			time: time.Now().Unix(),
		}
		for _, opt := range opts {
			opt(nil, value)
		}
		logger.distinctLogs.Add(1)
		return value
	})

	value.count.Add(1)
}

func (logger *logger) Payload() transport.Payload {
	logs := make([]transport.LogMessage, 0, logger.store.Size()+1)
	logger.store.Range(func(key loggerKey, value *loggerValue) bool {
		logger.store.Delete(key)
		logger.distinctLogs.Add(-1)
		logs = append(logs, transport.LogMessage{
			Message:    key.message,
			Level:      key.level,
			Tags:       key.tags,
			Count:      value.count.Load(),
			StackTrace: value.stacktrace,
			TracerTime: value.time,
		})
		return true
	})

	if len(logs) == 0 {
		return nil
	}

	return transport.Logs{Logs: logs}
}
