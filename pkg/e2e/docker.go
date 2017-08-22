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
	"os"
	"os/exec"
	"strings"
)

// Docker is a simple shim to a Docker instance. Most methods will bail with Fatal
// if there is an error.
type Docker interface {
	// Start the daemon (if needed)
	Start()
	// Stop the daemon
	Stop()
	// Pull images into docker.
	Pull(images ...string)
	// Run calls "docker run" args, returning the UUID of the container.
	Run(args ...string) string
	// Remove the container named by tag.
	Remove(tag string)
	// Kill the container named by tag.
	Kill(tag string)
	// List tags of containers that match filter. If filter is "", then all running containers
	// will be listed.
	List(filter string) []string
}

// NewDocker returns a Docker for the default instance running on the host.
func NewDocker() Docker {
	return &dockerWrapper{
		dockerExec:   "docker",
		manageDaemon: false,
		baseDir:      "/",
		cidr:         "10.123.0.0/24",
		bridge:       "docker0",
		socket:       "unix:///var/run/docker.sock",
	}
}

type dockerWrapper struct {
	dockerExec string

	manageDaemon bool
	baseDir      string
	cidr         string
	bridge       string

	socket string
	cmd    *exec.Cmd
}

var _ Docker = (*dockerWrapper)(nil)

func (d *dockerWrapper) Start() {
	if !d.manageDaemon {
		return
	}

	execDir := d.baseDir + "/var/lib/docker"
	graphDir := d.baseDir + "/var/run/docker"

	if err := os.MkdirAll(execDir, 0755); err != nil {
		Log.Fatal(err)
	}
	if err := os.MkdirAll(graphDir, 0755); err != nil {
		Log.Fatal(err)
	}

	pidfile := d.baseDir + "/pid"
	d.socket = "unix://" + d.baseDir + "/var/run/docker.sock"

	d.ensureBridge()

	args := []string{
		d.dockerExec, "daemon",
		"--bridge=" + d.bridge,
		"--exec-root=" + execDir,
		"--graph=" + graphDir,
		"--host=" + d.socket,
		"--pidfile=" + pidfile,
	}

	d.cmd = exec.Command("sudo", args...)

	Log.Logf("Starting Docker %v", args)
	if err := d.cmd.Start(); err != nil {
		Log.Fatal(err)
	}

	d.waitForStart()
}

func (d *dockerWrapper) Stop() {
	if !d.manageDaemon {
		return
	}

	// Need to use sudo kill as the docker daemon is running as `root`.
	if err := exec.Command(
		"sudo", "kill", fmt.Sprintf("%v", d.cmd.Process.Pid)).Run(); err != nil {
		Log.Fatal(err)
	}
	state, err := d.cmd.Process.Wait()
	if err != nil {
		Log.Logf("Wait for docker returned %v", err)
	}
	Log.Logf("Docker exited with %v", state)
}

func (d *dockerWrapper) Pull(images ...string) {
	for _, image := range images {
		d.runCommand([]string{"-H", d.socket, "pull", image})
	}
}

func (d *dockerWrapper) Run(args ...string) string {
	args = append(
		[]string{"-H", d.socket, "run"},
		args...)
	Log.Logf("docker run %v", args)

	cmd := exec.Command(d.dockerExec, args...)
	output, err := cmd.CombinedOutput()
	Log.LogWithPrefix("docker", string(output))

	if err != nil {
		Log.LogWithPrefix("docker", string(output))
		Log.Fatalf("docker returned exit code %v", err)
	}

	// This will be the UUID of the running container.
	return strings.TrimSpace(string(output))
}

func (d *dockerWrapper) Remove(tag string) {
	d.runCommand([]string{"-H", d.socket, "rm", "-f", tag})
}

func (d *dockerWrapper) Kill(tag string) {
	d.runCommand([]string{"-H", d.socket, "kill", tag})
}

func (d *dockerWrapper) List(filter string) []string {
	args := []string{"-H", d.socket, "ps", "-q"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	Log.Logf("docker %v", args)
	out, err := exec.Command(d.dockerExec, args...).Output()

	if err != nil {
		Log.Fatalf("Error getting containers: %v", err)
	}

	var ret []string
	for _, tag := range strings.Split(string(out), "\n") {
		if tag := strings.TrimSpace(tag); tag != "" {
			ret = append(ret, tag)
		}
	}

	return ret
}

func (d *dockerWrapper) runCommand(args []string) {
	Log.Logf("docker %v", args)

	cmd := exec.Command(d.dockerExec, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		Log.LogWithPrefix("docker", string(output))
		Log.Fatal(err)
	}
}

func (d *dockerWrapper) ensureBridge() {
	if exec.Command("ip", "link", "show", d.bridge).Run() == nil {
		Log.Logf("Bridge device %v exists", d.bridge)
		return
	}

	Log.Logf("Creating bridge device %v (%v)", d.bridge, d.cidr)
	if err := exec.Command("sudo", "brctl", "addbr", d.bridge).Run(); err != nil {
		Log.Fatal(err)
	}
	if err := exec.Command("sudo", "ip", "addr", "add", d.cidr, "dev", d.bridge); err != nil {
		Log.Fatal(err)
	}
	if err := exec.Command("sudo", "ip", "link", "set", "dev", d.bridge, "up"); err != nil {
		Log.Fatal(err)
	}
}

func (d *dockerWrapper) waitForStart() {
	for {
		if err := exec.Command(d.dockerExec, "-H", d.socket, "info").Run(); err == nil {
			return
		}
	}
}
