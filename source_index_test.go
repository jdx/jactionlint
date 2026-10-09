package jactionlint

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestSourceIndex(t *testing.T) {
	src := []byte("on: push\njobs:\n  j:\n    uses: \"./.github/workflows/é.yml\"\n    x: 日本語 y\n")
	x := newSourceIndex(src)

	// Columns count code points, offsets bytes
	tests := []struct {
		line, col int
		want      int
		ok        bool
	}{
		{1, 1, 0, true},
		{4, 11, strings.Index(string(src), `"./.github`), true},
		{5, 11, strings.Index(string(src), "日本語") + len("日本語"), true}, // after three characters of three bytes each
		{9, 1, 0, false},
		{1, 99, 0, false},
		{0, 1, 0, false},
	}
	for _, tc := range tests {
		got, ok := x.offset(tc.line, tc.col)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("offset(%d, %d) = %d, %v but want %d, %v", tc.line, tc.col, got, ok, tc.want, tc.ok)
		}
	}

	edit, ok := x.selfRepositoryEdit(&Pos{Line: 4, Col: 11})
	if !ok || string(src[edit.Start:edit.End]) != "." || edit.NewText != "$" {
		t.Errorf("unexpected edit %+v, %v", edit, ok)
	}
	if _, ok := x.selfRepositoryEdit(&Pos{Line: 1, Col: 1}); ok {
		t.Error("a value that does not start with ./ must not be edited")
	}

	if got := x.newline(); got != "\n" {
		t.Errorf("newline is %q", got)
	}
	if got := newSourceIndex([]byte("a\r\nb\r\n")).newline(); got != "\r\n" {
		t.Errorf("newline is %q", got)
	}
	// A lone CR or a NEL is a line break for the YAML parser but not for the index
	for _, s := range []string{"a\rb\n", "a\u0085b\n", "a\u2028b\n"} {
		if newSourceIndex([]byte(s)).valid {
			t.Errorf("%q must make the index invalid", s)
		}
	}
	if _, ok := newSourceIndex([]byte("a\rb\n")).offset(1, 1); ok {
		t.Error("an invalid index must not convert positions")
	}
}

// The column of a finding inside an expression is counted in characters, like every other column, also after
// characters which take several bytes, in a block scalar and on a single line.
func TestExpressionColumnAfterMultiByteCharacters(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo hi\n          echo ${{ 'ééé' == github.nope }}\n      - run: echo ${{ 'ééé' == github.nope }}\n"
	errs := lintFileWithConfig(t, &Config{}, "ci.yaml", src)
	var cols []string
	for _, e := range errs {
		if strings.Contains(e.Message, `property "nope"`) {
			cols = append(cols, fmt.Sprintf("%d:%d", e.Line, e.Column))
		}
	}
	// the expression "github.nope" starts at the 29th character of line 8 and at the 32nd of line 9
	want := []string{"8:29", "9:32"}
	if !slices.Equal(cols, want) {
		t.Errorf("want %v but got %v", want, cols)
	}
}
