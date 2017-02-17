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
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"k8s.io/dns/pkg/dns/config"
)

// Nanny encapsulates a dnsmasq process and manages its configuration.
type Nanny struct {
	Exec string

	args        []string
	ExitChannel chan error
	cmd         *exec.Cmd
}

// ExtractDnsmasqArgs returns the arguments that appear after "--" in the
// the command line. This function will also remove "--" and subsequent
// arguments from cmdlineArgs.
func ExtractDnsmasqArgs(cmdlineArgs *[]string) []string {
	for i, arg := range *cmdlineArgs {
		if arg == "--" {
			args := (*cmdlineArgs)[i+1:]
			*cmdlineArgs = (*cmdlineArgs)[0:i]
			return args
		}
	}

	return []string{}
}

// Configure the nanny. This must be called before Start().
func (d *Nanny) Configure(args []string, config *config.Config) {
	d.args = args

	munge := func(s string) string {
		return strings.Replace(s, ":", "#", -1)
	}

	for domain, serverList := range config.StubDomains {
		for _, server := range serverList {
			// dnsmasq port separator is '#' for some reason.
			server = munge(server)
			d.args = append(
				d.args, "--server", fmt.Sprintf("/%v/%v", domain, server))
		}
	}

	for _, server := range config.UpstreamNameservers {
		// dnsmasq port separator is '#' for some reason.
		server = munge(server)
		d.args = append(d.args, "--server", server)
	}
}

// Start the nanny.
func (d *Nanny) Start() error {
	glog.V(0).Infof("Starting dnsmasq %v", d.args)

	d.cmd = exec.Command(d.Exec, d.args...)
	stderrReader, err := d.cmd.StderrPipe()
	if err != nil {
		return err
	}

	stdoutReader, err := d.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := d.cmd.Start(); err != nil {
		return err
	}

	logToGlog := func(stream string, reader io.Reader) {
		bufReader := bufio.NewReader(reader)
		for {
			bytes, err := bufReader.ReadBytes('\n')
			if len(bytes) > 0 {
				glog.V(1).Infof("%v", string(bytes))
			}
			if err == io.EOF {
				glog.V(1).Infof("%v", string(bytes))
				glog.Warningf("Got EOF from %v", stream)
				return
			} else if err != nil {
				glog.V(1).Infof("%v", string(bytes))
				glog.Errorf("Error reading from %v: %v", stream, err)
				return
			}
		}
	}

	go logToGlog("stderr", stderrReader)
	go logToGlog("stdout", stdoutReader)

	d.ExitChannel = make(chan error)
	go func() {
		d.ExitChannel <- d.cmd.Wait()
	}()

	return nil
}

// Kill the running Nanny.
func (d *Nanny) Kill() error {
	glog.V(0).Infof("Killing dnsmasq")
	if d.cmd == nil {
		return fmt.Errorf("Process is not running")
	}

	if err := d.cmd.Process.Kill(); err != nil {
		glog.Errorf("Error killing dnsmasq: %v", err)
		return err
	}

	d.cmd = nil

	return nil
}
