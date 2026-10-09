package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeWorkflowRepo creates a Git repository whose .github/workflows directory holds the given files.
func makeWorkflowRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		p := filepath.Join(root, ".github", "workflows", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const validWorkflowSrc = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"

// GitHub loads only the files directly in .github/workflows, so YAML files in a subdirectory (data of a
// test, configuration of a tool) must not be linted as workflows by the repository mode (bug bash:
// 58 false positives in django, cli and dotnet).
func TestRepositoryModeLintsOnlyDirectChildrenOfWorkflowsDir(t *testing.T) {
	root := makeWorkflowRepo(t, map[string]string{
		"ci.yaml":                 validWorkflowSrc,
		"data/conda/geolibs.yml":  "name: geolibs\nchannels:\n  - conda-forge\ndependencies:\n  - python\n",
		"evals/prompt.eval.yaml":  "prompt: hello\n",
		"scripts/nested/x.yaml":   "key: value\n",
		"not-yaml.txt":            "hello\n",
		"directory.yml/other.yml": "key: value\n",
	})
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(&Config{})
	files, _, err := l.repositoryFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "ci.yaml" {
		t.Fatalf("only ci.yaml is a workflow but got %v", files)
	}
	errs, err := l.LintRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("subdirectories must not be linted: %v", errs)
	}

	// A file given explicitly is linted as before
	errs, err = l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "data", "conda", "geolibs.yml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) == 0 || !strings.Contains(errs[0].Message, "section is missing") {
		t.Fatalf("an explicit file must be linted: %v", errs)
	}
}

func TestRepositoryModeWithOnlySubdirectoriesHasNothingToLint(t *testing.T) {
	root := makeWorkflowRepo(t, map[string]string{"data/x.yml": "key: value\n"})
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.LintRepository(root)
	if err == nil || !strings.Contains(err.Error(), "no YAML file was found") {
		t.Fatalf("unexpected error %v", err)
	}
}

// A wrong value of a flag is a usage error (2) whichever flag it is; a failure of the run itself is 3
// (bug bash: -profile bogus exited with 2 but -format bogus with 3).
func TestFlagValueErrorsExitWithUsageStatus(t *testing.T) {
	root := makeWorkflowRepo(t, map[string]string{"ci.yaml": validWorkflowSrc})
	for _, args := range [][]string{
		{"-profile", "bogus"},
		{"-format", "bogus"},
		{"-format", "{{ .Nope"},
		{"-ignore", "(unclosed"},
		{"-min-severity", "loud"},
		{"-online=maybe"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut strings.Builder
			cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
			status := cmd.Main(append(append([]string{"jactionlint", "-shellcheck=", "-pyflakes="}, args...), filepath.Join(root, ".github", "workflows", "ci.yaml")))
			if status != ExitStatusInvalidCommandOption {
				t.Errorf("want %d but got %d: %s", ExitStatusInvalidCommandOption, status, errOut.String())
			}
		})
	}

	// An unreadable file is a failure of the run
	var errOut strings.Builder
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &errOut}
	if status := cmd.Main([]string{"jactionlint", "-shellcheck=", "-pyflakes=", filepath.Join(root, "missing.yaml")}); status != ExitStatusFailure {
		t.Errorf("a missing file must exit with %d but got %d", ExitStatusFailure, status)
	}
}

