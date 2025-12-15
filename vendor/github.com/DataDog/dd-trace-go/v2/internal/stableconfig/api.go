// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package stableconfig provides utilities to load and manage APM configurations
// loaded from YAML configuration files
package stableconfig

import (
	"errors"
	"fmt"
	"iter"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

// ConfigData holds configuration value with its origin and config ID
type ConfigData struct {
	Origin   telemetry.Origin
	Value    string
	ConfigID string
}

func reportTelemetryAndReturnWithErr(env string, value bool, origin telemetry.Origin, id string, err error) (bool, telemetry.Origin, error) {
	if env == "DD_APPSEC_SCA_ENABLED" && origin == telemetry.OriginDefault {
		return value, origin, err
	}
	telemetry.RegisterAppConfigs(telemetry.Configuration{Name: telemetry.EnvToTelemetryName(env), Value: value, Origin: origin, ID: id})
	return value, origin, err
}

func reportTelemetryAndReturn(env string, value string, origin telemetry.Origin, id string) (string, telemetry.Origin) {
	telemetry.RegisterAppConfigs(telemetry.Configuration{Name: telemetry.EnvToTelemetryName(env), Value: value, Origin: origin, ID: id})
	return value, origin
}

// Bool returns a boolean config value from managed file-based config, environment variable,
// or local file-based config, in that order. If none provide a valid boolean, it returns the default.
// Also returns the value's origin and any parse error encountered.
func Bool(env string, def bool) (value bool, origin telemetry.Origin, err error) {
	for configData := range stableConfigByPriority(env) {
		if val, err := strconv.ParseBool(configData.Value); err == nil {
			return reportTelemetryAndReturnWithErr(env, val, configData.Origin, configData.ConfigID, nil)
		}
		err = errors.Join(err, fmt.Errorf("non-boolean value for %s: '%s' in %s configuration, dropping", env, configData.Value, configData.Origin))
	}
	return reportTelemetryAndReturnWithErr(env, def, telemetry.OriginDefault, telemetry.EmptyID, err)
}

// String returns a string config value from managed file-based config, environment variable,
// or local file-based config, in that order. If none are set, it returns the default value and origin.
func String(env string, def string) (string, telemetry.Origin) {
	for configData := range stableConfigByPriority(env) {
		return reportTelemetryAndReturn(env, configData.Value, configData.Origin, configData.ConfigID)
	}
	return reportTelemetryAndReturn(env, def, telemetry.OriginDefault, telemetry.EmptyID)
}

func stableConfigByPriority(key string) iter.Seq[ConfigData] {
	return func(yield func(ConfigData) bool) {
		if v := ManagedConfig.Get(key); v != "" && !yield(ConfigData{
			Origin:   telemetry.OriginManagedStableConfig,
			Value:    v,
			ConfigID: ManagedConfig.GetID(),
		}) {
			return
		}
		if v, ok := env.Lookup(key); ok && !yield(ConfigData{
			Origin:   telemetry.OriginEnvVar,
			Value:    v,
			ConfigID: telemetry.EmptyID, // environment variables do not have config ID
		}) {
			return
		}
		if v := LocalConfig.Get(key); v != "" && !yield(ConfigData{
			Origin:   telemetry.OriginLocalStableConfig,
			Value:    v,
			ConfigID: LocalConfig.GetID(),
		}) {
			return
		}
	}
}
