// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package telemetry

import (
	"errors"
	"os"
	"strconv"
	"sync"

	"github.com/puzpuzpuz/xsync/v3"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/knownmetrics"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/mapper"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// NewClient creates a new telemetry client with the given service, environment, and version and config.
func NewClient(service, env, version string, config ClientConfig) (Client, error) {
	if service == "" {
		return nil, errors.New("service name must not be empty")
	}

	config = defaultConfig(config)
	if err := config.validateConfig(); err != nil {
		return nil, err
	}

	return newClient(internal.TracerConfig{Service: service, Env: env, Version: version}, config)
}

func newClient(tracerConfig internal.TracerConfig, config ClientConfig) (*client, error) {
	writerConfig, err := newWriterConfig(config, tracerConfig)
	if err != nil {
		return nil, err
	}

	writer, err := internal.NewWriter(writerConfig)
	if err != nil {
		return nil, err
	}

	client := &client{
		tracerConfig: tracerConfig,
		writer:       writer,
		clientConfig: config,
		flushMapper:  mapper.NewDefaultMapper(config.HeartbeatInterval, config.ExtendedHeartbeatInterval),
		payloadQueue: internal.NewRingQueue[transport.Payload](config.PayloadQueueSize),

		dependencies: dependencies{
			DependencyLoader: config.DependencyLoader,
		},
		metrics: metrics{
			store:         xsync.NewMapOf[metricKey, metricHandle](xsync.WithPresize(knownmetrics.SizeWithFilter(func(decl knownmetrics.Declaration) bool { return decl.Type != transport.DistMetric }))),
			skipAllowlist: config.Debug,
		},
		distributions: distributions{
			store:         xsync.NewMapOf[metricKey, *distribution](xsync.WithPresize(knownmetrics.SizeWithFilter(func(decl knownmetrics.Declaration) bool { return decl.Type == transport.DistMetric }))),
			pool:          internal.NewSyncPool(func() []float64 { return make([]float64, config.DistributionsSize.Min) }),
			skipAllowlist: config.Debug,
			queueSize:     config.DistributionsSize,
		},
		backend: newLoggerBackend(config.MaxDistinctLogs),
	}

	client.dataSources = append(client.dataSources,
		&client.integrations,
		&client.products,
		&client.configuration,
		&client.dependencies,
	)

	if config.LogsEnabled {
		client.dataSources = append(client.dataSources, client.backend)
	}

	if config.MetricsEnabled {
		client.dataSources = append(client.dataSources, &client.metrics, &client.distributions)
	}

	client.flushTicker = internal.NewTicker(client.Flush, config.FlushInterval)

	return client, nil
}

// dataSources is where the data that will be flushed is coming from. I.e metrics, logs, configurations, etc.
type dataSource interface {
	Payload() transport.Payload
}

type client struct {
	tracerConfig internal.TracerConfig
	clientConfig ClientConfig

	// Data sources
	dataSources   []dataSource
	integrations  integrations
	products      products
	configuration configuration
	dependencies  dependencies
	backend       *loggerBackend
	metrics       metrics
	distributions distributions

	// flushMapper is the transformer to use for the next flush on the gathered bodies on this tick
	flushMapper   mapper.Mapper
	flushMapperMu sync.Mutex

	// flushTicker is the ticker that triggers a call to client.Flush every flush interval
	flushTicker *internal.Ticker
	// flushMu is used to ensure that only one flush is happening at a time
	flushMu sync.Mutex

	// writer is the writer to use to send the payloads to the backend or the agent
	writer internal.Writer

	// payloadQueue is used when we cannot flush previously built payload for multiple reasons.
	payloadQueue *internal.RingQueue[transport.Payload]

	// flushTickerFuncs are functions that are called just before flushing the data to the backend.
	flushTickerFuncs   []func(Client)
	flushTickerFuncsMu sync.Mutex
}

func (c *client) Log(record Record, options ...LogOption) {
	if !c.clientConfig.LogsEnabled {
		return
	}

	c.backend.Add(record, options...)
}

func (c *client) MarkIntegrationAsLoaded(integration Integration) {
	c.integrations.Add(integration)
}

func (c *client) Count(namespace Namespace, name string, tags []string) MetricHandle {
	if !c.clientConfig.MetricsEnabled {
		return noopMetricHandle{}
	}
	return c.metrics.LoadOrStore(namespace, transport.CountMetric, name, tags)
}

func (c *client) Rate(namespace Namespace, name string, tags []string) MetricHandle {
	if !c.clientConfig.MetricsEnabled {
		return noopMetricHandle{}
	}
	return c.metrics.LoadOrStore(namespace, transport.RateMetric, name, tags)
}

func (c *client) Gauge(namespace Namespace, name string, tags []string) MetricHandle {
	if !c.clientConfig.MetricsEnabled {
		return noopMetricHandle{}
	}
	return c.metrics.LoadOrStore(namespace, transport.GaugeMetric, name, tags)
}

func (c *client) Distribution(namespace Namespace, name string, tags []string) MetricHandle {
	if !c.clientConfig.MetricsEnabled {
		return noopMetricHandle{}
	}
	return c.distributions.LoadOrStore(namespace, name, tags)
}

func (c *client) ProductStarted(product Namespace) {
	c.products.Add(product, true, nil)
}

func (c *client) ProductStopped(product Namespace) {
	c.products.Add(product, false, nil)
}

func (c *client) ProductStartError(product Namespace, err error) {
	c.products.Add(product, false, err)
}

func (c *client) RegisterAppConfig(key string, value any, origin Origin) {
	c.configuration.Add(Configuration{Name: key, Value: value, Origin: origin})
}

func (c *client) RegisterAppConfigs(kvs ...Configuration) {
	for _, value := range kvs {
		c.configuration.Add(value)
	}
}

func (c *client) AddFlushTicker(f func(Client)) {
	c.flushTickerFuncsMu.Lock()
	defer c.flushTickerFuncsMu.Unlock()
	c.flushTickerFuncs = append(c.flushTickerFuncs, f)
}

func (c *client) callFlushTickerFuncs() {
	c.flushTickerFuncsMu.Lock()
	defer c.flushTickerFuncsMu.Unlock()

	for _, f := range c.flushTickerFuncs {
		f(c)
	}
}

func (c *client) Config() ClientConfig {
	return c.clientConfig
}

// Flush sends all the data sources before calling flush
// This function is called by the flushTicker so it should not panic, or it will crash the whole customer application.
// If a panic occurs, we stop the telemetry and log the error.
func (c *client) Flush() {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if err, ok := r.(error); ok {
			log.Warn("panic while flushing telemetry data, stopping telemetry: %s", err.Error())
		} else {
			log.Warn("panic while flushing telemetry data, stopping telemetry!")
		}
		telemetryClientDisabled = true
		if gc, ok := GlobalClient().(*client); ok && gc == c {
			SwapClient(nil)
		}
	}()

	// We call the flushTickerFuncs before flushing the data for data sources
	c.callFlushTickerFuncs()

	payloads := make([]transport.Payload, 0, 8)
	for _, ds := range c.dataSources {
		if payload := ds.Payload(); payload != nil {
			payloads = append(payloads, payload)
		}
	}

	nbBytes, err := c.flush(payloads)
	if err != nil {
		// We check if the failure is about telemetry or appsec data to log the error at the right level
		var dependenciesFound bool
		for _, payload := range payloads {
			if payload.RequestType() == transport.RequestTypeAppDependenciesLoaded {
				dependenciesFound = true
				break
			}
		}
		if dependenciesFound {
			log.Warn("appsec: error while flushing SCA Security Data: %s", err.Error())
		} else {
			log.Debug("telemetry: error while flushing telemetry data: %s", err.Error())
		}

		return
	}

	if c.clientConfig.Debug {
		log.Debug("telemetry: flushed %d bytes of data", nbBytes)
	}
}

