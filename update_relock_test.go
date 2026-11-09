package jactionlint

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// runRelock runs the update in a repository, with a fake `gh actions-lock --relock`, and returns the directories it
// ran in and the error of the update.
func runRelock(t *testing.T, mode UpdateMode, lock bool, workflow string, fail error) ([]string, error) {
	t.Helper()
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "ci.yaml"), []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	if lock {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(actionsLockPath)), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, log bytes.Buffer
	client := updateFixtureClient(t, standardReleases(), standardAliases())
	l, err := NewLinter(&out, &LinterOptions{GitHubClient: client, Online: true, WorkingDir: dir, LogWriter: &log, Format: "oneline", Color: ColorOptionKindNever})
	if err != nil {
		t.Fatal(err)
	}
	var ran []string
	uo := UpdateOptions{
		Mode: mode,
		now:  func() time.Time { return updateNow },
		relock: func(_ context.Context, d string) error {
			ran = append(ran, d)
			return fail
		},
	}
	_, err = l.Update(nil, uo)
	return ran, err
}

func TestUpdateApplyRelocksWhenTheLockFileExists(t *testing.T) {
	ran, err := runRelock(t, UpdateApply, true, wf("example/action@v1.3.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 {
		t.Fatalf("want one relock, got %v", ran)
	}
}

func TestUpdateApplyRelocksWithoutChanges(t *testing.T) {
	ran, err := runRelock(t, UpdateApply, true, wf("./local"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 {
		t.Fatalf("want a relock although nothing changed, got %v", ran)
	}
}

func TestUpdateApplySkipsRelockWithoutLockFile(t *testing.T) {
	ran, err := runRelock(t, UpdateApply, false, wf("example/action@v1.3.0"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 {
		t.Fatalf("want no relock, got %v", ran)
	}
}

func TestUpdateReadOnlyModesNeverRelock(t *testing.T) {
	for _, mode := range []UpdateMode{UpdateReport, UpdateCheck, UpdateDiff} {
		ran, err := runRelock(t, mode, true, wf("example/action@v1.3.0"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(ran) != 0 {
			t.Errorf("mode %d relocked: %v", mode, ran)
		}
	}
}

func TestUpdateApplyReportsARelockFailure(t *testing.T) {
	_, err := runRelock(t, UpdateApply, true, wf("example/action@v1.3.0"), errors.New("boom"))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the relock error, got %v", err)
	}
}

func TestRelockRootsDedupes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(actionsLockPath)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := relockRoots([]string{dir, dir, "", t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != dir {
		t.Fatalf("got %v", got)
	}
}

func TestRelockRootsDedupesSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(actionsLockPath)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	got, err := relockRoots([]string{dir, link})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want one root, got %v", got)
	}
}

func TestRelockNamesTheFailingRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(actionsLockPath)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	l := &Linter{logOut: &bytes.Buffer{}}
	err := l.relock([]string{dir}, UpdateOptions{relock: func(context.Context, string) error { return errors.New("boom") }})
	if err == nil || !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the root and the cause, got %v", err)
	}
}

// The default runner calls `gh actions-lock --relock` in the root.
func TestRunGHActionsLockRelockArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as gh")
	}
	bin := t.TempDir()
	root := t.TempDir()
	rec := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\necho \"$@\" > " + rec + "\npwd >> " + rec + "\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := runGHActionsLockRelock(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(rec)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if lines[0] != "actions-lock --relock" {
		t.Errorf("args = %q", lines[0])
	}
	if real, _ := filepath.EvalSymlinks(root); len(lines) < 2 || lines[1] != real {
		t.Errorf("dir = %v, want %s", lines, real)
	}
}
