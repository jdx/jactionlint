package jactionlint

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const injectionSrc = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ github.event.issue.title }}\n      - run: echo ${{ undefined_var }}\n"

func lintIDs(t *testing.T, opts *LinterOptions, cfg *Config, src string) []string {
	t.Helper()
	l, err := NewLinter(io.Discard, opts)
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(cfg)
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range errs {
		ids = append(ids, e.ID)
	}
	return ids
}

func TestParseIgnorePattern(t *testing.T) {
	p, err := ParseIgnorePattern("unpinned-uses")
	if err != nil || p.ID != "unpinned-uses" || p.Regexp != nil || p.String() != "unpinned-uses" {
		t.Errorf("a rule ID must be an ID pattern: %+v %v", p, err)
	}
	p, err = ParseIgnorePattern("not-a-rule-id")
	if err != nil || p.ID != "" || p.Regexp == nil || p.String() != "not-a-rule-id" {
		t.Errorf("an unknown ID must be a regular expression: %+v %v", p, err)
	}
	p, err = ParseIgnorePattern("unpinned-uses.*")
	if err != nil || p.ID != "" || p.Regexp == nil {
		t.Errorf("a regular expression must stay: %+v %v", p, err)
	}
	if _, err := ParseIgnorePattern("("); err == nil {
		t.Error("an invalid regular expression must be an error")
	}

	id, _ := ParseIgnorePattern("template-injection")
	if !id.Match(&Error{ID: "template-injection", Message: "x"}) || id.Match(&Error{ID: "expression-type", Message: "template-injection"}) {
		t.Error("an ID pattern matches only the ID, not the message")
	}
	re, _ := ParseIgnorePattern("untrusted")
	if !re.Match(&Error{ID: "x", Message: "is untrusted"}) || re.Match(&Error{ID: "untrusted", Message: "x"}) {
		t.Error("a regular expression matches only the message")
	}
	var empty IgnorePattern
	if empty.Match(&Error{}) {
		t.Error("an empty pattern matches nothing")
	}
}

