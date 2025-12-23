// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package appsec

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/orchestrion"
	"github.com/DataDog/dd-trace-go/v2/internal/remoteconfig"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
	"github.com/DataDog/go-libddwaf/v4"
)

// onRCRulesUpdate is the RC callback called when security rules related RC updates are available
func (a *appsec) onRCRulesUpdate(updates map[string]remoteconfig.ProductUpdate) map[string]state.ApplyStatus {
	statuses := make(map[string]state.ApplyStatus)

	// If appsec was deactivated through RC, stop here
	if !a.started {
		for _, pu := range updates {
			for path := range pu {
				// We are not acknowledging anything... since we are ignoring all these updates...
				statuses[path] = state.ApplyStatus{State: state.ApplyStateUnacknowledged}
			}
		}
		return statuses
	}

	// Updates the local [config.WAFManager] with the new data... We track deletions and add/updates
	// separately as it is important to process all deletions first.
	// See: https://docs.google.com/document/d/1t6U7WXko_QChhoNIApn0-CRNe6SAKuiiAQIyCRPUXP4/edit?tab=t.0#heading=h.pqke0ujtvm2j
	type UpdatedConfig struct {
		Product string
		Content map[string]any
	}
	addOrUpdates := make(map[string]UpdatedConfig)

	for product, prodUpdates := range updates {
		for path, data := range prodUpdates {
			switch product {
			case state.ProductASMDD, state.ProductASMData, state.ProductASM:
				if data == nil {
					// Perofrm the deletion right away; we need to do these before any other updates...
					log.Debug("appsec: remote config: removing configuration %q", path)
					a.cfg.WAFManager.RemoveConfig(path)
					statuses[path] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
					continue
				}
				cfg := UpdatedConfig{Product: product}
				if err := json.Unmarshal(data, &cfg.Content); err != nil {
					log.Error("appsec: unmarshaling remote config update for %s (%q): %s", product, path, err.Error())
					statuses[product] = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
					continue
				}
				addOrUpdates[path] = cfg
				if product == state.ProductASMDD {
					// Remove the default config if present when an ASM_DD config is received.
					log.Debug("appsec: remote config: processed ASM_DD addition/update; removing default config if present")
					if deletedDefault := a.cfg.WAFManager.RemoveDefaultConfig(); deletedDefault {
						log.Debug("appsec: remote config: successfully removed default config")
					}
				}
			case state.ProductASMFeatures:
				// This is a global hook, so it'll receive [remoteconfig.ProductASMFeatures] updates as well, which are not
				// relevant for this particular handler. These are handled by a product-specific handler,
				// [appsec.handleASMFeatures].
			default:
				log.Debug("appsec: remote config: ignoring RC update for non-ASM product %q", path)
			}
		}
	}

	// Sort the paths to apply updates in a deterministic order...
	addOrUpdatePaths := slices.Collect(maps.Keys(addOrUpdates))
	slices.Sort(addOrUpdatePaths)

	// Apply all the additions and updates
	for _, path := range addOrUpdatePaths {
		update := addOrUpdates[path]
		log.Debug("appsec: remote config: adding/updating configuration %q", path)
		diag, err := a.cfg.WAFManager.AddOrUpdateConfig(path, update.Content)
		if err != nil {
			log.Debug("appsec: remote config: error while adding/updating configuration %q: %s", path, err.Error())
			// Configuration object has been fully rejected; or there was an error processing it or parsing the diagnostics
			// value. If we have a diagnostics object, encode all errors from the diagnostics object as a JSON value, as
			// described by:
			// https://docs.google.com/document/d/1t6U7WXko_QChhoNIApn0-CRNe6SAKuiiAQIyCRPUXP4/edit?tab=t.0#heading=h.6ud96uy74pzs
			type errInfo struct {
				Error  string              `json:"error,omitempty"`
				Errors map[string][]string `json:"errors,omitempty"`
			}
			var errs map[string]errInfo
			diag.EachFeature(func(name string, feat *libddwaf.Feature) {
				var (
					info errInfo
					some bool
				)
				if feat.Error != "" {
					log.Debug("appsec: remote config: error in %q feature %s: %s", path, name, feat.Error)
					info.Error = feat.Error
					some = true
				}
				for msg, ids := range feat.Errors {
					log.Debug("appsec: remote config: error in %q feature %s: %s for %q", path, name, msg, ids)
					if info.Errors == nil {
						info.Errors = make(map[string][]string)
					}
					info.Errors[msg] = ids
					some = true
				}
				if !some {
					return
				}
				if errs == nil {
					errs = make(map[string]errInfo)
				}
				errs[name] = info
			})

			errMsg := err.Error()
			if len(errs) > 0 {
				if data, err := json.Marshal(errs); err == nil {
					errMsg = string(data)
				} else {
					telemetrylog.Error("appsec: remote config: failed to marshal error details", slog.Any("error", telemetrylog.NewSafeError(err)))
				}
			}

			statuses[path] = state.ApplyStatus{State: state.ApplyStateError, Error: errMsg}
			continue
		}

		statuses[path] = state.ApplyStatus{State: state.ApplyStateAcknowledged}
		diag.EachFeature(logDiagnosticMessages(update.Product, path))
	}
	if len(a.cfg.WAFManager.ConfigPaths(`^(?:datadog/\d+|employee)/ASM_DD/.+`)) == 0 {
		log.Debug("appsec: remote config: no ASM_DD config loaded; restoring default config if available")
		if err := a.cfg.WAFManager.RestoreDefaultConfig(); err != nil {
			telemetrylog.Error("appsec: RC could not restore default config", slog.Any("error", telemetrylog.NewSafeError(err)))
		}
	}

	if log.DebugEnabled() {
		// Avoiding the call to ConfigPaths if the Debug level is not enabled...
		log.Debug("appsec: remote config: rules loaded after update: %q", a.cfg.WAFManager.ConfigPaths(""))
	}

	// If an error occurs while updating the WAF handle, don't swap the RulesManager and propagate the error
	// to all config statuses since we can't know which config is the faulty one
	if err := a.SwapRootOperation(); err != nil {
		log.Error("appsec: remote config: could not apply the new security rules: %s", err.Error())
		for k := range statuses {
			if statuses[k].State == state.ApplyStateError || statuses[k].State == state.ApplyStateUnacknowledged {
				// Leave failed & un-acknowledged configs as-is... This failure is not related to these...
				continue
			}
			statuses[k] = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
		}
	}

	return statuses
}

