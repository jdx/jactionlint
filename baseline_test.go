//go:build !js

package jactionlint

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two jobs: each has a missing-timeout finding, and the first has a template injection.
const baselineWorkflow = `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo ${{ github.event.head_commit.message }}
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`

// baselineProject creates a repository with the workflow and makes it the working directory.
func baselineProject(t *testing.T, workflow string, config string) (root string) {
	t.Helper()
	root = t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	writeTestFile(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeTestFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), workflow)
	if config == "" {
		// The findings the tests count: a template injection and a missing timeout per job. The profile
		// does not enable the second one.
		config = "rules:\n  missing-timeout: error\n"
	}
	writeTestFile(t, filepath.Join(root, ".github", "jactionlint.yaml"), config)
	t.Chdir(root)
	return root
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func baselineCmd(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status = cmd.Main(append([]string{"jactionlint", "--no-color", "--shellcheck=", "--pyflakes=", "--rule-ids", "--format", "oneline"}, args...))
	return status, out.String(), errOut.String()
}

func reportedIDs(t *testing.T, stdout string) []string {
	t.Helper()
	var ids []string
	for _, l := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if l == "" {
			continue
		}
		i := strings.LastIndex(l, " [")
		if i < 0 {
			t.Fatalf("unexpected output line %q", l)
		}
		ids = append(ids, strings.Trim(l[i+2:], "]"))
	}
	return ids
}

func countID(ids []string, id string) int {
	n := 0
	for _, i := range ids {
		if i == id {
			n++
		}
	}
	return n
}

func TestBaselineWriteThenLintIsClean(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	status, out, _ := baselineCmd(t)
	if status != ExitStatusSuccessProblemFound || !strings.Contains(out, "template-injection") {
		t.Fatalf("the workflow must have findings before the baseline: %d %q", status, out)
	}

	status, out, errOut := baselineCmd(t, "--baseline-write")
	if status != 0 {
		t.Fatalf("--baseline-write exit status is %d: %q %q", status, out, errOut)
	}
	if !strings.Contains(out, "Wrote 3 entries for 1 file") {
		t.Fatalf("unexpected output %q", out)
	}
	if _, err := os.Stat(filepath.Join(root, DefaultBaselineFile)); err != nil {
		t.Fatalf("the default baseline file was not written: %v", err)
	}

	status, out, errOut = baselineCmd(t, "--baseline")
	if status != 0 || out != "" {
		t.Fatalf("every finding is baselined but got status %d: %q", status, out)
	}
	if !strings.Contains(errOut, "3 finding(s) are hidden by the baseline") {
		t.Errorf("the note about hidden findings is missing: %q", errOut)
	}

	// The file is not applied unless asked
	if status, _, _ := baselineCmd(t); status != ExitStatusSuccessProblemFound {
		t.Errorf("without --baseline the findings are reported, got status %d", status)
	}
}

func TestBaselineIsIdempotentAndHasNoLineNumbers(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	path := filepath.Join(root, DefaultBaselineFile)
	baselineCmd(t, "--baseline-write")
	first := readTestFile(t, path)

	_, out, _ := baselineCmd(t, "--baseline-write")
	if !strings.Contains(out, "up to date") {
		t.Errorf("a second write must say nothing changed: %q", out)
	}
	if second := readTestFile(t, path); second != first {
		t.Errorf("writing twice changed the file:\n%s\n%s", first, second)
	}

	// An unrelated edit which moves every line does not change the file
	writeTestFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), "# a new comment\n# another one\n"+baselineWorkflow)
	baselineCmd(t, "--baseline-write")
	if third := readTestFile(t, path); third != first {
		t.Errorf("a shift of the lines changed the baseline:\n%s\n%s", first, third)
	}
	if strings.Contains(first, `"line"`) {
		t.Errorf("line numbers must not be in the baseline:\n%s", first)
	}
}

func TestBaselineSurvivesLineShifts(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")

	// Comments and a new step which is clean move the findings
	shifted := strings.Replace(baselineWorkflow, "    steps:\n      - uses: actions/checkout@v4", "    # explain\n    steps:\n      - name: Setup\n        run: echo setup\n      - uses: actions/checkout@v4", 1)
	writeTestFile(t, wf, "# top\n\n"+shifted)
	status, out, _ := baselineCmd(t, "--baseline")
	if status != 0 || out != "" {
		t.Fatalf("a shifted finding must stay baselined: %d %q", status, out)
	}

	// The indentation changes with a reformatting
	writeTestFile(t, wf, "name: CI\non: push\njobs:\n    build:\n        runs-on: ubuntu-latest\n        steps:\n            - uses: actions/checkout@v4\n            - run: echo ${{ github.event.head_commit.message }}\n    test:\n        runs-on: ubuntu-latest\n        steps:\n            - run: echo hi\n")
	if status, out, _ := baselineCmd(t, "--baseline"); status != 0 || out != "" {
		t.Fatalf("a reindented finding must stay baselined: %d %q", status, out)
	}
}

