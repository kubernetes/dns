module k8s.io/dns

go 1.14

require (
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/caddyserver/caddy v1.0.5
	github.com/coredns/coredns v1.6.10-0.20200611141247-86df1282cb0a
	github.com/coreos/bbolt v1.3.4 // indirect
	github.com/coreos/etcd v3.3.22+incompatible
	// TODO(peter.novotnak@reddit.com) Unsafe fd reuse allowed here. Should upgrade
	github.com/coreos/go-systemd v0.0.0-20180409111510-d1b7d058aa2a // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/godbus/dbus v0.0.0-20181025153459-66d97aec3384
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/hpcloud/tail v1.0.1-0.20180514194441-a1dbeea552b7 // indirect
	github.com/miekg/dns v1.1.29
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/client_golang v1.6.0
	github.com/skynetservices/skydns v0.0.0-20191015171621-94b2ea0d8bfa
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.5.1
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/vishvananda/netlink v1.0.0
	github.com/vishvananda/netns v0.0.0-20180720170159-13995c7128cc // indirect
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7 // indirect
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/apiserver v0.0.0-20181001130900-c0373f43cffe
	k8s.io/client-go v0.18.3
	k8s.io/component-base v0.18.3
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.18.3
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
)

replace (
	github.com/caddyserver/caddy => github.com/caddyserver/caddy v1.0.5
	github.com/coreos/bbolt v1.3.4 => go.etcd.io/bbolt v1.3.4
	github.com/godbus/dbus => github.com/godbus/dbus v0.0.0-20190422162347-ade71ed3457e
	github.com/mholt/caddy => github.com/caddyserver/caddy v1.0.5
	k8s.io/client-go/pkg/api => k8s.io/api v0.0.0-20190118113203-912cbe2bfef3
)
