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

package dns

import (
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/miekg/dns"
	om "github.com/onsi/gomega"

	"k8s.io/dns/pkg/e2e"
)

// KubeDNS daemon
type KubeDNS struct {
	cmd       *exec.Cmd
	isRunning bool
}

// Start kube DNS, passing in extra arguments
func (kd *KubeDNS) Start(args ...string) {
	fr := e2e.GetFramework()
	bin := fr.Path("bin/amd64/kube-dns")

	args = append(
		args,
		"--logtostderr",
		"--dns-port", "10053",
		"--kubecfg-file", fr.Path("test/e2e/cluster/config"))

	var err error
	kd.cmd, err = fr.RunInBackground("kube-dns", bin, args...)
	if err != nil {
		log.Fatal(err)
	}

	kd.isRunning = true

	go func() {
		kd.cmd.Wait()
		kd.isRunning = false
	}()

	om.Eventually(func() error {
		conn, err := net.Dial("tcp", "localhost:10053")
		if err == nil {
			conn.Close()
		}
		return err
	}).Should(om.Succeed())

	e2e.Log.Logf("kube-dns started")
}

// Stop kube DNS
func (kd *KubeDNS) Stop() {
	e2e.Log.Logf("Stopping kube-dns")

	om.Expect(kd.isRunning).Should(om.Equal(true), "kube-dns is not running")

	// kube-dns will flush its logs if sent a SIGINT (will not exit until it
	// is sent a SIGKILL). This allows us to pick up anything that may still
	// be buffered in glog.
	kd.cmd.Process.Signal(os.Interrupt)
	time.Sleep(200 * time.Millisecond)

	kd.cmd.Process.Signal(os.Kill)
}

// Query the DNS server. Returns the DNS records as strings.
func (kd *KubeDNS) Query(name string, qtype uint16) ([]string, error) {
	msg := &dns.Msg{}
	msg.Id = dns.Id()
	msg.Question = append(
		msg.Question,
		dns.Question{Name: name, Qtype: qtype, Qclass: dns.ClassINET})

	client := &dns.Client{}
	msg, _, err := client.Exchange(msg, "localhost:10053")
	if err != nil {
		return []string{}, err
	}

	var ret []string
	for _, ans := range msg.Answer {
		ret = append(ret, ans.String())
	}

	return ret, nil
}
