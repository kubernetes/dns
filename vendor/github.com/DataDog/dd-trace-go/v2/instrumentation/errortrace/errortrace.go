// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package errortrace

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

// TracerError is an error type that holds stackframes from when the error was thrown.
// It can be used interchangeably with the built-in Go error type.
type TracerError struct {
	stackFrames *runtime.Frames
	inner       error
	stack       *bytes.Buffer
}

// defaultStackLength specifies the default maximum size of a stack trace.
const defaultStackLength = 32

func (err *TracerError) Error() string {
	return err.inner.Error()
}

func New(text string) *TracerError {
	// Skip one to exclude New(...)
	return Wrap(errors.New(text))
}

// Wrap takes in an error and records the stack trace at the moment that it was thrown.
func Wrap(err error) *TracerError {
	return WrapN(err, 0, 1)
}

// WrapN takes in an error and records the stack trace at the moment that it was thrown.
// It will capture a maximum of `n` entries, skipping the first `skip` entries.
// If n is 0, it will capture up to 32 entries instead.
func WrapN(err error, n uint, skip uint) *TracerError {
	if err == nil {
		return nil
	}
	var e *TracerError
	if errors.As(err, &e) {
		return e
	}
	if n <= 0 {
		n = defaultStackLength
	}

	telemetry.Count(telemetry.NamespaceTracers, "errorstack.source", []string{"source:TracerError"}).Submit(1)
	now := time.Now()
	defer func() {
		dur := float64(time.Since(now))
		telemetry.Distribution(telemetry.NamespaceTracers, "errorstack.duration", []string{"source:TracerError"}).Submit(dur)
	}()

	pcs := make([]uintptr, n)
	var stackFrames *runtime.Frames
	// +2 to exclude runtime.Callers and Wrap
	numFrames := runtime.Callers(2+int(skip), pcs)
	if numFrames == 0 {
		stackFrames = nil
	} else {
		stackFrames = runtime.CallersFrames(pcs[:numFrames])
	}

	tracerErr := &TracerError{
		stackFrames: stackFrames,
		inner:       err,
	}
	return tracerErr
}

// Format returns a string representation of the stack trace.
func (err *TracerError) Format() string {
	if err == nil || err.stackFrames == nil {
		return ""
	}
	if err.stack != nil {
		return err.stack.String()
	}

	out := bytes.Buffer{}
	for i := 0; ; i++ {
		frame, more := err.stackFrames.Next()
		if i != 0 {
			out.WriteByte('\n')
		}
		out.WriteString(frame.Function)
		out.WriteByte('\n')
		out.WriteByte('\t')
		out.WriteString(frame.File)
		out.WriteByte(':')
		out.WriteString(strconv.Itoa(frame.Line))
		if !more {
			break
		}
	}
	// CallersFrames returns an iterator that is consumed as we read it. In order to
	// allow calling Format() multiple times, we save the result into err.stack, which can be
	// returned in future calls
	err.stack = &out
	return out.String()
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
