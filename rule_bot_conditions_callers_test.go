package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

const botReusable = `on:
  workflow_call:
jobs:
  j:
    runs-on: ubuntu-latest
    if: github.event.sender.id == 29139614
    steps:
      - run: echo
`

func botCaller(on string) string {
	return "on: " + on + "\njobs:\n  c:\n    uses: ./.github/workflows/reusable.yml\n"
}

// A reusable workflow is judged by the events of the workflows which call it (issue #115).
func TestBotConditionsReusableWorkflowCallers(t *testing.T) {
	const viaMid = "on: workflow_call\njobs:\n  c:\n    uses: ./.github/workflows/reusable.yml\n"
	tests := []struct {
		name    string
		files   map[string]string
		want    int
		mention string // text which the message must contain
		absent  string // text which the message must not contain
	}{
		{"called only on push", map[string]string{"a.yml": botCaller("push")}, 0, "", ""},
		{"called on schedule and dispatch", map[string]string{"a.yml": botCaller("[schedule, workflow_dispatch]")}, 0, "", ""},
		{"called only on pull request", map[string]string{"a.yml": botCaller("pull_request")}, 1, "github.event.pull_request.user.id", "does not exist"},
		{"called on push and pull request", map[string]string{"a.yml": botCaller("push"), "b.yml": botCaller("pull_request")}, 1, "does not exist on the other events", ""},
		{"called on an event without a pull request", map[string]string{"a.yml": botCaller("issue_comment")}, 1, "no event of this workflow has a pull request", "github.event.pull_request.user"},
		{"called through another reusable workflow", map[string]string{
			"a.yml":   "on: push\njobs:\n  c:\n    uses: ./.github/workflows/mid.yml\n",
			"mid.yml": viaMid,
		}, 0, "", ""},
		{"a pull request reaches it through another reusable workflow", map[string]string{
			"a.yml":   botCaller("push"),
			"b.yml":   "on: pull_request\njobs:\n  c:\n    uses: ./.github/workflows/mid.yml\n",
			"mid.yml": viaMid,
		}, 1, "does not exist on the other events", ""},
		{"no local caller", map[string]string{}, 1, "github.event.pull_request.user.id", "does not exist"},
		{"the only caller is itself only called", map[string]string{"mid.yml": viaMid}, 1, "github.event.pull_request.user.id", "does not exist"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{".github/workflows/reusable.yml": botReusable}
			for k, v := range tc.files {
				files[".github/workflows/"+k] = v
			}
			root := writeProject(t, files)
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  bot-conditions: warn\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "workflows", "reusable.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			errs = errsWithID(errs, "bot-conditions")
			if len(errs) != tc.want {
				t.Fatalf("want %d findings but got %v", tc.want, errs)
			}
			if tc.want == 0 {
				return
			}
			if !strings.Contains(errs[0].Message, tc.mention) {
				t.Errorf("message lacks %q: %s", tc.mention, errs[0].Message)
			}
			if tc.absent != "" && strings.Contains(errs[0].Message, tc.absent) {
				t.Errorf("message has %q: %s", tc.absent, errs[0].Message)
			}
			// The unsafe fix needs every caller to run on a pull request event, as for an action
			if want := tc.name == "called only on pull request"; (errs[0].Fix != nil) != want {
				t.Errorf("fix = %+v, want one: %v", errs[0].Fix, want)
			}
		})
	}
}

// The events of a workflow itself decide too.
func TestBotConditionsOwnEvents(t *testing.T) {
	for on, want := range map[string]int{
		"push":                      0,
		"[push, workflow_dispatch]": 0,
		"[push, pull_request]":      1,
		"pull_request_target":       1,
		"workflow_run":              1,
		"[push, workflow_run]":      1,
		"issues":                    1,
		"pull_request_review":       1,
	} {
		src := "on: " + on + "\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.actor == 'dependabot[bot]'\n    steps:\n      - run: echo\n"
		if got := len(errsWithID(lintWithConfig(t, botConfig(t), src), "bot-conditions")); got != want {
			t.Errorf("on %s: want %d findings but got %d", on, want, got)
		}
	}
}
