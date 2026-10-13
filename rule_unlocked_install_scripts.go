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
//   - a Yarn that does not run them by default: `packageManager: yarn@4.14.0` or later (Yarn 4.14 changed the default of
//     `enableScripts` to false; an unknown version is not assumed to be one);
//
// pnpm is not reported: pnpm 10 and later run no lifecycle script of a dependency unless it is allowed (a project on
// pnpm 9 or earlier does run them, which the check cannot tell without knowing the version of pnpm).
//
// aube is not reported either, for the same reason: it runs the scripts of a dependency only when `allowBuilds` allows
// the package or it is on the built-in list of trusted packages that aube takes from pnpm (368 names), and
// `--ignore-scripts` turns the rest off (https://aube.sh/package-manager/lifecycle-scripts). It is reported when the
// project opted out of that: `aube install --dangerously-allow-all-builds` (`aube ci` ignores the flag), the same setting as
// `dangerouslyAllowAllBuilds` in .npmrc or in the environment (`AUBE_DANGEROUSLY_ALLOW_ALL_BUILDS`,
// `npm_config_dangerously_allow_all_builds`), or an `allowBuilds` entry `"*": true` in aube-workspace.yaml (or the
// pnpm-workspace.yaml that aube reads when there is none) or in `aube.allowBuilds` / `pnpm.allowBuilds` of package.json.

var (
	// the ways a script turns the option on, see scriptSetsIgnoreScripts
	configSetIgnoreScriptsRe = regexp.MustCompile(`(?i)\bconfig\s+set\s+ignore[-_]scripts(?:\s+|\s*=\s*)["']?(?:true|1)\b`)
	// the ways a script turns on `dangerouslyAllowAllBuilds` of aube
	configSetAllowAllBuildsRe = regexp.MustCompile(`(?i)\bconfig\s+set\s+(?:--?[\w-]+(?:=\S+)?\s+)*dangerously[-_]?allow[-_]?all[-_]?builds(?:\s+|\s*=\s*)["']?(?:true|1)\b`)
	// only an `export` or a write to $GITHUB_ENV reaches a later command: `X=1 aube install` is the environment of that command
	envAllowAllBuildsRe     = regexp.MustCompile(`(?im)(?:\bexport\s+(?:-\w+\s+)*(?:aube|npm_config)_dangerously_allow_all_builds\s*=\s*["']?(?:true|1)\b|^.*\b(?:aube|npm_config)_dangerously_allow_all_builds\s*=\s*["']?(?:true|1)\b.*>>\s*["']?\$\{?GITHUB_ENV)`)
	allowAllBuildsSettingRe = regexp.MustCompile(`(?i)dangerously[-_]?allow[-_]?all[-_]?builds\s*=\s*["']?(?:true|1)\b`)
	aubeIgnoreScriptsRe     = regexp.MustCompile(`(?m)^ignoreScripts:\s*["']?true\b`)
	envIgnoreScriptsRe      = regexp.MustCompile(`(?i)\bnpm_config_ignore_scripts\s*=\s*["']?(?:true|1)\b`)
	npmrcRedirectRe         = regexp.MustCompile(`(?:>>?|\btee\b(?:\s+-a)?)\s*["']?[^\s"'|;&]*\.npmrc\b`)
	heredocStartRe          = regexp.MustCompile(`<<-?\s*["']?([A-Za-z_][A-Za-z0-9_]*)["']?`)
	settingRe               = regexp.MustCompile(`(?i)ignore[-_]scripts\s*=\s*["']?(?:true|1)\b`)
	bunfigIgnoreScriptsRe   = regexp.MustCompile(`(?m)^\s*ignoreScripts\s*=\s*true\b`)
	yarnrcEnableScriptsRe   = regexp.MustCompile(`(?m)^enableScripts:\s*["']?(\w+)`)
)

