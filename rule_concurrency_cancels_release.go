package jactionlint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleConcurrencyCancelsRelease reports `cancel-in-progress` that cancels a release or a deployment which
// is still running. A new push must not kill an in-flight release.
type RuleConcurrencyCancelsRelease struct {
	RuleBase
	wf        *Workflow
	src       *sourceIndex
	scenarios []releaseScenario
}

// releaseScenario is a way the workflow can be triggered other than by a pull request.
type releaseScenario struct {
	scenario
	// high is true when the trigger is a release by itself: a tag push or a release event. Other triggers
	// only matter when a job does something that looks like a release.
	high bool
	// explicit is true when a second run for the same ref is a person starting it again: a tag push, a
	// release or a manual run. With the ref in the group the second run only replaces the first one then,
	// which is what the person wants.
	explicit bool
	// what describes the trigger for the message.
	what string
}

// NewRuleConcurrencyCancelsRelease creates a new RuleConcurrencyCancelsRelease instance.
func NewRuleConcurrencyCancelsRelease(src []byte) *RuleConcurrencyCancelsRelease {
	r := &RuleConcurrencyCancelsRelease{
		RuleBase: RuleBase{
			name: "concurrency-cancels-release",
			desc: "Checks that cancel-in-progress does not cancel a release or a deployment which is running",
		},
	}
	if len(src) > 0 {
		r.src = newSourceIndex(src)
	}
	return r
}

// deployTools are the commands that deploy when they are given one of the verbs.
var deployTools = map[string][]string{
	"wrangler": {"deploy", "publish"}, "flyctl": {"deploy"}, "fly": {"deploy"}, "vercel": {"deploy"}, "netlify": {"deploy"},
	"firebase": {"deploy"}, "serverless": {"deploy"}, "sls": {"deploy"}, "cdk": {"deploy"}, "docker": {"push"},
	"kubectl": {"apply", "rollout"}, "helm": {"upgrade", "install"}, "terraform": {"apply"}, "goreleaser": {"release"},
}

// launchers start a tool from a package: `bunx wrangler deploy`, `pnpm dlx wrangler deploy`.
var launchers = map[string]bool{"npx": true, "bunx": true, "pnpx": true, "yarn": true, "pnpm": true, "bun": true, "npm": true, "dlx": true, "exec": true, "x": true, "-y": true, "--yes": true}

// deployCommand reports whether the command deploys or publishes something and describes it.
func deployCommand(c *runscript.Command) (string, bool) {
	words := make([]string, 0, len(c.Words))
	for _, w := range c.Words {
		words = append(words, strings.ToLower(w.Value))
	}
	i := 0
	for i < len(words) && launchers[words[i]] {
		i++
	}
	if i >= len(words) {
		return "", false
	}
	tool := strings.SplitN(words[i], "@", 2)[0]
	verbs, ok := deployTools[tool]
	if !ok {
		return "", false
	}
	if tool == "goreleaser" {
		if goreleaserReleasesNothing(words[i+1:]) {
			return "", false
		}
	}
	for _, w := range words[i+1:] {
		if slices.Contains(verbs, w) {
			return tool + " " + w, true
		}
	}
	return "", false
}

// goreleaserReleasesNothing reports whether the arguments of goreleaser make a run that publishes nothing: a snapshot, or
// a run that skips the publish step (--skip-publish, --skip=publish, --skip publish, with other steps in the list as
// well). Other skips (--skip=validate, --skip-validate, --skip=sign) still release.
func goreleaserReleasesNothing(args []string) bool {
	skipsPublish := func(list string) bool {
		return slices.Contains(strings.Split(list, ","), "publish")
	}
	for i, w := range args {
		switch {
		case w == "--snapshot" || w == "--snapshot=true":
			return true
		case w == "--skip-publish" || w == "--skip-publish=true":
			return true
		case strings.HasPrefix(w, "--skip="):
			if skipsPublish(strings.TrimPrefix(w, "--skip=")) {
				return true
			}
		case w == "--skip" && i+1 < len(args):
			if skipsPublish(args[i+1]) {
				return true
			}
		}
	}
	return false
}

