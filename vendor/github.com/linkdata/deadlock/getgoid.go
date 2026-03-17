package deadlock

import (
	"runtime"

	"github.com/petermattis/goid"
)

func init() {
	testGoid(getGoidFallback())
}

func getGoidFallback() int64 {
	var buf [64]byte
	return goid.ExtractGID(buf[:runtime.Stack(buf[:], false)])
}

func testGoid(slowId int64) {
	if goid.Get() != slowId {
		panic("github.com/petermattis/goid doesn't support this Go version, use '-tags=slowgoid'")
	}
}