func (rule *RuleUnlockedInstall) checkLifecycleScripts(step *Step, s *runscript.Script, origin runscript.Origin, c *runscript.Command, in *runscript.Install) {
	if in.Ecosystem != "npm" || in.Run || in.Global || in.NoScripts || !rule.pedantic("unlocked-install") {
		return
	}
	switch c.Tool {
	case "npm", "yarn", "bun":
	case "aube":
		rule.checkAubeBuilds(step, s, origin, c, in)
		return
	default:
		return
	}
	if !lifecycleVerb(in) {
		return
	}
	if rule.scriptsDisabledByWorkflow(step, c.Tool, s.Source[:c.Offset]) {
		return
	}
	if ignoreScriptsInCommandEnv(c) {
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

// ignoreScriptsInCommandEnv reports whether the environment of the command itself turns the scripts off:
// `npm_config_ignore_scripts=true npm ci`, and the wrappers that take assignments as arguments
// (`env npm_config_ignore_scripts=true npm ci`, `sudo -E npm_config_ignore_scripts=true npm ci`), which the parser
// does not list as assignments of the command.
func ignoreScriptsInCommandEnv(c *runscript.Command) bool {
	on := func(name string, v *runscript.Word) bool {
		return (strings.EqualFold(name, "npm_config_ignore_scripts") || c.Tool == "aube" && strings.EqualFold(name, "aube_ignore_scripts")) && v != nil && (v.Dynamic() || strings.EqualFold(v.Value, "true") || v.Value == "1")
	}
	for _, a := range c.Assigns {
		if on(a.Name, a.Value) {
			return true
		}
	}
	for _, w := range c.Words {
		if w == c.NameWord {
			break
		}
		if name, value, ok := strings.Cut(w.Value, "="); ok && shellVarNameRe.MatchString(name) {
			if on(name, &runscript.Word{Value: value, Raw: w.Raw[min(len(name)+1, len(w.Raw)):]}) {
				return true
			}
		}
	}
	return false
}

var shellVarNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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
// environment variable, or a script of the job that sets the option. before is the part of the script of the step that
// comes before the install.
func (rule *RuleUnlockedInstall) scriptsDisabledByWorkflow(step *Step, tool, before string) bool {
	names := []string{"npm_config_ignore_scripts", "NPM_CONFIG_IGNORE_SCRIPTS"}
	switch tool {
	case "yarn":
		names = append(names, "YARN_ENABLE_SCRIPTS")
	case "aube":
		names = append(names, "AUBE_IGNORE_SCRIPTS")
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
	return rule.jobScriptMatches(step, before, scriptSetsIgnoreScripts)
}

// jobScriptMatches reports whether the script of an earlier step of the job matches, or the part of the script of the
// step itself that comes before the install (before): what a script does after the install does not change it.
func (rule *RuleUnlockedInstall) jobScriptMatches(step *Step, before string, match func(string) bool) bool {
	if rule.job == nil {
		return false
	}
	for _, st := range rule.job.Steps {
		if st == step {
			return match(before)
		}
		if r, ok := st.Exec.(*ExecRun); ok && r.Run != nil && match(r.Run.Value) {
			return true
		}
	}
	return false
}

// scriptSetsIgnoreScripts reports whether the script turns the option on in the configuration that a later install
// reads: `npm config set ignore-scripts true` (or `ignore-scripts=true`), the environment variable
// `npm_config_ignore_scripts=true`, or `ignore-scripts=true` written to a .npmrc (`>> ~/.npmrc`, `> .npmrc`,
// `tee -a .npmrc`, also with the setting in the heredoc that follows). Text that only prints the setting
// (`echo "ignore-scripts=true"`) does not.
func scriptSetsIgnoreScripts(text string) bool {
	if configSetIgnoreScriptsRe.MatchString(text) || envIgnoreScriptsRe.MatchString(text) {
		return true
	}
	if !strings.Contains(strings.ToLower(text), "ignore") {
		return false
	}
	for _, loc := range npmrcRedirectRe.FindAllStringIndex(text, -1) {
		// the setting on the same line (`echo ignore-scripts=true >> .npmrc`) or in the heredoc of that command, and
		// nowhere else: a later echo or a comment does not write it
		start := lineStartOf(text, loc[0])
		end := len(text)
		if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
			end = start + i
		}
		line := text[start:end]
		if settingRe.MatchString(line) {
			return true
		}
		if m := heredocStartRe.FindStringSubmatch(line); m != nil && settingRe.MatchString(heredocBody(text[min(end+1, len(text)):], m[1])) {
			return true
		}
	}
	return false
}

// heredocBody returns the lines of a heredoc, from the start of text up to the line that is the delimiter.
func heredocBody(text, delim string) string {
	var body strings.Builder
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) == delim {
			break
		}
		body.WriteString(l)
		body.WriteByte('\n')
	}
	return body.String()
}

