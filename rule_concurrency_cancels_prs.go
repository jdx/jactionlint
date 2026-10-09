package jactionlint

import (
	"slices"
	"strings"
)

// RuleConcurrencyCancelsPRs reports concurrency groups that are shared by all pull requests while
// cancel-in-progress can cancel the running workflow: a new run for one pull request then cancels the
// run of an unrelated pull request.
type RuleConcurrencyCancelsPRs struct {
	RuleBase
	prEvents []string
}

// NewRuleConcurrencyCancelsPRs creates a new RuleConcurrencyCancelsPRs instance.
func NewRuleConcurrencyCancelsPRs() *RuleConcurrencyCancelsPRs {
	return &RuleConcurrencyCancelsPRs{
		RuleBase: RuleBase{
			name: "concurrency-cancels-prs",
			desc: "Checks that a concurrency group which cancels runs has a value that differs between pull requests",
		},
	}
}

// pullRequestEvents are the events that run for a pull request.
var pullRequestEvents = []string{"pull_request", "pull_request_target", "pull_request_review", "pull_request_review_comment"}

// perPullRequestContexts returns the contexts that differ between pull requests (or between runs)
// for the event. They are prefixes of context paths.
func perPullRequestContexts(event string) [][]string {
	common := [][]string{
		{"github", "run_id"},
		{"github", "run_number"},
		{"github", "event", "pull_request", "number"},
		{"github", "event", "pull_request", "id"},
		{"github", "event", "pull_request", "node_id"},
		{"github", "event", "pull_request", "url"},
		{"github", "event", "pull_request", "html_url"},
		{"github", "event", "pull_request", "head", "sha"},
		{"github", "event", "pull_request", "head", "ref"},
		{"github", "event", "pull_request", "head", "label"},
	}
	// The payloads of pull_request and pull_request_target have a number of their own. The ones of the review events
	// have only pull_request.number.
	number := []string{"github", "event", "number"}
	switch event {
	case "pull_request_target":
		// github.ref and github.sha are the ones of the base branch for this event
		return append(common, number, []string{"github", "head_ref"})
	case "pull_request":
		return append(common, number,
			[]string{"github", "head_ref"}, []string{"github", "ref"}, []string{"github", "ref_name"},
			[]string{"github", "sha"}, []string{"github", "workflow_ref"},
			[]string{"github", "event", "pull_request", "merge_commit_sha"})
	}
	// pull_request_review and pull_request_review_comment run on refs/pull/<number>/merge but
	// github.head_ref is empty
	return append(common,
		[]string{"github", "ref"}, []string{"github", "ref_name"}, []string{"github", "sha"}, []string{"github", "workflow_ref"})
}

// unjudgeableContexts are the contexts whose value the rule cannot know. A group built from them is
// not reported.
var unjudgeableContexts = []string{"env", "vars", "inputs", "needs", "steps", "secrets"}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleConcurrencyCancelsPRs) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("concurrency-cancels-prs") {
		return nil
	}
	rule.prEvents = rule.prEvents[:0]
	for _, e := range n.On {
		if slices.Contains(pullRequestEvents, e.EventName()) {
			rule.prEvents = append(rule.prEvents, e.EventName())
		}
	}
	if len(rule.prEvents) == 0 {
		return nil
	}
	rule.check(n.Concurrency)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleConcurrencyCancelsPRs) VisitJobPre(n *Job) error {
	if len(rule.prEvents) > 0 {
		rule.check(n.Concurrency)
	}
	return nil
}

func (rule *RuleConcurrencyCancelsPRs) check(c *Concurrency) {
	if c == nil || c.Group == nil || c.CancelInProgress == nil {
		return
	}
	refs, ok := stringExprRefs(c.Group.Value)
	if !ok {
		return
	}
	for _, r := range refs {
		if slices.Contains(unjudgeableContexts, r.chain[0]) {
			return
		}
	}
	for _, event := range rule.prEvents {
		sc := scenario{event: event}
		if event != "pull_request_target" {
			sc.refPrefix = "refs/pull/"
		}
		if !sc.isTrue(c.CancelInProgress) {
			continue
		}
		if groupDiffersPerPullRequest(refs, event) {
			continue
		}
		hint := ""
		if event == "pull_request_target" {
			for _, r := range refs {
				if refCovers(r.chain, []string{"github", "ref"}) || refCovers(r.chain, []string{"github", "ref_name"}) || refCovers(r.chain, []string{"github", "sha"}) {
					hint = ". note that github.ref is the base branch for \"pull_request_target\", so it is the same for every pull request"
					break
				}
			}
		}
		rule.ReportIDf(
			"concurrency-cancels-prs",
			c.Group.Pos,
			"concurrency group %q is the same for every pull request (event %q) and \"cancel-in-progress\" is enabled for them, so a new run for one pull request cancels the run of an unrelated one. add \"github.head_ref\" or \"github.event.pull_request.number\" to the group%s",
			strings.TrimSpace(c.Group.Value), event, hint,
		)
		return
	}
}

func groupDiffersPerPullRequest(refs []exprRef, event string) bool {
	for _, r := range refs {
		for _, d := range perPullRequestContexts(event) {
			// The reference must read the discriminator itself: `github` alone is every context and
			// `github.event.pull_request` is its number, but `github.event_name` is neither.
			if refCovers(r.chain, d) {
				return true
			}
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "concurrency-cancels-prs", Group: RuleGroupCorrectness, Summary: "A concurrency group that cancels runs is shared by all pull requests, so unrelated pull requests cancel each other.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-concurrency-cancels-prs"},
	)
	registerRuleFactory("concurrency-cancels-prs", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleConcurrencyCancelsPRs()}
	})
}
