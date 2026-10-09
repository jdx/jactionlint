package jactionlint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Generators for the scaling regression tests and benchmarks. Each returns a workflow with n steps.

func perfHeader() *strings.Builder {
	var b strings.Builder
	b.WriteString("name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n")
	return &b
}

func perfWorkflowExprs(n int) string {
	b := perfHeader()
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "      - name: s%d\n        id: i%d\n        run: echo ${{ steps.i%d.outputs.x }}\n", i, i, max(0, i-1))
	}
	return b.String()
}

func perfWorkflowIgnores(n int) string {
	b := perfHeader()
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "      - run: echo %d # jactionlint ignore=shellcheck\n", i)
	}
	return b.String()
}

func perfWorkflowZizmorIgnores(n int) string {
	b := perfHeader()
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "      - uses: actions/checkout@v4 # zizmor: ignore[unpinned-uses]\n")
	}
	return b.String()
}

func perfWorkflowMatrix(n int) string {
	var b strings.Builder
	b.WriteString("name: t\non: push\njobs:\n  a:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n        include:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "          - os: ubuntu-latest\n            v: %d\n", i)
	}
	b.WriteString("    steps:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "      - run: echo ${{ matrix.v }} ${{ github.event.pull_request.title }}\n")
	}
	return b.String()
}

func perfLint(tb testing.TB, profile Profile, src string) []*Error {
	tb.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		tb.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{Profile: profile})
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		tb.Fatal(err)
	}
	return errs
}

// TestScalingBudget guards against quadratic behavior in the number of steps. The budgets are an
// order of magnitude above the time with the race detector so that slow CI machines do not flake, but
// they are below what the quadratic code needed (4000 steps took 73 s for the expressions and 45 s for
// the ignore comments; the sizes of the benchmarks below show the growth).
func TestScalingBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling budget tests are skipped in short mode")
	}
	tests := []struct {
		name    string
		profile Profile
		src     string
		budget  time.Duration
	}{
		{"expressions pedantic 4k steps", ProfilePedantic, perfWorkflowExprs(4000), 20 * time.Second},
		{"ignore comments 4k steps", ProfileCorrectness, perfWorkflowIgnores(4000), 20 * time.Second},
		{"zizmor ignore comments 4k steps", ProfileDefault, perfWorkflowZizmorIgnores(4000), 20 * time.Second},
		{"matrix include pedantic 2k", ProfilePedantic, perfWorkflowMatrix(2000), 20 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			done := make(chan struct{})
			go func() {
				defer close(done)
				perfLint(t, tc.profile, tc.src)
			}()
			select {
			case <-done:
			case <-time.After(tc.budget):
				t.Fatalf("linting did not finish within %s", tc.budget)
			}
			t.Logf("took %s", time.Since(start))
		})
	}
}

func benchmarkLint(b *testing.B, profile Profile, gen func(int) string, sizes ...int) {
	for _, n := range sizes {
		src := gen(n)
		b.Run(fmt.Sprintf("steps=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				perfLint(b, profile, src)
			}
		})
	}
}

func BenchmarkLintExpressionsPedantic(b *testing.B) {
	benchmarkLint(b, ProfilePedantic, perfWorkflowExprs, 1000, 10000)
}

func BenchmarkLintInlineIgnores(b *testing.B) {
	benchmarkLint(b, ProfileCorrectness, perfWorkflowIgnores, 1000, 10000)
}

func BenchmarkLintZizmorIgnores(b *testing.B) {
	benchmarkLint(b, ProfileDefault, perfWorkflowZizmorIgnores, 1000, 10000)
}

func BenchmarkLintMatrixPedantic(b *testing.B) {
	benchmarkLint(b, ProfilePedantic, perfWorkflowMatrix, 1000, 5000)
}

// A repository with many workflows which wait for another one (workflow_run) used to read and parse all
// its workflows for each of them.
func TestScalingBudgetWorkflowRunRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling budget tests are skipped in short mode")
	}
	const n = 2000
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		src := fmt.Sprintf("name: W%d\non:\n  workflow_run:\n    workflows: [W%d]\n    types: [completed]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo %d\n", i, (i+1)%n, i)
		if err := os.WriteFile(filepath.Join(wfDir, fmt.Sprintf("w%d.yaml", i)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Error(err)
			return
		}
		l.defaultConfig = &Config{Profile: ProfilePedantic}
		if _, err := l.LintRepository(dir); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("linting did not finish within 20s")
	}
}

// Writing a baseline found the scope of each finding by scanning the file from the finding up and back
// down: quadratic in the number of findings.
func TestScalingBudgetBaselineWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling budget tests are skipped in short mode")
	}
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	for _, d := range []string{wfDir, filepath.Join(dir, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var b strings.Builder
	b.WriteString("name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n")
	for i := 0; i < 40000; i++ {
		fmt.Fprintf(&b, "      - name: s%d\n        uses: actions/checkout@v%d\n", i, i%7+1)
	}
	if err := os.WriteFile(filepath.Join(wfDir, "w.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: dir})
		if err != nil {
			t.Error(err)
			return
		}
		res, err := l.WriteBaseline(nil, "")
		if err != nil {
			t.Error(err)
			return
		}
		if res.Entries < 10000 {
			t.Errorf("want thousands of entries, got %d", res.Entries)
		}
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("writing the baseline did not finish within 15s")
	}
}
