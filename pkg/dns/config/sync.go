/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/json"

	fed "k8s.io/dns/pkg/dns/federation"

	"github.com/golang/glog"
)

// Sync manages synchronization of the config map.
type Sync interface {
	// Once does a blocking synchronization of the config map. If the
	// ConfigMap fails to validate, this method will return nil, err.
	Once() (*Config, error)

	// Start a periodic synchronization of the configuration map. When a
	// successful configuration map update is detected, the
	// configuration will be sent to the channel.
	//
	// It is an error to call this more than once.
	Periodic() <-chan *Config
}

type syncResult struct {
	Version string
	Data    map[string]string
}

type syncSource interface {
	Once() (syncResult, error)
	Periodic() <-chan syncResult
}

// NewSync uses the given source to provide config
func newSync(source syncSource) Sync {
	sync := &kubeSync{
		syncSource: source,
		channel:    make(chan *Config),
	}
	return sync
}

// kubeSync implements Sync using the provided syncSource
type kubeSync struct {
	syncSource syncSource

	channel chan *Config

	latestVersion string
}

var _ Sync = (*kubeSync)(nil)

func (sync *kubeSync) Once() (*Config, error) {
	result, err := sync.syncSource.Once()
	if err != nil {
		return nil, err
	}
	// Always build a config object so we return non-nil
	config, _, err := sync.processUpdate(result, true)
	return config, err
}

func (sync *kubeSync) Periodic() <-chan *Config {
	go func() {
		resultChan := sync.syncSource.Periodic()
		for {
			syncResult := <-resultChan
			config, changed, err := sync.processUpdate(syncResult, false)
			if err != nil {
				continue
			}
			if !changed {
				continue
			}
			sync.channel <- config
		}
	}()
	return sync.channel
}

func (sync *kubeSync) processUpdate(result syncResult, buildUnchangedConfig bool) (config *Config, changed bool, err error) {
	glog.V(4).Infof("processUpdate %+v", result)

	if result.Version != sync.latestVersion {
		glog.V(3).Infof("Updating config to version %v (was %v)",
			result.Version, sync.latestVersion)
		changed = true
		sync.latestVersion = result.Version
	} else {
		glog.V(4).Infof("Config was unchanged (version %v)", sync.latestVersion)
		// short-circuit if we haven't been asked to build an unchanged config object
		if !buildUnchangedConfig {
			return
		}
	}

	if result.Version == "" && len(result.Data) == 0 {
		config = NewDefaultConfig()
		return
	}

	config = &Config{}

	for key, updateFn := range map[string]fieldUpdateFn{
		"federations":         updateFederations,
		"stubDomains":         updateStubDomains,
		"upstreamNameservers": updateUpstreamNameservers,
	} {
		value, ok := result.Data[key]
		if !ok {
			glog.V(3).Infof("No %v present", key)
			continue
		}

		if err = updateFn(key, value, config); err != nil {
			glog.Errorf("Invalid configuration for %v, ignoring update: %v", key, err)
			return
		}
	}

	if err = config.Validate(); err != nil {
		glog.Errorf("Invalid configuration: %v (value was %+v), ignoring update", err, config)
		config = nil
		return
	}

	return
}

type fieldUpdateFn func(key string, data string, config *Config) error

func updateFederations(key string, value string, config *Config) error {
	config.Federations = make(map[string]string)
	if err := fed.ParseFederationsFlag(value, config.Federations); err != nil {
		glog.Errorf("Invalid federations value: %v (value was %q)", err, value)
		return err
	}
	glog.V(2).Infof("Updated %v to %v", key, config.Federations)

	return nil
}

func updateStubDomains(key string, value string, config *Config) error {
	config.StubDomains = make(map[string][]string)
	if err := json.Unmarshal([]byte(value), &config.StubDomains); err != nil {
		glog.Errorf("Invalid JSON %q: %v", value, err)
		return err
	}
	glog.V(2).Infof("Updated %v to %v", key, config.StubDomains)

	return nil
}

func updateUpstreamNameservers(key string, value string, config *Config) error {
	if err := json.Unmarshal([]byte(value), &config.UpstreamNameservers); err != nil {
		glog.Errorf("Invalid JSON %q: %v", value, err)
		return err
	}
	glog.V(2).Infof("Updated %v to %v", key, config.UpstreamNameservers)

	return nil
}
