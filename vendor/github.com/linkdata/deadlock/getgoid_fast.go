//go:build !slowgoid
// +build !slowgoid

package deadlock

import (
	"github.com/petermattis/goid"
)

func getGoid() int64 {
	return goid.Get()
}
