// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

import (
	"context"
	"math"
	"reflect"
	"strings"
	"time"
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
	// For each TruncationReason, holds the size that is required to avoid truncation for each truncation that happened.
	truncations map[TruncationReason][]int

	cgoRefs          cgoRefPool
	containerMaxSize int
	stringMaxSize    int
	objectMaxDepth   int
}

// TruncationReason is a flag representing reasons why some input was not encoded in full.
type TruncationReason uint8

const (
	StringTooLong TruncationReason = 1 << iota
	ContainerTooLarge
	ObjectTooDeep
)

const (
	AppsecFieldTag            = "ddwaf"
	AppsecFieldTagValueIgnore = "ignore"
)

type native interface {
	int64 | uint64 | uintptr
}

func newLimitedEncoder() encoder {
	return encoder{
		containerMaxSize: wafMaxContainerSize,
		stringMaxSize:    wafMaxStringLength,
		objectMaxDepth:   wafMaxContainerDepth,
	}
}

func newMaxEncoder() encoder {
	return encoder{
		containerMaxSize: math.MaxInt,
		stringMaxSize:    math.MaxInt,
		objectMaxDepth:   math.MaxInt,
	}
}

// Encode takes a Go value and returns a wafObject pointer and an error.
// The returned wafObject is the root of the tree of nested wafObjects representing the Go value.
// The only error case is if the top-level object is "Unusable" which means that the data is nil or a non-data type
// like a function or a channel.
func (encoder *encoder) Encode(data any) (wo *wafObject, err error) {
	value := reflect.ValueOf(data)
	wo = &wafObject{}

	err = encoder.encode(value, wo, encoder.objectMaxDepth)
	if len(encoder.truncations[ObjectTooDeep]) != 0 {
		encoder.measureObjectDepth(value)
	}

	return
}

// Truncations returns all truncations that happened since the last call to `Truncations()`, and clears the internal
// list. This is a map from truncation reason to the list of un-truncated value sizes.
func (encoder *encoder) Truncations() map[TruncationReason][]int {
	result := encoder.truncations
	encoder.truncations = nil
	return result
}

// EncodeAddresses takes a map of Go values and returns a wafObject pointer and an error.
// The returned wafObject is the root of the tree of nested wafObjects representing the Go values.
// This function is further optimized from Encode to take addresses as input and avoid further
// errors in case the top-level map with addresses as keys is nil.
// Since errors returned by Encode are not sent up between levels of the tree, this means that all errors come from the
// top layer of encoding, which is the map of addresses. Hence, all errors should be developer errors since the map of
// addresses is not user defined custom data.
func (encoder *encoder) EncodeAddresses(addresses map[string]any) (*wafObject, error) {
	if addresses == nil {
		return nil, errUnsupportedValue
	}

	return encoder.Encode(addresses)
}

func encodeNative[T native](val T, t wafObjectType, obj *wafObject) {
	obj._type = t
	obj.value = (uintptr)(val)
}

var nullableTypeKinds = map[reflect.Kind]struct{}{
	reflect.Interface:     {},
	reflect.Pointer:       {},
	reflect.UnsafePointer: {},
	reflect.Map:           {},
	reflect.Slice:         {},
	reflect.Func:          {},
	reflect.Chan:          {},
}

// isValueNil check if the value is nullable and if it is actually nil
// we cannot directly use value.IsNil() because it panics on non-pointer values
func isValueNil(value reflect.Value) bool {
	_, nullable := nullableTypeKinds[value.Kind()]
	return nullable && value.IsNil()
}

func (encoder *encoder) encode(value reflect.Value, obj *wafObject, depth int) error {
	value, kind := resolvePointer(value)
	if (kind == reflect.Interface || kind == reflect.Pointer) && !value.IsNil() {
		// resolvePointer failed to resolve to something that's not a pointer, it
		// has indirected too many times...
		return errTooManyIndirections
	}

	// Measure-only runs for leaves
	if obj == nil && kind != reflect.Array && kind != reflect.Slice && kind != reflect.Map && kind != reflect.Struct {
		// Nothing to do, we were only here to measure object depth!
		return nil
	}

	switch {
	// Terminal cases (leaves of the tree)
	//		Is invalid type: nil interfaces for example, cannot be used to run any reflect method or it's susceptible to panic
	case !value.IsValid() || kind == reflect.Invalid:
		return errUnsupportedValue
	// 		Is nullable type: nil pointers, channels, maps or functions
	case isValueNil(value):
		encodeNative[uintptr](0, wafNilType, obj)

	// 		Booleans
	case kind == reflect.Bool:
		encodeNative(nativeToUintptr(value.Bool()), wafBoolType, obj)

	// 		Numbers
	case value.CanInt(): // any int type or alias
		encodeNative(value.Int(), wafIntType, obj)
	case value.CanUint(): // any Uint type or alias
		encodeNative(value.Uint(), wafUintType, obj)
	case value.CanFloat(): // any float type or alias
		encodeNative(nativeToUintptr(value.Float()), wafFloatType, obj)

	//		Strings
	case kind == reflect.String: // string type
		encoder.encodeString(value.String(), obj)

	case (kind == reflect.Array || kind == reflect.Slice) && value.Type().Elem().Kind() == reflect.Uint8:
		// Byte Arrays are skipped voluntarily because they are often used
		// to do partial parsing which leads to false positives
		return nil

	// Containers (internal nodes of the tree)

	// 		All recursive cases can only execute if the depth is superior to 0.
	case depth <= 0:
		// Record that there was a truncation; we will try to measure the actual depth of the object afterwards.
		encoder.addTruncation(ObjectTooDeep, -1)
		return errMaxDepthExceeded

	// 		Either an array or a slice of an array
	case kind == reflect.Array || kind == reflect.Slice:
		encoder.encodeArray(value, obj, depth-1)
	case kind == reflect.Map:
		encoder.encodeMap(value, obj, depth-1)
	case kind == reflect.Struct:
		encoder.encodeStruct(value, obj, depth-1)

	default:
		return errUnsupportedValue
	}

	return nil
}

