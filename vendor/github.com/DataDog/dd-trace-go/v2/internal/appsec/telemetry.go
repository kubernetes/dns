// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package appsec

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/internal/appsec/config"
	"github.com/DataDog/dd-trace-go/v2/internal/log"
	"github.com/DataDog/dd-trace-go/v2/internal/telemetry"
	telemetrylog "github.com/DataDog/dd-trace-go/v2/internal/telemetry/log"
	"github.com/DataDog/go-libddwaf/v4"
	"github.com/DataDog/go-libddwaf/v4/waferrors"
)

var (
	detectLibDLOnce     sync.Once
	wafUsable, wafError = libddwaf.Usable()
	wafSupported        = !errors.As(wafError, &waferrors.UnsupportedOSArchError{}) && !errors.As(wafError, &waferrors.UnsupportedGoVersionError{})
	staticConfigs       = []telemetry.Configuration{
		{Name: "goos", Value: runtime.GOOS, Origin: telemetry.OriginCode},
		{Name: "goarch", Value: runtime.GOARCH, Origin: telemetry.OriginCode},
		{Name: "cgo_enabled", Value: cgoEnabled, Origin: telemetry.OriginCode},
		{Name: "waf_supports_target", Value: wafSupported, Origin: telemetry.OriginCode},
		{Name: "waf_healthy", Value: wafUsable, Origin: telemetry.OriginCode},
	}
)

// init sends the static telemetry for AppSec.
func init() {
	telemetry.RegisterAppConfigs(staticConfigs...)
}

func registerAppsecStartTelemetry(mode config.EnablementMode, origin telemetry.Origin) {
	if mode == config.RCStandby {
		return
	}

	if origin == telemetry.OriginCode {
		telemetry.RegisterAppConfig("WithEnablementMode", mode, telemetry.OriginCode)
	}

	telemetry.ProductStarted(telemetry.NamespaceAppSec)
	// TODO: add appsec.enabled metric once this metric is enabled backend-side

	detectLibDLOnce.Do(detectLibDL)
}

func detectLibDL() {
	if runtime.GOOS != "linux" {
		return
	}

	for _, method := range detectLibDLMethods {
		if ok, err := method.method(); ok {
			telemetrylog.Debug("libdl detected using method: %s", method.name, telemetry.WithTags([]string{"method:" + method.name}))
			log.Debug("libdl detected using method: %s", method.name)
			telemetry.RegisterAppConfig("libdl_present", true, telemetry.OriginCode)
			return
		} else if err != nil {
			log.Debug("failed to detect libdl with method %s: %v", method.name, err.Error())
		}
	}

	telemetry.RegisterAppConfig("libdl_present", false, telemetry.OriginCode)
}

func registerAppsecStopTelemetry() {
	telemetry.ProductStopped(telemetry.NamespaceAppSec)
}

var detectLibDLMethods = []struct {
	name   string
	method func() (bool, error)
}{
	{"cgo", func() (bool, error) {
		return cgoEnabled, nil
	}},
	{"ldconfig", ldconfig},
	{"ldsocache", ldCache},
	{"ldd", ldd},
	{"proc_maps", procMaps},
	{"manual_search", manualSearch},
}

// ldCache is messily looking into /etc/ld.so.cache to check if libdl.so.2 is present.
// Normally ld.so.cache should be parsed but standards differ so simply looking for the string should make sense.
// It is sadly disabled by default in alpine images.
func ldCache() (bool, error) {
	fp, err := os.Open("/etc/ld.so.cache")
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // ld.so.cache does not exist, so we assume libdl is not present
		}
		return false, err
	}
	defer fp.Close()

	output, err := io.ReadAll(io.LimitReader(fp, libDLReadLimit))
	if err != nil {
		return false, err
	}

	return searchLibdl(output), nil
}

// ldd on ourself will check if libdl.so if we are currently running with libdl. It needs the ldd binary.
// We first try to check the whole system, then we check the current process.
func ldd() (bool, error) {
	var output limitedBuffer
	cmd := exec.Command("ldd", "/proc/1/exe")
	cmd.Stdout = &output
	cmd.Stderr = io.Discard

	oneErr := cmd.Run()

	if searchLibdl(output.Bytes()) {
		return true, nil
	}

	var selfOutput limitedBuffer
	cmd = exec.Command("ldd", "/proc/self/exe")
	cmd.Stdout = &output
	cmd.Stderr = io.Discard

	selfErr := cmd.Run()

	return searchLibdl(selfOutput.Bytes()), errors.Join(oneErr, selfErr)
}

// ldconfig -p is the most reliable way to check for libdl.so presence but it does not work on musl. It also
// needs the ldconfig binary, which is not always available in containers or minimal environments.
func ldconfig() (bool, error) {
	var output limitedBuffer
	cmd := exec.Command("ldconfig", "-p")
	cmd.Stdout = &output
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return false, err
	}

	return searchLibdl(output.Bytes()), nil
}

// procMaps is another way to check for libdl.so presence, that works on musl if we are running with libdl already.
// but does not always work because libdl can be symlink.
// We first try to check the whole system, then we check the current process.
func procMaps() (bool, error) {
	fp, err := os.Open("/proc/1/maps")
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return false, nil
		}
		return false, err
	}

	defer fp.Close()

	output, oneErr := io.ReadAll(io.LimitReader(fp, libDLReadLimit))

	if searchLibdl(output) {
		return true, nil
	}

	fp, err = os.Open("/proc/self/maps")
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	defer fp.Close()

	output, selfErr := io.ReadAll(io.LimitReader(fp, libDLReadLimit))

	return searchLibdl(output), errors.Join(oneErr, selfErr)
}

// manualSearch is a fallback method to search for libdl.so.2 in common library directories.
// See ld.so(8) for more details on the directories searched by the dynamic linker.
func manualSearch() (bool, error) {
	for _, dir := range []string{"/lib", "/usr/lib", "/lib64", "/usr/lib64"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, err
		}

		for _, entry := range entries {
			if !entry.IsDir() && entry.Name() == libDLName {
				return true, nil
			}
		}
	}

	return false, nil
}

func searchLibdl(input []byte) bool {
	data := bytes.TrimSpace(input)
	if len(data) == 0 {
		return false
	}

	return bytes.Contains(data, []byte(libDLName))
}

// limitedBuffer is a custom buffer that limits its size to 256 KiB.
type limitedBuffer struct {
	bytes.Buffer
}

const (
	libDLReadLimit = 256 * 1024
	libDLName      = "libdl.so.2"
)

func (b *limitedBuffer) Write(p []byte) (n int, err error) {
	if b.Len()+len(p) > libDLReadLimit { // 256 KiB limit
		return 0, io.ErrShortWrite
	}
	return b.Buffer.Write(p)
}