// releaseActions are actions that publish a release or deploy something.
var releaseActions = map[string]bool{
	"softprops/action-gh-release":                   true,
	"ncipollo/release-action":                       true,
	"actions/create-release":                        true,
	"actions/upload-release-asset":                  true,
	"marvinpinto/action-automatic-releases":         true,
	"googleapis/release-please-action":              true,
	"google-github-actions/release-please-action":   true,
	"release-drafter/release-drafter":               true,
	"pypa/gh-action-pypi-publish":                   true,
	"js-devtools/npm-publish":                       true,
	"rubygems/release-gem":                          true,
	"actions/deploy-pages":                          true,
	"peaceiris/actions-gh-pages":                    true,
	"cloudflare/wrangler-action":                    true,
	"cloudflare/pages-action":                       true,
	"azure/webapps-deploy":                          true,
	"aws-actions/amazon-ecs-deploy-task-definition": true,
	"google-github-actions/deploy-cloudrun":         true,
	"google-github-actions/deploy-appengine":        true,
	"amondnet/vercel-action":                        true,
	"nwtgck/actions-netlify":                        true,
	"jamesives/github-pages-deploy-action":          true,
	"crazy-max/ghaction-github-release":             true,
	"changesets/action":                             true,
	"semantic-release/semantic-release":             true,
	"cycjimmy/semantic-release-action":              true,
}

// inputTruth evaluates the value of an input that is true or false in the scenario.
func (sc scenario) inputTruth(v string) condTruth {
	v = strings.TrimSpace(v)
	switch {
	case isTrueLiteral(v):
		return condTrue
	case isFalseLiteral(v):
		return condFalse
	}
	exprs, ok := parseTemplateExprs(v)
	if !ok || len(exprs) != 1 || !isExprAssigned(v) {
		return condUnknown
	}
	return sc.eval(exprs[0])
}

// conditionIn evaluates the `if:` condition in the scenario. A missing condition is true.
func (sc scenario) conditionIn(c *String) condTruth {
	if c == nil {
		return condTrue
	}
	exprs, ok := conditionExprs(c)
	if !ok || len(exprs) != 1 || !isExprAssigned(c.Value) && c.ContainsExpression() {
		return condUnknown
	}
	return sc.eval(exprs[0])
}

// releaseSignal tells why the job releases or deploys something when the workflow runs in the
// scenario, "" if it does not. A job or a step whose condition is false in the scenario is ignored.
func releaseSignal(j *Job, sc scenario) string {
	if sc.conditionIn(j.If) == condFalse {
		return ""
	}
	if j.Environment != nil && j.Environment.Name != nil && j.Environment.Name.Value != "" {
		return fmt.Sprintf("job %q uses the environment %q", j.ID.Value, j.Environment.Name.Value)
	}
	windows := false
	if j.RunsOn != nil {
		for _, l := range j.RunsOn.Labels {
			if strings.Contains(strings.ToLower(l.Value), "windows") {
				windows = true
			}
		}
	}
	for _, s := range flattenSteps(j.Steps) {
		if sc.conditionIn(s.If) == condFalse {
			continue
		}
		switch e := s.Exec.(type) {
		case *ExecAction:
			_, u := stepAction(s)
			if u == nil || u.Kind != UsesAction {
				continue
			}
			name := strings.ToLower(u.CanonicalName())
			switch {
			case releaseActions[name]:
				return fmt.Sprintf("job %q uses %q", j.ID.Value, u.CanonicalName())
			case name == "docker/build-push-action":
				if v, ok := e.input("push"); ok && sc.inputTruth(v) == condTrue {
					return fmt.Sprintf("job %q pushes a Docker image", j.ID.Value)
				}
			case name == "goreleaser/goreleaser-action":
				// A check or a snapshot build does not release anything
				if v, ok := e.input("args"); ok && slices.Contains(strings.Fields(v), "release") && !goreleaserReleasesNothing(strings.Fields(strings.ToLower(v))) {
					return fmt.Sprintf("job %q runs goreleaser release", j.ID.Value)
				}
			}
		case *ExecRun:
			if windows && e.Shell == nil {
				continue // the default shell is not bash
			}
			script, _ := analyzeRun(e)
			if script == nil {
				continue
			}
			for _, c := range script.Commands {
				if p := c.Publishes(); p != nil && !p.DryRun {
					return fmt.Sprintf("job %q runs \"%s %s\"", j.ID.Value, p.Tool, p.Verb)
				}
				if what, ok := deployCommand(c); ok {
					return fmt.Sprintf("job %q runs \"%s\"", j.ID.Value, what)
				}
			}
		}
	}
	return ""
}

