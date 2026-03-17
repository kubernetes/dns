// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/tinylib/msgp/msgp"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/internal"
	appsecconfig "github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/civisibility/constants"
	internalconfig "github.com/DataDog/dd-trace-go/v2/internal/config"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/globalconfig"
	llmobsconfig "github.com/DataDog/dd-trace-go/v2/internal/llmobs/config"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/namingschema"
	"github.com/DataDog/dd-trace-go/v2/internal/normalizer"
	"github.com/DataDog/dd-trace-go/v2/internal/orchestrion"
	"github.com/DataDog/dd-trace-go/v2/internal/stableconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	"github.com/DataDog/dd-trace-go/v2/internal/version"

	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	envLLMObsEnabled          = "DD_LLMOBS_ENABLED"
	envLLMObsMlApp            = "DD_LLMOBS_ML_APP"
	envLLMObsAgentlessEnabled = "DD_LLMOBS_AGENTLESS_ENABLED"
	envLLMObsProjectName      = "DD_LLMOBS_PROJECT_NAME"
)

var contribIntegrations = map[string]struct {
	name     string // user readable name for startup logs
	imported bool   // true if the user has imported the integration
}{
	"github.com/99designs/gqlgen":                   {"gqlgen", false},
	"github.com/aws/aws-sdk-go":                     {"AWS SDK", false},
	"github.com/aws/aws-sdk-go-v2":                  {"AWS SDK v2", false},
	"github.com/bradfitz/gomemcache":                {"Memcache", false},
	"cloud.google.com/go/pubsub.v1":                 {"Pub/Sub", false},
	"cloud.google.com/go/pubsub/v2":                 {"Pub/Sub v2", false},
	"github.com/confluentinc/confluent-kafka-go":    {"Kafka (confluent)", false},
	"github.com/confluentinc/confluent-kafka-go/v2": {"Kafka (confluent) v2", false},
	"database/sql":                                  {"SQL", false},
	"github.com/dimfeld/httptreemux/v5":             {"HTTP Treemux", false},
	"github.com/elastic/go-elasticsearch/v6":        {"Elasticsearch v6", false},
	"github.com/emicklei/go-restful/v3":             {"go-restful v3", false},
	"github.com/gin-gonic/gin":                      {"Gin", false},
	"github.com/globalsign/mgo":                     {"MongoDB (mgo)", false},
	"github.com/go-chi/chi":                         {"chi", false},
	"github.com/go-chi/chi/v5":                      {"chi v5", false},
	"github.com/go-pg/pg/v10":                       {"go-pg v10", false},
	"github.com/go-redis/redis":                     {"Redis", false},
	"github.com/go-redis/redis/v7":                  {"Redis v7", false},
	"github.com/go-redis/redis/v8":                  {"Redis v8", false},
	"go.mongodb.org/mongo-driver":                   {"MongoDB", false},
	"github.com/gocql/gocql":                        {"Cassandra", false},
	"github.com/gofiber/fiber/v2":                   {"Fiber", false},
	"github.com/gomodule/redigo":                    {"Redigo", false},
	"google.golang.org/api":                         {"Google API", false},
	"google.golang.org/grpc":                        {"gRPC", false},
	"github.com/gorilla/mux":                        {"Gorilla Mux", false},
	"gorm.io/gorm.v1":                               {"Gorm v1", false},
	"github.com/graph-gophers/graphql-go":           {"Graph Gophers GraphQL", false},
	"github.com/graphql-go/graphql":                 {"GraphQL-Go GraphQL", false},
	"github.com/hashicorp/consul/api":               {"Consul", false},
	"github.com/hashicorp/vault/api":                {"Vault", false},
	"github.com/jackc/pgx/v5":                       {"PGX", false},
	"github.com/jmoiron/sqlx":                       {"SQLx", false},
	"github.com/julienschmidt/httprouter":           {"HTTP Router", false},
	"k8s.io/client-go/kubernetes":                   {"Kubernetes", false},
	"github.com/labstack/echo/v4":                   {"echo v4", false},
	"log/slog":                                      {"log/slog", false},
	"github.com/mark3labs/mcp-go":                   {"MCP", false},
	"github.com/miekg/dns":                          {"miekg/dns", false},
	"net/http":                                      {"HTTP", false},
	"gopkg.in/olivere/elastic.v5":                   {"Elasticsearch v5", false},
	"github.com/redis/go-redis/v9":                  {"Redis v9", false},
	"github.com/redis/rueidis":                      {"Rueidis", false},
	"github.com/segmentio/kafka-go":                 {"Kafka v0", false},
	"github.com/IBM/sarama":                         {"IBM sarama", false},
	"github.com/Shopify/sarama":                     {"Shopify sarama", false},
	"github.com/sirupsen/logrus":                    {"Logrus", false},
	"github.com/syndtr/goleveldb":                   {"LevelDB", false},
	"github.com/tidwall/buntdb":                     {"BuntDB", false},
	"github.com/twitchtv/twirp":                     {"Twirp", false},
	"github.com/uptrace/bun":                        {"Bun", false},
	"github.com/urfave/negroni":                     {"Negroni", false},
	"github.com/valyala/fasthttp":                   {"FastHTTP", false},
	"github.com/valkey-io/valkey-go":                {"Valkey", false},
}

var (
	// defaultSocketDSD specifies the socket path to use for connecting to the statsd server.
	// Replaced in tests
	defaultSocketDSD = "/var/run/datadog/dsd.socket"

	// defaultStatsdPort specifies the default port to use for connecting to the statsd server.
	defaultStatsdPort = "8125"

	// defaultMaxTagsHeaderLen specifies the default maximum length of the X-Datadog-Tags header value.
	defaultMaxTagsHeaderLen = 512
)

// Supported trace protocols.
const (
	traceProtocolV04 = 0.4 // v0.4 (default)
	traceProtocolV1  = 1.0 // v1.0
)

