// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

package options

import "github.com/DataDog/dd-trace-go/v2/internal"

// Copy should be used any time existing options are copied into
// a new locally scoped set of options. This is to avoid data races and
// accidental side effects.
func Copy[T any](opts []T) []T {
	return Expand(opts, 0, 0)
}

// Expand should be used any time existing options are copied into
// a new locally scoped set of options and extra space is required.
// This is to avoid data accidental side effects and undesired reallocations
// when appending to the new slice.
// The initialPosition parameter specifies the position in the new slice
// where the existing options should be copied to. It's assumed that the new
// slice will at least have a length of initialPosition + len(opts).
// The trailCapacity parameter specifies the number of additional options that may be
// appended to the new slice.
// The new slice will have a capacity of initialPosition + len(opts) + trailCapacity
// and a length of initialPosition + len(opts).
func Expand[T any](opts []T, initialPosition, trailCapacity int) []T {
	capacity := initialPosition + len(opts)
	dup := make([]T, capacity, capacity+trailCapacity)
	copy(dup[initialPosition:], opts)
	return dup
}

// This is a workaround needed because of v2 changes that prevents contribs from accessing
// the internal directory. This function should not be used if the internal directory
// can be
func GetBoolEnv(key string, def bool) bool {
	return internal.BoolEnv(key, def)
}
