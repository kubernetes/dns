// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v3"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/knownmetrics"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// metricKey is used as a key in the metrics store hash map.
type metricKey struct {
	namespace Namespace
	kind      transport.MetricType
	name      string
	tags      string
}

func (k metricKey) SplitTags() []string {
	if k.tags == "" {
		return nil
	}
	return strings.Split(k.tags, ",")
}

func (k metricKey) String() string {
	return fmt.Sprintf("%s.%s.%s.%s", k.namespace, k.kind, k.name, k.tags)
}

func validateMetricKey(namespace Namespace, kind transport.MetricType, name string, tags []string) error {
	if len(name) == 0 {
		return fmt.Errorf("metric name with tags %v should be empty", tags)
	}

	if !knownmetrics.IsKnownMetric(namespace, kind, name) {
		return fmt.Errorf("metric name %q of kind %q in namespace %q is not a known metric, please update the list of metric names running ./scripts/gen_known_metrics.sh or check that you wrote the name correctly. "+
			"The metric will still be sent", name, string(kind), namespace)
	}

	for _, tag := range tags {
		if len(tag) == 0 {
			return fmt.Errorf("metric %q should not have empty tags", name)
		}

		if strings.Contains(tag, ",") {
			return fmt.Errorf("metric %q tag %q should not contain commas", name, tag)
		}
	}

	return nil
}

// newMetricKey returns a new metricKey with the given parameters with the tags sorted and joined by commas.
func newMetricKey(namespace Namespace, kind transport.MetricType, name string, tags []string) metricKey {
	sort.Strings(tags)
	return metricKey{namespace: namespace, kind: kind, name: name, tags: strings.Join(tags, ",")}
}

// metricsHandle is the internal equivalent of MetricHandle for Count/Rate/Gauge metrics that are sent via the payload [transport.GenerateMetrics].
type metricHandle interface {
	MetricHandle
	Payload() transport.MetricData
}

type metrics struct {
	store         *xsync.MapOf[metricKey, metricHandle]
	skipAllowlist bool // Debugging feature to skip the allowlist of known metrics
}

// LoadOrStore returns a MetricHandle for the given metric key. If the metric key does not exist, it will be created.
func (m *metrics) LoadOrStore(namespace Namespace, kind transport.MetricType, name string, tags []string) MetricHandle {

	var (
		key    = newMetricKey(namespace, kind, name, tags)
		handle MetricHandle
		loaded bool
	)
	switch kind {
	case transport.CountMetric:
		handle, loaded = m.store.LoadOrCompute(key, func() metricHandle { return &count{metric: metric{key: key}} })
	case transport.GaugeMetric:
		handle, loaded = m.store.LoadOrCompute(key, func() metricHandle { return &gauge{metric: metric{key: key}} })
	case transport.RateMetric:
		handle, loaded = m.store.LoadOrCompute(key, func() metricHandle {
			rate := &rate{count: count{metric: metric{key: key}}}
			now := time.Now()
			rate.intervalStart.Store(&now)
			return rate
		})
	default:
		log.Warn("telemetry: unknown metric type %q", kind)
		return nil
	}

	if !loaded && !m.skipAllowlist { // The metric is new: validate and log issues about it
		if err := validateMetricKey(namespace, kind, name, tags); err != nil {
			log.Warn("telemetry: %s", err.Error())
		}
	}

	return handle
}

func (m *metrics) Payload() transport.Payload {
	series := make([]transport.MetricData, 0, m.store.Size())
	m.store.Range(func(_ metricKey, handle metricHandle) bool {
		if payload := handle.Payload(); payload.Type != "" {
			series = append(series, payload)
		}
		return true
	})

	if len(series) == 0 {
		return nil
	}

	return transport.GenerateMetrics{Series: series, SkipAllowlist: m.skipAllowlist}
}

type metricPoint struct {
	value float64
	time  time.Time
}

// metric is a meta t
type metric struct {
	key metricKey
	ptr atomic.Pointer[metricPoint]
}

func (m *metric) Get() float64 {
	if ptr := m.ptr.Load(); ptr != nil {
		return ptr.value
	}

	return math.NaN()
}

func (m *metric) Payload() transport.MetricData {
	point := m.ptr.Swap(nil)
	if point == nil {
		return transport.MetricData{}
	}
	return m.payload(point)
}

func (m *metric) payload(point *metricPoint) transport.MetricData {
	if point == nil {
		return transport.MetricData{}
	}

	return transport.MetricData{
		Metric:    m.key.name,
		Namespace: m.key.namespace,
		Tags:      m.key.SplitTags(),
		Type:      m.key.kind,
		Common:    knownmetrics.IsCommonMetric(m.key.namespace, m.key.kind, m.key.name),
		Points: [][2]any{
			{point.time.Unix(), point.value},
		},
	}
}

// count is a metric that represents a single value that is incremented over time and flush and reset at zero when flushed
type count struct {
	metric
}

func (m *count) Submit(newValue float64) {
	newPoint := new(metricPoint)
	newPoint.time = time.Now()
	for {
		oldPoint := m.ptr.Load()
		var oldValue float64
		if oldPoint != nil {
			oldValue = oldPoint.value
		}
		newPoint.value = oldValue + newValue
		if m.ptr.CompareAndSwap(oldPoint, newPoint) {
			return
		}
	}
}

// gauge is a metric that represents a single value at a point in time that is not incremental
type gauge struct {
	metric
}

func (g *gauge) Submit(value float64) {
	newPoint := new(metricPoint)
	newPoint.time = time.Now()
	newPoint.value = value
	for {
		oldPoint := g.ptr.Load()
		if g.ptr.CompareAndSwap(oldPoint, newPoint) {
			return
		}
	}
}

// rate is like a count metric but the value sent is divided by an interval of time that is also sent/
type rate struct {
	count
	intervalStart atomic.Pointer[time.Time]
}

func (r *rate) Get() float64 {
	sum := r.count.Get()
	intervalStart := r.intervalStart.Load()
	if intervalStart == nil {
		return math.NaN()
	}

	intervalSeconds := time.Since(*intervalStart).Seconds()
	if int64(intervalSeconds) == 0 { // Interval for rate is too small, we prefer not sending data over sending something wrong
		return math.NaN()
	}

	return sum / intervalSeconds
}

func (r *rate) Payload() transport.MetricData {
	now := time.Now()
	intervalStart := r.intervalStart.Swap(&now)
	if intervalStart == nil {
		return transport.MetricData{}
	}

	intervalSeconds := time.Since(*intervalStart).Seconds()
	if int64(intervalSeconds) == 0 { // Interval for rate is too small, we prefer not sending data over sending something wrong
		return transport.MetricData{}
	}

	point := r.ptr.Swap(nil)
	if point == nil {
		return transport.MetricData{}
	}

	point.value /= intervalSeconds
	payload := r.metric.payload(point)
	payload.Interval = int64(intervalSeconds)
	return payload
}
