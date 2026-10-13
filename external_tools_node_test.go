package jactionlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMissingNodeIsNoted checks that a run without node says that it did not check the scripts of
// actions/github-script, like it does for shellcheck and pyflakes.
func TestMissingNodeIsNoted(t *testing.T) {
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/github-script@v7
        with:
          script: return 1
      - uses: actions/github-script@v7
        with:
          script: |
            const n = ${{
              github.run_number
            }}
      - uses: actions/github-script@v7
        with:
          script: ${{ inputs.script }}
`
	f := filepath.Join(wd, "ci.yaml")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyPath(t)
	status, _, stderr := runToolCommand(t, "--shellcheck=", "--pyflakes=", f)
	if status >= ExitStatusInvalidCommandOption {
		t.Fatalf("a missing node must not break the run: status %d: %s", status, stderr)
	}
	// The script with an expression over several lines is not checked even when node is there
	want := "note: node 20 or later was not found, so it did not check 2 JavaScript scripts. install it, or turn the check off with \"rules: {github-script-syntax: off}\" in the configuration\n"
	if strings.Count(stderr, want) != 1 {
		t.Errorf("stderr should contain %q once: %q", want, stderr)
	}
}
