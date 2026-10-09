package jactionlint

import "strings"

// RuleSelfRepository is a rule checker which reports `uses: ./path` and prefers the self-repository
// form `uses: $/path`. Both name an action or a reusable workflow of the repository running the
// workflow. The workspace-relative form is looked up in the workspace at run time, where an earlier
// step can have put something else (for instance a checkout of another ref or repository), and it
// cannot be told apart from an arbitrary directory by a policy that requires pinning. The `$/` form
// always resolves to the commit that runs the workflow.
//
// The fix is unsafe: `$/` is newer syntax which not every GitHub Enterprise Server or runner understands and
// which actionlint 1.7.12 and older reject (the repository then fails the other linter), and
// when a step replaces the workspace with another checkout `./` and `$/` are not the same code.
type RuleSelfRepository struct {
	RuleBase
	src []byte
	idx *sourceIndex
}

// NewRuleSelfRepository creates a new RuleSelfRepository instance. The source of the file is needed
// to attach fixes. It can be nil, in which case findings carry no fix.
func NewRuleSelfRepository(src []byte) *RuleSelfRepository {
	return &RuleSelfRepository{
		RuleBase: RuleBase{
			name: "self-repository",
			desc: "Checks that actions and reusable workflows of the same repository are referenced as \"$/path\" instead of \"./path\"",
		},
		src: src,
	}
}

func (rule *RuleSelfRepository) check(u *String, workflowCall bool) {
	if u == nil || !strings.HasPrefix(u.Value, "./") {
		return
	}
	suggested := "$/" + strings.TrimPrefix(u.Value, "./")
	if workflowCall {
		// A reusable workflow is resolved before any step runs, so nothing can replace it
		rule.ReportIDf(
			"self-repository",
			u.Pos,
			"%q is a path that cannot be told apart from an arbitrary directory by a policy that requires pinning. use the self-repository syntax %q, which names the reusable workflow of this repository at the commit running the workflow",
			u.Value, suggested,
		)
	} else {
		rule.ReportIDf(
			"self-repository",
			u.Pos,
			"%q is looked up in the workspace at run time, where an earlier step can replace it. use the self-repository syntax %q which always refers to the commit running the workflow",
			u.Value, suggested,
		)
	}
	if rule.src == nil {
		return
	}
	if rule.idx == nil {
		rule.idx = newSourceIndex(rule.src)
	}
	if edit, ok := rule.idx.selfRepositoryEdit(u.Pos); ok {
		rule.errs[len(rule.errs)-1].Fix = &Fix{
			Description: "Use the self-repository syntax \"$/\" (needs a recent runner and GitHub Enterprise Server; actionlint 1.7.12 and older reject it)",
			Unsafe:      true,
			Edits:       []TextEdit{edit},
		}
	}
}

// VisitStep is callback when visiting Step node.
func (rule *RuleSelfRepository) VisitStep(n *Step) error {
	if !rule.Config().RuleEnabled("self-repository") {
		return nil
	}
	if a, ok := n.Exec.(*ExecAction); ok {
		rule.check(a.Uses, false)
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleSelfRepository) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("self-repository") {
		return nil
	}
	if n.WorkflowCall != nil {
		rule.check(n.WorkflowCall.Uses, true)
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "self-repository", Group: RuleGroupSecurity, Summary: "A local action or workflow is referenced as ./path instead of $/path.", DefaultLevel: SeverityInfo, Profile: ProfilePedantic, Fixable: true, DocsAnchor: "check-self-repository"},
	)
	registerRuleFactory("self-repository", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleSelfRepository(env.Source())}
	})
}
