// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.astrophena.name/base/metrics"
)

func TestServerMetrics(t *testing.T) {
	r := new(metrics.Registry)
	if err := RegisterMetrics(r); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /artifact/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})
	s := &Server{Mux: mux, MetricsEndpoint: "public"}
	ctx := metrics.Put(context.Background(), r)
	for _, target := range []string{"/artifact/private-name", "/absent"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx))
	}
	var buf bytes.Buffer
	if err := r.Write(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{
		`http_server_requests_total{endpoint="public",method="GET",route="GET /artifact/{id}",status="201"} 1`,
		`http_server_requests_total{endpoint="public",method="GET",route="unmatched",status="404"} 1`,
		`http_server_response_size_bytes_bucket{endpoint="public",method="GET",route="GET /artifact/{id}",status="201",le="1024"} 1`,
		`http_server_requests_in_flight{endpoint="public",method="GET",route="GET /artifact/{id}"} 0`,
		`go_goroutines `,
		`go_gc_cycles_total `,
	} {
		if !strings.Contains(buf.String(), part) {
			t.Errorf("snapshot missing %q", part)
		}
	}
	if strings.Contains(buf.String(), "private-name") {
		t.Fatal("raw URL leaked into metric label")
	}
}

func TestHijackedConnectionMetrics(t *testing.T) {
	r := new(metrics.Registry)
	if err := RegisterMetrics(r); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /plug", func(w http.ResponseWriter, req *http.Request) {
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer close(done)
		if _, err := buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: plug\r\n\r\n"); err != nil {
			_ = conn.Close()
			return
		}
		if err := buffered.Flush(); err != nil {
			_ = conn.Close()
			return
		}
		<-release
		_ = conn.Close()
	})
	ready := make(chan struct{})
	sock := filepath.Join(t.TempDir(), "web.sock")
	s := &Server{Addr: sock, Mux: mux, MetricsEndpoint: "admin", Ready: func() { close(ready) }}
	ctx, cancel := context.WithCancel(metrics.Put(context.Background(), r))
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- s.ListenAndServe(ctx) }()
	<-ready

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprint(conn, "GET /plug HTTP/1.1\r\nHost: localhost\r\nConnection: Upgrade\r\nUpgrade: plug\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.Contains(line, "101 Switching Protocols") {
		t.Fatalf("upgrade response = %q, %v", line, err)
	}
	var buf bytes.Buffer
	if err := r.Write(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{
		`http_server_requests_total{endpoint="admin",method="GET",route="GET /plug",status="101"} 1`,
		`http_server_requests_in_flight{endpoint="admin",method="GET",route="GET /plug"} 0`,
		`http_server_connections_open{endpoint="admin"} 0`,
		`http_server_connections_hijacked_total{endpoint="admin"} 1`,
		`http_server_connections_hijacked_open{endpoint="admin"} 1`,
	} {
		if !strings.Contains(buf.String(), part) {
			t.Errorf("active snapshot missing %q", part)
		}
	}
	close(release)
	<-done
	buf.Reset()
	if err := r.Write(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `http_server_connections_hijacked_open{endpoint="admin"} 0`) ||
		!strings.Contains(buf.String(), `http_server_hijacked_connection_duration_seconds_bucket{endpoint="admin",method="GET",route="GET /plug",le="+Inf"} 1`) {
		t.Fatalf("closed snapshot missing connection lifetime:\n%s", &buf)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}
