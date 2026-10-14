package jactionlint

import (
	"fmt"
	"strings"
)

// superfluousAction is an action that does what a tool of the runner image does as well (usually the GitHub CLI
// `gh`, `git` or `docker`). Such an action adds a dependency, and the supply chain risk that comes with it, for
// nothing.
type superfluousAction struct {
	// action is the canonical (lower case) name of the action.
	action string
	// instead says what to do instead, in a script step.
	instead string
	// pedantic entries are reported with "superfluous-actions-pedantic": replacing the action takes more than
	// one command, or the action does more than the tool, so not everybody agrees that it is superfluous.
	pedantic bool
	// source says where the entry comes from.
	source string
}

const (
	sourceZizmor      = "zizmor superfluous-actions"
	sourceArchivedGH  = "the README of the action: unmaintained and archived by GitHub, which points to the release actions"
	instReleaseCreate = "use `gh release create` in a script step"
)

// superfluousActions is the table of the actions the rule reports. Each entry names its source. Entries are
// matched on the canonical name of the action, whatever its subpath or ref.
var superfluousActions = []superfluousAction{
	{action: "ncipollo/release-action", instead: instReleaseCreate, source: sourceZizmor},
	{action: "softprops/action-gh-release", instead: instReleaseCreate, source: sourceZizmor},
	{action: "elgohr/github-release-action", instead: instReleaseCreate, source: sourceZizmor},
	{action: "svenstaro/upload-release-action", instead: "use `gh release create` and `gh release upload` in a script step", source: sourceZizmor},
	{action: "actions/create-release", instead: instReleaseCreate, source: sourceArchivedGH},
	{action: "actions/upload-release-asset", instead: "use `gh release upload` in a script step", source: sourceArchivedGH},
	{action: "dacbd/create-issue-action", instead: "use `gh issue create` in a script step", source: sourceZizmor},
	{action: "actions-ecosystem/action-add-labels", instead: "use `gh issue edit --add-label` or `gh pr edit --add-label` in a script step", source: sourceZizmor},
	{action: "actions-ecosystem/action-remove-labels", instead: "use `gh issue edit --remove-label` or `gh pr edit --remove-label` in a script step", source: sourceZizmor},
	{action: "addnab/docker-run-action", instead: "use `docker run` in a script step", source: sourceZizmor},
	{action: "benc-uk/workflow-dispatch", instead: "use `gh workflow run` (with `--ref` and `-f name=value` for the inputs) in a script step", source: "zizmor#2181"},
	{action: "sergeysova/jq-action", instead: "use `jq`, which the runner images include, in a script step", source: sourceZizmor},
	// The following replacements take more than one command or miss a feature of the action. zizmor puts them in
	// its pedantic persona as well.
	{action: "peter-evans/create-pull-request", instead: "use `gh pr create` in a script step", pedantic: true, source: sourceZizmor},
	{action: "peter-evans/create-or-update-comment", instead: "use `gh pr comment` or `gh issue comment` in a script step", pedantic: true, source: sourceZizmor},
	{action: "dtolnay/rust-toolchain", instead: "use `rustup` in a script step", pedantic: true, source: sourceZizmor},
	{action: "stefanzweifel/git-auto-commit-action", instead: "use `git add`, `git commit` and `git push` in a script step", pedantic: true, source: sourceZizmor},
	{action: "endbug/add-and-commit", instead: "use `git add`, `git commit` and `git push` in a script step", pedantic: true, source: sourceZizmor},
}

// RuleSuperfluousActions detects steps that use an action which is known to do what the runner image can do
// without it. See superfluousActions for the table.
type RuleSuperfluousActions struct {
	RuleBase
	selfHosted bool // the job runs, or may run, on a self-hosted runner: it cannot be shown to be hosted
	wf         *Workflow
}

// NewRuleSuperfluousActions creates a new RuleSuperfluousActions instance.
func NewRuleSuperfluousActions() *RuleSuperfluousActions {
	return &RuleSuperfluousActions{
		RuleBase: NewRuleBase("superfluous-actions", "Checks for actions that do what the runner image can do without them"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleSuperfluousActions) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleSuperfluousActions) VisitJobPre(n *Job) error {
	rule.selfHosted = false
	if n.RunsOn == nil {
		return nil
	}
	scope := &runnerScope{job: n, wf: rule.wf}
	check := func(l *String) {
		if l == nil {
			return
		}
		if !l.ContainsExpression() {
			if strings.EqualFold(l.Value, "self-hosted") {
				rule.selfHosted = true
			}
			return
		}
		// A label written as text and matrix values (`${{ matrix.os }}-latest`, `${{ matrix.target.os }}`) is
		// resolved for every value of the matrix, when all of them are known
		if labels, ok := matrixImages(n, l.Value); ok {
			for _, v := range labels {
				if strings.EqualFold(strings.TrimSpace(v), "self-hosted") {
					rule.selfHosted = true
				}
			}
			return
		}
		// An expression is resolved to the labels it can have when the repository tells all of them (a condition
		// between labels, the inputs that every caller passes, fromJSON() of a text). Whatever cannot be resolved
		// (`vars.RUNNER`, `inputs.runner` without a caller, a matrix computed by fromJSON) counts as maybe self-hosted.
		if !l.IsExpressionAssigned() {
			rule.selfHosted = true
			return
		}
		exprs, ok := l.templateExprs()
		if !ok || len(exprs) != 1 {
			rule.selfHosted = true
			return
		}
		labels, ok := scope.staticRunnerLabels(exprs[0])
		if !ok {
			rule.selfHosted = true
			return
		}
		for _, v := range labels {
			if strings.EqualFold(strings.TrimSpace(v), "self-hosted") {
				rule.selfHosted = true
			}
		}
	}
	check(n.RunsOn.LabelsExpr)
	for _, l := range n.RunsOn.Labels {
		check(l)
	}
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleSuperfluousActions) VisitJobPost(n *Job) error {
	rule.selfHosted = false
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleSuperfluousActions) VisitStep(n *Step) error {
	if rule.selfHosted {
		// The tools of the GitHub-hosted images are not there, so the action is what installs them (zizmor#1865)
		return nil
	}
	a, u := usesOfStep(n)
	if a == nil || u.Kind != UsesAction {
		return nil
	}
	name := u.CanonicalName()
	for _, s := range superfluousActions {
		if s.action != name {
			continue
		}
		msg := fmt.Sprintf("action %q is superfluous: the runner already has the tools to do this. %s", a.Uses.Value, s.instead)
		if s.pedantic {
			if rule.pedantic("superfluous-actions") {
				rule.errorIDAt("superfluous-actions", a.Uses.Pos, msg)
			}
		} else {
			rule.errorIDAt("superfluous-actions", a.Uses.Pos, msg)
		}
		break
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "superfluous-actions", Group: RuleGroupSecurity, Summary: "An action does what a tool of the runner image does as well, such as gh release create.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-superfluous-actions", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("superfluous-actions", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("superfluous-actions") {
			return nil
		}
		return []Rule{NewRuleSuperfluousActions()}
	})
}
