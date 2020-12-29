// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tracer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/internal/globalconfig"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/version"
)

const (
	unknown = "unknown"
)

// startupInfo contains various information about the status of the tracer on startup.
type startupInfo struct {
	Date                  string            `json:"date"`                    // ISO 8601 date and time of start
	OSName                string            `json:"os_name"`                 // Windows, Darwin, Debian, etc.
	OSVersion             string            `json:"os_version"`              // Version of the OS
	Version               string            `json:"version"`                 // Tracer version
	Lang                  string            `json:"lang"`                    // "Go"
	LangVersion           string            `json:"lang_version"`            // Go version, e.g. go1.13
	Env                   string            `json:"env"`                     // Tracer env
	Service               string            `json:"service"`                 // Tracer Service
	AgentURL              string            `json:"agent_url"`               // The address of the agent
	AgentError            string            `json:"agent_error"`             // Any error that occurred trying to connect to agent
	Debug                 bool              `json:"debug"`                   // Whether debug mode is enabled
	AnalyticsEnabled      bool              `json:"analytics_enabled"`       // True if there is a global analytics rate set
	SampleRate            string            `json:"sample_rate"`             // The default sampling rate for the rules sampler
	SamplingRules         []SamplingRule    `json:"sampling_rules"`          // Rules used by the rules sampler
	SamplingRulesError    string            `json:"sampling_rules_error"`    // Any errors that occurred while parsing sampling rules
	Tags                  map[string]string `json:"tags"`                    // Global tags
	RuntimeMetricsEnabled bool              `json:"runtime_metrics_enabled"` // Whether or not runtime metrics are enabled
	HealthMetricsEnabled  bool              `json:"health_metrics_enabled"`  // Whether or not health metrics are enabled
	ApplicationVersion    string            `json:"dd_version"`              // Version of the user's application
	Architecture          string            `json:"architecture"`            // Architecture of host machine
	GlobalService         string            `json:"global_service"`          // Global service string. If not-nil should be same as Service. (#614)
	LambdaMode            string            `json:"lambda_mode"`             // Whether or not the client has enabled lambda mode
}

// checkEndpoint tries to connect to the URL specified by endpoint.
// If the endpoint is not reachable, checkEndpoint returns an error
// explaining why.
func checkEndpoint(endpoint string) error {
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader([]byte{0x90}))
	if err != nil {
		return fmt.Errorf("cannot create http request: %v", err)
	}
	req.Header.Set(traceCountHeader, "0")
	req.Header.Set("Content-Type", "application/msgpack")
	_, err = defaultClient.Do(req)
	if err != nil {
		return err
	}
	return nil
}

// logStartup generates a startupInfo for a tracer and writes it to the log in
// JSON format.
func logStartup(t *tracer) {
	tags := make(map[string]string)
	for k, v := range t.globalTags {
		tags[k] = fmt.Sprintf("%v", v)
	}

	info := startupInfo{
		Date:                  time.Now().Format(time.RFC3339),
		OSName:                osName(),
		OSVersion:             osVersion(),
		Version:               version.Tag,
		Lang:                  "Go",
		LangVersion:           runtime.Version(),
		Env:                   t.config.env,
		Service:               t.config.serviceName,
		AgentURL:              t.transport.endpoint(),
		Debug:                 t.config.debug,
		AnalyticsEnabled:      !math.IsNaN(globalconfig.AnalyticsRate()),
		SampleRate:            fmt.Sprintf("%f", t.rulesSampling.globalRate),
		SamplingRules:         t.rulesSampling.rules,
		Tags:                  tags,
		RuntimeMetricsEnabled: t.config.runtimeMetrics,
		HealthMetricsEnabled:  t.config.runtimeMetrics,
		ApplicationVersion:    t.config.version,
		Architecture:          runtime.GOARCH,
		GlobalService:         globalconfig.ServiceName(),
		LambdaMode:            fmt.Sprintf("%t", t.config.logToStdout),
	}
	if _, err := samplingRulesFromEnv(); err != nil {
		info.SamplingRulesError = fmt.Sprintf("%s", err)
	}
	if !t.config.logToStdout {
		if err := checkEndpoint(t.transport.endpoint()); err != nil {
			info.AgentError = fmt.Sprintf("%s", err)
			log.Warn("DIAGNOSTICS Unable to reach agent: %s", err)
		}
	}
	bs, err := json.Marshal(info)
	if err != nil {
		log.Warn("DIAGNOSTICS Failed to serialize json for startup log (%v) %#v\n", err, info)
		return
	}
	log.Info("DATADOG TRACER CONFIGURATION %s\n", string(bs))
}
