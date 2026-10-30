package runner

import (
	"bytes"
	"encoding/json"
	"github.com/jdx/jactionlint/v2/internal/runner/cache"
	"os"
	"path/filepath"
	"runtime"
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
	if test == nil || !test.Selected || len(test.Cells) != 3 || test.Cells[2].Matrix["os"] != "my-runner" {
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
		"allow-net without proxy":      {[]string{"--allow-net", "x.example", "--no-lint", "a"}, ExitUsage, "only applies to --net=proxy"},
		"learn with none":              {[]string{"--learn", "--net=none", "--no-lint", "a"}, ExitUsage, "--learn needs --net=proxy"},
		"bad sandbox value":            {[]string{"--sandbox=maybe", "--no-lint", "a"}, ExitUsage, "--sandbox must be on or off"},
		"bad secret":                   {[]string{"--secret", "nope", "a"}, ExitUsage, "NAME=VALUE"},
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

func TestCacheCommand(t *testing.T) {
	state := t.TempDir()
	t.Setenv("JACTIONLINT_CACHE_DIR", state)
	dir := repoWith(t, nil)
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{In: &Inputs{Repo: readRepoInfo(findRepoRoot(dir))}}
	repoDir, err := cfg.repoCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	st, err := cache.Open(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"go-a", "go-b", "rust-a"} {
		p, _ := st.Reserve("refs/heads/main", k, "v")
		os.WriteFile(p.Path, []byte("12345"), 0o600)
		if _, err := st.Commit(p, 5); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := CacheMain(args, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	if code, out, _ := run("path"); code != 0 || strings.TrimSpace(out) != repoDir {
		t.Fatalf("path: %d %q", code, out)
	}
	code, out, _ := run("ls")
	if code != 0 || !strings.Contains(out, "go-a") || !strings.Contains(out, "rust-a") || !strings.Contains(out, "5 B") || !strings.Contains(out, "main") {
		t.Fatalf("ls: %d\n%s", code, out)
	}
	if code, out, _ := run("rm", "go-*"); code != 0 || !strings.Contains(out, "removed 2 entries") {
		t.Fatalf("rm prefix: %d %q", code, out)
	}
	if code, _, _ := run("rm", "nothing"); code != ExitFailure {
		t.Fatalf("rm of nothing must fail, got %d", code)
	}
	if code, out, _ := run("prune"); code != 0 || !strings.Contains(out, "removed") {
		t.Fatalf("prune: %d %q", code, out)
	}
	if code, out, _ := run("ls", "--all"); code != 0 || !strings.Contains(out, "rust-a") {
		t.Fatalf("ls --all: %d %q", code, out)
	}
	for _, args := range [][]string{{}, {"nope"}, {"rm"}, {"--bogus"}, {"ls", "--cache-size", "x"}} {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want usage", args, code)
		}
	}
	if code, out, _ := run("--help"); code != 0 || !strings.Contains(out, "prune") {
		t.Errorf("help: %d", code)
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

func TestDryRun(t *testing.T) {
	src := `name: dry
on: push
env:
  WORKFLOW_ENV: from-workflow
jobs:
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    steps:
      - id: v
        run: |
          echo "version=1" >> "$GITHUB_OUTPUT"
          echo "building on ${{ matrix.os }} as ${{ github.actor }} in ${{ github.repository }}"
        env:
          TOKEN: ${{ secrets.TOKEN }}
          PLAIN: x
      - if: failure()
        run: echo "only after a failure"
      - uses: actions/cache@v4
        with:
          path: ~/.cache
          key: k-${{ matrix.os }}
      - run: echo "the output is '${{ steps.v.outputs.version }}'"
        shell: python
        working-directory: sub
`
	dir := repoWith(t, map[string]string{"dry.yml": src})
	marker := filepath.Join(dir, "must-not-exist")
	os.WriteFile(filepath.Join(dir, ".github", "workflows", "dry.yml"), []byte(strings.Replace(src, `echo "version=1" >> "$GITHUB_OUTPUT"`, `touch `+marker+`; echo "version=1" >> "$GITHUB_OUTPUT"`, 1)), 0o644)
	code, out, errOut := runMain(t, dir, "dry", "--dry-run", "--no-lint", "--secret", "TOKEN=hunter2")
	if code != ExitOK {
		t.Fatalf("code %d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{
		"[job] build (ubuntu-latest)  [dry run, on this machine]",
		`building on ubuntu-latest as `,
		"env TOKEN=***",
		"env PLAIN=x",
		"if: failure()  => false",
		"uses: actions/cache@v4",
		"with key=k-ubuntu-latest",
		"with path=~/.cache",
		"shell: python {0}",
		"working-directory: sub",
		"the output is ''",
		"[job] build (macos-latest): skipped (runs-on macos-latest needs macOS",
	} {
		if !strings.Contains(out, want) && runtime.GOOS == "linux" {
			t.Errorf("dry run lacks %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("a secret leaked into the dry run\n%s", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a dry run ran a step")
	}
	if _, err := os.Stat(filepath.Join(dir, ".jactionlint")); err == nil {
		t.Error("a dry run created a run directory")
	}
}

func TestListShowsWhereCellsRun(t *testing.T) {
	src := `on: push
jobs:
  j:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps: [{run: e}]
  boxed:
    runs-on: ubuntu-latest
    container: ubuntu:24.04
    steps: [{run: e}]
  db:
    runs-on: ubuntu-latest
    services:
      pg:
        image: postgres
    steps: [{run: e}]
`
	dir := repoWith(t, map[string]string{"w.yml": src})
	_, out, _ := runMain(t, dir, "--list", "w", "--container")
	for _, want := range []string{
		"j (ubuntu-latest)   runs-on: ubuntu-latest   => runs in a container: ubuntu:24.04@sha256:fe81",
		"j (macos-latest)   runs-on: macos-latest   => skipped: needs macOS",
		"j (windows-latest)   runs-on: windows-latest   => skipped: needs Windows",
		"=> runs in the job container ubuntu:24.04",
	} {
		if !strings.Contains(out, want) && runtime.GOOS != "darwin" {
			t.Errorf("--list lacks %q\n%s", want, out)
		}
	}
	_, out, _ = runMain(t, dir, "--list", "w")
	if !strings.Contains(out, "=> runs on this machine with its services in containers") {
		t.Errorf("--list lacks the note about services\n%s", out)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(out, "j (ubuntu-latest)   runs-on: ubuntu-latest   => skipped: needs Linux (--container runs it in a Linux container)") {
		t.Errorf("on macOS a Linux cell says how to run it\n%s", out)
	}
}

// The workflows of this repository go through --list and --dry-run in CI, which catches regressions of the evaluator and
// of the matrix expansion without running a step.
func TestOwnWorkflowsPlanAndDryRun(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	if len(files) == 0 {
		t.Skip("no workflows in " + root)
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			for _, args := range [][]string{{"--list"}, {"--list", "--json"}, {"--dry-run", "--no-lint"}} {
				var out, errOut bytes.Buffer
				code := Main(append([]string{"-C", root, name}, args...), strings.NewReader(""), &out, &errOut)
				if code != ExitOK {
					t.Errorf("%s %v: exit %d\n%s\n%s", name, args, code, out.String(), errOut.String())
				}
				if out.Len() == 0 {
					t.Errorf("%s %v: no output", name, args)
				}
			}
		})
	}
}
