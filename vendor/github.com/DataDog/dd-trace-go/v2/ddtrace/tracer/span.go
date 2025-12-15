// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -unexported -marshal=false -o=span_msgp.go -tests=false

package tracer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"runtime/pprof"
	rt "runtime/trace"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/errortrace"
	sharedinternal "github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/env"
	"github.com/DataDog/dd-trace-go/v2/internal/globalconfig"
	illmobs "github.com/DataDog/dd-trace-go/v2/internal/llmobs"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/orchestrion"
	"github.com/DataDog/dd-trace-go/v2/internal/samplernames"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	"github.com/DataDog/dd-trace-go/v2/internal/traceprof"

	"github.com/tinylib/msgp/msgp"

	"golang.org/x/xerrors"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
)

type (
	// spanList implements msgp.Encodable on top of a slice of spans.
	spanList []*Span

	// spanLists implements msgp.Decodable on top of a slice of spanList.
	// This type is only used in tests.
	spanLists []spanList
)

var (
	_ msgp.Encodable = (*spanList)(nil)
	_ msgp.Decodable = (*spanLists)(nil)
)

// errorConfig holds customization options for setting error tags.
type errorConfig struct {
	noDebugStack bool
	stackFrames  uint
	stackSkip    uint
}

// AsMap places tags and span properties into a map and returns it.
//
// Note that this is not performant, nor are spans guaranteed to have all of their
// properties set at any time during normal operation! This is used for testing only,
// and should not be used in non-test code, or you may run into performance or other
// issues.
func (s *Span) AsMap() map[string]interface{} {
	m := make(map[string]interface{})
	if s == nil {
		return m
	}
	m[ext.SpanName] = s.name
	m[ext.ServiceName] = s.service
	m[ext.ResourceName] = s.resource
	m[ext.SpanType] = s.spanType
	m[ext.MapSpanStart] = s.start
	m[ext.MapSpanDuration] = s.duration
	for k, v := range s.meta {
		m[k] = v
	}
	for k, v := range s.metrics {
		m[k] = v
	}
	for k, v := range s.metaStruct {
		m[k] = v
	}
	m[ext.MapSpanID] = s.spanID
	m[ext.MapSpanTraceID] = s.traceID
	m[ext.MapSpanParentID] = s.parentID
	m[ext.MapSpanError] = s.error
	if events := s.spanEventsAsJSONString(); events != "" {
		m[ext.MapSpanEvents] = events
	}
	return m
}

func (s *Span) spanEventsAsJSONString() string {
	if !s.supportsEvents {
		return s.meta["events"]
	}
	if s.spanEvents == nil {
		return ""
	}
	events, err := json.Marshal(s.spanEvents)
	if err != nil {
		log.Error("failed to marshal span events: %s", err.Error())
		return ""
	}
	return string(events)
}

// Span represents a computation. Callers must call Finish when a Span is
// complete to ensure it's submitted.
type Span struct {
	mu sync.RWMutex `msg:"-"` // all fields are protected by this RWMutex

	name       string             `msg:"name"`                  // operation name
	service    string             `msg:"service"`               // service name (i.e. "grpc.server", "http.request")
	resource   string             `msg:"resource"`              // resource name (i.e. "/user?id=123", "SELECT * FROM users")
	spanType   string             `msg:"type"`                  // protocol associated with the span (i.e. "web", "db", "cache")
	start      int64              `msg:"start"`                 // span start time expressed in nanoseconds since epoch
	duration   int64              `msg:"duration"`              // duration of the span expressed in nanoseconds
	meta       map[string]string  `msg:"meta,omitempty"`        // arbitrary map of metadata
	metaStruct metaStructMap      `msg:"meta_struct,omitempty"` // arbitrary map of metadata with structured values
	metrics    map[string]float64 `msg:"metrics,omitempty"`     // arbitrary map of numeric metrics
	spanID     uint64             `msg:"span_id"`               // identifier of this span
	traceID    uint64             `msg:"trace_id"`              // lower 64-bits of the root span identifier
	parentID   uint64             `msg:"parent_id"`             // identifier of the span's direct parent
	error      int32              `msg:"error"`                 // error status of the span; 0 means no errors
	spanLinks  []SpanLink         `msg:"span_links,omitempty"`  // links to other spans
	spanEvents []spanEvent        `msg:"span_events,omitempty"` // events produced related to this span

	goExecTraced   bool         `msg:"-"`
	noDebugStack   bool         `msg:"-"` // disables debug stack traces
	finished       bool         `msg:"-"` // true if the span has been submitted to a tracer. Can only be read/modified if the trace is locked.
	context        *SpanContext `msg:"-"` // span propagation context
	integration    string       `msg:"-"` // where the span was started from, such as a specific contrib or "manual"
	supportsEvents bool         `msg:"-"` // whether the span supports native span events or not

	pprofCtxActive  context.Context `msg:"-"` // contains pprof.WithLabel labels to tell the profiler more about this span
	pprofCtxRestore context.Context `msg:"-"` // contains pprof.WithLabel labels of the parent span (if any) that need to be restored when this span finishes

	taskEnd func() // ends execution tracer (runtime/trace) task, if started
}

