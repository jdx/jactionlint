package jactionlint

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintDependabot(t *testing.T, src string, cfg *Config, project *Project) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.Lint(dependabotFixturePath, []byte(src), project)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func cooldownConfig(opts map[string]any) *Config {
	return &Config{Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityWarning, Options: opts}}}
}

func TestDependabotCooldownFix(t *testing.T) {
	const head = "version: 2\nupdates:\n"
	tests := []struct {
		name string
		src  string
		opts map[string]any
		want string // expected source after the safe fixes; "" means the finding has no fix
	}{
		{
			"missing cooldown",
			head + "  - package-ecosystem: npm\n    directory: \"/\"\n    schedule:\n      interval: weekly\n",
			map[string]any{"default-days": 14},
			head + "  - package-ecosystem: npm\n    cooldown:\n      default-days: 14\n    directory: \"/\"\n    schedule:\n      interval: weekly\n",
		},
		{
			"missing cooldown, quoted ecosystem with a comment and 4 spaces indent",
			"version: 2\nupdates:\n    -   package-ecosystem: 'npm' # js\n        directory: /\n        schedule: {interval: weekly}\n",
			map[string]any{"default-days": 7},
			"version: 2\nupdates:\n    -   package-ecosystem: 'npm' # js\n        cooldown:\n          default-days: 7\n        directory: /\n        schedule: {interval: weekly}\n",
		},
		{
			"missing cooldown when the ecosystem is not the first key",
			head + "  - directory: /\n    package-ecosystem: pip\n    schedule:\n      interval: weekly\n",
			map[string]any{"default-days": 7},
			head + "  - directory: /\n    package-ecosystem: pip\n    cooldown:\n      default-days: 7\n    schedule:\n      interval: weekly\n",
		},
		{
			"CRLF",
			"version: 2\r\nupdates:\r\n  - package-ecosystem: npm\r\n    directory: /\r\n    schedule:\r\n      interval: weekly\r\n",
			map[string]any{"default-days": 7},
			"version: 2\r\nupdates:\r\n  - package-ecosystem: npm\r\n    cooldown:\r\n      default-days: 7\r\n    directory: /\r\n    schedule:\r\n      interval: weekly\r\n",
		},
		{
			"ecosystem on the last line without a newline",
			head + "  - directory: /\n    schedule:\n      interval: weekly\n    package-ecosystem: npm",
			map[string]any{"default-days": 7},
			head + "  - directory: /\n    schedule:\n      interval: weekly\n    package-ecosystem: npm\n    cooldown:\n      default-days: 7",
		},
		{
			"cooldown without default-days",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown: # note\n      # comment\n      semver-major-days: 30\n",
			map[string]any{"default-days": 7},
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown: # note\n      default-days: 7\n      # comment\n      semver-major-days: 30\n",
		},
		{
			"cooldown as the first key of the item",
			head + "  - cooldown:\n      semver-major-days: 30\n    package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n",
			map[string]any{"default-days": 7},
			head + "  - cooldown:\n      default-days: 7\n      semver-major-days: 30\n    package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n",
		},
		{
			"too small default-days",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 2 # short\n",
			map[string]any{"default-days": 10},
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 10 # short\n",
		},
		{
			"too small default-days in flow style",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown: {default-days: 2}\n",
			map[string]any{"default-days": 10},
			"",
		},
		{
			"cooldown in flow style without default-days",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown: {semver-major-days: 2}\n",
			map[string]any{"default-days": 10},
			"",
		},
		{
			"ecosystem in flow style",
			head + "  - {package-ecosystem: npm, directory: /, schedule: {interval: weekly}}\n",
			map[string]any{"default-days": 10},
			"",
		},
		{
			"no default-days option, no fix",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n",
			nil,
			"",
		},
		{
			"the fix value would not satisfy the rule",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n",
			map[string]any{"default-days": 5},
			"",
		},
		{
			"the fix value must satisfy a raised minimum",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 10\n",
			map[string]any{"days": 14, "default-days": 10},
			"",
		},
		{
			"raised minimum",
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 10\n",
			map[string]any{"days": 14, "default-days": 21},
			head + "  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 21\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := cooldownConfig(tc.opts)
			errs := lintDependabot(t, tc.src, cfg, nil)
			if len(errs) != 1 || errs[0].ID != "dependabot-cooldown" {
				t.Fatalf("one dependabot-cooldown error is expected: %v", errs)
			}
			if tc.want == "" {
				if errs[0].Fix != nil {
					t.Fatalf("no fix is expected but got %+v", errs[0].Fix)
				}
				return
			}
			if errs[0].Fix == nil || errs[0].Fix.Unsafe {
				t.Fatalf("a safe fix is expected: %+v", errs[0].Fix)
			}
			got, n := applyFixes([]byte(tc.src), errs, FixModeSafe)
			if n != 1 || string(got) != tc.want {
				t.Fatalf("applied %d fixes. want:\n%q\ngot:\n%q", n, tc.want, got)
			}
			if again := lintDependabot(t, string(got), cfg, nil); len(again) != 0 {
				t.Fatalf("the fixed source still has errors: %v", again)
			}
		})
	}
}

