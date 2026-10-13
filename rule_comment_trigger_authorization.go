package jactionlint

// commentTriggers are the events whose payload is a comment that anyone who can comment writes.
var commentTriggers = []string{"issue_comment", "pull_request_review_comment", "discussion_comment"}

// RuleCommentTriggerAuthorization finds the jobs of the comment-triggered workflows that are started by the text of
// the comment and check nothing about who wrote it (zizmor#459).
//
// A workflow with the trigger `issue_comment` runs on the comment of anyone who can comment, on a public repository
// that is every user of GitHub, and it runs in the base repository, with the default token and the secrets. The
// usual shape is a command bot:
//
//	if: github.event.issue.pull_request && contains(github.event.comment.body, '/deploy')
//
// which looks like a restriction but only reads what the commenter chose to write. It lets anyone start the job. The
// job is reported when its condition (or the condition of one of its steps) reads the comment body, and nothing
// checks the commenter: no condition on the association (`github.event.comment.author_association`), the actor
// or the user, no step before it that asks for the permission of the actor, no `environment:` and no such check in a
// job that it needs. That is the same check of the user that agentic-actions makes (jobChecksWhoStarted). It is a
// heuristic: it cannot tell a condition that is right from one that only mentions the actor.
type RuleCommentTriggerAuthorization struct {
	RuleBase
	wf      *Workflow
	trigger string
}

// NewRuleCommentTriggerAuthorization creates a new RuleCommentTriggerAuthorization instance.
func NewRuleCommentTriggerAuthorization() *RuleCommentTriggerAuthorization {
	return &RuleCommentTriggerAuthorization{
		RuleBase: NewRuleBase("comment-trigger-authorization", "Checks for jobs started by the text of a comment that do not check who wrote it"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleCommentTriggerAuthorization) VisitWorkflowPre(n *Workflow) error {
	rule.wf, rule.trigger = n, ""
	for _, e := range n.TriggerEvents() {
		for _, t := range commentTriggers {
			if e.EventName() == t && rule.trigger == "" {
				rule.trigger = t
			}
		}
	}
	return nil
}

// readsCommentBody reports whether the condition reads the text of the comment.
func readsCommentBody(cond *String) bool {
	exprs, ok := conditionExprs(cond)
	if !ok {
		return false
	}
	var refs []exprRef
	for _, e := range exprs {
		collectExprRefs(e, nil, &refs)
	}
	for _, r := range refs {
		if len(r.chain) >= 4 && refCovers(r.chain, []string{"github", "event", "comment", "body"}) {
			return true
		}
	}
	return false
}

// readsEarlierDecision reports whether the condition reads the outputs of an earlier job (`needs`) or step
// (`steps`): something that came before has decided, which is how a job that checks the commenter in a separate job
// or step passes the verdict on. The check itself is not judged. A condition on the labels of the issue or the pull
// request counts too: only people with triage access can set them, so the comment alone does not start the job.
func readsEarlierDecision(cond *String) bool {
	exprs, ok := conditionExprs(cond)
	if !ok {
		return true
	}
	var refs []exprRef
	for _, e := range exprs {
		collectExprRefs(e, nil, &refs)
	}
	for _, r := range refs {
		for _, t := range [][]string{{"needs"}, {"steps"}, {"github", "event", "issue", "labels"}, {"github", "event", "pull_request", "labels"}, {"github", "event", "label"}} {
			if refCovers(r.chain, t) {
				return true
			}
		}
	}
	return false
}

// runsGatedAgent reports whether a step runs an AI agent action that checks that the user has write access before
// it does anything (and is not told to let everybody in).
func runsGatedAgent(steps []*Step) bool {
	for _, s := range steps {
		a, ok := s.Exec.(*ExecAction)
		if !ok {
			continue
		}
		agent := agentOf(a)
		if agent == nil || !agent.Gated {
			continue
		}
		open := false
		for _, g := range agent.OpenGate {
			if v, ok := a.input(g.Input); ok && v == g.Wildcard {
				open = true
			}
		}
		if !open {
			return true
		}
	}
	return false
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleCommentTriggerAuthorization) VisitJobPre(n *Job) error {
	if rule.trigger == "" || n.WorkflowCall != nil || n.If == nil && len(n.Steps) == 0 {
		// the job that calls a reusable workflow is not looked into: the callee may check the user
		return nil
	}
	steps := flattenSteps(n.Steps)
	site, prev, step := (*String)(nil), []*Step(nil), (*Step)(nil)
	if readsCommentBody(n.If) {
		// the job is started by the text, and a step of the job that checks the user (and fails) comes before the work
		site, prev = n.If, steps
	} else {
		for i, s := range steps {
			if readsCommentBody(s.If) {
				site, prev, step = s.If, steps[:i], s
				break
			}
		}
	}
	if site == nil || readsEarlierDecision(site) || runsGatedAgent(steps) || jobChecksWhoStarted(rule.wf, n, prev, step) {
		return nil
	}
	rule.ReportIDf("comment-trigger-authorization", site.Pos,
		"this condition starts the job from the text of a comment (the event is %q), and nothing checks who wrote it, so anyone who can comment can run the job with the token and the secrets of the repository. check github.event.comment.author_association (OWNER, MEMBER or COLLABORATOR), ask for the permission of github.actor in an earlier step or job, or run the job in an environment with required reviewers",
		rule.trigger)
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "comment-trigger-authorization", Group: RuleGroupSecurity, Summary: "A job of a comment-triggered workflow is started by the text of the comment and nothing checks who wrote it.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-comment-trigger-authorization"},
	)
	registerRuleFactory("comment-trigger-authorization", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("comment-trigger-authorization") {
			return nil
		}
		return []Rule{NewRuleCommentTriggerAuthorization()}
	})
}
