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
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/reuseport"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/pkg/uniq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
)

var (
	log = clog.NewWithPlugin("prometheus")
	u   = uniq.New()

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

// logAdapter adapts our existing logging to the slog.Logger interface
type logAdapter struct{}

func (l *logAdapter) Info(msg string, args ...any) {
	log.Infof(msg, args...)
}

func (l *logAdapter) Error(msg string, args ...any) {
	log.Errorf(msg, args...)
}

func (l *logAdapter) Debug(msg string, args ...any) {
	log.Debugf(msg, args...)
}

func (l *logAdapter) Warn(msg string, args ...any) {
	log.Warningf(msg, args...)
}

func (l *logAdapter) With(args ...any) *slog.Logger {
	return slog.Default()
}

func (l *logAdapter) WithGroup(name string) slog.Handler {
	// Since our logging system doesn't support groups, we'll just return the same handler
	return l
}

func (l *logAdapter) Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	switch level {
	case slog.LevelDebug:
		l.Debug(msg, args...)
	case slog.LevelInfo:
		l.Info(msg, args...)
	case slog.LevelWarn:
		l.Warn(msg, args...)
	case slog.LevelError:
		l.Error(msg, args...)
	}
}

func (l *logAdapter) Enabled(ctx context.Context, level slog.Level) bool {
	// We'll consider all levels enabled since we want to pass through all logs
	return true
}

func (l *logAdapter) Handle(ctx context.Context, r slog.Record) error {
	// Convert the record to our logging format
	msg := r.Message
	args := make([]any, 0, r.NumAttrs())
	r.Attrs(func(attr slog.Attr) bool {
		args = append(args, attr.Key, attr.Value)
		return true
	})

	switch r.Level {
	case slog.LevelDebug:
		l.Debug(msg, args...)
	case slog.LevelInfo:
		l.Info(msg, args...)
	case slog.LevelWarn:
		l.Warn(msg, args...)
	case slog.LevelError:
		l.Error(msg, args...)
	}
	return nil
}

func (l *logAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Create a new adapter with the additional attributes
	// Since our logging system doesn't support structured logging, we'll just return the same handler
	return l
}

func (l *logAdapter) Handler() slog.Handler {
	return l
}

func newLoggerAdapter() *slog.Logger {
	return slog.New(&logAdapter{})
}

// Metrics holds the prometheus configuration. The metrics' path is fixed to be /metrics .
type Metrics struct {
	Next plugin.Handler
	Addr string
	Reg  *prometheus.Registry

	ln      net.Listener
	lnSetup bool

	mux *http.ServeMux
	srv *http.Server

	tlsConfig *tlsConfig
}

// tlsConfig is the TLS configuration for Metrics
type tlsConfig struct {
	// Enabled controls whether TLS is active
	// Optional: Defaults to true when tls block is present
	Enabled bool

	// CertFile is the path to the server's certificate file in PEM format
	// Required when TLS is enabled
	CertFile string

	// KeyFile is the path to the server's private key file in PEM format
	// Required when TLS is enabled
	KeyFile string

	// ClientCAFile is the path to the CA certificate file for client verification
	// Optional: Only needed for client authentication
	// Default: No client verification
	// Can contain multiple CA certificates in a single PEM file
	ClientCAFile string

	// MinVersion is the minimum TLS version to accept
	// Optional: Defaults to tls.VersionTLS13
	// Possible values: tls.VersionTLS10 through tls.VersionTLS13
	MinVersion uint

	// ClientAuthType controls how client certificates are handled
	// Optional: Defaults to "RequestClientCert" (strictest)
	// Possible values:
	//   - "RequestClientCert"
	//   - "RequireAnyClientCert"
	//   - "VerifyClientCertIfGiven"
	//   - "RequireAndVerifyClientCert"
	//   - "NoClientCert"
	ClientAuthType string

	CancelFunc context.CancelFunc
}

// New returns a new instance of Metrics with the given address.
func New(addr string, cfg *tlsConfig) *Metrics {
	met := &Metrics{
		Addr:      addr,
		Reg:       prometheus.DefaultRegisterer.(*prometheus.Registry),
		tlsConfig: cfg,
	}

	return met
}

