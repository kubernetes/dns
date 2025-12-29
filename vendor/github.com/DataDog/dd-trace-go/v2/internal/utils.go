// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package internal

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/internal/samplernames"
	"github.com/puzpuzpuz/xsync/v3"
)

// OtelTagsDelimeter is the separator between key-val pairs for OTEL env vars
const OtelTagsDelimeter = "="

// DDTagsDelimiter is the separator between key-val pairs for DD env vars
const DDTagsDelimiter = ":"

// LockMap uses an RWMutex to synchronize map access to allow for concurrent access.
// This should not be used for cases with heavy write load and performance concerns.
type LockMap struct {
	sync.RWMutex
	c uint32
	m map[string]string
}

func NewLockMap(m map[string]string) *LockMap {
	return &LockMap{m: m, c: uint32(len(m))}
}

// Iter iterates over all the map entries passing in keys and values to provided func f. Note this is READ ONLY.
func (l *LockMap) Iter(f func(key string, val string)) {
	c := atomic.LoadUint32(&l.c)
	if c == 0 { //Fast exit to avoid the cost of RLock/RUnlock for empty maps
		return
	}
	l.RLock()
	defer l.RUnlock()
	for k, v := range l.m {
		f(k, v)
	}
}

func (l *LockMap) Len() int {
	l.RLock()
	defer l.RUnlock()
	return len(l.m)
}

func (l *LockMap) Clear() {
	l.Lock()
	defer l.Unlock()
	l.m = map[string]string{}
	atomic.StoreUint32(&l.c, 0)
}

func (l *LockMap) Set(k, v string) {
	l.Lock()
	defer l.Unlock()
	if _, ok := l.m[k]; !ok {
		atomic.AddUint32(&l.c, 1)
	}
	l.m[k] = v
}

func (l *LockMap) Get(k string) string {
	l.RLock()
	defer l.RUnlock()
	return l.m[k]
}

// XSyncMapCounterMap uses xsync protect counter increments and reads during
// concurrent access.
// Implementation and related tests were taken/inspired by felixge/countermap
// https://github.com/felixge/countermap/pull/2
type XSyncMapCounterMap struct {
	counts *xsync.MapOf[string, *xsync.Counter]
}

func NewXSyncMapCounterMap() *XSyncMapCounterMap {
	return &XSyncMapCounterMap{counts: xsync.NewMapOf[string, *xsync.Counter]()}
}

func (cm *XSyncMapCounterMap) Inc(key string) {
	val, ok := cm.counts.Load(key)
	if !ok {
		val, _ = cm.counts.LoadOrStore(key, xsync.NewCounter())
	}
	val.Inc()
}

func (cm *XSyncMapCounterMap) GetAndReset() map[string]int64 {
	ret := map[string]int64{}
	cm.counts.Range(func(key string, _ *xsync.Counter) bool {
		v, ok := cm.counts.LoadAndDelete(key)
		if ok {
			ret[key] = v.Value()
		}
		return true
	})
	return ret
}

// ToFloat64 attempts to convert value into a float64. If the value is an integer
// greater or equal to 2^53 or less than or equal to -2^53, it will not be converted
// into a float64 to avoid losing precision. If it succeeds in converting, toFloat64
// returns the value and true, otherwise 0 and false.
func ToFloat64(value any) (f float64, ok bool) {
	const maxFloat = (int64(1) << 53) - 1
	const minFloat = -maxFloat
	// If any other type is added here, remember to add it to the type switch in
	// the `span.SetTag` function to handle pointers to these supported types.
	switch i := value.(type) {
	case byte:
		return float64(i), true
	case float32:
		return float64(i), true
	case float64:
		return i, true
	case int:
		return float64(i), true
	case int8:
		return float64(i), true
	case int16:
		return float64(i), true
	case int32:
		return float64(i), true
	case int64:
		if i > maxFloat || i < minFloat {
			return 0, false
		}
		return float64(i), true
	case uint:
		return float64(i), true
	case uint16:
		return float64(i), true
	case uint32:
		return float64(i), true
	case uint64:
		if i > uint64(maxFloat) {
			return 0, false
		}
		return float64(i), true
	case samplernames.SamplerName:
		return float64(i), true
	default:
		return 0, false
	}
}

// ToInt64 attempts to convert an any value into an int64.
func ToInt64(val any) (int64, error) {
	switch v := val.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", val)
	}
}

// ToUint64 attempts to convert an any value into an uint64.
func ToUint64(val any) (uint64, error) {
	switch v := val.(type) {
	case uint:
		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint64:
		return v, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to uint64", val)
	}
}
