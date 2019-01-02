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

package dnsmasq

import (
	"sort"
	"testing"
	"time"

	"k8s.io/dns/pkg/dns/config"

	"github.com/onsi/gomega"
)

func TestExtractDnsmasqArgs(t *testing.T) {
	gomega.RegisterTestingT(t)

	testCases := []struct {
		args        []string
		dnsmasqArgs []string
		otherArgs   []string
	}{
		{[]string{}, []string{}, []string{}},
		{[]string{"a"}, []string{}, []string{"a"}},
		{[]string{"a", "--"}, []string{}, []string{"a"}},
		{[]string{"a", "--", "b"}, []string{"b"}, []string{"a"}},
		{[]string{"--", "b"}, []string{"b"}, []string{}},
		{
			[]string{"a", "b", "--", "c", "d"},
			[]string{"c", "d"},
			[]string{"a", "b"},
		},
	}

	for _, tc := range testCases {
		args := tc.args
		gomega.Expect(ExtractDnsmasqArgs(&args)).To(
			gomega.Equal(tc.dnsmasqArgs))
		gomega.Expect(args).To(gomega.Equal(tc.otherArgs))
	}
}

func TestNannyConfig(t *testing.T) {
	gomega.RegisterTestingT(t)

	for _, testCase := range []struct {
		c    *config.Config
		e    []string
		sort bool
	}{
		{c: &config.Config{}, e: []string{"--abc"}},
		{
			c: &config.Config{
				StubDomains: map[string][]string{
					"acme.local":   []string{"1.1.1.1"},
					"widget.local": []string{"2.2.2.2:10053", "3.3.3.3"},
					"google.local": []string{"google-public-dns-a.google.com"},
				}},
			e: []string{
				"--abc",
				"--server",
				"--server",
				"--server",
				"--server",
				"/acme.local/1.1.1.1",
				"/google.local/8.8.8.8",
				"/widget.local/2.2.2.2#10053",
				"/widget.local/3.3.3.3",
			},
			sort: true,
		},
		{
			c: &config.Config{
				UpstreamNameservers: []string{"2.2.2.2:10053", "3.3.3.3"}},
			e: []string{
				"--abc",
				"--server",
				"2.2.2.2#10053",
				"--server",
				"3.3.3.3",
				"--no-resolv",
			},
		},
		{
			c: &config.Config{
				UpstreamNameservers: []string{"2001:db8:1::1", "[2001:db8:2::2]", "[2001:db8:3::3]:53"}},
			e: []string{
				"--abc",
				"--server",
				"2001:db8:1::1",
				"--server",
				"[2001:db8:2::2]",
				"--server",
				"[2001:db8:3::3]#53",
				"--no-resolv",
			},
		},
	} {
		nanny := &Nanny{Exec: "dnsmasq"}
		nanny.Configure([]string{"--abc"}, testCase.c, "127.0.0.1:10053")
		if testCase.sort {
			sort.Sort(sort.StringSlice(nanny.args))
		}
		gomega.Expect(nanny.args).To(gomega.Equal(testCase.e))
	}
}

func TestNannyLifecycle(t *testing.T) {
	gomega.RegisterTestingT(t)

	const mockDnsmasq = "../../test/fixtures/mock-dnsmasq.sh"
	var nanny *Nanny
	kubednsServer := "127.0.0.1:10053"

	// Exit with success.
	nanny = &Nanny{Exec: mockDnsmasq}
	nanny.Configure(
		[]string{"--exitWithSuccess"},
		&config.Config{},
		kubednsServer)
	gomega.Expect(nanny.Start()).To(gomega.Succeed())
	gomega.Expect(<-nanny.ExitChannel).To(gomega.Succeed())

	// Exit with error.
	nanny = &Nanny{Exec: mockDnsmasq}
	nanny.Configure(
		[]string{"--exitWithError"},
		&config.Config{},
		kubednsServer)
	gomega.Expect(nanny.Start()).To(gomega.Succeed())
	gomega.Expect(<-nanny.ExitChannel).NotTo(gomega.Succeed())

	// Exit with error after delay.
	nanny = &Nanny{Exec: mockDnsmasq}
	nanny.Configure(
		[]string{"--sleepThenError"},
		&config.Config{},
		kubednsServer)
	gomega.Expect(nanny.Start()).To(gomega.Succeed())
	gomega.Expect(<-nanny.ExitChannel).NotTo(gomega.Succeed())

	// Run forever. Kill while running.
	nanny = &Nanny{Exec: mockDnsmasq}
	nanny.Configure(
		[]string{"--runForever"},
		&config.Config{},
		kubednsServer)
	gomega.Expect(nanny.Start()).To(gomega.Succeed())
	time.Sleep(250 * time.Millisecond)
	gomega.Expect(nanny.Kill()).To(gomega.Succeed())
	gomega.Expect(nanny.Kill()).NotTo(gomega.Succeed())
}
