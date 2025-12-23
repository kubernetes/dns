// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package json

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/DataDog/go-libddwaf/v4"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
	json "github.com/minio/simdjson-go"
)

type Encodable struct {
	truncated  bool
	data       []byte
	parsedJSON *json.ParsedJson
}

var (
	parsedJSONPool sync.Pool
)

func NewEncodableFromData(data []byte, truncated bool) libddwaf.Encodable {
	parsedJSON, _ := parsedJSONPool.Get().(*json.ParsedJson)
	pj, err := json.Parse(data, parsedJSON, json.WithCopyStrings(false))
	if err != nil {
		// This can happen if a trivial JSON type is found like a string or number, in this case simply return a
		// simpler encoder where performance is not critical.
		return newJSONIterEncodableFromData(data, truncated)
	}

	return &Encodable{
		truncated:  truncated,
		data:       data,
		parsedJSON: pj,
	}
}

func (e *Encodable) ToEncoder(config libddwaf.EncoderConfig) *encoder {
	return &encoder{
		Encodable: e,
		config:    config,
	}
}

func (e *Encodable) Encode(config libddwaf.EncoderConfig, obj *libddwaf.WAFObject, remainingDepth int) (map[libddwaf.TruncationReason][]int, error) {
	encoder := e.ToEncoder(config)
	defer parsedJSONPool.Put(encoder.parsedJSON)

	iter := encoder.parsedJSON.Iter()
	if err := encoder.Encode(obj, iter.Advance(), &iter, remainingDepth); err != nil && (errors.Is(err, waferrors.ErrTimeout) || !e.truncated) {
		// Return an error if a waf timeout error occurred, or we are in normal parsing mode
		return nil, err
	}

	if obj.IsUnusable() {
		// Do not return an invalid root object
		return nil, fmt.Errorf("invalid json at root")
	}

	return encoder.truncations, nil
}

type encoder struct {
	*Encodable
	truncations map[libddwaf.TruncationReason][]int
	config      libddwaf.EncoderConfig
}

type skipError struct{}

func (skipError) Error() string {
	return "skip error"
}

var skipErr error = skipError{}

// addTruncation records a truncation event.
func (e *encoder) addTruncation(reason libddwaf.TruncationReason, size int) {
	if e.truncations == nil {
		e.truncations = make(map[libddwaf.TruncationReason][]int, 3)
	}
	e.truncations[reason] = append(e.truncations[reason], size)
}

func (e *encoder) Encode(obj *libddwaf.WAFObject, typ json.Type, iter *json.Iter, remainingDepth int) (err error) {
	if e.config.Timer.Exhausted() {
		return waferrors.ErrTimeout
	}

	// EOF errors and non-fatal if we truncated the input.
	defer func() {
		if err == io.EOF && e.truncated {
			err = nil
		}

		if err != nil {
			obj.SetInvalid()
		}
	}()

	if typ == json.TypeRoot {
		typ, _, err = iter.Root(iter)
		if err != nil {
			return fmt.Errorf("failed to get root element: %w", err)
		}
	}

	switch typ {
	case json.TypeObject:
		return e.encodeObject(obj, iter, remainingDepth-1)
	case json.TypeArray:
		return e.encodeArray(obj, iter, remainingDepth-1)
	case json.TypeString:
		var value []byte
		value, err = iter.StringBytes()
		e.encodeString(value, obj)
	case json.TypeInt:
		var value int64
		value, err = iter.Int()
		obj.SetInt(value)
	case json.TypeUint:
		var value uint64
		value, err = iter.Uint()
		obj.SetUint(value)
	case json.TypeFloat:
		var value float64
		value, err = iter.Float()
		obj.SetFloat(value)
	case json.TypeBool:
		var value bool
		value, err = iter.Bool()
		obj.SetBool(value)
	case json.TypeNull:
		obj.SetNil()
	case json.TypeNone:
		err = io.EOF
	default:
		return fmt.Errorf("unexpected JSON token: %v", typ)
	}

	return err
}

func (e *encoder) encodeString(str []byte, obj *libddwaf.WAFObject) {
	strLen := len(str)
	if strLen > e.config.MaxStringSize {
		str = str[:e.config.MaxStringSize]
		e.addTruncation(libddwaf.StringTooLong, strLen)
		strLen = e.config.MaxStringSize
	}

	obj.SetString(e.config.Pinner, unsafe.String(unsafe.SliceData(str), strLen))
}