// validate validates the Metrics configuration
func (m *Metrics) validate() error {
	if m.tlsConfig != nil && m.tlsConfig.Enabled {
		if m.tlsConfig.CertFile == "" {
			return fmt.Errorf("TLS enabled but no certificate file specified")
		}
		if m.tlsConfig.KeyFile == "" {
			return fmt.Errorf("TLS enabled but no key file specified")
		}
		// Check if files exist
		if _, err := os.Stat(m.tlsConfig.CertFile); err != nil {
			return fmt.Errorf("certificate file not found: %s", m.tlsConfig.CertFile)
		}
		if _, err := os.Stat(m.tlsConfig.KeyFile); err != nil {
			return fmt.Errorf("key file not found: %s", m.tlsConfig.KeyFile)
		}
	}
	return nil
}

// OnStartup sets up the metrics on startup.
func (m *Metrics) OnStartup() error {
	if err := m.validate(); err != nil {
		return err
	}

	ln, err := reuseport.Listen("tcp", m.Addr)
	if err != nil {
		log.Errorf("Failed to start metrics handler: %s", err)
		return err
	}

	m.ln = ln
	m.lnSetup = true

	m.mux = http.NewServeMux()
	m.mux.Handle("/metrics", promhttp.HandlerFor(m.Reg, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:    m.Addr,
		Handler: m.mux,
	}
	// Create server without TLS based on configuration
	tlsEnabled := m.tlsConfig != nil && m.tlsConfig.Enabled
	m.srv = server

	if !tlsEnabled {
		go func() {
			if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
				clog.Errorf("Failed to start HTTP metrics server: %s", err)
			}
		}()
		ListenAddr = ln.Addr().String() // For tests.
		return nil
	}

	// Create web config for ListenAndServe
	webConfig := &web.FlagConfig{
		WebListenAddresses: &[]string{m.Addr},
		WebSystemdSocket:   new(bool), // false by default
		WebConfigFile:      new(string),
	}

	// Create temporary YAML config file for TLS settings
	tmpFile, err := os.CreateTemp("", "metrics-tls-config-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary TLS config file: %w", err)
	}

	minVersion, err := tlsVersionToString(m.tlsConfig.MinVersion)
	if err != nil {
		return fmt.Errorf("failed to convert TLS version to string: %w", err)
	}

	yamlConfig := fmt.Sprintf(`
tls_server_config:
  cert_file: %s
  key_file: %s
  client_ca_file: %s
  client_auth_type: %s
  min_version: %s
`, m.tlsConfig.CertFile, m.tlsConfig.KeyFile, m.tlsConfig.ClientCAFile, m.tlsConfig.ClientAuthType, minVersion)

	if _, err := tmpFile.WriteString(yamlConfig); err != nil {
		return fmt.Errorf("failed to write TLS config to temporary file: %w", err)
	}

	*webConfig.WebConfigFile = tmpFile.Name()

	// Create logger
	logger := newLoggerAdapter()

	// Create channel to signal when server is ready
	ready := make(chan bool)
	errChan := make(chan error)

	go func() {
		// Signal when server is ready
		close(ready)
		err := web.Serve(m.ln, server, webConfig, logger)
		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for server to be ready or error
	select {
	case <-ready:
		// Server started successfully
		clog.Infof("Nodecache metrics are served over HTTPS at %s", m.Addr)
	case err := <-errChan:
		clog.Errorf("Failed to start HTTPS metrics server: %v", err)
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timeout waiting for server to start")
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
	// In tls case, just running the cancelFunc will signal shutdown
	if m.tlsConfig != nil && m.tlsConfig.CancelFunc != nil {
		m.tlsConfig.CancelFunc()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := m.srv.Shutdown(ctx); err != nil {
			log.Infof("Failed to stop prometheus http server: %s", err)
			return err
		}
	}
	m.lnSetup = false
	m.ln.Close()
	prometheus.Unregister(setupErrCount)
	return nil
}

// OnFinalShutdown tears down the metrics listener on shutdown and restart.
func (m *Metrics) OnFinalShutdown() error { return m.stopServer() }

func tlsVersionToString(version uint) (string, error) {
	switch version {
	case tls.VersionTLS10:
		return "TLS10", nil
	case tls.VersionTLS11:
		return "TLS11", nil
	case tls.VersionTLS12:
		return "TLS12", nil
	case tls.VersionTLS13:
		return "TLS13", nil
	default:
		return "", fmt.Errorf("unknown TLS version: %d", version)
	}
}

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
