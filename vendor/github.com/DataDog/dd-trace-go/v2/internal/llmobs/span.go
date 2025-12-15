// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package llmobs

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

const (
	// TagKeySessionID is the tag key used to set the session ID for LLMObs spans.
	TagKeySessionID = "session_id"
)

// StartSpanConfig contains configuration options for starting an LLMObs span.
type StartSpanConfig struct {
	// SessionID sets the session ID for the span.
	SessionID string
	// ModelName sets the model name for LLM and embedding spans.
	ModelName string
	// ModelProvider sets the model provider for LLM and embedding spans.
	ModelProvider string
	// MLApp sets the ML application name for the span.
	MLApp string
	// StartTime sets a custom start time for the span. If zero, uses current time.
	StartTime time.Time
}

// FinishSpanConfig contains configuration options for finishing an LLMObs span.
type FinishSpanConfig struct {
	// FinishTime sets a custom finish time for the span. If zero, uses current time.
	FinishTime time.Time
	// Error sets an error on the span when finishing.
	Error error
}

// EvaluationConfig contains configuration for submitting evaluation metrics.
type EvaluationConfig struct {
	// Method 1: Direct span/trace ID join
	// SpanID is the span ID to evaluate.
	SpanID string
	// TraceID is the trace ID to evaluate.
	TraceID string

	// Method 2: Tag-based join
	// TagKey is the tag key to search for spans.
	TagKey string
	// TagValue is the tag value to match for spans.
	TagValue string

	// Required fields
	// Label is the name of the evaluation metric.
	Label string

	// Value fields (exactly one must be provided)
	// CategoricalValue is the categorical value of the evaluation metric.
	CategoricalValue *string
	// ScoreValue is the score value of the evaluation metric.
	ScoreValue *float64
	// BooleanValue is the boolean value of the evaluation metric.
	BooleanValue *bool

	// Optional fields
	// Tags are optional string key-value pairs to tag the evaluation metric.
	Tags []string
	// MLApp is the ML application name. If empty, uses the global config.
	MLApp string
	// TimestampMS is the timestamp in milliseconds. If zero, uses current time.
	TimestampMS int64
}

// Prompt represents a prompt template used with LLM spans.
type Prompt struct {
	// Template is the prompt template string.
	Template string `json:"template,omitempty"`
	// ID is the unique identifier for the prompt.
	ID string `json:"id,omitempty"`
	// Version is the version of the prompt.
	Version string `json:"version,omitempty"`
	// Variables contains the variables used in the prompt template.
	Variables map[string]string `json:"variables,omitempty"`
	// RAGContextVariables specifies which variables contain RAG context.
	RAGContextVariables []string `json:"rag_context_variables,omitempty"`
	// RAGQueryVariables specifies which variables contain RAG queries.
	RAGQueryVariables []string `json:"rag_query_variables,omitempty"`
}

// ToolDefinition represents a tool definition for LLM spans.
type ToolDefinition struct {
	// Name is the name of the tool.
	Name string `json:"name"`
	// Description is the description of what the tool does.
	Description string `json:"description,omitempty"`
	// Schema is the JSON schema defining the tool's parameters.
	Schema json.RawMessage `json:"schema,omitempty"`
}

// ToolCall represents a call to a tool within an LLM message.
type ToolCall struct {
	// Name is the name of the tool being called.
	Name string `json:"name"`
	// Arguments are the JSON-encoded arguments passed to the tool.
	Arguments json.RawMessage `json:"arguments"`
	// ToolID is the unique identifier for this tool call.
	ToolID string `json:"tool_id,omitempty"`
	// Type is the type of the tool call.
	Type string `json:"type,omitempty"`
}

// ToolResult represents the result of a tool call within an LLM message.
type ToolResult struct {
	// Result is the result returned by the tool.
	Result any `json:"result"`
	// Name is the name of the tool that was called.
	Name string `json:"name,omitempty"`
	// ToolID is the unique identifier for the tool call this result corresponds to.
	ToolID string `json:"tool_id,omitempty"`
	// Type is the type of the tool result.
	Type string `json:"type,omitempty"`
}

