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

package e2e

import (
	"log"
	"strings"
)

var Log Logger = &StandardLogger{}

// Logger wraps common log and Gingko logging.
type Logger interface {
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	LogWithPrefix(prefix string, str string)
}

type StandardLogger struct{}

func (*StandardLogger) Fatal(args ...interface{}) {
	log.Fatal(args...)
}

func (*StandardLogger) Fatalf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

func (*StandardLogger) Log(args ...interface{}) {
	log.Print(args...)
}

func (*StandardLogger) Logf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func (l *StandardLogger) LogWithPrefix(prefix string, str string) {
	LogWithPrefix(log.Printf, prefix, str)
}

func LogWithPrefix(lf func(format string, args ...interface{}), prefix string, str string) {
	lines := strings.Split(str, "\n")
	for _, line := range lines {
		lf("%v | %v", prefix, line)
	}
}
