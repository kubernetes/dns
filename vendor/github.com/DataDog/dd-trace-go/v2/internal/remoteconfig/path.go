// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

package remoteconfig

import (
	"fmt"
	"strings"
)

type (
	Path struct {
		// The source of the config. Either "datadog/<org_id>", or "employee"
		Source Source
		// The name of the product that produced this config (e.g, "ASM_DD").
		Product string
		// The ID of the config (e.g, "blocked_ips")
		ConfigID string
		// The name of the config object (e.g, "config")
		Name string
	}
	Source interface {
		fmt.Stringer
		isSource()
	}
	DatadogSource struct {
		source
		OrgID string
	}
	EmployeeSource struct {
		source
	}
	source struct{}
)

// ParsePath parses a remote config target file path into its components.
func ParsePath(filename string) (Path, bool) {
	// See: https://docs.google.com/document/d/1u_G7TOr8wJX0dOM_zUDKuRJgxoJU_hVTd5SeaMucQUs/edit?tab=t.0#bookmark=id.ew0e2fwzf8p7
	parts := strings.Split(filename, "/")
	if len(parts) < 4 {
		return Path{}, false
	}

	var source Source
	switch parts[0] {
	case "datadog":
		orgID := parts[1]
		if orgID == "" {
			// Invalid org ID (empty)...
			return Path{}, false
		}
		for _, c := range orgID {
			if c < '0' || c > '9' {
				// Invalid org ID (non-numeric)...
				return Path{}, false
			}
		}
		source = DatadogSource{OrgID: orgID}
		parts = parts[2:]
	case "employee":
		source = EmployeeSource{}
		parts = parts[1:]
	default:
		// Invalid source...
		return Path{}, false
	}

	if len(parts) != 3 {
		// Invalid number of parts...
		return Path{}, false
	}

	product, configID, name := parts[0], parts[1], parts[2]
	if product == "" || configID == "" || name == "" {
		// Invalid product, config ID, or name (none of these can be empty)...
		return Path{}, false
	}

	return Path{Source: source, Product: product, ConfigID: configID, Name: name}, true
}

func (p Path) String() string {
	return p.Source.String() + "/" + p.Product + "/" + p.ConfigID + "/" + p.Name
}

func (s DatadogSource) String() string {
	return fmt.Sprintf("datadog/%s", s.OrgID)
}

func (s EmployeeSource) String() string {
	return "employee"
}

func (source) isSource() {}