// config holds the tracer configuration.
type config struct {
	// internalConfig holds a reference to the global configuration singleton.
	internalConfig *internalconfig.Config

	// appsecStartOptions controls the options used when starting appsec features.
	appsecStartOptions []appsecconfig.StartOption

	// agent holds the capabilities of the agent and determines some
	// of the behaviour of the tracer.
	agent agentFeatures

	// integrations reports if the user has instrumented a Datadog integration and
	// if they have a version of the library available to integrate.
	integrations map[string]integrationConfig

	// sendRetries is the number of times a trace or CI Visibility payload send is retried upon
	// failure.
	sendRetries int

	// serviceName specifies the name of this application.
	serviceName string

	// universalVersion, reports whether span service name and config service name
	// should match to set application version tag. False by default
	universalVersion bool

	// sampler specifies the sampler that will be used for sampling traces.
	sampler RateSampler

	// agentURL is the agent URL that receives traces from the tracer.
	agentURL *url.URL

	// originalAgentURL is the agent URL that receives traces from the tracer and does not get changed.
	originalAgentURL *url.URL

	// globalTags holds a set of tags that will be automatically applied to
	// all spans.
	globalTags dynamicConfig[map[string]interface{}]

	// transport specifies the Transport interface which will be used to send data to the agent.
	transport transport

	// httpClientTimeout specifies the timeout for the HTTP client.
	httpClientTimeout time.Duration

	// propagator propagates span context cross-process
	propagator Propagator

	// httpClient specifies the HTTP client to be used by the agent's transport.
	httpClient *http.Client

	// logger specifies the logger to use when printing errors. If not specified, the "log" package
	// will be used.
	logger Logger

	// dogstatsdAddr specifies the address to connect for sending metrics to the
	// Datadog Agent. If not set, it defaults to "localhost:8125" or to the
	// combination of the environment variables DD_AGENT_HOST and DD_DOGSTATSD_PORT.
	dogstatsdAddr string

	// statsdClient is set when a user provides a custom statsd client for tracking metrics
	// associated with the runtime and the tracer.
	statsdClient internal.StatsdClient

	// spanRules contains user-defined rules to determine the sampling rate to apply
	// to a single span without affecting the entire trace
	spanRules []SamplingRule

	// traceRules contains user-defined rules to determine the sampling rate to apply
	// to the entire trace if any spans satisfy the criteria
	traceRules []SamplingRule

	// tickChan specifies a channel which will receive the time every time the tracer must flush.
	// It defaults to time.Ticker; replaced in tests.
	tickChan <-chan time.Time

	// enabled reports whether tracing is enabled.
	enabled dynamicConfig[bool]

	// enableHostnameDetection specifies whether the tracer should enable hostname detection.
	enableHostnameDetection bool

	// spanAttributeSchemaVersion holds the selected DD_TRACE_SPAN_ATTRIBUTE_SCHEMA version.
	spanAttributeSchemaVersion int

	// peerServiceDefaultsEnabled indicates whether the peer.service tag calculation is enabled or not.
	peerServiceDefaultsEnabled bool

	// peerServiceMappings holds a set of service mappings to dynamically rename peer.service values.
	peerServiceMappings map[string]string

	// orchestrionCfg holds Orchestrion (aka auto-instrumentation) configuration.
	// Only used for telemetry currently.
	orchestrionCfg orchestrionConfig

	// traceSampleRate holds the trace sample rate.
	traceSampleRate dynamicConfig[float64]

	// traceSampleRules holds the trace sampling rules
	traceSampleRules dynamicConfig[[]SamplingRule]

	// headerAsTags holds the header as tags configuration.
	headerAsTags dynamicConfig[[]string]

	// dynamicInstrumentationEnabled controls whether the target application can
	// be modified by Dynamic Instrumentation / Live Debugger. If the value is
	// explicitly set to false (as opposed to starting as false by default), then
	// it is frozen -- it cannot be overwritten by Remote Config.
	dynamicInstrumentationEnabled dynamicConfig[bool]

	// ciVisibilityAgentless controls if the tracer is loaded with CI Visibility agentless mode. default false
	ciVisibilityAgentless bool

	// ciVisibilityNoopTracer controls if CI Visibility must set a wrapper to behave like a noop tracer. default false
	ciVisibilityNoopTracer bool

	// tracingAsTransport specifies whether the tracer is running in transport-only mode, where traces are only sent when other products request it.
	tracingAsTransport bool

	// traceProtocol specifies the trace protocol to use.
	traceProtocol float64

	// llmobs contains the LLM Observability config
	llmobs llmobsconfig.Config
}

// orchestrionConfig contains Orchestrion configuration.
type (
	orchestrionConfig struct {
		// Enabled indicates whether this tracer was instanciated via Orchestrion.
		Enabled bool `json:"enabled"`

		// Metadata holds Orchestrion specific metadata (e.g orchestrion version, mode (toolexec or manual) etc..)
		Metadata *orchestrionMetadata `json:"metadata,omitempty"`
	}
	orchestrionMetadata struct {
		// Version is the version of the orchestrion tool that was used to instrument the application.
		Version string `json:"version,omitempty"`
	}
)

// StartOption represents a function that can be provided as a parameter to Start.
type StartOption func(*config)

// maxPropagatedTagsLength limits the size of DD_TRACE_X_DATADOG_TAGS_MAX_LENGTH to prevent HTTP 413 responses.
const maxPropagatedTagsLength = 512

