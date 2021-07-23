package app

import (
	"net"
	"net/http"

	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var setupErrCount = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: "nodecache",
	Name:      "setup_errors_total",
	Help:      "The number of errors during periodic network setup for node-cache",
}, []string{"errortype"})

func initMetrics(ipport string) {
	if err := serveMetrics(ipport); err != nil {
		clog.Errorf("Failed to start metrics handler: %s", err)
		return
	}
	registerMetrics()
}

func registerMetrics() {
	prometheus.MustRegister(setupErrCount)
	setupErrCount.WithLabelValues("iptables").Add(0)
	setupErrCount.WithLabelValues("iptables_lock").Add(0)
	setupErrCount.WithLabelValues("interface_add").Add(0)
	setupErrCount.WithLabelValues("interface_check").Add(0)
	setupErrCount.WithLabelValues("configmap").Add(0)
}

func publishErrorMetric(label string) {
	setupErrCount.WithLabelValues(label).Inc()
}

func serveMetrics(ipport string) error {
	ln, err := net.Listen("tcp", ipport)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Handler: mux}
	go func() {
		srv.Serve(ln)
	}()
	return nil
}