func TestIgnoreByRuleIDOption(t *testing.T) {
	all := lintIDs(t, &LinterOptions{}, &Config{}, injectionSrc)
	if diff := cmp.Diff([]string{"template-injection", "undefined-property"}, all); diff != "" {
		t.Fatalf("(-want +got): %s", diff)
	}

	got := lintIDs(t, &LinterOptions{IgnorePatterns: []string{"template-injection"}}, &Config{}, injectionSrc)
	if diff := cmp.Diff([]string{"undefined-property"}, got); diff != "" {
		t.Errorf("an ID must ignore the errors of the rule (-want +got): %s", diff)
	}

	got = lintIDs(t, &LinterOptions{IgnorePatterns: []string{"template-injection", "undefined variable"}}, &Config{}, injectionSrc)
	if len(got) != 0 {
		t.Errorf("an ID and a regular expression can be mixed: %v", got)
	}

	// The same word as a regular expression still matches messages, which keeps old command lines working
	got = lintIDs(t, &LinterOptions{IgnorePatterns: []string{"potentially"}}, &Config{}, injectionSrc)
	if diff := cmp.Diff([]string{"undefined-property"}, got); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestIgnoreByRuleIDInPaths(t *testing.T) {
	cfg := mustParseConfig(t, "paths:\n  'test.yaml':\n    ignore: [template-injection]\n  'other.yaml':\n    ignore: [undefined-property]\n")
	got := lintIDs(t, &LinterOptions{}, cfg, injectionSrc)
	if diff := cmp.Diff([]string{"undefined-property"}, got); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestIgnoreByRuleIDInComment(t *testing.T) {
	src := func(comment string) string {
		return "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      " + comment + "\n      - if: ${{ undefined_var }}\n        run: echo ${{ github.event.issue.title }}\n"
	}
	tests := []struct {
		comment string
		want    []string
	}{
		{"# jactionlint ignore=template-injection", []string{"undefined-property"}},
		{"# jactionlint ignore=template-injection,undefined-property", nil},
		{"# jactionlint ignore=template-injection, undefined-property", nil},
		{"# actionlint ignore=undefined-property", []string{"template-injection"}},
		{"# jactionlint ignore=template-injection,undefined variable", nil},
		{"# jactionlint ignore=expression-type", []string{"undefined-property", "template-injection"}},
		{"# jactionlint ignore=this-id-does-not-exist", []string{"undefined-property", "template-injection"}},
	}
	for _, tc := range tests {
		got := lintIDs(t, &LinterOptions{}, &Config{}, src(tc.comment))
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("%s (-want +got): %s", tc.comment, diff)
		}
	}
}

func TestUnusedIgnoreDetection(t *testing.T) {
	src := `on: push
jobs:
  j:
    runs-on: ubuntu-24.04
    steps:
      # jactionlint ignore=template-injection,expression-type
      - run: echo ${{ github.event.issue.title }}
      # jactionlint ignore=this does not match, undefined-property
      - run: echo ${{ undefined_var }}
      # jactionlint ignore=require-shell
      - run: echo
      # jactionlint ignore=invalid-glob
`
	strict := mustParseConfig(t, "profile: pedantic\nrules:\n  missing-permissions: off\n  missing-timeout: off\n")

	var got []*Error
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = strict
	got, err = l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}

	var have []string
	// Findings of the other rules of the strict profile are not the point of the test
	got = slices.DeleteFunc(got, func(e *Error) bool { return e.Kind != "ignore" })
	for _, e := range got {
		have = append(have, fmt.Sprintf("%d:%d %s", e.Line, e.Column, e.ID))
		if e.Kind != "ignore" || e.Severity != SeverityError {
			t.Errorf("unexpected kind or severity: %+v", e)
		}
	}
	want := []string{
		"6:47 unused-ignore",  // expression-type did not suppress anything
		"8:28 unused-ignore",  // the regular expression did not match
		"12:28 unused-ignore", // the comment applies to nothing
	}
	// require-shell is off in the strict profile so the pattern is not reported
	if diff := cmp.Diff(want, have); diff != "" {
		for _, e := range got {
			t.Log(e)
		}
		t.Errorf("(-want +got): %s", diff)
	}
	if len(got) > 0 && !strings.Contains(got[0].Message, `"expression-type" did not suppress any error`) {
		t.Errorf("unexpected message: %q", got[0].Message)
	}
	if len(got) > 2 && !strings.Contains(got[2].Message, "is not followed by any line") {
		t.Errorf("unexpected message: %q", got[2].Message)
	}

	// The check is not in the default profile
	ids := lintIDs(t, &LinterOptions{}, &Config{}, src)
	for _, id := range ids {
		if id == "unused-ignore" {
			t.Error("unused-ignore must be off by default")
		}
	}

	// It can be turned on alone and lowered
	cfg := mustParseConfig(t, "rules:\n  unused-ignore: warn\n")
	l.defaultConfig = cfg
	got, _ = l.Lint("test.yaml", []byte(src), nil)
	n := 0
	for _, e := range got {
		if e.ID == "unused-ignore" {
			n++
			if e.Severity != SeverityWarning {
				t.Errorf("unexpected severity %v", e.Severity)
			}
		}
	}
	if n != 3 {
		t.Errorf("want 3 unused ignores but got %d: %v", n, got)
	}

	// An ignore of -ignore or "paths" suppresses an error first but the inline pattern still counts as used
	l, _ = NewLinter(io.Discard, &LinterOptions{IgnorePatterns: []string{"template-injection"}})
	l.defaultConfig = mustParseConfig(t, "profile: pedantic\nrules:\n  missing-permissions: off\n  missing-timeout: off\n  require-shell: off\n")
	got, _ = l.Lint("test.yaml", []byte(src), nil)
	for _, e := range got {
		if e.ID == "unused-ignore" && e.Line == 6 && e.Column == 28 {
			t.Errorf("template-injection pattern was used: %v", e)
		}
	}
}

func TestMinSeverity(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  template-injection: info\n  undefined-property: warn\n  expression-type: error\n")
	src := injectionSrc + "      - run: echo ${{ 1 == 'a' && null.x }}\n"
	tests := []struct {
		min  Severity
		want []string
	}{
		{SeverityOff, []string{"template-injection", "undefined-property", "expression-type"}},
		{SeverityInfo, []string{"template-injection", "undefined-property", "expression-type"}},
		{SeverityWarning, []string{"undefined-property", "expression-type"}},
		{SeverityError, []string{"expression-type"}},
	}
	for _, tc := range tests {
		got := lintIDs(t, &LinterOptions{MinSeverity: tc.min}, cfg, src)
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("min=%v (-want +got): %s", tc.min, diff)
		}
	}
}

