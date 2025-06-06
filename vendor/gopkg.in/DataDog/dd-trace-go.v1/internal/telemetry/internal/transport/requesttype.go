// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// RequestType determines how the Payload of a request should be handled
type RequestType string

const (
	// RequestTypeAppStarted is the first message sent by the telemetry
	// client, containing the configuration loaded at startup
	RequestTypeAppStarted RequestType = "app-started"

	// RequestTypeAppHeartbeat is sent periodically by the client to indicate
	// that the app is still running
	RequestTypeAppHeartbeat RequestType = "app-heartbeat"

	// RequestTypeGenerateMetrics contains count, gauge, or rate metrics accumulated by the
	// client, and is sent periodically along with the heartbeat
	RequestTypeGenerateMetrics RequestType = "generate-metrics"

	// RequestTypeDistributions is to send distribution type metrics accumulated by the
	// client, and is sent periodically along with the heartbeat
	RequestTypeDistributions RequestType = "distributions"

	// RequestTypeAppClosing is sent when the telemetry client is stopped
	RequestTypeAppClosing RequestType = "app-closing"

	// RequestTypeAppDependenciesLoaded is sent if DD_TELEMETRY_DEPENDENCY_COLLECTION_ENABLED
	// is enabled. Sent when Start is called for the telemetry client.
	RequestTypeAppDependenciesLoaded RequestType = "app-dependencies-loaded"

	// RequestTypeAppClientConfigurationChange is sent if there are changes
	// to the client library configuration
	RequestTypeAppClientConfigurationChange RequestType = "app-client-configuration-change"

	// RequestTypeAppProductChange is sent when products are enabled/disabled
	RequestTypeAppProductChange RequestType = "app-product-change"

	// RequestTypeAppIntegrationsChange is sent when the telemetry client starts
	// with info on which integrations are used.
	RequestTypeAppIntegrationsChange RequestType = "app-integrations-change"

	// RequestTypeMessageBatch is a wrapper over a list of payloads
	RequestTypeMessageBatch RequestType = "message-batch"

	// RequestTypeAppExtendedHeartBeat This event will be used as a failsafe if there are any catastrophic data failure.
	// The data will be used to reconstruct application records in our db.
	RequestTypeAppExtendedHeartBeat RequestType = "app-extended-heartbeat"

	// RequestTypeLogs is used to send logs to the backend
	RequestTypeLogs RequestType = "logs"
)
