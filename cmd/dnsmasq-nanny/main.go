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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"

	"k8s.io/dns/pkg/dns/config"
	"k8s.io/dns/pkg/dnsmasq"
)

var (
	opts = struct {
		dnsmasq.RunNannyOpts
		configDir    string
		syncInterval time.Duration
	}{
		RunNannyOpts: dnsmasq.RunNannyOpts{
			DnsmasqExec:     "/usr/sbin/dnsmasq",
			RestartOnChange: false,
		},
		configDir:    "/etc/k8s/dns/dnsmasq-nanny",
		syncInterval: 10 * time.Second,
	}

	currentConfig *config.Config
)

func parseFlags() {
	opts.DnsmasqArgs = dnsmasq.ExtractDnsmasqArgs(&os.Args)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, `
Manages the dnsmasq daemon, handles configuration given by the ConfigMap.
Any arguments given after "--" will be passed directly to dnsmasq itself.

`)
		flag.PrintDefaults()
	}

	flag.StringVar(&opts.DnsmasqExec, "dnsmasqExec", opts.DnsmasqExec,
		"location of the dnsmasq executable")
	flag.BoolVar(&opts.RestartOnChange, "restartDnsmasq",
		opts.RestartOnChange,
		"if true, restart dnsmasq when the configuration changes")
	flag.StringVar(&opts.configDir, "configDir", opts.configDir,
		"location of the configuration")
	flag.DurationVar(&opts.syncInterval, "syncInterval",
		opts.syncInterval,
		"interval to check for configuration updates")
	flag.Parse()
}

func main() {
	parseFlags()
	glog.V(0).Infof("opts: %v", opts)

	sync := config.NewFileSync(opts.configDir, opts.syncInterval)

	dnsmasq.RunNanny(sync, opts.RunNannyOpts)
}
