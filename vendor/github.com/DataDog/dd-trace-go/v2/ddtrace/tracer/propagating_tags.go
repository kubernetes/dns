// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"github.com/DataDog/dd-trace-go/v2/internal"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

func (t *trace) hasPropagatingTag(k string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.propagatingTags[k]
	return ok
}

func (t *trace) propagatingTag(k string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.propagatingTags[k]
}

// setPropagatingTag sets the key/value pair as a trace propagating tag.
func (t *trace) setPropagatingTag(key, value string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setPropagatingTagLocked(key, value)
}

func (t *trace) setTraceSourcePropagatingTag(key string, value internal.TraceSource) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If there is already a TraceSource value set in the trace
	// we need to add the new value to the bitmask.
	if source := t.propagatingTags[key]; source != "" {
		tSource, err := internal.ParseTraceSource(source)
		if err != nil {
			log.Error("failed to parse trace source tag: %s", err.Error())
		}

		tSource |= value

		t.setPropagatingTagLocked(key, tSource.String())
		return
	}

	t.setPropagatingTagLocked(key, value.String())
}

// setPropagatingTagLocked sets the key/value pair as a trace propagating tag.
// Not safe for concurrent use, setPropagatingTag should be used instead in that case.
func (t *trace) setPropagatingTagLocked(key, value string) {
	if t.propagatingTags == nil {
		t.propagatingTags = make(map[string]string, 1)
	}
	t.propagatingTags[key] = value
}

// unsetPropagatingTag deletes the key/value pair from the trace's propagated tags.
func (t *trace) unsetPropagatingTag(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.propagatingTags, key)
}

// iteratePropagatingTags allows safe iteration through the propagating tags of a trace.
// the trace must not be modified during this call, as it is locked for reading.
//
// f should return whether the iteration should continue.
func (t *trace) iteratePropagatingTags(f func(k, v string) bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for k, v := range t.propagatingTags {
		if !f(k, v) {
			break
		}
	}
}

func (t *trace) replacePropagatingTags(tags map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.propagatingTags = tags
}

func (t *trace) propagatingTagsLen() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.propagatingTags)
}
