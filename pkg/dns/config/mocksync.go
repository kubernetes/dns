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

// MockSync is a testing mock.
type MockSync struct {
	// Config that will be returned from Once().
	Config *Config
	// Error that will be returned from Once().
	Error error

	// Chan to send new configurations on.
	Chan chan *Config
}

var _ Sync = (*MockSync)(nil)

func NewMockSync(config *Config, error error) *MockSync {
	return &MockSync{
		Config: config,
		Error:  error,
		Chan:   make(chan *Config),
	}
}

func (sync *MockSync) Once() (*Config, error) {
	return sync.Config, sync.Error
}

func (sync *MockSync) Periodic() <-chan *Config {
	return sync.Chan
}

type mockSource struct {
	result syncResult
	err    error
	ch     chan syncResult
}

func newMockSource(result syncResult, err error) *mockSource {
	return &mockSource{
		result: result,
		err:    err,
		ch:     make(chan syncResult),
	}
}

var _ syncSource = (*mockSource)(nil)

func (m *mockSource) Once() (syncResult, error) {
	return m.result, m.err
}
func (m *mockSource) Periodic() <-chan syncResult {
	return m.ch
}
