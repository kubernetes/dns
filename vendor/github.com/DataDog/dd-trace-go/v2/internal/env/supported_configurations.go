// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package env

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/internal/log"
)

// SupportedConfiguration represents the content of the supported_configurations.json file.
type SupportedConfiguration struct {
	SupportedConfigurations map[string][]string `json:"supportedConfigurations"`
	Aliases                 map[string][]string `json:"aliases"`
}

var (
	configFilePath string
	once           sync.Once
	mu             sync.Mutex
	skipLock       bool
)

// getConfigFilePath returns the path to the supported_configurations.json file
// in the same directory as this Go file. The path is calculated once and cached.
//
// This needs to be computed, if we use a relative path, the file will be read
// from current working directory of the running process, not the directory of
// this file.
func getConfigFilePath() string {
	once.Do(func() {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		configFilePath = filepath.Join(dir, "supported_configurations.json")
	})
	return configFilePath
}

// addSupportedConfigurationToFile adds a supported configuration to the json file.
// it is used only in testing mode.
//
// It reads the json file, adds the new configuration, and writes it back to the file.
// The JSON output will have sorted keys since Go's json.Marshal sorts map keys automatically.
//
// When called with DD_CONFIG_INVERSION_UNKNOWN nothing is done as it is a special value
// used in a unit test to verify the behavior of unknown env var.
func addSupportedConfigurationToFile(name string) {
	mu.Lock()
	defer mu.Unlock()

	filePath := getConfigFilePath()

	cfg, err := readSupportedConfigurations(filePath)
	if err != nil {
		log.Error("config: failed to read supported configurations: %s", err.Error())
		return
	}

	if _, ok := cfg.SupportedConfigurations[name]; !ok {
		cfg.SupportedConfigurations[name] = []string{"A"}
	}

	if err := writeSupportedConfigurations(filePath, cfg); err != nil {
		log.Error("config: failed to write supported configurations: %s", err.Error())
	}
}

func readSupportedConfigurations(filePath string) (*SupportedConfiguration, error) {
	// read the json file
	jsonFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open supported_configurations.json: %w", err)
	}

	var cfg SupportedConfiguration
	if err := json.Unmarshal(jsonFile, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SupportedConfiguration: %w", err)
	}
	return &cfg, nil
}

func writeSupportedConfigurations(filePath string, cfg *SupportedConfiguration) error {
	// write the json file - Go's json.MarshalIndent automatically sorts map keys
	jsonFile, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal SupportedConfiguration: %w", err)
	}

	if err := os.WriteFile(filePath, jsonFile, 0644); err != nil {
		return fmt.Errorf("failed to write supported_configurations.json: %w", err)
	}

	return nil
}
