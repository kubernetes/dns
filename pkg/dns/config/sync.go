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
	config, _, err := sync.processUpdate(result)
	return config, err
}

func (sync *kubeSync) Periodic() <-chan *Config {
	go func() {
		resultChan := sync.syncSource.Periodic()
		for {
			syncResult := <-resultChan
			config, changed, err := sync.processUpdate(syncResult)
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

func (sync *kubeSync) processUpdate(result syncResult) (config *Config, changed bool, err error) {
	glog.V(4).Infof("processUpdate %+v", result)

	if result.Version != sync.latestVersion {
		glog.V(3).Infof("Updating config to version %v (was %v)", result.Version, sync.latestVersion)
		changed = true
		sync.latestVersion = result.Version
	} else {
		glog.V(4).Infof("Config was unchanged (version %v)", sync.latestVersion)
		return
	}

	if result.Version == "" && len(result.Data) == 0 {
		config = NewDefaultConfig()
		return
	}

	config = &Config{}

	if err = sync.updateFederations(result.Data, config); err != nil {
		glog.Errorf("Invalid configuration, ignoring update")
		return
	}

	if err = config.Validate(); err != nil {
		glog.Errorf("Invalid onfiguration: %v (value was %+v), ignoring update", err, config)
		config = nil
		return
	}

	return
}

func (sync *kubeSync) updateFederations(data map[string]string, config *Config) (err error) {
	if flagValue, ok := data["federations"]; ok {
		config.Federations = make(map[string]string)
		if err = fed.ParseFederationsFlag(flagValue, config.Federations); err != nil {
			glog.Errorf("Invalid federations value: %v (value was %q)",
				err, data["federations"])
			return
		}
		glog.V(2).Infof("Updated federations to %v", config.Federations)
	} else {
		glog.V(2).Infof("No federations present")
	}

	return
}
