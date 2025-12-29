// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package waf

import (
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/emitter/waf/addresses"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
	"github.com/DataDog/go-libddwaf/v4"
	"github.com/DataDog/go-libddwaf/v4/timer"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
	"github.com/puzpuzpuz/xsync/v3"
)

// newHandleTelemetryMetric is the name of the metric that will be used to track the initialization of the WAF handle
// this values is changed to waf.updates after the first call to [NewMetricsInstance]
var newHandleTelemetryMetric = "waf.init"
var changeToWafUpdates sync.Once

// RequestMilestones is a list of things that can happen as a result of a waf call. They are stacked for each requests
// and used as tags to the telemetry metric `waf.requests`.
// this struct can be modified concurrently.
// TODO: add request_excluded and block_failure to the mix once we have the capability to track them
type RequestMilestones struct {
	requestBlocked bool
	ruleTriggered  bool
	wafTimeout     bool
	rateLimited    bool
	wafError       bool
	inputTruncated bool
}

// raspMetricKey is used as a cache key for the metrics having tags depending on the RASP rule type
type raspMetricKey[T any] struct {
	typ           addresses.RASPRuleType
	additionalTag T
}

// HandleMetrics is a struct that holds all the telemetry metrics for the WAF that lives and die alongside with the WAF handle
// It basically serves as a big cache to not go through the telemetry package each time we want to submit a metric
// and have to recompute all tags that are static (from a WAF handle lifetime perspective)
type HandleMetrics struct {
	baseTags     []string
	baseRASPTags [len(addresses.RASPRuleTypes)][]string

	// Common metric types

	// externalTimerDistributions holds the telemetry metrics for the `rasp.duration_ext`, `waf.duration_ext` metrics
	externalTimerDistributions map[addresses.Scope]telemetry.MetricHandle
	// internalTimerDistributions holds the telemetry metrics for the `rasp.duration`, `waf.duration` metrics
	internalTimerDistributions map[addresses.Scope]telemetry.MetricHandle

	// wafRequestsCounts holds the telemetry metrics for the `waf.requests` metric, lazily filled
	wafRequestsCounts *xsync.MapOf[RequestMilestones, telemetry.MetricHandle]

	// Uncommon metric types

	// raspTimeout holds the telemetry metrics for the rasp.timeout metrics since there is no waf.timeout metric
	raspTimeout [len(addresses.RASPRuleTypes)]telemetry.MetricHandle
	// raspRuleEval holds the telemetry metrics for the `rasp.rule_eval` metric by rule type
	raspRuleEval [len(addresses.RASPRuleTypes)]telemetry.MetricHandle

	// Rare metric types

	// truncationCounts holds the telemetry metrics for the `waf.input_truncated` metric, lazily filled
	truncationCounts *xsync.MapOf[libddwaf.TruncationReason, telemetry.MetricHandle]
	// truncationDistributions holds the telemetry metrics for the `waf.truncated_value_size` metric, lazily filled
	truncationDistributions *xsync.MapOf[libddwaf.TruncationReason, telemetry.MetricHandle]

	// Epic metric types

	// wafErrorCount holds the telemetry metrics for the `waf.error` metric, lazily filled
	wafErrorCount *xsync.MapOf[int, telemetry.MetricHandle]
	// raspErrorCount holds the telemetry metrics for the `rasp.error` metric, lazily filled
	raspErrorCount *xsync.MapOf[raspMetricKey[int], telemetry.MetricHandle]

	// Legendary metric types

	// raspRuleMatch holds the telemetry metrics for the `rasp.rule.match` metric, lazily filled
	raspRuleMatch *xsync.MapOf[raspMetricKey[string], telemetry.MetricHandle]
}

var baseRASPTags = [len(addresses.RASPRuleTypes)][]string{
	addresses.RASPRuleTypeLFI:          {"rule_type:" + addresses.RASPRuleTypeLFI.String()},
	addresses.RASPRuleTypeSSRFRequest:  {"rule_type:" + addresses.RASPRuleTypeSSRFResponse.String(), "rule_variant:request"},
	addresses.RASPRuleTypeSSRFResponse: {"rule_type:" + addresses.RASPRuleTypeSSRFResponse.String(), "rule_variant:response"},
	addresses.RASPRuleTypeSQLI:         {"rule_type:" + addresses.RASPRuleTypeSQLI.String()},
	addresses.RASPRuleTypeCMDI:         {"rule_type:" + addresses.RASPRuleTypeCMDI.String(), "rule_variant:exec"},
}

