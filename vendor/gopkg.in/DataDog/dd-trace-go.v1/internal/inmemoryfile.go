// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

//go:build !linux

package internal

func CreateMemfd(name string, data []byte) (int, error) {
	return 0, nil
}
