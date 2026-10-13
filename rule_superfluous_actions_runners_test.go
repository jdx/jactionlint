package jactionlint

import (
	"strings"
	"testing"
)

const superfluousRunnerCfg = "profile: default\nrules:\n  superfluous-actions: error\n"

// superfluousFile is a job on the runner, with one superfluous action.
func superfluousFile(on, runsOn, extra string) string {
	return on + "jobs:\n  j:\n" + extra + "    runs-on: " + runsOn + "\n    steps:\n      - uses: softprops/action-gh-release@v2\n"
}

const (
	callOnly   = "on:\n  workflow_call:\n    inputs:\n      runner:\n        type: string\n"
	callDflt   = "on:\n  workflow_call:\n    inputs:\n      runner:\n        type: string\n        default: ubuntu-24.04\n"
	callNoDflt = callOnly
)

// callerWorkflow calls the reusable workflow main.yml with the value (or without "with:" when it is empty).
func callerWorkflow(with string) string {
	src := "on: push\njobs:\n  call:\n    uses: ./.github/workflows/main.yml\n"
	if with != "" {
		src += "    with:\n      runner: " + with + "\n"
	}
	return src
}

// A runner that the repository itself shows to be GitHub-hosted is reported, and one that may be self-hosted, or whose
// value is not known, is not (zizmor#1865).
func TestSuperfluousActionsRunnerExpressions(t *testing.T) {
	in := func(runsOn string) string { return superfluousFile("on: push\n", runsOn, "") }
	matrix := func(runsOn string) string {
		return superfluousFile("on: push\n", runsOn, "    strategy:\n      matrix:\n        os: [ubuntu-24.04, macos-14]\n")
	}
	cases := []struct {
		name string
		src  string
		want int
	}{
		// conditions between labels
		{"ternary of two labels", in("${{ github.event_name == 'push' && 'ubuntu-24.04' || 'macos-14' }}"), 1},
		{"ternary with a larger runner", in("${{ github.repository_owner == 'astral-sh' && 'github-ubuntu-24.04-x86_64-4' || 'ubuntu-latest' }}"), 1},
		{"ternary of an expression with !contains", in("${{ !contains(github.event.pull_request.labels.*.name, 'full') && 'ubuntu-24.04' || 'macos-15-intel' }}"), 1},
		{"nested ternary", in("${{ a && 'x' || b && 'ubuntu-24.04' || 'macos-14' }}"), 1},
		{"ternary with a matrix value", matrix("${{ github.event_name == 'push' && matrix.os || 'ubuntu-24.04' }}"), 1},
		{"ternary with a self-hosted label", in("${{ github.event_name == 'push' && 'self-hosted' || 'ubuntu-24.04' }}"), 0},
		{"ternary with an unknown else", in("${{ github.event_name == 'push' && 'ubuntu-24.04' || vars.RUNNER }}"), 0},
		{"ternary with an unknown then", in("${{ github.event_name == 'push' && vars.RUNNER || 'ubuntu-24.04' }}"), 0},
		{"default for a variable", in("${{ vars.RUNNER || 'ubuntu-24.04' }}"), 0},
		{"default for an input", in("${{ inputs.runner || 'ubuntu-24.04' }}"), 0},
		{"a condition only", in("${{ github.event_name == 'push' }}"), 0},
		{"a variable", in("${{ vars.RUNNER }}"), 0},
		{"an environment variable", in("${{ env.RUNNER }}"), 0},
		{"a secret", in("${{ secrets.RUNNER }}"), 0},
		{"an output of another job", in("${{ needs.setup.outputs.runner }}"), 0},
		{"a provider label", in("runs-on=${{ github.run_id }}/runner=2cpu-linux-x64"), 0},
		{"a provider label from a variable", in("${{ format('runs-on={0}/runner=2cpu-linux-x64', github.run_id) }}"), 0},
		// fromJSON of a literal
		{"fromJSON of a string", in("${{ fromJSON('\"ubuntu-24.04\"') }}"), 1},
		{"fromJSON of a list", in("${{ fromJSON('[\"ubuntu-24.04\", \"x64\"]') }}"), 1},
		{"fromJSON of a list in a condition", in("${{ github.event_name == 'push' && fromJSON('[\"ubuntu-24.04\"]') || 'macos-14' }}"), 1},
		{"fromJSON of a list with self-hosted", in("${{ fromJSON('[\"self-hosted\", \"linux\"]') }}"), 0},
		{"fromJSON of a list in a condition with self-hosted", in("${{ github.event_name == 'push' && fromJSON('[\"self-hosted\"]') || 'ubuntu-24.04' }}"), 0},
		{"fromJSON of an output", in("${{ fromJSON(needs.setup.outputs.runner) }}"), 0},
		{"fromJSON of text that is not JSON", in("${{ fromJSON('ubuntu-24.04') }}"), 0},
		{"fromJSON of a list that holds a number", in("${{ fromJSON('[1]') }}"), 0},
		{"fromJSON of an object", in("${{ fromJSON('{\"a\": \"b\"}') }}"), 0},
		{"fromJSON of an empty list", in("${{ fromJSON('[]') }}"), 0},
		{"a literal expression", in("${{ 'ubuntu-24.04' }}"), 1},
		// the matrix, as before
		{"matrix value", matrix("${{ matrix.os }}"), 1},
		{"matrix value in a list", matrix("[\"${{ matrix.os }}\"]"), 1},
		{"matrix value and a label", matrix("[\"${{ matrix.os }}\", x64]"), 1},
		{"matrix value of a property that is not there", matrix("${{ matrix.other }}"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := errsWithID(lintFileWithConfig(t, mustParseConfig(t, superfluousRunnerCfg), "ci.yaml", tc.src), "superfluous-actions")
			if len(got) != tc.want {
				t.Errorf("want %d findings, got %v", tc.want, got)
			}
		})
	}
}