// NewMetricsInstance creates a new HandleMetrics struct and submit the `waf.init` or `waf.updates` metric. To be called with the raw results of the WAF handle initialization
func NewMetricsInstance(newHandle *libddwaf.Handle, eventRulesVersion string) HandleMetrics {
	telemetry.Count(telemetry.NamespaceAppSec, newHandleTelemetryMetric, []string{
		"waf_version:" + libddwaf.Version(),
		"event_rules_version:" + eventRulesVersion,
		"success:" + strconv.FormatBool(newHandle != nil),
	}).Submit(1)

	changeToWafUpdates.Do(func() {
		newHandleTelemetryMetric = "waf.updates"
	})

	baseTags := []string{
		"event_rules_version:" + eventRulesVersion,
		"waf_version:" + libddwaf.Version(),
	}

	metrics := HandleMetrics{
		baseTags: baseTags,
		externalTimerDistributions: map[addresses.Scope]telemetry.MetricHandle{
			addresses.RASPScope: telemetry.Distribution(telemetry.NamespaceAppSec, "rasp.duration_ext", baseTags),
			addresses.WAFScope:  telemetry.Distribution(telemetry.NamespaceAppSec, "waf.duration_ext", baseTags),
		},
		internalTimerDistributions: map[addresses.Scope]telemetry.MetricHandle{
			addresses.RASPScope: telemetry.Distribution(telemetry.NamespaceAppSec, "rasp.duration", baseTags),
			addresses.WAFScope:  telemetry.Distribution(telemetry.NamespaceAppSec, "waf.duration", baseTags),
		},
		wafRequestsCounts:       xsync.NewMapOf[RequestMilestones, telemetry.MetricHandle](xsync.WithGrowOnly(), xsync.WithPresize(2^6)),
		truncationCounts:        xsync.NewMapOf[libddwaf.TruncationReason, telemetry.MetricHandle](xsync.WithGrowOnly(), xsync.WithPresize(2^3)),
		truncationDistributions: xsync.NewMapOf[libddwaf.TruncationReason, telemetry.MetricHandle](xsync.WithGrowOnly(), xsync.WithPresize(2^2)),
		wafErrorCount:           xsync.NewMapOf[int, telemetry.MetricHandle](xsync.WithGrowOnly(), xsync.WithPresize(2^3)),
		raspErrorCount:          xsync.NewMapOf[raspMetricKey[int], telemetry.MetricHandle](xsync.WithGrowOnly(), xsync.WithPresize(2^3)),
		raspRuleMatch:           xsync.NewMapOf[raspMetricKey[string], telemetry.MetricHandle](xsync.WithGrowOnly(), xsync.WithPresize(2^3)),
	}

	for ruleType := range metrics.baseRASPTags {
		tags := make([]string, len(baseRASPTags[ruleType])+len(baseTags))
		copy(tags, baseRASPTags[ruleType])
		copy(tags[len(baseRASPTags[ruleType]):], baseTags)
		metrics.baseRASPTags[ruleType] = tags
	}

	for ruleType := range metrics.raspRuleEval {
		metrics.raspRuleEval[ruleType] = telemetry.Count(telemetry.NamespaceAppSec, "rasp.rule.eval", metrics.baseRASPTags[ruleType])
	}

	for ruleType := range metrics.raspTimeout {
		metrics.raspTimeout[ruleType] = telemetry.Count(telemetry.NamespaceAppSec, "rasp.timeout", metrics.baseRASPTags[ruleType])
	}

	return metrics
}

func (m *HandleMetrics) NewContextMetrics() *ContextMetrics {
	return &ContextMetrics{
		HandleMetrics: m,
		SumDurations: map[addresses.Scope]map[timer.Key]*atomic.Int64{
			addresses.WAFScope: {
				libddwaf.EncodeTimeKey:   &atomic.Int64{},
				libddwaf.DurationTimeKey: &atomic.Int64{},
				libddwaf.DecodeTimeKey:   &atomic.Int64{},
			},
			addresses.RASPScope: {
				libddwaf.EncodeTimeKey:   &atomic.Int64{},
				libddwaf.DurationTimeKey: &atomic.Int64{},
				libddwaf.DecodeTimeKey:   &atomic.Int64{},
			},
		},
		logger: telemetrylog.With(telemetry.WithTags([]string{"product:appsec"})),
	}
}

