// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package orchestrion

// Orchestrion will change this at build-time
//
//orchestrion:enabled
var enabled = false

// The version of the orchestrion binary used to build the current binray, or
// blank if the current binary was not built using orchestrion.
//
//orchestrion:version
const Version = ""

// Enabled returns whether the current build was compiled with orchestrion or not.
func Enabled() bool {
	return enabled
}
