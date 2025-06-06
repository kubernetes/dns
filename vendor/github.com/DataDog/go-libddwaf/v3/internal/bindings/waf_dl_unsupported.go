// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Build when the target OS or architecture are not supported
//go:build (!linux && !darwin) || (!amd64 && !arm64) || go1.25 || datadog.no_waf || (!cgo && !appsec)

package bindings

import (
	"errors"
)

type WafDl struct{}

func NewWafDl() (*WafDl, error) {
	return nil, errors.New("WAF is not supported on this platform")
}

func (waf *WafDl) WafGetVersion() string {
	return ""
}

func (waf *WafDl) WafInit(*WafObject, *WafConfig, *WafObject) WafHandle {
	return 0
}

func (waf *WafDl) WafUpdate(WafHandle, *WafObject, *WafObject) WafHandle {
	return 0
}

func (waf *WafDl) WafDestroy(WafHandle) {
}

func (waf *WafDl) WafKnownAddresses(WafHandle) []string {
	return nil
}

func (waf *WafDl) WafKnownActions(WafHandle) []string {
	return nil
}

func (waf *WafDl) WafContextInit(WafHandle) WafContext {
	return 0
}

func (waf *WafDl) WafContextDestroy(WafContext) {
}

func (waf *WafDl) WafResultFree(*WafResult) {
}

func (waf *WafDl) WafObjectFree(*WafObject) {
}

func (waf *WafDl) WafRun(WafContext, *WafObject, *WafObject, *WafResult, uint64) WafReturnCode {
	return WafErrInternal
}
