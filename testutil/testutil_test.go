// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValueDiff(t *testing.T) {
	cases := map[string]struct {
		wantValue any
		gotValue  any
		wantDiff  string
	}{
		"scalar": {
			wantValue: 1,
			gotValue:  2,
			wantDiff:  "--- want\n+++ got\n-1\n+2\n",
		},
		"different types": {
			wantValue: int(1),
			gotValue:  int64(1),
			wantDiff:  "--- want\n+++ got\n-int: 1\n+int64: 1\n",
		},
		"multiline string": {
			wantValue: "first\nold\nlast\n",
			gotValue:  "first\nnew\nlast\n",
			wantDiff:  "--- want\n+++ got\n first\n-old\n+new\n last\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := valueDiff(tc.wantValue, tc.gotValue); got != tc.wantDiff {
				t.Fatalf("valueDiff() mismatch:\n%s", lineDiff([]byte(tc.wantDiff), []byte(got)))
			}
		})
	}
}

func TestLineDiffElidesUnchangedLines(t *testing.T) {
	want := "old\n" + strings.Repeat("same\n", 10) + "tail\n"
	got := "new\n" + strings.Repeat("same\n", 10) + "tail\n"
	diff := lineDiff([]byte(want), []byte(got))
	if !strings.Contains(diff, "... 8 unchanged lines ...") {
		t.Fatalf("lineDiff() did not elide unchanged lines:\n%s", diff)
	}
}

func TestLineDiffMarksMissingNewline(t *testing.T) {
	diff := lineDiff([]byte("old"), []byte("new"))
	if count := strings.Count(diff, "\\ No newline at end of value"); count != 2 {
		t.Fatalf("missing-newline annotations = %d, want 2:\n%s", count, diff)
	}
}

func TestGlobFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "case.txt")
	if err := os.WriteFile(file, []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	matches, err := globFiles(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != file {
		t.Fatalf("matches = %q, want [%q]", matches, file)
	}

	if _, err := globFiles(filepath.Join(dir, "*.missing")); err == nil || !strings.Contains(err.Error(), "matched no files") {
		t.Fatalf("missing glob error = %v, want matched-no-files error", err)
	}
}

func TestRunGoldenUpdateAndCompare(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "case.in")
	if err := os.WriteFile(input, []byte("input\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	produce := func(t *testing.T, match string) []byte {
		t.Helper()
		if match != input {
			t.Fatalf("match = %q, want %q", match, input)
		}
		return []byte("output\n")
	}
	RunGolden(t, filepath.Join(dir, "*.in"), produce, true)
	RunGolden(t, filepath.Join(dir, "*.in"), produce, false)
}
