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

// cacheDisabledByExpression reports whether the value of an input is an expression which looks at
// the trigger, which is the usual way of caching only outside of releases.
func cacheDisabledByExpression(v string) bool {
	if !strings.Contains(v, "${{") {
		return false
	}
	return looksAtTrigger(v)
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
	for _, e := range n.On {
		name := e.EventName()
		if slices.Contains(privilegedTriggers, name) && rule.privileged == "" {
			rule.privileged = name
		}
		switch ev := e.(type) {
		case *WebhookEvent:
			if name == "release" {
				rule.releaseWhy = "the workflow runs on the release event"
			}
			if name == "push" && !ev.Tags.IsEmpty() && rule.releaseWhy == "" {
				rule.releaseWhy = "the workflow runs on pushed tags"
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
		if !reads || stepCacheIsConditional(s, a, name) {
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

// cacheGateInputs lists, for the actions of cacheActions, the inputs which decide whether the action uses
// the cache at all. Only an expression in one of these can make the caching conditional on the trigger.
// Everything else (key, restore-keys, versions, paths) is ignored: a release workflow often has github.ref
// in a key or in node-version without the cache being off.
//
// Sources: the action.yml of each action. actions/cache and actions/cache/restore have none: their only
// switch is the step's own "if:".
//   - actions/setup-node, setup-python, setup-java, setup-dotnet, setup-go: "cache"
//     (setup-node also "package-manager-cache", which turns the automatic caching of v5 off)
//   - ruby/setup-ruby: "bundler-cache"
//   - astral-sh/setup-uv: "enable-cache"
//   - swatinem/rust-cache: "lookup-only" (its "save-if" only decides about saving, not about restoring)
//   - jdx/mise-action: "cache"
//   - gradle/actions/setup-gradle, gradle/gradle-build-action: "cache-disabled" ("cache-read-only" still
//     restores)
//   - docker/build-push-action: "cache-from"
var cacheGateInputs = map[string][]string{
	"actions/setup-node":                        {"cache", "package-manager-cache"},
	"actions/setup-python":                      {"cache"},
	"actions/setup-java":                        {"cache"},
	"actions/setup-dotnet":                      {"cache"},
	"actions/setup-go":                          {"cache"},
	"ruby/setup-ruby":                           {"bundler-cache"},
	"astral-sh/setup-uv":                        {"enable-cache"},
	"swatinem/rust-cache":                       {"lookup-only"},
	"jdx/mise-action":                           {"cache"},
	"gradle/actions/setup-gradle":               {"cache-disabled"},
	"gradle/gradle-build-action":                {"cache-disabled"},
	"docker/build-push-action":                  {"cache-from"},
	"actions/cache":                             nil,
	"actions/cache/restore":                     nil,
	"hendrikmuhs/ccache-action":                 nil,
	"determinatesystems/magic-nix-cache-action": nil,
}

// stepCacheIsConditional reports whether the step or an input which controls the caching of the action
// depends on the trigger, which is how a workflow caches outside of releases only.
func stepCacheIsConditional(s *Step, a *ExecAction, name string) bool {
	if s.If != nil && looksAtTrigger(s.If.Value) {
		return true
	}
	for _, in := range cacheGateInputs[name] {
		if v, ok := a.input(in); ok && cacheDisabledByExpression(v) {
			return true
		}
	}
	return false
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