func logDiagnosticMessages(product string, path string) func(string, *libddwaf.Feature) {
	return func(name string, feat *libddwaf.Feature) {
		if feat.Error == "" && len(feat.Errors) == 0 && len(feat.Warnings) == 0 {
			// No errors or warnings; nothing to report...
			return
		}

		path, _ := remoteconfig.ParsePath(path)
		// As defined @ https://docs.google.com/document/d/1t6U7WXko_QChhoNIApn0-CRNe6SAKuiiAQIyCRPUXP4/edit?tab=t.0#bookmark=id.cthxzqjuodhh
		logger := telemetrylog.With(telemetry.WithTags([]string{
			"log_type:rc::" + strings.ToLower(product) + "::diagnostic",
			"appsec_config_key:" + name,
			"rc_config_id:" + path.ConfigID,
		}))
		if err := feat.Error; err != "" {
			logger.Error("rule error occurred", slog.String("error", err))
		}
		for err, ids := range feat.Errors {
			logger.Error("rule error occurred", slog.String("error", err), slog.Any("affected_rule_ids", telemetrylog.NewSafeSlice(ids)))
		}
		for err, ids := range feat.Warnings {
			logger.Warn("rule warning occurred", slog.String("error", err), slog.Any("affected_rule_ids", telemetrylog.NewSafeSlice(ids)))
		}
	}
}