func lineStartOf(text string, i int) int {
	if j := strings.LastIndexByte(text[:i], '\n'); j >= 0 {
		return j + 1
	}
	return 0
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
		case "aube":
			if v, ok := rc["ignoreScripts"]; ok && strings.EqualFold(v, "true") {
				return true
			}
			if text, ok := rule.aubeWorkspaceYAML(d); ok && aubeIgnoreScriptsRe.MatchString(text) {
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
		// Yarn 2+ without the setting: the default is true up to Yarn 4.13 and false from 4.14 (Configuration.ts of
		// the release tags), so only a version that package.json names can tell
		major, minor, ok := pkg.managerVersion("yarn")
		return ok && (major > 4 || major == 4 && minor >= 14)
	}
	if major, minor, ok := pkg.managerVersion("yarn"); ok {
		return major > 4 || major == 4 && minor >= 14
	}
	return false
}

// aubeWorkspaceYAML returns the workspace file that aube reads in the directory: aube-workspace.yaml, and the
// pnpm-workspace.yaml only when there is none (aube ignores it next to an aube-workspace.yaml).
func (rule *RuleUnlockedInstall) aubeWorkspaceYAML(dir string) (string, bool) {
	if text, ok := rule.files.read(filepath.Join(dir, "aube-workspace.yaml")); ok {
		return text, true
	}
	return rule.files.read(filepath.Join(dir, "pnpm-workspace.yaml"))
}

// checkAubeBuilds reports `aube install` and `aube add` that run the build scripts of every dependency because the
// project opted out of the allow list of aube, see the comment at the top of this file.
func (rule *RuleUnlockedInstall) checkAubeBuilds(step *Step, s *runscript.Script, origin runscript.Origin, c *runscript.Command, in *runscript.Install) {
	switch in.Verb {
	case "install", "i", "add", "a":
	default:
		return // `aube ci` sets dangerouslyAllowAllBuilds to false itself, and `aube dlx` has --allow-build=PKG
	}
	before := s.Source[:c.Offset]
	if rule.scriptsDisabledByWorkflow(step, c.Tool, before) || ignoreScriptsInCommandEnv(c) {
		return
	}
	dirs, ok := rule.commandDirs(rule.root, step, s, c)
	if !ok || rule.scriptsDisabledByRepository(dirs, c.Tool) {
		return
	}
	why := rule.aubeAllowsEveryBuild(step, c, before, dirs)
	if why == "" {
		return
	}
	name := c.Name + " " + in.Verb
	start, end := commandRange(s, origin, c)
	rule.errorIDAt("unlocked-install", start, quote(name)+" runs the lifecycle scripts of every dependency ("+why+"), and a compromised release uses them to run code on the runner. aube runs only the scripts that `allowBuilds` and its built-in list of trusted packages allow: remove this, review what was skipped with `aube ignored-builds` and approve the packages that need a build with `aube approve-builds`").endAt(end)
}

// aubeAllowsEveryBuild says how the install is told to run the build scripts of every dependency, "" if it is not.
// Only a literal true counts: a value that is an expression is not known.
func (rule *RuleUnlockedInstall) aubeAllowsEveryBuild(step *Step, c *runscript.Command, before string, dirs []string) string {
	// the command line beats every other place, and the last flag wins
	if f := lastFlag(c, "--dangerously-allow-all-builds"); f != nil {
		if f.Value == nil || (!f.Value.Dynamic() && ageSettingOn(f.Value.Value)) {
			return "`--dangerously-allow-all-builds`"
		}
		return ""
	}
	isTrue := func(v string) bool {
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	}
	for _, a := range c.Assigns {
		if strings.EqualFold(a.Name, "aube_dangerously_allow_all_builds") || strings.EqualFold(a.Name, "npm_config_dangerously_allow_all_builds") {
			if a.Value != nil && !a.Value.Dynamic() && isTrue(a.Value.Value) {
				return "`" + a.Name + "` is true"
			}
		}
	}
	for _, n := range []string{"AUBE_DANGEROUSLY_ALLOW_ALL_BUILDS", "npm_config_dangerously_allow_all_builds", "NPM_CONFIG_DANGEROUSLY_ALLOW_ALL_BUILDS"} {
		if v, ok := rule.envValue(step, n); ok && !v.ContainsExpression() && isTrue(v.Value) {
			return "`" + n + "` is true"
		}
	}
	if rule.jobScriptMatches(step, before, scriptAllowsEveryBuild) {
		return "an earlier script sets `dangerouslyAllowAllBuilds`"
	}
	for _, d := range dirs {
		rc := rule.files.npmrc(d)
		for _, k := range []string{"dangerouslyAllowAllBuilds", "dangerously-allow-all-builds"} {
			if v, ok := rc[k]; ok && isTrue(v) {
				return "`" + k + "=true` in .npmrc"
			}
		}
		if text, ok := rule.aubeWorkspaceYAML(d); ok && allowBuildsHasWildcard(text) {
			return "`allowBuilds` has the entry `\"*\": true`"
		}
		if pkg, _ := rule.files.packageJSON(d); pkg.allowsEveryBuild() {
			return "`allowBuilds` in package.json has the entry `\"*\": true`"
		}
	}
	return ""
}

