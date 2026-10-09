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
