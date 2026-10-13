package jactionlint

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// The pedantic finding of unlocked-install about the release age: an install that resolves versions (it names
// packages, or reads a manifest without a lock file) takes the newest release of every dependency, however new. Most
// compromised releases are found and pulled within days, so a cooldown (only versions that are some days old) keeps
// them out of the workflow. It is the counterpart of dependabot-cooldown for the installs of a `run:` script
// (zizmor#2044).
//
// The setting of each tool, as of the version that has it:
//
//   - uv: `--exclude-newer` or `UV_EXCLUDE_NEWER`, `exclude-newer` in uv.toml or `[tool.uv]` of pyproject.toml. A date
//     or a duration ("7 days").
//   - pip 26.0: `--uploaded-prior-to` or `PIP_UPLOADED_PRIOR_TO`, only for indexes that tell the upload time.
//   - npm 11.10: `--min-release-age` (days) or `min-release-age` in .npmrc, or `--before` (a date).
//   - pnpm 10.16: `minimumReleaseAge` (minutes) in pnpm-workspace.yaml. pnpm 11 has a default of one day.
//   - Yarn 4.10: `npmMinimalAgeGate` in .yarnrc.yml. Yarn 4.15 has a default of one day. Yarn 1 has none.
//   - bun 1.3: `minimumReleaseAge` (seconds) under `[install]` of bunfig.toml.
//
// An install that does not resolve is not reported: `npm ci`, `uv sync`, `--frozen-lockfile`, `--require-hashes`, a
// constraints file `-c`, and a manifest install in a repository with the lock file. `npx` and the other runners of
// npm packages are not reported either, because they usually run a binary of the project. A command after a `cd` is
// skipped as in the check of the lifecycle scripts, because the configuration in force is not known.

var (
	pnpmWorkspaceAgeRe = regexp.MustCompile(`(?m)^minimumReleaseAge:\s*["']?(\d+)`)
	bunfigAgeRe        = regexp.MustCompile(`(?m)^\s*minimumReleaseAge\s*=\s*(\d[\d_]*)`)
	yarnrcGateRe       = regexp.MustCompile(`(?m)^npmMinimalAgeGate:\s*["']?([^"'\s#]+)`)
	pnpmVersionTextRe  = regexp.MustCompile(`pnpm@\^?(\d+)`)
	tomlHeaderRe       = regexp.MustCompile(`^\s*\[+\s*([^\]]*?)\s*\]+`)
	tomlExcludeNewerRe = regexp.MustCompile(`^\s*exclude-newer\s*=\s*(.+?)\s*(#.*)?$`)
)

// cooldownSetting is how a tool is given a release age by the command line, the text of a script and the environment.
// Each tool reads its own: `UV_EXCLUDE_NEWER` does nothing for npm, and `git log --before` is not a setting at all.
type cooldownSetting struct {
	// flag matches the name of an option of the command.
	flag *regexp.Regexp
	// texts find the setting and its value in a script that writes the configuration, or in the value of an option
	// (`pipx install --pip-args="--uploaded-prior-to P7D"`), see settingValue.
	texts []*regexp.Regexp
	// env matches the lower case name of an environment variable.
	env *regexp.Regexp
}

var noMatch = regexp.MustCompile(`a^`)

// settingValue builds the pattern of a setting written in a script or an option: the name, a separator (`=`, `:` or
// white space) and the value, which is the one capture group. The name must end there, so that
// `minimumReleaseAgeExclude` is not `minimumReleaseAge`.
func settingValue(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:` + name + `)\s*(?:=|:|\s)\s*["']?([^\s"']*)`)
}

var cooldownSettings = map[string]cooldownSetting{
	"uv":  {regexp.MustCompile(`^--exclude-newer$`), []*regexp.Regexp{settingValue(`exclude[-_]newer`)}, regexp.MustCompile(`^uv_exclude_newer$`)},
	"pip": {regexp.MustCompile(`^--uploaded-prior-to$`), []*regexp.Regexp{settingValue(`uploaded[-_]prior[-_]to`)}, regexp.MustCompile(`^pip_uploaded_prior_to$`)},
	"npm": {regexp.MustCompile(`^--(min-release-age|before)$`),
		[]*regexp.Regexp{settingValue(`(?:npm_config_)?min[-_]release[-_]age`), settingValue(`npm_config_before`), settingValue(`config\s+set\s+before`)},
		regexp.MustCompile(`^npm_config_(min_release_age|before)$`)},
	// pnpm reads minimum-release-age (not min-release-age or before) from the environment, pnpm-workspace.yaml and, up to
	// pnpm 10, .npmrc; minimumReleaseAgeExclude and minimumReleaseAgeStrict are other settings
	"pnpm": {regexp.MustCompile(`(?i)^--config\.(minimumReleaseAge|minimum-release-age)$`),
		[]*regexp.Regexp{settingValue(`minimum(?:-release-age|ReleaseAge|_release_age)`)},
		regexp.MustCompile(`^(npm|pnpm)_config_minimum_release_age$`)},
	"yarn": {noMatch, []*regexp.Regexp{settingValue(`npmMinimalAgeGate|npm_minimal_age_gate`)}, regexp.MustCompile(`^yarn_npm_minimal_age_gate$`)},
	// bun: --minimum-release-age (seconds) and [install] minimumReleaseAge of bunfig.toml, nothing in .npmrc
	"bun": {regexp.MustCompile(`^--minimum-release-age$`), []*regexp.Regexp{settingValue(`minimumReleaseAge|minimum-release-age`)}, noMatch},
}

