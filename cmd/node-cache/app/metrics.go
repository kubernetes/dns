/*
Copyright 2021 The Kubernetes Authors.
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

package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/reuseport"

	"github.com/coredns/coredns/plugin/pkg/uniq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
)

var (
	u = uniq.New()

	// ListenAddr is assigned the address of the prometheus listener. Its use is mainly in tests where
	// we listen on "localhost:0" and need to retrieve the actual address.
	ListenAddr string
)

// shutdownTimeout is the maximum amount of time the metrics plugin will wait
// before erroring when it tries to close the metrics server
const shutdownTimeout time.Duration = time.Second * 5

var setupErrCount = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: "nodecache",
	Name:      "setup_errors_total",
	Help:      "The number of errors during periodic network setup for node-cache",
}, []string{"errortype"})

// Metrics holds the prometheus configuration. The metrics' path is fixed to be /metrics .
type Metrics struct {
	Next plugin.Handler
	Addr string
	Reg  *prometheus.Registry

	ln      net.Listener
	lnSetup bool

	mux *http.ServeMux
	srv *http.Server

	tlsConfigPath string
}

// New returns a new instance of Metrics with the given address.
func New(addr string) *Metrics {
	met := &Metrics{
		Addr:          addr,
		Reg:           prometheus.DefaultRegisterer.(*prometheus.Registry),
		tlsConfigPath: "",
	}

	return met
}

// OnStartup sets up the metrics on startup.
func (m *Metrics) OnStartup() error {
	ln, err := reuseport.Listen("tcp", m.Addr)
	if err != nil {
		return fmt.Errorf("Failed to start metrics handler: %s", err)
	}

	m.ln = ln
	m.lnSetup = true

	m.mux = http.NewServeMux()
	m.mux.Handle("/metrics", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:         m.Addr,
		Handler:      m.mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  5 * time.Second,
	}
	m.srv = server

	// No TLS config file given, start without TLS
	if m.tlsConfigPath == "" {
		go func() {
			if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
				slog.Error("Failed to start HTTP metrics server", "error", err)
			}
		}()
		ListenAddr = ln.Addr().String() // For tests.
		return nil
	}

	// Check TLS config file existence
	if _, err := os.Stat(m.tlsConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("TLS config file does not exist: %s", m.tlsConfigPath)
	}

	// Create web config for ListenAndServe
	webConfig := &web.FlagConfig{
		WebListenAddresses: &[]string{m.Addr},
		WebSystemdSocket:   new(bool), // false by default
		WebConfigFile:      &m.tlsConfigPath,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Create channels for synchronization
	startResult := make(chan error, 1)

	go func() {
		// Try to start the server and immediately report result
		err := web.Serve(m.ln, server, webConfig, logger)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start HTTPS metrics server", "error", err)
			startResult <- err
		}
		// If we get here without error, server is running
	}()

	// Wait for startup errors
	select {
	case err := <-startResult:
		return err
	case <-time.After(200 * time.Millisecond):
		// No immediate error, server likely started succesfully
		// web.Serve() validates TLS config at startup
	}

	registerMetrics()
	ListenAddr = ln.Addr().String() // For tests.
	return nil
}

// OnRestart stops the listener on reload.
func (m *Metrics) OnRestart() error {
	if !m.lnSetup {
		return nil
	}
	u.Unset(m.Addr)
	return m.stopServer()
}

func (m *Metrics) stopServer() error {
	if !m.lnSetup {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := m.srv.Shutdown(ctx); err != nil {
		slog.Error("Failed to stop prometheus http server", "error", err)
		return err
	}
	m.lnSetup = false
	m.ln.Close()
	prometheus.Unregister(setupErrCount)
	return nil
}

// OnFinalShutdown tears down the metrics listener on shutdown and restart.
func (m *Metrics) OnFinalShutdown() error { return m.stopServer() }

func publishErrorMetric(label string) {
	setupErrCount.WithLabelValues(label).Inc()
}

func registerMetrics() {
	prometheus.MustRegister(setupErrCount)
	setupErrCount.WithLabelValues("iptables").Add(0)
	setupErrCount.WithLabelValues("iptables_lock").Add(0)
	setupErrCount.WithLabelValues("interface_add").Add(0)
	setupErrCount.WithLabelValues("interface_check").Add(0)
	setupErrCount.WithLabelValues("configmap").Add(0)
}
