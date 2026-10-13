package jactionlint

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// The pedantic finding of unlocked-install about lifecycle scripts: npm, Yarn and bun installs that run the
// preinstall, install and postinstall scripts of every dependency. That is how npm worms spread (a compromised
// release is installed, its script steals the tokens of the runner and publishes itself with them), and the
// workflow rarely needs the scripts of all of its dependencies. See zizmor#1405.
//
// Whether the scripts run is decided on the command line and by the configuration of the repository. The check
// stays quiet when any of these says that someone chose:
//
//   - `--ignore-scripts` (Yarn 2+: `--mode=skip-build`), the environment variable `npm_config_ignore_scripts` (or
//     `YARN_ENABLE_SCRIPTS=false`), or a script of the job that writes `ignore-scripts=true` (`npm config set`);
//   - `ignore-scripts=true` in the `.npmrc` of the repository (also read by Yarn 1 and bun), `ignoreScripts = true`
//     in `bunfig.toml`, `enableScripts: false` in `.yarnrc.yml`;
//   - `trustedDependencies` in the package.json of a bun project (an allow list replaces the default list of
//     popular packages), `allowScripts` in the package.json of an npm project (the allow list of npm 12) or
//     `allow-scripts` in its `.npmrc`;
//   - a Yarn that does not run them by default: `packageManager: yarn@4.14.0` or later (and a `.yarnrc.yml` without
//     the version known, because Yarn 4.14 changed the default of `enableScripts` to false);
//
// pnpm is not reported: pnpm 10 and later run no lifecycle script of a dependency unless it is allowed (a project on
// pnpm 9 or earlier does run them, which the check cannot tell without knowing the version of pnpm).

var (
	// ignoreScriptsInScriptRe finds the setting `ignore-scripts=true` or `config set ignore-scripts true` in the text
	// of a script, as `npm config set` and an echo into .npmrc write it.
	ignoreScriptsInScriptRe = regexp.MustCompile(`(?i)ignore[-_]scripts["']?\s*(=|\s)\s*["']?(true|1)\b`)
	bunfigIgnoreScriptsRe   = regexp.MustCompile(`(?m)^\s*ignoreScripts\s*=\s*true\b`)
	yarnrcEnableScriptsRe   = regexp.MustCompile(`(?m)^enableScripts:\s*["']?(\w+)`)
)

func (rule *RuleUnlockedInstall) checkLifecycleScripts(step *Step, s *runscript.Script, origin runscript.Origin, c *runscript.Command, in *runscript.Install) {
	if in.Ecosystem != "npm" || in.Run || in.Global || in.NoScripts || !rule.pedantic("unlocked-install") {
		return
	}
	switch c.Tool {
	case "npm", "yarn", "bun":
	default:
		return
	}
	if !lifecycleVerb(in) {
		return
	}
	if rule.scriptsDisabledByWorkflow(step, c.Tool) {
		return
	}
	dirs, ok := rule.commandDirs(rule.root, step, s, c)
	if !ok {
		return
	}
	if rule.scriptsDisabledByRepository(dirs, c.Tool) {
		return
	}
	name := c.Name
	if in.Verb != "" && !(c.Tool == "yarn" && c.Sub(0) == "") {
		name += " " + in.Verb
	}
	var msg string
	switch c.Tool {
	case "npm":
		msg = quote(name) + " runs the lifecycle scripts (preinstall, install, postinstall) of every dependency, and a compromised release uses them to run code on the runner. pass `--ignore-scripts` or set `ignore-scripts=true` in .npmrc, and run what a package needs with `npm rebuild`"
	case "yarn":
		msg = quote(name) + " runs the lifecycle scripts (preinstall, install, postinstall) of every dependency, and a compromised release uses them to run code on the runner. pass `--ignore-scripts` (`--mode=skip-build` in Yarn 2+) or set `enableScripts: false` in .yarnrc.yml"
	default:
		msg = quote(name) + " runs the lifecycle scripts of the packages on the default trusted list of bun, and a compromised release of one of them uses them to run code on the runner. pass `--ignore-scripts`, or list the packages that need them in \"trustedDependencies\" of package.json (a list replaces the default one)"
	}
	start, end := commandRange(s, origin, c)
	rule.errorIDAt("unlocked-install", start, msg).endAt(end)
}

