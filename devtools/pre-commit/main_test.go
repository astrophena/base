// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import "testing"

func TestClipCommand(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		termWidth int
		prefix    string
		command   string
		want      string
	}{
		"no clipping when command fits": {
			termWidth: 40,
			prefix:    "[1/3] Running check ",
			command:   "go test ./...",
			want:      "go test ./...",
		},
		"clips and appends ellipsis": {
			termWidth: 30,
			prefix:    "[1/3] Running check ",
			command:   "deno check services/deploy/internal/admin/frontend",
			want:      "deno ch...",
		},
		"tiny available space clips without ellipsis": {
			termWidth: 22,
			prefix:    "[1/3] Running check ",
			command:   "go test ./...",
			want:      "go",
		},
		"rune safe clipping": {
			termWidth: 26,
			prefix:    "[1/3] Running check ",
			command:   "go test пример",
			want:      "go ...",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := clipCommand(tc.termWidth, tc.prefix, tc.command)
			if got != tc.want {
				t.Fatalf("clipCommand() = %q, want %q", got, tc.want)
			}
		})
	}
}
