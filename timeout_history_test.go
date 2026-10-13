//go:build !js

package jactionlint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const timeoutHistoryWorkflow = `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    strategy:
      matrix:
        os: [a, b]
    steps:
      - run: make test
  lint:
    name: Lint ${{ matrix.x }}
    runs-on: ubuntu-latest
    strategy:
      matrix:
        x: [one]
    steps:
      - run: make lint
  rare:
    runs-on: ubuntu-latest
    steps:
      - run: make rare
  never:
    runs-on: ubuntu-latest
    steps:
      - run: make never
  deploy:
    uses: ./.github/workflows/deploy.yaml
`

func TestTimeoutJobSpecMatches(t *testing.T) {
	parse := func(src string) map[string]*timeoutJobSpec {
		w, _ := Parse([]byte("on: push\njobs:\n" + src))
		specs := map[string]*timeoutJobSpec{}
		for id, j := range w.Jobs {
			specs[id] = newTimeoutJobSpec(id, j)
		}
		return specs
	}
	specs := parse(`  a:
    runs-on: x
    steps: [{run: x}]
  m:
    runs-on: x
    strategy: {matrix: {os: [a]}}
    steps: [{run: x}]
  named:
    name: Build it
    runs-on: x
    steps: [{run: x}]
  dyn:
    name: Build ${{ matrix.os }} (${{ matrix.v }})
    runs-on: x
    strategy: {matrix: {os: [a], v: [1]}}
    steps: [{run: x}]
  nm:
    name: Run
    runs-on: x
    strategy: {matrix: {os: [a]}}
    steps: [{run: x}]
  call:
    uses: ./.github/workflows/c.yaml
`)
	tests := []struct {
		job, name string
		want      bool
	}{
		{"a", "a", true},
		{"a", "a (x)", false}, // not a matrix
		{"m", "m", true},
		{"m", "m (a, 1)", true},
		{"m", "m2 (a)", false},
		{"named", "Build it", true},
		{"named", "named", false},        // the name wins over the ID
		{"named", "Build it (x)", false}, // not a matrix
		{"nm", "Run (ubuntu-latest)", true},
		{"nm", "Run", true},
		{"nm", "Running (a)", false},
		{"dyn", "Build linux (3.11) (x)", true},
		{"dyn", "Build linux (3.11)", true},
		{"dyn", "Build ", false},
		{"call", "call / inner job", true},
		{"call", "call", true},
		{"call", "caller / x", false},
	}
	for _, tc := range tests {
		if got := specs[tc.job].matches(tc.name); got != tc.want {
			t.Errorf("job %s matches %q: %v, want %v", tc.job, tc.name, got, tc.want)
		}
	}
}

// runJobsJSON makes the answer of the API for the jobs of a run: name, conclusion and the duration in seconds.
func runJobsJSON(jobs ...[3]string) string {
	start := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	var parts []string
	for _, j := range jobs {
		var d int
		fmt.Sscan(j[2], &d)
		parts = append(parts, fmt.Sprintf(`{"name":%q,"conclusion":%q,"started_at":%q,"completed_at":%q}`, j[0], j[1], start.Format(time.RFC3339), start.Add(time.Duration(d)*time.Second).Format(time.RFC3339)))
	}
	return fmt.Sprintf(`{"total_count":%d,"jobs":[%s]}`, len(parts), strings.Join(parts, ","))
}

