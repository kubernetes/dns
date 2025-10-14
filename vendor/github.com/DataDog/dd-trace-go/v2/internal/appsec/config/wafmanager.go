// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package config

import (
	"bytes"
	"encoding/json"
	"runtime"
	"sync"

	"github.com/DataDog/appsec-internal-go/appsec"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetryLog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
	"github.com/DataDog/go-libddwaf/v4"
)

type (
	// WAFManager holds a [libddwaf.Builder] and allows managing its configuration.
	WAFManager struct {
		builder      *libddwaf.Builder
		initRules    []byte
		rulesVersion string
		closed       bool
		mu           sync.RWMutex
	}
)

const defaultRulesPath = "ASM_DD/default"

// NewWAFManager creates a new [WAFManager] with the provided [appsec.ObfuscatorConfig] and initial
// rules (if any).
func NewWAFManager(obfuscator appsec.ObfuscatorConfig, defaultRules []byte) (*WAFManager, error) {
	builder, err := libddwaf.NewBuilder(obfuscator.KeyRegex, obfuscator.ValueRegex)
	if err != nil {
		return nil, err
	}

	mgr := &WAFManager{
		builder:   builder,
		initRules: defaultRules,
	}

	if err := mgr.RestoreDefaultConfig(); err != nil {
		return nil, err
	}

	// Attach a finalizer to close the builder when it is garbage collected, in case
	// [WAFManager.Close] is not called explicitly by the user. The call to [libddwaf.Builder.Close]
	// is safe to make multiple times.
	runtime.SetFinalizer(mgr, func(m *WAFManager) { m.doClose(true) })

	return mgr, nil
}

// Reset resets the WAF manager to its initial state.
func (m *WAFManager) Reset() error {
	for _, path := range m.ConfigPaths("") {
		m.RemoveConfig(path)
	}
	return m.RestoreDefaultConfig()
}

// ConfigPaths returns the list of configuration paths currently loaded in the receiving
// [WAFManager]. This is typically used for testing purposes. An optional filter regular expression
// can be provided to limit what paths are returned.
func (m *WAFManager) ConfigPaths(filter string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.builder.ConfigPaths(filter)
}

// NewHandle returns a new [*libddwaf.Handle] (which may be nil if no valid WAF could be built) and the
// version of the rules that were used to build it.
func (m *WAFManager) NewHandle() (*libddwaf.Handle, string) {
	m.mu.RLock()
	rulesVersion := m.rulesVersion
	hdl := m.builder.Build()
	m.mu.RUnlock()
	return hdl, rulesVersion
}

// Close releases all resources associated with this [WAFManager].
func (m *WAFManager) Close() {
	m.doClose(false)
}

func (m *WAFManager) doClose(leaked bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}
	if leaked {
		telemetryLog.Warn("WAFManager was leaked and is being closed by GC. Remember to call WAFManager.Close() explicitly!")
	}

	m.builder.Close()
	m.rulesVersion = ""
	m.closed = true
}

// RemoveConfig removes a configuration from the receiving [WAFManager].
func (m *WAFManager) RemoveConfig(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builder.RemoveConfig(path)
}

// RemoveDefaultConfig removes the initial configuration from the receiving [WAFManager]. Returns
// true if the default config was actually removed; false otherwise (e.g, if it had previously been
// removed, or there was no default config to begin with).
func (m *WAFManager) RemoveDefaultConfig() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.builder.RemoveConfig(defaultRulesPath)
}

// AddOrUpdateConfig adds or updates a configuration in the receiving [WAFManager].
func (m *WAFManager) AddOrUpdateConfig(path string, fragment any) (libddwaf.Diagnostics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	diag, err := m.builder.AddOrUpdateConfig(path, fragment)
	if err == nil && diag.Version != "" {
		m.rulesVersion = diag.Version
	}

	// Submit the telemetry metrics for error counts obtained from the [libddwaf.Diagnostics] object.
	// See: https://docs.google.com/document/d/1lcCvURsWTS_p01-MvrI6SmDB309L1e8bx9txuUR1zCk/edit?tab=t.0#heading=h.nwzm8andnx41
	eventRulesVersion := diag.Version
	if eventRulesVersion == "" {
		eventRulesVersion = m.rulesVersion
	}
	diag.EachFeature(updateTelemetryMetrics(eventRulesVersion))

	return diag, err
}

// RestoreDefaultConfig restores the initial configurations to the receiving [WAFManager].
func (m *WAFManager) RestoreDefaultConfig() error {
	if m.initRules == nil {
		return nil
	}
	var rules map[string]any
	dec := json.NewDecoder(bytes.NewReader(m.initRules))
	dec.UseNumber()
	if err := dec.Decode(&rules); err != nil {
		return err
	}
	diag, err := m.AddOrUpdateConfig(defaultRulesPath, rules)
	diag.EachFeature(logLocalDiagnosticMessages)
	return err
}

func logLocalDiagnosticMessages(name string, feature *libddwaf.Feature) {
	if feature.Error != "" {
		telemetryLog.Error("%s", feature.Error, telemetry.WithTags([]string{"appsec_config_key:" + name, "log_type:local::diagnostic"}))
	}
	for msg, ids := range feature.Errors {
		telemetryLog.Error("%s: %q", msg, ids, telemetry.WithTags([]string{"appsec_config_key:" + name, "log_type:local::diagnostic"}))
	}
	for msg, ids := range feature.Warnings {
		telemetryLog.Warn("%s: %q", msg, ids, telemetry.WithTags([]string{"appsec_config_key:" + name, "log_type:local::diagnostic"}))
	}
}

func updateTelemetryMetrics(eventRulesVersion string) func(name string, feat *libddwaf.Feature) {
	return func(name string, feat *libddwaf.Feature) {
		errCount := telemetry.Count(telemetry.NamespaceAppSec, "waf.config_errors", []string{
			"waf_version:" + libddwaf.Version(),
			"event_rules_version:" + eventRulesVersion,
			"config_key:" + name,
			"scope:item",
			"action:update",
		})
		errCount.Submit(0)
		for _, ids := range feat.Errors {
			errCount.Submit(float64(len(ids)))
		}
	}
}
