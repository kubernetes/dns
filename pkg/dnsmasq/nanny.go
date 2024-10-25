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
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"

	"k8s.io/dns/pkg/dns/config"
	"k8s.io/klog/v2"
)

// This is a noop change just to verify the correctness of tests.

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
// kubednsServer is the address of the local kubedns instance used to do name resolution for non-IP names.
func (n *Nanny) Configure(args []string, config *config.Config, kubednsServer string) {
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
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "udp", kubednsServer)
			},
		}
		for _, server := range serverList {
			if isIP := (net.ParseIP(server) != nil); !isIP {
				switch {
				case strings.HasSuffix(server, "cluster.local"):
					if IPs, err := resolver.LookupIPAddr(context.Background(), server); err != nil {
						klog.Errorf("Error looking up IP for name %q: %v", server, err)
					} else if len(IPs) > 0 {
						server = IPs[0].String()
					} else {
						klog.Errorf("Name %q does not resolve to any IPs", server)
					}
				default:
					if IPs, err := net.LookupIP(server); err != nil {
						klog.Errorf("Error looking up IP for name %q: %v", server, err)
					} else if len(IPs) > 0 {
						server = IPs[0].String()
					} else {
						klog.Errorf("Name %q does not resolve to any IPs", server)
					}
				}
			}
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
	// at /etc/resolv.conf.
	if len(config.UpstreamNameservers) > 0 {
		n.args = append(n.args, "--no-resolv")
	}
}

// Start the nanny.
func (n *Nanny) Start() error {
	klog.V(0).Infof("Starting dnsmasq %v", n.args)

	n.cmd = exec.Command(n.Exec, n.args...)
	stderrReader, err := n.cmd.StderrPipe()
	if err != nil {
		return err
	}

	stdoutReader, err := n.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := n.cmd.Start(); err != nil {
		return err
	}

	logToGlog := func(stream string, reader io.Reader) {
		bufReader := bufio.NewReader(reader)
		for {
			bytes, err := bufReader.ReadBytes('\n')
			if len(bytes) > 0 {
				klog.V(1).Infof("%v", string(bytes))
			}
			if err == io.EOF {
				klog.V(1).Infof("%v", string(bytes))
				klog.Warningf("Got EOF from %v", stream)
				return
			} else if err != nil {
				klog.V(1).Infof("%v", string(bytes))
				klog.Errorf("Error reading from %v: %v", stream, err)
				return
			}
		}
	}

	go logToGlog("stderr", stderrReader)
	go logToGlog("stdout", stdoutReader)

	n.ExitChannel = make(chan error)
	go func() {
		n.ExitChannel <- n.cmd.Wait()
	}()

	return nil
}

// Kill the running Nanny.
func (n *Nanny) Kill() error {
	klog.V(0).Infof("Killing dnsmasq")
	if n.cmd == nil {
		return fmt.Errorf("Process is not running")
	}

	if err := n.cmd.Process.Kill(); err != nil {
		klog.Errorf("Error killing dnsmasq: %v", err)
		return err
	}

	n.cmd = nil

	return nil
}

// RunNannyOpts for running the nanny.
type RunNannyOpts struct {
	// Location of the dnsmasq executable.
	DnsmasqExec string
	// Extra arguments to dnsmasq.
	DnsmasqArgs []string
	// Restart the daemon on ConfigMap changes.
	RestartOnChange bool
}

// RunNanny runs the nanny and handles configuration updates.
func RunNanny(sync config.Sync, opts RunNannyOpts, kubednsServer string) {
	defer klog.Flush()

	currentConfig, err := sync.Once()
	if err != nil {
		klog.Errorf("Error getting initial config, using default: %v", err)
		currentConfig = config.NewDefaultConfig()
	}

	nanny := &Nanny{Exec: opts.DnsmasqExec}
	nanny.Configure(opts.DnsmasqArgs, currentConfig, kubednsServer)
	if err := nanny.Start(); err != nil {
		klog.Fatalf("Could not start dnsmasq with initial configuration: %v", err)
	}

	configChan := sync.Periodic()

	for {
		select {
		case status := <-nanny.ExitChannel:
			klog.Flush()
			klog.Fatalf("dnsmasq exited: %v", status)
			break
		case currentConfig = <-configChan:
			if opts.RestartOnChange {
				klog.V(0).Infof("Restarting dnsmasq with new configuration")
				nanny.Kill()
				nanny = &Nanny{Exec: opts.DnsmasqExec}
				nanny.Configure(opts.DnsmasqArgs, currentConfig, kubednsServer)
				nanny.Start()
			} else {
				klog.V(2).Infof("Not restarting dnsmasq (--restartDnsmasq=false)")
			}
			break
		}
	}
}
