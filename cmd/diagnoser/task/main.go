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

package task

import (
	"k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/dns/cmd/diagnoser/flags"
)

var bundle []Task

// Task represents the checks to be done
type Task interface {
	Run(*flags.Options, v1.CoreV1Interface) error
}

// register adds a task to the set
func register(t Task) {
	bundle = append(bundle, t)
}

// Bundle returns the current set
func Bundle() []Task {
	return bundle
}
