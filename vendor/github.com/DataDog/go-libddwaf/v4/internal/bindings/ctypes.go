// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bindings

import (
	"fmt"
	"structs"

	"github.com/DataDog/go-libddwaf/v4/internal/pin"
	"github.com/DataDog/go-libddwaf/v4/internal/unsafe"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
	"github.com/pkg/errors"
)

const (
	MaxStringLength   = 4096
	MaxContainerDepth = 20
	MaxContainerSize  = 256
)

type WAFReturnCode int32

const (
	WAFErrInternal WAFReturnCode = iota - 3
	WAFErrInvalidObject
	WAFErrInvalidArgument
	WAFOK
	WAFMatch
)

// WAFObjectType is an enum in C which has the size of DWORD.
// But DWORD is 4 bytes in amd64 and arm64 so uint32 it is.
type WAFObjectType uint32

const WAFInvalidType WAFObjectType = 0
const (
	WAFIntType WAFObjectType = 1 << iota
	WAFUintType
	WAFStringType
	WAFArrayType
	WAFMapType
	WAFBoolType
	WAFFloatType
	WAFNilType
)

func (w WAFObjectType) String() string {
	switch w {
	case WAFInvalidType:
		return "invalid"
	case WAFIntType:
		return "int"
	case WAFUintType:
		return "uint"
	case WAFStringType:
		return "string"
	case WAFArrayType:
		return "array"
	case WAFMapType:
		return "map"
	case WAFBoolType:
		return "bool"
	case WAFFloatType:
		return "float"
	case WAFNilType:
		return "nil"
	default:
		return fmt.Sprintf("unknown(%d)", w)
	}
}

type WAFObject struct {
	_                   structs.HostLayout
	ParameterName       uintptr
	ParameterNameLength uint64
	Value               uintptr
	NbEntries           uint64
	Type                WAFObjectType
	_                   [4]byte
	// Forced padding
	// We only support 2 archs and cgo generated the same padding to both.
	// We don't want the C struct to be packed because actually go will do the same padding itself,
	// we just add it explicitly to not take any chance.
	// And we cannot pack a struct in go so it will get tricky if the struct is
	// packed (apart from breaking all tracers of course)
}

// IsInvalid determines whether this WAF Object has the invalid type (which is the 0-value).
func (w *WAFObject) IsInvalid() bool {
	return w.Type == WAFInvalidType
}

// IsNil determines whether this WAF Object is nil or not.
func (w *WAFObject) IsNil() bool {
	return w.Type == WAFNilType
}

// IsArray determines whether this WAF Object is an array or not.
func (w *WAFObject) IsArray() bool {
	return w.Type == WAFArrayType
}

// IsMap determines whether this WAF Object is a map or not.
func (w *WAFObject) IsMap() bool {
	return w.Type == WAFMapType
}

// IsInt determines whether this WAF Object is a iny or not.
func (w *WAFObject) IsInt() bool {
	return w.Type == WAFIntType
}

// IsUint determines whether this WAF Object is a uint or not.
func (w *WAFObject) IsUint() bool {
	return w.Type == WAFUintType
}

// IsBool determines whether this WAF Object is a bool or not.
func (w *WAFObject) IsBool() bool {
	return w.Type == WAFBoolType
}

// IsFloat determines whether this WAF Object is a float or not.
func (w *WAFObject) IsFloat() bool {
	return w.Type == WAFFloatType
}

// IsString determines whether this WAF Object is a string or not.
func (w *WAFObject) IsString() bool {
	return w.Type == WAFStringType
}

// IsUnusable returns true if the wafObject has no impact on the WAF execution
// But we still need this kind of objects to forward map keys in case the value of the map is invalid
func (w *WAFObject) IsUnusable() bool {
	return w.Type == WAFInvalidType || w.Type == WAFNilType
}

// SetArray sets the receiving [WAFObject] to a new array with the given
// capacity.
func (w *WAFObject) SetArray(pinner pin.Pinner, capacity uint64) []WAFObject {
	return w.setArrayTyped(pinner, capacity, WAFArrayType)
}

