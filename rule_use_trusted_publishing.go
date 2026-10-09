package jactionlint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

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
	"crates-io", "https://crates.io", "https://crates.io/", "https://github.com/rust-lang/crates.io-index",
	"sparse+https://index.crates.io/", "https://index.crates.io/",
}

func (rule *RuleUseTrustedPublishing) checkRun(step *Step, run *ExecRun) {
	if rule.permissionsComeFromCaller() {
		return
	}
	// "id-token: write" is also asked for provenance or to sign in to a cloud provider, so it does not say that
	// the publish uses it. A job that can request the token but passes a long-lived credential to the publish
	// command is still publishing with the credential (zizmor#1848)
	granted := rule.grantsIDToken()
	s, origin := rule.script(run)
	if s == nil {
		if shell := rule.shellName(run); run != nil && run.Run != nil && (shell == "pwsh" || shell == "powershell") {
			rule.checkPowerShell(step, run, run.Run.scriptOrigin(), granted)
		}
		return
	}
	for _, c := range s.Commands {
		eco, cmd, ok := publishCommand(c)
		if !ok {
			continue
		}
		if eco == "npm" && rule.npmRegistryIsPrivate(step, s, c) {
			continue
		}
		info := trustedPublishing[eco]
		cred := rule.credentialVar(step, eco)
		for _, f := range c.Flags {
			// -p is the password of twine, and the package of cargo (`cargo publish -p crate`)
			if cred == "" && f.Value != nil && slices.Contains([]string{"--token", "--password", "--api-key", "-p"}, f.Name) && c.Tool != "gh" && (f.Name != "-p" || eco == "pypi") && !rule.exchangedToken(f.Value.Exprs) {
				cred = f.Name
			}
		}
		if granted && cred == "" {
			continue
		}
		start, end := commandRange(s, origin, c)
		rule.errorIDAt("use-trusted-publishing", start, trustedPublishingMessage(cmd, info.registry, info.instead, cred, granted)).endAt(end)
	}
}

// npmRegistrySettings are the keys of the configuration of npm, pnpm and yarn that choose the registry.
var npmRegistrySettings = []string{"registry", "npmregistryserver", "npmpublishregistry"}

// npmRegistryIsPrivate reports whether the publish goes to a registry other than the public one although the command
// does not say so. The registry of the publish is decided in the order npm decides it, and the LAST word wins:
//
//  1. the `--registry` of the publish command itself (a public one here: a private one skipped the command already);
//  2. the environment (`npm_config_registry`, `YARN_NPM_REGISTRY_SERVER`, `YARN_NPM_PUBLISH_REGISTRY`) of the step,
//     else of the job, else of the workflow, which wins over every file;
//  3. the files, written in the order the job runs: the `registry-url` of `actions/setup-node` in the steps before
//     this one, then `npm config set registry URL`, `yarn config set npmRegistryServer URL` in this script before
//     the publish. A later one replaces an earlier one, so a reset to the public registry counts; a set that may
//     not run (in an `if`, a loop, after `||`) can leave either one, so it counts as private when either is.
//
// A value that is an expression or otherwise unknown is a private registry. Those registries have no trusted
// publishing.
func (rule *RuleUseTrustedPublishing) npmRegistryIsPrivate(step *Step, s *runscript.Script, publish *runscript.Command) bool {
	for _, f := range publish.Flags {
		if f.Name == "--registry" && f.Value != nil {
			return !registryValueIsPublic(f.Value.Value)
		}
	}
	envPublic := false
	for _, name := range []string{"npm_config_registry", "YARN_NPM_REGISTRY_SERVER", "YARN_NPM_PUBLISH_REGISTRY"} {
		for _, env := range []*Env{step.Env, rule.jobEnv(), rule.workflowEnv()} {
			if env == nil {
				continue
			}
			found := false
			for key, v := range env.Vars {
				if strings.EqualFold(key, name) && v != nil && v.Value != nil {
					found = true
					if !registryValueIsPublic(v.Value.Value) {
						return true
					}
				}
			}
			if found {
				// the nearest scope that sets it decides, and it is public here: a file cannot override the
				// environment of the process
				envPublic = envPublic || rule.registryEnvAppliesTo(name, publish.Tool)
				break
			}
		}
	}
	if envPublic {
		return false
	}
	private := false
	if rule.job != nil {
		for _, st := range rule.job.Steps {
			if st == step {
				break
			}
			e, ok := st.Exec.(*ExecAction)
			if !ok || e.Uses == nil || e.Uses.ContainsExpression() || ParseUses(e.Uses.Value).CanonicalName() != "actions/setup-node" {
				continue
			}
			if r, ok := inputValue(e, "registry-url"); ok && r != "" {
				private = !registryValueIsPublic(r)
			}
		}
	}
	for _, c := range s.Commands {
		if c.Offset >= publish.Offset {
			break
		}
		if (c.Tool != "npm" && c.Tool != "yarn" && c.Tool != "pnpm") || len(c.Positional) < 3 {
			continue
		}
		verb := 0
		if c.Sub(0) == "config" && c.Sub(1) == "set" {
			verb = 2
		} else if c.Sub(0) == "set" {
			verb = 1
		} else {
			continue
		}
		key, val := c.Positional[verb], c.Positional[verb+1:]
		if len(val) == 0 {
			continue
		}
		k := strings.ToLower(key.Value)
		if i := strings.LastIndex(k, ":"); i >= 0 {
			k = k[i+1:] // @scope:registry
		}
		if key.Dynamic() || !slices.Contains(npmRegistrySettings, k) {
			continue
		}
		if now := !registryValueIsPublic(val[0].Value); c.Cond {
			private = private || now
		} else {
			private = now
		}
	}
	return private
}

