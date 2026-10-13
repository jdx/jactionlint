package jactionlint

import (
	"fmt"
	"strings"
)

// maxReportedPinProblems is how many dependencies a finding lists; the others are counted.
const maxReportedPinProblems = 3

// RuleUnpinnableAction reports an action which a workflow pins to a commit SHA but whose own steps use an
// action by a tag or a branch, or a Docker image by a tag: the pin does not freeze what runs, because the
// action pulls in something which can change. It is the "unpinnable action" check of poutine.
type RuleUnpinnableAction struct {
	onlineUsesRule
}

// NewRuleUnpinnableAction creates a new RuleUnpinnableAction instance.
func NewRuleUnpinnableAction(sess *onlineSession) *RuleUnpinnableAction {
	return &RuleUnpinnableAction{newOnlineUsesRule("unpinnable-action", "Checks that actions pinned to a commit SHA do not pull in actions or images by a ref which can move", sess)}
}

// VisitWorkflowPost implements Pass.
func (r *RuleUnpinnableAction) VisitWorkflowPost(*Workflow) error {
	for _, s := range r.sites {
		// A workflow call is not followed: its steps are not in the metadata which is read
		if s.ref.Kind != UsesAction || s.ref.RefKind != RefFullSHA || !remoteActionLookup(s.ref) {
			continue
		}
		problems, err := r.sess.PinProblems(s.ref.Owner, s.ref.Repo, s.ref.Subpath, s.ref.Ref)
		if err != nil {
			r.skipped(s, "the action metadata", err)
			continue
		}
		if len(problems) == 0 {
			continue
		}
		shown := make([]string, 0, maxReportedPinProblems)
		for _, p := range problems[:min(len(problems), maxReportedPinProblems)] {
			shown = append(shown, p.String())
		}
		more := ""
		if n := len(problems) - len(shown); n > 0 {
			more = fmt.Sprintf(" (and %d more)", n)
		}
		r.ReportIDf("unpinnable-action", s.pos,
			"action %q is pinned to commit %s, but the pin does not hold: %s%s. so what runs can change without a change to this workflow. use an action which pins its dependencies, or fork it and pin them there",
			s.ref.Raw, shortSHA(strings.ToLower(s.ref.Ref)), strings.Join(shown, "; "), more)
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "unpinnable-action", Group: RuleGroupSecurity, Summary: "A hash-pinned action pulls in an action or a Docker image by a tag or a branch, so its pin does not freeze what runs. Only with the option pedantic.", DefaultLevel: SeverityWarning, Online: true, DocsAnchor: "check-unpinnable-action", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("unpinnable-action", func(env *RuleEnv) []Rule {
		// Every pinned action is read, also the popular ones, so the check runs only when it is asked for
		if env.online == nil || !env.config.RuleEnabled("unpinnable-action") || !env.config.auditPedantic("unpinnable-action") {
			return nil
		}
		return []Rule{NewRuleUnpinnableAction(env.online)}
	})
}
