package jactionlint

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// cacheAction describes an action which restores a cache. reads reports whether a given use of the
// action restores one, with a short hint how to switch that off.
type cacheAction struct {
	name  string
	reads func(a *ExecAction, u *UsesRef) (bool, string)
}

// looksAtTrigger reports whether the text mentions the event or the ref which started the run:
// github.event_name, github.ref, github.ref_name, github.ref_type or a tag ref.
func looksAtTrigger(v string) bool {
	return strings.Contains(v, "github.event_name") || strings.Contains(v, "github.ref") || strings.Contains(v, "refs/tags")
}

// majorVersionRegex captures the major version of a tag such as v3.1.0.
var majorVersionRegex = regexp.MustCompile(`^v?([0-9]+)`)

// inputEnabled returns the value of the input and whether it is set to something which is neither
// empty nor a literal false.
func inputEnabled(a *ExecAction, name string) (string, bool) {
	v, ok := a.input(name)
	return v, ok && v != "" && !isFalseLiteral(v)
}

// cacheActions lists the actions that restore a cache. It is not exhaustive: it has the actions of
// GitHub and the popular third-party ones whose caching behavior is known.
var cacheActions = []cacheAction{
	{"actions/cache", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, _ := a.input("lookup-only"); isTrueLiteral(v) {
			return false, ""
		}
		return true, "remove this step or set \"lookup-only: true\""
	}},
	{"actions/cache/restore", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, _ := a.input("lookup-only"); isTrueLiteral(v) {
			return false, ""
		}
		return true, "remove this step or set \"lookup-only: true\""
	}},
	{"actions/setup-node", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "package-manager-cache: false" only turns off the automatic caching of v5. An explicit "cache"
		// input still restores a cache.
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
	}},
	{"actions/setup-python", func(a *ExecAction, u *UsesRef) (bool, string) {
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
	}},
	{"actions/setup-java", func(a *ExecAction, u *UsesRef) (bool, string) {
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
	}},
	{"actions/setup-dotnet", func(a *ExecAction, u *UsesRef) (bool, string) {
		v, ok := a.input("cache")
		return ok && !isFalseLiteral(v) && v != "", "remove the \"cache\" input"
	}},
	{"actions/setup-go", func(a *ExecAction, u *UsesRef) (bool, string) {
		v, set := a.input("cache")
		if set && isFalseLiteral(v) {
			return false, ""
		}
		if !set && u.RefKind == RefSemverTag {
			// Caching is on by default since v4
			if m := majorVersionRegex.FindStringSubmatch(u.Ref); m != nil {
				if major, err := strconv.Atoi(m[1]); err == nil && major < 4 {
					return false, ""
				}
			}
		}
		return true, "set \"cache: false\""
	}},
	{"ruby/setup-ruby", func(a *ExecAction, u *UsesRef) (bool, string) {
		_, ok := inputEnabled(a, "bundler-cache")
		return ok, "remove the \"bundler-cache\" input"
	}},
	{"astral-sh/setup-uv", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "auto" is the default and enables the cache on GitHub-hosted runners
		v, set := a.input("enable-cache")
		if set && isFalseLiteral(v) {
			return false, ""
		}
		return true, "set \"enable-cache: false\""
	}},
	{"swatinem/rust-cache", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, _ := a.input("lookup-only"); isTrueLiteral(v) {
			return false, ""
		}
		return true, "remove this step or set \"lookup-only: true\""
	}},
	{"jdx/mise-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, ok := a.input("cache"); ok && isFalseLiteral(v) {
			return false, ""
		}
		return true, "set \"cache: false\""
	}},
	{"gradle/actions/setup-gradle", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, _ := a.input("cache-disabled"); isTrueLiteral(v) {
			return false, ""
		}
		return true, "set \"cache-disabled: true\""
	}},
	{"gradle/gradle-build-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, _ := a.input("cache-disabled"); isTrueLiteral(v) {
			return false, ""
		}
		return true, "set \"cache-disabled: true\""
	}},
	{"docker/build-push-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		v, _ := a.input("cache-from")
		return strings.Contains(v, "type=gha"), "remove the \"cache-from\" input"
	}},
	{"hendrikmuhs/ccache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
	{"determinatesystems/magic-nix-cache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
}

// publishingActions are actions that publish a release or a package. A job using one of them is a
// release job whatever triggers the workflow.
var publishingActions = []string{
	"pypa/gh-action-pypi-publish",
	"softprops/action-gh-release",
	"ncipollo/release-action",
	"goreleaser/goreleaser-action",
	"rubygems/release-gem",
	"svenstaro/upload-release-action",
	"js-devtools/npm-publish",
	"actions/create-release",
	"actions/upload-release-asset",
	"actions/deploy-pages",
	"peaceiris/actions-gh-pages",
	"jamesives/github-pages-deploy-action",
	"cloudflare/wrangler-action",
}

// publishCommandRegex matches a command of a `run:` script that publishes a release or a package.
var publishCommandRegex = regexp.MustCompile(`(?m)\b(cargo\s+publish|npm\s+publish|pnpm\s+publish|yarn\s+(npm\s+)?publish|twine\s+upload|gem\s+push|poetry\s+publish|uv\s+publish|docker\s+push|gh\s+release\s+(create|upload)|goreleaser\s+(release|publish)|vsce\s+publish|mvn\b[^\n]*\bdeploy|gradle\w*\s+[^\n]*\bpublish)\b`)

// RuleCachePoisoning is a rule checker for two ways the GitHub Actions cache turns into an attack path:
//
//   - a job that publishes artifacts (a workflow triggered by a release or by pushed tags, or a job
//     using a publishing action or command) restores a cache. Whoever can write that cache can slip
//     code into the release. `cache-mode: none` at the workflow or the job switches caching off.
//   - a workflow on a privileged trigger saves a cache with `cache-mode: write` or `write-only`, which
//     lets untrusted code plant the entries that release workflows later restore.
type RuleCachePoisoning struct {
	RuleBase
	wf         *Workflow
	releaseWhy string // why the workflow is a release workflow, or ""
	privileged string // the privileged trigger of the workflow, or ""
	// scenarios are the runs of a release workflow that publish; eventScenarios one run per event of the workflow,
	// for a job that publishes in a workflow which is not triggered by a release
	scenarios, eventScenarios []triggerScenario
}

// NewRuleCachePoisoning creates a new RuleCachePoisoning instance.
func NewRuleCachePoisoning() *RuleCachePoisoning {
	return &RuleCachePoisoning{
		RuleBase: RuleBase{
			name: "cache-poisoning",
			desc: "Checks for caches restored in release workflows and caches written by privileged triggers",
		},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleCachePoisoning) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	rule.releaseWhy, rule.privileged = "", ""
	rule.scenarios, rule.eventScenarios = nil, nil
	for _, e := range n.On {
		name := e.EventName()
		rule.eventScenarios = append(rule.eventScenarios, triggerScenario{"event_name": name})
		if slices.Contains(privilegedTriggers, name) && rule.privileged == "" {
			rule.privileged = name
		}
		switch ev := e.(type) {
		case *WebhookEvent:
			if name == "release" {
				rule.releaseWhy = "the workflow runs on the release event"
				rule.scenarios = append(rule.scenarios, scenarioRelease)
			}
			if name == "push" && !ev.Tags.IsEmpty() {
				if rule.releaseWhy == "" {
					rule.releaseWhy = "the workflow runs on pushed tags"
				}
				rule.scenarios = append(rule.scenarios, scenarioTagPush)
			}
		}
	}
	if !rule.Config().RuleEnabled("cache-poisoning") {
		return nil
	}
	if rule.privileged != "" {
		rule.checkWrite(n.CacheMode)
	}
	return nil
}

func (rule *RuleCachePoisoning) checkWrite(m *String) {
	if m == nil || (m.Value != "write" && m.Value != "write-only") {
		return
	}
	rule.ReportIDf(
		"cache-poisoning",
		m.Pos,
		"\"cache-mode: %s\" lets code triggered by %q write cache entries that other workflows, including release workflows, restore later. use \"cache-mode: read\" or \"none\"",
		m.Value, rule.privileged,
	)
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleCachePoisoning) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("cache-poisoning") {
		return nil
	}
	if rule.privileged != "" {
		rule.checkWrite(n.CacheMode)
	}

	mode := n.CacheMode
	if mode == nil {
		mode = rule.wf.CacheMode
	}
	if mode != nil && mode.Value == "none" {
		return nil
	}

	steps := flattenSteps(n.Steps)
	why := rule.releaseWhy
	if why == "" {
		why = publishingReason(steps)
	}
	if why == "" {
		return nil
	}

	scenarios := rule.scenarios
	if rule.releaseWhy == "" {
		scenarios = rule.eventScenarios // a publishing job in a workflow which is not a release workflow
	}
	for _, s := range steps {
		a, ref := stepAction(s)
		if a == nil || ref.Kind != UsesAction {
			continue
		}
		name := ref.CanonicalName()
		i := slices.IndexFunc(cacheActions, func(c cacheAction) bool { return c.name == name })
		if i < 0 {
			continue
		}
		reads, hint := cacheActions[i].reads(a, ref)
		if !reads || !cacheCanRunOnReleaseTrigger(n, s, a, name, scenarios) {
			continue
		}
		rule.ReportIDf(
			"cache-poisoning",
			a.Uses.Pos,
			"%s restores a cache although %s, so a poisoned cache entry can end up in the published artifacts. %s, or set \"cache-mode: none\" on the job",
			name, why, hint,
		)
	}
	return nil
}

