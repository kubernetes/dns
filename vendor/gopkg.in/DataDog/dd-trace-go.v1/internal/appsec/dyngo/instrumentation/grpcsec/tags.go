// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package grpcsec

import (
	"encoding/json"
	"net"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo/instrumentation"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/dyngo/instrumentation/httpsec"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
)

// SetSecurityEventTags sets the AppSec-specific span tags when a security event
// occurred into the service entry span.
func SetSecurityEventTags(span ddtrace.Span, events []json.RawMessage, addr net.Addr, md map[string][]string) {
	if err := setSecurityEventTags(span, events, addr, md); err != nil {
		log.Error("appsec: %v", err)
	}
}

func setSecurityEventTags(span ddtrace.Span, events []json.RawMessage, addr net.Addr, md map[string][]string) error {
	if err := instrumentation.SetEventSpanTags(span, events); err != nil {
		return err
	}
	var ip string
	switch actual := addr.(type) {
	case *net.UDPAddr:
		ip = actual.IP.String()
	case *net.TCPAddr:
		ip = actual.IP.String()
	}
	if ip != "" {
		span.SetTag("network.client.ip", ip)
	}
	for h, v := range httpsec.NormalizeHTTPHeaders(md) {
		span.SetTag("grpc.metadata."+h, v)
	}
	return nil
}
