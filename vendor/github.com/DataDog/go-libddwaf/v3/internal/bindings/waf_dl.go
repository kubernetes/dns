// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux || darwin) && (amd64 || arm64) && !go1.24 && !datadog.no_waf && (cgo || appsec)

package bindings

import (
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/go-libddwaf/v3/internal/lib"
	"github.com/DataDog/go-libddwaf/v3/internal/log"
	"github.com/DataDog/go-libddwaf/v3/internal/unsafe"
	"github.com/ebitengine/purego"
)

// WafDl is the type wrapper for all C calls to the waf
// It uses `libwaf` to make C calls
// All calls must go through this one-liner to be type safe
// since purego calls are not type safe
type WafDl struct {
	wafSymbols
	handle uintptr
}

type wafSymbols struct {
	init           uintptr
	update         uintptr
	destroy        uintptr
	knownAddresses uintptr
	knownActions   uintptr
	getVersion     uintptr
	contextInit    uintptr
	contextDestroy uintptr
	objectFree     uintptr
	resultFree     uintptr
	run            uintptr
}

// NewWafDl loads the libddwaf shared library and resolves all tge relevant symbols.
// The caller is responsible for calling wafDl.Close on the returned object once they
// are done with it so that associated resources can be released.
func NewWafDl() (dl *WafDl, err error) {
	path, closer, err := lib.DumpEmbeddedWAF()
	if err != nil {
		return nil, fmt.Errorf("dump embedded WAF: %w", err)
	}
	defer func() {
		if rmErr := closer(); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("error removing %s: %w", path, rmErr))
		}
	}()

	var handle uintptr
	if handle, err = purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		return nil, fmt.Errorf("load a dynamic library file: %w", err)
	}

	var symbols wafSymbols
	if symbols, err = resolveWafSymbols(handle); err != nil {
		if closeErr := purego.Dlclose(handle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("error released the shared libddwaf library: %w", closeErr))
		}
		return
	}

	dl = &WafDl{symbols, handle}

	// Try calling the waf to make sure everything is fine
	if _, err = tryCall(dl.WafGetVersion); err != nil {
		if closeErr := purego.Dlclose(handle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("error released the shared libddwaf library: %w", closeErr))
		}
		return
	}

	if val := os.Getenv(log.EnvVarLogLevel); val != "" {
		setLogSym, symErr := purego.Dlsym(handle, "ddwaf_set_log_cb")
		if symErr != nil {
			return nil, fmt.Errorf("get symbol: %w", symErr)
		}
		logLevel := log.LevelNamed(val)
		dl.syscall(setLogSym, log.CallbackFunctionPointer(), uintptr(logLevel))
	}

	return
}

func (waf *WafDl) Close() error {
	return purego.Dlclose(waf.handle)
}

// WafGetVersion returned string is a static string so we do not need to free it
func (waf *WafDl) WafGetVersion() string {
	return unsafe.Gostring(unsafe.Cast[byte](waf.syscall(waf.getVersion)))
}

// WafInit initializes a new WAF with the provided ruleset, configuration and info objects. A
// cgoRefPool ensures that the provided input values are not moved or garbage collected by the Go
// runtime during the WAF call.
func (waf *WafDl) WafInit(ruleset *WafObject, config *WafConfig, info *WafObject) WafHandle {
	handle := WafHandle(waf.syscall(waf.init, unsafe.PtrToUintptr(ruleset), unsafe.PtrToUintptr(config), unsafe.PtrToUintptr(info)))
	unsafe.KeepAlive(ruleset)
	unsafe.KeepAlive(config)
	unsafe.KeepAlive(info)
	return handle
}

func (waf *WafDl) WafUpdate(handle WafHandle, ruleset *WafObject, info *WafObject) WafHandle {
	newHandle := WafHandle(waf.syscall(waf.update, uintptr(handle), unsafe.PtrToUintptr(ruleset), unsafe.PtrToUintptr(info)))
	unsafe.KeepAlive(ruleset)
	unsafe.KeepAlive(info)
	return newHandle
}

func (waf *WafDl) WafDestroy(handle WafHandle) {
	waf.syscall(waf.destroy, uintptr(handle))
	unsafe.KeepAlive(handle)
}