// LLMMessage represents a message in an LLM conversation.
type LLMMessage struct {
	// Role is the role of the message sender (e.g., "user", "assistant", "system").
	Role string `json:"role"`
	// Content is the text content of the message.
	Content string `json:"content"`
	// ToolCalls are the tool calls made in this message.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolResults are the results of tool calls in this message.
	ToolResults []ToolResult `json:"tool_results,omitempty"`
}

// EmbeddedDocument represents a document used for embedding operations.
type EmbeddedDocument struct {
	// Text is the text content of the document.
	Text string `json:"text"`
	// Name is the name or title of the document.
	Name string `json:"name,omitempty"`
	// Score is the relevance score of the document (typically 0.0-1.0).
	Score float64 `json:"score,omitempty"`
	// ID is the unique identifier of the document.
	ID string `json:"id,omitempty"`
}

// RetrievedDocument represents a document for retrieval operations.
type RetrievedDocument struct {
	// Text is the text content of the document.
	Text string `json:"text"`
	// Name is the name or title of the document.
	Name string `json:"name,omitempty"`
	// Score is the relevance score of the document (typically 0.0-1.0).
	Score float64 `json:"score,omitempty"`
	// ID is the unique identifier of the document.
	ID string `json:"id,omitempty"`
}

// SpanAnnotations contains data to annotate an LLMObs span with.
type SpanAnnotations struct {
	// InputText is the text input for the span.
	InputText string
	// InputMessages are the input messages for LLM spans.
	InputMessages []LLMMessage
	// InputEmbeddedDocs are the input documents for embedding spans.
	InputEmbeddedDocs []EmbeddedDocument

	// OutputText is the text output for the span.
	OutputText string
	// OutputMessages are the output messages for LLM spans.
	OutputMessages []LLMMessage
	// OutputRetrievedDocs are the output documents for retrieval spans.
	OutputRetrievedDocs []RetrievedDocument

	// ExperimentInput is the input data for experiment spans.
	ExperimentInput any
	// ExperimentOutput is the output data for experiment spans.
	ExperimentOutput any
	// ExperimentExpectedOutput is the expected output for experiment spans.
	ExperimentExpectedOutput any

	// Prompt is the prompt information for LLM spans.
	Prompt *Prompt
	// ToolDefinitions are the tool definitions for LLM spans.
	ToolDefinitions []ToolDefinition

	// AgentManifest is the agent manifest for agent spans.
	AgentManifest string

	// Metadata contains arbitrary metadata key-value pairs.
	Metadata map[string]any
	// Metrics contains numeric metrics key-value pairs.
	Metrics map[string]float64
	// Tags contains string tags key-value pairs.
	Tags map[string]string
}

// Span represents an LLMObs span with its associated metadata and context.
type Span struct {
	mu sync.RWMutex

	apm        APMSpan
	parent     *Span
	propagated *PropagatedLLMSpan

	llmCtx llmobsContext

	llmTraceID string
	name       string
	mlApp      string
	spanKind   SpanKind
	sessionID  string

	integration string
	scope       string
	error       error
	finished    bool

	startTime  time.Time
	finishTime time.Time

	spanLinks []SpanLink
}

func (s *Span) Name() string {
	return s.name
}

// SpanID returns the span ID of the underlying APM span.
func (s *Span) SpanID() string {
	return s.apm.SpanID()
}

func (s *Span) Kind() string {
	return string(s.spanKind)
}

// APMTraceID returns the trace ID of the underlying APM span.
func (s *Span) APMTraceID() string {
	return s.apm.TraceID()
}

// TraceID returns the LLMObs trace ID for this span.
func (s *Span) TraceID() string {
	return s.llmTraceID
}

// MLApp returns the ML application name for this span.
func (s *Span) MLApp() string {
	return s.mlApp
}

// AddLink adds a span link to this span.
func (s *Span) AddLink(link SpanLink) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.apm.AddLink(link)
	s.spanLinks = append(s.spanLinks, link)
}

// StartTime returns the start time of this span.
func (s *Span) StartTime() time.Time {
	return s.startTime
}

// FinishTime returns the finish time of this span.
func (s *Span) FinishTime() time.Time {
	return s.finishTime
}

