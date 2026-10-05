package actionlint

import (
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNewRuleRequiredActions(t *testing.T) {
	tests := []struct {
		name     string
		required []RequiredActionRule
		want     *RuleRequiredActions
	}{
		{
			name:     "nil when no rules",
			required: []RequiredActionRule{},
			want:     nil,
		},
		{
			name: "creates rule with requirements",
			required: []RequiredActionRule{
				{Action: "actions/checkout", Version: "v3"},
			},
			want: &RuleRequiredActions{
				RuleBase: RuleBase{
					name: "required-actions",
					desc: "Checks that required GitHub Actions are used in workflows",
				},
				required: []RequiredActionRule{
					{Action: "actions/checkout", Version: "v3"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewRuleRequiredActions(tt.required)
			if diff := cmp.Diff(got, tt.want,
				cmpopts.IgnoreUnexported(RuleBase{}, RuleRequiredActions{})); diff != "" {
				t.Errorf("NewRuleRequiredActions mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestParseActionRef(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
	}{
		{
			name:        "valid action reference",
			input:       "actions/checkout@v3",
			wantName:    "actions/checkout",
			wantVersion: "v3",
		},
		{
			name:        "empty string",
			input:       "",
			wantName:    "",
			wantVersion: "",
		},
		{
			name:        "docker reference",
			input:       "docker://alpine:latest",
			wantName:    "",
			wantVersion: "",
		},
		{name: "local action", input: "./.github/actions/foo@v1"},
		{name: "empty version", input: "actions/checkout@"},
		{name: "empty name", input: "/foo@v1"},
		{name: "subpath", input: "github/codeql-action/init@v3", wantName: "github/codeql-action/init", wantVersion: "v3"},
		{name: "at sign in version", input: "a/b@c@d", wantName: "a/b", wantVersion: "c@d"},
		{
			name:        "no version",
			input:       "actions/checkout",
			wantName:    "",
			wantVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion := parseActionRef(tt.input)
			if gotName != tt.wantName || gotVersion != tt.wantVersion {
				t.Errorf("parseActionRef(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotName, gotVersion, tt.wantName, tt.wantVersion)
			}
		})
	}
}

func TestRuleRequiredActions(t *testing.T) {
	tests := []struct {
		name        string
		required    []RequiredActionRule
		workflow    *Workflow
		wantNilRule bool
		wantErrs    int
		wantMsg     string
	}{
		{
			name:        "nil workflow",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}},
			workflow:    nil,
			wantNilRule: false,
			wantErrs:    0,
		},
		{
			name:        "empty workflow",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}},
			workflow:    &Workflow{},
			wantNilRule: false,
			wantErrs:    1,
			wantMsg:     `:1:1: required action "actions/checkout" (version "v3") is not used in this workflow [required-actions]`,
		},
		{
			name:        "NoRequiredActions",
			required:    []RequiredActionRule{},
			workflow:    &Workflow{},
			wantNilRule: true,
			wantErrs:    0,
		},
		{
			name:        "SingleRequiredAction_Present",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/checkout@v3"}}}}}}},
			wantNilRule: false,
			wantErrs:    0,
		},
		{
			name:        "SingleRequiredAction_Missing_With_Version",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/setup-node@v2"}}}}}}},
			wantNilRule: false,
			wantErrs:    1,
			wantMsg:     `:1:1: required action "actions/checkout" (version "v3") is not used in this workflow [required-actions]`,
		},
		{
			name:        "SingleRequiredAction_Missing_Without_Version",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: ""}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/setup-node@v2"}}}}}}},
			wantNilRule: false,
			wantErrs:    1,
			wantMsg:     `:1:1: required action "actions/checkout" is not used in this workflow [required-actions]`,
		},
		{
			name:        "SingleRequiredAction_WrongVersion",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/checkout@v2"}}}}}}},
			wantNilRule: false,
			wantErrs:    1,
			wantMsg:     `:1:1: action "actions/checkout" must use version "v3" but found "v2" [required-actions]`,
		},
		{
			name:        "MultipleRequiredActions_Present",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}, {Action: "actions/setup-node", Version: "v2"}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/checkout@v3"}}}, {Exec: &ExecAction{Uses: &String{Value: "actions/setup-node@v2"}}}}}}},
			wantNilRule: false,
			wantErrs:    0,
		},
		{
			name:        "MultipleRequiredActions_MissingOne",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}, {Action: "actions/setup-node", Version: "v2"}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/checkout@v3"}}}}}}},
			wantNilRule: false,
			wantErrs:    1,
			wantMsg:     `:1:1: required action "actions/setup-node" (version "v2") is not used in this workflow [required-actions]`,
		},
		{
			name:        "MultipleRequiredActions_WrongVersion",
			required:    []RequiredActionRule{{Action: "actions/checkout", Version: "v3"}, {Action: "actions/setup-node", Version: "v2"}},
			workflow:    &Workflow{Jobs: map[string]*Job{"build": {Steps: []*Step{{Exec: &ExecAction{Uses: &String{Value: "actions/checkout@v2"}}}, {Exec: &ExecAction{Uses: &String{Value: "actions/setup-node@v2"}}}}}}},
			wantNilRule: false,
			wantErrs:    1,
			wantMsg:     `:1:1: action "actions/checkout" must use version "v3" but found "v2" [required-actions]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := NewRuleRequiredActions(tt.required)

			if tt.wantNilRule {
				if rule != nil {
					t.Fatal("Expected nil rule")
				}
				return
			}

			if rule == nil {
				t.Fatal("Expected non-nil rule")
			}

			rule.VisitWorkflowPre(tt.workflow)
			errs := rule.Errs()
			if len(errs) != tt.wantErrs {
				t.Errorf("got %d errors, want %d", len(errs), tt.wantErrs)
			}
			if tt.wantMsg != "" && len(errs) > 0 && errs[0].Error() != tt.wantMsg {
				t.Errorf("error message mismatch\ngot:  %q\nwant: %q", errs[0].Error(), tt.wantMsg)
			}
		})
	}
}

func lintRequired(t *testing.T, src string, req []RequiredActionRule) []string {
	t.Helper()
	w, errs := Parse([]byte(src))
	if len(errs) > 0 || w == nil {
		t.Fatalf("parse error: %v", errs)
	}
	r := NewRuleRequiredActions(req)
	if err := r.VisitWorkflowPre(w); err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, e := range r.Errs() {
		msgs = append(msgs, e.Error())
	}
	return msgs
}

func TestRuleRequiredActionsParsedWorkflow(t *testing.T) {
	src := `on: push
jobs:
  zzz:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: Actions/Setup-Node@v2
      - uses: ./local
      - uses: docker://alpine:3
  aaa:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: github/codeql-action/init@v3
  call:
    uses: org/repo/.github/workflows/ci.yml@main
`
	tests := []struct {
		name string
		req  []RequiredActionRule
		want []string
	}{
		{"any matching use satisfies", []RequiredActionRule{{Action: "actions/checkout", Version: "v4"}}, nil},
		{"no version constraint", []RequiredActionRule{{Action: "actions/checkout"}}, nil},
		{"case insensitive", []RequiredActionRule{{Action: "actions/setup-node", Version: "v2"}}, nil},
		{"subpath", []RequiredActionRule{{Action: "github/codeql-action/init", Version: "v3"}}, nil},
		{"base repo does not match subpath", []RequiredActionRule{{Action: "github/codeql-action"}}, []string{`:3:3: required action "github/codeql-action" is not used in this workflow [required-actions]`}},
		{"reusable workflow", []RequiredActionRule{{Action: "org/repo/.github/workflows/ci.yml", Version: "main"}}, nil},
		{"mismatch lists all versions", []RequiredActionRule{{Action: "actions/checkout", Version: "v2"}}, []string{`:3:3: action "actions/checkout" must use version "v2" but found "v3", "v4" [required-actions]`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := lintRequired(t, src, tc.req)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestRuleRequiredActionsPosIsDeterministic(t *testing.T) {
	src := "on: push\njobs:\n  b:\n    runs-on: x\n    steps:\n      - run: a\n  a:\n    runs-on: x\n    steps:\n      - run: a\n"
	for i := 0; i < 20; i++ {
		got := lintRequired(t, src, []RequiredActionRule{{Action: "a/b"}})
		if len(got) != 1 || !strings.HasPrefix(got[0], ":3:3:") {
			t.Fatalf("unexpected: %v", got)
		}
	}
}

func TestParseConfigRequiredActions(t *testing.T) {
	c, err := ParseConfig([]byte("required-actions:\n  - action: actions/checkout\n    version: v4\n  - action: a/b\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []RequiredActionRule{{Action: "actions/checkout", Version: "v4"}, {Action: "a/b"}}
	if diff := cmp.Diff(want, c.RequiredActions); diff != "" {
		t.Fatal(diff)
	}
	for _, bad := range []string{
		"required-actions:\n  - version: v1\n",
		"required-actions:\n  - action: checkout\n",
		"required-actions:\n  - action: actions/checkout@v4\n",
		"required-actions:\n  - action: ./foo\n",
	} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestLinterRequiredActionsOptIn(t *testing.T) {
	src := []byte("on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	run := func(cfg *Config) int {
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = cfg
		errs, err := l.Lint("a.yaml", src, nil)
		if err != nil {
			t.Fatal(err)
		}
		return len(errs)
	}
	if n := run(&Config{}); n != 0 {
		t.Errorf("rule must be opt-in, got %d errors", n)
	}
	if n := run(&Config{RequiredActions: []RequiredActionRule{{Action: "actions/checkout"}}}); n != 1 {
		t.Errorf("expected 1 error, got %d", n)
	}
}
