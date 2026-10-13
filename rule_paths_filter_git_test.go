package jactionlint

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// TestUnmatchedPathsFilterUsesTheFilesOfGit checks that an ignored tree (node_modules, which can have more files than
// the bound of the walk) is not a file of the repository for an entry of `paths`, and that a file which is not added
// yet is.
func TestUnmatchedPathsFilterUsesTheFilesOfGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	files := map[string]string{
		".gitignore":                 "node_modules/\n.venv/\n",
		"src/a.go":                   "",
		"node_modules/pkg/index.js":  "",
		".venv/lib/site.py":          "",
		"newdir/untracked-so-far.md": "",
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".gitignore", "src")
	paths := readRepoPaths(dir)
	if paths == nil {
		t.Fatal("the files were not listed")
	}
	has := func(p string) bool {
		i := sort.SearchStrings(paths.paths, p)
		return i < len(paths.paths) && paths.paths[i] == p
	}
	for p, want := range map[string]bool{
		"src/a.go": true, "src": true, "newdir/untracked-so-far.md": true, "newdir": true,
		"node_modules": false, "node_modules/pkg/index.js": false, ".venv/lib/site.py": false,
	} {
		if has(p) != want {
			t.Errorf("%q in the files: %v, want %v", p, has(p), want)
		}
	}
}

// TestUnmatchedPathsFilterWalkSkipsInstalledPackages checks the walk, which is used when git cannot list the files.
func TestUnmatchedPathsFilterWalkSkipsInstalledPackages(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"src/a.go", "node_modules/pkg/index.js", ".venv/lib/site.py"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := walkRepoPaths(dir) // not a git work tree
	if paths == nil {
		t.Fatal("the walk failed")
	}
	got := map[string]bool{}
	for _, p := range paths.paths {
		got[p] = true
	}
	if !got["src/a.go"] || got["node_modules"] || got["node_modules/pkg/index.js"] || got[".venv"] {
		t.Errorf("paths: %v", paths.paths)
	}
}
