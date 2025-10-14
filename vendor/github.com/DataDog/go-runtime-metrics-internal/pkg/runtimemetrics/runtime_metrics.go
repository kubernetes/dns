// Package runtimemetrics exports all runtime/metrics via statsd on a regular interval.
package runtimemetrics

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"runtime/metrics"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options are the options for the runtime metrics emitter.
type Options struct {
	// Logger is used to log errors. Defaults to slog.Default() if nil.
	Logger *slog.Logger
	// Tags are added to all metrics.
	Tags []string
	// Period is the period at which we poll runtime/metrics and report
	// them to statsd. Defaults to 10s.
	//
	// The statsd client aggregates this data, usually over a 2s window [1], and
	// so does the agent, usually over a 10s window [2].
	//
	// We submit one data point per aggregation window, using the
	// CountWithTimestamp / GaugeWithTimestamp APIs for submitting precisely
	// aligned metrics, to enable comparing them with one another.
	//
	// [1] https://github.com/DataDog/datadog-go/blob/e612112c8bb396b33ad5d9edd645d289b07d0e40/statsd/options.go/#L23
	// [2] https://docs.datadoghq.com/developers/dogstatsd/data_aggregation/#how-is-aggregation-performed-with-the-dogstatsd-server
	Period time.Duration
	// AllowMultipleInstances is used to allow multiple instances of the runtime
	// metrics emitter to be started. This is useful in cases where the
	// application is using multiple runtimemetrics.Emitter instances to report
	// metrics to different statsd clients.
	AllowMultipleInstances bool
}

// instances is used prevent multiple instances of the runtime metrics emitter
// from being started concurrently by accident.
var instances atomic.Int64

// NewEmitter creates a new runtime metrics emitter and starts it. Unless
// AllowMultipleInstances is set to true, it will return an error if an emitter
// has already been started and not stopped yet. This is to prevent
// accidental misconfigurations in larger systems.
func NewEmitter(statsd partialStatsdClientInterface, opts *Options) (*Emitter, error) {
	if opts == nil {
		opts = &Options{}
	}
	if n := instances.Add(1); n > 1 && !opts.AllowMultipleInstances {
		instances.Add(-1)
		return nil, errors.New("runtimemetrics has already been started")
	}
	e := &Emitter{
		statsd:  statsd,
		logger:  cmp.Or(opts.Logger, slog.Default()),
		tags:    opts.Tags,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		period:  cmp.Or(opts.Period, 10*time.Second),
	}
	go e.emit()
	return e, nil
}

// Emitter submits runtime/metrics to statsd on a regular interval.
type Emitter struct {
	statsd partialStatsdClientInterface
	logger *slog.Logger
	tags   []string
	period time.Duration

	stop    chan struct{}
	stopped chan struct{}
}

// emit emits runtime/metrics to statsd on a regular interval.
func (e *Emitter) emit() {
	descs := supportedMetrics()
	tags := append(getBaseTags(), e.tags...)
	rms := newRuntimeMetricStore(descs, e.statsd, e.logger, tags)
	// TODO: Go services experiencing high scheduling latency might see a
	// large variance for the period in between rms.report calls. This might
	// cause spikes in cumulative metric reporting. Should we try to correct
	// for this by measuring the actual reporting time delta to adjust
	// the numbers?
	//
	// Another challenge is that some metrics only update after GC mark
	// termination, see [1][2]. This means that it's likely that the rate of
	// submission for those metrics will be dependant on the service's workload
	// and GC configuration.
	//
	// [1] https://github.com/golang/go/blob/go1.21.3/src/runtime/mstats.go#L939
	// [2] https://github.com/golang/go/issues/59749
	tick := time.Tick(e.period)
	for {
		select {
		case <-e.stop:
			close(e.stopped)
			return
		case <-tick:
			rms.report()
		}
	}
}

// Stop stops the emitter. It is idempotent.
func (e *Emitter) Stop() {
	select {
	case <-e.stop:
		<-e.stopped
		return
	default:
		close(e.stop)
		<-e.stopped
		instances.Add(-1)
	}
}

type runtimeMetric struct {
	ddMetricName string
	cumulative   bool

	currentValue  metrics.Value
	previousValue metrics.Value
}

// the map key is the name of the metric in runtime/metrics
type runtimeMetricStore struct {
	metrics                map[string]*runtimeMetric
	statsd                 partialStatsdClientInterface
	logger                 *slog.Logger
	baseTags               []string
	unknownMetricLogOnce   *sync.Once
	unsupportedKindLogOnce *sync.Once
}

