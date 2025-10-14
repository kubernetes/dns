// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package ddtrace

import (
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/version"
)

func init() {
	checkV1NonTransitional()
}

func checkV1NonTransitional() {
	version, transitional, found := version.FindV1Version()
	if !found {
		// No v1 version detected
		return
	}
	if transitional {
		// v1 version is transitional
		return
	}
	log.Warn("Detected %q non-transitional version of dd-trace-go. This version is not compatible with v2 - please upgrade to v1.74.0 or later", version)
}
