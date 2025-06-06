// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bindings

import (
	"structs"
)

const (
	WafMaxStringLength   = 4096
	WafMaxContainerDepth = 20
	WafMaxContainerSize  = 256
	WafRunTimeout        = 5000
)

type WafReturnCode int32

const (
	WafErrInternal WafReturnCode = iota - 3
	WafErrInvalidObject
	WafErrInvalidArgument
	WafOK
	WafMatch
)

// WafObjectType is an enum in C which has the size of DWORD.
// But DWORD is 4 bytes in amd64 and arm64 so uint32 it is.
type WafObjectType uint32

const WafInvalidType WafObjectType = 0
const (
	WafIntType WafObjectType = 1 << iota
	WafUintType
	WafStringType
	WafArrayType
	WafMapType
	WafBoolType
	WafFloatType
	WafNilType
)

type WafObject struct {
	_                   structs.HostLayout
	ParameterName       uintptr
	ParameterNameLength uint64
	Value               uintptr
	NbEntries           uint64
	Type                WafObjectType
	_                   [4]byte
	// Forced padding
	// We only support 2 archs and cgo generated the same padding to both.
	// We don't want the C struct to be packed because actually go will do the same padding itself,
	// we just add it explicitly to not take any chance.
	// And we cannot pack a struct in go so it will get tricky if the struct is
	// packed (apart from breaking all tracers of course)
}

// IsInvalid determines whether this WAF Object has the invalid type (which is the 0-value).
func (w *WafObject) IsInvalid() bool {
	return w.Type == WafInvalidType
}

// IsNil determines whether this WAF Object is nil or not.
func (w *WafObject) IsNil() bool {
	return w.Type == WafNilType
}

// IsArray determines whether this WAF Object is an array or not.
func (w *WafObject) IsArray() bool {
	return w.Type == WafArrayType
}

// IsMap determines whether this WAF Object is a map or not.
func (w *WafObject) IsMap() bool {
	return w.Type == WafMapType
}

// IsUnusable returns true if the wafObject has no impact on the WAF execution
// But we still need this kind of objects to forward map keys in case the value of the map is invalid
func (w *WafObject) IsUnusable() bool {
	return w.Type == WafInvalidType || w.Type == WafNilType
}

type WafConfig struct {
	_          structs.HostLayout
	Limits     WafConfigLimits
	Obfuscator WafConfigObfuscator
	FreeFn     uintptr
}

type WafConfigLimits struct {
	_                 structs.HostLayout
	MaxContainerSize  uint32
	MaxContainerDepth uint32
	MaxStringLength   uint32
}

type WafConfigObfuscator struct {
	_          structs.HostLayout
	KeyRegex   uintptr // char *
	ValueRegex uintptr // char *
}

type WafResult struct {
	_            structs.HostLayout
	Timeout      byte
	Events       WafObject
	Actions      WafObject
	Derivatives  WafObject
	TotalRuntime uint64
}

// WafHandle is a forward declaration in ddwaf.h header
// We basically don't need to modify it, only to give it to the waf
type WafHandle uintptr

// WafContext is a forward declaration in ddwaf.h header
// We basically don't need to modify it, only to give it to the waf
type WafContext uintptr
