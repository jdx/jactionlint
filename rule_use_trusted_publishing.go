package jactionlint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleUseTrustedPublishing detects packaging workflows that publish with a long-lived credential although the
// registry supports trusted publishing, an OIDC based tokenless authentication.
//
// It reports `run:` steps with a publish command (`twine upload`, `uv publish`, `cargo publish`, `npm publish`,
// `gem push`, `dotnet nuget push`, ...) in a job that cannot request an OIDC token (`id-token: write`), and the
// steps of the well known publishing actions that are given a password or token. Publishing to another registry
// than the public one, and dry runs, are not reported.
type RuleUseTrustedPublishing struct {
	RuleBase
	runContext
}

// NewRuleUseTrustedPublishing creates a new RuleUseTrustedPublishing instance.
func NewRuleUseTrustedPublishing() *RuleUseTrustedPublishing {
	return &RuleUseTrustedPublishing{
		RuleBase: NewRuleBase("use-trusted-publishing", "Checks for publishing with long-lived credentials instead of trusted publishing"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUseTrustedPublishing) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUseTrustedPublishing) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleUseTrustedPublishing) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleUseTrustedPublishing) VisitStep(n *Step) error {
	switch e := n.Exec.(type) {
	case *ExecAction:
		rule.checkAction(e)
	case *ExecRun:
		rule.checkRun(n, e)
	}
	return nil
}

// trustedPublishing says how an ecosystem is published to without a long-lived credential.
var trustedPublishing = map[string]struct{ registry, instead string }{
	"pypi":     {"PyPI", `trusted publishing with pypa/gh-action-pypi-publish and the permission "id-token: write"`},
	"rubygems": {"RubyGems", `trusted publishing with rubygems/release-gem and the permission "id-token: write"`},
	"crates":   {"crates.io", `trusted publishing with rust-lang/crates-io-auth-action and the permission "id-token: write"`},
	"npm":      {"npm", `trusted publishing: configure the trusted publisher of the package on npmjs.com and grant the permission "id-token: write", then no token is needed`},
	"nuget":    {"NuGet", `trusted publishing with NuGet/login and the permission "id-token: write"`},
}

// publishCredentialVars are the environment variables that carry the long-lived credential of a registry.
var publishCredentialVars = map[string][]string{
	"pypi":     {"TWINE_PASSWORD", "UV_PUBLISH_TOKEN", "UV_PUBLISH_PASSWORD", "POETRY_PYPI_TOKEN_PYPI", "POETRY_HTTP_BASIC_PYPI_PASSWORD", "HATCH_INDEX_AUTH", "FLIT_PASSWORD", "MATURIN_PYPI_TOKEN", "PDM_PUBLISH_PASSWORD"},
	"rubygems": {"GEM_HOST_API_KEY", "RUBYGEMS_API_KEY"},
	"crates":   {"CARGO_REGISTRY_TOKEN"},
	"npm":      {"NODE_AUTH_TOKEN", "NPM_TOKEN", "NPM_CONFIG_TOKEN", "YARN_NPM_AUTH_TOKEN"},
	"nuget":    {"NUGET_API_KEY"},
}

// publicRegistries are the values of the registry options that mean the public registry, which is the one that
// supports trusted publishing.
var publicRegistries = []string{
	"pypi", "testpypi", "https://upload.pypi.org/legacy/", "https://test.pypi.org/legacy/",
	"https://registry.npmjs.org", "https://registry.npmjs.org/", "https://rubygems.org", "https://rubygems.org/",
	"nuget.org", "https://api.nuget.org/v3/index.json",
}

