// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package llmobs

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/internal/llmobs/config"
	"github.com/DataDog/dd-trace-go/v2/internal/llmobs/transport"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/version"
)

var (
	mu           sync.Mutex
	activeLLMObs *LLMObs
)

var (
	errLLMObsNotEnabled        = errors.New("LLMObs is not enabled. Ensure the tracer has been started with the option tracer.WithLLMObsEnabled(true) or set DD_LLMOBS_ENABLED=true")
	errAgentlessRequiresAPIKey = errors.New("LLMOBs agentless mode requires a valid API key - set the DD_API_KEY env variable to configure one")
	errMLAppRequired           = errors.New("ML App is required for sending LLM Observability data")
	errAgentModeNotSupported   = errors.New("DD_LLMOBS_AGENTLESS_ENABLED has been configured to false but the agent is not available or does not support LLMObs")
	errInvalidMetricLabel      = errors.New("label is required for evaluation metrics")
	errFinishedSpan            = errors.New("span is already finished")
	errEvalJoinBothPresent     = errors.New("provide either span/trace IDs or tag key/value, not both")
	errEvalJoinNonePresent     = errors.New("must provide either span/trace IDs or tag key/value for joining")
	errInvalidSpanJoin         = errors.New("both span and trace IDs are required for span-based joining")
	errInvalidTagJoin          = errors.New("both tag key and value are required for tag-based joining")
)

const (
	baggageKeyExperimentID = "_ml_obs.experiment_id"
)

const (
	defaultParentID = "undefined"
)

// SpanKind represents the type of an LLMObs span.
type SpanKind string

const (
	// SpanKindExperiment represents an experiment span for testing and evaluation.
	SpanKindExperiment SpanKind = "experiment"
	// SpanKindWorkflow represents a workflow span that orchestrates multiple operations.
	SpanKindWorkflow SpanKind = "workflow"
	// SpanKindLLM represents a span for Large Language Model operations.
	SpanKindLLM SpanKind = "llm"
	// SpanKindEmbedding represents a span for embedding generation operations.
	SpanKindEmbedding SpanKind = "embedding"
	// SpanKindAgent represents a span for AI agent operations.
	SpanKindAgent SpanKind = "agent"
	// SpanKindRetrieval represents a span for document retrieval operations.
	SpanKindRetrieval SpanKind = "retrieval"
	// SpanKindTask represents a span for general task operations.
	SpanKindTask SpanKind = "task"
	// SpanKindTool represents a span for tool usage operations.
	SpanKindTool SpanKind = "tool"
)

const (
	defaultFlushInterval = 2 * time.Second
)

const (
	sizeLimitEVPEvent        = 5_000_000 // 5MB
	collectionErrorDroppedIO = "dropped_io"
	droppedValueText         = "[This value has been dropped because this span's size exceeds the 1MB size limit.]"
)

// See: https://docs.datadoghq.com/getting_started/site/#access-the-datadog-site
var ddSitesNeedingAppSubdomain = []string{"datadoghq.com", "datadoghq.eu", "ddog-gov.com"}

type llmobsContext struct {
	// apply to all spans
	metadata map[string]any
	metrics  map[string]float64
	tags     map[string]string

	// agent specific
	agentManifest string

	// llm specific
	modelName       string
	modelProvider   string
	prompt          *Prompt
	toolDefinitions []ToolDefinition

	// input
	inputDocuments []EmbeddedDocument
	inputMessages  []LLMMessage
	inputText      string

	// output
	outputDocuments []RetrievedDocument
	outputMessages  []LLMMessage
	outputText      string

	// experiment specific
	experimentInput          any
	experimentExpectedOutput any
	experimentOutput         any
}

