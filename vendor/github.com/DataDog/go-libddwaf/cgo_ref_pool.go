// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

import (
	"strconv"
)

// cgoRefPool is a way to make sure we can safely send go allocated data on the C side of the WAF
// The main issue is the following: the wafObject uses a C union to store the tree structure of the full object,
// union equivalent in go are interfaces and they are not compatible with C unions. The only way to be 100% sure
// that the Go wafObject struct have the same layout as the C one is to only use primitive types. So the only way to
// store a raw pointer is to use the uintptr type. But since uintptr do not have pointer semantics (and are just
// basically integers), we need another structure to store the value as Go pointer because the GC is lurking. That's
// where the cgoRefPool object comes into play: All new wafObject elements are created via this API whose especially
// built to make sure there is no gap for the Garbage Collector to exploit. From there, since underlying values of the
// wafObject are either arrays (for maps, structs and arrays) or string (for all ints, booleans and strings),
// we can store 2 slices of arrays and use runtime.KeepAlive in each code path to protect them from the GC.
type cgoRefPool struct {
	stringRefs [][]byte
	arrayRefs  [][]wafObject
}

func (refPool *cgoRefPool) append(newRefs cgoRefPool) {
	refPool.stringRefs = append(refPool.stringRefs, newRefs.stringRefs...)
	refPool.arrayRefs = append(refPool.arrayRefs, newRefs.arrayRefs...)
}

func (refPool *cgoRefPool) AllocCString(str string) uintptr {
	goArray := make([]byte, len(str)+1)
	copy(goArray, str)
	refPool.stringRefs = append(refPool.stringRefs, goArray)
	goArray[len(str)] = 0 // Null termination byte for C strings

	return sliceToUintptr(goArray)
}

func (refPool *cgoRefPool) AllocWafString(obj *wafObject, str string) {
	obj._type = wafStringType

	if len(str) == 0 {
		obj.nbEntries = 0
		obj.value = 0
		return
	}

	goArray := make([]byte, len(str))
	copy(goArray, str)
	refPool.stringRefs = append(refPool.stringRefs, goArray)

	obj.value = sliceToUintptr(goArray)
	obj.nbEntries = uint64(len(goArray))
}

func (refPool *cgoRefPool) AllocWafArray(obj *wafObject, typ wafObjectType, size uint64) []wafObject {
	if typ != wafMapType && typ != wafArrayType {
		panic("Cannot allocate this waf object data type as an array: " + strconv.Itoa(int(typ)))
	}

	obj._type = typ
	obj.nbEntries = size

	// If the array size is zero no need to allocate anything
	if size == 0 {
		obj.value = 0
		return nil
	}

	goArray := make([]wafObject, size)
	refPool.arrayRefs = append(refPool.arrayRefs, goArray)

	obj.value = sliceToUintptr(goArray)
	return goArray
}

func (refPool *cgoRefPool) AllocWafMapKey(obj *wafObject, str string) {
	if len(str) == 0 {
		return
	}

	goArray := make([]byte, len(str))
	copy(goArray, str)
	refPool.stringRefs = append(refPool.stringRefs, goArray)

	obj.parameterName = sliceToUintptr(goArray)
	obj.parameterNameLength = uint64(len(goArray))
}
