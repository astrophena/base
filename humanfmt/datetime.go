// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package humanfmt

import (
	"strconv"
	"strings"
	"time"
)

// DateTime formats t using strftime-like directives.
//
// Supported directives are %% (percent), %a, %A, %b, %B, %c, %C, %d, %D,
// %e, %F, %G, %h, %H, %I, %j, %m, %M, %n, %p, %P, %r, %R, %s, %S, %t,
// %T, %u, %V, %w, %x, %X, %y, %Y, %z, %:z, and %Z. Text and unsupported
// directives are copied unchanged.
func DateTime(t time.Time, format string) string {
	var result strings.Builder
	result.Grow(len(format) + 16)

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			result.WriteByte(format[i])
			continue
		}

		if i+1 >= len(format) {
			result.WriteByte('%')
			continue
		}

		directiveStart := i
		i++
		directive := format[i]

		if directive == ':' && i+1 < len(format) && format[i+1] == 'z' {
			result.WriteString(t.Format("-07:00"))
			i++
			continue
		}

		if writeDirective(&result, t, directive) {
			continue
		}

		result.WriteString(format[directiveStart : i+1])
	}

	return result.String()
}

func writeDirective(result *strings.Builder, t time.Time, directive byte) bool {
	switch directive {
	case '%':
		result.WriteByte('%')
	case 'a':
		result.WriteString(t.Format("Mon"))
	case 'A':
		result.WriteString(t.Format("Monday"))
	case 'b', 'h':
		result.WriteString(t.Format("Jan"))
	case 'B':
		result.WriteString(t.Format("January"))
	case 'c':
		result.WriteString(t.Format("Mon Jan 2 15:04:05 2006"))
	case 'C':
		writePadded(result, t.Year()/100, 2, '0')
	case 'd':
		writePadded(result, t.Day(), 2, '0')
	case 'D', 'x':
		result.WriteString(t.Format("01/02/06"))
	case 'e':
		writePadded(result, t.Day(), 2, ' ')
	case 'F':
		result.WriteString(t.Format("2006-01-02"))
	case 'G':
		year, _ := t.ISOWeek()
		writePadded(result, year, 4, '0')
	case 'H':
		writePadded(result, t.Hour(), 2, '0')
	case 'I':
		writePadded(result, hour12(t.Hour()), 2, '0')
	case 'j':
		writePadded(result, t.YearDay(), 3, '0')
	case 'm':
		writePadded(result, int(t.Month()), 2, '0')
	case 'M':
		writePadded(result, t.Minute(), 2, '0')
	case 'n':
		result.WriteByte('\n')
	case 'p':
		result.WriteString(t.Format("PM"))
	case 'P':
		result.WriteString(strings.ToLower(t.Format("PM")))
	case 'r':
		result.WriteString(t.Format("03:04:05 PM"))
	case 'R':
		result.WriteString(t.Format("15:04"))
	case 's':
		result.WriteString(strconv.FormatInt(t.Unix(), 10))
	case 'S':
		writePadded(result, t.Second(), 2, '0')
	case 't':
		result.WriteByte('\t')
	case 'T', 'X':
		result.WriteString(t.Format("15:04:05"))
	case 'u':
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		result.WriteString(strconv.Itoa(weekday))
	case 'V':
		_, week := t.ISOWeek()
		writePadded(result, week, 2, '0')
	case 'w':
		result.WriteString(strconv.Itoa(int(t.Weekday())))
	case 'y':
		writePadded(result, t.Year()%100, 2, '0')
	case 'Y':
		writePadded(result, t.Year(), 4, '0')
	case 'z':
		result.WriteString(t.Format("-0700"))
	case 'Z':
		result.WriteString(t.Format("MST"))
	default:
		return false
	}

	return true
}

func hour12(hour int) int {
	hour %= 12
	if hour == 0 {
		return 12
	}
	return hour
}

func writePadded(result *strings.Builder, value, width int, padding byte) {
	formatted := strconv.Itoa(value)
	for range width - len(formatted) {
		result.WriteByte(padding)
	}
	result.WriteString(formatted)
}
