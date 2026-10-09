package jactionlint

import (
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleAdhocPackages detects `run:` steps which install a package by name with a JavaScript package manager
// (npm, yarn, pnpm, bun) or a Ruby tool (gem, bundle add), outside of a manifest that produces a lock file.
//
// A package installed this way is usually not pinned, and even when it is, its dependencies are resolved anew on
// every run. Adding the package to package.json or a Gemfile, committing the lock file and installing with
// `npm ci` or `bundle install` makes the workflow reproducible.
type RuleAdhocPackages struct {
	RuleBase
	runContext
}

// NewRuleAdhocPackages creates a new RuleAdhocPackages instance.
func NewRuleAdhocPackages() *RuleAdhocPackages {
	return &RuleAdhocPackages{
		RuleBase: NewRuleBase("adhoc-packages", "Checks for packages installed by name outside of a lock file at \"run:\""),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleAdhocPackages) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleAdhocPackages) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleAdhocPackages) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleAdhocPackages) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	for _, c := range s.Commands {
		if tool, instead, ok := adhocInstall(c); ok {
			start, end := commandRange(s, origin, c)
			resolved := "its version and its dependencies are resolved anew on every run"
			if in := c.Installs(); in != nil && registryPackagesPinned(in) {
				// The version is fixed, but the packages it depends on are not
				resolved = "its dependencies are resolved anew on every run, although its version is pinned"
			}
			rule.errorIDAt("adhoc-packages", start, "command "+quote(c.Name+" "+c.Verb())+" installs a package outside of a lock file: "+resolved+". add the package to "+tool+" and install with "+instead).endAt(end)
		}
	}
	return nil
}

// adhocInstall reports whether the command installs packages by name with a tool whose packages should come from
// a manifest and a lock file. It returns the manifest to add the package to and the command to install from it.
func adhocInstall(c *runscript.Command) (manifest, instead string, ok bool) {
	switch c.Name {
	case "bundle":
		// `bundle add` changes the lock file of the run
		if c.Sub(0) == "add" && len(c.Positional) > 1 {
			return "the Gemfile and commit the Gemfile.lock", "`bundle install`", true
		}
		return "", "", false
	case "gem":
		in := c.Installs()
		if in == nil || in.Verb != "install" || !hasRegistryPackage(in) {
			return "", "", false
		}
		return "a Gemfile and commit the Gemfile.lock", "`bundle install`", true
	}
	if lernaAddsPackage(c) {
		return "the package.json of the package and commit the lock file", "the frozen lock file install of the package manager (`npm ci`, `yarn install --immutable`, `pnpm install --frozen-lockfile`)", true
	}
	in := c.Installs()
	if in == nil || in.Ecosystem != "npm" || in.Run || !hasRegistryPackage(in) {
		return "", "", false
	}
	switch c.Tool {
	case "npm":
		return "package.json and commit the package-lock.json", "`npm ci`", true
	case "yarn":
		return "package.json and commit the yarn.lock", "`yarn install --immutable` (or `--frozen-lockfile` for Yarn 1)", true
	case "pnpm":
		return "package.json and commit the pnpm-lock.yaml", "`pnpm install --frozen-lockfile`", true
	case "bun":
		return "package.json and commit the lock file", "`bun ci`", true
	}
	return "", "", false
}

// lernaAddsPackage reports whether the command is `lerna add pkg`, also started through yarn, pnpm, npx or
// `pnpm exec`: it writes the package into the package.json files of the repository.
func lernaAddsPackage(c *runscript.Command) bool {
	pos := c.Positional
	if c.Name == "lerna" {
		pos = append([]*runscript.Word{c.NameWord}, pos...)
	}
	for i, w := range pos {
		if i > 1 {
			break
		}
		if w.Dynamic() || w.Value != "lerna" {
			continue
		}
		rest := pos[i+1:]
		if len(rest) < 2 || rest[0].Dynamic() || rest[0].Value != "add" {
			return false
		}
		for _, p := range rest[1:] {
			if !expandsList(p) && !strings.HasPrefix(p.Value, ".") && !strings.HasPrefix(p.Value, "/") {
				return true
			}
		}
		return false
	}
	return false
}

// hasRegistryPackage reports whether the install names a package from a registry, a repository or a URL. Local
// paths do not count: `npm install .` and `npm install ./vendor/pkg` install from the checkout.
func hasRegistryPackage(in *runscript.Install) bool {
	for _, p := range in.Packages {
		if !p.Local && p.Kind != runscript.KindPath && !expandsList(p.Word) {
			return true
		}
	}
	return false
}

// expandsList reports whether the word is a list of arguments the script builds, `"${filters[@]}"` or `"$@"`. What
// is in it is not known: it holds the flags of the command as well as packages.
func expandsList(w *runscript.Word) bool {
	if w == nil {
		return false
	}
	for _, m := range []string{"[@]}", "[*]}", "$@", "$*", "${@", "${*"} {
		if strings.Contains(w.Raw, m) {
			return true
		}
	}
	return false
}

// registryPackagesPinned reports whether every package of the install that comes from a registry is given an exact
// version (`left-pad@1.3.0`, `gem install bundler -v 2.4.22`).
func registryPackagesPinned(in *runscript.Install) bool {
	n := 0
	for _, p := range in.Packages {
		if p.Local || p.Kind == runscript.KindPath {
			continue
		}
		if !p.Pinned || p.Dynamic {
			return false
		}
		n++
	}
	return n > 0
}

func init() {
	registerRules(
		RuleInfo{ID: "adhoc-packages", Group: RuleGroupSecurity, Summary: "A package is installed by name with npm, yarn, pnpm, bun, gem or bundle add outside of a lock file.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-adhoc-packages"},
	)
	registerRuleFactory("adhoc-packages", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("adhoc-packages") {
			return nil
		}
		return []Rule{NewRuleAdhocPackages()}
	})
}
