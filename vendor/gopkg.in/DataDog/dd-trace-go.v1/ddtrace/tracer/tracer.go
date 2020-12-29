// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tracer

import (
	"os"
	"strconv"
	"sync"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/internal"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

var _ ddtrace.Tracer = (*tracer)(nil)

// tracer creates, buffers and submits Spans which are used to time blocks of
// computation. They are accumulated and streamed into an internal payload,
// which is flushed to the agent whenever its size exceeds a specific threshold
// or when a certain interval of time has passed, whichever happens first.
//
// tracer operates based on a worker loop which responds to various request
// channels. It additionally holds two buffers which accumulates error and trace
// queues to be processed by the payload encoder.
type tracer struct {
	*config

	// traceWriter is responsible for sending finished traces to their
	// destination, such as the Trace Agent or Datadog Forwarder.
	traceWriter traceWriter

	// out receives traces to be added to the payload.
	out chan []*span

	// stop causes the tracer to shut down when closed.
	stop chan struct{}

	// stopOnce ensures the tracer is stopped exactly once.
	stopOnce sync.Once

	// wg waits for all goroutines to exit when stopping.
	wg sync.WaitGroup

	// prioritySampling holds an instance of the priority sampler.
	prioritySampling *prioritySampler

	// pid of the process
	pid string

	// These integers track metrics about spans and traces as they are started,
	// finished, and dropped
	spansStarted, spansFinished, tracesDropped int64

	// rulesSampling holds an instance of the rules sampler. These are user-defined
	// rules for applying a sampling rate to spans that match the designated service
	// or operation name.
	rulesSampling *rulesSampler
}

const (
	// flushInterval is the interval at which the payload contents will be flushed
	// to the transport.
	flushInterval = 2 * time.Second

	// payloadMaxLimit is the maximum payload size allowed and should indicate the
	// maximum size of the package that the agent can receive.
	payloadMaxLimit = 9.5 * 1024 * 1024 // 9.5 MB

	// payloadSizeLimit specifies the maximum allowed size of the payload before
	// it will trigger a flush to the transport.
	payloadSizeLimit = payloadMaxLimit / 2

	// concurrentConnectionLimit specifies the maximum number of concurrent outgoing
	// connections allowed.
	concurrentConnectionLimit = 100
)

// statsInterval is the interval at which health metrics will be sent with the
// statsd client; replaced in tests.
var statsInterval = 10 * time.Second

// Start starts the tracer with the given set of options. It will stop and replace
// any running tracer, meaning that calling it several times will result in a restart
// of the tracer by replacing the current instance with a new one.
func Start(opts ...StartOption) {
	if internal.Testing {
		return // mock tracer active
	}
	t := newTracer(opts...)
	internal.SetGlobalTracer(t)
	if t.config.logStartup {
		logStartup(t)
	}
}

// Stop stops the started tracer. Subsequent calls are valid but become no-op.
func Stop() {
	internal.SetGlobalTracer(&internal.NoopTracer{})
	log.Flush()
}

// Span is an alias for ddtrace.Span. It is here to allow godoc to group methods returning
// ddtrace.Span. It is recommended and is considered more correct to refer to this type as
// ddtrace.Span instead.
type Span = ddtrace.Span

// StartSpan starts a new span with the given operation name and set of options.
// If the tracer is not started, calling this function is a no-op.
func StartSpan(operationName string, opts ...StartSpanOption) Span {
	return internal.GetGlobalTracer().StartSpan(operationName, opts...)
}

// Extract extracts a SpanContext from the carrier. The carrier is expected
// to implement TextMapReader, otherwise an error is returned.
// If the tracer is not started, calling this function is a no-op.
func Extract(carrier interface{}) (ddtrace.SpanContext, error) {
	return internal.GetGlobalTracer().Extract(carrier)
}

// Inject injects the given SpanContext into the carrier. The carrier is
// expected to implement TextMapWriter, otherwise an error is returned.
// If the tracer is not started, calling this function is a no-op.
func Inject(ctx ddtrace.SpanContext, carrier interface{}) error {
	return internal.GetGlobalTracer().Inject(ctx, carrier)
}

// payloadQueueSize is the buffer size of the trace channel.
const payloadQueueSize = 1000

func newUnstartedTracer(opts ...StartOption) *tracer {
	c := newConfig(opts...)
	envRules, err := samplingRulesFromEnv()
	if err != nil {
		log.Warn("DIAGNOSTICS Error(s) parsing DD_TRACE_SAMPLING_RULES: %s", err)
	}
	if envRules != nil {
		c.samplingRules = envRules
	}
	sampler := newPrioritySampler()
	var writer traceWriter
	if c.logToStdout {
		writer = newLogTraceWriter(c)
	} else {
		writer = newAgentTraceWriter(c, sampler)
	}
	return &tracer{
		config:           c,
		traceWriter:      writer,
		out:              make(chan []*span, payloadQueueSize),
		stop:             make(chan struct{}),
		rulesSampling:    newRulesSampler(c.samplingRules),
		prioritySampling: sampler,
		pid:              strconv.Itoa(os.Getpid()),
	}
}

func newTracer(opts ...StartOption) *tracer {
	t := newUnstartedTracer(opts...)
	c := t.config
	t.config.statsd.Incr("datadog.tracer.started", nil, 1)
	if c.runtimeMetrics {
		log.Debug("Runtime metrics enabled.")
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.reportRuntimeMetrics(defaultMetricsReportInterval)
		}()
	}
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		tick := t.config.tickChan
		if tick == nil {
			ticker := time.NewTicker(flushInterval)
			defer ticker.Stop()
			tick = ticker.C
		}
		t.worker(tick)
	}()

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.reportHealthMetrics(statsInterval)
	}()
	return t
}