// handleASMFeatures deserializes an ASM_FEATURES configuration received through remote config
// and starts/stops appsec accordingly.
func (a *appsec) handleASMFeatures(u remoteconfig.ProductUpdate) map[string]state.ApplyStatus {
	if len(u) == 0 {
		// That should not actually happen; but would not be "invalid" per se.
		return nil
	}

	if len(u) > 1 {
		log.Warn("appsec: Remote Config: received multiple ASM_FEATURES update; not processing any.")
		statuses := make(map[string]state.ApplyStatus, len(u))
		for path := range u {
			statuses[path] = state.ApplyStatus{State: state.ApplyStateUnacknowledged}
		}
		return statuses
	}

	// NOTE: There is exactly 1 item in the map at this point; but it's a map, so we for-range over it.
	var (
		path string
		raw  []byte
	)
	for p, r := range u {
		path, raw = p, r
	}

	log.Debug("appsec: remote config: processing %s", path)

	// A nil config means ASM was disabled, and we stopped receiving the config file
	// Don't ack the config in this case and return early
	if raw == nil {
		log.Debug("appsec: remote config: Stopping AppSec")
		a.stop()
		return map[string]state.ApplyStatus{path: {State: state.ApplyStateAcknowledged}}
	}

	// Parse the config object we just received...
	var parsed state.ASMFeaturesData
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Error("appsec: remote config: error while unmarshalling %q: %s. Configuration won't be applied.", path, err.Error())
		return map[string]state.ApplyStatus{path: {State: state.ApplyStateError, Error: err.Error()}}
	}

	// RC triggers activation of ASM; ASM is not started yet... Starting it!
	if parsed.ASM.Enabled && !a.started {
		log.Debug("appsec: remote config: Starting AppSec")
		if err := a.start(); err != nil {
			log.Error("appsec: remote config: error while processing %q. Configuration won't be applied: %s", path, err.Error())
			return map[string]state.ApplyStatus{path: {State: state.ApplyStateError, Error: err.Error()}}
		}
		registerAppsecStartTelemetry(config.RCStandby, telemetry.OriginRemoteConfig)
	}

	// RC triggers desactivation of ASM; ASM is started... Stopping it!
	if !parsed.ASM.Enabled && a.started {
		log.Debug("appsec: remote config: Stopping AppSec")
		a.stop()
		registerAppsecStartTelemetry(config.ForcedOff, telemetry.OriginRemoteConfig)
		return map[string]state.ApplyStatus{path: {State: state.ApplyStateAcknowledged}}
	}

	// If we got here, we have an idempotent success!
	return map[string]state.ApplyStatus{path: {State: state.ApplyStateAcknowledged}}
}

func (a *appsec) startRC() error {
	if a.cfg.RC != nil {
		return remoteconfig.Start(*a.cfg.RC)
	}
	return nil
}

func (a *appsec) stopRC() {
	if a.cfg.RC != nil {
		remoteconfig.Stop()
	}
}

func (a *appsec) registerRCProduct(p string) error {
	if a.cfg.RC == nil {
		return fmt.Errorf("no valid remote configuration client")
	}
	return remoteconfig.RegisterProduct(p)
}

func (a *appsec) registerRCCapability(c remoteconfig.Capability) error {
	if a.cfg.RC == nil {
		return fmt.Errorf("no valid remote configuration client")
	}
	return remoteconfig.RegisterCapability(c)
}

func (a *appsec) unregisterRCCapability(c remoteconfig.Capability) error {
	if a.cfg.RC == nil {
		log.Debug("appsec: remote config: no valid remote configuration client")
		return nil
	}
	return remoteconfig.UnregisterCapability(c)
}

func (a *appsec) enableRemoteActivation() error {
	if a.cfg.RC == nil {
		return errors.New("no valid remote configuration client")
	}
	log.Debug("appsec: Remote Config: subscribing to ASM_FEATURES updates...")
	return remoteconfig.Subscribe(state.ProductASMFeatures, a.handleASMFeatures, remoteconfig.ASMActivation)
}

