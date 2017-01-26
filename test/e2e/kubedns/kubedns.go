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

	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo"
	om "github.com/onsi/gomega"

	e2edns "k8s.io/dns/pkg/e2e/dns"
)

var _ = Describe("kube-dns", func() {
	kubeDNS := &e2edns.KubeDNS{}

	Context("basic functionality", func() {
		It("should start", func() {
			kubeDNS.Start("-v=4")
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
			}).Should(om.Succeed())
		})

		It("should stop", func() {
			kubeDNS.Stop()
		})
	})
})
