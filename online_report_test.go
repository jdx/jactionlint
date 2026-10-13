//go:build !js

package jactionlint

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var updateOnlineReportGoldens = flag.Bool("update-online-report-goldens", false, "rewrite the expected files of testdata/online")

func runWithReport(t *testing.T, wf string, args ...string) (report map[string]any, raw string, status int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	status, _, _ = runOnlineCommand(t, append(append([]string{"--online-report", path}, args...), wf)...)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no report: %v", err)
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return report, string(b), status
}

func TestOnlineReportStates(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	githubFailingFor(f, "corp/private", http.StatusForbidden)
	setOnlineEnv(t, f, "tok")

	_, okWf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	_, badWf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")

	// Online checks off: a value, not a missing file
	if rep, _, _ := runWithReport(t, okWf); rep["status"] != "disabled" || rep["version"] != float64(1) || rep["answers"] != nil {
		t.Errorf("disabled: %v", rep)
	}

	// Checked: complete, every answer came from GitHub
	rep, _, status := runWithReport(t, okWf, "--online")
	ans, _ := rep["answers"].(map[string]any)
	if status != ExitStatusSuccessNoProblem || rep["status"] != "complete" || rep["skippedLookups"] != float64(0) || ans == nil || ans["network"].(float64) < 1 || ans["cache"] != float64(0) || ans["oldestFetched"] != nil {
		t.Errorf("complete: status %d %v", status, rep)
	}

	// The same lookups again are answered by the cache within the TTL, and the report says how old they may be
	rep, _, _ = runWithReport(t, okWf, "--online")
	ans, _ = rep["answers"].(map[string]any)
	if rep["status"] != "complete" || ans["cache"].(float64) < 1 || ans["network"] != float64(0) || ans["oldestFetched"] == nil || ans["cacheTtlSeconds"] != float64(3600) {
		t.Errorf("cached: %v", rep)
	}

	// The cache mode has no age limit
	rep, _, _ = runWithReport(t, okWf, "--online=cache")
	ans, _ = rep["answers"].(map[string]any)
	if rep["mode"] != "cache" || rep["evidence"] != "cache" || ans["cache"].(float64) < 1 || ans["cacheTtlSeconds"] != nil {
		t.Errorf("cache mode: %v", rep)
	}

	// A failed lookup: incomplete, with the reason; the exit status is unchanged (strict is tested elsewhere)
	rep, _, status = runWithReport(t, badWf, "--online")
	by, _ := rep["skippedBy"].(map[string]any)
	if status != ExitStatusSuccessNoProblem || rep["status"] != "incomplete" || rep["skippedLookups"].(float64) < 1 || by["forbidden"] != rep["skippedLookups"] {
		t.Errorf("incomplete: status %d %v", status, rep)
	}
	if status, _, _ := runOnlineCommand(t, "--online=strict", badWf); status != ExitStatusFailure {
		t.Errorf("--online=strict must still fail on a skipped lookup, got %d", status)
	}

	// Left out on purpose is not a failure
	rep, _, _ = runWithReport(t, badWf, "--online", "--online-deny", "corp/*")
	if rep["status"] != "complete" || rep["excludedRepositories"] != float64(1) || rep["skippedLookups"] != float64(0) {
		t.Errorf("excluded: %v", rep)
	}
}

func TestOnlineReportDoesNotChangeTheOutput(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	for _, format := range []string{"json", "jsonl", "sarif"} {
		_, without, _ := runOnlineCommand(t, "--online", "--format", format, wf)
		_, with, _ := runOnlineCommand(t, "--online", "--format", format, "--online-report", filepath.Join(t.TempDir(), "r.json"), wf)
		if format == "sarif" {
			continue // its run properties carry the coverage, which differs between the runs by the cache
		}
		if without != with {
			t.Errorf("--format %s changes with --online-report:\n%s\n%s", format, without, with)
		}
	}
	// JSON stays a bare array
	_, out, _ := runOnlineCommand(t, "--online", "--format", "json", wf)
	if !strings.HasPrefix(out, "[") {
		t.Errorf("json: %s", out)
	}
}

// TestOnlineCoverageGoldens pins the report and the SARIF run properties for an incomplete run which used cached
// answers. Regenerate with go test -run TestOnlineCoverageGoldens -update-online-report-goldens .
func TestOnlineCoverageGoldens(t *testing.T) {
	cov := &OnlineCoverage{
		Status: "incomplete", Mode: "online", SkippedLookups: 3, ExcludedRepositories: 1, Evidence: "network",
		SkippedBy: map[string]int{"rate limit": 2, "forbidden": 1},
		Answers:   &OnlineAnswers{Network: 4, Cache: 5, Stale: 1, OldestFetched: "2026-10-12T08:30:00Z", CacheTTLSeconds: 3600},
	}
	var rep bytes.Buffer
	b, _ := json.MarshalIndent(OnlineReport{Version: 1, OnlineCoverage: *cov}, "", "  ")
	rep.Write(append(b, '\n'))
	var sarif bytes.Buffer
	if err := (sarifPrinter{}).print(&sarif, nil, runInfo{online: cov}); err != nil {
		t.Fatal(err)
	}
	for file, have := range map[string]string{"coverage_report.json": rep.String(), "coverage.sarif": sarif.String()} {
		path := filepath.Join("testdata", "online", file)
		if *updateOnlineReportGoldens {
			if err := os.WriteFile(path, []byte(have), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(string(want), have); diff != "" {
			t.Errorf("%s (-want +got): %s", file, diff)
		}
	}
}

// The age bound that the report gives must never be tighter than the TTL the client enforces.
func TestOnlineAnswerStatsRoundTheTTLUp(t *testing.T) {
	for _, tc := range []struct {
		ttl  time.Duration
		want int
	}{
		{time.Hour, 3600}, {1500 * time.Millisecond, 2}, {500 * time.Millisecond, 1}, {time.Second, 1}, {0, 0},
	} {
		c := &httpGitHubClient{ttl: tc.ttl}
		if got := c.answerStats().CacheTTLSeconds; got != tc.want {
			t.Errorf("ttl %v: cacheTtlSeconds = %d, want %d", tc.ttl, got, tc.want)
		}
	}
	if got := (&httpGitHubClient{ttl: time.Hour, offline: true}).answerStats().CacheTTLSeconds; got != 0 {
		t.Errorf("the cache mode has no age limit: %d", got)
	}
}
