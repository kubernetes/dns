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
	"fmt"
	"time"

	"k8s.io/dns/pkg/e2e"
	e2ed "k8s.io/dns/pkg/e2e/dnsmasq"

	. "github.com/onsi/ginkgo"
	om "github.com/onsi/gomega"
)

var _ = Describe("dnsmasq-nanny", func() {
	It("should update dnsmasq configuration", func() {
		fr := e2e.GetFramework()
		harness := &e2ed.Harness{
			fr.Options.WorkDir,
			fr.Options.BaseDir + "/bin/amd64/dnsmasq-nanny",
			fr.Options.BaseDir + "/test/fixtures/mock-dnsmasq.sh",
		}
		harness.Setup()

		nannyArgs := []string{
			"-v=2",
			"-logtostderr",
			"--configDir", fmt.Sprintf("%v/config", harness.TmpDir),
			"--syncInterval", "100ms",
			"--dnsmasqExec", harness.MockDnsmasq,
			"--restartDnsmasq=true",
			"--",
			"--argsFile", fmt.Sprintf("%v/args.txt", harness.TmpDir),
			"--runForever",
		}

		cmd, err := fr.RunInBackground(
			"dnsmasq-nanny", harness.NannyExec, nannyArgs...)
		By(fmt.Sprintf("Starting the nanny: %v", nannyArgs))
		om.Expect(err).To(om.Succeed())

		By("Waiting for the initial output to be written")
		prefix := fmt.Sprintf("--argsFile %v/args.txt --runForever", harness.TmpDir)
		harness.WaitForArgs(prefix)

		By("Updating dnsmasq via a configuration change 1")
		harness.Configure(`{"acme.local":["1.2.3.4"]}`, `[]`)
		harness.WaitForArgs(prefix + " --server /acme.local/1.2.3.4")

		By("Updating dnsmasq via a configuration change 2")
		harness.Configure(``, `["5.6.7.8"]`)
		harness.WaitForArgs(prefix + " --server 5.6.7.8 --no-resolv")

		By("Updating dnsmasq to invalid change (ignored)")
		harness.Configure(`$$$$asdf`, ``)
		time.Sleep(1 * time.Second)

		By("Updating dnsmasq via a configuration change 3")
		harness.Configure(`{"acme.local":["8.8.8.8"]}`, `[]`)
		harness.WaitForArgs(prefix + " --server /acme.local/8.8.8.8")

		By("Stopping the nanny")
		cmd.Process.Kill()
	})
})
