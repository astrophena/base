// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt

import (
	"strconv"
	"strings"
)

const byteUnit = 1024

var byteUnits = [...]string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

// Bytes formats n using binary units. Values larger than bytes are rounded to
// one decimal place, with a trailing ".0" removed.
func Bytes(n uint64) string {
	if n < byteUnit {
		return strconv.FormatUint(n, 10) + " B"
	}

	value := float64(n)
	unit := 0
	for value >= byteUnit && unit < len(byteUnits)-1 {
		value /= byteUnit
		unit++
	}

	formatted := strconv.FormatFloat(value, 'f', 1, 64)
	formatted = strings.TrimSuffix(formatted, ".0")
	return formatted + " " + byteUnits[unit]
}
