// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.
package tracer

import (
	"fmt"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/stableconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

// otelDDEnv contains env vars from both dd (DD) and ot (OTEL) that map to the same tracer configuration
// remapper contains functionality to remap OTEL values to DD values
type otelDDEnv struct {
	dd       string
	ot       string
	remapper func(string) (string, error)
	handsOff bool // if true, check for configuration set in application_monitoring.yaml file
}

var otelDDConfigs = map[string]*otelDDEnv{
	"service": {
		dd:       "DD_SERVICE",
		ot:       "OTEL_SERVICE_NAME",
		remapper: mapService,
		handsOff: false,
	},
	"metrics": {
		dd:       "DD_RUNTIME_METRICS_ENABLED",
		ot:       "OTEL_METRICS_EXPORTER",
		remapper: mapMetrics,
		handsOff: true,
	},
	"debugMode": {
		dd:       "DD_TRACE_DEBUG",
		ot:       "OTEL_LOG_LEVEL",
		remapper: mapLogLevel,
		handsOff: true,
	},
	"enabled": {
		dd:       "DD_TRACE_ENABLED",
		ot:       "OTEL_TRACES_EXPORTER",
		remapper: mapEnabled,
		handsOff: false,
	},
	"sampleRate": {
		dd:       "DD_TRACE_SAMPLE_RATE",
		ot:       "OTEL_TRACES_SAMPLER",
		remapper: mapSampleRate,
		handsOff: false,
	},
	"propagationStyle": {
		dd:       "DD_TRACE_PROPAGATION_STYLE",
		ot:       "OTEL_PROPAGATORS",
		remapper: mapPropagationStyle,
		handsOff: false,
	},
	"resourceAttributes": {
		dd:       "DD_TAGS",
		ot:       "OTEL_RESOURCE_ATTRIBUTES",
		remapper: mapDDTags,
		handsOff: false,
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

// getDDorOtelConfig determines whether the provided otelDDOpt will be set via DD or OTEL env vars, and returns the value
func getDDorOtelConfig(configName string) string {
	config, ok := otelDDConfigs[configName]
	if !ok {
		panic(fmt.Sprintf("Programming Error: %v not found in supported configurations", configName))
	}

	// 1. Check managed stable config if handsOff
	if config.handsOff {
		if v := stableconfig.ManagedConfig.Get(config.dd); v != "" {
			telemetry.RegisterAppConfigs(telemetry.Configuration{Name: telemetry.EnvToTelemetryName(config.dd), Value: v, Origin: telemetry.OriginManagedStableConfig, ID: stableconfig.ManagedConfig.GetID()})
			return v
		}
	}

	// 2. Check environment variables (DD or OT)
	val := env.Get(config.dd)
	key := config.dd // Store the environment variable that will be used to set the config
	if otVal := env.Get(config.ot); otVal != "" {
		ddPrefix := "config_datadog:"
		otelPrefix := "config_opentelemetry:"
		if val != "" {
			log.Warn("Both %q and %q are set, using %s=%s", config.ot, config.dd, config.dd, val)
			telemetryTags := []string{ddPrefix + strings.ToLower(config.dd), otelPrefix + strings.ToLower(config.ot)}
			telemetry.Count(telemetry.NamespaceTracers, "otel.env.hiding", telemetryTags).Submit(1)
		} else {
			v, err := config.remapper(otVal)
			if err != nil {
				log.Warn("%s", err.Error())
				telemetryTags := []string{ddPrefix + strings.ToLower(config.dd), otelPrefix + strings.ToLower(config.ot)}
				telemetry.Count(telemetry.NamespaceTracers, "otel.env.invalid", telemetryTags).Submit(1)
			}
			key = config.ot
			val = v
		}
	}
	if val != "" {
		telemetry.RegisterAppConfig(telemetry.EnvToTelemetryName(key), val, telemetry.OriginEnvVar)
		return val
	}

	// 3. If handsOff, check local stable config
	if config.handsOff {
		if v := stableconfig.LocalConfig.Get(config.dd); v != "" {
			telemetry.RegisterAppConfigs(telemetry.Configuration{Name: telemetry.EnvToTelemetryName(config.dd), Value: v, Origin: telemetry.OriginLocalStableConfig, ID: stableconfig.LocalConfig.GetID()})
			return v
		}
	}

	// 4. Not found, return empty string
	return ""
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
	return "", fmt.Errorf("the following configuration is not supported: OTEL_METRICS_EXPORTER=%v", ot)
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
