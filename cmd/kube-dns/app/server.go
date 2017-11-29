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

package app

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/skynetservices/skydns/metrics"
	"github.com/skynetservices/skydns/server"
	"github.com/spf13/pflag"

	"k8s.io/dns/cmd/kube-dns/app/options"
	"k8s.io/dns/pkg/dns"
	dnsconfig "k8s.io/dns/pkg/dns/config"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/dns/pkg/version"
)

type KubeDNSServer struct {
	// DNS domain name.
	domain         string
	healthzPort    int
	dnsBindAddress string
	dnsPort        int
	nameServers    string
	kd             *dns.KubeDNS
}

func NewKubeDNSServerDefault(config *options.KubeDNSConfig) *KubeDNSServer {
	kubeClient, err := newKubeClient(config)
	if err != nil {
		glog.Fatalf("Failed to create a kubernetes client: %v", err)
	}

	var configSync dnsconfig.Sync
	switch {
	case config.ConfigMap != "" && config.ConfigDir != "":
		glog.Fatal("Cannot use both ConfigMap and ConfigDir")

	case config.ConfigMap != "":
		glog.V(0).Infof("Using configuration read from ConfigMap: %v:%v", config.ConfigMapNs, config.ConfigMap)
		configSync = dnsconfig.NewConfigMapSync(kubeClient, config.ConfigMapNs, config.ConfigMap)

	case config.ConfigDir != "":
		glog.V(0).Infof("Using configuration read from directory: %v with period %v", config.ConfigDir, config.ConfigPeriod)
		configSync = dnsconfig.NewFileSync(config.ConfigDir, config.ConfigPeriod)

	default:
		glog.V(0).Infof("ConfigMap and ConfigDir not configured, using values from command line flags")
		configSync = dnsconfig.NewNopSync(&dnsconfig.Config{Federations: config.Federations})
	}

	return &KubeDNSServer{
		domain:         config.ClusterDomain,
		healthzPort:    config.HealthzPort,
		dnsBindAddress: config.DNSBindAddress,
		dnsPort:        config.DNSPort,
		nameServers:    config.NameServers,
		kd:             dns.NewKubeDNS(kubeClient, config.ClusterDomain, config.InitialSyncTimeout, configSync),
	}
}

func newKubeClient(dnsConfig *options.KubeDNSConfig) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	// If both kubeconfig and master URL are empty, use service account
	if dnsConfig.KubeConfigFile == "" && dnsConfig.KubeMasterURL == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags(
			dnsConfig.KubeMasterURL, dnsConfig.KubeConfigFile)
		if err != nil {
			return nil, err
		}
	}
	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"
	config.UserAgent = userAgent()

	return kubernetes.NewForConfig(config)
}

func userAgent() string {
	return fmt.Sprintf("kube-dns/%s (%s/%s)", version.VERSION, runtime.GOOS, runtime.GOARCH)
}

func (server *KubeDNSServer) Run() {
	pflag.VisitAll(func(flag *pflag.Flag) {
		glog.V(0).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})
	setupSignalHandlers()
	server.startSkyDNSServer()
	server.kd.Start()
	server.setupHandlers()

	glog.V(0).Infof("Status HTTP port %v", server.healthzPort)
	if server.nameServers != "" {
		glog.V(0).Infof("Upstream nameservers: %s", server.nameServers)
	}
	glog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", server.healthzPort), nil))
}

// setupHandlers sets up a readiness and liveness endpoint for kube-dns.
func (server *KubeDNSServer) setupHandlers() {
	glog.V(0).Infof("Setting up Healthz Handler (/readiness)")
	http.HandleFunc("/readiness", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "ok\n")
	})

	glog.V(0).Infof("Setting up cache handler (/cache)")
	http.HandleFunc("/cache", func(w http.ResponseWriter, req *http.Request) {
		serializedJSON, err := server.kd.GetCacheAsJSON()
		if err == nil {
			fmt.Fprint(w, serializedJSON)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		}
	})
}

// setupSignalHandlers installs signal handler to ignore SIGINT and
// SIGTERM. This daemon will be killed by SIGKILL after the grace
// period to allow for some manner of graceful shutdown.
func setupSignalHandlers() {
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			glog.V(0).Infof("Ignoring signal %v (can only be terminated by SIGKILL)", <-sigChan)
			glog.Flush()
		}
	}()
}

func validateHostAndPort(hostAndPort string) error {
	host, port, err := net.SplitHostPort(hostAndPort)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil {
		return fmt.Errorf("bad IP address: %s", host)
	}

	if p, _ := strconv.Atoi(port); p < 1 || p > 65535 {
		return fmt.Errorf("bad port number %s", port)
	}
	return nil
}

// validateNameServer validates a configured name server and
// appends a ":53" if a :port is not included. IPv6 addresses are also
// enclosed in square brackets if not already so enclosed.
func validateNameServer(nameServer string) (string, error) {
	if ip := net.ParseIP(nameServer); ip != nil {
		if ip.To4() != nil {
			// IPv4 without port
			nameServer = nameServer + ":53"
		} else {
			// IPv6 without port
			nameServer = "[" + nameServer + "]:53"
		}
		return nameServer, nil
	}
	// Assume it's IP:port
	err := validateHostAndPort(nameServer)
	return nameServer, err
}

func (d *KubeDNSServer) startSkyDNSServer() {
	glog.V(0).Infof("Starting SkyDNS server (%v:%v)", d.dnsBindAddress, d.dnsPort)
	skydnsConfig := &server.Config{
		Domain:  d.domain,
		DnsAddr: fmt.Sprintf("%s:%d", d.dnsBindAddress, d.dnsPort),
	}
	if d.nameServers != "" {
		for _, nameServer := range strings.Split(d.nameServers, ",") {
			server, err := validateNameServer(nameServer)
			if err != nil {
				glog.Fatalf("nameserver '%s' is invalid: %v\n", nameServer, err)
			}
			skydnsConfig.Nameservers = append(skydnsConfig.Nameservers, server)
		}
	}
	server.SetDefaults(skydnsConfig)
	s := server.New(d.kd, skydnsConfig)
	if err := metrics.Metrics(); err != nil {
		glog.Fatalf("Skydns metrics error: %s", err)
	} else if metrics.Port != "" {
		glog.V(0).Infof("Skydns metrics enabled (%v:%v)", metrics.Path, metrics.Port)
	} else {
		glog.V(0).Infof("Skydns metrics not enabled")
	}

	go s.Run()
}
