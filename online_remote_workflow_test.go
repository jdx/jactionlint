package jactionlint

import (
	"fmt"
	"strings"
	"testing"
)

const remoteCallee = `on:
  workflow_call:
    inputs:
      name:
        type: string
        required: true
      count:
        type: number
      flag:
        type: boolean
        default: true
      anything:
    secrets:
      token:
        required: true
      extra: {}
    outputs:
      result:
        value: ${{ jobs.a.outputs.r }}
permissions:
  contents: read
jobs:
  a:
    runs-on: ubuntu-latest
    steps:
      - run: echo
`

func jobsWith(jobs ...string) string {
	return "on: push\njobs:\n" + strings.Join(jobs, "")
}

func TestRemoteWorkflowCallIsCheckedAtThePinnedRef(t *testing.T) {
	c := newFilesClient(t, map[string]string{"o/w@v1:.github/workflows/ci.yml": remoteCallee})
	src := jobsWith(
		"  a:\n    uses: o/w/.github/workflows/ci.yml@v1\n",
		"  b:\n    uses: o/w/.github/workflows/ci.yml@v1\n    with:\n      name: x\n      cuont: 1\n    secrets:\n      token: x\n      tokn: x\n",
		"  c:\n    uses: o/w/.github/workflows/ci.yml@v1\n    with:\n      name: x\n      count: one\n      flag: ${{ fromJSON('1') }}\n    secrets: inherit\n", // type of count
		"  d:\n    uses: o/w/.github/workflows/ci.yml@v1\n    with:\n      name: x\n      count: 1\n      flag: false\n      anything: x\n    secrets:\n      token: x\n      extra: y\n",
	)
	errs, l := lintRemote(t, c, src)
	var got []string
	for _, e := range errs {
		got = append(got, fmt.Sprintf("%d:%s", e.Line, e.ID))
	}
	want := []string{
		"4:missing-workflow-input", "4:missing-workflow-secret", "9:unknown-workflow-input", "12:unknown-workflow-secret", "17:workflow-input-type",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("findings = %v, want %v\n%v", got, want, errs)
	}
	if n := c.calls.Load(); n != 1 {
		t.Errorf("one workflow at one ref is read once, whatever the number of jobs and rules which use it: %d lookups %v", n, c.asked)
	}
	if l.OnlineSkipped() != 0 {
		t.Errorf("skipped = %d", l.OnlineSkipped())
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, "read from GitHub") {
			t.Errorf("the message must say where the workflow came from: %s", e.Message)
		}
	}
}

func TestRemoteWorkflowOutputsOfNeedsAreTyped(t *testing.T) {
	c := newFilesClient(t, map[string]string{"o/w@v1:.github/workflows/ci.yml": remoteCallee})
	src := jobsWith(
		"  call:\n    uses: o/w/.github/workflows/ci.yml@v1\n    with:\n      name: x\n    secrets: inherit\n",
		"  use:\n    needs: call\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ needs.call.outputs.result }} ${{ needs.call.outputs.RESULT }}\n      - run: echo ${{ needs.call.outputs.other }}\n",
	)
	errs, _ := lintRemote(t, c, src)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, `property "other" is not defined in object type {result: string}`) {
		t.Errorf("findings = %v", errs)
	}
}

func TestRemoteWorkflowIsNotLookedUpWhenThereIsNothingToCheckAgainst(t *testing.T) {
	for name, uses := range map[string]string{
		"local":               "./.github/workflows/ci.yml",
		"self repository":     "$/.github/workflows/ci.yml",
		"expression":          "o/w/.github/workflows/ci.yml@${{ vars.REF }}",
		"expression in path":  "o/w/.github/workflows/${{ vars.F }}.yml@v1",
		"short sha":           "o/w/.github/workflows/ci.yml@abcdef1",
		"not a workflow file": "o/w/.github/workflows/ci.txt@v1",
		"an action":           "o/w/sub@v1",
		"odd path":            "o/w/.github/work flows/ci.yml@v1",
		"dot dot":             "o/w/.github/../ci.yml@v1",
		"no ref":              "o/w/.github/workflows/ci.yml",
		"bad owner":           "-o/w/.github/workflows/ci.yml@v1",
	} {
		t.Run(name, func(t *testing.T) {
			c := newFilesClient(t, map[string]string{})
			lintRemote(t, c, jobsWith("  a:\n    uses: "+uses+"\n"))
			if n := c.calls.Load(); n != 0 {
				t.Errorf("%d lookups for %q: %v", n, uses, c.asked)
			}
		})
	}
}

