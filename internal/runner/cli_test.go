package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		p := filepath.Join(dir, ".github", "workflows", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runMain(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Main(append([]string{"-C", dir}, args...), strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestListGolden(t *testing.T) {
	src, err := os.ReadFile("testdata/list/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := os.ReadFile("testdata/list/ci.golden")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(strings.ReplaceAll(string(wantRaw), "\r\n", "\n")) // git on Windows may check the golden out with CRLF
	dir := repoWith(t, map[string]string{"ci.yml": string(src)})
	for _, arg := range []string{"ci", "ci.yml", filepath.Join(dir, ".github", "workflows", "ci.yml")} {
		code, out, errOut := runMain(t, dir, "--list", arg)
		if code != ExitOK || out != string(want) {
			t.Fatalf("run --list %s: code %d\n--- got ---\n%s\n--- want ---\n%s\nstderr: %s", arg, code, out, want, errOut)
		}
	}
	code, out, _ := runMain(t, dir, "--list")
	if code != ExitOK || out != string(want) {
		t.Fatalf("run --list (no workflow): code %d\n%s", code, out)
	}
}

func TestListJobSelection(t *testing.T) {
	src, _ := os.ReadFile("testdata/list/ci.yml")
	dir := repoWith(t, map[string]string{"ci.yml": string(src)})
	code, out, errOut := runMain(t, dir, "--list", "-j", "shards")
	if code != ExitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if !strings.Contains(out, "shards") || !strings.Contains(out, "lint") || strings.Contains(out, "test (") || strings.Contains(out, "release") {
		t.Fatalf("selection wrong:\n%s", out)
	}
	code, _, errOut = runMain(t, dir, "--list", "-j", "nope")
	if code != ExitUsage || !strings.Contains(errOut, `no job "nope"`) {
		t.Fatalf("unknown job: code %d: %s", code, errOut)
	}
}

func TestListJSON(t *testing.T) {
	src, _ := os.ReadFile("testdata/list/ci.yml")
	dir := repoWith(t, map[string]string{"ci.yml": string(src)})
	code, out, errOut := runMain(t, dir, "--list", "--json", "-j", "test")
	if code != ExitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	var plans []jsonPlan
	if err := json.Unmarshal([]byte(out), &plans); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(plans) != 1 || len(plans[0].Jobs) != 4 {
		t.Fatalf("plans = %+v", plans)
	}
	var test *jsonJob
	for i := range plans[0].Jobs {
		if plans[0].Jobs[i].ID == "test" {
			test = &plans[0].Jobs[i]
		}
	}
	if test == nil || !test.Selected || len(test.Cells) != 3 || test.Cells[2].Matrix["os"] != "macos-latest" {
		t.Fatalf("test job = %+v", test)
	}
}

func TestEventNote(t *testing.T) {
	dir := repoWith(t, map[string]string{"a.yml": "on: pull_request\njobs:\n  j:\n    runs-on: x\n    steps: [{run: e}]\n"})
	_, out, _ := runMain(t, dir, "--list", "--event", "push")
	if !strings.Contains(out, `no "push" trigger`) {
		t.Fatalf("no note:\n%s", out)
	}
	_, out, _ = runMain(t, dir, "--list", "--event", "pull_request")
	if strings.Contains(out, "note:") {
		t.Fatalf("unexpected note:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	dir := repoWith(t, map[string]string{"a.yml": "on: push\njobs:\n  j:\n    runs-on: x\n    steps: [{run: e}]\n", "b.yml": "on: push\njobs:\n  j:\n    runs-on: x\n    steps: [{run: e}]\n"})
	for name, tc := range map[string]struct {
		args []string
		code int
		msg  string
	}{
		"two workflows without --list": {nil, ExitUsage, "2 workflows"},
		"unknown workflow":             {[]string{"--list", "zzz"}, ExitUsage, `no workflow "zzz"`},
		"json without list":            {[]string{"--json", "a"}, ExitUsage, "--json can only be used with --list"},
		"unknown flag":                 {[]string{"--bogus"}, ExitUsage, "bogus"},
		"two args":                     {[]string{"--list", "a", "b"}, ExitUsage, "one workflow"},
		"run is not there yet":         {[]string{"a"}, ExitError, "not available"},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, errOut := runMain(t, dir, tc.args...)
			if code != tc.code || !strings.Contains(errOut, tc.msg) {
				t.Fatalf("code %d (want %d), stderr %q (want %q)", code, tc.code, errOut, tc.msg)
			}
		})
	}
}

func TestInvalidWorkflow(t *testing.T) {
	dir := repoWith(t, map[string]string{"bad.yml": "on: push\njobs:\n  j:\n    steps: [{run: e}\n"})
	code, _, errOut := runMain(t, dir, "--list", "bad")
	if code != ExitError || !strings.Contains(errOut, "not a valid workflow") {
		t.Fatalf("code %d: %q", code, errOut)
	}
}

func TestListJobInSomeWorkflowsOnly(t *testing.T) {
	dir := repoWith(t, map[string]string{
		"a.yml": "on: push\njobs:\n  build:\n    runs-on: x\n    steps: [{run: e}]\n",
		"b.yml": "on: push\njobs:\n  test:\n    runs-on: x\n    steps: [{run: e}]\n",
		"c.yml": "on: push\njobs:\n  lint:\n    runs-on: x\n    steps: [{run: e}]\n",
	})
	code, out, errOut := runMain(t, dir, "--list", "-j", "test")
	if code != ExitOK || !strings.Contains(out, "b.yml") || strings.Contains(out, "a.yml") || errOut != "" {
		t.Fatalf("a job that only some workflows have: code %d\n%s\n%s", code, out, errOut)
	}
	code, _, errOut = runMain(t, dir, "--list", "-j", "nowhere")
	if code != ExitUsage || !strings.Contains(errOut, "no workflow has the job nowhere") {
		t.Fatalf("a job that no workflow has: code %d %q", code, errOut)
	}
}

func TestListResolvesExpressions(t *testing.T) {
	src := `name: dyn
on: workflow_dispatch
jobs:
  build:
    runs-on: self-hosted
    steps: [{run: e}]
  matrixed:
    name: Test ${{ matrix.os }} on ${{ inputs.suffix }}
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: ${{ fromJSON(inputs.oses) }}
        include: ${{ fromJSON(inputs.extra) }}
    steps: [{run: e}]
  whole:
    runs-on: self-hosted
    strategy: ${{ fromJSON(inputs.strategy) }}
    steps: [{run: e}]
  skipped:
    if: github.event_name == 'push'
    runs-on: self-hosted
    steps: [{run: e}]
  runtime:
    needs: build
    runs-on: self-hosted
    strategy:
      matrix:
        n: ${{ fromJSON(needs.build.outputs.n) }}
    steps: [{run: e}]
`
	dir := repoWith(t, map[string]string{"dyn.yml": src})
	code, out, errOut := runMain(t, dir, "--list", "dyn", "-e", "workflow_dispatch",
		"-i", `oses=["box-a","box-b"]`, "-i", `extra=[{"os":"box-a","gpu":true}]`, "-i", "suffix=ci",
		"-i", `strategy={"matrix":{"v":[1,2]}}`)
	if code != ExitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	for _, want := range []string{
		"matrixed (box-a, true)   runs-on: box-a   name: Test box-a on ci",
		"matrixed (box-b)   runs-on: box-b   name: Test box-b on ci",
		"whole (1)",
		"whole (2)",
		"skipped   runs-on: self-hosted   if: github.event_name == 'push'   => if is false",
		"runtime (matrix known at run time: ${{ fromJSON(needs.build.outputs.n) }})",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestListBadInput(t *testing.T) {
	dir := repoWith(t, map[string]string{"a.yml": "on: push\njobs:\n  j:\n    runs-on: x\n    steps: [{run: e}]\n"})
	if code, _, errOut := runMain(t, dir, "--list", "-i", "novalue"); code != ExitUsage || !strings.Contains(errOut, "NAME=VALUE") {
		t.Fatalf("code %d %q", code, errOut)
	}
	if code, _, errOut := runMain(t, dir, "--list", "--event-file", filepath.Join(dir, "missing.json")); code != ExitUsage || !strings.Contains(errOut, "missing.json") {
		t.Fatalf("code %d %q", code, errOut)
	}
}

func TestListWithoutTheInputsAMatrixNeeds(t *testing.T) {
	src := `on:
  workflow_dispatch:
    inputs:
      debug:
        type: boolean
        default: true
      count:
        type: number
        default: "3"
      env:
        default: staging
jobs:
  matrixed:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: ${{ fromJSON(inputs.oses) }}
    steps: [{run: e}]
  typed:
    if: inputs.debug == true && inputs.count == 3 && inputs.env == 'staging'
    runs-on: x
    steps: [{run: e}]
  later:
    needs: typed
    if: needs.typed.result == 'success'
    runs-on: x
    steps: [{run: e}]
`
	dir := repoWith(t, map[string]string{"w.yml": src})
	code, out, errOut := runMain(t, dir, "--list", "w", "-e", "workflow_dispatch")
	if code != ExitOK {
		t.Fatalf("a matrix that needs an input that was not given made the plan fail: %d\n%s", code, errOut)
	}
	for _, want := range []string{"matrixed (matrix known at run time: cannot be evaluated yet", "typed   runs-on: x   if: inputs.debug == true", "=> if is true"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q\n%s", want, out)
		}
	}
	// the condition that reads needs is left alone instead of being evaluated against an empty context
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "later") && strings.Contains(line, "=> if is") {
			t.Errorf("a condition on needs was evaluated: %s", line)
		}
	}
	// a boolean given with -i is a boolean
	_, out, _ = runMain(t, dir, "--list", "w", "-e", "workflow_dispatch", "-i", "debug=false")
	if !strings.Contains(out, "=> if is false") {
		t.Errorf("-i debug=false:\n%s", out)
	}
}

func TestListSeesEventInputsAndDoesNotGuessStatuses(t *testing.T) {
	src := `on:
  workflow_dispatch:
    inputs:
      tag:
        default: v1
      flag:
        type: boolean
        default: true
jobs:
  first:
    runs-on: x
    steps: [{run: e}]
  by-event-input:
    needs: first
    if: github.event.inputs.tag == 'v1' && github.event.inputs.flag == 'true'
    runs-on: x
    steps: [{run: e}]
  after-failure:
    needs: first
    if: failure()
    runs-on: x
    steps: [{run: e}]
  always-true:
    needs: first
    if: failure() || true
    runs-on: x
    steps: [{run: e}]
  not-pushed:
    needs: first
    if: github.event_name == 'push'
    runs-on: x
    steps: [{run: e}]
`
	dir := repoWith(t, map[string]string{"w.yml": src})
	_, out, _ := runMain(t, dir, "--list", "w", "-e", "workflow_dispatch")
	line := func(job string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), job+" ") {
				return l
			}
		}
		return ""
	}
	if l := line("by-event-input"); strings.Contains(l, "=> if is") {
		t.Errorf("success of the needed job is not known, so the condition is not decided: %s", l)
	}
	if l := line("after-failure"); strings.Contains(l, "=> if is") {
		t.Errorf("failure() with needs must stay open: %s", l)
	}
	if l := line("always-true"); !strings.Contains(l, "=> if is true") {
		t.Errorf("failure() || true is definite: %s", l)
	}
	if l := line("not-pushed"); !strings.Contains(l, "=> if is false") {
		t.Errorf("a false condition stays false whatever the needs do: %s", l)
	}
	// without needs the inputs are visible in github.event.inputs
	src2 := strings.Replace(src, "  by-event-input:\n    needs: first\n", "  by-event-input:\n", 1)
	dir2 := repoWith(t, map[string]string{"w.yml": src2})
	_, out, _ = runMain(t, dir2, "--list", "w", "-e", "workflow_dispatch")
	if l := line("by-event-input"); !strings.Contains(l, "=> if is true") {
		t.Errorf("github.event.inputs is not filled: %s", l)
	}
}
