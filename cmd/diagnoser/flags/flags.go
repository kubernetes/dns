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

package flags

import (
	"flag"
)

const (
	DefaultSleepTime = 20
	DefaultExitCode  = 123
)

// Options captures the command line flags passed
type Options struct {
	RunInfo        bool
	KubeConfigFile string
	KubeMasterURL  string
	SleepTime      int
	ExitCode       int
}

// Parse analyzes the given flags and return them inside an Options struct
func Parse() *Options {
	var (
		runInfo        = flag.Bool("run-info", true, "run info checks?")
		kubeConfigFile = flag.String("kubecfg-file", "", "Location of kubecfg file for access to kubernetes master service")
		kubeMasterURL  = flag.String("kube-master-url", "", "URL to reach master")
		sleepTime      = flag.Int("sleep-time", DefaultSleepTime, "Time to wait after finishing the tasks and exiting")
		exitCode       = flag.Int("exit-code", DefaultExitCode, "error exit code to use on exit (because of the error the diagnoser job will be rescheduled)")
	)
	flag.Parse()

	return &Options{
		RunInfo:        *runInfo,
		KubeConfigFile: *kubeConfigFile,
		KubeMasterURL:  *kubeMasterURL,
		SleepTime:      *sleepTime,
		ExitCode:       *exitCode,
	}
}
