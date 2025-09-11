// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/testutil"
)

func TestServerConfig(t *testing.T) {
	cases := map[string]struct {
		s       *Server
		wantErr error
	}{
		"no Addr": {
			s: &Server{
				Addr: "",
				Mux:  http.NewServeMux(),
			},
			wantErr: errNoAddr,
		},
		"invalid port": {
			s: &Server{
				Addr: ":100000",
				Mux:  http.NewServeMux(),
			},
			wantErr: errListen,
		},
	}
	for _, tc := range cases {
		err := tc.s.ListenAndServe(context.Background())

		// Don't use && because we want to trap all cases where err is nil.
		if err == nil {
			if tc.wantErr != nil {
				t.Fatalf("must fail with error: %v", tc.wantErr)
			}
		}

		if err != nil && !errors.Is(err, tc.wantErr) {
			t.Fatalf("got error: %v", err)
		}
	}
}

func TestServerListenAndServe(t *testing.T) {
	// Find a free port for us.
	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to find a free port: %v", err)
	}
	addr := fmt.Sprintf("localhost:%d", port)

	var wg sync.WaitGroup

	ready := make(chan struct{})
	readyFunc := func() {
		ready <- struct{}{}
	}
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())

	var logBuf bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	h := slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: level})
	l := &logger.Logger{
		Logger: slog.New(h),
		Level:  level,
	}
	ctx = logger.Put(ctx, l)

	s := &Server{
		Addr:       addr,
		Mux:        http.NewServeMux(),
		Debuggable: true,
		Ready:      readyFunc,
	}

	wg.Go(func() {
		if err := s.ListenAndServe(ctx); err != nil {
			errCh <- err
		}
	})

	// Wait until the server is ready.
	select {
	case err := <-errCh:
		t.Fatalf("Test server crashed during startup or runtime: %v", err)
	case <-ready:
	}

	// Make some HTTP requests.
	urls := []struct {
		url        string
		wantStatus int
	}{
		{url: "/static/css/main.css", wantStatus: http.StatusOK},
		{url: "/" + s.StaticHashName("static/css/main.css"), wantStatus: http.StatusOK},
		{url: "/health", wantStatus: http.StatusOK},
		{url: "/version", wantStatus: http.StatusOK},
	}

	for _, u := range urls {
		req, err := http.Get("http://" + addr + u.url)
		if err != nil {
			t.Fatal(err)
		}
		if req.StatusCode != u.wantStatus {
			t.Fatalf("GET %s: want status code %d, got %d", u.url, u.wantStatus, req.StatusCode)
		}
		testutil.AssertEqual(t, req.Header.Get("X-Content-Type-Options"), "nosniff")
		testutil.AssertEqual(t, req.Header.Get("Referer-Policy"), "same-origin")
		testutil.AssertEqual(t, req.Header.Get("Content-Security-Policy"), defaultCSP.String())
	}

	// Try to gracefully shutdown the server.
	cancel()
	// Wait until the server shuts down.
	wg.Wait()
	// See if the server failed to shutdown.
	select {
	case err := <-errCh:
		t.Fatalf("Test server crashed during shutdown: %v", err)
	default:
	}

	// Check logs.
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, `"msg":"listening for HTTP requests"`) {
		t.Error("expected 'listening for HTTP requests' message in logs")
	}
	if !strings.Contains(logOutput, `"msg":"handled request"`) {
		t.Error("expected 'handled request' message in logs")
	}
	if !strings.Contains(logOutput, `"url":"/health"`) {
		t.Error("expected '/health' URL in logs")
	}
	if !strings.Contains(logOutput, `"status":200`) {
		t.Error("expected status 200 in logs")
	}
	if !strings.Contains(logOutput, `"msg":"HTTP server gracefully shutting down"`) {
		t.Error("expected 'HTTP server gracefully shutting down' message in logs")
	}
}

func TestServerCSP(t *testing.T) {
	newMux := func() *http.ServeMux {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "api")
		})
		mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "page")
		})
		return mux
	}

	cspMux := NewCSPMux()
	apiPolicy := CSP{
		DefaultSrc: []string{CSPNone},
		ConnectSrc: []string{CSPSelf},
	}
	cspMux.Handle("/api/", apiPolicy)

	s := &Server{
		Mux: newMux(),
		CSP: cspMux,
	}

	// Test request to /api/data with specific policy.
	reqAPI := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	wAPI := httptest.NewRecorder()
	s.ServeHTTP(wAPI, reqAPI)
	testutil.AssertEqual(t, apiPolicy.String(), wAPI.Header().Get("Content-Security-Policy"))

	// Test request to /page, which should fall back to the default policy.
	reqPage := httptest.NewRequest(http.MethodGet, "/page", nil)
	wPage := httptest.NewRecorder()
	s.ServeHTTP(wPage, reqPage)
	testutil.AssertEqual(t, defaultCSP.String(), wPage.Header().Get("Content-Security-Policy"))

	// Test server without a CSP mux, which should also use the default policy.
	s2 := &Server{Mux: newMux()}
	reqPage2 := httptest.NewRequest(http.MethodGet, "/page", nil)
	wPage2 := httptest.NewRecorder()
	s2.ServeHTTP(wPage2, reqPage2)
	testutil.AssertEqual(t, defaultCSP.String(), wPage2.Header().Get("Content-Security-Policy"))
}

// getFreePort asks the kernel for a free open port that is ready to use.
// Copied from
// https://github.com/phayes/freeport/blob/74d24b5ae9f58fbe4057614465b11352f71cdbea/freeport.go.
func getFreePort() (port int, err error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
