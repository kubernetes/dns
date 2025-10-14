// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin

package ruleset

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"runtime"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
	"github.com/DataDog/go-libddwaf/v4/json"
) // For go:embed

//go:embed recommended.json.gz
var defaultRuleset []byte

func DefaultRuleset(pinner *runtime.Pinner) (bindings.WAFObject, error) {
	gz, err := gzip.NewReader(bytes.NewReader(defaultRuleset))
	if err != nil {
		return bindings.WAFObject{}, err
	}

	dec := json.NewDecoder(gz, pinner)

	var ruleset bindings.WAFObject
	if err := dec.Decode(&ruleset); err != nil {
		return bindings.WAFObject{}, err
	}
	return ruleset, nil
}
