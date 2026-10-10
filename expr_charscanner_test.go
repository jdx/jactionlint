package jactionlint

import (
	"math/rand"
	"strings"
	"testing"
	"text/scanner"
)

// TestCharScannerMatchesTextScanner drives charScanner and text/scanner.Scanner with the same random
// sequence of Peek, Next and Pos calls over random input, including invalid UTF-8, NUL, byte order
// marks and line breaks, and compares every result and every scan error with its position.
func TestCharScannerMatchesTextScanner(t *testing.T) {
	pieces := []string{"a", "Z", "0", " ", "\n", "\r\n", "'", "}", "{", "$", ".", "é", "日本", "😀", "\x00", "\xff", "\xc3", "\xe6\x97", "\ufeff", "\t"}
	rng := rand.New(rand.NewSource(1))
	for iter := 0; iter < 20000; iter++ {
		var b strings.Builder
		for n := rng.Intn(12); n > 0; n-- {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		src := b.String()

		var wantErrs, gotErrs []string
		var ref scanner.Scanner
		ref.Init(strings.NewReader(src))
		ref.Error = func(s *scanner.Scanner, m string) { wantErrs = append(wantErrs, m+" "+s.Pos().String()) }
		var got charScanner
		got.src, got.ch = src, -2
		got.onError = func(m string) { gotErrs = append(gotErrs, m+" "+got.Pos().String()) }

		for step := 0; step < 40; step++ {
			switch rng.Intn(3) {
			case 0:
				if w, g := ref.Peek(), got.Peek(); w != g {
					t.Fatalf("%q step %d: Peek %q != %q", src, step, w, g)
				}
			case 1:
				if w, g := ref.Next(), got.Next(); w != g {
					t.Fatalf("%q step %d: Next %q != %q", src, step, w, g)
				}
			case 2:
				if w, g := ref.Pos(), got.Pos(); w != g {
					t.Fatalf("%q step %d: Pos %v != %v", src, step, w, g)
				}
			}
			if strings.Join(wantErrs, "|") != strings.Join(gotErrs, "|") {
				t.Fatalf("%q step %d: errors %q != %q", src, step, wantErrs, gotErrs)
			}
		}
	}
}
