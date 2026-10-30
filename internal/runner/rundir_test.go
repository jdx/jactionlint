package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, rel, body string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCopyWorkspaceGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is needed")
	}
	src := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", src, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	write(t, src, "tracked.txt", "tracked", 0o644)
	write(t, src, "bin/run.sh", "#!/bin/sh\n", 0o755)
	write(t, src, ".gitignore", "target/\n*.log\n", 0o644)
	write(t, src, "gone.txt", "will be deleted", 0o644)
	if err := os.Symlink("tracked.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	os.Remove(filepath.Join(src, "gone.txt"))
	write(t, src, "untracked.txt", "new", 0o644)
	write(t, src, "target/big.bin", "ignored", 0o644)
	write(t, src, "debug.log", "ignored", 0o644)
	write(t, src, ".jactionlint/run/old/work/x", "never copied", 0o644)

	dst := filepath.Join(t.TempDir(), "work")
	if err := CopyWorkspace(src, dst); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]bool{
		"tracked.txt": true, "untracked.txt": true, "bin/run.sh": true, ".gitignore": true, "link": true,
		"gone.txt": false, "target/big.bin": false, "debug.log": false, ".jactionlint/run/old/work/x": false,
		".git/HEAD": true,
	} {
		_, err := os.Lstat(filepath.Join(dst, rel))
		if (err == nil) != want {
			t.Errorf("%s: exists = %v, want %v", rel, err == nil, want)
		}
	}
	if st, _ := os.Stat(filepath.Join(dst, "bin/run.sh")); st == nil || st.Mode().Perm()&0o100 == 0 {
		t.Error("the executable bit was lost")
	}
	if l, _ := os.Readlink(filepath.Join(dst, "link")); l != "tracked.txt" {
		t.Errorf("link target = %q", l)
	}
	// a write in the copy must not reach the original
	if err := os.WriteFile(filepath.Join(dst, "tracked.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(src, "tracked.txt")); string(b) != "tracked" {
		t.Fatalf("the original changed: %q", b)
	}
	// and the copy is a working repository
	cmd := exec.Command("git", "-C", dst, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status in the copy: %v", err)
	}
	if string(out) == "" {
		t.Error("expected the changes to show in the copy")
	}
}

func TestCopyWorkspacePlain(t *testing.T) {
	src := t.TempDir()
	write(t, src, "a/b.txt", "b", 0o644)
	write(t, src, ".jactionlint/run/x", "skip", 0o644)
	dst := filepath.Join(t.TempDir(), "w")
	if err := CopyWorkspace(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a/b.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".jactionlint")); err == nil {
		t.Fatal(".jactionlint was copied")
	}
}

func TestNewRunDir(t *testing.T) {
	t.Setenv("JACTIONLINT_CACHE_DIR", t.TempDir())
	repo := t.TempDir()
	rd, err := NewRunDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{rd.Work, rd.Home, rd.Temp, rd.Files, rd.ToolCache, filepath.Join(rd.Home, ".config")} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Errorf("%s: %v", d, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(repo, ".jactionlint", ".gitignore")); string(b) != "*\n" {
		t.Errorf(".gitignore = %q", b)
	}
	env := rd.BaseEnv()
	if env["HOME"] != rd.Home || env["RUNNER_TEMP"] != rd.Temp || env["CI"] != "true" {
		t.Errorf("env = %v", env)
	}
	if err := rd.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rd.Root); err == nil {
		t.Error("run dir not removed")
	}
}
