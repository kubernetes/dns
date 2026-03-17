//go:build !go1.20
// +build !go1.20

package mutexasserts

import (
	"reflect"
	"sync"
)

// Prior to go1.20, readerCount was an int value.
func readerCount(rw *sync.RWMutex) int64 {
	return reflect.ValueOf(rw).Elem().FieldByName("readerCount").Int()
}
