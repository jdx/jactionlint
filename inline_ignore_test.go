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

func TestParseInlineIgnoresNoPanic(t *testing.T) {
	for _, s := range []string{"", "# actionlint ignore=", "# actionlint ignore=,,", "# actionlint ignore=a", "\n\n# actionlint ignore=a\n"} {
		parseInlineIgnores([]byte(s))
	}
}