// Context yields the SpanContext for this Span. Note that the return
// value of Context() is still valid after a call to Finish(). This is
// called the span context and it is different from Go's context.
func (s *Span) Context() *SpanContext {
	if s == nil {
		return nil
	}
	return s.context
}

// SetBaggageItem sets a key/value pair as baggage on the span. Baggage items
// are propagated down to descendant spans and injected cross-process. Use with
// care as it adds extra load onto your tracing layer.
func (s *Span) SetBaggageItem(key, val string) {
	if s == nil {
		return
	}
	s.context.setBaggageItem(key, val)
}

// BaggageItem gets the value for a baggage item given its key. Returns the
// empty string if the value isn't found in this Span.
func (s *Span) BaggageItem(key string) string {
	if s == nil {
		return ""
	}
	return s.context.baggageItem(key)
}

// SetTag adds a set of key/value metadata to the span.
func (s *Span) SetTag(key string, value interface{}) {
	if s == nil {
		return
	}
	// To avoid dumping the memory address in case value is a pointer, we dereference it.
	// Any pointer value that is a pointer to a pointer will be dumped as a string.
	value = dereference(value)
	s.mu.Lock()
	defer s.mu.Unlock()

	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		return
	}
	switch key {
	case ext.Error:
		s.setTagError(value, errorConfig{
			noDebugStack: s.noDebugStack,
		})
		return
	case ext.ErrorNoStackTrace:
		s.setTagError(value, errorConfig{
			noDebugStack: true,
		})
		return
	case ext.Component:
		integration, ok := value.(string)
		if ok {
			s.integration = integration
		}
	}
	if v, ok := value.(bool); ok {
		s.setTagBool(key, v)
		return
	}
	if v, ok := value.(string); ok {
		if key == ext.ResourceName && s.pprofCtxActive != nil && spanResourcePIISafe(s) {
			// If the user overrides the resource name for the span,
			// update the endpoint label for the runtime profilers.
			//
			// We don't change s.pprofCtxRestore since that should
			// stay as the original parent span context regardless
			// of what we change at a lower level.
			s.pprofCtxActive = pprof.WithLabels(s.pprofCtxActive, pprof.Labels(traceprof.TraceEndpoint, v))
			pprof.SetGoroutineLabels(s.pprofCtxActive)
		}
		s.setMeta(key, v)
		return
	}
	if v, ok := sharedinternal.ToFloat64(value); ok {
		s.setMetric(key, v)
		return
	}
	if v, ok := value.(fmt.Stringer); ok {
		defer func() {
			if e := recover(); e != nil {
				if v := reflect.ValueOf(value); v.Kind() == reflect.Ptr && v.IsNil() {
					// If .String() panics due to a nil receiver, we want to catch this
					// and replace the string value with "<nil>", just as Sprintf does.
					// Other panics should not be handled.
					s.setMeta(key, "<nil>")
					return
				}
				panic(e)
			}
		}()
		s.setMeta(key, v.String())
		return
	}

	if v, ok := value.([]byte); ok {
		s.setMeta(key, string(v))
		return
	}

	if value != nil {
		// Arrays will be translated to dot notation. e.g.
		// {"myarr.0": "foo", "myarr.1": "bar"}
		// which will be displayed as an array in the UI.
		switch reflect.TypeOf(value).Kind() {
		case reflect.Slice:
			slice := reflect.ValueOf(value)
			for i := 0; i < slice.Len(); i++ {
				key := fmt.Sprintf("%s.%d", key, i)
				v := slice.Index(i)
				if num, ok := sharedinternal.ToFloat64(v.Interface()); ok {
					s.setMetric(key, num)
				} else {
					s.setMeta(key, fmt.Sprintf("%v", v))
				}
			}
			return
		}

		// Can be sent as messagepack in `meta_struct` instead of `meta`
		// reserved for internal use only
		if v, ok := value.(sharedinternal.MetaStructValue); ok {
			s.setMetaStruct(key, v.Value)
			return
		}

		// Support for v1 shim meta struct values (only _dd.stack uses this)
		if key == "_dd.stack" {
			s.setMetaStruct(key, value)
			return
		}

		// Add this trace source tag to propagating tags and to span tags
		// reserved for internal use only
		if v, ok := value.(sharedinternal.TraceSourceTagValue); ok {
			s.context.trace.setTraceSourcePropagatingTag(key, v.Value)
		}
	}

	// not numeric, not a string, not a fmt.Stringer, not a bool, and not an error
	s.setMeta(key, fmt.Sprint(value))
}