// anySetting reports whether the text sets the tool's release age to a value that is not off.
func (s cooldownSetting) anySetting(text string) bool {
	for _, re := range s.texts {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if !cooldownValueOff(m[1]) {
				return true
			}
		}
	}
	return false
}

// cooldownKey maps the tool of a command to the key of cooldownSettings.
func cooldownKey(tool string) string {
	switch tool {
	case "uvx":
		return "uv"
	case "pipx":
		return "pip"
	}
	return tool
}

// cooldownValueOff reports whether the value of a setting means no cooldown: nothing, false, none, or a zero
// with or without a unit (`0`, `0m`, `0d`).
func cooldownValueOff(v string) bool {
	v = strings.ToLower(strings.Trim(strings.TrimSpace(v), `"'`))
	switch v {
	case "", "false", "none", "off", "null":
		return true
	}
	return strings.Trim(strings.TrimRight(v, "abcdefghijklmnopqrstuvwxyz"), "0._") == "" && strings.TrimRight(v, "abcdefghijklmnopqrstuvwxyz") != ""
}

// lockFilesOf are the lock files of the package managers of npm.
var lockFilesOf = map[string][]string{
	"npm":  {"package-lock.json", "npm-shrinkwrap.json"},
	"yarn": {"yarn.lock"},
	"pnpm": {"pnpm-lock.yaml"},
	"bun":  {"bun.lock", "bun.lockb"},
}

func (rule *RuleUnlockedInstall) checkReleaseAge(step *Step, s *runscript.Script, origin runscript.Origin, c *runscript.Command, in *runscript.Install) {
	if !rule.pedantic("unlocked-install") || in.Locked {
		return
	}
	switch in.Ecosystem {
	case "pypi":
		switch c.Tool {
		case "pip", "uv", "uvx", "pipx":
		default:
			return
		}
		if in.FromManifest || c.HasFlag("-c", "--constraint") {
			return
		}
	case "npm":
		switch c.Tool {
		case "npm", "pnpm", "yarn", "bun":
		default:
			return
		}
		if in.Run || !lifecycleVerb(in) {
			return
		}
	default:
		return
	}
	set := cooldownSettings[cooldownKey(c.Tool)]
	for _, f := range c.Flags {
		// the value counts: `--min-release-age=0` and `--exclude-newer false` switch the age off. An option whose
		// value is not known (an expression, a variable) is taken as set.
		if set.flag.MatchString(f.Name) && (f.Value == nil || f.Value.Dynamic() || !cooldownValueOff(f.Value.Value)) {
			return
		}
		if f.Value != nil && set.anySetting(f.Value.Value) {
			return
		}
	}
	// these point uv at another configuration, which is not known; `--no-config` is the opposite and is handled where
	// the files are read
	if c.HasFlag("--project", "--directory", "--config-file") && c.Tool == "uv" {
		return
	}
	dirs, ok := rule.commandDirs(rule.root, step, s, c)
	if !ok {
		return
	}
	if rule.cooldownSetByWorkflow(step, set, s.Source[:c.Offset]) {
		return
	}
	if in.Ecosystem == "npm" && in.FromManifest && !toldNotToFreeze(c) && (rule.root == "" || rule.hasAnyFile(dirs, lockFilesOf[c.Tool])) {
		// nothing is resolved when the lock file decides the versions, and without the repository it is not known
		// whether there is one
		return
	}
	if rule.cooldownSetByRepository(dirs, step, c, s.Source[:c.Offset]) {
		return
	}
	name := c.Name
	switch c.Tool {
	case "uv":
		name = "uv"
		if p := c.Sub(0); p != "" {
			name += " " + p
			if c.Sub(1) == "install" || c.Sub(1) == "run" {
				name += " " + c.Sub(1)
			}
		}
	case "uvx":
	case "pip":
		if !strings.HasPrefix(c.Name, "pip") {
			name = "python -m pip"
		}
		name += " " + in.Verb
	default:
		if in.Verb != "" && !(c.Tool == "yarn" && c.Sub(0) == "") {
			name += " " + in.Verb
		}
	}
	const why = "takes the newest release of every dependency, however new, so a compromised release reaches the workflow before it is found and pulled. "
	var msg string
	switch c.Tool {
	case "uv", "uvx":
		msg = quote(name) + " " + why + "pass `--exclude-newer` with a duration (`--exclude-newer \"7 days\"`) or a date, or set `exclude-newer` in uv.toml"
	case "pip", "pipx":
		msg = quote(name) + " " + why + "pass `--uploaded-prior-to` (pip 26.0 and later; a date, or a duration such as `P7D`), or install with uv and `--exclude-newer`"
	case "npm":
		msg = quote(name) + " " + why + "pass `--min-release-age=7` or set `min-release-age=7` in .npmrc (npm 11.10 and later)"
	case "pnpm":
		msg = quote(name) + " " + why + "set `minimumReleaseAge` (in minutes, pnpm 10.16 and later) in pnpm-workspace.yaml"
	case "yarn":
		msg = quote(name) + " " + why + "set `npmMinimalAgeGate` in .yarnrc.yml (Yarn 4.10 and later, Yarn 1 has no such setting)"
	default:
		msg = quote(name) + " " + why + "set `minimumReleaseAge` (in seconds, bun 1.3 and later) under `[install]` in bunfig.toml"
	}
	start, end := commandRange(s, origin, c)
	rule.errorIDAt("unlocked-install", start, msg).endAt(end)
}

