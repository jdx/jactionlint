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
	// repo is the "owner/repo" of the repository, lower case, "" when it is not known
	repo       string
	checkedOut bool
	// populated is whether an earlier step may have put files in the workspace by other means than a checkout
	// (an artifact, an archive that was extracted)
	populated bool
	reported  bool
	handled   map[*Step]bool
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
	rule.populated = false
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
	if stepPopulatesWorkspace(n) {
		rule.populated = true
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
			// An action outside of ".github/" in the workspace can come from an artifact or an archive that
			// an earlier step unpacked, which is not a checkout
			if rule.populated && !strings.HasPrefix(spec, "./.github/") {
				return false
			}
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
		return isCheckoutActionSpecIn(spec, rule.repo) || hasCheckoutInputs(e)
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

// extractCommandRegex matches the commands that unpack files into the workspace.
var extractCommandRegex = regexp.MustCompile(`(?m)\b(tar|unzip|7z|7za|gunzip|unxz|rsync)\b`)

// stepPopulatesWorkspace reports whether the step may put files in the workspace without a checkout: it downloads an
// artifact or unpacks an archive.
func stepPopulatesWorkspace(n *Step) bool {
	switch e := n.Exec.(type) {
	case *ExecRun:
		return e.Run != nil && extractCommandRegex.MatchString(e.Run.Value)
	case *ExecAction:
		if e.Uses == nil {
			return false
		}
		name := strings.ToLower(ParseUses(e.Uses.Value).CanonicalName())
		return name == "actions/download-artifact"
	}
	return false
}

// isCheckoutActionSpec returns whether the `uses:` value looks like an action to check out a repository such as
// "actions/checkout@v4". What a remote composite action does is unknown without fetching it, so any action whose
// name contains "checkout" is accepted, in the owner, the repository or the path in the repository:
// "pytorch/pytorch/.github/actions/checkout-pytorch@main" is a wrapper which checks out. A local action ("./...")
// is never one, because it needs the checkout itself.
func isCheckoutActionSpec(spec string) bool {
	return isCheckoutActionSpecIn(spec, "")
}

// isCheckoutActionSpecIn is isCheckoutActionSpec for a workflow of the repository ("owner/repo", lower case, or ""
// when it is not known). A composite action of the repository itself, `uses: owner/repo/.github/actions/prep@main`,
// is a wrapper that may check the repository out whatever its name is.
func isCheckoutActionSpecIn(spec, repo string) bool {
	if strings.HasPrefix(spec, "docker://") || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "$/") {
		return false
	}
	s, _, _ := strings.Cut(spec, "@")
	parts := strings.SplitN(s, "/", 3)
	if len(parts) < 2 {
		return false
	}
	if repo != "" && len(parts) == 3 && strings.EqualFold(parts[0]+"/"+parts[1], repo) && strings.HasPrefix(parts[2], ".github/actions/") {
		return true
	}
	return strings.Contains(strings.ToLower(s), "checkout")
}

// checkoutInputs are the inputs of actions/checkout which a wrapper with another name passes on. A remote action
// which is given one of them is taken for a checkout.
var checkoutInputs = []string{"fetch-depth", "persist-credentials", "submodules", "sparse-checkout", "sparse-checkout-cone-mode", "lfs", "fetch-tags", "set-safe-directory"}

// hasCheckoutInputs reports whether a remote action is configured like a checkout. The name of the action does not
// say what it does, but nobody passes fetch-depth to an action that does not fetch.
func hasCheckoutInputs(e *ExecAction) bool {
	spec := e.Uses.Value
	if strings.HasPrefix(spec, "docker://") || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "$/") {
		return false
	}
	for _, name := range checkoutInputs {
		if _, ok := e.Inputs[name]; ok {
			return true
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "local-action-checkout", Group: RuleGroupCorrectness, Summary: "A local action is used before any step checks out the repository.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-local-action-checkout"},
	)
	registerRuleFactory("local-action-checkout", func(env *RuleEnv) []Rule {
		r := NewRuleLocalActionCheckout()
		if env.project != nil {
			r.repo = githubRepositoryOf(env.project.RootDir())
		}
		return []Rule{r}
	})
}
