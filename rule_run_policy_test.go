package jactionlint

import (
	"io"
	"strings"
	"testing"
)

func lintRunPolicy(t *testing.T, cfg *Config, src string) []string {
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
	var ret []string
	for _, e := range errs {
		if e.Kind == "run-policy" {
			ret = append(ret, e.Message)
		}
	}
	return ret
}

func TestRunPolicyRequireShell(t *testing.T) {
	const pre = "on: push\n"
	tests := []struct {
		what string
		src  string
		want int
	}{
		{"no shell", pre + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", 1},
		{"step shell", pre + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        shell: bash\n", 0},
		{"job default", pre + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: echo\n", 0},
		{"workflow default", pre + "defaults:\n  run:\n    shell: bash\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", 0},
		{"default does not leak between jobs", pre + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        shell: bash\n    steps:\n      - run: echo\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", 1},
		{"defaults without shell", pre + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    defaults:\n      run:\n        working-directory: x\n    steps:\n      - run: echo\n", 1},
		{"uses step", pre + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n", 0},
		{"expression shell", pre + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        shell: ${{ matrix.shell }}\n", 0},
		{"reusable workflow call", pre + "jobs:\n  j:\n    uses: ./.github/workflows/x.yml\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := lintRunPolicy(t, ruleConfig("require-shell"), tc.src); len(got) != tc.want {
				t.Errorf("want %d errors but got %v", tc.want, got)
			}
			if got := lintRunPolicy(t, &Config{}, tc.src); len(got) != 0 {
				t.Errorf("rule must be disabled by default but got %v", got)
			}
			if got := lintRunPolicy(t, nil, tc.src); len(got) != 0 {
				t.Errorf("rule must be disabled without config but got %v", got)
			}
		})
	}
}

func TestRunPolicyMaxRunLines(t *testing.T) {
	wf := func(run string) string {
		return "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + run + "\n"
	}
	tests := []struct {
		what string
		run  string
		max  int
		want int
	}{
		{"single line", "echo", 1, 0},
		{"at limit", "|\n          echo 1\n          echo 2\n", 2, 0},
		{"over limit", "|\n          echo 1\n          echo 2\n          echo 3\n", 2, 1},
		{"blank lines not counted", "|\n          echo 1\n\n          echo 2\n\n", 2, 0},
		{"keep chomping", "|+\n          echo 1\n          echo 2\n\n\n\n", 2, 0},
		{"disabled", "|\n          echo 1\n          echo 2\n          echo 3\n", 0, 0},
		{"empty script", "''", 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			got := lintRunPolicy(t, maxRunLinesConfig(tc.max), wf(tc.run))
			if len(got) != tc.want {
				t.Errorf("want %d errors but got %v", tc.want, got)
			}
			for _, m := range got {
				if !strings.Contains(m, "max-run-lines") {
					t.Errorf("unexpected message %q", m)
				}
			}
		})
	}
}

func TestRunPolicyConfigParse(t *testing.T) {
	c := mustParseConfig(t, "require-shell: true\nmax-run-lines: 5\n")
	if m, ok := c.ruleOptionNumber("max-run-lines", "max"); !c.RuleEnabled("require-shell") || !c.RuleEnabled("max-run-lines") || !ok || m != 5 {
		t.Errorf("not translated: %+v", c)
	}
	c = mustParseConfig(t, "rules:\n  max-run-lines: {level: warn, max: 7}\n")
	if m, ok := c.ruleOptionNumber("max-run-lines", "max"); c.RuleLevel("max-run-lines") != SeverityWarning || !ok || m != 7 {
		t.Errorf("rules not parsed: %+v", c)
	}
	c = mustParseConfig(t, "config-variables: null\n")
	if c.RuleEnabled("require-shell") || c.RuleEnabled("max-run-lines") {
		t.Errorf("must be disabled by default: %+v", c)
	}
	if _, err := ParseConfig([]byte("max-run-lines: -1\n")); err == nil {
		t.Error("negative max-run-lines must be rejected")
	}
}