func TestBaselineReportsNewFindings(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")

	// A new job above the old ones, with the same kind of findings
	added := strings.Replace(baselineWorkflow, "jobs:\n", `jobs:
  extra:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ github.event.head_commit.message }}
`, 1)
	writeTestFile(t, wf, added)
	status, out, _ := baselineCmd(t, "--baseline")
	ids := reportedIDs(t, out)
	if status != ExitStatusSuccessProblemFound || countID(ids, "template-injection") != 1 || countID(ids, "missing-timeout") != 1 || len(ids) != 2 {
		t.Fatalf("only the findings of the new job must be reported: %d %q", status, out)
	}
	if !strings.Contains(out, ":4:") && !strings.Contains(out, ":5:") {
		t.Errorf("the new findings must be at the new job: %q", out)
	}
}

func TestBaselineChangedLineResurfaces(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
	writeTestFile(t, wf, strings.Replace(baselineWorkflow, "echo ${{ github.event.head_commit.message }}", "echo \"${{ github.event.head_commit.message }}\"", 1))
	status, out, _ := baselineCmd(t, "--baseline")
	ids := reportedIDs(t, out)
	if status != ExitStatusSuccessProblemFound || len(ids) != 1 || ids[0] != "template-injection" {
		t.Fatalf("an edited line is code to review again: %d %q", status, out)
	}
}

const duplicateWorkflow = `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - run: echo ${{ github.event.issue.title }}
      - run: echo ${{ github.event.issue.title }}
`

func TestBaselineDuplicatedFindings(t *testing.T) {
	root := baselineProject(t, duplicateWorkflow, "")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
	baselineCmd(t, "--baseline-write")

	var bl Baseline
	if err := json.Unmarshal([]byte(readTestFile(t, filepath.Join(root, DefaultBaselineFile))), &bl); err != nil {
		t.Fatal(err)
	}
	if len(bl.Entries) != 2 || bl.Entries[0].Fingerprint != bl.Entries[1].Fingerprint || bl.Entries[0].Occurrence != 0 || bl.Entries[1].Occurrence != 1 {
		t.Fatalf("identical findings share a fingerprint and differ by occurrence: %+v", bl.Entries)
	}

	if status, out, _ := baselineCmd(t, "--baseline"); status != 0 || out != "" {
		t.Fatalf("both are baselined: %d %q", status, out)
	}

	// A third identical finding is new
	writeTestFile(t, wf, duplicateWorkflow+"      - run: echo ${{ github.event.issue.title }}\n")
	status, out, _ := baselineCmd(t, "--baseline")
	if status != ExitStatusSuccessProblemFound || len(reportedIDs(t, out)) != 1 {
		t.Fatalf("exactly one of three identical findings is new: %d %q", status, out)
	}

	// One of the two was fixed: the other stays accepted and one entry is unused
	writeTestFile(t, wf, strings.Replace(duplicateWorkflow, "      - run: echo ${{ github.event.issue.title }}\n", "      - run: echo fixed\n", 1))
	status, out, _ = baselineCmd(t, "--baseline-check")
	if status != 0 {
		t.Fatalf("an unused entry is info and does not fail: %d %q", status, out)
	}
	ids := reportedIDs(t, out)
	if len(ids) != 1 || ids[0] != "unused-baseline-entry" {
		t.Fatalf("one entry must be unused: %q", out)
	}
}

