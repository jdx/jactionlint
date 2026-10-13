package jactionlint

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// eventPayloadFindings lints a workflow with the given "on:" and one step which reads the expression. It returns the
// messages of the findings about a property of github.event.
func eventPayloadFindings(t *testing.T, on, expr, cfg string) []string {
	t.Helper()
	src := "on: " + on + "\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"${{ " + expr + " }}\"\n"
	var c *Config
	if cfg != "" {
		c = mustParseConfig(t, cfg)
	}
	var ret []string
	for _, e := range lintWithConfig(t, c, src) {
		if e.ID == "undefined-property" {
			ret = append(ret, e.Message)
		}
	}
	return ret
}

const strictEventPayloadsConfig = "rules:\n  undefined-property:\n    strict-event-payloads: true\n"

func TestEventPayloadTypos(t *testing.T) {
	// A property which no webhook event has is reported in every mode, where it is.
	tests := []struct {
		on, expr, want string
	}{
		{"workflow_run", "github.event.worflow_run.head_commit.message", `property "worflow_run" is not defined in github.event: no webhook event has it in its payload. did you mean "workflow_run"?`},
		{"pull_request", "github.event.pull_request.titel", `property "titel" is not defined in github.event.pull_request: no webhook event has it in its payload. did you mean "title"?`},
		{"pull_request", "github.event.pull_request.head.rfe", `property "rfe" is not defined in github.event.pull_request.head: `},
		{"push", "github.event.head_commit.mesage", `property "mesage" is not defined in github.event.head_commit: `},
		{"push", "github.event.commits[0].mesage", `property "mesage" is not defined in github.event.commits[]: `},
		{"push", "github.event.commits.*.mesage", `property "mesage" is not defined in github.event.commits.*: `},
		{"push", "join(github.event.commits.*.author.nmae)", `property "nmae" is not defined in github.event.commits.*.author: `},
		{"push", "github.event.push.ref", `property "push" is not defined in github.event: `},
		{"push", "github['event']['pull_reqeust']", `property "pull_reqeust" is not defined in github.event: `},
		{"issues", "github.event.issue['titel']", `property "titel" is not defined in github.event.issue: `},
		{"[push, pull_request]", "github.event.repository.defalut_branch", `property "defalut_branch" is not defined in github.event.repository: `},
		{"release", "github.event.release.assets[0].browser_download_ulr", `property "browser_download_ulr" is not defined in github.event.release.assets[]: `},
	}
	for _, tc := range tests {
		for _, cfg := range []string{"", strictEventPayloadsConfig} {
			t.Run(fmt.Sprintf("%s/%s/%v", tc.on, tc.expr, cfg != ""), func(t *testing.T) {
				got := eventPayloadFindings(t, tc.on, tc.expr, cfg)
				if len(got) != 1 {
					t.Fatalf("want one finding but got %q", got)
				}
				// In the strict mode the message says which events the workflow has instead
				if cfg == "" && !strings.Contains(got[0], tc.want) {
					t.Errorf("want %q in %q", tc.want, got[0])
				}
				if cfg != "" && !strings.Contains(got[0], strings.SplitN(tc.want, ":", 2)[0]) {
					t.Errorf("want %q in %q", tc.want, got[0])
				}
			})
		}
	}
}

func TestEventPayloadStrictReportsPropertiesOfOtherEvents(t *testing.T) {
	// A property of the payload of another event is a read which evaluates to null: it is reported only in the strict
	// mode, so that a defensive read (`github.event.pull_request.number || github.ref`) is not.
	tests := []struct{ on, expr, want string }{
		{"push", "github.event.pull_request.number", `property "pull_request" is not defined in github.event, the payload of the event "push"`},
		{"push", "github.event.action", `property "action" is not defined in github.event, the payload of the event "push"`},
		{"issues", "github.event.number", `property "number" is not defined in github.event, the payload of the event "issues"`},
		{"pull_request", "github.event.issue.title", `property "issue" is not defined in github.event, the payload of the event "pull_request"`},
		{"[issue_comment, issues]", "github.event.pull_request.number", `the payload of the events "issue_comment", "issues"`},
		{"issue_comment", "github.event.issue.pull_request.head.ref", `property "head" is not defined in github.event.issue.pull_request, the payload of the event "issue_comment"`},
		{"pull_request_target", "github.event.inputs.x", `property "inputs" is not defined in github.event, the payload of the event "pull_request_target"`},
	}
	for _, tc := range tests {
		t.Run(tc.on+"/"+tc.expr, func(t *testing.T) {
			if got := eventPayloadFindings(t, tc.on, tc.expr, ""); len(got) != 0 {
				// issue.pull_request.head is in no payload, so it is a typo of every mode
				if !strings.Contains(tc.expr, "issue.pull_request.head") {
					t.Errorf("the default mode reports %q", got)
				}
			}
			got := eventPayloadFindings(t, tc.on, tc.expr, strictEventPayloadsConfig)
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Errorf("want one finding with %q but got %q", tc.want, got)
			}
		})
	}
}

