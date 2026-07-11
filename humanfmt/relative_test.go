// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt

import (
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
)

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)

	cases := map[string]struct {
		t    time.Time
		want string
	}{
		"same time":      {t: now, want: "just now"},
		"recent past":    {t: now.Add(-4 * time.Second), want: "just now"},
		"recent future":  {t: now.Add(4 * time.Second), want: "just now"},
		"seconds ago":    {t: now.Add(-42 * time.Second), want: "42 seconds ago"},
		"one minute ago": {t: now.Add(-time.Minute), want: "1 minute ago"},
		"future minutes": {t: now.Add(17 * time.Minute), want: "in 17 minutes"},
		"whole hours": {
			t:    now.Add(-(3*time.Hour + 59*time.Minute)),
			want: "3 hours ago",
		},
		"one day": {t: now.Add(24 * time.Hour), want: "in 1 day"},
		"months":  {t: now.Add(-90 * 24 * time.Hour), want: "3 months ago"},
		"years":   {t: now.Add(730 * 24 * time.Hour), want: "in 2 years"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testutil.AssertEqual(t, RelativeTime(tc.t, now), tc.want)
		})
	}
}
