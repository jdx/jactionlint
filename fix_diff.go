package jactionlint

import (
	"fmt"
	"slices"
	"strings"
)

// unifiedDiff returns the changes from old to new as a unified diff with three lines of context, or
// an empty string when they are the same. The names are the paths in the headers of the diff.
func unifiedDiff(oldName, newName string, oldSrc, newSrc []byte) string {
	if string(oldSrc) == string(newSrc) {
		return ""
	}
	a := splitDiffLines(string(oldSrc))
	b := splitDiffLines(string(newSrc))

	var ops []diffOp
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	for i := 0; i < pre; i++ {
		ops = append(ops, diffOp{' ', a[i]})
	}
	am, bm := a[pre:len(a)-suf], b[pre:len(b)-suf]
	ops = append(ops, myersDiff(am, bm)...)
	for i := len(a) - suf; i < len(a); i++ {
		ops = append(ops, diffOp{' ', a[i]})
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", diffHeaderName(oldName), diffHeaderName(newName))
	const context = 3
	i := 0
	for i < len(ops) {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		// A hunk starts `context` lines before the first change and goes on while the changes are
		// closer than 2*context lines to each other
		start := max(0, i-context)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			j := end
			for j < len(ops) && ops[j].kind == ' ' {
				j++
			}
			if j == len(ops) || j-end > 2*context {
				end = min(end+context, len(ops))
				break
			}
			end = j
		}
		oldStart, newStart := 1, 1
		for _, o := range ops[:start] {
			if o.kind != '+' {
				oldStart++
			}
			if o.kind != '-' {
				newStart++
			}
		}
		oldCount, newCount := 0, 0
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				oldCount++
			}
			if o.kind != '-' {
				newCount++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", diffRange(oldStart, oldCount), diffRange(newStart, newCount))
		for _, o := range ops[start:end] {
			out.WriteByte(o.kind)
			text, noEOL := strings.CutSuffix(o.text, noEOLMark)
			out.WriteString(text)
			out.WriteByte('\n')
			if noEOL {
				out.WriteString("\\ No newline at end of file\n")
			}
		}
		i = end
	}
	return out.String()
}

// diffHeaderName returns the name for the "---" and "+++" lines. patch(1) reads the name up to the first
// white space unless a tab ends it, so a name with a space gets the tab that git writes after it too.
func diffHeaderName(name string) string {
	if strings.ContainsAny(name, " ") {
		return name + "\t"
	}
	return name
}

func diffRange(start, count int) string {
	switch count {
	case 0:
		return fmt.Sprintf("%d,0", start-1)
	case 1:
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

// noEOLMark is put after the last line of a text which has no line break, so that the line differs from
// the same line with a line break.
const noEOLMark = "\x00"

// splitDiffLines splits the text into lines without their "\n". A "\r" before the "\n" stays in the
// line so that the diff shows it.
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	lines[len(lines)-1] += noEOLMark
	return lines
}

type diffOp struct {
	kind byte // ' ', '-' or '+'
	text string
}

// maxDiffEdits is the number of changed lines after which myersDiff gives up on finding the shortest edit
// script and returns every line of a as removed and every line of b as added.
const maxDiffEdits = 3000

// myersDiff returns the shortest edit script from a to b (Myers, "An O(ND) Difference Algorithm and Its
// Variations"), which is what diff -u shows: the lines which stay are matched wherever they are, so a
// file with repeated blocks gets one small hunk for each change instead of one for the whole run of
// repeated lines. Time and space grow with the square of the number of changed lines, not with the product
// of the lengths of the files.
func myersDiff(a, b []string) []diffOp {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return replaceAllOps(a, b)
	}
	maxD := min(n+m, maxDiffEdits)
	offset := maxD + 1
	v := make([]int, 2*maxD+3)
	var trace [][]int
	found := -1
search:
	for d := 0; d <= maxD; d++ {
		// Only the diagonals -d..d are used in this round, so only they are saved
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				found = d
				trace = append(trace, slices.Clone(v[offset-d:offset+d+1]))
				break search
			}
		}
		trace = append(trace, slices.Clone(v[offset-d:offset+d+1]))
	}
	if found < 0 {
		return replaceAllOps(a, b)
	}
	// Walk back from the end to the start
	var rev []diffOp
	x, y := n, m
	for d := found; d > 0; d-- {
		prev := trace[d-1] // the diagonals -(d-1)..(d-1)
		at := func(k int) int { return prev[k+(d-1)] }
		k := x - y
		var pk int
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := at(pk)
		py := px - pk
		sx, sy := px+1, py // where the snake of this round starts, after the removal of a line
		if pk == k+1 {
			sx, sy = px, py+1 // after the addition of a line
		}
		for x > sx && y > sy {
			x--
			y--
			rev = append(rev, diffOp{' ', a[x]})
		}
		if pk == k+1 {
			y--
			rev = append(rev, diffOp{'+', b[y]})
		} else {
			x--
			rev = append(rev, diffOp{'-', a[x]})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, diffOp{' ', a[x]})
	}
	slices.Reverse(rev)
	return rev
}

func replaceAllOps(a, b []string) []diffOp {
	ops := make([]diffOp, 0, len(a)+len(b))
	for _, l := range a {
		ops = append(ops, diffOp{'-', l})
	}
	for _, l := range b {
		ops = append(ops, diffOp{'+', l})
	}
	return ops
}
