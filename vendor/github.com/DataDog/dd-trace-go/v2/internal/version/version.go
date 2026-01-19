// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package version

import (
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
)

// Tag specifies the current release tag. It needs to be manually
// updated. A test checks that the value of Tag never points to a
// git tag that is older than HEAD.
var Tag = "v2.5.0"

type v1version struct {
	Transitional bool
	Version      string
}

var v1Tag *v1version

// Dissected version number. Filled during init()
var (
	// Major is the current major version number
	Major int
	// Minor is the current minor version number
	Minor int
	// Patch is the current patch version number
	Patch int
	// RC is the current release candidate version number
	RC int
	// once is used to ensure that the v1 version is only found once
	once sync.Once
)

func FindV1Version() (string, bool, bool) {
	once.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		v1Tag = findV1Version(info.Deps)
	})
	if v1Tag == nil {
		return "", false, false
	}
	return v1Tag.Version, v1Tag.Transitional, true
}

func init() {
	// Check if we are using a transitional v1.74.x or later version
	vt, _, found := FindV1Version()
	if found {
		Tag = vt
	}
	v := parseVersion(Tag)
	Major, Minor, Patch, RC = v.Major, v.Minor, v.Patch, v.RC
}

func findV1Version(deps []*debug.Module) *v1version {
	var version string
	for _, dep := range deps {
		if dep.Path != "gopkg.in/DataDog/dd-trace-go.v1" {
			continue
		}
		version = dep.Version
		break
	}
	if version == "" {
		return nil
	}
	vt := &v1version{
		Version: version,
	}
	v := parseVersion(vt.Version)
	if v.Major == 1 && v.Minor >= 74 {
		vt.Transitional = true
	}
	return vt
}

type version struct {
	Major int
	Minor int
	Patch int
	RC    int
}

func parseVersion(value string) version {
	var v version

	if !semver.IsValid(value) {
		// This shouldn't happen, but it must be handled.
		// `golang.org/x/mod/semver` doesn't expose the parsed parts of the version.
		return v
	}

	i := strings.Index(value, ".")
	v.Major, _ = strconv.Atoi(value[1:i])

	value = value[i+1:]
	i = strings.Index(value, ".")
	v.Minor, _ = strconv.Atoi(value[:i])

	value = value[i+1:]
	i = strings.Index(value, "-")
	if i == -1 {
		v.Patch, _ = strconv.Atoi(value)
		return v
	}

	v.Patch, _ = strconv.Atoi(value[:i])

	value = value[i+1:]
	i = strings.Index(value, ".")
	if i == -1 {
		// Prerelease doesn't have a specific number.
		return v
	}

	value = value[i+1:]
	if len(value) == 0 {
		return v
	}
	v.RC, _ = strconv.Atoi(value)

	return v
}