func (rule *RuleUseTrustedPublishing) checkRun(step *Step, run *ExecRun) {
	if rule.grantsIDToken() || rule.permissionsComeFromCaller() {
		return
	}
	s, origin := rule.script(run)
	if s == nil {
		return
	}
	for _, c := range s.Commands {
		eco, cmd, ok := publishCommand(c)
		if !ok {
			continue
		}
		info := trustedPublishing[eco]
		cred := ""
		for _, name := range publishCredentialVars[eco] {
			if _, ok := rule.envValue(step, name); ok {
				cred = name
				break
			}
		}
		for _, f := range c.Flags {
			if cred == "" && f.Value != nil && slices.Contains([]string{"--token", "--password", "--api-key", "-p"}, f.Name) && c.Tool != "gh" {
				cred = f.Name
			}
		}
		msg := quote(cmd) + " publishes to " + info.registry
		if cred != "" {
			msg += " with the long-lived credential " + cred
		}
		msg += ". prefer " + info.instead
		start, end := commandRange(s, origin, c)
		rule.errorIDAt("use-trusted-publishing", start, msg).endAt(end)
	}
}

// publishCommand reports whether the command publishes a package to a public registry that supports trusted
// publishing. It returns the ecosystem and the command as written in messages.
func publishCommand(c *runscript.Command) (eco, cmd string, ok bool) {
	if p := c.Publishes(); p != nil {
		if p.Kind != "package" || p.DryRun {
			return "", "", false
		}
		switch p.Tool {
		case "twine", "uv", "poetry", "flit", "hatch":
			eco = "pypi"
		case "cargo":
			eco = "crates"
		case "npm", "pnpm", "bun", "yarn":
			eco = "npm"
		case "gem":
			eco = "rubygems"
		default:
			return "", "", false
		}
		if p.Registry != nil && !registryIsPublic(p.Registry) {
			return "", "", false
		}
		return eco, strings.TrimSpace(p.Tool + " " + p.Verb), true
	}
	// Forms the analyzer does not know as publishes. A dry run and a registry of your own are skipped like they are
	// for the commands it knows. The analyzer does not know the options of these tools, so the words are read here.
	if c.HasFlag("--dry-run") || fallbackRegistryIsPrivate(c) {
		return "", "", false
	}
	pos := func(i int) string { return c.Sub(i) }
	has := func(words ...string) bool {
		for _, w := range words {
			if !slices.ContainsFunc(c.Positional, func(p *runscript.Word) bool { return p.Value == w }) {
				return false
			}
		}
		return true
	}
	switch {
	case c.Name == "pdm" && pos(0) == "publish":
		return "pypi", "pdm publish", true
	case c.Name == "nuget" || c.Name == "nuget.exe":
		if pos(0) == "push" {
			return "nuget", "nuget push", true
		}
	case c.Name == "dotnet" && pos(0) == "nuget" && pos(1) == "push":
		return "nuget", "dotnet nuget push", true
	case c.Name == "bundle" && pos(0) == "exec" && pos(1) == "gem" && pos(2) == "push":
		return "rubygems", "bundle exec gem push", true
	case c.Tool == "uvx" && len(c.Positional) > 0 && strings.HasPrefix(c.Positional[0].Value, "twine") && has("upload"):
		return "pypi", "uvx twine upload", true
	case c.Tool == "uv" && pos(0) == "run" && has("twine", "upload"):
		return "pypi", "uv run twine upload", true
	case c.Tool == "pipx" && pos(0) == "run" && len(c.Positional) > 1 && strings.HasPrefix(c.Positional[1].Value, "twine") && has("upload"):
		return "pypi", "pipx run twine upload", true
	}
	return "", "", false
}

// fallbackRegistryIsPrivate reports whether a command selects a registry other than the public one with
// -r, --repository, --repository-url, -s, --source or -Source, either as the next word or after "=".
func fallbackRegistryIsPrivate(c *runscript.Command) bool {
	names := []string{"-r", "--repository", "--repository-url", "-s", "--source", "-source"}
	for i, a := range c.Args {
		name, value, hasValue := strings.Cut(a.Value, "=")
		if !slices.Contains(names, strings.ToLower(name)) || (a.Dynamic() && !hasValue) {
			continue
		}
		if !hasValue {
			if i+1 >= len(c.Args) {
				continue
			}
			next := c.Args[i+1]
			if next.Dynamic() {
				return true
			}
			value = next.Value
		}
		if !slices.Contains(publicRegistries, value) {
			return true
		}
	}
	return false
}

