package jactionlint

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUnmatchedPathsFilterWalkSkipsJujutsu checks that the directory of a Jujutsu workspace is not a part of the
// repository: it does not satisfy an entry and does not count toward the bound.
func TestUnmatchedPathsFilterWalkSkipsJujutsu(t *testing.T) {
	defer func(n int) { pathsFilterMaxEntries = n }(pathsFilterMaxEntries)
	pathsFilterMaxEntries = 5
	dir := t.TempDir()
	files := []string{"src/a.go"}
	for i := 0; i < 20; i++ {
		files = append(files, filepath.Join(".jj", "repo", "store", "obj"+string(rune('a'+i))))
	}
	for _, f := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := walkRepoPaths(dir) // no .git: the walk
	if paths == nil {
		t.Fatal("the files of .jj count toward the bound")
	}
	if m := compilePathFilter("**/store"); paths.matches(m) {
		t.Errorf("a path of .jj satisfies the entry: %v", paths.paths)
	}
	if !paths.matches(compilePathFilter("src/**")) {
		t.Errorf("the files of the repository are missing: %v", paths.paths)
	}
}

func TestIsRepositoryInternal(t *testing.T) {
	for p, want := range map[string]bool{
		".git": true, ".git/config": true, ".jj/repo/store": true, ".jj": true,
		".github/workflows/ci.yaml": false, ".gitignore": false, "src/.git/x": false, "a/.jj": false,
	} {
		if isRepositoryInternal(p) != want {
			t.Errorf("%q: %v, want %v", p, isRepositoryInternal(p), want)
		}
	}
}
