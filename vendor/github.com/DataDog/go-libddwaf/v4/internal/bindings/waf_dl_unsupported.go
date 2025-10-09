// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Build when the target OS or architecture are not supported
//go:build (!linux && !darwin) || (!amd64 && !arm64) || go1.26 || datadog.no_waf || (!cgo && !appsec)

package bindings

import (
	"errors"

	"github.com/DataDog/go-libddwaf/v4/internal/log"
)

type WAFLib struct{}

func NewWAFLib() (*WAFLib, error) {
	return nil, errors.New("go-libddwaf is not supported on this platform")
}

func (*WAFLib) Close() error { return nil }

func (*WAFLib) GetVersion() string { return "" }

func (*WAFLib) BuilderInit(*WAFConfig) WAFBuilder { return 0 }

func (*WAFLib) BuilderAddOrUpdateConfig(WAFBuilder, string, *WAFObject, *WAFObject) bool {
	return false
}

func (*WAFLib) BuilderRemoveConfig(WAFBuilder, string) bool { return false }

func (*WAFLib) BuilderBuildInstance(WAFBuilder) WAFHandle { return 0 }

func (*WAFLib) BuilderGetConfigPaths(WAFBuilder, string) []string { return nil }

func (*WAFLib) BuilderDestroy(WAFBuilder) {}

func (*WAFLib) SetLogCb(uintptr, log.Level) {}

func (*WAFLib) Destroy(WAFHandle) {}

func (*WAFLib) KnownAddresses(WAFHandle) []string { return nil }

func (*WAFLib) KnownActions(WAFHandle) []string { return nil }

func (*WAFLib) ContextInit(WAFHandle) WAFContext { return 0 }

func (*WAFLib) ContextDestroy(WAFContext) {}

func (*WAFLib) ObjectFree(*WAFObject) {}

func (*WAFLib) Run(WAFContext, *WAFObject, *WAFObject, *WAFObject, uint64) WAFReturnCode {
	return WAFErrInternal
}

func (*WAFLib) Handle() uintptr { return 0 }
