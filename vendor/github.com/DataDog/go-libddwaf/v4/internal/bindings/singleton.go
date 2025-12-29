// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bindings

import (
	"errors"
	"sync"

	"github.com/DataDog/go-libddwaf/v4/internal/support"
)

// Globally dlopen() libddwaf only once because several dlopens (eg. in tests)
// aren't supported by macOS.
var (
	// Lib is libddwaf's dynamic library handle and entrypoints. This is only safe to
	// read after calling [Load] or having acquired [gMu].
	Lib *WAFLib
	// libddwaf's dlopen error if any. This is only safe to read after calling
	// [Load] or having acquired [gMu].
	gWafLoadErr error
	// Protects the global variables above.
	gMu sync.Mutex

	openWafOnce sync.Once
)

// Load loads libddwaf's dynamic library. The dynamic library is opened only
// once by the first call to this function and internally stored globally.
// No function is currently provided in this API to unload it.
//
// This function is automatically called by [NewBuilder], and most users need
// not explicitly call it. It is however useful in order to explicitly check
// for the status of the Lib library's initialization.
//
// The function returns true when libddwaf was successfully loaded, along with
// an error value. An error might still be returned even though the Lib load was
// successful: in such cases the error is indicative that some non-critical
// features are not available; but the Lib may still be used.
func Load() (bool, error) {
	if ok, err := Usable(); !ok {
		return false, err
	}

	openWafOnce.Do(func() {
		// Acquire the global state mutex so we don't have a race condition with
		// [Usable] here.
		gMu.Lock()
		defer gMu.Unlock()

		Lib, gWafLoadErr = newWAFLib()
		if gWafLoadErr != nil {
			return
		}
		wafVersion = Lib.GetVersion()
	})

	return Lib != nil, gWafLoadErr
}

var wafVersion string

// Version returns the version returned by libddwaf.
// It relies on the dynamic loading of the library, which can fail and return
// an empty string or the previously loaded version, if any.
func Version() string {
	_, _ = Load()
	return wafVersion
}

// Usable returns true if the Lib is usable, false and an error otherwise.
//
// If the Lib is usable, an error value may still be returned and should be
// treated as a warning (it is non-blocking).
//
// The following conditions are checked:
//   - The Lib library has been loaded successfully (you need to call [Load] first for this case to be
//     taken into account)
//   - The Lib library has not been manually disabled with the `datadog.no_waf` go build tag
//   - The Lib library is not in an unsupported OS/Arch
//   - The Lib library is not in an unsupported Go version
func Usable() (bool, error) {
	wafSupportErrors := errors.Join(support.WafSupportErrors()...)
	wafManuallyDisabledErr := support.WafManuallyDisabledError()

	// Acquire the global state mutex as we are not calling [Load] here, so we
	// need to explicitly avoid a race condition with it.
	gMu.Lock()
	defer gMu.Unlock()
	return (Lib != nil || gWafLoadErr == nil) && wafSupportErrors == nil && wafManuallyDisabledErr == nil, errors.Join(gWafLoadErr, wafSupportErrors, wafManuallyDisabledErr)
}
