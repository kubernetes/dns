// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package internal

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"

	// OTel did a breaking change to the module go.opentelemetry.io/collector/pdata which is imported by the agent
	// and go.opentelemetry.io/collector/pdata/pprofile depends on it and is breaking because of it
	// For some reason the dependency closure won't let use upgrade this module past the point where it does not break anymore
	// So we are forced to add a blank import of this module to give us back the control over its version
	//
	// TODO: remove this once github.com/datadog-agent/pkg/trace has upgraded both modules past the breaking change
	_ "go.opentelemetry.io/collector/pdata/pprofile"
)

const (
	DefaultAgentHostname  = "localhost"
	DefaultTraceAgentPort = "8126"
)

// This is a variable rather than a constant so it can be replaced in unit tests
var DefaultTraceAgentUDSPath = "/var/run/datadog/apm.socket"

// AgentURLFromEnv resolves the URL for the trace agent based on
// the default host/port and UDS path, and via standard environment variables.
// AgentURLFromEnv has the following priority order:
//   - First, DD_TRACE_AGENT_URL if it is set
//   - Then, if either of DD_AGENT_HOST and DD_TRACE_AGENT_PORT are set,
//     use http://DD_AGENT_HOST:DD_TRACE_AGENT_PORT,
//     defaulting to localhost and 8126, respectively
//   - Then, DefaultTraceAgentUDSPath, if the path exists
//   - Finally, localhost:8126
func AgentURLFromEnv() *url.URL {
	if agentURL := env.Get("DD_TRACE_AGENT_URL"); agentURL != "" {
		u, err := url.Parse(agentURL)
		if err != nil {
			log.Warn("Failed to parse DD_TRACE_AGENT_URL: %s", err.Error())
		} else {
			switch u.Scheme {
			case "unix", "http", "https":
				return u
			default:
				log.Warn("Unsupported protocol %q in Agent URL %q. Must be one of: http, https, unix.", u.Scheme, agentURL)
			}
		}
	}

	host, providedHost := env.Lookup("DD_AGENT_HOST")
	port, providedPort := env.Lookup("DD_TRACE_AGENT_PORT")
	if host == "" {
		// We treat set but empty the same as unset
		providedHost = false
		host = DefaultAgentHostname
	}
	if port == "" {
		// We treat set but empty the same as unset
		providedPort = false
		port = DefaultTraceAgentPort
	}
	httpURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
	}
	if providedHost || providedPort {
		return httpURL
	}

	if _, err := os.Stat(DefaultTraceAgentUDSPath); err == nil {
		return &url.URL{
			Scheme: "unix",
			Path:   DefaultTraceAgentUDSPath,
		}
	}
	return httpURL
}

func DefaultDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
}

func DefaultHTTPClient(timeout time.Duration, disableKeepAlives bool) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           DefaultDialer(timeout).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     disableKeepAlives,
		},
		Timeout: timeout,
	}
}