// setSamplingPriority locks the span, then updates the sampling priority.
// It also updates the trace's sampling priority.
func (s *Span) setSamplingPriority(priority int, sampler samplernames.SamplerName) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setSamplingPriorityLocked(priority, sampler)
}

func (s *Span) setProcessTags(pTags string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setMeta(keyProcessTags, pTags)
}

// root returns the root span of the span's trace. The return value shouldn't be
// nil as long as the root span is valid and not finished.
func (s *Span) Root() *Span {
	if s == nil || s.context == nil {
		return nil
	}
	if s.context.trace == nil {
		return nil
	}
	return s.context.trace.root
}

// SetUser associates user information to the current trace which the
// provided span belongs to. The options can be used to tune which user
// bit of information gets monitored. In case of distributed traces,
// the user id can be propagated across traces using the WithPropagation() option.
// See https://docs.datadoghq.com/security_platform/application_security/setup_and_configure/?tab=set_user#add-user-information-to-traces
func (s *Span) SetUser(id string, opts ...UserMonitoringOption) {
	if s == nil {
		return
	}
	cfg := UserMonitoringConfig{
		Metadata: make(map[string]string),
	}
	for _, fn := range opts {
		fn(&cfg)
	}
	root := s.Root()
	trace := root.context.trace
	root.mu.Lock()
	defer root.mu.Unlock()

	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if root.finished {
		return
	}
	if cfg.PropagateID {
		// Delete usr.id from the tags since _dd.p.usr.id takes precedence
		delete(root.meta, keyUserID)
		idenc := base64.StdEncoding.EncodeToString([]byte(id))
		trace.setPropagatingTag(keyPropagatedUserID, idenc)
		s.context.updated = true
	} else {
		if trace.hasPropagatingTag(keyPropagatedUserID) {
			// Unset the propagated user ID so that a propagated user ID coming from upstream won't be propagated anymore.
			trace.unsetPropagatingTag(keyPropagatedUserID)
			s.context.updated = true
		}
		delete(root.meta, keyPropagatedUserID)
	}

	usrData := map[string]string{
		keyUserID:        id,
		keyUserLogin:     cfg.Login,
		keyUserEmail:     cfg.Email,
		keyUserName:      cfg.Name,
		keyUserScope:     cfg.Scope,
		keyUserRole:      cfg.Role,
		keyUserSessionID: cfg.SessionID,
	}
	for k, v := range cfg.Metadata {
		usrData[fmt.Sprintf("usr.%s", k)] = v
	}
	for k, v := range usrData {
		if v != "" {
			// setMeta is used since the span is already locked
			root.setMeta(k, v)
		}
	}
}

// StartChild starts a new child span with the given operation name and options.
func (s *Span) StartChild(operationName string, opts ...StartSpanOption) *Span {
	if s == nil {
		return nil
	}
	opts = append(opts, ChildOf(s.Context()))
	return getGlobalTracer().StartSpan(operationName, opts...)
}

