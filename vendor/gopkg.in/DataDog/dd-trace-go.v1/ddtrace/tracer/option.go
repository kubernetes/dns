// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tracer

import (
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/internal"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/globalconfig"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/version"

	"github.com/DataDog/datadog-go/statsd"
)

// config holds the tracer configuration.
type config struct {
	// debug, when true, writes details to logs.
	debug bool

	// lambda, when true, enables the lambda trace writer
	logToStdout bool

	// logStartup, when true, causes various startup info to be written
	// when the tracer starts.
	logStartup bool

	// serviceName specifies the name of this application.
	serviceName string

	// version specifies the version of this application
	version string

	// env contains the environment that this application will run under.
	env string

	// sampler specifies the sampler that will be used for sampling traces.
	sampler Sampler

	// agentAddr specifies the hostname and port of the agent where the traces
	// are sent to.
	agentAddr string

	// globalTags holds a set of tags that will be automatically applied to
	// all spans.
	globalTags map[string]interface{}

	// transport specifies the Transport interface which will be used to send data to the agent.
	transport transport

	// propagator propagates span context cross-process
	propagator Propagator

	// httpClient specifies the HTTP client to be used by the agent's transport.
	httpClient *http.Client

	// hostname is automatically assigned when the DD_TRACE_REPORT_HOSTNAME is set to true,
	// and is added as a special tag to the root span of traces.
	hostname string

	// logger specifies the logger to use when printing errors. If not specified, the "log" package
	// will be used.
	logger ddtrace.Logger

	// runtimeMetrics specifies whether collection of runtime metrics is enabled.
	runtimeMetrics bool

	// dogstatsdAddr specifies the address to connect for sending metrics to the
	// Datadog Agent. If not set, it defaults to "localhost:8125" or to the
	// combination of the environment variables DD_AGENT_HOST and DD_DOGSTATSD_PORT.
	dogstatsdAddr string

	// statsd is used for tracking metrics associated with the runtime and the tracer.
	statsd statsdClient

	// samplingRules contains user-defined rules determine the sampling rate to apply
	// to spans.
	samplingRules []SamplingRule

	// tickChan specifies a channel which will receive the time every time the tracer must flush.
	// It defaults to time.Ticker; replaced in tests.
	tickChan <-chan time.Time

	// noDebugStack disables the collection of debug stack traces globally. No traces reporting
	// errors will record a stack trace when this option is set.
	noDebugStack bool
}

// StartOption represents a function that can be provided as a parameter to Start.
type StartOption func(*config)

// newConfig renders the tracer configuration based on defaults, environment variables
// and passed user opts.
func newConfig(opts ...StartOption) *config {
	c := new(config)
	c.sampler = NewAllSampler()
	c.agentAddr = defaultAddress
	statsdHost, statsdPort := "localhost", "8125"
	if v := os.Getenv("DD_AGENT_HOST"); v != "" {
		statsdHost = v
	}
	if v := os.Getenv("DD_DOGSTATSD_PORT"); v != "" {
		statsdPort = v
	}
	c.dogstatsdAddr = net.JoinHostPort(statsdHost, statsdPort)

	if internal.BoolEnv("DD_TRACE_ANALYTICS_ENABLED", false) {
		globalconfig.SetAnalyticsRate(1.0)
	}
	if os.Getenv("DD_TRACE_REPORT_HOSTNAME") == "true" {
		var err error
		c.hostname, err = os.Hostname()
		if err != nil {
			log.Warn("unable to look up hostname: %v", err)
		}
	}
	if v := os.Getenv("DD_ENV"); v != "" {
		c.env = v
	}
	if v := os.Getenv("DD_SERVICE"); v != "" {
		c.serviceName = v
		globalconfig.SetServiceName(v)
	}
	if ver := os.Getenv("DD_VERSION"); ver != "" {
		c.version = ver
	}
	if v := os.Getenv("DD_TAGS"); v != "" {
		for _, tag := range strings.Split(v, ",") {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			kv := strings.SplitN(tag, ":", 2)
			k := strings.TrimSpace(kv[0])
			switch len(kv) {
			case 1:
				WithGlobalTag(k, "")(c)
			case 2:
				WithGlobalTag(k, strings.TrimSpace(kv[1]))(c)
			}
		}
	}
	if _, ok := os.LookupEnv("AWS_LAMBDA_FUNCTION_NAME"); ok {
		// AWS_LAMBDA_FUNCTION_NAME being set indicates that we're running in an AWS Lambda environment.
		// See: https://docs.aws.amazon.com/lambda/latest/dg/configuration-envvars.html
		c.logToStdout = true
	}
	c.logStartup = internal.BoolEnv("DD_TRACE_STARTUP_LOGS", true)
	c.runtimeMetrics = internal.BoolEnv("DD_RUNTIME_METRICS_ENABLED", false)
	c.debug = internal.BoolEnv("DD_TRACE_DEBUG", false)
	for _, fn := range opts {
		fn(c)
	}
	WithGlobalTag(ext.RuntimeID, globalconfig.RuntimeID())(c)
	if c.env == "" {
		if v, ok := c.globalTags["env"]; ok {
			if e, ok := v.(string); ok {
				c.env = e
			}
		}
	}
	if c.version == "" {
		if v, ok := c.globalTags["version"]; ok {
			if ver, ok := v.(string); ok {
				c.version = ver
			}
		}
	}
	if c.serviceName == "" {
		if v, ok := c.globalTags["service"]; ok {
			if s, ok := v.(string); ok {
				c.serviceName = s
				globalconfig.SetServiceName(s)
			}
		} else {
			c.serviceName = filepath.Base(os.Args[0])
		}
	}
	if c.transport == nil {
		c.transport = newTransport(c.agentAddr, c.httpClient)
	}
	if c.propagator == nil {
		c.propagator = NewPropagator(nil)
	}
	if c.logger != nil {
		log.UseLogger(c.logger)
	}
	if c.debug {
		log.SetLevel(log.LevelDebug)
	}
	if c.statsd == nil {
		client, err := statsd.New(c.dogstatsdAddr, statsd.WithMaxMessagesPerPayload(40), statsd.WithTags(statsTags(c)))
		if err != nil {
			log.Warn("Runtime and health metrics disabled: %v", err)
			c.statsd = &statsd.NoOpClient{}
		} else {
			c.statsd = client
		}
	}
	return c
}