// LLMObs represents the main LLMObs instance that handles span collection and transport.
type LLMObs struct {
	// Config contains the LLMObs configuration.
	Config *config.Config
	// Transport handles sending data to the Datadog backend.
	Transport *transport.Transport
	// Tracer is the underlying APM tracer.
	Tracer Tracer

	// channels used by producers
	spanEventsCh  chan *transport.LLMObsSpanEvent
	evalMetricsCh chan *transport.LLMObsMetric

	// runtime buffers, payloads are accumulated here and flushed periodically
	bufSpanEvents  []*transport.LLMObsSpanEvent
	bufEvalMetrics []*transport.LLMObsMetric

	// lifecycle
	mu            sync.Mutex
	running       bool
	wg            sync.WaitGroup
	stopCh        chan struct{} // signal stop
	flushNowCh    chan struct{}
	flushInterval time.Duration
}

func newLLMObs(cfg *config.Config, tracer Tracer) (*LLMObs, error) {
	agentSupportsLLMObs := cfg.AgentFeatures.EVPProxyV2
	if !agentSupportsLLMObs {
		log.Debug("llmobs: agent not available or does not support llmobs")
	}
	if cfg.AgentlessEnabled != nil {
		if !*cfg.AgentlessEnabled && !agentSupportsLLMObs {
			return nil, errAgentModeNotSupported
		}
		cfg.ResolvedAgentlessEnabled = *cfg.AgentlessEnabled
	} else {
		// if agentlessEnabled is not set and evp_proxy is supported in the agent, default to use the agent
		cfg.ResolvedAgentlessEnabled = !agentSupportsLLMObs
		if cfg.ResolvedAgentlessEnabled {
			log.Debug("llmobs: DD_LLMOBS_AGENTLESS_ENABLED not set, defaulting to agentless mode")
		} else {
			log.Debug("llmobs: DD_LLMOBS_AGENTLESS_ENABLED not set, defaulting to agent mode")
		}
	}

	if cfg.ResolvedAgentlessEnabled && !isAPIKeyValid(cfg.TracerConfig.APIKey) {
		return nil, errAgentlessRequiresAPIKey
	}
	if cfg.MLApp == "" {
		return nil, errMLAppRequired
	}
	if cfg.TracerConfig.HTTPClient == nil {
		cfg.TracerConfig.HTTPClient = cfg.DefaultHTTPClient()
	}
	return &LLMObs{
		Config:        cfg,
		Transport:     transport.New(cfg),
		Tracer:        tracer,
		spanEventsCh:  make(chan *transport.LLMObsSpanEvent),
		evalMetricsCh: make(chan *transport.LLMObsMetric),
		stopCh:        make(chan struct{}),
		flushNowCh:    make(chan struct{}, 1),
		flushInterval: defaultFlushInterval,
	}, nil
}

// Start starts the global LLMObs instance with the given configuration and tracer.
// Returns an error if LLMObs is already running or if configuration is invalid.
func Start(cfg config.Config, tracer Tracer) (err error) {
	startTime := time.Now()
	defer func() {
		trackLLMObsStart(startTime, err, cfg)
	}()
	mu.Lock()
	defer mu.Unlock()

	if activeLLMObs != nil {
		activeLLMObs.Stop()
	}
	if !cfg.Enabled {
		return nil
	}
	l, err := newLLMObs(&cfg, tracer)
	if err != nil {
		return err
	}
	activeLLMObs = l
	activeLLMObs.Run()
	return nil
}

// Stop stops the active LLMObs instance and cleans up resources.
func Stop() {
	mu.Lock()
	defer mu.Unlock()

	if activeLLMObs != nil {
		activeLLMObs.Stop()
		activeLLMObs = nil
	}
}

// ActiveLLMObs returns the current active LLMObs instance, or an error if LLMObs is not enabled or started.
func ActiveLLMObs() (*LLMObs, error) {
	if activeLLMObs == nil || !activeLLMObs.Config.Enabled {
		return nil, errLLMObsNotEnabled
	}
	return activeLLMObs, nil
}

// Flush forces a flush of all buffered LLMObs data to the transport.
func Flush() {
	if activeLLMObs != nil {
		activeLLMObs.Flush()
		trackUserFlush()
	}
}

