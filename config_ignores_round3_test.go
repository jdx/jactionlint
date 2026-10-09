package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An explicit "online: false" wins over a mode of online-options, which turns the checks on only when
// "online" says nothing.
func TestExplicitOnlineFalseWinsOverMode(t *testing.T) {
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cfg  string
		want bool
	}{
		{"online-options: {mode: cache}\n", true},
		{"online: true\n", true},
		{"online: false\nonline-options: {mode: strict}\n", false},
		{"online: false\n", false},
		{"profile: default\n", false},
	} {
		cfg := mustParseConfig(t, tc.cfg)
		if got := l.onlineOn(cfg); got != tc.want {
			t.Errorf("%q: online = %v, want %v", tc.cfg, got, tc.want)
		}
	}
	// The flag still wins over the configuration
	l, err = NewLinter(io.Discard, &LinterOptions{Online: true})
	if err != nil {
		t.Fatal(err)
	}
	if !l.onlineOn(mustParseConfig(t, "online: false\n")) {
		t.Error("--online turns the checks on whatever the configuration says")
	}
	// ... also through extends
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	writeTestFile(t, base, "online-options: {mode: cache}\n")
	writeTestFile(t, filepath.Join(dir, "c.yaml"), "extends: [base.yaml]\nonline: false\n")
	cfg, err := ReadConfigFile(filepath.Join(dir, "c.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	l, _ = NewLinter(io.Discard, &LinterOptions{})
	if l.onlineOn(cfg) {
		t.Error("online: false after extends still wins")
	}
}

// An inline ignore for an online rule that is enabled through online-options.mode only (no "online: true")
// is judged: the rule ran.
func TestUnusedInlineIgnoreSeesOnlineModeFromTheConfig(t *testing.T) {
	src := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      # jactionlint ignore=impostor-commit\n      - uses: actions/checkout@v4\n"
	cfg := mustParseConfig(t, "profile: pedantic\nonline-options: {mode: cache}\nrules:\n  unused-ignore: error\n  missing-timeout: off\n  stale-action-refs: off\n")
	l, err := NewLinter(io.Discard, &LinterOptions{GitHubClient: onlineFixtureClient(t)})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.Lint("test.yaml", []byte(src), &Project{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(errsWithID(errs, "unused-ignore")); got != 1 {
		t.Errorf("the rule ran and found nothing to ignore: %v", lineIDsOf(errs))
	}
}

// An ignore for a rule that could not be created (the command of shellcheck is missing) is not unused.
func TestUnusedInlineIgnoreOfRuleThatCouldNotRun(t *testing.T) {
	src := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      # jactionlint ignore=shellcheck\n      - run: echo $FOO\n"
	cfg := mustParseConfig(t, "profile: pedantic\nrules:\n  unused-ignore: error\n  missing-timeout: off\n")
	for _, tc := range []struct {
		name       string
		shellcheck string
		want       int
	}{
		{"no command", "", 0},
		{"missing command", filepath.Join(t.TempDir(), "no-such-shellcheck"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, err := NewLinter(io.Discard, &LinterOptions{Shellcheck: tc.shellcheck})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = cfg
			errs, err := l.Lint("test.yaml", []byte(src), &Project{root: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(errsWithID(errs, "unused-ignore")); got != tc.want {
				t.Errorf("want %d unused-ignore but got %v", tc.want, lineIDsOf(errs))
			}
		})
	}
}

func TestOptionIntegerTooLargeIsRejected(t *testing.T) {
	_, err := ParseConfig([]byte("rules:\n  max-run-lines:\n    max: 18446744073709551615\n"))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want a clear error for an integer which does not fit: %v", err)
	}
	if _, err := ParseConfig([]byte("rules:\n  max-run-lines:\n    max: 9223372036854775807\n")); err != nil {
		t.Errorf("the greatest int is fine: %v", err)
	}
}

const round3Parallel = `name: ci
on: push
permissions: {}
jobs:
  Build:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - parallel:
          - uses: actions/checkout@v4
          - name: second
            uses: actions/cache@v4
      - uses: actions/setup-node@v4
`

func TestConfigIgnoreJobIsCaseInsensitive(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, job: build}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": round3Parallel})
	errs := lintIgnoreProject(t, root, fixedNow)
	if got := unpinnedAt(errs); len(got) != 0 {
		t.Errorf("job IDs are case-insensitive, the entry must match Build: %v", got)
	}
	if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
		t.Errorf("the entry is used: %v", got)
	}
}

func TestConfigIgnoreFindsStepsOfParallelGroups(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ignores string
		want    []int // remaining unpinned-uses lines
	}{
		{"none", "", []int{10, 12, 13}},
		{"uses in the group", "  - {rule: unpinned-uses, uses: actions/checkout}\n", []int{12, 13}},
		{"second of the group", "  - {rule: unpinned-uses, uses: actions/cache}\n", []int{10, 13}},
		{"step name in the group", "  - {rule: unpinned-uses, step: second}\n", []int{10, 13}},
		{"the step after the group", "  - {rule: unpinned-uses, uses: actions/setup-node}\n", []int{10, 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := ignoreProject(t, ignoreConfigHead+"ignores:\n"+tc.ignores, map[string]string{"ci.yaml": round3Parallel})
			if tc.ignores == "" {
				root = ignoreProject(t, ignoreConfigHead, map[string]string{"ci.yaml": round3Parallel})
			}
			errs := lintIgnoreProject(t, root, fixedNow)
			if got := unpinnedAt(errs); !sameInts(got, tc.want) {
				t.Errorf("remaining %v, want %v (%v)", got, tc.want, lineIDsOf(errs))
			}
			if tc.ignores != "" {
				if got := ofRule(errs, "unused-ignore"); len(got) != 0 {
					t.Errorf("the entry is used: %v", got)
				}
			}
		})
	}
}

func TestConfigIgnoreLocalUsesSpellings(t *testing.T) {
	for _, tc := range []struct {
		pattern, value string
		want           bool
	}{
		{"./.github/actions/foo", "./.github/actions/foo", true},
		{"./.github/actions/foo", "$/.github/actions/foo", true},
		{"$/.github/actions/foo", "./.github/actions/foo", true},
		{"$/.github/actions/foo", "$/.github/actions/foo", true},
		{"./.github/actions/*", "$/.github/actions/foo", true},
		{"$/.github/actions/*", "./.github/actions/foo", true},
		{"$/.github/actions/foo", "./.github/actions/bar", false},
		{"/^\\.\\/\\.github\\/actions\\//", "$/.github/actions/foo", true},
		{"/^\\$\\/\\.github\\/actions\\//", "./.github/actions/foo", true},
		{"docker://alpine*", "docker://alpine:3.20", true},
	} {
		ig := ConfigIgnore{Rules: []string{"unpinned-uses"}, Uses: tc.pattern}
		if err := ig.validate(); err != nil {
			t.Fatal(err)
		}
		if got := ig.matchUses(tc.value); got != tc.want {
			t.Errorf("uses %q vs %q: %v, want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}

func TestConfigIgnoreExpiringAndUnusedAreBothReported(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: nothing/here, expires: 2026-10-15}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	errs := lintIgnoreProject(t, root, fixedNow)
	exp, unused := ofRule(errs, "expired-ignore"), ofRule(errs, "unused-ignore")
	if len(exp) != 1 || exp[0].Severity != SeverityInfo || len(unused) != 1 {
		t.Fatalf("want the expiry warning (info) and the unused entry: %v", lineIDsOf(errs))
	}
	// An expired entry suppresses nothing by definition: only the expiry is said
	cfg = ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: nothing/here, expires: 2026-10-01}\n"
	root = ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	errs = lintIgnoreProject(t, root, fixedNow)
	if len(ofRule(errs, "expired-ignore")) != 1 || len(ofRule(errs, "unused-ignore")) != 0 {
		t.Errorf("expired: %v", lineIDsOf(errs))
	}
}

func TestConfigIgnoreFindingsFollowIgnoreOption(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: nothing/here, expires: 2026-10-15}\n  - {rule: unpinned-uses, uses: nothing/else}\n"
	for _, id := range []string{"unused-ignore", "expired-ignore"} {
		root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
		var out strings.Builder
		l, err := NewLinter(&out, &LinterOptions{
			WorkingDir: root, ConfigFile: filepath.Join(root, ".github", "jactionlint.yaml"),
			Now: func() time.Time { return fixedNow }, IgnorePatterns: []string{id},
		})
		if err != nil {
			t.Fatal(err)
		}
		errs, err := l.LintRepository(root)
		if err != nil {
			t.Fatal(err)
		}
		if got := ofRule(errs, id); len(got) != 0 {
			t.Errorf("--ignore %s must filter the findings about the config: %v", id, got)
		}
		other := "expired-ignore"
		if id == other {
			other = "unused-ignore"
		}
		if len(ofRule(errs, other)) == 0 {
			t.Errorf("--ignore %s hid %s too: %v", id, other, lineIDsOf(errs))
		}
	}
}

func TestFixReportsConfigIgnoreFindings(t *testing.T) {
	cfg := ignoreConfigHead + "ignores:\n  - {rule: unpinned-uses, uses: nothing/here, expires: 2026-10-15}\n  - {rule: unpinned-uses, uses: nothing/else}\n"
	root := ignoreProject(t, cfg, map[string]string{"ci.yaml": ignoreWorkflow})
	l, _ := newIgnoreLinter(t, root, fixedNow)
	res, err := l.FixRepository(root, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if len(ofRule(res.Errors, "unused-ignore")) != 2 || len(ofRule(res.Errors, "expired-ignore")) != 1 {
		t.Errorf("--fix must report the findings about the config like a plain run: %v", lineIDsOf(res.Errors))
	}
	for _, f := range res.Fixed {
		if strings.HasSuffix(f, ".yaml") && strings.Contains(f, "jactionlint") {
			t.Errorf("the config file is not a file to fix: %s", f)
		}
	}
}

// Findings inside the children of a "parallel" group that never runs are dropped like those of any dead
// step, and a dead child of a live group drops only its own lines.
func TestUnreachableFilteringCoversParallelChildren(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []int // remaining unpinned-uses lines
	}{
		{"dead group", "      - if: false\n        parallel:\n          - uses: actions/checkout@v4\n          - uses: actions/cache@v4\n      - uses: actions/setup-node@v4\n", []int{13}},
		{"dead child", "      - parallel:\n          - if: false\n            uses: actions/checkout@v4\n          - uses: actions/cache@v4\n      - uses: actions/setup-node@v4\n", []int{12, 13}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "name: ci\non: push\npermissions: {}\njobs:\n  b:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n" + tc.src
			root := ignoreProject(t, ignoreConfigHead, map[string]string{"ci.yaml": src})
			errs := lintIgnoreProject(t, root, fixedNow)
			if got := unpinnedAt(errs); !sameInts(got, tc.want) {
				t.Errorf("remaining %v, want %v (%v)", got, tc.want, lineIDsOf(errs))
			}
		})
	}
}