func statsTags(c *config) []string {
	tags := []string{
		"lang:go",
		"version:" + version.Tag,
		"lang_version:" + runtime.Version(),
	}
	if c.serviceName != "" {
		tags = append(tags, "service:"+c.serviceName)
	}
	if c.env != "" {
		tags = append(tags, "env:"+c.env)
	}
	if c.hostname != "" {
		tags = append(tags, "host:"+c.hostname)
	}
	for k, v := range c.globalTags {
		if vstr, ok := v.(string); ok {
			tags = append(tags, k+":"+vstr)
		}
	}
	return tags
}

// WithLogger sets logger as the tracer's error printer.
func WithLogger(logger ddtrace.Logger) StartOption {
	return func(c *config) {
		c.logger = logger
	}
}

// WithPrioritySampling is deprecated, and priority sampling is enabled by default.
// When using distributed tracing, the priority sampling value is propagated in order to
// get all the parts of a distributed trace sampled.
// To learn more about priority sampling, please visit:
// https://docs.datadoghq.com/tracing/getting_further/trace_sampling_and_storage/#priority-sampling-for-distributed-tracing
func WithPrioritySampling() StartOption {
	return func(c *config) {
		// This is now enabled by default.
	}
}

// WithDebugStack can be used to globally enable or disable the collection of stack traces when
// spans finish with errors. It is enabled by default. This is a global version of the NoDebugStack
// FinishOption.
func WithDebugStack(enabled bool) StartOption {
	return func(c *config) {
		c.noDebugStack = !enabled
	}
}

// WithDebugMode enables debug mode on the tracer, resulting in more verbose logging.
func WithDebugMode(enabled bool) StartOption {
	return func(c *config) {
		c.debug = enabled
	}
}

// WithLambdaMode enables lambda mode on the tracer, for use with AWS Lambda.
func WithLambdaMode(enabled bool) StartOption {
	return func(c *config) {
		c.logToStdout = enabled
	}
}

// WithPropagator sets an alternative propagator to be used by the tracer.
func WithPropagator(p Propagator) StartOption {
	return func(c *config) {
		c.propagator = p
	}
}

// WithServiceName is deprecated. Please use WithService.
// If you are using an older version and you are upgrading from WithServiceName
// to WithService, please note that WithService will determine the service name of
// server and framework integrations.
func WithServiceName(name string) StartOption {
	return func(c *config) {
		c.serviceName = name
		if globalconfig.ServiceName() != "" {
			log.Warn("ddtrace/tracer: deprecated config WithServiceName should not be used " +
				"with `WithService` or `DD_SERVICE`; integration service name will not be set.")
		}
		globalconfig.SetServiceName("")
	}
}

// WithService sets the default service name for the program.
func WithService(name string) StartOption {
	return func(c *config) {
		c.serviceName = name
		globalconfig.SetServiceName(c.serviceName)
	}
}

// WithAgentAddr sets the address where the agent is located. The default is
// localhost:8126. It should contain both host and port.
func WithAgentAddr(addr string) StartOption {
	return func(c *config) {
		c.agentAddr = addr
	}
}

// WithEnv sets the environment to which all traces started by the tracer will be submitted.
// The default value is the environment variable DD_ENV, if it is set.
func WithEnv(env string) StartOption {
	return func(c *config) {
		c.env = env
	}
}

// WithGlobalTag sets a key/value pair which will be set as a tag on all spans
// created by tracer. This option may be used multiple times.
func WithGlobalTag(k string, v interface{}) StartOption {
	return func(c *config) {
		if c.globalTags == nil {
			c.globalTags = make(map[string]interface{})
		}
		c.globalTags[k] = v
	}
}

