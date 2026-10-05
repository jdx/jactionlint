package jactionlint

import (
	"io"
	"strings"
	"testing"
)

func lintWithConfig(t *testing.T, cfg *Config, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func countMsg(errs []*Error, sub string) int {
	n := 0
	for _, e := range errs {
		if strings.Contains(e.Message, sub) {
			n++
		}
	}
	return n
}

func TestRequireExpressionWrapping(t *testing.T) {
	tests := []struct {
		cond string
		want int
	}{
		{"github.ref == 'refs/heads/main'", 1},
		{"always()", 1},
		{"${{ github.ref == 'refs/heads/main' }}", 0},
		{"${{ always() }}", 0},
		{"'${{ github.ref }}' == 'x'", 0}, // contains ${{ }}; the if-cond rule handles it
	}
	for _, tc := range tests {
		src := "on: push\njobs:\n  j:\n    if: " + tc.cond + "\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        if: " + tc.cond + "\n"
		for _, enabled := range []bool{false, true} {
			errs := lintWithConfig(t, &Config{RequireExpressionWrapping: enabled}, src)
			want := 0
			if enabled {
				want = tc.want * 2 // job and step
			}
			if got := countMsg(errs, "require-expression-wrapping"); got != want {
				t.Errorf("cond=%q enabled=%v: want %d errors got %d (%v)", tc.cond, enabled, want, got, errs)
			}
		}
	}
}

func TestCheckFalsyTernary(t *testing.T) {
	tests := []struct {
		expr string
		want int
	}{
		{"github.ref == 'x' && '' || 'staging-'", 1},
		{"github.ref == 'x' && 0 || 1", 1},
		{"github.ref == 'x' && 0.0 || 1", 1},
		{"github.ref == 'x' && false || 'y'", 1},
		{"github.ref == 'x' && null || 'y'", 1},
		{"a.b && c.d && '' || 'y'", 1},
		{"(github.ref == 'x' && '') || 'y'", 1},
		{"github.ref == 'x' && 'a' || 'b'", 0},
		{"github.ref == 'x' && 1 || 2", 0},
		{"github.ref == 'x' && true || false", 0},
		{"github.ref == 'x' && github.sha || 'b'", 0}, // not a literal; could be falsy but undecidable
		{"github.ref == 'x' && '' ", 0},               // no ||
		{"github.ref == 'x' || ''", 0},
		{"github.ref == 'x' && (false || 'y')", 0},
	}
	for _, tc := range tests {
		src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ " + tc.expr + " }}\n"
		for _, enabled := range []bool{false, true} {
			errs := lintWithConfig(t, &Config{CheckFalsyTernary: enabled}, src)
			want := 0
			if enabled {
				want = tc.want
			}
			if got := countMsg(errs, "always falsy"); got != want {
				t.Errorf("expr=%q enabled=%v: want %d got %d (%v)", tc.expr, enabled, want, got, errs)
			}
		}
	}
}

func TestCheckFalsyTernaryPosition(t *testing.T) {
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ github.ref == 'x' && '' || 'y' }}\n"
	errs := lintWithConfig(t, &Config{CheckFalsyTernary: true}, src)
	if len(errs) != 1 {
		t.Fatalf("want 1 error: %v", errs)
	}
	if errs[0].Line != 6 || errs[0].Column != 44 {
		t.Errorf("unexpected position %d:%d", errs[0].Line, errs[0].Column)
	}
}

func TestOptInRulesConfigParse(t *testing.T) {
	c, err := ParseConfig([]byte("require-expression-wrapping: true\ncheck-falsy-ternary: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !c.RequireExpressionWrapping || !c.CheckFalsyTernary {
		t.Errorf("not parsed: %+v", c)
	}
	c, err = ParseConfig([]byte("config-variables: null\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.RequireExpressionWrapping || c.CheckFalsyTernary {
		t.Error("must be disabled by default")
	}
}

func TestCheckFalsyTernaryPositionInLiteralBlock(t *testing.T) {
	// The column of a token on a later line of a literal block must account for the stripped indentation.
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo ${{\n            github.ref == 'x' && '' || 'y'\n          }}\n"
	errs := lintWithConfig(t, &Config{CheckFalsyTernary: true}, src)
	if len(errs) != 1 {
		t.Fatalf("want 1 error: %v", errs)
	}
	if errs[0].Line != 8 || errs[0].Column != 34 {
		t.Errorf("unexpected position %d:%d", errs[0].Line, errs[0].Column)
	}
}