// newConfig renders the tracer configuration based on defaults, environment variables
// and passed user opts.
func newConfig(opts ...StartOption) (*config, error) {
	c := new(config)
	c.internalConfig = internalconfig.CreateNew()

	// If this was built with a recent-enough version of Orchestrion, force the orchestrion config to
	// the baked-in values. We do this early so that opts can be used to override the baked-in values,
	// which is necessary for some tests to work properly.
	c.orchestrionCfg.Enabled = orchestrion.Enabled()
	if orchestrion.Version != "" {
		c.orchestrionCfg.Metadata = &orchestrionMetadata{Version: orchestrion.Version}
	}

	c.sampler = NewAllSampler()
	c.httpClientTimeout = time.Second * 10 // 10 seconds

	if v := env.Get("OTEL_LOGS_EXPORTER"); v != "" {
		log.Warn("OTEL_LOGS_EXPORTER is not supported")
	}
	if internal.BoolEnv("DD_TRACE_ANALYTICS_ENABLED", false) {
		globalconfig.SetAnalyticsRate(1.0)
	}
	if c.internalConfig.ReportHostname() {
		if err := c.internalConfig.HostnameLookupError(); err != nil {
			return c, fmt.Errorf("unable to look up hostname: %s", err.Error())
		}
	}
	if v := getDDorOtelConfig("service"); v != "" {
		c.serviceName = v
		globalconfig.SetServiceName(v)
	}
	c.headerAsTags = newDynamicConfig("trace_header_tags", nil, setHeaderTags, equalSlice[string])
	if v := env.Get("DD_TRACE_HEADER_TAGS"); v != "" {
		c.headerAsTags.update(strings.Split(v, ","), telemetry.OriginEnvVar)
		// Required to ensure that the startup header tags are set on reset.
		c.headerAsTags.startup = c.headerAsTags.current
	}
	if v := getDDorOtelConfig("resourceAttributes"); v != "" {
		tags := internal.ParseTagString(v)
		internal.CleanGitMetadataTags(tags)
		for key, val := range tags {
			WithGlobalTag(key, val)(c)
		}
		// TODO: should we track the origin of these tags individually?
		c.globalTags.cfgOrigin = telemetry.OriginEnvVar
	}
	c.enabled = newDynamicConfig("tracing_enabled", internal.BoolVal(getDDorOtelConfig("enabled"), true), func(_ bool) bool { return true }, equal[bool])
	if _, ok := env.Lookup("DD_TRACE_ENABLED"); ok {
		c.enabled.cfgOrigin = telemetry.OriginEnvVar
	}
	if compatMode := env.Get("DD_TRACE_CLIENT_HOSTNAME_COMPAT"); compatMode != "" {
		if semver.IsValid(compatMode) {
			c.enableHostnameDetection = semver.Compare(semver.MajorMinor(compatMode), "v1.66") <= 0
		} else {
			log.Warn("ignoring DD_TRACE_CLIENT_HOSTNAME_COMPAT, invalid version %q", compatMode)
		}
	}

	dynamicInstrumentationEnabledDefault, origin, _ := stableconfig.Bool("DD_DYNAMIC_INSTRUMENTATION_ENABLED", false)
	c.dynamicInstrumentationEnabled = newDynamicConfig(
		"dynamic_instrumentation_enabled",
		dynamicInstrumentationEnabledDefault,
		func(bool) bool {
			// NOTE: the side effects of changes are performed in onRemoteConfigUpdate.
			return true
		}, /* apply */
		equal[bool],
	)
	c.dynamicInstrumentationEnabled.cfgOrigin = origin

	namingschema.LoadFromEnv()
	c.spanAttributeSchemaVersion = int(namingschema.GetVersion())

	// peer.service tag default calculation is enabled by default if using attribute schema >= 1
	c.peerServiceDefaultsEnabled = true
	if c.spanAttributeSchemaVersion == int(namingschema.SchemaV0) {
		c.peerServiceDefaultsEnabled = internal.BoolEnv("DD_TRACE_PEER_SERVICE_DEFAULTS_ENABLED", false)
	}
	c.peerServiceMappings = make(map[string]string)
	if v := env.Get("DD_TRACE_PEER_SERVICE_MAPPING"); v != "" {
		internal.ForEachStringTag(v, internal.DDTagsDelimiter, func(key, val string) { c.peerServiceMappings[key] = val })
	}

	// LLM Observability config
	c.llmobs = llmobsconfig.Config{
		Enabled:          internal.BoolEnv(envLLMObsEnabled, false),
		MLApp:            env.Get(envLLMObsMlApp),
		AgentlessEnabled: llmobsAgentlessEnabledFromEnv(),
		ProjectName:      env.Get(envLLMObsProjectName),
	}
	for _, fn := range opts {
		if fn == nil {
			continue
		}
		fn(c)
	}
	if c.agentURL == nil {
		c.agentURL = internal.AgentURLFromEnv()
	}
	c.originalAgentURL = c.agentURL // Preserve the original agent URL for logging
	if c.httpClient == nil || orchestrion.Enabled() {
		if orchestrion.Enabled() && c.httpClient != nil {
			// Make sure we don't create http client traces from inside the tracer by using our http client
			// TODO(eliott.bouhana): remove once dd:no-span is implemented
			log.Debug("Orchestrion is enabled, but a custom HTTP client was provided to tracer.Start. This is not supported and will be ignored.")
		}
		if c.agentURL.Scheme == "unix" {
			// If we're connecting over UDS we can just rely on the agent to provide the hostname
			log.Debug("connecting to agent over unix, do not set hostname on any traces")
			c.httpClient = internal.UDSClient(c.agentURL.Path, cmp.Or(c.httpClientTimeout, defaultHTTPTimeout))
			c.agentURL = internal.UnixDataSocketURL(c.agentURL.Path)
		} else {
			c.httpClient = internal.DefaultHTTPClient(c.httpClientTimeout, false)
		}
	}
	WithGlobalTag(ext.RuntimeID, globalconfig.RuntimeID())(c)
	globalTags := c.globalTags.get()
	if c.internalConfig.Env() == "" {
		if v, ok := globalTags["env"]; ok {
			if e, ok := v.(string); ok {
				c.internalConfig.SetEnv(e, c.globalTags.cfgOrigin)
			}
		}
	}
	if c.internalConfig.Version() == "" {
		if v, ok := globalTags["version"]; ok {
			if ver, ok := v.(string); ok {
				c.internalConfig.SetVersion(ver, c.globalTags.cfgOrigin)
			}
		}
	}
	if c.serviceName == "" {
		if v, ok := globalTags["service"]; ok {
			if s, ok := v.(string); ok {
				c.serviceName = s
				globalconfig.SetServiceName(s)
			}
		} else {
			// There is not an explicit service set, default to binary name.
			// In this case, don't set a global service name so the contribs continue using their defaults.
			c.serviceName = filepath.Base(os.Args[0])
		}
	}
	if c.transport == nil {
		c.transport = newHTTPTransport(c.agentURL.String(), c.httpClient)
	}
	if c.propagator == nil {
		envKey := "DD_TRACE_X_DATADOG_TAGS_MAX_LENGTH"
		maxLen := internal.IntEnv(envKey, defaultMaxTagsHeaderLen)
		if maxLen < 0 {
			log.Warn("Invalid value %d for %s. Setting to 0.", maxLen, envKey)
			maxLen = 0
		}
		if maxLen > maxPropagatedTagsLength {
			log.Warn("Invalid value %d for %s. Maximum allowed is %d. Setting to %d.", maxLen, envKey, maxPropagatedTagsLength, maxPropagatedTagsLength)
			maxLen = maxPropagatedTagsLength
		}
		c.propagator = NewPropagator(&PropagatorConfig{
			MaxTagsHeaderLen: maxLen,
		})
	}
	if c.logger != nil {
		log.UseLogger(c.logger)
	}
	if c.internalConfig.Debug() {
		log.SetLevel(log.LevelDebug)
	}
	// Check if CI Visibility mode is enabled
	if c.internalConfig.CIVisibilityEnabled() {
		c.httpClientTimeout = time.Second * 45                                 // Increase timeout up to 45 seconds (same as other tracers in CIVis mode)
		c.internalConfig.SetLogStartup(false, internalconfig.OriginCalculated) // If we are in CI Visibility mode we don't want to log the startup to stdout to avoid polluting the output
		ciTransport := newCiVisibilityTransport(c)                             // Create a default CI Visibility Transport
		c.transport = ciTransport                                              // Replace the default transport with the CI Visibility transport
		c.ciVisibilityAgentless = ciTransport.agentless
		c.ciVisibilityNoopTracer = internal.BoolEnv(constants.CIVisibilityUseNoopTracer, false)
	}

	// if using stdout or traces are disabled or we are in ci visibility agentless mode, agent is disabled
	agentDisabled := c.internalConfig.LogToStdout() || !c.enabled.current || c.ciVisibilityAgentless
	c.agent = loadAgentFeatures(agentDisabled, c.agentURL, c.httpClient)
	if c.agent.v1ProtocolAvailable {
		c.traceProtocol = traceProtocolV1
		if t, ok := c.transport.(*httpTransport); ok {
			t.traceURL = fmt.Sprintf("%s%s", c.agentURL.String(), tracesAPIPathV1)
		}
	} else {
		c.traceProtocol = traceProtocolV04
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		c.loadContribIntegrations([]*debug.Module{})
	} else {
		c.loadContribIntegrations(info.Deps)
	}
	if c.statsdClient == nil {
		// configure statsd client
		addr := resolveDogstatsdAddr(c)
		globalconfig.SetDogstatsdAddr(addr)
		c.dogstatsdAddr = addr
	}
	// Re-initialize the globalTags config with the value constructed from the environment and start options
	// This allows persisting the initial value of globalTags for future resets and updates.
	globalTagsOrigin := c.globalTags.cfgOrigin
	c.initGlobalTags(c.globalTags.get(), globalTagsOrigin)
	if tracingEnabled, _, _ := stableconfig.Bool("DD_APM_TRACING_ENABLED", true); !tracingEnabled {
		apmTracingDisabled(c)
	}
	// Update the llmobs config with stuff needed from the tracer.
	c.llmobs.TracerConfig = llmobsconfig.TracerConfig{
		DDTags:     c.globalTags.get(),
		Env:        c.internalConfig.Env(),
		Service:    c.serviceName,
		Version:    c.internalConfig.Version(),
		AgentURL:   c.agentURL,
		APIKey:     env.Get("DD_API_KEY"),
		APPKey:     env.Get("DD_APP_KEY"),
		HTTPClient: c.httpClient,
		Site:       env.Get("DD_SITE"),
	}
	c.llmobs.AgentFeatures = llmobsconfig.AgentFeatures{
		EVPProxyV2: c.agent.evpProxyV2,
	}

	return c, nil
}

