// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import (
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

const defaultSeqID uint64 = 1

// seqId is a global counter for configuration telemetry sequence IDs.
// Initialized to defaultSeqID in init() so config values start reporting at 2+.
//
// For telemetry reporting, it's recommended to:
// - Use reportTelemetry() for all config telemetry, OR
// - Use nextSeqID() if calling telemetry.RegisterAppConfigs directly
// - If accessing seqId directly, always use seqId.Add(1) to get the next ID
// - Defaults should always use defaultSeqID (not nextSeqID)
var seqId atomic.Uint64

func init() {
	seqId.Store(defaultSeqID)
}

// nextSeqID returns the next sequence ID for configuration telemetry.
// All non-default configuration telemetry must use this function to obtain sequence IDs.
func nextSeqID() uint64 {
	return seqId.Add(1)
}

// reportTelemetry reports configuration telemetry with an auto-incremented sequence ID.
// This is the preferred way to report non-default configuration values and should be used when the source does not have a config ID.
func reportTelemetry(name string, value any, origin telemetry.Origin) {
	telemetry.RegisterAppConfigs(telemetry.Configuration{
		Name:   name,
		Value:  value,
		Origin: origin,
		ID:     telemetry.EmptyID,
		SeqID:  nextSeqID(),
	})
}

// reportTelemetryWithID reports configuration telemetry with a provided config ID, and an auto-incremented sequence ID.
// config sources of type idAwareConfigSource should use this function to report telemetry with their config ID.
func reportTelemetryWithID(name string, value any, origin telemetry.Origin, id string) {
	telemetry.RegisterAppConfigs(telemetry.Configuration{
		Name:   name,
		Value:  value,
		Origin: origin,
		ID:     id,
		SeqID:  nextSeqID(),
	})
}

func reportDefaultTelemetry(name string, value any) {
	telemetry.RegisterAppConfigs(telemetry.Configuration{
		Name:   name,
		Value:  value,
		Origin: telemetry.OriginDefault,
		ID:     telemetry.EmptyID,
		SeqID:  defaultSeqID,
	})
}

var provider = defaultconfigProvider()

type configProvider struct {
	sources []configSource // In order of priority
}

type configSource interface {
	get(key string) string
	origin() telemetry.Origin
}

// idAwareConfigSource is a config source that has a config ID.
// Currently, only DeclarativeConfigSource implements this interface.
type idAwareConfigSource interface {
	configSource
	getID() string
}

func defaultconfigProvider() *configProvider {
	return &configProvider{
		sources: []configSource{
			newDeclarativeConfigSource(managedFilePath, telemetry.OriginManagedStableConfig),
			new(envConfigSource),
			new(otelEnvConfigSource),
			newDeclarativeConfigSource(localFilePath, telemetry.OriginLocalStableConfig),
		},
	}
}

// get is a generic helper that iterates through config sources and parses values, returning the first successfully parsed value.
// The parse function should return the parsed value and true if parsing succeeded, or false otherwise.
//
// Telemetry Reporting:
//   - Reports telemetry for ALL non-empty values found across ALL sources, regardless of priority
//   - SeqID reflects priority: higher priority sources get higher seqIds, while default sources always report defaultSeqID
func get[T any](p *configProvider, key string, def T, parse func(string) (T, bool)) T {
	var final *T
	for i := len(p.sources) - 1; i >= 0; i-- {
		source := p.sources[i]
		v := source.get(key)
		if v != "" {
			var id string
			if s, ok := source.(idAwareConfigSource); ok {
				id = s.getID()
			}
			reportTelemetryWithID(key, v, source.origin(), id)
			if parsed, ok := parse(v); ok {
				// Always overwrite final so higher priority sources win
				final = &parsed
			}
		}
	}
	reportDefaultTelemetry(key, def)
	if final != nil {
		return *final
	}
	return def
}

func (p *configProvider) getString(key string, def string) string {
	return get(p, key, def, func(v string) (string, bool) {
		return v, true
	})
}

func (p *configProvider) getBool(key string, def bool) bool {
	return get(p, key, def, func(v string) (bool, bool) {
		boolVal, err := strconv.ParseBool(v)
		if err == nil {
			return boolVal, true
		}
		return false, false
	})
}

func (p *configProvider) getInt(key string, def int) int {
	return get(p, key, def, func(v string) (int, bool) {
		intVal, err := strconv.Atoi(v)
		return intVal, err == nil
	})
}

func (p *configProvider) getIntWithValidator(key string, def int, validate func(int) bool) int {
	return get(p, key, def, func(v string) (int, bool) {
		intVal, err := strconv.Atoi(v)
		if err == nil {
			if validate != nil && !validate(intVal) {
				return 0, false
			}
			return intVal, true
		}
		return 0, false
	})
}

func (p *configProvider) getMap(key string, def map[string]string) map[string]string {
	return get(p, key, def, func(v string) (map[string]string, bool) {
		m := parseMapString(v)
		return m, len(m) > 0
	})
}

func (p *configProvider) getDuration(key string, def time.Duration) time.Duration {
	return get(p, key, def, func(v string) (time.Duration, bool) {
		d, err := time.ParseDuration(v)
		return d, err == nil
	})
}

func (p *configProvider) getFloat(key string, def float64) float64 {
	return get(p, key, def, func(v string) (float64, bool) {
		floatVal, err := strconv.ParseFloat(v, 64)
		return floatVal, err == nil
	})
}

func (p *configProvider) getFloatWithValidator(key string, def float64, validate func(float64) bool) float64 {
	return get(p, key, def, func(v string) (float64, bool) {
		floatVal, err := strconv.ParseFloat(v, 64)
		if err == nil {
			if validate != nil && !validate(floatVal) {
				return 0, false
			}
			return floatVal, true
		}
		return 0, false
	})
}

func (p *configProvider) getURL(key string, def *url.URL) *url.URL {
	return get(p, key, def, func(v string) (*url.URL, bool) {
		u, err := url.Parse(v)
		return u, err == nil
	})
}

// normalizeKey is a helper function for configSource implementations to normalize the key to a valid environment variable name.
func normalizeKey(key string) string {
	if strings.HasPrefix(key, "DD_") || strings.HasPrefix(key, "OTEL_") {
		return key
	}
	return "DD_" + strings.ToUpper(key)
}

// parseMapString parses a string containing key:value pairs separated by comma or space.
// Format: "key1:value1,key2:value2" or "key1:value1 key2:value2"
// Uses internal.ForEachStringTag to ensure consistent parsing with other tag-like env vars.
func parseMapString(str string) map[string]string {
	result := make(map[string]string)
	// NOTE: Beware of potential import cycle if config moves to another package.
	internal.ForEachStringTag(str, internal.DDTagsDelimiter, func(key, val string) {
		result[key] = val
	})
	return result
}
