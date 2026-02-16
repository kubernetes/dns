// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import (
	"net/url"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

var (
	useFreshConfig bool
	instance       *Config
	// mu protects instance and useFreshConfig
	mu sync.Mutex
)

// Config represents global configuration properties.
// Config instances should be obtained via Get() which always returns a non-nil value.
// Methods on Config assume a non-nil receiver and will panic if called on nil.
type Config struct {
	mu sync.RWMutex
	// Config fields are protected by the mutex.
	agentURL                      *url.URL
	debug                         bool
	logStartup                    bool
	serviceName                   string
	version                       string
	env                           string
	serviceMappings               map[string]string
	hostname                      string
	runtimeMetrics                bool
	runtimeMetricsV2              bool
	profilerHotspots              bool
	profilerEndpoints             bool
	spanAttributeSchemaVersion    int
	peerServiceDefaultsEnabled    bool
	peerServiceMappings           map[string]string
	debugAbandonedSpans           bool
	spanTimeout                   time.Duration
	partialFlushMinSpans          int
	partialFlushEnabled           bool
	statsComputationEnabled       bool
	dataStreamsMonitoringEnabled  bool
	dynamicInstrumentationEnabled bool
	globalSampleRate              float64
	ciVisibilityEnabled           bool
	ciVisibilityAgentless         bool
	logDirectory                  string
	traceRateLimitPerSecond       float64
}

// loadConfig initializes and returns a new config by reading from all configured sources.
// This function is NOT thread-safe and should only be called once through Get's sync.Once.
func loadConfig() *Config {
	cfg := new(Config)

	// TODO: Use defaults from config json instead of hardcoding them here
	cfg.agentURL = provider.getURL("DD_TRACE_AGENT_URL", &url.URL{Scheme: "http", Host: "localhost:8126"})
	cfg.debug = provider.getBool("DD_TRACE_DEBUG", false)
	cfg.logStartup = provider.getBool("DD_TRACE_STARTUP_LOGS", false)
	cfg.serviceName = provider.getString("DD_SERVICE", "")
	cfg.version = provider.getString("DD_VERSION", "")
	cfg.env = provider.getString("DD_ENV", "")
	cfg.serviceMappings = provider.getMap("DD_SERVICE_MAPPING", nil)
	cfg.hostname = provider.getString("DD_TRACE_SOURCE_HOSTNAME", "")
	cfg.runtimeMetrics = provider.getBool("DD_RUNTIME_METRICS_ENABLED", false)
	cfg.runtimeMetricsV2 = provider.getBool("DD_RUNTIME_METRICS_V2_ENABLED", false)
	cfg.profilerHotspots = provider.getBool("DD_PROFILING_CODE_HOTSPOTS_COLLECTION_ENABLED", false)
	cfg.profilerEndpoints = provider.getBool("DD_PROFILING_ENDPOINT_COLLECTION_ENABLED", false)
	cfg.spanAttributeSchemaVersion = provider.getInt("DD_TRACE_SPAN_ATTRIBUTE_SCHEMA", 0)
	cfg.peerServiceDefaultsEnabled = provider.getBool("DD_TRACE_PEER_SERVICE_DEFAULTS_ENABLED", false)
	cfg.peerServiceMappings = provider.getMap("DD_TRACE_PEER_SERVICE_MAPPING", nil)
	cfg.debugAbandonedSpans = provider.getBool("DD_TRACE_DEBUG_ABANDONED_SPANS", false)
	cfg.spanTimeout = provider.getDuration("DD_TRACE_ABANDONED_SPAN_TIMEOUT", 0)
	cfg.partialFlushMinSpans = provider.getInt("DD_TRACE_PARTIAL_FLUSH_MIN_SPANS", 0)
	cfg.partialFlushEnabled = provider.getBool("DD_TRACE_PARTIAL_FLUSH_ENABLED", false)
	cfg.statsComputationEnabled = provider.getBool("DD_TRACE_STATS_COMPUTATION_ENABLED", false)
	cfg.dataStreamsMonitoringEnabled = provider.getBool("DD_DATA_STREAMS_ENABLED", false)
	cfg.dynamicInstrumentationEnabled = provider.getBool("DD_DYNAMIC_INSTRUMENTATION_ENABLED", false)
	cfg.globalSampleRate = provider.getFloat("DD_TRACE_SAMPLE_RATE", 0.0)
	cfg.ciVisibilityEnabled = provider.getBool("DD_CIVISIBILITY_ENABLED", false)
	cfg.ciVisibilityAgentless = provider.getBool("DD_CIVISIBILITY_AGENTLESS_ENABLED", false)
	cfg.logDirectory = provider.getString("DD_TRACE_LOG_DIRECTORY", "")
	cfg.traceRateLimitPerSecond = provider.getFloat("DD_TRACE_RATE_LIMIT", 0.0)

	return cfg
}

// Get returns the global configuration singleton.
// This function is thread-safe and can be called from multiple goroutines concurrently.
// The configuration is lazily initialized on first access using sync.Once, ensuring
// loadConfig() is called exactly once even under concurrent access.
func Get() *Config {
	mu.Lock()
	defer mu.Unlock()
	if useFreshConfig || instance == nil {
		instance = loadConfig()
	}

	return instance
}

func SetUseFreshConfig(use bool) {
	mu.Lock()
	defer mu.Unlock()
	useFreshConfig = use
}

func (c *Config) Debug() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.debug
}

func (c *Config) SetDebug(enabled bool, origin telemetry.Origin) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.debug = enabled
	telemetry.RegisterAppConfig("DD_TRACE_DEBUG", enabled, origin)
}