func (waf *WafDl) wafKnownX(handle WafHandle, symbol uintptr) []string {
	var nbAddresses uint32

	arrayVoidC := waf.syscall(symbol, uintptr(handle), unsafe.PtrToUintptr(&nbAddresses))
	if arrayVoidC == 0 {
		return nil
	}

	// These C strings are static strings so we do not need to free them
	addresses := make([]string, int(nbAddresses))
	for i := 0; i < int(nbAddresses); i++ {
		addresses[i] = unsafe.Gostring(*unsafe.CastWithOffset[*byte](arrayVoidC, uint64(i)))
	}

	unsafe.KeepAlive(&nbAddresses)
	unsafe.KeepAlive(handle)

	return addresses
}

func (waf *WafDl) WafKnownAddresses(handle WafHandle) []string {
	return waf.wafKnownX(handle, waf.knownAddresses)
}

func (waf *WafDl) WafKnownActions(handle WafHandle) []string {
	return waf.wafKnownX(handle, waf.knownActions)
}

func (waf *WafDl) WafContextInit(handle WafHandle) WafContext {
	ctx := WafContext(waf.syscall(waf.contextInit, uintptr(handle)))
	unsafe.KeepAlive(handle)
	return ctx
}

func (waf *WafDl) WafContextDestroy(context WafContext) {
	waf.syscall(waf.contextDestroy, uintptr(context))
	unsafe.KeepAlive(context)
}

func (waf *WafDl) WafResultFree(result *WafResult) {
	waf.syscall(waf.resultFree, unsafe.PtrToUintptr(result))
	unsafe.KeepAlive(result)
}

func (waf *WafDl) WafObjectFree(obj *WafObject) {
	waf.syscall(waf.objectFree, unsafe.PtrToUintptr(obj))
	unsafe.KeepAlive(obj)
}

func (waf *WafDl) WafRun(context WafContext, persistentData, ephemeralData *WafObject, result *WafResult, timeout uint64) WafReturnCode {
	rc := WafReturnCode(waf.syscall(waf.run, uintptr(context), unsafe.PtrToUintptr(persistentData), unsafe.PtrToUintptr(ephemeralData), unsafe.PtrToUintptr(result), uintptr(timeout)))
	unsafe.KeepAlive(context)
	unsafe.KeepAlive(persistentData)
	unsafe.KeepAlive(ephemeralData)
	unsafe.KeepAlive(result)
	unsafe.KeepAlive(timeout)
	return rc
}

func (waf *WafDl) Handle() uintptr {
	return waf.handle
}

// syscall is the only way to make C calls with this interface.
// purego implementation limits the number of arguments to 9, it will panic if more are provided
// Note: `purego.SyscallN` has 3 return values: these are the following:
//
//	1st - The return value is a pointer or a int of any type
//	2nd - The return value is a float
//	3rd - The value of `errno` at the end of the call
func (waf *WafDl) syscall(fn uintptr, args ...uintptr) uintptr {
	ret, _, _ := purego.SyscallN(fn, args...)
	return ret
}

// resolveWafSymbols resolves relevant symbols from the libddwaf shared library using the provided
// purego.Dlopen handle.
func resolveWafSymbols(handle uintptr) (symbols wafSymbols, err error) {
	if symbols.init, err = purego.Dlsym(handle, "ddwaf_init"); err != nil {
		return
	}
	if symbols.update, err = purego.Dlsym(handle, "ddwaf_update"); err != nil {
		return
	}
	if symbols.destroy, err = purego.Dlsym(handle, "ddwaf_destroy"); err != nil {
		return
	}
	if symbols.knownAddresses, err = purego.Dlsym(handle, "ddwaf_known_addresses"); err != nil {
		return
	}
	if symbols.knownActions, err = purego.Dlsym(handle, "ddwaf_known_actions"); err != nil {
		return
	}
	if symbols.getVersion, err = purego.Dlsym(handle, "ddwaf_get_version"); err != nil {
		return
	}
	if symbols.contextInit, err = purego.Dlsym(handle, "ddwaf_context_init"); err != nil {
		return
	}
	if symbols.contextDestroy, err = purego.Dlsym(handle, "ddwaf_context_destroy"); err != nil {
		return
	}
	if symbols.resultFree, err = purego.Dlsym(handle, "ddwaf_result_free"); err != nil {
		return
	}
	if symbols.objectFree, err = purego.Dlsym(handle, "ddwaf_object_free"); err != nil {
		return
	}
	if symbols.run, err = purego.Dlsym(handle, "ddwaf_run"); err != nil {
		return
	}

	return
}
