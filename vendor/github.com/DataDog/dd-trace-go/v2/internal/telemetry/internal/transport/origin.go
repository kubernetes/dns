// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package transport

// Origin describes the source of a configuration change
type Origin string

const (
	OriginDefault             Origin = "default"
	OriginCode                Origin = "code"
	OriginDDConfig            Origin = "dd_config"
	OriginEnvVar              Origin = "env_var"
	OriginRemoteConfig        Origin = "remote_config"
	OriginLocalStableConfig   Origin = "local_stable_config"
	OriginManagedStableConfig Origin = "fleet_stable_config"
)