// scriptAllowsEveryBuild reports whether the script turns `dangerouslyAllowAllBuilds` on in the configuration that a later
// install reads, the way scriptSetsIgnoreScripts does for `ignore-scripts`.
func scriptAllowsEveryBuild(text string) bool {
	if configSetAllowAllBuildsRe.MatchString(text) || envAllowAllBuildsRe.MatchString(text) {
		return true
	}
	if !strings.Contains(strings.ToLower(text), "dangerously") {
		return false
	}
	for _, loc := range npmrcRedirectRe.FindAllStringIndex(text, -1) {
		start := lineStartOf(text, loc[0])
		end := len(text)
		if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
			end = start + i
		}
		line := text[start:end]
		if allowAllBuildsSettingRe.MatchString(line) {
			return true
		}
		if m := heredocStartRe.FindStringSubmatch(line); m != nil && allowAllBuildsSettingRe.MatchString(heredocBody(text[min(end+1, len(text)):], m[1])) {
			return true
		}
	}
	return false
}

// allowBuildsHasWildcard reports whether the `allowBuilds` map of a workspace file has the entry `"*": true`, written as
// a block or as a flow mapping. Only the key `*` alone is the catch-all: `"@babel/*": true` allows one scope.
func allowBuildsHasWildcard(text string) bool {
	lines := strings.Split(text, "\n")
	in := false
	for i, line := range lines {
		t := strings.TrimSpace(stripYAMLComment(line))
		switch {
		case strings.HasPrefix(line, "allowBuilds:"):
			rest := strings.TrimSpace(stripYAMLComment(strings.TrimPrefix(line, "allowBuilds:")))
			in = rest == ""
			if strings.HasPrefix(rest, "{") {
				// a flow mapping, on this line or going on over the next ones until its closing brace
				body := rest[1:]
				for k := i + 1; !strings.Contains(body, "}") && k < len(lines) && k < i+50; k++ {
					body += " " + stripYAMLComment(lines[k])
				}
				body, _, _ = strings.Cut(body, "}")
				for _, entry := range strings.Split(body, ",") {
					if key, value, ok := strings.Cut(entry, ":"); ok && isAllowEverything(key, value) {
						return true
					}
				}
			}
		case t == "":
		case line[0] != ' ' && line[0] != '\t':
			in = false
		case in:
			if key, value, ok := strings.Cut(t, ":"); ok && isAllowEverything(key, value) {
				return true
			}
		}
	}
	return false
}

// isAllowEverything reports whether an entry of `allowBuilds` is `*: true`, with the key and the value quoted or not.
func isAllowEverything(key, value string) bool {
	unquote := func(v string) string { return strings.Trim(strings.TrimSpace(v), `"'`) }
	return unquote(key) == "*" && strings.EqualFold(unquote(value), "true")
}

// stripYAMLComment removes a trailing comment from a line of YAML: a `#` at the start or after white space, outside quotes.
func stripYAMLComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return line[:i]
		}
	}
	return line
}

// lastFlag returns the last flag of the command with the name, nil if there is none.
func lastFlag(c *runscript.Command, name string) *runscript.Flag {
	var found *runscript.Flag
	for _, f := range c.Flags {
		if f.Name == name {
			found = f
		}
	}
	return found
}
