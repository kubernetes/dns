// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

//go:build deadlock
// +build deadlock

package assert

import (
	"reflect"
	"sync"
	"unsafe"

	"github.com/trailofbits/go-mutexasserts"

	"github.com/DataDog/dd-trace-go/v2/internal/locking"
)

func MutexLocked(m *locking.Mutex) {
	v := reflect.ValueOf(m).Elem()
	muField := v.FieldByName("mu")
	if !muField.IsValid() {
		panic("could not find mu field in deadlock.Mutex")
	}
	muPtr := (*sync.Mutex)(unsafe.Pointer(muField.UnsafeAddr()))
	mutexasserts.AssertMutexLocked(muPtr)
}

func RWMutexLocked(m *locking.RWMutex) {
	v := reflect.ValueOf(m).Elem()
	muField := v.FieldByName("mu")
	if !muField.IsValid() {
		panic("could not find mu field in deadlock.RWMutex")
	}
	muPtr := (*sync.RWMutex)(unsafe.Pointer(muField.UnsafeAddr()))
	mutexasserts.AssertRWMutexLocked(muPtr)
}

func RWMutexRLocked(m *locking.RWMutex) {
	v := reflect.ValueOf(m).Elem()
	muField := v.FieldByName("mu")
	if !muField.IsValid() {
		panic("could not find mu field in deadlock.RWMutex")
	}
	muPtr := (*sync.RWMutex)(unsafe.Pointer(muField.UnsafeAddr()))
	// A write lock also satisfies the read lock requirement
	if mutexasserts.RWMutexRLocked(muPtr) || mutexasserts.RWMutexLocked(muPtr) {
		return
	}
	mutexasserts.AssertRWMutexRLocked(muPtr)
}
