// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package telemetry

import (
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"time"

	globalinternal "github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal"
)

type ClientConfig struct {
	// DependencyLoader determines how dependency data is sent via telemetry.
	// The default value is [debug.ReadBuildInfo] since Application Security Monitoring uses this data to detect vulnerabilities in the ASM-SCA product
	// To disable this feature, please implement a function that returns nil, false.
	// This can only be controlled via the env var DD_TELEMETRY_DEPENDENCY_COLLECTION_ENABLED
	DependencyLoader func() (*debug.BuildInfo, bool)

	// MetricsEnabled determines whether metrics are sent via telemetry.
	// If false, libraries should not send the generate-metrics or distributions events.
	// This can only be controlled via the env var DD_TELEMETRY_METRICS_ENABLED
	MetricsEnabled bool

	// LogsEnabled determines whether logs are sent via telemetry.
	// This can only be controlled via the env var DD_TELEMETRY_LOG_COLLECTION_ENABLED
	LogsEnabled bool

	// AgentlessURL is the full URL to the agentless telemetry endpoint. (optional)
	// Defaults to https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry
	AgentlessURL string

	// AgentURL is the url of the agent to send telemetry to. (optional)
	// If the AgentURL is not set, the telemetry client will not attempt to connect to the agent before sending to the agentless endpoint.
	AgentURL string

	// HTTPClient is the http client to use for sending telemetry, defaults to a http.DefaultClient copy.
	HTTPClient *http.Client

	// HeartbeatInterval is the interval at which to send a heartbeat payload, defaults to 60s.
	// The maximum value is 60s.
	HeartbeatInterval time.Duration

	// ExtendedHeartbeatInterval is the interval at which to send an extended heartbeat payload, defaults to 24h.
	ExtendedHeartbeatInterval time.Duration

	// FlushInterval is the interval at which the client flushes the data.
	// By default, the client will start to Flush at 60s intervals and will reduce the interval based on the load till it hit 15s
	// Both values cannot be higher than 60s because the heartbeat need to be sent at least every 60s. Values will be clamped otherwise.
	FlushInterval internal.Range[time.Duration]

	// PayloadQueueSize is the size of the payload queue. Default range is [4, 32].
	PayloadQueueSize internal.Range[int]

	// DistributionsSize is the size of the distribution queue. Default range is [2^8, 2^14].
	DistributionsSize internal.Range[int]

	// Debug enables debug mode for the telemetry client and sent it to the backend so it logs the request. The
	// DD_TELEMETRY_DEBUG environment variable, when set to a truthy value, overrides this setting.
	Debug bool

	// APIKey is the API key to use for sending telemetry to the agentless endpoint. (using DD_API_KEY env var by default)
	APIKey string

	// EarlyFlushPayloadSize is the size of the payload that will trigger an early flush.
	// This is necessary because backend won't allow bodies larger than 5MB.
	// The default value here will be 2MB to take into account the large inaccuracy in estimating the size of bodies
	EarlyFlushPayloadSize int

	// MaxDistributionsSize is the maximum number of logs with distinct message, level and tags that can be stored per flush window.
	// If the limit is reached, logs will be dropped and a log will be sent to the backend about it
	// The default value is 1024.
	MaxDistinctLogs int32

	// internalMetricsEnabled determines whether client stats metrics are sent via telemetry. Default to true.
	internalMetricsEnabled bool
}

