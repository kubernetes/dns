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
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

const (
	globalTimeout = 10 * time.Second
)

type Harness struct {
	TmpDir      string
	NannyExec   string
	MockDnsmasq string
}

func (h *Harness) Setup() {
	if err := os.Mkdir(h.TmpDir+"/config", 0755); err != nil {
		panic(err)
	}
}

func (h *Harness) Configure(stubDomains string, upstreamNameservers string) {
	writeOrRemove := func(key string, value string) {
		filename := h.TmpDir + "/config/" + key
		if value == "" {
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				return
			}
			if err := os.Remove(filename); err != nil {
				panic(err)
			}
		} else if err := ioutil.WriteFile(filename, []byte(value), 0644); err != nil {
			panic(err)
		}
	}

	writeOrRemove("stubDomains", stubDomains)
	writeOrRemove("upstreamNameservers", upstreamNameservers)
}

func (h *Harness) readOutput() []string {
	bytes, err := ioutil.ReadFile(h.TmpDir + "/args.txt")
	if err != nil {
		return []string{}
	}

	ret := []string{}
	for _, line := range strings.Split(string(bytes), "\n") {
		if line != "" {
			ret = append(ret, line)
		}
	}
	return ret
}

func (h *Harness) WaitForArgs(line string) {
	deadline := time.Now().Add(globalTimeout)
	for !time.Now().After(deadline) {
		lines := h.readOutput()
		if len(lines) > 0 && lines[len(lines)-1] == line {
			return
		}
		time.Sleep(1000 * time.Millisecond)
	}
	panic(fmt.Errorf("timeout waiting for line '%v'", line))
}
