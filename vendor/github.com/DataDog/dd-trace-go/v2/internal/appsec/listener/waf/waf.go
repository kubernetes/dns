// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package waf

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/appsec/events"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/dyngo"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/actions"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/addresses"
	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/emitter/waf"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/limiter"
	"github.com/DataDog/dd-trace-go/v2/internal/appsec/listener"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/stacktrace"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
	"github.com/DataDog/go-libddwaf/v4"
	"github.com/DataDog/go-libddwaf/v4/timer"
)

type Feature struct {
	timeout         time.Duration
	limiter         *limiter.TokenTicker
	handle          *libddwaf.Handle
	supportedAddrs  config.AddressSet
	rulesVersion    string
	reportRulesTags sync.Once

	telemetryMetrics waf.HandleMetrics

	// Determine if we can use [internal.MetaStructValue] to delegate the WAF events serialization to the trace writer
	// or if we have to use the [SerializableTag] method to serialize the events
	metaStructAvailable bool
}

func NewWAFFeature(cfg *config.Config, rootOp dyngo.Operation) (listener.Feature, error) {
	if ok, err := libddwaf.Load(); err != nil {
		// 1. If there is an error and the loading is not ok: log as an unexpected error case and quit appsec
		// Note that we assume here that the test for the unsupported target has been done before calling
		// this method, so it is now considered an error for this method
		if !ok {
			return nil, fmt.Errorf("error while loading libddwaf: %w", err)
		}
		// 2. If there is an error and the loading is ok: log as an informative error where appsec can be used
		logger := telemetrylog.With(telemetry.WithTags([]string{"product:appsec"}))
		logger.Warn("appsec: non-critical error while loading libddwaf", slog.Any("error", telemetrylog.NewSafeError(err)))
	}

	newHandle, rulesVersion := cfg.NewHandle()
	telemetryMetrics := waf.NewMetricsInstance(newHandle, rulesVersion)
	if newHandle == nil {
		// As specified @ https://docs.google.com/document/d/1t6U7WXko_QChhoNIApn0-CRNe6SAKuiiAQIyCRPUXP4/edit?tab=t.0#bookmark=id.vddhd140geg7
		logger := telemetrylog.With(telemetry.WithTags([]string{
			"log_type:rc::asm_dd::diagnostic",
			"appsec_config_key:*",
			"rc_config_id:*",
		}))
		logger.Error("Failed to build WAF instance: no valid rules or processors available")
		return nil, fmt.Errorf("failed to obtain WAF instance from the waf.Builder (loaded paths: %q)", cfg.WAFManager.ConfigPaths(""))
	}

	cfg.SupportedAddresses = config.NewAddressSet(newHandle.Addresses())

	tokenTicker := limiter.NewTokenTicker(cfg.TraceRateLimit, cfg.TraceRateLimit)
	tokenTicker.Start()

	feature := &Feature{
		handle:              newHandle,
		timeout:             cfg.WAFTimeout,
		limiter:             tokenTicker,
		supportedAddrs:      cfg.SupportedAddresses,
		telemetryMetrics:    telemetryMetrics,
		metaStructAvailable: cfg.MetaStructAvailable,
		rulesVersion:        rulesVersion,
	}

	dyngo.On(rootOp, feature.onStart)
	dyngo.OnFinish(rootOp, feature.onFinish)

	return feature, nil
}

func (waf *Feature) onStart(op *waf.ContextOperation, _ waf.ContextArgs) {
	waf.reportRulesTags.Do(func() {
		AddRulesMonitoringTags(op)
	})

	ctx, err := waf.handle.NewContext(timer.WithBudget(waf.timeout), timer.WithComponents(addresses.Scopes[:]...))
	if err != nil {
		log.Debug("appsec: failed to create WAF context: %s", err.Error())
	}

	op.SwapContext(ctx)
	op.SetLimiter(waf.limiter)
	op.SetSupportedAddresses(waf.supportedAddrs)
	op.SetMetricsInstance(waf.telemetryMetrics.NewContextMetrics())

	// Run the WAF with the given address data
	dyngo.OnData(op, op.OnEvent)

	waf.SetupActionHandlers(op)
}

func (*Feature) SetupActionHandlers(op *waf.ContextOperation) {
	// Set the blocking tag on the operation when a blocking event is received
	dyngo.OnData(op, func(*events.BlockingSecurityEvent) {
		log.Debug("appsec: blocking event detected")
		op.SetTag(blockedRequestTag, true)
		op.SetRequestBlocked()
	})

	// Register the stacktrace if one is requested by a WAF action
	dyngo.OnData(op, func(action *actions.StackTraceAction) {
		log.Debug("appsec: registering stack trace for security purposes")
		op.AddStackTraces(action.Event)
	})

	dyngo.OnData(op, func(*waf.SecurityEvent) {
		log.Debug("appsec: WAF detected a suspicious event")
		SetEventSpanTags(op)
	})
}

func (waf *Feature) onFinish(op *waf.ContextOperation, _ waf.ContextRes) {
	ctx := op.SwapContext(nil)
	if ctx == nil {
		return
	}

	ctx.Close()

	truncations := ctx.Truncations()
	timerStats := ctx.Timer.Stats()
	metrics := op.GetMetricsInstance()
	AddWAFMonitoringTags(op, metrics, waf.rulesVersion, truncations, timerStats)
	metrics.Submit(truncations, timerStats)

	if wafEvents := op.Events(); len(wafEvents) > 0 {
		tagValue := map[string][]any{"triggers": wafEvents}
		if waf.metaStructAvailable {
			op.SetTag("appsec", internal.MetaStructValue{Value: tagValue})
		} else {
			op.SetSerializableTag("_dd.appsec.json", tagValue)
		}
	}

	op.SetSerializableTags(op.Derivatives())
	if stacks := op.StackTraces(); len(stacks) > 0 {
		op.SetTag(stacktrace.SpanKey, stacktrace.GetSpanValue(stacks...))
	}
}

func (*Feature) String() string {
	return "Web Application Firewall"
}

func (waf *Feature) Stop() {
	waf.limiter.Stop()
	waf.handle.Close()
}
