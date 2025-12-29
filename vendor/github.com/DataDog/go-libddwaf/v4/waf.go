// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libddwaf

import (
	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
)

// Load loads libddwaf's dynamic library. The dynamic library is opened only
// once by the first call to this function and internally stored globally.
// No function is currently provided in this API to unload it.
//
// This function is automatically called by [NewBuilder], and most users need
// not explicitly call it. It is however useful in order to explicitly check
// for the status of the WAF library's initialization.
//
// The function returns true when libddwaf was successfully loaded, along with
// an error value. An error might still be returned even though the WAF load was
// successful: in such cases the error is indicative that some non-critical
// features are not available; but the WAF may still be used.
func Load() (bool, error) {
	return bindings.Load()
}

// Version returns the version returned by libddwaf.
// It relies on the dynamic loading of the library, which can fail and return
// an empty string or the previously loaded version, if any.
func Version() string {
	return bindings.Version()
}

// Usable returns true if the WAF is usable, false and an error otherwise.
//
// If the WAF is usable, an error value may still be returned and should be
// treated as a warning (it is non-blocking).
//
// The following conditions are checked:
//   - The WAF library has been loaded successfully (you need to call [Load] first for this case to be
//     taken into account)
//   - The WAF library has not been manually disabled with the `datadog.no_waf` go build tag
//   - The WAF library is not in an unsupported OS/Arch
//   - The WAF library is not in an unsupported Go version
func Usable() (bool, error) {
	return bindings.Usable()
}