func llmobsAgentlessEnabledFromEnv() *bool {
	v, ok := internal.BoolEnvNoDefault(envLLMObsAgentlessEnabled)
	if !ok {
		return nil
	}
	return &v
}

func apmTracingDisabled(c *config) {
	// Enable tracing as transport layer mode
	// This means to stop sending trace metrics, send one trace per minute and those force-kept by other products
	// using the tracer as transport layer for their data. And finally adding the _dd.apm.enabled=0 tag to all traces
	// to let the backend know that it needs to keep APM UI disabled.
	c.internalConfig.SetGlobalSampleRate(1.0, internalconfig.OriginCalculated)
	c.internalConfig.SetTraceRateLimitPerSecond(1.0/60, internalconfig.OriginCalculated)
	c.tracingAsTransport = true
	WithGlobalTag("_dd.apm.enabled", 0)(c)
	// Disable runtime metrics. In `tracingAsTransport` mode, we'll still
	// tell the agent we computed them, so it doesn't do it either.
	c.internalConfig.SetRuntimeMetricsEnabled(false, internalconfig.OriginCalculated)
	c.internalConfig.SetRuntimeMetricsV2Enabled(false, internalconfig.OriginCalculated)
}

// resolveDogstatsdAddr resolves the Dogstatsd address to use, based on the user-defined
// address and the agent-reported port. If the agent reports a port, it will be used
// instead of the user-defined address' port. UDS paths are honored regardless of the
// agent-reported port.
func resolveDogstatsdAddr(c *config) string {
	addr := c.dogstatsdAddr
	if addr == "" {
		// no config defined address; use host and port from env vars
		// or default to localhost:8125 if not set
		addr = defaultDogstatsdAddr()
	}
	agentport := c.agent.StatsdPort
	if agentport == 0 {
		// the agent didn't report a port; use the already resolved address as
		// features are loaded from the trace-agent, which might be not running
		return addr
	}
	// the agent reported a port
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// parsing the address failed; use the already resolved address as is
		return addr
	}
	if host == "unix" {
		// no need to change the address because it's a UDS connection
		// and these don't have ports
		return addr
	}
	if host == "" {
		// no host was provided; use the default hostname
		host = defaultHostname
	}
	// use agent-reported address if it differs from the user-defined TCP-based protocol URI
	// we have a valid host:port address; replace the port because the agent knows better
	addr = net.JoinHostPort(host, strconv.Itoa(agentport))
	return addr
}

func newStatsdClient(c *config) (internal.StatsdClient, error) {
	if c.statsdClient != nil {
		return c.statsdClient, nil
	}
	return internal.NewStatsdClient(c.dogstatsdAddr, statsTags(c))
}

// defaultDogstatsdAddr returns the default connection address for Dogstatsd.
func defaultDogstatsdAddr() string {
	envHost, envPort := env.Get("DD_DOGSTATSD_HOST"), env.Get("DD_DOGSTATSD_PORT")
	if envHost == "" {
		envHost = env.Get("DD_AGENT_HOST")
	}
	if _, err := os.Stat(defaultSocketDSD); err == nil && envHost == "" && envPort == "" {
		// socket exists and user didn't specify otherwise via env vars
		return "unix://" + defaultSocketDSD
	}
	host, port := defaultHostname, defaultStatsdPort
	if envHost != "" {
		host = envHost
	}
	if envPort != "" {
		port = envPort
	}
	return net.JoinHostPort(host, port)
}

type integrationConfig struct {
	Instrumented bool   `json:"instrumented"`      // indicates if the user has imported and used the integration
	Available    bool   `json:"available"`         // indicates if the user is using a library that can be used with DataDog integrations
	Version      string `json:"available_version"` // if available, indicates the version of the library the user has
}

// agentFeatures holds information about the trace-agent's capabilities.
// When running WithLambdaMode, a zero-value of this struct will be used
// as features.
type agentFeatures struct {
	// DropP0s reports whether it's ok for the tracer to not send any
	// P0 traces to the agent.
	DropP0s bool

	// Stats reports whether the agent can receive client-computed stats on
	// the /v0.6/stats endpoint.
	Stats bool

	// StatsdPort specifies the Dogstatsd port as provided by the agent.
	// If it's the default, it will be 0, which means 8125.
	StatsdPort int

	// featureFlags specifies all the feature flags reported by the trace-agent.
	featureFlags map[string]struct{}

	// peerTags specifies precursor tags to aggregate stats on when client stats is enabled
	peerTags []string

	// defaultEnv is the trace-agent's default env, used for stats calculation if no env override is present
	defaultEnv string

	// metaStructAvailable reports whether the trace-agent can receive spans with the `meta_struct` field.
	metaStructAvailable bool

	// obfuscationVersion reports the trace-agent's version of obfuscation logic. A value of 0 means this field wasn't present.
	obfuscationVersion int

	// spanEvents reports whether the trace-agent can receive spans with the `span_events` field.
	spanEventsAvailable bool

	// evpProxyV2 reports if the trace-agent can receive payloads on the /evp_proxy/v2 endpoint.
	evpProxyV2 bool

	// v1ProtocolAvailable reports whether the trace-agent and tracer are configured to use the v1 protocol.
	v1ProtocolAvailable bool
}

// HasFlag reports whether the agent has set the feat feature flag.
func (a *agentFeatures) HasFlag(feat string) bool {
	_, ok := a.featureFlags[feat]
	return ok
}

// loadAgentFeatures queries the trace-agent for its capabilities and updates
// the tracer's behaviour.
func loadAgentFeatures(agentDisabled bool, agentURL *url.URL, httpClient *http.Client) (features agentFeatures) {
	if agentDisabled {
		// there is no agent; all features off
		return
	}
	resp, err := httpClient.Get(fmt.Sprintf("%s/info", agentURL))
	if err != nil {
		log.Error("Loading features: %s", err.Error())
		return
	}
	if resp.StatusCode == http.StatusNotFound {
		// agent is older than 7.28.0, features not discoverable
		return
	}
	defer resp.Body.Close()
	type infoResponse struct {
		Endpoints          []string `json:"endpoints"`
		ClientDropP0s      bool     `json:"client_drop_p0s"`
		FeatureFlags       []string `json:"feature_flags"`
		PeerTags           []string `json:"peer_tags"`
		SpanMetaStruct     bool     `json:"span_meta_structs"`
		ObfuscationVersion int      `json:"obfuscation_version"`
		SpanEvents         bool     `json:"span_events"`
		Config             struct {
			StatsdPort int    `json:"statsd_port"`
			DefaultEnv string `json:"default_env"`
		} `json:"config"`
	}

	var info infoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		log.Error("Decoding features: %s", err.Error())
		return
	}

	features.DropP0s = info.ClientDropP0s
	features.StatsdPort = info.Config.StatsdPort
	features.defaultEnv = info.Config.DefaultEnv
	features.metaStructAvailable = info.SpanMetaStruct
	features.peerTags = info.PeerTags
	features.obfuscationVersion = info.ObfuscationVersion
	features.spanEventsAvailable = info.SpanEvents
	for _, endpoint := range info.Endpoints {
		switch endpoint {
		case "/v0.6/stats":
			features.Stats = true
		case "/evp_proxy/v2/":
			features.evpProxyV2 = true
		case "/v1.0/traces":
			// Set the trace protocol to use.
			if internal.BoolEnv("DD_TRACE_V1_PAYLOAD_FORMAT_ENABLED", false) {
				features.v1ProtocolAvailable = true
			}
		}
	}
	features.featureFlags = make(map[string]struct{}, len(info.FeatureFlags))
	for _, flag := range info.FeatureFlags {
		features.featureFlags[flag] = struct{}{}
	}
	return features
}

