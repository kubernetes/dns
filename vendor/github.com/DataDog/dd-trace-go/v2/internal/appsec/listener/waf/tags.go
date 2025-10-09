// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package waf

import (
	"slices"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/appsec/trace"
	"github.com/DataDog/dd-trace-go/v2/internal"
	emitter "github.com/DataDog/dd-trace-go/v2/internal/appsec/emitter/waf"
	"github.com/DataDog/dd-trace-go/v2/internal/samplernames"
	"github.com/DataDog/go-libddwaf/v4"
	"github.com/DataDog/go-libddwaf/v4/timer"
)

const (
	wafSpanTagPrefix     = "_dd.appsec."
	eventRulesVersionTag = wafSpanTagPrefix + "event_rules.version"
	wafVersionTag        = wafSpanTagPrefix + "waf.version"
	wafErrorTag          = wafSpanTagPrefix + "waf.error"
	wafTimeoutTag        = wafSpanTagPrefix + "waf.timeouts"
	raspRuleEvalTag      = wafSpanTagPrefix + "rasp.rule.eval"
	raspErrorTag         = wafSpanTagPrefix + "rasp.error"
	raspTimeoutTag       = wafSpanTagPrefix + "rasp.timeout"
	truncationTagPrefix  = wafSpanTagPrefix + "truncated."

	durationExtSuffix = ".duration_ext"

	blockedRequestTag = "appsec.blocked"
)

// AddRulesMonitoringTags adds the tags related to security rules monitoring
func AddRulesMonitoringTags(th trace.TagSetter) {
	th.SetTag(wafVersionTag, libddwaf.Version())
	th.SetTag(ext.ManualKeep, samplernames.AppSec)
}

// AddWAFMonitoringTags adds the tags related to the monitoring of the WAF
func AddWAFMonitoringTags(th trace.TagSetter, metrics *emitter.ContextMetrics, rulesVersion string, truncations map[libddwaf.TruncationReason][]int, timerStats map[timer.Key]time.Duration) {
	// Rules version is set for every request to help the backend associate Feature duration metrics with rule version
	th.SetTag(eventRulesVersionTag, rulesVersion)

	if raspCallsCount := metrics.SumRASPCalls.Load(); raspCallsCount > 0 {
		th.SetTag(raspRuleEvalTag, raspCallsCount)
	}

	if raspErrorsCount := metrics.SumRASPErrors.Load(); raspErrorsCount > 0 {
		th.SetTag(raspErrorTag, raspErrorsCount)
	}

	if wafErrorsCount := metrics.SumWAFErrors.Load(); wafErrorsCount > 0 {
		th.SetTag(wafErrorTag, wafErrorsCount)
	}

	// Add metrics like `waf.duration` and `rasp.duration_ext`
	for scope, value := range timerStats {
		th.SetTag(wafSpanTagPrefix+string(scope)+durationExtSuffix, float64(value.Nanoseconds())/float64(time.Microsecond.Nanoseconds()))
		for component, atomicValue := range metrics.SumDurations[scope] {
			if value := atomicValue.Load(); value > 0 {
				th.SetTag(wafSpanTagPrefix+string(scope)+"."+string(component), float64(value)/float64(time.Microsecond.Nanoseconds()))
			}
		}
	}

	if value := metrics.SumWAFTimeouts.Load(); value > 0 {
		th.SetTag(wafTimeoutTag, value)
	}

	var sumRASPTimeouts uint32
	for ruleType := range metrics.SumRASPTimeouts {
		sumRASPTimeouts += metrics.SumRASPTimeouts[ruleType].Load()
	}

	if sumRASPTimeouts > 0 {
		th.SetTag(raspTimeoutTag, sumRASPTimeouts)
	}

	for reason, count := range truncations {
		if len(count) > 0 {
			th.SetTag(truncationTagPrefix+reason.String(), slices.Max(count))
		}
	}
}

// SetEventSpanTags sets the security event span tags related to an appsec event
func SetEventSpanTags(span trace.TagSetter) {
	span.SetTag("_dd.origin", "appsec")
	// Set the appsec.event tag needed by the appsec backend
	span.SetTag("appsec.event", true)
	span.SetTag("_dd.p.ts", internal.TraceSourceTagValue{Value: internal.ASMTraceSource})
}
