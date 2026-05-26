// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file located at
// https://github.com/tailscale/tailscale/blob/main/LICENSE.

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"

	"go.astrophena.name/base/testutil"
	"go.astrophena.name/base/version"
)

func TestDebugger(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	dbg1 := Debugger(mux)
	if dbg1 == nil {
		t.Fatal("didn't get a debugger from mux")
	}

	dbg2 := Debugger(mux)
	if dbg2 != dbg1 {
		t.Fatal("Debugger returned different debuggers for the same mux")
	}
}

func TestDebuggerKV(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	dbg := Debugger(mux)
	dbg.KV("Donuts", 42)
	dbg.KV("Secret code", "hunter2")
	val := "red"
	dbg.KVFunc("Condition", func() any { return val })

	body := getDebug(t, mux)
	for _, want := range []string{"Donuts", "42", "Secret code", "hunter2", "Condition", "red"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %q in output, not found", want)
		}
	}

	val = "green"
	body = getDebug(t, mux)
	for _, want := range []string{"Condition", "green"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %q in output, not found", want)
		}
	}
}

func TestDebuggerLink(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	dbg := Debugger(mux)
	dbg.Link("https://www.tailscale.com", "Homepage")

	body := getDebug(t, mux)
	for _, want := range []string{"https://www.tailscale.com", "Homepage"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %q in output, not found", want)
		}
	}
}

func TestDebuggerHandle(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	dbg := Debugger(mux)
	dbg.Handle("check", "Consistency check", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Test output")
	}))

	body := getDebug(t, mux)
	for _, want := range []string{"/debug/check", "Consistency check"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %q in output, not found", want)
		}
	}

	body = send(t, mux, http.MethodGet, "/debug/check", http.StatusOK)
	want := "Test output"
	if !strings.Contains(body, want) {
		t.Errorf("want %q in output, not found", want)
	}
}

func TestDebuggerGC(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	Debugger(mux)

	body := send(t, mux, http.MethodGet, "/debug/gc", http.StatusOK)
	testutil.AssertEqual(t, "Running GC...\nDone.\n", body)
}

func TestDebuggerDiscovery(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	dbg := Debugger(mux)

	// Add a custom link to ensure it appears in the discovery output.
	dbg.Link("/custom/path", "My Custom Link")

	body := send(t, mux, http.MethodGet, "/debug/discovery", http.StatusOK)

	var discovery DebugHandlerDiscovery
	if err := json.Unmarshal([]byte(body), &discovery); err != nil {
		t.Fatalf("failed to unmarshal discovery response: %v\nbody: %s", err, body)
	}

	// Check process and version info.
	hostname, _ := os.Hostname()
	testutil.AssertEqual(t, discovery.Hostname, hostname)
	testutil.AssertEqual(t, discovery.PID, os.Getpid())
	testutil.AssertEqual(t, discovery.Version.Name, version.CmdName())

	// Sanity check runtime stats.
	if discovery.Runtime.NumGoroutines < 1 {
		t.Errorf("expected at least 1 goroutine, got %d", discovery.Runtime.NumGoroutines)
	}
	if discovery.Runtime.Uptime == "" {
		t.Error("expected Uptime to not be empty")
	}

	// Check that links are present.
	wantLinks := []link{
		{URL: "/debug/pprof/", Desc: "pprof"},
		{URL: "/debug/gc", Desc: "Force GC"},
		{URL: "/custom/path", Desc: "My Custom Link"},
	}

	for _, want := range wantLinks {
		found := false
		for _, got := range discovery.Links {
			if got.URL == want.URL && got.Desc == want.Desc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find link %+v in discovery response, but it was missing", want)
		}
	}
}

func TestXFFDebugHandler(t *testing.T) {
	t.Parallel()

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/debug/xff", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", " 203.0.113.9 , invalid")
	req.Header.Set("X-Real-IP", "198.51.100.10")
	req.Header.Set("Forwarded", "for=203.0.113.9")

	w := httptest.NewRecorder()
	s.xffDebugHandler().ServeHTTP(w, req)

	var got xffDebugResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal xff debug response: %v\nbody: %s", err, w.Body.String())
	}

	testutil.AssertEqual(t, true, got.UsingDefaultTrustedProxies)
	testutil.AssertEqual(t, true, got.TrustedForwardedSource)
	testutil.AssertEqual(t, []string{"127.0.0.0/8"}, got.TrustedProxies)
	testutil.AssertEqual(t, "203.0.113.9", got.RealIPResult)
	testutil.AssertEqual(t, "198.51.100.10", got.XRealIP)
	testutil.AssertEqual(t, "for=203.0.113.9", got.Forwarded)
	if len(got.XForwardedForParts) != 2 {
		t.Fatalf("got %d XFF parts, want 2: %+v", len(got.XForwardedForParts), got.XForwardedForParts)
	}
	testutil.AssertEqual(t, true, got.XForwardedForParts[0].Valid)
	testutil.AssertEqual(t, false, got.XForwardedForParts[1].Valid)

	s.TrustedProxies = []netip.Prefix{}
	w = httptest.NewRecorder()
	s.xffDebugHandler().ServeHTTP(w, req)
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal xff debug response: %v\nbody: %s", err, w.Body.String())
	}
	testutil.AssertEqual(t, false, got.UsingDefaultTrustedProxies)
	testutil.AssertEqual(t, false, got.TrustedForwardedSource)
	testutil.AssertEqual(t, "127.0.0.1", got.RealIPResult)
}

func getDebug(t *testing.T, mux *http.ServeMux) string {
	return send(t, mux, http.MethodGet, "/debug/", http.StatusOK)
}

func TestLinkItemToHTMLEscapesTarget(t *testing.T) {
	t.Parallel()

	item := LinkItem{Name: "name", Target: `" onclick="alert(1)`}
	h := item.ToHTML()
	body := string(h)
	if !strings.Contains(body, "&#34; onclick=&#34;alert(1)") {
		t.Fatalf("expected escaped target, got: %q", body)
	}
}
