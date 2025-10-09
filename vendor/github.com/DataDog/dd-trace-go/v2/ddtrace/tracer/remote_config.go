// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/globalconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/remoteconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type configData struct {
	Action        string    `json:"action"`
	ServiceTarget target    `json:"service_target"`
	LibConfig     libConfig `json:"lib_config"`
}

type target struct {
	Service string `json:"service"`
	Env     string `json:"env"`
}

type libConfig struct {
	Enabled            *bool             `json:"tracing_enabled,omitempty"`
	SamplingRate       *float64          `json:"tracing_sampling_rate,omitempty"`
	TraceSamplingRules *[]rcSamplingRule `json:"tracing_sampling_rules,omitempty"`
	HeaderTags         *headerTags       `json:"tracing_header_tags,omitempty"`
	Tags               *tags             `json:"tracing_tags,omitempty"`
}

type rcTag struct {
	Key       string `json:"key"`
	ValueGlob string `json:"value_glob"`
}

// Sampling rules provided by the remote config define tags differently other than using a map.
type rcSamplingRule struct {
	Service    string     `json:"service"`
	Provenance provenance `json:"provenance"`
	Name       string     `json:"name,omitempty"`
	Resource   string     `json:"resource"`
	Tags       []rcTag    `json:"tags,omitempty"`
	SampleRate float64    `json:"sample_rate"`
}

func convertRemoteSamplingRules(rules *[]rcSamplingRule) *[]SamplingRule {
	if rules == nil {
		return nil
	}
	var convertedRules []SamplingRule
	for _, rule := range *rules {
		if rule.Tags != nil {
			tags := make(map[string]*regexp.Regexp, len(rule.Tags))
			tagsStrs := make(map[string]string, len(rule.Tags))
			for _, tag := range rule.Tags {
				tags[tag.Key] = globMatch(tag.ValueGlob)
				tagsStrs[tag.Key] = tag.ValueGlob
			}
			x := SamplingRule{
				Service:    globMatch(rule.Service),
				Name:       globMatch(rule.Name),
				Resource:   globMatch(rule.Resource),
				Rate:       rule.SampleRate,
				Tags:       tags,
				Provenance: rule.Provenance,
				globRule: &jsonRule{
					Name:     rule.Name,
					Service:  rule.Service,
					Resource: rule.Resource,
					Tags:     tagsStrs,
				},
			}

			convertedRules = append(convertedRules, x)
		} else {
			x := SamplingRule{
				Service:    globMatch(rule.Service),
				Name:       globMatch(rule.Name),
				Resource:   globMatch(rule.Resource),
				Rate:       rule.SampleRate,
				Provenance: rule.Provenance,
				globRule:   &jsonRule{Name: rule.Name, Service: rule.Service, Resource: rule.Resource},
			}
			convertedRules = append(convertedRules, x)
		}
	}
	return &convertedRules
}

type headerTags []headerTag

type headerTag struct {
	Header  string `json:"header"`
	TagName string `json:"tag_name"`
}

func (hts *headerTags) toSlice() *[]string {
	if hts == nil {
		return nil
	}
	s := make([]string, len(*hts))
	for i, ht := range *hts {
		s[i] = ht.toString()
	}
	return &s
}

func (ht headerTag) toString() string {
	var sb strings.Builder
	sb.WriteString(ht.Header)
	sb.WriteString(":")
	sb.WriteString(ht.TagName)
	return sb.String()
}

type tags []string