func (encoder *encoder) encodeString(str string, obj *wafObject) {
	size := len(str)
	if size > encoder.stringMaxSize {
		str = str[:encoder.stringMaxSize]
		encoder.addTruncation(StringTooLong, size)
	}
	encoder.cgoRefs.AllocWafString(obj, str)
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
// - Even if the element values are invalid or null we still keep them to report the field name
func (encoder *encoder) encodeStruct(value reflect.Value, obj *wafObject, depth int) {
	typ := value.Type()
	nbFields := typ.NumField()

	capacity := nbFields
	length := 0
	if capacity > encoder.containerMaxSize {
		capacity = encoder.containerMaxSize
	}

	objArray := encoder.cgoRefs.AllocWafArray(obj, wafMapType, uint64(capacity))
	for i := 0; i < nbFields; i++ {
		if length == capacity {
			encoder.addTruncation(ContainerTooLarge, nbFields)
			break
		}

		fieldType := typ.Field(i)
		fieldName, usable := getFieldNameFromType(fieldType)
		if tag, ok := fieldType.Tag.Lookup(AppsecFieldTag); !usable || ok && tag == AppsecFieldTagValueIgnore {
			// Either the struct field is ignored by json marshaling so can we,
			// 		or the field was explicitly set with `ddwaf:ignore`
			continue
		}

		objElem := &objArray[length]
		// If the Map key is of unsupported type, skip it
		encoder.encodeMapKeyFromString(fieldName, objElem)

		if err := encoder.encode(value.Field(i), objElem, depth); err != nil {
			// We still need to keep the map key, so we can't discard the full object, instead, we make the value a noop
			encodeNative[uintptr](0, wafInvalidType, objElem)
		}

		length++
	}

	// Set the length to the final number of successfully encoded elements
	obj.nbEntries = uint64(length)
}

// encodeMap takes a reflect.Value and a wafObject pointer and iterates on the map elements and returns
// a wafObject map of type wafMapType. The specificities are the following:
// - It will only take the first encoder.containerMaxSize elements of the map
// - Even if the element values are invalid or null we still keep them to report the map key
func (encoder *encoder) encodeMap(value reflect.Value, obj *wafObject, depth int) {
	capacity := value.Len()
	if capacity > encoder.containerMaxSize {
		capacity = encoder.containerMaxSize
	}

	objArray := encoder.cgoRefs.AllocWafArray(obj, wafMapType, uint64(capacity))

	length := 0
	for iter := value.MapRange(); iter.Next(); {
		if length == capacity {
			encoder.addTruncation(ContainerTooLarge, value.Len())
			break
		}

		objElem := &objArray[length]
		if err := encoder.encodeMapKey(iter.Key(), objElem); err != nil {
			continue
		}

		if err := encoder.encode(iter.Value(), objElem, depth); err != nil {
			// We still need to keep the map key, so we can't discard the full object, instead, we make the value a noop
			encodeNative[uintptr](0, wafInvalidType, objElem)
		}

		length++
	}

	// Fix the size because we skipped map entries
	obj.nbEntries = uint64(length)
}

// encodeMapKey takes a reflect.Value and a wafObject and returns a wafObject ready to be considered a map entry. We use
// the function cgoRefPool.AllocWafMapKey to store the key in the wafObject. But first we need to grab the real
// underlying value by recursing through the pointer and interface values.
func (encoder *encoder) encodeMapKey(value reflect.Value, obj *wafObject) error {
	value, kind := resolvePointer(value)

	var keyStr string
	switch {
	case kind == reflect.Invalid:
		return errInvalidMapKey
	case kind == reflect.String:
		keyStr = value.String()
	case value.Type() == reflect.TypeOf([]byte(nil)):
		keyStr = string(value.Bytes())
	default:
		return errInvalidMapKey
	}

	encoder.encodeMapKeyFromString(keyStr, obj)
	return nil
}

// encodeMapKeyFromString takes a string and a wafObject and sets the map key attribute on the wafObject to the supplied
// string. The key may be truncated if it exceeds the maximum string size allowed by the encoder.
func (encoder *encoder) encodeMapKeyFromString(keyStr string, obj *wafObject) {
	size := len(keyStr)
	if size > encoder.stringMaxSize {
		keyStr = keyStr[:encoder.stringMaxSize]
		encoder.addTruncation(StringTooLong, size)
	}

	encoder.cgoRefs.AllocWafMapKey(obj, keyStr)
}

// encodeArray takes a reflect.Value and a wafObject pointer and iterates on the elements and returns
// a wafObject array of type wafArrayType. The specificities are the following:
// - It will only take the first encoder.containerMaxSize elements of the array
// - Elements producing an error at encoding or null values will be skipped
func (encoder *encoder) encodeArray(value reflect.Value, obj *wafObject, depth int) {
	length := value.Len()

	capacity := length
	if capacity > encoder.containerMaxSize {
		capacity = encoder.containerMaxSize
	}

	currIndex := 0

	objArray := encoder.cgoRefs.AllocWafArray(obj, wafArrayType, uint64(capacity))

	for i := 0; i < length; i++ {
		if currIndex == capacity {
			encoder.addTruncation(ContainerTooLarge, length)
			break
		}

		objElem := &objArray[currIndex]
		if err := encoder.encode(value.Index(i), objElem, depth); err != nil {
			continue
		}

		// If the element is null or invalid it has no impact on the waf execution, therefore we can skip its
		// encoding. In this specific case we just overwrite it at the next loop iteration.
		if objElem == nil || objElem.IsUnusable() {
			continue
		}

		currIndex++
	}

	// Fix the size because we skipped map entries
	obj.nbEntries = uint64(currIndex)
}

func (encoder *encoder) addTruncation(reason TruncationReason, size int) {
	if encoder.truncations == nil {
		encoder.truncations = make(map[TruncationReason][]int, 3)
	}
	encoder.truncations[reason] = append(encoder.truncations[reason], size)
}

// mesureObjectDepth traverses the provided object recursively to try and obtain
// the real object depth, but limits itself to about 1ms of time budget, past
// which it'll stop and return whatever it has go to so far.
func (encoder *encoder) measureObjectDepth(obj reflect.Value) {
	ctx, cancelCtx := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelCtx()

	depth, _ := depthOf(ctx, obj)
	encoder.truncations[ObjectTooDeep] = []int{depth}
}

// depthOf returns the depth of the provided object. This is 0 for scalar values,
// such as strings.
func depthOf(ctx context.Context, obj reflect.Value) (depth int, err error) {
	if err = ctx.Err(); err != nil {
		// Timed out, won't go any deeper
		return 0, err
	}

	obj, kind := resolvePointer(obj)

	//TODO: Remove this once Go 1.21 is the minimum supported version (it adds `builtin.max`)
	max := func(x, y int) int {
		if x > y {
			return x
		}
		return y
	}

	var itemDepth int
	switch kind {
	case reflect.Array, reflect.Slice:
		if obj.Type() == reflect.TypeOf([]byte(nil)) {
			// We treat byte slices as strings
			return 0, nil
		}
		for i := 0; i < obj.Len(); i++ {
			itemDepth, err = depthOf(ctx, obj.Index(i))
			depth = max(depth, itemDepth)
			if err != nil {
				break
			}
		}
		return depth + 1, err
	case reflect.Map:
		for iter := obj.MapRange(); iter.Next(); {
			itemDepth, err = depthOf(ctx, iter.Value())
			depth = max(depth, itemDepth)
			if err != nil {
				break
			}
		}
		return depth + 1, err
	case reflect.Struct:
		typ := obj.Type()
		for i := 0; i < obj.NumField(); i++ {
			fieldType := typ.Field(i)
			_, usable := getFieldNameFromType(fieldType)
			if !usable {
				continue
			}

			itemDepth, err = depthOf(ctx, obj.Field(i))
			depth = max(depth, itemDepth)
			if err != nil {
				break
			}
		}
		return depth + 1, err
	default:
		return 0, nil
	}
}

// resovlePointer attempts to resolve a pointer while limiting the pointer depth
// to be traversed, so that this is not susceptible to an infinite loop when
// provided a self-referencing pointer.
func resolvePointer(obj reflect.Value) (reflect.Value, reflect.Kind) {
	kind := obj.Kind()
	for limit := 8; limit > 0 && kind == reflect.Pointer || kind == reflect.Interface; limit-- {
		if obj.IsNil() {
			return obj, kind
		}
		obj = obj.Elem()
		kind = obj.Kind()
	}
	return obj, kind
}
