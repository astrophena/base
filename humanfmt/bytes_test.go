// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt

import (
	"math"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestBytes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		n    uint64
		want string
	}{
		"zero":                 {n: 0, want: "0 B"},
		"bytes":                {n: 999, want: "999 B"},
		"one kibibyte":         {n: 1 << 10, want: "1 KiB"},
		"fractional kibibytes": {n: 1536, want: "1.5 KiB"},
		"mebibytes":            {n: 10 << 20, want: "10 MiB"},
		"gibibytes":            {n: 3 << 30, want: "3 GiB"},
		"largest value":        {n: math.MaxUint64, want: "16 EiB"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testutil.AssertEqual(t, Bytes(tc.n), tc.want)
		})
	}
}