// Run starts the worker loop that processes span events and metrics.
func (l *LLMObs) Run() {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return
	}
	l.running = true
	l.mu.Unlock()

	l.wg.Add(1)
	go func() {
		// this goroutine should be the only one writing to the internal buffers
		defer l.wg.Done()

		ticker := time.NewTicker(l.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case ev := <-l.spanEventsCh:
				l.bufSpanEvents = append(l.bufSpanEvents, ev)

			case evalMetric := <-l.evalMetricsCh:
				l.bufEvalMetrics = append(l.bufEvalMetrics, evalMetric)

			case <-ticker.C:
				params := l.clearBuffersNonLocked()
				l.wg.Add(1)
				go func() {
					defer l.wg.Done()
					l.batchSend(params)
				}()

			case <-l.flushNowCh:
				log.Debug("llmobs: on-demand flush signal")
				params := l.clearBuffersNonLocked()
				l.wg.Add(1)
				go func() {
					defer l.wg.Done()
					l.batchSend(params)
				}()

			case <-l.stopCh:
				log.Debug("llmobs: stop signal")
				l.drainChannels()
				params := l.clearBuffersNonLocked()
				l.batchSend(params)
				return
			}
		}
	}()
}

// clearBuffersNonLocked clears the internal buffers and returns the corresponding batchSendParams to send to the backend.
// It is meant to be called only from the main Run worker goroutine.
func (l *LLMObs) clearBuffersNonLocked() batchSendParams {
	params := batchSendParams{
		spanEvents:  l.bufSpanEvents,
		evalMetrics: l.bufEvalMetrics,
	}
	l.bufSpanEvents = nil
	l.bufEvalMetrics = nil
	return params
}

// Flush forces an immediate flush of anything currently buffered.
// It does not wait for new items to arrive.
func (l *LLMObs) Flush() {
	// non-blocking edge trigger so multiple calls coalesce
	select {
	case l.flushNowCh <- struct{}{}:
	default:
	}
}

// Stop requests shutdown, drains what’s already in the channels, flushes, and waits.
func (l *LLMObs) Stop() {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return
	}
	l.running = false
	l.mu.Unlock()

	// Stop the sender/flush loop
	select {
	case <-l.stopCh:
	default:
		close(l.stopCh)
	}

	// Wait for the main worker to exit (it will do a final flush)
	l.wg.Wait()
}

// drainChannels pulls everything currently buffered in the channels into our in-memory buffers.
func (l *LLMObs) drainChannels() {
	for {
		progress := false
		select {
		case ev := <-l.spanEventsCh:
			l.mu.Lock()
			l.bufSpanEvents = append(l.bufSpanEvents, ev)
			l.mu.Unlock()
			progress = true
		default:
		}

		select {
		case evalMetric := <-l.evalMetricsCh:
			l.mu.Lock()
			l.bufEvalMetrics = append(l.bufEvalMetrics, evalMetric)
			l.mu.Unlock()
			progress = true
		default:
		}

		if !progress {
			return
		}
	}
}

type batchSendParams struct {
	spanEvents  []*transport.LLMObsSpanEvent
	evalMetrics []*transport.LLMObsMetric
}

// batchSend sends the buffered payloads to the backend.
func (l *LLMObs) batchSend(params batchSendParams) {
	if len(params.spanEvents) == 0 && len(params.evalMetrics) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	if len(params.spanEvents) > 0 {
		wg.Add(1)
		events := params.spanEvents
		go func() {
			defer wg.Done()
			log.Debug("llmobs: sending %d LLMObs Span Events", len(events))
			if log.DebugEnabled() {
				for _, ev := range events {
					if b, err := json.Marshal(ev); err == nil {
						log.Debug("llmobs: LLMObs Span Event: %s", b)
					}
				}
			}
			if err := l.Transport.PushSpanEvents(ctx, events); err != nil {
				log.Error("llmobs: failed to push span events: %v", err.Error())
				trackDroppedPayload(len(events), telemetryMetricDroppedSpanEvents, "transport_error")
			} else {
				log.Debug("llmobs: push span events success")
			}
		}()
	}
	if len(params.evalMetrics) > 0 {
		wg.Add(1)
		metrics := params.evalMetrics
		go func() {
			defer wg.Done()
			log.Debug("llmobs: sending %d LLMObs Span Eval Metrics", len(metrics))
			if log.DebugEnabled() {
				for _, eval := range metrics {
					if b, err := json.Marshal(eval); err == nil {
						log.Debug("llmobs: LLMObs Span Eval Metric: %s", b)
					}
				}
			}
			if err := l.Transport.PushEvalMetrics(ctx, metrics); err != nil {
				log.Error("llmobs: failed to push eval metrics: %v", err.Error())
				trackDroppedPayload(len(metrics), telemetryMetricDroppedEvalEvents, "transport_error")
			} else {
				log.Debug("llmobs: push eval metrics success")
			}
		}()
	}
	wg.Wait()
}

