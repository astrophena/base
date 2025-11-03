// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build linux && !android

package systemd

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestSocket(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	listenerAddr := l.Addr()

	lf, err := l.(*net.TCPListener).File()
	if err != nil {
		l.Close()
		t.Fatalf("File(): %v", err)
	}

	fd := lf.Fd()

	// Move the file descriptor to the correct position (3).
	if fd != 3 {
		err := syscall.Dup2(int(fd), 3)
		if err != nil {
			lf.Close()
			l.Close()
			t.Fatalf("dup2: %v", err)
		}
		// The original fd from lf is now duplicated to 3. We can close the original.
		syscall.Close(int(fd))
	}

	// We must close the original listener so that the runtime doesn't know about it.
	// This allows net.FileListener to create a new listener object for the same socket.
	l.Close()

	// Clean up fd 3 when the test function completes.
	t.Cleanup(func() {
		syscall.Close(3)
	})

	cases := map[string]struct {
		env         map[string]string
		socketName  string
		wantErr     bool
		wantErrText string
	}{
		"success": {
			env: map[string]string{
				"LISTEN_PID":     strconv.Itoa(os.Getpid()),
				"LISTEN_FDS":     "1",
				"LISTEN_FDNAMES": "http",
			},
			socketName: "http",
		},
		"wrong PID": {
			env: map[string]string{
				"LISTEN_PID":     "1",
				"LISTEN_FDS":     "1",
				"LISTEN_FDNAMES": "http",
			},
			socketName:  "http",
			wantErr:     true,
			wantErrText: "does not match current PID",
		},
		"name not found": {
			env: map[string]string{
				"LISTEN_PID":     strconv.Itoa(os.Getpid()),
				"LISTEN_FDS":     "1",
				"LISTEN_FDNAMES": "https",
			},
			socketName:  "http",
			wantErr:     true,
			wantErrText: `socket name "http" not found`,
		},
		"no LISTEN_PID": {
			env:         map[string]string{},
			socketName:  "http",
			wantErr:     true,
			wantErrText: "LISTEN_PID not set",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			listener, err := Socket(context.Background(), tc.socketName)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, but got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("expected error to contain %q, got: %v", tc.wantErrText, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if listener == nil {
				t.Fatal("expected a listener, but got nil")
			}
			defer listener.Close()

			testutil.AssertEqual(t, listenerAddr.String(), listener.Addr().String())
		})
	}
}
