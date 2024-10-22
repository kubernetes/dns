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

package kubedns

import (
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo"
	om "github.com/onsi/gomega"

	"k8s.io/dns/pkg/e2e"
	e2edns "k8s.io/dns/pkg/e2e/dns"
)

var _ = Describe("kube-dns", func() {

	Context("basic functionality", func() {
		kubeDNS := &e2edns.KubeDNS{}
		It("should start", func() {
			kubeDNS.Start("kube-dns", "-v=4")
		})

		It("should answer query for kubernetes service", func() {
			om.Eventually(func() error {
				names, err := kubeDNS.Query(
					"kubernetes.default.svc.cluster.local.", dns.TypeA)
				if err != nil {
					return err
				}

				const expected = "kubernetes.default.svc.cluster.local.\t30\tIN\tA\t10.0.0.1"
				if len(names) == 1 && names[0] == expected {
					return nil
				}

				return fmt.Errorf("expected %v, but got %v", expected, names)
			}, "5s", "1s").Should(om.Succeed())
		})

		It("should stop", func() {
			kubeDNS.Stop()
		})
	})

	It("should forward PTR queries to the upstream server", func() {
		By("Setting up environment without upstream server")
		kubeDNS := &e2edns.KubeDNS{}
		fr := e2e.GetFramework()
		workDir := fr.Options.WorkDir + "/ptr_forwarding"

		if _, err := os.Stat(workDir); err == nil {
			os.RemoveAll(workDir)
		}
		defer func() {
			if _, err := os.Stat(workDir); err == nil {
				os.RemoveAll(workDir)
			}
		}()

		configDir := workDir + "/kube-dns-config"
		if err := os.MkdirAll(configDir, 0744); err == nil {
			om.Expect(err).NotTo(om.HaveOccurred())
		}
		if err := os.WriteFile(configDir+"/upstreamNameservers", []byte("[\"127.0.0.1:10054\"]"), 0744); err != nil {
			om.Expect(err).NotTo(om.HaveOccurred())
		}
		dnsmasqConfigDir := workDir + "/dnsmasq-config"
		if err := os.MkdirAll(dnsmasqConfigDir, 0744); err != nil {
			om.Expect(err).NotTo(om.HaveOccurred())
		}
		if err := os.WriteFile(dnsmasqConfigDir+"/dnsmasq.conf", []byte("user=root\naddn-hosts=/etc/dnsmasq-hosts"), 0744); err != nil {
			om.Expect(err).NotTo(om.HaveOccurred())
		}
		if err := os.WriteFile(dnsmasqConfigDir+"/dnsmasq-hosts", []byte("192.0.2.123 my.test"), 0744); err != nil {
			om.Expect(err).NotTo(om.HaveOccurred())
		}
		fr.Docker.Pull(fr.Options.DnsmasqImage)

		By("Getting answer without numb upstream server")
		dnsmasq_numb := fr.Docker.Run(
			"-d",
			"-p=10054:53/tcp",
			"-p=10054:53/udp",
			"--cap-add=NET_ADMIN",
			fr.Options.DnsmasqImage)
		defer func() {
			fr.Docker.Kill(dnsmasq_numb)
		}()

		kubeDNS.Start("kube-dns-ptrfwd", "-v=4", "--config-dir="+configDir)
		defer func() {
			kubeDNS.Stop()
		}()

		om.Eventually(func() error {
			return doPtrQuery(kubeDNS)
		}, 1*time.Minute).ShouldNot(om.Succeed())

		By("Getting answer from upstream server")
		dnsmasq := fr.Docker.Run(
			"-d",
			"-p=10055:53/tcp",
			"-p=10055:53/udp",
			"-v="+dnsmasqConfigDir+"/dnsmasq.conf:/etc/dnsmasq.conf",
			"-v="+dnsmasqConfigDir+"/dnsmasq-hosts:/etc/dnsmasq-hosts",
			"--cap-add=NET_ADMIN",
			fr.Options.DnsmasqImage)
		defer func() {
			fr.Docker.Kill(dnsmasq)
		}()
		By("Configuring upstream nameserver")
		if err := os.WriteFile(configDir+"/upstreamNameservers", []byte("[\"127.0.0.1:10055\"]"), 0744); err != nil {
			om.Expect(err).NotTo(om.HaveOccurred())
		}

		om.Eventually(func() error {
			return doPtrQuery(kubeDNS)
		}, 1*time.Minute).Should(om.Succeed())
	})
})

func doPtrQuery(kubeDNS *e2edns.KubeDNS) error {
	time.Sleep(1 * time.Second)
	names, err := kubeDNS.Query("123.2.0.192.in-addr.arpa.", dns.TypePTR)
	expected := "123.2.0.192.in-addr.arpa.\t0\tIN\tPTR\tmy.test."
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("Inverse lookup responsed empty answer.")
	}
	if names[0] != expected {
		return fmt.Errorf("expected '%s', but got %s", expected, names[0])
	}
	return nil
}