func (c *client) transform(payloads []transport.Payload) []transport.Payload {
	c.flushMapperMu.Lock()
	defer c.flushMapperMu.Unlock()
	payloads, c.flushMapper = c.flushMapper.Transform(payloads)
	return payloads
}

// flush sends all the data sources to the writer after having sent them through the [transform] function.
// It returns the amount of bytes sent to the writer.
func (c *client) flush(payloads []transport.Payload) (int, error) {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	payloads = c.transform(payloads)

	if c.payloadQueue.IsEmpty() && len(payloads) == 0 {
		return 0, nil
	}

	emptyQueue := c.payloadQueue.IsEmpty()
	// We enqueue the new payloads to preserve the order of the payloads
	c.payloadQueue.Enqueue(payloads...)
	payloads = c.payloadQueue.Flush()

	var (
		nbBytes        int
		speedIncreased bool
		failedCalls    []internal.EndpointRequestResult
	)

	for i, payload := range payloads {
		results, err := c.writer.Flush(payload)
		c.computeFlushMetrics(results, err)
		if err != nil {
			// We stop flushing when we encounter a fatal error, put the bodies in the queue and return the error
			if results[len(results)-1].StatusCode == 413 { // If the payload is too large we have no way to divide it, we can only skip it...
				log.Warn("telemetry: tried sending a payload that was too large, dropping it")
				continue
			}
			c.payloadQueue.Enqueue(payloads[i:]...)
			return nbBytes, err
		}

		failedCalls = append(failedCalls, results[:len(results)-1]...)
		successfulCall := results[len(results)-1]

		if !speedIncreased && successfulCall.PayloadByteSize > c.clientConfig.EarlyFlushPayloadSize {
			// We increase the speed of the flushTicker to try to flush the remaining bodies faster as we are at risk of sending too large bodies to the backend
			c.flushTicker.CanIncreaseSpeed()
			speedIncreased = true
		}

		nbBytes += successfulCall.PayloadByteSize
	}

	if emptyQueue && !speedIncreased { // If we did not send a very big payload, and we have no payloads
		c.flushTicker.CanDecreaseSpeed()
	}

	if len(failedCalls) > 0 {
		var errs []error
		for _, call := range failedCalls {
			errs = append(errs, call.Error)
		}
		log.Debug("telemetry: non-fatal error(s) while flushing telemetry data: %v", errors.Join(errs...).Error())
	}

	return nbBytes, nil
}