// toldNotToFreeze reports whether the command switches the freeze of the lock file off (`--no-frozen-lockfile`,
// `--frozen-lockfile=false`): it then resolves the versions again although the repository has the lock file.
func toldNotToFreeze(c *runscript.Command) bool {
	if c.HasFlag("--no-frozen-lockfile") {
		return true
	}
	for _, f := range c.Flags {
		switch f.Name {
		case "--frozen-lockfile", "--immutable", "--ci":
			if f.Value != nil && !f.Value.Dynamic() && cooldownValueOff(f.Value.Value) {
				return true
			}
		}
	}
	return false
}

func (rule *RuleUnlockedInstall) hasAnyFile(dirs, names []string) bool {
	for _, d := range dirs {
		for _, n := range names {
			if _, ok := rule.files.read(filepath.Join(d, n)); ok {
				return true
			}
		}
	}
	return false
}

// cooldownSetByWorkflow reports whether the environment of the step, or a script of the job that runs before the
// install, gives the tool a release age. before is the part of the script of the step that comes before the install.
// The nearest scope of the environment that sets the variable decides, and a value that switches the gate off
// (`0`, `0m`, `false`, empty) is no cooldown.
func (rule *RuleUnlockedInstall) cooldownSetByWorkflow(step *Step, set cooldownSetting, before string) bool {
	for _, env := range []*Env{step.Env, rule.jobEnv(), rule.workflowEnv()} {
		if env == nil {
			continue
		}
		found, on := false, false
		for name, v := range env.Vars {
			if !set.env.MatchString(name) {
				continue
			}
			found = true
			if v != nil && v.Value != nil && (v.Value.ContainsExpression() || !cooldownValueOff(v.Value.Value)) {
				on = true
			}
		}
		if found {
			return on
		}
	}
	return rule.jobScriptMatches(step, before, set.anySetting)
}

// cooldownSetByRepository reports whether the files of the repository, or the version of the tool, give a cooldown
// to the command.
func (rule *RuleUnlockedInstall) cooldownSetByRepository(dirs []string, step *Step, c *runscript.Command, before string) bool {
	for _, d := range dirs {
		pkg, _ := rule.files.packageJSON(d)
		switch c.Tool {
		case "uv", "uvx":
			// --no-config: uv reads no uv.toml and no pyproject.toml
			if !c.HasFlag("--no-config") && rule.uvExcludesNewer(d) {
				return true
			}
		case "npm":
			rc := rule.files.npmrc(d)
			if v, ok := rc["min-release-age"]; ok && !cooldownValueOff(v) {
				return true
			}
			if v, ok := rc["before"]; ok && !cooldownValueOff(v) {
				return true
			}
		case "pnpm":
			if text, ok := rule.files.read(filepath.Join(d, "pnpm-workspace.yaml")); ok {
				if m := pnpmWorkspaceAgeRe.FindStringSubmatch(text); m != nil && !cooldownValueOff(m[1]) {
					return true
				}
			}
			// up to pnpm 10 the setting is read from .npmrc in this spelling; the keys of npm are not pnpm's
			if v, ok := rule.files.npmrc(d)["minimum-release-age"]; ok && !cooldownValueOff(v) {
				return true
			}
			if major, _, ok := pkg.managerVersion("pnpm"); ok && major >= 11 {
				return true // pnpm 11 waits one day by default
			}
			if rule.pnpmVersionInJob(step, before) >= 11 {
				return true
			}
		case "yarn":
			if rule.yarnHasAgeGate(d, pkg) {
				return true
			}
		case "bun":
			// bunfig.toml only, and only its [install] table; bun reads no release age from .npmrc
			if text, ok := rule.files.read(filepath.Join(d, "bunfig.toml")); ok && bunfigHasAge(text) {
				return true
			}
		}
	}
	return false
}

