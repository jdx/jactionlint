package jactionlint

import (
	"slices"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// The pedantic findings of use-trusted-publishing are about how a job that does use trusted publishing is built
// (zizmor#1968, #237 and #938). Trusted publishing replaces a stored token with an OIDC token that the job asks
// for, so whatever runs in the job while it holds "id-token: write" can ask for it as well and publish.
//
//   - A job that publishes with the OIDC token and also checks out the repository and builds it (installs the
//     dependencies, runs a build script) lets all of that code, and the code of every dependency, publish. The package
//     is better built in another job without the permission and handed over as an artifact. Not reported for the
//     ecosystems whose publish command builds the package itself and cannot be separated from the build: cargo
//     (`cargo publish` compiles the crate) and `rubygems/release-gem` (it runs `rake release`).
//   - A job that publishes with the OIDC token and has no `environment:`. The trusted publisher of PyPI, npm,
//     crates.io and RubyGems can be tied to an environment, and an environment can ask for an approval, so that a run
//     that someone else started (a compromised workflow, a pull request that reached it) cannot publish on its own.
//     Only the absence of `environment:` is reported: whether the environment is protected is not in the file.
//   - `pypa/gh-action-pypi-publish` and `rubygems/release-gem` with `attestations: false`. Both attest by default
//     (PEP 740, sigstore), which lets the users of the package check where it was built.

// oidcPublishActions are the actions that publish, or get the credential to publish, with the OIDC token of the
// job. buildsItself marks the ones that cannot be separated from a build.
var oidcPublishActions = map[string]struct {
	eco          string
	buildsItself bool
	// trusted decides, from the inputs of the step, whether it uses trusted publishing.
	trusted func(in func(name string) (string, bool)) bool
}{
	"pypa/gh-action-pypi-publish": {eco: "pypi", trusted: func(in func(string) (string, bool)) bool {
		v, ok := in("password")
		return (!ok || v == "") && defaultOrPublicRepository(in, "repository-url", "repository_url")
	}},
	"rubygems/release-gem": {eco: "rubygems", buildsItself: true, trusted: func(in func(string) (string, bool)) bool {
		v, ok := in("setup-trusted-publisher")
		return !ok || !strings.EqualFold(v, "false")
	}},
	"rust-lang/crates-io-auth-action": {eco: "crates", buildsItself: true, trusted: func(func(string) (string, bool)) bool { return true }},
	"nuget/login":                     {eco: "nuget", trusted: func(func(string) (string, bool)) bool { return true }},
}

// oidcBuildActions are actions that build the project or install its dependencies. The input names the setting that
// makes it do so, "" when it always does.
var oidcBuildActions = map[string]string{
	"pypa/cibuildwheel":            "",
	"pyo3/maturin-action":          "",
	"docker/build-push-action":     "",
	"goreleaser/goreleaser-action": "",
	"bahmutov/npm-install":         "",
	"ruby/setup-ruby":              "bundler-cache",
	"pnpm/action-setup":            "run_install",
}

// publisherTools are the packages that a publishing job installs to publish, which is not a build.
var publisherTools = []string{"twine", "npm", "pnpm", "yarn", "bun", "corepack", "uv", "pip", "pipx", "poetry", "hatch", "flit", "pdm", "pkginfo", "packaging", "keyring", "id"}

func (rule *RuleUseTrustedPublishing) checkJobPedantic(job *Job) {
	if !rule.pedantic("use-trusted-publishing") || job == nil || !rule.grantsIDToken() || rule.permissionsComeFromCaller() {
		return
	}
	checkedOut := false
	build := "" // the first build step before the publish
	reportedBuild, reportedEnv := false, false
	for _, step := range job.Steps {
		switch e := step.Exec.(type) {
		case *ExecAction:
			if e.Uses == nil || e.Uses.ContainsExpression() {
				continue
			}
			u := ParseUses(e.Uses.Value)
			if u.Kind != UsesAction {
				continue
			}
			name := u.CanonicalName()
			if name == "actions/checkout" {
				checkedOut = true
			}
			if a, ok := oidcPublishActions[name]; ok {
				if !a.trusted(func(n string) (string, bool) { return inputValue(e, n) }) {
					continue
				}
				rule.attestations(name, e)
				rule.reportOIDCPublish(e.Uses.Value, e.Uses.Pos, a.eco, a.buildsItself, job, checkedOut, build, &reportedBuild, &reportedEnv)
				continue
			}
			if input, ok := oidcBuildActions[name]; ok && build == "" && checkedOut {
				if input == "" {
					build = "uses: " + e.Uses.Value
				} else if v, set := inputValue(e, input); set && !isFalseLiteral(v) && v != "" {
					build = "uses: " + e.Uses.Value
				}
			}
		case *ExecRun:
			s, origin := rule.script(e)
			if s == nil {
				continue
			}
			for _, c := range s.Commands {
				if eco, cmd, ok := publishCommand(c); ok {
					if eco == "npm" && rule.npmRegistryIsPrivate(step, s, c) {
						continue
					}
					if rule.credentialVar(step, eco) != "" || c.HasFlag("--token", "--password", "--api-key") {
						continue // a long-lived credential: the default finding is about that
					}
					start, _ := commandRange(s, origin, c)
					rule.reportOIDCPublish(cmd, start, eco, eco == "crates", job, checkedOut, build, &reportedBuild, &reportedEnv)
					continue
				}
				if build == "" && checkedOut && isBuildCommand(c) {
					build = commandLabel(c)
				}
			}
		}
	}
}

// reportOIDCPublish reports the findings about a step that publishes with the OIDC token, each once for the job.
func (rule *RuleUseTrustedPublishing) reportOIDCPublish(what string, pos *Pos, eco string, buildsItself bool, job *Job, checkedOut bool, build string, reportedBuild, reportedEnv *bool) {
	info := trustedPublishing[eco]
	if !*reportedBuild && !buildsItself && checkedOut && build != "" {
		*reportedBuild = true
		rule.errorIDAt("use-trusted-publishing", pos, quote(what)+" publishes to "+info.registry+" with the OIDC token of the job (\"id-token: write\"), but the job also checks out the repository and runs "+quote(build)+" before it, so that code and the code of its dependencies can use the token as well. build the package in another job without \"id-token: write\", pass it on as an artifact, and publish from a job that only downloads and publishes it")
	}
	if !*reportedEnv && job.Environment == nil {
		*reportedEnv = true
		rule.errorIDAt("use-trusted-publishing", pos, quote(what)+" publishes to "+info.registry+" with the OIDC token of the job (\"id-token: write\"), and the job has no \"environment:\". tie the trusted publisher to an environment and add it to the job (with required reviewers if you can), so that a run that someone else started cannot publish on its own")
	}
}

// attestations reports `attestations: false` of an action that attests by default.
func (rule *RuleUseTrustedPublishing) attestations(name string, e *ExecAction) {
	if name != "pypa/gh-action-pypi-publish" && name != "rubygems/release-gem" {
		return
	}
	v, ok := inputValue(e, "attestations")
	if !ok || !isFalseLiteral(v) {
		return
	}
	rule.errorIDAt("use-trusted-publishing", e.Inputs["attestations"].Value.Pos, "action "+quote(e.Uses.Value)+" is told not to publish attestations (\"attestations: false\"), although it does by default: they let the users of the package check where and how it was built. remove the input")
}

// commandLabel is the start of a command as written, for messages: the name and up to two more words that are not
// expansions (`python -m build`, `npm run build`).
func commandLabel(c *runscript.Command) string {
	i := slices.Index(c.Words, c.NameWord)
	if i < 0 {
		return c.Name
	}
	var parts []string
	for _, w := range c.Words[i:] {
		if len(parts) == 3 || w.Dynamic() {
			break
		}
		parts = append(parts, w.Value)
	}
	return strings.Join(parts, " ")
}

// isBuildCommand reports whether the command builds the project or installs its dependencies, which runs the code of
// the project and of its dependencies.
func isBuildCommand(c *runscript.Command) bool {
	if in := c.Installs(); in != nil {
		if in.Run || in.Global {
			return false
		}
		switch in.Ecosystem {
		case "pypi", "npm", "crates", "go", "rubygems":
		default:
			return false
		}
		if len(in.Packages) > 0 {
			onlyTools := true
			for _, p := range in.Packages {
				if p.Dynamic || !slices.Contains(publisherTools, strings.ToLower(p.Name)) {
					onlyTools = false
				}
			}
			if onlyTools {
				return false
			}
		}
		return true
	}
	sub := c.Sub(0)
	name := strings.TrimSuffix(strings.TrimPrefix(c.Name, "./"), ".exe")
	switch {
	case name == "make" || name == "gmake" || name == "cmake" || name == "ninja" || name == "rake" || name == "tox" || name == "nox" ||
		name == "gradle" || name == "gradlew" || name == "mvn" || name == "mvnw" || name == "msbuild" || name == "pyproject-build":
		return true
	case name == "build" && c.NameWord != nil:
		return true
	case strings.HasPrefix(name, "python"):
		for i, a := range c.Args {
			if a.Value == "-m" && i+1 < len(c.Args) && (c.Args[i+1].Value == "build" || c.Args[i+1].Value == "setuptools") || strings.HasSuffix(a.Value, "setup.py") {
				return true
			}
		}
	case c.Tool == "uv":
		return sub == "build" || sub == "sync"
	case name == "poetry" || name == "hatch" || name == "flit" || name == "pdm":
		return sub == "build" || sub == "install"
	case c.Tool == "cargo":
		return sub == "build" || sub == "test" || sub == "check" || sub == "package"
	case c.Tool == "go":
		return sub == "build" || sub == "test" || sub == "generate"
	case name == "dotnet":
		return sub == "build" || sub == "pack" || sub == "restore" || sub == "test"
	case c.Tool == "npm" || c.Tool == "pnpm" || c.Tool == "yarn" || c.Tool == "bun":
		return slices.Contains([]string{"run", "run-script", "test", "build", "pack", "rebuild", "prepare", "compile"}, sub)
	case c.Tool == "gem":
		return sub == "build"
	case name == "bundle":
		return sub == "install" || sub == "exec"
	}
	return false
}
