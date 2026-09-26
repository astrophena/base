// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package tgbot

import (
	"unicode/utf16"
	"unicode/utf8"

	"go.astrophena.name/base/tgmarkup"
)

// split splits rendered text and adjusts entity offsets for each part.
// The first part may have a smaller limit when used as a media caption.
func split(msg tgmarkup.Message, firstLimit int) []tgmarkup.Message {
	if msg.Text == "" {
		return nil
	}

	type boundary struct {
		byte  int
		units int
	}

	bounds := []boundary{{}}
	units := 0
	for offset, r := range msg.Text {
		units += utf16.RuneLen(r)
		bounds = append(bounds, boundary{
			byte:  offset + utf8.RuneLen(r),
			units: units,
		})
	}

	var parts []tgmarkup.Message
	limit := firstLimit
	for start := 0; start < len(bounds)-1; {
		end := start + 1
		newline := 0
		for end < len(bounds) && bounds[end].units-bounds[start].units <= limit {
			if msg.Text[bounds[end-1].byte:bounds[end].byte] == "\n" {
				newline = end
			}
			end++
		}
		end--
		if newline > start && end < len(bounds)-1 {
			end = newline
		}

		lo, hi := bounds[start].units, bounds[end].units
		part := tgmarkup.Message{
			Text: msg.Text[bounds[start].byte:bounds[end].byte],
		}

		for _, entity := range msg.Entities {
			from := max(entity.Offset, lo)
			to := min(entity.Offset+entity.Length, hi)
			if from >= to {
				continue
			}
			entity.Offset = from - lo
			entity.Length = to - from
			part.Entities = append(part.Entities, entity)
		}

		parts = append(parts, part)
		start = end
		limit = messageLimit
	}
	return parts
}
