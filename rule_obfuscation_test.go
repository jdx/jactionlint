package jactionlint

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestObfuscatedUsesPath(t *testing.T) {
	tests := []struct {
		uses  string
		clean string // empty: not obfuscated, "!": obfuscated without a plain form
	}{
		{"actions/checkout@v4", ""},
		{"actions/checkout/sub@v4", ""},
		{"actions/checkout/sub/@v4", ""},
		{"actions/checkout/./sub@v4", "actions/checkout/sub"},
		{"actions/checkout/a//b@v4", "actions/checkout/a/b"},
		{"actions/checkout/a/../b@v4", "actions/checkout/b"},
		{"actions/checkout/../b@v4", "!"},
		{"actions/checkout/a/./../b/.@v4", "actions/checkout/b"},
		{"./", ""},
		{"./.github/actions/x", ""},
		{"./.github/actions/x/", ""},
		{"./.github/./actions/x", "./.github/actions/x"},
		{"./a/../b", "./b"},
		{"./a//b", "./a/b"},
		{"./../other-checkout/x", ""},
		{"./../../x", ""},
		{"./a/../../x", "./../x"},
		{"$/x/./y", "$/x/y"},
		{"docker://alpine:3", ""},
		{"actions/${{ matrix.x }}/./y@v4", ""},
		{"actions//checkout@v4", ""}, // invalid: reported by the action rule
	}
	for _, tc := range tests {
		t.Run(tc.uses, func(t *testing.T) {
			path, clean, ok := obfuscatedUsesPath(tc.uses)
			switch {
			case tc.clean == "" && ok:
				t.Errorf("unexpectedly obfuscated: %q -> %q", path, clean)
			case tc.clean == "!" && (!ok || clean != ""):
				t.Errorf("want obfuscated without a plain form but got %q, %v", clean, ok)
			case tc.clean != "" && tc.clean != "!" && (!ok || clean != tc.clean):
				t.Errorf("want %q but got %q, %v", tc.clean, clean, ok)
			}
		})
	}
}

func TestObfuscationUsesFix(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  obfuscation: warn\n")
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout/a/../b@v4\n      - uses: \"./.github/./actions/x\"\n"
	errs := lintWithConfig(t, cfg, src)
	if len(errs) != 2 {
		t.Fatalf("want two errors but got %v", errs)
	}
	if out, n := applyFixes([]byte(src), errs, FixModeSafe); n != 0 || string(out) != src {
		t.Errorf("the fix is unsafe so it must not be applied by default")
	}
	got, rest := fixAll(t, cfg, src, FixModeUnsafe)
	want := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout/b@v4\n      - uses: \"./.github/actions/x\"\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
	for _, e := range rest {
		if e.ID == "obfuscation" {
			t.Errorf("still reported: %v", e)
		}
	}
}

func TestFormatOfLiterals(t *testing.T) {
	tests := []struct {
		expr string
		want string
		ok   bool
	}{
		{"format('{0}/{1}', 'a', 'b')", "a/b", true},
		{"format('{1}{0}', 'a', 'b')", "ba", true},
		{"format('{{0}} {0}', 1)", "{0} 1", true},
		{"format('{0} {1} {2}', true, null, 'x')", "true  x", true},
		{"format('plain')", "plain", true},
		{"format('{0}', github.sha)", "", false},
		{"format('{2}', 'a')", "", false},
		{"format('{0', 'a')", "", false},
		{"format('}', 'a')", "", false},
		{"format('{0}', 1.5)", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			n, err := NewExprParser().Parse(NewExprLexer(tc.expr + "}}"))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := formatOfLiterals(n.(*FuncCallNode).Args)
			if ok != tc.ok || got != tc.want {
				t.Errorf("want %q, %v but got %q, %v", tc.want, tc.ok, got, ok)
			}
		})
	}
}

func TestObfuscationOfExpressions(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  obfuscation: warn\n")
	tests := []struct {
		expr string
		want int
	}{
		{"format('{0}', 'a')", 1},
		{"'literal'", 1},
		{"1 == 1", 1},
		{"fromJSON(toJSON('[1]'))", 1},
		{"fromJSON(toJSON(github.event))", 0},
		{"vars[github.job]", 1},
		{"vars[format('X_{0}', github.job)]", 1},
		{"github['sha']", 0},
		{"github.event.commits[0]", 0},
		{"format('{0}', github.sha)", 0},
		{"github.sha", 0},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      X: ${{ " + tc.expr + " }}\n    steps:\n      - run: echo\n"
			n := 0
			for _, e := range lintWithConfig(t, cfg, src) {
				if e.ID == "obfuscation" {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d findings but got %d", tc.want, n)
			}
		})
	}
}
