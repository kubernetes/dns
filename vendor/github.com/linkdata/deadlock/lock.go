package deadlock

import (
	"sync/atomic"
	"time"
)

func lock(tryLockFn func() bool, lockFn func(), curMtx interface{}) bool {
	gid := getGoid()
	curStack := callers(2)

	if lockFn != nil {
		if ms := atomic.LoadInt32(&maxMapSize); ms > 0 {
			lo.preLock(int(ms), gid, curStack, curMtx)
		}
	}

	if tryLockFn == nil || !tryLockFn() {
		if lockFn == nil {
			return false
		}
		if to := atomic.LoadInt32(&deadlockTimeout); to > 0 {
			ch := make(chan struct{})
			defer close(ch)
			go lo.timeoutFn(ch, time.Duration(to)*time.Millisecond, gid, curStack, curMtx)
		}
		lockFn()
	}

	lo.postLock(gid, curStack, curMtx)
	return true
}