// The inputs of a reusable workflow are the values that the callers of the repository pass.
func TestSuperfluousActionsRunnerInputsOfCallers(t *testing.T) {
	use := "${{ inputs.runner }}"
	cases := []struct {
		name    string
		src     string            // the reusable workflow, .github/workflows/main.yml
		callers map[string]string // the other workflows
		want    int
	}{
		{"a literal", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("ubuntu-24.04")}, 1},
		{"two callers, both hosted", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("ubuntu-24.04"), "b.yml": callerWorkflow("macos-14")}, 1},
		{"a caller that leaves it out and the default", superfluousFile(callDflt, use, ""), map[string]string{"a.yml": callerWorkflow("")}, 1},
		{"a quoted literal", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow(`"ubuntu-24.04"`)}, 1},
		{"a condition of literals", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("${{ github.event_name == 'push' && 'ubuntu-24.04' || 'macos-14' }}")}, 1},
		{"a JSON list", superfluousFile(callOnly, "${{ fromJSON(inputs.runner) }}", ""), map[string]string{"a.yml": callerWorkflow(`'["ubuntu-24.04"]'`)}, 1},
		{"text and an input", superfluousFile(callOnly, "${{ inputs.runner }}-latest", ""), map[string]string{"a.yml": callerWorkflow("ubuntu")}, 0},
		{"the matrix of the caller", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": "on: push\njobs:\n  call:\n    strategy:\n      matrix:\n        os: [ubuntu-24.04, macos-14]\n    uses: ./.github/workflows/main.yml\n    with:\n      runner: ${{ matrix.os }}\n"}, 1},
		{"the matrix of the caller with self-hosted", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": "on: push\njobs:\n  call:\n    strategy:\n      matrix:\n        os: [ubuntu-24.04, self-hosted]\n    uses: ./.github/workflows/main.yml\n    with:\n      runner: ${{ matrix.os }}\n"}, 0},

		{"no caller", superfluousFile(callOnly, use, ""), nil, 0},
		{"no caller and a default", superfluousFile(callDflt, use, ""), nil, 0},
		{"a self-hosted caller", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("self-hosted"), "b.yml": callerWorkflow("ubuntu-24.04")}, 0},
		{"a caller that passes a variable", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("ubuntu-24.04"), "b.yml": callerWorkflow("${{ vars.RUNNER }}")}, 0},
		{"a caller that passes a secret", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("${{ secrets.RUNNER }}")}, 0},
		{"a caller that leaves it out and no default", superfluousFile(callNoDflt, use, ""), map[string]string{"a.yml": callerWorkflow("")}, 0},
		{"a caller that passes its own input", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": callerWorkflow("${{ inputs.runner }}")}, 0},
		{"a caller that is only called", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": "on:\n  workflow_call:\n    inputs:\n      runner:\n        type: string\njobs:\n  call:\n    uses: ./.github/workflows/main.yml\n    with:\n      runner: ubuntu-24.04\n"}, 0},
		{"also started by a person", superfluousFile(callOnly+"  workflow_dispatch:\n    inputs:\n      runner:\n        type: string\n", use, ""), map[string]string{"a.yml": callerWorkflow("ubuntu-24.04")}, 0},
		{"an input that is not declared", superfluousFile("on:\n  workflow_call:\n", use, ""), map[string]string{"a.yml": callerWorkflow("ubuntu-24.04")}, 0},
		{"another input", superfluousFile(callOnly, "${{ inputs.other }}", ""), map[string]string{"a.yml": callerWorkflow("ubuntu-24.04")}, 0},
		{"a default that is an expression", superfluousFile("on:\n  workflow_call:\n    inputs:\n      runner:\n        type: string\n        default: ${{ vars.RUNNER }}\n", use, ""), map[string]string{"a.yml": callerWorkflow("")}, 0},
		{"a workflow that calls another repository", superfluousFile(callOnly, use, ""), map[string]string{"a.yml": "on: push\njobs:\n  call:\n    uses: octo/repo/.github/workflows/main.yml@v1\n    with:\n      runner: ubuntu-24.04\n"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := onlyID(lintInProject(t, tc.callers, tc.src, superfluousRunnerCfg), "superfluous-actions")
			if len(got) != tc.want {
				t.Errorf("want %d findings, got %v", tc.want, got)
			}
		})
	}
}

// The message does not change: the finding is the same, it only reaches more jobs.
func TestSuperfluousActionsRunnerExpressionMessage(t *testing.T) {
	src := superfluousFile("on: push\n", "${{ github.event_name == 'push' && 'ubuntu-24.04' || 'macos-14' }}", "")
	got := errsWithID(lintFileWithConfig(t, mustParseConfig(t, superfluousRunnerCfg), "ci.yaml", src), "superfluous-actions")
	if len(got) != 1 || !strings.Contains(got[0].Message, "action-gh-release") || got[0].Line != 6 {
		t.Errorf("unexpected finding: %v", got)
	}
}