// SetArrayData sets the receiving [WAFObject] to the provided array items.
func (w *WAFObject) SetArrayData(pinner pin.Pinner, data []WAFObject) {
	w.setArrayDataTyped(pinner, data, WAFArrayType)
}

// SetMap sets the receiving [WAFObject] to a new map with the given capacity.
func (w *WAFObject) SetMap(pinner pin.Pinner, capacity uint64) []WAFObject {
	return w.setArrayTyped(pinner, capacity, WAFMapType)
}

// SetMapData sets the receiving [WAFObject] to the provided map items.
func (w *WAFObject) SetMapData(pinner pin.Pinner, data []WAFObject) {
	w.setArrayDataTyped(pinner, data, WAFMapType)
}

// SetMapKey sets the receiving [WAFObject] to a new map key with the given
// string.
func (w *WAFObject) SetMapKey(pinner pin.Pinner, key string) {
	header := unsafe.NativeStringUnwrap(key)

	w.ParameterNameLength = uint64(header.Len)
	if w.ParameterNameLength == 0 {
		w.ParameterName = 0
		return
	}
	pinner.Pin(unsafe.Pointer(header.Data))
	w.ParameterName = uintptr(unsafe.Pointer(header.Data))
}

func (w *WAFObject) MapKey() string {
	return string(unsafe.Slice(*(**byte)(unsafe.Pointer(&w.ParameterName)), w.ParameterNameLength))
}

func (w *WAFObject) Values() ([]WAFObject, error) {
	if !w.IsArray() && !w.IsMap() {
		return nil, errors.New("value is not an array or map")
	}
	return unsafe.Slice(*(**WAFObject)(unsafe.Pointer(&w.Value)), w.NbEntries), nil
}

func (w *WAFObject) AnyValue() (any, error) {
	switch w.Type {
	case WAFArrayType:
		return w.ArrayValue()
	case WAFBoolType:
		return w.BoolValue()
	case WAFFloatType:
		return w.FloatValue()
	case WAFIntType:
		return w.IntValue()
	case WAFMapType:
		return w.MapValue()
	case WAFStringType:
		return w.StringValue()
	case WAFUintType:
		return w.UIntValue()
	case WAFNilType:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: %s", waferrors.ErrUnsupportedValue, w.Type)
	}
}

func (w *WAFObject) ArrayValue() ([]any, error) {
	if w.IsNil() {
		return nil, nil
	}

	if !w.IsArray() {
		return nil, errors.New("value is not an array")
	}

	items, err := w.Values()
	if err != nil {
		return nil, err
	}

	res := make([]any, len(items))
	for i, item := range items {
		res[i], err = item.AnyValue()
		if err != nil {
			return nil, fmt.Errorf("while decoding item at index %d: %w", i, err)
		}
	}
	return res, nil
}

func (w *WAFObject) MapValue() (map[string]any, error) {
	if w.IsNil() {
		return nil, nil
	}

	if !w.IsMap() {
		return nil, errors.New("value is not a map")
	}

	items, err := w.Values()
	if err != nil {
		return nil, err
	}

	res := make(map[string]any, len(items))
	for _, item := range items {
		key := item.MapKey()
		res[key], err = item.AnyValue()
		if err != nil {
			return nil, fmt.Errorf("while decoding value at %q: %w", key, err)
		}
	}
	return res, nil
}

func (w *WAFObject) BoolValue() (bool, error) {
	if !w.IsBool() {
		return false, errors.New("value is not a boolean")
	}
	return w.Value != 0, nil
}

func (w *WAFObject) FloatValue() (float64, error) {
	if !w.IsFloat() {
		return 0, errors.New("value is not a uint")
	}
	return *(*float64)(unsafe.Pointer(&w.Value)), nil
}

func (w *WAFObject) IntValue() (int64, error) {
	if !w.IsInt() {
		return 0, errors.New("value is not a uint")
	}
	return int64(w.Value), nil
}

func (w *WAFObject) StringValue() (string, error) {
	if !w.IsString() {
		return "", errors.New("value is not a string")
	}
	return string(unsafe.Slice(*(**byte)(unsafe.Pointer(&w.Value)), w.NbEntries)), nil
}

