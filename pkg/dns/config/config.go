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

package config

import (
	"fmt"
	"net"
	"strconv"

	"github.com/coredns/coredns/plugin/pkg/parse"
	types "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	fed "k8s.io/dns/pkg/dns/federation"
	"k8s.io/dns/pkg/dns/util"
)

// Config populated either from the configuration source (command
// line flags or via the config map mechanism).
type Config struct {
	// The inclusion of TypeMeta is to ensure future compatibility if the
	// Config object was populated directly via a Kubernetes API mechanism.
	//
	// For example, instead of the custom implementation here, the
	// configuration could be obtained from an API that unifies
	// command-line flags, config-map, etc mechanisms.
	types.TypeMeta

	// Map of federation names that the cluster in which this kube-dns
	// is running belongs to, to the corresponding domain names.
	Federations map[string]string `json:"federations"`

	// Map of stub domain to nameserver IP. The key is the domain name suffix,
	// e.g. "acme.local". Key cannot be equal to the cluster domain. Value is
	// the IP of the nameserver to send DNS request for the given subdomain.
	StubDomains map[string][]string `json:"stubDomains"`

	// List of upstream nameservers to use. Overrides nameservers inherited
	// from the node.
	UpstreamNameservers []string `json:"upstreamNameservers"`
}

func NewDefaultConfig() *Config {
	return &Config{
		Federations: map[string]string{},
		StubDomains: map[string][]string{},
	}
}

// Validate returns whether or not the configuration is valid.
func (config *Config) Validate() error {
	if err := config.validateFederations(); err != nil {
		return err
	}

	if err := config.validateStubDomains(); err != nil {
		return err
	}

	if err := config.validateUpstreamNameserver(); err != nil {
		return err
	}

	return nil
}

func (config *Config) validateFederations() error {
	for name, domain := range config.Federations {
		if err := fed.ValidateName(name); err != nil {
			return err
		}
		if err := fed.ValidateDomain(domain); err != nil {
			return err
		}
	}
	return nil
}

func (config *Config) validateStubDomains() error {
	for domain, nsList := range config.StubDomains {
		if len(validation.IsDNS1123Subdomain(domain)) != 0 {
			return fmt.Errorf("invalid domain name: %q", domain)
		}

		for _, ns := range nsList {
			host, port, err := net.SplitHostPort(ns)
			// it can error if the port is missing
			// or if there are too many colons (invalid host)
			// so we assume that ns is passed without port
			// and fail later in validation if the host was invalid
			if err != nil {
				host = ns
			}
			// Validate port if specified
			if port != "" {
				if _, err := strconv.ParseUint(port, 10, 16); err != nil {
					return fmt.Errorf("invalid nameserver: %q", ns)
				}
			}
			if len(validation.IsValidIP(host)) > 0 && len(validation.IsDNS1123Subdomain(ns)) > 0 {
				return fmt.Errorf("invalid nameserver: %q", ns)
			}
		}
	}

	return nil
}

func (config *Config) validateUpstreamNameserver() error {
	if len(config.UpstreamNameservers) > 3 {
		return fmt.Errorf("upstreamNameserver cannot have more than three entries")
	}

	for _, nameServer := range config.UpstreamNameservers {
		if _, _, err := util.ValidateNameserverIpAndPort(nameServer); err != nil {
			return err
		}
	}
	return nil
}

// ValidateNodeLocalCacheConfig returns nil if the config can be compiled
// to a valid Corefile.
func (config *Config) ValidateNodeLocalCacheConfig() error {
	for domain, nameservers := range config.StubDomains {
		if err := validateForwardProxy(nameservers...); err != nil {
			return fmt.Errorf("invalid nameservers %s for the stub domain %s: %v", nameservers, domain, err)
		}
	}
	if err := validateForwardProxy(config.UpstreamNameservers...); err != nil {
		return err
	}
	return nil
}

// validateForwardProxy returns nil if the nameservers are valid proxy addresses
// for the CoreDNS plugin forward.
// The function is ported from coredns/plugin/forward:parseStanza
func validateForwardProxy(nameservers ...string) error {
	if len(nameservers) == 0 {
		return nil
	}
	hosts, err := parse.HostPortOrFile(nameservers...)
	if err != nil {
		return err
	}
	for _, host := range hosts {
		trans, _ := parse.Transport(host)
		switch trans {
		case "dns", "tls":
		default:
			return fmt.Errorf("unsupported transport %s of nameserver %s", trans, host)
		}
	}
	return nil
}
