package jactionlint

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lintRequiredChecks lints a repository with the config and the workflows and returns the findings of
// unreachable-required-check as "FILE:LINE: [info: ]MESSAGE".
func lintRequiredChecks(t *testing.T, cfg string, workflows map[string]string) []string {
	t.Helper()
	root := ignoreProject(t, cfg, workflows)
	var ret []string
	for _, e := range ofRule(lintIgnoreProject(t, root, time.Now()), "unreachable-required-check") {
		sev := ""
		if e.Severity == SeverityInfo {
			sev = "info: "
		}
		ret = append(ret, fmt.Sprintf("%s:%d: %s%s", filepath.ToSlash(e.Filepath), e.Line, sev, e.Message))
	}
	return ret
}

// expectFindings checks that each finding contains the parts of one expectation, in order.
func expectFindings(t *testing.T, got []string, want ...[]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i, parts := range want {
		for _, p := range parts {
			if !strings.Contains(got[i], p) {
				t.Errorf("finding %d does not contain %q:\n%s", i, p, got[i])
			}
		}
	}
}

const gateWorkflow = `on:
  pull_request:
  merge_group:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: make test
  gate:
    name: CI gate
    needs: [test]
    if: always()
    runs-on: ubuntu-latest
    steps:
      - run: test "${{ needs.test.result }}" = success
`

func TestRequiredCheckAlwaysReportingGate(t *testing.T) {
	cfg := `required-checks:
  - name: CI gate
    events: [pull_request, merge_group]
    branches: [main]
`
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": gateWorkflow}))
}

func TestRequiredCheckUnresolvableName(t *testing.T) {
	cfg := `required-checks:
  - name: ci gate
`
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": gateWorkflow}),
		[]string{".github/jactionlint.yaml:2: ", `no job of the workflows in .github/workflows reports the required check "ci gate"`, `did you mean "CI gate"?`})
}

func TestRequiredCheckJobOfReusableWorkflowIsNamedByItsCaller(t *testing.T) {
	callee := `on: workflow_call
jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - run: make test
`
	caller := `on: pull_request
jobs:
  call:
    name: ci
    uses: ./.github/workflows/unit.yaml
`
	ws := map[string]string{"ci.yaml": caller, "unit.yaml": callee}
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: ci / unit\n", ws))
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: unit\n", ws),
		[]string{".github/jactionlint.yaml:2: ", `the required check "unit"`, `did you mean "ci / unit"?`})
}

func TestRequiredCheckMissingEvent(t *testing.T) {
	wf := `on:
  pull_request:
jobs:
  gate:
    name: CI gate
    runs-on: ubuntu-latest
    steps:
      - run: make test
`
	cfg := `required-checks:
  - name: CI gate
    events: [pull_request, merge_group]
`
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": wf}),
		[]string{".github/workflows/ci.yaml:2: ", `required check "CI gate" never reports for a merge queue group, because the workflow does not run on "merge_group"`, `add "merge_group" to "on:"`, "declared at .github/jactionlint.yaml:2"})
}

func TestRequiredCheckEventCoveredByAnotherWorkflow(t *testing.T) {
	// GitHub accepts the check from any workflow which reports it
	pr := "on: pull_request\njobs:\n  gate:\n    name: CI gate\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n"
	mq := "on: merge_group\njobs:\n  gate:\n    name: CI gate\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n"
	cfg := "required-checks:\n  - name: CI gate\n    events: [pull_request, merge_group]\n"
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"pr.yaml": pr, "mq.yaml": mq}))
}

func TestRequiredCheckExcludedByPathFilters(t *testing.T) {
	cfg := "required-checks:\n  - name: test\n"
	paths := `on:
  pull_request:
    paths:
      - 'src/**'
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: make test
`
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": paths}),
		[]string{".github/workflows/ci.yaml:3: ", `required check "test" never reports for a pull request which changes only "README.md", because no entry of "paths" of "pull_request" matches "README.md"`})

	ignore := `on:
  pull_request:
    paths-ignore:
      - '**.md'
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: make test
`
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": ignore}),
		[]string{".github/workflows/ci.yaml:4: ", `for a pull request which changes only "README.md", because the entry "**.md" of "paths-ignore" of "pull_request" matches "README.md"`})
}

func TestRequiredCheckPathFilterWithOppositeTwin(t *testing.T) {
	// GitHub's documented workaround: a second workflow with the same job name and the opposite filter reports the
	// check when the first one is skipped
	run := "on:\n  pull_request:\n    paths: ['src/**']\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n"
	skip := "on:\n  pull_request:\n    paths-ignore: ['src/**']\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo skipped\n"
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: test\n", map[string]string{"ci.yaml": run, "ci-skip.yaml": skip}))
}

func TestRequiredCheckBranchFilter(t *testing.T) {
	wf := "on:\n  pull_request:\n    branches: [main]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n"
	ws := map[string]string{"ci.yaml": wf}
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: test\n    branches: [main, release]\n", ws),
		[]string{".github/workflows/ci.yaml:3: ", `never reports for a pull request into "release", because no entry of "branches" of "pull_request" matches the branch "release"`})
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: test\n    branches: [main]\n", ws))
	// Without declared branches the filter cannot be judged
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: test\n", ws),
		[]string{".github/jactionlint.yaml:2: info: ", `cannot determine whether the required check "test"`, `filters "pull_request" by branch and the declaration names no "branches"`})
}

