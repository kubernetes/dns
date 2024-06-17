// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import "unsafe"

// gostring copies a char* to a Go string.
func gostring(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var length int
	for {
		if *(*byte)(unsafe.Add(unsafe.Pointer(ptr), uintptr(length))) == '\x00' {
			break
		}
		length++
	}
	//string builtin copies the slice
	return string(unsafe.Slice(ptr, length))
}
