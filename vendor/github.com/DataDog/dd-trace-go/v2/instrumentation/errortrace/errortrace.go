// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package errortrace

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/stacktrace"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

// TracerError is an error type that holds stackframes from when the error was thrown.
// It can be used interchangeably with the built-in Go error type.
type TracerError struct {
	rawStack stacktrace.RawStackTrace

	inner error
}

func (err *TracerError) Error() string {
	return err.inner.Error()
}

func New(text string) *TracerError {
	// Skip one to exclude New(...)
	return Wrap(errors.New(text))
}

// Wrap takes in an error and records the stack trace at the moment that it was thrown.
func Wrap(err error) *TracerError {
	return WrapN(err, 1)
}

// WrapN takes in an error and records the stack trace at the moment that it was thrown.
// Note: The n parameter is ignored; internal/stacktrace uses its own default depth.
// The skip parameter specifies how many stack frames to skip before capturing.
func WrapN(err error, skip uint) *TracerError {
	if err == nil {
		return nil
	}
	var e *TracerError
	if errors.As(err, &e) {
		return e
	}

	telemetry.Count(telemetry.NamespaceTracers, "errorstack.source", []string{"source:TracerError"}).Submit(1)
	now := time.Now()
	defer func() {
		dur := float64(time.Since(now))
		telemetry.Distribution(telemetry.NamespaceTracers, "errorstack.duration", []string{"source:TracerError"}).Submit(dur)
	}()

	// Use SkipAndCaptureUnfiltered to capture all frames including internal DD frames.
	// +4 to account for: runtime.Callers, iterator, SkipAndCaptureUnfiltered, and this WrapN function
	stack := stacktrace.CaptureRaw(int(skip) + 2)

	tracerErr := &TracerError{
		rawStack: stack,
		inner:    err,
	}
	return tracerErr
}

// Format returns a string representation of the stack trace.
// Uses the centralized internal/stacktrace formatting.
func (err *TracerError) Format() string {
	if err == nil {
		return ""
	}
	return stacktrace.Format(err.rawStack.Symbolicate())
}

// Errorf serves the same purpose as fmt.Errorf, but returns a TracerError
// and prevents wrapping errors of type TracerError twice.
// The %w flag will only wrap errors if they are not already of type *TracerError.
func Errorf(format string, a ...any) *TracerError {
	switch len(a) {
	case 0:
		return New(format)
	case 1:
		if _, ok := a[0].(*TracerError); ok {
			format = strings.Replace(format, "%w", "%v", 1)
		}
	default:
		aIndex := 0
		var newFormat strings.Builder
		for i := 0; i < len(format); i++ {
			c := format[i]
			newFormat.WriteByte(c)
			if c != '%' {
				continue
			}
			if i+1 >= len(format) {
				break
			}
			if format[i+1] == '%' {
				continue
			}
			if format[i+1] == 'w' {
				if _, ok := a[aIndex].(*TracerError); ok {
					newFormat.WriteString("v")
					i++
				}
			}
			aIndex++
		}
		format = newFormat.String()
	}
	err := fmt.Errorf(format, a...)
	return Wrap(err)
}

// Unwrap takes a wrapped error and returns the inner error.
func (err *TracerError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.inner
}
