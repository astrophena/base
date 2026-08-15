// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package testutil provides helpers for common testing scenarios.
package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.astrophena.name/base/txtar"
)

// AssertEqual fails the test if got is not deeply equal to want. Failures are
// reported as a line-oriented diff so that large strings and values remain
// readable.
func AssertEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values differ (-want +got):\n%s", valueDiff(want, got))
	}
}

func valueDiff(want, got any) string {
	wantText, gotText := fmt.Sprintf("%#v\n", want), fmt.Sprintf("%#v\n", got)
	if reflect.TypeOf(want) != reflect.TypeOf(got) {
		wantText = fmt.Sprintf("%T: %#v\n", want, want)
		gotText = fmt.Sprintf("%T: %#v\n", got, got)
	}

	wantString, wantIsString := want.(string)
	gotString, gotIsString := got.(string)
	if wantIsString && gotIsString && (strings.Contains(wantString, "\n") || strings.Contains(gotString, "\n")) {
		wantText, gotText = wantString, gotString
	}

	wantBytes, wantIsBytes := want.([]byte)
	gotBytes, gotIsBytes := got.([]byte)
	if wantIsBytes && gotIsBytes && (bytes.Contains(wantBytes, []byte("\n")) || bytes.Contains(gotBytes, []byte("\n"))) {
		wantText, gotText = string(wantBytes), string(gotBytes)
	}

	return lineDiff([]byte(wantText), []byte(gotText))
}

func globFiles(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("filepath.Glob(%q): %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("filepath.Glob(%q) matched no files", pattern)
	}
	return matches, nil
}

// Run runs a subtest for each file that matches the provided glob pattern.
// The subtest name is the file's path relative to its directory.
func Run(t *testing.T, glob string, f func(t *testing.T, match string)) {
	t.Helper()
	matches, err := globFiles(glob)
	if err != nil {
		t.Fatal(err)
	}

	for _, match := range matches {
		name := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		t.Run(name, func(t *testing.T) {
			f(t, match)
		})
	}
}

// RunGolden runs a test for each file matching a glob pattern and compares
// the result of a function f with the contents of a corresponding ".golden"
// file.
//
// If update is true, the golden file is updated with the new result instead
// of being compared.
func RunGolden(t *testing.T, glob string, f func(t *testing.T, match string) []byte, update bool) {
	t.Helper()
	Run(t, glob, func(t *testing.T, match string) {
		got := f(t, match)
		goldenFile := strings.TrimSuffix(match, filepath.Ext(match)) + ".golden"

		if update {
			if err := os.WriteFile(goldenFile, got, 0o644); err != nil {
				t.Fatalf("failed to write golden file %q: %v", goldenFile, err)
			}
			return
		}

		want, err := os.ReadFile(goldenFile)
		if err != nil {
			t.Fatalf("failed to read golden file %q: %v", goldenFile, err)
		}

		if !bytes.Equal(got, want) {
			t.Fatalf("golden file %q differs (-want +got):\n%s", goldenFile, lineDiff(want, got))
		}
	})
}

// MockHTTPClient returns an [http.Client] that directs all requests to the
// provided [http.Handler].
func MockHTTPClient(h http.Handler) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w.Result(), nil
		}),
	}
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// UnmarshalJSON parses a JSON byte slice into a value of type V, failing
// the test if an error occurs.
func UnmarshalJSON[V any](t *testing.T, b []byte) V {
	t.Helper()
	var v V
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	return v
}

// BuildTxtar creates a txtar-formatted byte slice from the contents of a directory.
func BuildTxtar(t *testing.T, dir string) []byte {
	t.Helper()
	ar, err := txtar.FromDir(dir)
	if err != nil {
		t.Fatalf("failed to build txtar from dir %q: %v", dir, err)
	}
	return txtar.Format(ar)
}

// ExtractTxtar extracts a txtar archive to a specified directory.
func ExtractTxtar(t *testing.T, ar *txtar.Archive, dir string) {
	t.Helper()
	if err := txtar.Extract(ar, dir); err != nil {
		t.Fatalf("failed to extract txtar to dir %q: %v", dir, err)
	}
}
