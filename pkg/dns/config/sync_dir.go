/*
Copyright 2017 The Kubernetes Authors.

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
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/util/clock"
)

// NewFileSync returns a Sync that scans the given dir periodically for config data
func NewFileSync(dir string, period time.Duration) Sync {
	return newSync(newFileSyncSource(dir, period, clock.RealClock{}))
}

// newFileSyncSource returns a syncSource that scans the given dir periodically as determined by the specified clock
func newFileSyncSource(dir string, period time.Duration, clock clock.Clock) syncSource {
	return &kubeFileSyncSource{
		dir:     dir,
		clock:   clock,
		period:  period,
		channel: make(chan syncResult),
	}
}

type kubeFileSyncSource struct {
	dir     string
	clock   clock.Clock
	period  time.Duration
	channel chan syncResult
}

var _ syncSource = (*kubeFileSyncSource)(nil)

func (syncSource *kubeFileSyncSource) Once() (syncResult, error) {
	return syncSource.load()
}

func (syncSource *kubeFileSyncSource) Periodic() <-chan syncResult {
	// TODO: drive via inotify?
	go func() {
		ticker := syncSource.clock.NewTicker(syncSource.period).C()
		for {
			if result, err := syncSource.load(); err != nil {
				glog.Errorf("Error loading config from %s: %v", syncSource.dir, err)
			} else {
				syncSource.channel <- result
			}
			<-ticker
		}
	}()
	return syncSource.channel
}

func (syncSource *kubeFileSyncSource) load() (syncResult, error) {
	hasher := sha256.New()
	data := map[string]string{}
	err := filepath.Walk(syncSource.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// special case for the root
		if path == syncSource.dir {
			if info.IsDir() {
				return nil
			}
			return fmt.Errorf("config path %q is not a directory", path)
		}

		// don't recurse
		if info.IsDir() {
			return filepath.SkipDir
		}
		// skip hidden files
		filename := filepath.Base(path)
		if strings.HasPrefix(filename, ".") {
			return nil
		}
		filedata, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		if !utf8.Valid(filedata) {
			return fmt.Errorf("non-utf8 data in %s", path)
		}

		// Add data to version hash
		hasher.Write([]byte(filename))
		hasher.Write([]byte{0})
		hasher.Write(filedata)
		hasher.Write([]byte{0})

		// Add data to map
		data[filename] = string(filedata)

		return nil
	})
	if err != nil {
		return syncResult{}, err
	}

	// compute a version string from the hashed data
	version := ""
	if len(data) > 0 {
		version = fmt.Sprintf("%x", hasher.Sum(nil))
	}

	return syncResult{Version: version, Data: data}, nil
}