var (
	// agentlessURL is the endpoint used to send telemetry in an agentless environment. It is
	// also the default URL in case connecting to the agent URL fails.
	agentlessURL = "https://instrumentation-telemetry-intake.datadoghq.com/api/v2/apmtelemetry"

	// defaultHeartbeatInterval is the default interval at which the agent sends a heartbeat.
	defaultHeartbeatInterval = time.Minute

	// defaultExtendedHeartbeatInterval is the default interval at which the agent sends an extended heartbeat.
	defaultExtendedHeartbeatInterval = 24 * time.Hour

	// defaultMinFlushInterval is the default interval at which the client flushes the data.
	defaultFlushIntervalRange = internal.Range[time.Duration]{
		Min: 15 * time.Second,
		Max: 60 * time.Second,
	}

	defaultAuthorizedHearbeatRange = internal.Range[time.Duration]{
		Min: time.Microsecond,
		Max: time.Minute,
	}

	agentProxyAPIPath = "/telemetry/proxy/api/v2/apmtelemetry"

	defaultEarlyFlushPayloadSize = 2 * 1024 * 1024 // 2MB

	// authorizedPayloadSize.Max is specified by the backend to be 5MB. The goal is to never reach this value otherwise our data will be silently dropped.
	authorizedPayloadSize = internal.Range[int]{
		Min: 0,
		Max: 5 * 1024 * 1024, // 5MB
	}

	// TODO: tweak this value once we get real telemetry data from the telemetry client
	// This means that, by default, we incur dataloss if we spend ~30mins without flushing, considering we send telemetry data this looks reasonable.
	// This also means that in the worst case scenario, memory-wise, the app is stabilized after running for 30mins.
	// Ideally both values should be power of 2 because of the way the ring queue is implemented as it's growing
	defaultPayloadQueueSize = internal.Range[int]{
		Min: 4,
		Max: 32,
	}

	// TODO: tweak this value once we get telemetry data from the telemetry client
	// Default max size is a 2^14 array of float64 (2^3 bytes) which makes a distribution 128KB bytes array _at worse_.
	// Considering we add a point per user request on a simple http server, we would be losing data after 2^14 requests per minute or about 280 requests per second or under 3ms per request.
	// If this throughput is constant, the telemetry client flush ticker speed will increase to, at best, double twice to flush 15 seconds of data each time.
	// Which will bring our max throughput to 1100 points per second or about 750µs per request.
	distributionsSize = internal.Range[int]{
		Min: 1 << 8,
		Max: 1 << 14,
	}

	// defaultMaxDistinctLogs is the default maximum number of logs with distinct message, level and tags that can be stored in a flush windows. 1024 per minute is already plenty, it's just to avoid memory leaks.
	defaultMaxDistinctLogs = int32(256)
)

func (config ClientConfig) validateConfig() error {
	if config.HeartbeatInterval > time.Minute {
		return fmt.Errorf("HeartbeatInterval cannot be higher than 60s, got %v", config.HeartbeatInterval)
	}

	if config.FlushInterval.Min > time.Minute || config.FlushInterval.Max > time.Minute {
		return fmt.Errorf("FlushIntervalRange cannot be higher than 60s, got Min: %v, Max: %v", config.FlushInterval.Min, config.FlushInterval.Max)
	}

	if !config.FlushInterval.IsOrdered() {
		return fmt.Errorf("FlushIntervalRange Min cannot be higher than Max, got Min: %v, Max: %v", config.FlushInterval.Min, config.FlushInterval.Max)
	}

	if !authorizedPayloadSize.Contains(config.EarlyFlushPayloadSize) {
		return fmt.Errorf("EarlyFlushPayloadSize must be between 0 and 5MB, got %v", config.EarlyFlushPayloadSize)
	}

	return nil
}

