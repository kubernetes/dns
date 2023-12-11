// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

import (
	"errors"
	"fmt"
	"sync"
)

// UnsupportedTargetError is a wrapper error type helping to handle the error
// case of trying to execute this package on an unsupported target environment.
type UnsupportedTargetError struct {
	error
}

// Unwrap the error and return it.
// Required by errors.Is and errors.As functions.
func (e *UnsupportedTargetError) Unwrap() error {
	return e.error
}

// RulesetInfo stores the information - provided by the WAF - about WAF rules initialization.
type RulesetInfo struct {
	// Number of rules successfully loaded
	Loaded uint16
	// Number of rules which failed to parse
	Failed uint16
	// Map from an error string to an array of all the rule ids for which
	// that error was raised. {error: [rule_ids]}
	Errors map[string][]string
	// Ruleset version
	Version string
}

// Encoder/Decoder errors
var (
	errMaxDepth          = errors.New("max depth reached")
	errUnsupportedValue  = errors.New("unsupported Go value")
	errInvalidMapKey     = errors.New("invalid WAF object map key")
	errNilObjectPtr      = errors.New("nil WAF object pointer")
	errInvalidObjectType = errors.New("invalid type encountered when decoding")
)

// RunError the WAF can return when running it.
type RunError int

// Errors the WAF can return when running it.
const (
	ErrInternal RunError = iota + 1
	ErrInvalidObject
	ErrInvalidArgument
	ErrTimeout
	ErrOutOfMemory
	ErrEmptyRuleAddresses
)

// Error returns the string representation of the RunError.
func (e RunError) Error() string {
	switch e {
	case ErrInternal:
		return "internal waf error"
	case ErrTimeout:
		return "waf timeout"
	case ErrInvalidObject:
		return "invalid waf object"
	case ErrInvalidArgument:
		return "invalid waf argument"
	case ErrOutOfMemory:
		return "out of memory"
	case ErrEmptyRuleAddresses:
		return "empty rule addresses"
	default:
		return fmt.Sprintf("unknown waf error %d", e)
	}
}

// Globally dlopen() libddwaf only once because several dlopens (eg. in tests)
// aren't supported by macOS.
var (
	// libddwaf's dynamic library handle and entrypoints
	wafLib *wafDl
	// libddwaf's dlopen error if any
	wafErr      error
	openWafOnce sync.Once
)

// Load loads libddwaf's dynamic library. The dynamic library is opened only
// once by the first call to this function and internally stored globally, and
// no function is currently provided in this API to close the opened handle.
// Calling this function is not mandatory and is automatically performed by
// calls to NewHandle, the entrypoint of libddwaf, but Load is useful in order
// to explicitly check libddwaf's general health where calling NewHandle doesn't
// necessarily apply nor is doable.
// The function returns ok when libddwaf was successfully loaded, along with a
// non-nil error if any. Note that both ok and err can be set, meaning that
// libddwaf is usable but some non-critical errors happened, such as failures
// to remove temporary files. It is safe to continue using libddwaf in such
// case.
func Load() (ok bool, err error) {
	openWafOnce.Do(func() {
		wafLib, wafErr = newWafDl()
		if wafErr != nil {
			return
		}
		wafVersion = wafLib.wafGetVersion()
	})

	return wafLib != nil, wafErr
}

// SupportsTarget returns true and a nil error when the target host environment
// is supported by this package and can be further used.
// Otherwise, it returns false along with an error detailing why.
func SupportsTarget() (bool, error) {
	return supportsTarget()
}

var wafVersion string

// Version returns the version returned by libddwaf.
// It relies on the dynamic loading of the library, which can fail and return
// an empty string or the previously loaded version, if any.
func Version() string {
	Load()
	return wafVersion
}
