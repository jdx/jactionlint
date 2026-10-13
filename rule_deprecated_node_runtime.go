package jactionlint

import (
	"regexp"
	"strconv"
	"strings"
)

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
	comments *CommentIndex
}

// NewRuleDeprecatedNodeRuntime creates a new RuleDeprecatedNodeRuntime instance.
func NewRuleDeprecatedNodeRuntime() *RuleDeprecatedNodeRuntime {
	return &RuleDeprecatedNodeRuntime{
		RuleBase: RuleBase{
			name: "deprecated-node-runtime",
			desc: "Checks for Node.js runtimes which GitHub Actions removed: runs.using of an action and the popular actions at majors that declare one",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleDeprecatedNodeRuntime) VisitWorkflowPre(n *Workflow) error {
	if !rule.Config().RuleEnabled("deprecated-node-runtime") {
		return nil
	}
	rule.comments = n.Comments
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

// VisitStep is callback when visiting Step node.
func (rule *RuleDeprecatedNodeRuntime) VisitStep(n *Step) error {
	if !rule.Config().RuleEnabled("deprecated-node-runtime") {
		return nil
	}
	if a, ok := n.Exec.(*ExecAction); ok {
		rule.checkUses(a)
	}
	return nil
}

// checkUses reports an action of the popular actions data set whose major version declares a runtime that GitHub
// removed. The data holds the "runs.using" of every major, as it was when the data was generated.
func (rule *RuleDeprecatedNodeRuntime) checkUses(a *ExecAction) {
	if a.Uses == nil {
		return
	}
	ref := ParseUses(a.Uses.Value)
	if ref.Kind != UsesAction {
		return
	}
	ps, ok := resolvePopularSpec(ref, a.Uses, rule.comments, knownPopularAction)
	if !ok {
		return
	}
	runtime := PopularActionRuntimes[ps.Key]
	why, removed := nodeRuntimeRemoved[runtime]
	if !removed {
		return
	}
	msg := "action " + strconv.Quote(a.Uses.Value) + ps.note() + " declares the Node.js runtime " + strconv.Quote(runtime) + " in its metadata, but " + why +
		", so the runner runs the action on Node.js 24 instead of the runtime it declares."
	if next, nextRuntime, ok := newerPopularMajor(ps.Key); ok {
		msg += " update the action to " + strconv.Quote(next) + " or newer"
		if nextRuntime != "" {
			msg += ", which declares " + strconv.Quote(nextRuntime)
		}
	} else {
		msg += " update the action to a release that declares \"node24\""
	}
	rule.ReportID("deprecated-node-runtime", a.Uses.Pos, msg)
}

var popularKeyRe = regexp.MustCompile(`^(.+)@v([0-9]+)(?:\.[0-9]+)*$`)

// newerPopularMajor returns the oldest major version, newer than the one of the key, whose runtime is not a removed
// one. It is the spec ("actions/checkout@v5") and the runtime that the major declares ("node24"), which is empty for
// an action that does not run JavaScript.
func newerPopularMajor(key string) (spec, runtime string, ok bool) {
	m := popularKeyRe.FindStringSubmatch(key)
	if m == nil {
		return "", "", false
	}
	cur, err := strconv.Atoi(m[2])
	if err != nil {
		return "", "", false
	}
	best := -1
	for k := range PopularActions {
		km := popularKeyRe.FindStringSubmatch(k)
		if km == nil || km[1] != m[1] || k != km[1]+"@v"+km[2] {
			continue // another action, or not the key of a major version such as "v4.3.6" or "v3-node20"
		}
		v, err := strconv.Atoi(km[2])
		if err != nil || v <= cur || (best >= 0 && v >= best) {
			continue
		}
		if _, removed := nodeRuntimeRemoved[PopularActionRuntimes[k]]; removed {
			continue
		}
		best = v
	}
	if best < 0 {
		return "", "", false
	}
	spec = m[1] + "@v" + strconv.Itoa(best)
	return spec, PopularActionRuntimes[spec], true
}

func init() {
	registerRules(
		RuleInfo{ID: "deprecated-node-runtime", Group: RuleGroupPolicy, Summary: "A Node.js runtime that GitHub Actions removed is declared by an action or by an action that is used.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-deprecated-node-runtime"},
	)
	registerRuleFactory("deprecated-node-runtime", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleDeprecatedNodeRuntime()}
	})
}