func (t *tags) toMap() *map[string]interface{} {
	if t == nil {
		return nil
	}
	m := make(map[string]interface{}, len(*t))
	for _, tag := range *t {
		if kv := strings.SplitN(tag, ":", 2); len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	return &m
}

// onRemoteConfigUpdate is a remote config callaback responsible for processing APM_TRACING RC-product updates.
func (t *tracer) onRemoteConfigUpdate(u remoteconfig.ProductUpdate) map[string]state.ApplyStatus {
	statuses := map[string]state.ApplyStatus{}
	if len(u) == 0 {
		return statuses
	}
	removed := func() bool {
		// Returns true if all the values in the update are nil.
		for _, raw := range u {
			if raw != nil {
				return false
			}
		}
		return true
	}
	var telemConfigs []telemetry.Configuration
	if removed() {
		// The remote-config client is signaling that the configuration has been deleted for this product.
		// We re-apply the startup configuration values.
		for path := range u {
			log.Debug("Nil payload from RC. Path: %s.", path)
			statuses[path] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
		}
		log.Debug("Resetting configurations")
		updated := t.config.traceSampleRate.reset()
		if updated {
			telemConfigs = append(telemConfigs, t.config.traceSampleRate.toTelemetry())
		}
		updated = t.config.traceSampleRules.reset()
		if updated {
			telemConfigs = append(telemConfigs, t.config.traceSampleRules.toTelemetry())
		}
		updated = t.config.headerAsTags.reset()
		if updated {
			telemConfigs = append(telemConfigs, t.config.headerAsTags.toTelemetry())
		}
		updated = t.config.globalTags.reset()
		if updated {
			telemConfigs = append(telemConfigs, t.config.globalTags.toTelemetry())
		}
		if !t.config.enabled.current {
			log.Debug("APM Tracing is disabled. Restart the service to enable it.")
		}
		if len(telemConfigs) > 0 {
			log.Debug("Reporting %d configuration changes to telemetry", len(telemConfigs))
			telemetry.RegisterAppConfigs(telemConfigs...)
		}
		return statuses
	}
	for path, raw := range u {
		if raw == nil {
			continue
		}
		log.Debug("Processing config from RC. Path: %s. Raw: %s", path, raw)
		var c configData
		if err := json.Unmarshal(raw, &c); err != nil {
			log.Debug("Error while unmarshalling payload for %q: %v. Configuration won't be applied.", path, err.Error())
			statuses[path] = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
			continue
		}
		statuses[path] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
		updated := t.config.traceSampleRate.handleRC(c.LibConfig.SamplingRate)
		if updated {
			telemConfigs = append(telemConfigs, t.config.traceSampleRate.toTelemetry())
		}
		updated = t.config.traceSampleRules.handleRC(convertRemoteSamplingRules(c.LibConfig.TraceSamplingRules))
		if updated {
			telemConfigs = append(telemConfigs, t.config.traceSampleRules.toTelemetry())
		}
		updated = t.config.headerAsTags.handleRC(c.LibConfig.HeaderTags.toSlice())
		if updated {
			telemConfigs = append(telemConfigs, t.config.headerAsTags.toTelemetry())
		}
		updated = t.config.globalTags.handleRC(c.LibConfig.Tags.toMap())
		if updated {
			telemConfigs = append(telemConfigs, t.config.globalTags.toTelemetry())
		}
		if c.LibConfig.Enabled != nil {
			if t.config.enabled.current && !*c.LibConfig.Enabled {
				log.Debug("Disabled APM Tracing through RC. Restart the service to enable it.")
				t.config.enabled.handleRC(c.LibConfig.Enabled)
				telemConfigs = append(telemConfigs, t.config.enabled.toTelemetry())
			} else if !t.config.enabled.current && *c.LibConfig.Enabled {
				log.Debug("APM Tracing is disabled. Restart the service to enable it.")
			}
		}
	}
	if len(telemConfigs) > 0 {
		log.Debug("Reporting %d configuration changes to telemetry", len(telemConfigs))
		telemetry.RegisterAppConfigs(telemConfigs...)
	}
	return statuses
}

type dynamicInstrumentationRCProbeConfig struct {
	configPath    string
	configContent string
}

type dynamicInstrumentationRCState struct {
	sync.Mutex
	state map[string]dynamicInstrumentationRCProbeConfig

	// symdbExport is a flag that indicates that this tracer is resposible
	// for uploading symbols to the symbol database. The tracer will learn
	// about this fact through the callbacks like the other dynamic
	// instrumentation RC callbacks.
	//
	// The system is designed such that only a single tracer at a time is
	// responsible for uploading symbols to the symbol database. This is
	// communicated through a single RC key with a constant value. In order to
	// simplify the internal state of the tracer an avoid risks of excess memory
	// usage, we use a single boolean flag to track this state as opposed to
	// tracking the actual RC key and value.
	symdbExport bool
}

var (
	diRCState   dynamicInstrumentationRCState
	initalizeRC sync.Once
)

func (t *tracer) dynamicInstrumentationRCUpdate(u remoteconfig.ProductUpdate) map[string]state.ApplyStatus {
	applyStatus := make(map[string]state.ApplyStatus, len(u))

	diRCState.Lock()
	defer diRCState.Unlock()
	for k, v := range u {
		log.Debug("Received dynamic instrumentation RC configuration for %s\n", k)
		if len(v) == 0 {
			delete(diRCState.state, k)
			applyStatus[k] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
		} else {
			diRCState.state[k] = dynamicInstrumentationRCProbeConfig{
				configPath:    k,
				configContent: string(v),
			}
			applyStatus[k] = state.ApplyStatus{State: state.ApplyStateUnknown}
		}
	}
	return applyStatus
}

func (t *tracer) dynamicInstrumentationSymDBRCUpdate(
	u remoteconfig.ProductUpdate,
) map[string]state.ApplyStatus {
	applyStatus := make(map[string]state.ApplyStatus, len(u))
	diRCState.Lock()
	defer diRCState.Unlock()
	symDBEnabled := false
	for k, v := range u {
		if len(v) == 0 {
			applyStatus[k] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
		} else {
			applyStatus[k] = state.ApplyStatus{State: state.ApplyStateUnknown}
			symDBEnabled = true
		}
	}
	diRCState.symdbExport = symDBEnabled
	return applyStatus
}

// passProbeConfiguration is used as a stable interface to find the
// configuration in via bpf. Go-DI attaches a bpf program to this function and
// extracts the raw bytes accordingly.
//
//nolint:all
//go:noinline
func passProbeConfiguration(runtimeID, configPath, configContent string) {}

// passAllProbeConfigurationsComplete is used to signal to the bpf program that
// all probe configurations have been passed.
//
//nolint:all
//go:noinline
func passAllProbeConfigurationsComplete(runtimeID string) {}

// passSymDBState is used as a stable interface to find the symbol database
// state via bpf. Go-DI attaches a bpf program to this function and extracts
// the arguments accordingly.
//
//nolint:all
//go:noinline
func passSymDBState(runtimeID string, enabled bool) {}

// passAllProbeConfigurations is used to pass all probe configurations to the
// bpf program.
//
//go:noinline
func passAllProbeConfigurations(runtimeID string) {
	defer passAllProbeConfigurationsComplete(runtimeID)
	diRCState.Lock()
	defer diRCState.Unlock()
	for _, v := range diRCState.state {
		accessStringsToMitigatePageFault(runtimeID, v.configPath, v.configContent)
		passProbeConfiguration(runtimeID, v.configPath, v.configContent)
	}
	passSymDBState(runtimeID, diRCState.symdbExport)
}

func initalizeDynamicInstrumentationRemoteConfigState() {
	diRCState = dynamicInstrumentationRCState{
		state: map[string]dynamicInstrumentationRCProbeConfig{},
	}

	go func() {
		for {
			time.Sleep(time.Second * 5)
			passAllProbeConfigurations(globalconfig.RuntimeID())
		}
	}()
}

// accessStringsToMitigatePageFault iterates over each string to trigger a page fault,
// ensuring it is loaded into RAM or listed in the translation lookaside buffer.
// This is done by writing the string to io.Discard.
//
// This function addresses an issue with the bpf program that hooks the
// `passProbeConfiguration()` function from system-probe. The bpf program fails
// to read strings if a page fault occurs because the `bpf_probe_read()` helper
// disables paging (uprobe bpf programs can't sleep). Consequently, page faults
// cause `bpf_probe_read()` to return an error and not read any data.
// By preloading the strings, we mitigate this issue, enhancing the reliability
// of the Go Dynamic Instrumentation product.
func accessStringsToMitigatePageFault(strs ...string) {
	for i := range strs {
		io.WriteString(io.Discard, strs[i])
	}
}

// startRemoteConfig starts the remote config client. It registers the
// APM_TRACING product unconditionally and it registers the LIVE_DEBUGGING and
// LIVE_DEBUGGING_SYMBOL_DB with their respective callbacks if the tracer is
// configured to use the dynamic instrumentation product.
func (t *tracer) startRemoteConfig(rcConfig remoteconfig.ClientConfig) error {
	err := remoteconfig.Start(rcConfig)
	if err != nil {
		return err
	}

	var dynamicInstrumentationError, apmTracingError error

	if t.config.dynamicInstrumentationEnabled {
		liveDebuggingError := remoteconfig.Subscribe(
			"LIVE_DEBUGGING", t.dynamicInstrumentationRCUpdate,
		)
		liveDebuggingSymDBError := remoteconfig.Subscribe(
			"LIVE_DEBUGGING_SYMBOL_DB", t.dynamicInstrumentationSymDBRCUpdate,
		)
		if liveDebuggingError != nil && liveDebuggingSymDBError != nil {
			dynamicInstrumentationError = errors.Join(
				liveDebuggingError,
				liveDebuggingSymDBError,
			)
		} else if liveDebuggingError != nil {
			dynamicInstrumentationError = liveDebuggingError
		} else if liveDebuggingSymDBError != nil {
			dynamicInstrumentationError = liveDebuggingSymDBError
		}
	}

	initalizeRC.Do(initalizeDynamicInstrumentationRemoteConfigState)

	apmTracingError = remoteconfig.Subscribe(
		state.ProductAPMTracing,
		t.onRemoteConfigUpdate,
		remoteconfig.APMTracingSampleRate,
		remoteconfig.APMTracingHTTPHeaderTags,
		remoteconfig.APMTracingCustomTags,
		remoteconfig.APMTracingEnabled,
		remoteconfig.APMTracingSampleRules,
	)

	if apmTracingError != nil || dynamicInstrumentationError != nil {
		return fmt.Errorf(
			"could not subscribe to at least one remote config product: %w; %w",
			apmTracingError,
			dynamicInstrumentationError,
		)
	}

	return nil
}
