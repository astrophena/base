// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt

import (
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
)

func TestDateTime(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("NPT", 5*60*60+45*60)
	date := time.Date(2024, time.January, 2, 15, 4, 5, 0, zone)

	cases := map[string]struct {
		format string
		want   string
	}{
		"calendar date":         {format: "%Y-%m-%d", want: "2024-01-02"},
		"clock time":            {format: "%H:%M:%S", want: "15:04:05"},
		"names":                 {format: "%A, %B %e", want: "Tuesday, January  2"},
		"twelve hour clock":     {format: "%I:%M %p", want: "03:04 PM"},
		"common aliases":        {format: "%F %T %R", want: "2024-01-02 15:04:05 15:04"},
		"timezone":              {format: "%z %:z %Z", want: "+0545 +05:45 NPT"},
		"week values":           {format: "%G-W%V-%u", want: "2024-W01-2"},
		"ordinal and weekday":   {format: "%j %w", want: "002 2"},
		"escaped percent":       {format: "%%Y=%Y", want: "%Y=2024"},
		"unsupported directive": {format: "%Q", want: "%Q"},
		"trailing percent":      {format: "done%", want: "done%"},
		"control characters":    {format: "a%tb%nc", want: "a\tb\nc"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testutil.AssertEqual(t, DateTime(date, tc.format), tc.want)
		})
	}
}

func TestDateTimeMidnightAndNoon(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		hour int
		want string
	}{
		"midnight": {hour: 0, want: "12 AM am"},
		"noon":     {hour: 12, want: "12 PM pm"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			date := time.Date(2026, time.July, 11, tc.hour, 0, 0, 0, time.UTC)
			testutil.AssertEqual(t, DateTime(date, "%I %p %P"), tc.want)
		})
	}
}