func TestEventPayloadPedanticProfileIsStrict(t *testing.T) {
	cfg := "profile: pedantic\n"
	if got := eventPayloadFindings(t, "push", "github.event.pull_request.number", cfg); len(got) != 1 {
		t.Errorf("the pedantic profile reports %q", got)
	}
	off := cfg + "rules:\n  undefined-property:\n    strict-event-payloads: false\n"
	if got := eventPayloadFindings(t, "push", "github.event.pull_request.number", off); len(got) != 0 {
		t.Errorf("the option switches it off: %q", got)
	}
	if got := eventPayloadFindings(t, "push", "github.event.pull_reqest.number", off); len(got) != 1 {
		t.Errorf("a typo is reported with the option off: %q", got)
	}
	// The rule turned off reports nothing
	offRule := "rules:\n  undefined-property: off\n"
	if got := eventPayloadFindings(t, "push", "github.event.pull_reqest.number", offRule); len(got) != 0 {
		t.Errorf("a rule which is off reports %q", got)
	}
}

// The properties of the payload which belong to dispatches and schedules are in no webhook, and a read of them in a
// workflow of other events is reported in the strict mode only.
func TestEventPayloadNotReportedByDefault(t *testing.T) {
	for _, expr := range []string{"github.event.inputs.x", "github.event.client_payload.x.y", "github.event.schedule"} {
		if got := eventPayloadFindings(t, "pull_request", expr, ""); len(got) != 0 {
			t.Errorf("%s: %q", expr, got)
		}
		if got := eventPayloadFindings(t, "pull_request", expr, strictEventPayloadsConfig); len(got) != 1 {
			t.Errorf("%s in the strict mode: %q", expr, got)
		}
	}
}

func TestEventPayloadNotReported(t *testing.T) {
	tests := []struct{ on, expr string }{
		// The properties of the payloads
		{"pull_request", "github.event.pull_request.title"},
		{"pull_request", "github.event.pull_request.head.ref"},
		{"pull_request", "github.event.pull_request.head.repo.fork"},
		{"pull_request", "github.event.pull_request.labels.*.name"},
		{"pull_request", "contains(github.event.pull_request.labels.*.name, 'x')"},
		{"pull_request", "github.event.pull_request.requested_reviewers[0].login"},
		{"pull_request", "github.event.number"},
		{"pull_request_target", "github.event.pull_request.merged"},
		{"push", "github.event.head_commit.message"},
		{"push", "github.event.commits[0].author.name"},
		{"push", "join(github.event.commits.*.id)"},
		{"push", "github.event.repository.default_branch"},
		{"workflow_run", "github.event.workflow_run.head_branch"},
		{"workflow_run", "github.event.workflow_run.pull_requests[0].number"},
		{"issue_comment", "github.event.issue.pull_request.url"},
		{"issue_comment", "github.event.comment.body"},
		{"issues", "github.event.issue.type"},
		{"release", "github.event.release.assets[0].name"},
		{"release", "github.event.release.tag_name"},
		{"[push, pull_request]", "github.event.pull_request.title"},
		// Names are not case sensitive
		{"pull_request", "github.event.Pull_Request.TITLE"},
		{"pull_request", "github['event']['PULL_REQUEST']['Title']"},
		// What is arbitrary or an account is not looked into
		{"push", "github.event.sender.anything"},
		{"push", "github.event.pusher.anything"},
		{"pull_request", "github.event.pull_request.user.anything"},
		{"pull_request", "github.event.organization.anything"},
		{"pull_request", "github.event.pull_request.head.repo.anything"},
		{"pull_request", "github.event.changes.title.from"},
		// The payload is not known
		{"workflow_dispatch", "github.event.worflow_run"},
		{"[push, workflow_dispatch]", "github.event.worflow_run"},
		{"repository_dispatch", "github.event.worflow_run"},
		{"schedule:\n    - cron: '0 0 * * *'", "github.event.worflow_run"},
		{"{push: {}, schedule: [{cron: '0 0 * * *'}]}", "github.event.worflow_run"},
		{"workflow_call", "github.event.worflow_run"},
		{"[push, workflow_call]", "github.event.worflow_run"},
		{"project_card", "github.event.worflow_run"},
		{"[push, project_card]", "github.event.worflow_run"},
	}
	for _, tc := range tests {
		for _, cfg := range []string{"", strictEventPayloadsConfig} {
			t.Run(fmt.Sprintf("%s/%s/%v", tc.on, tc.expr, cfg != ""), func(t *testing.T) {
				if got := eventPayloadFindings(t, tc.on, tc.expr, cfg); len(got) != 0 {
					t.Errorf("want no finding but got %q", got)
				}
			})
		}
	}
}

