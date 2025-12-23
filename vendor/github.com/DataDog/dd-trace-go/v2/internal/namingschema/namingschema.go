// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

// Package namingschema allows to use the naming schema from the integrations to set different
// service and span/operation names based on the value of the DD_TRACE_SPAN_ATTRIBUTE_SCHEMA environment variable.
package namingschema

import (
	"strings"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/globalconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// Version represents the available naming schema versions.
type Version int

const (
	// SchemaV0 represents naming schema v0.
	SchemaV0 Version = iota
	// SchemaV1 represents naming schema v1.
	SchemaV1
)

type Config struct {
	NamingSchemaVersion           Version
	RemoveIntegrationServiceNames bool
	DDService                     string
}

var (
	activeNamingSchema            atomic.Int32
	removeIntegrationServiceNames atomic.Bool
)

func LoadFromEnv() {
	schemaVersionStr := env.Get("DD_TRACE_SPAN_ATTRIBUTE_SCHEMA")
	if v, ok := parseVersionStr(schemaVersionStr); ok {
		setVersion(v)
	} else {
		setVersion(SchemaV0)
		log.Warn("DD_TRACE_SPAN_ATTRIBUTE_SCHEMA=%s is not a valid value, setting to default of v%d", schemaVersionStr, v)
	}
	// Allow DD_TRACE_SPAN_ATTRIBUTE_SCHEMA=v0 users to disable default integration (contrib AKA v0) service names.
	// These default service names are always disabled for v1 onwards.
	SetRemoveIntegrationServiceNames(internal.BoolEnv("DD_TRACE_REMOVE_INTEGRATION_SERVICE_NAMES_ENABLED", false))
}

// ReloadConfig is used to reload the configuration in tests.
func ReloadConfig() {
	LoadFromEnv()
	globalconfig.SetServiceName(env.Get("DD_SERVICE"))
}

// GetConfig returns the naming schema config.
func GetConfig() Config {
	return Config{
		NamingSchemaVersion:           GetVersion(),
		RemoveIntegrationServiceNames: getRemoveIntegrationServiceNames(),
		DDService:                     globalconfig.ServiceName(),
	}
}

// GetVersion returns the global naming schema version used for this application.
func GetVersion() Version {
	return Version(activeNamingSchema.Load())
}

// setVersion sets the global naming schema version used for this application.
func setVersion(v Version) {
	activeNamingSchema.Store(int32(v))
}

// parseVersionStr attempts to parse the version string.
func parseVersionStr(v string) (Version, bool) {
	switch strings.ToLower(v) {
	case "", "v0":
		return SchemaV0, true
	case "v1":
		return SchemaV1, true
	default:
		return SchemaV0, false
	}
}

func getRemoveIntegrationServiceNames() bool {
	return removeIntegrationServiceNames.Load()
}

// SetRemoveIntegrationServiceNames sets the value of the RemoveIntegrationServiceNames setting for this application.
func SetRemoveIntegrationServiceNames(v bool) {
	removeIntegrationServiceNames.Store(v)
}