// submitLLMObsSpan generates and submits an LLMObs span event to the LLMObs intake.
func (l *LLMObs) submitLLMObsSpan(span *Span) {
	event := l.llmobsSpanEvent(span)
	l.spanEventsCh <- event
}

func (l *LLMObs) llmobsSpanEvent(span *Span) *transport.LLMObsSpanEvent {
	meta := make(map[string]any)

	spanKind := span.spanKind
	meta["span.kind"] = string(spanKind)

	if (spanKind == SpanKindLLM || spanKind == SpanKindEmbedding) && span.llmCtx.modelName != "" || span.llmCtx.modelProvider != "" {
		modelName := span.llmCtx.modelName
		if modelName == "" {
			modelName = "custom"
		}
		modelProvider := strings.ToLower(span.llmCtx.modelProvider)
		if modelProvider == "" {
			modelProvider = "custom"
		}
		meta["model_name"] = modelName
		meta["model_provider"] = modelProvider
	}

	metadata := span.llmCtx.metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if spanKind == SpanKindAgent && span.llmCtx.agentManifest != "" {
		metadata["agent_manifest"] = span.llmCtx.agentManifest
	}
	if len(metadata) > 0 {
		meta["metadata"] = metadata
	}

	input := make(map[string]any)
	output := make(map[string]any)

	if spanKind == SpanKindLLM && len(span.llmCtx.inputMessages) > 0 {
		input["messages"] = span.llmCtx.inputMessages
	} else if txt := span.llmCtx.inputText; len(txt) > 0 {
		input["value"] = txt
	}

	if spanKind == SpanKindLLM && len(span.llmCtx.outputMessages) > 0 {
		output["messages"] = span.llmCtx.outputMessages
	} else if txt := span.llmCtx.outputText; len(txt) > 0 {
		output["value"] = txt
	}

	if spanKind == SpanKindExperiment {
		if expectedOut := span.llmCtx.experimentExpectedOutput; expectedOut != nil {
			meta["expected_output"] = expectedOut
		}
		if expInput := span.llmCtx.experimentInput; expInput != nil {
			meta["input"] = expInput
		}
		if out := span.llmCtx.experimentOutput; out != nil {
			meta["output"] = out
		}
	}

	if spanKind == SpanKindEmbedding {
		if inputDocs := span.llmCtx.inputDocuments; len(inputDocs) > 0 {
			input["documents"] = inputDocs
		}
	}
	if spanKind == SpanKindRetrieval {
		if outputDocs := span.llmCtx.outputDocuments; len(outputDocs) > 0 {
			output["documents"] = outputDocs
		}
	}
	if inputPrompt := span.llmCtx.prompt; inputPrompt != nil {
		if spanKind != SpanKindLLM {
			log.Warn("llmobs: dropping prompt on non-LLM span kind, annotating prompts is only supported for LLM span kinds")
		} else {
			input["prompt"] = inputPrompt
		}
	} else if spanKind == SpanKindLLM {
		if span.parent != nil && span.parent.llmCtx.prompt != nil {
			input["prompt"] = span.parent.llmCtx.prompt
		}
	}

	if toolDefinitions := span.llmCtx.toolDefinitions; len(toolDefinitions) > 0 {
		meta["tool_definitions"] = toolDefinitions
	}

	spanStatus := "ok"
	var errMsg *transport.ErrorMessage
	if span.error != nil {
		spanStatus = "error"
		errMsg = transport.NewErrorMessage(span.error)
		meta["error.message"] = errMsg.Message
		meta["error.stack"] = errMsg.Stack
		meta["error.type"] = errMsg.Type
	}

	if len(input) > 0 {
		meta["input"] = input
	}
	if len(output) > 0 {
		meta["output"] = output
	}

	spanID := span.apm.SpanID()
	parentID := defaultParentID
	if span.parent != nil {
		parentID = span.parent.apm.SpanID()
	}
	if span.llmTraceID == "" {
		log.Warn("llmobs: span has no trace ID")
		span.llmTraceID = newLLMObsTraceID()
	}

	tags := make(map[string]string)
	for k, v := range l.Config.TracerConfig.DDTags {
		tags[k] = fmt.Sprintf("%v", v)
	}
	tags["version"] = l.Config.TracerConfig.Version
	tags["env"] = l.Config.TracerConfig.Env
	tags["service"] = l.Config.TracerConfig.Service
	tags["source"] = "integration"
	tags["ml_app"] = span.mlApp
	tags["ddtrace.version"] = version.Tag
	tags["language"] = "go"

	sessionID := span.propagatedSessionID()
	if sessionID != "" {
		tags["session_id"] = sessionID
	}

	errTag := "0"
	if span.error != nil {
		errTag = "1"
	}
	tags["error"] = errTag

	if errMsg != nil {
		tags["error_type"] = errMsg.Type
	}
	if span.integration != "" {
		tags["integration"] = span.integration
	}

	for k, v := range span.llmCtx.tags {
		tags[k] = v
	}
	tagsSlice := make([]string, 0, len(tags))
	for k, v := range tags {
		tagsSlice = append(tagsSlice, fmt.Sprintf("%s:%s", k, v))
	}

	ev := &transport.LLMObsSpanEvent{
		SpanID:           spanID,
		TraceID:          span.llmTraceID,
		ParentID:         parentID,
		SessionID:        sessionID,
		Tags:             tagsSlice,
		Name:             span.name,
		StartNS:          span.startTime.UnixNano(),
		Duration:         span.finishTime.Sub(span.startTime).Nanoseconds(),
		Status:           spanStatus,
		StatusMessage:    "",
		Meta:             meta,
		Metrics:          span.llmCtx.metrics,
		CollectionErrors: nil,
		SpanLinks:        span.spanLinks,
		Scope:            span.scope,
	}
	if b, err := json.Marshal(ev); err == nil {
		rawSize := len(b)
		trackSpanEventRawSize(ev, rawSize)

		truncated := false
		if rawSize > sizeLimitEVPEvent {
			log.Warn(
				"llmobs: dropping llmobs span event input/output because its size (%s) exceeds the event size limit (5MB)",
				readableBytes(rawSize),
			)
			truncated = dropSpanEventIO(ev)
			if !truncated {
				log.Debug("llmobs: attempted to drop span event IO but it was not present")
			}
		}
		actualSize := rawSize
		if truncated {
			if b, err := json.Marshal(ev); err == nil {
				actualSize = len(b)
			}
		}
		trackSpanEventSize(ev, actualSize, truncated)
	}
	return ev
}

