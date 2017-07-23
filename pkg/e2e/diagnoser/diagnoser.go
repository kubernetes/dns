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
	"bufio"
	"log"
	"os"
	"os/exec"
	"strings"

	"k8s.io/dns/pkg/e2e"
)

// Diagnoser task executor
type Diagnoser struct {
	cmd       *exec.Cmd
	CmdErr    error
	IsRunning bool
}

// Start diagnoser tasks, passing in extra arguments
func (d *Diagnoser) Start(args ...string) {
	fr := e2e.GetFramework()
	bin := fr.Path("bin/amd64/diagnoser")

	args = append(args,
		"--kubecfg-file", fr.Path("test/e2e/cluster/config"),
		"--sleep-time", "0")

	var err error
	d.cmd, err = fr.RunInBackground("diagnoser", bin, args...)
	if err != nil {
		log.Fatal(err)
	}

	d.IsRunning = true

	go func() {
		d.CmdErr = d.cmd.Wait()
		d.IsRunning = false
	}()

	e2e.Log.Logf("diagnoser started")
}

// CheckLog returns a scanner to check
func (d *Diagnoser) CheckLog(needle string) bool {
	fr := e2e.GetFramework()

	f, err := os.Open(fr.StderrLogfile("diagnoser"))
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), needle) {
			return true
		}
	}
	return false
}
