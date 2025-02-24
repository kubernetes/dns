// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	internal "github.com/DataDog/appsec-internal-go/appsec"

	"gopkg.in/DataDog/dd-trace-go.v1/internal/log"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/remoteconfig"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/telemetry"
)

func init() {
	registerAppConfigTelemetry()
}

// Register the global app telemetry configuration.
func registerAppConfigTelemetry() {
	registerSCAAppConfigTelemetry(telemetry.GlobalClient)
}

// Register the global app telemetry configuration related to the Software Composition Analysis (SCA) product.
// Report over telemetry whether SCA's enablement env var was set or not along with its value. Nothing is reported in
// case of an error or if the env var is not set.
func registerSCAAppConfigTelemetry(client telemetry.Client) {
	val, defined, err := parseBoolEnvVar(EnvSCAEnabled)
	if err != nil {
		log.Error("appsec: %v", err)
		return
	}
	if defined {
		client.RegisterAppConfig(EnvSCAEnabled, val, telemetry.OriginEnvVar)
	}
}

// The following environment variables dictate the enablement of different the ASM products.
const (
	// EnvEnabled controls ASM Threats Protection's enablement.
	EnvEnabled = "DD_APPSEC_ENABLED"
	// EnvSCAEnabled controls ASM Software Composition Analysis (SCA)'s enablement.
	EnvSCAEnabled = "DD_APPSEC_SCA_ENABLED"
)

// StartOption is used to customize the AppSec configuration when invoked with appsec.Start()
type StartOption func(c *Config)

// Config is the AppSec configuration.
type Config struct {
	// rules loaded via the env var DD_APPSEC_RULES. When not set, the builtin rules will be used
	// and live-updated with remote configuration.
	RulesManager *RulesManager
	// Maximum WAF execution time
	WAFTimeout time.Duration
	// AppSec trace rate limit (traces per second).
	TraceRateLimit int64
	// Obfuscator configuration
	Obfuscator internal.ObfuscatorConfig
	// APISec configuration
	APISec internal.APISecConfig
	// RC is the remote configuration client used to receive product configuration updates. Nil if RC is disabled (default)
	RC   *remoteconfig.ClientConfig
	RASP bool
	// SupportedAddresses are the addresses that the AppSec listener will bind to.
	SupportedAddresses AddressSet
}

// AddressSet is a set of WAF addresses.
type AddressSet map[string]struct{}

func NewAddressSet(addrs []string) AddressSet {
	set := make(AddressSet, len(addrs))
	for _, addr := range addrs {
		set[addr] = struct{}{}
	}
	return set
}

// AnyOf returns true if any of the addresses in the set are in the given list.
func (set AddressSet) AnyOf(anyOf ...string) bool {
	for _, addr := range anyOf {
		if _, ok := set[addr]; ok {
			return true
		}
	}

	return false
}

// WithRCConfig sets the AppSec remote config client configuration to the specified cfg
func WithRCConfig(cfg remoteconfig.ClientConfig) StartOption {
	return func(c *Config) {
		c.RC = &cfg
	}
}

// IsEnabled returns true when appsec is enabled by the environment variable DD_APPSEC_ENABLED (as of strconv's boolean
// parsing rules). When false, it also returns whether the env var was actually set or not.
// In case of a parsing error, it returns a detailed error.
func IsEnabled() (enabled bool, set bool, err error) {
	return parseBoolEnvVar(EnvEnabled)
}

// Return true when the given environment variable is defined and set to true (as of strconv's
// parsing rules). When false, it also returns whether the env var was actually set or not.
// In case of a parsing error, it returns a detailed error.
func parseBoolEnvVar(env string) (enabled bool, set bool, err error) {
	str, set := os.LookupEnv(env)
	if str == "" {
		return false, set, nil
	} else if enabled, err = strconv.ParseBool(str); err != nil {
		return false, set, fmt.Errorf("could not parse %s value `%s` as a boolean value", env, str)
	}

	return enabled, set, nil
}

// NewConfig returns a fresh appsec configuration read from the env
func NewConfig() (*Config, error) {
	rules, err := internal.RulesFromEnv()
	if err != nil {
		return nil, err
	}

	r, err := NewRulesManager(rules)
	if err != nil {
		return nil, err
	}

	return &Config{
		RulesManager:   r,
		WAFTimeout:     internal.WAFTimeoutFromEnv(),
		TraceRateLimit: int64(internal.RateLimitFromEnv()),
		Obfuscator:     internal.NewObfuscatorConfig(),
		APISec:         internal.NewAPISecConfig(),
		RASP:           internal.RASPEnabled(),
	}, nil
}