func TestBaselineUnusedEntries(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
	writeTestFile(t, wf, strings.Replace(baselineWorkflow, "      - run: echo ${{ github.event.head_commit.message }}\n", "", 1))

	// Not reported without --baseline-check, but the note says so
	status, out, errOut := baselineCmd(t, "--baseline")
	if status != 0 || out != "" || !strings.Contains(errOut, "1 baseline entr") {
		t.Fatalf("unused entries are only mentioned: %d %q %q", status, out, errOut)
	}

	status, out, _ = baselineCmd(t, "--baseline-check")
	if status != 0 || !strings.Contains(out, "unused-baseline-entry") || !strings.Contains(out, "jactionlint-baseline.json") {
		t.Fatalf("--baseline-check must list the entry in the baseline file: %d %q", status, out)
	}
	// The finding points at the entry in the baseline file
	if !strings.Contains(out, `rule "template-injection"`) {
		t.Errorf("the message must name the rule: %q", out)
	}
	if line := strings.SplitN(out, ":", 3); line[1] != "9" && line[1] != "16" {
		// Entry 2 of 3: {"version", "entries": [ and each entry is 8 lines
		t.Logf("entry line is %s", line[1])
	}

	// The ratchet: a project can make unused entries fail the build
	writeTestFile(t, filepath.Join(root, ".github", "jactionlint.yaml"), "rules:\n  unused-baseline-entry: error\n")
	if status, _, _ := baselineCmd(t, "--baseline-check"); status != ExitStatusSuccessProblemFound {
		t.Errorf("unused-baseline-entry: error must fail the run, got %d", status)
	}
	writeTestFile(t, filepath.Join(root, ".github", "jactionlint.yaml"), "rules:\n  unused-baseline-entry: off\n")
	if status, out, _ := baselineCmd(t, "--baseline-check"); status != 0 || out != "" {
		t.Errorf("unused-baseline-entry: off must hide it: %d %q", status, out)
	}
	writeTestFile(t, filepath.Join(root, ".github", "jactionlint.yaml"), "")
	os.Remove(filepath.Join(root, ".github", "jactionlint.yaml"))

	// Writing again shrinks the file
	baselineCmd(t, "--baseline-write")
	if status, out, _ := baselineCmd(t, "--baseline-check"); status != 0 || out != "" {
		t.Errorf("a fresh baseline has no unused entry: %d %q", status, out)
	}
	if strings.Contains(readTestFile(t, filepath.Join(root, DefaultBaselineFile)), "template-injection") {
		t.Error("the refreshed baseline must not keep the fixed finding")
	}
}

func TestBaselineRenamedFile(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	old := filepath.Join(root, ".github", "workflows", "ci.yaml")
	renamed := filepath.Join(root, ".github", "workflows", "build.yaml")
	if err := os.Rename(old, renamed); err != nil {
		t.Fatal(err)
	}

	// The entries are keyed by file: the findings come back and the old entries are unused
	status, out, _ := baselineCmd(t, "--baseline-check")
	ids := reportedIDs(t, out)
	if status != ExitStatusSuccessProblemFound || countID(ids, "template-injection") != 1 || countID(ids, "unused-baseline-entry") != 3 {
		t.Fatalf("a renamed file loses its baseline: %d %q", status, out)
	}

	baselineCmd(t, "--baseline-write")
	if status, out, _ := baselineCmd(t, "--baseline-check"); status != 0 || out != "" {
		t.Fatalf("writing the baseline again follows the rename: %d %q", status, out)
	}
	if strings.Contains(readTestFile(t, filepath.Join(root, DefaultBaselineFile)), "ci.yaml") {
		t.Error("the old file name must be gone from the baseline")
	}
}

func TestBaselineCRLF(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
	baselineCmd(t, "--baseline-write")
	lf := readTestFile(t, filepath.Join(root, DefaultBaselineFile))

	// A checkout with CRLF line endings is the same code
	writeTestFile(t, wf, strings.ReplaceAll(baselineWorkflow, "\n", "\r\n"))
	if status, out, _ := baselineCmd(t, "--baseline-check"); status != 0 || out != "" {
		t.Fatalf("CRLF must not resurface findings or leave unused entries: %d %q", status, out)
	}
	baselineCmd(t, "--baseline-write")
	if got := readTestFile(t, filepath.Join(root, DefaultBaselineFile)); got != lf {
		t.Errorf("the baseline written from CRLF differs:\n%s\n%s", lf, got)
	}
}

func TestBaselineScopeSeparatesJobs(t *testing.T) {
	lines := sourceLines([]byte(baselineWorkflow))
	for line, want := range map[int]string{
		1: "name", 3: "jobs", 4: "jobs/build", 8: "jobs/build", 9: "jobs/test", 12: "jobs/test",
	} {
		if got := baselineScope(lines, line); got != want {
			t.Errorf("scope of line %d is %q, want %q", line, got, want)
		}
	}
	if got := baselineScope(lines, 0); got != "" {
		t.Errorf("scope of an unknown line is %q", got)
	}
	if got := baselineScope(sourceLines([]byte("{a: 1}\n")), 1); got != "" {
		t.Errorf("flow style has no scope, got %q", got)
	}
}

