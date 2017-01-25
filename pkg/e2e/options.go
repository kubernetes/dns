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

package e2e

const (
	etcdImage      = "quay.io/coreos/etcd:v3.0.14"
	hyperkubeImage = "gcr.io/google_containers/hyperkube:v1.5.1"
)

type Options struct {
	Prefix  string
	Docker  string
	Kubectl string

	BaseDir string
	WorkDir string

	EtcdImage      string
	HyperkubeImage string
	ClusterIpRange string
}

// DefaultOptions to use to run the e2e test.
func DefaultOptions(baseDir string, workDir string) Options {
	ret := Options{
		Prefix:  "xxx",
		Kubectl: "kubectl",

		BaseDir: baseDir,
		WorkDir: workDir,

		Docker:         "docker",
		EtcdImage:      etcdImage,
		HyperkubeImage: hyperkubeImage,
		ClusterIpRange: "10.0.0.0/24",
	}

	return ret
}
