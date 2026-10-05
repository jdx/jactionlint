package actionlint

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
		"  # actionlint ignore=x\r\n" + // 2
		"  - run: |\r\n" + // 3
		"      line\r\n" + // 4
		"\r\n" + // 5
		"      line\r\n" + // 6
		"  - run: y\r\n" + // 7
		"  # actionlint ignore=z\r\n" + // 8
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
		"    # actionlint ignore=x\n" + // 3
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
		"- # actionlint ignore=x\n" + // 2 (not an own-line comment: ignored)
		"  run: a\n" // 3
	if ignores, _ = parseInlineIgnores([]byte(src)); len(ignores) != 0 {
		t.Errorf("trailing comment must not create an ignore: %v", ignores)
	}
	src = "steps:\n" + // 1
		"  # actionlint ignore=x\n" + // 2
		"  - run: a\n" + // 3
		"  - run: b\n" // 4
	ignores, _ = parseInlineIgnores([]byte(src))
	if len(ignores) != 1 || ignores[0].start != 3 || ignores[0].end != 3 {
		t.Errorf("sibling item must not be covered: %v", ignores)
	}
}

func TestParseInlineIgnoresCommentInsideBlock(t *testing.T) {
	src := "# actionlint ignore=x\n" + // 1
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
	for _, s := range []string{"", "# actionlint ignore=", "# actionlint ignore=,,", "# actionlint ignore=a", "\n\n# actionlint ignore=a\n"} {
		parseInlineIgnores([]byte(s))
	}
}
