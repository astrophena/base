// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt_test

import (
	"fmt"
	"time"

	"go.astrophena.name/base/humanfmt"
)

func ExampleRelativeTime() {
	var (
		t   = time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
		now = time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC)
	)
	rel := humanfmt.RelativeTime(t, now)
	fmt.Println(rel)
	// Output: 1 day ago
}

func ExampleDateTime() {
	t := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
	dt := humanfmt.DateTime(t, "%Y-%m-%d %H:%M")
	fmt.Println(dt)
	// Output: 2026-07-10 00:00
}

func ExampleBytes() {
	fmt.Println(humanfmt.Bytes(1536))
	// Output: 1.5 KiB
}
