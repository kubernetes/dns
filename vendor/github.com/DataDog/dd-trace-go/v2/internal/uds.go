// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func UnixDataSocketURL(path string) *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("UDS_%s", strings.NewReplacer(":", "_", "/", "_", `\`, "_").Replace(path)),
	}
}

// UDSClient returns a new http.Client which connects using the given UDS socket path.
func UDSClient(socketPath string, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return DefaultDialer(timeout).DialContext(ctx, "unix", (&net.UnixAddr{
					Name: socketPath,
					Net:  "unix",
				}).String())
			},
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: timeout,
	}
}
