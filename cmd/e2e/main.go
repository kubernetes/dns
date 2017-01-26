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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/dns/pkg/e2e"
)

var opts struct {
	action  string
	baseDir string
	workDir string
}

func parseFlags() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, `
Runs the e2e framework SetUp and TearDown without running tests. You
can use this executable to setup a test environment without running
the end-to-end tests themselves.

Note: this executable will clobber the contents of /var/lib/kubelet and
start/stop containers on the local docker instance.
`)
		flag.PrintDefaults()
	}

	flag.StringVar(&opts.baseDir, "baseDir", "",
		"kubernetes/dns source code directory (default is current directory)")
	flag.StringVar(&opts.workDir, "workDir", "/tmp/k8s-dns", "temporary directory")
	flag.Parse()

	if opts.baseDir == "" {
		var err error
		// TODO: verify that this is actually the source code
		if opts.baseDir, err = os.Getwd(); err != nil {
			log.Fatal(err)
		}
	}

	if opts.workDir == "" {
		log.Fatalf("need to specify a workDir")
	}
	if _, err := os.Stat(opts.workDir); os.IsNotExist(err) {
		os.MkdirAll(opts.workDir, 0755)
	}
}

func waitForSignal() {
	log.Printf("Waiting for SIGINT, SIGTERM (use ctrl-c to stop cluster)")
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	log.Printf("Cleaning up")
}

func main() {
	parseFlags()

	e2e.InitFramework(opts.baseDir, opts.workDir)
	fr := e2e.GetFramework()
	fr.SetUp()
	waitForSignal()
	fr.TearDown()
}