func isLiteralBranch(p string) bool {
	return p != "" && !strings.ContainsAny(p, "*?[]+!\\")
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
// callerEvent stands for the event of the caller of a reusable workflow in a scenario. github.event_name is
// the caller's event, which is not known here, so the scenario has to leave it unknown for the expressions.
const callerEvent = ""

func (rule *RuleConcurrencyCancelsRelease) VisitWorkflowPre(n *Workflow) error {
	rule.scenarios, rule.wf = nil, n
	if !rule.Config().RuleEnabled("concurrency-cancels-release") {
		return nil
	}
	if n.Concurrency == nil && !anyJobConcurrency(n) {
		return nil
	}
	for _, e := range n.On {
		name := e.EventName()
		switch name {
		case "pull_request", "pull_request_target", "pull_request_review", "pull_request_review_comment":
		case "push":
			if we, ok := e.(*WebhookEvent); ok {
				rule.addPushScenarios(we)
			}
		case "release":
			rule.scenarios = append(rule.scenarios, releaseScenario{scenario{event: name, refPrefix: "refs/tags/"}, true, true, "release events"})
		case "workflow_dispatch":
			rule.scenarios = append(rule.scenarios, releaseScenario{scenario{event: name}, false, true, "manual runs"})
		case "workflow_call":
			// github.event_name is the event of the caller, which is not known here
			rule.scenarios = append(rule.scenarios, releaseScenario{scenario{event: callerEvent}, false, false, "calls from other workflows"})
		default:
			rule.scenarios = append(rule.scenarios, releaseScenario{scenario{event: name}, false, false, fmt.Sprintf("%q events", name)})
		}
	}
	if len(rule.scenarios) == 0 {
		return nil
	}
	rule.check(n.Concurrency, jobsInOrder(n))
	return nil
}

func jobsInOrder(n *Workflow) []*Job {
	ids := jobIDsInOrder(n)
	jobs := make([]*Job, 0, len(ids))
	for _, id := range ids {
		jobs = append(jobs, n.Jobs[id])
	}
	return jobs
}

func anyJobConcurrency(n *Workflow) bool {
	for _, j := range n.Jobs {
		if j.Concurrency != nil {
			return true
		}
	}
	return false
}

func (rule *RuleConcurrencyCancelsRelease) addPushScenarios(we *WebhookEvent) {
	hasTags, hasBranches := !we.Tags.IsEmpty(), !we.Branches.IsEmpty()
	if hasTags {
		rule.scenarios = append(rule.scenarios, releaseScenario{scenario{event: "push", refPrefix: "refs/tags/"}, true, true, "pushes of tags"})
	}
	if hasBranches {
		for _, b := range we.Branches.Values {
			sc := scenario{event: "push", refPrefix: "refs/heads/"}
			if isLiteralBranch(b.Value) {
				sc.ref, sc.refPrefix = "refs/heads/"+b.Value, ""
			}
			rule.scenarios = append(rule.scenarios, releaseScenario{sc, false, false, fmt.Sprintf("pushes to %q", b.Value)})
		}
	}
	if !hasTags && !hasBranches {
		rule.scenarios = append(rule.scenarios, releaseScenario{scenario{event: "push"}, false, false, "pushes"})
	}
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleConcurrencyCancelsRelease) VisitJobPre(n *Job) error {
	if len(rule.scenarios) == 0 || n.Concurrency == nil {
		return nil
	}
	// The trigger of the workflow is not enough for a job: only a job that does the release is reported
	rule.check(n.Concurrency, []*Job{n})
	return nil
}

// groupSeparatesReleases reports whether the group has something that differs between runs for
// different refs (the ref, the commit, a run).
func groupSeparatesRefs(refs []exprRef) bool {
	for _, r := range refs {
		for _, d := range [][]string{{"github", "ref"}, {"github", "ref_name"}, {"github", "sha"}, {"github", "run_id"}, {"github", "run_number"}} {
			if refCovers(r.chain, d) {
				return true
			}
		}
	}
	return false
}

// groupIsScopedToRelease reports whether the group names the release itself for a run of the event: the release of a
// release event, or an input of a manual or reusable run. A second run then replaces a run for the same release only.
// The inputs of a push of a tag are empty, so every tag shares the group.
func groupIsScopedToRelease(refs []exprRef, event string) bool {
	for _, r := range refs {
		switch event {
		case "release":
			if refCovers(r.chain, []string{"github", "event", "release"}) {
				return true
			}
		case "workflow_dispatch", callerEvent:
			for _, d := range [][]string{{"github", "event", "inputs"}, {"inputs"}} {
				if refCovers(r.chain, d) {
					return true
				}
			}
		}
	}
	return false
}

// check reports the concurrency block when it cancels a release. jobs are the jobs it covers.
func (rule *RuleConcurrencyCancelsRelease) check(c *Concurrency, jobs []*Job) {
	if c == nil || c.CancelInProgress == nil {
		return
	}
	var groupRefs []exprRef
	if c.Group != nil {
		groupRefs, _ = stringExprRefs(c.Group.Value)
	}
	jobLevel := c != rule.wf.Concurrency
	for _, sc := range rule.scenarios {
		if !sc.isTrue(c.CancelInProgress) || (sc.explicit && groupSeparatesRefs(groupRefs)) || groupIsScopedToRelease(groupRefs, sc.event) {
			continue
		}
		signal := ""
		for _, j := range jobs {
			if signal = releaseSignal(j, sc.scenario); signal != "" {
				break
			}
		}
		var why string
		switch {
		case signal != "":
			why = fmt.Sprintf("%s and the workflow runs for %s", signal, sc.what)
		case sc.high && !jobLevel:
			why = fmt.Sprintf("the workflow runs for %s", sc.what)
		default:
			continue
		}
		rule.report(c, why)
		return
	}
}

func (rule *RuleConcurrencyCancelsRelease) report(c *Concurrency, why string) {
	pos := c.CancelInProgress.Pos
	if pos == nil {
		pos = c.Pos
	}
	rule.ReportIDf(
		"concurrency-cancels-release",
		pos,
		"\"cancel-in-progress\" is enabled here (%s), so a new run cancels a release or deployment which is still running and can leave it half done. set \"cancel-in-progress: false\" (or remove it) to let the running one finish first. keep the group per ref or tag so that unrelated releases do not wait for each other, and add \"queue: max\" when no release may be skipped (otherwise a newer pending run replaces an older pending one)",
		why,
	)
	if c.CancelInProgress.Expression != nil || !c.CancelInProgress.Value {
		return
	}
	if edit, ok := rule.cancelEdit(c.CancelInProgress.Pos); ok {
		errs := rule.Errs()
		errs[len(errs)-1].Fix = &Fix{
			Description: "Set cancel-in-progress to false",
			// A queued run now waits for the running one instead of replacing it
			Unsafe: true,
			Edits:  []TextEdit{edit},
		}
	}
}

// cancelEdit returns the edit that changes the literal `true` at the position to `false`.
func (rule *RuleConcurrencyCancelsRelease) cancelEdit(p *Pos) (TextEdit, bool) {
	off, ok := rule.src.offsetOf(p)
	if !ok {
		return TextEdit{}, false
	}
	src := rule.src.src
	quote := byte(0)
	if off < len(src) && (src[off] == '"' || src[off] == '\'') {
		quote = src[off]
		off++
	}
	if quote != 0 {
		return TextEdit{}, false // a quoted "true" is a string, not a boolean
	}
	if off+4 > len(src) || !strings.EqualFold(string(src[off:off+4]), "true") {
		return TextEdit{}, false
	}
	if off+4 < len(src) {
		switch src[off+4] {
		case ' ', '\t', '\r', '\n', '#', ',', '}':
		default:
			return TextEdit{}, false
		}
	}
	return TextEdit{Start: off, End: off + 4, NewText: "false"}, true
}

func init() {
	registerRules(
		RuleInfo{ID: "concurrency-cancels-release", Group: RuleGroupCorrectness, Summary: "cancel-in-progress can cancel a release or a deployment which is still running.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-concurrency-cancels-release"},
	)
	registerRuleFactory("concurrency-cancels-release", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleConcurrencyCancelsRelease(env.Source())}
	})
}
