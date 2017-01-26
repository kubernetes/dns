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

package e2e

import (
	"log"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/dns/pkg/e2e"

	"os"
	"path/filepath"
	"testing"

	_ "k8s.io/dns/test/e2e/kubedns"
)

func TestE2e(t *testing.T) {
	gomega.RegisterFailHandler(failureHandler)
	ginkgo.RunSpecs(t, "k8s-dns e2e test suite")
}

func failureHandler(message string, callerSkip ...int) {
	e2e.Failed = true
	ginkgo.Fail(message, callerSkip...)
}

var _ = ginkgo.SynchronizedBeforeSuite(
	func() []byte {
		// We expect the directory to be "baseDir/test/e2e"
		pkgDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("Error getting working directory: %v", err)
		}

		baseDir, err := filepath.Abs(pkgDir + "../../..")
		if err != nil {
			log.Fatalf("Error getting base directory: %v", err)
		}

		const workDir = "/tmp/k8s-dns"
		if err := os.RemoveAll(workDir); err != nil {
			log.Fatalf("Cannot remove %v: %v", workDir, err)
		}

		e2e.InitFramework(baseDir, workDir)
		e2e.GetFramework().SetUp()

		return []byte{}
	},
	func(data []byte) {})

var _ = ginkgo.SynchronizedAfterSuite(
	func() {
		e2e.GetFramework().TearDown()
	},
	func() {})