// setSamplingPriorityLocked updates the sampling priority.
// It also updates the trace's sampling priority.
func (s *Span) setSamplingPriorityLocked(priority int, sampler samplernames.SamplerName) {
	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		return
	}
	s.setMetric(keySamplingPriority, float64(priority))
	s.context.setSamplingPriority(priority, sampler)
}

// forceSetSamplingPriorityLocked updates the sampling priority.
// If the trace is locked, the sampling priority is forced to the given value.
//
// This function is should only be used when applying a manual keep or drop decision.
func (s *Span) forceSetSamplingPriorityLocked(priority int, sampler samplernames.SamplerName) {
	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		return
	}
	s.setMetric(keySamplingPriority, float64(priority))
	s.context.forceSetSamplingPriority(priority, sampler)
}

// setTagError sets the error tag. It accounts for various valid scenarios.
// This method is not safe for concurrent use.
func (s *Span) setTagError(value interface{}, cfg errorConfig) {
	setError := func(yes bool) {
		if yes {
			if s.error == 0 {
				// new error
				s.context.errors.Add(1)
			}
			s.error = 1
		} else {
			if s.error > 0 {
				// flip from active to inactive
				s.context.errors.Add(-1)
			}
			s.error = 0
		}
	}
	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		return
	}
	switch v := value.(type) {
	case bool:
		// bool value as per Opentracing spec.
		setError(v)
	case error:
		// if anyone sets an error value as the tag, be nice here
		// and provide all the benefits.
		// TODO: once Error Tracking fix is resolved, update relevant tags here. See #4095
		setError(true)
		s.setMeta(ext.ErrorMsg, v.Error())
		s.setMeta(ext.ErrorType, reflect.TypeOf(v).String())
		if cfg.noDebugStack {
			return
		}
		switch err := v.(type) {
		case xerrors.Formatter:
			s.setMeta(ext.ErrorDetails, fmt.Sprintf("%+v", v))
		case fmt.Formatter:
			// pkg/errors approach
			s.setMeta(ext.ErrorDetails, fmt.Sprintf("%+v", v))
		case *errortrace.TracerError:
			// instrumentation/errortrace approach
			s.setMeta(ext.ErrorStack, fmt.Sprintf("%+v", v))
			s.setMeta(ext.ErrorHandlingStack, err.Format())
			return
		}
		stack := takeStacktrace(cfg.stackFrames, cfg.stackSkip)
		s.setMeta(ext.ErrorStack, stack)
	case nil:
		// no error
		setError(false)
	default:
		// in all other cases, let's assume that setting this tag
		// is the result of an error.
		setError(true)
	}
}

// defaultStackLength specifies the default maximum size of a stack trace.
const defaultStackLength = 32

// takeStacktrace takes a stack trace of maximum n entries, skipping the first skip entries.
// If n is 0, up to 20 entries are retrieved.
func takeStacktrace(n, skip uint) string {
	telemetry.Count(telemetry.NamespaceTracers, "errorstack.source", []string{"source:takeStacktrace"}).Submit(1)
	now := time.Now()
	defer func() {
		dur := float64(time.Since(now))
		telemetry.Distribution(telemetry.NamespaceTracers, "errorstack.duration", []string{"source:takeStacktrace"}).Submit(dur)
	}()
	if n == 0 {
		n = defaultStackLength
	}
	var builder strings.Builder
	pcs := make([]uintptr, n)

	// +2 to exclude runtime.Callers and takeStacktrace
	numFrames := runtime.Callers(2+int(skip), pcs)
	if numFrames == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:numFrames])
	for i := 0; ; i++ {
		frame, more := frames.Next()
		if i != 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(frame.Function)
		builder.WriteByte('\n')
		builder.WriteByte('\t')
		builder.WriteString(frame.File)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(frame.Line))
		if !more {
			break
		}
	}
	return builder.String()
}

// setMeta sets a string tag. This method is not safe for concurrent use.
func (s *Span) setMeta(key, v string) {
	if s.meta == nil {
		s.meta = make(map[string]string, 1)
	}
	delete(s.metrics, key)
	switch key {
	case ext.SpanName:
		s.name = v
	case ext.ServiceName:
		s.service = v
	case ext.ResourceName:
		s.resource = v
	case ext.SpanType:
		s.spanType = v
	default:
		s.meta[key] = v
	}
}