// registryEnvAppliesTo reports whether the environment variable chooses the registry of the tool: `npm_config_*` is
// read by npm, pnpm and yarn 1, `YARN_*` by yarn.
func (rule *RuleUseTrustedPublishing) registryEnvAppliesTo(name, tool string) bool {
	if strings.HasPrefix(name, "YARN_") {
		return tool == "yarn"
	}
	return tool == "npm" || tool == "pnpm" || tool == "yarn"
}

// registryValueIsPublic reports whether the value is the public registry of npm or empty. An expression is not.
func registryValueIsPublic(v string) bool {
	v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"'`))
	return v == "" || slices.Contains(publicRegistries, v)
}

// credentialVar returns the name of the variable of the step that holds a long-lived credential for the registry of
// the ecosystem, or "". A variable that is set to nothing (setup-node writes a placeholder NODE_AUTH_TOKEN, which
// `NODE_AUTH_TOKEN: ”` blanks so that npm falls back to the OIDC token), and a variable that is set from the output
// of an action that exchanges the OIDC token for a short-lived one (rust-lang/crates-io-auth-action), is not one.
func (rule *RuleUseTrustedPublishing) credentialVar(step *Step, eco string) string {
	for _, name := range publishCredentialVars[eco] {
		v, ok := rule.envValue(step, name)
		if !ok {
			continue
		}
		if strings.TrimSpace(v.Value) == "" || isEmptyExpression(v.Value) {
			continue
		}
		if exprs := expressionsOf(v.Value); len(exprs) > 0 && rule.exchangedToken(exprs) {
			continue
		}
		return name
	}
	return ""
}

// oidcExchangeActions are the actions that trade the OIDC token of the job for a short-lived registry token, which
// they return as an output. A token from one of them is trusted publishing.
var oidcExchangeActions = []string{"rust-lang/crates-io-auth-action", "nuget/login"}

var stepOutputRe = regexp.MustCompile(`(?i)\bsteps\.([a-z_][a-z0-9_-]*)\.outputs\.`)

// exchangedToken reports whether the expressions read the output of a step of the job that runs an action of
// oidcExchangeActions.
func (rule *RuleUseTrustedPublishing) exchangedToken(exprs []string) bool {
	if rule.job == nil || len(exprs) == 0 {
		return false
	}
	for _, expr := range exprs {
		if secretRefRe.MatchString(expr) {
			continue // `secrets.TOKEN || steps.auth.outputs.token` publishes with the secret whenever it is set
		}
		for _, m := range stepOutputRe.FindAllStringSubmatch(expr, -1) {
			for _, st := range rule.job.Steps {
				if st == nil || st.ID == nil || !strings.EqualFold(st.ID.Value, m[1]) {
					continue
				}
				if a, ok := st.Exec.(*ExecAction); ok && a.Uses != nil {
					if slices.Contains(oidcExchangeActions, ParseUses(a.Uses.Value).CanonicalName()) {
						return true
					}
				}
			}
		}
	}
	return false
}

// expressionsOf returns the inner texts of the ${{ }} expressions in the string.
func expressionsOf(s string) []string {
	var out []string
	for {
		i := strings.Index(s, "${{")
		if i < 0 {
			return out
		}
		j := strings.Index(s[i:], "}}")
		if j < 0 {
			return out
		}
		out = append(out, s[i+3:i+j])
		s = s[i+j+2:]
	}
}

