// +build !debug

package mutexasserts

import "sync"

func AssertMutexLocked(m *sync.Mutex) {}

func AssertRWMutexLocked(m *sync.RWMutex) {}

func AssertRWMutexRLocked(m *sync.RWMutex) {}
