// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libddwaf

import (
	"errors"
	"fmt"
)

// Diagnostics stores the information as provided by the WAF about WAF rules parsing and loading. It
// is returned by [Builder.AddOrUpdateConfig].
type Diagnostics struct {
	// Rules contains information about the loaded rules.
	Rules *Feature
	// CustomRules contains information about the loaded custom rules.
	CustomRules *Feature
	// Actions contains information about the loaded actions.
	Actions *Feature
	// Exclusions contains information about the loaded exclusions.
	Exclusions *Feature
	// RulesOverrides contains information about the loaded rules overrides.
	RulesOverrides *Feature
	// RulesData contains information about the loaded rules data.
	RulesData *Feature
	// ExclusionData contains information about the loaded exclusion data.
	ExclusionData *Feature
	// Processors contains information about the loaded processors.
	Processors *Feature
	// Scanners contains information about the loaded scanners.
	Scanners *Feature
	// Version is the version of the parsed ruleset if available.
	Version string
}

// EachFeature calls the provided callback for each (non-nil) feature in this diagnostics object.
func (d *Diagnostics) EachFeature(cb func(string, *Feature)) {
	byName := map[string]*Feature{
		"rules":           d.Rules,
		"custom_rules":    d.CustomRules,
		"actions":         d.Actions,
		"exclusions":      d.Exclusions,
		"rules_overrides": d.RulesOverrides,
		"rules_data":      d.RulesData,
		"exclusion_data":  d.ExclusionData,
		"processors":      d.Processors,
		"scanners":        d.Scanners,
	}

	for name, feat := range byName {
		if feat != nil {
			cb(name, feat)
		}
	}
}

// TopLevelError returns the list of top-level errors reported by the WAF on any of the Diagnostics
// entries, rolled up into a single error value. Returns nil if no top-level errors were reported.
// Individual, item-level errors might still exist.
func (d *Diagnostics) TopLevelError() error {
	var err error
	d.EachFeature(func(name string, feat *Feature) {
		if feat.Error != "" {
			err = errors.Join(err, fmt.Errorf("%q: %s", name, feat.Error))
		}
	})
	return err
}

// Feature stores the information as provided by the WAF about loaded and failed
// rules for a specific feature of the WAF ruleset.
type Feature struct {
	// Errors is a map of parsing errors to a list of unique identifiers from the elements which
	// failed loading due to this specific error.
	Errors map[string][]string
	// Warnings is a map of parsing warnings to a list of unique identifiers from the elements which
	// resulted in this specific warning.
	Warnings map[string][]string
	// Error is the single error which prevented parsing this feature.
	Error string
	// Loaded is a list of the unique identifiers from successfully loaded elements.
	Loaded []string
	// Failed is a list of the unique identifiers from the elements which couldn't be loaded.
	Failed []string
	// Skipped is a list of the unique identifiers from the elements which were skipped.
	Skipped []string
}
