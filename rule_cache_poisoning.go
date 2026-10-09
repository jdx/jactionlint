package jactionlint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// majorOf returns the major version of a ref that is a version tag.
func majorOf(u *UsesRef) (int, bool) {
	if u.RefKind != RefSemverTag {
		return 0, false
	}
	m := majorVersionRegex.FindStringSubmatch(u.Ref)
	if m == nil {
		return 0, false
	}
	major, err := strconv.Atoi(m[1])
	return major, err == nil
}

// withCommentVersion returns the reference of a step, as the version of its comment when the action is pinned to a
// commit and the comment says which version that is ("# v10.0.0"). Rules that decide by the version of an action
// can then tell it from a commit hash. The reference is returned as it is when there is no such comment.
func withCommentVersion(w *Workflow, a *ExecAction, u *UsesRef) *UsesRef {
	if u.RefKind != RefFullSHA || w == nil || w.Comments == nil || a.Uses == nil || a.Uses.Pos == nil {
		return u
	}
	c := w.Comments.Inline(a.Uses.Pos.Line)
	if c == nil {
		return u
	}
	v, ok := commentVersion(c.Text)
	if !ok {
		return u
	}
	cp := *u
	cp.Ref, cp.RefKind = v, RefSemverTag
	return &cp
}

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
		// "auto" is the default and enables the cache on GitHub-hosted runners. From v10 it does not on the
		// events that are open to cache poisoning (zizmor#2320)
		v, set := a.input("enable-cache")
		if set && isFalseLiteral(v) {
			return false, ""
		}
		if !set || strings.EqualFold(strings.TrimSpace(v), "auto") {
			if major, ok := majorOf(u); ok && major >= 10 {
				return false, ""
			}
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
	{"docker/setup-buildx-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		// The buildx binary is cached in the GitHub Actions cache unless "cache-binary" is false (input of v3)
		if v, ok := a.input("cache-binary"); ok && isFalseLiteral(v) {
			return false, ""
		}
		if major, ok := majorOf(u); ok && major < 3 {
			return false, ""
		}
		return true, "set \"cache-binary: false\""
	}},
	{"hendrikmuhs/ccache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
	{"determinatesystems/magic-nix-cache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
}

// automaticCaches are the actions that restore a cache without an input asking for one, when the repository is set
// up for it. The function reports whether the use of the action does, as far as the files of the repository (root,
// "" when unknown) tell, and how to switch it off.
var automaticCaches = map[string]func(a *ExecAction, u *UsesRef, root string) (bool, string){
	"actions/setup-node": setupNodeCachesAutomatically,
}

// setupNodeCachesAutomatically tells whether actions/setup-node restores the cache of the package manager on its
// own. From v5 it does when package.json names a package manager and "package-manager-cache" is not false (see
// packageManagerOf for what v5 and v6 read). The action reads package.json at the root of the
// workspace, so the file of the repository decides: a repository without one (or without the field) gets no cache.
// Without a repository to look at (the source of a workflow on its own) the answer is yes.
func setupNodeCachesAutomatically(a *ExecAction, u *UsesRef, root string) (bool, string) {
	const hint = "set \"package-manager-cache: false\""
	if v, ok := a.input("package-manager-cache"); ok && isFalseLiteral(v) {
		return false, ""
	}
	major, known := majorOf(u)
	if known && major < 5 {
		return false, ""
	}
	if root == "" {
		return true, hint + ". the action caches on its own when package.json has a \"packageManager\" field"
	}
	pm := packageManagerOf(filepath.Join(root, "package.json"), !known || major >= 6)
	if pm == "" {
		return false, ""
	}
	return true, hint + ". package.json has \"packageManager\": \"" + pm + "\", so the action caches on its own"
}

// npmPackageManagerRe is the pattern with which setup-node v6 recognizes npm: "npm", "npm@10" or "^npm@10".
var npmPackageManagerRe = regexp.MustCompile(`^(\^)?npm(@.*)?$`)

// packageManagerOf returns the name of the package manager that package.json selects for the automatic caching of
// setup-node, or "" when the file does not exist or does not select one. It follows the code of the action: v5 reads
// only the top-level "packageManager" ("npm@10", "^pnpm@9" for npm, yarn and pnpm), v6 and later check every entry of
// "devEngines.packageManager" and then the top-level field, and know npm only.
func packageManagerOf(file string, v6 bool) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var pkg struct {
		PackageManager any `json:"packageManager"`
		DevEngines     struct {
			PackageManager json.RawMessage `json:"packageManager"`
		} `json:"devEngines"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return ""
	}
	top, _ := pkg.PackageManager.(string)
	if !v6 {
		name, _, found := strings.Cut(strings.TrimPrefix(top, "^"), "@")
		if found && (name == "npm" || name == "yarn" || name == "pnpm") {
			return name
		}
		return ""
	}
	isNpm := npmPackageManagerRe.MatchString
	var one struct {
		Name any `json:"name"`
	}
	var many []struct {
		Name any `json:"name"`
	}
	if json.Unmarshal(pkg.DevEngines.PackageManager, &many) == nil {
		for _, m := range many {
			if s, ok := m.Name.(string); ok && isNpm(s) {
				return "npm"
			}
		}
	} else if json.Unmarshal(pkg.DevEngines.PackageManager, &one) == nil {
		if s, ok := one.Name.(string); ok && isNpm(s) {
			return "npm"
		}
	}
	if isNpm(top) {
		return "npm"
	}
	return ""
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
	root       string // the root of the repository, "" when unknown
	wf         *Workflow
	tagOnly    bool   // the workflow is a release workflow only because it runs on pushed tags
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

// releaseTrigger tells how the events publish a release ("runs on the release event" or "runs on
// pushed tags"), or returns "" when they do not. scenarios are the runs of the events that publish.
func releaseTrigger(events []Event) (why string, scenarios []triggerScenario) {
	for _, e := range events {
		ev, ok := e.(*WebhookEvent)
		if !ok {
			continue
		}
		if e.EventName() == "release" {
			why = "runs on the release event"
			scenarios = append(scenarios, scenarioRelease)
		}
		if e.EventName() == "push" && tagFilterMatches(ev.Tags) {
			if why == "" {
				why = "runs on pushed tags"
			}
			scenarios = append(scenarios, scenarioTagPush)
		}
	}
	return why, scenarios
}

// tagFilterMatches reports whether the `tags` filter of a push event lets a pushed tag through. A filter of
// negative patterns only (`tags: ['!**']`) matches nothing: GitHub needs a positive pattern in the list, and the
// last pattern that matches decides, so a list that ends with `!**` excludes every tag.
func tagFilterMatches(f *WebhookEventFilter) bool {
	if f.IsEmpty() {
		return false
	}
	positive := false
	for _, v := range f.Values {
		if v != nil && !strings.HasPrefix(strings.TrimSpace(v.Value), "!") {
			positive = true
		}
	}
	if !positive {
		return false
	}
	last := f.Values[len(f.Values)-1]
	return last == nil || strings.TrimSpace(last.Value) != "!**"
}

// eventScenarios lists one run per event.
func eventScenarios(events []Event) []triggerScenario {
	var ret []triggerScenario
	for _, e := range events {
		ret = append(ret, triggerScenario{"event_name": e.EventName()})
	}
	return ret
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleCachePoisoning) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	rule.releaseWhy, rule.privileged, rule.tagOnly = "", "", false
	rule.scenarios, rule.eventScenarios = nil, nil
	for _, e := range n.On {
		if name := e.EventName(); slices.Contains(privilegedTriggers, name) && rule.privileged == "" {
			rule.privileged = name
		}
	}
	if n.Action == nil {
		if why, sc := releaseTrigger(n.On); why != "" {
			rule.releaseWhy, rule.scenarios = "the workflow "+why, sc
			rule.tagOnly = !slices.ContainsFunc(sc, func(s triggerScenario) bool { return s["event_name"] == "release" })
		}
		rule.eventScenarios = eventScenarios(n.On)
	} else if c := n.Action.Callers; c.Known() {
		// The action runs in the context of the workflow which calls it. A caller which releases is enough:
		// a cache restored there ends up in the published artifacts.
		for _, cl := range c.Callers {
			if why, sc := releaseTrigger(cl.Events); why != "" {
				if rule.releaseWhy == "" {
					rule.releaseWhy = fmt.Sprintf("%s %s and calls this action", cl.describe(), why)
				}
				rule.scenarios = append(rule.scenarios, sc...)
			}
			rule.eventScenarios = append(rule.eventScenarios, eventScenarios(cl.Events)...)
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

// cannotPublish reports whether a job of a workflow that runs on pushed tags is a check and not a release: the
// workflow says what the token may do and grants nothing but read access (`permissions: read-all`, `{}`,
// `contents: read`), and the job has no environment and calls no reusable workflow. A tag is often only a way to
// start the checks (pytorch pushes ciflow/* tags), and nothing is published without a write permission, a secret or
// an environment. A job that runs a publishing command or action is judged by publishingReason. Without
// `permissions:` the token is whatever the repository sets, which is not known, so the job may publish.
func (rule *RuleCachePoisoning) cannotPublish(j *Job) bool {
	if j.Environment != nil || j.WorkflowCall != nil {
		return false
	}
	p := j.Permissions
	if p == nil {
		p = rule.wf.Permissions
	}
	if p == nil {
		return false
	}
	if p.All != nil {
		return !strings.EqualFold(strings.TrimSpace(p.All.Value), "write-all")
	}
	for _, sc := range p.Scopes {
		if sc != nil && sc.Value != nil && strings.EqualFold(strings.TrimSpace(sc.Value.Value), "write") {
			return false
		}
	}
	return true
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
	why, scenarios := rule.releaseWhy, rule.scenarios
	if why != "" && rule.tagOnly && rule.cannotPublish(n) {
		why = "" // a tag only starts a check here, see cannotPublish
	}
	if why == "" {
		// a publishing job in a workflow which is not a release workflow
		if why = publishingReason(steps); why == "" {
			return nil
		}
		scenarios = rule.eventScenarios
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
		reads, hint := cacheActions[i].reads(a, withCommentVersion(rule.wf, a, ref))
		if auto := automaticCaches[name]; !reads && auto != nil {
			root := rule.root
			if rule.wf.Action != nil {
				root = "" // the workspace of the caller decides
			}
			reads, hint = auto(a, withCommentVersion(rule.wf, a, ref), root)
		}
		if !reads || !cacheCanRunOnReleaseTrigger(n, s, a, name, scenarios) {
			continue
		}
		rule.ReportIDf(
			"cache-poisoning",
			a.Uses.Pos,
			"%s restores a cache although %s, so a poisoned cache entry can end up in the published artifacts. %s, or set \"cache-mode: none\" on the job%s",
			name, why, hint, callerJob(rule.wf),
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
// Left out on purpose: rust-cache's "save-if" and gradle's "cache-read-only" only stop saving, the restore still
// happens. setup-node's "package-manager-cache" only turns off the automatic detection of v5 and leaves an explicit
// "cache" as it is, so cacheCanRunOnReleaseTrigger looks at it only when there is no explicit "cache".
var cacheGateInputs = map[string][]cacheGate{
	// "cache" names the package manager to cache: empty or false means none
	"actions/setup-node":   {{input: "cache"}},
	"actions/setup-python": {{input: "cache"}},
	"actions/setup-java":   {{input: "cache"}},
	"actions/setup-dotnet": {{input: "cache"}},
	"actions/setup-go":     {{input: "cache"}}, // "cache: false" turns off the default
	// "cache-binary: false" stops setup-buildx-action from restoring the buildx binary from the cache
	"docker/setup-buildx-action": {{input: "cache-binary"}},
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
		RuleInfo{ID: "cache-poisoning", Group: RuleGroupSecurity, Summary: "A cache is restored in a release job or written by a privileged trigger.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-cache-poisoning"},
	)
	registerRuleFactory("cache-poisoning", func(env *RuleEnv) []Rule {
		r := NewRuleCachePoisoning()
		if env.project != nil {
			r.root = env.project.RootDir()
		}
		return []Rule{r}
	})
}

// callerJob completes "on the job" for the metadata of an action, which has no job of its own.
func callerJob(w *Workflow) string {
	if w.Action != nil {
		return " that calls this action"
	}
	return ""
}
