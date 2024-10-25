/* Copyright 2016 The Kubernetes Authors.

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
	"net/http"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	startupTimeout = 10 * time.Second
)

// Cluster encapsulates a mock Kubernetes cluster.
type Cluster struct {
	Options
	Docker Docker

	containers struct {
		etcd    string
		api     string
		kubelet string
	}

	manifestDir   string
	varLibDocker  string
	varLibKubelet string
	varRun        string
}

// SetUp the e2e cluster.
func (cl *Cluster) SetUp() {
	Log.Logf("SetUp")

	cl.resolveDirs()
	cl.pullImages()

	cl.StartEtcd()
	cl.StartApiServer()

	cl.WaitForApiServer()
}

// TearDown the e2e cluster.
func (cl *Cluster) TearDown() {
	Log.Logf("Teardown")

	cl.StopApiServer()
	cl.StopEtcd()
}

func (cl *Cluster) resolveDirs() {
	// TODO: directories should be configurable, but there seem to be issues with the
	// the nsenter mounter that prevent us from moving the location of /var/lib/kubelet.
	cl.manifestDir = fmt.Sprintf("%v/test/e2e/cluster/manifests", cl.BaseDir)
	cl.varLibDocker = "/var/lib/docker"
	cl.varLibKubelet = "/var/lib/kubelet"
	cl.varRun = "/var/run"

	var err error

	cl.manifestDir, err = filepath.Abs(cl.manifestDir)
	if err != nil {
		Log.Fatal(err)
	}

	cl.varLibDocker, err = filepath.Abs(cl.varLibDocker)
	if err != nil {
		Log.Fatal(err)
	}

	cl.varRun, err = filepath.Abs(cl.varRun)
	if err != nil {
		Log.Fatal(err)
	}

	cl.varLibKubelet, err = filepath.Abs(cl.varLibKubelet)
	if err != nil {
		Log.Fatal(err)
	}
}

func (cl *Cluster) pullImages() {
	cl.Docker.Pull(
		cl.EtcdImage,
		cl.HyperkubeImage)
}

func (cl *Cluster) StartEtcd() {
	Log.Logf("Starting etcd")

	cl.containers.etcd = cl.Docker.Run("-d", "--net=host", cl.EtcdImage)
}

func (cl *Cluster) StopEtcd() {
	if cl.containers.etcd == "" {
		return
	}

	Log.Logf("Stopping etcd")

	cl.Docker.Kill(cl.containers.etcd)
	cl.containers.etcd = ""
}

func (cl *Cluster) StartApiServer() {
	Log.Logf("Starting API server")

	cl.containers.api = cl.Docker.Run(
		"-d",
		fmt.Sprintf("--volume=%v:/src:ro", cl.BaseDir),
		fmt.Sprintf("--volume=%v:/data:rw", cl.WorkDir),
		"--net=host",
		"--pid=host",
		cl.HyperkubeImage,
		"kube-apiserver",
		"--insecure-bind-address=0.0.0.0",
		"--service-cluster-ip-range=10.0.0.1/24",
		"--etcd-servers=http://127.0.0.1:2379",
		"--v=2")
}

func (cl *Cluster) StopApiServer() {
	if cl.containers.api == "" {
		return
	}

	Log.Logf("Stopping API server")
	cl.Docker.Kill(cl.containers.api)
	cl.containers.api = ""
}

func (cl *Cluster) WaitForApiServer() {
	deadline := time.Now().Add(startupTimeout)

	for time.Now().Before(deadline) {
		if _, err := http.Get("http://localhost:8080"); err == nil {
			Log.Logf("API server started")
			return
		}
		Log.Logf("Waiting for API server to start")
		time.Sleep(1 * time.Second)
	}

	Log.Fatal("API server failed to start")
}

func (cl *Cluster) StartKubelet() {
	Log.Logf("Starting Kubelet")

	if err := exec.Command("sudo", "mkdir", "-p", cl.varLibKubelet).Run(); err != nil {
		Log.Fatalf("Could not create %v: %v", cl.varLibKubelet, err)
	}
	makeSharedMount(cl.varLibKubelet)

	args := []string{
		"-d",
		"--volume=/:/rootfs:ro", // This is used by the nsenter mounter.
		"--volume=/sys:/sys:ro",
		"--volume=/dev:/dev",
		fmt.Sprintf("--volume=%v:/src:ro", cl.BaseDir),
		fmt.Sprintf("--volume=%v:/data:rw", cl.WorkDir),
		fmt.Sprintf("--volume=%v:/etc/kubernetes/manifests-e2e:ro", cl.manifestDir),
		fmt.Sprintf("--volume=%v:/var/lib/docker:rw", cl.varLibDocker),
		fmt.Sprintf("--volume=%v:/var/run:rw", cl.varRun),
		fmt.Sprintf("--volume=%v:/var/lib/kubelet:shared", cl.varLibKubelet),
		"--net=host",
		"--pid=host",
		"--privileged=true",
		cl.HyperkubeImage,
		"/hyperkube", "kubelet",
		"--v=4",
		"--containerized",
		"--hostname-override=0.0.0.0",
		"--address=0.0.0.0",
		"--cluster_dns=10.0.0.10",
		"--cluster_domain=cluster.local",
		"--api-servers=http://localhost:8080",
		"--config=/etc/kubernetes/manifests-e2e",
	}

	cl.containers.kubelet = cl.Docker.Run(args...)
}

func (cl *Cluster) StopKubelet() {
	if cl.containers.kubelet == "" {
		return
	}

	Log.Logf("Stopping Kubelet")

	cl.Docker.Kill(cl.containers.kubelet)
	cl.containers.kubelet = ""

	// Remove all containers created by kubelet.
	for _, tag := range cl.Docker.List("name=k8s_*") {
		cl.Docker.Kill(tag)
	}

	umount(cl.varLibKubelet)
	if err := exec.Command("sudo", "rm", "-rf", cl.varLibKubelet).Run(); err != nil {
		Log.Fatalf("Could not remove kubelet dir %v: %v", cl.varLibKubelet, err)
	}
}
