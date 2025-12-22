// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"
)

type TracerConfig struct {
	DDTags     map[string]any
	Env        string
	Service    string
	Version    string
	AgentURL   *url.URL
	APIKey     string
	APPKey     string
	HTTPClient *http.Client
	Site       string
}

type AgentFeatures struct {
	EVPProxyV2 bool
}

type Config struct {
	Enabled                  bool
	MLApp                    string
	AgentlessEnabled         *bool
	ResolvedAgentlessEnabled bool
	ProjectName              string
	TracerConfig             TracerConfig
	AgentFeatures            AgentFeatures
}

// We copy the transport to avoid using the default one, as it might be
// augmented with tracing and we don't want these calls to be recorded.
// See https://golang.org/pkg/net/http/#DefaultTransport .
// Note: We don't set a global Timeout on the client; instead, we manage
// timeouts per-request using context.WithTimeout for better control.
func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func (c *Config) DefaultHTTPClient() *http.Client {
	var cl *http.Client
	if c.ResolvedAgentlessEnabled || c.TracerConfig.AgentURL.Scheme != "unix" {
		cl = newHTTPClient()
	} else {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		cl = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", (&net.UnixAddr{
						Name: c.TracerConfig.AgentURL.Path,
						Net:  "unix",
					}).String())
				},
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
	}
	return cl
}
