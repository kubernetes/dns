// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package stacktrace

// unwindConfig holds configuration for stack unwinding
type unwindConfig struct {
	maxDepth        int
	skipFrames      int
	redactUserCode  bool
	includeInternal bool
}

// UnwindOption is a functional option for configuring stack unwinding
type UnwindOption func(*unwindConfig)

// WithRedaction enables or disables customer code redaction
func WithRedaction() UnwindOption {
	return func(cfg *unwindConfig) {
		cfg.redactUserCode = true
	}
}

// WithoutRedaction disables customer code redaction
func WithoutRedaction() UnwindOption {
	return func(cfg *unwindConfig) {
		cfg.redactUserCode = false
	}
}

// WithMaxDepth sets the maximum number of frames to capture
func WithMaxDepth(depth int) UnwindOption {
	return func(cfg *unwindConfig) {
		cfg.maxDepth = depth
	}
}

// WithSkipFrames sets the number of frames to skip from the top of the stack
func WithSkipFrames(skip int) UnwindOption {
	return func(cfg *unwindConfig) {
		cfg.skipFrames = skip
	}
}

// WithInternalFrames controls whether to include internal Datadog frames
func WithInternalFrames(include bool) UnwindOption {
	return func(cfg *unwindConfig) {
		cfg.includeInternal = include
	}
}
