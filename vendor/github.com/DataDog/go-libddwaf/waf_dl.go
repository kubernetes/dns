// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux || darwin) && (amd64 || arm64) && !go1.22

package waf

import (
	"fmt"
	"os"
)

// wafDl is the type wrapper for all C calls to the waf
// It uses `libwaf` to make C calls
// All calls must go through this one-liner to be type safe
// since purego calls are not type safe
type wafDl struct {
	libDl

	Ddwaf_ruleset_info_free  uintptr `dlsym:"ddwaf_ruleset_info_free"`
	Ddwaf_init               uintptr `dlsym:"ddwaf_init"`
	Ddwaf_destroy            uintptr `dlsym:"ddwaf_destroy"`
	Ddwaf_required_addresses uintptr `dlsym:"ddwaf_required_addresses"`
	Ddwaf_get_version        uintptr `dlsym:"ddwaf_get_version"`
	Ddwaf_context_init       uintptr `dlsym:"ddwaf_context_init"`
	Ddwaf_context_destroy    uintptr `dlsym:"ddwaf_context_destroy"`
	Ddwaf_result_free        uintptr `dlsym:"ddwaf_result_free"`
	Ddwaf_run                uintptr `dlsym:"ddwaf_run"`
}

func dumpWafLibrary() (*os.File, error) {
	file, err := os.CreateTemp("", "libddwaf-*.so")
	if err != nil {
		return nil, fmt.Errorf("Error creating temp file: %w", err)
	}

	if err := os.WriteFile(file.Name(), libddwaf, 0400); err != nil {
		return nil, fmt.Errorf("Error writing file: %w", err)
	}

	return file, nil
}

// newWafDl loads the libddwaf shared library along with all the needed symbols.
// The returned dynamic library handle dl can be non-nil even with a returned
// error, meaning that the dynamic library handle can be used but some errors
// happened in the last internal steps following the successful call to
// dlopen().
func newWafDl() (dl *wafDl, err error) {
	file, err := dumpWafLibrary()
	if err != nil {
		return nil, err
	}
	fName := file.Name()
	defer func() {
		rmErr := os.Remove(fName)
		if rmErr != nil {
			if err == nil {
				err = rmErr
			} else {
				// TODO: rely on errors.Join() once go1.20 is our min supported Go version
				err = fmt.Errorf("%w; along with an error while removing %s: %v", err, fName, rmErr)
			}
		}
	}()

	var waf wafDl
	if err := dlOpen(fName, &waf); err != nil {
		return nil, fmt.Errorf("error while opening libddwaf library at %s: %w", fName, err)
	}
	defer func() {
		closeErr := file.Close()
		if closeErr != nil {
			if err == nil {
				err = closeErr
			} else {
				// TODO: rely on errors.Join() once go1.20 is our min supported Go version
				err = fmt.Errorf("%w; along with an error while closing the shared libddwaf library file: %v", err, closeErr)
			}
		}
	}()

	// Try calling the waf to make sure everything is fine
	err = tryCall(func() error {
		waf.wafGetVersion()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &waf, nil
}

// wafGetVersion returned string is a static string so we do not need to free it
func (waf *wafDl) wafGetVersion() string {
	return gostring(cast[byte](waf.syscall(waf.Ddwaf_get_version)))
}

func (waf *wafDl) wafInit(obj *wafObject, config *wafConfig, info *wafRulesetInfo) wafHandle {
	handle := wafHandle(waf.syscall(waf.Ddwaf_init, ptrToUintptr(obj), ptrToUintptr(config), ptrToUintptr(info)))
	keepAlive(obj)
	keepAlive(config)
	keepAlive(info)
	return handle
}

func (waf *wafDl) wafRulesetInfoFree(info *wafRulesetInfo) {
	waf.syscall(waf.Ddwaf_ruleset_info_free, ptrToUintptr(info))
	keepAlive(info)
}

func (waf *wafDl) wafDestroy(handle wafHandle) {
	waf.syscall(waf.Ddwaf_destroy, uintptr(handle))
	keepAlive(handle)
}

// wafRequiredAddresses returns static strings so we do not need to free them
func (waf *wafDl) wafRequiredAddresses(handle wafHandle) []string {
	var nbAddresses uint32

	arrayVoidC := waf.syscall(waf.Ddwaf_required_addresses, uintptr(handle), ptrToUintptr(&nbAddresses))
	if arrayVoidC == 0 {
		return nil
	}

	addresses := make([]string, int(nbAddresses))
	for i := 0; i < int(nbAddresses); i++ {
		addresses[i] = gostring(*castWithOffset[*byte](arrayVoidC, uint64(i)))
	}

	keepAlive(&nbAddresses)
	keepAlive(handle)

	return addresses
}

func (waf *wafDl) wafContextInit(handle wafHandle) wafContext {
	ctx := wafContext(waf.syscall(waf.Ddwaf_context_init, uintptr(handle)))
	keepAlive(handle)
	return ctx
}

func (waf *wafDl) wafContextDestroy(context wafContext) {
	waf.syscall(waf.Ddwaf_context_destroy, uintptr(context))
	keepAlive(context)
}

func (waf *wafDl) wafResultFree(result *wafResult) {
	waf.syscall(waf.Ddwaf_result_free, ptrToUintptr(result))
	keepAlive(result)
}

func (waf *wafDl) wafRun(context wafContext, obj *wafObject, result *wafResult, timeout uint64) wafReturnCode {
	rc := wafReturnCode(waf.syscall(waf.Ddwaf_run, uintptr(context), ptrToUintptr(obj), ptrToUintptr(result), uintptr(timeout)))
	keepAlive(context)
	keepAlive(obj)
	keepAlive(result)
	keepAlive(timeout)
	return rc
}

// Implement SupportsTarget()
func supportsTarget() (bool, error) {
	return true, nil
}
