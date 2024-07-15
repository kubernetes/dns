module k8s.io/dns

go 1.22
toolchain go1.22.5

require (
	github.com/coredns/caddy v1.1.1
	github.com/coredns/coredns v1.10.0
	github.com/coreos/etcd v3.3.13+incompatible
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/miekg/dns v1.1.61
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.33.1
	github.com/prometheus/client_golang v1.19.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.9.0
	github.com/vishvananda/netlink v1.1.0
	go.etcd.io/etcd/api/v3 v3.5.14
	go.etcd.io/etcd/client/v2 v2.305.14
	go.etcd.io/etcd/client/v3 v3.5.14
	golang.org/x/net v0.26.0
	k8s.io/api v0.25.0
	k8s.io/apimachinery v0.25.0
	k8s.io/client-go v0.24.7
	k8s.io/component-base v0.24.7
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubernetes v1.30.2
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b
)

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.0.0-20211129110424-6491aa3bf583 // indirect
	github.com/DataDog/datadog-go v4.8.2+incompatible // indirect
	github.com/DataDog/datadog-go/v5 v5.0.2 // indirect
	github.com/DataDog/sketches-go v1.2.1 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dnstap/golang-dnstap v0.4.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/farsightsec/golang-framestream v0.3.0 // indirect
	github.com/flynn/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/opentracing-contrib/go-observer v0.0.0-20170622124052-a52f23424492 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/openzipkin-contrib/zipkin-go-opentracing v0.4.5 // indirect
	github.com/openzipkin/zipkin-go v0.4.0 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/tinylib/msgp v1.1.2 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.14 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/mod v0.18.0 // indirect
	golang.org/x/oauth2 v0.16.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/term v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.22.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.41.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	// pinned latest version for vulnerability fixes
	// this one is used by coredns
	// if coredns starts using >= v0.14.0 this pinned version can be removed
	github.com/apache/thrift => github.com/apache/thrift v0.14.0

	k8s.io/api => k8s.io/api v0.24.7
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.7
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.7
	k8s.io/apiserver => k8s.io/apiserver v0.24.7
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.7
	k8s.io/client-go => k8s.io/client-go v0.24.7
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.7
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.7
	k8s.io/code-generator => k8s.io/code-generator v0.24.7
	k8s.io/component-base => k8s.io/component-base v0.24.7
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.7
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.7
	k8s.io/cri-api => k8s.io/cri-api v0.24.7
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.7
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.7
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.7
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.7
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.7
	k8s.io/kubectl => k8s.io/kubectl v0.24.7
	k8s.io/kubelet => k8s.io/kubelet v0.24.7
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.7
	k8s.io/metrics => k8s.io/metrics v0.24.7
	k8s.io/mount-utils => k8s.io/mount-utils v0.24.7
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.7
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.7
)
