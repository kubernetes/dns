module k8s.io/dns

go 1.13

require (
	github.com/coredns/caddy v1.1.0
	github.com/coredns/coredns v1.8.3
	github.com/coreos/etcd v3.3.25+incompatible
	github.com/miekg/dns v1.1.42
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/prometheus/client_golang v1.11.0
	github.com/skynetservices/skydns v0.0.0-20191015171621-94b2ea0d8bfa
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/component-base v0.21.1
	k8s.io/klog/v2 v2.8.0
	k8s.io/kubernetes v1.19.12
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
)

replace (
	// Needed to pin old version for skydns.
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180409111510-d1b7d058aa2a

	k8s.io/api => k8s.io/api v0.19.12
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.12
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.12
	k8s.io/apiserver => k8s.io/apiserver v0.19.12
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.19.12
	k8s.io/client-go => k8s.io/client-go v0.19.12
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.19.12
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.19.12
	k8s.io/code-generator => k8s.io/code-generator v0.19.12
	k8s.io/component-base => k8s.io/component-base v0.19.12
	k8s.io/component-helpers => k8s.io/component-helpers v0.19.12
	k8s.io/controller-manager => k8s.io/controller-manager v0.19.12
	k8s.io/cri-api => k8s.io/cri-api v0.19.12
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.19.12
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.12
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.19.12
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.19.12
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.19.12
	k8s.io/kubectl => k8s.io/kubectl v0.19.12
	k8s.io/kubelet => k8s.io/kubelet v0.19.12
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.19.12
	k8s.io/metrics => k8s.io/metrics v0.19.12
	k8s.io/mount-utils => k8s.io/mount-utils v0.19.12
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.19.12
)