func dropSpanEventIO(ev *transport.LLMObsSpanEvent) bool {
	if ev == nil {
		return false
	}
	droppedIO := false
	if _, ok := ev.Meta["input"]; ok {
		ev.Meta["input"] = map[string]any{"value": droppedValueText}
		droppedIO = true
	}
	if _, ok := ev.Meta["output"]; ok {
		ev.Meta["output"] = map[string]any{"value": droppedValueText}
		droppedIO = true
	}
	if droppedIO {
		ev.CollectionErrors = []string{collectionErrorDroppedIO}
	} else {
		log.Debug("llmobs: attempted to drop span event IO but it was not present")
	}
	return droppedIO
}

// StartSpan starts a new LLMObs span with the given kind, name, and configuration.
// Returns the created span and a context containing the span.
func (l *LLMObs) StartSpan(ctx context.Context, kind SpanKind, name string, cfg StartSpanConfig) (*Span, context.Context) {
	defer trackSpanStarted()

	spanName := name
	if spanName == "" {
		spanName = string(kind)
	}

	if cfg.StartTime.IsZero() {
		cfg.StartTime = time.Now()
	}

	startCfg := StartAPMSpanConfig{
		SpanType:  ext.SpanTypeLLM,
		StartTime: cfg.StartTime,
	}
	apmSpan, ctx := l.Tracer.StartSpan(ctx, spanName, startCfg)
	span := &Span{
		name:      spanName,
		apm:       apmSpan,
		startTime: cfg.StartTime,
	}
	if !l.Config.Enabled {
		log.Warn("llmobs: LLMObs span was started without enabling LLMObs")
		return span, ctx
	}

	if parent, ok := ActiveLLMSpanFromContext(ctx); ok {
		log.Debug("llmobs: found active llm span in context: (trace_id: %q, span_id: %q, ml_app: %q)",
			parent.TraceID(), parent.SpanID(), parent.MLApp())
		span.parent = parent
		span.llmTraceID = parent.llmTraceID
	} else if propagated, ok := PropagatedLLMSpanFromContext(ctx); ok {
		log.Debug("llmobs: found propagated llm span in context: (trace_id: %q, span_id: %q, ml_app: %q)",
			propagated.TraceID, propagated.SpanID, propagated.MLApp)
		span.propagated = propagated
		span.llmTraceID = propagated.TraceID
	} else {
		span.llmTraceID = newLLMObsTraceID()
	}

	span.mlApp = cfg.MLApp
	span.spanKind = kind
	span.sessionID = cfg.SessionID

	span.llmCtx = llmobsContext{
		modelName:     cfg.ModelName,
		modelProvider: cfg.ModelProvider,
	}

	if span.sessionID == "" {
		span.sessionID = span.propagatedSessionID()
	}
	if span.mlApp == "" {
		span.mlApp = span.propagatedMLApp()
		if span.mlApp == "" {
			// We should ensure there's always an ML App to fall back to during startup, so in theory this should never happen.
			log.Warn("llmobs: ML App is required for sending LLM Observability data.")
		}
	}
	log.Debug("llmobs: starting LLMObs span: %s, span_kind: %s, ml_app: %s", spanName, kind, span.mlApp)
	return span, contextWithActiveLLMSpan(ctx, span)
}

