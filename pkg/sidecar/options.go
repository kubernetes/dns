/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sidecar

import "time"

// DNSProbeOption for periodic DNS health check and latency probes.
type DNSProbeOption struct {
	// Label to use for healthcheck URL
	Label string
	// Endpoint to send DNS requests to.
	Server string
	// Name to resolve to test endpoint.
	Name string
	// Interval to use for probing
	Interval time.Duration
	// Type of Record to query for.
	Type uint16
}

// Options for the daemon
type Options struct {
	DnsMasqPort           int
	DnsMasqAddr           string
	DnsMasqPollIntervalMs int

	Probes []DNSProbeOption

	DatadogAddr      string
	DatadogPort      int
	DatadogNamespace string
}

// NewOptions creates a new options struct with default values.
func NewOptions() *Options {
	return &Options{
		DnsMasqAddr:           "127.0.0.1",
		DnsMasqPort:           53,
		DnsMasqPollIntervalMs: 5000,

		DatadogAddr:      "0.0.0.0",
		DatadogPort:      8125,
		DatadogNamespace: "kubedns",
	}
}
