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
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// Framework for e2e testing.
type Framework struct {
	Options Options
	Docker  Docker
	Cluster Cluster

	Processes map[string]*exec.Cmd
}

var framework *Framework

// Failed is set to true if a test case has failed.
var Failed bool

// InitFramework initializes the global framework.
func InitFramework(baseDir string, workDir string) {
	log.Printf("Creating framework (baseDir=%v, workDir=%v)",
		baseDir, workDir)

	if !CanSudo() {
		log.Fatalf(
			"e2e test requires `sudo` to be active. Run `sudo -v` before running the e2e test.")
	}
	KeepSudoActive()

	options := DefaultOptions(baseDir, workDir)
	docker := NewDocker()

	framework = &Framework{
		Options: options,
		Docker:  docker,
		Cluster: Cluster{
			Options: options,
			Docker:  docker,
		},
		Processes: make(map[string]*exec.Cmd),
	}

	for _, dir := range []string{
		workDir + "/logs",
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Could not mkdir %v: %v", workDir, err)
		}
	}
}

// GetFramework returns the global framework.
func GetFramework() *Framework {
	if framework == nil {
		log.Fatal("InitFramework must be called before use")
	}
	return framework
}

// SetUp the framework.
func (fr *Framework) SetUp() {
	fr.Cluster.SetUp()
}

// TearDown the framework.
func (fr *Framework) TearDown() {
	fr.Cluster.TearDown()

	if Failed {
		for name := range fr.Processes {
			log.Printf("Failure detected, dumping logs for '%v'", name)
			log.Printf("==== %v stdout ====", name)
			f, err := os.Open(fr.StdoutLogfile(name))
			if err != nil {
				log.Fatalf("Could not open %v: %v", fr.StdoutLogfile(name), err)
			}
			io.Copy(os.Stderr, f)

			log.Printf("==== %v stderr ====", name)
			f, err = os.Open(fr.StderrLogfile(name))
			if err != nil {
				log.Fatalf("Could not open %v: %v", fr.StderrLogfile(name), err)
			}
			io.Copy(os.Stderr, f)
		}
	}
}

// Path returns an absolute path for a relative path in the repository.
func (fr *Framework) Path(relative string) string {
	ret, err := filepath.Abs(fr.Options.BaseDir + "/" + relative)
	if err != nil {
		log.Fatal(err)
	}
	return ret
}

// StdoutLogfile is stdout log file for RunInBackground.
func (fr *Framework) StdoutLogfile(name string) string {
	return fmt.Sprintf("%v/logs/%v.out", fr.Options.WorkDir, name)
}

// StderrLogfile is the stderr log file for RunInBackground.
func (fr *Framework) StderrLogfile(name string) string {
	return fmt.Sprintf("%v/logs/%v.err", fr.Options.WorkDir, name)
}

// RunInBackground starts the given process in the background, redirecting the
// output of the process to external log files.
func (fr *Framework) RunInBackground(name string, binary string, args ...string) (*exec.Cmd, error) {
	log.Printf("Starting %v (%v %v)", name, binary, args)

	if _, ok := fr.Processes[name]; ok {
		log.Fatalf("Cannot run more than one process with the same name: %v", name)
	}

	cmd := exec.Command(binary, args...)

	if stdout, err := os.Create(fr.StdoutLogfile(name)); err == nil {
		cmd.Stdout = stdout
	} else {
		log.Fatalf("Could not create %v: %v", fr.StdoutLogfile(name), err)
	}

	if stderr, err := os.Create(fr.StderrLogfile(name)); err == nil {
		cmd.Stderr = stderr
	} else {
		log.Fatalf("Could not create %v: %v", fr.StderrLogfile(name), err)
	}

	fr.Processes[name] = cmd

	return cmd, cmd.Start()
}