func registryIsPublic(w *runscript.Word) bool {
	if w.Dynamic() {
		return false
	}
	return slices.Contains(publicRegistries, w.Value)
}

// publishingAction is an action that publishes to a registry which supports trusted publishing.
type publishingAction struct {
	action string
	eco    string
	// manual decides, given the inputs of the step, whether it uses a manually configured credential. It returns
	// the input to point at and what the step does.
	manual func(in func(name string) (string, bool)) (input, what string, ok bool)
}

var trustedPublishingActions = []publishingAction{
	{action: "pypa/gh-action-pypi-publish", eco: "pypi", manual: func(in func(string) (string, bool)) (string, string, bool) {
		if v, ok := in("password"); !ok || v == "" || !defaultOrPublicRepository(in, "repository-url", "repository_url") {
			return "", "", false
		}
		return "password", "is given a password", true
	}},
	{action: "rubygems/configure-rubygems-credentials", eco: "rubygems", manual: func(in func(string) (string, bool)) (string, string, bool) {
		if v, ok := in("api-token"); !ok || v == "" {
			return "", "", false
		}
		if g, ok := in("gem-server"); ok && g != "https://rubygems.org" && g != "https://rubygems.org/" {
			return "", "", false
		}
		return "api-token", "is given an API token", true
	}},
	{action: "rubygems/release-gem", eco: "rubygems", manual: func(in func(string) (string, bool)) (string, string, bool) {
		if v, ok := in("setup-trusted-publisher"); !ok || !strings.EqualFold(v, "false") {
			return "", "", false
		}
		return "setup-trusted-publisher", "is told not to set up trusted publishing", true
	}},
	{action: "actions/setup-node", eco: "npm", manual: func(in func(string) (string, bool)) (string, string, bool) {
		r, ok := in("registry-url")
		if !ok || (r != "https://registry.npmjs.org" && r != "https://registry.npmjs.org/") {
			return "", "", false
		}
		if v, ok := in("always-auth"); !ok || !strings.EqualFold(v, "true") {
			return "", "", false
		}
		return "always-auth", "sends a token with every request to npm", true
	}},
}

func defaultOrPublicRepository(in func(string) (string, bool), names ...string) bool {
	for _, n := range names {
		if v, ok := in(n); ok && !slices.Contains(publicRegistries, v) {
			return false
		}
	}
	return true
}

func (rule *RuleUseTrustedPublishing) checkAction(e *ExecAction) {
	if e.Uses == nil || e.Uses.ContainsExpression() {
		return
	}
	u := ParseUses(e.Uses.Value)
	if u.Kind != UsesAction {
		return
	}
	name := u.CanonicalName()
	for _, a := range trustedPublishingActions {
		if a.action != name {
			continue
		}
		input, what, ok := a.manual(func(n string) (string, bool) { return inputValue(e, n) })
		if !ok {
			return
		}
		pos := e.Uses.Pos
		if i := e.Inputs[input]; i != nil && i.Value != nil {
			pos = i.Value.Pos
		}
		info := trustedPublishing[a.eco]
		rule.errorIDAt("use-trusted-publishing", pos, fmt.Sprintf("action %q publishes to %s but %s (input %q) instead of using trusted publishing. prefer %s", e.Uses.Value, info.registry, what, input, info.instead))
		return
	}
}

func init() {
	registerRules(
		RuleInfo{ID: "use-trusted-publishing", Group: RuleGroupSecurity, Summary: "A package is published with a long-lived credential although the registry supports trusted publishing.", DefaultLevel: SeverityWarning, Profile: ProfileDefault, DocsAnchor: "check-use-trusted-publishing"},
	)
	registerRuleFactory("use-trusted-publishing", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("use-trusted-publishing") {
			return nil
		}
		return []Rule{NewRuleUseTrustedPublishing()}
	})
}
