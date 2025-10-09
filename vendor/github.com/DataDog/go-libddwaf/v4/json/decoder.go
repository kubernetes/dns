// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"unique"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
)

type Decoder struct {
	pinner *runtime.Pinner
	json   *json.Decoder
}

func NewDecoder(rd io.Reader, pinner *runtime.Pinner) *Decoder {
	js := json.NewDecoder(rd)
	js.UseNumber()
	return &Decoder{pinner: pinner, json: js}
}

func (d *Decoder) Decode(v *bindings.WAFObject) error {
	tok, err := d.json.Token()
	if err != nil {
		return err
	}

	switch tok := tok.(type) {
	case json.Delim:
		switch tok {
		case '{':
			return d.decodeMap(v)
		case '[':
			return d.decodeArray(v)
		default:
			return fmt.Errorf("%w: %q", errors.ErrUnsupported, tok)
		}

	case json.Number:
		return decodeNumber(v, tok)

	case bool:
		v.SetBool(tok)
		return nil

	case string:
		v.SetString(d.pinner, tok)
		return nil

	case nil:
		v.SetNil()
		return nil

	default:
		return fmt.Errorf("%w: %T %v", errors.ErrUnsupported, tok, tok)
	}
}

func (d *Decoder) decodeArray(v *bindings.WAFObject) error {
	var items []bindings.WAFObject
	for d.json.More() {
		var v bindings.WAFObject
		if err := d.Decode(&v); err != nil {
			return err
		}
		items = append(items, v)
	}

	// Consume the closing bracket...
	if _, err := d.json.Token(); err != nil {
		return err
	}

	v.SetArrayData(d.pinner, items)
	return nil
}

func (d *Decoder) decodeMap(v *bindings.WAFObject) error {
	var items []bindings.WAFObject
	for d.json.More() {
		keyTok, err := d.json.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %T %q", keyTok, keyTok)
		}
		// To reduce the overall amount of memory that is retained by the resulting WAFObjects, we make
		// the keys unique, as they are repeated a lot in the original JSON.
		key = unique.Make(key).Value()

		var v bindings.WAFObject
		v.SetMapKey(d.pinner, key)
		if err := d.Decode(&v); err != nil {
			return err
		}
		items = append(items, v)
	}

	// Consume the closing brace...
	if _, err := d.json.Token(); err != nil {
		return err
	}

	v.SetMapData(d.pinner, items)
	return nil
}

func decodeNumber(v *bindings.WAFObject, tok json.Number) error {
	if i, err := strconv.ParseUint(string(tok), 10, 64); err == nil {
		v.SetUint(i)
		return nil
	}

	if i, err := tok.Int64(); err == nil {
		v.SetInt(i)
		return nil
	}

	f, err := tok.Float64()
	if err != nil {
		return fmt.Errorf("invalid number %q: %w", tok, err)
	}

	v.SetFloat(f)

	return nil
}
