//go:build go1.20
// +build go1.20

package mutexasserts

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// Starting in go1.20, readerCount is an atomic int32 value.
// See: https://go-review.googlesource.com/c/go/+/429767
func readerCount(rw *sync.RWMutex) int64 {
	// Look up the address of the readerCount field and use it to create a pointer to an atomic.Int32,
	// then load the value to return.
	rc := (*atomic.Int32)(reflect.ValueOf(rw).Elem().FieldByName("readerCount").Addr().UnsafePointer())
	return int64(rc.Load())
}
