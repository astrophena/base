// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file located at
// https://github.com/tailscale/tailscale/blob/main/LICENSE.

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func getDebug(t *testing.T, mux *http.ServeMux) string {
	return send(t, mux, http.MethodGet, "/debug/", http.StatusOK)
}