func (s *Span) setMetaStruct(key string, v any) {
	if s.metaStruct == nil {
		s.metaStruct = make(metaStructMap, 1)
	}
	s.metaStruct[key] = v
}

// setTagBool sets a boolean tag on the span.
func (s *Span) setTagBool(key string, v bool) {
	switch key {
	case ext.AnalyticsEvent:
		if v {
			s.setMetric(ext.EventSampleRate, 1.0)
		} else {
			s.setMetric(ext.EventSampleRate, 0.0)
		}
	case ext.ManualDrop:
		if v {
			s.forceSetSamplingPriorityLocked(ext.PriorityUserReject, samplernames.Manual)
		}
	case ext.ManualKeep:
		if v {
			s.forceSetSamplingPriorityLocked(ext.PriorityUserKeep, samplernames.Manual)
		}
	default:
		if v {
			s.setMeta(key, "true")
		} else {
			s.setMeta(key, "false")
		}
	}
}

// setMetric sets a numeric tag, in our case called a metric. This method
// is not safe for concurrent use.
func (s *Span) setMetric(key string, v float64) {
	if s.metrics == nil {
		s.metrics = make(map[string]float64, 1)
	}
	delete(s.meta, key)
	switch key {
	case ext.ManualKeep:
		if v == float64(samplernames.AppSec) {
			s.setSamplingPriorityLocked(ext.PriorityUserKeep, samplernames.AppSec)
		}
	case "_sampling_priority_v1shim":
		// We have this for backward compatibility with the v1 shim.
		s.setSamplingPriorityLocked(int(v), samplernames.Manual)
	default:
		s.metrics[key] = v
	}
}

// AddLink appends the given link to the span's span links.
func (s *Span) AddLink(link SpanLink) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		// already finished
		return
	}
	s.spanLinks = append(s.spanLinks, link)
}

// serializeSpanLinksInMeta saves span links as a JSON string under `Span[meta][_dd.span_links]`.
func (s *Span) serializeSpanLinksInMeta() {
	if len(s.spanLinks) == 0 {
		return
	}
	spanLinkBytes, err := json.Marshal(s.spanLinks)
	if err != nil {
		log.Debug("Unable to marshal span links. Not adding span links to span meta.")
		return
	}
	if s.meta == nil {
		s.meta = make(map[string]string)
	}
	s.meta["_dd.span_links"] = string(spanLinkBytes)
}

// serializeSpanEvents sets the span events from the current span in the correct transport, depending on whether the
// agent supports the native method or not.
func (s *Span) serializeSpanEvents() {
	if len(s.spanEvents) == 0 {
		return
	}
	// if span events are natively supported by the agent, there's nothing to do
	// as the events will be already included when the span is serialized.
	if s.supportsEvents {
		return
	}
	// otherwise, we need to serialize them as a string tag and remove them from the struct
	// so they are not sent twice.
	b, err := json.Marshal(s.spanEvents)
	s.spanEvents = nil
	if err != nil {
		log.Debug("Unable to marshal span events; events dropped from span meta\n%s", err.Error())
		return
	}
	s.meta["events"] = string(b)
}

// Finish closes this Span (but not its children) providing the duration
// of its part of the tracing session.
func (s *Span) Finish(opts ...FinishOption) {
	if s == nil {
		return
	}

	t := now()
	if len(opts) > 0 {
		cfg := FinishConfig{
			NoDebugStack: s.noDebugStack,
		}
		for _, fn := range opts {
			if fn == nil {
				continue
			}
			fn(&cfg)
		}
		if !cfg.FinishTime.IsZero() {
			t = cfg.FinishTime.UnixNano()
		}
		if cfg.Error != nil {
			s.mu.Lock()
			s.setTagError(cfg.Error, errorConfig{
				noDebugStack: cfg.NoDebugStack,
				stackFrames:  cfg.StackFrames,
				stackSkip:    cfg.SkipStackFrames,
			})
			s.mu.Unlock()
		}
	}

	if s.goExecTraced && rt.IsEnabled() {
		// Only tag spans as traced if they both started & ended with
		// execution tracing enabled. This is technically not sufficient
		// for spans which could straddle the boundary between two
		// execution traces, but there's really nothing we can do in
		// those cases since execution tracing tasks aren't recorded in
		// traces if they started before the trace.
		s.SetTag("go_execution_traced", "yes")
	} else if s.goExecTraced {
		// If the span started with tracing enabled, but tracing wasn't
		// enabled when the span finished, we still have some data to
		// show. If tracing wasn't enabled when the span started, we
		// won't have data in the execution trace to identify it so
		// there's nothign we can show.
		s.SetTag("go_execution_traced", "partial")
	}

	if s.Root() == s {
		if tr, ok := getGlobalTracer().(*tracer); ok && tr.rulesSampling.traces.enabled() {
			if !s.context.trace.isLocked() && s.context.trace.propagatingTag(keyDecisionMaker) != "-4" {
				tr.rulesSampling.SampleTrace(s)
			}
		}
	}

	s.finish(t)
	orchestrion.GLSPopValue(sharedinternal.ActiveSpanKey)
}