// partialStatsdClientInterface is the subset of statsd.ClientInterface that is
// used by this package.
type partialStatsdClientInterface interface {
	// Rate is used in the datadog-go statsd library to sample to values sent,
	// we should always submit a rate >=1 to ensure our submissions are not sampled.
	// The rate is forwarded to the agent but then discarded for gauge metrics.
	GaugeWithTimestamp(name string, value float64, tags []string, rate float64, timestamp time.Time) error
	CountWithTimestamp(name string, value int64, tags []string, rate float64, timestamp time.Time) error
	DistributionSamples(name string, values []float64, tags []string, rate float64) error
}

func newRuntimeMetricStore(descs []metrics.Description, statsdClient partialStatsdClientInterface, logger *slog.Logger, tags []string) runtimeMetricStore {
	rms := runtimeMetricStore{
		metrics:                map[string]*runtimeMetric{},
		statsd:                 statsdClient,
		logger:                 logger,
		baseTags:               tags,
		unknownMetricLogOnce:   &sync.Once{},
		unsupportedKindLogOnce: &sync.Once{},
	}

	for _, d := range descs {
		cumulative := d.Cumulative

		// /sched/latencies:seconds is incorrectly set as non-cumulative,
		// fixed by https://go-review.googlesource.com/c/go/+/486755
		// TODO: Use a build tag to apply this logic to Go versions < 1.20.
		if d.Name == "/sched/latencies:seconds" {
			cumulative = true
		}

		ddMetricName, err := datadogMetricName(d.Name)
		if err != nil {
			rms.logger.Warn("runtimemetrics: not reporting one of the runtime metrics", slog.Attr{Key: "error", Value: slog.StringValue(err.Error())})
			continue
		}

		rms.metrics[d.Name] = &runtimeMetric{
			ddMetricName: ddMetricName,
			cumulative:   cumulative,
		}
	}

	rms.update()

	return rms
}

func (rms runtimeMetricStore) update() {
	// TODO: Reuse this slice to avoid allocations? Note: I don't see these
	// allocs show up in profiling.
	samples := make([]metrics.Sample, len(rms.metrics))
	i := 0
	// NOTE: Map iteration in Go is randomized, so we end up randomizing the
	// samples slice. In theory this should not impact correctness, but it's
	// worth keeping in mind in case problems are observed in the future.
	for name := range rms.metrics {
		samples[i].Name = name
		i++
	}
	metrics.Read(samples)
	for _, s := range samples {
		runtimeMetric := rms.metrics[s.Name]

		runtimeMetric.previousValue = runtimeMetric.currentValue
		runtimeMetric.currentValue = s.Value
	}
}

