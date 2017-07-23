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

package diagnoser

import (
	"fmt"
	"os/exec"
	"syscall"

	. "github.com/onsi/ginkgo"
	om "github.com/onsi/gomega"
	"k8s.io/dns/cmd/diagnoser/flags"
	"k8s.io/dns/pkg/e2e/diagnoser"
)

var _ = Describe("diagnoser", func() {
	diagnoser := &diagnoser.Diagnoser{}

	Context("basic functionality", func() {
		It("should start", func() {
			diagnoser.Start()
			om.Eventually(func() error {
				expected := "Version v"

				if diagnoser.CheckLog(expected) {
					return nil
				}

				return fmt.Errorf("expected %q not found in logs", expected)
			}).Should(om.Succeed())
		})
		It("should exit with error, so that the job gets rescheduled", func() {
			om.Eventually(func() error {
				if diagnoser.IsRunning {
					return fmt.Errorf("diagnoser still running")
				}
				if diagnoser.CmdErr == nil {
					return fmt.Errorf("diagnoser succeeded")
				}

				exiterr, ok := diagnoser.CmdErr.(*exec.ExitError)
				if !ok {
					return fmt.Errorf("diagnoser error is not an exit error")
				}
				// The program has exited with an exit code != 0

				// This works on both Unix and Windows. Although package
				// syscall is generally platform dependent, WaitStatus is
				// defined for both Unix and Windows and in both cases has
				// an ExitStatus() method with the same signature.
				status, ok := exiterr.Sys().(syscall.WaitStatus)
				if ok && status.ExitStatus() == flags.DefaultExitCode {
					return nil
				}
				return diagnoser.CmdErr
			}).Should(om.Succeed())
		})
	})
	Context("diagnosis tasks", func() {
		It("should return generic info", func() {
			om.Eventually(func() error {
				const expected = "Total DNS pods: 0"

				if diagnoser.CheckLog(expected) {
					return nil
				}

				return fmt.Errorf("expected %q not found in logs", expected)
			}).Should(om.Succeed())
		})
	})
})