// StartExperimentSpan starts a new experiment span with the given name, experiment ID, and configuration.
// Returns the created span and a context containing the span.
func (l *LLMObs) StartExperimentSpan(ctx context.Context, name string, experimentID string, cfg StartSpanConfig) (*Span, context.Context) {
	span, ctx := l.StartSpan(ctx, SpanKindExperiment, name, cfg)

	if experimentID != "" {
		span.apm.SetBaggageItem(baggageKeyExperimentID, experimentID)
		span.scope = "experiments"
	}
	return span, ctx
}

// SubmitEvaluation submits an evaluation metric for a span.
// The span can be identified either by span/trace IDs or by tag key-value pairs.
func (l *LLMObs) SubmitEvaluation(cfg EvaluationConfig) (err error) {
	var metric *transport.LLMObsMetric
	defer func() {
		trackSubmitEvaluationMetric(metric, err)
	}()

	if cfg.Label == "" {
		return errInvalidMetricLabel
	}
	var (
		hasTagJoin  bool
		hasSpanJoin bool
	)
	if cfg.SpanID != "" || cfg.TraceID != "" {
		if !(cfg.SpanID != "" && cfg.TraceID != "") {
			return errInvalidSpanJoin
		}
		hasSpanJoin = true
	}
	if cfg.TagKey != "" || cfg.TagValue != "" {
		if !(cfg.TagKey != "" && cfg.TagValue != "") {
			return errInvalidTagJoin
		}
		hasTagJoin = true
	}
	if hasSpanJoin && hasTagJoin {
		return errEvalJoinBothPresent
	}
	if !hasSpanJoin && !hasTagJoin {
		return errEvalJoinNonePresent
	}

	numValues := 0
	if cfg.CategoricalValue != nil {
		numValues++
	}
	if cfg.ScoreValue != nil {
		numValues++
	}
	if cfg.BooleanValue != nil {
		numValues++
	}
	if numValues != 1 {
		return errors.New("exactly one metric value (categorical, score, or boolean) must be provided")
	}

	mlApp := cfg.MLApp
	if mlApp == "" {
		mlApp = l.Config.MLApp
	}
	timestampMS := cfg.TimestampMS
	if timestampMS == 0 {
		timestampMS = time.Now().UnixMilli()
	}

	// Build the appropriate join condition
	var joinOn transport.EvaluationJoinOn
	if hasSpanJoin {
		joinOn.Span = &transport.EvaluationSpanJoin{
			SpanID:  cfg.SpanID,
			TraceID: cfg.TraceID,
		}
	} else {
		joinOn.Tag = &transport.EvaluationTagJoin{
			Key:   cfg.TagKey,
			Value: cfg.TagValue,
		}
	}

	tags := make([]string, 0, len(cfg.Tags)+1)
	for _, tag := range cfg.Tags {
		if !strings.HasPrefix(tag, "ddtrace.version:") {
			tags = append(tags, tag)
		}
	}
	tags = append(tags, fmt.Sprintf("ddtrace.version:%s", version.Tag))

	metric = &transport.LLMObsMetric{
		JoinOn:      joinOn,
		Label:       cfg.Label,
		MLApp:       mlApp,
		TimestampMS: timestampMS,
		Tags:        tags,
	}

	if cfg.CategoricalValue != nil {
		metric.CategoricalValue = cfg.CategoricalValue
		metric.MetricType = "categorical"
	} else if cfg.ScoreValue != nil {
		metric.ScoreValue = cfg.ScoreValue
		metric.MetricType = "score"
	} else if cfg.BooleanValue != nil {
		metric.BooleanValue = cfg.BooleanValue
		metric.MetricType = "boolean"
	} else {
		return errors.New("a metric value (categorical, score, or boolean) is required for evaluation metrics")
	}

	l.evalMetricsCh <- metric
	return nil
}