// defaultConfig returns a ClientConfig with default values set.
func defaultConfig(config ClientConfig) ClientConfig {
	config.Debug = config.Debug || globalinternal.BoolEnv("DD_TELEMETRY_DEBUG", false)

	if config.AgentlessURL == "" {
		config.AgentlessURL = agentlessURL
	}

	if config.APIKey == "" {
		config.APIKey = env.Get("DD_API_KEY")
	}

	if config.FlushInterval.Min == 0 {
		config.FlushInterval.Min = defaultFlushIntervalRange.Min
	} else {
		config.FlushInterval.Min = defaultAuthorizedHearbeatRange.Clamp(config.FlushInterval.Min)
	}

	if config.FlushInterval.Max == 0 {
		config.FlushInterval.Max = defaultFlushIntervalRange.Max
	} else {
		config.FlushInterval.Max = defaultAuthorizedHearbeatRange.Clamp(config.FlushInterval.Max)
	}

	heartBeatInterval := defaultHeartbeatInterval
	if config.HeartbeatInterval != 0 {
		heartBeatInterval = config.HeartbeatInterval
	}

	envVal := globalinternal.FloatEnv("DD_TELEMETRY_HEARTBEAT_INTERVAL", heartBeatInterval.Seconds())
	config.HeartbeatInterval = defaultAuthorizedHearbeatRange.Clamp(time.Duration(envVal * float64(time.Second)))
	if config.HeartbeatInterval != defaultHeartbeatInterval {
		log.Debug("telemetry: using custom heartbeat interval %s", config.HeartbeatInterval)
	}
	// Make sure we flush at least at each heartbeat interval
	config.FlushInterval = config.FlushInterval.ReduceMax(config.HeartbeatInterval)

	if config.HeartbeatInterval == config.FlushInterval.Max { // Since the go ticker is not exact when it comes to the interval, we need to make sure the heartbeat is actually sent
		config.HeartbeatInterval = config.HeartbeatInterval - 10*time.Millisecond
	}

	if config.DependencyLoader == nil && globalinternal.BoolEnv("DD_TELEMETRY_DEPENDENCY_COLLECTION_ENABLED", true) {
		config.DependencyLoader = debug.ReadBuildInfo
	}

	if !config.MetricsEnabled {
		config.MetricsEnabled = globalinternal.BoolEnv("DD_TELEMETRY_METRICS_ENABLED", true)
	}

	if !config.LogsEnabled {
		config.LogsEnabled = globalinternal.BoolEnv("DD_TELEMETRY_LOG_COLLECTION_ENABLED", true)
	}

	if !config.internalMetricsEnabled {
		config.internalMetricsEnabled = true
	}

	if config.EarlyFlushPayloadSize == 0 {
		config.EarlyFlushPayloadSize = defaultEarlyFlushPayloadSize
	}

	if config.ExtendedHeartbeatInterval == 0 {
		config.ExtendedHeartbeatInterval = defaultExtendedHeartbeatInterval
	}

	if config.PayloadQueueSize.Min == 0 {
		config.PayloadQueueSize.Min = defaultPayloadQueueSize.Min
	}

	if config.PayloadQueueSize.Max == 0 {
		config.PayloadQueueSize.Max = defaultPayloadQueueSize.Max
	}

	if config.DistributionsSize.Min == 0 {
		config.DistributionsSize.Min = distributionsSize.Min
	}

	if config.DistributionsSize.Max == 0 {
		config.DistributionsSize.Max = distributionsSize.Max
	}

	if config.MaxDistinctLogs == 0 {
		config.MaxDistinctLogs = defaultMaxDistinctLogs
	}

	return config
}

func newWriterConfig(config ClientConfig, tracerConfig internal.TracerConfig) (internal.WriterConfig, error) {
	endpoints := make([]*http.Request, 0, 2)
	if config.AgentURL != "" {
		baseURL, err := url.Parse(config.AgentURL)
		if err != nil {
			return internal.WriterConfig{}, fmt.Errorf("invalid agent URL: %s", err)
		}

		baseURL.Path = agentProxyAPIPath
		request, err := http.NewRequest(http.MethodPost, baseURL.String(), nil)
		if err != nil {
			return internal.WriterConfig{}, fmt.Errorf("failed to create request: %s", err)
		}

		endpoints = append(endpoints, request)
	}

	if config.AgentlessURL != "" && config.APIKey != "" {
		request, err := http.NewRequest(http.MethodPost, config.AgentlessURL, nil)
		if err != nil {
			return internal.WriterConfig{}, fmt.Errorf("failed to create request: %s", err)
		}

		request.Header.Set("DD-API-KEY", config.APIKey)
		endpoints = append(endpoints, request)
	}

	if len(endpoints) == 0 {
		return internal.WriterConfig{}, fmt.Errorf("telemetry: could not build any endpoint, please provide an AgentURL or an APIKey with an optional AgentlessURL")
	}

	return internal.WriterConfig{
		TracerConfig: tracerConfig,
		Endpoints:    endpoints,
		HTTPClient:   config.HTTPClient,
		Debug:        config.Debug,
	}, nil
}
