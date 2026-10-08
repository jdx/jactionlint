package jactionlint

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// dependabotFixturePrefix is the prefix of the fixtures which are Dependabot configurations. The
	// linter chooses the rules by the file name so they are linted as if they were this file.
	dependabotFixturePrefix = "dependabot_"
	dependabotFixturePath   = ".github/dependabot.yml"
)

// dependabotRuleFixtures are the fixtures which test a rule of dependabot.yml. The other fixtures test
// the syntax, so they run with these rules turned off.
var dependabotRuleFixtures = map[string]string{
	"dependabot_cooldown":  "dependabot-cooldown",
	"dependabot_execution": "dependabot-execution",
}

func dependabotFixtureConfig(base string) *Config {
	cfg := fixtureConfig()
	for _, id := range dependabotRuleFixtures {
		cfg.Rules[id] = RuleConfig{Level: SeverityOff}
	}
	if id, ok := dependabotRuleFixtures[filepath.Base(base)]; ok {
		delete(cfg.Rules, id)
	}
	return cfg
}

func TestDependabotFixtures(t *testing.T) {
	for _, subdir := range []string{"ok", "err", "examples"} {
		files, err := filepath.Glob(filepath.Join("testdata", subdir, dependabotFixturePrefix+"*.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no Dependabot fixture in testdata/%s", subdir)
		}
		for _, f := range files {
			base := strings.TrimSuffix(f, ".yaml")
			t.Run(subdir+"/"+filepath.Base(base), func(t *testing.T) {
				src, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				l, err := NewLinter(io.Discard, &LinterOptions{})
				if err != nil {
					t.Fatal(err)
				}
				l.defaultConfig = dependabotFixtureConfig(base)
				errs, err := l.Lint(dependabotFixturePath, src, &Project{root: filepath.Dir(f)})
				if err != nil {
					t.Fatal(err)
				}
				if subdir == "ok" {
					if len(errs) > 0 {
						t.Fatal(errs)
					}
					return
				}
				for _, e := range errs {
					if e.ID != "dependabot-syntax" && e.ID != "yaml-syntax" && e.ID != dependabotRuleFixtures[filepath.Base(base)] {
						t.Errorf("unexpected ID %q: %s", e.ID, e)
					}
				}
				checkErrors(t, base+".out", errs)
			})
		}
	}
}

func TestIsDependabotPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{".github/dependabot.yml", true},
		{".github/dependabot.yaml", true},
		{"/repo/.github/dependabot.yml", true},
		{`C:\repo\.github\dependabot.yml`, true},
		{"dependabot.yml", true},
		{"./dependabot.yaml", true},
		// A workflow which runs for pull requests of Dependabot may have this name
		{".github/workflows/dependabot.yml", false},
		{"dependabot.yml/test.yaml", false},
		{"docs/dependabot.yml", false},
		{".github/dependabot.json", false},
		{".github/dependabot.yml.bak", false},
		{".github/Dependabot.yml", false},
		{"<stdin>", false},
		{"test.yaml", false},
	} {
		if have := isDependabotPath(tc.path); have != tc.want {
			t.Errorf("isDependabotPath(%q) = %v, want %v", tc.path, have, tc.want)
		}
	}
}

func TestDependabotFileIsResolvedAgainstWorkingDir(t *testing.T) {
	root := t.TempDir()
	workflows := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	// Not valid as a Dependabot configuration, valid as a workflow
	workflow := []byte("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo hi\n")

	for _, tc := range []struct {
		name string
		dir  string
		path string
		want bool
	}{
		{"bare name in the workflows directory", workflows, "dependabot.yml", false},
		{"bare name in the .github directory", filepath.Join(root, ".github"), "dependabot.yml", true},
		{"relative path", root, filepath.Join(".github", "dependabot.yml"), true},
		{"relative workflow path", root, filepath.Join(".github", "workflows", "dependabot.yml"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &LinterOptions{WorkingDir: tc.dir}
			l, err := NewLinter(io.Discard, opts)
			if err != nil {
				t.Fatal(err)
			}
			if have := l.isDependabotFile(tc.path); have != tc.want {
				t.Errorf("isDependabotFile(%q) in %q = %v, want %v", tc.path, tc.dir, have, tc.want)
			}
			if !tc.want {
				errs, err := l.Lint(tc.path, workflow, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range errs {
					if e.ID == "dependabot-syntax" {
						t.Errorf("workflow was linted as a Dependabot configuration: %v", e)
					}
				}
			}
		})
	}

	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: workflows, StdinFileName: "dependabot.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if !l.isDependabotFile("dependabot.yml") {
		t.Error("the name given for STDIN must keep meaning a Dependabot configuration")
	}
}

