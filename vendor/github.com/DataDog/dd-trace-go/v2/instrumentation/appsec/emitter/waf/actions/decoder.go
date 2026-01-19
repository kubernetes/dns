// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package actions

import "fmt"

const errDecodingFmt = "decoding failed for %q"

func decodeInt(p map[string]any, k string) (int, error) {
	v, ok := p[k].(uint64)
	if !ok {
		return 0, fmt.Errorf(errDecodingFmt, k)
	}
	return int(v), nil
}

func decodeStr(p map[string]any, k string) (string, error) {
	v, ok := p[k].(string)
	if !ok {
		return "", fmt.Errorf(errDecodingFmt, k)
	}
	return v, nil
}
