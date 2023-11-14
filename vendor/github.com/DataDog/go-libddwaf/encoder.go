// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// Encode Go values into wafObjects. Only the subset of Go types representable into wafObjects
// will be encoded while ignoring the rest of it.
// The encoder allocates the memory required for new wafObjects into the Go memory, which must be kept
// referenced for their lifetime in the C world. This lifetime depends on the ddwaf function being used with.
// the encoded result. The Go references of the allocated wafObjects, along with every Go pointer they may
// reference now or in the future, are stored and referenced in the `cgoRefs` field. The user MUST leverage
// `keepAlive()` with it according to its ddwaf use-case.
type encoder struct {
	containerMaxSize int
	stringMaxSize    int
	objectMaxDepth   int
	cgoRefs          cgoRefPool
}

func newMaxEncoder() *encoder {
	return &encoder{
		containerMaxSize: math.MaxInt,
		stringMaxSize:    math.MaxInt,
		objectMaxDepth:   math.MaxInt,
	}
}

func (encoder *encoder) Encode(data any) (*wafObject, error) {
	value := reflect.ValueOf(data)
	wo := &wafObject{}

	if err := encoder.encode(value, wo, encoder.objectMaxDepth); err != nil {
		return nil, err
	}

	return wo, nil
}

func (encoder *encoder) encode(value reflect.Value, obj *wafObject, depth int) error {
	switch kind := value.Kind(); {
	// Terminal cases (leafs of the tree)
	case kind == reflect.Invalid:
		return errUnsupportedValue

	// 		Booleans
	case kind == reflect.Bool && value.Bool(): // true
		return encoder.encodeString("true", wafStringType, obj)
	case kind == reflect.Bool && !value.Bool(): // false
		return encoder.encodeString("false", wafStringType, obj)

	// 		Numbers
	case value.CanInt(): // any int type or alias
		return encoder.encodeString(strconv.FormatInt(value.Int(), 10), wafStringType, obj)
	case value.CanUint(): // any Uint type or alias
		return encoder.encodeString(strconv.FormatUint(value.Uint(), 10), wafStringType, obj)
	case value.CanFloat(): // any float type or alias
		return encoder.encodeString(strconv.FormatInt(int64(math.Round(value.Float())), 10), wafStringType, obj)

	//		Strings
	case kind == reflect.String: // string type
		return encoder.encodeString(value.String(), wafStringType, obj)
	case value.Type() == reflect.TypeOf([]byte(nil)): // byte array -> string
		return encoder.encodeString(string(value.Bytes()), wafStringType, obj)

	// Recursive cases (internal nodes of the tree)
	case kind == reflect.Interface || kind == reflect.Pointer: // Pointer and interfaces are not taken into account
		return encoder.encode(value.Elem(), obj, depth)
	case kind == reflect.Array || kind == reflect.Slice: // either an array or a slice of an array
		return encoder.encodeArray(value, obj, depth)
	case kind == reflect.Map:
		return encoder.encodeMap(value, obj, depth)
	case kind == reflect.Struct:
		return encoder.encodeStruct(value, obj, depth)

	default:
		return errUnsupportedValue
	}
}

func (encoder *encoder) encodeString(str string, typ wafObjectType, obj *wafObject) error {
	if len(str) > encoder.stringMaxSize {
		str = str[:encoder.stringMaxSize]
	}

	encoder.cgoRefs.AllocWafString(obj, str)
	return nil
}

func getFieldNameFromType(field reflect.StructField) (string, bool) {
	fieldName := field.Name

	// Private and synthetics fields
	if len(fieldName) < 1 || unicode.IsLower(rune(fieldName[0])) {
		return "", false
	}

	// Use the json tag name as field name if present
	if tag, ok := field.Tag.Lookup("json"); ok {
		if i := strings.IndexByte(tag, byte(',')); i > 0 {
			tag = tag[:i]
		}
		if len(tag) > 0 {
			fieldName = tag
		}
	}

	return fieldName, true
}

