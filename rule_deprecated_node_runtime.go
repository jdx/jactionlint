package jactionlint

import "strings"

// nodeRuntimeRemoved says what GitHub did to a Node.js runtime of JavaScript actions, by the name that
// runs.using has. The facts come from the changelog of 2026-09-23 ("Node 20 is no longer available in GitHub
// Actions") and from the runner (src/Runner.Worker/Handlers/HandlerFactory.cs and
// src/Runner.Common/Util/NodeUtil.cs): since then every JavaScript action runs on Node.js 24 on github.com,
// whatever it declares, and the runner still accepts the old names, so such an action runs and does not fail.
var nodeRuntimeRemoved = map[string]string{
	"node12": "Node.js 12 was removed from the runner in 2023",
	"node16": "Node.js 16 was removed from the runner in 2024",
	"node20": "Node.js 20 was removed from GitHub Actions on 2026-09-23",
}

// RuleDeprecatedNodeRuntime reports the settings of GitHub Actions about the Node.js runtime of JavaScript
// actions that GitHub removed. A JavaScript action declares its runtime in runs.using. The runner no longer
// has Node.js 20 (or 16, or 12): it runs the action on Node.js 24 whatever it declares. The action still
// works unless its code breaks on Node.js 24, so this is hygiene and not an error: the author never tested
// the action on the runtime it runs on, and a runner that is not updated (GitHub Enterprise Server, an old
// self-hosted runner) behaves differently. It is a policy rule of the pedantic profile.
type RuleDeprecatedNodeRuntime struct {
	RuleBase
}

// NewRuleDeprecatedNodeRuntime creates a new RuleDeprecatedNodeRuntime instance.
func NewRuleDeprecatedNodeRuntime() *RuleDeprecatedNodeRuntime {
	return &RuleDeprecatedNodeRuntime{
		RuleBase: RuleBase{
			name: "deprecated-node-runtime",
			desc: "Checks for Node.js runtimes which GitHub Actions removed: runs.using of an action",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleDeprecatedNodeRuntime) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("deprecated-node-runtime") {
		return nil
	}
	if n.Action != nil {
		rule.checkRunsUsing(n.Action.Runs)
	}
	return nil
}

func (rule *RuleDeprecatedNodeRuntime) checkRunsUsing(r *ActionFileRuns) {
	if r == nil || r.Using == nil {
		return
	}
	why, ok := nodeRuntimeRemoved[strings.ToLower(r.Using.Value)]
	if !ok {
		return
	}
	rule.ReportIDf(
		"deprecated-node-runtime",
		r.Using.Pos,
		"\"runs.using\" of this action is %q, but %s, so the runner runs the action on Node.js 24 instead of the runtime it declares. declare \"node24\" and test the action on Node.js 24 (GitHub Enterprise Server and old self-hosted runners may still need the old name)",
		r.Using.Value, why,
	)
}

func init() {
	registerRules(
		RuleInfo{ID: "deprecated-node-runtime", Group: RuleGroupPolicy, Summary: "A Node.js runtime that GitHub Actions removed is declared or configured.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-deprecated-node-runtime"},
	)
	registerRuleFactory("deprecated-node-runtime", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleDeprecatedNodeRuntime()}
	})
}
