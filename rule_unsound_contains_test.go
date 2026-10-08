package jactionlint

import (
	"strings"
	"testing"
)

func TestUnsoundContains(t *testing.T) {
	cfg := ruleConfig("unsound-contains")
	tests := []struct {
		cond string
		want int
	}{
		{"contains('refs/heads/main refs/heads/develop', github.ref)", 1},
		{"${{ contains('a b', github.ref_name) }}", 1},
		{"${{ github.event_name == 'push' && contains('a b', github.ref_name) }}", 1},
		{"contains('a b', github.ref) || contains('c d', github.ref)", 2},
		{"contains(fromJSON('[\"a\", \"b\"]'), github.ref)", 0},
		{"contains(github.ref, 'main')", 0},
		{"contains('a', 'b')", 0},
		{"CONTAINS('a b', github.ref)", 1},
	}
	for _, tc := range tests {
		t.Run(tc.cond, func(t *testing.T) {
			for _, quoted := range []string{"", "'"} {
				if quoted != "" && strings.Contains(tc.cond, "'") {
					continue
				}
				src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    if: " + quoted + tc.cond + quoted + "\n    steps:\n      - run: echo\n"
				if got := len(errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "unsound-contains")); got != tc.want {
					t.Errorf("want %d errors but got %d for %s", tc.want, got, src)
				}
			}
		})
	}

	// Only conditions are checked
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ contains('a b', github.ref) }}\n"
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", src), "unsound-contains")

	// The position is the call, also in a multi-line condition
	src = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    if: |\n      github.event_name == 'push' &&\n      contains('a b', github.ref)\n    steps:\n      - run: echo\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "unsound-contains")
	if len(errs) != 1 || errs[0].Line != 7 || errs[0].Column != 7 {
		t.Errorf("unexpected errors: %v", errs)
	}
}
