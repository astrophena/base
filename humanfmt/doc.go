// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Package humanfmt formats times and byte amounts for display to people.

# Relative times

[RelativeTime] uses the largest whole unit and supports past and future values.
Times less than five seconds apart are returned as "just now".
Months and years use fixed approximations of 30 and 365 days.

# Date and time formatting

[DateTime] accepts text mixed with strftime-like directives.

	+-----------------------+--------------------------------------------------+
	| Category              | Directives                                       |
	+-----------------------+--------------------------------------------------+
	| Calendar              | %a %A %b %B %C %d %e %F %G %h %j %m %u %V %w     |
	|                       | %y %Y                                            |
	| Clock                 | %H %I %M %p %P %r %R %S %T                       |
	| Zone and epoch        | %s %z %:z %Z                                     |
	| Composite and control | %% %c %D %n %t %x %X                             |
	+-----------------------+--------------------------------------------------+

Names are formatted in English. The time's location controls its UTC offset
and zone name. Unsupported directives are preserved in the result.

# Byte amounts

[Bytes] uses binary units: B, KiB, MiB, GiB, TiB, PiB, and EiB.
Values above bytes are rounded to one decimal place.
*/
package humanfmt