// SetOperationName sets or changes the operation name.
func (s *Span) SetOperationName(operationName string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		// already finished
		return
	}
	s.name = operationName
}

func (s *Span) finish(finishTime int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		// already finished
		return
	}

	s.serializeSpanLinksInMeta()
	s.serializeSpanEvents()

	if s.duration == 0 {
		s.duration = finishTime - s.start
	}
	if s.duration < 0 {
		s.duration = 0
	}
	if s.taskEnd != nil {
		s.taskEnd()
	}

	keep := true
	tracer, hasTracer := getGlobalTracer().(*tracer)
	if hasTracer {
		if !tracer.config.enabled.current {
			return
		}
		if tracer.config.canDropP0s() {
			// the agent supports dropping p0's in the client
			keep = shouldKeep(s)
		}
		if tracer.config.debugAbandonedSpans {
			// the tracer supports debugging abandoned spans
			tracer.submitAbandonedSpan(s, true)
		}
		tracer.spansFinished.Inc(s.integration)
	}
	if keep {
		// a single kept span keeps the whole trace.
		s.context.trace.keep()
	}
	if log.DebugEnabled() {
		// avoid allocating the ...interface{} argument if debug logging is disabled
		log.Debug("Finished Span: %v, Operation: %s, Resource: %s, Tags: %v, %v", //nolint:gocritic // Debug logging needs full span representation
			s, s.name, s.resource, s.meta, s.metrics)
	}
	s.context.finish()

	// compute stats after finishing the span. This ensures any normalization or tag propagation has been applied
	if hasTracer {
		tracer.submit(s)
	}

	if s.pprofCtxRestore != nil {
		// Restore the labels of the parent span so any CPU samples after this
		// point are attributed correctly.
		pprof.SetGoroutineLabels(s.pprofCtxRestore)
	}
}

// textNonParsable specifies the text that will be assigned to resources for which the resource
// can not be parsed due to an obfuscation error.
const textNonParsable = "Non-parsable SQL query"

// obfuscatedResource returns the obfuscated version of the given resource. It is
// obfuscated using the given obfuscator for the given span type typ.
func obfuscatedResource(o *obfuscate.Obfuscator, typ, resource string) string {
	if o == nil {
		return resource
	}
	switch typ {
	case "sql", "cassandra":
		oq, err := o.ObfuscateSQLString(resource)
		if err != nil {
			log.Error("Error obfuscating stats group resource %q: %v", resource, err.Error())
			return textNonParsable
		}
		return oq.Query
	case "redis":
		return o.QuantizeRedisString(resource)
	default:
		return resource
	}
}

// shouldKeep reports whether the trace should be kept.
// a single span being kept implies the whole trace being kept.
func shouldKeep(s *Span) bool {
	if p, ok := s.context.SamplingPriority(); ok && p > 0 {
		// positive sampling priorities stay
		return true
	}
	if s.context.errors.Load() > 0 {
		// traces with any span containing an error get kept
		return true
	}
	if v, ok := s.metrics[ext.EventSampleRate]; ok {
		return sampledByRate(s.traceID, v)
	}
	return false
}

// shouldComputeStats mentions whether this span needs to have stats computed for.
// Warning: callers must guard!
func shouldComputeStats(s *Span) bool {
	if v, ok := s.metrics[keyMeasured]; ok && v == 1 {
		return true
	}
	if v, ok := s.metrics[keyTopLevel]; ok && v == 1 {
		return true
	}
	return false
}