// Finish finishes the span with the provided configuration.
func (s *Span) Finish(cfg FinishSpanConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		log.Debug("llmobs: attempted to finish an already finished span")
		return
	}
	defer func() {
		trackSpanFinished(s)
	}()

	if cfg.FinishTime.IsZero() {
		cfg.FinishTime = time.Now()
	}
	s.finishTime = cfg.FinishTime
	apmFinishCfg := FinishAPMSpanConfig{
		FinishTime: cfg.FinishTime,
	}
	if cfg.Error != nil {
		s.error = cfg.Error
		apmFinishCfg.Error = cfg.Error
	}

	s.apm.Finish(apmFinishCfg)
	l, err := ActiveLLMObs()
	if err != nil {
		return
	}
	l.submitLLMObsSpan(s)
	s.finished = true
}

// Annotate adds annotations to the span using the provided SpanAnnotations.
func (s *Span) Annotate(a SpanAnnotations) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	defer func() {
		if err != nil {
			log.Warn("llmobs: failed to annotate span: %v", err.Error())
		}
		trackSpanAnnotations(s, err)
	}()

	if s.finished {
		err = errFinishedSpan
		return
	}

	s.llmCtx.metadata = updateMapKeys(s.llmCtx.metadata, a.Metadata)
	s.llmCtx.metrics = updateMapKeys(s.llmCtx.metrics, a.Metrics)

	if len(a.Tags) > 0 {
		s.llmCtx.tags = updateMapKeys(s.llmCtx.tags, a.Tags)
		if sessionID, ok := a.Tags[TagKeySessionID]; ok {
			s.sessionID = sessionID
		}
	}

	if a.Prompt != nil {
		if s.spanKind != SpanKindLLM {
			log.Warn("llmobs: input prompt can only be annotated on llm spans, ignoring")
		} else {
			if a.Prompt.RAGContextVariables == nil {
				a.Prompt.RAGContextVariables = []string{"context"}
			}
			if a.Prompt.RAGQueryVariables == nil {
				a.Prompt.RAGQueryVariables = []string{"question"}
			}
			s.llmCtx.prompt = a.Prompt
		}
	}

	if len(a.ToolDefinitions) > 0 {
		if s.spanKind != SpanKindLLM {
			log.Warn("llmobs: tool definitions can only be annotated on llm spans, ignoring")
		} else {
			s.llmCtx.toolDefinitions = a.ToolDefinitions
		}
	}

	if a.AgentManifest != "" {
		if s.spanKind != SpanKindAgent {
			log.Warn("llmobs: agent manifest can only be annotated on agent spans, ignoring")
		} else {
			s.llmCtx.agentManifest = a.AgentManifest
		}
	}

	s.annotateIO(a)
}

func (s *Span) annotateIO(a SpanAnnotations) {
	if a.OutputRetrievedDocs != nil && s.spanKind != SpanKindRetrieval {
		log.Warn("llmobs: retrieve docs can only be used to annotate outputs for retrieval spans, ignoring")
	}
	if a.InputEmbeddedDocs != nil && s.spanKind != SpanKindEmbedding {
		log.Warn("llmobs: embedding docs can only be used to annotate inputs for embedding spans, ignoring")
	}
	switch s.spanKind {
	case SpanKindLLM:
		s.annotateIOLLM(a)
	case SpanKindEmbedding:
		s.annotateIOEmbedding(a)
	case SpanKindRetrieval:
		s.annotateIORetrieval(a)
	case SpanKindExperiment:
		s.annotateIOExperiment(a)
	default:
		s.annotateIOText(a)
	}
}

func (s *Span) annotateIOLLM(a SpanAnnotations) {
	if a.InputMessages != nil {
		s.llmCtx.inputMessages = a.InputMessages
	} else if a.InputText != "" {
		s.llmCtx.inputMessages = []LLMMessage{{Content: a.InputText}}
	}
	if a.OutputMessages != nil {
		s.llmCtx.outputMessages = a.OutputMessages
	} else if a.OutputText != "" {
		s.llmCtx.outputMessages = []LLMMessage{{Content: a.OutputText}}
	}
}

