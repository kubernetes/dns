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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/dns/pkg/dnsmasq"
)

// initMetrics initializes dnsmasq.Metrics with values for testing.
func initMetrics(metricsList []*dnsmasq.Metrics, values []int64) {
	defineDnsmasqMetrics(&Options{PrometheusNamespace: "dnsmasq"})
	for i := range metricsList {
		metricsList[i] = &dnsmasq.Metrics{}
		for j := range dnsmasq.AllMetrics {
			metric := dnsmasq.AllMetrics[j]
			// Avoids giving each metric the same value.
			(*(metricsList[i]))[metric] = values[j] * int64(i+1)
		}
	}
}

// TestExportMetrics tests if our countersCache works as expected.
func TestExportMetrics(t *testing.T) {
	var m1, m2, m3 *dnsmasq.Metrics
	l := []*dnsmasq.Metrics{m1, m2, m3}

	testMetricValues := []int64{10, 20, 30, 40, 50}
	initMetrics(l, testMetricValues)

	for i := range l {
		exportMetrics(l[i])
		for j := range dnsmasq.AllMetrics {
			assert.Equal(t, countersCache[dnsmasq.AllMetrics[j]], float64(testMetricValues[j]*int64(i+1)))
		}
	}
}
