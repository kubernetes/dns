// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux || darwin) && (amd64 || arm64) && !go1.26 && !datadog.no_waf && (cgo || appsec)

package bindings

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/DataDog/go-libddwaf/v4/internal/lib"
	"github.com/DataDog/go-libddwaf/v4/internal/log"
	"github.com/DataDog/go-libddwaf/v4/internal/unsafe"
	"github.com/ebitengine/purego"
)

// WAFLib is the type wrapper for all C calls to the waf
// It uses `libwaf` to make C calls
// All calls must go through this one-liner to be type safe
// since purego calls are not type safe
type WAFLib struct {
	wafSymbols
	handle uintptr
}

// NewWAFLib loads the libddwaf shared library and resolves all tge relevant symbols.
// The caller is responsible for calling wafDl.Close on the returned object once they
// are done with it so that associated resources can be released.
func NewWAFLib() (dl *WAFLib, err error) {
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
	if symbols, err = newWafSymbols(handle); err != nil {
		if closeErr := purego.Dlclose(handle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("error released the shared libddwaf library: %w", closeErr))
		}
		return
	}

	dl = &WAFLib{symbols, handle}

	// Try calling the waf to make sure everything is fine
	if _, err = tryCall(dl.GetVersion); err != nil {
		if closeErr := purego.Dlclose(handle); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("error released the shared libddwaf library: %w", closeErr))
		}
		return
	}

	if val := os.Getenv(log.EnvVarLogLevel); val != "" {
		logLevel := log.LevelNamed(val)
		if logLevel != log.LevelOff {
			dl.SetLogCb(log.CallbackFunctionPointer(), logLevel)
		}
	}

	return
}

func (waf *WAFLib) Close() error {
	return purego.Dlclose(waf.handle)
}

// GetVersion returned string is a static string so we do not need to free it
func (waf *WAFLib) GetVersion() string {
	return unsafe.Gostring(unsafe.Cast[byte](waf.syscall(waf.getVersion)))
}

// BuilderInit initializes a new WAF builder with the provided configuration,
// which may be nil. Returns nil in case of an error.
func (waf *WAFLib) BuilderInit(cfg *WAFConfig) WAFBuilder {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(cfg)

	return WAFBuilder(waf.syscall(waf.builderInit, unsafe.PtrToUintptr(cfg)))
}

// BuilderAddOrUpdateConfig adds or updates a configuration based on the
// given path, which must be a unique identifier for the provided configuration.
// Returns false in case of an error.
func (waf *WAFLib) BuilderAddOrUpdateConfig(builder WAFBuilder, path string, config *WAFObject, diags *WAFObject) bool {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(config)
	pinner.Pin(diags)

	res := waf.syscall(waf.builderAddOrUpdateConfig,
		uintptr(builder),
		unsafe.PtrToUintptr(unsafe.Cstring(&pinner, path)),
		uintptr(len(path)),
		unsafe.PtrToUintptr(config),
		unsafe.PtrToUintptr(diags),
	)
	return byte(res) != 0
}

// BuilderRemoveConfig removes a configuration based on the provided path.
// Returns false in case of an error.
func (waf *WAFLib) BuilderRemoveConfig(builder WAFBuilder, path string) bool {
	var pinner runtime.Pinner
	defer pinner.Unpin()

	return byte(waf.syscall(waf.builderRemoveConfig,
		uintptr(builder),
		unsafe.PtrToUintptr(unsafe.Cstring(&pinner, path)),
		uintptr(len(path)),
	)) != 0
}

// BuilderBuildInstance builds a WAF instance based on the current set of configurations.
// Returns nil in case of an error.
func (waf *WAFLib) BuilderBuildInstance(builder WAFBuilder) WAFHandle {
	return WAFHandle(waf.syscall(waf.builderBuildInstance, uintptr(builder)))
}

