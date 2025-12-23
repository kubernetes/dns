// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package env

import (
	"os"
	"strings"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// Get is a wrapper around env.Get that validates the environment variable
// against a list of supported environment variables.
//
// If the environment variable has aliases, the function will also check the aliases
// and return the value of the first alias that is set.
//
// When a environment variable is not supported because it is not
// listed in the list of supported environment variables, the function will log an error
// and behave as if the environment variable was not set.
//
// In testing mode, the reader will automatically add the environment variable
// to the configuration file.
func Get(name string) string {
	if !verifySupportedConfiguration(name) {
		return ""
	}

	if v := os.Getenv(name); v != "" {
		return v
	}

	for _, alias := range keyAliases[name] {
		if v := os.Getenv(alias); v != "" {
			return v
		}
	}

	return ""
}

// Lookup is a wrapper around os.LookupEnv that validates the environment variable
// against a list of supported environment variables.
//
// If the environment variable has aliases, the function will also check the aliases.
// and return the value of the first alias that is set.
//
// When a environment variable is not supported because it is not
// listed in the list of supported environment variables, the function will log an error
// and behave as if the environment variable was not set.
//
// In testing mode, the reader will automatically add the environment variable
// to the configuration file.
func Lookup(name string) (string, bool) {
	if !verifySupportedConfiguration(name) {
		return "", false
	}

	if v, ok := os.LookupEnv(name); ok {
		return v, true
	}

	for _, alias := range keyAliases[name] {
		if v, ok := os.LookupEnv(alias); ok {
			return v, true
		}
	}

	return "", false
}

func verifySupportedConfiguration(name string) bool {
	if strings.HasPrefix(name, "DD_") || strings.HasPrefix(name, "OTEL_") {
		if _, ok := SupportedConfigurations[name]; !ok {
			if testing.Testing() {
				addSupportedConfigurationToFile(name)
			}

			log.Error("config: usage of a unlisted environment variable: %s", name)

			return false
		}
	}

	return true
}