func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	code := cmd.Main(append([]string{"jactionlint", "-no-color"}, args...))
	return code, stdout.String(), stderr.String()
}

func TestCommandExitStatusBySeverity(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wf, []byte(injectionSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	errorCfg := write("error.yaml", "rules:\n  missing-timeout: off\n")
	warnCfg := write("warn.yaml", "rules:\n  missing-timeout: off\n  template-injection: warn\n  undefined-property: warn\n")
	infoCfg := write("info.yaml", "rules:\n  missing-timeout: off\n  template-injection: info\n  undefined-property: info\n")
	mixedCfg := write("mixed.yaml", "rules:\n  missing-timeout: off\n  template-injection: warn\n")
	offCfg := write("off.yaml", "rules:\n  missing-timeout: off\n  template-injection: off\n  undefined-property: off\n")

	tests := []struct {
		name string
		args []string
		want int
		out  string // substring of stdout. empty means no output
	}{
		{"errors", []string{"-config-file", errorCfg}, 1, "potentially untrusted"},
		{"warnings do not fail", []string{"-config-file", warnCfg}, 0, "potentially untrusted"},
		{"warnings fail with -strict-exit", []string{"-strict-exit", "-config-file", warnCfg}, 1, "potentially untrusted"},
		{"infos do not fail", []string{"-config-file", infoCfg}, 0, "potentially untrusted"},
		{"infos fail with -strict-exit", []string{"-strict-exit", "-config-file", infoCfg}, 1, "potentially untrusted"},
		{"one error among warnings", []string{"-config-file", mixedCfg}, 1, "undefined variable"},
		{"min-severity hides warnings", []string{"-min-severity", "error", "-config-file", warnCfg}, 0, ""},
		{"min-severity error with -strict-exit", []string{"-min-severity", "error", "-strict-exit", "-config-file", warnCfg}, 0, ""},
		{"min-severity warn shows warnings", []string{"-min-severity", "warn", "-config-file", warnCfg}, 0, "potentially untrusted"},
		{"min-severity warn hides infos", []string{"-min-severity", "warn", "-strict-exit", "-config-file", infoCfg}, 0, ""},
		{"rules which are off", []string{"-strict-exit", "-config-file", offCfg}, 0, ""},
		{"ignore by ID", []string{"-ignore", "template-injection", "-ignore", "undefined-property", "-config-file", errorCfg}, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCommand(t, append(tc.args, wf)...)
			if code != tc.want {
				t.Errorf("want exit status %d but got %d\nstdout: %s\nstderr: %s", tc.want, code, stdout, stderr)
			}
			if tc.out == "" && stdout != "" {
				t.Errorf("no output is expected but got %q", stdout)
			}
			if tc.out != "" && !strings.Contains(stdout, tc.out) {
				t.Errorf("output must contain %q but got %q", tc.out, stdout)
			}
		})
	}

	for _, v := range []string{"fatal", "off", ""} {
		code, _, stderr := runCommand(t, "-min-severity", v, wf)
		if code != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "-min-severity") {
			t.Errorf("-min-severity %q: exit status %d, stderr %q", v, code, stderr)
		}
	}
}

func TestExitStatusOf(t *testing.T) {
	e := func(s Severity) *Error { return &Error{Severity: s} }
	tests := []struct {
		errs   []*Error
		strict bool
		want   int
	}{
		{nil, false, 0},
		{nil, true, 0},
		{[]*Error{e(SeverityInfo), e(SeverityWarning)}, false, 0},
		{[]*Error{e(SeverityInfo), e(SeverityWarning)}, true, 1},
		{[]*Error{e(SeverityInfo), e(SeverityError)}, false, 1},
	}
	for i, tc := range tests {
		if got := exitStatusOf(tc.errs, tc.strict); got != tc.want {
			t.Errorf("case %d: want %d but got %d", i, tc.want, got)
		}
	}
}