func (w *WAFObject) UIntValue() (uint64, error) {
	if !w.IsUint() {
		return 0, errors.New("value is not a uint")
	}
	return uint64(w.Value), nil
}

var blankCStringValue = unsafe.Pointer(unsafe.NativeStringUnwrap("\x00").Data)

// SetString sets the receiving [WAFObject] value to the given string.
func (w *WAFObject) SetString(pinner pin.Pinner, str string) {
	header := unsafe.NativeStringUnwrap(str)

	w.Type = WAFStringType
	w.NbEntries = uint64(header.Len)
	if w.NbEntries == 0 {
		w.Value = uintptr(blankCStringValue)
		return
	}
	pinner.Pin(unsafe.Pointer(header.Data))
	w.Value = uintptr(unsafe.Pointer(header.Data))
}

// SetInt sets the receiving [WAFObject] value to the given int.
func (w *WAFObject) SetInt(i int64) {
	w.Type = WAFIntType
	w.Value = unsafe.NativeToUintptr(i)
}

// SetUint sets the receiving [WAFObject] value to the given uint.
func (w *WAFObject) SetUint(i uint64) {
	w.Type = WAFUintType
	w.Value = unsafe.NativeToUintptr(i)
}

// SetBool sets the receiving [WAFObject] value to the given bool.
func (w *WAFObject) SetBool(b bool) {
	w.Type = WAFBoolType
	if b {
		w.Value = uintptr(1)
	} else {
		w.Value = uintptr(0)
	}
}

// SetFloat sets the receiving [WAFObject] value to the given float.
func (w *WAFObject) SetFloat(f float64) {
	w.Type = WAFFloatType
	w.Value = unsafe.NativeToUintptr(f)
}

// SetNil sets the receiving [WAFObject] to nil.
func (w *WAFObject) SetNil() {
	w.Type = WAFNilType
	w.Value = 0
}

// SetInvalid sets the receiving [WAFObject] to invalid.
func (w *WAFObject) SetInvalid() {
	w.Type = WAFInvalidType
	w.Value = 0
}

func (w *WAFObject) setArrayTyped(pinner pin.Pinner, capacity uint64, t WAFObjectType) []WAFObject {
	var arr []WAFObject
	if capacity > 0 {
		arr = make([]WAFObject, capacity)
	}
	w.setArrayDataTyped(pinner, arr, t)
	return arr
}

func (w *WAFObject) setArrayDataTyped(pinner pin.Pinner, arr []WAFObject, t WAFObjectType) {
	w.Type = t
	w.NbEntries = uint64(len(arr))
	if w.NbEntries == 0 {
		w.Value = 0
		return
	}

	ptr := unsafe.Pointer(unsafe.SliceData(arr))
	pinner.Pin(ptr)
	w.Value = uintptr(ptr)
}

type WAFConfig struct {
	_          structs.HostLayout
	Limits     WAFConfigLimits
	Obfuscator WAFConfigObfuscator
	FreeFn     uintptr
}

type WAFConfigLimits struct {
	_                 structs.HostLayout
	MaxContainerSize  uint32
	MaxContainerDepth uint32
	MaxStringLength   uint32
}

type WAFConfigObfuscator struct {
	_          structs.HostLayout
	KeyRegex   uintptr // char *
	ValueRegex uintptr // char *
}

type WAFResult struct {
	_            structs.HostLayout
	Timeout      byte
	Events       WAFObject
	Actions      WAFObject
	Derivatives  WAFObject
	TotalRuntime uint64
}

// WAFBuilder is a forward declaration in ddwaf.h header
// We basically don't need to modify it, only to give it to the waf
type WAFBuilder uintptr

// WAFHandle is a forward declaration in ddwaf.h header
// We basically don't need to modify it, only to give it to the waf
type WAFHandle uintptr

// WAFContext is a forward declaration in ddwaf.h header
// We basically don't need to modify it, only to give it to the waf
type WAFContext uintptr