// The type of github.event stays an untyped object: the payload only decides about the names of properties.
func TestEventPayloadKeepsTypesOfExpressions(t *testing.T) {
	for _, expr := range []string{
		"github.event.pull_request",
		"github.event.pull_request.labels",
		"github.event.commits",
		"github.event.pull_request.number == 'x'",
		"github.event.pull_request == ''",
		"github.event.issue.pull_request != null",
		"startsWith(github.event.pull_request.title, 'x')",
	} {
		src := "on: [pull_request, push, issues]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"${{ " + expr + " }}\"\n"
		for _, e := range lintWithConfig(t, mustParseConfig(t, strictEventPayloadsConfig), src) {
			if e.ID == "expression-type" {
				t.Errorf("%s: %s", expr, e.Message)
			}
		}
	}
}

func TestEventPayloadWhereTheExpressionIs(t *testing.T) {
	src := `on: push
run-name: ${{ github.event.pushh.ref }}
concurrency:
  group: ${{ github.event.pul.number }}
env:
  A: ${{ github.event.pull_reqest.number }}
jobs:
  j:
    if: github.event.issue_comment
    runs-on: ubuntu-latest
    env:
      B: ${{ github.event.headcommit.id }}
    steps:
      - name: ${{ github.event.sendr.login }}
        if: ${{ github.event.bse_ref }}
        with-nothing: x
        run: echo ok
`
	got := 0
	for _, e := range lintWithConfig(t, nil, src) {
		if e.ID == "undefined-property" {
			got++
		}
	}
	if got != 7 {
		t.Errorf("want 7 findings but got %d", got)
	}
}

func TestEventPayloadComposite(t *testing.T) {
	// A composite action is called by any workflow
	src := "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - run: echo ${{ github.event.worflow_run.head_commit.message }}\n      shell: bash\n"
	for _, e := range lintActionSource(t, src) {
		if e.ID == "undefined-property" {
			t.Errorf("a composite action is loose: %s", e.Message)
		}
	}
}

func TestPayloadOfWorkflow(t *testing.T) {
	tests := []struct {
		on     string
		strict bool
		want   []string // the events, nil for no payload
	}{
		{"push", true, []string{"push"}},
		{"[push, pull_request]", true, []string{"push", "pull_request"}},
		{"{push: {branches: [main]}, pull_request: {}}", true, []string{"push", "pull_request"}},
		{"[push, pull_request, push]", true, []string{"push", "pull_request"}},
		{"push", false, nil}, // the universe has no events
		{"workflow_dispatch", true, nil},
		{"[push, workflow_dispatch]", true, nil},
		{"{push: {}, schedule: [{cron: '0 0 * * *'}]}", true, nil},
		{"[push, workflow_call]", true, nil},
		{"repository_dispatch", true, nil},
		{"[push, repository_dispatch]", true, nil},
		{"[push, image_version]", true, nil},
		{"[push, project]", true, nil},
		{"[push, no_such_event]", true, nil},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%v", tc.on, tc.strict), func(t *testing.T) {
			w, errs := Parse([]byte("on: " + tc.on + "\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"))
			if w == nil {
				t.Fatalf("parse error: %v", errs)
			}
			p := payloadOfWorkflow(w, tc.strict)
			if tc.want == nil {
				if tc.strict && p != nil {
					t.Fatalf("want no payload but got the one of %v", p.events)
				}
				if !tc.strict && (p == nil || p.events != nil) {
					t.Fatalf("want the payload of all the events but got %v", p)
				}
				return
			}
			if p == nil || strings.Join(p.events, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("want the events %v but got %v", tc.want, p)
			}
		})
	}
}

