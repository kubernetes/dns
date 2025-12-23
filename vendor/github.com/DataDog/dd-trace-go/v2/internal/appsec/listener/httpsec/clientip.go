// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package httpsec

import (
	"net"
	"net/netip"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/theckman/httpforwarded"
)

// ClientIP returns the first public IP address found in the given headers. If
// none is present, it returns the first valid IP address present, possibly
// being a local IP address. The remote address, when valid, is used as fallback
// when no IP address has been found at all.
func ClientIP(hdrs map[string][]string, hasCanonicalHeaders bool, remoteAddr string, monitoredHeaders []string) (remoteIP, clientIP netip.Addr) {
	// Walk IP-related headers
	var foundIP netip.Addr
headersLoop:
	for _, headerName := range monitoredHeaders {
		if hasCanonicalHeaders {
			headerName = textproto.CanonicalMIMEHeaderKey(headerName)
		}

		headerValues, exists := hdrs[headerName]
		if !exists {
			continue // this monitored header is not present
		}

		// Assuming a list of comma-separated IP addresses, split them and build
		// the list of values to try to parse as IP addresses
		var ips []string
		for _, headerValue := range headerValues {
			if strings.ToLower(headerName) == "forwarded" {
				ips = append(ips, parseForwardedHeader(headerValue)...)
			} else {
				ips = append(ips, strings.Split(headerValue, ",")...)
			}
		}

		// Look for the first valid or global IP address in the comma-separated list
		for _, ipstr := range ips {
			ip := parseIP(strings.TrimSpace(ipstr))
			if !ip.IsValid() {
				continue
			}
			// Replace foundIP if still not valid in order to keep the oldest
			if !foundIP.IsValid() {
				foundIP = ip
			}
			if isGlobalIP(ip) {
				foundIP = ip
				break headersLoop
			}
		}
	}

	// Decide which IP address is the client one by starting with the remote IP
	if ip := parseIP(remoteAddr); ip.IsValid() {
		remoteIP = ip
		clientIP = ip
	}

	// The IP address found in the headers supersedes a private remote IP address.
	if foundIP.IsValid() && !isGlobalIP(remoteIP) || isGlobalIP(foundIP) {
		clientIP = foundIP
	}

	return remoteIP, clientIP
}

var (
	forwardedPortRe = regexp.MustCompile(`^(?:\[([a-f0-9:]+)\]|(\d+\.\d+\.\d+\.\d+))(?::\d+)?$`)
)

// parseForwardedHeader parses the value of the `Forwarded` header, returning
// the values of all `for` directives it contains, in the order they appear.
// Values may not always be IP addresses; but those values that are will have
// any quoting and port information removed.
//
// If the value is found to be syntactically incorrect, a null slice is returned.
//
// See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Forwarded
func parseForwardedHeader(value string) []string {
	result, err := httpforwarded.ParseParameter("for", []string{value})
	if err != nil {
		log.Debug("invalid Forwarded header value: %v", err.Error())
		return nil
	}

	for idx, val := range result {
		matches := forwardedPortRe.FindStringSubmatch(val)
		if matches == nil {
			continue
		}
		// Remove the port information from the value, and un-brace IPv6 addresses.
		if matches[1] != "" {
			result[idx] = matches[1]
		} else {
			result[idx] = matches[2]
		}
	}

	return result
}

func parseIP(s string) netip.Addr {
	if ip, err := netip.ParseAddr(s); err == nil {
		return ip
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		if ip, err := netip.ParseAddr(h); err == nil {
			return ip
		}
	}
	return netip.Addr{}
}

var (
	ipv6SpecialNetworks = [...]netip.Prefix{
		netip.MustParsePrefix("fec0::/10"), // site local
	}

	// This IP block is not routable on internet and an industry standard/trend
	// is emerging to use it for traditional IT-managed networking environments
	// with limited RFC1918 space allocations. This is also frequently used by
	// kubernetes pods' internal networking. It is hence deemed private for the
	// purpose of Client IP extraction.
	k8sInternalIPv4Prefix = netip.MustParsePrefix("100.65.0.0/10")
)

func isGlobalIP(ip netip.Addr) bool {
	// IsPrivate also checks for ipv6 ULA.
	// We care to check for these addresses are not considered public, hence not global.
	// See https://www.rfc-editor.org/rfc/rfc4193.txt for more details.
	isGlobal := ip.IsValid() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !k8sInternalIPv4Prefix.Contains(ip)
	if !isGlobal || !ip.Is6() {
		return isGlobal
	}
	for _, n := range ipv6SpecialNetworks {
		if n.Contains(ip) {
			return false
		}
	}
	return isGlobal
}
