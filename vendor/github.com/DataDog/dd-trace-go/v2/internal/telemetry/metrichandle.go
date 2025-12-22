// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal"
)

// noopMetricHandle is a no-op implementation of a metric handle.
type noopMetricHandle struct{}

func (noopMetricHandle) Submit(_ float64) {}

func (noopMetricHandle) Get() float64 {
	return math.NaN()
}

var metricLogLossOnce sync.Once

// swappableMetricHandle is a MetricHandle that holds a pointer to another MetricHandle and a recorder to replay actions done before the actual MetricHandle is set.
type swappableMetricHandle struct {
	ptr      atomic.Pointer[MetricHandle]
	recorder internal.Recorder[MetricHandle]
	maker    func(client Client) MetricHandle
}

func (t *swappableMetricHandle) Submit(value float64) {
	if Disabled() {
		return
	}

	inner := t.ptr.Load()
	if inner == nil || *inner == nil {
		if !t.recorder.Record(func(handle MetricHandle) {
			handle.Submit(value)
		}) {
			metricLogLossOnce.Do(func() {
				msg := "telemetry: metric is losing values because the telemetry client has not been started yet, dropping telemetry data, please start the telemetry client earlier to avoid data loss"
				log.Debug("%s\n", msg)
				Log(NewRecord(LogError, msg), WithStacktrace())
			})
		}
		return
	}

	(*inner).Submit(value)
}

func (t *swappableMetricHandle) Get() float64 {
	inner := t.ptr.Load()
	if inner == nil || *inner == nil {
		return 0
	}

	return (*inner).Get()
}

func (t *swappableMetricHandle) swap(handle MetricHandle) {
	if t.ptr.Swap(&handle) == nil {
		t.recorder.Replay(handle)
	}
}

var _ MetricHandle = (*swappableMetricHandle)(nil)
