// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package addresses

import (
	"github.com/DataDog/go-libddwaf/v4/timer"
)

// Scope is used to divide the time spend in go-libddwaf between multiple parts. These scopes are then fed into
// [liddwaf.RunAddressData.TimerKey] to decide where to store the time spent in the WAF.
// Time which is then added to [libddwaf.Context.Timer].
type Scope = timer.Key

const (
	RASPScope Scope = "rasp"
	WAFScope  Scope = "waf"
)

var Scopes = [...]Scope{
	RASPScope,
	WAFScope,
}
