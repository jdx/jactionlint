package jactionlint

import (
	"regexp"
	"strings"
)

// checkoutCommandRegex matches a shell command which fetches or prepares a repository working tree. It is used to
// tell that a `run:` step prepares the workspace by hand instead of using an checkout action. It is intentionally
// loose (anything on the same line as `git` mentioning one of the sub commands) to avoid false positives.
var checkoutCommandRegex = regexp.MustCompile(`(?m)\bgit\b[^\n]*\b(clone|init|fetch|pull|checkout|worktree|submodule)\b|\bgh\s+(repo\s+clone|pr\s+checkout)\b`)

// RuleLocalActionCheckout is a rule to check that a local action (`uses: ./path`) is not used before the repository
// is checked out in the same job. This rule is opt-in and enabled by the "require-checkout-before-local-action"
// configuration.
type RuleLocalActionCheckout struct {
	RuleBase
	checkedOut bool
	reported   bool
	handled    map[*Step]bool
}

// NewRuleLocalActionCheckout creates a new RuleLocalActionCheckout instance.
func NewRuleLocalActionCheckout() *RuleLocalActionCheckout {
	return &RuleLocalActionCheckout{
		RuleBase: RuleBase{
			name: "local-action-checkout",
			desc: "Checks that a local action is not used before checking out the repository (opt-in)",
		},
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleLocalActionCheckout) VisitJobPre(n *Job) error {
	rule.checkedOut = false
	rule.reported = false
	rule.handled = nil
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleLocalActionCheckout) VisitStep(n *Step) error {
	if rule.handled[n] {
		// Already checked as a part of the enclosing `parallel` group
		return nil
	}
	if rule.checkedOut || rule.reported {
		return nil
	}
	if !rule.Config().RuleEnabled("local-action-checkout") {
		return nil
	}
	if rule.checkStep(n, false) {
		rule.checkedOut = true
	}
	return nil
}

// checkStep checks the step and reports whether it checks out the repository. 'checkedOut' is whether the
// repository was already checked out before the step starts. Steps in a `parallel` group run concurrently so each of
// them sees the state at the start of the group. A checkout in the group is only available after the group.
func (rule *RuleLocalActionCheckout) checkStep(n *Step, checkedOut bool) bool {
	switch e := n.Exec.(type) {
	case *ExecRun:
		return e.Run != nil && checkoutCommandRegex.MatchString(e.Run.Value)
	case *ExecAction:
		if e.Uses == nil {
			return false
		}
		spec := e.Uses.Value
		// "$/path" is resolved by the runner without a checkout so it is not subject to this check
		if strings.HasPrefix(spec, "./") {
			if !checkedOut && !rule.reported {
				rule.reported = true
				rule.ReportIDf(
					"local-action-checkout",
					e.Uses.Pos,
					"local action %q is used before any checkout step in this job. the repository is not on the runner yet so the action cannot be found. add \"actions/checkout\" before this step or use \"$/\" syntax. this is reported because the \"local-action-checkout\" rule is enabled",
					spec,
				)
			}
			return false
		}
		return isCheckoutActionSpec(spec)
	case *ExecParallel:
		if rule.handled == nil {
			rule.handled = map[*Step]bool{}
		}
		any := false
		for _, c := range e.Steps {
			if c == nil {
				continue
			}
			rule.handled[c] = true
			if rule.checkStep(c, checkedOut) {
				any = true
			}
		}
		return any
	}
	return false
}

// isCheckoutActionSpec returns whether the `uses:` value looks like an action to check out a repository such as
// "actions/checkout@v4". Any action whose owner/repo name contains "checkout" is accepted.
func isCheckoutActionSpec(spec string) bool {
	if strings.HasPrefix(spec, "docker://") || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "$/") {
		return false
	}
	s, _, _ := strings.Cut(spec, "@")
	parts := strings.SplitN(s, "/", 3)
	if len(parts) < 2 {
		return false
	}
	return strings.Contains(strings.ToLower(parts[0]+"/"+parts[1]), "checkout")
}

func init() {
	registerRules(
		RuleInfo{ID: "local-action-checkout", Group: RuleGroupCorrectness, Summary: "A local action is used before any step checks out the repository.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-local-action-checkout"},
	)
	registerRuleFactory("local-action-checkout", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleLocalActionCheckout()}
	})
}
