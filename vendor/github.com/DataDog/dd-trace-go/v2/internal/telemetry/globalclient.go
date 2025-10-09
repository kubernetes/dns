// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package telemetry

import (
	"sync"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v3"

	globalinternal "github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/knownmetrics"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

var (
	globalClient atomic.Pointer[Client]

	// globalClientRecorder contains all actions done on the global client done before StartApp() with an actual client object is called
	globalClientRecorder = internal.NewRecorder[Client]()

	// metricsHandleSwappablePointers contains all the swappableMetricHandle, used to replay actions done before the actual MetricHandle is set
	metricsHandleSwappablePointers = xsync.NewMapOf[metricKey, *swappableMetricHandle](xsync.WithPresize(knownmetrics.Size()))
)

// GlobalClient returns the global telemetry client.
func GlobalClient() Client {
	client := globalClient.Load()
	if client == nil {
		return nil
	}
	return *client
}

// StartApp starts the telemetry client with the given client send the app-started telemetry and sets it as the global (*client)
// then calls client.Flush on the client asynchronously.
func StartApp(client Client) {
	if Disabled() {
		return
	}

	if GlobalClient() != nil || SwapClient(client) != nil {
		log.Debug("telemetry: StartApp called multiple times, ignoring")
		return
	}

	client.AppStart()
	go client.Flush()
}

// SwapClient swaps the global client with the given client and Flush the old (*client).
func SwapClient(client Client) Client {
	if Disabled() {
		return nil
	}

	oldClientPtr := globalClient.Swap(&client)
	var oldClient Client
	if oldClientPtr != nil && *oldClientPtr != nil {
		oldClient = *oldClientPtr
	}

	if oldClient != nil {
		oldClient.Close()
	}

	if client == nil {
		return oldClient
	}

	globalClientRecorder.Replay(client)
	// Swap all metrics hot pointers to the new MetricHandle
	metricsHandleSwappablePointers.Range(func(_ metricKey, value *swappableMetricHandle) bool {
		value.swap(value.maker(client))
		return true
	})

	return oldClient
}

// MockClient swaps the global client with the given client and clears the recorder to make sure external calls are not replayed.
// It returns a function that can be used to swap back the global client
func MockClient(client Client) func() {
	globalClientRecorder.Clear()
	metricsHandleSwappablePointers.Clear()

	oldClient := SwapClient(client)
	return func() {
		SwapClient(oldClient)
	}
}

// StopApp creates the app-stopped telemetry, adding to the queue and Flush all the queue before stopping the (*client).
func StopApp() {
	if client := globalClient.Swap(nil); client != nil && *client != nil {
		(*client).AppStop()
		(*client).Flush()
		(*client).Close()
	}
}

var telemetryClientDisabled = !globalinternal.BoolEnv("DD_INSTRUMENTATION_TELEMETRY_ENABLED", true)

// Disabled returns whether instrumentation telemetry is disabled
// according to the DD_INSTRUMENTATION_TELEMETRY_ENABLED env var
func Disabled() bool {
	return telemetryClientDisabled
}

// Count creates a new metric handle for the given parameters that can be used to submit values.
// Count will always return a [MetricHandle], even if telemetry is disabled or the client has yet to start.
// The [MetricHandle] is then swapped with the actual [MetricHandle] once the client is started.
func Count(namespace Namespace, name string, tags []string) MetricHandle {
	return globalClientNewMetric(namespace, transport.CountMetric, name, tags)
}

// Rate creates a new metric handle for the given parameters that can be used to submit values.
// Rate will always return a [MetricHandle], even if telemetry is disabled or the client has yet to start.
// The [MetricHandle] is then swapped with the actual [MetricHandle] once the client is started.
func Rate(namespace Namespace, name string, tags []string) MetricHandle {
	return globalClientNewMetric(namespace, transport.RateMetric, name, tags)
}

// Gauge creates a new metric handle for the given parameters that can be used to submit values.
// Gauge will always return a [MetricHandle], even if telemetry is disabled or the client has yet to start.
// The [MetricHandle] is then swapped with the actual [MetricHandle] once the client is started.
func Gauge(namespace Namespace, name string, tags []string) MetricHandle {
	return globalClientNewMetric(namespace, transport.GaugeMetric, name, tags)
}

// Distribution creates a new metric handle for the given parameters that can be used to submit values.
// Distribution will always return a [MetricHandle], even if telemetry is disabled or the client has yet to start.
// The [MetricHandle] is then swapped with the actual [MetricHandle] once the client is started.
// The Get() method of the [MetricHandle] will return the last value submitted.
// Distribution MetricHandle is advised to be held in a variable more than the rest of the metric types to avoid too many useless allocations.
func Distribution(namespace Namespace, name string, tags []string) MetricHandle {
	return globalClientNewMetric(namespace, transport.DistMetric, name, tags)
}

func Log(level LogLevel, text string, options ...LogOption) {
	globalClientCall(func(client Client) {
		client.Log(level, text, options...)
	})
}

// ProductStarted declares a product to have started at the customer’s request. If telemetry is disabled, it will do nothing.
// If the telemetry client has not started yet, it will record the action and replay it once the client is started.
func ProductStarted(product Namespace) {
	globalClientCall(func(client Client) {
		client.ProductStarted(product)
	})
}

// ProductStopped declares a product to have being stopped by the customer. If telemetry is disabled, it will do nothing.
// If the telemetry client has not started yet, it will record the action and replay it once the client is started.
func ProductStopped(product Namespace) {
	globalClientCall(func(client Client) {
		client.ProductStopped(product)
	})
}

// ProductStartError declares that a product could not start because of the following error. If telemetry is disabled, it will do nothing.
// If the telemetry client has not started yet, it will record the action and replay it once the client is started.
func ProductStartError(product Namespace, err error) {
	globalClientCall(func(client Client) {
		client.ProductStartError(product, err)
	})
}

// RegisterAppConfig adds a key value pair to the app configuration and send the change to telemetry
// value has to be json serializable and the origin is the source of the change. If telemetry is disabled, it will do nothing.
// If the telemetry client has not started yet, it will record the action and replay it once the client is started.
func RegisterAppConfig(key string, value any, origin Origin) {
	globalClientCall(func(client Client) {
		client.RegisterAppConfig(key, value, origin)
	})
}

// RegisterAppConfigs adds a list of key value pairs to the app configuration and sends the change to telemetry.
// Same as AddAppConfig but for multiple values. If telemetry is disabled, it will do nothing.
// If the telemetry client has not started yet, it will record the action and replay it once the client is started.
func RegisterAppConfigs(kvs ...Configuration) {
	globalClientCall(func(client Client) {
		client.RegisterAppConfigs(kvs...)
	})
}

// MarkIntegrationAsLoaded marks an integration as loaded in the telemetry. If telemetry is disabled
// or the client has not started yet it will record the action and replay it once the client is started.
func MarkIntegrationAsLoaded(integration Integration) {
	globalClientCall(func(client Client) {
		client.MarkIntegrationAsLoaded(integration)
	})
}

// LoadIntegration marks an integration as loaded in the telemetry client. If telemetry is disabled, it will do nothing.
// If the telemetry client has not started yet, it will record the action and replay it once the client is started.
func LoadIntegration(integration string) {
	globalClientCall(func(client Client) {
		client.MarkIntegrationAsLoaded(Integration{
			Name: integration,
		})
	})
}

// AddFlushTicker adds a function that is called at each telemetry Flush. By default, every minute
func AddFlushTicker(ticker func(Client)) {
	globalClientCall(func(client Client) {
		client.AddFlushTicker(ticker)
	})
}

var globalClientLogLossOnce sync.Once

// globalClientCall takes a function that takes a Client and calls it with the global client if it exists.
// otherwise, it records the action for when the client is started.
func globalClientCall(fun func(client Client)) {
	if Disabled() {
		return
	}

	client := globalClient.Load()
	if client == nil || *client == nil {
		if !globalClientRecorder.Record(fun) {
			globalClientLogLossOnce.Do(func() {
				log.Debug("telemetry: global client recorder queue is full, dropping telemetry data, please start the telemetry client earlier to avoid data loss")
			})
		}
		return
	}

	fun(*client)
}

var noopMetricHandleInstance = noopMetricHandle{}

func globalClientNewMetric(namespace Namespace, kind transport.MetricType, name string, tags []string) MetricHandle {
	if Disabled() {
		return noopMetricHandleInstance
	}

	key := newMetricKey(namespace, kind, name, tags)
	hotPtr, _ := metricsHandleSwappablePointers.LoadOrCompute(key, func() *swappableMetricHandle {
		maker := func(client Client) MetricHandle {
			switch kind {
			case transport.CountMetric:
				return client.Count(namespace, name, tags)
			case transport.RateMetric:
				return client.Rate(namespace, name, tags)
			case transport.GaugeMetric:
				return client.Gauge(namespace, name, tags)
			case transport.DistMetric:
				return client.Distribution(namespace, name, tags)
			}
			log.Warn("telemetry: unknown metric type %q", kind)
			return nil
		}
		wrapper := &swappableMetricHandle{maker: maker}
		if client := globalClient.Load(); client == nil || *client == nil {
			wrapper.recorder = internal.NewRecorder[MetricHandle]()
		}
		globalClientCall(func(client Client) {
			wrapper.swap(maker(client))
		})
		return wrapper
	})
	return hotPtr
}
