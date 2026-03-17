package deadlock

import "sync"

type rlocker DeadlockRWMutex

func (r *rlocker) Lock()   { (*DeadlockRWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*DeadlockRWMutex)(r).RUnlock() }

// RLocker returns a Locker interface that implements
// the Lock and Unlock methods by calling RLock and RUnlock.
func (m *DeadlockRWMutex) RLocker() sync.Locker {
	return (*rlocker)(m)
}
