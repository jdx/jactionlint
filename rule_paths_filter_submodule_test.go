package jactionlint

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitProject makes a git repository in a new directory with the files and runs the git commands on it.
func gitProject(t *testing.T, files map[string]string, commands ...[]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range commands {
		run(c...)
	}
	return dir
}

// lintPathsFilterIn lints the workflow in the project at dir and returns the lines of the findings of the rule.
func lintPathsFilterIn(t *testing.T, dir, workflow string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfig(t, "profile: correctness\nrules:\n  unmatched-paths-filter: warn\n")
	errs, err := l.Lint(filepath.Join(dir, ".github", "workflows", "ci.yaml"), []byte(workflow), &Project{root: dir})
	if err != nil {
		t.Fatal(err)
	}
	var ret []*Error
	for _, e := range errs {
		if e.ID == "unmatched-paths-filter" {
			ret = append(ret, e)
		}
	}
	return ret
}

func TestUnmatchedPathsFilterWithSubmodule(t *testing.T) {
	// git lists a submodule as its path (the gitlink), without the files in it
	dir := gitProject(t, map[string]string{
		".gitmodules":   "[submodule \"vendor/lib\"]\n\tpath = vendor/lib\n\turl = https://example.com/lib.git\n",
		"docs/guide.md": "",
	}, []string{"update-index", "--add", "--cacheinfo", "160000,1234567890123456789012345678901234567890,vendor/lib"})
	wf := "      - '*.md'\n" + // no .md at the root: it cannot reach into the submodule, so it is dead
		"      - '**/*.js'\n" + // could match inside the submodule: not decided
		"      - 'vendor/lib/**'\n" + // inside the submodule: not decided
		"      - 'vendor/**'\n" + // reaches into it: not decided
		"      - 'docs/*.md'\n" + // there
		"      - 'other/**'\n" + // dead
		"      - 'vendor/readme.txt'\n" // dead: not in the submodule and not at vendor/
	errs := lintPathsFilterIn(t, dir, sprintfPaths(wf))
	checkLines(t, errs, "4", "9", "10")
}

func sprintfPaths(entries string) string {
	return "on:\n  push:\n    paths:\n" + entries + "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
}

func TestUnmatchedPathsFilterSaysNothingWhenGitHasNothingToMatch(t *testing.T) {
	t.Run("no file at all", func(t *testing.T) {
		// Everything is ignored, so git lists nothing; the walk would find the files and report the dead entry
		dir := gitProject(t, map[string]string{".gitignore": "*\n", "a.txt": ""})
		if errs := lintPathsFilterIn(t, dir, sprintfPaths("      - 'missing/**'\n")); len(errs) != 0 {
			t.Errorf("unexpected: %v", errs)
		}
	})
	t.Run("more files than the bound", func(t *testing.T) {
		defer func(n int) { pathsFilterMaxEntries = n }(pathsFilterMaxEntries)
		pathsFilterMaxEntries = 3
		dir := gitProject(t, map[string]string{"a/1": "", "a/2": "", "b/1": "", "b/2": ""}, []string{"add", "."})
		if errs := lintPathsFilterIn(t, dir, sprintfPaths("      - 'missing/**'\n")); len(errs) != 0 {
			t.Errorf("unexpected: %v", errs)
		}
		if p := walkRepoPaths(dir); p != nil {
			t.Errorf("the repository is over the bound and must not be read: %v", p.paths)
		}
	})
	t.Run("the bound is not a reason to walk", func(t *testing.T) {
		defer func(n int) { pathsFilterMaxEntries = n }(pathsFilterMaxEntries)
		pathsFilterMaxEntries = 3
		dir := gitProject(t, map[string]string{"a/1": "", "a/2": "", "b/1": "", "b/2": ""})
		if _, st := gitListedPaths(dir); st != gitUnusable {
			t.Errorf("status %v, want gitUnusable", st)
		}
	})
	t.Run("not a work tree: the walk is used", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil { // not a repository
			t.Fatal(err)
		}
		if _, st := gitListedPaths(dir); st != gitUnavailable {
			t.Errorf("status %v, want gitUnavailable", st)
		}
	})
}
