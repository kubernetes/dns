package app

import (
	"bytes"
	"io/ioutil"
	"strings"
	"text/template"

	clog "github.com/coredns/coredns/plugin/pkg/log"
	"k8s.io/dns/pkg/dns/config"
)

const (
	stubDomainBlock = `
{{.DomainName}}:{{.Port}} {
    errors
    cache {{.CacheTTL}}
    bind {{.LocalIP}}
    forward . {{.UpstreamServers}} {
      force_tcp
    }
}
`  // cache TTL is 30s by default
	defaultTTL = 30
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
		// use resolv.conf by default.
		upstreamServers = "/etc/resolv.conf"
	}
	baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__UPSTREAM__SERVERS__"), []byte(upstreamServers), -1)
	baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__CLUSTER__DNS__"), []byte(c.clusterDNSIP.String()), -1)
	baseConfig = bytes.Replace(baseConfig, []byte("__PILLAR__LOCAL__DNS__"), []byte(c.params.LocalIPStr), -1)

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

func (c *CacheApp) syncKubeDNSConfig(syncChan <-chan *config.Config) {
	for {
		nextConfig := <-syncChan
		c.updateCorefile(nextConfig)
	}
}

func (c *CacheApp) initKubeDNSConfigSync() {
	if c.params.KubednsCMPath == "" {
		clog.Infof("No kube-dns configmap path specified, exiting sync")
		return
	}
	c.kubednsConfig.ConfigDir = c.params.KubednsCMPath
	configSync := config.NewFileSync(c.kubednsConfig.ConfigDir, c.kubednsConfig.ConfigPeriod)
	initialConfig, err := configSync.Once()
	if err != nil {
		clog.Errorf("Failed to sync kube-dns config directory %s, err: %v", c.params.KubednsCMPath, err)
		return
	}
	c.updateCorefile(initialConfig)
	go c.syncKubeDNSConfig(configSync.Periodic())
}