// MarkIntegrationImported labels the given integration as imported
func MarkIntegrationImported(integration string) bool {
	s, ok := contribIntegrations[integration]
	if !ok {
		return false
	}
	s.imported = true
	contribIntegrations[integration] = s
	return true
}

func (c *config) loadContribIntegrations(deps []*debug.Module) {
	integrations := map[string]integrationConfig{}
	for _, s := range contribIntegrations {
		integrations[s.name] = integrationConfig{
			Instrumented: s.imported,
		}
	}
	for _, d := range deps {
		p := d.Path
		s, ok := contribIntegrations[p]
		if !ok {
			continue
		}
		conf := integrations[s.name]
		conf.Available = true
		conf.Version = d.Version
		integrations[s.name] = conf
	}
	c.integrations = integrations
}

// canComputeStats determines whether Client-Side Stats can be computed, which requires:
// - 'client_drop_p0' is enabled
// - Trace Agent exposes 'stats' endpoint
// - Stats Computation is enabled on the tracer (or has 'discovery' FF)
func (c *config) canComputeStats() bool {
	return c.agent.Stats && c.agent.DropP0s && (c.internalConfig.HasFeature("discovery") || c.internalConfig.StatsComputationEnabled())
}

// canDropP0s determines whether P0 spans can be dropped.
// Currently equivalent to canComputeStats() as both capabilities are part
// of the Client-Side Stats feature and cannot be enabled independently.
func (c *config) canDropP0s() bool {
	return c.canComputeStats()
}

func statsTags(c *config) []string {
	tags := []string{
		"lang:go",
		"lang_version:" + runtime.Version(),
	}
	if c.internalConfig.Env() != "" {
		tags = append(tags, "env:"+c.internalConfig.Env())
	}
	if hostname := c.internalConfig.Hostname(); hostname != "" {
		tags = append(tags, "host:"+hostname)
	}
	for k, v := range c.globalTags.get() {
		if vstr, ok := v.(string); ok {
			tags = append(tags, k+":"+vstr)
		}
	}
	globalconfig.SetStatsTags(tags)
	tags = append(tags, "tracer_version:"+version.Tag)
	if c.serviceName != "" {
		tags = append(tags, "service:"+c.serviceName)
	}
	return tags
}

// withNoopStats is used for testing to disable statsd client
func withNoopStats() StartOption {
	return func(c *config) {
		c.statsdClient = &statsd.NoOpClientDirect{}
	}
}

// WithAppSecEnabled specifies whether AppSec features should be activated
// or not.
//
// By default, AppSec features are enabled if `DD_APPSEC_ENABLED` is set to a
// truthy value; and may be enabled by remote configuration if
// `DD_APPSEC_ENABLED` is not set at all.
//
// Using this option to explicitly disable appsec also prevents it from being
// remote activated.
func WithAppSecEnabled(enabled bool) StartOption {
	mode := appsecconfig.ForcedOff
	if enabled {
		mode = appsecconfig.ForcedOn
	}
	return func(c *config) {
		c.appsecStartOptions = append(c.appsecStartOptions, appsecconfig.WithEnablementMode(mode))
	}
}

// WithFeatureFlags specifies a set of feature flags to enable. Please take into account
// that most, if not all features flags are considered to be experimental and result in
// unexpected bugs.
func WithFeatureFlags(feats ...string) StartOption {
	return func(c *config) {
		c.internalConfig.SetFeatureFlags(feats, telemetry.OriginCode)
		log.Info("FEATURES enabled: %s", feats)
	}
}

// WithLogger sets logger as the tracer's error printer.
// Diagnostic and startup tracer logs are prefixed to simplify the search within logs.
// If JSON logging format is required, it's possible to wrap tracer logs using an existing JSON logger with this
// function. To learn more about this possibility, please visit: https://github.com/DataDog/dd-trace-go/issues/2152#issuecomment-1790586933
func WithLogger(logger Logger) StartOption {
	return func(c *config) {
		c.logger = logger
	}
}

// WithDebugStack can be used to globally enable or disable the collection of stack traces when
// spans finish with errors. It is enabled by default. This is a global version of the NoDebugStack
// FinishOption.
func WithDebugStack(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetDebugStack(enabled, internalconfig.OriginCode)
	}
}

// WithDebugMode enables debug mode on the tracer, resulting in more verbose logging.
func WithDebugMode(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetDebug(enabled, telemetry.OriginCode)
	}
}

// WithLambdaMode enables lambda mode on the tracer, for use with AWS Lambda.
// This option is only required if the the Datadog Lambda Extension is not
// running.
func WithLambdaMode(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetLogToStdout(enabled, telemetry.OriginCode)
	}
}

// WithSendRetries enables re-sending payloads that are not successfully
// submitted to the agent.  This will cause the tracer to retry the send at
// most `retries` times.
func WithSendRetries(retries int) StartOption {
	return func(c *config) {
		c.sendRetries = retries
	}
}

// WithRetryInterval sets the interval, in seconds, for retrying submitting payloads to the agent.
func WithRetryInterval(interval int) StartOption {
	return func(c *config) {
		c.internalConfig.SetRetryInterval(time.Duration(interval)*time.Second, telemetry.OriginCode)
	}
}

// WithPropagator sets an alternative propagator to be used by the tracer.
func WithPropagator(p Propagator) StartOption {
	return func(c *config) {
		c.propagator = p
	}
}

// WithService sets the default service name for the program.
func WithService(name string) StartOption {
	return func(c *config) {
		c.serviceName = name
		globalconfig.SetServiceName(c.serviceName)
	}
}

// WithGlobalServiceName causes contrib libraries to use the global service name and not any locally defined service name.
// This is synonymous with `DD_TRACE_REMOVE_INTEGRATION_SERVICE_NAMES_ENABLED`.
func WithGlobalServiceName(enabled bool) StartOption {
	return func(_ *config) {
		namingschema.SetRemoveIntegrationServiceNames(enabled)
	}
}

// WithAgentAddr sets the address where the agent is located. The default is
// localhost:8126. It should contain both host and port.
func WithAgentAddr(addr string) StartOption {
	return func(c *config) {
		c.agentURL = &url.URL{
			Scheme: "http",
			Host:   addr,
		}
	}
}

// WithAgentURL sets the full trace agent URL
func WithAgentURL(agentURL string) StartOption {
	return func(c *config) {
		u, err := url.Parse(agentURL)
		if err != nil {
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				u, _ = url.Parse(urlErr.URL)
				if u != nil {
					urlErr.URL = u.Redacted()
					log.Warn("Fail to parse Agent URL: %s", urlErr.Err)
					return
				}
				log.Warn("Fail to parse Agent URL: %s", err.Error())
				return
			}
			log.Warn("Fail to parse Agent URL")
			return
		}
		switch u.Scheme {
		case "http", "https":
			c.agentURL = &url.URL{
				Scheme: u.Scheme,
				Host:   u.Host,
			}
		case "unix":
			c.agentURL = internal.UnixDataSocketURL(u.Path)
		default:
			log.Warn("Unsupported protocol %q in Agent URL %q. Must be one of: http, https, unix.", u.Scheme, agentURL)
		}
	}
}

