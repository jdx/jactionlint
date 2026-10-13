package jactionlint

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The scaling tests do not compare the time of a run with a number of seconds, which says more about
// the machine (a macOS runner with the race detector and a growing number of rules is several times slower
// than a laptop) than about the code. They lint a workload at a quarter of its size and at its size in the
// same process and compare the two: the time of linear code grows about four times, and of quadratic code
// sixteen times, whatever the speed of the machine. An absolute ceiling is left to stop a run which does not
// end at all.
const (
	scalingShrink = 4 // the reference workload is this many times smaller
	// scalingGrowth is how many times longer than the reference the full workload may take: between the
	// growth of linear code (4) and of quadratic code (16), with room for the noise of a CI runner.
	scalingGrowth = 12
	// scalingFloor is the time under which the growth is not looked at: a run this fast is no problem
	// whatever the code does, and its noise is larger than the growth.
	scalingFloor   = time.Second
	scalingCeiling = time.Minute
)

type scalingLimits struct {
	ceiling time.Duration
	growth  float64
	floor   time.Duration
}

var defaultScaling = scalingLimits{ceiling: scalingCeiling, growth: scalingGrowth, floor: scalingFloor}

// runWithin runs f and returns how long it took, or an error when it takes more than the ceiling.
func runWithin(ceiling time.Duration, f func()) (time.Duration, error) {
	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
		return time.Since(start), nil
	case <-time.After(ceiling):
		return 0, fmt.Errorf("did not finish within %s", ceiling)
	}
}

func (lim scalingLimits) growthError(small, full time.Duration) error {
	if full < lim.floor || float64(full) <= lim.growth*float64(small) {
		return nil
	}
	return fmt.Errorf("took %s at a size of 1/%d and %s at the size of the test: %.1f times longer, where linear code takes %d times and more than %.0f times is quadratic or worse",
		small.Round(time.Millisecond), scalingShrink, full.Round(time.Millisecond), float64(full)/float64(small), scalingShrink, lim.growth)
}

// check runs the workload which prepare(n) returns at n/scalingShrink and at n (prepare builds the input
// outside of the measured time), and fails when the larger one grows faster than the size. A run which
// grew too much is measured once more: a noisy runner can slow down one run, a quadratic does not get
// faster.
func (lim scalingLimits) check(n int, prepare func(n int) func()) (small, full time.Duration, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		if small, err = runWithin(lim.ceiling, prepare(n/scalingShrink)); err != nil {
			return small, full, fmt.Errorf("at a size of 1/%d: %w", scalingShrink, err)
		}
		if full, err = runWithin(lim.ceiling, prepare(n)); err != nil {
			return small, full, err
		}
		if err = lim.growthError(small, full); err == nil {
			break
		}
	}
	return small, full, err
}

func checkScaling(t *testing.T, n int, prepare func(n int) func()) {
	t.Helper()
	small, full, err := defaultScaling.check(n, prepare)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("took %s at a size of 1/%d and %s at the size of the test", small.Round(time.Millisecond), scalingShrink, full.Round(time.Millisecond))
}

// The check has to tell quadratic code from linear code, or the tests above pass whatever the code does.
func TestScalingCheckTellsQuadraticFromLinear(t *testing.T) {
	lim := scalingLimits{ceiling: 10 * time.Second, growth: scalingGrowth, floor: 10 * time.Millisecond}
	work := func(cost func(n int) time.Duration) func(n int) func() {
		return func(n int) func() { return func() { time.Sleep(cost(n)) } }
	}
	linear := work(func(n int) time.Duration { return time.Duration(n) * 100 * time.Microsecond })
	nlogn := work(func(n int) time.Duration {
		return time.Duration(float64(n) * 100 * float64(time.Microsecond) * (1 + float64(n)/400))
	})
	quadratic := work(func(n int) time.Duration { return time.Duration(n) * time.Duration(n) * 2 * time.Microsecond })
	if small, full, err := lim.check(400, linear); err != nil {
		t.Errorf("linear code is reported (%s, %s): %v", small, full, err)
	}
	if small, full, err := lim.check(400, nlogn); err != nil {
		t.Errorf("code which is a little worse than linear is reported (%s, %s): %v", small, full, err)
	}
	if small, full, err := lim.check(400, quadratic); err == nil {
		t.Errorf("quadratic code is not reported (%s, %s)", small, full)
	}
	hang := func(int) func() { return func() { time.Sleep(time.Second) } }
	short := scalingLimits{ceiling: 50 * time.Millisecond, growth: scalingGrowth, floor: scalingFloor}
	if _, _, err := short.check(400, hang); err == nil {
		t.Error("a run which does not end is not reported")
	}
}

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