// bunfigHasAge reports whether the [install] table of a bunfig.toml sets minimumReleaseAge to something but zero.
// The key means nothing in another table.
func bunfigHasAge(text string) bool {
	table := ""
	for _, line := range strings.Split(text, "\n") {
		if m := tomlHeaderRe.FindStringSubmatch(line); m != nil {
			table = m[1]
			continue
		}
		if table != "install" {
			continue
		}
		if m := bunfigAgeRe.FindStringSubmatch(line); m != nil && !cooldownValueOff(strings.ReplaceAll(m[1], "_", "")) {
			return true
		}
	}
	return false
}

// uvExcludesNewer reports whether uv.toml or the [tool.uv] table of pyproject.toml sets exclude-newer.
func (rule *RuleUnlockedInstall) uvExcludesNewer(dir string) bool {
	check := func(file string, tables ...string) bool {
		text, ok := rule.files.read(filepath.Join(dir, file))
		if !ok {
			return false
		}
		table := ""
		for _, line := range strings.Split(text, "\n") {
			if m := tomlHeaderRe.FindStringSubmatch(line); m != nil {
				table = m[1]
				continue
			}
			if m := tomlExcludeNewerRe.FindStringSubmatch(line); m != nil {
				for _, t := range tables {
					if table == t && strings.Trim(m[1], `"' `) != "false" {
						return true
					}
				}
			}
		}
		return false
	}
	return check("uv.toml", "", "pip") || check("pyproject.toml", "tool.uv", "tool.uv.pip")
}

// yarnHasAgeGate reports whether Yarn waits for a release to age: the setting is set, or the version of Yarn
// has a default (4.15 and later). A Yarn 2+ project of a version that is not known is not reported.
func (rule *RuleUnlockedInstall) yarnHasAgeGate(dir string, pkg packageJSON) bool {
	text, hasRC := rule.files.read(filepath.Join(dir, ".yarnrc.yml"))
	if hasRC {
		if m := yarnrcGateRe.FindStringSubmatch(text); m != nil {
			// 0, 0m and the like switch it off
			return strings.Trim(strings.TrimRight(strings.ToLower(m[1]), "abcdefghijklmnopqrstuvwxyz"), "0.") != ""
		}
	}
	if major, minor, ok := pkg.managerVersion("yarn"); ok {
		return major > 4 || major == 4 && minor >= 15
	}
	return hasRC
}

// pnpmVersionInJob returns the major version of pnpm that the job names: `pnpm/action-setup` with a version, or a
// script that installs `pnpm@11`. It is 0 when none does.
func (rule *RuleUnlockedInstall) pnpmVersionInJob(step *Step, before string) int {
	if rule.job == nil {
		return 0
	}
	best := 0
	versionsIn := func(text string) {
		for _, m := range pnpmVersionTextRe.FindAllStringSubmatch(text, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n > best {
				best = n
			}
		}
	}
	for _, st := range rule.job.Steps {
		if st == step {
			// in the script of the step itself, only what comes before the install
			versionsIn(before)
			break
		}
		switch e := st.Exec.(type) {
		case *ExecAction:
			if e.Uses != nil && ParseUses(e.Uses.Value).CanonicalName() == "pnpm/action-setup" {
				if v, ok := e.Inputs["version"]; ok && v != nil && v.Value != nil {
					if n, err := strconv.Atoi(strings.SplitN(strings.TrimLeft(strings.TrimSpace(v.Value.Value), "^~v"), ".", 2)[0]); err == nil && n > best {
						best = n
					}
				}
			}
		case *ExecRun:
			if e.Run != nil {
				versionsIn(e.Run.Value)
			}
		}
	}
	return best
}
