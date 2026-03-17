// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package httpsec

import (
	"net"
	"net/netip"
	"net/textproto"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
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

// parseForwardedHeader parses the value of the `Forwarded` header according to
// RFC 7239, returning the values of all `for` directives it contains, in the
// order they appear. Values may not always be IP addresses; but those values
// that are will have any quoting and port information removed.
//
// If the value is found to be syntactically incorrect, a null slice is returned.
//
// See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Forwarded
// See: https://www.rfc-editor.org/rfc/rfc7239
func parseForwardedHeader(value string) []string {
	// State machine states
	const (
		stateKey          = iota // reading parameter key (token)
		stateTokenValue          // reading/emitted token value, waiting for delimiter
		stateQuotedValue         // reading quoted-string value
		stateQuotedEscape        // after '\' inside quoted-string
	)

	// Pre-allocate to avoid slice growth (covers most real-world cases)
	result := make([]string, 0, 3)
	state := stateKey

	// Index-based tracking to avoid allocations
	keyStart := -1
	valStart := -1
	isForKey := false
	hasEscape := false

	// Buffer only used when quoted values contain escape sequences
	var escapeBuf strings.Builder

	for i := 0; i < len(value); i++ {
		c := value[i]

		switch state {
		case stateKey:
			switch {
			case c == '=' && keyStart >= 0:
				// Check if key is "for" (case-insensitive) without allocating
				keyLen := i - keyStart
				isForKey = keyLen == 3 &&
					// Bitwise trick for case-insensitive ASCII comparison.
					// Uppercase and lowercase characters differ only in bit 5
					// (value 32 = 0x20). ORing with 0x20 sets bit 5, forcing
					// any character to lowercase.
					(value[keyStart]|0x20) == 'f' &&
					(value[keyStart+1]|0x20) == 'o' &&
					(value[keyStart+2]|0x20) == 'r'
				keyStart = -1
				// Peek next char to determine value type.
				// This avoids tracking an additional state.
				i++
				if i >= len(value) {
					log.Debug("invalid Forwarded header: unexpected end of input after '='")
					return nil
				}
				c = value[i]
				if c == '"' {
					valStart = i + 1
					hasEscape = false
					state = stateQuotedValue
				} else if isTokenChar(c) {
					valStart = i
					state = stateTokenValue
				} else {
					log.Debug("invalid Forwarded header: unexpected character %q at start of value", c)
					return nil
				}
			case isTokenChar(c):
				if keyStart < 0 {
					keyStart = i
				}
			case isOWS(c):
				// Skip optional whitespace before key
			default:
				log.Debug("invalid Forwarded header: unexpected character %q in key", c)
				return nil
			}
		case stateTokenValue:
			// Handles both reading value (valStart >= 0) and waiting for delimiter (valStart < 0)
			switch {
			case c == ';', c == ',':
				if valStart >= 0 && isForKey {
					result = append(result, extractForValue(value[valStart:i]))
				}
				valStart = -1
				isForKey = false
				state = stateKey
			case isOWS(c):
				if valStart >= 0 {
					if isForKey {
						result = append(result, extractForValue(value[valStart:i]))
					}
					valStart = -1
					isForKey = false
				}
				// Stay in stateTokenValue, waiting for delimiter
			case isTokenChar(c):
				if valStart < 0 {
					log.Debug("invalid Forwarded header: unexpected character %q after value", c)
					return nil
				}
				// Continue reading token
			default:
				log.Debug("invalid Forwarded header: unexpected character %q in token value", c)
				return nil
			}
		case stateQuotedValue:
			switch {
			case c == '\\':
				if !hasEscape {
					// First escape: copy everything so far to buffer
					hasEscape = true
					escapeBuf.Reset()
					escapeBuf.WriteString(value[valStart:i])
				}
				state = stateQuotedEscape
			case c == '"':
				if isForKey {
					var val string
					if hasEscape {
						val = escapeBuf.String()
					} else {
						val = value[valStart:i]
					}
					result = append(result, extractForValue(val))
				}
				valStart = -1
				isForKey = false
				hasEscape = false
				state = stateTokenValue // Reuse stateTokenValue for waiting
			default:
				if hasEscape {
					escapeBuf.WriteByte(c)
				}
			}
		case stateQuotedEscape:
			// RFC 7230: quoted-pair = "\" ( HTAB / SP / VCHAR / obs-text )
			escapeBuf.WriteByte(c)
			state = stateQuotedValue
		}
	}
	// Handle end-of-input based on final state
	switch state {
	case stateKey:
		// Valid: empty input, trailing separator, or whitespace-only
		if keyStart >= 0 {
			log.Debug("invalid Forwarded header: unexpected end of input while reading key")
			return nil
		}
	case stateTokenValue:
		// Valid if we already emitted (valStart < 0) or emit final value now
		if valStart >= 0 && isForKey {
			result = append(result, extractForValue(value[valStart:]))
		}
	case stateQuotedValue, stateQuotedEscape:
		log.Debug("invalid Forwarded header: unexpected end of input in quoted value")
		return nil
	}
	return result
}

// isTokenChar returns true if c is a valid token character per RFC 7230.
// token = 1*tchar
// tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." /
//
//	"^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
func isTokenChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '!' || c == '#' || c == '$' || c == '%' || c == '&' ||
		c == '\'' || c == '*' || c == '+' || c == '-' || c == '.' ||
		c == '^' || c == '_' || c == '`' || c == '|' || c == '~'
}

// isOWS returns true if c is optional whitespace (SP or HTAB) per RFC 7230.
func isOWS(c byte) bool {
	return c == ' ' || c == '\t'
}

func extractForValue(s string) string {
	if len(s) == 0 {
		return s
	}
	// Remove brackets from IPv6 values: "[ipv6]" or "[ipv6]:port"
	if s[0] == '[' {
		for i := 1; i < len(s); i++ {
			if s[i] == ']' {
				return s[1:i] // Zero-copy substring
			}
		}
		return s
	}
	// If it's already a valid IP without port, return as-is
	if _, err := netip.ParseAddr(s); err == nil {
		return s
	}
	// At this point we have an IPv4 with port - find colon and strip port
	i := strings.IndexRune(s, ':')
	if i == -1 {
		// Not sure what we have here, returning as-is.
		return s
	}
	return s[:i]
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
