package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ignoreProject writes a repository with the workflows and the config, and returns its root.
func ignoreProject(t *testing.T, cfg string, workflows map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	write := func(path, body string) {
		p := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range workflows {
		write(".github/workflows/"+name, body)
	}
	write(".github/jactionlint.yaml", cfg)
	return root
}

func newIgnoreLinter(t *testing.T, root string, now time.Time) (*Linter, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{
		WorkingDir:    root,
		ConfigFile:    filepath.Join(root, ".github", "jactionlint.yaml"),
		Now:           func() time.Time { return now },
		Shellcheck:    "",
		Pyflakes:      "",
		StdinFileName: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	return l, &out
}

func lintIgnoreProject(t *testing.T, root string, now time.Time) []*Error {
	t.Helper()
	l, _ := newIgnoreLinter(t, root, now)
	errs, err := l.LintRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func ofRule(errs []*Error, id string) []*Error {
	var ret []*Error
	for _, e := range errs {
		if e.ID == id {
			ret = append(ret, e)
		}
	}
	return ret
}

const ignoreWorkflow = `name: ci
on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        id: node
        name: Set up node
  lint:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@v4
`

const ignoreConfigHead = "profile: strict\n"

func unpinnedAt(errs []*Error) []int {
	var lines []int
	for _, e := range ofRule(errs, "unpinned-uses") {
		lines = append(lines, e.Line)
	}
	return lines
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var fixedNow = time.Date(2026, 10, 8, 15, 30, 0, 0, time.UTC)

func TestConfigIgnoreMatching(t *testing.T) {
	tests := []struct {
		name    string
		ignores string
		want    []int // lines of the remaining unpinned-uses errors
	}{
		{"none", "", []int{9, 10, 17}},
		{"uses name", "  - {rule: unpinned-uses, uses: actions/checkout}\n", []int{10}},
		{"uses name with ref", "  - {rule: unpinned-uses, uses: 'actions/checkout@v4'}\n", []int{10}},
		{"uses name with other ref", "  - {rule: unpinned-uses, uses: 'actions/checkout@v3'}\n", []int{9, 10, 17}},
		{"uses owner glob", "  - {rule: unpinned-uses, uses: 'actions/*'}\n", nil},
		{"uses case-insensitive", "  - {rule: unpinned-uses, uses: Actions/Checkout}\n", []int{10}},
		{"uses regex", "  - {rule: unpinned-uses, uses: '/^actions\\/(checkout|setup-node)@/'}\n", nil},
		{"uses regex not anchored by default semantics", "  - {rule: unpinned-uses, uses: '/^checkout/'}\n", []int{9, 10, 17}},
		{"job", "  - {rule: unpinned-uses, job: lint}\n", []int{9, 10}},
		{"job and uses", "  - {rule: unpinned-uses, job: build, uses: actions/checkout}\n", []int{10, 17}},
		{"step id", "  - {rule: unpinned-uses, step: node}\n", []int{9, 17}},
		{"step name", "  - {rule: unpinned-uses, step: Set up node}\n", []int{9, 17}},
		{"step unknown", "  - {rule: unpinned-uses, step: nothing}\n", []int{9, 10, 17}},
		{"rule list", "  - {rule: [missing-permissions, unpinned-uses], job: build}\n", []int{17}},
		{"other rule", "  - {rule: missing-timeout, uses: actions/checkout}\n", []int{9, 10, 17}},
		{"file exact", "  - {rule: unpinned-uses, file: .github/workflows/ci.yaml}\n", nil},
		{"file glob", "  - {rule: unpinned-uses, file: '.github/workflows/*.yaml'}\n", nil},
		{"file doublestar", "  - {rule: unpinned-uses, file: '**/ci.yaml'}\n", nil},
		{"file other", "  - {rule: unpinned-uses, file: '.github/workflows/other.yaml'}\n", []int{9, 10, 17}},
		{"file and job", "  - {rule: unpinned-uses, file: '**/ci.yaml', job: lint}\n", []int{9, 10}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ignoreConfigHead
			if tc.ignores != "" {
				cfg += "ignores:\n" + tc.ignores
			}
			// Entries which can never match are reported as unused; this test is about what remains
			cfg += "rules:\n  unused-ignore: off\n"
			root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
			got := unpinnedAt(lintIgnoreProject(t, root, fixedNow))
			if !sameInts(got, tc.want) {
				t.Errorf("remaining unpinned-uses lines: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConfigIgnoreExpiry(t *testing.T) {
	tests := []struct {
		name     string
		expires  string
		wantLive bool   // the ignore still suppresses
		wantMsg  string // message of the expired-ignore error, or empty
		wantSev  Severity
	}{
		{"far future", "2027-01-31", true, "", 0},
		{"in 14 days", "2026-10-22", true, "expires on 2026-10-22 (in 14 days)", SeverityInfo},
		{"in 15 days", "2026-10-23", true, "", 0},
		{"today", "2026-10-08", true, "expires on 2026-10-08 (in 0 days)", SeverityInfo},
		{"yesterday", "2026-10-07", false, "expired on 2026-10-07", SeverityError},
		{"long ago", "2020-01-01", false, "expired on 2020-01-01", SeverityError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ignoreConfigHead + "ignores:\n  - rule: unpinned-uses\n    uses: actions/checkout\n    reason: org ruleset pins it\n    expires: " + tc.expires + "\n"
			root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
			errs := lintIgnoreProject(t, root, fixedNow)
			if got := len(ofRule(errs, "unpinned-uses")) == 1; got != tc.wantLive {
				t.Errorf("suppressing: got %v, want %v: %v", got, tc.wantLive, unpinnedAt(errs))
			}
			exp := ofRule(errs, "expired-ignore")
			if tc.wantMsg == "" {
				if len(exp) != 0 {
					t.Fatalf("unexpected expired-ignore: %v", exp)
				}
				return
			}
			if len(exp) != 1 {
				t.Fatalf("want one expired-ignore, got %v", exp)
			}
			e := exp[0]
			if !strings.Contains(e.Message, tc.wantMsg) || !strings.Contains(e.Message, "reason: org ruleset pins it") {
				t.Errorf("message %q lacks %q", e.Message, tc.wantMsg)
			}
			if e.Severity != tc.wantSev {
				t.Errorf("severity: got %v, want %v", e.Severity, tc.wantSev)
			}
			if !strings.HasSuffix(filepath.ToSlash(e.Filepath), ".github/jactionlint.yaml") || e.Line != 3 {
				t.Errorf("location: got %s:%d, want the config entry at line 3", e.Filepath, e.Line)
			}
		})
	}
}

func TestConfigIgnoreExpiryClockIsUTC(t *testing.T) {
	// 23:30 on the 7th in New York is already the 8th in UTC
	ny := time.FixedZone("EST", -5*3600)
	now := time.Date(2026, 10, 7, 23, 30, 0, 0, ny)
	ig := ConfigIgnore{expires: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)}
	if !ig.expired(now) {
		t.Error("the ignore should have expired at 2026-10-08 UTC")
	}
	if ig.expired(time.Date(2026, 10, 7, 23, 59, 0, 0, time.UTC)) {
		t.Error("the ignore is valid through the whole last day")
	}
}

func TestConfigIgnoreExpiredRuleCanBeTurnedOff(t *testing.T) {
	cfg := ignoreConfigHead + "rules:\n  expired-ignore: off\nignores:\n  - {rule: unpinned-uses, uses: actions/checkout, expires: 2020-01-01}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	errs := lintIgnoreProject(t, root, fixedNow)
	if len(ofRule(errs, "expired-ignore")) != 0 {
		t.Errorf("rule is off: %v", errs)
	}
	if len(ofRule(errs, "unpinned-uses")) != 3 {
		t.Errorf("an expired entry must not suppress: %v", unpinnedAt(errs))
	}
}

func TestConfigIgnoreUnused(t *testing.T) {
	cfg := ignoreConfigHead + `ignores:
  - {rule: unpinned-uses, uses: actions/checkout}
  - {rule: unpinned-uses, uses: actions/cache, reason: cache is gone}
  - {rule: unpinned-uses, file: .github/workflows/deleted.yaml}
`
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})

	errs := lintIgnoreProject(t, root, fixedNow)
	unused := ofRule(errs, "unused-ignore")
	if len(unused) != 2 {
		t.Fatalf("want two unused entries, got %v", unused)
	}
	if unused[0].Line != 4 || unused[1].Line != 5 {
		t.Errorf("lines: got %d and %d, want 4 and 5", unused[0].Line, unused[1].Line)
	}
	if !strings.Contains(unused[0].Message, `uses "actions/cache"`) || !strings.Contains(unused[0].Message, "reason: cache is gone") {
		t.Errorf("message: %s", unused[0].Message)
	}
	if !strings.HasSuffix(filepath.ToSlash(unused[0].Filepath), ".github/jactionlint.yaml") {
		t.Errorf("path: %s", unused[0].Filepath)
	}
	if unused[0].Severity != SeverityError {
		t.Errorf("severity: %v", unused[0].Severity)
	}

	// The default profile does not enable unused-ignore
	cfg = strings.Replace(cfg, "profile: strict", "profile: default", 1)
	root = ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	if got := ofRule(lintIgnoreProject(t, root, fixedNow), "unused-ignore"); len(got) != 0 {
		t.Errorf("unused-ignore is a strict rule: %v", got)
	}
}

func TestConfigIgnoreUnusedNeedsAWholeRun(t *testing.T) {
	// A pre-commit hook lints only the changed file. An entry which matched nothing in that file may be
	// needed by another one, so it is not reported.
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/cache}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow, "other.yaml": ignoreWorkflow})
	l, _ := newIgnoreLinter(t, root, fixedNow)
	errs, err := l.LintFile(filepath.Join(root, ".github", "workflows", "ci.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
		t.Errorf("one file does not tell: %v", got)
	}
	// An entry scoped to the linted file by glob is judged by that file alone
	cfg = ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/cache, file: '**/ci.yaml'}\n"
	root = ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow, "other.yaml": ignoreWorkflow})
	l, _ = newIgnoreLinter(t, root, fixedNow)
	errs, err = l.LintFile(filepath.Join(root, ".github", "workflows", "ci.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ofRule(errs, "unused-ignore"); len(got) != 1 {
		t.Errorf("the only file the entry applies to was linted: %v", errs)
	}
	// Linting a file the glob does not match says nothing about the entry
	l, _ = newIgnoreLinter(t, root, fixedNow)
	errs, err = l.LintFile(filepath.Join(root, ".github", "workflows", "other.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
		t.Errorf("ci.yaml was not linted: %v", got)
	}
}

func TestConfigIgnoreAlongsideInline(t *testing.T) {
	// An error suppressed by an inline comment is still seen by the config entry, so neither is "unused"
	src := strings.Replace(ignoreWorkflow, "      - uses: actions/checkout@v4\n      - uses: actions/setup-node", "      - uses: actions/checkout@v4 # jactionlint ignore=unpinned-uses\n      - uses: actions/setup-node", 1)
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/checkout, job: build}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": src})
	errs := lintIgnoreProject(t, root, fixedNow)
	if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
		t.Errorf("both ignores suppress the same error: %v", got)
	}
}

