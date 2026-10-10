package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const regressionSHA = "11bd71901bbe5b1630ceea73d27597364c9af683"

// Unfixed workflow with a fixable finding (missing-permissions) so that --diff
// and --fix have something to report.
const readOnlyWorkflow = "on: push\r\n" +
	"jobs:\r\n" +
	"  j:\r\n" +
	"    runs-on: ubuntu-latest\r\n" +
	"    steps:\r\n" +
	"      # keep me\r\n" +
	"      - uses: actions/checkout@" + regressionSHA + " # v4.2.2\r\n" +
	"      - run: echo hi\r\n"

func runRegression(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	code := cmd.Main(append([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes="}, args...))
	return int(code), stdout.String(), stderr.String()
}

func writeWorkflow(t *testing.T, content string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir) // keep the repository's own config out of the run
	path := filepath.Join(dir, "w.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// dirSnapshot returns every file under dir with its content and modification time.
func dirSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		snap[p] = string(b) + "\x00" + info.ModTime().String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestReadOnlyOutputsDoNotWriteSources(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{"check", nil, "[missing-timeout]", 1},
		{"check-json", []string{"--format", "json"}, `"missing-timeout"`, 1},
		{"sarif", []string{"--format", "sarif"}, `"ruleId"`, 1},
		{"diff", []string{"--diff"}, "+permissions:", 1},
		{"diff-sarif", []string{"--diff", "--format", "sarif"}, "", -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, path := writeWorkflow(t, readOnlyWorkflow)
			before := dirSnapshot(t, dir)
			code, stdout, stderr := runRegression(t, append(tc.args, path)...)
			if tc.wantCode >= 0 && code != tc.wantCode {
				t.Errorf("exit status = %d, want %d\nstdout: %s\nstderr: %s", code, tc.wantCode, stdout, stderr)
			}
			if !strings.Contains(stdout+stderr, tc.wantOut) {
				t.Errorf("output should contain %q\nstdout: %s\nstderr: %s", tc.wantOut, stdout, stderr)
			}
			after := dirSnapshot(t, dir)
			if len(before) != len(after) {
				t.Errorf("files created or removed: before=%d after=%d", len(before), len(after))
			}
			for p, want := range before {
				if after[p] != want {
					t.Errorf("%s was modified by a read-only run", p)
				}
			}
			if b, _ := os.ReadFile(path); string(b) != readOnlyWorkflow {
				t.Errorf("source bytes changed: %q", b)
			}
		})
	}
}

func TestDiffPlansFixWithoutApplying(t *testing.T) {
	_, path := writeWorkflow(t, readOnlyWorkflow)
	_, diff, _ := runRegression(t, "--diff", path)
	if !strings.Contains(diff, "+permissions:") {
		t.Fatalf("diff should plan the permissions fix: %q", diff)
	}
	if b, _ := os.ReadFile(path); string(b) != readOnlyWorkflow {
		t.Fatalf("--diff applied the fix: %q", b)
	}
	// The plan is real: applying it with --fix produces what the diff described,
	// and the SHA/version comment and unrelated comment survive with CRLF intact.
	if code, out, errOut := runRegression(t, "--fix", path); code != 0 && code != 1 {
		t.Fatalf("--fix failed: %d %s %s", code, out, errOut)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, s := range []string{"permissions:", "# keep me\r\n", "actions/checkout@" + regressionSHA + " # v4.2.2\r\n"} {
		if !strings.Contains(got, s) {
			t.Errorf("fixed file should contain %q: %q", s, got)
		}
	}
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("fix introduced bare LF: %q", got)
	}
}

func TestMigrateIgnoresKeepsVersionCommentOnUses(t *testing.T) {
	src := "on: push\r\n" +
		"jobs:\r\n" +
		"  j:\r\n" +
		"    runs-on: ubuntu-latest\r\n" +
		"    timeout-minutes: 5\r\n" +
		"    steps:\r\n" +
		"      # unrelated\r\n" +
		"      - name: x\r\n" +
		"        uses: actions/checkout@" + regressionSHA + " # v4.2.2 # zizmor: ignore[artipacked]\r\n" +
		"      - run: echo hi # plain trailing comment\r\n"
	_, path := writeWorkflow(t, src)
	var out bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	if code := cmd.Main([]string{"jactionlint", "--migrate-ignores", path}); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "uses: actions/checkout@"+regressionSHA+" # v4.2.2\r\n") {
		t.Errorf("version comment must stay on the uses: line: %q", got)
	}
	for _, s := range []string{"# unrelated\r\n", "# jactionlint ignore=artipacked\r\n", "# plain trailing comment\r\n"} {
		if !strings.Contains(got, s) {
			t.Errorf("result should contain %q: %q", s, got)
		}
	}
	if strings.Contains(got, "zizmor") {
		t.Errorf("zizmor comment should be migrated away: %q", got)
	}
	if strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Errorf("migration introduced bare LF: %q", got)
	}
}

func TestFixKeepsVersionCommentOnUses(t *testing.T) {
	_, path := writeWorkflow(t, readOnlyWorkflow)
	runRegression(t, "--fix", path)
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "uses: actions/checkout@"+regressionSHA+" # v4.2.2") {
		t.Errorf("--fix detached the SHA/version comment: %q", b)
	}
}
