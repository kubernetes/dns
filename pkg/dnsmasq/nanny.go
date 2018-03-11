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
// command line. This function will also remove "--" and subsequent
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
func (n *Nanny) Configure(args []string, config *config.Config) {
	n.args = args

	munge := func(s string) string {
		if colonIndex := strings.LastIndex(s, ":"); colonIndex != -1 {
			bracketIndex := strings.Index(s, "]")
			isV4 := strings.Count(s, ":") == 1
			isBracketedV6 := bracketIndex != -1
			if isV4 || isBracketedV6 && colonIndex > bracketIndex {
				s = s[:colonIndex] + "#" + s[colonIndex+1:]
			}
		}
		return s
	}

	for domain, serverList := range config.StubDomains {
		for _, server := range serverList {
			// dnsmasq port separator is '#' for some reason.
			server = munge(server)
			n.args = append(
				n.args, "--server", fmt.Sprintf("/%v/%v", domain, server))
		}
	}

	for _, server := range config.UpstreamNameservers {
		// dnsmasq port separator is '#' for some reason.
		server = munge(server)
		n.args = append(n.args, "--server", server)
	}

	// If upstream nameservers are explicitly specified, then do not look
	// at /etc/resolv.co