// TestConfigIgnoreSurvivesRenovate simulates what Renovate does to a pinned action. The line is
// rewritten, so the trailing ignore comment is gone, but the config entry keeps working.
func TestConfigIgnoreSurvivesRenovate(t *testing.T) {
	before := `on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@93cb6efe18208431cddfb8368fd83d5badbf9bfd # v5.0.1 # jactionlint ignore=forbidden-uses
`
	// Renovate bumps the SHA and the version comment and does not know about the ignore
	after := strings.Replace(before, "93cb6efe18208431cddfb8368fd83d5badbf9bfd # v5.0.1 # jactionlint ignore=forbidden-uses", "08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0", 1)
	if after == before {
		t.Fatal("test setup")
	}
	const rules = "rules:\n  forbidden-uses: {deny: [actions/checkout]}\n"

	// Inline: works before, breaks after
	inline := func(src string) int {
		root := ignoreProject(t, ignoreConfigHead+rules, map[string]string{"ci.yaml": src})
		return len(ofRule(lintIgnoreProject(t, root, fixedNow), "forbidden-uses"))
	}
	if n := inline(before); n != 0 {
		t.Fatalf("the trailing comment should ignore the finding: %d", n)
	}
	if n := inline(after); n != 1 {
		t.Fatalf("the rewrite dropped the comment, so the finding is back: %d", n)
	}

	// Config entry: works before and after
	cfg := ignoreConfigHead + rules + "ignores:\n  - rule: forbidden-uses\n    uses: actions/checkout\n    job: build\n    reason: allowed for now\n"
	for name, src := range map[string]string{"before": before, "after": after} {
		root := ignoreProject(t, cfg, map[string]string{"ci.yaml": src})
		errs := lintIgnoreProject(t, root, fixedNow)
		if got := ofRule(errs, "forbidden-uses"); len(got) != 0 {
			t.Errorf("%s: the config entry must keep ignoring: %v", name, got)
		}
		if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
			t.Errorf("%s: unexpected unused-ignore: %v", name, got)
		}
	}
}

