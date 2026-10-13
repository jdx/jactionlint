package jactionlint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const findingContextWorkflow = `on:
  push:
  pull_request_target:
  issue_comment:
    types: [created]
permissions:
  contents: read
concurrency:
  group: ci
  cancel-in-progress: true
jobs:
  inherits:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
  own:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      id-token: write
    steps:
      - run: echo hi
  nothing:
    runs-on: ubuntu-latest
    permissions: {}
    steps:
      - run: echo hi
  everything:
    runs-on: ubuntu-latest
    permissions: write-all
    steps:
      - run: echo hi
`

func findingContextRun(t *testing.T, workflow string, args ...string) (map[int]map[string]any, string) {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".git", filepath.Join(".github", "workflows")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := filepath.Join(root, ".github", "workflows", "ci.yaml")
	if err := os.WriteFile(f, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	cmd.Main(append([]string{"jactionlint", "--profile", "default", "--shellcheck=", "--pyflakes="}, append(args, f)...))
	if strings.Contains(strings.Join(args, " "), "sarif") {
		return nil, out.String()
	}
	byLine := map[int]map[string]any{}
	var items []map[string]any
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("not JSON: %v: %s %s", err, out.String(), errOut.String())
	}
	for _, it := range items {
		if it["id"] == "missing-timeout" {
			byLine[int(it["line"].(float64))] = it
		}
	}
	return byLine, out.String()
}

func TestFindingContextJSON(t *testing.T) {
	byLine, _ := findingContextRun(t, findingContextWorkflow, "--format", "json", "--finding-context")
	lineOf := func(job string) int {
		for i, l := range strings.Split(findingContextWorkflow, "\n") {
			if l == "  "+job+":" {
				return i + 1
			}
		}
		t.Fatalf("no job %s", job)
		return 0
	}
	want := map[int]map[string]any{
		lineOf("inherits"):   {"contents": "read"},                       // the permissions of the workflow
		lineOf("own"):        {"contents": "write", "id-token": "write"}, // its own
		lineOf("nothing"):    {"*": "none"},                              // an empty mapping
		lineOf("everything"): {"*": "write"},                             // write-all
	}
	if len(byLine) != len(want) {
		t.Fatalf("findings of missing-timeout at lines %v, want %d", keysInt(byLine), len(want))
	}
	for line, perms := range want {
		it := byLine[line]
		if it == nil {
			t.Errorf("no finding at line %d", line)
			continue
		}
		if !reflect.DeepEqual(it["permissions"], perms) {
			t.Errorf("line %d: permissions = %v, want %v", line, it["permissions"], perms)
		}
		if !reflect.DeepEqual(it["events"], []any{"issue_comment", "pull_request_target", "push"}) {
			t.Errorf("line %d: events = %v", line, it["events"])
		}
	}
}

func keysInt(m map[int]map[string]any) []int {
	var ks []int
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestFindingContextOffByDefault(t *testing.T) {
	_, out := findingContextRun(t, findingContextWorkflow, "--format", "json")
	if strings.Contains(out, `"events"`) || strings.Contains(out, `"permissions"`) {
		t.Errorf("the context is printed without --finding-context:\n%s", out)
	}
	_, out = findingContextRun(t, findingContextWorkflow, "--format", "sarif")
	if strings.Contains(out, `"events"`) || strings.Contains(out, `"permissions"`) {
		t.Errorf("the SARIF log has the context without --finding-context:\n%s", out)
	}
}

func TestFindingContextWithoutPermissions(t *testing.T) {
	// A workflow which configures no token: the default token of the repository applies, so nothing is claimed
	wf := "on: pull_request\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	byLine, _ := findingContextRun(t, wf, "--format", "json", "--finding-context")
	if len(byLine) != 1 {
		t.Fatalf("%v", byLine)
	}
	for _, it := range byLine {
		if _, ok := it["permissions"]; ok {
			t.Errorf("permissions must be absent: %v", it)
		}
		if !reflect.DeepEqual(it["events"], []any{"pull_request"}) {
			t.Errorf("events = %v", it["events"])
		}
	}
}

func TestFindingContextSARIF(t *testing.T) {
	_, out := findingContextRun(t, findingContextWorkflow, "--format", "sarif", "--finding-context")
	var log struct {
		Runs []struct {
			Results []struct {
				RuleID     string `json:"ruleId"`
				Properties struct {
					Events      []string          `json:"events"`
					Permissions map[string]string `json:"permissions"`
				} `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range log.Runs[0].Results {
		if r.RuleID != "missing-timeout" {
			continue
		}
		n++
		if !reflect.DeepEqual(r.Properties.Events, []string{"issue_comment", "pull_request_target", "push"}) || len(r.Properties.Permissions) == 0 {
			t.Errorf("properties of the result: %+v", r.Properties)
		}
	}
	if n != 4 {
		t.Errorf("%d results of missing-timeout", n)
	}
}

func TestFindingContextOfActionsAndDependabot(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{".git", ".github"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dep := filepath.Join(root, ".github", "dependabot.yml")
	if err := os.WriteFile(dep, []byte("version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    schedule: {interval: weekly}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	act := filepath.Join(root, "action.yml")
	if err := os.WriteFile(act, []byte("name: a\ndescription: b\nruns:\n  using: composite\n  steps:\n    - run: echo hi\n      shell: bash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	cmd.Main([]string{"jactionlint", "--profile", "pedantic", "--shellcheck=", "--pyflakes=", "--format", "json", "--finding-context", dep, act})
	if !strings.Contains(out.String(), `"id"`) {
		t.Fatalf("the fixtures must give findings: %s", out.String())
	}
	if strings.Contains(out.String(), `"events"`) || strings.Contains(out.String(), `"permissions"`) {
		t.Errorf("a Dependabot file and an action have no events and no token of their own:\n%s", out.String())
	}
}

// TestFindingContextUnknownPermissions: what an expression decides is "unknown", not the "none" of an empty mapping.
func TestFindingContextUnknownPermissions(t *testing.T) {
	wf := func(perm string) string {
		return "on: push\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + perm + "    steps:\n      - run: echo hi\n"
	}
	for _, tc := range []struct {
		name, perm string
		want       map[string]any
	}{
		{"an empty mapping", "    permissions: {}\n", map[string]any{"*": "none"}},
		{"an expression", "    permissions: ${{ fromJSON(vars.PERMS) }}\n", map[string]any{"*": "unknown"}},
		{"expressions for the scopes", "    permissions:\n      contents: ${{ vars.C }}\n      issues: ${{ vars.I }}\n", map[string]any{"contents": "unknown", "issues": "unknown"}},
		{"some scopes known", "    permissions:\n      contents: read\n      issues: ${{ vars.I }}\n", map[string]any{"contents": "read", "issues": "unknown"}},
		{"known", "    permissions: read-all\n", map[string]any{"*": "read"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byLine, _ := findingContextRun(t, wf(tc.perm), "--format", "json", "--finding-context")
			if len(byLine) != 1 {
				t.Fatalf("%d findings of missing-timeout", len(byLine))
			}
			for _, it := range byLine {
				if !reflect.DeepEqual(it["permissions"], tc.want) {
					t.Errorf("permissions = %v, want %v", it["permissions"], tc.want)
				}
			}
		})
	}
}
