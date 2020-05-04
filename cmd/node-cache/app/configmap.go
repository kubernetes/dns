package app

import (
	"bytes"
	"io/ioutil"
	"path"
	"strings"
	"text/template"
	"time"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"k8s.io/dns/pkg/dns/config"
)

const (
	stubDomainBlock = `
{{.DomainName}}:{{.Port}} {
    errors
    cache {{.CacheTTL}}
    bind {{.LocalIP}}
    forward . {{.UpstreamServers}}
}
`  // cache TTL is 30s by default
	defaultTTL    = 30
	upstreamBlock = `
    forward . __PILLAR__UPSTREAM__SERVERS__ {
            force_tcp
    }
`
	upstreamUDPBlock = `
    forward . __PILLAR__UPSTREAM__SERVERS__
`
	DefaultConfigSyncPeriod = 10 * time.Second
)

// stubDomainInfo contains all the parameters needed to compute
// a stubDomain block in the Corefile.
type stubDomainInfo struct {
	DomainName      string
	LocalIP         string
	Port            string
	CacheTTL        int
	UpstreamServers string
}

func getStubDomainStr(stubDomainMap map[string][]string, info *stubDomainInfo) string {
	var tpl bytes.Buffer
	for domainName, servers := range stubDomainMap {
		tmpl, err := template.New("stubDomainBlock").Parse(stubDomainBlock)
		if err != nil {
			clog.Errorf("Failed to create stubDomain template, err : %v", err)
			setupErrCount.WithLabelValues("configmap").Inc()
			continue
		}
		info.DomainName = domainName
		info.UpstreamServers = strings.Join(servers, " ")
		if err := tmpl.Execute(&tpl, *info); err != nil {
			clog.Errorf("Failed to parse stubDomain template, err : %v", err)
			setupErrCount.WithLabelValues("configmap").Inc()
		}
	}
	return tpl.String()
}

func (c *CacheApp) updateCorefile(dnsConfig *config.Config) {
	// construct part of the Corefile
	baseConfig, err := ioutil.ReadFile(c.params.BaseCoreFile)
	if err != nil {
		clog.Errorf("Failed to read node-cache coreFile %s - %v", c.params.BaseCoreFile, err)
		setupErrCount.WithLabelValues("configmap").Inc()
		return
	}
	stubDomainStr := getStubDomainStr(dnsConfig.StubDomains, &stubDomainInfo{Port: c.params.LocalPort, CacheTTL: defaultTTL,
		LocalIP: strings.Replace(c.params.LocalIPStr, ",", " ", -1)})
	upstreamServers := strings.Join(dnsConfig.UpstreamNameservers, " ")
	if upstreamServers == "" {
		// forward plugin supports both nameservers as well as resolv.conf
		// use resolv.conf by default and use TCP for upstream.
		upstreamServers = "/etc/resolv.conf"
		baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__UPSTREAM__SERVERS__"), []byte(upstreamServers), -1)
	} else {
		// Use UDP to connect to custom upstream DNS servers.
		upstreamUDP := bytes.Replace([]byte(upstreamUDPBlock), []byte("__PILLAR__UPSTREAM__SERVERS__"), []byte(upstreamServers), -1)
		baseConfig = bytes.Replace(baseConfig, []byte(upstreamBlock), upstreamUDP, -1)
		// Just in case previous replace failed due to different indetation in config file
		// this step will put in the correct upstream servers, though it might still use TCP.
		if bytes.Contains(baseConfig, []byte("__PILLAR__UPSTREAM__SERVERS__")) {
			clog.Warningf("Failed to replace TCP upstream block with UDP, node-cache will connect to custom upstream servers via TCP.")
			baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__UPSTREAM__SERVERS__"), []byte(upstreamServers), -1)
		}
	}
	baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__CLUSTER__DNS__"), []byte(c.clusterDNSIP.String()), -1)
	baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__LOCAL__DNS__"), []byte(strings.Replace(c.params.LocalIPStr, ",", " ", -1)), -1)
	newConfig := bytes.Buffer{}
	newConfig.WriteString(string(baseConfig))
	newConfig.WriteString(stubDomainStr)
	if err := ioutil.WriteFile(c.params.CoreFile, newConfig.Bytes(), 0666); err != nil {
		clog.Errorf("Failed to write config file %s - err %v", c.params.CoreFile, err)
		setupErrCount.WithLabelValues("configmap").Inc()
		return
	}
	clog.Infof("Updated Corefile with %d custom stubdomains and upstream servers %s", len(dnsConfig.StubDomains), upstreamServers)
	clog.Infof("Using config file:\n%s", newConfig.String())
}

// syncInfo contains all parameters needed to watch a configmap directory for updates
type syncInfo struct {
	configName string
	filePath   string
	period     time.Duration
	updateFunc func(*config.Config)
	// channel where updates will be sent
	chanAddr      *<-chan *config.Config
	initialConfig *config.Config
}

// syncDNSConfig updates the node-cache config file whenever there are changes to
// kube-dns or node-local-dns configmaps.
func (c *CacheApp) syncDNSConfig(kubeDNSSyncChan, NodeLocalDNSSyncChan <-chan *config.Config, currentKubeDNSConfig *config.Config) {
	for {
		select {
		case currentKubeDNSConfig = <-kubeDNSSyncChan:
			c.updateCorefile(currentKubeDNSConfig)
		case <-NodeLocalDNSSyncChan:
			// Disregard the updated config from channel since updateCoreFile will read the file once again.
			// This call passes in the latest kube-dns config as parameter.
			c.updateCorefile(currentKubeDNSConfig)
		}
	}
}

// initDNSConfigSync starts syncers to watch the configmap directories for
// kube-dns(stubDomains) and node-local-dns(Corefile).
func (c *CacheApp) initDNSConfigSync() {
	var syncList []*syncInfo
	var kubeDNSChan, NodeLocalDNSChan <-chan *config.Config
	initialKubeDNSConfig := &config.Config{}

	if c.params.KubednsCMPath != "" {
		c.kubednsConfig.ConfigDir = c.params.KubednsCMPath
		syncList = append(syncList, &syncInfo{configName: "kube-dns",
			filePath:   c.kubednsConfig.ConfigDir,
			period:     c.kubednsConfig.ConfigPeriod,
			updateFunc: c.updateCorefile,
			chanAddr:   &kubeDNSChan,
		})
	} else {
		clog.Infof("Skipping kube-dns configmap sync as no directory was specified")
	}
	syncList = append(syncList, &syncInfo{configName: "node-local-dns",
		filePath: path.Dir(c.params.BaseCoreFile),
		period:   DefaultConfigSyncPeriod,
		chanAddr: &NodeLocalDNSChan,
	})

	for _, info := range syncList {
		configSync := config.NewFileSync(info.filePath, info.period)
		initialConfig, err := configSync.Once()
		if err != nil {
			clog.Errorf("Failed to sync %s config directory %s, err: %v", info.configName, info.filePath, err)
			continue
		}
		if info.updateFunc != nil {
			info.updateFunc(initialConfig)
		}
		if info.configName == "kube-dns" {
			initialKubeDNSConfig = initialConfig
		}
		*(info.chanAddr) = configSync.Periodic()
	}
	go c.syncDNSConfig(kubeDNSChan, NodeLocalDNSChan, initialKubeDNSConfig)
}