// cacheGate is an input which decides whether an action uses the cache. offWhenTrue is set for the inputs that
// switch the cache off ("lookup-only: true"); for the others a value that is empty or false switches it off.
type cacheGate struct {
	input       string
	offWhenTrue bool
}

// cacheGateInputs lists, for the actions of cacheActions, the inputs which decide whether the action
// restores the cache at all, because `reads` reports exactly the cases they switch on. Only an expression
// in one of these can make the caching conditional on the trigger. Everything else (key, restore-keys,
// versions, paths) is ignored: a release workflow often has github.ref in a key or in node-version
// without the cache being off. Sources are the action.yml files of the actions.
//
// Left out on purpose: setup-node's "package-manager-cache" only turns off the automatic detection of v5 and
// leaves an explicit "cache" as it is, and rust-cache's "save-if" and gradle's "cache-read-only" only
// stop saving, the restore still happens.
var cacheGateInputs = map[string][]cacheGate{
	// "cache" names the package manager to cache: empty or false means none
	"actions/setup-node":   {{input: "cache"}},
	"actions/setup-python": {{input: "cache"}},
	"actions/setup-java":   {{input: "cache"}},
	"actions/setup-dotnet": {{input: "cache"}},
	"actions/setup-go":     {{input: "cache"}}, // "cache: false" turns off the default
	// "bundler-cache: true" runs bundle install with a cache
	"ruby/setup-ruby": {{input: "bundler-cache"}},
	// "enable-cache: false" turns off the default
	"astral-sh/setup-uv": {{input: "enable-cache"}},
	// "lookup-only: true" checks for a cache without restoring it
	"swatinem/rust-cache": {{input: "lookup-only", offWhenTrue: true}},
	// "cache: false" turns off the cache of mise-action
	"jdx/mise-action": {{input: "cache"}},
	// "cache-disabled: true" turns off all caching of the action
	"gradle/actions/setup-gradle": {{input: "cache-disabled", offWhenTrue: true}},
	"gradle/gradle-build-action":  {{input: "cache-disabled", offWhenTrue: true}},
	// "cache-from" with type=gha is what restores the cache; empty means none
	"docker/build-push-action": {{input: "cache-from"}},
	// the next ones have no switch: only the "if:" of the step can gate them (actions/cache has "lookup-only",
	// which `reads` handles as a literal)
	"actions/cache":                             nil,
	"actions/cache/restore":                     nil,
	"hendrikmuhs/ccache-action":                 nil,
	"determinatesystems/magic-nix-cache-action": nil,
}

