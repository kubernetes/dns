// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import "github.com/DataDog/dd-trace-go/v2/ddtrace/internal"

func init() {
	var tracer Tracer = &NoopTracer{}
	internal.SetGlobalTracer(tracer)
}

// setGlobalTracer sets the global tracer to t.
func setGlobalTracer(t Tracer) {
	internal.SetGlobalTracer(t)
}

// getGlobalTracer returns the currently active tracer.
func getGlobalTracer() Tracer {
	return internal.GetGlobalTracer[Tracer]()
}
