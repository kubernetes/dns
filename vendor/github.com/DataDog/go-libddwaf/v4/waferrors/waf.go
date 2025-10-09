// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waferrors

import (
	"errors"
	"fmt"
)

var (
	// ErrContextClosed is returned when an operation is attempted on a
	// [github.com/DataDog/go-libddwaf/v4.Context] that has already been closed.
	ErrContextClosed = errors.New("closed WAF context")

	// ErrMaxDepthExceeded is returned when the WAF encounters a value that
	// exceeds the maximum depth.
	ErrMaxDepthExceeded = errors.New("max depth exceeded")
	// ErrUnsupportedValue is returned when the WAF encounters a value that
	// is not supported by the encoder or decoder.
	ErrUnsupportedValue = errors.New("unsupported Go value")
	// ErrInvalidMapKey is returned when the WAF encounters an invalid map key.
	ErrInvalidMapKey = errors.New("invalid WAF object map key")
	// ErrNilObjectPtr is returned when the WAF encounters a nil object pointer at
	// an unexpected location.
	ErrNilObjectPtr = errors.New("nil WAF object pointer")
	// ErrInvalidObjectType is returned when the WAF encounters an invalid type
	// when decoding a value.
	ErrInvalidObjectType = errors.New("invalid type encountered when decoding")
	// ErrTooManyIndirections is returned when the WAF encounters a value that
	// exceeds the maximum number of indirections (pointer to pointer to...).
	ErrTooManyIndirections = errors.New("too many indirections")
)

// RunError the WAF can return when running it.
type RunError int

// Errors the WAF can return when running it.
const (
	// ErrInternal denotes a WAF internal error.
	ErrInternal RunError = iota + 1
	// ErrInvalidObject is returned when the WAF received an invalid object.
	ErrInvalidObject
	// ErrInvalidArgument is returned when the WAF received an invalid argument.
	ErrInvalidArgument
	// ErrTimeout is returned when the WAF ran out of time budget to spend.
	ErrTimeout
	// ErrOutOfMemory is returned when the WAF ran out of memory when trying to
	// allocate a result object.
	ErrOutOfMemory
	// ErrEmptyRuleAddresses is returned when the WAF received an empty list of
	// rule addresses.
	ErrEmptyRuleAddresses
)

var errorStrMap = map[RunError]string{
	ErrInternal:           "internal waf error",
	ErrInvalidObject:      "invalid waf object",
	ErrInvalidArgument:    "invalid waf argument",
	ErrTimeout:            "waf timeout",
	ErrOutOfMemory:        "out of memory",
	ErrEmptyRuleAddresses: "empty rule addresses",
}

// Error returns the string representation of the [RunError].
func (e RunError) Error() string {
	description, ok := errorStrMap[e]
	if !ok {
		return fmt.Sprintf("unknown waf error %d", e)
	}

	return description
}

// ToWafErrorCode converts an error to a WAF error code, returns zero if the
// error is not a [RunError].
func ToWafErrorCode(in error) int {
	var runError RunError
	if !errors.As(in, &runError) {
		return 0
	}
	return int(runError)
}

// PanicError is an error type wrapping a recovered panic value that happened
// during a function call. Such error must be considered unrecoverable and be
// used to try to gracefully abort. Keeping using this package after such an
// error is unreliable and the caller must rather stop using the library.
// Examples include safety checks errors.
type PanicError struct {
	// The recovered panic error while executing the function `in`.
	Err error
	// The function symbol name that was given to `tryCall()`.
	In string
}

// Unwrap the error and return it.
// Required by errors.Is and errors.As functions.
func (e *PanicError) Unwrap() error {
	return e.Err
}

// Error returns the error string representation.
func (e *PanicError) Error() string {
	return fmt.Sprintf("panic while executing %s: %#+v", e.In, e.Err)
}
