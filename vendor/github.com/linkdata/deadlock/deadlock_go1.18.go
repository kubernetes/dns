//go:build go1.18
// +build go1.18

package deadlock

import (
	"sync"
)

// A DeadlockMutex is a drop-in replacement for sync.Mutex.
type DeadlockMutex struct {
	mu sync.Mutex
}

// Lock locks the mutex.
// If the lock is already in use, the calling goroutine
// blocks until the mutex is available.
//
// Logs potential deadlocks to Opts.LogBuf,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *DeadlockMutex) Lock() {
	lock(m.mu.TryLock, m.mu.Lock, m)
}

func (m *DeadlockMutex) TryLock() bool {
	return lock(m.mu.TryLock, nil, m)
}

// Unlock unlocks the mutex.
// It is a run-time error if m is not locked on entry to Unlock.
//
// A locked Mutex is not associated with a particular goroutine.
// It is allowed for one goroutine to lock a Mutex and then
// arrange for another goroutine to unlock it.
func (m *DeadlockMutex) Unlock() {
	m.mu.Unlock()
	lo.postUnlock(m)
}

// An DeadlockRWMutex is a drop-in replacement for sync.RWMutex.
type DeadlockRWMutex struct {
	mu sync.RWMutex
}

// Lock locks rw for writing.
// If the lock is already locked for reading or writing,
// Lock blocks until the lock is available.
// To ensure that the lock eventually becomes available,
// a blocked Lock call excludes new readers from acquiring
// the lock.
//
// Logs potential deadlocks to Opts.LogBuf,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *DeadlockRWMutex) Lock() {
	lock(m.mu.TryLock, m.mu.Lock, m)
}

func (m *DeadlockRWMutex) TryLock() bool {
	return lock(m.mu.TryLock, nil, m)
}

// Unlock unlocks the mutex for writing.  It is a run-time error if rw is
// not locked for writing on entry to Unlock.
//
// As with Mutexes, a locked RWMutex is not associated with a particular
// goroutine.  One goroutine may RLock (Lock) an RWMutex and then
// arrange for another goroutine to RUnlock (Unlock) it.
func (m *DeadlockRWMutex) Unlock() {
	m.mu.Unlock()
	lo.postUnlock(m)
}

// RLock locks the mutex for reading.
//
// Logs potential deadlocks to Opts.LogBuf,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *DeadlockRWMutex) RLock() {
	lock(m.mu.TryRLock, m.mu.RLock, m)
}

func (m *DeadlockRWMutex) TryRLock() bool {
	return lock(m.mu.TryRLock, nil, m)
}

// RUnlock undoes a single RLock call;
// it does not affect other simultaneous readers.
// It is a run-time error if rw is not locked for reading
// on entry to RUnlock.
func (m *DeadlockRWMutex) RUnlock() {
	m.mu.RUnlock()
	lo.postUnlock(m)
}
