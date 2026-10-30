package runner

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RunDir is the scratch area of one run, under <repo>/.jactionlint/run/<id>: the copy of the workspace the steps work
// in, the fake home, the temp directories and the files the steps use to talk to the runner.
type RunDir struct {
	Root string // .jactionlint/run/<id>
	Work string // copy of the repository (GITHUB_WORKSPACE)
	Home string // HOME of the steps
	Temp string // RUNNER_TEMP, also TMPDIR
	// Files holds GITHUB_ENV, GITHUB_PATH, GITHUB_OUTPUT, GITHUB_STATE and GITHUB_STEP_SUMMARY of each step.
	Files string
	// ToolCache is persistent and shared between runs (RUNNER_TOOL_CACHE).
	ToolCache string
}

// StateDir returns the persistent directory of jactionlint (the cache service, the tool cache, action checkouts):
// $XDG_CACHE_HOME/jactionlint, or ~/.cache/jactionlint, or the platform's cache directory.
func StateDir() (string, error) {
	if d := os.Getenv("JACTIONLINT_CACHE_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "jactionlint"), nil
}

// NewRunDir creates the scratch area for a run of the repository at repoRoot.
func NewRunDir(repoRoot string) (*RunDir, error) {
	state, err := StateDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(repoRoot, ".jactionlint")
	if err := os.MkdirAll(filepath.Join(base, "run"), 0o755); err != nil {
		return nil, err
	}
	// Keep the area out of git without touching the repository's own ignore files.
	if err := os.WriteFile(filepath.Join(base, ".gitignore"), []byte("*\n"), 0o644); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	root, err := os.MkdirTemp(filepath.Join(base, "run"), time.Now().Format("20060102-150405-"))
	if err != nil {
		return nil, err
	}
	rd := &RunDir{
		Root: root, Work: filepath.Join(root, "work"), Home: filepath.Join(root, "home"),
		Temp: filepath.Join(root, "temp"), Files: filepath.Join(root, "files"),
		ToolCache: filepath.Join(state, "toolcache", runtime.GOOS+"-"+runtime.GOARCH),
	}
	for _, d := range []string{rd.Work, rd.Home, rd.Temp, rd.Files, rd.ToolCache,
		filepath.Join(rd.Home, ".config"), filepath.Join(rd.Home, ".cache"), filepath.Join(rd.Home, ".local", "share"), filepath.Join(rd.Home, ".local", "state")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return rd, nil
}

// Remove deletes the scratch area (not the tool cache).
func (r *RunDir) Remove() error { return os.RemoveAll(r.Root) }

// BaseEnv is the environment every step starts from, before the GITHUB_* variables and the step's own env.
func (r *RunDir) BaseEnv() map[string]string {
	return map[string]string{
		"HOME":                 r.Home,
		"XDG_CONFIG_HOME":      filepath.Join(r.Home, ".config"),
		"XDG_CACHE_HOME":       filepath.Join(r.Home, ".cache"),
		"XDG_DATA_HOME":        filepath.Join(r.Home, ".local", "share"),
		"XDG_STATE_HOME":       filepath.Join(r.Home, ".local", "state"),
		"TMPDIR":               r.Temp,
		"RUNNER_TEMP":          r.Temp,
		"RUNNER_TOOL_CACHE":    r.ToolCache,
		"AGENT_TOOLSDIRECTORY": r.ToolCache,
		"CI":                   "true",
		"GITHUB_ACTIONS":       "true",
	}
}

// CopyWorkspace fills dst with what a fresh checkout of the repository at src plus its working changes would hold:
// the tracked files and the untracked files that git does not ignore, and the .git directory. Without git it copies
// everything. The .jactionlint directory is never copied. Files are cloned where the file system can (btrfs, xfs,
// APFS) and copied otherwise; they are never hard-linked, because a step that writes to a link would change the original.
func CopyWorkspace(src, dst string) error {
	files, err := listWorkspaceFiles(src)
	if err != nil {
		return err
	}
	for _, rel := range files {
		if err := copyEntry(filepath.Join(src, rel), filepath.Join(dst, rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if st, err := os.Stat(filepath.Join(src, ".git")); err == nil && st.IsDir() {
		return copyTree(filepath.Join(src, ".git"), filepath.Join(dst, ".git"))
	} else if err == nil {
		// A .git file (a linked work tree or a submodule) points outside the repository; the copy is a plain directory.
		return nil
	}
	return nil
}

func listWorkspaceFiles(src string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(src, ".git")); err == nil {
		cmd := exec.Command("git", "-C", src, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
		out, err := cmd.Output()
		if err == nil {
			seen := map[string]bool{}
			var files []string
			for _, f := range strings.Split(string(out), "\x00") {
				if f == "" || seen[f] || f == ".jactionlint" || strings.HasPrefix(f, ".jactionlint/") {
					continue
				}
				seen[f] = true
				files = append(files, filepath.FromSlash(f))
			}
			return files, nil
		}
	}
	var files []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if rel == ".jactionlint" || rel == ".git" {
			return fs.SkipDir
		}
		if !d.IsDir() {
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyEntry(p, target)
	})
}

// copyEntry copies a file, a symlink (as a link) or skips anything else (sockets, devices).
func copyEntry(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	switch {
	case st.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		os.Remove(dst)
		return os.Symlink(target, dst)
	case st.Mode().IsRegular():
		if cloneFile(src, dst) == nil {
			return os.Chmod(dst, st.Mode().Perm())
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, st.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return fmt.Errorf("copy %s: %w", src, err)
		}
		return out.Close()
	}
	return nil
}