// encodeStruct takes a reflect.Value and a wafObject pointer and iterates on the struct field to build
// a wafObject map of type wafMapType. The specificities are the following:
// - It will only take the first encoder.containerMaxSize elements of the struct
// - If the field has a json tag it will become the field name
// - Private fields and also values producing an error at encoding will be skipped
func (encoder *encoder) encodeStruct(value reflect.Value, obj *wafObject, depth int) error {
	if depth < 0 {
		return errMaxDepth
	}

	typ := value.Type()
	nbFields := typ.NumField()
	capacity := nbFields
	length := 0
	if capacity > encoder.containerMaxSize {
		capacity = encoder.containerMaxSize
	}

	objArray := encoder.cgoRefs.AllocWafArray(obj, wafMapType, uint64(capacity))
	for i := 0; length < capacity && i < nbFields; i++ {
		fieldType := typ.Field(i)
		fieldName, usable := getFieldNameFromType(fieldType)
		if !usable {
			continue
		}

		objElem := &objArray[length]
		if encoder.encodeMapKey(reflect.ValueOf(fieldName), objElem) != nil {
			continue
		}

		if encoder.encode(value.Field(i), objElem, depth-1) != nil {
			continue
		}

		length++
	}

	// Set the length to the final number of successfully encoded elements
	obj.nbEntries = uint64(length)
	return nil
}

// encodeMap takes a reflect.Value and a wafObject pointer and iterates on the map elements and returns
// a wafObject map of type wafMapType. The specificities are the following:
// - It will only take the first encoder.containerMaxSize elements of the map
// - Values and keys producing an error at encoding will be skipped
func (encoder *encoder) encodeMap(value reflect.Value, obj *wafObject, depth int) error {
	if depth < 0 {
		return errMaxDepth
	}

	capacity := value.Len()
	length := 0
	if capacity > encoder.containerMaxSize {
		capacity = encoder.containerMaxSize
	}

	objArray := encoder.cgoRefs.AllocWafArray(obj, wafMapType, uint64(capacity))
	for iter := value.MapRange(); iter.Next(); {
		if length == capacity {
			break
		}

		objElem := &objArray[length]
		if encoder.encodeMapKey(iter.Key(), objElem) != nil {
			continue
		}

		if encoder.encode(iter.Value(), objElem, depth-1) != nil {
			continue
		}

		length++
	}

	// Fix the size because we skipped map entries
	obj.nbEntries = uint64(length)
	return nil
}

// encodeMapKey takes a reflect.Value and a wafObject and returns a wafObject ready to be considered a map key
// We use the function cgoRefPool.AllocWafMapKey to store the key in the wafObject. But first we need
// to grab the real underlying value by recursing through the pointer and interface values.
func (encoder *encoder) encodeMapKey(value reflect.Value, obj *wafObject) error {
	kind := value.Kind()
	for ; kind == reflect.Pointer || kind == reflect.Interface; value, kind = value.Elem(), value.Elem().Kind() {
		if value.IsNil() {
			return errInvalidMapKey
		}
	}

	if kind != reflect.String && value.Type() != reflect.TypeOf([]byte(nil)) {
		return errInvalidMapKey
	}

	if value.Type() == reflect.TypeOf([]byte(nil)) {
		encoder.cgoRefs.AllocWafMapKey(obj, string(value.Bytes()))
	}

	if reflect.String == kind {
		encoder.cgoRefs.AllocWafMapKey(obj, value.String())
	}

	return nil
}

// encodeArray takes a reflect.Value and a wafObject pointer and iterates on the elements and returns
// a wafObject array of type wafArrayType. The specificities are the following:
// - It will only take the first encoder.containerMaxSize elements of the array
// - Values producing an error at encoding will be skipped
func (encoder *encoder) encodeArray(value reflect.Value, obj *wafObject, depth int) error {
	if depth < 0 {
		return errMaxDepth
	}

	length := value.Len()
	capacity := length
	if capacity > encoder.containerMaxSize {
		capacity = encoder.containerMaxSize
	}

	currIndex := 0
	objArray := encoder.cgoRefs.AllocWafArray(obj, wafArrayType, uint64(capacity))
	for i := 0; currIndex < capacity && i < length; i++ {
		objElem := &objArray[currIndex]
		if encoder.encode(value.Index(i), objElem, depth-1) != nil {
			continue
		}

		currIndex++
	}

	// Fix the size because we skipped map entries
	obj.nbEntries = uint64(currIndex)
	return nil
}