func TestRequiredCheckActivityTypes(t *testing.T) {
	wf := "on:\n  pull_request:\n    types: [opened, reopened]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n"
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: test\n", map[string]string{"ci.yaml": wf}),
		[]string{".github/workflows/ci.yaml:3: ", `never reports for a commit pushed to a pull request, because the "types" of "pull_request" do not include "synchronize"`})
}

func TestRequiredCheckMatrixNames(t *testing.T) {
	wf := `on: pull_request
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
        node: [20, 22]
    runs-on: ${{ matrix.os }}
    steps:
      - run: make test
  named:
    name: e2e on ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - run: make e2e
`
	cfg := `required-checks:
  - name: test (ubuntu-latest, 20)
  - name: test (22, macos-latest)
  - name: e2e on ubuntu-latest
  - name: test (windows-latest, 20)
`
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": wf}),
		[]string{".github/jactionlint.yaml:5: ", `reports the required check "test (windows-latest, 20)"`})
}

func TestRequiredCheckMatrixJobThatNeverRuns(t *testing.T) {
	wf := `on: pull_request
jobs:
  test:
    if: false
    strategy:
      matrix:
        os: [ubuntu-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - run: make test
`
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: test (ubuntu-latest)\n", map[string]string{"ci.yaml": wf}),
		[]string{".github/workflows/ci.yaml:4: ", `required check "test (ubuntu-latest)" is never reported, because job "test" never runs`})
}

func TestRequiredCheckConditionalJobStillReports(t *testing.T) {
	// A job skipped by its condition reports a check which counts as passing: it is not a missing check
	wf := `on: pull_request
jobs:
  deploy-preview:
    if: github.event.pull_request.head.repo.fork == false
    runs-on: ubuntu-latest
    steps:
      - run: make preview
`
	expectFindings(t, lintRequiredChecks(t, "required-checks:\n  - name: deploy-preview\n", map[string]string{"ci.yaml": wf}))
}

func TestRequiredCheckUndecidable(t *testing.T) {
	for _, tc := range []struct {
		what, check, wf, why string
	}{
		{
			"dynamic job name", "test-main",
			"on: pull_request\njobs:\n  test:\n    name: test-${{ github.base_ref }}\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n",
			`job "test" of .github/workflows/ci.yaml has a name computed at runtime`,
		},
		{
			"matrix from an expression", "test (ubuntu-latest)",
			"on: pull_request\njobs:\n  test:\n    strategy:\n      matrix: ${{ fromJSON(vars.MATRIX) }}\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n",
			`has a matrix that is not a literal list of values`,
		},
		{
			"reusable workflow of another repository", "test / unit",
			"on: pull_request\njobs:\n  test:\n    uses: octo-org/ci/.github/workflows/test.yaml@v1\n",
			`calls the reusable workflow "octo-org/ci/.github/workflows/test.yaml@v1" of another repository`,
		},
		{
			"paths filter with a negation", "test",
			"on:\n  pull_request:\n    paths: ['src/**', '!src/docs/**']\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n",
			`cannot evaluate the "paths" of "pull_request"`,
		},
		{
			"paths filter with an expression", "test",
			"on:\n  pull_request:\n    paths: ['${{ vars.SRC }}/**']\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make test\n",
			`cannot evaluate the "paths" of "pull_request"`,
		},
	} {
		t.Run(tc.what, func(t *testing.T) {
			expectFindings(t, lintRequiredChecks(t, fmt.Sprintf("required-checks:\n  - name: %s\n", tc.check), map[string]string{"ci.yaml": tc.wf}),
				[]string{".github/jactionlint.yaml:2: info: ", "cannot determine whether the required check", tc.why})
		})
	}
}

func TestRequiredCheckRuleCanBeTurnedOff(t *testing.T) {
	cfg := "rules:\n  unreachable-required-check: off\nrequired-checks:\n  - name: missing\n"
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": gateWorkflow}))
}

func TestRequiredCheckRunsInEveryProfile(t *testing.T) {
	cfg := "profile: correctness\nrequired-checks:\n  - name: missing\n"
	expectFindings(t, lintRequiredChecks(t, cfg, map[string]string{"ci.yaml": gateWorkflow}),
		[]string{`reports the required check "missing"`})
}

func TestParseRequiredChecks(t *testing.T) {
	c, err := ParseConfig([]byte("required-checks:\n  - name: CI gate\n    events: [pull_request, merge_group]\n    branches: [main]\n  - name: lint\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.RequiredChecks) != 2 || c.RequiredChecks[0].Line != 2 || c.RequiredChecks[0].Column != 11 {
		t.Fatalf("unexpected declarations: %+v", c.RequiredChecks)
	}
	if got := strings.Join(c.RequiredChecks[1].events(), ","); got != "pull_request" {
		t.Errorf("default events are %q", got)
	}
	for _, tc := range []struct{ cfg, want string }{
		{"required-checks:\n  - events: [pull_request]\n", `"name" is required`},
		{"required-checks:\n  - name: x\n    events: [issues]\n", `unknown event "issues"`},
		{"required-checks:\n  - name: x\n    branches: ['release/*']\n", `must be a literal branch name`},
		{"required-checks:\n  - name: x\n    workflow: ci.yaml\n", `unknown key "workflow"`},
	} {
		if _, err := ParseConfig([]byte(tc.cfg)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("config %q: got error %v, want %q", tc.cfg, err, tc.want)
		}
	}
}
