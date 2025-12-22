// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"strings"
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
	return func(_ *loggerKey, value *loggerValue) {
		if value == nil {
			return
		}
		value.captureStacktrace = true
	}
}
