package jactionlint

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGitListingIgnoresTheEnvironmentOfGit checks that the files come from the project even when the process has
// variables that point git to another repository or to another index, as it has inside a hook of git.
func TestGitListingIgnoresTheEnvironmentOfGit(t *testing.T) {
	other := gitProject(t, map[string]string{"other/only-here.txt": ""}, []string{"add", "."})
	dir := gitProject(t, map[string]string{"src/a.go": ""}, []string{"add", "."})
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "index"))
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(other, ".git", "objects"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.quotepath")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	if _, err := os.Stat(filepath.Join(other, ".git")); err != nil {
		t.Fatal(err)
	}

	seen, st := gitListedPaths(dir)
	if st != gitListed {
		t.Fatalf("status %v", st)
	}
	if _, ok := seen["src/a.go"]; !ok {
		t.Errorf("the project is not listed: %v", seen)
	}
	if _, ok := seen["other/only-here.txt"]; ok {
		t.Errorf("the files of the other repository are listed: %v", seen)
	}
}

func TestGitListingDoesNotQuoteNonASCIINames(t *testing.T) {
	dir := gitProject(t, map[string]string{"docs/ünï.md": ""}, []string{"add", "."})
	// The configuration of the user can turn the quoting on, which is the default of git
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	seen, st := gitListedPaths(dir)
	if st != gitListed {
		t.Fatalf("status %v", st)
	}
	if _, ok := seen["docs/ünï.md"]; !ok {
		t.Errorf("the name is not listed as it is: %v", seen)
	}
}

func TestCleanGitEnv(t *testing.T) {
	env := cleanGitEnv([]string{
		"PATH=/bin", "HOME=/h", "GIT_DIR=/x", "GIT_WORK_TREE=/x", "GIT_INDEX_FILE=/i", "GIT_OBJECT_DIRECTORY=/o",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/a", "GIT_COMMON_DIR=/c", "GIT_NAMESPACE=n", "GIT_PREFIX=p", "GIT_CONFIG=f",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=k", "GIT_CONFIG_VALUE_0=v", "GIT_CEILING_DIRECTORIES=/c", "GIT_TERMINAL_PROMPT=0",
	})
	want := map[string]bool{"PATH=/bin": true, "HOME=/h": true, "GIT_CEILING_DIRECTORIES=/c": true, "GIT_TERMINAL_PROMPT=0": true}
	if len(env) != len(want) {
		t.Fatalf("env %v", env)
	}
	for _, e := range env {
		if !want[e] {
			t.Errorf("%q must be removed", e)
		}
	}
}
