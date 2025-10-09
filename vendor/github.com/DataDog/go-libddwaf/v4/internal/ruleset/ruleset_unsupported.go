// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(linux || darwin)

package ruleset

import (
	"errors"
	"runtime"

	"github.com/DataDog/go-libddwaf/v4/internal/bindings"
)

func DefaultRuleset(pinner *runtime.Pinner) (bindings.WAFObject, error) {
	return bindings.WAFObject{}, errors.New("the default ruleset is not available on unsupported platforms")
}