// encodeMapKeyFromString takes a string and a wafObject and sets the map key attribute on the wafObject to the supplied
// string. The key may be truncated if it exceeds the maximum string size allowed by the jsonEncoder.
func (e *encoder) encodeMapKeyFromString(keyStr []byte, obj *libddwaf.WAFObject) {
	size := len(keyStr)
	if size > e.config.MaxStringSize {
		keyStr = keyStr[:e.config.MaxStringSize]
		e.addTruncation(libddwaf.StringTooLong, size)
		size = e.config.MaxStringSize
	}

	obj.SetMapKey(e.config.Pinner, unsafe.String(unsafe.SliceData(keyStr), size))
}

func (e *encoder) encodeObject(parentObj *libddwaf.WAFObject, iter *json.Iter, depth int) error {
	if e.config.Timer.Exhausted() {
		return waferrors.ErrTimeout
	}

	if depth < 0 {
		e.addTruncation(libddwaf.ObjectTooDeep, e.config.MaxObjectDepth-depth)
		return skipErr
	}

	var (
		errs    []error
		length  int
		wafObjs []libddwaf.WAFObject
	)

	var jsonObj json.Object
	_, err := iter.Object(&jsonObj)
	if err != nil {
		return err
	}

	var innerIter json.Iter
	for key, typ, err := jsonObj.NextElementBytes(&innerIter); typ != json.TypeNone; key, typ, err = jsonObj.NextElementBytes(&innerIter) {
		length++
		if e.config.Timer.Exhausted() {
			errs = append(errs, waferrors.ErrTimeout)
			break
		}

		if err != nil {
			errs = append(errs, err)
			continue
		}

		if len(wafObjs) >= e.config.MaxContainerSize {
			innerIter.Advance()
			continue
		}

		wafObjs = append(wafObjs, libddwaf.WAFObject{})
		entryObj := &wafObjs[len(wafObjs)-1]
		e.encodeMapKeyFromString(key, entryObj)

		if err := e.Encode(entryObj, typ, &innerIter, depth); err != nil {
			entryObj.SetInvalid()
			if err == skipErr || errors.Is(err, io.EOF) && e.truncated {
				continue
			}

			errs = append(errs, fmt.Errorf("failed to encode value for key %q: %w", key, err))
			break
		}
	}

	if len(wafObjs) >= e.config.MaxContainerSize {
		e.addTruncation(libddwaf.ContainerTooLarge, length)
	}

	parentObj.SetMapData(e.config.Pinner, wafObjs)
	return errors.Join(errs...)
}

func (e *encoder) encodeArray(parentObj *libddwaf.WAFObject, iter *json.Iter, depth int) error {
	if e.config.Timer.Exhausted() {
		return waferrors.ErrTimeout
	}

	if depth < 0 {
		e.addTruncation(libddwaf.ObjectTooDeep, e.config.MaxObjectDepth-depth)
		return skipErr
	}

	var (
		errs    []error
		length  int
		wafObjs []libddwaf.WAFObject
	)

	var jsonArray json.Array
	_, err := iter.Array(&jsonArray)
	if err != nil {
		return err
	}

	innerIter := jsonArray.Iter()
	for typ := innerIter.Advance(); typ != json.TypeNone; typ = innerIter.Advance() {
		length++
		if e.config.Timer.Exhausted() {
			errs = append(errs, waferrors.ErrTimeout)
			break
		}

		if len(wafObjs) >= e.config.MaxContainerSize {
			continue
		}

		wafObjs = append(wafObjs, libddwaf.WAFObject{})
		entryObj := &wafObjs[len(wafObjs)-1]
		if err := e.Encode(entryObj, typ, &innerIter, depth); err != nil {
			wafObjs = wafObjs[:len(wafObjs)-1]
			if err == skipErr {
				continue
			}
			errs = append(errs, fmt.Errorf("failed to encode value: %w", err))
			break
		}

		if entryObj.IsUnusable() {
			// If the entry object is unusable, we skip it and continue with the next element.
			wafObjs = wafObjs[:len(wafObjs)-1] // Remove the last element if nothing is worth encoding
		}
	}

	if len(wafObjs) >= e.config.MaxContainerSize {
		e.addTruncation(libddwaf.ContainerTooLarge, length)
	}

	parentObj.SetArrayData(e.config.Pinner, wafObjs)
	return errors.Join(errs...)
}