// WithAgentTimeout sets the timeout for the agent connection. Timeout is in seconds.
func WithAgentTimeout(timeout int) StartOption {
	return func(c *config) {
		c.httpClientTimeout = time.Duration(timeout) * time.Second
	}
}

// WithEnv sets the environment to which all traces started by the tracer will be submitted.
// The default value is the environment variable DD_ENV, if it is set.
func WithEnv(env string) StartOption {
	return func(c *config) {
		c.internalConfig.SetEnv(env, telemetry.OriginCode)
	}
}

// WithServiceMapping determines service "from" to be renamed to service "to".
// This option is is case sensitive and can be used multiple times.
func WithServiceMapping(from, to string) StartOption {
	return func(c *config) {
		c.internalConfig.SetServiceMapping(from, to, telemetry.OriginCode)
	}
}

// WithPeerServiceDefaults sets default calculation for peer.service.
// Related documentation: https://docs.datadoghq.com/tracing/guide/inferred-service-opt-in/?tab=go#apm-tracer-configuration
func WithPeerServiceDefaults(enabled bool) StartOption {
	return func(c *config) {
		c.peerServiceDefaultsEnabled = enabled
	}
}

// WithPeerServiceMapping determines the value of the peer.service tag "from" to be renamed to service "to".
func WithPeerServiceMapping(from, to string) StartOption {
	return func(c *config) {
		if c.peerServiceMappings == nil {
			c.peerServiceMappings = make(map[string]string)
		}
		c.peerServiceMappings[from] = to
	}
}

// WithGlobalTag sets a key/value pair which will be set as a tag on all spans
// created by tracer. This option may be used multiple times.
func WithGlobalTag(k string, v interface{}) StartOption {
	return func(c *config) {
		if c.globalTags.get() == nil {
			c.initGlobalTags(map[string]interface{}{}, telemetry.OriginDefault)
		}
		c.globalTags.Lock()
		defer c.globalTags.Unlock()
		c.globalTags.current[k] = v
	}
}

// initGlobalTags initializes the globalTags config with the provided init value
func (c *config) initGlobalTags(init map[string]interface{}, origin telemetry.Origin) {
	apply := func(map[string]interface{}) bool {
		// always set the runtime ID on updates
		c.globalTags.current[ext.RuntimeID] = globalconfig.RuntimeID()
		return true
	}
	c.globalTags = newDynamicConfig("trace_tags", init, apply, equalMap[string])
	c.globalTags.cfgOrigin = origin
}

// WithSampler sets the given sampler to be used with the tracer. By default
// an all-permissive sampler is used.
// Deprecated: Use WithSamplerRate instead. Custom sampling will be phased out in a future release.
func WithSampler(s Sampler) StartOption {
	return func(c *config) {
		c.sampler = &customSampler{s: s}
	}
}

// WithSamplerRate sets the given sampler rate to be used with the tracer.
// The rate must be between 0 and 1. By default an all-permissive sampler rate (1) is used.
func WithSamplerRate(rate float64) StartOption {
	return func(c *config) {
		c.sampler = NewRateSampler(rate)
	}
}

// WithHTTPClient specifies the HTTP client to use when emitting spans to the agent.
func WithHTTPClient(client *http.Client) StartOption {
	return func(c *config) {
		c.httpClient = client
	}
}

// WithUDS configures the HTTP client to dial the Datadog Agent via the specified Unix Domain Socket path.
func WithUDS(socketPath string) StartOption {
	return func(c *config) {
		c.agentURL = &url.URL{
			Scheme: "unix",
			Path:   socketPath,
		}
	}
}

// WithAnalytics allows specifying whether Trace Search & Analytics should be enabled
// for integrations.
func WithAnalytics(on bool) StartOption {
	return func(_ *config) {
		if on {
			globalconfig.SetAnalyticsRate(1.0)
		} else {
			globalconfig.SetAnalyticsRate(math.NaN())
		}
	}
}

// WithAnalyticsRate sets the global sampling rate for sampling APM events.
func WithAnalyticsRate(rate float64) StartOption {
	return func(_ *config) {
		if rate >= 0.0 && rate <= 1.0 {
			globalconfig.SetAnalyticsRate(rate)
		} else {
			globalconfig.SetAnalyticsRate(math.NaN())
		}
	}
}

// WithRuntimeMetrics enables automatic collection of runtime metrics every 10 seconds.
func WithRuntimeMetrics() StartOption {
	return func(cfg *config) {
		telemetry.RegisterAppConfig("runtime_metrics_enabled", true, telemetry.OriginCode)
		cfg.internalConfig.SetRuntimeMetricsEnabled(true, internalconfig.OriginCode)
	}
}

// WithDogstatsdAddr specifies the address to connect to for sending metrics to the Datadog
// Agent. It should be a "host:port" string, or the path to a unix domain socket.If not set, it
// attempts to determine the address of the statsd service according to the following rules:
//  1. Look for /var/run/datadog/dsd.socket and use it if present. IF NOT, continue to #2.
//  2. The host is determined by DD_AGENT_HOST, and defaults to "localhost"
//  3. The port is retrieved from the agent. If not present, it is determined by DD_DOGSTATSD_PORT, and defaults to 8125
//
// This option is in effect when WithRuntimeMetrics is enabled.
func WithDogstatsdAddr(addr string) StartOption {
	return func(cfg *config) {
		cfg.dogstatsdAddr = addr
		globalconfig.SetDogstatsdAddr(addr)
	}
}

// WithSamplingRules specifies the sampling rates to apply to spans based on the
// provided rules.
func WithSamplingRules(rules []SamplingRule) StartOption {
	return func(cfg *config) {
		for _, rule := range rules {
			if rule.ruleType == SamplingRuleSpan {
				cfg.spanRules = append(cfg.spanRules, rule)
			} else {
				cfg.traceRules = append(cfg.traceRules, rule)
			}
		}
	}
}

// WithServiceVersion specifies the version of the service that is running. This will
// be included in spans from this service in the "version" tag, provided that
// span service name and config service name match. Do NOT use with WithUniversalVersion.
func WithServiceVersion(version string) StartOption {
	return func(cfg *config) {
		cfg.internalConfig.SetVersion(version, telemetry.OriginCode)
		cfg.universalVersion = false
	}
}

// WithUniversalVersion specifies the version of the service that is running, and will be applied to all spans,
// regardless of whether span service name and config service name match.
// See: WithService, WithServiceVersion. Do NOT use with WithServiceVersion.
func WithUniversalVersion(version string) StartOption {
	return func(c *config) {
		c.internalConfig.SetVersion(version, telemetry.OriginCode)
		c.universalVersion = true
	}
}

// WithHostname allows specifying the hostname with which to mark outgoing traces.
func WithHostname(name string) StartOption {
	return func(c *config) {
		c.internalConfig.SetHostname(name, telemetry.OriginCode)
	}
}