type ContextMetrics struct {
	*HandleMetrics

	// SumRASPCalls is the sum of all the RASP calls made by the WAF whatever the rasp rule type it is.
	SumRASPCalls atomic.Uint32
	// SumWAFErrors is the sum of all the WAF errors that happened not in the RASP scope.
	SumWAFErrors atomic.Uint32
	// SumRASPErrors is the sum of all the RASP errors that happened in the RASP scope.
	SumRASPErrors atomic.Uint32

	// SumWAFTimeouts is the sum of all the WAF timeouts that happened not in the RASP scope.
	SumWAFTimeouts atomic.Uint32

	// SumRASPTimeouts is the sum of all the RASP timeouts that happened in the RASP scope by rule type.
	SumRASPTimeouts [len(addresses.RASPRuleTypes)]atomic.Uint32

	// SumDurations is the sum of all the run durations calls to ddwaf_run behind go-libddwaf
	// This map is built statically when ContextMetrics is created and readonly after that.
	SumDurations map[addresses.Scope]map[timer.Key]*atomic.Int64

	// Milestones are the tags of the metric `waf.requests` that will be submitted at the end of the waf context
	Milestones RequestMilestones

	// logger is a pre-configured logger with appsec product tags
	logger *telemetrylog.Logger
}

// Submit increment the metrics for the WAF run stats at the end of each waf context lifecycle
// It registers the metrics:
// - `waf.duration_ext` and `rasp.duration_ext` using [libddwaf.Context.Timer]
// - `waf.duration` and `rasp.duration` using [libddwaf.Result.TimerStats] accumulated in the ContextMetrics
// - `rasp.timeout` for the RASP scope using [libddwaf.Stats.TimeoutRASPCount]
// - `waf.input_truncated` and `waf.truncated_value_size` for the truncations using [libddwaf.Stats.Truncations]
// - `waf.requests` for the milestones using [ContextMetrics.Milestones]
func (m *ContextMetrics) Submit(truncations map[libddwaf.TruncationReason][]int, timerStats map[timer.Key]time.Duration) {
	for scope, value := range timerStats {
		// Add metrics `{waf,rasp}.duration_ext`
		metric, found := m.externalTimerDistributions[scope]
		if !found {
			m.logger.Error("unexpected scope name", slog.String("scope", string(scope)))
			continue
		}

		metric.Submit(float64(value) / float64(time.Microsecond.Nanoseconds()))

		// Add metrics `{waf,rasp}.duration`
		for key, value := range m.SumDurations[scope] {
			if key != libddwaf.DurationTimeKey {
				continue
			}

			if metric, found := m.internalTimerDistributions[scope]; found {
				metric.Submit(float64(value.Load()) / float64(time.Microsecond.Nanoseconds()))
			}
		}
	}

	for ruleTyp := range m.SumRASPTimeouts {
		if nbTimeouts := m.SumRASPTimeouts[ruleTyp].Load(); nbTimeouts > 0 {
			m.raspTimeout[ruleTyp].Submit(float64(nbTimeouts))
		}
	}

	var truncationTypes libddwaf.TruncationReason
	for reason, sizes := range truncations {
		truncationTypes |= reason
		handle, _ := m.truncationDistributions.LoadOrCompute(reason, func() telemetry.MetricHandle {
			return telemetry.Distribution(telemetry.NamespaceAppSec, "waf.truncated_value_size", []string{"truncation_reason:" + strconv.Itoa(int(reason))})
		})
		for _, size := range sizes {
			handle.Submit(float64(size))
		}
	}

	if truncationTypes != 0 {
		handle, _ := m.truncationCounts.LoadOrCompute(truncationTypes, func() telemetry.MetricHandle {
			return telemetry.Count(telemetry.NamespaceAppSec, "waf.input_truncated", []string{"truncation_reason:" + strconv.Itoa(int(truncationTypes))})
		})
		handle.Submit(1)
	}

	if len(truncations) > 0 {
		m.Milestones.inputTruncated = true
	}

	m.incWafRequestsCounts()
}

// incWafRequestsCounts increments the `waf.requests` metric with the current milestones and creates a new metric handle if it does not exist
func (m *ContextMetrics) incWafRequestsCounts() {
	handle, _ := m.wafRequestsCounts.LoadOrCompute(m.Milestones, func() telemetry.MetricHandle {
		return telemetry.Count(telemetry.NamespaceAppSec, "waf.requests", append([]string{
			"request_blocked:" + strconv.FormatBool(m.Milestones.requestBlocked),
			"rule_triggered:" + strconv.FormatBool(m.Milestones.ruleTriggered),
			"waf_timeout:" + strconv.FormatBool(m.Milestones.wafTimeout),
			"rate_limited:" + strconv.FormatBool(m.Milestones.rateLimited),
			"waf_error:" + strconv.FormatBool(m.Milestones.wafError),
			"input_truncated:" + strconv.FormatBool(m.Milestones.inputTruncated),
		}, m.baseTags...))
	})

	handle.Submit(1)
}

