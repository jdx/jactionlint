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
// github.event_name, github.ref, github.ref_name or github.ref_type. A tag ref in a text does not count: it
// may be compared with a value which is not the ref of the run (github.event.ref in a release).
func looksAtTrigger(v string) bool {
	return strings.Contains(v, "github.event_name") || strings.Contains(v, "github.ref")
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
	{"jdx/aube-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "cache: true" restores node_modules with actions/cache (it also needs "run-install: true"; the action fails without)
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
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
		// The buildx binary is cached in the GitHub Actions cache unless "cache-binary" is false (input of v3), but the
		// action downloads it only when "version" is set or the runner has no buildx. The images of the runners of
		// GitHub have it, so without "version" nothing is restored there (zizmor asks for "version" too).
		if v, ok := a.input("cache-binary"); ok && isFalseLiteral(v) {
			return false, ""
		}
		if v, _ := a.input("version"); strings.TrimSpace(v) == "" {
			return false, ""
		}
		if major, ok := majorOf(u); ok && major < 3 {
			return false, ""
		}
		return true, "set \"cache-binary: false\""
	}},
	{"oven-sh/setup-bun", func(a *ExecAction, u *UsesRef) (bool, string) {
		// the downloaded bun executable is cached unless "no-cache" is true
		if v, _ := a.input("no-cache"); isTrueLiteral(v) {
			return false, ""
		}
		return true, "set \"no-cache: true\""
	}},
	{"actions-rust-lang/setup-rust-toolchain", func(a *ExecAction, u *UsesRef) (bool, string) {
		// it runs Swatinem/rust-cache unless "cache" is false
		if v, ok := a.input("cache"); ok && isFalseLiteral(v) {
			return false, ""
		}
		return true, "set \"cache: false\""
	}},
	{"pnpm/action-setup", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "cache: true" caches the pnpm store (the input is "false" by default)
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
	}},
	{"pnpm/setup", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "cache: true" restores the pnpm store, keyed on the lock file. "save-cache: false" only stops the saving.
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
	}},
	{"graalvm/setup-graalvm", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "cache" names the build tool ("maven", "gradle" or "sbt") whose dependencies are cached
		_, ok := inputEnabled(a, "cache")
		return ok, "remove the \"cache\" input"
	}},
	{"ramsey/composer-install", func(a *ExecAction, u *UsesRef) (bool, string) {
		// the Composer cache is restored unless "ignore-cache" says so (see ramsey/composer-install bin/should_cache.sh)
		if v, ok := a.input("ignore-cache"); ok && composerIgnoresCache(v) {
			return false, ""
		}
		return true, "set \"ignore-cache: true\""
	}},
	{"mlugg/setup-zig", func(a *ExecAction, u *UsesRef) (bool, string) {
		if v, ok := a.input("use-cache"); ok && isFalseLiteral(v) {
			return false, ""
		}
		return true, "set \"use-cache: false\""
	}},
	{"pyo3/maturin-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		// "sccache: true" (or "cache: sccache") makes the action set SCCACHE_GHA_ENABLED, which makes sccache
		// restore its compiler outputs from the GitHub Actions cache
		if _, ok := inputEnabled(a, "sccache"); ok {
			return true, "remove the \"sccache\" input"
		}
		if v, ok := a.input("cache"); ok && strings.EqualFold(strings.TrimSpace(v), "sccache") {
			return true, "remove the \"cache\" input"
		}
		return false, ""
	}},
	{"mozilla-actions/sccache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		// the action only installs sccache; cacheEnvSwitch says what makes sccache use the GitHub Actions cache
		return true, "do not set SCCACHE_GHA_ENABLED"
	}},
	{"awalsh128/cache-apt-pkgs-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
	{"nix-community/cache-nix-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
	{"hendrikmuhs/ccache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
	{"determinatesystems/magic-nix-cache-action", func(a *ExecAction, u *UsesRef) (bool, string) {
		return true, "remove this step"
	}},
}

// composerIgnoresCache reports whether the value of the "ignore-cache" input of ramsey/composer-install turns its cache
// off. The action compares the text with 1, yes, Yes, y, Y, true and True; any other value (an expression included, which
// is not known) leaves the cache on.
func composerIgnoresCache(v string) bool {
	switch strings.TrimSpace(v) {
	case "1", "yes", "Yes", "y", "Y", "true", "True":
		return true
	}
	return false
}

// buildxMayDownload is the pedantic reading of docker/setup-buildx-action: without "version" the action downloads
// buildx, and restores it from the cache, only on a runner that has none. The runners of GitHub have it, a
// self-hosted runner may not, and the rule does not know which one the job runs on.
func buildxMayDownload(a *ExecAction, u *UsesRef) (bool, string) {
	if v, ok := a.input("cache-binary"); ok && isFalseLiteral(v) {
		return false, ""
	}
	if major, ok := majorOf(u); ok && major < 3 {
		return false, ""
	}
	return true, "set \"cache-binary: false\""
}

// cacheEnvSwitch lists the actions which restore a cache only when an environment variable says so (of the step, the
// job or the workflow, or written to GITHUB_ENV by a script of the job): sccache uses the GitHub Actions cache when
// SCCACHE_GHA_ENABLED is true, and without it the action only installs the binary. A value of false switches it off;
// a value that is not known (an expression) keeps the finding, as does a script of the job that mentions the
// variable.
var cacheEnvSwitch = map[string]string{
	"mozilla-actions/sccache-action": "SCCACHE_GHA_ENABLED",
}

// envVarState tells what the environment says about the variable: whether it sets it, and if so whether the value
// is a false one.
func envVarState(e *Env, name string) (set, on bool) {
	if e == nil {
		return false, false
	}
	for _, v := range e.Vars {
		if v == nil || v.Name == nil || !strings.EqualFold(v.Name.Value, name) {
			continue
		}
		val := ""
		if v.Value != nil {
			val = strings.TrimSpace(v.Value.Value)
		}
		return true, val != "" && !isFalseLiteral(val) && val != "0"
	}
	return false, false
}

// Invariants of the checks of this file that were added for the findings of zizmor. A new case is reported only when
// it is certain (no finding when unsure): the environment of a step is resolved by precedence (step, job, workflow; see
// cacheEnvOn), a condition is evaluated for the runs that publish with gateAllowsRun and the scenarios of the rule
// (feedsPublishingJob uses it for the upload and for the jobs that need it), and the version of an action pinned to a
// commit is the one of its comment (withCommentVersion) in every check.

// cacheEnvOn reports whether the variable that switches the cache of an action on is set for the step. The most
// specific value decides, as it does for the runner: the env of the step, then the env of the job, then the env of the
// workflow. A script of the job that mentions the variable (it writes it to GITHUB_ENV) keeps the finding unless the
// step sets it. In a composite action the variable may come from the workflow that calls it, which is not known
// here, so the finding stays unless the step switches it off.
func (rule *RuleCachePoisoning) cacheEnvOn(j *Job, s *Step, name string) bool {
	if set, on := envVarState(s.Env, name); set {
		return on
	}
	for _, st := range flattenSteps(j.Steps) {
		if r, ok := st.Exec.(*ExecRun); ok && r.Run != nil && strings.Contains(r.Run.Value, name) {
			return true
		}
	}
	if set, on := envVarState(j.Env, name); set {
		return on
	}
	if set, on := envVarState(rule.wf.Env, name); set {
		return on
	}
	return rule.wf.Action != nil // the caller may set it, which is not known: the finding stays
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
	"googleapis/release-please-action",
	"google-github-actions/release-please-action",
	"cycjimmy/semantic-release-action",
	"marvinpinto/action-automatic-releases",
	"helm/chart-releaser-action",
	"getsentry/action-release",
	"jasonetco/build-and-tag-action",
	"actions/publish-action",
	"vedantmgoyal9/winget-releaser",
	"tauri-apps/tauri-action",
	"changesets/action",
	"crazy-max/ghaction-github-release",
	"haaleo/publish-vscode-extension",
	"elgohr/publish-docker-github-action",
	"rust-lang/crates-io-auth-action",
	"redhat-actions/push-to-registry",
	"taiki-e/upload-rust-binary-action",
	"taiki-e/create-gh-release-action",
	"shogo82148/actions-upload-release-asset",
	"mislav/bump-homebrew-formula-action",
	"dawidd6/action-homebrew-bump-formula",
	"azure/webapps-deploy",
	"azure/functions-action",
	"azure/static-web-apps-deploy",
	"google-github-actions/deploy-cloudrun",
	"google-github-actions/deploy-appengine",
	"aws-actions/amazon-ecs-deploy-task-definition",
	"amondnet/vercel-action",
	"nwtgck/actions-netlify",
	"r0adkll/upload-google-play",
	"apple-actions/upload-testflight-build",
	"jreleaser/release-action",
	"release-drafter/release-drafter",
	"aws-actions/aws-cloudformation-github-deploy",
	"azure/aci-deploy",
	"azure/container-apps-deploy-action",
	"azure/sql-action",
	"google-github-actions/deploy-cloud-functions",
}

// publishCommandRegex matches a command of a `run:` script that publishes a release or a package.
var publishCommandRegex = regexp.MustCompile(`(?m)\b(cargo\s+publish|npm\s+publish|pnpm\s+publish|aube\s+publish|yarn\s+(npm\s+)?publish|cargo\s+(mono|workspaces)\s+publish|(?:pnpm|npx)\s+(?:-r\s+)?changeset\s+publish|lerna\s+publish|semantic-release|ovsx\s+publish|dotnet\s+nuget\s+push|(?:dart|flutter)\s+pub\s+publish|helm\s+push|oras\s+push|twine\s+upload|gem\s+push|poetry\s+publish|uv\s+publish|docker\s+push|gh\s+release\s+(create|upload)|goreleaser\s+(release|publish)|vsce\s+publish|mvn\b[^\n]*\bdeploy|gradle\w*\s+[^\n]*\bpublish)\b`)

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
	tagOnly    bool   // the workflow is a release workflow only because it runs on pushed tags or release branches (no release event)
	releaseWhy string // why the workflow is a release workflow, or ""
	privileged string // the privileged trigger of the workflow, or ""
	loose      bool   // the pedantic checks run: the heuristics of zizmor for what is a release
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
func releaseTrigger(events []Event, loose bool) (why string, scenarios []triggerScenario) {
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
			scenarios = append(scenarios, tagPushScenarios(ev.Tags)...)
		}
		if e.EventName() == "push" && (releaseBranchFilter(ev.Branches) || loose && namesReleaseBranch(ev.Branches)) {
			if why == "" {
				why = "runs on pushes to release branches"
			}
			scenarios = append(scenarios, scenarioBranchPush)
		}
	}
	return why, scenarios
}

