// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build linux && !android

package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestServerSocketActivation(t *testing.T) {
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
		syscall.Close(int(fd))
	}

	l.Close()

	t.Cleanup(func() {
		syscall.Close(3)
	})

	t.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
	t.Setenv("LISTEN_FDS", "1")
	t.Setenv("LISTEN_FDNAMES", "http")

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	s := &Server{
		Addr: "sd-socket:http",
		Mux:  mux,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		if err := s.ListenAndServe(ctx); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make request
	resp, err := http.Get("http://" + listenerAddr.String() + "/test")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d; want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("got body %q; want %q", body, "ok")
	}

	cancel()
	wg.Wait()

	select {
	case err := <-errCh:
		t.Fatalf("ListenAndServe returned an error: %v", err)
	default:
	}
}
