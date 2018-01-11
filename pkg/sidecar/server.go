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

import (
	"fmt"
	"net/http"
	"time"

	"github.com/datadog/datadog-go/statsd"
	"github.com/golang/glog"
	"k8s.io/dns/pkg/dnsmasq"
)

var countersCache = make(map[dnsmasq.MetricName]int64)

// Server that runs the dnsmasq-metrics daemon.
type Server interface {
	Run(options *Options)
}

type server struct {
	options       *Options
	metricsClient dnsmasq.MetricsClient
	statsdClient  *statsd.Client
	probes        []*dnsProbe
}

// NewServer creates a new server instance
func NewServer() Server {
	return &server{}
}

// Run the server (does not return)
func (s *server) Run(options *Options) {
	s.options = options

	statsdClient, err := statsd.New(fmt.Sprintf("%s:%d", options.DatadogAddr, options.DatadogPort))

	if err != nil {
		panic(err)
	}

	statsdClient.Namespace = options.DatadogNamespace
	s.statsdClient = statsdClient

	glog.Infof("Starting server (options %+v)", *s.options)

	for _, probeOption := range options.Probes {
		probe := &dnsProbe{DNSProbeOption: probeOption, statsdClient: statsdClient}
		s.probes = append(s.probes, probe)
		probe.Start(options)
	}

	s.runMetrics(options)

	http.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "ok (%v)\n", time.Now())
	})

	go func() {
		err := http.ListenAndServe("0.0.0.0:9000", nil)

		if err != nil {
			glog.Fatalf("Error starting metrics server: %v", err)
		}
	}()
}

func (s *server) runMetrics(options *Options) {
	s.metricsClient = dnsmasq.NewMetricsClient(options.DnsMasqAddr, options.DnsMasqPort)

	for {
		metrics, err := s.metricsClient.GetMetrics()

		if err != nil {
			glog.Warningf("Error getting metrics from dnsmasq: %v", err)
			s.statsdClient.Incr("errors", nil, 1)
		} else {
			glog.V(3).Infof("DnsMasq metrics %+v", metrics)
			err := s.exportMetrics(metrics)

			if err != nil {
				glog.Warningf("Error sending metrics to statsd: %v", err)
			}
		}

		time.Sleep(time.Duration(options.DnsMasqPollIntervalMs) * time.Millisecond)
	}
}

func (s *server) exportMetrics(metrics *dnsmasq.Metrics) error {
	for key := range *metrics {
		// Retrieve the previous value of the metric and get the delta
		// between the previous and current values. Add the delta to the
		// previous to get the proper value. This is needed because the
		// Counter API does not allow us to set the counter to a value.
		previousValue := countersCache[key]
		delta := (*metrics)[key] - previousValue
		newValue := previousValue + delta
		// Update cache to new value.
		countersCache[key] = newValue

		// Could this be a gauge and the above messing removed?
		err := s.statsdClient.Count(fmt.Sprintf("%s", key), delta, nil, 1)

		if err != nil {
			return err
		}
	}

	return nil
}
