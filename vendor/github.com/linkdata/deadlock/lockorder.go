package deadlock

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/petermattis/goid"
)

const header = "POTENTIAL DEADLOCK:"

type lockOrder struct {
	mu    sync.Mutex                          // protects following
	cur   map[interface{}]stackGID            // stacktraces + gids for the locks currently taken.
	order map[beforeAfterMtx]beforeAfterStack // expected order of locks.
}

type stackGID struct {
	stack []uintptr
	gid   int64
}

type beforeAfterMtx struct {
	beforeMtx interface{}
	afterMtx  interface{}
}

type beforeAfterStack struct {
	beforeStack []uintptr
	afterStack  []uintptr
}

var lo = newLockOrder()

func newLockOrder() (lo *lockOrder) {
	lo = &lockOrder{
		cur:   map[interface{}]stackGID{},
		order: map[beforeAfterMtx]beforeAfterStack{},
	}
	return
}

func (l *lockOrder) postLock(gid int64, curStack []uintptr, curMtx interface{}) {
	l.mu.Lock()
	l.cur[curMtx] = stackGID{curStack, gid}
	l.mu.Unlock()
}

func (l *lockOrder) preLock(maxMapSize int, gid int64, curStack []uintptr, curMtx interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Reset the map to keep memory footprint bounded
	if len(l.order) >= maxMapSize {
		// This gets optimized to calling runtime.mapclear()
		for k := range l.order {
			delete(l.order, k)
		}
	}

	for otherMtx, otherStackGID := range l.cur {
		if otherMtx == curMtx {
			if otherStackGID.gid == gid {
				fmt.Fprintln(&Opts, header, "Recursive locking:")
				fmt.Fprintf(&Opts, "goroutine %d lock %p:\n", gid, otherMtx)
				printStack(&Opts, curStack)
				fmt.Fprintln(&Opts, "same goroutine previously locked it from:")
				printStack(&Opts, otherStackGID.stack)
				l.otherLocked(curMtx)
				_ = Opts.Flush()
				Opts.PotentialDeadlock()
			}
			continue
		}
		if otherStackGID.gid != gid { // We want locks taken in the same goroutine only.
			continue
		}
		if otherStacks, ok := l.order[beforeAfterMtx{curMtx, otherMtx}]; ok {
			fmt.Fprintln(&Opts, header, "Inconsistent locking:")
			fmt.Fprintln(&Opts, "in one goroutine: happened before")
			printStack(&Opts, otherStacks.beforeStack)
			fmt.Fprintln(&Opts, "happened after")
			printStack(&Opts, otherStacks.afterStack)

			fmt.Fprintln(&Opts, "in another goroutine: happened before")
			printStack(&Opts, otherStackGID.stack)
			fmt.Fprintln(&Opts, "happened after")
			printStack(&Opts, curStack)
			l.otherLocked(curMtx)
			fmt.Fprintln(&Opts)
			_ = Opts.Flush()
			Opts.PotentialDeadlock()
		}

		l.order[beforeAfterMtx{otherMtx, curMtx}] = beforeAfterStack{otherStackGID.stack, curStack}
	}
}

func (l *lockOrder) postUnlock(curMtx interface{}) {
	l.mu.Lock()
	delete(l.cur, curMtx)
	l.mu.Unlock()
}

func (l *lockOrder) timeoutFn(ch <-chan struct{}, timeout time.Duration, gid int64, curStack []uintptr, curMtx interface{}) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-t.C:
		fmt.Fprintln(&Opts, header)
		fmt.Fprintf(&Opts, "goroutine %v have been trying to lock %p for more than %v:\n",
			gid, curMtx, &Opts.DeadlockTimeout)
		printStack(&Opts, curStack)

		curStacks := stacks()

		func() {
			lo.mu.Lock()
			defer lo.mu.Unlock()
			if prev, ok := lo.cur[curMtx]; ok {
				fmt.Fprintf(&Opts, "goroutine %v previously locked it from:\n", prev.gid)
				printStack(&Opts, prev.stack)
				goroutineStackList := bytes.Split(curStacks, []byte("\n\n"))
				for _, goroutineStack := range goroutineStackList {
					if goid.ExtractGID(goroutineStack) == prev.gid {
						fmt.Fprintf(&Opts, "goroutine %v current stack:\n", prev.gid)
						_, _ = Opts.Write(goroutineStack)
						fmt.Fprintln(&Opts)
					}
				}
			}
			lo.otherLocked(curMtx)
		}()

		if Opts.PrintAllCurrentGoroutines {
			fmt.Fprintln(&Opts, "All current goroutines:")
			_, _ = Opts.Write(curStacks)
		}

		fmt.Fprintln(&Opts)
		_ = Opts.Flush()
		Opts.PotentialDeadlock()
		<-ch
	case <-ch:
	}
}

func (l *lockOrder) otherLocked(curMtx interface{}) {
	printedHeader := false
	for otherMtx, otherStackGID := range l.cur {
		if otherMtx != curMtx {
			if !printedHeader {
				printedHeader = true
				fmt.Fprintln(&Opts, "Other goroutines holding locks:")
			}
			fmt.Fprintf(&Opts, "goroutine %v lock %p\n", otherStackGID.gid, otherMtx)
			printStack(&Opts, otherStackGID.stack)
		}
	}
	if printedHeader {
		fmt.Fprintln(&Opts)
	}
}
