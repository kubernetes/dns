// +build debug

package mutexasserts

import (
	"os"
	"runtime/debug"
	"sync"
)

// Note: we do `exit` instead of `panic` to prevent the app from handling it

// We do it this way, so we can test it
var exit func(code int) = os.Exit

func AssertMutexLocked(m *sync.Mutex) {
	if !MutexLocked(m) {
		os.Stderr.Write([]byte("AssertMutexLocked failed!"))
		debug.PrintStack()
		exit(1)
	}
}

func AssertRWMutexLocked(m *sync.RWMutex) {
	if !RWMutexLocked(m) {
		os.Stderr.Write([]byte("AssertRWMutexLocked failed!"))
		debug.PrintStack()
		exit(1)
	}
}

func AssertRWMutexRLocked(m *sync.RWMutex) {
	if !RWMutexRLocked(m) {
		os.Stderr.Write([]byte("AssertRWMutexRLocked failed!"))
		debug.PrintStack()
		exit(1)
	}
}
