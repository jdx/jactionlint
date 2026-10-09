package jactionlint

import (
	"slices"
)

// RuleDuplicateTriggers reports workflows that run twice for a commit pushed to a branch with an
// open pull request: they are triggered by `push` and by `pull_request` and `push` has no branch
// filter.
type RuleDuplicateTriggers struct {
	RuleBase
}

// NewRuleDuplicateTriggers creates a new RuleDuplicateTriggers instance.
func NewRuleDuplicateTriggers() *RuleDuplicateTriggers {
	return &RuleDuplicateTriggers{
		RuleBase: RuleBase{
			name: "duplicate-triggers",
			desc: "Checks that push and pull_request do not both run the workflow for the same commit",
		},
	}
}

// pullRequestActivityTypes are the types of pull_request that GitHub runs a workflow for when
// `types` is not set. A run for another type is for something else than a new commit.
var pullRequestActivityTypes = []string{"opened", "synchronize", "reopened"}

// pushRunsForBranches reports whether the push event starts the workflow for any branch.
func pushRunsForBranches(e *WebhookEvent) bool {
	if !e.Branches.IsEmpty() {
		// Only a filter that matches every branch keeps it a duplicate
		for _, b := range e.Branches.Values {
			if b.Value == "**" {
				return true
			}
		}
		return false
	}
	if !e.Tags.IsEmpty() {
		return false // only tags are given, so no branch starts the workflow
	}
	// branches-ignore and paths filters still let the commits of a pull request branch through
	return true
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleDuplicateTriggers) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("duplicate-triggers") {
		return nil
	}
	pushes, prs := webhookEvents(n, "push"), webhookEvents(n, "pull_request")
	if len(pushes) == 0 || len(prs) == 0 {
		return nil
	}
	push := pushes[0]
	if !pushRunsForBranches(push) {
		return nil
	}
	runsForCommits := false
	for _, pr := range prs {
		if len(pr.Types) == 0 {
			runsForCommits = true
		}
		for _, t := range pr.Types {
			if slices.Contains(pullRequestActivityTypes, t.Value) {
				runsForCommits = true
			}
		}
	}
	if !runsForCommits || rule.deduplicated(n) {
		return nil
	}
	rule.ReportID(
		"duplicate-triggers",
		push.Pos,
		"\"push\" has no branch filter and \"pull_request\" is used too, so a commit pushed to a branch of this repository that has a pull request runs the workflow twice. limit \"push\" to the branches that need it, for example the default branch",
	)
	return nil
}

// deduplicated reports whether the workflow already avoids the second run: every job has a
// condition on the event or on the repository of the pull request, or a concurrency group that
// is the same for both runs and cancels one.
func (rule *RuleDuplicateTriggers) deduplicated(n *Workflow) bool {
	if len(n.Jobs) == 0 {
		return false
	}
	if c := n.Concurrency; c != nil && c.Group != nil && c.CancelInProgress != nil {
		refs, _ := stringExprRefs(c.Group.Value)
		if refsRead(refs, []string{"github", "head_ref"}) && refsRead(refs, []string{"github", "ref_name"}) && (scenario{}).isTrue(c.CancelInProgress) {
			return true
		}
	}
	for _, j := range n.Jobs {
		if j.If == nil {
			return false
		}
		exprs, ok := conditionExprs(j.If)
		if !ok {
			return true // cannot tell
		}
		var refs []exprRef
		for _, e := range exprs {
			collectExprRefs(e, nil, &refs)
		}
		if !refsRead(refs, []string{"github", "event_name"}) && !refsRead(refs, []string{"github", "event", "pull_request", "head", "repo"}) {
			return false
		}
	}
	return true
}

func init() {
	registerRules(
		RuleInfo{ID: "duplicate-triggers", Group: RuleGroupPolicy, Summary: "push and pull_request both run the workflow for the same commit.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-duplicate-triggers"},
	)
	registerRuleFactory("duplicate-triggers", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleDuplicateTriggers()}
	})
}
