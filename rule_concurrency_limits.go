package jactionlint

import (
	"bytes"
	"path/filepath"
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
	src  []byte
	path string
}

// NewRuleConcurrencyLimits creates a new RuleConcurrencyLimits instance. The source is used to find
// where to report a workflow without any concurrency setting. It can be empty.
func NewRuleConcurrencyLimits(path string, src []byte) *RuleConcurrencyLimits {
	return &RuleConcurrencyLimits{
		RuleBase: RuleBase{
			name: "concurrency-limits",
			desc: "Checks that workflows limit concurrent runs with \"concurrency:\"",
		},
		src:  src,
		path: path,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleConcurrencyLimits) VisitWorkflowPre(n *Workflow) error {
	if isCopilotSetupSteps(rule.path) {
		return nil
	}
	if c := n.Concurrency; c != nil {
		if onlyScheduledOrManual(n) {
			return nil // nothing supersedes a timer or a manual run, so there is nothing to cancel
		}
		// Whether runs are cancelled is a choice: serializing a release pipeline is as valid as cancelling
		// the superseded runs of a test pipeline. Only the form which cannot cancel at all is reported.
		if c.Bare && !onlyWorkflowCall(n) && releaseReason(n) == "" {
			rule.ReportID("concurrency-limits", c.Pos, "\"concurrency:\" is only a group name, so it cannot cancel superseded runs. use the mapping form with \"group:\" and \"cancel-in-progress: true\"")
		}
		return nil
	}
	if onlyWorkflowCall(n) {
		// The caller decides how many runs of a reusable workflow exist
		return nil
	}
	if onlyScheduledOrManual(n) {
		// Nothing supersedes a run of a timer or of a person who started it by hand
		return nil
	}
	// Every job needs a limit, at the workflow or on the job itself. That includes the jobs which call a reusable
	// workflow: a concurrency group in the called workflow can deadlock with the one of the caller, so the caller is
	// where the limit belongs (zizmor#1619).
	hasJob := false
	limited := true
	for _, j := range n.Jobs {
		if j == nil {
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
	if why := releaseReason(n); why != "" {
		// Cancelling a release or a deployment that is running leaves it half done (concurrency-cancels-release), so the
		// group is there to make a new run wait for the running one
		msg = "workflow has no \"concurrency:\", so every run of it executes at the same time even when a newer run is started while one is running (" + why + "). add a top-level \"concurrency:\" with a \"group:\" and \"cancel-in-progress: false\", so that a new run waits for the running release or deployment to finish instead of cancelling it. a newer run that is waiting replaces an older one that is waiting, so add \"queue: max\" when no release may be skipped"
	} else if startedByPullRequest(n) {
		msg += ". use a group per pull request such as \"" + callerAwareGroup(n, rule.path) + "\", so that a new push cancels only the older runs of the same pull request and not the runs of the others"
	} else if callsReusableWorkflow(n) {
		msg += ". use a group such as \"" + callerAwareGroup(n, rule.path) + "\""
	}
	if callsReusableWorkflow(n) {
		msg += ". the workflow has jobs that call reusable workflows, and inside a called workflow \"github.workflow\" is the name of the caller, so a group that the caller and the called workflow both build from \"${{ github.workflow }}\" is the same one and the called workflow is cancelled at once. give the group of the caller its own part, such as \"-caller\", so that it never equals the group of a called workflow"
	}
	rule.ReportID("concurrency-limits", pos, msg)
	if fix := fixConcurrencyLimits(n); fix != nil {
		rule.errs[len(rule.errs)-1].Fix = fix
	}
	return nil
}

// releaseReason tells why the workflow releases or deploys something, or returns "" when it does not: it runs on the
// release event or on pushed tags, or a job has an environment or publishes or deploys when the workflow is run by one
// of its events. cancel-in-progress must not be recommended for such a workflow, see RuleConcurrencyCancelsRelease.
func releaseReason(w *Workflow) string {
	for _, e := range w.On {
		switch ev := e.(type) {
		case *WebhookEvent:
			if ev.EventName() == "release" {
				return "it runs for release events"
			}
			if ev.EventName() == "push" && tagFilterMatches(ev.Tags) {
				return "it runs for pushed tags"
			}
		}
	}
	for _, e := range w.On {
		event := e.EventName()
		if event == "workflow_call" {
			event = callerEvent // the caller decides what github.event_name is, so no condition on it is known
		}
		for _, id := range jobIDsInOrder(w) {
			if j := w.Jobs[id]; j != nil {
				if why := releaseSignal(j, scenario{event: event}); why != "" {
					return why
				}
			}
		}
	}
	return ""
}

// pullRequestGroup is the group that the fix writes: the runs of one pull request cancel each other and nothing else.
const pullRequestGroup = "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}"

// callerGroupMarker is what the group of a workflow that calls reusable workflows has in addition: inside a called
// workflow github.workflow is the name of the caller, so the same expression in both would be one group, and the run of
// the called workflow would cancel itself (or be cancelled by the caller) at once.
const callerGroupMarker = "-caller"

// callsReusableWorkflow reports whether a job of the workflow calls a reusable workflow with uses:.
func callsReusableWorkflow(w *Workflow) bool {
	for _, j := range w.Jobs {
		if j != nil && j.WorkflowCall != nil {
			return true
		}
	}
	return false
}

// callerAwareGroup is the group that the advice and the fix write. A workflow that calls reusable workflows gets a
// group that cannot equal the one of a callee.
//
// A workflow that is itself called and calls others in turn sees the name of the outermost caller in
// github.workflow, like its callees do, so the marker has the name of its own file to differ from both.
func callerAwareGroup(w *Workflow, path string) string {
	if callsReusableWorkflow(w) {
		marker := callerGroupMarker
		if isReusableWorkflow(w) {
			if stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)); path != "" && stem != "" {
				marker = "-" + stem + callerGroupMarker
			}
		}
		return "${{ github.workflow }}" + marker + "-${{ github.event.pull_request.number || github.ref }}"
	}
	return pullRequestGroup
}

// isReusableWorkflow reports whether the workflow can be called with workflow_call.
func isReusableWorkflow(w *Workflow) bool {
	for _, e := range w.On {
		if _, ok := e.(*WorkflowCallEvent); ok {
			return true
		}
	}
	return false
}

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
		Edits: []TextEdit{d.insertAfterLine(end, "", // a blank line separates the block from "on:"
			pad+"concurrency:",
			pad+unit+"group: "+callerAwareGroup(w, ""),
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
		return []Rule{NewRuleConcurrencyLimits(env.path, env.src)}
	})
}

// onlyScheduledOrManual reports whether the workflow is started only by a schedule or by hand. A new commit does not
// supersede such a run, so there is nothing for a concurrency group to cancel.
func onlyScheduledOrManual(n *Workflow) bool {
	if len(n.On) == 0 {
		return false
	}
	for _, e := range n.On {
		switch e.EventName() {
		case "schedule", "workflow_dispatch":
		default:
			return false
		}
	}
	return true
}
