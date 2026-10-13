package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// The labels of "self-hosted-runner.labels" and a runner group are self-hosted too, a pull request event makes the finding
// say so, and the options allow and pull-request-only narrow it down.
func TestSelfHostedRunnerConfiguredLabelsAndGroups(t *testing.T) {
	const base = "self-hosted-runner:\n  labels: [arc-runner-set, \"gpu-*\"]\nrules:\n  self-hosted-runner: info\n"
	tests := []struct {
		name string
		cfg  string
		on   string
		runs string
		// want is a text of the message, "" for no finding
		want string
		// absent is a text which the message must not have
		absent string
	}{
		{"label of the configuration", "", "push", "arc-runner-set", `label "arc-runner-set", which "self-hosted-runner.labels"`, "pull request of a fork"},
		{"glob of the configuration", "", "push", "[gpu-a100, linux]", `label "gpu-a100"`, ""},
		{"case of the label", "", "push", "ARC-Runner-Set", `label "ARC-Runner-Set"`, ""},
		{"label of no configuration", "", "push", "arc-other", "", ""},
		{"group", "", "push", "\n      group: ci-runners", `runner group "ci-runners"`, ""},
		{"group and labels", "", "push", "\n      group: ci-runners\n      labels: [linux]", `runner group "ci-runners"`, ""},
		{"group of an expression", "", "push", "\n      group: ${{ inputs.group }}", "", ""},
		{"hosted label next to a group is still a group", "", "push", "\n      group: g\n      labels: ubuntu-latest", `runner group "g"`, ""},
		{"self-hosted on a pull request", "", "pull_request", "[self-hosted, linux]", "started by pull_request, so code of a pull request of a fork can run on the runner", ""},
		{"several pull request events", "", "[pull_request_target, pull_request, push]", "arc-runner-set", "started by pull_request_target and pull_request,", ""},
		{"push is not a pull request", "", "[push, schedule]", "arc-runner-set", "label", "pull request of a fork"},
		{"allow a label", "rules:\n  self-hosted-runner:\n    allow: [\"arc-*\"]\n", "pull_request", "arc-runner-set", "", ""},
		{"allow does not cover other labels", "rules:\n  self-hosted-runner:\n    allow: [\"arc-*\"]\n", "push", "[self-hosted, gpu-1]", `label "self-hosted"`, ""},
		{"allow a label next to self-hosted", "rules:\n  self-hosted-runner:\n    allow: [ephemeral]\n", "push", "[self-hosted, ephemeral]", "", ""},
		{"allow a group", "rules:\n  self-hosted-runner:\n    allow: [\"ci-*\"]\n", "push", "\n      group: ci-runners", "", ""},
		{"only pull requests: push", "rules:\n  self-hosted-runner:\n    pull-request-only: true\n", "push", "self-hosted", "", ""},
		{"only pull requests: pull request", "rules:\n  self-hosted-runner:\n    pull-request-only: true\n", "pull_request", "self-hosted", "started by pull_request", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgSrc := base
			if tc.cfg != "" {
				// the rule section of the case replaces the one of the base
				cfgSrc = "self-hosted-runner:\n  labels: [arc-runner-set, \"gpu-*\"]\n" + tc.cfg
			}
			src := "on: " + tc.on + "\njobs:\n  a:\n    runs-on: " + tc.runs + "\n    steps:\n      - run: echo\n"
			var got []string
			for _, e := range lintFileWithConfig(t, mustParseConfig(t, cfgSrc), "ci.yaml", src) {
				if e.ID == "self-hosted-runner" {
					got = append(got, e.Message)
				}
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no finding, got %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want one finding with %q, got %q", tc.want, got)
			}
			if tc.absent != "" && strings.Contains(got[0], tc.absent) {
				t.Errorf("the message has %q: %q", tc.absent, got[0])
			}
		})
	}
}

// A reusable workflow is judged by the events of the workflows which call it.
func TestSelfHostedRunnerPullRequestThroughCallers(t *testing.T) {
	const callee = "on: workflow_call\njobs:\n  j:\n    runs-on: [self-hosted]\n    steps:\n      - run: echo\n"
	caller := func(on string) string {
		return "on:\n  " + on + "\njobs:\n  c:\n    uses: ./.github/workflows/callee.yml\n"
	}
	for name, tc := range map[string]struct {
		callers map[string]string
		want    bool
	}{
		"called on a pull request": {map[string]string{"a.yml": caller("pull_request:")}, true},
		"called on push":           {map[string]string{"a.yml": caller("push:")}, false},
		"no known caller":          {map[string]string{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			files := map[string]string{".github/workflows/callee.yml": callee}
			for k, v := range tc.callers {
				files[".github/workflows/"+k] = v
			}
			root := writeProject(t, files)
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  self-hosted-runner: info\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "callee.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := errsWithID(errs, "self-hosted-runner")
			if len(got) != 1 {
				t.Fatalf("want one finding, got %v", got)
			}
			if has := strings.Contains(got[0].Message, "pull request of a fork"); has != tc.want {
				t.Errorf("pull request note = %v, want %v: %s", has, tc.want, got[0].Message)
			}
		})
	}
}
