package app

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"k8s.io/dns/pkg/dns/config"
)

const (
	templateCoreFileContents = `
	cluster.local:53 {
        errors
        cache {
                success 9984 30
                denial 9984 5
        }
        reload
        loop
        bind __PILLAR__LOCAL__DNS__
        forward . __PILLAR__CLUSTER__DNS__ {
                force_tcp
        }
        prometheus :9253
        health __PILLAR__LOCAL__DNS__:8080
        }
    .:53 {
        errors
        cache 30
        reload
        loop
        bind __PILLAR__LOCAL__DNS__
        forward . __PILLAR__UPSTREAM__SERVERS__ {
                force_tcp
        }
        prometheus :9253
        }
	`
	templateCoreFileName   = "testCoreFile.base"
	coreFileName           = "testCoreFile"
	cmDirName              = "testKubeDNSDir"
	stubDomainFileName     = "stubDomains"
	upstreamServerFileName = "upstreamNameservers"
	UpstreamClusterDNS     = "test-svc"
)

func updateStubDomainsAndUpstreamServers(t *testing.T, p *ConfigParams, c *config.Config) string {
	if stubDomainBlob, err := json.Marshal(c.StubDomains); err != nil {
		t.Errorf("Failed to marshal stubdomains info, err %v", err)
	} else {
		err = ioutil.WriteFile(filepath.Join(p.KubednsCMPath, stubDomainFileName), stubDomainBlob, os.ModePerm)
		if err != nil {
			t.Errorf("Failed to write stubDomains file - %s, err %v", stubDomainFileName, err)
		}
	}

	if upstreamBlob, err := json.Marshal(c.UpstreamNameservers); err != nil {
		t.Errorf("Failed to marshal upstream nameservers info, err %v", err)
	} else {
		err = ioutil.WriteFile(filepath.Join(p.KubednsCMPath, upstreamServerFileName), upstreamBlob, os.ModePerm)
		if err != nil {
			t.Errorf("Failed to write stubDomains file - %s, err %v", upstreamServerFileName, err)
		}
	}
	return ""
}

func createBaseFiles(t *testing.T, p *ConfigParams) {
	err := ioutil.WriteFile(p.BaseCoreFile, []byte(templateCoreFileContents), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to write template config file - %v", err)
	}
	if err = os.Mkdir(p.KubednsCMPath, os.ModePerm); err != nil {
		t.Fatalf("Failed to create KubeDNS configmap dir - %v", err)
	}
}

func compareFileContents(filename, contents string, t *testing.T) (string, int) {
	out, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Errorf("Failed to read file %s , err %v", filename, err)
		return "", -1
	}
	return string(out), strings.Compare(string(out), contents)
}

func stubDomainsEqual(str1, str2 string, t *testing.T) bool {
	// Double newline separates one stubdomain block from next
	blocks1 := strings.Split(str1, "\n\n")
	blocks2 := strings.Split(str2, "\n\n")
	if len(blocks1) != len(blocks2) {
		return false
	}
	sort.Strings(blocks1)
	sort.Strings(blocks2)
	for i, v := range blocks1 {
		if v != blocks2[i] {
			// Printing raw bytes is more useful to identify the inequality
			t.Errorf("Stubdomains not equal - %+v and %+v", []byte(v), []byte(blocks2[i]))
			return false
		}
	}
	return true
}

func TestUpdateCoreFile(t *testing.T) {
	baseDir, err := ioutil.TempDir("", "dnstest")
	if err != nil {
		t.Fatalf("Failed to obtain temp directory for testing, err %v", err)
	}
	envName := strings.ToUpper(strings.Replace(UpstreamClusterDNS, "-", "_", -1)) + "_SERVICE_HOST"
	os.Setenv(envName, "9.10.11.12")
	defer func() { os.RemoveAll(baseDir) }()
	c, err := NewCacheApp(&ConfigParams{LocalIPStr: "169.254.20.10",
		LocalPort:       "53",
		BaseCoreFile:    filepath.Join(baseDir, templateCoreFileName),
		CoreFile:        filepath.Join(baseDir, coreFileName),
		KubednsCMPath:   filepath.Join(baseDir, cmDirName),
		UpstreamSvcName: UpstreamClusterDNS,
		SetupIptables:   false,
	})
	if err != nil {
		t.Fatalf("Failed to obtain CacheApp instance, err %v", err)
	}
	createBaseFiles(t, c.params)
	c.initKubeDNSConfigSync()
	r := strings.NewReplacer("__PILLAR__LOCAL__DNS__", c.params.LocalIPStr, "__PILLAR__CLUSTER__DNS__", "9.10.11.12",
		"__PILLAR__UPSTREAM__SERVERS__", "/etc/resolv.conf")
	expectedContents := r.Replace(templateCoreFileContents)
	if out, diff := compareFileContents(c.params.CoreFile, expectedContents, t); diff != 0 {
		t.Errorf("Expected contents '%s', Got '%s'", expectedContents, out)
	}
	customConfig := &config.Config{StubDomains: map[string][]string{
		"acme.local":   {"1.1.1.1"},
		"google.local": {"google-public-dns-a.google.com"},
		"widget.local": {"2.2.2.2:10053", "3.3.3.3"},
	},
		UpstreamNameservers: []string{"2.2.2.2:10053", "3.3.3.3"},
	}
	updateStubDomainsAndUpstreamServers(t, c.params, customConfig)
	expectedContents = strings.Replace(expectedContents, "/etc/resolv.conf", strings.Join(customConfig.UpstreamNameservers, " "), -1)
	expectedStubStr := getStubDomainStr(customConfig.StubDomains, &stubDomainInfo{Port: c.params.LocalPort, CacheTTL: defaultTTL, LocalIP: c.params.LocalIPStr})

	time.Sleep(10 * time.Second)
	out, _ := compareFileContents(c.params.CoreFile, expectedContents, t)
	if !strings.Contains(out, expectedContents) {
		t.Fatalf("Could not find contents '%s' in CoreFile '%s'", expectedContents, out)
	}
	// The entire file cannot be compared because the stubDomains block
	// will be in a different order as they are generated by iterating over
	// a map. They will be converted  to a list and compared individually.
	stubStr := strings.TrimPrefix(out, expectedContents)
	if !stubDomainsEqual(strings.TrimSpace(stubStr), strings.TrimSpace(expectedStubStr), t) {
		t.Fail()
	}
}
