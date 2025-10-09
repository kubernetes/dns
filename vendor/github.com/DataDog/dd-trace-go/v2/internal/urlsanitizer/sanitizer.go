// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016 Datadog, Inc.

// Package urlsanitizer provides utilities for sanitizing URLs and DSNs by removing sensitive information.
package urlsanitizer

import (
	"net/url"
	"strings"
)

// SanitizeURL removes user credentials from URLs for safe logging.
// It uses Go's built-in url.Redacted() when possible, which preserves usernames but redacts passwords.
// If the URL can't be parsed but appears to contain credentials, it's fully redacted for security.
func SanitizeURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails but we suspect credentials,
		// redact entirely for safety.
		if containsCredentials(rawURL) {
			return "[REDACTED_URL_WITH_CREDENTIALS]"
		}
		// If no credentials suspected,
		// return as-is (might just be a malformed URL without sensitive data)
		return rawURL
	}

	// If URL has user info,
	// use Go's built-in redaction (preserves username, redacts password).
	if parsedURL.User != nil {
		return parsedURL.Redacted()
	}

	// No credentials detected, return as-is
	return rawURL
}

// containsCredentials checks if a URL string might contain credentials
func containsCredentials(rawURL string) bool {
	// Look for patterns that suggest credentials: username:password@host
	// This is a simple heuristic - if we see :...@ we assume credentials.
	if !strings.Contains(rawURL, "://") {
		return false
	}

	// Find the scheme part.
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd == -1 {
		return false
	}

	// Look in the part after the scheme for credentials.
	rest := rawURL[schemeEnd+3:]

	// If we see a colon followed by an @ sign, likely credentials.
	colonIndex := strings.Index(rest, ":")
	if colonIndex == -1 {
		return false
	}

	// Check if there's an @ after the colon
	atIndex := strings.Index(rest[colonIndex:], "@")
	return atIndex != -1
}
