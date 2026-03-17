//go:build !nodeadlock && (deadlock || race)
// +build !nodeadlock
// +build deadlock race

package deadlock

// Mutex is deadlock.DeadlockMutex wrapper
type Mutex struct{ DeadlockMutex }

// RWMutex is deadlock.DeadlockRWMutex wrapper
type RWMutex struct{ DeadlockRWMutex }

// Enabled is true if deadlock checking is enabled
const Enabled = true
