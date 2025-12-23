// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libddwaf

import (
	"fmt"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
	"github.com/DataDog/go-libddwaf/v4/internal/unsafe"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
)

// decodeErrors transforms the wafObject received by the wafRulesetInfo after the call to wafDl.wafInit to a map where
// keys are the error message and the value is a array of all the rule ids which triggered this specific error
func decodeErrors(obj *bindings.WAFObject) (map[string][]string, error) {
	if !obj.IsMap() {
		return nil, fmt.Errorf("decodeErrors: %w: expected map, got %s", waferrors.ErrInvalidObjectType, obj.Type)
	}

	if obj.Value == 0 && obj.NbEntries == 0 {
		return nil, nil
	}

	if obj.Value == 0 && obj.NbEntries > 0 {
		return nil, waferrors.ErrNilObjectPtr
	}

	wafErrors := map[string][]string{}
	for i := uint64(0); i < obj.NbEntries; i++ {
		objElem := unsafe.CastWithOffset[bindings.WAFObject](obj.Value, i)

		errorMessage := unsafe.GostringSized(unsafe.Cast[byte](objElem.ParameterName), objElem.ParameterNameLength)
		ruleIds, err := decodeStringArray(objElem)
		if err != nil {
			return nil, err
		}

		wafErrors[errorMessage] = ruleIds
	}

	return wafErrors, nil
}

func decodeDiagnostics(obj *bindings.WAFObject) (Diagnostics, error) {
	if !obj.IsMap() {
		return Diagnostics{}, fmt.Errorf("decodeDiagnostics: %w: expected map, got %s", waferrors.ErrInvalidObjectType, obj.Type)
	}
	if obj.Value == 0 && obj.NbEntries > 0 {
		return Diagnostics{}, waferrors.ErrNilObjectPtr
	}

	var (
		diags Diagnostics
		err   error
	)
	for i := uint64(0); i < obj.NbEntries; i++ {
		objElem := unsafe.CastWithOffset[bindings.WAFObject](obj.Value, i)
		key := unsafe.GostringSized(unsafe.Cast[byte](objElem.ParameterName), objElem.ParameterNameLength)
		switch key {
		case "actions":
			diags.Actions, err = decodeFeature(objElem)
		case "custom_rules":
			diags.CustomRules, err = decodeFeature(objElem)
		case "exclusions":
			diags.Exclusions, err = decodeFeature(objElem)
		case "rules":
			diags.Rules, err = decodeFeature(objElem)
		case "rules_data":
			diags.RulesData, err = decodeFeature(objElem)
		case "exclusion_data":
			diags.ExclusionData, err = decodeFeature(objElem)
		case "rules_override":
			diags.RulesOverrides, err = decodeFeature(objElem)
		case "processors":
			diags.Processors, err = decodeFeature(objElem)
		case "processor_overrides":
			diags.ProcessorOverrides, err = decodeFeature(objElem)
		case "scanners":
			diags.Scanners, err = decodeFeature(objElem)
		case "ruleset_version":
			diags.Version = unsafe.GostringSized(unsafe.Cast[byte](objElem.Value), objElem.NbEntries)
		default:
			// ignore?
		}
		if err != nil {
			return Diagnostics{}, err
		}
	}

	return diags, nil
}

func decodeFeature(obj *bindings.WAFObject) (*Feature, error) {
	if !obj.IsMap() {
		return nil, fmt.Errorf("decodeFeature: %w: expected map, got %s", waferrors.ErrInvalidObjectType, obj.Type)
	}
	if obj.Value == 0 && obj.NbEntries > 0 {
		return nil, waferrors.ErrNilObjectPtr
	}
	var feature Feature
	var err error

	for i := uint64(0); i < obj.NbEntries; i++ {
		objElem := unsafe.CastWithOffset[bindings.WAFObject](obj.Value, i)
		key := unsafe.GostringSized(unsafe.Cast[byte](objElem.ParameterName), objElem.ParameterNameLength)
		switch key {
		case "error":
			feature.Error = unsafe.GostringSized(unsafe.Cast[byte](objElem.Value), objElem.NbEntries)
		case "errors":
			feature.Errors, err = decodeErrors(objElem)
		case "failed":
			feature.Failed, err = decodeStringArray(objElem)
		case "loaded":
			feature.Loaded, err = decodeStringArray(objElem)
		case "skipped":
			feature.Skipped, err = decodeStringArray(objElem)
		case "warnings":
			feature.Warnings, err = decodeErrors(objElem)
		default:
			return nil, fmt.Errorf("%w: %s", waferrors.ErrUnsupportedValue, key)
		}

		if err != nil {
			return nil, err
		}
	}

	return &feature, nil
}

func decodeStringArray(obj *bindings.WAFObject) ([]string, error) {
	// We consider that nil is an empty array
	if obj.IsNil() {
		return nil, nil
	}

	if !obj.IsArray() {
		return nil, fmt.Errorf("decodeStringArray: %w: expected array, got %s", waferrors.ErrInvalidObjectType, obj.Type)
	}

	if obj.Value == 0 && obj.NbEntries > 0 {
		return nil, waferrors.ErrNilObjectPtr
	}

	if obj.NbEntries == 0 {
		return nil, nil
	}

	strArr := make([]string, 0, obj.NbEntries)
	for i := uint64(0); i < obj.NbEntries; i++ {
		objElem := unsafe.CastWithOffset[bindings.WAFObject](obj.Value, i)
		if objElem.Type != bindings.WAFStringType {
			return nil, fmt.Errorf("decodeStringArray: %w: expected string, got %s", waferrors.ErrInvalidObjectType, objElem.Type)
		}

		strArr = append(strArr, unsafe.GostringSized(unsafe.Cast[byte](objElem.Value), objElem.NbEntries))
	}

	return strArr, nil
}

// Deprecated: This is merely wrapping [bindings.WAFObject.AnyValue], which should be used directly
// instead.
func DecodeObject(obj *WAFObject) (any, error) {
	return obj.AnyValue()
}
