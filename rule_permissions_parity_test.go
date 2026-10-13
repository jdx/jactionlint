package jactionlint

import (
	"strings"
	"testing"
)

// The pedantic persona of zizmor reports a workflow without a top-level "permissions:" (at the start of the
// document) and a comment-less permission block that holds a read scope. These tests pin how the pedantic
// profile, the other profiles and the explicit options decide that.

const permsLicenseHeader = `# Copyright 2026 Example
#
# Licensed under the Apache License.

`

func permLines(errs []*Error, id string) []int {
	var lines []int
	for _, e := range policyErrorsOf(errs, id) {
		lines = append(lines, e.Line)
	}
	return lines
}

func TestExcessivePermissionsWorkflowLevelByProfile(t *testing.T) {
	src := permsLicenseHeader + `name: ci
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    permissions:
      contents: read # checkout
    steps:
      - run: echo hello
`
	tests := []struct {
		name string
		cfg  *Config
		want []int
	}{
		// the line of the first key, not line 1: the license header must not move the finding away from zizmor's
		{"pedantic profile", &Config{Profile: ProfilePedantic}, []int{5}},
		{"default profile", &Config{Profile: ProfileDefault}, nil},
		{"pedantic profile, option off", &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{
			"excessive-permissions": {Level: SeverityError, Options: map[string]any{"require-workflow-permissions": false}}}}, nil},
		{"default profile, option on", &Config{Profile: ProfileDefault, Rules: map[string]RuleConfig{
			"excessive-permissions": {Level: SeverityError, Options: map[string]any{"require-workflow-permissions": true}}}}, []int{5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := permLines(lintWithConfig(t, withFixtureRules(tc.cfg), src), "excessive-permissions")
			if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Errorf("excessive-permissions lines = %v, want %v", got, tc.want)
			}
		})
	}

	// A workflow without "on:" or without jobs is a syntax error of its own; there is no token to restrict
	for name, bad := range map[string]string{
		"no on":   "jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n",
		"no jobs": "name: x\non: push\n",
	} {
		if got := permLines(lintWithConfig(t, withFixtureRules(&Config{Profile: ProfilePedantic}), bad), "excessive-permissions"); len(got) != 0 {
			t.Errorf("%s: a workflow with a syntax error is reported at %v", name, got)
		}
	}

	// A workflow with a top-level permissions is not reported by the option
	ok := permsLicenseHeader + "on: push\npermissions: {}\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n"
	if got := permLines(lintWithConfig(t, withFixtureRules(&Config{Profile: ProfilePedantic}), ok), "excessive-permissions"); len(got) != 0 {
		t.Errorf("a workflow with permissions is reported at %v", got)
	}
}

func TestUndocumentedPermissionsReadScopesByProfile(t *testing.T) {
	src := `on: push
permissions:
  contents: read
  issues: read
  pull-requests: read # review comments
  packages: write # publish
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	off := false
	tests := []struct {
		name string
		cfg  *Config
		want []int
	}{
		// contents: read is exempt, as in zizmor
		{"pedantic profile", &Config{Profile: ProfilePedantic}, []int{4}},
		{"rule enabled in the default profile", &Config{Profile: ProfileDefault, Rules: map[string]RuleConfig{
			"undocumented-permissions": {Level: SeverityError}}}, nil},
		{"pedantic profile, include-read off", &Config{Profile: ProfilePedantic, Rules: map[string]RuleConfig{
			"undocumented-permissions": {Level: SeverityError, Options: map[string]any{"include-read": off}}}}, nil},
		{"include-read on", &Config{Profile: ProfileDefault, Rules: map[string]RuleConfig{
			"undocumented-permissions": {Level: SeverityError, Options: map[string]any{"include-read": true}}}}, []int{4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := permLines(lintWithConfig(t, withFixtureRules(tc.cfg), src), "undocumented-permissions")
			if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Errorf("undocumented-permissions lines = %v, want %v", got, tc.want)
			}
		})
	}

	// "none" grants nothing and is never reported, although zizmor reports it
	none := "on: push\npermissions:\n  contents: none\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n"
	if got := permLines(lintWithConfig(t, withFixtureRules(&Config{Profile: ProfilePedantic}), none), "undocumented-permissions"); len(got) != 0 {
		t.Errorf("a none scope is reported at %v", got)
	}
	// A comment above the scope documents it, which zizmor does not accept
	above := strings.Replace(src, "  issues: read\n", "  # triage\n  issues: read\n", 1)
	if got := permLines(lintWithConfig(t, withFixtureRules(&Config{Profile: ProfilePedantic}), above), "undocumented-permissions"); len(got) != 0 {
		t.Errorf("a scope with a comment above is reported at %v", got)
	}
}

func TestFirstKeyLineSkipsByteOrderMark(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
		ok        bool
	}{
		{"comment after a BOM", "\xef\xbb\xbf# header\n\nname: x\non: push\n", 3, true},
		{"key after a BOM", "\xef\xbb\xbfname: x\non: push\n", 1, true},
		{"no BOM", "# header\nname: x\n", 2, true},
	} {
		got, ok := firstKeyLine([]byte(tc.src))
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: firstKeyLine = %d, %v, want %d, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
