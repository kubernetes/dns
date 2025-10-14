// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

import (
	"cmp"
)

// Range is a type that represents a range of values.
type Range[T cmp.Ordered] struct {
	Min T
	Max T
}

// IsOrdered checks if the range is ordered. e.g. Min <= Max.
func (r Range[T]) IsOrdered() bool {
	return r.Min <= r.Max
}

// Contains checks if a value is within the range.
func (r Range[T]) Contains(value T) bool {
	return value >= r.Min && value <= r.Max
}

// Clamp squeezes a value between a minimum and maximum value.
func (r Range[T]) Clamp(value T) T {
	return max(min(r.Max, value), r.Min)
}

// ReduceMax returns a new range where value is the new max and min is either the current min or the new value to make sure the range is ordered.
func (r Range[T]) ReduceMax(value T) Range[T] {
	return Range[T]{
		Min: min(r.Min, value),
		Max: value,
	}
}
