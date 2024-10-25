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
	etcdImage = "quay.io/coreos/etcd:v3.5.16"
	// TODO remove hyperkube, it is deprecated
	hyperkubeImage = "registry.k8s.io/hyperkube:v1.18.20"
	// TODO Fix kubedns e2e test that uses this image, stops working after 1.14.10
	dnsmasqImage = "registry.k8s.io/k8s-dns-dnsmasq-amd64:1.14.10"
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
	DnsmasqImage   string
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
		DnsmasqImage:   dnsmasqImage,
		ClusterIpRange: "10.0.0.0/24",
	}

	return ret
}