func TestBaselineMatchesAfterMessageIsReworded(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	path := filepath.Join(root, DefaultBaselineFile)
	// Another release words the messages differently: the fingerprints (which include the message)
	// change but the contexts (which do not) are the same
	var bl Baseline
	if err := json.Unmarshal([]byte(readTestFile(t, path)), &bl); err != nil {
		t.Fatal(err)
	}
	for _, e := range bl.Entries {
		e.Fingerprint = "0000" + e.Fingerprint[4:]
	}
	out, err := bl.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, string(out))
	if status, out, _ := baselineCmd(t, "--baseline"); status != 0 || out != "" {
		t.Fatalf("the context must keep the entries working: %d %q", status, out)
	}
}

func TestBaselineFromConfig(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "baseline: auto\n")
	// No file yet: auto does nothing
	if status, _, _ := baselineCmd(t); status != ExitStatusSuccessProblemFound {
		t.Fatalf("auto without a file reports findings, got %d", status)
	}
	// --baseline-write is not affected by the config
	if status, _, _ := baselineCmd(t, "--baseline-write"); status != 0 {
		t.Fatalf("write failed: %d", status)
	}
	if status, out, _ := baselineCmd(t); status != 0 || out != "" {
		t.Fatalf("auto applies the default file: %d %q", status, out)
	}
	if status, _, _ := baselineCmd(t, "--no-baseline"); status != ExitStatusSuccessProblemFound {
		t.Errorf("--no-baseline must ignore the baseline, got %d", status)
	}

	// A path is relative to the repository and must exist
	writeTestFile(t, filepath.Join(root, ".github", "jactionlint.yaml"), "baseline: ci/accepted.json\n")
	if status, _, errOut := baselineCmd(t); status != ExitStatusFailure || !strings.Contains(errOut, "does not exist") {
		t.Errorf("a missing configured baseline is an error: %d %q", status, errOut)
	}
	if err := os.MkdirAll(filepath.Join(root, "ci"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, DefaultBaselineFile), filepath.Join(root, "ci", "accepted.json")); err != nil {
		t.Fatal(err)
	}
	if status, out, _ := baselineCmd(t); status != 0 || out != "" {
		t.Errorf("the configured path applies: %d %q", status, out)
	}

	writeTestFile(t, filepath.Join(root, ".github", "jactionlint.yaml"), "baseline: false\n")
	if status, _, _ := baselineCmd(t); status != ExitStatusSuccessProblemFound {
		t.Errorf("baseline: false does not apply a baseline, got %d", status)
	}
}

func TestBaselineFlagsAndErrors(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	if status, _, errOut := baselineCmd(t, "--baseline"); status != ExitStatusFailure || !strings.Contains(errOut, "--baseline-write") {
		t.Errorf("a missing baseline must say how to create it: %d %q", status, errOut)
	}
	if status, _, _ := baselineCmd(t, "--baseline-write", "--baseline"); status != ExitStatusInvalidCommandOption {
		t.Errorf("--baseline-write --baseline is invalid, got %d", status)
	}
	if status, _, _ := baselineCmd(t, "--baseline-write", "--fix"); status != ExitStatusInvalidCommandOption {
		t.Errorf("--baseline-write --fix is invalid, got %d", status)
	}
	if status, _, _ := baselineCmd(t, "--baseline-write", "-"); status != ExitStatusInvalidCommandOption {
		t.Errorf("--baseline-write with stdin is invalid, got %d", status)
	}

	// An explicit file
	if status, _, _ := baselineCmd(t, "--baseline-write=ci/base.json"); status != 0 {
		t.Fatalf("--baseline-write=FILE failed: %d", status)
	}
	if _, err := os.Stat(filepath.Join(root, "ci", "base.json")); err != nil {
		t.Errorf("the file was not written: %v", err)
	}
	if status, out, _ := baselineCmd(t, "--baseline=ci/base.json"); status != 0 || out != "" {
		t.Errorf("--baseline=FILE applies it: %d %q", status, out)
	}

	writeTestFile(t, filepath.Join(root, "bad.json"), `{"version": 99, "entries": []}`)
	if status, _, errOut := baselineCmd(t, "--baseline=bad.json"); status != ExitStatusFailure || !strings.Contains(errOut, "unsupported baseline version") {
		t.Errorf("an unknown version is an error: %d %q", status, errOut)
	}
	writeTestFile(t, filepath.Join(root, "bad.json"), `[]`)
	if status, _, _ := baselineCmd(t, "--baseline=bad.json"); status != ExitStatusFailure {
		t.Errorf("a malformed baseline is an error, got %d", status)
	}
}