func TestConfigIgnoreScopeIsNeverGuessed(t *testing.T) {
	// Flow style puts two steps on one line, so the step cannot be told. Entries which ask for a step or
	// an action must not match, rather than suppress something they were not written for.
	src := `on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps: [{uses: 'actions/checkout@v4'}, {uses: 'actions/cache@v4'}]
`
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/checkout}\n  - {rule: unpinned-uses, step: x}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": src})
	if got := ofRule(lintIgnoreProject(t, root, fixedNow), "unpinned-uses"); len(got) != 2 {
		t.Errorf("both findings must remain: %v", got)
	}
	// An entry about the job alone is still accurate
	cfg = ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, job: build}\n"
	root = ignoreProject(t, cfg, map[string]string{"ci.yaml": src})
	if got := ofRule(lintIgnoreProject(t, root, fixedNow), "unpinned-uses"); len(got) != 0 {
		t.Errorf("the job covers both: %v", got)
	}
}

func TestConfigIgnoreUsesOfNonRepositoryValues(t *testing.T) {
	src := `on: push
permissions: {}
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: docker://alpine:3.20
      - uses: docker://busybox
`
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: 'docker://alpine*'}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": src})
	errs := lintIgnoreProject(t, root, fixedNow)
	got := ofRule(errs, "unpinned-uses")
	if len(got) != 1 || !strings.Contains(got[0].Message, "busybox") {
		t.Errorf("only the busybox finding should remain: %v", got)
	}
}

