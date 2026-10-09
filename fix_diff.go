package jactionlint

import (
	"fmt"
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

	type op struct {
		kind byte // ' ', '-' or '+'
		text string
	}
	var ops []op
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	for i := 0; i < pre; i++ {
		ops = append(ops, op{' ', a[i]})
	}
	am, bm := a[pre:len(a)-suf], b[pre:len(b)-suf]
	n, m := len(am), len(bm)
	if n*m > 4_000_000 {
		for _, l := range am {
			ops = append(ops, op{'-', l})
		}
		for _, l := range bm {
			ops = append(ops, op{'+', l})
		}
	} else {
		l := make([][]int32, n+1)
		for i := range l {
			l[i] = make([]int32, m+1)
		}
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if am[i] == bm[j] {
					l[i][j] = l[i+1][j+1] + 1
				} else {
					l[i][j] = max(l[i+1][j], l[i][j+1])
				}
			}
		}
		i, j := 0, 0
		for i < n || j < m {
			switch {
			case i < n && j < m && am[i] == bm[j]:
				ops = append(ops, op{' ', am[i]})
				i++
				j++
			case j >= m || (i < n && l[i+1][j] >= l[i][j+1]):
				ops = append(ops, op{'-', am[i]})
				i++
			default:
				ops = append(ops, op{'+', bm[j]})
				j++
			}
		}
	}
	for i := len(a) - suf; i < len(a); i++ {
		ops = append(ops, op{' ', a[i]})
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", oldName, newName)
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