func timeoutHistoryAPI(t *testing.T) *fakeGitHub {
	t.Helper()
	f := newFakeGitHub(t)
	f.handle("/repos/o/r/actions/workflows/ci.yaml/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "success" || r.URL.Query().Get("per_page") != "5" {
			t.Errorf("the query of the runs: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"total_count":4,"workflow_runs":[
			{"id":4,"created_at":"2026-10-04T08:00:00Z","event":"push"},
			{"id":3,"created_at":"2026-10-03T08:00:00Z","event":"push"},
			{"id":2,"created_at":"2026-10-02T08:00:00Z","event":"pull_request"},
			{"id":1,"created_at":"2026-09-30T23:30:00Z","event":"push"}]}`)
	})
	runs := map[int][][3]string{
		1: {{"build", "success", "60"}, {"test (a)", "success", "600"}, {"test (b)", "success", "300"}, {"Lint one", "success", "30"}, {"rare", "success", "100"}, {"deploy / publish", "success", "50"}, {"Custom name", "success", "10"}},
		2: {{"build", "success", "120"}, {"test (a)", "success", "660"}, {"test (b)", "success", "320"}, {"Lint one", "success", "40"}, {"rare", "success", "130"}, {"build", "failure", "9999"}, {"test (a)", "skipped", "0"}},
		3: {{"build", "success", "90"}, {"test (a)", "success", "630"}, {"test (b)", "success", "310"}, {"Lint one", "success", "35"}, {"Custom name", "success", "10"}},
		4: {{"build", "success", "200"}, {"test (a)", "success", "900"}, {"test (b)", "success", "330"}},
	}
	for id, jobs := range runs {
		f.json(fmt.Sprintf("/repos/o/r/actions/runs/%d/jobs", id), runJobsJSON(jobs...))
	}
	return f
}

func TestSuggestTimeouts(t *testing.T) {
	f := timeoutHistoryAPI(t)
	root, _ := onlineProject(t, timeoutHistoryWorkflow, "")
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	t.Chdir(root)

	status, out, stderr := runOnlineCommand(t, "--suggest-timeouts", "--timeout-history-runs", "5", "--format", "json")
	if status != 0 {
		t.Fatalf("status %d: %s", status, stderr)
	}
	var rep TimeoutReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if rep.Version != 1 || rep.Repository != "o/r" || rep.Runs != 5 || rep.Buffer != 2 || !rep.Complete || len(rep.Workflows) != 1 {
		t.Fatalf("report: %+v", rep)
	}
	wf := rep.Workflows[0]
	if wf.Path != ".github/workflows/ci.yaml" || wf.Runs != 4 || wf.Oldest != "2026-09-30" || wf.Newest != "2026-10-04" {
		t.Errorf("workflow: %+v", wf)
	}
	f64 := func(v float64) *float64 { return &v }
	type row struct {
		kind             string
		declared         *float64
		legs, samples    int
		min, median, max *float64
		suggested        int
		note             string
	}
	got := map[string]row{}
	for _, j := range wf.Jobs {
		got[j.ID] = row{j.Kind, j.DeclaredMinutes, j.Legs, j.Samples, j.MinMinutes, j.MedianMinutes, j.MaxMinutes, j.SuggestedMinutes, j.Note}
	}
	want := map[string]row{
		// 60, 90, 120 and 200 seconds: the failed run of the job is not a sample
		"build": {"job", nil, 1, 4, f64(1), f64(1.8), f64(3.3), 7, ""},
		// two legs, 4 runs each (8 samples; the skipped execution is not one), the longest is 900 seconds = 15 minutes
		"test": {"matrix", f64(20), 2, 8, f64(5), f64(7.8), f64(15), 30, ""},
		// the name has an expression
		"lint": {"matrix", nil, 1, 3, f64(0.5), f64(0.6), f64(0.7), 2, ""},
		// two samples: too few
		"rare":   {"job", nil, 1, 2, f64(1.7), f64(1.9), f64(2.2), 0, "only 2 sample(s): too few for a suggestion"},
		"never":  {"job", nil, 0, 0, nil, nil, nil, 0, "no successful execution of this job on record"},
		"deploy": {"reusable-workflow", nil, 0, 0, nil, nil, nil, 0, "a call of a reusable workflow: the timeouts belong to the jobs of the called workflow, which this report does not follow"},
	}
	if d := cmp.Diff(want, got, cmp.AllowUnexported(row{})); d != "" {
		t.Errorf("jobs (-want +got): %s", d)
	}
	if d := cmp.Diff([]TimeoutUnmatched{{Name: "Custom name", Samples: 2}}, wf.Unmatched); d != "" {
		t.Errorf("unmatched (-want +got): %s", d)
	}
	if rep.LargestSuggestedMinutes != 30 {
		t.Errorf("the largest suggestion: %d", rep.LargestSuggestedMinutes)
	}
	// One request for the runs and one for the jobs of each run (the other requests are the client asking whether the
	// repository is public, to share what it fetched with a token)
	api := 0
	f.mu.Lock()
	for p, n := range f.hits {
		if strings.Contains(p, "/actions/") {
			api += n
		}
	}
	f.mu.Unlock()
	if api != 5 || f.count("/repos/o/r/actions/workflows/ci.yaml/runs") != 1 {
		t.Errorf("requests for the runs: %d", api)
	}

	// A different buffer
	status, out, _ = runOnlineCommand(t, "--suggest-timeouts", "--timeout-history-runs", "5", "--timeout-buffer", "1.5", "--format", "json")
	if status != 0 || !strings.Contains(out, `"suggested_minutes": 5`) { // 200 seconds = 3.33 minutes, times 1.5
		t.Errorf("buffer 1.5: status %d\n%s", status, out)
	}
}

func TestSuggestTimeoutsText(t *testing.T) {
	f := timeoutHistoryAPI(t)
	root, _ := onlineProject(t, timeoutHistoryWorkflow, "")
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	t.Chdir(root)

	status, out, stderr := runOnlineCommand(t, "--suggest-timeouts", "--timeout-history-runs", "5")
	if status != 0 || stderr != "" {
		t.Fatalf("status %d: %s", status, stderr)
	}
	for _, want := range []string{
		".github/workflows/ci.yaml (4 runs, 2026-09-30 to 2026-10-04)",
		"build",
		"timeout-minutes not set",
		"samples 4",
		"max 3.3",
		"suggested 7",
		"test (matrix, 2 legs)",
		"timeout-minutes: 20",
		"suggested 30",
		"only 2 sample(s): too few for a suggestion",
		"deploy (calls a workflow)",
		`not matched to a job of the file: "Custom name" (2 samples`,
		"The largest suggestion is 30 minutes",
		"a single default-minutes of the rule missing-timeout",
		"it is what happened in the runs on record, not a bound on the next one",
		"jactionlint writes none of these numbers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not contain %q:\n%s", want, out)
		}
	}
	// The workflow is not touched
	if b, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yaml")); string(b) != timeoutHistoryWorkflow {
		t.Errorf("the workflow was changed")
	}
}

func TestSuggestTimeoutsFailures(t *testing.T) {
	f := newFakeGitHub(t) // answers 404 for the workflow: GitHub does not know it
	root, _ := onlineProject(t, timeoutHistoryWorkflow, "")
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	t.Chdir(root)

	status, out, _ := runOnlineCommand(t, "--suggest-timeouts")
	if status != 0 || !strings.Contains(out, "GitHub has no runs for this file") || strings.Contains(out, "suggested") {
		t.Errorf("a workflow GitHub does not know: status %d\n%s", status, out)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--suggest-timeouts", "--no-online"}, "cannot be combined with --no-online"},
		{[]string{"--suggest-timeouts", "--timeout-history-runs", "0"}, "between 1 and 100"},
		{[]string{"--suggest-timeouts", "--timeout-history-runs", "101"}, "between 1 and 100"},
		{[]string{"--suggest-timeouts", "--timeout-buffer", "0.5"}, "at least 1"},
		{[]string{"--timeout-buffer", "3"}, "only with --suggest-timeouts"},
		{[]string{"--suggest-timeouts", "--format", "sarif"}, "text and json only"},
		{[]string{"--suggest-timeouts", "--fix"}, "only reads"},
		{[]string{"--suggest-timeouts", "-"}, "stdin"},
	} {
		if status, _, stderr := runOnlineCommand(t, c.args...); status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, c.want) {
			t.Errorf("%v: status %d: %s", c.args, status, stderr)
		}
	}
	t.Setenv("GITHUB_REPOSITORY", "")
	if status, _, stderr := runOnlineCommand(t, "--suggest-timeouts"); status != ExitStatusFailure || !strings.Contains(stderr, "could not tell the GitHub repository") {
		t.Errorf("no repository: status %d: %s", status, stderr)
	}
}

// A name that is only an expression fits every name of a run, so it must fit none: it would take the durations of the
// other jobs, and the names that nothing else matches would count as its samples.
func TestSuggestTimeoutsNameThatIsOnlyAnExpression(t *testing.T) {
	w, _ := Parse([]byte("on: push\njobs:\n  os:\n    name: ${{ matrix.os }}\n    runs-on: x\n    strategy: {matrix: {os: [a]}}\n    steps: [{run: x}]\n  both:\n    name: \"${{ matrix.a }}${{ matrix.b }}\"\n    runs-on: x\n    steps: [{run: x}]\n"))
	for id, j := range w.Jobs {
		sp := newTimeoutJobSpec(id, j)
		for _, name := range []string{"ubuntu-latest", "build", id, "os (a)"} {
			if sp.matches(name) {
				t.Errorf("job %s matches %q by an expression alone", id, name)
			}
		}
	}

	const fixtures = `{"repos":{"o/r":{"workflow_runs":{"ci.yaml":{
		"runs":[{"id":1,"created_at":"2026-10-01T00:00:00Z"},{"id":2,"created_at":"2026-10-02T00:00:00Z"},{"id":3,"created_at":"2026-10-03T00:00:00Z"}],
		"jobs":{
			"1":{"jobs":[{"name":"build","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:01:00Z"},{"name":"ubuntu-latest","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:50:00Z"}]},
			"2":{"jobs":[{"name":"build","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:02:00Z"},{"name":"ubuntu-latest","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:50:00Z"}]},
			"3":{"jobs":[{"name":"build","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:03:00Z"},{"name":"ubuntu-latest","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:50:00Z"}]}}}}}}}`
	client, err := NewFixtureGitHubClient([]byte(fixtures))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := onlineProject(t, "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n  matrix:\n    name: ${{ matrix.os }}\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n    steps:\n      - run: make\n", "")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root, Online: true, GitHubClient: client})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := l.SuggestTimeouts(nil, nil, TimeoutOptions{Runs: 3})
	if err != nil {
		t.Fatal(err)
	}
	wf := rep.Workflows[0]
	byID := map[string]*TimeoutJob{}
	for _, j := range wf.Jobs {
		byID[j.ID] = j
	}
	if b := byID["build"]; b.Samples != 3 || *b.MaxMinutes != 3 || b.SuggestedMinutes != 6 {
		t.Errorf("build has only its own runs: %+v", b)
	}
	if m := byID["matrix"]; m.Samples != 0 || m.SuggestedMinutes != 0 || !strings.Contains(m.Note, "only an expression") {
		t.Errorf("the job named by an expression has no numbers and says why: %+v", m)
	}
	if d := cmp.Diff([]TimeoutUnmatched{{Name: "ubuntu-latest", Samples: 3}}, wf.Unmatched); d != "" {
		t.Errorf("unmatched (-want +got): %s", d)
	}
}

func TestSuggestTimeoutsFixtureClient(t *testing.T) {
	fixtures := `{"repos":{"o/r":{"workflow_runs":{"ci.yaml":{
		"runs":[{"id":1,"created_at":"2026-10-01T00:00:00Z"},{"id":2,"created_at":"2026-10-02T00:00:00Z"},{"id":3,"created_at":"2026-10-03T00:00:00Z"}],
		"jobs":{"1":{"jobs":[{"name":"build","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:01:00Z"}]},
		        "2":{"jobs":[{"name":"build","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:02:00Z"}]},
		        "3":{"jobs":[{"name":"build","conclusion":"success","started_at":"2026-10-01T10:00:00Z","completed_at":"2026-10-01T10:05:00Z"}]}}}}}}}`
	client, err := NewFixtureGitHubClient([]byte(fixtures))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := onlineProject(t, "on: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n", "")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root, Online: true, GitHubClient: client})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := l.SuggestTimeouts(nil, nil, TimeoutOptions{Runs: 3})
	if err != nil {
		t.Fatal(err)
	}
	j := rep.Workflows[0].Jobs[0]
	if j.Samples != 3 || j.SuggestedMinutes != 10 || *j.MaxMinutes != 5 || !rep.Complete {
		t.Errorf("%+v", j)
	}
}

// A run whose jobs cannot be read is counted and said, and the numbers are from the others.
func TestSuggestTimeoutsIncomplete(t *testing.T) {
	f := timeoutHistoryAPI(t)
	f.handle("/repos/o/r/actions/runs/2/jobs", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Resource not accessible by integration"}`, http.StatusForbidden)
	})
	root, _ := onlineProject(t, timeoutHistoryWorkflow, "")
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	t.Chdir(root)

	status, out, stderr := runOnlineCommand(t, "--suggest-timeouts", "--timeout-history-runs", "5", "--format", "json")
	if status != 0 {
		t.Fatalf("status %d: %s", status, stderr)
	}
	var rep TimeoutReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Complete || rep.SkippedLookups == 0 {
		t.Errorf("a failed lookup must make the report incomplete: %+v", rep)
	}
	for _, j := range rep.Workflows[0].Jobs {
		if j.ID == "build" && j.Samples != 3 { // the run 2 (120 seconds) is missing
			t.Errorf("build has %d samples", j.Samples)
		}
	}
	if status, _, stderr := runOnlineCommand(t, "--suggest-timeouts", "--timeout-history-runs", "5", "--online=strict"); status != ExitStatusFailure || !strings.Contains(stderr, "were skipped") {
		t.Errorf("--online=strict: status %d: %s", status, stderr)
	}
	_, text, _ := runOnlineCommand(t, "--suggest-timeouts", "--timeout-history-runs", "5")
	if !strings.Contains(text, "Incomplete: ") {
		t.Errorf("the text says nothing:\n%s", text)
	}
}