func TestPayloadShapes(t *testing.T) {
	shapes := loadPayloadShapes()
	for _, e := range []string{"push", "pull_request", "pull_request_target", "issue_comment", "workflow_run", "release", "*"} {
		if s := shapes[e]; s == nil || s.props == nil {
			t.Errorf("%s has no payload", e)
		}
	}
	if shapes["pull_request"] != shapes["pull_request_target"] {
		t.Error("pull_request_target has the payload of pull_request")
	}
	// What is not an event of a workflow is not in it
	for _, e := range []string{"workflow_dispatch", "workflow_call", "schedule", "repository_dispatch"} {
		if _, ok := shapes[e]; ok {
			t.Errorf("%s has a payload", e)
		}
	}
	// The universe has the properties of all the payloads, and the properties of the events which have no webhook
	u := shapes[payloadUniverse]
	for _, p := range []string{"action", "pull_request", "issue", "head_commit", "workflow_run", "inputs", "client_payload", "schedule", "zen"} {
		if _, ok := u.props[p]; !ok {
			t.Errorf("the universe lacks %s", p)
		}
	}
	// A property which an object has in one event is known when the workflow has more events
	union := unionPayloadShapes(shapes["push"], shapes["issues"])
	for _, p := range []string{"head_commit", "issue", "repository"} {
		if _, ok := union.props[p]; !ok {
			t.Errorf("the union lacks %s", p)
		}
	}
	if union == shapes["push"] || union == shapes["issues"] {
		t.Error("the union must not change the payloads")
	}
	if _, ok := shapes["push"].props["issue"]; ok {
		t.Error("the union changed the payload of push")
	}
	// Anything unknown is not known in the union
	if unionPayloadShapes(shapes["push"], nil) != nil || unionPayloadShapes(nil, shapes["push"]) != nil {
		t.Error("a union with something unknown is unknown")
	}
}

// Every object of the generated file is parsed and used, and every alias has its event.
func TestPayloadShapesOfTheGeneratedFile(t *testing.T) {
	l := &payloadLoader{objects: map[string]*payloadShape{}}
	for name := range eventPayloadObjects {
		if s := l.object(name); s == nil || s.props == nil {
			t.Errorf("%s is not an object", name)
		}
	}
	used := map[string]bool{}
	for _, text := range eventPayloads {
		for _, part := range strings.Split(text, "@")[1:] {
			used[strings.FieldsFunc(part, func(r rune) bool { return strings.ContainsRune("{}[],: \t\n", r) })[0]] = true
		}
	}
	for _, text := range eventPayloadObjects {
		for _, part := range strings.Split(text, "@")[1:] {
			used[strings.FieldsFunc(part, func(r rune) bool { return strings.ContainsRune("{}[],: \t\n", r) })[0]] = true
		}
	}
	for name := range eventPayloadObjects {
		if !used[name] {
			t.Errorf("%s is not used", name)
		}
	}
	for name, text := range eventPayloads {
		if alias, ok := strings.CutPrefix(text, "="); ok {
			if _, ok := eventPayloads[alias]; !ok || strings.HasPrefix(eventPayloads[alias], "=") {
				t.Errorf("%s is an alias of %s, which is not an event with a payload", name, alias)
			}
		}
	}
}

func TestPayloadParser(t *testing.T) {
	objects := map[string]*payloadShape{"o": {props: map[string]*payloadShape{"x": nil}}}
	p := &payloadParser{src: " {a, b : [ @o ], c:[], d:{e,\n f:{g}}, \n} ", object: func(n string) *payloadShape { return objects[n] }}
	s := p.shape()
	if len(s.props) != 4 {
		t.Fatalf("want four properties but got %d", len(s.props))
	}
	if s.props["a"] != nil {
		t.Error("a is a value")
	}
	if b := s.props["b"]; b == nil || !b.array || b.elem != objects["o"] {
		t.Errorf("b is an array of o: %+v", b)
	}
	if c := s.props["c"]; c == nil || !c.array || c.elem != nil {
		t.Errorf("c is an array of anything: %+v", c)
	}
	if d := s.props["d"]; d == nil || d.props["f"] == nil || d.props["f"].props["g"] != nil {
		t.Errorf("d is nested: %+v", d)
	}
}

