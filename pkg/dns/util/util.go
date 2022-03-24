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

package util

import (
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/skynetservices/skydns/msg"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	// ArpaSuffix is the standard suffix for PTR IP reverse lookups.
	ArpaSuffix = ".in-addr.arpa."
	// ArpaSuffixV6 is the suffix for PTR IPv6 reverse lookups.
	ArpaSuffixV6 = ".ip6.arpa."
	// defaultPriority used for service records
	defaultPriority = 10
	// defaultWeight used for service records
	defaultWeight = 10
	// defaultTTL used for service records
	defaultTTL = 30
)

// ExtractIP turns a standard PTR reverse record lookup name
// into an IP address
// Returns "", error if the reverseName is not a valid PTR lookup name
func ExtractIP(reverseName string) (string, error) {
	if strings.HasSuffix(reverseName, ArpaSuffix) {
		ip, err := extractIPv4(strings.TrimSuffix(reverseName, ArpaSuffix))
		if err != nil {
			return "", errors.Wrap(err, "incorrect PTR IPv4")
		}
		return ip, nil
	}

	if strings.HasSuffix(reverseName, ArpaSuffixV6) {
		ip, err := extractIPv6(strings.TrimSuffix(reverseName, ArpaSuffixV6))
		if err != nil {
			return "", errors.Wrap(err, "incorrect PTR IPv6")
		}
		return ip, nil
	}

	return "", fmt.Errorf("incorrect PTR: %v", reverseName)
}

// extractIPv4 turns a standard PTR reverse record lookup name
// into an IP address
func extractIPv4(reverseName string) (string, error) {
	// reverse the segments and then combine them
	segments := ReverseArray(strings.Split(reverseName, "."))

	ip := net.ParseIP(strings.Join(segments, ".")).To4()
	if ip == nil {
		return "", fmt.Errorf("failed to parse IPv4 reverse name: %v", reverseName)
	}
	return ip.String(), nil
}

// extractIPv6 turns a IPv6 PTR reverse record lookup name
// into an IPv6 address according to RFC3596
// b.a.9.8.7.6.5.0.4.0.0.0.3.0.0.0.2.0.0.0.1.0.0.0.0.0.0.0.1.2.3.4.ip6.arpa.
// is reversed to 4321:0:1:2:3:4:567:89ab
func extractIPv6(reverseName string) (string, error) {
	segments := ReverseArray(strings.Split(reverseName, "."))

	// IPv6nibbleCount is the expected number of nibbles in IPv6 PTR record as defined in rfc3596
	const ipv6nibbleCount = 32

	if len(segments) != ipv6nibbleCount {
		return "", fmt.Errorf("incorrect number of segments in IPv6 PTR: %v", len(segments))
	}

	var slice6 []string
	for i := 0; i < len(segments); i += 4 {
		slice6 = append(slice6, strings.Join(segments[i:i+4], ""))
	}

	ip := net.ParseIP(strings.Join(slice6, ":")).To16()
	if ip == nil {
		return "", fmt.Errorf("failed to parse IPv6 segments: %v", slice6)
	}
	return ip.String(), nil
}

// ReverseArray reverses an array.
func ReverseArray(arr []string) []string {
	for i := 0; i < len(arr)/2; i++ {
		j := len(arr) - i - 1
		arr[i], arr[j] = arr[j], arr[i]
	}
	return arr
}

// Returns record in a format that SkyDNS understands.
// Also return the hash of the record.
func GetSkyMsg(ip string, port int) (*msg.Service, string) {
	msg := NewServiceRecord(ip, port)
	hash := HashServiceRecord(msg)
	klog.V(5).Infof("Constructed new DNS record: %s, hash:%s",
		fmt.Sprintf("%v", msg), hash)
	return msg, fmt.Sprintf("%x", hash)
}

// NewServiceRecord creates a new service DNS message.
func NewServiceRecord(ip string, port int) *msg.Service {
	return &msg.Service{
		Host:     ip,
		Port:     port,
		Priority: defaultPriority,
		Weight:   defaultWeight,
		Ttl:      defaultTTL,
	}
}

// HashServiceRecord hashes the string representation of a DNS
// message.
func HashServiceRecord(msg *msg.Service) string {
	s := fmt.Sprintf("%v", msg)
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}

// ValidateNameserverIpAndPort splits and validates ip and port for nameserver.
// If there is no port in the given address, a default 53 port will be returned.
func ValidateNameserverIpAndPort(nameServer string) (string, string, error) {
	if ip := net.ParseIP(nameServer); ip != nil {
		return ip.String(), "53", nil
	}

	host, port, err := net.SplitHostPort(nameServer)
	if err != nil {
		return "", "", err
	}
	if ip := net.ParseIP(host); ip == nil {
		return "", "", fmt.Errorf("bad IP address: %q", host)
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return "", "", fmt.Errorf("bad port number: %q", port)
	}
	return host, port, nil
}

// IsServiceIPSet aims to check if the service's ClusterIP is set or not
// the objective is not to perform validation here
func IsServiceIPSet(service *corev1.Service) bool {
	return service.Spec.ClusterIP != corev1.ClusterIPNone && service.Spec.ClusterIP != ""
}

// GetClusterIPs returns IPs set for the service
func GetClusterIPs(service *corev1.Service) []string {
	clusterIPs := []string{service.Spec.ClusterIP}
	if len(service.Spec.ClusterIPs) > 0 {
		clusterIPs = service.Spec.ClusterIPs
	}

	// Same IPv6 could be represented differently (as from rfc5952):
	// 2001:db8:0:0:aaaa::1
	// 2001:db8::aaaa:0:0:1
	// 2001:db8:0::aaaa:0:0:1
	// net.ParseIP(ip).String() output is used as a normalization form
	// for all cases above it returns 2001:db8::aaaa:0:0:1
	// without the normalization there could be mismatches in key lookups e.g. for PTR
	normalized := make([]string, 0, len(clusterIPs))
	for _, ip := range clusterIPs {
		normalized = append(normalized, net.ParseIP(ip).String())
	}

	return normalized
}
