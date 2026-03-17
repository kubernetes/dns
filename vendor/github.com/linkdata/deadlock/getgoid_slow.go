//go:build slowgoid
// +build slowgoid

package deadlock

func getGoid() int64 {
	return getGoidFallback()
}
