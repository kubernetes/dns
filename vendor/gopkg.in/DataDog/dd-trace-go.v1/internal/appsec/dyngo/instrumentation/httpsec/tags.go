// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package httpsec

import (
	"encoding/json"
	"sort"
	"strings"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo/instrumentation"
)

// SetAppSecTags sets the AppSec-specific span tags that are expected to be in
// the web service entry span (span of type `web`) when AppSec is enabled.
func SetAppSecTags(span ddtrace.Span) {
	span.SetTag("_dd.appsec.enabled", 1)
	span.SetTag("_dd.runtime_family", "go")
}

// SetSecurityEventTags sets the AppSec-specific span tags when a security event occurred into the service entry span.
func SetSecurityEventTags(span ddtrace.Span, events []json.RawMessage, remoteIP string, headers, respHeaders map[string][]string) {
	instrumentation.SetEventSpanTags(span, events)
	span.SetTag("network.client.ip", remoteIP)
	for h, v := range NormalizeHTTPHeaders(headers) {
		span.SetTag("http.request.headers."+h, v)
	}
	for h, v := range NormalizeHTTPHeaders(respHeaders) {
		span.SetTag("http.response.headers."+h, v)
	}
}

// List of HTTP headers we collect and send.
var collectedHTTPHeaders = [...]string{
	"host",
	"x-forwarded-for",
	"x-client-ip",
	"x-real-ip",
	"x-forwarded",
	"x-cluster-client-ip",
	"forwarded-for",
	"forwarded",
	"via",
	"true-client-ip",
	"content-length",
	"content-type",
	"content-encoding",
	"content-language",
	"forwarded",
	"user-agent",
	"accept",
	"accept-encoding",
	"accept-language",
}

func init() {
	// Required by sort.SearchStrings
	sort.Strings(collectedHTTPHeaders[:])
}

// NormalizeHTTPHeaders returns the HTTP headers following Datadog's
// normalization format.
func NormalizeHTTPHeaders(headers map[string][]string) (normalized map[string]string) {
	if len(headers) == 0 {
		return nil
	}
	normalized = make(map[string]string)
	for k, v := range headers {
		k = strings.ToLower(k)
		if i := sort.SearchStrings(collectedHTTPHeaders[:], k); i < len(collectedHTTPHeaders) && collectedHTTPHeaders[i] == k {
			normalized[k] = strings.Join(v, ",")
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
