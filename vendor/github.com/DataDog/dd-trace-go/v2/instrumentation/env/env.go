// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package env

import (
	"github.com/DataDog/dd-trace-go/v2/internal/env"
)

// Get is a wrapper around os.Getenv that validates the environment variable
// against a list of supported environment variables.
//
// When a environment variable is not supported because it is not
// listed in the list of supported environment variables, the function will log an error
// and behave as if the environment variable was not set.
//
// In testing mode, the reader will automatically add the environment variable
// to the configuration file.
//
// This function is a passthrough to the internal env package.
func Get(name string) string {
	return env.Get(name)
}

// Lookup is a wrapper around os.LookupEnv that validates the environment variable
// against a list of supported environment variables.
//
// When a environment variable is not supported because it is not
// listed in the list of supported environment variables, the function will log an error
// and behave as if the environment variable was not set.
//
// In testing mode, the reader will automatically add the environment variable
// to the configuration file.
//
// This function is a passthrough to the internal env package.
func Lookup(name string) (string, bool) {
	return env.Lookup(name)
}
