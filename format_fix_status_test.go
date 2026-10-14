package jactionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixStatusWorkflow has a finding with a safe fix (missing-permissions), one with an unsafe fix (insecure-commands),
// one whose fix needs an option (missing-timeout) and one that is advice only (concurrency-limits of a push workflow).
func fixStatusWorkflow(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	wd := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "on: push\nenv:\n  ACTIONS_ALLOW_UNSECURE_COMMANDS: true\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	f := filepath.Join(wd, "ci.yaml")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestShowFixStatus(t *testing.T) {
	f := fixStatusWorkflow(t)

	t.Run("off by default", func(t *testing.T) {
		for _, format := range []string{"text", "github", "oneline", "gcc"} {
			if out := runDocURLs(t, "--format", format, f); strings.Contains(out, "no fix (") || strings.Contains(out, "fix: ") {
				t.Errorf("%s: the fix status is printed without --show-fix-status:\n%s", format, out)
			}
		}
	})

	t.Run("text: a line under each finding that can be fixed", func(t *testing.T) {
		out := runDocURLs(t, "--show-fix-status", f)
		for _, want := range []string{
			"  = fix: Add permissions: contents: read (apply it with --fix)\n",
			"  = fix: Remove ACTIONS_ALLOW_UNSECURE_COMMANDS (may change behavior: apply it with --fix=unsafe)\n",
			"  = no fix (option-required): set the missing-timeout option default-minutes to enable the fix",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("the output does not contain %q:\n%s", want, out)
			}
		}
		// The finding that is advice only has no line: it is not a fix that is missing
		for _, block := range strings.Split(out, "ci.yaml:") {
			if strings.Contains(block, "[concurrency-limits]") && strings.Contains(block, "  = ") {
				t.Errorf("an advice-only finding has a fix status:\n%s", block)
			}
		}
	})

	t.Run("oneline stays one line", func(t *testing.T) {
		out := runDocURLs(t, "--show-fix-status", "--format", "oneline", f)
		if strings.Contains(out, "= ") || strings.Count(out, "\n") != strings.Count(out, "ci.yaml:") {
			t.Errorf("a finding must stay one line:\n%s", out)
		}
	})

	t.Run("github: in the message", func(t *testing.T) {
		out := runDocURLs(t, "--show-fix-status", "--format", "github", f)
		if !strings.Contains(out, "%0Ano fix (option-required): set the missing-timeout option default-minutes") {
			t.Errorf("the message has no reason:\n%s", out)
		}
		if strings.Count(out, "\n") != strings.Count(out, "::error ")+strings.Count(out, "::warning ")+strings.Count(out, "::notice ") {
			t.Errorf("an annotation must stay one line:\n%s", out)
		}
	})

	t.Run("formats which have the status as data are unchanged", func(t *testing.T) {
		for _, format := range []string{"json", "sarif", "gcc", "summary"} {
			if runDocURLs(t, "--format", format, f) != runDocURLs(t, "--show-fix-status", "--format", format, f) {
				t.Errorf("%s changes with --show-fix-status", format)
			}
		}
	})
}