// computeFlushMetrics computes and submits the metrics for the flush operation using the output from the writer.Flush method.
// It will submit the number of requests, responses, errors, the number of bytes sent and the duration of the call that was successful.
func (c *client) computeFlushMetrics(results []internal.EndpointRequestResult, reason error) {
	if !c.clientConfig.internalMetricsEnabled {
		return
	}

	indexToEndpoint := func(i int) string {
		if i == 0 && c.clientConfig.AgentURL != "" {
			return "agent"
		}
		return "agentless"
	}

	for i, result := range results {
		endpoint := "endpoint:" + indexToEndpoint(i)
		c.Count(transport.NamespaceTelemetry, "telemetry_api.requests", []string{endpoint}).Submit(1)
		if result.StatusCode != 0 {
			c.Count(transport.NamespaceTelemetry, "telemetry_api.responses", []string{endpoint, "status_code:" + strconv.Itoa(result.StatusCode)}).Submit(1)
		}

		if result.Error != nil {
			typ := "type:network"
			if os.IsTimeout(result.Error) {
				typ = "type:timeout"
			}
			var writerStatusCodeError *internal.WriterStatusCodeError
			if errors.As(result.Error, &writerStatusCodeError) {
				typ = "type:status_code"
			}
			c.Count(transport.NamespaceTelemetry, "telemetry_api.errors", []string{endpoint, typ}).Submit(1)
		}
	}

	if reason != nil {
		return
	}

	successfulCall := results[len(results)-1]
	endpoint := "endpoint:" + indexToEndpoint(len(results)-1)
	c.Distribution(transport.NamespaceTelemetry, "telemetry_api.bytes", []string{endpoint}).Submit(float64(successfulCall.PayloadByteSize))
	c.Distribution(transport.NamespaceTelemetry, "telemetry_api.ms", []string{endpoint}).Submit(float64(successfulCall.CallDuration.Milliseconds()))
}

func (c *client) AppStart() {
	c.flushMapperMu.Lock()
	defer c.flushMapperMu.Unlock()
	c.flushMapper = mapper.NewAppStartedMapper(c.flushMapper)
}

func (c *client) AppStop() {
	c.flushMapperMu.Lock()
	defer c.flushMapperMu.Unlock()
	c.flushMapper = mapper.NewAppClosingMapper(c.flushMapper)
}

func (c *client) Close() error {
	c.flushTicker.Stop()
	return nil
}