func TestBaselineWriteForFilesKeepsOthers(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	other := filepath.Join(root, ".github", "workflows", "other.yaml")
	writeTestFile(t, other, strings.Replace(baselineWorkflow, "name: CI", "name: Other", 1))
	baselineCmd(t, "--baseline-write")
	full := readTestFile(t, filepath.Join(root, DefaultBaselineFile))
	if strings.Count(full, `"file"`) != 6 {
		t.Fatalf("expected 6 entries:\n%s", full)
	}

	// Fix other.yaml and refresh only it
	writeTestFile(t, other, "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 1\n    steps:\n      - run: echo hi\n")
	status, out, _ := baselineCmd(t, "--baseline-write", ".github/workflows/other.yaml")
	if status != 0 || !strings.Contains(out, "3 entries") {
		t.Fatalf("partial write: %d %q", status, out)
	}
	got := readTestFile(t, filepath.Join(root, DefaultBaselineFile))
	if strings.Contains(got, "other.yaml") || strings.Count(got, `"file"`) != 3 {
		t.Errorf("only the entries of ci.yaml must remain:\n%s", got)
	}

	// An entry of a file that is gone is dropped by a partial write, an existing one is kept
	if err := os.Remove(filepath.Join(root, ".github", "workflows", "ci.yaml")); err != nil {
		t.Fatal(err)
	}
	baselineCmd(t, "--baseline-write", ".github/workflows/other.yaml")
	if got := readTestFile(t, filepath.Join(root, DefaultBaselineFile)); strings.Contains(got, `"file"`) {
		t.Errorf("entries of a deleted file must go:\n%s", got)
	}
}

func TestBaselineWriteOrderIsDeterministic(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	for _, n := range []string{"z.yaml", "a.yaml", "m.yaml"} {
		writeTestFile(t, filepath.Join(root, ".github", "workflows", n), baselineWorkflow)
	}
	baselineCmd(t, "--baseline-write")
	var bl Baseline
	if err := json.Unmarshal([]byte(readTestFile(t, filepath.Join(root, DefaultBaselineFile))), &bl); err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range bl.Entries {
		if len(files) == 0 || files[len(files)-1] != e.File {
			files = append(files, e.File)
		}
	}
	want := []string{".github/workflows/a.yaml", ".github/workflows/ci.yaml", ".github/workflows/m.yaml", ".github/workflows/z.yaml"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Errorf("entries are sorted by file: %v", files)
	}
}

