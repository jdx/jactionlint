package jactionlint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// selectProject writes a project whose workflow has findings of several rules: no timeout, no concurrency, an
// unpinned action and (for the pedantic profile) a job without a name.
func selectProject(t *testing.T, workflow string, files map[string]string) (root, wf string) {
	t.Helper()
	root = t.TempDir()
	for _, d := range []string{".git", filepath.Join(".github", "workflows")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if workflow == "" {
		workflow = "on: push\npermissions: {}\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	}
	wf = filepath.Join(root, ".github", "workflows", "ci.yaml")
	if err := os.WriteFile(wf, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, wf
}

// findingIDs lints with the arguments and returns the sorted rule IDs of the findings, with the exit status and stderr.
func findingIDs(t *testing.T, args ...string) (ids []string, status int, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status = cmd.Main(append([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes=", "--format", "json"}, args...))
	if status > 1 {
		return nil, status, errOut.String()
	}
	var fields []struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
		t.Fatalf("not JSON: %v: %s", err, out.String())
	}
	for _, f := range fields {
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	return slices.Compact(ids), status, errOut.String()
}

func TestSelect(t *testing.T) {
	_, wf := selectProject(t, "", nil)

	all, _, _ := findingIDs(t, wf)
	for _, want := range []string{"missing-timeout", "unpinned-uses", "concurrency-limits"} {
		if !slices.Contains(all, want) {
			t.Fatalf("the fixture must have a finding of %s: %v", want, all)
		}
	}

	t.Run("one rule", func(t *testing.T) {
		ids, status, _ := findingIDs(t, "--select=missing-timeout", wf)
		if !slices.Equal(ids, []string{"missing-timeout"}) || status != 1 {
			t.Errorf("ids = %v, status = %d", ids, status)
		}
	})
	t.Run("a list and a repeated option", func(t *testing.T) {
		ids, _, _ := findingIDs(t, "--select=missing-timeout,unpinned-uses", "--select", "concurrency-limits", wf)
		if !slices.Equal(ids, []string{"concurrency-limits", "missing-timeout", "unpinned-uses"}) {
			t.Errorf("ids = %v", ids)
		}
	})
	t.Run("a rule the profile does not enable", func(t *testing.T) {
		if slices.Contains(all, "anonymous-definition") {
			t.Fatal("anonymous-definition is pedantic and must not be reported by default")
		}
		ids, status, _ := findingIDs(t, "--select=anonymous-definition", wf)
		if !slices.Equal(ids, []string{"anonymous-definition"}) || status != 0 {
			// The level of anonymous-definition is warn: the exit status stays 0
			t.Errorf("ids = %v, status = %d", ids, status)
		}
	})
	t.Run("no finding of the selected rule", func(t *testing.T) {
		ids, status, _ := findingIDs(t, "--select=unused-needs", wf)
		if len(ids) != 0 || status != 0 {
			t.Errorf("ids = %v, status = %d", ids, status)
		}
	})
	t.Run("unknown rule", func(t *testing.T) {
		_, status, stderr := findingIDs(t, "--select=missing-timout", wf)
		if status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, `unknown rule ID "missing-timout" in --select`) || !strings.Contains(stderr, `did you mean "missing-timeout"?`) {
			t.Errorf("status = %d, stderr = %q", status, stderr)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, status, stderr := findingIDs(t, "--select=", wf)
		if status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "lists no rule ID") {
			t.Errorf("status = %d, stderr = %q", status, stderr)
		}
	})
}

func TestSelectAndTheConfiguration(t *testing.T) {
	cfg := "rules:\n  missing-timeout: off\n  unpinned-uses: warn\n"
	root, wf := selectProject(t, "", map[string]string{".github/jactionlint.yaml": cfg})
	_ = root

	ids, _, _ := findingIDs(t, wf)
	if slices.Contains(ids, "missing-timeout") {
		t.Fatalf("the config turns missing-timeout off: %v", ids)
	}
	// The command line wins over an "off" of the config
	ids, _, _ = findingIDs(t, "--select=missing-timeout", wf)
	if !slices.Equal(ids, []string{"missing-timeout"}) {
		t.Errorf("select must turn the rule on: %v", ids)
	}
	// A level of the config stays: warn does not fail the run
	var out bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	if status := cmd.Main([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes=", "--select=unpinned-uses", wf}); status != 0 || !strings.Contains(out.String(), "warning: ") {
		t.Errorf("the configured level warn must stay: status = %d: %s", status, out.String())
	}
}

func TestSelectDoesNotRunTheExternalTools(t *testing.T) {
	// A shellcheck which would fail the run when it is started
	dir := t.TempDir()
	script := filepath.Join(dir, "shellcheck")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, wf := selectProject(t, "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo $A\n", nil)
	var out bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	if status := cmd.Main([]string{"jactionlint", "--profile", "default", "--format", "json", "--shellcheck=" + script, "--select=missing-timeout", wf}); status != 1 {
		t.Errorf("shellcheck must not be started when it is not selected: status = %d: %s", status, out.String())
	}
	out.Reset()
	if status := cmd.Main([]string{"jactionlint", "--profile", "default", "--format", "json", "--shellcheck=" + script, "--select=shellcheck", wf}); status != 3 {
		t.Errorf("shellcheck must be started when it is selected: status = %d: %s", status, out.String())
	}
}

func TestNoIgnores(t *testing.T) {
	workflow := `on: push
permissions: {}
concurrency:
  group: ci
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      # jactionlint ignore=unpinned-uses,missing-timeout
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5 # zizmor: ignore[unpinned-uses]
`
	cfg := "ignores:\n  - rule: unpinned-uses\n    uses: actions/cache\n    reason: because\npaths:\n  .github/workflows/ci.yaml:\n    ignore: [excessive-permissions]\n"
	root, wf := selectProject(t, workflow, map[string]string{".github/jactionlint.yaml": cfg})
	t.Chdir(root) // the "paths" of the config are relative to the repository root, like the paths of the findings
	wf = filepath.Join(".github", "workflows", "ci.yaml")

	count := func(args ...string) (n int, status int, stderr string) {
		var out, errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		status = cmd.Main(append([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes=", "--format", "json", "--select=unpinned-uses,missing-timeout"}, args...))
		var fields []map[string]any
		if status <= 1 {
			if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
				t.Fatalf("not JSON: %v: %s", err, out.String())
			}
		}
		return len(fields), status, errOut.String()
	}

	with, _, _ := count(wf)
	without, status, stderr := count("--no-ignores", wf)
	if with != 1 || without != 3 || status != 1 {
		t.Errorf("findings with the ignores = %d (want 1: the timeout, which the comment above the step does not cover), without = %d (want 3), status %d: %s", with, without, status, stderr)
	}
}

func TestNoIgnoresKeepsTheCommandLineIgnoresAndDropsTheOthers(t *testing.T) {
	workflow := "on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      # jactionlint ignore=missing-timeout\n      - uses: actions/checkout@v4\n"
	cfg := "paths:\n  .github/workflows/ci.yaml:\n    ignore: [unpinned-uses]\nignores:\n  - rule: missing-timeout\n    job: build\n"
	root, wf := selectProject(t, workflow, map[string]string{".github/jactionlint.yaml": cfg})
	t.Chdir(root)
	wf = filepath.Join(".github", "workflows", "ci.yaml")

	ids, _, _ := findingIDs(t, wf)
	if slices.Contains(ids, "unpinned-uses") || slices.Contains(ids, "missing-timeout") {
		t.Fatalf("the ignores must hide both: %v", ids)
	}
	ids, status, _ := findingIDs(t, "--no-ignores", wf)
	if !slices.Contains(ids, "unpinned-uses") || !slices.Contains(ids, "missing-timeout") || status != 1 {
		t.Errorf("--no-ignores must show both: %v", ids)
	}
	// --ignore is an explicit request of this run
	ids, _, _ = findingIDs(t, "--no-ignores", "--ignore", "unpinned-uses", wf)
	if slices.Contains(ids, "unpinned-uses") || !slices.Contains(ids, "missing-timeout") {
		t.Errorf("--ignore still applies: %v", ids)
	}
	// There is nothing to report as unused or expired
	if slices.Contains(ids, "unused-ignore") || slices.Contains(ids, "expired-ignore") {
		t.Errorf("no ignore is applied, so none is unused: %v", ids)
	}
}

func TestNoIgnoresTurnsTheBaselineOff(t *testing.T) {
	_, wf := selectProject(t, "", nil)
	root := filepath.Dir(filepath.Dir(filepath.Dir(wf)))
	var out bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	if status := cmd.Main([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes=", "--baseline-write", wf}); status != 0 {
		t.Fatalf("baseline-write: %d: %s", status, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, DefaultBaselineFile)); err != nil {
		t.Fatal(err)
	}
	ids, status, _ := findingIDs(t, "--baseline", wf)
	if len(ids) != 0 || status != 0 {
		t.Fatalf("the baseline must hide everything: %v, %d", ids, status)
	}
	ids, status, _ = findingIDs(t, "--baseline", "--no-ignores", wf)
	if status != ExitStatusInvalidCommandOption {
		t.Errorf("--baseline with --no-ignores must be an error: %v, %d", ids, status)
	}
	ids, status, _ = findingIDs(t, "--no-ignores", wf)
	if len(ids) == 0 || status != 1 {
		t.Errorf("--no-ignores must show the baselined findings: %v, %d", ids, status)
	}
}

func TestListRules(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	if status := cmd.Main([]string{"jactionlint", "--list-rules"}); status != 0 {
		t.Fatalf("status %d: %s", status, errOut.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != len(Rules())+1 {
		t.Errorf("%d lines for %d rules", len(lines), len(Rules()))
	}
	if !strings.HasPrefix(lines[0], "RULE") || !strings.Contains(lines[0], "SUMMARY") {
		t.Errorf("header: %q", lines[0])
	}
	var found string
	for _, l := range lines {
		if strings.HasPrefix(l, "unpinned-uses ") {
			found = l
		}
	}
	if !strings.Contains(found, "policy") || !strings.Contains(found, "default") || !strings.Contains(found, "error") {
		t.Errorf("unpinned-uses line: %q", found)
	}

	out.Reset()
	if status := cmd.Main([]string{"jactionlint", "--list-rules", "--format", "json"}); status != 0 {
		t.Fatalf("json status %d", status)
	}
	var list []ruleListEntry
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != len(Rules()) {
		t.Errorf("%d entries", len(list))
	}
	profiles := map[string]string{}
	for _, e := range list {
		profiles[e.ID] = e.Profile
		if e.DocURL != RuleDocURL(e.ID) {
			t.Errorf("%s: doc_url %q", e.ID, e.DocURL)
		}
	}
	if profiles["archived-uses"] != "online" || profiles["forbidden-uses"] != "opt-in" || profiles["anonymous-definition"] != "pedantic" || profiles["unpinned-uses"] != "default" {
		t.Errorf("profiles: %v", profiles)
	}

	out.Reset()
	errOut.Reset()
	if status := cmd.Main([]string{"jactionlint", "--list-rules", "--format", "sarif"}); status != ExitStatusInvalidCommandOption || !strings.Contains(errOut.String(), "--list-rules supports") {
		t.Errorf("sarif: status %d: %s", status, errOut.String())
	}
}
