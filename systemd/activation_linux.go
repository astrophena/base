// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build linux

package systemd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"go.astrophena.name/base/cli"
)

const sdListenFdsStart = 3

func socket(ctx context.Context, name string) (net.Listener, error) {
	env := cli.GetEnv(ctx)

	pidStr := env.Getenv("LISTEN_PID")
	if pidStr == "" {
		return nil, errors.New("systemd: LISTEN_PID not set, not running under systemd socket activation?")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, fmt.Errorf("systemd: invalid LISTEN_PID: %w", err)
	}
	if pid != os.Getpid() {
		return nil, fmt.Errorf("systemd: LISTEN_PID (%d) does not match current PID (%d)", pid, os.Getpid())
	}

	fdsStr := env.Getenv("LISTEN_FDS")
	if fdsStr == "" {
		return nil, errors.New("systemd: LISTEN_FDS not set")
	}
	numFds, err := strconv.Atoi(fdsStr)
	if err != nil {
		return nil, fmt.Errorf("systemd: invalid LISTEN_FDS: %w", err)
	}
	if numFds < 1 {
		return nil, errors.New("systemd: no file descriptors received")
	}

	namesStr := env.Getenv("LISTEN_FDNAMES")
	if namesStr == "" {
		return nil, errors.New("systemd: LISTEN_FDNAMES not set")
	}
	names := strings.Split(namesStr, ":")
	if len(names) != numFds {
		return nil, fmt.Errorf("systemd: number of file descriptor names (%d) does not match LISTEN_FDS (%d)", len(names), numFds)
	}

	fdIndex := -1
	for i, n := range names {
		if n == name {
			fdIndex = i
			break
		}
	}
	if fdIndex == -1 {
		return nil, fmt.Errorf("systemd: socket name %q not found in LISTEN_FDNAMES", name)
	}

	fd := sdListenFdsStart + fdIndex
	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		return nil, fmt.Errorf("systemd: failed to create file from descriptor %d", fd)
	}

	return net.FileListener(f)
}
