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
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"os/exec"
)

const (
	dnsmasqBinary = "/usr/sbin/dnsmasq"
	sidecarBinary = "/sidecar"
	digBinary     = "/usr/bin/dig"

	dnsmasqPort = 10053
	sidecarPort = 10054
)

var opts struct {
	mode string

	dnsmasqBinary string
	sidecarBinary string
	digBinary     string
}

func parseArgs() {
	flag.StringVar(&opts.mode,
		"mode", "harness",
		"harness or test. Harness runs the test (builds the image, etc) and test"+
			" runs actual tests inside the container.")
	flag.StringVar(&opts.dnsmasqBinary,
		"dnsmasqBinary", "/usr/sbin/dnsmasq", "location of dnsmasq")
	flag.StringVar(&opts.sidecarBinary,
		"sidecarBinary", "/sidecar", "location of sidecar")
	flag.StringVar(&opts.digBinary,
		"digBinary", "/usr/bin/dig", "location of dig")

	flag.Parse()
}

func waitForPredicate(predicate func() (bool, error), duration time.Duration, interval time.Duration) error {
	var err error

	start := time.Now()

	for time.Since(start) < duration {
		var stop bool
		stop, err = predicate()
		if stop {
			return err
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("timeout (last error was %v)", err)
}

func waitForTcpOrExit(cmd *exec.Cmd, endpoint string) error {
	exitChan := make(chan error)
	go func() {
		_, err := cmd.Process.Wait()
		exitChan <- err
	}()

	return waitForPredicate(
		func() (bool, error) {
			// Check to see that the process has not died early.
			select {
			case err := <-exitChan:
				return true, fmt.Errorf("Process died: %v", err)
			default:
			}

			conn, err := net.Dial("tcp", endpoint)
			if err == nil {
				conn.Close()
				return true, nil
			} else {
				return false, err
			}
		},
		10*time.Second,
		1*time.Second)
}

func readToBuf(lock *sync.Mutex, out *string, reader io.Reader) {
	for {
		buf := make([]byte, 4096)
		n, err := io.ReadAtLeast(reader, buf, 1)
		if err != nil {
			return
		}

		lock.Lock()
		*out += string(buf[0:n])
		lock.Unlock()
	}
}

func startDumpToBuf(lock *sync.Mutex, out *string, cmd *exec.Cmd) {
	if pipe, err := cmd.StdoutPipe(); err == nil {
		go readToBuf(lock, out, pipe)
	} else {
		log.Fatal(err)
	}
	if pipe, err := cmd.StderrPipe(); err == nil {
		go readToBuf(lock, out, pipe)
	} else {
		log.Fatal(err)
	}
}

type harness struct {
	outputDir string
	image     string
}

func (h *harness) run() {
	log.Printf("Running as harness")

	h.build()
	defer h.cleanup()

	h.runTests()
}

func (h *harness) runTests() {
}

func (h *harness) build() {
	exec.Command("docker", "build", "-f", "Dockerfile.e2e", "-t", h.image)
}

func (h *harness) cleanup() {
}

type test struct {
	dnsmasq *exec.Cmd
	sidecar *exec.Cmd

	lock          sync.Mutex
	dnsmasqOutput string
	sidecarOutput string
	metricsOutput string

	digOutput string
	errors    []error
}

func (t *test) run() {
	log.Printf("Running as test")

	t.runDnsmasq()
	t.runSidecar()
	t.runDig()
	t.waitForMetrics()
	t.getMetrics()
	t.dump()
}

func (t *test) runDnsmasq() {
	args := []string{
		"-q", "-k",
		"-a", "127.0.0.1",
		"-p", strconv.Itoa(dnsmasqPort),
		"-c", "1337",
		"-8", "-",
		"-A", "/ok.local/1.2.3.4",
		"-A", "/nxdomain.local/",
	}
	t.dnsmasq = exec.Command(opts.dnsmasqBinary, args...)

	log.Printf("Starting dnsmasq %v", args)
	startDumpToBuf(&t.lock, &t.dnsmasqOutput, t.dnsmasq)
	err := t.dnsmasq.Start()
	if err != nil {
		log.Fatal(err)
	}

	if err := waitForTcpOrExit(t.dnsmasq, fmt.Sprintf("127.0.0.1:%v", dnsmasqPort)); err != nil {
		log.Fatal(err)
	}
	log.Printf("dnsmasq started")
}

func (t *test) runSidecar() {
	args := []string{
		"-v", "4",
		"--prometheus-port", strconv.Itoa(sidecarPort),
		"--dnsmasq-port", strconv.Itoa(dnsmasqPort),
		"--probe",
		fmt.Sprintf("ok,127.0.0.1:%v,ok.local,1", dnsmasqPort),
		"--probe",
		fmt.Sprintf("nxdomain,127.0.0.1:%v,nx.local,1", dnsmasqPort),
		"--probe",
		fmt.Sprintf("notpresent,127.0.0.1:%v,notpresent.local,1", dnsmasqPort+1),
	}
	t.sidecar = exec.Command(opts.sidecarBinary, args...)

	log.Printf("Starting sidecar %v", args)
	startDumpToBuf(&t.lock, &t.sidecarOutput, t.sidecar)
	err := t.sidecar.Start()
	if err != nil {
		log.Fatal(err)
	}

	if err := waitForTcpOrExit(t.sidecar, fmt.Sprintf("127.0.0.1:%v", sidecarPort)); err != nil {
		log.Fatal(err)
	}
	log.Printf("sidecar started")
}

func (t *test) runDig() {
	log.Printf("running `dig`")
	for i := 0; i < 100; i++ {
		dig := exec.Command(
			opts.digBinary, "@127.0.0.1", "-p", strconv.Itoa(dnsmasqPort), "localhost")
		output, err := dig.CombinedOutput()
		if err == nil {
			t.digOutput += string(output)
		} else {
			t.errors = append(t.errors, fmt.Errorf("Error running dig: %v", err))
		}
	}
}

func (t *test) waitForMetrics() {
	log.Printf("Waiting for hits to be reported to be greater than 100")
	waitForPredicate(
		func() (bool, error) {
			response, err := http.Get(
				fmt.Sprintf("http://127.0.0.1:%v/metrics", sidecarPort))
			if err != nil {
				log.Fatal(err)
			}

			defer response.Body.Close()
			buf, err := ioutil.ReadAll(response.Body)

			lines := strings.Split(string(buf), "\n")
			for _, line := range lines {
				if !strings.HasPrefix(line, "kubedns_dnsmasq_hits ") {
					continue
				}

				parts := strings.Split(line, " ")
				if len(parts) < 2 {
					return false, fmt.Errorf("invalid output for kubedns_dnsmasq_hits metric")
				}

				value, err := strconv.Atoi(parts[1])
				if err != nil {
					return false, fmt.Errorf("invalid output for kubedns_dnsmasq_hits metric")
				}

				if value >= 100 {
					return true, nil
				}
			}
			return false, nil
		},
		10*time.Second,
		1*time.Second)
}

func (t *test) getMetrics() {
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%v/metrics", sidecarPort))
	if err != nil {
		log.Fatal(err)
	}

	defer response.Body.Close()
	buf, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	t.metricsOutput = string(buf)
}

func (t *test) dump() {
	t.lock.Lock()
	defer t.lock.Unlock()

	fmt.Println("BEGIN dnsmasq ====")
	fmt.Println(t.dnsmasqOutput)
	fmt.Println("END dnsmasq ====")
	fmt.Println()
	fmt.Println("BEGIN sidecar ====")
	fmt.Println(t.sidecarOutput)
	fmt.Println("END sidecar ====")
	fmt.Println()
	fmt.Println("BEGIN dig ====")
	fmt.Println(t.digOutput)
	fmt.Println("END dig ====")
	fmt.Println()
	fmt.Println("BEGIN metrics ====")
	fmt.Println(t.metricsOutput)
	fmt.Println("END metrics ====")
	fmt.Println()
	fmt.Println("BEGIN errors ====")
	for _, err := range t.errors {
		fmt.Println(err)
	}
	fmt.Println("END errors ====")

}

func main() {
	parseArgs()

	switch opts.mode {
	case "harness":
		h := &harness{
			outputDir: "/tmp",
			image:     fmt.Sprintf("k8s-dns-sidecar-e2e-%v", time.Now().Second()),
		}
		h.run()
		break

	case "test":
		t := &test{}
		t.run()
		break

	default:
		log.Fatal(fmt.Errorf("Invalid --mode: %v", opts.mode))
	}
}
