// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"strings"
)

// WithTags returns a LogOption that appends the tags for the telemetry log message. Tags are key-value pairs that are then
// serialized into a simple "key:value,key2:value2" format. No quoting or escaping is performed.
// Multiple calls to WithTags will append tags without duplications, preserving the order of first occurrence.
func WithTags(tags []string) LogOption {
	if len(tags) == 0 {
		return func(*loggerKey, *loggerValue) {}
	}

	// Pre-compute joined string to minimize closure size (string vs slice header)
	// and avoid repeated joins in the common fast-path case
	compiled := strings.Join(tags, ",")

	return func(key *loggerKey, _ *loggerValue) {
		if key == nil {
			return
		}

		if key.tags == "" {
			// Fast path: no existing tags, just assign
			key.tags = compiled
			return
		}

		// Slow path: merge and deduplicate
		seen := make(map[string]struct{})

		var builder strings.Builder
		builder.Grow(len(key.tags) + len(compiled) + 1)

		// Add existing tags
		for tag := range strings.SplitSeq(key.tags, ",") {
			if builder.Len() > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(tag)
			seen[tag] = struct{}{}
		}

		// Add new tags, skipping duplicates
		for tag := range strings.SplitSeq(compiled, ",") {
			if _, exists := seen[tag]; !exists {
				if builder.Len() > 0 {
					builder.WriteByte(',')
				}
				builder.WriteString(tag)
				seen[tag] = struct{}{}
			}
		}

		key.tags = builder.String()
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