// String returns a human readable representation of the span. Not for
// production, just debugging.
func (s *Span) String() string {
	if s == nil {
		return "<nil>"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	lines := []string{
		fmt.Sprintf("Name: %s", s.name),
		fmt.Sprintf("Service: %s", s.service),
		fmt.Sprintf("Resource: %s", s.resource),
		fmt.Sprintf("TraceID: %d", s.traceID),
		fmt.Sprintf("TraceID128: %s", s.context.TraceID()),
		fmt.Sprintf("SpanID: %d", s.spanID),
		fmt.Sprintf("ParentID: %d", s.parentID),
		fmt.Sprintf("Start: %s", time.Unix(0, s.start)),
		fmt.Sprintf("Duration: %s", time.Duration(s.duration)),
		fmt.Sprintf("Error: %d", s.error),
		fmt.Sprintf("Type: %s", s.spanType),
		"Tags:",
	}
	for key, val := range s.meta {
		lines = append(lines, fmt.Sprintf("\t%s:%s", key, val))
	}
	for key, val := range s.metrics {
		lines = append(lines, fmt.Sprintf("\t%s:%f", key, val))
	}
	return strings.Join(lines, "\n")
}

// Format implements fmt.Formatter.
func (s *Span) Format(f fmt.State, c rune) {
	if s == nil {
		fmt.Fprintf(f, "<nil>")
	}
	switch c {
	case 's':
		fmt.Fprint(f, s.String())
	case 'v':
		if svc := globalconfig.ServiceName(); svc != "" {
			fmt.Fprintf(f, "dd.service=%s ", svc)
		}
		if tr := getGlobalTracer(); tr != nil {
			tc := tr.TracerConf()
			if tc.EnvTag != "" {
				fmt.Fprintf(f, "dd.env=%s ", tc.EnvTag)
			} else if env := env.Get("DD_ENV"); env != "" {
				fmt.Fprintf(f, "dd.env=%s ", env)
			}
			if tc.VersionTag != "" {
				fmt.Fprintf(f, "dd.version=%s ", tc.VersionTag)
			} else if v := env.Get("DD_VERSION"); v != "" {
				fmt.Fprintf(f, "dd.version=%s ", v)
			}
		}
		var traceID string
		if sharedinternal.BoolEnv("DD_TRACE_128_BIT_TRACEID_LOGGING_ENABLED", true) && s.context.traceID.HasUpper() {
			traceID = s.context.TraceID()
		} else {
			traceID = fmt.Sprintf("%d", s.traceID)
		}
		fmt.Fprintf(f, `dd.trace_id=%q `, traceID)
		fmt.Fprintf(f, `dd.span_id="%d" `, s.spanID)
		fmt.Fprintf(f, `dd.parent_id="%d"`, s.parentID)
	default:
		fmt.Fprintf(f, "%%!%c(tracer.Span=%v)", c, s)
	}
}

// AddEvent attaches a new event to the current span.
func (s *Span) AddEvent(name string, opts ...SpanEventOption) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// We don't lock spans when flushing, so we could have a data race when
	// modifying a span as it's being flushed. This protects us against that
	// race, since spans are marked `finished` before we flush them.
	if s.finished {
		return
	}
	cfg := SpanEventConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Time.IsZero() {
		cfg.Time = time.Now()
	}
	event := spanEvent{
		Name:         name,
		TimeUnixNano: uint64(cfg.Time.UnixNano()),
	}
	if s.supportsEvents {
		event.Attributes = toSpanEventAttributeMsg(cfg.Attributes)
	} else {
		event.RawAttributes = cfg.Attributes
	}
	s.spanEvents = append(s.spanEvents, event)
}

func setLLMObsPropagatingTags(ctx context.Context, spanCtx *SpanContext) {
	llmSpan, ok := illmobs.ActiveLLMSpanFromContext(ctx)
	if !ok {
		return
	}
	spanCtx.trace.setPropagatingTag(keyPropagatedLLMObsParentID, llmSpan.SpanID())
	spanCtx.trace.setPropagatingTag(keyPropagatedLLMObsTraceID, llmSpan.TraceID())
	spanCtx.trace.setPropagatingTag(keyPropagatedLLMObsMLAPP, llmSpan.MLApp())
}

