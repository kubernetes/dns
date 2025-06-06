// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import (
	"sync"
)

type SyncMap[K comparable, V any] struct {
	sync.Map
}

func (m *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.Map.Load(key)
	if !ok {
		return
	}
	value = v.(V)
	return
}

func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.Map.Range(func(key, value interface{}) bool {
		return f(key.(K), value.(V))
	})
}

func (m *SyncMap[K, V]) Len() int {
	count := 0
	m.Map.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (m *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	v, loaded := m.Map.LoadOrStore(key, value)
	return v.(V), loaded
}

func (m *SyncMap[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	v, loaded := m.Map.LoadAndDelete(key)
	if !loaded {
		var zero V
		return zero, loaded
	}
	return v.(V), true
}