var baseCapabilities = [...]remoteconfig.Capability{
	remoteconfig.ASMDDMultiConfig,
	remoteconfig.ASMDDRules,
	remoteconfig.ASMExclusions,
	remoteconfig.ASMCustomRules,
	remoteconfig.ASMTrustedIPs,
	remoteconfig.ASMProcessorOverrides,
	remoteconfig.ASMCustomDataScanners,
	remoteconfig.ASMExclusionData,
	remoteconfig.ASMEndpointFingerprinting,
	remoteconfig.ASMSessionFingerprinting,
	remoteconfig.ASMNetworkFingerprinting,
	remoteconfig.ASMHeaderFingerprinting,
	remoteconfig.ASMTraceTaggingRules,
}

var blockingCapabilities = [...]remoteconfig.Capability{
	remoteconfig.ASMUserBlocking,
	remoteconfig.ASMRequestBlocking,
	remoteconfig.ASMIPBlocking,
	remoteconfig.ASMCustomBlockingResponse,
}

func (a *appsec) enableRCBlocking() {
	if a.cfg.RC == nil {
		log.Debug("appsec: remote config: no valid remote configuration client")
		return
	}

	products := []string{state.ProductASM, state.ProductASMDD, state.ProductASMData}
	for _, p := range products {
		if err := a.registerRCProduct(p); err != nil {
			log.Debug("appsec: remote config: couldn't register product %q: %s", p, err.Error())
		}
	}

	log.Debug("appsec: remote config: registering onRCRulesUpdate callback")
	if err := remoteconfig.RegisterCallback(a.onRCRulesUpdate); err != nil {
		log.Debug("appsec: remote config: couldn't register callback: %s", err.Error())
	}

	for _, c := range baseCapabilities {
		if err := a.registerRCCapability(c); err != nil {
			log.Debug("appsec: remote config: couldn't register capability %d: %s", c, err.Error())
		}
	}

	if localRulesPath, hasLocalRules := env.Lookup(config.EnvRules); hasLocalRules {
		log.Debug("appsec: remote config: using rules from %s; will not register blocking capabilities", localRulesPath)
		return
	}
	if !a.cfg.BlockingUnavailable {
		for _, c := range blockingCapabilities {
			if err := a.registerRCCapability(c); err != nil {
				log.Debug("appsec: remote config: couldn't register capability %d: %s", c, err.Error())
			}
		}
	}
}

func (a *appsec) enableRASP() {
	if !a.cfg.RASP {
		return
	}
	if err := remoteconfig.RegisterCapability(remoteconfig.ASMRASPSSRF); err != nil {
		log.Debug("appsec: remote config: couldn't register RASP SSRF: %s", err.Error())
	}
	if err := remoteconfig.RegisterCapability(remoteconfig.ASMRASPSQLI); err != nil {
		log.Debug("appsec: remote config: couldn't register RASP SQLI: %s", err.Error())
	}
	if orchestrion.Enabled() {
		if err := remoteconfig.RegisterCapability(remoteconfig.ASMRASPLFI); err != nil {
			log.Debug("appsec: remote config: couldn't register RASP LFI: %s", err.Error())
		}
	}
}

func (a *appsec) disableRCBlocking() {
	if a.cfg.RC == nil {
		return
	}
	for _, c := range baseCapabilities {
		if err := a.unregisterRCCapability(c); err != nil {
			log.Debug("appsec: remote config: couldn't unregister capability %d: %s", c, err.Error())
		}
	}
	if !a.cfg.BlockingUnavailable {
		for _, c := range blockingCapabilities {
			if err := a.unregisterRCCapability(c); err != nil {
				log.Debug("appsec: remote config: couldn't unregister capability %d: %s", c, err.Error())
			}
		}
	}
	if err := remoteconfig.UnregisterCallback(a.onRCRulesUpdate); err != nil {
		log.Debug("appsec: remote config: couldn't unregister callback: %s", err.Error())
	}
}
