// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

//go:build !deadlock
// +build !deadlock

package locking

import "sync"

// Mutex is a type alias for sync.Mutex when not building with deadlock detection.
// Using a type alias preserves all methods and allows static checkers to work properly.
type Mutex = sync.Mutex

// RWMutex is a type alias for sync.RWMutex when not building with deadlock detection.
// Using a type alias preserves all methods and allows static checkers to work properly.
type RWMutex = sync.RWMutex
