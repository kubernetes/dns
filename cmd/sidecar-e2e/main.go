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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	dockerfile = "Dockerfile.e2e"

	dnsmasqPort = 10053
	sidecarPort = 10054
)

var opts = struct {
	mode    string
	cleanup bool

	baseDir    string
	dockerfile string

	dnsmasqBinary string
	sidecarBinary string
	digBinary     string

	outputDir string
}{
	"harness",
	false,
	".",
	"Dockerfile.e2e",
	"/usr/sbin/dnsmasq",
	"/sidecar",
	"/usr/bin/dig",

	"/test",
}

func parseArgs() {
	flag.StringVar(&opts.mode,
		"mode", opts.mode,
		"harness or test. Harness runs the test (builds the image, etc) and test"+
			" runs actual tests inside the container.")
	flag.BoolVar(&opts.cleanup,
		"cleanup", opts.cleanup,
		"Set to false to not cleanup tmp directory")

	flag.StringVar(&opts.baseDir, "baseDir", opts.baseDir,
		"base directory for the e2e test")
	flag.StringVar(&opts.dockerfile, "dockerfile", opts.dockerfile,
		"Dockerfile for e2e test")

	flag.StringVar(&opts.dnsmasqBinary, "dnsmasqBinary", opts.dnsmasqBinary,
		"location of dnsmasq")
	flag.StringVar(&opts.sidecarBinary, "sidecarBinary", opts.sidecarBinary,
		"location of sidecar")
	flag.StringVar(&opts.digBinary, "digBinary", opts.digBinary,
		"location of dig")

	flag.StringVar(&opts.outputDir, "outputDir", opts.outputDir,
		"location of output dir inside container")

	flag.Parse()
}

func logWithPrefix(prefix string, str string) {
	lines := strings.Split(str, "\n")
	for _, line := range lines {
		log.Printf("%v | %v", prefix, line)
	}
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
	tmpDir string
	image  string
}

func (h *harness) run() int {
	log.Printf("Running as harness (tmpdir = %v)", h.tmpDir)

	h.build()
	defer h.cleanup()

	h.runTests()
	return h.validate()
}

func (h *harness) build() {
	h.docker("build",
		"-f", fmt.Sprintf("%v/test/e2e/sidecar/%v", opts.baseDir, dockerfile),
		"-t", h.image,
		opts.baseDir)
}

func (h *harness) cleanup() {
	h.docker("rmi", "-f", h.image)
	if opts.cleanup {
		os.RemoveAll(h.tmpDir)
	}
}

func (h *harness) runTests() {
	dir, err := filepath.Abs(h.tmpDir)
	if err != nil {
		log.Fatal(err)
	}

	output := h.docker(
		"run", "--rm=true", "-v", fmt.Sprintf("%v:%v", dir, opts.outputDir), h.image)
	logWithPrefix("test", string(output))
}

func (h *harness) validate() int {
	var errors []error

	text, err := ioutil.ReadFile(h.tmpDir + "/metrics.log")
	if err != nil {
		log.Fatal(err)
	}

	metrics := make(map[string]float64)
	metrics["kubedns_dnsmasq_hits"] = 0
	metrics["kubedns_dnsmasq_max_size"] = 0
	metrics["kubedns_probe_notpresent_errors"] = 0
	metrics["kubedns_probe_nxdomain_errors"] = 0
	metrics["kubedns_probe_ok_errors"] = 0

	for _, line := range strings.Split(string(text), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}

		items := strings.Split(line, " ")
		if len(items) < 2 {
			continue
		}

		key := items[0]
		if _, ok := metrics[key]; ok {
			if val, err := strconv.ParseFloat(items[1], 64); err == nil {
				metrics[key] = val
			} else {
				errors = append(errors,
					fmt.Errorf("metric %v is not a number (%v)", key, items[1]))
			}
		}
	}

	expect := func(name string, op string, value float64) {
		makeError := func() error {
			return fmt.Errorf("expected %v %v %v but got %v",
				name, op, value, metrics[name])
		}

		switch op {
		case "<":
			if !(metrics[name] < value) {
				errors = append(errors, makeError())
			}
			break
		case "<=":
			if !(metrics[name] <= value) {
				errors = append(errors, makeError())
			}
			break
		case ">":
			if !(metrics[name] > value) {
				errors = append(errors, makeError())
			}
			break
		case ">=":
			if !(metrics[name] >= value) {
				errors = append(errors, makeError())
			}
			break
		case "==":
			if metrics[name] != value {
				errors = append(errors, makeError())
			}
			break
		case "!=":
			if metrics[name] == value {
				errors = append(errors, makeError())
			}
			break
		default:
			panic(fmt.Errorf("invalid op"))
		}
	}

	expect("kubedns_dnsmasq_hits", ">", 100)
	expect("kubedns_dnsmasq_max_size", "==", 1337)
	expect("kubedns_probe_notpresent_errors", ">=", 5)
	expect("kubedns_probe_nxdomain_errors", ">=", 5)
	expect("kubedns_probe_ok_errors", "==", 0)

	if len(errors) == 0 {
		log.Printf("All tests passed")

		return 0
	}

	log.Printf("Tests failed")
	for _, err := range errors {
		log.Printf("error | %v", err)
	}

	return 1
}

func (h *harness) docker(args ...string) string {
	log.Printf("docker %v", args)
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logWithPrefix("docker", string(output))
		log.Fatal(err)
	}

	return string(output)
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
		"--probe", fmt.Sprintf("ok,127.0.0.1:%v,ok.local,1", dnsmasqPort),
		"--probe", fmt.Sprintf("nxdomain,127.0.0.1:%v,nx.local,1", dnsmasqPort),
		"--probe", fmt.Sprintf("notpresent,127.0.0.1:%v,notpresent.local,1", dnsmasqPort+1),
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
			if err != nil {
				log.Fatal(err)
			}

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
	response, err := http.Get(
		fmt.Sprintf("http://127.0.0.1:%v/metrics", sidecarPort))
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

	dumpOutput := func(name string, output string) {
		err := ioutil.WriteFile(
			fmt.Sprintf("%v/%v.log", opts.outputDir, name),
			[]byte(output), 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

	dumpOutput("dnsmasq", t.dnsmasqOutput)
	dumpOutput("sidecar", t.sidecarOutput)
	dumpOutput("dig", t.digOutput)
	dumpOutput("metrics", t.metricsOutput)

	f, err := os.Create(fmt.Sprintf("%v/errors.log", opts.outputDir))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	for _, err := range t.errors {
		if _, err := f.Write([]byte(fmt.Sprintf("%v", err))); err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	parseArgs()

	log.Printf("opts=%+v", opts)

	switch opts.mode {
	case "harness":
		tmpdir, err := ioutil.TempDir("", "k8s-dns-sidecar-e2e")
		if err != nil {
			log.Fatal(err)
		}
		h := &harness{
			tmpDir: tmpdir,
			image:  fmt.Sprintf("k8s-dns-sidecar-e2e-%v", "test"),
		}
		os.Exit(h.run())
		break

	case "test":
		t := &test{}
		t.run()
		break

	default:
		log.Fatal(fmt.Errorf("Invalid --mode: %v", opts.mode))
	}
}
