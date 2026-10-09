package jactionlint

import (
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
			rule.errorIDAt("adhoc-packages", start, "command "+quote(c.Name+" "+c.Verb())+" installs a package outside of a lock file: its version and its dependencies are resolved anew on every run. add the package to "+tool+" and install with "+instead).endAt(end)
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

// hasRegistryPackage reports whether the install names a package from a registry, a repository or a URL. Local
// paths do not count: `npm install .` and `npm install ./vendor/pkg` install from the checkout.
func hasRegistryPackage(in *runscript.Install) bool {
	for _, p := range in.Packages {
		if !p.Local && p.Kind != runscript.KindPath {
			return true
		}
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "adhoc-packages", Group: RuleGroupSecurity, Summary: "A package is installed by name with npm, yarn, pnpm, bun, gem or bundle add outside of a lock file.", DefaultLevel: SeverityWarning, Profile: ProfileDefault, DocsAnchor: "check-adhoc-packages"},
	)
	registerRuleFactory("adhoc-packages", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("adhoc-packages") {
			return nil
		}
		return []Rule{NewRuleAdhocPackages()}
	})
}
