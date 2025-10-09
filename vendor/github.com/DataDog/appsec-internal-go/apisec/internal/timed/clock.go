// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package timed

import "time"

type (
	ClockFunc = func() int64

	// biasedClock is a specialized clock implementation used to ensure we can get
	// 32-bit wide timestamps without having to worry about wraparound.
	biasedClock struct {
		// clock is the underlying clock, returning a timestamp in seconds.
		clock ClockFunc
		// bias is effectively the time at which the biasedClock was initialized.
		bias int64
	}
)

// newBiasedClock creates a new [biasedClock] with the given clock function and
// horizon.
func newBiasedClock(clock ClockFunc, horizon uint32) biasedClock {
	return biasedClock{
		clock: clock,
		bias:  clock() - int64(horizon),
	}
}

// Now returns the current timestamp, relative to this [biasedClock].
func (c *biasedClock) Now() uint32 {
	// We clamp it to [0,) to be absolutely safe...
	return uint32(max(0, c.clock()-c.bias))
}

// UnixTime is a [ClockFunc] that returns the current Unix time (seconds since
// the Unix epoch).
func UnixTime() int64 {
	return time.Now().Unix()
}
