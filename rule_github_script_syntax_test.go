package jactionlint

import (
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/execabs"
)

// skipWithoutNode skips a test that needs node (version 20 or later) to check a script.
func skipWithoutNode(t testing.TB) {
	t.Helper()
	p, err := execabs.LookPath("node")
	if err != nil {
		t.Skip("skipped because the \"node\" command does not exist in system")
	}
	if !nodeIsRecent(p) {
		t.Skip("skipped because \"node\" is older than version 20")
	}
}

func TestGithubScriptArguments(t *testing.T) {
	common := []string{"context", "core", "github", "io", "require"}
	tests := []struct {
		ref  string
		want []string
	}{
		{"v1", common},
		{"v2.0.0", common},
		{"v3", append(append([]string{}, common...), "glob")},
		{"v4", append(append([]string{}, common...), "glob", "exec", "__original_require__")},
		{"v7", append(append([]string{}, common...), "glob", "exec", "__original_require__")},
		{"7.0.1", append(append([]string{}, common...), "glob", "exec", "__original_require__")},
		{"v8", append(append([]string{}, common...), "glob", "exec", "__original_require__", "octokit")},
		{"v9.1", append(append([]string{}, common...), "glob", "exec", "__original_require__", "octokit", "getOctokit")},
		// What does not tell the version has what every version has
		{"60a0d83039c74a4aee543508d2ffcb1c3799cdea", common},
		{"main", common},
		{"v7-beta", common},
		{"", common},
	}
	for _, tc := range tests {
		if have := githubScriptArguments(tc.ref); !reflect.DeepEqual(have, tc.want) {
			t.Errorf("%q: have %v, want %v", tc.ref, have, tc.want)
		}
	}
}

func TestParseNodeSyntaxError(t *testing.T) {
	out := "[stdin]:5\n  state: ,\n         ^\n\nSyntaxError: Unexpected token ','\n    at wrapSafe (node:internal/modules/cjs/loader:1861:18)\n\nNode.js v24.0.0\n"
	e, ok := parseNodeSyntaxError(out)
	want := nodeSyntaxError{msg: "Unexpected token ','", line: 4, col: 9, n: 1}
	if !ok || e != want {
		t.Errorf("have %+v (%v), want %+v", e, ok, want)
	}
	// A region of several characters and CRLF
	out = "[stdin]:2\r\nconsole.log(title n)\r\n            ^^^^^\r\n\r\nSyntaxError: missing ) after argument list\r\n"
	e, ok = parseNodeSyntaxError(out)
	want = nodeSyntaxError{msg: "missing ) after argument list", line: 1, col: 12, n: 5}
	if !ok || e != want {
		t.Errorf("have %+v (%v), want %+v", e, ok, want)
	}
	// Nothing which is a syntax error
	for _, out := range []string{"", "Error: Cannot find module\n", "[stdin]:2\nfoo\n^\n\nReferenceError: x is not defined\n"} {
		if _, ok := parseNodeSyntaxError(out); ok {
			t.Errorf("%q is not a syntax error", out)
		}
	}
}

func TestGithubScriptSyntaxWithNode(t *testing.T) {
	skipWithoutNode(t)
	lint := func(t *testing.T, script string) []*Error {
		t.Helper()
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/github-script@v7\n        with:\n          script: " + script + "\n"
		var ret []*Error
		for _, e := range lintWithConfig(t, &Config{}, src) {
			if e.ID == "github-script-syntax" {
				ret = append(ret, e)
			}
		}
		return ret
	}

	t.Run("a script which is valid", func(t *testing.T) {
		for _, s := range []string{
			`"await github.rest.users.getAuthenticated(); return 1"`,
			`"const a = [1, 2, 3].map(x => x ** 2); return a // comment"`,
			`"const s = ` + "`${1 + 1}`" + `; return s"`,
			`"/* a */ return await Promise.all([])"`,
			`"label: for (;;) { break label }"`,
			`'"use strict"; return this === undefined'`,
		} {
			if errs := lint(t, s); len(errs) != 0 {
				t.Errorf("%s: unexpected %v", s, errs)
			}
		}
	})

	t.Run("the column counts the characters of the line, not the bytes or the UTF-16 units", func(t *testing.T) {
		// The emoji is two UTF-16 units and four bytes. The error is the second +
		errs := lint(t, `"const s = '😀 ' + + ; return s"`)
		if len(errs) != 1 {
			t.Fatalf("want one error, have %v", errs)
		}
		// script: is at column 11, the quote at 19 and the text starts at 20
		line := "          script: \"const s = '😀 ' + + ; return s\""
		want := len([]rune(line[:strings.Index(line, ";")])) + 1
		if errs[0].Line != 8 || errs[0].Column != want {
			t.Errorf("the error is at %d:%d, want 8:%d (%v)", errs[0].Line, errs[0].Column, want, errs[0])
		}
	})

	t.Run("an expression is not an error and keeps the places", func(t *testing.T) {
		errs := lint(t, `"return '${{ github.run_id }}' +* 1"`)
		if len(errs) != 1 || errs[0].Line != 8 {
			t.Fatalf("want one error at line 8, have %v", errs)
		}
		if want := strings.Index("          script: \"return '${{ github.run_id }}' +* 1\"", "*") + 1; errs[0].Column != want {
			t.Errorf("column %d, want %d", errs[0].Column, want)
		}
	})
}