func (rms runtimeMetricStore) report() {
	ts := time.Now()
	rms.update()
	samples := []distributionSample{}

	rms.statsd.GaugeWithTimestamp(datadogMetricPrefix+"enabled", 1, rms.baseTags, 1, ts)
	for name, rm := range rms.metrics {
		switch rm.currentValue.Kind() {
		case metrics.KindUint64:
			v := rm.currentValue.Uint64()
			// if the value didn't change between two reporting
			// cycles, don't submit anything. this avoids having
			// inaccurate drops to zero
			// we submit 0 values to be able to distinguish between
			// cases where the metric was never reported as opposed
			// to the metric always being equal to zero
			if rm.cumulative && v != 0 && v == rm.previousValue.Uint64() {
				continue
			}

			// Some of the Uint64 metrics are actually calculated as a difference by the Go runtime: v = uint64(x - y)
			//
			// Notably, this means that if x < y, then v will be roughly MaxUint64 (minus epsilon).
			// This then shows up as '16 EiB' in Datadog graphs, because MaxUint64 bytes = 2^64 = 2^(4 + 10x6) = 2^4 x (2^10)^6 = 16 x 1024^6 = 16 EiB.
			//
			// This is known to happen with the '/memory/classes/heap/unused:bytes' metric: https://github.com/golang/go/blob/go1.22.1/src/runtime/metrics.go#L364
			// Until this bug is fixed, we log the problematic value and skip submitting that point to avoid spurious spikes in graphs.
			if v > math.MaxUint64/2 {
				tags := make([]string, 0, len(rms.baseTags)+1)
				tags = append(tags, rms.baseTags...)
				tags = append(tags, "metric_name:"+rm.ddMetricName)
				rms.statsd.CountWithTimestamp(datadogMetricPrefix+"skipped_values", 1, tags, 1, ts)

				// Some metrics are ~sort of expected to report this high value (e.g.
				// "runtime.go.metrics.gc_gogc.percent" will consistently report "MaxUint64 - 1" if
				// GOGC is OFF). We only want to log the full heap stats for the not-so-expected
				// case of "heap unused bytes".
				if name == "/memory/classes/heap/unused:bytes" {
					logAttrs := []any{
						slog.Attr{Key: "metric_name", Value: slog.StringValue(rm.ddMetricName)},
						slog.Attr{Key: "timestamp", Value: slog.TimeValue(ts)},
						slog.Attr{Key: "uint64(x-y)", Value: slog.Uint64Value(v)},
						slog.Attr{
							// If v is very close to MaxUint64, it will be hard to read "how negative was x-y", so we compute it here for convenience:
							Key:   "int64(x-y)",
							Value: slog.Int64Value(-int64(math.MaxUint64 - v + 1)), // the '+1' is necessary because if int64(x-y)=-1, then uint64(x-y)=MaxUint64
						},
					}

					// Append all Uint64 values for maximum observability
					for name, rm := range rms.metrics {
						if rm.currentValue.Kind() == metrics.KindUint64 {
							logAttrs = append(logAttrs, slog.Attr{Key: name, Value: slog.Uint64Value(rm.currentValue.Uint64())})
						}
					}

					rms.logger.Warn("runtimemetrics: skipped submission of absurd value", logAttrs...)
				}
				continue
			}

			rms.statsd.GaugeWithTimestamp(rm.ddMetricName, float64(v), rms.baseTags, 1, ts)
		case metrics.KindFloat64:
			v := rm.currentValue.Float64()
			// if the value didn't change between two reporting
			// cycles, don't submit anything. this avoids having
			// inaccurate drops to zero
			// we submit 0 values to be able to distinguish between
			// cases where the metric was never reported as opposed
			// to the metric always being equal to zero
			if rm.cumulative && v != 0 && v == rm.previousValue.Float64() {
				continue
			}
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName, v, rms.baseTags, 1, ts)
		case metrics.KindFloat64Histogram:
			v := rm.currentValue.Float64Histogram()
			var equal bool
			if rm.cumulative {
				// Note: This branch should ALWAYS be taken as of go1.21.
				v, equal = sub(v, rm.previousValue.Float64Histogram())
				// if the histogram didn't change between two reporting
				// cycles, don't submit anything. this avoids having
				// inaccurate drops to zero for percentile metrics
				if equal {
					continue
				}
			}

			samples = samples[:0]
			distSamples := distributionSamplesFromHist(v, samples)
			values := make([]float64, len(distSamples))
			for i, ds := range distSamples {
				values[i] = ds.Value
				rms.statsd.DistributionSamples(rm.ddMetricName, values[i:i+1], rms.baseTags, ds.Rate)
			}

			stats := statsFromHist(v)
			// TODO: Could/should we use datadog distribution metrics for this?
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName+".avg", stats.Avg, rms.baseTags, 1, ts)
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName+".min", stats.Min, rms.baseTags, 1, ts)
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName+".max", stats.Max, rms.baseTags, 1, ts)
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName+".median", stats.Median, rms.baseTags, 1, ts)
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName+".p95", stats.P95, rms.baseTags, 1, ts)
			rms.statsd.GaugeWithTimestamp(rm.ddMetricName+".p99", stats.P99, rms.baseTags, 1, ts)
		case metrics.KindBad:
			// This should never happen because all metrics are supported
			// by construction.
			rms.unknownMetricLogOnce.Do(func() {
				rms.logger.Error("runtimemetrics: encountered an unknown metric, this should never happen and might indicate a bug", slog.Attr{Key: "metric_name", Value: slog.StringValue(name)})
			})
		default:
			// This may happen as new metric kinds get added.
			//
			// The safest thing to do here is to simply log it somewhere once
			// as something to look into, but ignore it for now.
			rms.unsupportedKindLogOnce.Do(func() {
				rms.logger.Error("runtimemetrics: unsupported metric kind, support for that kind should be added in pkg/runtimemetrics",
					slog.Attr{Key: "metric_name", Value: slog.StringValue(name)},
					slog.Attr{Key: "kind", Value: slog.AnyValue(rm.currentValue.Kind())},
				)
			})
		}
	}
}

// regex extracted from https://cs.opensource.google/go/go/+/refs/tags/go1.20.3:src/runtime/metrics/description.go;l=13
var runtimeMetricRegex = regexp.MustCompile("^(?P<name>/[^:]+):(?P<unit>[^:*/]+(?:[*/][^:*/]+)*)$")

// see https://docs.datadoghq.com/metrics/custom_metrics/#naming-custom-metrics
var datadogMetricRegex = regexp.MustCompile(`[^a-zA-Z0-9\._]`)

const datadogMetricPrefix = "runtime.go.metrics."

func datadogMetricName(runtimeName string) (string, error) {
	m := runtimeMetricRegex.FindStringSubmatch(runtimeName)

	if len(m) != 3 {
		return "", fmt.Errorf("failed to parse metric name for metric %s", runtimeName)
	}

	// strip leading "/"
	metricPath := strings.TrimPrefix(m[1], "/")
	metricUnit := m[2]

	name := datadogMetricRegex.ReplaceAllString(metricPath+"."+metricUnit, "_")

	// Note: This prefix is special. Don't change it without consulting the
	// runtime/metrics squad.
	return datadogMetricPrefix + name, nil
}