func TestParseDependabotTree(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ok", "dependabot_full.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	d, errs := ParseDependabot(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	if d.Version == nil || d.Version.Value != 2 || *d.Version.Pos != (Pos{1, 10}) {
		t.Errorf("unexpected version %+v", d.Version)
	}
	if d.EnableBetaEcosystems == nil || !d.EnableBetaEcosystems.Value {
		t.Errorf("unexpected enable-beta-ecosystems %+v", d.EnableBetaEcosystems)
	}
	if len(d.Registries) != 2 || d.Registries[0].Name.Value != "npm-github" || d.Registries[0].Type.Value != "npm-registry" ||
		!d.Registries[0].ReplacesBase.Value || *d.Registries[0].Pos != (Pos{4, 3}) {
		t.Errorf("unexpected registries %+v", d.Registries)
	}
	if r := d.Registries[1]; len(r.Settings) != 2 || r.Settings[0].Key.Value != "tenant-id" || r.Settings[0].Value.Value != "1234" {
		t.Errorf("unexpected registry settings %+v", r.Settings)
	}
	if len(d.Updates) != 3 {
		t.Fatalf("want 3 updates but got %d", len(d.Updates))
	}

	u := d.Updates[0]
	if u.PackageEcosystem.Value != "npm" || *u.Pos != (Pos{15, 5}) || *u.PackageEcosystem.Pos != (Pos{15, 24}) {
		t.Errorf("unexpected update %+v", u)
	}
	if u.Schedule.Interval.Value != "weekly" || u.Schedule.Day.Value != "monday" || u.Schedule.Time.Value != "09:30" ||
		u.Schedule.Timezone.Value != "Asia/Tokyo" || *u.Schedule.Pos != (Pos{17, 5}) {
		t.Errorf("unexpected schedule %+v", u.Schedule)
	}
	if u.Labels == nil || len(u.Labels) != 0 {
		t.Errorf("empty labels must be kept as an empty list: %#v", u.Labels)
	}
	if u.OpenPullRequestsLimit.Value != 10 || u.Milestone.Value != 4 || u.Vendor.Value {
		t.Errorf("unexpected scalars %+v", u)
	}
	c := u.Cooldown
	if c.DefaultDays.Value != 7 || c.SemverMajorDays.Value != 30 || c.SemverMinorDays.Value != 14 || c.SemverPatchDays.Value != 3 ||
		len(c.Include) != 1 || c.Include[0].Value != "*" || len(c.Exclude) != 1 || c.Exclude[0].Value != "lodash" {
		t.Errorf("unexpected cooldown %+v", c)
	}
	if len(u.Allow) != 2 || u.Allow[0].DependencyType.Value != "direct" || u.Allow[1].DependencyName.Value != "react*" {
		t.Errorf("unexpected allow %+v", u.Allow)
	}
	if len(u.Ignore) != 1 || len(u.Ignore[0].Versions) != 2 || len(u.Ignore[0].UpdateTypes) != 1 {
		t.Errorf("unexpected ignore %+v", u.Ignore)
	}
	if len(u.Groups) != 2 || u.Groups[0].Name.Value != "production" || len(u.Groups[0].UpdateTypes) != 2 || u.Groups[1].GroupBy.Value != "dependency-name" {
		t.Errorf("unexpected groups %+v", u.Groups)
	}
	if u.CommitMessage.Prefix.Value != "deps" || u.CommitMessage.Include.Value != "scope" || u.PullRequestBranchName.Separator.Value != "-" {
		t.Errorf("unexpected commit-message %+v", u.CommitMessage)
	}

	m := d.Updates[1]
	if len(m.Directories) != 2 || m.Directory != nil || m.Schedule.Cronjob.Value != "0 9 * * 1" || len(m.Registries) != 1 || m.Registries[0].Value != "*" {
		t.Errorf("unexpected update %+v", m)
	}
}

func TestParseDependabotMultiEcosystemGroups(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ok", "dependabot_multi_ecosystem.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	d, errs := ParseDependabot(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(d.MultiEcosystemGroups) != 1 {
		t.Fatalf("unexpected groups %+v", d.MultiEcosystemGroups)
	}
	g := d.MultiEcosystemGroups[0]
	if g.Name.Value != "infra" || g.Schedule.Interval.Value != "weekly" || len(g.Labels) != 1 || g.CommitMessage.Prefix.Value != "chore" {
		t.Errorf("unexpected group %+v", g)
	}
	if u := d.Updates[0]; u.MultiEcosystemGroup.Value != "infra" || len(u.Patterns) != 1 || u.Schedule != nil {
		t.Errorf("unexpected update %+v", u)
	}
}

// The sample rule of the plumbing for Dependabot rules. Rules such as dependabot-cooldown follow
// the same shape.
type testNoNpmRule struct {
	DependabotRuleBase
	pre, post, updates int
}

func newTestNoNpmRule() *testNoNpmRule {
	return &testNoNpmRule{DependabotRuleBase: NewDependabotRuleBase("test-no-npm", "reports npm")}
}

func (r *testNoNpmRule) VisitDependabotPre(*Dependabot) error  { r.pre++; return nil }
func (r *testNoNpmRule) VisitDependabotPost(*Dependabot) error { r.post++; return nil }
func (r *testNoNpmRule) VisitDependabotUpdate(n *DependabotUpdate) error {
	r.updates++
	if n.PackageEcosystem != nil && n.PackageEcosystem.Value == "npm" {
		r.Errorf(n.PackageEcosystem.Pos, "package ecosystem %q is not allowed", n.PackageEcosystem.Value)
	}
	return nil
}

func TestDependabotRuleVisitor(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ok", "dependabot_full.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rule := newTestNoNpmRule()
	l, err := NewLinter(io.Discard, &LinterOptions{
		OnDependabotRulesCreated: func(rules []DependabotRule) []DependabotRule {
			return append(rules, rule)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = dependabotFixtureConfig("")

	errs, err := l.Lint(dependabotFixturePath, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rule.pre != 1 || rule.post != 1 || rule.updates != 3 {
		t.Errorf("unexpected number of visits: %+v", rule)
	}
	if len(errs) != 2 {
		t.Fatalf("want 2 errors but got %v", errs)
	}
	e := errs[0]
	if e.Line != 15 || e.Column != 24 || e.ID != "test-no-npm" || e.Kind != "test-no-npm" || e.Filepath != dependabotFixturePath ||
		e.Message != `package ecosystem "npm" is not allowed` {
		t.Errorf("unexpected error %+v", e)
	}
	if errs[1].Line != 72 {
		t.Errorf("unexpected error %+v", errs[1])
	}
}

func TestDependabotRuleIsNotCalledForWorkflow(t *testing.T) {
	rule := newTestNoNpmRule()
	l, err := NewLinter(io.Discard, &LinterOptions{
		OnDependabotRulesCreated: func(rules []DependabotRule) []DependabotRule { return append(rules, rule) },
	})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{}
	// A workflow named dependabot.yml in the workflows directory is a workflow
	src := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	errs, err := l.Lint(".github/workflows/dependabot.yml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 || rule.pre != 0 {
		t.Errorf("workflow was handled as Dependabot configuration: %v %+v", errs, rule)
	}
}

func TestWorkflowRulesDoNotRunOnDependabot(t *testing.T) {
	var called []string
	l, err := NewLinter(io.Discard, &LinterOptions{
		OnRulesCreated: func(rules []Rule) []Rule {
			called = append(called, "workflow rules")
			return rules
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{}
	src := "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: daily\n    cooldown:\n      default-days: 7\n"
	errs, err := l.Lint(dependabotFixturePath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) > 0 || len(called) > 0 {
		t.Errorf("workflow rules ran on dependabot.yml: %v %v", errs, called)
	}

	// The syntax errors are those of dependabot.yml, not of workflows
	errs, err = l.Lint(dependabotFixturePath, []byte("on: push\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 {
		t.Fatal("no error was found")
	}
	for _, e := range errs {
		if e.ID != "dependabot-syntax" {
			t.Errorf("unexpected error %s (ID: %s)", e, e.ID)
		}
	}
}

// makeDependabotProject creates a repository with a workflow and the given Dependabot configuration.
func makeDependabotProject(t *testing.T, dependabot map[string]string, config string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".github/workflows/ci.yaml", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
	for name, content := range dependabot {
		write(".github/"+name, content)
	}
	if config != "" {
		write(".github/jactionlint.yaml", config)
	}
	return root
}

// brokenDependabot has one syntax error. It sets the cooldown so that no other rule reports.
const brokenDependabot = "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: daily\n      dayy: monday\n    cooldown:\n      default-days: 7\n"

func lintRepo(t *testing.T, root string, opts LinterOptions) ([]*Error, string) {
	t.Helper()
	var out bytes.Buffer
	opts.WorkingDir = root
	opts.Color = ColorOptionKindNever
	l, err := NewLinter(&out, &opts)
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.LintRepository("")
	if err != nil {
		t.Fatal(err)
	}
	return errs, out.String()
}

func TestLintRepositoryFindsDependabot(t *testing.T) {
	for _, name := range []string{"dependabot.yml", "dependabot.yaml"} {
		t.Run(name, func(t *testing.T) {
			root := makeDependabotProject(t, map[string]string{name: brokenDependabot}, "")
			errs, out := lintRepo(t, root, LinterOptions{})
			if len(errs) != 1 {
				t.Fatalf("want 1 error but got %v", errs)
			}
			want := filepath.Join(".github", name)
			if e := errs[0]; e.Filepath != want || e.ID != "dependabot-syntax" || e.Line != 7 || e.Column != 7 {
				t.Errorf("unexpected error %+v", e)
			}
			if !strings.Contains(out, want+":7:7: unexpected key \"dayy\"") {
				t.Errorf("unexpected output %q", out)
			}
		})
	}
}

func TestLintRepositoryWithoutDependabot(t *testing.T) {
	root := makeDependabotProject(t, nil, "")
	errs, _ := lintRepo(t, root, LinterOptions{})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestLintRepositoryDependabotAndWorkflowErrors(t *testing.T) {
	root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot}, "")
	if err := os.WriteFile(filepath.Join(root, ".github", "workflows", "bad.yaml"), []byte("on: push\njobs: {}\nfoo: bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	errs, _ := lintRepo(t, root, LinterOptions{})
	ids := map[string]int{}
	for _, e := range errs {
		ids[e.ID]++
	}
	if ids["dependabot-syntax"] != 1 || ids["workflow-syntax"] == 0 {
		t.Errorf("unexpected errors %v", errs)
	}
}

func TestLintDependabotFileByPath(t *testing.T) {
	root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot}, "")
	path := filepath.Join(root, ".github", "dependabot.yml")
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{WorkingDir: root, Color: ColorOptionKindNever})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.LintFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "dependabot-syntax" {
		t.Fatalf("unexpected errors %v", errs)
	}

	// Together with a workflow
	l, err = NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	errs, err = l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "ci.yaml"), path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 {
		t.Fatalf("unexpected errors %v", errs)
	}
}

func TestLintDependabotFromStdin(t *testing.T) {
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{StdinFileName: ".github/dependabot.yml", Color: ColorOptionKindNever})
	if err != nil {
		t.Fatal(err)
	}
	errs, err := l.LintStdin(strings.NewReader(brokenDependabot))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].Filepath != ".github/dependabot.yml" || !strings.HasPrefix(out.String(), ".github/dependabot.yml:7:7:") {
		t.Errorf("unexpected result: %v %q", errs, out.String())
	}

	// A bare file name is also detected
	l, err = NewLinter(io.Discard, &LinterOptions{StdinFileName: "dependabot.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	errs, err = l.LintStdin(strings.NewReader(brokenDependabot))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "dependabot-syntax" {
		t.Errorf("unexpected errors %v", errs)
	}

	// Otherwise it is a workflow
	l, err = NewLinter(io.Discard, &LinterOptions{StdinFileName: "test.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	errs, err = l.LintStdin(strings.NewReader(brokenDependabot))
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 || errs[0].ID == "dependabot-syntax" {
		t.Errorf("unexpected errors %v", errs)
	}
}

func TestDependabotIgnores(t *testing.T) {
	t.Run("path config by rule ID", func(t *testing.T) {
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot},
			"paths:\n  .github/dependabot.yml:\n    ignore:\n      - dependabot-syntax\n")
		if errs, out := lintRepo(t, root, LinterOptions{}); len(errs) != 0 || out != "" {
			t.Errorf("unexpected errors %v\n%s", errs, out)
		}
	})
	t.Run("path config by glob and message", func(t *testing.T) {
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot},
			"paths:\n  .github/dependabot.y*ml:\n    ignore:\n      - 'unexpected key \"dayy\"'\n")
		if errs, out := lintRepo(t, root, LinterOptions{}); len(errs) != 0 || out != "" {
			t.Errorf("unexpected errors %v\n%s", errs, out)
		}
	})
	t.Run("path config of workflows does not apply", func(t *testing.T) {
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot},
			"paths:\n  .github/workflows/**:\n    ignore:\n      - dependabot-syntax\n")
		if errs, _ := lintRepo(t, root, LinterOptions{}); len(errs) != 1 {
			t.Errorf("unexpected errors %v", errs)
		}
	})
	t.Run("rule is turned off", func(t *testing.T) {
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot},
			"rules:\n  dependabot-syntax: off\n")
		if errs, _ := lintRepo(t, root, LinterOptions{}); len(errs) != 0 {
			t.Errorf("unexpected errors %v", errs)
		}
	})
	t.Run("rule level", func(t *testing.T) {
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot},
			"rules:\n  dependabot-syntax: warn\n")
		errs, _ := lintRepo(t, root, LinterOptions{})
		if len(errs) != 1 || errs[0].Severity != SeverityWarning {
			t.Errorf("unexpected errors %v", errs)
		}
	})
	t.Run("-ignore option", func(t *testing.T) {
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot}, "")
		if errs, _ := lintRepo(t, root, LinterOptions{IgnorePatterns: []string{`unexpected key`}}); len(errs) != 0 {
			t.Errorf("unexpected errors %v", errs)
		}
	})
	t.Run("inline comment", func(t *testing.T) {
		src := strings.Replace(brokenDependabot, "      dayy: monday", "      # jactionlint ignore=dependabot-syntax\n      dayy: monday", 1)
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": src}, "")
		if errs, out := lintRepo(t, root, LinterOptions{}); len(errs) != 0 || out != "" {
			t.Errorf("unexpected errors %v\n%s", errs, out)
		}
	})
	t.Run("unused inline comment", func(t *testing.T) {
		src := "version: 2\n# jactionlint ignore=dependabot-syntax\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule:\n      interval: daily\n    cooldown:\n      default-days: 7\n"
		root := makeDependabotProject(t, map[string]string{"dependabot.yml": src}, "rules:\n  unused-ignore: error\n")
		if errs, _ := lintRepo(t, root, LinterOptions{}); len(errs) != 1 || errs[0].ID != "unused-ignore" {
			t.Errorf("unexpected errors %v", errs)
		}
	})
}

