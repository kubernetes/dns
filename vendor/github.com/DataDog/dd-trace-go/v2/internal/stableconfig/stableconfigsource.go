// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package stableconfig provides utilities to load and manage APM configurations
// loaded from YAML configuration files
package stableconfig

import (
	"os"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	"gopkg.in/yaml.v3"
)

const (
	// File paths are supported on linux only
	localFilePath   = "/etc/datadog-agent/application_monitoring.yaml"
	managedFilePath = "/etc/datadog-agent/managed/datadog-agent/stable/application_monitoring.yaml"

	// maxFileSize defines the maximum size in bytes for stable config files (4KB). This limit ensures predictable memory use and guards against malformed large files.
	maxFileSize = 4 * 1024
)

// LocalConfig holds the configuration loaded from the user-managed file.
var LocalConfig = newStableConfigSource(localFilePath, telemetry.OriginLocalStableConfig)

// ManagedConfig holds the configuration loaded from the fleet-managed file.
var ManagedConfig = newStableConfigSource(managedFilePath, telemetry.OriginManagedStableConfig)

// stableConfigSource represents a source of stable configuration loaded from a file.
type stableConfigSource struct {
	filePath string           // Path to the configuration file.
	origin   telemetry.Origin // Origin identifier for telemetry.
	config   *stableConfig    // Parsed stable configuration.
}

func (s *stableConfigSource) Get(key string) string {
	return s.config.get(key)
}

func (s *stableConfigSource) GetID() string {
	return s.config.getID()
}

// newStableConfigSource initializes a new stableConfigSource from the given file.
func newStableConfigSource(filePath string, origin telemetry.Origin) *stableConfigSource {
	return &stableConfigSource{
		filePath: filePath,
		origin:   origin,
		config:   parseFile(filePath),
	}
}

// ParseFile reads and parses the config file at the given path.
// Returns an empty config if the file doesn't exist or is invalid.
func parseFile(filePath string) *stableConfig {
	info, err := os.Stat(filePath)
	if err != nil {
		// It's expected that the stable config file may not exist; its absence is not an error.
		if !os.IsNotExist(err) {
			log.Warn("Failed to stat stable config file %q, dropping: %v", filePath, err.Error())
		}
		return emptyStableConfig()
	}

	if info.Size() > maxFileSize {
		log.Warn("Stable config file %s exceeds size limit (%d bytes > %d bytes), dropping",
			filePath, info.Size(), maxFileSize)
		return emptyStableConfig()
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		// It's expected that the stable config file may not exist; its absence is not an error.
		if !os.IsNotExist(err) {
			log.Warn("Failed to read stable config file %q, dropping: %v", filePath, err.Error())
		}
		return emptyStableConfig()
	}

	return fileContentsToConfig(data, filePath)
}

// fileContentsToConfig parses YAML data into a stableConfig struct.
// Returns an empty config if parsing fails or the data is malformed.
func fileContentsToConfig(data []byte, fileName string) *stableConfig {
	scfg := &stableConfig{}
	err := yaml.Unmarshal(data, scfg)
	if err != nil {
		log.Warn("Parsing stable config file %s failed due to error, dropping: %v", fileName, err.Error())
		return emptyStableConfig()
	}
	if scfg.Config == nil {
		scfg.Config = make(map[string]string, 0)
	}
	return scfg
}