func TestBaselineSARIFSuppressions(t *testing.T) {
	baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")

	_, out, _ := baselineCmd(t, "--baseline", "--format", "sarif")
	var log struct {
		Runs []struct {
			Results []struct {
				RuleID       string `json:"ruleId"`
				Suppressions []struct {
					Kind string `json:"kind"`
				} `json:"suppressions"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(log.Runs[0].Results) != 3 {
		t.Fatalf("baselined findings are in the log: %s", out)
	}
	for _, r := range log.Runs[0].Results {
		if len(r.Suppressions) != 1 || r.Suppressions[0].Kind != "external" {
			t.Errorf("%s needs an external suppression: %+v", r.RuleID, r.Suppressions)
		}
	}

	status, out, _ := baselineCmd(t, "--baseline", "--format", "sarif", "--sarif-hide-baselined")
	if status != 0 {
		t.Errorf("status is %d", status)
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil || len(log.Runs[0].Results) != 0 {
		t.Errorf("--sarif-hide-baselined must leave them out: %v %s", err, out)
	}
}

func TestBaselineSARIFMixesNewAndSuppressed(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
	writeTestFile(t, wf, baselineWorkflow+"  third:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
	status, out, _ := baselineCmd(t, "--baseline", "--format", "sarif")
	if status != ExitStatusSuccessProblemFound {
		t.Errorf("a new finding fails the run: %d", status)
	}
	if strings.Count(out, `"suppressions"`) != 3 {
		t.Errorf("three results are suppressed:\n%s", out)
	}
	if strings.Count(out, `"ruleId"`) != 4 {
		t.Errorf("four results:\n%s", out)
	}
}

func TestBaselineSummaryFormat(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	status, out, _ := baselineCmd(t, "--format", "summary")
	if status != ExitStatusSuccessProblemFound {
		t.Errorf("status %d", status)
	}
	for _, want := range []string{"3 findings in 1 of 1 files\n", "by rule", "by file", "missing-timeout", "template-injection", filepath.Join(".github", "workflows", "ci.yaml")} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "baselined") {
		t.Errorf("without a baseline the summary has no baselined column:\n%s", out)
	}

	baselineCmd(t, "--baseline-write")
	wf := filepath.Join(root, ".github", "workflows", "ci.yaml")
	writeTestFile(t, wf, strings.Replace(strings.Replace(baselineWorkflow, "      - run: echo hi\n", "      - run: echo ${{ github.event.issue.title }}\n", 1), "echo ${{ github.event.head_commit.message }}", "echo fixed", 1))
	status, out, _ = baselineCmd(t, "--baseline", "--format", "summary")
	if status != ExitStatusSuccessProblemFound {
		t.Errorf("status %d", status)
	}
	for _, want := range []string{"3 findings in 1 of 1 files: 1 new, 2 baselined", "1 baseline entry matches nothing any more", "template-injection      1          0      1"} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary lacks %q:\n%s", want, out)
		}
	}
}

func TestBaselineWithFixFixesBaselinedFindings(t *testing.T) {
	cfg := "rules:\n  missing-timeout:\n    default-minutes: 10\n"
	root := baselineProject(t, baselineWorkflow, cfg)
	baselineCmd(t, "--baseline-write")
	status, _, _ := baselineCmd(t, "--baseline", "--fix")
	if status != 0 {
		t.Fatalf("every remaining finding is baselined: %d", status)
	}
	if got := readTestFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml")); strings.Count(got, "timeout-minutes: 10") != 2 {
		t.Errorf("--fix must fix the baselined findings, too:\n%s", got)
	}
	status, out, _ := baselineCmd(t, "--baseline-check")
	if status != 0 || countID(reportedIDs(t, out), "unused-baseline-entry") != 2 {
		t.Errorf("the fixed findings leave unused entries: %d %q", status, out)
	}
}

func TestBaselineIgnoresMinSeverityForOccurrences(t *testing.T) {
	root := baselineProject(t, duplicateWorkflow, "rules:\n  template-injection: warn\n")
	baselineCmd(t, "--baseline-write")
	_ = root
	if status, out, _ := baselineCmd(t, "--baseline", "--min-severity", "error"); status != 0 || out != "" {
		t.Errorf("%d %q", status, out)
	}
	if status, out, _ := baselineCmd(t, "--baseline", "--strict-exit"); status != 0 || out != "" {
		t.Errorf("%d %q", status, out)
	}
}

func TestBaselineStdinUsesFilename(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	baselineCmd(t, "--baseline-write")
	var out, errOut bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(baselineWorkflow), Stdout: &out, Stderr: &errOut}
	status := cmd.Main([]string{"jactionlint", "--no-color", "--shellcheck=", "--pyflakes=", "--baseline", "--stdin-filename", ".github/workflows/ci.yaml", "-"})
	if status != 0 || out.String() != "" {
		t.Errorf("stdin with the file name is matched against the baseline: %d %q %q", status, out.String(), errOut.String())
	}
	_ = root
}

func TestBaselineConfigValues(t *testing.T) {
	for in, want := range map[string]string{"baseline: true\n": "true", "baseline: auto\n": "auto", "baseline: false\n": "false", "baseline: ci/b.json\n": "ci/b.json", "{}\n": ""} {
		c, err := ParseConfig([]byte(in))
		if err != nil || c.Baseline != want {
			t.Errorf("%q: %q %v", in, c.Baseline, err)
		}
	}
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "base.yaml"), "baseline: auto\n")
	writeTestFile(t, filepath.Join(dir, "c.yaml"), "extends: [base.yaml]\n")
	c, err := ReadConfigFile(filepath.Join(dir, "c.yaml"))
	if err != nil || c.Baseline != "auto" {
		t.Errorf("extends must carry the baseline: %q %v", c.Baseline, err)
	}
}

func TestParseBaselineLines(t *testing.T) {
	bl, err := ParseBaseline([]byte("{\n  \"version\": 1,\n  \"extra\": [1],\n  \"entries\": [\n    {\"file\": \"a\", \"rule\": \"r\", \"fingerprint\": \"f\"},\n    {\n      \"file\": \"b\", \"rule\": \"r\", \"fingerprint\": \"f\"\n    }\n  ]\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(bl.Entries) != 2 || bl.Entries[0].line != 5 || bl.Entries[1].line != 6 {
		t.Errorf("entry lines: %+v", bl.Entries)
	}
	for _, bad := range []string{``, `{"version":1,"entries":[{"file":"a"}]}`, `{"version":2}`, `{"entries":{}}`} {
		if _, err := ParseBaseline([]byte(bad)); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	empty, err := (&Baseline{}).Marshal()
	if err != nil || !strings.Contains(string(empty), `"entries": []`) {
		t.Errorf("an empty baseline lists no entries: %v %s", err, empty)
	}
}

func TestBaselineIsPortableAcrossCheckouts(t *testing.T) {
	// The message of this finding contains the absolute path of the repository
	wf := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 1\n    steps:\n      - uses: ./.github/actions/x\n"
	root := baselineProject(t, wf, "")
	writeTestFile(t, filepath.Join(root, ".github", "actions", "x", "action.yaml"), "name: x\ndescription: x\ninputs:\n  a:\n    type: string\nruns:\n  using: composite\n  steps: []\n")
	_, out, _ := baselineCmd(t)
	if !strings.Contains(out, root) {
		t.Skipf("the message does not contain the path of the repository any more: %q", out)
	}
	baselineCmd(t, "--baseline-write")

	other := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(other); err == nil {
		other = resolved
	}
	other = filepath.Join(other, "elsewhere")
	if err := os.CopyFS(other, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)
	if status, out, _ := baselineCmd(t, "--baseline"); status != 0 || out != "" {
		t.Fatalf("a baseline must work in another checkout: %d %q", status, out)
	}
}

// A file that is linted again in the same run (the passes of --fix) is judged by its last lint: an entry
// whose finding was fixed in between is not seen.
func TestBaselineMatchForgetsEarlierLints(t *testing.T) {
	e := &BaselineEntry{File: "a.yaml", Rule: "r", Fingerprint: "f", Context: "c"}
	s := newBaselineState("b.json", t.TempDir(), &Baseline{Entries: []*BaselineEntry{e}})
	finding := &Error{ID: "r"}
	s.match("a.yaml", []*Error{finding}, map[*Error]*baselineInfo{finding: {file: "a.yaml", fingerprint: "f", context: "c"}})
	if !s.wasSeen(e) || !finding.Baselined {
		t.Fatal("the first lint sees the finding")
	}
	s.match("a.yaml", nil, nil)
	if s.wasSeen(e) {
		t.Error("the finding is gone in the second lint, so the entry matches nothing any more")
	}
}

// A different problem of the same local action is not the old finding reworded.
func TestBaselineContextFallbackSkipsInvalidLocalAction(t *testing.T) {
	for _, id := range []string{"invalid-local-action", "invalid-local-workflow", "known-vulnerable-actions", "missing-action-input", "missing-workflow-input", "missing-workflow-secret", "workflow-call-permissions", "required-actions"} {
		e := &BaselineEntry{File: "a.yaml", Rule: id, Fingerprint: "old", Context: "c"}
		s := newBaselineState("b.json", t.TempDir(), &Baseline{Entries: []*BaselineEntry{e}})
		finding := &Error{ID: id}
		s.match("a.yaml", []*Error{finding}, map[*Error]*baselineInfo{finding: {file: "a.yaml", fingerprint: "new", context: "c"}})
		if finding.Baselined {
			t.Errorf("%s: the new problem must be reported", id)
		}
	}
	other := &Error{ID: "missing-timeout"}
	e2 := &BaselineEntry{File: "a.yaml", Rule: "missing-timeout", Fingerprint: "old", Context: "c"}
	s := newBaselineState("b.json", t.TempDir(), &Baseline{Entries: []*BaselineEntry{e2}})
	s.match("a.yaml", []*Error{other}, map[*Error]*baselineInfo{other: {file: "a.yaml", fingerprint: "new", context: "c"}})
	if !other.Baselined {
		t.Error("a reworded message of another rule still matches")
	}
}

// Whether an online rule ran for the unused entries follows --online, --no-online and the online keys.
func TestBaselineRuleRanAsIsFollowsTheOnlineSettings(t *testing.T) {
	newLinter := func(opts LinterOptions) *Linter {
		l, err := NewLinter(io.Discard, &opts)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	mode, err := ParseConfig([]byte("online-options:\n  mode: cache\n"))
	if err != nil {
		t.Fatal(err)
	}
	on, err := ParseConfig([]byte("online: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	const id = "archived-uses"
	if !newLinter(LinterOptions{}).ruleRanAsIs(id, mode) {
		t.Error("online-options.mode turns the online checks on")
	}
	if newLinter(LinterOptions{}).ruleRanAsIs(id, &Config{}) {
		t.Error("offline by default")
	}
	if newLinter(LinterOptions{OnlineOff: true}).ruleRanAsIs(id, on) {
		t.Error("--no-online wins over the config")
	}
	if !newLinter(LinterOptions{Online: true}).ruleRanAsIs(id, &Config{}) {
		t.Error("--online turns them on")
	}
	// A lookup which failed left findings unreported: their entries are not unused
	l := newLinter(LinterOptions{Online: true})
	l.online.sess = &onlineSession{}
	l.online.sess.skipped = 1
	if l.ruleRanAsIs(id, &Config{}) {
		t.Error("an online rule with a skipped lookup did not run as is")
	}
	if !l.ruleRanAsIs("missing-action-input", &Config{}) {
		t.Error("an offline rule is not affected by a skipped lookup")
	}
}

// --baseline-check asks for the baseline although the configuration switches it off.
func TestBaselineCheckWithBaselineFalseInTheConfig(t *testing.T) {
	baselineProject(t, baselineWorkflow, "baseline: false\n")
	if status, _, _ := baselineCmd(t, "--baseline-write"); status != 0 {
		t.Fatalf("setup: %d", status)
	}
	// Fix everything the baseline knows about so that all of its entries are stale
	root, _ := os.Getwd()
	writeTestFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"), "name: ci\non: push\npermissions: {}\njobs: {}\n")
	_, out, _ := baselineCmd(t, "--baseline-check", "--rule-ids")
	if !strings.Contains(out, "unused-baseline-entry") {
		t.Errorf("the stale entries must be reported:\n%s", out)
	}
}

// baselineScopes must give what baselineScope gives for every line, for the fixtures of all kinds of
// files and for lines which are not YAML at all.
func TestBaselineScopesMatchBaselineScope(t *testing.T) {
	var sources [][]byte
	for _, pat := range []string{"testdata/ok/*.yaml", "testdata/err/*.yaml", "testdata/examples/*.yaml", "testdata/positions/*.yaml", "testdata/projects/*/.github/workflows/*.y*ml"} {
		files, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, b)
		}
	}
	sources = append(sources,
		[]byte("a: 1\nfoo\n  b: 2\n# c\nc:\n  d: 1\n---\n  e: 1\nf:\n\tg: 2\n...\nh:\n- i\n  j: 1\n \n"),
		[]byte("   \n# x\n  indented: 1\ntop:\n    deep: 1\n  shallow: 2\n"),
		[]byte("{a: 1}\n"), []byte(""), []byte("\n"))
	if len(sources) < 100 {
		t.Fatalf("too few sources: %d", len(sources))
	}
	for _, src := range sources {
		lines := sourceLines(src)
		got := baselineScopes(lines)
		for i := range lines {
			if want := baselineScope(lines, i+1); got[i] != want {
				t.Fatalf("line %d %q: baselineScopes gives %q, baselineScope %q\n%s", i+1, lines[i], got[i], want, src)
			}
		}
	}
}

// Renaming the required input of a local action makes the call miss a different input: the baselined finding
// of the old name must not accept it as a reworded one.
func TestBaselineDoesNotHideAnotherMissingInput(t *testing.T) {
	const wf = "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - uses: ./.github/actions/greet\n"
	root := baselineProject(t, wf, "")
	action := filepath.Join(root, ".github", "actions", "greet", "action.yml")
	writeTestFile(t, action, "name: greet\ndescription: d\ninputs:\n  a:\n    description: d\n    required: true\nruns:\n  using: composite\n  steps: []\n")
	if status, out, errOut := baselineCmd(t, "--baseline-write"); status != 0 {
		t.Fatalf("setup: %d\n%s%s", status, out, errOut)
	}
	if _, out, _ := baselineCmd(t, "--baseline"); strings.Contains(out, "missing-action-input") {
		t.Fatalf("setup: the finding is baselined:\n%s", out)
	}
	writeTestFile(t, action, "name: greet\ndescription: d\ninputs:\n  c:\n    description: d\n    required: true\nruns:\n  using: composite\n  steps: []\n")
	_, out, _ := baselineCmd(t, "--baseline")
	if !strings.Contains(out, "missing-action-input") || !strings.Contains(out, `"c"`) {
		t.Errorf("the new missing input must be reported:\n%s", out)
	}
}

// The guidance names the configuration file which was selected, also one outside the repository.
func TestBaselineWriteNamesTheSelectedConfigFile(t *testing.T) {
	root := baselineProject(t, baselineWorkflow, "")
	outside := filepath.Join(filepath.Dir(root), "outside-"+filepath.Base(root)+".yaml")
	writeTestFile(t, outside, "rules:\n  missing-timeout: error\n")
	status, out, errOut := baselineCmd(t, "--config-file", outside, "--baseline-write")
	if status != 0 {
		t.Fatalf("%d\n%s%s", status, out, errOut)
	}
	if !strings.Contains(out, "put this line in "+filepath.ToSlash(outside)+" ") {
		t.Errorf("the selected file must be named:\n%s", out)
	}
	if strings.Contains(out, ".github/jactionlint.yaml") {
		t.Errorf("another file is named:\n%s", out)
	}
	// Inside the repository it is still relative to it
	_, out, _ = baselineCmd(t, "--baseline-write", "--config-file", filepath.Join(root, ".github", "jactionlint.yaml"))
	if strings.Contains(out, root) {
		t.Errorf("a file of the repository is named relative to it:\n%s", out)
	}
}