// WithTraceEnabled allows specifying whether tracing will be enabled
func WithTraceEnabled(enabled bool) StartOption {
	return func(c *config) {
		telemetry.RegisterAppConfig("trace_enabled", enabled, telemetry.OriginCode)
		c.enabled = newDynamicConfig("tracing_enabled", enabled, func(_ bool) bool { return true }, equal[bool])
	}
}

// WithLogStartup allows enabling or disabling the startup log.
func WithLogStartup(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetLogStartup(enabled, internalconfig.OriginCode)
	}
}

// WithProfilerCodeHotspots enables the code hotspots integration between the
// tracer and profiler. This is done by automatically attaching pprof labels
// called "span id" and "local root span id" when new spans are created. You
// should not use these label names in your own code when this is enabled. The
// enabled value defaults to the value of the
// DD_PROFILING_CODE_HOTSPOTS_COLLECTION_ENABLED env variable or true.
func WithProfilerCodeHotspots(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetProfilerHotspotsEnabled(enabled, telemetry.OriginCode)
	}
}

// WithProfilerEndpoints enables the endpoints integration between the tracer
// and profiler. This is done by automatically attaching a pprof label called
// "trace endpoint" holding the resource name of the top-level service span if
// its type is "http", "rpc" or "" (default). You should not use this label
// name in your own code when this is enabled. The enabled value defaults to
// the value of the DD_PROFILING_ENDPOINT_COLLECTION_ENABLED env variable or
// true.
func WithProfilerEndpoints(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetProfilerEndpoints(enabled, telemetry.OriginCode)
	}
}

// WithDebugSpansMode enables debugging old spans that may have been
// abandoned, which may prevent traces from being set to the Datadog
// Agent, especially if partial flushing is off.
// This setting can also be configured by setting DD_TRACE_DEBUG_ABANDONED_SPANS
// to true. The timeout will default to 10 minutes, unless overwritten
// by DD_TRACE_ABANDONED_SPAN_TIMEOUT.
// This feature is disabled by default. Turning on this debug mode may
// be expensive, so it should only be enabled for debugging purposes.
func WithDebugSpansMode(timeout time.Duration) StartOption {
	return func(c *config) {
		c.internalConfig.SetDebugAbandonedSpans(true, internalconfig.OriginCode)
		c.internalConfig.SetSpanTimeout(timeout, internalconfig.OriginCode)
	}
}

// WithPartialFlushing enables flushing of partially finished traces.
// This is done after "numSpans" have finished in a single local trace at
// which point all finished spans in that trace will be flushed, freeing up
// any memory they were consuming. This can also be configured by setting
// DD_TRACE_PARTIAL_FLUSH_ENABLED to true, which will default to 1000 spans
// unless overriden with DD_TRACE_PARTIAL_FLUSH_MIN_SPANS. Partial flushing
// is disabled by default.
func WithPartialFlushing(numSpans int) StartOption {
	return func(c *config) {
		c.internalConfig.SetPartialFlushEnabled(true, internalconfig.OriginCode)
		c.internalConfig.SetPartialFlushMinSpans(numSpans, internalconfig.OriginCode)
	}
}

// WithStatsComputation enables client-side stats computation, allowing
// the tracer to compute stats from traces. This can reduce network traffic
// to the Datadog Agent, and produce more accurate stats data.
// This can also be configured by setting DD_TRACE_STATS_COMPUTATION_ENABLED to true.
// Client-side stats is off by default.
func WithStatsComputation(enabled bool) StartOption {
	return func(c *config) {
		c.internalConfig.SetStatsComputationEnabled(enabled, internalconfig.OriginCode)
	}
}

// WithDynamicInstrumentationEnabled enables or disables dynamic
// instrumentation, allowing the tracer to place probes for the Live Debugger
// and Dynamic Instrumentation products.
func WithDynamicInstrumentationEnabled(enabled bool) StartOption {
	return func(c *config) {
		apply := func(bool) bool {
			// NOTE: the side effects of changes are performed in onRemoteConfigUpdate.
			return true
		}
		c.dynamicInstrumentationEnabled = newDynamicConfig("dynamic_instrumentation_enabled", enabled, apply, equal[bool])
		c.dynamicInstrumentationEnabled.cfgOrigin = telemetry.OriginCode
	}
}

// Tag sets the given key/value pair as a tag on the started Span.
func Tag(k string, v interface{}) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		if cfg.Tags == nil {
			cfg.Tags = map[string]interface{}{}
		}
		cfg.Tags[k] = v
	}
}

// ServiceName sets the given service name on the started span. For example "http.server".
func ServiceName(name string) StartSpanOption {
	return Tag(ext.ServiceName, name)
}

// ResourceName sets the given resource name on the started span. A resource could
// be an SQL query, a URL, an RPC method or something else.
func ResourceName(name string) StartSpanOption {
	return Tag(ext.ResourceName, name)
}

// SpanType sets the given span type on the started span. Some examples in the case of
// the Datadog APM product could be "web", "db" or "cache".
func SpanType(name string) StartSpanOption {
	return Tag(ext.SpanType, name)
}

// WithSpanLinks sets span links on the started span.
func WithSpanLinks(links []SpanLink) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		cfg.SpanLinks = append(cfg.SpanLinks, links...)
	}
}

var measuredTag = Tag(keyMeasured, 1)

// Measured marks this span to be measured for metrics and stats calculations.
func Measured() StartSpanOption {
	// cache a global instance of this tag: saves one alloc/call
	return measuredTag
}

// WithSpanID sets the SpanID on the started span, instead of using a random number.
// If there is no parent Span (eg from ChildOf), then the TraceID will also be set to the
// value given here.
func WithSpanID(id uint64) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		cfg.SpanID = id
	}
}

// ChildOf tells StartSpan to use the given span context as a parent for the created span.
//
// Deprecated: Use [Span.StartChild] instead.
func ChildOf(ctx *SpanContext) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		cfg.Parent = ctx
	}
}

// withContext associates the ctx with the span.
func withContext(ctx context.Context) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		cfg.Context = ctx
	}
}

// StartTime sets a custom time as the start time for the created span. By
// default a span is started using the creation time.
func StartTime(t time.Time) StartSpanOption {
	return func(cfg *StartSpanConfig) {
		cfg.StartTime = t
	}
}

// AnalyticsRate sets a custom analytics rate for a span. It decides the percentage
// of events that will be picked up by the App Analytics product. It's represents a
// float64 between 0 and 1 where 0.5 would represent 50% of events.
func AnalyticsRate(rate float64) StartSpanOption {
	if math.IsNaN(rate) {
		return func(_ *StartSpanConfig) {}
	}
	return Tag(ext.EventSampleRate, rate)
}

// WithStartSpanConfig merges the given StartSpanConfig into the one used to start the span.
// It is useful when you want to set a common base config, reducing the number of function calls in hot loops.
func WithStartSpanConfig(cfg *StartSpanConfig) StartSpanOption {
	return func(c *StartSpanConfig) {
		// copy cfg into c only if cfg fields are not zero values
		// c fields have precedence, as they may have been set up before running this option
		if c.SpanID == 0 {
			c.SpanID = cfg.SpanID
		}
		if c.Parent == nil {
			c.Parent = cfg.Parent
		}
		if c.Context == nil {
			c.Context = cfg.Context
		}
		if c.SpanLinks == nil {
			c.SpanLinks = cfg.SpanLinks
		}
		if c.StartTime.IsZero() {
			c.StartTime = cfg.StartTime
		}
		// tags are a special case, as we need to merge them
		if c.Tags == nil {
			// if cfg.Tags is nil, this is a no-op
			c.Tags = cfg.Tags
		} else if cfg.Tags != nil {
			for k, v := range cfg.Tags {
				c.Tags[k] = v
			}
		}
	}
}