// makeBrokenActionRepo creates a repository whose workflows all use two local actions with problems that
// are reported with the absolute path of the action in the message.
func makeBrokenActionRepo(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		".github/actions/bad/action.yml":    "name: bad\ndescription: x\nruns: [\n",
		".github/actions/nofile/action.yml": "name: x\ndescription: y\nruns:\n  using: node20\n  main: dist/index.js\n",
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		files[".github/workflows/"+n+".yaml"] = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/bad\n      - uses: ./.github/actions/nofile\n"
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The findings about a local action (its metadata is broken) used to be reported in the file which a goroutine
// happened to lint first, so the finding moved between runs and a baseline written once did not accept it in
// the next run (bug bash: two findings reappeared after moving a pytorch checkout).
func TestFindingsAboutALocalActionAreReportedOnceInTheFirstFile(t *testing.T) {
	root := t.TempDir()
	makeBrokenActionRepo(t, root)
	for i := 0; i < 15; i++ {
		l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = withFixtureRules(&Config{})
		errs, err := l.LintRepository(root)
		if err != nil {
			t.Fatal(err)
		}
		var where []string
		for _, e := range errs {
			if e.ID == "invalid-local-action" {
				if strings.Contains(e.Message, root) || !strings.Contains(e.Message, "./.github/actions/") {
					t.Errorf("the message must show the path relative to the repository: %s", e.Message)
				}
				where = append(where, e.Filepath+":"+strings.SplitN(e.Message, " ", 2)[0])
			}
		}
		want := filepath.Join(".github", "workflows", "a.yaml")
		if len(where) != 2 || !strings.HasPrefix(where[0], want) || !strings.HasPrefix(where[1], want) {
			t.Fatalf("run %d: the two findings must be reported once, in a.yaml: %v", i, where)
		}
	}
}

func TestBaselineSurvivesMovingTheCheckout(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	makeBrokenActionRepo(t, first)
	args := []string{"jactionlint", "-no-color", "-shellcheck=", "-pyflakes=", "-profile", "correctness"}
	run := func(dir string, extra ...string) (int, string) {
		t.Helper()
		var out, errOut strings.Builder
		cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		t.Chdir(dir)
		st := cmd.Main(append(append([]string{}, args...), extra...))
		return st, out.String() + errOut.String()
	}
	if st, out := run(first, "-baseline-write"); st != 0 {
		t.Fatalf("%d %s", st, out)
	}
	second := filepath.Join(parent, "moved", "elsewhere")
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(first, second); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		st, out := run(second, "-baseline", "-baseline-check")
		if st != 0 || strings.Contains(out, "invalid-local-action") || strings.Contains(out, "unused-baseline-entry") {
			t.Fatalf("run %d: the moved checkout must match its baseline: %d\n%s", i, st, out)
		}
	}
}

// Every rule which sets Error.Fix must say so in RuleInfo.Fixable: the rules documentation, the SARIF rule
// metadata and the docs of -fix are generated from it (bug bash: unused-ignore and the -online fix of
// unpinned-uses produced fixes but were not marked). The fixers are found by linting the test data with every
// rule on and watching which findings carry a fix.
func TestEveryRuleWhichSetsAFixIsMarkedFixable(t *testing.T) {
	cfg := mustParseConfig(t, `profile: pedantic
rules:
  missing-timeout:
    default-minutes: 30
  mutable-runner-label:
    pin:
      ubuntu-latest: ubuntu-24.04
  unused-ignore: error
  dependabot-cooldown:
    default-days: 7
`)
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	l.shellcheck, l.pyflakes = "", ""

	seen := map[string]string{}
	err = filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		errs, err := l.Lint(path, src, nil)
		if err != nil {
			return nil // a fixture that is not meant to be linted
		}
		for _, e := range errs {
			if e.Fix != nil {
				if _, ok := seen[e.ID]; !ok {
					seen[e.ID] = path
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) < 10 {
		t.Fatalf("the test data must show the fixers but only %d were found: %v", len(seen), seen)
	}
	for id, path := range seen {
		info, ok := ruleIndex[id]
		if !ok {
			t.Errorf("%s: %q has a fix but is no registered rule", path, id)
		} else if !info.Fixable {
			t.Errorf("%s: rule %q sets Error.Fix but RuleInfo.Fixable is false", path, id)
		}
	}
}

// A rule has a section in docs/checks.md and says where: the rules documentation links to DocsAnchor. These are
// documented elsewhere (the syntax of the YAML file in the sections about the structure, required-actions and the
// baseline in config.md and usage.md).
func TestEveryRuleHasADocsAnchor(t *testing.T) {
	elsewhere := map[string]bool{"yaml-syntax": true, "required-actions": true, "unused-baseline-entry": true}
	for _, info := range Rules() {
		if info.DocsAnchor == "" && !elsewhere[info.ID] {
			t.Errorf("rule %q has no DocsAnchor", info.ID)
		}
	}
}

// The pin fix of unpinned-uses exists only with -online, so the walk over the test data above cannot see it.
func TestUnpinnedUsesIsFixableWithOnline(t *testing.T) {
	errs, _ := lintOnline(t, onlineFixtureClient(t), unpinnedConfig(), workflowWith("uses: actions/checkout@v4"))
	var fixed bool
	for _, e := range errs {
		if e.ID == "unpinned-uses" && e.Fix != nil {
			fixed = true
		}
	}
	if !fixed {
		t.Fatalf("the online pin fix is gone: %v", errs)
	}
	if !ruleIndex["unpinned-uses"].Fixable {
		t.Error("unpinned-uses sets Error.Fix with -online but RuleInfo.Fixable is false")
	}
}