// BuilderGetConfigPaths returns the list of currently loaded paths.
// Returns nil in case of an error.
func (waf *WAFLib) BuilderGetConfigPaths(builder WAFBuilder, filter string) []string {
	var paths WAFObject
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&filter)
	pinner.Pin(&paths)

	count := waf.syscall(waf.builderGetConfigPaths,
		uintptr(builder),
		unsafe.PtrToUintptr(&paths),
		unsafe.PtrToUintptr(unsafe.StringData(filter)),
		uintptr(len(filter)),
	)
	defer waf.ObjectFree(&paths)

	list := make([]string, 0, count)
	for i := range uint64(count) {
		obj := unsafe.CastWithOffset[WAFObject](paths.Value, i)
		path := unsafe.GostringSized(unsafe.Cast[byte](obj.Value), obj.NbEntries)
		list = append(list, path)
	}
	return list
}

// BuilderDestroy destroys a WAF builder instance.
func (waf *WAFLib) BuilderDestroy(builder WAFBuilder) {
	waf.syscall(waf.builderDestroy, uintptr(builder))
}

// SetLogCb sets the log callback function for the WAF.
func (waf *WAFLib) SetLogCb(cb uintptr, level log.Level) {
	waf.syscall(waf.setLogCb, cb, uintptr(level))
}

// Destroy destroys a WAF instance.
func (waf *WAFLib) Destroy(handle WAFHandle) {
	waf.syscall(waf.destroy, uintptr(handle))
}

func (waf *WAFLib) KnownAddresses(handle WAFHandle) []string {
	return waf.knownX(handle, waf.knownAddresses)
}

func (waf *WAFLib) KnownActions(handle WAFHandle) []string {
	return waf.knownX(handle, waf.knownActions)
}

func (waf *WAFLib) knownX(handle WAFHandle, symbol uintptr) []string {
	var nbAddresses uint32

	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(&nbAddresses)

	arrayVoidC := waf.syscall(symbol, uintptr(handle), unsafe.PtrToUintptr(&nbAddresses))
	if arrayVoidC == 0 {
		return nil
	}

	if nbAddresses == 0 {
		return nil
	}

	// These C strings are static strings so we do not need to free them
	addresses := make([]string, int(nbAddresses))
	for i := 0; i < int(nbAddresses); i++ {
		addresses[i] = unsafe.Gostring(*unsafe.CastWithOffset[*byte](arrayVoidC, uint64(i)))
	}

	return addresses
}

func (waf *WAFLib) ContextInit(handle WAFHandle) WAFContext {
	return WAFContext(waf.syscall(waf.contextInit, uintptr(handle)))
}

func (waf *WAFLib) ContextDestroy(context WAFContext) {
	waf.syscall(waf.contextDestroy, uintptr(context))
}

func (waf *WAFLib) ObjectFree(obj *WAFObject) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(obj)

	waf.syscall(waf.objectFree, unsafe.PtrToUintptr(obj))
}

func (waf *WAFLib) Run(context WAFContext, persistentData, ephemeralData *WAFObject, result *WAFObject, timeout uint64) WAFReturnCode {
	var pinner runtime.Pinner
	defer pinner.Unpin()

	pinner.Pin(persistentData)
	pinner.Pin(ephemeralData)
	pinner.Pin(result)

	return WAFReturnCode(waf.syscall(waf.run, uintptr(context), unsafe.PtrToUintptr(persistentData), unsafe.PtrToUintptr(ephemeralData), unsafe.PtrToUintptr(result), uintptr(timeout)))
}

func (waf *WAFLib) Handle() uintptr {
	return waf.handle
}

// syscall is the only way to make C calls with this interface.
// purego implementation limits the number of arguments to 9, it will panic if more are provided
// Note: `purego.SyscallN` has 3 return values: these are the following:
//
//	1st - The return value is a pointer or a int of any type
//	2nd - The return value is a float
//	3rd - The value of `errno` at the end of the call
func (waf *WAFLib) syscall(fn uintptr, args ...uintptr) uintptr {
	ret, _, _ := purego.SyscallN(fn, args...)
	return ret
}
