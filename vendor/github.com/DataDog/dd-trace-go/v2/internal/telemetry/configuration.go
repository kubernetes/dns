// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package telemetry

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry/internal/transport"
)

type configuration struct {
	mu     sync.Mutex
	config map[string]transport.ConfKeyValue
	seqID  uint64
}

func idOrEmpty(id string) string {
	if id == EmptyID {
		return ""
	}
	return id
}

func (c *configuration) Add(kv Configuration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config == nil {
		c.config = make(map[string]transport.ConfKeyValue)
	}

	ID := idOrEmpty(kv.ID)

	c.config[kv.Name] = transport.ConfKeyValue{
		Name:   kv.Name,
		Value:  kv.Value,
		Origin: kv.Origin,
		ID:     ID,
	}
}

func (c *configuration) Payload() transport.Payload {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.config) == 0 {
		return nil
	}

	configs := make([]transport.ConfKeyValue, len(c.config))
	idx := 0
	for _, conf := range c.config {
		if conf.Origin == "" {
			conf.Origin = transport.OriginDefault
		}
		conf.Value = SanitizeConfigValue(conf.Value)
		conf.SeqID = c.seqID
		configs[idx] = conf
		idx++
		c.seqID++
		delete(c.config, conf.Name)
	}

	return transport.AppClientConfigurationChange{
		Configuration: configs,
	}
}

// SanitizeConfigValue sanitizes the value of a configuration key to ensure it can be marshalled.
func SanitizeConfigValue(value any) any {
	if value == nil {
		return ""
	}

	// Skip reflection for basic types
	switch val := value.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return val
	case float32:
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return ""
		}
		return val
	case float64:
		// https://github.com/golang/go/issues/59627
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		return val
	case []string:
		return strings.Join(val, ",") // Retro compatibility with old code
	}

	if _, ok := value.(json.Marshaler); ok {
		return value
	}

	if v, ok := value.(fmt.Stringer); ok {
		return v.String()
	}

	valueOf := reflect.ValueOf(value)

	// Unwrap pointers and interfaces up to 10 levels deep.
	for i := 0; i < 10; i++ {
		if valueOf.Kind() == reflect.Ptr || valueOf.Kind() == reflect.Interface {
			valueOf = valueOf.Elem()
		} else {
			break
		}
	}

	switch {
	case valueOf.Kind() == reflect.Slice, valueOf.Kind() == reflect.Array:
		var sb strings.Builder
		sb.WriteString("[")
		for i := 0; i < valueOf.Len(); i++ {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(fmt.Sprintf("%v", valueOf.Index(i).Interface()))
		}
		sb.WriteString("]")
		return sb.String()
	case valueOf.Kind() == reflect.Map:
		kvPair := make([]struct {
			key   string
			value string
		}, valueOf.Len())

		iter := valueOf.MapRange()
		for i := 0; iter.Next(); i++ {
			kvPair[i].key = fmt.Sprintf("%v", iter.Key().Interface())
			kvPair[i].value = fmt.Sprintf("%v", iter.Value().Interface())
		}

		slices.SortStableFunc(kvPair, func(a, b struct {
			key   string
			value string
		}) int {
			return strings.Compare(a.key, b.key)
		})

		var sb strings.Builder
		for _, k := range kvPair {
			if sb.Len() > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(k.key)
			sb.WriteString(":")
			sb.WriteString(k.value)
		}

		return sb.String()
	}

	return fmt.Sprintf("%v", value)
}

func EnvToTelemetryName(env string) string {
	switch env {
	case "DD_TRACE_DEBUG":
		return "trace_debug_enabled"
	case "DD_APM_TRACING_ENABLED":
		return "apm_tracing_enabled"
	case "DD_RUNTIME_METRICS_ENABLED":
		return "runtime_metrics_enabled"
	case "DD_DATA_STREAMS_ENABLED":
		return "data_streams_enabled"
	case "DD_APPSEC_ENABLED":
		return "appsec_enabled"
	case "DD_DYNAMIC_INSTRUMENTATION_ENABLED":
		return "dynamic_instrumentation_enabled"
	case "DD_PROFILING_ENABLED":
		return "profiling_enabled"
	default:
		return env
	}
}