// lifecycleVerb reports whether the command installs packages (which runs their scripts) and does not only
// look at them.
func lifecycleVerb(in *runscript.Install) bool {
	switch in.Verb {
	case "install", "i", "add", "ci", "in", "ins", "inst", "insta", "instal", "isnt", "isnta", "isntal", "isntall", "clean-install",
		"cit", "clean-install-test", "install-ci-test", "a":
		return true
	}
	return false
}

// scriptsDisabledByWorkflow reports whether the workflow itself turns the scripts off for the step: an
// environment variable, or a script of the job that sets the option.
func (rule *RuleUnlockedInstall) scriptsDisabledByWorkflow(step *Step, tool string) bool {
	names := []string{"npm_config_ignore_scripts", "NPM_CONFIG_IGNORE_SCRIPTS"}
	if tool == "yarn" {
		names = append(names, "YARN_ENABLE_SCRIPTS")
	}
	for _, n := range names {
		v, ok := rule.envValue(step, n)
		if !ok {
			continue
		}
		val := strings.ToLower(strings.TrimSpace(v.Value))
		if strings.HasPrefix(n, "YARN_") {
			if val == "false" || val == "0" || v.ContainsExpression() {
				return true
			}
			continue
		}
		if val == "true" || val == "1" || v.ContainsExpression() {
			return true
		}
	}
	if rule.job != nil {
		for _, st := range rule.job.Steps {
			if r, ok := st.Exec.(*ExecRun); ok && r.Run != nil && ignoreScriptsInScriptRe.MatchString(r.Run.Value) {
				return true
			}
			if st == step {
				break
			}
		}
	}
	return false
}

// scriptsDisabledByRepository reports whether the files of the repository in the directories settle the scripts of
// the tool: they are off, or an allow list shows that someone decided.
func (rule *RuleUnlockedInstall) scriptsDisabledByRepository(dirs []string, tool string) bool {
	for _, d := range dirs {
		rc := rule.files.npmrc(d)
		if v, ok := rc["ignore-scripts"]; ok && strings.EqualFold(v, "true") {
			return true // npm, and Yarn 1 and bun read it too
		}
		pkg, _ := rule.files.packageJSON(d)
		switch tool {
		case "npm":
			if _, ok := rc["allow-scripts"]; ok || len(pkg.AllowScripts) > 0 {
				return true
			}
		case "bun":
			if len(pkg.TrustedDependencies) > 0 && string(pkg.TrustedDependencies) != "null" {
				return true
			}
			if text, ok := rule.files.read(filepath.Join(d, "bunfig.toml")); ok && bunfigIgnoreScriptsRe.MatchString(text) {
				return true
			}
		case "yarn":
			if rule.yarnSkipsScripts(d, pkg) {
				return true
			}
		}
	}
	return false
}

// yarnSkipsScripts reports whether Yarn does not run the scripts of dependencies in the directory.
func (rule *RuleUnlockedInstall) yarnSkipsScripts(dir string, pkg packageJSON) bool {
	if text, ok := rule.files.read(filepath.Join(dir, ".yarnrc.yml")); ok {
		if m := yarnrcEnableScriptsRe.FindStringSubmatch(text); m != nil {
			return strings.EqualFold(m[1], "false")
		}
		// Yarn 2+ without the setting: 4.14 changed the default to false, and the version is not known unless
		// package.json names it
		major, minor, ok := pkg.yarnVersion()
		return !ok || major > 4 || major == 4 && minor >= 14
	}
	if major, minor, ok := pkg.yarnVersion(); ok {
		return major > 4 || major == 4 && minor >= 14
	}
	return false
}
