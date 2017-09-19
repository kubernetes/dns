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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"k8s.io/dns/pkg/sidecar"
)

// verifyOption confirms that a DNS probe option has been populated correctly
// based on a probe configuration string
func verifyOption(t *testing.T, configStr string, option sidecar.DNSProbeOption) {
	splits := strings.Split(configStr, ",")
	if option.Label != splits[0] {
		t.Errorf("Incorrect label for '%s', expected '%s', got '%s'", configStr, splits[0], option.Label)
	}
	if option.Server != splits[1] {
		t.Errorf("Incorrect server for '%s', expected '%s', got '%s'", configStr, splits[1], option.Server)
	}
	expName := splits[2] + "."
	if option.Name != expName {
		t.Errorf("Incorrect name for '%s', expected '%s', got '%s'", configStr, expName, option.Name)
	}
	mSecs, _ := strconv.Atoi(splits[3])
	expInterval := time.Duration(mSecs) * time.Second
	if option.Interval != expInterval {
		t.Errorf("Incorrect interval for '%s', expected '%s', got '%s'", configStr, expInterval, option.Interval)
	}
	var expType uint16
	if len(splits) >= 5 {
		switch splits[4] {
		case "ANY":
			expType = dns.TypeANY
		case "A":
			expType = dns.TypeA
		case "AAAA":
			expType = dns.TypeAAAA
		case "SRV":
			expType = dns.TypeSRV
		}
	} else {
		expType = dns.TypeANY
	}
	if option.Type != expType {
		t.Errorf("Incorrect type for '%s' (type %s), expected '%d', got '%d'", configStr, splits[4], expType, option.Type)
	}
}

func TestProbeOptionsSet(t *testing.T) {
	// Expected errors
	const (
		noError         = ""
		invalidFormat   = "invalid format to --probe"
		invalidLabel    = "label must be of format"
		invalidDuration = "invalid syntax"
		invalidType     = "invalid type for DNS"
	)

	testCases := []struct {
		configStr string
		expError  string
	}{
		{"kubedns,[127.0.0.1]:10053,kubernetes.default.svc.cluster.local,5,SRV", noError},
		{"dnsmasq,[127.0.0.1]:53,kubernetes.default.svc.cluster.local,5,SRV", noError},
		{"dnsmasq,[127.0.0.1]:53,kubernetes.default.svc.cluster.local,5,ANY", noError},
		{"dnsmasq,[127.0.0.1]:53,kubernetes.default.svc.cluster.local,5,A", noError},
		{"dnsmasq,[::1]:53,kubernetes.default.svc.cluster.local,5,AAAA", noError},
		{"dnsmasq,[::1]:53,kubernetes.default.svc.cluster.local,5,SRV", noError},
		{"dnsmasq,[::1]:53,kubernetes.default.svc.cluster.local,5", noError},
		{"dnsmasq,[::1]:53", invalidFormat},
		{"dnsmasq,[::1]:53,kubernetes.default.svc.cluster.local,5,SRV,BogusField", invalidFormat},
		{"dn$m@s#,[::1]:53,kubernetes.default.svc.cluster.local,5,SRV", invalidLabel},
		{"dnsmasq,[::1]:53,kubernetes.default.svc.cluster.local,0.5,SRV", invalidDuration},
		{"dnsmasq,[::1]:53,kubernetes.default.svc.cluster.local,5,BogusType", invalidType},
	}

	var options probeOptions
	var optionIndex int
	for _, tc := range testCases {
		if err := options.Set(tc.configStr); err == nil {
			if tc.expError == "" {
				// Confirm that probe options were populated from config string
				option := sidecar.DNSProbeOption(options[optionIndex])
				verifyOption(t, tc.configStr, option)
			} else {
				t.Errorf("Error did not occur for '%s', expected '%s'", tc.configStr, tc.expError)
			}
			optionIndex++
		} else {
			if tc.expError == "" {
				t.Errorf("Unexpected error for '%s': '%v'", tc.configStr, err)
			} else {
				if !strings.Contains(err.Error(), tc.expError) {
					t.Errorf("Unexpected error for '%s', expected '%s', got '%v'", tc.configStr, tc.expError, err)
				}
			}
		}
	}
}