// used in internal/civisibility/integrations/manual_api_common.go using linkname
func getMeta(s *Span, key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.meta[key]
	return val, ok
}

// used in internal/civisibility/integrations/manual_api_common.go using linkname
func getMetric(s *Span, key string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.metrics[key]
	return val, ok
}

const (
	keySamplingPriority     = "_sampling_priority_v1"
	keySamplingPriorityRate = "_dd.agent_psr"
	keyDecisionMaker        = "_dd.p.dm"
	keyOrigin               = "_dd.origin"
	keyReparentID           = "_dd.parent_id"
	// keyHostname can be used to override the agent's hostname detection when using `WithHostname`.
	// which is set via auto-detection.
	keyHostname                = "_dd.hostname"
	keyRulesSamplerAppliedRate = "_dd.rule_psr"
	keyRulesSamplerLimiterRate = "_dd.limit_psr"
	keyMeasured                = "_dd.measured"
	// keyTopLevel is the key of top level metric indicating if a span is top level.
	// A top level span is a local root (parent span of the local trace) or the first span of each service.
	keyTopLevel = "_dd.top_level"
	// keyPropagationError holds any error from propagated trace tags (if any)
	keyPropagationError = "_dd.propagation_error"
	// keySpanSamplingMechanism specifies the sampling mechanism by which an individual span was sampled
	keySpanSamplingMechanism = "_dd.span_sampling.mechanism"
	// keySingleSpanSamplingRuleRate specifies the configured sampling probability for the single span sampling rule.
	keySingleSpanSamplingRuleRate = "_dd.span_sampling.rule_rate"
	// keySingleSpanSamplingMPS specifies the configured limit for the single span sampling rule
	// that the span matched. If there is no configured limit, then this tag is omitted.
	keySingleSpanSamplingMPS = "_dd.span_sampling.max_per_second"
	// keyPropagatedUserID holds the propagated user identifier, if user id propagation is enabled.
	keyPropagatedUserID = "_dd.p.usr.id"
	// keyPropagatedTraceSource holds a 2 character hexadecimal string representation of the product responsible
	// for the span creation.
	keyPropagatedTraceSource = "_dd.p.ts"
	// keyTraceID128 is the lowercase, hex encoded upper 64 bits of a 128-bit trace id, if present.
	keyTraceID128 = "_dd.p.tid"
	// keySpanAttributeSchemaVersion holds the selected DD_TRACE_SPAN_ATTRIBUTE_SCHEMA version.
	keySpanAttributeSchemaVersion = "_dd.trace_span_attribute_schema"
	// keyPeerServiceSource indicates the precursor tag that was used as the value of peer.service.
	keyPeerServiceSource = "_dd.peer.service.source"
	// keyPeerServiceRemappedFrom indicates the previous value for peer.service, in case remapping happened.
	keyPeerServiceRemappedFrom = "_dd.peer.service.remapped_from"
	// keyBaseService contains the globally configured tracer service name. It is only set for spans that override it.
	keyBaseService = "_dd.base_service"
	// keyProcessTags contains a list of process tags to identify the service.
	keyProcessTags = "_dd.tags.process"
	// keyKnuthSamplingRate holds the propagated Knuth-based sampling rate applied by agent or trace sampling rules.
	// Value is a string with up to 6 decimal digits and is forwarded unchanged.
	keyKnuthSamplingRate = "_dd.p.ksr"
	// keyPropagatedLLMObsParentID contains the propagated llmobs span ID.
	keyPropagatedLLMObsParentID = "_dd.p.llmobs_parent_id"
	// keyPropagatedLLMObsMLAPP contains the propagated ML App.
	keyPropagatedLLMObsMLAPP = "_dd.p.llmobs_ml_app"
	// keyPropagatedLLMObsTraceID contains the propagated llmobs trace ID.
	keyPropagatedLLMObsTraceID = "_dd.p.llmobs_trace_id"
)

// The following set of tags is used for user monitoring and set through calls to span.SetUser().
const (
	keyUserID        = "usr.id"
	keyUserLogin     = "usr.login"
	keyUserEmail     = "usr.email"
	keyUserName      = "usr.name"
	keyUserRole      = "usr.role"
	keyUserScope     = "usr.scope"
	keyUserSessionID = "usr.session_id"
)
