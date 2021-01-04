package hosts

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// hostsEntries is the combined number of entries in hosts and Corefile.
	hostsEntries = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "hosts",
		Name:      "entries",
		Help:      "The combined number of entries in hosts and Corefile.",
	}, []string{})
	// hostsReloadTime is the timestamp of the last reload of hosts file.
	hostsReloadTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "hosts",
		Name:      "reload_timestamp_seconds",
		Help:      "The timestamp of the last reload of hosts file.",
	})
)