// Nothing is reported when the workflow is not known, whatever the reason.
func TestRemoteWorkflowReportsNothingWhenItIsUnknown(t *testing.T) {
	src := jobsWith(
		"  a:\n    uses: o/w/.github/workflows/ci.yml@v1\n    with:\n      typo: x\n    secrets:\n      typo: x\n",
		"  b:\n    needs: a\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ needs.a.outputs.anything }}\n",
	)
	for name, c := range map[string]*filesClient{
		"not found":         newFilesClient(t, nil),
		"rate limit":        failingFilesClient(t, &GitHubRateLimitError{}),
		"server error":      failingFilesClient(t, &GitHubStatusError{Status: 502}),
		"forbidden":         failingFilesClient(t, &GitHubStatusError{Status: 403}),
		"no workflow_call":  newFilesClient(t, map[string]string{"o/w@v1:.github/workflows/ci.yml": "on: push\njobs: {}\n"}),
		"not even yaml":     newFilesClient(t, map[string]string{"o/w@v1:.github/workflows/ci.yml": "on: [\n"}),
		"inputs not a map":  newFilesClient(t, map[string]string{"o/w@v1:.github/workflows/ci.yml": "on:\n  workflow_call:\n    inputs: [a]\n"}),
		"secrets not a map": newFilesClient(t, map[string]string{"o/w@v1:.github/workflows/ci.yml": "on:\n  workflow_call:\n    secrets: [a]\n"}),
	} {
		t.Run(name, func(t *testing.T) {
			errs, l := lintRemote(t, c, src)
			if len(errs) != 0 {
				t.Errorf("findings = %v", errs)
			}
			if l.OnlineSkipped() == 0 {
				t.Errorf("a lookup which cannot be used is skipped, which --online=strict and the coverage report")
			}
		})
	}
}

func TestRemoteWorkflowHonorsTheAllowAndDenyLists(t *testing.T) {
	c := newFilesClient(t, map[string]string{"corp/w@v1:.github/workflows/ci.yml": remoteCallee})
	var out strings.Builder
	l, err := NewLinter(&out, &LinterOptions{Online: true, GitHubClient: c, OnlineOptions: OnlineOptions{Deny: []string{"corp/*"}}})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(remoteMetadataConfig())
	errs, err := l.Lint("test.yaml", []byte(jobsWith("  a:\n    uses: corp/w/.github/workflows/ci.yml@v1\n")), &Project{root: t.TempDir()})
	if err != nil || len(errs) != 0 {
		t.Fatalf("%v %v", errs, err)
	}
	if c.calls.Load() != 0 || l.OnlineSkipped() != 0 {
		t.Errorf("an excluded repository is not asked and is not a failure: %d lookups, %d skipped", c.calls.Load(), l.OnlineSkipped())
	}
}

func TestRemoteWorkflowNeedsAClientWhichReadsFiles(t *testing.T) {
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	errs, l := lintRemote(t, c, jobsWith("  a:\n    uses: o/w/.github/workflows/ci.yml@v1\n"))
	if len(errs) != 0 || l.OnlineSkipped() != 0 || c.n.Load() != 0 {
		t.Errorf("%v skipped=%d calls=%d", errs, l.OnlineSkipped(), c.n.Load())
	}
}

// The same workflow at two refs are two files: a commit can add an input which the tag lacks.
func TestRemoteWorkflowIsReadPerRef(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/w@v1:.github/workflows/ci.yml": "on:\n  workflow_call:\n    inputs:\n      a: {}\n",
		"o/w@v2:.github/workflows/ci.yml": "on:\n  workflow_call:\n    inputs:\n      b:\n        required: true\n",
	})
	errs, _ := lintRemote(t, c, jobsWith(
		"  one:\n    uses: o/w/.github/workflows/ci.yml@v1\n    with:\n      a: x\n",
		"  two:\n    uses: o/w/.github/workflows/ci.yml@v2\n    with:\n      a: x\n",
	))
	got := lineIDsOf(errs)
	if len(got) != 2 || !strings.HasSuffix(got[0], "missing-workflow-input") || !strings.HasSuffix(got[1], "unknown-workflow-input") {
		t.Errorf("findings = %v\n%v", got, errs)
	}
	if n := c.calls.Load(); n != 2 {
		t.Errorf("%d lookups: %v", n, c.asked)
	}
}