func TestConfigIgnoreExtendsConcatenates(t *testing.T) {
	root := ignoreProject(t, ignoreConfigHead+"extends: [shared.yaml]\nignores:\n  - {rule: unpinned-uses, job: lint}\n", map[string]string{"ci.yaml": ignoreWorkflow})
	shared := "ignores:\n  - {rule: unpinned-uses, uses: actions/setup-node, expires: 2020-01-01}\n"
	if err := os.WriteFile(filepath.Join(root, ".github", "shared.yaml"), []byte(shared), 0o644); err != nil {
		t.Fatal(err)
	}
	errs := lintIgnoreProject(t, root, fixedNow)
	if got := unpinnedAt(errs); !sameInts(got, []int{9, 10}) {
		t.Errorf("remaining: %v", got)
	}
	exp := ofRule(errs, "expired-ignore")
	if len(exp) != 1 || !strings.HasSuffix(filepath.ToSlash(exp[0].Filepath), ".github/shared.yaml") || exp[0].Line != 2 {
		t.Errorf("the expired entry is reported in the file that has it: %v", exp)
	}
}

func TestConfigIgnoreOutput(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/checkout, expires: 2020-01-01}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	l, out := newIgnoreLinter(t, root, fixedNow)
	if _, err := l.LintRepository(root); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, filepath.Join(".github", "jactionlint.yaml")+":3:5: the ignore for") {
		t.Errorf("the config error is printed with its location:\n%s", s)
	}
}

