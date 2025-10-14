// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

//go:build linux

package internal

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func CreateMemfd(name string, data []byte) (int, error) {
	fd, err := unix.MemfdCreate(name, unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return 0, fmt.Errorf("failed to create memfd '%s': %v", name, err)
	}

	bytesWritten, err := unix.Write(fd, data)
	if err != nil {
		return 0, fmt.Errorf("failed to write data to memfd (fd: %d): %v", fd, err)
	}
	if bytesWritten != len(data) {
		return 0, fmt.Errorf("data mismatch in memfd (fd: %d): expected to write %d bytes, but wrote %d bytes", fd, len(data), bytesWritten)
	}

	_, err = unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS, unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_WRITE|unix.F_SEAL_SEAL)
	if err != nil {
		return 0, fmt.Errorf("failed to seal memfd (fd: %d): %v", fd, err)
	}

	return fd, nil
}