// worker receives finished traces to be added into the payload, as well
// as periodically flushes traces to the transport.
func (t *tracer) worker(tick <-chan time.Time) {
	for {
		select {
		case trace := <-t.out:
			t.traceWriter.add(trace)

		case <-tick:
			t.config.statsd.Incr("datadog.tracer.flush_triggered", []string{"reason:scheduled"}, 1)
			t.traceWriter.flush()

		case <-t.stop:
		loop:
			// the loop ensures that the payload channel is fully drained
			// before the final flush to ensure no traces are lost (see #526)
			for {
				select {
				case trace := <-t.out:
					t.traceWriter.add(trace)
				default:
					break loop
				}
			}
			return
		}
	}
}

func (t *tracer) pushTrace(trace []*span) {
	select {
	case <-t.stop:
		return
	default:
	}
	select {
	case t.out <- trace:
	default:
		log.Error("payload queue full, dropping %d traces", len(trace))
	}
}

// StartSpan creates, starts, and returns a new Span with the given `operationName`.
func (t *tracer) StartSpan(operationName string, options ...ddtrace.StartSpanOption) ddtrace.Span {
	var opts ddtrace.StartSpanConfig
	for _, fn := range options {
		fn(&opts)
	}
	var startTime int64
	if opts.StartTime.IsZero() {
		startTime = now()
	} else {
		startTime = opts.StartTime.UnixNano()
	}
	var context *spanContext
	if opts.Parent != nil {
		if ctx, ok := opts.Parent.(*spanContext); ok {
			context = ctx
		}
	}
	id := opts.SpanID
	if id == 0 {
		id = random.Uint64()
	}
	// span defaults
	span := &span{
		Name:         operationName,
		Service:      t.config.serviceName,
		Resource:     operationName,
		SpanID:       id,
		TraceID:      id,
		Start:        startTime,
		taskEnd:      startExecutionTracerTask(operationName),
		noDebugStack: t.config.noDebugStack,
	}
	if context != nil {
		// this is a child span
		span.TraceID = context.traceID
		span.ParentID = context.spanID
		if p, ok := context.samplingPriority(); ok {
			span.setMetric(keySamplingPriority, float64(p))
		}
		if context.span != nil {
			// local parent, inherit service
			context.span.RLock()
			span.Service = context.span.Service
			context.span.RUnlock()
		} else {
			// remote parent
			if context.origin != "" {
				// mark origin
				span.setMeta(keyOrigin, context.origin)
			}
		}
	}
	span.context = newSpanContext(span, context)
	if context == nil || context.span == nil {
		// this is either a root span or it has a remote parent, we should add the PID.
		span.setMeta(ext.Pid, t.pid)
		if t.hostname != "" {
			span.setMeta(keyHostname, t.hostname)
		}
		if _, ok := opts.Tags[ext.ServiceName]; !ok && t.config.runtimeMetrics {
			// this is a root span in the global service; runtime metrics should
			// be linked to it:
			span.setMeta("language", "go")
		}
	}
	// add tags from options
	for k, v := range opts.Tags {
		span.SetTag(k, v)
	}
	// add global tags
	for k, v := range t.config.globalTags {
		span.SetTag(k, v)
	}
	if t.config.version != "" && span.Service == t.config.serviceName {
		span.SetTag(ext.Version, t.config.version)
	}
	if t.config.env != "" {
		span.SetTag(ext.Environment, t.env)
	}
	if context == nil {
		// this is a brand new trace, sample it
		t.sample(span)
	}
	return span
}

// Stop stops the tracer.
func (t *tracer) Stop() {
	t.stopOnce.Do(func() {
		close(t.stop)
		t.config.statsd.Incr("datadog.tracer.stopped", nil, 1)
	})
	t.wg.Wait()
	t.traceWriter.stop()
	t.config.statsd.Close()
}

// Inject uses the configured or default TextMap Propagator.
func (t *tracer) Inject(ctx ddtrace.SpanContext, carrier interface{}) error {
	return t.config.propagator.Inject(ctx, carrier)
}

// Extract uses the configured or default TextMap Propagator.
func (t *tracer) Extract(carrier interface{}) (ddtrace.SpanContext, error) {
	return t.config.propagator.Extract(carrier)
}

// sampleRateMetricKey is the metric key holding the applied sample rate. Has to be the same as the Agent.
const sampleRateMetricKey = "_sample_rate"

// Sample samples a span with the internal sampler.
func (t *tracer) sample(span *span) {
	if _, ok := span.context.samplingPriority(); ok {
		// sampling decision was already made
		return
	}
	sampler := t.config.sampler
	if !sampler.Sample(span) {
		span.context.drop = true
		return
	}
	if rs, ok := sampler.(RateSampler); ok && rs.Rate() < 1 {
		span.setMetric(sampleRateMetricKey, rs.Rate())
	}
	if t.rulesSampling.apply(span) {
		return
	}
	t.prioritySampling.apply(span)
}