func TestParseConfigIgnores(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want string // substring of the error, empty for success
	}{
		{"ok", "ignores:\n  - {rule: unpinned-uses, uses: actions/checkout}\n", ""},
		{"all fields", "ignores:\n  - rule: [unpinned-uses, missing-timeout]\n    file: '**/*.yml'\n    uses: 'actions/*'\n    job: a\n    step: b\n    reason: r\n    expires: 2027-02-28\n", ""},
		{"no rule", "ignores:\n  - {uses: actions/checkout}\n", `"rule" is required`},
		{"unknown rule", "ignores:\n  - {rule: unpined-uses, uses: x}\n", `unknown rule ID "unpined-uses"`},
		{"no scope", "ignores:\n  - {rule: unpinned-uses}\n", `needs at least one of "file", "uses", "job" and "step"`},
		{"unknown key", "ignores:\n  - {rule: unpinned-uses, use: x}\n", `unknown key "use"`},
		{"bad date", "ignores:\n  - {rule: unpinned-uses, uses: x, expires: 31/12/2026}\n", `invalid date "31/12/2026"`},
		{"impossible date", "ignores:\n  - {rule: unpinned-uses, uses: x, expires: 2026-02-31}\n", `invalid date "2026-02-31"`},
		{"bad glob", "ignores:\n  - {rule: unpinned-uses, file: '[a'}\n", `invalid glob pattern`},
		{"bad regex", "ignores:\n  - {rule: unpinned-uses, uses: '/(/'}\n", `invalid regular expression`},
		{"not a list", "ignores: x\n", ""}, // reported by the decoder
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.cfg))
			switch {
			case tc.name == "not a list":
				if err == nil {
					t.Error("want an error")
				}
			case tc.want == "" && err != nil:
				t.Error(err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestConfigIgnoreSnippet(t *testing.T) {
	ig := ConfigIgnore{Rules: []string{"unpinned-uses"}, File: ".github/workflows/ci.yaml", Uses: "actions/checkout", Reason: "needs: a colon", Expires: "2027-01-01"}
	got := ig.Snippet()
	want := "- rule: unpinned-uses\n  file: .github/workflows/ci.yaml\n  uses: actions/checkout\n  reason: 'needs: a colon'\n  expires: \"2027-01-01\"\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// The snippet reads back as the same entry
	cfg, err := ParseConfig([]byte("ignores:\n" + indentLines(got, "  ")))
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Ignores[0]
	if r.Reason != ig.Reason || r.Expires != ig.Expires || r.Uses != ig.Uses || r.File != ig.File || len(r.Rules) != 1 {
		t.Errorf("round trip: %+v", r)
	}
	multi := ConfigIgnore{Rules: []string{"a", "b"}, Job: "x"}.Snippet()
	if multi != "- rule: [a, b]\n  job: x\n" {
		t.Errorf("multi: %q", multi)
	}
}

func indentLines(s, prefix string) string {
	lines := strings.SplitAfter(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "")
}

// The last step ends where its own block ends: the keys of the job that follow "steps" are not in it.
func TestScopeOfTheLastStepStopsBeforeTheKeysOfTheJob(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - name: last\n        uses: actions/setup-node@v4\n        with:\n          node-version: 20\n    timeout-minutes: 5\n    env:\n      X: 1\n"
	w, errs := Parse([]byte(src))
	if w == nil || len(errs) > 0 {
		t.Fatal(errs)
	}
	idx := newScopeIndex(w, []byte(src))
	for line, want := range map[int]bool{6: true, 7: true, 8: true, 10: true, 11: false, 12: false, 13: false} {
		sc := idx.scopeAt(line)
		if line > 6 && sc.hasStep != want {
			t.Errorf("line %d: in a step = %v, want %v", line, sc.hasStep, want)
		}
		if sc.job != "a" {
			t.Errorf("line %d is in the job a: %q", line, sc.job)
		}
	}
	// A step whose first key sits below a "-" alone
	src2 := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      -\n        uses: actions/checkout@v4\n        with:\n          x: 1\n    timeout-minutes: 5\n"
	w, _ = Parse([]byte(src2))
	idx = newScopeIndex(w, []byte(src2))
	if !idx.scopeAt(9).hasStep || idx.scopeAt(10).hasStep {
		t.Errorf("the step ends before timeout-minutes: %+v %+v", idx.scopeAt(9), idx.scopeAt(10))
	}
}

// An entry without a file can apply to an action.yml: it is unused only when the actions were linted too.
func TestConfigIgnoreUnusedCountsTheActionFiles(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/cache}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	dir := filepath.Join(root, ".github", "actions", "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	action := "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - uses: actions/cache@v4\n"
	if err := os.WriteFile(filepath.Join(dir, "action.yml"), []byte(action), 0o644); err != nil {
		t.Fatal(err)
	}
	l, _ := newIgnoreLinter(t, root, fixedNow)
	errs, err := l.LintFile(filepath.Join(root, ".github", "workflows", "ci.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
		t.Errorf("the action was not linted, so the entry may be needed there: %v", got)
	}
	// A whole run lints the action, which the entry suppresses
	if got := ofRule(lintIgnoreProject(t, root, fixedNow), "unused-ignore"); len(got) != 0 {
		t.Errorf("the entry suppresses the cache in the action: %v", got)
	}
}

// An exact docker image or local path matches as it is written.
func TestConfigIgnoreExactDockerImage(t *testing.T) {
	wf := "name: ci\non: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - uses: docker://alpine:3.20\n"
	root := ignoreProject(t, ignoreConfigHead, map[string]string{"ci.yaml": wf})
	if got := unpinnedAt(lintIgnoreProject(t, root, fixedNow)); len(got) != 1 {
		t.Fatalf("setup: the image is not pinned: %v", got)
	}
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: 'docker://alpine:3.20'}\n"
	root = ignoreProject(t, cfg, map[string]string{"ci.yaml": wf})
	errs := lintIgnoreProject(t, root, fixedNow)
	if got := unpinnedAt(errs); len(got) != 0 {
		t.Errorf("the exact image is ignored: %v", got)
	}
	if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
		t.Errorf("and the entry is used: %v", got)
	}
}

// An entry for a rule that did not run cannot have matched anything, so it is not unused.
func TestConfigIgnoreForARuleThatDoesNotRunIsNotUnused(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: stale-action-refs, uses: actions/checkout}\n  - {rule: missing-permissions, uses: actions/cache}\nrules:\n  missing-permissions: off\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	if got := ofRule(lintIgnoreProject(t, root, fixedNow), "unused-ignore"); len(got) != 0 {
		t.Errorf("an online rule offline and a rule that is off: %v", got)
	}
}

// A repository reference is judged by the pattern alone: the case of a ref matters.
func TestConfigIgnoreRefCaseOfARepositoryReference(t *testing.T) {
	ig := &ConfigIgnore{Rules: []string{"unpinned-uses"}, Uses: "actions/checkout@v4"}
	if err := ig.validate(); err != nil {
		t.Fatal(err)
	}
	if ig.matchUses("actions/checkout@V4") {
		t.Error("@V4 is not @v4")
	}
	if !ig.matchUses("actions/checkout@v4") {
		t.Error("@v4 is @v4")
	}
}

// An action-only repository has no workflows directory, and its unused entries are still judged.
func TestConfigIgnoreUnusedInAnActionOnlyRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: actions/cache}\n"
	if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	action := "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - run: echo hi\n      shell: bash\n"
	if err := os.WriteFile(filepath.Join(root, "action.yml"), []byte(action), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ofRule(lintIgnoreProject(t, root, fixedNow), "unused-ignore"); len(got) != 1 {
		t.Errorf("want the unused entry: %v", got)
	}
}