var startTags struct {
	sync.Mutex
	tags []string
}

// Start starts reporting runtime/metrics to the given statsd client.
//
// Deprecated: Use NewEmitter instead.
func Start(statsd partialStatsdClientInterface, logger *slog.Logger) error {
	startTags.Lock()
	defer startTags.Unlock()
	_, err := NewEmitter(statsd, &Options{Logger: logger, Tags: startTags.tags})
	return err
}

// SetBaseTags sets the base tags that will be added to all metrics when using
// the Start function.
//
// Deprecated: Use NewEmitter with Options.Tags instead.
func SetBaseTags(tags []string) {
	startTags.Lock()
	defer startTags.Unlock()
	startTags.tags = tags
}

// supportedMetrics returns the list of metrics that are supported.
func supportedMetrics() []metrics.Description {
	descs := metrics.All()
	supported := make([]metrics.Description, 0, len(supportedMetricsTable))
	for _, d := range descs {
		if _, ok := supportedMetricsTable[d.Name]; ok {
			supported = append(supported, d)
		}
	}
	return supported
}

// supportedMetricsTable contains all metrics as of go1.24, except godebug
// metrics to limit cardinality. New metrics are added manually b/c they need
// to be registered in the backend first.
var supportedMetricsTable = map[string]struct{}{
	"/cgo/go-to-c-calls:calls":                     {},
	"/cpu/classes/gc/mark/assist:cpu-seconds":      {},
	"/cpu/classes/gc/mark/dedicated:cpu-seconds":   {},
	"/cpu/classes/gc/mark/idle:cpu-seconds":        {},
	"/cpu/classes/gc/pause:cpu-seconds":            {},
	"/cpu/classes/gc/total:cpu-seconds":            {},
	"/cpu/classes/idle:cpu-seconds":                {},
	"/cpu/classes/scavenge/assist:cpu-seconds":     {},
	"/cpu/classes/scavenge/background:cpu-seconds": {},
	"/cpu/classes/scavenge/total:cpu-seconds":      {},
	"/cpu/classes/total:cpu-seconds":               {},
	"/cpu/classes/user:cpu-seconds":                {},
	"/gc/cycles/automatic:gc-cycles":               {},
	"/gc/cycles/forced:gc-cycles":                  {},
	"/gc/cycles/total:gc-cycles":                   {},
	"/gc/gogc:percent":                             {},
	"/gc/gomemlimit:bytes":                         {},
	"/gc/heap/allocs-by-size:bytes":                {},
	"/gc/heap/allocs:bytes":                        {},
	"/gc/heap/allocs:objects":                      {},
	"/gc/heap/frees-by-size:bytes":                 {},
	"/gc/heap/frees:bytes":                         {},
	"/gc/heap/frees:objects":                       {},
	"/gc/heap/goal:bytes":                          {},
	"/gc/heap/live:bytes":                          {},
	"/gc/heap/objects:objects":                     {},
	"/gc/heap/tiny/allocs:objects":                 {},
	"/gc/limiter/last-enabled:gc-cycle":            {},
	"/gc/pauses:seconds":                           {},
	"/gc/scan/globals:bytes":                       {},
	"/gc/scan/heap:bytes":                          {},
	"/gc/scan/stack:bytes":                         {},
	"/gc/scan/total:bytes":                         {},
	"/gc/stack/starting-size:bytes":                {},
	"/memory/classes/heap/free:bytes":              {},
	"/memory/classes/heap/objects:bytes":           {},
	"/memory/classes/heap/released:bytes":          {},
	"/memory/classes/heap/stacks:bytes":            {},
	"/memory/classes/heap/unused:bytes":            {},
	"/memory/classes/metadata/mcache/free:bytes":   {},
	"/memory/classes/metadata/mcache/inuse:bytes":  {},
	"/memory/classes/metadata/mspan/free:bytes":    {},
	"/memory/classes/metadata/mspan/inuse:bytes":   {},
	"/memory/classes/metadata/other:bytes":         {},
	"/memory/classes/os-stacks:bytes":              {},
	"/memory/classes/other:bytes":                  {},
	"/memory/classes/profiling/buckets:bytes":      {},
	"/memory/classes/total:bytes":                  {},
	"/sched/gomaxprocs:threads":                    {},
	"/sched/goroutines:goroutines":                 {},
	"/sched/latencies:seconds":                     {},
	"/sched/pauses/stopping/gc:seconds":            {},
	"/sched/pauses/stopping/other:seconds":         {},
	"/sched/pauses/total/gc:seconds":               {},
	"/sched/pauses/total/other:seconds":            {},
	"/sync/mutex/wait/total:seconds":               {},
}