// releaseBranchRe matches the name of a branch that is made to release: "release", "releases/**", "release/v1.x",
// "release-*". A name that only starts with the word ("releaser", "release_notes") does not count.
var releaseBranchRe = regexp.MustCompile(`^releases?(?:[/-].*)?$`)

// releaseBranchFilter reports whether a `branches` filter names release branches and nothing else: every positive
// pattern is one (see releaseBranchRe). A filter that also lists main or a wildcard is the filter of the ordinary
// checks, which run on the release branches as well as on the others, so it does not make a release workflow.
func releaseBranchFilter(f *WebhookEventFilter) bool {
	if f.IsEmpty() {
		return false
	}
	positive := 0
	for _, v := range f.Values {
		if v == nil {
			continue
		}
		pat := strings.ToLower(strings.TrimSpace(v.Value))
		if strings.HasPrefix(pat, "!") {
			continue
		}
		if !releaseBranchRe.MatchString(pat) {
			return false
		}
		positive++
	}
	return positive > 0
}

// namesReleaseBranch reports whether a `branches` filter has a positive pattern that names a release branch,
// whatever else it lists (`branches: [main, 'release/**']`). zizmor takes that for a release trigger, which it is when a
// release is made from the branch and not when it is the filter of the ordinary checks, so the rule asks for it
// with the option pedantic only.
func namesReleaseBranch(f *WebhookEventFilter) bool {
	if f.IsEmpty() {
		return false
	}
	for _, v := range f.Values {
		if v == nil {
			continue
		}
		pat := strings.ToLower(strings.TrimSpace(v.Value))
		if !strings.HasPrefix(pat, "!") && strings.Contains(pat, "release") {
			return true
		}
	}
	return false
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

// eventScenarios lists one run per event. A push event runs for a branch push when it has a branch filter or
// no ref filter at all, and for a tag push when it has a tag filter (which lets the tag through) or no ref
// filter at all: a filter on one kind of ref keeps the other away. A run for a branch push is not a tag ref, a run
// for a tag push is; a bare "push" run would let a condition on the ref pass as if a branch were pushed.
func eventScenarios(events []Event) []triggerScenario {
	var ret []triggerScenario
	for _, e := range events {
		ev, ok := e.(*WebhookEvent)
		if !ok || e.EventName() != "push" {
			ret = append(ret, triggerScenario{"event_name": e.EventName()})
			continue
		}
		branchFilter := ev.Branches != nil || ev.BranchesIgnore != nil
		tagFilter := ev.Tags != nil || ev.TagsIgnore != nil
		if branchFilter || !tagFilter {
			ret = append(ret, triggerScenario{"event_name": "push", refPrefixKey: "refs/heads/"})
		}
		if !branchFilter && !tagFilter || tagFilter && (ev.Tags == nil || tagFilterMatches(ev.Tags)) {
			ret = append(ret, tagPushScenarios(ev.Tags)...)
		}
	}
	return ret
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleCachePoisoning) VisitWorkflowPre(n *Workflow) error {
	rule.wf = n
	rule.releaseWhy, rule.privileged, rule.tagOnly = "", "", false
	rule.scenarios, rule.eventScenarios = nil, nil
	rule.loose = rule.pedantic("cache-poisoning")
	for _, e := range n.On {
		if name := e.EventName(); slices.Contains(privilegedTriggers, name) && rule.privileged == "" {
			rule.privileged = name
		}
	}
	if n.Action == nil {
		if why, sc := releaseTrigger(n.On, rule.loose); why != "" {
			rule.releaseWhy, rule.scenarios = "the workflow "+why, sc
			rule.tagOnly = !slices.ContainsFunc(sc, func(s triggerScenario) bool { return s["event_name"] == "release" })
		}
		rule.eventScenarios = eventScenarios(n.On)
	} else if c := n.Action.Callers; c.Known() {
		// The action runs in the context of the workflow which calls it. A caller which releases is enough:
		// a cache restored there ends up in the published artifacts.
		for _, cl := range c.Callers {
			if why, sc := releaseTrigger(cl.Events, rule.loose); why != "" {
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
// an environment, and a job that reads a secret other than GITHUB_TOKEN may have a credential. A job that runs a publishing command or action is judged by publishingReason. Without
// `permissions:` the token is whatever the repository sets, which is not known, so the job may publish.
func (rule *RuleCachePoisoning) cannotPublish(j *Job) bool {
	if j.Environment != nil || j.WorkflowCall != nil {
		return false
	}
	if rule.usesOwnSecret(j) {
		return false // a secret other than the token of the workflow can be the credential to publish with
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

// publishScopes are the permissions with which a job can publish something: it can write to the repository (a release,
// a tag), push a package, sign an attestation, deploy pages or a deployment, or ask for an OIDC token (trusted
// publishing). Writing issues, pull requests or checks is not publishing.
var publishScopes = []string{"contents", "packages", "attestations", "pages", "deployments", "id-token"}

// canPublish reports whether a job can publish: it has an environment (what deployments are guarded with), grants a
// write permission of publishScopes (the job's own, or the workflow's when it has none), or runs a publishing action or
// command. A secret alone does not count here, a job that notifies a chat uses one.
func (rule *RuleCachePoisoning) canPublish(j *Job) bool {
	if j.Environment != nil || publishingReason(flattenSteps(j.Steps)) != "" {
		return true
	}
	p := j.Permissions
	if p == nil {
		p = rule.wf.Permissions
	}
	if p == nil {
		return false
	}
	if p.All != nil {
		return strings.EqualFold(strings.TrimSpace(p.All.Value), "write-all")
	}
	for _, sc := range p.Scopes {
		if sc != nil && sc.Name != nil && sc.Value != nil && strings.EqualFold(strings.TrimSpace(sc.Value.Value), "write") &&
			slices.Contains(publishScopes, strings.ToLower(strings.TrimSpace(sc.Name.Value))) {
			return true
		}
	}
	return false
}

// uploadsArtifact reports whether the job uploads an artifact in the runs that publish: it hands files to the jobs
// that need it. An upload with an `if:` that keeps those runs away does not.
func uploadsArtifact(j *Job, scenarios []triggerScenario) bool {
	return slices.ContainsFunc(flattenSteps(j.Steps), func(s *Step) bool {
		a, ref := stepAction(s)
		return a != nil && ref.Kind == UsesAction && strings.HasPrefix(ref.CanonicalName(), "actions/upload-artifact") &&
			(s.If == nil || gateAllowsRun(s.If.Value, scenarios))
	})
}

// feedsPublishingJob reports whether the job j hands artifacts to a job that can publish: j uploads an artifact and
// a job that can publish depends on it, directly or through other jobs (`needs`). Then j is not only a check that
// the permissions leave without a way to publish: the files that it builds are what the other job publishes, so a
// poisoned cache in j ends up in the release. A job that only has to pass before the release (a test job) does not
// hand over anything, and is not reported.
func (rule *RuleCachePoisoning) feedsPublishingJob(j *Job, scenarios []triggerScenario) bool {
	if j.ID == nil || rule.wf == nil || !uploadsArtifact(j, scenarios) {
		return false
	}
	// Invariant. Every job has a set of possible results among success, failure, cancelled and skipped. The set of j
	// is the three that ran. The set of any other job comes from its `if:` evaluated over the sets of ITS needs
	// (recursively, memoised): {skipped} when the condition is false for every choice of their results, the three that
	// ran when it is true for every choice, all four when it is open (or the job is not known). One evaluation of a
	// condition uses ONE choice of the results of the needed jobs, from which success(), failure(), cancelled() and
	// needs.<job>.result all follow. A publisher counts when its condition is not false for some choice (unknown means
	// possibly publishing); a condition that is false whatever the results are is settled first, and past a bound on
	// the choices the job counts as possibly running.
	root := strings.ToLower(j.ID.Value)
	jobs := map[string]*Job{}
	var ids []string
	for id, dep := range rule.wf.Jobs {
		if dep != nil && strings.ToLower(id) != root {
			jobs[strings.ToLower(id)] = dep
			ids = append(ids, strings.ToLower(id))
		}
	}
	slices.Sort(ids)
	needsOf := func(dep *Job) (ret []string) {
		for _, n := range dep.Needs {
			if n != nil {
				ret = append(ret, strings.ToLower(n.Value))
			}
		}
		return ret
	}
	reach := map[string]bool{root: true}
	for grew := true; grew; {
		grew = false
		for _, id := range ids {
			if !reach[id] && slices.ContainsFunc(needsOf(jobs[id]), func(n string) bool { return reach[n] }) {
				reach[id], grew = true, true
			}
		}
	}
	all := []string{"success", "failure", "cancelled", "skipped"}
	ran := all[:3]
	memo := map[string][]string{root: ran}
	var results func(id string) []string
	results = func(id string) []string {
		if r, ok := memo[id]; ok {
			return r
		}
		memo[id] = all // a cycle, while it is worked out
		job := jobs[id]
		if job == nil {
			return all
		}
		sets := map[string][]string{}
		for _, n := range needsOf(job) {
			sets[n] = results(n)
		}
		canRun, canSkip := runsWithNeeds(jobIf(job), scenarios, sets)
		switch {
		case !canRun:
			memo[id] = []string{"skipped"}
		case !canSkip:
			memo[id] = ran
		}
		return memo[id]
	}
	for _, id := range ids {
		if reach[id] && rule.canPublish(jobs[id]) {
			if canRun, _ := runsWithNeeds(jobIf(jobs[id]), scenarios, setsOf(needsOf(jobs[id]), results)); canRun {
				return true
			}
		}
	}
	return false
}

func jobIf(j *Job) string {
	if j.If != nil {
		return j.If.Value
	}
	return ""
}

func setsOf(needs []string, results func(string) []string) map[string][]string {
	sets := map[string][]string{}
	for _, n := range needs {
		sets[n] = results(n)
	}
	return sets
}

// runsWithNeeds evaluates the condition of a job (empty for none) over the possible results of the jobs that it needs
// (see the invariant in feedsPublishingJob) and tells whether the job can run and whether it can be skipped. Without a
// status function success() is added to the condition. The needed jobs that the condition names (needs.<job>.result)
// are tried one by one; the others only matter through success(), failure() and cancelled(), which they can each
// make true or false.
func runsWithNeeds(cond string, scenarios []triggerScenario, sets map[string][]string) (canRun, canSkip bool) {
	var e ExprNode
	if cond != "" {
		if e = parseGateExpression(cond); e == nil {
			return true, true
		}
	}
	hasStatus := e != nil && exprCallsStatus(e)
	var named []string
	if e != nil {
		named = namedNeedResults(e)
	}
	var tried, rest []string
	for n := range sets {
		if slices.Contains(named, n) {
			tried = append(tried, n)
		} else {
			rest = append(rest, n)
		}
	}
	slices.Sort(tried)
	has := func(n, r string) bool { return slices.Contains(sets[n], r) }
	canFail, canCancel, allSucceed, anyOther := false, false, true, false
	for _, n := range rest {
		canFail, canCancel = canFail || has(n, "failure"), canCancel || has(n, "cancelled")
		allSucceed = allSucceed && has(n, "success")
		anyOther = anyOther || has(n, "failure") || has(n, "cancelled") || has(n, "skipped")
	}
	// the choices for the jobs that are not named: success() (all succeeded, or not), failure() and cancelled()
	type flags struct{ success, failure, cancelled bool }
	var restChoices []flags
	// one needed job can fail while another is cancelled, so both can be true when two different jobs can do each
	both := false
	for _, a := range rest {
		for _, b := range rest {
			both = both || a != b && has(a, "failure") && has(b, "cancelled")
		}
	}
	for _, f := range []flags{{true, false, false}, {false, false, false}, {false, true, false}, {false, false, true}, {false, true, true}} {
		if f.success && !allSucceed || f.failure && !canFail || f.cancelled && !canCancel || f.failure && f.cancelled && !both || !f.success && !f.failure && !f.cancelled && !anyOther {
			continue
		}
		restChoices = append(restChoices, f)
	}
	if len(restChoices) == 0 {
		return true, true
	}
	const maxChoices = 1 << 14
	choices := len(restChoices)
	for _, n := range tried {
		if choices *= len(sets[n]); choices > maxChoices {
			choices = -1
			break
		}
	}
	for _, sc := range scenarios {
		// a condition that is false whatever the results are is settled first, with them left unknown
		cp := triggerScenario{"status.success": "?", "status.failure": "?", "status.cancelled": "?"}
		for k, v := range sc {
			cp[k] = v
		}
		if e != nil && evalGate(e, cp).truth() == triFalse {
			canSkip = true
			continue
		}
		if choices < 0 {
			return true, true
		}
		combos := choices / len(restChoices)
		for c := 0; c < combos; c++ {
			for _, rc := range restChoices {
				cp := triggerScenario{}
				for k, v := range sc {
					cp[k] = v
				}
				success, failed, cancelled := rc.success, rc.failure, rc.cancelled
				left := c
				for _, n := range tried {
					res := sets[n][left%len(sets[n])]
					left /= len(sets[n])
					cp["needs."+n+".result"] = res
					success, failed, cancelled = success && res == "success", failed || res == "failure", cancelled || res == "cancelled"
				}
				cp["status.success"], cp["status.failure"], cp["status.cancelled"] = strconv.FormatBool(success), strconv.FormatBool(failed), strconv.FormatBool(cancelled)
				v := triTrue
				if e != nil {
					v = evalGate(e, cp).truth()
				}
				if !hasStatus && !success {
					v = triFalse
				}
				canRun, canSkip = canRun || v != triFalse, canSkip || v != triTrue
				if canRun && canSkip {
					return true, true
				}
			}
		}
	}
	return canRun, canSkip
}

// namedNeedResults lists the jobs whose result the expression reads (needs.<job>.result), in lower case.
func namedNeedResults(e ExprNode) []string {
	var ret []string
	VisitExprNode(e, func(n, _ ExprNode, entering bool) {
		d, ok := n.(*ObjectDerefNode)
		if !ok || !entering || d.Property != "result" {
			return
		}
		if r, ok := d.Receiver.(*ObjectDerefNode); ok {
			if v, ok := r.Receiver.(*VariableNode); ok && v.Name == "needs" {
				if name := strings.ToLower(r.Property); !slices.Contains(ret, name) {
					ret = append(ret, name)
				}
			}
		}
	})
	return ret
}

// exprCallsStatus reports whether the expression calls success(), failure(), cancelled() or always(), which
// stops GitHub from adding success() to the condition of a job.
func exprCallsStatus(e ExprNode) bool {
	found := false
	VisitExprNode(e, func(n, _ ExprNode, entering bool) {
		if f, ok := n.(*FuncCallNode); ok && entering {
			switch strings.ToLower(f.Callee) {
			case "success", "failure", "cancelled", "always":
				found = true
			}
		}
	})
	return found
}

// secretRefRe finds a read of a secret: secrets.NAME (group 1), secrets['NAME'] (group 2), or any other form
// (secrets[expr], a bare secrets as an argument), which leave both groups empty.
var secretRefRe = regexp.MustCompile(`(?i)\bsecrets(?:\.([a-z_][a-z0-9_-]*)|\s*\[\s*'([^']*)'\s*\]|\s*\[|\b\s*\))`)

// usesOwnSecret reports whether the job, or the env: of the workflow, reads a secret other than GITHUB_TOKEN.
func (rule *RuleCachePoisoning) usesOwnSecret(j *Job) bool {
	has := func(s *String) bool {
		if s == nil || !strings.Contains(s.Value, "secrets") {
			return false
		}
		for _, m := range secretRefRe.FindAllStringSubmatch(s.Value, -1) {
			if name := m[1] + m[2]; !strings.EqualFold(name, "github_token") {
				return true
			}
		}
		return false
	}
	env := func(e *Env) bool {
		if e == nil {
			return false
		}
		if has(e.Expression) {
			return true
		}
		for _, v := range e.Vars {
			if v != nil && has(v.Value) {
				return true
			}
		}
		return false
	}
	if env(rule.wf.Env) || env(j.Env) {
		return true
	}
	for _, s := range j.Steps {
		if s == nil || env(s.Env) {
			return true
		}
		switch e := s.Exec.(type) {
		case *ExecRun:
			if has(e.Run) {
				return true
			}
		case *ExecAction:
			for _, in := range e.Inputs {
				if in != nil && has(in.Value) {
					return true
				}
			}
		}
	}
	return false
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleCachePoisoning) VisitJobPre(n *Job) error {
	if !rule.Config().RuleEnabled("cache-poisoning") {
		return nil
	}
	if rule.privileged != "" {
		rule.checkWrite(n.CacheMode)
	}
	if rule.loose {
		rule.checkCredentialCaches(n)
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
	if why != "" && rule.tagOnly && !rule.loose && rule.cannotPublish(n) && !rule.feedsPublishingJob(n, scenarios) {
		why = "" // a tag only starts a check here, see cannotPublish
	}
	if why == "" {
		// a publishing job in a workflow which is not a release workflow
		if why = publishingReason(steps); why == "" {
			return nil
		}
		// the tag push of a tag-only workflow is a run too, and the only one that has a tag as its ref
		scenarios = append(slices.Clone(rule.eventScenarios), rule.scenarios...)
	}
	for _, s := range steps {
		a, ref := stepAction(s)
		if a == nil || ref.Kind != UsesAction || a.InputsUnknown() {
			// the inputs which turn the cache off may be in a "with:" that is an expression
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
		if !reads && rule.loose && name == "docker/setup-buildx-action" {
			reads, hint = buildxMayDownload(a, withCommentVersion(rule.wf, a, ref))
		}
		if !reads || !cacheCanRunOnReleaseTrigger(n, s, a, name, scenarios) || rule.callersTurnCacheOff(a, name) {
			continue
		}
		if v := cacheEnvSwitch[name]; v != "" && !rule.cacheEnvOn(n, s, v) {
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

var inputRefRe = regexp.MustCompile(`^\$\{\{\s*inputs\.([A-Za-z_][A-Za-z0-9_-]*)\s*\}\}$`)

// callersTurnCacheOff reports whether the step of a composite action restores a cache only when the input of the
// action says so, and every release workflow that calls the action sets that input to switch the cache off:
// `enable-cache: false` of setup-uv passed down as `enable-cache: ${{ inputs.enable-cache }}`. Without a call that
// is known to pass the value (a call through another action, or a caller that is not known), the answer is no.
func (rule *RuleCachePoisoning) callersTurnCacheOff(a *ExecAction, name string) bool {
	if rule.wf == nil || rule.wf.Action == nil || !rule.wf.Action.Callers.Known() {
		return false
	}
	for _, g := range cacheGateInputs[name] {
		v, ok := a.input(g.input)
		if !ok || gateInputIsRedundant(name, a, g.input, nil) {
			continue
		}
		m := inputRefRe.FindStringSubmatch(strings.TrimSpace(v))
		if m == nil {
			continue
		}
		if rule.everyReleaseCallerSets(m[1], g.offWhenTrue) {
			return true
		}
	}
	return false
}

// everyReleaseCallerSets reports whether every release workflow that runs the action passes the input with a
// value that switches the cache off (the literal false, or true for an input that switches it off when true, or
// an expression that evaluates to it in every scenario in which the caller publishes), or leaves the input out and
// the default of the action switches it off. An expression which cannot be evaluated keeps the finding.
func (rule *RuleCachePoisoning) everyReleaseCallerSets(input string, offWhenTrue bool) bool {
	// off reports whether the value passed by a caller switches the cache off in every run of the caller that
	// publishes. A literal is decided as is; an expression is evaluated for each scenario with the evaluator
	// of the conditions (cache_gate.go), and what it cannot decide counts as not off.
	off := func(v string, scenarios []triggerScenario) bool {
		v = strings.TrimSpace(v)
		if offWhenTrue && isTrueLiteral(v) || !offWhenTrue && isFalseLiteral(v) {
			return true
		}
		if !strings.Contains(v, "${{") || len(scenarios) == 0 {
			return false
		}
		e := parseGateExpression(v)
		if e == nil {
			return false
		}
		for _, sc := range scenarios {
			if !valueSwitchesOff(evalGate(e, sc), offWhenTrue) {
				return false
			}
		}
		return true
	}
	def, hasDefault := "", false
	for _, in := range rule.wf.Action.Inputs {
		if in != nil && in.ID != nil && strings.EqualFold(in.ID.Value, input) && in.Default != nil {
			def, hasDefault = in.Default.Value, true
		}
	}
	releasing := 0
	for _, cl := range rule.wf.Action.Callers.Callers {
		why, scenarios := releaseTrigger(cl.Events, rule.loose)
		if why == "" {
			continue
		}
		releasing++
		if len(cl.calls) == 0 {
			return false
		}
		for _, call := range cl.calls {
			v, ok := call.input(input)
			switch {
			case ok && off(v, scenarios):
			case !ok && hasDefault && off(def, nil):
			default:
				return false
			}
		}
	}
	return releasing > 0
}

// valueSwitchesOff reports whether the evaluated value of an input switches the cache off: a boolean, or the
// string a boolean becomes ("false", or "true" for an input that switches it off when true). An empty string
// switches off the inputs that are on when set. A value that is not known never does.
func valueSwitchesOff(v gateValue, offWhenTrue bool) bool {
	switch v.kind {
	case 1:
		return v.b == offWhenTrue
	case 2:
		if offWhenTrue {
			return isTrueLiteral(v.s)
		}
		return v.s == "" || isFalseLiteral(v.s)
	}
	return false
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
	// "cache: true" restores node_modules, and is off by default
	"jdx/aube-action": {{input: "cache"}},
	// "cache-disabled: true" turns off all caching of the action
	"gradle/actions/setup-gradle": {{input: "cache-disabled", offWhenTrue: true}},
	"gradle/gradle-build-action":  {{input: "cache-disabled", offWhenTrue: true}},
	// "cache-from" with type=gha is what restores the cache; empty means none
	"docker/build-push-action": {{input: "cache-from"}},
	// the next ones have no switch: only the "if:" of the step can gate them (actions/cache has "lookup-only",
	// which `reads` handles as a literal)
	"pyo3/maturin-action":                       {{input: "sccache"}},
	"mozilla-actions/sccache-action":            nil,
	"actions/cache":                             nil,
	"actions/cache/restore":                     nil,
	"hendrikmuhs/ccache-action":                 nil,
	"awalsh128/cache-apt-pkgs-action":           nil,
	"nix-community/cache-nix-action":            nil,
	"oven-sh/setup-bun":                         {{input: "no-cache", offWhenTrue: true}},
	"actions-rust-lang/setup-rust-toolchain":    {{input: "cache"}},
	"mlugg/setup-zig":                           {{input: "use-cache"}},
	"pnpm/setup":                                {{input: "cache"}},
	"pnpm/action-setup":                         {{input: "cache"}},
	"graalvm/setup-graalvm":                     {{input: "cache"}},
	"ramsey/composer-install":                   {{input: "ignore-cache", offWhenTrue: true}},
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
			if u.Kind == UsesAction && name == "docker/build-push-action" && buildPushPushes(e) {
				return "the job pushes an image"
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

// buildPushPushes reports whether a use of docker/build-push-action may push the image. A non-empty "outputs" is what
// buildx receives, and the shorthand "push" is not applied to it, so "outputs" decides alone when it is set: it pushes
// with "type=registry" or "push=true" (as in "type=image,push-by-digest=true,push=true"), and "push: false" does not
// cancel that, nor does "push: true" make a local export push. Without "outputs", "push" decides: anything but a false
// value pushes (an expression such as "${{ inputs.push }}" is not known to be false), and the default is false.
func buildPushPushes(a *ExecAction) bool {
	if out, _ := a.input("outputs"); strings.TrimSpace(out) != "" {
		return buildOutputPushRe.MatchString(out) || buildOutputMayChange(out)
	}
	v, _ := a.input("push")
	v = strings.TrimSpace(v)
	return v != "" && !isFalseLiteral(v)
}

// outputExprRe finds an expression in the outputs of a build.
// It spans lines, as an expression in a block scalar may.
var outputExprRe = regexp.MustCompile(`(?s)\$\{\{.*?\}\}`)

// buildOutputMayChange reports whether an expression in "outputs" can change what the output does: it is the whole
// field, or in a key, or in the value of "type" or "push" (it may resolve to the registry, or to nothing, and then
// "push" applies). An expression in another value, as in "type=local,dest=${{ runner.temp }}", only supplies a path
// or a name.
func buildOutputMayChange(out string) bool {
	if !strings.Contains(out, "${{") {
		return false
	}
	out = outputExprRe.ReplaceAllString(out, "\x00")
	for _, f := range strings.FieldsFunc(out, func(r rune) bool { return r == ',' || r == '\n' }) {
		f = strings.Trim(strings.TrimSpace(f), `"'`)
		if !strings.Contains(f, "\x00") {
			continue
		}
		key, val, ok := strings.Cut(f, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !ok || strings.Contains(key, "\x00") || (key == "type" || key == "push") && strings.Contains(val, "\x00") {
			return true
		}
	}
	return false
}

// buildOutputPushRe finds the options of the "outputs" of docker/build-push-action which push the image.
var buildOutputPushRe = regexp.MustCompile(`(?i)(?:^|[\s,"'])(?:type=registry|push=true)(?:[\s,"']|$)`)

func init() {
	registerRules(
		RuleInfo{ID: "cache-poisoning", Group: RuleGroupSecurity, Summary: "A cache is restored in a release job or written by a privileged trigger. With the option pedantic, a tag that only starts checks and a branch filter that names a release branch among others count as releases too, and a cache that saves a file with credentials is reported.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-cache-poisoning", Options: []RuleOption{pedanticOption}},
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
