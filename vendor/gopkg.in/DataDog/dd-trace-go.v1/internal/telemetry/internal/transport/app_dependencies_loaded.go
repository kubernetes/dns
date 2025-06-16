// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

type AppDependenciesLoaded struct {
	Dependencies []Dependency `json:"dependencies"`
}

func (AppDependenciesLoaded) RequestType() RequestType {
	return RequestTypeAppDependenciesLoaded
}

// Dependency is a Go module on which the application depends. This information
// can be accesed at run-time through the runtime/debug.ReadBuildInfo API.
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}
