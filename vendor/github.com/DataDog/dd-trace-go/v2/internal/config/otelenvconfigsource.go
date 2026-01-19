// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import (
	"fmt"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

const (
	ddPrefix   = "config_datadog:"
	otelPrefix = "config_opentelemetry:"
)

type otelEnvConfigSource struct{}

func (o *otelEnvConfigSource) get(key string) string {
	ddKey := normalizeKey(key)
	entry := otelConfigs[ddKey]
	if entry == nil {
		return ""
	}
	otVal := env.Get(entry.ot)
	if otVal == "" {
		return ""
	}
	if ddVal := env.Get(ddKey); ddVal != "" {
		log.Warn("Both %q and %q are set, using %s=%s", entry.ot, ddKey, entry.ot, ddVal)
		telemetryTags := []string{ddPrefix + strings.ToLower(ddKey), otelPrefix + strings.ToLower(entry.ot)}
		telemetry.Count(telemetry.NamespaceTracers, "otel.env.hiding", telemetryTags).Submit(1)
	}
	val, err := entry.remapper(otVal)
	if err != nil {
		log.Warn("%s", err.Error())
		telemetryTags := []string{ddPrefix + strings.ToLower(ddKey), otelPrefix + strings.ToLower(entry.ot)}
		telemetry.Count(telemetry.NamespaceTracers, "otel.env.invalid", telemetryTags).Submit(1)
		return ""
	}
	return val
}

func (o *otelEnvConfigSource) origin() telemetry.Origin {
	return telemetry.OriginEnvVar
}

type otelDDEnv struct {
	ot       string
	remapper func(string) (string, error)
}

var otelConfigs = map[string]*otelDDEnv{
	"DD_SERVICE": {
		ot:       "OTEL_SERVICE_NAME",
		remapper: mapService,
	},
	"DD_RUNTIME_METRICS_ENABLED": {
		ot:       "OTEL_METRICS_EXPORTER",
		remapper: mapMetrics,
	},
	"DD_TRACE_DEBUG": {
		ot:       "OTEL_LOG_LEVEL",
		remapper: mapLogLevel,
	},
	"DD_TRACE_ENABLED": {
		ot:       "OTEL_TRACES_EXPORTER",
		remapper: mapEnabled,
	},
	"DD_TRACE_SAMPLE_RATE": {
		ot:       "OTEL_TRACES_SAMPLER",
		remapper: mapSampleRate,
	},
	"DD_TRACE_PROPAGATION_STYLE": {
		ot:       "OTEL_PROPAGATORS",
		remapper: mapPropagationStyle,
	},
	"DD_TAGS": {
		ot:       "OTEL_RESOURCE_ATTRIBUTES",
		remapper: mapDDTags,
	},
}

var ddTagsMapping = map[string]string{
	"service.name":           "service",
	"deployment.environment": "env",
	"service.version":        "version",
}

var unsupportedSamplerMapping = map[string]string{
	"always_on":    "parentbased_always_on",
	"always_off":   "parentbased_always_off",
	"traceidratio": "parentbased_traceidratio",
}

var propagationMapping = map[string]string{
	"tracecontext": "tracecontext",
	"b3":           "b3 single header",
	"b3multi":      "b3multi",
	"datadog":      "datadog",
	"none":         "none",
}

// mapService maps OTEL_SERVICE_NAME to DD_SERVICE
func mapService(ot string) (string, error) {
	return ot, nil
}

// mapMetrics maps OTEL_METRICS_EXPORTER to DD_RUNTIME_METRICS_ENABLED
func mapMetrics(ot string) (string, error) {
	ot = strings.TrimSpace(strings.ToLower(ot))
	if ot == "none" {
		return "false", nil
	}
	return "", fmt.Errorf("the following configuration is not supported: OTEL_METRICS_EXPORTER=%v", ot)
}

// mapLogLevel maps OTEL_LOG_LEVEL to DD_TRACE_DEBUG
func mapLogLevel(ot string) (string, error) {
	if strings.TrimSpace(strings.ToLower(ot)) == "debug" {
		return "true", nil
	}
	return "", fmt.Errorf("the following configuration is not supported: OTEL_LOG_LEVEL=%v", ot)
}

// mapEnabled maps OTEL_TRACES_EXPORTER to DD_TRACE_ENABLED
func mapEnabled(ot string) (string, error) {
	if strings.TrimSpace(strings.ToLower(ot)) == "none" {
		return "false", nil
	}
	return "", fmt.Errorf("the following configuration is not supported: OTEL_TRACES_EXPORTER=%v", ot)
}

// mapSampleRate maps OTEL_TRACES_SAMPLER to DD_TRACE_SAMPLE_RATE
func otelTraceIDRatio() string {
	if v := env.Get("OTEL_TRACES_SAMPLER_ARG"); v != "" {
		return v
	}
	return "1.0"
}

// mapSampleRate maps OTEL_TRACES_SAMPLER to DD_TRACE_SAMPLE_RATE
func mapSampleRate(ot string) (string, error) {
	ot = strings.TrimSpace(strings.ToLower(ot))
	if v, ok := unsupportedSamplerMapping[ot]; ok {
		log.Warn("The following configuration is not supported: OTEL_TRACES_SAMPLER=%s. %s will be used", ot, v)
		ot = v
	}

	var samplerMapping = map[string]string{
		"parentbased_always_on":    "1.0",
		"parentbased_always_off":   "0.0",
		"parentbased_traceidratio": otelTraceIDRatio(),
	}
	if v, ok := samplerMapping[ot]; ok {
		return v, nil
	}
	return "", fmt.Errorf("unknown sampling configuration %v", ot)
}

// mapPropagationStyle maps OTEL_PROPAGATORS to DD_TRACE_PROPAGATION_STYLE
func mapPropagationStyle(ot string) (string, error) {
	ot = strings.TrimSpace(strings.ToLower(ot))
	supportedStyles := make([]string, 0)
	for _, otStyle := range strings.Split(ot, ",") {
		otStyle = strings.TrimSpace(otStyle)
		if _, ok := propagationMapping[otStyle]; ok {
			supportedStyles = append(supportedStyles, propagationMapping[otStyle])
		} else {
			log.Warn("Invalid configuration: %q is not supported. This propagation style will be ignored.", otStyle)
		}
	}
	return strings.Join(supportedStyles, ","), nil
}

// mapDDTags maps OTEL_RESOURCE_ATTRIBUTES to DD_TAGS
func mapDDTags(ot string) (string, error) {
	ddTags := make([]string, 0)
	internal.ForEachStringTag(ot, internal.OtelTagsDelimeter, func(key, val string) {
		// replace otel delimiter with dd delimiter and normalize tag names
		if ddkey, ok := ddTagsMapping[key]; ok {
			// map reserved otel tag names to dd tag names
			ddTags = append([]string{ddkey + internal.DDTagsDelimiter + val}, ddTags...)
		} else {
			ddTags = append(ddTags, key+internal.DDTagsDelimiter+val)
		}
	})

	if len(ddTags) > 10 {
		log.Warn("The following resource attributes have been dropped: %v. Only the first 10 resource attributes will be applied: %s", ddTags[10:], ddTags[:10]) //nolint:gocritic // Slice logging for debugging
		ddTags = ddTags[:10]
	}

	return strings.Join(ddTags, ","), nil
}
