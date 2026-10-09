package jactionlint

import (
	"reflect"
	"testing"
)

func TestSplitIgnoreList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a", []string{"a"}},
		{"a, b", []string{"a", " b"}},
		{"a{1,2},b", []string{"a{1,2}", "b"}},
		{"(a,b)|[,],c", []string{"(a,b)|[,]", "c"}},
		{`a\,b,c`, []string{`a\,b`, "c"}},
		{"", []string{""}},
		{`a\`, []string{`a\`}},
	}
	for _, tc := range tests {
		if have := splitIgnoreList(tc.in); !reflect.DeepEqual(have, tc.want) {
			t.Errorf("%q: want %q but have %q", tc.in, tc.want, have)
		}
	}
}

func TestParseInlineIgnoresRange(t *testing.T) {
	src := "a:\r\n" + // 1
		"  # jactionlint ignore=x\r\n" + // 2
		"  - run: |\r\n" + // 3
		"      line\r\n" + // 4
		"\r\n" + // 5
		"      line\r\n" + // 6
		"  - run: y\r\n" + // 7
		"  # jactionlint ignore=z\r\n" + // 8
		"b: 1\r\n" // 9
	ignores, errs := parseInlineIgnores([]byte(src))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(ignores) != 2 {
		t.Fatalf("want 2 ignores but have %d", len(ignores))
	}
	if ignores[0].start != 3 || ignores[0].end != 6 {
		t.Errorf("unexpected range: %d-%d", ignores[0].start, ignores[0].end)
	}
	// The second comment applies to "b: 1"
	if ignores[1].start != 9 || ignores[1].end != 9 {
		t.Errorf("unexpected range: %d-%d", ignores[1].start, ignores[1].end)
	}
}

func TestParseInlineIgnoresSameIndentSequence(t *testing.T) {
	// Items of a sequence may sit at the same column as the key which holds it.
	src := "jobs:\n" + // 1
		"  t:\n" + // 2
		"    # jactionlint ignore=x\n" + // 3
		"    steps:\n" + // 4
		"    - run: a\n" + // 5
		"    - run: |\n" + // 6
		"        b\n" + // 7
		"    timeout-minutes: 1\n" // 8
	ignores, errs := parseInlineIgnores([]byte(src))
	if len(errs) != 0 || len(ignores) != 1 {
		t.Fatal(errs, ignores)
	}
	if ignores[0].start != 4 || ignores[0].end != 7 {
		t.Errorf("unexpected range: %d-%d", ignores[0].start, ignores[0].end)
	}

	// When the target is a sequence item, its siblings are not covered.
	src = "steps:\n" + // 1
		"- # jactionlint ignore=x\n" + // 2 (a comment after the dash covers the whole item)
		"  run: a\n" + // 3
		"- run: b\n" // 4
	if ignores, _ = parseInlineIgnores([]byte(src)); len(ignores) != 1 || ignores[0].start != 2 || ignores[0].end != 3 {
		t.Errorf("a comment after the dash must cover the item: %v", ignores)
	}
	src = "steps:\n" + // 1
		"  # jactionlint ignore=x\n" + // 2
		"  - run: a\n" + // 3
		"  - run: b\n" // 4
	ignores, _ = parseInlineIgnores([]byte(src))
	if len(ignores) != 1 || ignores[0].start != 3 || ignores[0].end != 3 {
		t.Errorf("sibling item must not be covered: %v", ignores)
	}
}

func TestParseInlineIgnoresCommentInsideBlock(t *testing.T) {
	src := "# jactionlint ignore=x\n" + // 1
		"a:\n" + // 2
		"  b: 1\n" + // 3
		"# a comment at column 0\n" + // 4
		"  c: 2\n" + // 5
		"d: 3\n" // 6
	ignores, errs := parseInlineIgnores([]byte(src))
	if len(errs) != 0 || len(ignores) != 1 {
		t.Fatal(errs, ignores)
	}
	if ignores[0].start != 2 || ignores[0].end != 5 {
		t.Errorf("unexpected range: %d-%d", ignores[0].start, ignores[0].end)
	}
}

func TestParseInlineIgnoresNoPanic(t *testing.T) {
	for _, s := range []string{"", "# jactionlint ignore=", "# jactionlint ignore=,,", "# jactionlint ignore=a", "\n\n# jactionlint ignore=a\n"} {
		parseInlineIgnores([]byte(s))
	}
}

func TestParseInlineIgnoresAcceptsBothNames(t *testing.T) {
	// "actionlint" is the name used by the original actionlint
	src := "a:\n" + // 1
		"  # actionlint ignore=x\n" + // 2
		"  b: 1\n" + // 3
		"  # jactionlint ignore=y\n" + // 4
		"  c: 2\n" // 5
	ignores, errs := parseInlineIgnores([]byte(src))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(ignores) != 2 {
		t.Fatalf("want 2 ignores but have %d", len(ignores))
	}
	if ignores[0].start != 3 || ignores[1].start != 5 {
		t.Errorf("unexpected ranges: %d-%d and %d-%d", ignores[0].start, ignores[0].end, ignores[1].start, ignores[1].end)
	}
}

func TestParseInlineIgnoresInvalidPatternColumn(t *testing.T) {
	tests := []struct {
		comment string
		col     int
	}{
		{"# jactionlint ignore=(", 3},
		{"# actionlint ignore=(", 3},
	}
	for _, tc := range tests {
		_, errs := parseInlineIgnores([]byte(tc.comment + "\nb: 1\n"))
		if len(errs) != 1 {
			t.Fatalf("%q: want 1 error but have %v", tc.comment, errs)
		}
		if errs[0].Column != tc.col {
			t.Errorf("%q: want column %d but have %d", tc.comment, tc.col, errs[0].Column)
		}
	}
}

func TestParseInlineIgnoresTrailing(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		start, end int // 0 when there must be no ignore
		errs       int
	}{
		{"on a key line", "a:\n  b: 1 # jactionlint ignore=x\n  c: 2\n", 2, 2, 0},
		{"covers nested lines", "a:  # jactionlint ignore=x\n  b: 1\n  c: 2\nd: 3\n", 1, 3, 0},
		{"first line of a step covers the step", "steps:\n  - uses: a/b@v1 # jactionlint ignore=x\n    with:\n      k: v\n  - run: y\n", 2, 4, 0},
		{"after the version comment", "steps:\n  - uses: a/b@sha # v1.2.3 # jactionlint ignore=x\n  - run: y\n", 2, 2, 0},
		{"actionlint spelling", "steps:\n  - run: a # actionlint ignore=x\n", 2, 2, 0},
		{"no space after hash", "steps:\n  - run: a #jactionlint ignore=x\n", 2, 2, 0},
		{"in a quoted string", "steps:\n  - run: \"a # jactionlint ignore=x\"\n", 0, 0, 0},
		{"in a single quoted string", "steps:\n  - run: 'a # jactionlint ignore=x'\n", 0, 0, 0},
		{"in a plain scalar without a space", "steps:\n  - run: a#jactionlint ignore=x\n", 0, 0, 0},
		{"in a block scalar", "steps:\n  - run: |\n      echo a # jactionlint ignore=x\n", 0, 0, 0},
		{"mentioned in prose", "steps:\n  - run: a # please do not jactionlint ignore=x\n", 0, 0, 0},
		{"invalid regular expression", "steps:\n  - run: a # jactionlint ignore=(\n", 0, 0, 1},
		{"non-ASCII before the comment", "steps:\n  - name: \"héllo wörld\" # jactionlint ignore=x\n", 2, 2, 0},
		{"CRLF", "steps:\r\n  - run: a # jactionlint ignore=x\r\n  - run: b\r\n", 2, 2, 0},
		{"flow sequence item", "steps: [a, b] # jactionlint ignore=x\nother: 1\n", 1, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ignores, errs := parseInlineIgnores([]byte(tc.src))
			if len(errs) != tc.errs {
				t.Fatalf("errors: %v", errs)
			}
			if tc.start == 0 {
				if len(ignores) != 0 {
					t.Fatalf("no ignore expected: %v", ignores)
				}
				return
			}
			if len(ignores) != 1 || ignores[0].start != tc.start || ignores[0].end != tc.end || len(ignores[0].entries) != 1 {
				t.Fatalf("want range %d-%d, got %v", tc.start, tc.end, ignores)
			}
			if got := ignores[0].entries[0].pat.String(); got != "x" {
				t.Errorf("pattern: %q", got)
			}
		})
	}
}

func TestParseInlineIgnoresTrailingAndAbove(t *testing.T) {
	// A comment above and a comment at the end of the line both apply to the item
	src := "steps:\n  # jactionlint ignore=a\n  - run: x # jactionlint ignore=b\n  - run: y\n"
	ignores, errs := parseInlineIgnores([]byte(src))
	if len(errs) != 0 || len(ignores) != 1 || ignores[0].start != 3 || ignores[0].end != 3 || len(ignores[0].entries) != 2 {
		t.Fatal(ignores, errs)
	}
}

func TestParseInlineIgnoresTrailingPatternPositions(t *testing.T) {
	src := "steps:\n  - run: a # v1 # jactionlint ignore=aa, bb\n"
	ignores, _ := parseInlineIgnores([]byte(src))
	if len(ignores) != 1 || len(ignores[0].entries) != 2 {
		t.Fatal(ignores)
	}
	e := ignores[0].entries
	// "  - run: a # v1 # jactionlint ignore=" is 37 characters long
	if e[0].line != 2 || e[0].col != 38 || e[1].col != 42 {
		t.Errorf("positions: %d:%d and %d:%d", e[0].line, e[0].col, e[1].line, e[1].col)
	}
}

func TestUnusedTrailingIgnoreFix(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"alone", "      - uses: actions/checkout@v4 # jactionlint ignore=missing-timeout", "      - uses: actions/checkout@v4"},
		{"keeps the version comment", "      - uses: actions/checkout@v4 # v4 # jactionlint ignore=missing-timeout", "      - uses: actions/checkout@v4 # v4"},
		{"keeps the used pattern", "      - uses: actions/checkout@v4 # jactionlint ignore=missing-timeout,unpinned-uses", "      - uses: actions/checkout@v4 # jactionlint ignore=unpinned-uses"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n" + tc.line + "\n"
			errs := lintWithConfig(t, &Config{Profile: ProfilePedantic}, src)
			var fixed []*Error
			for _, e := range errs {
				if e.ID == "unused-ignore" {
					fixed = append(fixed, e)
				}
			}
			if len(fixed) != 1 || fixed[0].Fix == nil {
				t.Fatalf("want one fixable unused-ignore: %v", errs)
			}
			out, n := applyFixes([]byte(src), fixed, FixModeSafe)
			if n != 1 {
				t.Fatalf("applied %d fixes", n)
			}
			want := "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n" + tc.want + "\n"
			if string(out) != want {
				t.Errorf("got:\n%s\nwant:\n%s", out, want)
			}
		})
	}
}
