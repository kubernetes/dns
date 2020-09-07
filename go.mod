module k8s.io/dns

go 1.13

require (
	github.com/bifurcation/mint v0.0.0-20180715133206-93c51c6ce115 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/caddyserver/caddy v1.0.4
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/coredns/coredns v1.6.7
	github.com/coreos/etcd v3.3.25+incompatible
	github.com/dnstap/golang-dnstap v0.1.0 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/engine-api v0.4.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/emicklei/go-restful v1.1.4-0.20170129114709-ad3e7d5a0a11 // indirect
	github.com/farsightsec/golang-framestream v0.0.0-20190425193708-fa4b164d59b8 // indirect
	github.com/go-acme/lego v2.7.2+incompatible // indirect
	github.com/godbus/dbus v0.0.0-20181025153459-66d97aec3384 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/lint v0.0.0-20180702182130-06c8688daad7 // indirect
	github.com/howeyc/gopass v0.0.0-20160826175423-3ca23474a7c7 // indirect
	github.com/hpcloud/tail v1.0.1-0.20180514194441-a1dbeea552b7 // indirect
	github.com/juju/ratelimit v0.0.0-20151125201925-77ed1c8a0121 // indirect
	github.com/klauspost/cpuid v1.2.1 // indirect
	github.com/lucas-clemente/aes12 v0.0.0-20171027163421-cd47fb39b79f // indirect
	github.com/lucas-clemente/quic-clients v0.1.0 // indirect
	github.com/lucas-clemente/quic-go-certificates v0.0.0-20160823095156-d2f86524cced // indirect
	github.com/miekg/dns v1.1.27
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/prometheus/client_golang v1.3.0
	github.com/skynetservices/skydns v0.0.0-20191015171621-94b2ea0d8bfa
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	github.com/ugorji/go v0.0.0-20161130061742-9c7f9b7a2bc3 // indirect
	github.com/vishvananda/netlink v1.0.0
	github.com/vishvananda/netns v0.0.0-20180720170159-13995c7128cc // indirect
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v3.0.0+incompatible
	k8s.io/component-base v0.0.0
	k8s.io/kubernetes v0.0.0-00010101000000-000000000000
	k8s.io/utils v0.0.0-20200716102541-988ee3149bb2
)

replace (
	github.com/coreos/bbolt => go.etcd.io/bbolt v1.3.5
        github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180409111510-d1b7d058aa2a
	k8s.io/api => k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190620085554-14e95df34f1f
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190620085212-47dc9a115b18
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190620085706-2090e6d8f84c
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190620090043-8301c0bda1f0
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190620090013-c9a0fc045dc1
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190612205613-18da4a14b22b
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190620085130-185d68e6e6ea
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190531030430-6117653b35f1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20190620090116-299a7b270edc
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190620085325-f29e2b4a4f84
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20190620085942-b7f18460b210
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20190620085809-589f994ddf7f
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20190620085912-4acac5405ec6
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20190620085838-f1cb295a73c9
	k8s.io/kubernetes => k8s.io/kubernetes v1.15.0
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190620090156-2138f2c9de18
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190620085625-3b22d835f165
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190620085408-1aef9010884e
)
