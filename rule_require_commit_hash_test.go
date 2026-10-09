package jactionlint

import (
	"io"
	"strings"
	"testing"
)

func TestRequireCommitHashOptIn(t *testing.T) {
	const sha = "db41740e12847bb616a339b75eb9414e711417df"
	const digest = "sha256:3235326357dfb65f1781dbc4df3b834546d8bf914e82cce58e6e6b676e23ce8f"

	tests := []struct {
		uses   string
		reused bool
		want   bool // whether an error is expected when enabled
	}{
		{"actions/checkout@" + sha, false, false},
		{"actions/checkout@" + strings.ToUpper(sha), false, false},
		{"actions/checkout/sub@" + sha, false, false},
		{"actions/checkout@v4", false, true},
		{"actions/checkout@main", false, true},
		{"actions/checkout@" + sha[:7], false, true},
		{"actions/checkout@" + sha + "0", false, true},
		{"./local", false, false},
		{"$/local", false, false},
		{"docker://image@" + digest, false, false},
		{"docker://ghcr.io/o/image@" + digest, false, false},
		{"docker://image:latest", false, true},
		{"docker://image", false, true},
		{"docker://image:sha256:abcd", false, true},
		{"o/r/.github/workflows/w.yml@" + sha, true, false},
		{"o/r/.github/workflows/w.yml@v1", true, true},
	}

	for _, tc := range tests {
		for _, enabled := range []bool{false, true} {
			var src string
			if tc.reused {
				src = "on: push\njobs:\n  j:\n    uses: " + tc.uses + "\n"
			} else {
				src = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + tc.uses + "\n"
			}
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = ruleSwitch("unpinned-uses", enabled)
			errs, err := l.Lint("test.yaml", []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, e := range errs {
				if e.ID == "unpinned-uses" && strings.Contains(e.Message, "unpinned-uses") {
					found = true
				}
			}
			if want := tc.want && enabled; found != want {
				t.Errorf("uses=%q enabled=%v: want error=%v got=%v (%v)", tc.uses, enabled, want, found, errs)
			}
		}
	}
}

func TestRequireCommitHashConfigParse(t *testing.T) {
	c := mustParseConfig(t, "require-commit-hash: true\n")
	if !c.RuleEnabled("unpinned-uses") {
		t.Error("require-commit-hash: true was not translated to unpinned-uses")
	}
	if len(c.Deprecations) != 1 || !strings.Contains(c.Deprecations[0], "require-commit-hash") {
		t.Errorf("deprecation was not reported: %v", c.Deprecations)
	}
	c = mustParseConfig(t, "rules:\n  unpinned-uses: warn\n")
	if c.RuleLevel("unpinned-uses") != SeverityWarning {
		t.Error("rules: unpinned-uses: warn was not parsed")
	}
	c = mustParseConfig(t, "config-variables: null\n")
	if c.RuleEnabled("unpinned-uses") {
		t.Error("unpinned-uses must be disabled by default")
	}
}
