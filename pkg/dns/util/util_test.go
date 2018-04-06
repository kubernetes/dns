/*
Copyright 2018 The Kubernetes Authors.

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
	"testing"
)

func TestValidateNameserverIpAndPort(t *testing.T) {
	for _, tc := range []struct {
		wantErr bool
		ns      string
		ip      string
		port    string
	}{
		{wantErr: true},
		{ns: "1.2.3.4", ip: "1.2.3.4", port: "53"},
		{ns: "1.2.3.4:53", ip: "1.2.3.4", port: "53"},
		{wantErr: true, ns: "1.1.1.1:abc"},
		{wantErr: true, ns: "1.1.1.1:123456789"},
		{wantErr: true, ns: "1.1.1.1:"},
		{wantErr: true, ns: "invalidip"},
		{wantErr: true, ns: "invalidip:80"},
	} {
		ip, port, err := ValidateNameserverIpAndPort(tc.ns)
		gotErr := err != nil
		if gotErr != tc.wantErr {
			t.Errorf("ValidateNameserverIpAndPort(%q) = %q, %q, %v; gotErr = %t, want %t", tc.ns, ip, port, err, gotErr, tc.wantErr)
		}
		if ip != tc.ip || port != tc.port {
			t.Errorf("ValidateNameserverIpAndPort(%q) = %q, %q, nil; want %q, %q, nil", tc.ns, ip, port, tc.ip, tc.port)
		}
	}
}
