package jactionlint

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestUnmatchedPathsFilterWalkSaysNothingWhenADirectoryCannotBeRead checks that a tree that is not fully seen is not
// matched: an entry that is about a file in the directory which cannot be read is not dead.
func TestUnmatchedPathsFilterWalkSaysNothingWhenADirectoryCannotBeRead(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the permissions of a directory do not stop this user")
	}
	dir := t.TempDir()
	for _, f := range []string{"src/a.go", "secret/b.go"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "secret"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(dir, "secret"), 0o755)
	if p := walkRepoPaths(dir); p != nil {
		t.Errorf("the tree was not fully read and must not be matched: %v", p.paths)
	}
}