// RegisterWafRun register the different outputs of the WAF for the `waf.requests` and also directly increment the `rasp.rule.match` and `rasp.rule.eval` metrics.
// It registers the metrics:
// - `rasp.rule.match`
// - `rasp.rule.eval`
// It accumulate data for:
// - `waf.requests`
// - `rasp.duration`
// - `waf.duration`
func (m *ContextMetrics) RegisterWafRun(addrs libddwaf.RunAddressData, timerStats map[timer.Key]time.Duration, tags RequestMilestones) {
	for key, value := range timerStats {
		m.SumDurations[addrs.TimerKey][key].Add(int64(value))
	}

	switch addrs.TimerKey {
	case addresses.RASPScope:
		m.SumRASPCalls.Add(1)
		ruleType, ok := addresses.RASPRuleTypeFromAddressSet(addrs)
		if !ok {
			m.logger.Error("unexpected call to RASPRuleTypeFromAddressSet")
			return
		}
		if metric := m.raspRuleEval[ruleType]; metric != nil {
			metric.Submit(1)
		}
		if tags.ruleTriggered {
			blockTag := "block:irrelevant"
			if tags.requestBlocked { // TODO: add block:failure to the mix
				blockTag = "block:success"
			}

			handle, _ := m.raspRuleMatch.LoadOrCompute(raspMetricKey[string]{typ: ruleType, additionalTag: blockTag}, func() telemetry.MetricHandle {
				return telemetry.Count(telemetry.NamespaceAppSec, "rasp.rule.match", append([]string{
					blockTag,
				}, m.baseRASPTags[ruleType]...))
			})

			handle.Submit(1)
		}
		if tags.wafTimeout {
			m.SumRASPTimeouts[ruleType].Add(1)
		}
	case addresses.WAFScope, "":
		if tags.requestBlocked {
			m.Milestones.requestBlocked = true
		}
		if tags.ruleTriggered {
			m.Milestones.ruleTriggered = true
		}
		if tags.wafTimeout {
			m.Milestones.wafTimeout = true
			m.SumWAFTimeouts.Add(1)
		}
		if tags.rateLimited {
			m.Milestones.rateLimited = true
		}
		if tags.wafError {
			m.Milestones.wafError = true
		}
	default:
		m.logger.Error("unexpected scope name", slog.String("scope", string(addrs.TimerKey)))
	}
}

// IncWafError should be called if go-libddwaf.(*Context).Run() returns an error to increments metrics linked to WAF errors
// It registers the metrics:
// - `waf.error`
// - `rasp.error`
func (m *ContextMetrics) IncWafError(addrs libddwaf.RunAddressData, in error) {
	if in == nil {
		return
	}

	if !errors.Is(in, waferrors.ErrTimeout) {
		logger := m.logger.With(telemetry.WithTags(m.baseTags))
		logger.Error("unexpected WAF error", slog.Any("error", telemetrylog.NewSafeError(in)))
	}

	switch addrs.TimerKey {
	case addresses.RASPScope:
		ruleType, ok := addresses.RASPRuleTypeFromAddressSet(addrs)
		if !ok {
			m.logger.Error("unexpected call to RASPRuleTypeFromAddressSet", slog.Any("error", telemetrylog.NewSafeError(in)))
		}
		m.raspError(in, ruleType)
	case addresses.WAFScope, "":
		m.wafError(in)
	default:
		m.logger.Error("unexpected scope name", slog.String("scope", string(addrs.TimerKey)))
	}
}

// defaultWafErrorCode is the default error code if the error does not implement [libddwaf.RunError]
// meaning if the error actual come for the bindings and not from the WAF itself
const defaultWafErrorCode = -127

func (m *ContextMetrics) wafError(in error) {
	m.SumWAFErrors.Add(1)
	errCode := defaultWafErrorCode
	if code := waferrors.ToWafErrorCode(in); code != 0 {
		errCode = code
	}

	handle, _ := m.wafErrorCount.LoadOrCompute(errCode, func() telemetry.MetricHandle {
		return telemetry.Count(telemetry.NamespaceAppSec, "waf.error", append([]string{
			"error_code:" + strconv.Itoa(errCode),
		}, m.baseTags...))
	})

	handle.Submit(1)
}

func (m *ContextMetrics) raspError(in error, ruleType addresses.RASPRuleType) {
	m.SumRASPErrors.Add(1)
	errCode := defaultWafErrorCode
	if code := waferrors.ToWafErrorCode(in); code != 0 {
		errCode = code
	}

	handle, _ := m.raspErrorCount.LoadOrCompute(raspMetricKey[int]{typ: ruleType, additionalTag: errCode}, func() telemetry.MetricHandle {
		return telemetry.Count(telemetry.NamespaceAppSec, "rasp.error", append([]string{
			"error_code:" + strconv.Itoa(errCode),
		}, m.baseRASPTags[ruleType]...))
	})

	handle.Submit(1)
}
