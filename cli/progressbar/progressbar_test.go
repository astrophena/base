// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package progressbar

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNonInteractiveLifecycleDoesNotRender(t *testing.T) {
	var buf bytes.Buffer
	pb := New(&buf, 2, false)

	pb.Start()
	pb.SetTitle("installing")
	pb.Increment()
	pb.Stop(false)

	if got := buf.String(); got != "" {
		t.Fatalf("buf.String() = %q, want no progress output", got)
	}
}

func TestPrintf(t *testing.T) {
	cases := map[string]struct {
		interactive bool
		want        []string
	}{
		"interactive": {
			interactive: true,
			want:        []string{"\r\033[K", "processed 3 files\n", "\r|"},
		},
		"non-interactive": {
			interactive: false,
			want:        []string{"processed 3 files\n"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			pb := New(&buf, 1, tc.interactive)
			pb.startTime = time.Now()

			pb.Printf("processed %d files", 3)

			got := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("buf.String() = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestStopFinalRender(t *testing.T) {
	cases := map[string]struct {
		current  int
		failed   bool
		contains []string
	}{
		"complete": {
			current:  4,
			contains: []string{"|██████████|", "4/4 [100%]", "\n"},
		},
		"aborted": {
			current:  1,
			contains: []string{"⚠︎", "(!) 1/4 [25%]", "\n"},
		},
		"failed": {
			current:  2,
			failed:   true,
			contains: []string{"✗︎", "(!) 2/4 [50%]", "\n"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			pb := New(&buf, 4, true)
			pb.barWidth = 10

			pb.Start()
			for range tc.current {
				pb.Increment()
			}
			pb.Stop(tc.failed)

			got := lastRender(buf.String())
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("last render = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestSetTitle(t *testing.T) {
	var buf bytes.Buffer
	pb := New(&buf, 1, true)
	pb.barWidth = 10
	pb.startTime = time.Now()

	pb.SetTitle("installing")

	got := buf.String()
	if !strings.Contains(got, "\rinstalling |") {
		t.Fatalf("buf.String() = %q, want title before bar", got)
	}
}

func lastRender(output string) string {
	idx := strings.LastIndex(output, "\r")
	if idx == -1 {
		return output
	}
	return output[idx+1:]
}

func ExampleNew() {
	var out bytes.Buffer
	bar := New(&out, 2, false)

	bar.Start()
	bar.SetTitle("installing")
	bar.Increment()
	bar.Printf("processed %d packages", 1)
	bar.Increment()
	bar.Stop(false)
}

func ExampleBar_Printf() {
	bar := New(os.Stdout, 1, false)

	bar.Printf("processed %s", "tools")

	// Output:
	// processed tools
}
