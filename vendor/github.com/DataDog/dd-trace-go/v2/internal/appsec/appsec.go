// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package appsec

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/DataDog/go-libddwaf/v4"

	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	globalinternal "github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/listener"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
)

// Enabled returns true when AppSec is up and running. Meaning that the appsec build tag is enabled, the env var
// DD_APPSEC_ENABLED is set to true, and the tracer is started.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return activeAppSec != nil && activeAppSec.started
}

// RASPEnabled returns true when DD_APPSEC_RASP_ENABLED=true or is unset. Granted that AppSec is enabled.
func RASPEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return activeAppSec != nil && activeAppSec.started && activeAppSec.cfg.RASP
}

// Start AppSec when enabled is enabled by both using the appsec build tag and
// setting the environment variable DD_APPSEC_ENABLED to true.
func Start(opts ...config.StartOption) {
	// TODO: Add support to configure the tracer via a public interface
	if globalinternal.BoolEnv("_DD_APPSEC_BLOCKING_UNAVAILABLE", false) {
		opts = append(opts, config.WithBlockingUnavailable(true))
	}
	if globalinternal.BoolEnv("_DD_APPSEC_PROXY_ENVIRONMENT", false) {
		opts = append(opts, config.WithProxyEnvironment())
	}

	startConfig := config.NewStartConfig(opts...)

	// AppSec can start either:
	// 1. Manually thanks to DD_APPSEC_ENABLED (or via [config.WithEnablementMode])
	// 2. Remotely when DD_APPSEC_ENABLED is undefined
	// Note: DD_APPSEC_ENABLED=false takes precedence over remote configuration
	// and enforces to have AppSec disabled.
	mode, modeOrigin, err := startConfig.EnablementMode()
	if err != nil {
		// Even when DD_APPSEC_ENABLED is an empty string or unset, an error can be returned here
		log.Debug("appsec: could not determine the AppSec enablement mode from DD_APPSEC_ENABLED: %s", err.Error())
	}

	defer registerAppsecStartTelemetry(mode, modeOrigin)

	if mode == config.ForcedOff {
		log.Debug("appsec: disabled by the configuration: set the environment variable DD_APPSEC_ENABLED to true to enable it")
		return
	}

	// Check whether libddwaf - required for Threats Detection - is ok or not
	if ok, err := libddwaf.Usable(); !ok && err != nil {
		// We need to avoid logging an error to APM tracing users who don't necessarily intend to enable appsec
		if mode == config.ForcedOn {
			logUnexpectedStartError(err)
		} else {
			// DD_APPSEC_ENABLED is not set so we cannot know what the intent is here, we must log a
			// debug message instead to avoid showing an error to APM-tracing-only users.
			telemetrylog.Error("appsec: remote activation of threats detection cannot be enabled", slog.Any("error", telemetrylog.NewSafeError(err)))
		}
		return
	}

	// From this point we know that AppSec is either enabled or can be enabled through remote config
	cfg, err := startConfig.NewConfig()
	if err != nil {
		logUnexpectedStartError(err)
		return
	}
	appsec := newAppSec(cfg)

	// Start the remote configuration client
	log.Debug("appsec: starting the remote configuration client")
	if err := appsec.startRC(); err != nil {
		telemetrylog.Error("appsec: Remote config: disabled due to an instantiation error", slog.Any("error", telemetrylog.NewSafeError(err)))
	}

	if mode == config.RCStandby {
		// AppSec is not enforced by the env var and can be enabled through remote config
		log.Debug("appsec: %s is not set, appsec won't start until activated through remote configuration", config.EnvEnabled)
		if err := appsec.enableRemoteActivation(); err != nil {
			// ASM is not enabled and can't be enabled through remote configuration. Nothing more can be done.
			logUnexpectedStartError(err)
			appsec.stopRC()
			return
		}
		log.Debug("appsec: awaiting for possible remote activation")
		setActiveAppSec(appsec)
		return
	}

	if err := appsec.start(); err != nil { // AppSec is specifically enabled
		logUnexpectedStartError(err)
		appsec.stopRC()
		return
	}

	setActiveAppSec(appsec)
}

// Implement the AppSec log message C1
func logUnexpectedStartError(err error) {
	log.Error("appsec: could not start because of an unexpected error: %s\nNo security activities will be collected. Please contact support at https://docs.datadoghq.com/help/ for help.", err.Error())

	telemetrylog.Error("appsec: could not start because of an unexpected error", slog.Any("error", telemetrylog.NewSafeError(err)))
	telemetry.ProductStartError(telemetry.NamespaceAppSec, err)
}

// Stop AppSec.
func Stop() {
	setActiveAppSec(nil)
}

var (
	activeAppSec *appsec
	mu           sync.RWMutex
)

func setActiveAppSec(a *appsec) {
	mu.Lock()
	defer mu.Unlock()
	if activeAppSec != nil {
		activeAppSec.stopRC()
		activeAppSec.stop()
	}
	activeAppSec = a
}

type appsec struct {
	cfg        *config.Config
	features   []listener.Feature
	featuresMu sync.Mutex
	started    bool
}

func newAppSec(cfg *config.Config) *appsec {
	return &appsec{
		cfg: cfg,
	}
}

// Start AppSec by registering its security protections according to the configured the security rules.
func (a *appsec) start() error {
	// Load the waf to catch early errors if any
	if ok, err := libddwaf.Load(); err != nil {
		// 1. If there is an error and the loading is not ok: log as an unexpected error case and quit appsec
		// Note that we assume here that the test for the unsupported target has been done before calling
		// this method, so it is now considered an error for this method
		if !ok {
			return fmt.Errorf("error while loading libddwaf: %w", err)
		}
		// 2. If there is an error and the loading is ok: log as an informative error where appsec can be used
		log.Error("appsec: non-critical error while loading libddwaf: %s", err.Error())
	}

	// Register dyngo listeners
	if err := a.SwapRootOperation(); err != nil {
		return err
	}

	a.enableRCBlocking()
	a.enableRASP()

	a.started = true
	log.Info("appsec: up and running")

	// TODO: log the config like the APM tracer does but we first need to define
	//   an user-friendly string representation of our config and its sources

	return nil
}

// Stop AppSec by unregistering the security protections.
func (a *appsec) stop() {
	if !a.started {
		return
	}
	a.started = false
	registerAppsecStopTelemetry()
	// Disable RC blocking first so that the following is guaranteed not to be concurrent anymore.
	a.disableRCBlocking()

	a.featuresMu.Lock()
	defer a.featuresMu.Unlock()

	// Disable the currently applied instrumentation
	dyngo.SwapRootOperation(nil)

	// Close the WAF manager to release all resources associated with it
	a.cfg.WAFManager.Reset()

	// TODO: block until no more requests are using dyngo operations

	for _, feature := range a.features {
		feature.Stop()
	}

	a.features = nil
}
