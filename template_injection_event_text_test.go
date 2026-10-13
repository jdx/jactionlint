package jactionlint

import (
	"strings"
	"testing"
)

// The free text of an outsider which the table of attacker controlled properties lacks is reported by the default
// profile, and what is not free text or not from an outsider stays with the pedantic option.
func TestTemplateInjectionEventText(t *testing.T) {
	cfg := func(profile string) *Config {
		return mustParseConfig(t, "profile: "+profile+"\nrules:\n  concurrency-limits: off\n  missing-permissions: off\n  missing-timeout: off\n  dangerous-triggers: off\n  unpinned-uses: off\n")
	}
	run := func(on, expr string) string {
		return on + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo '${{ " + expr + " }}'\n"
	}
	tests := []struct {
		name    string
		on      string
		expr    string
		profile string
		want    int
	}{
		{"previous title", "on: issues\n", "github.event.changes.title.from", "default", 1},
		{"previous body", "on: pull_request_target\n", "github.event.changes.body.from", "default", 1},
		{"previous base", "on: pull_request_target\n", "github.event.changes.base.ref.from", "default", 1},
		{"a field no payload has", "on: pull_request_target\n", "github.event.changes.head.ref.from", "default", 0},
		{"mixed case", "on: issues\n", "GitHub.Event.Changes.Title.From", "default", 1},
		{"homepage", "on: pull_request_target\n", "github.event.pull_request.head.repo.homepage", "default", 1},
		{"homepage of the base repository", "on: pull_request_target\n", "github.event.pull_request.base.repo.homepage", "default", 0},
		{"wiki page", "on: gollum\n", "github.event.pages[0].title", "default", 1},
		{"wiki page url", "on: gollum\n", "github.event.pages[0].html_url", "default", 0},
		{"merge queue message", "on: merge_group\n", "github.event.merge_group.head_commit.message", "default", 1},
		{"merge queue author name", "on: merge_group\n", "github.event.merge_group.head_commit.author.name", "default", 1},
		{"merge queue author email", "on: merge_group\n", "github.event.merge_group.head_commit.author.email", "default", 0},
		{"merge queue committer", "on: merge_group\n", "github.event.merge_group.head_commit.committer.name", "default", 0},
		{"merge queue sha", "on: merge_group\n", "github.event.merge_group.head_sha", "default", 0},
		{"workflow run path", "on:\n  workflow_run:\n    workflows: [ci]\n", "github.event.workflow_run.path", "default", 1},
		{"workflow path with another trigger first", "on: [push, workflow_run]\n", "github.event.workflow.path", "default", 1},
		{"referenced workflows", "on:\n  workflow_run:\n    workflows: [ci]\n", "github.event.workflow_run.referenced_workflows[0].path", "default", 1},
		{"workflow path of a dispatch", "on: workflow_dispatch\n", "github.event.workflow.path", "default", 0},
		{"workflow path of a dispatch is a pedantic finding", "on: workflow_dispatch\n", "github.event.workflow.path", "pedantic", 1},
		{"workflow run path of a push", "on: push\n", "github.event.workflow_run.path", "default", 0},
		{"nothing of it under the correctness profile", "on: issues\n", "github.event.changes.title.from", "correctness", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintWithConfig(t, cfg(tc.profile), run(tc.on, tc.expr))
			var got []string
			for _, e := range errs {
				if e.ID == "template-injection" {
					got = append(got, e.Message)
				}
			}
			if len(got) != tc.want {
				t.Errorf("want %d findings, got %q", tc.want, got)
			}
			if tc.profile != "pedantic" && tc.want == 1 && !strings.Contains(got[0], "can hold shell syntax") {
				t.Errorf("not the message of an outsider's text: %q", got[0])
			}
		})
	}

	t.Run("sinks", func(t *testing.T) {
		src := "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    container:\n      image: node:20\n      options: --label ${{ github.event.changes.title.from }}\n    steps:\n      - run: echo\n"
		errs := lintWithConfig(t, cfg("default"), src)
		var got []string
		for _, e := range errs {
			if e.ID == "template-injection" {
				got = append(got, e.Message)
			}
		}
		if len(got) != 1 || !strings.Contains(got[0], "the container of the job") {
			t.Errorf("want one finding in the options of the container, got %q", got)
		}
	})
}