// TestScalingBudget guards against quadratic behavior in the number of steps: linting the workload takes
// about as many times longer as the workload is larger (see scalingGrowth). The quadratic code needed 73 s
// for 4000 steps of expressions and 45 s for 4000 ignore comments; the sizes of the benchmarks below show
// the growth.
func TestScalingBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling budget tests are skipped in short mode")
	}
	tests := []struct {
		name    string
		profile Profile
		gen     func(n int) string
		n       int
	}{
		{"expressions pedantic 2k steps", ProfilePedantic, perfWorkflowExprs, 2000},
		{"ignore comments 4k steps", ProfileCorrectness, perfWorkflowIgnores, 4000},
		{"zizmor ignore comments 2k steps", ProfileDefault, perfWorkflowZizmorIgnores, 2000},
		{"matrix include pedantic 1k", ProfilePedantic, perfWorkflowMatrix, 1000},
		{"checkouts of a pull request pedantic 1.5k", ProfilePedantic, perfWorkflowCheckouts, 1500},
		{"jobs pedantic 1.5k", ProfilePedantic, perfWorkflowJobs, 1500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkScaling(t, tc.n, func(n int) func() {
				src := tc.gen(n)
				return func() { perfLint(t, tc.profile, src) }
			})
		})
	}
}

func perfWorkflowCheckouts(n int) string {
	b := perfHeader()
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n")
	}
	return "on: [pull_request_target]\n" + b.String()[len("name: t\non: push\n"):]
}

func perfWorkflowJobs(n int) string {
	var b strings.Builder
	b.WriteString("name: t\non: push\npermissions: {}\njobs:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  j%d:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write\n    steps:\n      - uses: actions/checkout@v4\n      - run: echo ${{ github.sha }}\n", i)
	}
	return b.String()
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
	checkScaling(t, 1000, func(n int) func() {
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
		return func() {
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			l.defaultConfig = &Config{Profile: ProfilePedantic}
			if _, err := l.LintRepository(dir); err != nil {
				t.Error(err)
			}
		}
	})
}

// Writing a baseline found the scope of each finding by scanning the file from the finding up and back
// down: quadratic in the number of findings.
func TestScalingBudgetBaselineWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling budget tests are skipped in short mode")
	}
	checkScaling(t, 20000, func(n int) func() {
		dir := t.TempDir()
		wfDir := filepath.Join(dir, ".github", "workflows")
		for _, d := range []string{wfDir, filepath.Join(dir, ".git")} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		var b strings.Builder
		b.WriteString("name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "      - name: s%d\n        uses: actions/checkout@v%d\n", i, i%7+1)
		}
		if err := os.WriteFile(filepath.Join(wfDir, "w.yaml"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return func() {
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
			if res.Entries < n/4 {
				t.Errorf("want an entry for a quarter of the %d steps, got %d", n, res.Entries)
			}
		}
	})
}

func BenchmarkLintCheckoutsPedantic(b *testing.B) {
	benchmarkLint(b, ProfilePedantic, perfWorkflowCheckouts, 1000, 4000)
}

func BenchmarkLintJobsPedantic(b *testing.B) {
	benchmarkLint(b, ProfilePedantic, perfWorkflowJobs, 1000, 4000)
}

// realWorldCorpus returns the workflows of testdata/realworld/dataset.zip (about 1500 files from public
// repositories), the closest thing to a very large repository the tests have.
func realWorldCorpus(tb testing.TB) (names []string, srcs [][]byte) {
	tb.Helper()
	zr, err := zip.OpenReader(filepath.Join("testdata", "realworld", "dataset.zip"))
	if err != nil {
		tb.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			tb.Fatal(err)
		}
		src, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			tb.Fatal(err)
		}
		names = append(names, f.Name)
		srcs = append(srcs, src)
	}
	return names, srcs
}

// BenchmarkLintRealWorldCorpus lints every workflow of the corpus once per iteration. It is the
// benchmark to compare allocation (B/op, allocs/op) of changes to the expression machinery.
func BenchmarkLintRealWorldCorpus(b *testing.B) {
	names, srcs := realWorldCorpus(b)
	for _, profile := range []Profile{ProfileDefault, ProfilePedantic} {
		b.Run(string(profile), func(b *testing.B) {
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				b.Fatal(err)
			}
			l.defaultConfig = withoutMissingTimeout(&Config{Profile: profile})
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j, src := range srcs {
					if _, err := l.Lint(names[j], src, nil); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
