package jactionlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// relockTimeout bounds `gh actions-lock --relock`, which resolves every action of the lock file on GitHub.
const relockTimeout = 10 * time.Minute

// relockInstallHint is added to the error when the extension is not installed.
const relockInstallHint = "install it with `gh extension install github/gh-actions-lock`"

// runGHActionsLockRelock runs `gh actions-lock --relock` in dir, the root of a repository. gh does the work and writes
// the lock file; jactionlint does not touch it. The command gets no stdin and cannot prompt.
func runGHActionsLockRelock(ctx context.Context, dir string) error {
	path, err := exec.LookPath("gh")
	if err != nil {
		return fmt.Errorf("the GitHub CLI (gh) was not found, so %s was not relocked: install it and the gh-actions-lock extension (%s)", actionsLockPath, relockInstallHint)
	}
	ctx, cancel := context.WithTimeout(ctx, relockTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "actions-lock", "--relock")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if strings.Contains(msg, "unknown command") {
			return fmt.Errorf("`gh actions-lock --relock` failed, so %s was not relocked: the gh-actions-lock extension is not installed (%s): %s", actionsLockPath, relockInstallHint, msg)
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("`gh actions-lock --relock` failed, so %s was not relocked: %s", actionsLockPath, msg)
	}
	return nil
}

// relockRoots lists the repository roots, once each, whose Actions lock file exists.
func relockRoots(roots []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range roots {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		if st, err := os.Stat(filepath.Join(r, filepath.FromSlash(actionsLockPath))); err == nil && st.Mode().IsRegular() {
			out = append(out, r)
		}
	}
	return out
}

// relock runs the relock in every root that has a lock file. It runs even when no reference changed.
func (l *Linter) relock(roots []string, uo UpdateOptions) error {
	run := uo.relock
	if run == nil {
		run = runGHActionsLockRelock
	}
	ctx := l.online.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for _, r := range relockRoots(roots) {
		fmt.Fprintf(l.logOut, "Running gh actions-lock --relock in %s\n", r)
		if err := run(ctx, r); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
