package jactionlint

import "testing"

func TestLineBreakIsCRLF(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		off  int
		want bool
	}{
		{"crlf line", "a: |\r\n  x\r\n  y\r\n", 9, true},
		{"lf line", "a: |\n  x\n  y\n", 8, false},
		{"offset on the break", "a: |\r\n  x\r\n", 10, true},
		{"offset on the LF of an unterminated-CR file", "a: |\n  x\n", 8, false},
		{"last line without a break", "a: |\r\n  x\r\n  y", 14, true},
		{"last line without a break, lf", "a: |\n  x\n  y", 11, false},
		{"single unterminated line", "a: |", 3, false},
	} {
		if got := lineBreakIsCRLF([]byte(tc.src), tc.off); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

// A block scalar inserted into on its last line, which has no break, keeps the CRLF of the document.
func TestYAMLSiteCRLFOnTheLastLineWithoutBreak(t *testing.T) {
	src := []byte("run: |\r\n  echo a\r\n  echo b")
	site, ok := YAMLSiteAt(src, len(src)-1)
	if !ok || site.Context != YAMLBlockScalar || !site.CRLF {
		t.Errorf("site %+v, ok %v", site, ok)
	}
}