// WithSampler sets the given sampler to be used with the tracer. By default
// an all-permissive sampler is used.
func WithSampler(s Sampler) StartOption {
	return func(c *config) {
		c.sampler = s
	}
}

// WithHTTPRoundTripper is deprecated. Please consider using WithHTTPClient instead.
// The function allows customizing the underlying HTTP transport for emitting spans.
func WithHTTPRoundTripper(r http.RoundTripper) StartOption {
	return WithHTTPClient(&http.Client{
		Transport: r,
		Timeout:   defaultHTTPTimeout,
	})
}

// WithHTTPClient specifies the HTTP client to use when emitting spans to the agent.
func WithHTTPClient(client *http.Client) StartOption {
	return func(c *config) {
		c.httpClient = client
	}
}

// WithAnalytics allows specifying whether Trace Search & Analytics should be enabled
// for integrations.
func WithAnalytics(on bool) StartOption {
	return func(cfg *config) {
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
		cfg.runtimeMetrics = true
	}
}

// WithDogstatsdAddress specifies the address to connect to for sending metrics
// to the Datadog Agent. If not set, it defaults to "localhost:8125" or to the
// combination of the environment variables DD_AGENT_HOST and DD_DOGSTATSD_PORT.
// This option is in effect when WithRuntimeMetrics is enabled.
func WithDogstatsdAddress(addr string) StartOption {
	return func(cfg *config) {
		cfg.dogstatsdAddr = addr
	}
}

// WithSamplingRules specifies the sampling rates to apply to spans based on the
// provided rules.
func WithSamplingRules(rules []SamplingRule) StartOption {
	return func(cfg *config) {
		cfg.samplingRules = rules
	}
}

// WithServiceVersion specifies the version of the service that is running. This will
// be included in spans from this service in the "version" tag.
func WithServiceVersion(version string) StartOption {
	return func(cfg *config) {
		cfg.version = version
	}
}

// StartSpanOption is a configuration option for StartSpan. It is aliased in order
// to help godoc group all the functions returning it together. It is considered
// more correct to refer to it as the type as the origin, ddtrace.StartSpanOption.
type StartSpanOption = ddtrace.StartSpanOption

// Tag sets the given key/value pair as a tag on the started Span.
func Tag(k string, v interface{}) StartSpanOption {
	return func(cfg *ddtrace.StartSpanConfig) {
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

// Measured marks this span to be measured for metrics and stats calculations.
func Measured() StartSpanOption {
	return Tag(keyMeasured, 1)
}

// WithSpanID sets the SpanID on the started span, instead of using a random number.
// If there is no parent Span (eg from ChildOf), then the TraceID will also be set to the
// value given here.
func WithSpanID(id uint64) StartSpanOption {
	return func(cfg *ddtrace.StartSpanConfig) {
		cfg.SpanID = id
	}
}

// ChildOf tells StartSpan to use the given span context as a parent for the
// created span.
func ChildOf(ctx ddtrace.SpanContext) StartSpanOption {
	return func(cfg *ddtrace.StartSpanConfig) {
		cfg.Parent = ctx
	}
}

// StartTime sets a custom time as the start time for the created span. By
// default a span is started using the creation time.
func StartTime(t time.Time) StartSpanOption {
	return func(cfg *ddtrace.StartSpanConfig) {
		cfg.StartTime = t
	}
}

// AnalyticsRate sets a custom analytics rate for a span. It decides the percentage
// of events that will be picked up by the App Analytics product. It's represents a
// float64 between 0 and 1 where 0.5 would represent 50% of events.
func AnalyticsRate(rate float64) StartSpanOption {
	if math.IsNaN(rate) {
		return func(cfg *ddtrace.StartSpanConfig) {}
	}
	return Tag(ext.EventSampleRate, rate)
}

// FinishOption is a configuration option for FinishSpan. It is aliased in order
// to help godoc group all the functions returning it together. It is considered
// more correct to refer to it as the type as the origin, ddtrace.FinishOption.
type FinishOption = ddtrace.FinishOption

// FinishTime sets the given time as the finishing time for the span. By default,
// the current time is used.
func FinishTime(t time.Time) FinishOption {
	return func(cfg *ddtrace.FinishConfig) {
		cfg.FinishTime = t
	}
}

// WithError marks the span as having had an error. It uses the information from
// err to set tags such as the error message, error type and stack trace. It has
// no effect if the error is nil.
func WithError(err error) FinishOption {
	return func(cfg *ddtrace.FinishConfig) {
		cfg.Error = err
	}
}

// NoDebugStack prevents any error presented using the WithError finishing option
// from generating a stack trace. This is useful in situations where errors are frequent
// and performance is critical.
func NoDebugStack() FinishOption {
	return func(cfg *ddtrace.FinishConfig) {
		cfg.NoDebugStack = true
	}
}

// StackFrames limits the number of stack frames included into erroneous spans to n, starting from skip.
func StackFrames(n, skip uint) FinishOption {
	if n == 0 {
		return NoDebugStack()
	}
	return func(cfg *ddtrace.FinishConfig) {
		cfg.StackFrames = n
		cfg.SkipStackFrames = skip
	}
}