// isEmptyExpression reports whether the string is an expression of an empty string literal.
func isEmptyExpression(s string) bool {
	e := expressionsOf(s)
	return len(e) == 1 && strings.TrimSpace(strings.Replace(strings.TrimSpace(s), "${{"+e[0]+"}}", "", 1)) == "" &&
		(strings.TrimSpace(e[0]) == "''" || strings.TrimSpace(e[0]) == `""`)
}

func trustedPublishingMessage(cmd, registry, instead, cred string, granted bool) string {
	msg := quote(cmd) + " publishes to " + registry
	if cred != "" {
		msg += " with the long-lived credential " + cred
	}
	if granted {
		msg += ", although the job can request an OIDC token (\"id-token: write\")"
	}
	return msg + ". prefer " + instead
}

// powerShellPublishes are the commands of the publish tools as they are written in a PowerShell script. The analyzer
// understands bash and sh only, so these are matched on the words of a line.
var powerShellPublishes = []struct {
	eco, cmd string
	re       *regexp.Regexp
}{
	{"crates", "cargo publish", regexp.MustCompile(`(?i)\bcargo\s+publish\b`)},
	{"npm", "npm publish", regexp.MustCompile(`(?i)\b(?:npm|pnpm|bun)\s+publish\b`)},
	{"npm", "yarn npm publish", regexp.MustCompile(`(?i)\byarn\s+npm\s+publish\b`)},
	{"pypi", "twine upload", regexp.MustCompile(`(?i)\btwine\s+upload\b`)},
	{"pypi", "uv publish", regexp.MustCompile(`(?i)\buv\s+publish\b`)},
	{"pypi", "poetry publish", regexp.MustCompile(`(?i)\b(?:poetry|flit|hatch|pdm)\s+publish\b`)},
	{"rubygems", "gem push", regexp.MustCompile(`(?i)\bgem\s+push\b`)},
	{"nuget", "dotnet nuget push", regexp.MustCompile(`(?i)\bdotnet\s+nuget\s+push\b`)},
	{"nuget", "nuget push", regexp.MustCompile(`(?i)\bnuget(?:\.exe)?\s+push\b`)},
}

var (
	powerShellNoPublishRe = regexp.MustCompile(`(?i)(--dry-run|-dryrun|-whatif|--registry|--index-url|--repository-url|--repository\b|-source\b|--source\b|\s-r\s)`)
	powerShellTokenRe     = regexp.MustCompile(`(?i)\s(--token|--password|--api-key|-apikey|-k)[\s=]`)
)

// checkPowerShell reports the publish commands of a PowerShell script. The default shell of the Windows runners is
// pwsh, so the packages that are built on them are published from such a script (zizmor#1848).
func (rule *RuleUseTrustedPublishing) checkPowerShell(step *Step, run *ExecRun, origin runscript.Origin, granted bool) {
	for i, line := range strings.Split(strings.ReplaceAll(run.Run.Value, "\r\n", "\n"), "\n") {
		code := line
		if j := strings.Index(code, "#"); j >= 0 {
			code = code[:j]
		}
		if strings.TrimSpace(code) == "" || powerShellNoPublishRe.MatchString(code) {
			continue
		}
		for _, p := range powerShellPublishes {
			loc := p.re.FindStringIndex(code)
			if loc == nil {
				continue
			}
			cred := rule.credentialVar(step, p.eco)
			if m := powerShellTokenRe.FindStringSubmatch(code); cred == "" && m != nil && !rule.exchangedToken(expressionsOf(code)) {
				cred = strings.ToLower(m[1])
			}
			if granted && cred == "" {
				break
			}
			info := trustedPublishing[p.eco]
			pos := origin.Map(i+1, utf8.RuneCountInString(code[:loc[0]])+1)
			rule.errorIDAt("use-trusted-publishing", &Pos{Line: pos.Line, Col: pos.Col}, trustedPublishingMessage(p.cmd, info.registry, info.instead, cred, granted))
			break
		}
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
	case c.Tool == "uv" && pos(0) == "tool" && pos(1) == "run" && has("twine", "upload"):
		return "pypi", "uv tool run twine upload", true
	case c.Name == "cargo" && (pos(0) == "mono" || pos(0) == "workspaces") && pos(1) == "publish":
		return "crates", "cargo " + pos(0) + " publish", true
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
		RuleInfo{ID: "use-trusted-publishing", Group: RuleGroupSecurity, Summary: "A package is published with a long-lived credential although the registry supports trusted publishing.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-use-trusted-publishing"},
	)
	registerRuleFactory("use-trusted-publishing", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("use-trusted-publishing") {
			return nil
		}
		return []Rule{NewRuleUseTrustedPublishing()}
	})
}
