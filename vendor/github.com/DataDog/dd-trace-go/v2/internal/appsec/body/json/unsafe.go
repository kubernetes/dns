// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package json

import (
	"reflect"
	"runtime"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

const intSize = unsafe.Sizeof(int(0))

var headOffset = getOffset("head")

func getOffset(name string) uintptr {
	typ := reflect.TypeFor[jsoniter.Iterator]()
	field, found := typ.FieldByName(name)
	if !found {
		panic("jsoniter.Iterator does not have a field named '" + name + "'")
	}

	if field.Type.Size() != intSize {
		panic("jsoniter.Iterator field '" + name + "' is not of the right size")
	}

	return field.Offset
}

// getIteratorHead retrieves the head field from a jsoniter.Iterator.
// This is done using unsafe operations to avoid the overhead of reflection.
func getIteratorHead(iter *jsoniter.Iterator) int {
	head := *(*int)(unsafe.Add(unsafe.Pointer(iter), headOffset))
	runtime.KeepAlive(iter) // Ensure the iterator is not garbage collected while we're using it
	return head
}
