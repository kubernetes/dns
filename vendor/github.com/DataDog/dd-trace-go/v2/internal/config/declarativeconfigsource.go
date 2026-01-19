// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package config

import (
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
)

const (
	// File paths are supported on linux only
	localFilePath   = "/etc/datadog-agent/application_monitoring.yaml"
	managedFilePath = "/etc/datadog-agent/managed/datadog-agent/stable/application_monitoring.yaml"

	// maxFileSize defines the maximum size in bytes for declarative config files (4KB). This limit ensures predictable memory use and guards against malformed large files.
	maxFileSize = 4 * 1024
)

// declarativeConfigSource represents a source of declarative configuration loaded from a file.
type declarativeConfigSource struct {
	filePath    string             // Path to the configuration file.
	originValue telemetry.Origin   // Origin identifier for telemetry.
	config      *declarativeConfig // Parsed declarative configuration.
}

func (d *declarativeConfigSource) get(key string) string {
	return d.config.get(normalizeKey(key))
}

func (d *declarativeConfigSource) getID() string {
	return d.config.getID()
}

func (d *declarativeConfigSource) origin() telemetry.Origin {
	return d.originValue
}

// newDeclarativeConfigSource initializes a new declarativeConfigSource from the given file.
func newDeclarativeConfigSource(filePath string, origin telemetry.Origin) *declarativeConfigSource {
	return &declarativeConfigSource{
		filePath:    filePath,
		originValue: origin,
		config:      parseFile(filePath),
	}
}

// ParseFile reads and parses the config file at the given path.
// Returns an empty config if the file doesn't exist or is invalid.
func parseFile(filePath string) *declarativeConfig {
	info, err := os.Stat(filePath)
	if err != nil {
		// It's expected that the declarative config file may not exist; its absence is not an error.
		if !os.IsNotExist(err) {
			log.Warn("Failed to stat declarative config file %q, dropping: %v", filePath, err.Error())
		}
		return emptyDeclarativeConfig()
	}

	if info.Size() > maxFileSize {
		log.Warn("Declarative config file %s exceeds size limit (%d bytes > %d bytes), dropping",
			filePath, info.Size(), maxFileSize)
		return emptyDeclarativeConfig()
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		// It's expected that the declarative config file may not exist; its absence is not an error.
		if !os.IsNotExist(err) {
			log.Warn("Failed to read declarative config file %q, dropping: %v", filePath, err.Error())
		}
		return emptyDeclarativeConfig()
	}

	return fileContentsToConfig(data, filePath)
}

// fileContentsToConfig parses YAML data into a declarativeConfig struct.
// Returns an empty config if parsing fails or the data is malformed.
func fileContentsToConfig(data []byte, fileName string) *declarativeConfig {
	dc := &declarativeConfig{}
	err := yaml.Unmarshal(data, dc)
	if err != nil {
		log.Warn("Parsing declarative config file %s failed due to error, dropping: %v", fileName, err.Error())
		return emptyDeclarativeConfig()
	}
	if dc.Config == nil {
		dc.Config = make(map[string]string)
	}
	return dc
}
