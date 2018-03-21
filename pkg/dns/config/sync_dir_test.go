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

package config

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"os"

	"crypto/sha256"

	"k8s.io/apimachinery/pkg/util/clock"
)

func TestSyncFile(t *testing.T) {

	testParentDir, err := ioutil.TempDir("", "test.filesyncsource")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { os.RemoveAll(testParentDir) }()

	testDir := filepath.Join(testParentDir, "datadir")

	fakeClock := clock.NewFakeClock(time.Now())

	source := newFileSyncSource(testDir, time.Second, fakeClock)

	// missing dir should error
	if _, err := source.Once(); err == nil {
		t.Fatalf("expected error reading missing dir")
	}

	// empty dir should return empty results
	if err := os.Mkdir(testDir, os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	result, err := source.Once()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Version != "" || len(result.Data) != 0 {
		t.Fatalf("expected empty version and data reading empty dir, got %#v", result)
	}

	// should not recurse and should ignore dot files
	if err := os.Mkdir(filepath.Join(testDir, "subdir"), os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(testDir, "subdir", "subdirfile"), []byte("test"), os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(testDir, ".hiddenfile"), []byte("test"), os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	result, err = source.Once()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Version != "" || len(result.Data) != 0 {
		t.Fatalf("expected empty version and data reading dir containing subdirs and dotfiles, got %#v", result)
	}

	// should return error if non-utf8 data is encountered
	// https://en.wikipedia.org/wiki/UTF-8#Codepage_layout
	if err := ioutil.WriteFile(filepath.Join(testDir, "binary"), []byte{192}, os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	result, err = source.Once()
	if err == nil {
		t.Fatalf("expected error reading dir containing binary data")
	}
	if err := os.Remove(filepath.Join(testDir, "binary")); err != nil {
		t.Fatal(err)
	}

	if err := ioutil.WriteFile(filepath.Join(testDir, "file1"), []byte("data1"), os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(testDir, "file2"), []byte("data2"), os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	result, err = source.Once()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	expectedResult := syncResult{
		Version: fmt.Sprintf("%x", sha256.Sum256([]byte("file1\x00data1\x00file2\x00data2\x00"))),
		Data:    map[string]string{"file1": "data1", "file2": "data2"},
	}
	if !reflect.DeepEqual(result, expectedResult) {
		t.Fatalf("expected %#v, got %#v", expectedResult, result)
	}

	resultCh := source.Periodic()

	// Result should be available right away
	select {
	case periodicResult := <-resultCh:
		if !reflect.DeepEqual(periodicResult, expectedResult) {
			t.Fatalf("Expected %#v, got %#v", expectedResult, periodicResult)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial data from period sync")
	}

	// No additional results should be available until time has passed
	select {
	case periodicResult := <-resultCh:
		t.Fatalf("unexpected periodic result before time passed: %#v", periodicResult)
	case <-time.After(time.Second):
		// waited and got no result, success
	}

	// move time forward, check new result
	fakeClock.Step(time.Second)
	select {
	case periodicResult := <-resultCh:
		if !reflect.DeepEqual(periodicResult, expectedResult) {
			t.Fatalf("Expected %#v, got %#v", expectedResult, periodicResult)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for periodic data")
	}

	// modify data
	if err := os.Remove(filepath.Join(testDir, "file1")); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(testDir, "file3"), []byte("data3"), os.FileMode(0755)); err != nil {
		t.Fatal(err)
	}
	expectedResult = syncResult{
		Version: fmt.Sprintf("%x", sha256.Sum256([]byte("file2\x00data2\x00file3\x00data3\x00"))),
		Data:    map[string]string{"file2": "data2", "file3": "data3"},
	}
	// move time forward, check new result
	fakeClock.Step(time.Second)
	select {
	case periodicResult := <-resultCh:
		if !reflect.DeepEqual(periodicResult, expectedResult) {
			t.Fatalf("expected %#v, got %#v", expectedResult, periodicResult)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for periodic data")
	}
}
