// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt

import (
	"fmt"
	"time"
)

const justNowThreshold = 5 * time.Second

type relativeUnit struct {
	duration time.Duration
	name     string
}

var relativeUnits = [...]relativeUnit{
	{duration: 365 * 24 * time.Hour, name: "year"},
	{duration: 30 * 24 * time.Hour, name: "month"},
	{duration: 24 * time.Hour, name: "day"},
	{duration: time.Hour, name: "hour"},
	{duration: time.Minute, name: "minute"},
	{duration: time.Second, name: "second"},
}

// RelativeTime describes t in relation to now using the largest whole unit.
// Times less than five seconds apart are reported as "just now". Months and
// years are approximated as 30 and 365 days.
func RelativeTime(t, now time.Time) string {
	delta := t.Sub(now)
	future := delta > 0
	if delta < 0 {
		delta = -delta
	}

	if delta < justNowThreshold {
		return "just now"
	}

	for _, unit := range relativeUnits {
		if delta < unit.duration {
			continue
		}

		count := int64(delta / unit.duration)
		name := unit.name
		if count != 1 {
			name += "s"
		}

		if future {
			return fmt.Sprintf("in %d %s", count, name)
		}
		return fmt.Sprintf("%d %s ago", count, name)
	}

	return "just now"
}
