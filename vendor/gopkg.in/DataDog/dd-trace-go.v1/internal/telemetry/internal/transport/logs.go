// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

type Logs struct {
	Logs []LogMessage `json:"logs,omitempty"`
}

func (Logs) RequestType() RequestType {
	return RequestTypeLogs
}

type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

type LogMessage struct {
	Message    string   `json:"message"`
	Level      LogLevel `json:"level"`
	Count      uint32   `json:"count,omitempty"`
	Tags       string   `json:"tags,omitempty"` // comma separated list of tags, e.g. "tag1:1,tag2:toto"
	StackTrace string   `json:"stack_trace,omitempty"`
	TracerTime int64    `json:"tracer_time,omitempty"` // Unix timestamp in seconds
}
