// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package testutil

import (
	"fmt"
	"slices"
	"strings"
)

type diffOp struct {
	prefix byte
	line   string
}

// lineDiff returns a compact line-oriented diff. It uses a longest common
// subsequence for ordinary test output and falls back to a bounded full diff
// for inputs large enough to make the quadratic algorithm unreasonable.
func lineDiff(want, got []byte) string {
	wantLines := splitDiffLines(want)
	gotLines := splitDiffLines(got)
	ops := diffOperations(wantLines, gotLines)

	var out strings.Builder
	out.WriteString("--- want\n+++ got\n")
	writeDiffOperations(&out, ops)
	return out.String()
}

func splitDiffLines(text []byte) []string {
	if len(text) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(text), "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	lines[len(lines)-1] += "\n\\ No newline at end of value\n"
	return lines
}

func diffOperations(want, got []string) []diffOp {
	const maxCells = 1_000_000
	if len(want) > 0 && len(got) > maxCells/len(want) {
		return fullDiffOperations(want, got)
	}

	width := len(got) + 1
	lcs := make([]int, (len(want)+1)*width)
	for i := range slices.Backward(want) {
		for j := range slices.Backward(got) {
			index := i*width + j
			if want[i] == got[j] {
				lcs[index] = lcs[(i+1)*width+j+1] + 1
			} else {
				lcs[index] = max(lcs[(i+1)*width+j], lcs[i*width+j+1])
			}
		}
	}

	ops := make([]diffOp, 0, len(want)+len(got))
	for i, j := 0, 0; i < len(want) || j < len(got); {
		switch {
		case i < len(want) && j < len(got) && want[i] == got[j]:
			ops = append(ops, diffOp{prefix: ' ', line: want[i]})
			i++
			j++
		case j < len(got) && (i == len(want) || lcs[i*width+j+1] > lcs[(i+1)*width+j]):
			ops = append(ops, diffOp{prefix: '+', line: got[j]})
			j++
		default:
			ops = append(ops, diffOp{prefix: '-', line: want[i]})
			i++
		}
	}
	return ops
}

func fullDiffOperations(want, got []string) []diffOp {
	ops := make([]diffOp, 0, len(want)+len(got))
	for _, line := range want {
		ops = append(ops, diffOp{prefix: '-', line: line})
	}
	for _, line := range got {
		ops = append(ops, diffOp{prefix: '+', line: line})
	}
	return ops
}

func writeDiffOperations(out *strings.Builder, ops []diffOp) {
	const contextLines = 3
	for start := 0; start < len(ops); {
		if ops[start].prefix != ' ' {
			out.WriteByte(ops[start].prefix)
			out.WriteString(ops[start].line)
			start++
			continue
		}

		end := start
		for end < len(ops) && ops[end].prefix == ' ' {
			end++
		}
		length := end - start
		leadingChange := start > 0
		trailingChange := end < len(ops)
		keepStart, keepEnd := 0, 0
		if !leadingChange && !trailingChange {
			keepStart = length
		} else {
			if leadingChange {
				keepStart = min(contextLines, length)
			}
			if trailingChange {
				keepEnd = min(contextLines, length-keepStart)
			}
		}

		for _, op := range ops[start : start+keepStart] {
			out.WriteByte(op.prefix)
			out.WriteString(op.line)
		}
		omitted := length - keepStart - keepEnd
		if omitted > 0 {
			fmt.Fprintf(out, "... %d unchanged lines ...\n", omitted)
		}
		for _, op := range ops[end-keepEnd : end] {
			out.WriteByte(op.prefix)
			out.WriteString(op.line)
		}
		start = end
	}
}
