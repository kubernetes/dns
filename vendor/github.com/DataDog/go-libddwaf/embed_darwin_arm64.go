// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin && arm64 && !go1.22

package waf

import _ "embed" // Needed for go:embed

//go:embed lib/darwin-arm64/_libddwaf.dylib
var libddwaf []byte
