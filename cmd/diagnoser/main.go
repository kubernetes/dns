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
	"os"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/dns/cmd/diagnoser/flags"
	"k8s.io/dns/cmd/diagnoser/task"
	"k8s.io/dns/pkg/version"
	"k8s.io/kubernetes/pkg/util/logs"
)

func main() {
	options := flags.Parse()

	logs.InitLogs()
	defer logs.FlushLogs()

	glog.Infof("Version v%s", version.VERSION)

	version.PrintAndExitIfRequested()

	cs, err := newClientset(options)
	if err != nil {
		glog.Fatal(err)
	}

	if err := run(options, cs); err != nil {
		glog.Fatal(err)
	}
	time.Sleep(time.Duration(options.SleepTime) * time.Second)
	os.Exit(options.ExitCode)
}

func run(opt *flags.Options, cs v1.CoreV1Interface) error {
	ts := task.Bundle()

	for _, t := range ts {
		if err := t.Run(opt, cs); err != nil {
			return err
		}
	}

	return nil
}

func newClientset(opt *flags.Options) (v1.CoreV1Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags(
		opt.KubeMasterURL, opt.KubeConfigFile)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset.CoreV1(), nil
}