// PublicResourceBaseURL returns the base URL to access a resource (experiments, projects, etc.)
func PublicResourceBaseURL() string {
	site := "datadoghq.com"
	if activeLLMObs != nil && activeLLMObs.Config.TracerConfig.Site != "" {
		site = activeLLMObs.Config.TracerConfig.Site
	}

	baseURL := "https://"
	if slices.Contains(ddSitesNeedingAppSubdomain, site) {
		baseURL += "app."
	}
	baseURL += site
	return baseURL
}

func newLLMObsTraceID() string {
	var b [16]byte

	// High 32 bits: Unix seconds
	secs := uint32(time.Now().Unix())
	binary.BigEndian.PutUint32(b[0:4], secs)

	// Middle 32 bits: zero
	// (already zeroed by array initialization)

	// Low 64 bits: random
	if _, err := rand.Read(b[8:16]); err != nil {
		panic(err)
	}

	// Turn into a big.Int
	x := new(big.Int).SetBytes(b[:])

	// 32-byte hex string
	return fmt.Sprintf("%032x", x)
}

// isAPIKeyValid reports whether the given string is a structurally valid API key
func isAPIKeyValid(key string) bool {
	if len(key) != 32 {
		return false
	}
	for _, c := range key {
		if c > unicode.MaxASCII || (!unicode.IsLower(c) && !unicode.IsNumber(c)) {
			return false
		}
	}
	return true
}

func readableBytes(s int) string {
	const base = 1000
	sizes := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

	if s < 10 {
		return fmt.Sprintf("%dB", s)
	}
	e := math.Floor(logn(float64(s), base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	f := "%.0f%s"
	if val < 10 {
		f = "%.1f%s"
	}
	return fmt.Sprintf(f, val, suffix)
}

func logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}
