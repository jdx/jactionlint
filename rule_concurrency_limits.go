package jactionlint

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
)

// RuleConcurrencyLimits is a rule to detect workflows without a concurrency limit. By default GitHub
// runs every instance of a workflow at once even when a newer run supersedes the older ones, which
// wastes runner minutes and can race on artifacts.
// https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#concurrency
type RuleConcurrencyLimits struct {
	RuleBase
	src []byte
}

// NewRuleConcurrencyLimits creates a new RuleConcurrencyLimits instance. The source is used to find
// where to report a workflow without any concurrency setting. It can be empty.
func NewRuleConcurrencyLimits(src []byte) *RuleConcurrencyLimits {
	return &RuleConcurrencyLimits{
		RuleBase: RuleBase{
			name: "concurrency-limits",
			desc: "Checks that workflows limit concurrent runs with \"concurrency:\"",
		},
		src: src,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleConcurrencyLimits) VisitWorkflowPre(n *Workflow) error {
	if c := n.Concurrency; c != nil {
		// Whether runs are cancelled is a choice: serializing a release pipeline is as valid as cancelling
		// the superseded runs of a test pipeline. Only the form which cannot cancel at all is reported.
		if c.Bare && !onlyWorkflowCall(n) {
			rule.ReportID("concurrency-limits", c.Pos, "\"concurrency:\" is only a group name, so it cannot cancel superseded runs. use the mapping form with \"group:\" and \"cancel-in-progress: true\"")
		}
		return nil
	}
	if onlyWorkflowCall(n) {
		// The caller decides how many runs of a reusable workflow exist
		return nil
	}
	// Jobs which call a reusable workflow are limited by that workflow. Every other job needs a limit,
	// at the workflow or on the job itself.
	hasJob := false
	limited := true
	for _, j := range n.Jobs {
		if j == nil || j.WorkflowCall != nil {
			continue
		}
		hasJob = true
		if j.Concurrency == nil {
			limited = false
		}
	}
	if !hasJob || limited {
		return nil
	}

	pos := &Pos{Line: 1, Col: 1}
	if line, ok := onKeyLine(rule.src); ok {
		pos = &Pos{Line: line, Col: 1}
	} else if line, ok := firstKeyLine(rule.src); ok {
		pos = &Pos{Line: line, Col: 1}
	}
	msg := "workflow has no \"concurrency:\", so every run of it executes at the same time even when a newer run supersedes the older ones. add a top-level \"concurrency:\" with a \"group:\" and \"cancel-in-progress: true\""
	if startedByPullRequest(n) {
		msg += ". use a group per pull request such as \"" + pullRequestGroup + "\", so that a new push cancels only the older runs of the same pull request and not the runs of the others"
	}
	rule.ReportID("concurrency-limits", pos, msg)
	if fix := fixConcurrencyLimits(n); fix != nil {
		rule.errs[len(rule.errs)-1].Fix = fix
	}
	return nil
}

// pullRequestGroup is the group that the fix writes: the runs of one pull request cancel each other and nothing else.
const pullRequestGroup = "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}"

// startedByPullRequest reports whether the workflow runs for an event of a pull request.
func startedByPullRequest(w *Workflow) bool {
	for _, e := range w.On {
		if slices.Contains(pullRequestEvents, e.EventName()) {
			return true
		}
	}
	return false
}

// anchorRe matches an anchor or an alias used as a value or a sequence item, and the merge key.
var anchorRe = regexp.MustCompile(`(?m)^[^#\n]*(?:[:-][ \t]+[&*][A-Za-z0-9_-]+|<<[ \t]*:)`)

// fixConcurrencyLimits makes the fix which adds a top-level "concurrency:" that cancels the superseded runs of the
// same pull request. It is a safe fix, so it is offered only where cancelling cannot hurt:
//
//   - every trigger is an event of a pull request, so there is no push, tag, release or manual run to cancel;
//   - no job releases or deploys anything (an environment, a publish or deploy command or action);
//   - no job has a "concurrency:" of its own;
//   - the file is written in a shape the edit understands: no anchors or aliases, and the "on:" entry is a block
//     the parser and the helpers agree on.
func fixConcurrencyLimits(w *Workflow) *Fix {
	if len(w.On) == 0 {
		return nil
	}
	for _, e := range w.On {
		if !slices.Contains(pullRequestEvents, e.EventName()) {
			return nil
		}
	}
	sc := releaseScenario{scenario: scenario{event: "pull_request"}}
	for _, j := range w.Jobs {
		if j == nil || j.Concurrency != nil || releaseSignal(j, sc.scenario) != "" {
			return nil
		}
	}
	d := newSrcDoc(w.Source)
	if d == nil || anchorRe.Match(w.Source) {
		return nil
	}
	if _, _, _, ok := d.topLevelKey("concurrency"); ok {
		return nil // the parser did not see it, so the shape is not understood
	}
	line, indent, inline, ok := d.topLevelKey("on")
	if !ok {
		line, indent, inline, ok = d.topLevelKey(`"on"`)
	}
	if !ok || strings.HasPrefix(inline, "&") || strings.HasPrefix(inline, "*") {
		return nil
	}
	end, ok := d.entryEnd(line, indent, inline)
	if !ok {
		return nil
	}
	if (strings.HasPrefix(inline, "{") || strings.HasPrefix(inline, "[")) && end != line {
		return nil // a flow collection over several lines
	}
	pad := strings.Repeat(" ", indent)
	unit := strings.Repeat(" ", d.indentUnit())
	return &Fix{
		Description: "Add concurrency that cancels superseded runs of the same pull request",
		Edits: []TextEdit{d.insertAfterLine(end,
			pad+"concurrency:",
			pad+unit+"group: "+pullRequestGroup,
			pad+unit+"cancel-in-progress: true",
		)},
	}
}

var onKeyRegexp = regexp.MustCompile(`^(?:on|"on"|'on')[ \t]*:`)

// onKeyLine returns the 1-based line of the top-level "on:" key.
func onKeyLine(src []byte) (int, bool) {
	for i, l := range bytes.Split(src, []byte("\n")) {
		if onKeyRegexp.Match(l) {
			return i + 1, true
		}
	}
	return 0, false
}

// onlyWorkflowCall reports whether the workflow can be started only by being called.
func onlyWorkflowCall(n *Workflow) bool {
	if len(n.On) == 0 {
		return false
	}
	for _, e := range n.On {
		if _, ok := e.(*WorkflowCallEvent); !ok {
			return false
		}
	}
	return true
}

func init() {
	registerRules(
		RuleInfo{ID: "concurrency-limits", Group: RuleGroupPolicy, Summary: "A workflow does not cancel superseded runs with concurrency:.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-concurrency-limits"},
	)
	registerRuleFactory("concurrency-limits", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("concurrency-limits") {
			return nil
		}
		return []Rule{NewRuleConcurrencyLimits(env.src)}
	})
}
