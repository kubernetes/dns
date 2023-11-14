// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package waf

// decodeErrors transforms the wafObject received by the wafRulesetInfo after the call to wafDl.wafInit to a map where
// keys are the error message and the value is a array of all the rule ids which triggered this specific error
func decodeErrors(obj *wafObject) (map[string][]string, error) {
	if obj._type != wafMapType {
		return nil, errInvalidObjectType
	}

	if obj.value == 0 && obj.nbEntries > 0 {
		return nil, errNilObjectPtr
	}

	wafErrors := map[string][]string{}
	for i := uint64(0); i < obj.nbEntries; i++ {
		objElem := castWithOffset[wafObject](obj.value, i)
		if objElem._type != wafArrayType {
			return nil, errInvalidObjectType
		}

		errorMessage := gostringSized(cast[byte](objElem.parameterName), objElem.parameterNameLength)
		ruleIds, err := decodeRuleIdArray(objElem)
		if err != nil {
			return nil, err
		}

		wafErrors[errorMessage] = ruleIds
	}

	return wafErrors, nil
}

func decodeRuleIdArray(obj *wafObject) ([]string, error) {
	if obj._type != wafArrayType {
		return nil, errInvalidObjectType
	}

	if obj.value == 0 && obj.nbEntries > 0 {
		return nil, errNilObjectPtr
	}

	var ruleIds []string
	for i := uint64(0); i < obj.nbEntries; i++ {
		objElem := castWithOffset[wafObject](obj.value, i)
		if objElem._type != wafStringType {
			return nil, errInvalidObjectType
		}

		ruleIds = append(ruleIds, gostringSized(cast[byte](objElem.value), objElem.nbEntries))
	}

	return ruleIds, nil
}

func decodeActions(cActions uintptr, size uint64) []string {
	if size == 0 {
		return nil
	}

	actions := make([]string, size)
	for i := uint64(0); i < size; i++ {
		// This line does the following operation without casts:
		// gostring(*(cActions + i * sizeof(ptr)))
		actions[i] = gostring(*castWithOffset[*byte](cActions, i))
	}

	return actions
}
