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
package e2e

import (
	"os/exec"
	"time"
)

const (
	StandardTimeout = 10 * time.Second
)

// keepSudoActive periodically updates the sudo timestamp so we can keep
// running sudo.
func KeepSudoActive() {
	go func() {
		if err := exec.Command("sudo", "-nv").Run(); err != nil {
			if err != nil {
				Log.Fatalf("Unable to keep sudo active: %v", err)
			}
		}
		time.Sleep(10 * time.Second)
	}()
}

// CanSudo returns true if the sudo command is allowed without a password.
func CanSudo() bool {
	cmd := exec.Command("sudo", "-nv")
	return cmd.Run() == nil
}

func makeSharedMount(path string) {
	if err := exec.Command("sudo", "mount", "--bind", path, path).Run(); err != nil {
		Log.Fatalf("Error bind mounting %v: %v", path, err)
	}
	if err := exec.Command("sudo", "mount", "--make-rshared", path).Run(); err != nil {
		Log.Fatalf("Error mount --make-rshared %v: %v", path, err)
	}
}

func umount(path string) {
	if err := exec.Command("sudo", "umount", path).Run(); err != nil {
		Log.Fatalf("Error umount %v: %v", path, err)
	}
}
