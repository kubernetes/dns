package deadlock

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
)

func callers(skip int) (retv []uintptr) {
	tmp := make([]uintptr, 50)
	if n := runtime.Callers(2+skip, tmp); n > 0 {
		retv = make([]uintptr, n)
		copy(retv, tmp[:n])
	}
	return
}

func printStack(w io.Writer, stack []uintptr) {
	frames := runtime.CallersFrames(stack)
	var frame runtime.Frame
	more := true
	for more {
		frame, more = frames.Next()
		if strings.HasPrefix(frame.Function, "runtime.goexit") ||
			strings.HasPrefix(frame.Function, "testing.tRunner") {
			break
		}
		fmt.Fprintf(w, "  %s()\n", frame.Function)
		fmt.Fprintf(w, "      %s:%d +0x%x\n", frame.File, frame.Line, frame.PC-frame.Entry)
	}
	fmt.Fprintln(w)
}

var stackBufSize = int64(1024)

// Stacktraces for all goroutines.
func stacks() []byte {
	for {
		bufSize := atomic.LoadInt64(&stackBufSize)
		buf := make([]byte, bufSize)
		if n := runtime.Stack(buf, true); n < len(buf) {
			return buf[:n]
		}
		atomic.StoreInt64(&stackBufSize, bufSize*2)
	}
}