// The reads of github.event of the lists of the checks and the documentation must be in the payloads, in the events
// where they are documented.
func TestEventPayloadKnowsDocumentedProperties(t *testing.T) {
	tests := map[string][]string{
		"pull_request": {
			"github.event.action", "github.event.number", "github.event.pull_request.number", "github.event.pull_request.title",
			"github.event.pull_request.body", "github.event.pull_request.draft", "github.event.pull_request.merged",
			"github.event.pull_request.merge_commit_sha", "github.event.pull_request.head.sha", "github.event.pull_request.head.ref",
			"github.event.pull_request.head.label", "github.event.pull_request.base.ref", "github.event.pull_request.base.sha",
			"github.event.pull_request.labels", "github.event.pull_request.html_url", "github.event.pull_request.user.login",
			"github.event.pull_request.auto_merge", "github.event.pull_request.mergeable", "github.event.pull_request.state",
			"github.event.pull_request.created_at", "github.event.pull_request.updated_at", "github.event.pull_request.milestone.title",
			"github.event.pull_request.assignees", "github.event.pull_request.requested_reviewers", "github.event.pull_request.requested_teams",
			"github.event.label.name", "github.event.before", "github.event.after", "github.event.changes.base.ref.from",
			"github.event.repository.full_name", "github.event.repository.owner.login", "github.event.repository.private",
			"github.event.repository.html_url", "github.event.repository.clone_url", "github.event.repository.fork",
			"github.event.sender.login", "github.event.sender.type", "github.event.organization.login", "github.event.installation.id",
		},
		"push": {
			"github.event.ref", "github.event.before", "github.event.after", "github.event.created", "github.event.deleted",
			"github.event.forced", "github.event.compare", "github.event.base_ref", "github.event.commits",
			"github.event.head_commit.id", "github.event.head_commit.message", "github.event.head_commit.timestamp",
			"github.event.head_commit.author.name", "github.event.head_commit.author.email", "github.event.head_commit.author.username",
			"github.event.head_commit.committer.name", "github.event.head_commit.url", "github.event.head_commit.added",
			"github.event.head_commit.modified", "github.event.head_commit.removed", "github.event.pusher.name",
			"github.event.commits[0].message", "github.event.commits[0].id", "github.event.commits[0].url",
			"github.event.commits[0].distinct", "github.event.commits[0].timestamp", "github.event.commits[0].tree_id",
		},
		"issues": {
			"github.event.issue.number", "github.event.issue.title", "github.event.issue.body", "github.event.issue.state",
			"github.event.issue.user.login", "github.event.issue.labels", "github.event.issue.assignees", "github.event.issue.assignee",
			"github.event.issue.milestone", "github.event.issue.comments", "github.event.issue.html_url", "github.event.issue.node_id",
			"github.event.issue.author_association", "github.event.issue.state_reason", "github.event.issue.locked",
			"github.event.issue.created_at", "github.event.issue.closed_at", "github.event.issue.type", "github.event.label.name",
			"github.event.assignee.login", "github.event.changes.title.from",
		},
		"issue_comment": {
			"github.event.issue.pull_request.url", "github.event.issue.pull_request.html_url", "github.event.comment.id",
			"github.event.comment.body", "github.event.comment.user.login", "github.event.comment.html_url",
			"github.event.comment.author_association", "github.event.comment.created_at", "github.event.comment.issue_url",
			"github.event.comment.node_id", "github.event.comment.reactions", "github.event.changes.body.from",
		},
		"workflow_run": {
			"github.event.workflow_run.id", "github.event.workflow_run.name", "github.event.workflow_run.conclusion",
			"github.event.workflow_run.status", "github.event.workflow_run.event", "github.event.workflow_run.head_branch",
			"github.event.workflow_run.head_sha", "github.event.workflow_run.head_commit.message", "github.event.workflow_run.html_url",
			"github.event.workflow_run.pull_requests", "github.event.workflow_run.run_number", "github.event.workflow_run.run_attempt",
			"github.event.workflow_run.workflow_id", "github.event.workflow_run.jobs_url", "github.event.workflow_run.logs_url",
			"github.event.workflow_run.artifacts_url", "github.event.workflow_run.actor.login", "github.event.workflow_run.triggering_actor.login",
			"github.event.workflow_run.head_repository.full_name", "github.event.workflow_run.head_repository.fork",
			"github.event.workflow_run.repository.full_name", "github.event.workflow_run.path", "github.event.workflow_run.display_title",
			"github.event.workflow_run.created_at", "github.event.workflow_run.updated_at", "github.event.workflow_run.run_started_at",
			"github.event.workflow_run.referenced_workflows", "github.event.workflow.name", "github.event.workflow.path",
		},
		"release": {
			"github.event.release.tag_name", "github.event.release.name", "github.event.release.body", "github.event.release.draft",
			"github.event.release.prerelease", "github.event.release.target_commitish", "github.event.release.html_url",
			"github.event.release.assets", "github.event.release.created_at", "github.event.release.published_at",
			"github.event.release.author.login", "github.event.release.upload_url", "github.event.release.zipball_url",
			"github.event.release.tarball_url", "github.event.release.id", "github.event.release.node_id",
		},
		"discussion":                  {"github.event.discussion.title", "github.event.discussion.category.name", "github.event.discussion.number", "github.event.discussion.body", "github.event.discussion.html_url"},
		"pull_request_review":         {"github.event.review.state", "github.event.review.body", "github.event.review.user.login", "github.event.review.commit_id", "github.event.pull_request.number"},
		"pull_request_review_comment": {"github.event.comment.body", "github.event.comment.path", "github.event.comment.diff_hunk", "github.event.comment.line", "github.event.pull_request.number"},
		"check_run":                   {"github.event.check_run.name", "github.event.check_run.conclusion", "github.event.check_run.head_sha", "github.event.check_run.pull_requests", "github.event.check_run.check_suite.id", "github.event.check_run.app.slug"},
		"check_suite":                 {"github.event.check_suite.head_branch", "github.event.check_suite.head_sha", "github.event.check_suite.conclusion", "github.event.check_suite.pull_requests", "github.event.check_suite.app.slug"},
		"deployment_status":           {"github.event.deployment_status.state", "github.event.deployment_status.target_url", "github.event.deployment_status.environment_url", "github.event.deployment.environment", "github.event.deployment.ref", "github.event.deployment.sha"},
		"create":                      {"github.event.ref", "github.event.ref_type", "github.event.master_branch", "github.event.description", "github.event.pusher_type"},
		"delete":                      {"github.event.ref", "github.event.ref_type", "github.event.pusher_type"},
		"merge_group":                 {"github.event.merge_group.head_sha", "github.event.merge_group.head_ref", "github.event.merge_group.base_ref", "github.event.merge_group.base_sha", "github.event.merge_group.head_commit.message"},
		"label":                       {"github.event.label.name", "github.event.label.color", "github.event.changes.name.from"},
		"milestone":                   {"github.event.milestone.title", "github.event.milestone.number", "github.event.milestone.state"},
		"status":                      {"github.event.sha", "github.event.state", "github.event.context", "github.event.description", "github.event.target_url", "github.event.branches", "github.event.commit.sha", "github.event.name"},
		"watch":                       {"github.event.action"},
		"gollum":                      {"github.event.pages[0].page_name", "github.event.pages[0].action", "github.event.pages[0].title", "github.event.pages[0].html_url", "github.event.pages[0].sha"},
		"registry_package":            {"github.event.registry_package.name", "github.event.registry_package.package_version.version", "github.event.registry_package.package_type"},
		"fork":                        {"github.event.forkee.full_name", "github.event.forkee.html_url"},
		"public":                      {"github.event.repository.name"},
		"page_build":                  {"github.event.build.status", "github.event.id"},
		"branch_protection_rule":      {"github.event.rule.name", "github.event.rule.id"},
	}
	for event, exprs := range tests {
		for _, expr := range exprs {
			t.Run(event+"/"+expr, func(t *testing.T) {
				if got := eventPayloadFindings(t, event, expr, ""); len(got) != 0 {
					t.Errorf("want no finding in the default mode but got %q", got)
				}
				if got := eventPayloadFindings(t, event, expr, strictEventPayloadsConfig); len(got) != 0 {
					t.Errorf("want no finding in the strict mode but got %q", got)
				}
			})
		}
	}
}

// Linting in parallel reads the shared shapes.
func TestEventPayloadConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src := "on: [push, pull_request]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ github.event.commits.*.mesage }} ${{ github.event.pull_request.head.ref }}\n"
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			errs, err := l.Lint("test.yaml", []byte(src), nil)
			if err != nil {
				t.Error(err)
				return
			}
			n := 0
			for _, e := range errs {
				if e.ID == "undefined-property" {
					n++
				}
			}
			if n != 1 {
				t.Errorf("want one finding but got %d", n)
			}
		}()
	}
	wg.Wait()
}
