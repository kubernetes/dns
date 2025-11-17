/*
Copyright 2021 The Kubernetes Authors.
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

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"k8s.io/dns/cmd/node-cache/app"

	corednsmain "github.com/coredns/coredns/coremain"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/coredns/caddy"
	// blank imports to make sure the plugin code is pulled in from vendor when building node-cache image
	_ "github.com/coredns/coredns/plugin/bind"
	_ "github.com/coredns/coredns/plugin/bufsize"
	_ "github.com/coredns/coredns/plugin/cache"
	_ "github.com/coredns/coredns/plugin/debug"
	_ "github.com/coredns/coredns/plugin/dns64"
	_ "github.com/coredns/coredns/plugin/errors"
	_ "github.com/coredns/coredns/plugin/forward"
	_ "github.com/coredns/coredns/plugin/health"
	_ "github.com/coredns/coredns/plugin/hosts"
	_ "github.com/coredns/coredns/plugin/loadbalance"
	_ "github.com/coredns/coredns/plugin/log"
	_ "github.com/coredns/coredns/plugin/loop"
	_ "github.com/coredns/coredns/plugin/metrics"
	_ "github.com/coredns/coredns/plugin/pprof"
	_ "github.com/coredns/coredns/plugin/reload"
	_ "github.com/coredns/coredns/plugin/rewrite"
	_ "github.com/coredns/coredns/plugin/template"
	_ "github.com/coredns/coredns/plugin/trace"
	_ "github.com/coredns/coredns/plugin/whoami"
	"k8s.io/dns/pkg/version"
)

var cache *app.CacheApp

func init() {
	clog.Infof("Starting node-cache image: %+v", version.VERSION)
	params, err := parseAndValidateFlags()
	if err != nil {
		clog.Fatalf("Error parsing flags - %s, Exiting", err)
	}
	cache, err = app.NewCacheApp(params)
	if err != nil {
		clog.Fatalf("Failed to obtain CacheApp instance, err %v", err)
	}
	cache.Init()
	if !params.SkipTeardown {
		caddy.OnProcessExit = append(caddy.OnProcessExit, func() { cache.TeardownNetworking() })
	}
}

func parseAndValidateFlags() (*app.ConfigParams, error) {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Runs CoreDNS v%s as a nodelocal cache listening on the specified ip:port\n\n", corednsmain.CoreVersion)
		flag.PrintDefaults()
	}

	params := &app.ConfigParams{LocalPort: "53"}

	flag.StringVar(&params.LocalIPStr, "localip", "", "comma-separated string of ip addresses to bind dnscache to")
	flag.BoolVar(&params.SetupInterface, "setupinterface", true, "indicates whether network interface should be setup")
	flag.StringVar(&params.InterfaceName, "interfacename", "nodelocaldns", "name of the interface to be created")
	flag.DurationVar(&params.Interval, "syncinterval", 60, "interval(in seconds) to check for iptables rules")
	flag.StringVar(&params.MetricsListenAddress, "metrics-listen-address", "0.0.0.0:9353", "address to serve metrics on")
	flag.BoolVar(&params.SetupIptables, "setupiptables", true, "indicates whether iptables rules should be setup")
	flag.StringVar(&params.BaseCoreFile, "basecorefile", "/etc/coredns/Corefile.base", "Path to the template Corefile for node-cache")
	flag.StringVar(&params.CoreFile, "corefile", "/etc/Corefile", "Path to the Corefile to be used by node-cache")
	flag.StringVar(&params.KubednsCMPath, "kubednscm", "", "Path where the kube-dns configmap will be mounted")
	flag.StringVar(&params.UpstreamSvcName, "upstreamsvc", "kube-dns", "Service name whose cluster IP is upstream for node-cache")
	flag.StringVar(&params.HealthPort, "health-port", "8080", "port used by health plugin")
	flag.BoolVar(&params.SkipTeardown, "skipteardown", false, "indicates whether iptables rules should be torn down on exit")
	flag.BoolVar(&params.ReloadWithSignal, "reloadwithsignal", false, "use SIGUSR1 on self to reload CoreDNS")
	flag.Parse()

	for _, ipstr := range strings.Split(params.LocalIPStr, ",") {
		newIP := net.ParseIP(ipstr)
		if newIP == nil {
			return params, fmt.Errorf("invalid localip specified - %q", ipstr)
		}
		params.LocalIPs = append(params.LocalIPs, newIP)
	}

	// lookup specified dns port
	f := flag.Lookup("dns.port")
	if f == nil {
		return nil, fmt.Errorf("failed to lookup \"dns.port\" parameter")
	}
	params.LocalPort = f.Value.String()
	if _, err := strconv.Atoi(params.LocalPort); err != nil {
		return nil, fmt.Errorf("invalid port specified - %q", params.LocalPort)
	}
	if _, err := strconv.Atoi(params.HealthPort); err != nil {
		return nil, fmt.Errorf("invalid healthcheck port specified - %q", params.HealthPort)
	}
	if f = flag.Lookup("conf"); f != nil {
		params.CoreFile = f.Value.String()
		clog.Infof("Using Corefile %s", params.CoreFile)
	}
	if f = flag.Lookup("pidfile"); f != nil {
		params.Pidfile = f.Value.String()
		clog.Infof("Using Pidfile %s", params.Pidfile)
	}
	return params, nil
}

func main() {
	cache.RunApp()
}
