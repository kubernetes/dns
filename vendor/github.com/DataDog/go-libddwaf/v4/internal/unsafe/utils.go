// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package unsafe

import (
	"runtime"
	"unsafe"
)

type Pointer = unsafe.Pointer

func SliceData[E any, T ~[]E](slice T) *E {
	return unsafe.SliceData(slice)
}

func StringData(str string) *byte {
	return unsafe.StringData(str)
}

// Gostring copies a char* to a Go string.
func Gostring(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var length int
	for *(*byte)(unsafe.Add(unsafe.Pointer(ptr), uintptr(length))) != '\x00' {
		length++
	}
	//string builtin copies the slice
	return string(unsafe.Slice(ptr, length))
}

type StringHeader struct {
	Len  int
	Data *byte
}

// NativeStringUnwrap cast a native string type into it's runtime value.
func NativeStringUnwrap(str string) StringHeader {
	return StringHeader{
		Data: unsafe.StringData(str),
		Len:  len(str),
	}
}

func GostringSized(ptr *byte, size uint64) string {
	if ptr == nil {
		return ""
	}
	return string(unsafe.Slice(ptr, size))
}

// Cstring converts a go string to *byte that can be passed to C code.
func Cstring(pinner *runtime.Pinner, name string) *byte {
	var b = make([]byte, len(name)+1)
	copy(b, name)
	pinner.Pin(&b[0])
	return unsafe.SliceData(b)
}

// Cast is used to centralize unsafe use C of allocated pointer.
// We take the address and then dereference it to trick go vet from creating a possible misuse of unsafe.Pointer
func Cast[T any](ptr uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&ptr)))
}

type Native interface {
	~byte | ~float64 | ~float32 | ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~bool | ~uintptr
}

func CastNative[N Native, T Native](ptr *N) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&ptr)))
}

// NativeToUintptr is a helper used by populate WafObject values
// with Go values
func NativeToUintptr[T any](x T) uintptr {
	return *(*uintptr)(unsafe.Pointer(&x))
}

// UintToNative is a helper used retrieve Go values from an uintptr encoded
// value from a WafObject
func UintptrToNative[T any](x uintptr) T {
	return *(*T)(unsafe.Pointer(&x))
}

// CastWithOffset is the same as cast but adding an offset to the pointer by a multiple of the size
// of the type pointed.
func CastWithOffset[T any](ptr uintptr, offset uint64) *T {
	return (*T)(unsafe.Add(*(*unsafe.Pointer)(unsafe.Pointer(&ptr)), offset*uint64(unsafe.Sizeof(*new(T)))))
}

// PtrToUintptr is a helper to centralize of usage of unsafe.Pointer
// do not use this function to cast interfaces
func PtrToUintptr[T any](arg *T) uintptr {
	return uintptr(unsafe.Pointer(arg))
}

func SliceToUintptr[T any](arg []T) uintptr {
	return uintptr(unsafe.Pointer(unsafe.SliceData(arg)))
}

func Slice[T any](ptr *T, length uint64) []T {
	return unsafe.Slice(ptr, length)
}

func String(ptr *byte, length uint64) string {
	return unsafe.String(ptr, length)
}