func TestDependabotCooldownMinimum(t *testing.T) {
	src := "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n"
	// The implicit cooldown of Dependabot is 3 days, so a lower minimum is satisfied without the section.
	if errs := lintDependabot(t, src, cooldownConfig(map[string]any{"days": 3}), nil); len(errs) != 0 {
		t.Fatal(errs)
	}
	if errs := lintDependabot(t, src, cooldownConfig(map[string]any{"days": 4}), nil); len(errs) != 1 {
		t.Fatal(errs)
	}
	// Every update is checked independently
	two := src + "  - package-ecosystem: pip\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 30\n  - package-ecosystem: cargo\n    directory: /\n    schedule:\n      interval: weekly\n"
	errs := lintDependabot(t, two, cooldownConfig(nil), nil)
	if len(errs) != 2 || errs[0].Line != 3 || errs[1].Line != 13 {
		t.Fatal(errs)
	}
	// The profile "default" enables the rule at warning level
	errs = lintDependabot(t, src, &Config{}, nil)
	if len(errs) != 1 || errs[0].Severity != SeverityWarning {
		t.Fatal(errs)
	}
	// And the rule can be turned off
	if errs := lintDependabot(t, src, &Config{Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}, nil); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestDependabotExecutionFix(t *testing.T) {
	for _, value := range []string{"allow", `"allow"`, `'allow'`} {
		src := "version: 2\nupdates:\n  - package-ecosystem: pip\n    directory: /\n    schedule:\n      interval: weekly\n    cooldown:\n      default-days: 7\n    insecure-external-code-execution: " + value + " # needed\n"
		cfg := &Config{}
		errs := lintDependabot(t, src, cfg, nil)
		if len(errs) != 1 || errs[0].ID != "dependabot-execution" || errs[0].Severity != SeverityError {
			t.Fatalf("%s: %v", value, errs)
		}
		if errs[0].Fix == nil || !errs[0].Fix.Unsafe {
			t.Fatalf("%s: an unsafe fix is expected: %+v", value, errs[0].Fix)
		}
		if got, n := applyFixes([]byte(src), errs, FixModeSafe); n != 0 || string(got) != src {
			t.Fatalf("%s: an unsafe fix must not be applied by default: %q", value, got)
		}
		got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
		want := strings.Replace(src, value, strings.Replace(value, "allow", "deny", 1), 1)
		if n != 1 || string(got) != want {
			t.Fatalf("%s: want %q got %q", value, want, got)
		}
		if again := lintDependabot(t, string(got), cfg, nil); len(again) != 0 {
			t.Fatalf("%s: the fixed source still has errors: %v", value, again)
		}
	}
}

func TestDependabotMissingActionsUpdate(t *testing.T) {
	const withoutActions = "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n"
	const withActions = withoutActions + "  - package-ecosystem: github-actions\n    directory: /\n    schedule:\n      interval: weekly\n"
	cfg := &Config{Profile: ProfileStrict, Rules: map[string]RuleConfig{"dependabot-cooldown": {Level: SeverityOff}}}

	newProject := func(t *testing.T, files map[string]string) *Project {
		t.Helper()
		root := t.TempDir()
		for name, content := range files {
			p := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return &Project{root: root}
	}
	const usesAction = "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"

	tests := []struct {
		name  string
		src   string
		files map[string]string
		want  bool
	}{
		{"actions without update", withoutActions, map[string]string{".github/workflows/ci.yml": usesAction}, true},
		{"reusable workflow", withoutActions, map[string]string{".github/workflows/ci.yaml": "on: push\njobs:\n  call:\n    uses: org/repo/.github/workflows/x.yml@v1\n"}, true},
		{"quoted uses", withoutActions, map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  test:\n    runs-on: x\n    steps:\n      - uses: 'actions/checkout@v4' # c\n"}, true},
		{"update exists", withActions, map[string]string{".github/workflows/ci.yml": usesAction}, false},
		{"no workflows", withoutActions, map[string]string{"README.md": "x"}, false},
		{"workflows without uses", withoutActions, map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  test:\n    runs-on: x\n    steps:\n      - run: echo uses actions/checkout\n"}, false},
		{"only local actions and docker", withoutActions, map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  test:\n    runs-on: x\n    steps:\n      - uses: ./.github/actions/foo\n      - uses: docker://alpine:3\n"}, false},
		{"commented uses", withoutActions, map[string]string{".github/workflows/ci.yml": "on: push\njobs:\n  test:\n    runs-on: x\n    steps:\n      # - uses: actions/checkout@v4\n      - run: echo\n"}, false},
		{"renovate", withoutActions, map[string]string{".github/workflows/ci.yml": usesAction, "renovate.json": "{}"}, false},
		{"renovate in .github", withoutActions, map[string]string{".github/workflows/ci.yml": usesAction, ".github/renovate.json5": "{}"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintDependabot(t, tc.src, cfg, newProject(t, tc.files))
			if !tc.want {
				if len(errs) != 0 {
					t.Fatal(errs)
				}
				return
			}
			if len(errs) != 1 || errs[0].ID != "dependabot-missing-actions-update" || errs[0].Line != 1 || errs[0].Fix != nil {
				t.Fatal(errs)
			}
		})
	}

	t.Run("not enabled by default", func(t *testing.T) {
		p := newProject(t, map[string]string{".github/workflows/ci.yml": usesAction})
		if errs := lintDependabot(t, withoutActions, &Config{Rules: cfg.Rules}, p); len(errs) != 0 {
			t.Fatal(errs)
		}
	})
	t.Run("no project", func(t *testing.T) {
		r := NewRuleDependabotMissingActionsUpdate(nil)
		if err := r.VisitDependabotPost(&Dependabot{Pos: &Pos{Line: 1, Col: 1}}); err != nil || len(r.Errs()) != 0 {
			t.Fatal(err, r.Errs())
		}
	})
}

func TestDependabotFixesOfSeveralUpdates(t *testing.T) {
	src := "version: 2\nupdates:\n" +
		"  - package-ecosystem: npm\n    directory: /\n    schedule: {interval: weekly}\n" +
		"  - package-ecosystem: pip\n    directory: /\n    schedule: {interval: weekly}\n    cooldown:\n      default-days: 1\n    insecure-external-code-execution: allow\n" +
		"  - package-ecosystem: cargo\n    directory: /\n    schedule: {interval: weekly}\n    cooldown:\n      semver-major-days: 5\n"
	want := "version: 2\nupdates:\n" +
		"  - package-ecosystem: npm\n    cooldown:\n      default-days: 9\n    directory: /\n    schedule: {interval: weekly}\n" +
		"  - package-ecosystem: pip\n    directory: /\n    schedule: {interval: weekly}\n    cooldown:\n      default-days: 9\n    insecure-external-code-execution: deny\n" +
		"  - package-ecosystem: cargo\n    directory: /\n    schedule: {interval: weekly}\n    cooldown:\n      default-days: 9\n      semver-major-days: 5\n"
	cfg := cooldownConfig(map[string]any{"default-days": 9})
	errs := lintDependabot(t, src, cfg, nil)
	if len(errs) != 4 {
		t.Fatal(errs)
	}
	got, n := applyFixes([]byte(src), errs, FixModeUnsafe)
	if n != 4 || string(got) != want {
		t.Fatalf("applied %d fixes. want:\n%s\ngot:\n%s", n, want, got)
	}
	if again := lintDependabot(t, string(got), cfg, nil); len(again) != 0 {
		t.Fatal(again)
	}
	// Without the unsafe fixes only the cooldown is fixed and the allow stays
	got, n = applyFixes([]byte(src), errs, FixModeSafe)
	if n != 3 || !strings.Contains(string(got), "allow") {
		t.Fatalf("%d %s", n, got)
	}
}

func TestFixRepositoryFixesDependabot(t *testing.T) {
	src := "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    insecure-external-code-execution: allow\n"
	root := makeDependabotProject(t, map[string]string{"dependabot.yml": src},
		"rules:\n  dependabot-cooldown:\n    default-days: 7\n")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	res, err := l.FixRepository("", FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 2\nupdates:\n  - package-ecosystem: npm\n    cooldown:\n      default-days: 7\n    directory: /\n    schedule:\n      interval: weekly\n    insecure-external-code-execution: allow\n"
	if string(b) != want || res.Applied != 1 {
		t.Fatalf("applied %d fixes: %q", res.Applied, b)
	}
	// The unsafe fix remains as a finding
	if len(res.Errors) != 1 || res.Errors[0].ID != "dependabot-execution" {
		t.Fatal(res.Errors)
	}
}

// -fix without files fixes the Dependabot configuration and still reports what it cannot fix, a syntax error
// included, so that the exit status stays 1.
func TestFixRepositoryKeepsSyntaxErrorsOfDependabot(t *testing.T) {
	src := "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: weekly\n    unknown-key: 1\n"
	root := makeDependabotProject(t, map[string]string{"dependabot.yml": src},
		"rules:\n  dependabot-cooldown:\n    default-days: 7\n")
	t.Chdir(root)
	var stdout, stderr bytes.Buffer
	code := (&Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}).Main([]string{"jactionlint", "-no-color", "-fix"})
	if code != ExitStatusSuccessProblemFound || !strings.Contains(stdout.String(), "unknown-key") {
		t.Errorf("exit status %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".github", "dependabot.yml")); !strings.Contains(string(b), "default-days: 7") {
		t.Errorf("the cooldown was not added: %s", b)
	}
}
