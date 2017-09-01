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

package app

import (
	"strings"
	"testing"
)

func TestValidateNameServer(t *testing.T) {
	const noError = ""

	testCases := []struct {
		serverIn      string
		serverOut     string
		expectedError string
	}{
		{"1.2.3.4", "1.2.3.4:53", noError},
		{"1.2.3.4:10053", "1.2.3.4:10053", noError},
		{"2001:db8::1:1", "[2001:db8::1:1]:53", noError},
		{"[2001:db8::2:2]:10053", "[2001:db8::2:2]:10053", noError},
		{"1.2.3.4::10053", "", "too many colons"},
		{"1.2.3.4.5:10053", "", "bad IP address"},
		{"1.2.3.4:0", "", "bad port number"},
		{"1.2.3.4:65536", "", "bad port number"},
		{"[2001:db8::2:2]", "", "missing port in address"},
	}

	for _, tc := range testCases {
		serverOut, err := validateNameServer(tc.serverIn)
		if tc.expectedError == noError {
			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tc.serverIn, err)
			}
			if serverOut != tc.serverOut {
				t.Errorf("Unexpected output for %s: Expected: %s, Got %s", tc.serverIn, tc.serverOut, serverOut)
			}
		} else {
			if err == nil {
				t.Errorf("Error did not occur for %s, expected: '%s' error", tc.serverIn, tc.expectedError)
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Unexpected error for %s: Expected: %s, Got %v", tc.serverIn, tc.expectedError, err)
			}
		}
	}
}
