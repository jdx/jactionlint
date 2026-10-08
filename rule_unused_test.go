package jactionlint

import "testing"

func TestRuleUnusedJobOutput(t *testing.T) {
	tests := []struct {
		what string
		src  string
	}{
		{"never read", `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      version: ${{ steps.v.outputs.version }} # want
      used: ${{ steps.v.outputs.used }}
    steps:
      - id: v
        run: echo "version=1" >> "$GITHUB_OUTPUT"
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ needs.build.outputs.used }}
`},
		{"read in if, env, with, matrix and runs-on", `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      a: ${{ steps.v.outputs.a }}
      b: ${{ steps.v.outputs.b }}
      c: ${{ steps.v.outputs.c }}
      d: ${{ steps.v.outputs.d }}
      e: ${{ steps.v.outputs.e }}
    steps:
      - id: v
        run: echo
  test:
    needs: build
    if: needs.build.outputs.a == 'yes'
    runs-on: ${{ needs.build.outputs.b }}
    strategy:
      matrix:
        os: ${{ fromJSON(needs.build.outputs.c) }}
    env:
      D: ${{ needs['build'].outputs.d }}
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ needs.build.outputs.e }}
`},
		{"whole outputs object is read", `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      a: x
      b: y
    steps:
      - run: echo
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo '${{ toJSON(needs.build.outputs) }}'
`},
		{"whole needs object is read", `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      a: x
    steps:
      - run: echo
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo '${{ toJSON(needs) }}'
`},
		{"dynamic job name", `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      a: x
    steps:
      - run: echo
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo '${{ needs[format('b{0}', 'uild')].outputs.a }}'
`},
		{"workflow_call output", `on:
  workflow_call:
    outputs:
      version:
        value: ${{ jobs.build.outputs.version }}
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      version: 1
    steps:
      - run: echo
`},
		{"unused in a reusable workflow", `on:
  workflow_call:
    outputs:
      version:
        value: ${{ jobs.build.outputs.version }}
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      version: 1
      other: 2 # want
    steps:
      - run: echo
`},
		{"a different job with the same output name", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    outputs:
      x: 1 # want
    steps:
      - run: echo
  b:
    needs: a
    runs-on: ubuntu-latest
    outputs:
      x: 2
    steps:
      - run: echo
  c:
    needs: b
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ needs.b.outputs.x }}
`},
		{"an expression that does not parse hides references", `on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      a: x
    steps:
      - run: echo ${{ needs.build.outputs.a == }}
`},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, "", tc.src, "unused-job-output"), wantLines(tc.src)...)
		})
	}
}

func TestRuleUnusedWorkflowInput(t *testing.T) {
	tests := []struct {
		what string
		src  string
	}{
		{"dispatch input never read", `on:
  workflow_dispatch:
    inputs:
      used:
        type: string
      dry-run: # want
        type: boolean
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ inputs.used }}
`},
		{"read as github.event.inputs", `on:
  workflow_dispatch:
    inputs:
      a:
        type: string
      b:
        type: string
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ github.event.inputs.a }} ${{ github.event.inputs['b'] }}
`},
		{"call input never read", `on:
  workflow_call:
    inputs:
      used:
        type: string
      Unused: # want
        type: string
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ inputs.USED }}
`},
		{"read in if, run-name, job name and a reusable workflow call", `run-name: ${{ inputs.a }}
on:
  workflow_dispatch:
    inputs:
      a:
        type: string
      b:
        type: boolean
      c:
        type: string
      d:
        type: string
jobs:
  j:
    if: inputs.b
    name: ${{ inputs.c }}
    runs-on: ubuntu-latest
    steps:
      - run: echo
  k:
    uses: ./.github/workflows/x.yml
    with:
      d: ${{ inputs.d }}
`},
		{"all inputs are read at once", `on:
  workflow_dispatch:
    inputs:
      a:
        type: string
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: echo '${{ toJSON(inputs) }}'
`},
		{"the event payload is read by a script", `on:
  workflow_dispatch:
    inputs:
      a:
        type: string
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: jq .inputs "$GITHUB_EVENT_PATH"
`},
		{"dynamic access", `on:
  workflow_dispatch:
    inputs:
      a:
        type: string
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ inputs[matrix.name] }}
`},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, pedanticCfg, tc.src, "unused-workflow-input"), wantLines(tc.src)...)
		})
	}
}

func TestRuleUnusedNeeds(t *testing.T) {
	const cfg = "profile: strict\n"
	tests := []struct {
		what string
		src  string
	}{
		{"already needed through another job", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
  b:
    needs: a
    runs-on: ubuntu-latest
    steps:
      - run: echo
  c:
    needs:
      - a # want
      - b
    runs-on: ubuntu-latest
    steps:
      - run: echo
`},
		{"through two jobs and in flow style", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
  b:
    needs: a
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - run: echo
  c:
    needs: [b]
    runs-on: ubuntu-latest
    steps:
      - run: echo
  d:
    needs: [a, c]
    runs-on: ubuntu-latest
    steps:
      - run: echo
`},
		{"the output of the job is read", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    outputs:
      v: 1
    steps:
      - run: echo
  b:
    needs: a
    runs-on: ubuntu-latest
    steps:
      - run: echo
  c:
    needs: [a, b]
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ needs.a.outputs.v }}
`},
		{"the result of the job is read", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
  b:
    needs: a
    runs-on: ubuntu-latest
    steps:
      - run: echo
  c:
    needs: [a, b]
    if: always() && needs.a.result == 'success'
    runs-on: ubuntu-latest
    steps:
      - run: echo
`},
		{"the other job runs even if the first one failed", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
  b:
    needs: a
    if: always()
    runs-on: ubuntu-latest
    steps:
      - run: echo
  c:
    needs: [a, b]
    runs-on: ubuntu-latest
    steps:
      - run: echo
`},
		{"needed only for the order", `on: push
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
  b:
    runs-on: ubuntu-latest
    steps:
      - run: echo
  c:
    needs: [a, b]
    runs-on: ubuntu-latest
    steps:
      - run: echo
`},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			want := wantLines(tc.src)
			if tc.what == "through two jobs and in flow style" {
				want = []string{"19"}
			}
			checkLines(t, lintBatchH(t, cfg, tc.src, "unused-needs"), want...)
		})
	}
}