func TestDependabotOutputFormats(t *testing.T) {
	root := makeDependabotProject(t, map[string]string{"dependabot.yml": brokenDependabot}, "")

	t.Run("text", func(t *testing.T) {
		_, out := lintRepo(t, root, LinterOptions{})
		want := ".github/dependabot.yml:7:7: unexpected key \"dayy\" for \"schedule\" section. expected one of \"cronjob\", \"day\", \"interval\", \"time\", \"timezone\" [syntax-check]\n" +
			"  |\n7 |       dayy: monday\n  |       ^~~~~\n"
		if filepath.Separator == '/' && out != want {
			t.Errorf("unexpected output:\n%s", out)
		}
	})

	t.Run("rule IDs", func(t *testing.T) {
		_, out := lintRepo(t, root, LinterOptions{ShowRuleIDs: true})
		if !strings.Contains(out, "[dependabot-syntax]") {
			t.Errorf("unexpected output:\n%s", out)
		}
	})

	t.Run("json", func(t *testing.T) {
		_, out := lintRepo(t, root, LinterOptions{Format: FormatJSON})
		var got []struct {
			ID       string `json:"id"`
			Kind     string `json:"kind"`
			Path     string `json:"filepath"`
			Line     int    `json:"line"`
			Column   int    `json:"column"`
			Severity string `json:"severity"`
			DocURL   string `json:"doc_url"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if len(got) != 1 || got[0].ID != "dependabot-syntax" || got[0].Kind != "syntax-check" || got[0].Line != 7 || got[0].Column != 7 ||
			filepath.ToSlash(got[0].Path) != ".github/dependabot.yml" || got[0].Severity != "error" ||
			got[0].DocURL != "https://jactionlint.jdx.dev/rules#dependabot-syntax" {
			t.Errorf("unexpected JSON: %+v\n%s", got, out)
		}
	})

	t.Run("sarif", func(t *testing.T) {
		_, out := lintRepo(t, root, LinterOptions{Format: FormatSARIF})
		var doc struct {
			Runs []struct {
				Results []struct {
					RuleID    string `json:"ruleId"`
					Locations []struct {
						PhysicalLocation struct {
							ArtifactLocation struct {
								URI string `json:"uri"`
							} `json:"artifactLocation"`
							Region struct {
								StartLine   int `json:"startLine"`
								StartColumn int `json:"startColumn"`
							} `json:"region"`
						} `json:"physicalLocation"`
					} `json:"locations"`
				} `json:"results"`
			} `json:"runs"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if len(doc.Runs) != 1 || len(doc.Runs[0].Results) != 1 {
			t.Fatalf("unexpected SARIF: %s", out)
		}
		r := doc.Runs[0].Results[0]
		loc := r.Locations[0].PhysicalLocation
		if r.RuleID != "dependabot-syntax" || loc.ArtifactLocation.URI != ".github/dependabot.yml" || loc.Region.StartLine != 7 || loc.Region.StartColumn != 7 {
			t.Errorf("unexpected SARIF result %+v\n%s", r, out)
		}
	})
}

// Truncated and broken inputs must not make the parser panic.
func TestParseDependabotDoesNotPanic(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*", dependabotFixturePrefix+"*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i <= len(src); i++ {
			ParseDependabot(src[:i])
		}
	}
	for _, s := range []string{
		"", "~", "[]", "- a", "version: &a 2\nupdates: *a", "updates: &u\n  - *u", "version: 2\nupdates:\n  - &x {package-ecosystem: npm}\n  - *x",
		"version: 2\nupdates: [~]", "version: 2\nregistries: {a: ~}\nupdates: [{package-ecosystem: npm, directory: /, schedule: {interval: daily}, registries: [~, []]}]",
		"version: 2\nupdates:\n  - <<: {a: b}",
	} {
		ParseDependabot([]byte(s))
	}
}
