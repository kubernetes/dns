package deadlock

import (
	"bufio"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	// Waiting for a lock for longer than a non-zero DeadlockTimeout milliseconds is considered a deadlock.
	// Set to 30 seconds by default.
	DeadlockTimeout time.Duration
	// OnPotentialDeadlock is called each time a potential deadlock is detected -- either based on
	// lock order or on lock wait time. If nil, panics instead.
	OnPotentialDeadlock func()
	// Sets the maximum size of the map that tracks lock ordering.
	// Setting this to zero disables tracking of lock order. Default is a reasonable size.
	MaxMapSize int
	// Will dump stacktraces of all goroutines when inconsistent locking is detected.
	PrintAllCurrentGoroutines bool
	// Where to write reports, set to os.Stderr by default.
	LogBuf io.Writer
}

var optsLock sync.RWMutex
var maxMapSize int32 = 1024 * 64
var deadlockTimeout int32 = 30 * 1000

// Opts control how deadlock detection behaves.
// To safely read or change options during runtime, use Opts.ReadLocked() and Opts.WriteLocked()
var Opts = Options{
	DeadlockTimeout: time.Millisecond * time.Duration(deadlockTimeout),
	MaxMapSize:      int(maxMapSize),
	LogBuf:          os.Stderr,
}

// WriteLocked calls the given function with Opts locked for writing.
func (opts *Options) WriteLocked(fn func()) {
	optsLock.Lock()
	defer optsLock.Unlock()
	fn()
	atomic.StoreInt32(&maxMapSize, int32(opts.MaxMapSize))                                                 //#nosec G115
	atomic.StoreInt32(&deadlockTimeout, int32(opts.DeadlockTimeout.Nanoseconds()/int64(time.Millisecond))) //#nosec G115
}

// ReadLocked calls the given function with Opts locked for reading.
func (opts *Options) ReadLocked(fn func()) {
	optsLock.RLock()
	defer optsLock.RUnlock()
	fn()
}

// Write implements io.Writer for Options.
func (opts *Options) Write(b []byte) (int, error) {
	if opts.LogBuf != nil {
		return opts.LogBuf.Write(b)
	}
	return 0, nil
}

// Flush will flush the LogBuf if it is a *bufio.Writer
func (opts *Options) Flush() error {
	if opts.LogBuf != nil {
		if buf, ok := opts.LogBuf.(*bufio.Writer); ok {
			return buf.Flush()
		}
	}
	return nil
}

// PotentialDeadlock calls OnPotentialDeadlock if it is set, or panics if not.
func (opts *Options) PotentialDeadlock() {
	if opts.OnPotentialDeadlock == nil {
		panic("deadlock detected")
	}
	opts.OnPotentialDeadlock()
}
