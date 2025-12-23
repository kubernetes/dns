// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

// Package telemetry provides a telemetry client that is thread-safe burden-less telemetry client following the specification of the instrumentation telemetry from Datadog.
// Specification here: https://github.com/DataDog/instrumentation-telemetry-api-docs/tree/main
//
// The telemetry package has 6 main capabilities:
//   - Metrics: Support for [Count], [Rate], [Gauge], [Distribution] metrics.
//   - Logs: Support Debug, Warn, Error logs with tags and stack traces via the subpackage [log] or the [Log] function.
//   - Product: Start, Stop and Startup errors reporting to the backend
//   - App Config: Register and change the configuration of the application and declare its origin
//   - Integration: Loading and errors
//   - Dependencies: Sending all the dependencies of the application to the backend (for SCA purposes for example)
//
// Each of these capabilities is exposed through the [Client] interface but mainly through the package level functions.
// that mirror and call the global client that is started through the [StartApp] function.
//
// Before the [StartApp] function is called, all called to the global client will be recorded and replay
// when the [StartApp] function is called synchronously. The telemetry client is allowed to record at most 512 calls.
//
// At the end of the app lifetime. If [tracer.Stop] is called, the client should be stopped with the [StopApp] function.
// For all data to be flushed to the backend appropriately.
//
// Note: No public API is available for the dependencies payloads as this is does in-house with the `ClientConfig.DependencyLoader` function output.
package telemetry

import (
	"io"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

// Namespace describes a product to distinguish telemetry coming from
// different products used by the same application
type Namespace = transport.Namespace
type Origin = transport.Origin
type LogLevel = transport.LogLevel

//goland:noinspection GoVarAndConstTypeMayBeOmitted  Goland is having a hard time with the following const block, it keeps deleting the type
const (
	NamespaceGeneral      Namespace = transport.NamespaceGeneral
	NamespaceTracers      Namespace = transport.NamespaceTracers
	NamespaceProfilers    Namespace = transport.NamespaceProfilers
	NamespaceAppSec       Namespace = transport.NamespaceAppSec
	NamespaceIAST         Namespace = transport.NamespaceIAST
	NamespaceCIVisibility Namespace = transport.NamespaceCIVisibility
	NamespaceMLObs        Namespace = transport.NamespaceMLObs
	NamespaceRUM          Namespace = transport.NamespaceRUM
)

// Origin describes the source of a configuration change

//goland:noinspection GoVarAndConstTypeMayBeOmitted Goland is having a hard time with the following const block, it keeps deleting the type
const (
	OriginDefault             Origin = transport.OriginDefault
	OriginCode                Origin = transport.OriginCode
	OriginDDConfig            Origin = transport.OriginDDConfig
	OriginEnvVar              Origin = transport.OriginEnvVar
	OriginRemoteConfig        Origin = transport.OriginRemoteConfig
	OriginLocalStableConfig   Origin = transport.OriginLocalStableConfig
	OriginManagedStableConfig Origin = transport.OriginManagedStableConfig
)

// EmptyID represents the absence of a configuration ID.
// It can be assigned to the ID field of a Configuration when no ID is available or required.
const EmptyID = ""

// LogLevel describes the level of a log message

//goland:noinspection GoVarAndConstTypeMayBeOmitted Goland is having a hard time with the following const block, it keeps deleting the type
const (
	LogDebug LogLevel = transport.LogLevelDebug
	LogWarn  LogLevel = transport.LogLevelWarn
	LogError LogLevel = transport.LogLevelError
)

// MetricHandle can be used to submit different values for the same metric.
// MetricHandle is used to reduce lock contention when submitting metrics.
// This can also be used ephemerally to submit a single metric value like this:
//
//	telemetry.metric(telemetry.Appsec, "my-count", map[string]string{"tag1": "true", "tag2": "1.0"}).Submit(1.0)
type MetricHandle interface {
	// Submit submits a value to the metric handle.
	Submit(value float64)
	// Get returns the last value submitted to the metric handle.
	Get() float64
}

// Integration is an integration that is configured to be traced.
type Integration struct {
	// Name is an arbitrary string that must stay constant for the integration.
	Name string
	// Version is the version of the integration/dependency that is being loaded.
	Version string
	// Error is the error that occurred while loading the integration. If this field is specified, the integration is
	// considered to be having been forcefully disabled because of the error.
	Error string
}

// Configuration is a key-value pair that is used to configure the application.
type Configuration struct {
	// Key is the key of the configuration.
	Name string
	// Value is the value of the configuration. Need to be json serializable.
	Value any
	// Origin is the source of the configuration change.
	Origin Origin
	// ID is the config ID of the configuration change.
	ID string
}

// LogOption is a function that modifies the log message that is sent to the telemetry.
type LogOption func(key *loggerKey, value *loggerValue)

// Client constitutes all the functions available concurrently for the telemetry users. All methods are thread-safe
// This is an interface for easier testing but all functions will be mirrored at the package level to call
// the global client.
type Client interface {
	io.Closer

	// Count obtains the metric handle for the given parameters, or creates a new one if none was created just yet.
	// Tags cannot contain commas.
	Count(namespace Namespace, name string, tags []string) MetricHandle

	// Rate obtains the metric handle for the given parameters, or creates a new one if none was created just yet.
	// Tags cannot contain commas.
	Rate(namespace Namespace, name string, tags []string) MetricHandle

	// Gauge obtains the metric handle for the given parameters, or creates a new one if none was created just yet.
	// Tags cannot contain commas.
	Gauge(namespace Namespace, name string, tags []string) MetricHandle

	// Distribution obtains the metric handle for the given parameters, or creates a new one if none was created just yet.
	// Tags cannot contain commas.
	Distribution(namespace Namespace, name string, tags []string) MetricHandle

	// Log sends a telemetry log with the given [slog.Record] and options.
	// Options include sending key-value pairs as tags, and a stack trace frozen from inside the Log function.
	Log(record Record, options ...LogOption)

	// ProductStarted declares a product to have started at the customer's request
	ProductStarted(product Namespace)

	// ProductStopped declares a product to have being stopped by the customer
	ProductStopped(product Namespace)

	// ProductStartError declares that a product could not start because of the following error
	ProductStartError(product Namespace, err error)

	// RegisterAppConfig adds a key value pair to the app configuration and send the change to telemetry
	// value has to be json serializable and the origin is the source of the change.
	RegisterAppConfig(key string, value any, origin Origin)

	// RegisterAppConfigs adds a list of key value pairs to the app configuration and sends the change to telemetry.
	// Same as AddAppConfig but for multiple values.
	RegisterAppConfigs(kvs ...Configuration)

	// MarkIntegrationAsLoaded marks an integration as loaded in the telemetry
	MarkIntegrationAsLoaded(integration Integration)

	// Flush closes the client and flushes any remaining data.
	Flush()

	// AppStart sends the telemetry necessary to signal that the app is starting.
	// Preferred use via [StartApp] package level function
	AppStart()

	// AppStop sends the telemetry necessary to signal that the app is stopping.
	// Preferred use via [StopApp] package level function
	AppStop()

	// AddFlushTicker adds a function that is called at each telemetry Flush. By default, every minute
	AddFlushTicker(ticker func(Client))
}
