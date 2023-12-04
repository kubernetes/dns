// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Purego only works on linux/macOS with amd64 and arm64 from now
//go:build (linux || darwin) && (amd64 || arm64) && !go1.22

package waf

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/ebitengine/purego"
)

// libDl is created to wraps all interactions with the purego library
type libDl struct {
	handle uintptr
}

// dlOpen open a handle for the shared library `name`.
// the libLoader object is only a wrapped type for the linker handle
// the `loader` parameter must have tags in the form of `dlsym:<symbol_name>`
// dlOpen will fill the object with symbols loaded from the library
// the struct of type `loader` must have a field of type `LibLoader`
// to be able to close the handle later
func dlOpen(name string, lib any) error {
	handle, err := purego.Dlopen(name, purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return fmt.Errorf("error opening shared library '%s'. Reason: %w", name, err)
	}

	return dlOpenFromHandle(handle, lib)
}

func dlOpenFromHandle(handle uintptr, lib any) error {
	foundHandle := false

	libValue := reflect.ValueOf(lib).Elem()
	libType := reflect.TypeOf(lib).Elem()
	dl := libDl{handle: handle}

	for i := 0; i < libValue.NumField(); i++ {
		fieldType := libType.Field(i)

		symbolName, ok := fieldType.Tag.Lookup("dlsym")
		if ok {
			symbol, err := purego.Dlsym(handle, symbolName)
			if err != nil {
				return fmt.Errorf("cannot load symbol '%s'. Reason: %w", symbolName, err)
			}

			libValue.Field(i).Set(reflect.ValueOf(symbol))
			continue
		}

		if fieldType.Type == reflect.TypeOf(dl) {
			// Bypass the fact the reflect package doesn't allow writing to private struct fields by directly writing to the field's memory address ourselves
			reflect.NewAt(reflect.TypeOf(dl), unsafe.Pointer(libValue.Field(i).UnsafeAddr())).Elem().Set(reflect.ValueOf(dl))
			foundHandle = true
		}
	}

	if !foundHandle {
		return fmt.Errorf("could not find `libLoader` embedding to set the library handle, cowardly refusing the handle to be lost")
	}

	return nil
}

// syscall is the only way to make C calls with this interface.
// purego implementation limits the number of arguments to 9, it will panic if more are provided
// Note: `purego.SyscallN` has 3 return values: these are the following:
//
//	1st - The return value is a pointer or a int of any type
//	2nd - The return value is a float
//	3rd - The value of `errno` at the end of the call
func (lib *libDl) syscall(fn uintptr, args ...uintptr) uintptr {
	ret, _, _ := purego.SyscallN(fn, args...)
	return ret
}

func (lib *libDl) Close() error {
	return purego.Dlclose(lib.handle)
}
