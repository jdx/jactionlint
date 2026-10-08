package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNormalizePath(t *testing.T) {
	tests := []struct{ dir, in, want string }{
		{"/r", "./.github/workflows/a.yml", ".github/workflows/a.yml"},
		{"/r", "/r/.github/workflows/a.yml", ".github/workflows/a.yml"},
		{"/r/", "/r/.github/workflows/a.yml", ".github/workflows/a.yml"},
		{"/r", ".github\\workflows\\a.yml", ".github/workflows/a.yml"},
		{"/r", "a/../.github/workflows/a.yml", ".github/workflows/a.yml"},
	}
	for _, tc := range tests {
		if got := normalizePath(tc.dir, tc.in); got != tc.want {
			t.Errorf("normalizePath(%q, %q) = %q, want %q", tc.dir, tc.in, got, tc.want)
		}
	}
}

func TestIsWorkflowFile(t *testing.T) {
	for file, want := range map[string]bool{
		".github/workflows/a.yml":      true,
		".github/workflows/a.yaml":     true,
		".github/workflows/a.md":       false,
		".github/workflows/sub/a.yml":  false,
		"action.yml":                   false,
		".github/dependabot.yml":       false,
		"x/.github/workflows/a.yml":    false,
		".github/actions/x/action.yml": false,
	} {
		if got := isWorkflowFile(file); got != want {
			t.Errorf("isWorkflowFile(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestParseJactionlint(t *testing.T) {
	got, err := parseJactionlint("r", "/r", readFixture(t, "jactionlint.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, f := range got {
		rules = append(rules, fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Rule))
	}
	want := []string{
		".github/workflows/ci.yml:12:expression",
		".github/workflows/ci.yml:8:require-permissions",
		".github/workflows/ci.yml:9:runner-label",
	}
	if diff := cmp.Diff(want, rules); diff != "" {
		t.Error(diff)
	}
	if got[0].Repo != "r" || !strings.Contains(got[0].Message, "untrusted") {
		t.Errorf("unexpected finding: %+v", got[0])
	}
}

func TestParseJactionlintPrefersID(t *testing.T) {
	got, err := parseJactionlint("r", "/r", []byte(`[{"message":"m","filepath":"a.yml","line":1,"kind":"expression","id":"template-injection"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Rule != "template-injection" {
		t.Errorf("rule = %q, want the stable ID", got[0].Rule)
	}
}

func TestParseJactionlintEmpty(t *testing.T) {
	for _, in := range []string{"", "\n", "[]"} {
		got, err := parseJactionlint("r", "/r", []byte(in))
		if err != nil || len(got) != 0 {
			t.Errorf("parseJactionlint(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := parseJactionlint("r", "/r", []byte("not json")); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestParseZizmor(t *testing.T) {
	got, version, err := parseZizmor("r", "/r", readFixture(t, "zizmor.sarif"))
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.30.1" {
		t.Errorf("version = %q", version)
	}
	var s []string
	for _, f := range got {
		s = append(s, fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Rule))
	}
	want := []string{
		// The verbatim path wins over the URI, which is relative to the git root.
		".github/workflows/ci.yml:12:template-injection",
		".github/workflows/ci.yml:8:excessive-permissions",
		".github/workflows/other.yml:3:excessive-permissions",
		".github/workflows/ci.yml:1:future-audit",
		".github/dependabot.yml:4:dependabot-cooldown",
	}
	if diff := cmp.Diff(want, s); diff != "" {
		t.Error(diff)
	}
}

func TestParseZizmorErrors(t *testing.T) {
	for _, in := range []string{"", "{", `{"runs":[]}`} {
		if _, _, err := parseZizmor("r", "/r", []byte(in)); err == nil {
			t.Errorf("parseZizmor(%q) succeeded", in)
		}
	}
}

func TestDefaultMapping(t *testing.T) {
	m, err := loadMapping("")
	if err != nil {
		t.Fatal(err)
	}
	if m.Zizmor != "1.30.1" {
		t.Errorf("mapping targets zizmor %q", m.Zizmor)
	}
	for _, name := range []string{
		"template-injection", "hardcoded-container-credentials", "excessive-permissions",
		"unpinned-uses", "unpinned-images", "unsound-ternary", "forbidden-uses",
	} {
		if _, ok := m.Audits[name]; !ok {
			t.Errorf("audit %q is not mapped", name)
		}
	}
	if m.Audits["hardcoded-container-credentials"].Coverage != "full" {
		t.Error("hardcoded-container-credentials should be full")
	}
}

func TestParseMappingErrors(t *testing.T) {
	for name, in := range map[string]string{
		"bad json":    `{`,
		"no coverage": `{"audits":{"a":{"jactionlint":[{"rule":"x"}]}}}`,
		"no rules":    `{"audits":{"a":{"coverage":"full","jactionlint":[]}}}`,
		"empty rule":  `{"audits":{"a":{"coverage":"full","jactionlint":[{}]}}}`,
	} {
		if _, err := parseMapping([]byte(in)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestMatchMessageContains(t *testing.T) {
	a := AuditMapping{Jactionlint: []Match{{Rule: "expression", MessageContains: "untrusted"}}}
	if !a.covers(Finding{Rule: "expression", Message: "x is potentially untrusted"}) {
		t.Error("should cover")
	}
	if a.covers(Finding{Rule: "expression", Message: "type mismatch"}) {
		t.Error("message does not match")
	}
	if a.covers(Finding{Rule: "action", Message: "untrusted"}) {
		t.Error("rule does not match")
	}
}

func TestParseCorpus(t *testing.T) {
	in := `# comment

~/src/a-jactionlint
label=/abs/path   # trailing comment
~
/with=equals/dir
`
	got, err := parseCorpus(strings.NewReader(in), "/home/u")
	if err != nil {
		t.Fatal(err)
	}
	want := []Repo{
		{"a", filepath.Join("/home/u", "src", "a-jactionlint")},
		{"label", "/abs/path"},
		{"u", "/home/u"},
		{"dir", "/with=equals/dir"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error(diff)
	}
}

func testMapping(t *testing.T) *Mapping {
	t.Helper()
	m, err := loadMapping("")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func fixtureInput(t *testing.T) RepoInput {
	t.Helper()
	jl, err := parseJactionlint("r", "/r", readFixture(t, "jactionlint.json"))
	if err != nil {
		t.Fatal(err)
	}
	z, _, err := parseZizmor("r", "/r", readFixture(t, "zizmor.sarif"))
	if err != nil {
		t.Fatal(err)
	}
	return RepoInput{Name: "r", Dir: "/r", Jactionlint: jl, Zizmor: z}
}

func audit(rep *Report, name string) AuditRow {
	for _, a := range rep.Audits {
		if a.Audit == name {
			return a
		}
	}
	return AuditRow{}
}

func TestBuild(t *testing.T) {
	rep := Build([]RepoInput{fixtureInput(t)}, testMapping(t), 0)

	ti := audit(rep, "template-injection")
	if ti.Zizmor != 1 || ti.Matched != 1 || ti.Missed() != 0 || ti.Repos != 1 {
		t.Errorf("template-injection = %+v", ti)
	}
	// ci.yml:8 is reported by both tools, other.yml:3 only by zizmor.
	ep := audit(rep, "excessive-permissions")
	if ep.Zizmor != 2 || ep.Matched != 1 || ep.Missed() != 1 {
		t.Errorf("excessive-permissions = %+v", ep)
	}

	var unmapped []string
	for _, a := range rep.Unmapped {
		unmapped = append(unmapped, a.Audit)
	}
	if diff := cmp.Diff([]string{"future-audit"}, unmapped); diff != "" {
		t.Errorf("unmapped (dependabot is out of scope): %s", diff)
	}

	if len(rep.JactionlintOnly) != 1 || rep.JactionlintOnly[0].Rule != "runner-label" || rep.JactionlintOnly[0].Count != 1 {
		t.Errorf("jactionlint only = %+v", rep.JactionlintOnly)
	}

	r := rep.Repos[0]
	if r.Zizmor != 4 || r.Jactionlint != 3 || r.Shared != 2 || r.OutOfScope != 1 {
		t.Errorf("repo = %+v", r)
	}
}

func TestBuildLineTolerance(t *testing.T) {
	in := fixtureInput(t)
	in.Zizmor = []Finding{{Repo: "r", File: ".github/workflows/ci.yml", Line: 10, Rule: "template-injection"}}
	if got := audit(Build([]RepoInput{in}, testMapping(t), 0), "template-injection").Matched; got != 0 {
		t.Errorf("tolerance 0 matched %d", got)
	}
	if got := audit(Build([]RepoInput{in}, testMapping(t), 2), "template-injection").Matched; got != 1 {
		t.Errorf("tolerance 2 matched %d", got)
	}
}

func TestBuildDifferentFile(t *testing.T) {
	in := fixtureInput(t)
	in.Zizmor = []Finding{{Repo: "r", File: ".github/workflows/other.yml", Line: 12, Rule: "template-injection"}}
	if got := audit(Build([]RepoInput{in}, testMapping(t), 0), "template-injection").Matched; got != 0 {
		t.Errorf("a finding in another file matched")
	}
}

func TestBuildSkippedRepo(t *testing.T) {
	rep := Build([]RepoInput{{Name: "gone", Dir: "/gone", Skipped: "directory not found"}}, testMapping(t), 0)
	if len(rep.Repos) != 1 || rep.Repos[0].Skipped == "" {
		t.Errorf("repos = %+v", rep.Repos)
	}
}

func TestMarkdown(t *testing.T) {
	in := fixtureInput(t)
	rep := Build([]RepoInput{in, {Name: "gone", Skipped: "directory not found"}, {Name: "broken", ZizmorError: "exit 1: boom | bang"}}, testMapping(t), 0)
	rep.ZizmorVersion = "1.30.1"
	var b bytes.Buffer
	if err := rep.WriteMarkdown(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"## Mapped audits",
		"| template-injection | partial | `template-injection`, `expression` | 1 | 1 | 0 | 1 |",
		"| excessive-permissions | partial | `missing-permissions`, `require-permissions` | 2 | 1 | 1 | 1 |",
		"| **total** |",
		"## jactionlint only",
		"| runner-label | 1 | `r/.github/workflows/ci.yml:9` |",
		"## Unmapped zizmor audits",
		"| future-audit | 1 | 1 |",
		"skipped: directory not found",
		"zizmor failed: exit 1: boom \\| bang",
		"| r | 4 | 3 | 2 | 1 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
}

func TestReportJSON(t *testing.T) {
	rep := Build([]RepoInput{fixtureInput(t)}, testMapping(t), 0)
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(*rep, back); diff != "" {
		t.Error(diff)
	}
}

// fakeRunner serves the fixtures and records the command lines.
func fakeRunner(t *testing.T, zizmorCode int, zizmorOut []byte, calls *[]string) Runner {
	return func(_ context.Context, dir string, argv []string) ([]byte, []byte, int, error) {
		*calls = append(*calls, dir+": "+strings.Join(argv, " "))
		if argv[0] == "jl" {
			return readFixture(t, "jactionlint.json"), nil, 1, nil
		}
		return zizmorOut, []byte("boom"), zizmorCode, nil
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	c := &Config{
		Repos:           []Repo{{"r", dir}, {"gone", filepath.Join(dir, "nope")}},
		Jactionlint:     []string{"jl"},
		JactionlintArgs: []string{"-config-file", "/c.yaml"},
		Zizmor:          []string{"zz", "x"},
		Mapping:         testMapping(t),
		Jobs:            2,
	}
	rep := Run(context.Background(), fakeRunner(t, 14, readFixture(t, "zizmor.sarif"), &calls), c, &bytes.Buffer{})
	if rep.ZizmorVersion != "1.30.1" {
		t.Errorf("version = %q", rep.ZizmorVersion)
	}
	if rep.Repos[0].Zizmor != 4 || rep.Repos[1].Skipped == "" {
		t.Errorf("repos = %+v", rep.Repos)
	}
	want := []string{
		dir + ": jl -config-file /c.yaml -format {{json .}}",
		dir + ": zz x --offline --persona pedantic --format sarif .",
	}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Error(diff)
	}
}

func TestRunZizmorFailureIsRecorded(t *testing.T) {
	dir := t.TempDir()
	var calls []string
	c := &Config{Repos: []Repo{{"r", dir}}, Jactionlint: []string{"jl"}, Zizmor: []string{"zz"}, Mapping: testMapping(t), Jobs: 1}
	for name, tc := range map[string]struct {
		code int
		out  []byte
	}{
		"exit code": {1, nil},
		"bad json":  {0, []byte("not sarif")},
	} {
		rep := Run(context.Background(), fakeRunner(t, tc.code, tc.out, &calls), c, &bytes.Buffer{})
		r := rep.Repos[0]
		if r.ZizmorError == "" {
			t.Errorf("%s: no zizmor error recorded", name)
		}
		// A repo where one tool failed must not skew the totals.
		if r.Zizmor != 0 || r.Jactionlint != 0 {
			t.Errorf("%s: failed repo counted: %+v", name, r)
		}
	}
}
