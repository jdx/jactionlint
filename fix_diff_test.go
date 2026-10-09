package jactionlint

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		want     string
	}{
		{"same", "a\nb\n", "a\nb\n", ""},
		{"replace", "a\nb\nc\n", "a\nB\nc\n", "--- x\n+++ y\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"},
		{"insert at the start", "a\n", "z\na\n", "--- x\n+++ y\n@@ -1 +1,2 @@\n+z\n a\n"},
		{"delete the last line", "a\nb\n", "a\n", "--- x\n+++ y\n@@ -1,2 +1 @@\n a\n-b\n"},
		{"no newline at the end appears", "a\nb\n", "a\nb", "--- x\n+++ y\n@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n"},
		{"from empty", "", "a\n", "--- x\n+++ y\n@@ -0,0 +1 @@\n+a\n"},
		{"crlf is shown", "a\r\nb\r\n", "a\r\nB\r\n", "--- x\n+++ y\n@@ -1,2 +1,2 @@\n a\r\n-b\r\n+B\r\n"},
		{"far apart changes make two hunks", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n", "1\nX\n3\n4\n5\n6\n7\n8\n9\n10\n11\nY\n",
			"--- x\n+++ y\n@@ -1,5 +1,5 @@\n 1\n-2\n+X\n 3\n 4\n 5\n@@ -9,4 +9,4 @@\n 9\n 10\n 11\n-12\n+Y\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unifiedDiff("x", "y", []byte(tc.old), []byte(tc.new)); got != tc.want {
				t.Errorf("want:\n%q\ngot:\n%q", tc.want, got)
			}
		})
	}
}

// applyUnifiedDiff applies a diff made by unifiedDiff, to check that it describes the change exactly.
func applyUnifiedDiff(t *testing.T, old, diff string) string {
	t.Helper()
	lines := splitDiffLines(old)
	var out []string
	pos := 0
	dl := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	for i := 2; i < len(dl); i++ {
		l := dl[i]
		switch {
		case strings.HasPrefix(l, "@@"):
			var os, oc, ns, nc int
			rest := strings.TrimPrefix(l, "@@ -")
			rest = strings.Split(rest, " @@")[0]
			parts := strings.Split(rest, " +")
			os, oc = parseRange(parts[0])
			ns, nc = parseRange(parts[1])
			_, _ = ns, nc
			start := os - 1
			if oc == 0 {
				start = os
			}
			out = append(out, lines[pos:start]...)
			pos = start
		case strings.HasPrefix(l, " "):
			out = append(out, lines[pos])
			pos++
		case strings.HasPrefix(l, "-"):
			pos++
		case strings.HasPrefix(l, "+"):
			out = append(out, l[1:])
		case strings.HasPrefix(l, "\\"):
			if prev := dl[i-1]; prev[0] != '-' {
				out[len(out)-1] += noEOLMark
			}
		default:
			t.Fatalf("bad diff line %q", l)
		}
	}
	out = append(out, lines[pos:]...)
	return joinDiffLines(out)
}

func parseRange(s string) (int, int) {
	a, b, ok := strings.Cut(s, ",")
	n, c := atoi(a), 1
	if ok {
		c = atoi(b)
	}
	return n, c
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func joinDiffLines(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		if t, ok := strings.CutSuffix(l, noEOLMark); ok {
			b.WriteString(t)
		} else {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func TestUnifiedDiffRoundTrip(t *testing.T) {
	texts := []string{"", "a\n", "a", "a\nb\nc\n", "a\nX\nc\nd\ne\nf\ng\nh\ni\nj\nk\n", "x\ny\nz", "a\r\nb\r\n", "\n\n", "a\n\nb\n"}
	for _, a := range texts {
		for _, b := range texts {
			d := unifiedDiff("x", "y", []byte(a), []byte(b))
			if a == b {
				if d != "" {
					t.Errorf("same text must give no diff: %q", d)
				}
				continue
			}
			if got := applyUnifiedDiff(t, a, d); got != b {
				t.Errorf("applying the diff of %q -> %q gave %q\n%s", a, b, got, d)
			}
		}
	}
}
