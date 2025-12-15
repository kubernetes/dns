// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package llmobs

import (
	"errors"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/llmobs/config"
	"github.com/DataDog/dd-trace-go/v2/internal/llmobs/transport"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

const (
	telemetryMetricInitTime          = "init_time"
	telemetryMetricEnabled           = "product_enabled"
	telemetryMetricRawSpanSize       = "span.raw_size"
	telemetryMetricSpanSize          = "span.size"
	telemetryMetricSpanStarted       = "span.start"
	telemetryMetricSpanFinished      = "span.finished"
	telemetryMetricDroppedSpanEvents = "dropped_span_events"
	telemetryMetricDroppedEvalEvents = "dropped_eval_events"
	telemetryMetricAnnotations       = "annotations"
	telemetryMetricEvalsSubmitted    = "evals_submitted"
	telemetryMetricUserFlushes       = "user_flush"
)

var telemetryErrorTypes = map[error]string{
	errInvalidMetricLabel: "invalid_metric_label",
	errFinishedSpan:       "invalid_finished_span",
	errInvalidSpanJoin:    "invalid_span",
	errInvalidTagJoin:     "invalid_tag_join",
}

// Pre-computed telemetry metric handles
var (
	spanStartedHandle = telemetry.Count(telemetry.NamespaceMLObs, telemetryMetricSpanStarted, nil)
	userFlushHandle   = telemetry.Count(telemetry.NamespaceMLObs, telemetryMetricUserFlushes, []string{"error:0"})
)

func trackLLMObsStart(startTime time.Time, err error, cfg config.Config) {
	if telemetry.Disabled() {
		return
	}
	telemetry.ProductStarted(telemetry.NamespaceMLObs)
	telemetry.RegisterAppConfigs(
		telemetry.Configuration{Name: "site", Value: cfg.TracerConfig.Site},
		telemetry.Configuration{Name: "ml_app", Value: cfg.MLApp},
		telemetry.Configuration{Name: "agentless", Value: cfg.ResolvedAgentlessEnabled},
	)

	tags := errTelemetryTags(err)
	tags = append(tags, []string{
		"agentless:" + boolTag(cfg.ResolvedAgentlessEnabled),
		"site:" + cfg.TracerConfig.Site,
		"ml_app:" + valOrNA(cfg.MLApp),
	}...)

	initTimeMs := float64(time.Since(startTime).Milliseconds())
	telemetry.Distribution(telemetry.NamespaceMLObs, telemetryMetricInitTime, tags).Submit(initTimeMs)
	telemetry.Count(telemetry.NamespaceMLObs, telemetryMetricEnabled, tags).Submit(1)
}

func trackSpanStarted() {
	if telemetry.Disabled() {
		return
	}
	spanStartedHandle.Submit(1)
}

func trackSpanFinished(span *Span) {
	if telemetry.Disabled() {
		return
	}
	isRootSpan := span.parent == nil
	hasSessionID := span.sessionID != ""
	integration := span.integration
	autoinstrumented := integration != ""
	spanKind := string(span.spanKind)
	modelProvider := span.llmCtx.modelProvider
	mlApp := span.mlApp
	hasError := span.error != nil

	tags := []string{
		"autoinstrumented:" + boolTag(autoinstrumented),
		"has_session_id:" + boolTag(hasSessionID),
		"is_root_span:" + boolTag(isRootSpan),
		"span_kind:" + valOrNA(spanKind),
		"integration:" + valOrNA(integration),
		"ml_app:" + valOrNA(mlApp),
		"error:" + boolTag(hasError),
	}
	if modelProvider != "" {
		tags = append(tags, "model_provider:"+modelProvider)
	}

	telemetry.Count(telemetry.NamespaceMLObs, telemetryMetricSpanFinished, tags).Submit(1)
}

func trackSpanEventRawSize(event *transport.LLMObsSpanEvent, rawSize int) {
	if telemetry.Disabled() {
		return
	}
	tags := spanEventTags(event)
	telemetry.Distribution(telemetry.NamespaceMLObs, telemetryMetricRawSpanSize, tags).Submit(float64(rawSize))
}

func trackSpanEventSize(event *transport.LLMObsSpanEvent, size int, truncated bool) {
	if telemetry.Disabled() {
		return
	}
	tags := spanEventTags(event)
	tags = append(tags, "truncated:"+boolTag(truncated))
	telemetry.Distribution(telemetry.NamespaceMLObs, telemetryMetricSpanSize, tags).Submit(float64(size))
}

func trackDroppedPayload(numEvents int, metricName string, errType string) {
	if telemetry.Disabled() {
		return
	}
	tags := []string{"error:1", "error_type:" + errType}
	telemetry.Count(telemetry.NamespaceMLObs, metricName, tags).Submit(float64(numEvents))
}

func trackSpanAnnotations(span *Span, err error) {
	if telemetry.Disabled() {
		return
	}
	tags := errTelemetryTags(err)
	spanKind := ""
	isRootSpan := "0"
	if span != nil {
		spanKind = valOrNA(string(span.spanKind))
		isRootSpan = boolTag(span.parent == nil)
	}
	tags = append(tags,
		"span_kind:"+spanKind,
		"is_root_span:"+isRootSpan,
	)
	telemetry.Count(telemetry.NamespaceMLObs, telemetryMetricAnnotations, tags).Submit(1)
}

func trackSubmitEvaluationMetric(metric *transport.LLMObsMetric, err error) {
	if telemetry.Disabled() {
		return
	}
	metricType := "other"
	hasTag := false
	if metric != nil {
		metricType = metric.MetricType
		hasTag = metric.JoinOn.Tag != nil
	}

	tags := errTelemetryTags(err)
	tags = append(tags,
		"metric_type:"+metricType,
		"custom_joining_key:"+boolTag(hasTag),
	)
	telemetry.Count(telemetry.NamespaceMLObs, telemetryMetricEvalsSubmitted, tags).Submit(1)
}

func trackUserFlush() {
	if telemetry.Disabled() {
		return
	}
	userFlushHandle.Submit(1)
}

func spanEventTags(event *transport.LLMObsSpanEvent) []string {
	spanKind := "N/A"
	if meta, ok := event.Meta["span.kind"]; ok {
		if kind, ok := meta.(string); ok {
			spanKind = kind
		}
	}

	integration := findTagValue(event.Tags, "integration:")
	mlApp := findTagValue(event.Tags, "ml_app:")
	autoInstrumented := integration != ""
	hasError := event.Status == "error"

	return []string{
		"span_kind:" + spanKind,
		"autoinstrumented:" + boolTag(autoInstrumented),
		"error:" + boolTag(hasError),
		"integration:" + valOrNA(integration),
		"ml_app:" + valOrNA(mlApp),
	}
}

func findTagValue(tags []string, prefix string) string {
	for _, tag := range tags {
		if len(tag) > len(prefix) && tag[:len(prefix)] == prefix {
			return tag[len(prefix):]
		}
	}
	return ""
}

func valOrNA(value string) string {
	if value == "" {
		return "n/a"
	}
	return value
}

func errTelemetryTags(err error) []string {
	tags := []string{"error:" + boolTag(err != nil)}
	if err != nil {
		for targetErr, errType := range telemetryErrorTypes {
			if errors.Is(err, targetErr) {
				tags = append(tags, "error_type:"+errType)
				break
			}
		}
	}
	return tags
}

func boolTag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
