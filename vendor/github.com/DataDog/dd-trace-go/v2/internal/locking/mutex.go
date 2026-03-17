// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package locking

type TryLocker interface {
	TryLock() bool
	Unlock()
}

type TryRLocker interface {
	TryRLock() bool
	RUnlock()
}
