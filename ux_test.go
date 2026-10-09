package jactionlint

import (
	"fmt"
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
	// Copy the checkout to another place instead of renaming it: Windows refuses to rename a directory in which
	// a file was just read (the runner's scanners may still hold it)
	t.Chdir(parent)
	second := filepath.Join(parent, "moved", "elsewhere")
	if err := os.CopyFS(second, os.DirFS(first)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(first); err != nil {
		t.Log(err)
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

// The $/ fix stays unsafe and says why: actionlint 1.7.12 and older reject the syntax (bug bash: the fix made
// the repository fail the other linter).
func TestSelfRepositoryFixIsUnsafeAndWarnsAboutOlderTools(t *testing.T) {
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfig(t, "rules:\n  self-repository: error\n")
	src := []byte("on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n")
	errs, err := l.Lint("test.yaml", src, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range errs {
		if e.ID != "self-repository" {
			continue
		}
		if e.Fix == nil || !e.Fix.Unsafe || !strings.Contains(e.Fix.Description, "actionlint") {
			t.Fatalf("unexpected fix %+v", e.Fix)
		}
		return
	}
	t.Fatalf("no self-repository finding: %v", errs)
}

func TestActionlintConfigNoteIsNotPrintedWhenProfileIsGiven(t *testing.T) {
	root := makeWorkflowRepo(t, map[string]string{"ci.yaml": validWorkflowSrc})
	if err := os.WriteFile(filepath.Join(root, ".github", "actionlint.yaml"), []byte("self-hosted-runner:\n  labels: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	run := func(args ...string) string {
		var out, errOut strings.Builder
		cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		cmd.Main(append([]string{"jactionlint", "-no-color", "-shellcheck=", "-pyflakes="}, args...))
		return errOut.String()
	}
	if got := run(); !strings.Contains(got, "the default profile applies") {
		t.Errorf("without -profile the note is expected: %q", got)
	}
	for _, p := range []string{"correctness", "default", "pedantic"} {
		if got := run("-profile", p); strings.Contains(got, "default profile applies") {
			t.Errorf("-profile %s decides the profile, but the note was printed: %q", p, got)
		}
	}
}

func TestBaselineWriteSaysHowToApplyTheBaseline(t *testing.T) {
	root := t.TempDir()
	makeBrokenActionRepo(t, root)
	t.Chdir(root)
	run := func(args ...string) string {
		var out, errOut strings.Builder
		cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		cmd.Main(append([]string{"jactionlint", "-no-color", "-shellcheck=", "-pyflakes=", "-profile", "correctness"}, args...))
		return out.String()
	}
	got := run("-baseline-write")
	if !strings.Contains(got, "baseline: auto") || !strings.Contains(got, ".github/jactionlint.yaml") || !strings.Contains(got, "-baseline") {
		t.Errorf("the output must tell how to apply the baseline: %q", got)
	}
	got = run("-baseline-write=ci/baseline.json")
	if !strings.Contains(got, "baseline: ci/baseline.json") {
		t.Errorf("another file needs its path in the line: %q", got)
	}

	// Once the configuration applies it, nothing more is said
	if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte("baseline: auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run("-baseline-write"); strings.Contains(got, "baseline: auto") {
		t.Errorf("the configuration already applies the baseline: %q", got)
	}
}

func manyFindingsRepo(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("on: push\njobs:\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&b, "  j%d:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n", i)
	}
	return makeWorkflowRepo(t, map[string]string{"ci.yaml": b.String()})
}

func TestRunHintAfterManyFindings(t *testing.T) {
	root := manyFindingsRepo(t)
	t.Chdir(root)
	run := func(args ...string) (stdout, stderr string) {
		t.Helper()
		var out, errOut strings.Builder
		cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		cmd.Main(append([]string{"jactionlint", "-no-color", "-shellcheck=", "-pyflakes="}, args...))
		return out.String(), errOut.String()
	}
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("JACTIONLINT_NO_HINTS", "")

	// Neither a terminal nor CI: a script or a pipe reads the output
	if _, errOut := run("-profile", "default"); strings.Contains(errOut, "note:") {
		t.Errorf("no hint for a pipe: %q", errOut)
	}

	t.Setenv("CI", "true")
	_, errOut := run("-profile", "default")
	for _, want := range []string{"note: ", " findings in 1 file.", "-format summary", "-baseline-write", "-profile correctness", "-no-hints"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("the hint must contain %q: %q", want, errOut)
		}
	}
	if n := strings.Count(errOut, "silence this note"); n != 1 {
		t.Errorf("the hint is printed once but %d times in %q", n, errOut)
	}

	// Never in a structured format, and the flag and the variable silence it
	for _, args := range [][]string{{"-format", "json"}, {"-format", "sarif"}, {"-format", "summary"}, {"-format", "github"}, {"-no-hints"}} {
		if out, errOut := run(append([]string{"-profile", "default"}, args...)...); strings.Contains(errOut, "silence this note") || strings.Contains(out, "silence this note") {
			t.Errorf("%v: no hint expected: %q", args, errOut)
		}
	}
	t.Setenv("JACTIONLINT_NO_HINTS", "1")
	if _, errOut := run("-profile", "default"); strings.Contains(errOut, "silence this note") {
		t.Errorf("JACTIONLINT_NO_HINTS must silence the hint: %q", errOut)
	}
	t.Setenv("JACTIONLINT_NO_HINTS", "")

	// The checks of actionlint are what -profile correctness asks for: there is nothing to suggest
	if _, errOut := run("-profile", "correctness"); strings.Contains(errOut, "silence this note") {
		t.Errorf("no hint with the correctness profile: %q", errOut)
	}

	// With a baseline applied the advice to write one is left out, and -profile correctness is not suggested
	// to someone who already uses it
	if out, _ := run("-profile", "default", "-baseline-write"); !strings.Contains(out, "Wrote") {
		t.Fatal(out)
	}
	_, errOut = run("-profile", "default", "-baseline")
	if strings.Contains(errOut, "silence this note") {
		t.Errorf("everything is accepted by the baseline, so there is no hint: %q", errOut)
	}
}

// The workflows of this repository pass the default profile, which is what CI and hk run on them (dogfooding): every
// action is pinned to a commit SHA, permissions, timeouts and concurrency are explicit.
func TestOwnWorkflowsPassTheDefaultProfile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(".github", "workflows", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("workflows not found: %v %v", files, err)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{Profile: ProfileDefault})
	if err != nil {
		t.Fatal(err)
	}
	l.shellcheck, l.pyflakes = "", "" // the tools may be missing; they check the scripts and not the policy
	errs, err := l.LintFiles(files, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range errs {
		t.Errorf("%v", e)
	}
}

// A UTF-16 file has NUL bytes everywhere: one finding without a fix says that, instead of one finding per NUL whose
// fix would delete the NULs and damage the file (bug bash: 93 findings for a 93 character file).
func TestUTF16FileGetsOneInvisibleCharactersFinding(t *testing.T) {
	text := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	le := []byte{0xFF, 0xFE}
	be := []byte{0xFE, 0xFF}
	for _, r := range text {
		le = append(le, byte(r), 0)
		be = append(be, 0, byte(r))
	}
	for name, src := range map[string][]byte{"utf16le": le, "utf16be": be} {
		t.Run(name, func(t *testing.T) {
			errs := checkInvisibleCharacters(src, mustParseConfig(t, "rules:\n  invisible-characters: error\n"))
			if len(errs) != 1 {
				t.Fatalf("want one finding but got %d: %v", len(errs), errs)
			}
			e := errs[0]
			if e.Fix != nil || e.Line != 1 || !strings.Contains(e.Message, "UTF-16") || !strings.Contains(e.Message, "save it as UTF-8") {
				t.Errorf("unexpected finding %+v", e)
			}
		})
	}
	if errs := checkInvisibleCharacters([]byte("\xEF\xBB\xBF"+text), mustParseConfig(t, "rules:\n  invisible-characters: error\n")); len(errs) != 0 {
		t.Errorf("a UTF-8 byte order mark is not a finding here: %v", errs)
	}
}