// WithHeaderTags enables the integration to attach HTTP request headers as span tags.
// Warning:
// Using this feature can risk exposing sensitive data such as authorization tokens to Datadog.
// Special headers can not be sub-selected. E.g., an entire Cookie header would be transmitted, without the ability to choose specific Cookies.
func WithHeaderTags(headerAsTags []string) StartOption {
	return func(c *config) {
		c.headerAsTags = newDynamicConfig("trace_header_tags", headerAsTags, setHeaderTags, equalSlice[string])
		setHeaderTags(headerAsTags)
	}
}

// WithTestDefaults configures the tracer to not send spans to the agent, and to not collect metrics.
// Warning:
// This option should only be used in tests, as it will prevent the tracer from sending spans to the agent.
func WithTestDefaults(statsdClient any) StartOption {
	return func(c *config) {
		if statsdClient == nil {
			statsdClient = &statsd.NoOpClientDirect{}
		}
		c.statsdClient = statsdClient.(internal.StatsdClient)
		c.transport = newDummyTransport()
	}
}

// WithLLMObsEnabled allows to enable LLM Observability (it is disabled by default).
// This is equivalent to the DD_LLMOBS_ENABLED environment variable.
func WithLLMObsEnabled(enabled bool) StartOption {
	return func(c *config) {
		c.llmobs.Enabled = enabled
	}
}

// WithLLMObsMLApp allows to configure the default ML App for LLM Observability.
// It is required to have this configured to use any LLM Observability features.
// This is equivalent to the DD_LLMOBS_ML_APP environment variable.
func WithLLMObsMLApp(mlApp string) StartOption {
	return func(c *config) {
		c.llmobs.MLApp = mlApp
	}
}

// WithLLMObsProjectName allows to configure the default LLM Observability project to use.
// It is required when using the Experiments and Datasets feature.
// This is equivalent to the DD_LLMOBS_PROJECT_NAME environment variable.
func WithLLMObsProjectName(projectName string) StartOption {
	return func(c *config) {
		c.llmobs.ProjectName = projectName
	}
}

// WithLLMObsAgentlessEnabled allows to configure LLM Observability to work in agent/agentless mode.
// The default is using the agent if it is available and supports it, otherwise it will default to agentless mode.
// Please note when using agentless mode, a valid DD_API_KEY must also be set.
// This is equivalent to the DD_LLMOBS_AGENTLESS_ENABLED environment variable.
func WithLLMObsAgentlessEnabled(agentlessEnabled bool) StartOption {
	return func(c *config) {
		c.llmobs.AgentlessEnabled = &agentlessEnabled
	}
}

// Mock Transport with a real Encoder
type dummyTransport struct {
	sync.RWMutex
	traces     spanLists
	stats      []*pb.ClientStatsPayload
	obfVersion int
}

func newDummyTransport() *dummyTransport {
	return &dummyTransport{traces: spanLists{}, obfVersion: -1}
}

func (t *dummyTransport) Len() int {
	t.RLock()
	defer t.RUnlock()
	return len(t.traces)
}

func (t *dummyTransport) sendStats(p *pb.ClientStatsPayload, obfVersion int) error {
	t.Lock()
	t.stats = append(t.stats, p)
	t.obfVersion = obfVersion
	t.Unlock()
	return nil
}

func (t *dummyTransport) Stats() []*pb.ClientStatsPayload {
	t.RLock()
	defer t.RUnlock()
	return t.stats
}

func (t *dummyTransport) ObfuscationVersion() int {
	t.RLock()
	defer t.RUnlock()
	return t.obfVersion
}

func (t *dummyTransport) send(p payload) (io.ReadCloser, error) {
	traces, err := decode(p)
	if err != nil {
		return nil, err
	}
	t.Lock()
	t.traces = append(t.traces, traces...)
	t.Unlock()
	ok := io.NopCloser(strings.NewReader("OK"))
	return ok, nil
}

func (t *dummyTransport) endpoint() string {
	return "http://localhost:9/v0.4/traces"
}

func decode(p payloadReader) (spanLists, error) {
	var traces spanLists
	err := msgp.Decode(p, &traces)
	return traces, err
}

func (t *dummyTransport) Reset() {
	t.Lock()
	t.traces = t.traces[:0]
	t.Unlock()
}

func (t *dummyTransport) Traces() spanLists {
	t.Lock()
	defer t.Unlock()

	traces := t.traces
	t.traces = spanLists{}
	return traces
}

// setHeaderTags sets the global header tags.
// Always resets the global value and returns true.
func setHeaderTags(headerAsTags []string) bool {
	globalconfig.ClearHeaderTags()
	for _, h := range headerAsTags {
		header, tag := normalizer.HeaderTag(h)
		if len(header) == 0 || len(tag) == 0 {
			log.Debug("Header-tag input is in unsupported format; dropping input value %q", h)
			continue
		}
		globalconfig.SetHeaderTag(header, tag)
	}
	return true
}

// UserMonitoringConfig is used to configure what is used to identify a user.
// This configuration can be set by combining one or several UserMonitoringOption with a call to SetUser().
type UserMonitoringConfig struct {
	PropagateID bool
	Login       string
	Org         string
	Email       string
	Name        string
	Role        string
	SessionID   string
	Scope       string
	Metadata    map[string]string
}

// UserMonitoringOption represents a function that can be provided as a parameter to SetUser.
type UserMonitoringOption func(*UserMonitoringConfig)

// WithUserMetadata returns the option setting additional metadata of the authenticated user.
// This can be used multiple times and the given data will be tracked as `usr.{key}=value`.
func WithUserMetadata(key, value string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Metadata[key] = value
	}
}

// WithUserLogin returns the option setting the login of the authenticated user.
func WithUserLogin(login string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Login = login
	}
}

// WithUserOrg returns the option setting the organization of the authenticated user.
func WithUserOrg(org string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Org = org
	}
}

// WithUserEmail returns the option setting the email of the authenticated user.
func WithUserEmail(email string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Email = email
	}
}

// WithUserName returns the option setting the name of the authenticated user.
func WithUserName(name string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Name = name
	}
}

// WithUserSessionID returns the option setting the session ID of the authenticated user.
func WithUserSessionID(sessionID string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.SessionID = sessionID
	}
}

// WithUserRole returns the option setting the role of the authenticated user.
func WithUserRole(role string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Role = role
	}
}

// WithUserScope returns the option setting the scope (authorizations) of the authenticated user.
func WithUserScope(scope string) UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.Scope = scope
	}
}

// WithPropagation returns the option allowing the user id to be propagated through distributed traces.
// The user id is base64 encoded and added to the datadog propagated tags header.
// This option should only be used if you are certain that the user id passed to `SetUser()` does not contain any
// personal identifiable information or any kind of sensitive data, as it will be leaked to other services.
func WithPropagation() UserMonitoringOption {
	return func(cfg *UserMonitoringConfig) {
		cfg.PropagateID = true
	}
}