func (s *Span) annotateIOEmbedding(a SpanAnnotations) {
	if a.InputText != "" || a.InputMessages != nil {
		log.Warn("llmobs: embedding spans can only be annotated with input embedded docs, ignoring other inputs")
	}
	if a.OutputMessages != nil || a.OutputRetrievedDocs != nil {
		log.Warn("llmobs: embedding spans can only be annotated with output text, ignoring other outputs")
	}
	if a.InputEmbeddedDocs != nil {
		s.llmCtx.inputDocuments = a.InputEmbeddedDocs
	}
	if a.OutputText != "" {
		s.llmCtx.outputText = a.OutputText
	}
}

func (s *Span) annotateIORetrieval(a SpanAnnotations) {
	if a.InputMessages != nil || a.InputEmbeddedDocs != nil {
		log.Warn("llmobs: retrieval spans can only be annotated with input text, ignoring other inputs")
	}
	if a.OutputText != "" || a.OutputMessages != nil {
		log.Warn("llmobs: retrieval spans can only be annotated with output retrieved docs, ignoring other outputs")
	}
	if a.InputText != "" {
		s.llmCtx.inputText = a.InputText
	}
	if a.OutputRetrievedDocs != nil {
		s.llmCtx.outputDocuments = a.OutputRetrievedDocs
	}
}

func (s *Span) annotateIOExperiment(a SpanAnnotations) {
	if a.ExperimentInput != nil {
		s.llmCtx.experimentInput = a.ExperimentInput
	}
	if a.ExperimentOutput != nil {
		s.llmCtx.experimentOutput = a.ExperimentOutput
	}
	if a.ExperimentExpectedOutput != nil {
		s.llmCtx.experimentExpectedOutput = a.ExperimentExpectedOutput
	}
}

func (s *Span) annotateIOText(a SpanAnnotations) {
	if a.InputMessages != nil || a.InputEmbeddedDocs != nil {
		log.Warn("llmobs: %s spans can only be annotated with input text, ignoring other inputs", s.spanKind)
	}
	if a.OutputMessages != nil || a.OutputRetrievedDocs != nil {
		log.Warn("llmobs: %s spans can only be annotated with output text, ignoring other outputs", s.spanKind)
	}
	if a.InputText != "" {
		s.llmCtx.inputText = a.InputText
	}
	if a.OutputText != "" {
		s.llmCtx.outputText = a.OutputText
	}
}

// sessionID returns the session ID for a given span, by checking the span's nearest LLMObs span ancestor.
func (s *Span) propagatedSessionID() string {
	curSpan := s
	usingParent := false

	for curSpan != nil {
		if curSpan.sessionID != "" {
			if usingParent {
				log.Debug("llmobs: using session_id from parent span: %s", curSpan.sessionID)
			}
			return curSpan.sessionID
		}
		curSpan = curSpan.parent
		usingParent = true
	}
	return ""
}

// propagatedMLApp returns the ML App name for a given span, by checking the span's nearest LLMObs span ancestor.
// It defaults to the global config LLMObs ML App name.
func (s *Span) propagatedMLApp() string {
	curSpan := s
	usingParent := false

	for curSpan != nil {
		if curSpan.mlApp != "" {
			if usingParent {
				log.Debug("llmobs: using ml_app from parent span: %s", curSpan.mlApp)
			}
			return curSpan.mlApp
		}
		curSpan = curSpan.parent
		usingParent = true
	}

	if s.propagated != nil && s.propagated.MLApp != "" {
		log.Debug("llmobs: using ml_app from propagated span: %s", s.propagated.MLApp)
		return s.propagated.MLApp
	}
	if activeLLMObs != nil {
		log.Debug("llmobs: using ml_app from global config: %s", activeLLMObs.Config.MLApp)
		return activeLLMObs.Config.MLApp
	}
	return ""
}

// updateMapKeys adds key/values from updates into src, overriding existing keys.
func updateMapKeys[K comparable, V any](src map[K]V, updates map[K]V) map[K]V {
	if len(updates) == 0 {
		return src
	}
	if src == nil {
		src = make(map[K]V, len(updates))
	}
	for k, v := range updates {
		src[k] = v
	}
	return src
}