// publishingReason tells why the steps publish something, or returns "".
func publishingReason(steps []*Step) string {
	for _, s := range steps {
		switch e := s.Exec.(type) {
		case *ExecAction:
			if e.Uses == nil {
				continue
			}
			u := ParseUses(e.Uses.Value)
			name := strings.ToLower(u.CanonicalName())
			if u.Kind == UsesAction && slices.ContainsFunc(publishingActions, func(p string) bool { return name == p || strings.HasPrefix(name, p+"/") }) {
				return "the job publishes with " + name
			}
			if u.Kind == UsesAction && name == "docker/build-push-action" {
				if v, _ := e.input("push"); isTrueLiteral(v) {
					return "the job pushes an image"
				}
			}
		case *ExecRun:
			if e.Run != nil {
				if m := publishCommandRegex.FindString(e.Run.Value); m != "" {
					return "the job runs \"" + strings.Join(strings.Fields(m), " ") + "\""
				}
			}
		}
	}
	return ""
}

func init() {
	registerRules(
		RuleInfo{ID: "cache-poisoning", Group: RuleGroupSecurity, Summary: "A cache is restored in a release job or written by a privileged trigger.", DefaultLevel: SeverityWarning, Profile: ProfileStrict, DocsAnchor: "check-cache-poisoning"},
	)
	registerRuleFactory("cache-poisoning", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleCachePoisoning()}
	})
}
