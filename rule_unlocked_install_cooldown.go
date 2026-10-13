package jactionlint

import (
	"path/filepath"
	"regexp"
	"sort"
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
//   - bun 1.3: `--minimum-release-age` or `minimumReleaseAge` (seconds) under `[install]` of bunfig.toml.
//   - aube: `minimumReleaseAge` (minutes) in aube-workspace.yaml (or pnpm-workspace.yaml, which aube ignores when it has
//     an aube-workspace.yaml next to it) and `minimumReleaseAge` or `minimum-release-age` in .npmrc, or
//     `AUBE_MINIMUM_RELEASE_AGE`; aube has a default of one day (1440) since its first release, so only a value that
//     turns it off is reported (https://aube.sh/settings/#setting-minimumreleaseage).
//
// An install that does not resolve is not reported: `npm ci`, `uv sync`, `--frozen-lockfile`, `--require-hashes`, a
// constraints file `-c`, and a manifest install in a repository with the lock file. `npx` and the other runners of
// npm packages are not reported either, because they usually run a binary of the project. A command after a `cd` is
// skipped as in the check of the lifecycle scripts, because the configuration in force is not known.
//
// # One rule for every place a setting can come from
//
// The state of the release age at the install is decided by the places that set it, from the weakest to the
// strongest, and the LAST assignment of a place wins over what came before (see ageState):
//
//	the default of the version of the tool < the files of the repository (root, then the working directory) <
//	the environment of the workflow < of the job < of the step < the scripts that ran before the install <
//	the options of the install command itself
//
// Every value goes through one function, ageSettingOn: `0`, `0d`, `0 days`, `P0D`, `PT0S`, `false`, `none`, `off`
// and nothing are off, a variable or expression is taken as on, and so is anything else. Inside one place the last
// assignment of a key wins, and for the two keys of npm (`min-release-age` and `before`) the age is on when either
// is, because npm lets `before` win. The install is reported when the final state is "not set" or "off".

var (
	pnpmWorkspaceAgeRe = regexp.MustCompile(`(?m)^minimumReleaseAge:\s*["']?([^"'\s#]+)`)
	bunfigAgeRe        = regexp.MustCompile(`^\s*minimumReleaseAge\s*=\s*["']?([^"'\s#]+)`)
	yarnrcGateRe       = regexp.MustCompile(`(?m)^npmMinimalAgeGate:\s*["']?([^"'\s#]+)`)
	pnpmVersionTextRe  = regexp.MustCompile(`pnpm@\^?(\d+)`)
	tomlHeaderRe       = regexp.MustCompile(`^\s*\[+\s*([^\]]*?)\s*\]+`)
	tomlExcludeNewerRe = regexp.MustCompile(`^\s*exclude-newer\s*=\s*(.+?)\s*(#.*)?$`)

	isoZeroRe = regexp.MustCompile(`^p(?:0+[ymwd])*(?:t(?:0+[hms])+)?$`)
	numZeroRe = regexp.MustCompile(`^0+(?:\.0+)?(?:\s*[a-z]+)?$`)
)

// ageSettingOn turns the value of a release-age setting into on or off. Off is no value, `false`, `none`, `off`,
// `null`, a zero with or without a unit (`0`, `0d`, `0m`, `0 days`) and a zero ISO 8601 duration (`P0D`, `PT0S`);
// everything else is on, a variable or an expression (`$AGE`, `${{ vars.AGE }}`) included, because the author chose
// a value that is not known here.
func ageSettingOn(v string) bool {
	v = strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"'`)))
	switch v {
	case "", "false", "none", "off", "null", "no":
		return false
	case "p":
		return true
	}
	return !isoZeroRe.MatchString(v) && !numZeroRe.MatchString(v)
}

// ageAssignment is one place that gives the key a value.
type ageAssignment struct{ key, value string }

// ageState is what is known about the release age while the places are applied one after the other.
type ageState struct{ set, on bool }

// after applies the assignments of one place, which are in source order: the last one of each key wins, and the age is on
// when any key is on.
func (s ageState) after(as []ageAssignment) ageState {
	if len(as) == 0 {
		return s
	}
	last := map[string]string{}
	for _, a := range as {
		last[a.key] = a.value
	}
	on := false
	for _, v := range last {
		on = on || ageSettingOn(v)
	}
	return ageState{set: true, on: on}
}

// keyedText is a way to write a setting in a script: the pattern has one capture group, the value.
type keyedText struct {
	key string
	re  *regexp.Regexp
}

// settingValue builds the pattern of a setting written in a script or an option: the name, a separator (`=`, `:` or
// white space) and the value, which is the one capture group. The name must end there, so that
// `minimumReleaseAgeExclude` is not `minimumReleaseAge`.
func settingValue(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:` + name + `)\s*(?:=|:|\s)\s*["']?([^\s"']*)`)
}

// ageTool says how a tool is given a release age by the options of the command, the text of a script and the
// environment. Each tool reads its own: `UV_EXCLUDE_NEWER` does nothing for npm, and `git log --before` is not a
// setting at all.
type ageTool struct {
	// flagKey returns the key that an option of the command sets.
	flagKey func(name string) (string, bool)
	// texts are the ways to write the settings in a script, or in the value of an option
	// (`pipx install --pip-args="--uploaded-prior-to P7D"`).
	texts []keyedText
	// envKey returns the key that an environment variable (lower case name) sets.
	envKey func(name string) (string, bool)
}

func exactly(names map[string]string) func(string) (string, bool) {
	return func(n string) (string, bool) { k, ok := names[n]; return k, ok }
}

var ageTools = map[string]ageTool{
	"uv": {
		flagKey: exactly(map[string]string{"--exclude-newer": "exclude-newer"}),
		texts:   []keyedText{{"exclude-newer", settingValue(`exclude[-_]newer`)}},
		envKey:  exactly(map[string]string{"uv_exclude_newer": "exclude-newer"}),
	},
	"pip": {
		flagKey: exactly(map[string]string{"--uploaded-prior-to": "uploaded-prior-to"}),
		texts:   []keyedText{{"uploaded-prior-to", settingValue(`uploaded[-_]prior[-_]to`)}},
		envKey:  exactly(map[string]string{"pip_uploaded_prior_to": "uploaded-prior-to"}),
	},
	"npm": {
		flagKey: exactly(map[string]string{"--min-release-age": "min-release-age", "--before": "before"}),
		texts: []keyedText{
			{"min-release-age", settingValue(`(?:npm_config_)?min[-_]release[-_]age`)},
			{"before", settingValue(`npm_config_before`)},
			{"before", settingValue(`config\s+set\s+before`)},
		},
		envKey: exactly(map[string]string{"npm_config_min_release_age": "min-release-age", "npm_config_before": "before"}),
	},
	// pnpm reads minimum-release-age (not the min-release-age and before of npm) from the environment,
	// pnpm-workspace.yaml and, up to pnpm 10, .npmrc; minimumReleaseAgeExclude and minimumReleaseAgeStrict are other
	// settings
	"pnpm": {
		flagKey: func(n string) (string, bool) {
			n = strings.ToLower(n)
			return "minimum-release-age", n == "--config.minimumreleaseage" || n == "--config.minimum-release-age"
		},
		texts:  []keyedText{{"minimum-release-age", settingValue(`minimum(?:-release-age|ReleaseAge|_release_age)`)}},
		envKey: exactly(map[string]string{"npm_config_minimum_release_age": "minimum-release-age", "pnpm_config_minimum_release_age": "minimum-release-age"}),
	},
	"yarn": {
		flagKey: func(string) (string, bool) { return "", false },
		texts:   []keyedText{{"npmMinimalAgeGate", settingValue(`npmMinimalAgeGate|npm_minimal_age_gate`)}},
		envKey:  exactly(map[string]string{"yarn_npm_minimal_age_gate": "npmMinimalAgeGate"}),
	},
	// bun: --minimum-release-age (seconds) and [install] minimumReleaseAge of bunfig.toml, nothing in .npmrc
	"bun": {
		flagKey: exactly(map[string]string{"--minimum-release-age": "minimumReleaseAge"}),
		texts:   []keyedText{{"minimumReleaseAge", settingValue(`minimumReleaseAge|minimum-release-age`)}},
		envKey:  func(string) (string, bool) { return "", false },
	},
	// aube has no option for it. `aube config set minimumReleaseAge 0` and `AUBE_MINIMUM_RELEASE_AGE=0` in a script are
	// read by the text patterns, the environment of the workflow by envKey
	"aube": {
		flagKey: func(string) (string, bool) { return "", false },
		texts:   []keyedText{{"minimum-release-age", settingValue(`minimum(?:-release-age|ReleaseAge|_release_age)`)}},
		envKey:  exactly(map[string]string{"npm_config_minimum_release_age": "minimum-release-age", "aube_minimum_release_age": "minimum-release-age"}),
	},
}

// ageToolOf maps the tool of a command to its entry of ageTools.
func ageToolOf(tool string) ageTool {
	switch tool {
	case "uvx":
		tool = "uv"
	case "pipx":
		tool = "pip"
	}
	return ageTools[tool]
}

// textAssignments finds the settings that the text writes, in the order of the text.
func (t ageTool) textAssignments(text string) []ageAssignment {
	type found struct {
		pos int
		a   ageAssignment
	}
	var all []found
	for _, kt := range t.texts {
		for _, m := range kt.re.FindAllStringSubmatchIndex(text, -1) {
			all = append(all, found{m[0], ageAssignment{kt.key, text[m[2]:m[3]]}})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].pos < all[j].pos })
	out := make([]ageAssignment, len(all))
	for i, f := range all {
		out[i] = f.a
	}
	return out
}

// flagAssignments finds what the options of the command set, in order: the options of the tool, and a setting inside
// the value of any option.
func (t ageTool) flagAssignments(c *runscript.Command) []ageAssignment {
	var out []ageAssignment
	for _, f := range c.Flags {
		if key, ok := t.flagKey(f.Name); ok {
			value := "set" // an option without a value of its own is taken as set
			if f.Value != nil {
				value = f.Value.Value
			}
			out = append(out, ageAssignment{key, value})
		}
		if f.Value != nil {
			out = append(out, t.textAssignments(f.Value.Value)...)
		}
	}
	return out
}

// envAssignments finds what the environment variables of one scope set.
func (t ageTool) envAssignments(env *Env) []ageAssignment {
	if env == nil {
		return nil
	}
	names := make([]string, 0, len(env.Vars))
	for name := range env.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []ageAssignment
	for _, name := range names {
		v := env.Vars[name]
		if key, ok := t.envKey(name); ok && v != nil && v.Value != nil {
			out = append(out, ageAssignment{key, v.Value.Value})
		}
	}
	return out
}

// lockFilesOf are the lock files of the package managers of npm.
var lockFilesOf = map[string][]string{
	"npm":  {"package-lock.json", "npm-shrinkwrap.json"},
	"yarn": {"yarn.lock"},
	"pnpm": {"pnpm-lock.yaml"},
	"bun":  {"bun.lock", "bun.lockb"},
	// aube reads and writes the lock file the project has, see https://aube.sh/package-manager/lockfiles
	"aube": {"aube-lock.yaml", "pnpm-lock.yaml", "bun.lock", "yarn.lock", "npm-shrinkwrap.json", "package-lock.json"},
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
		case "aube":
			// `aube ci` is a frozen install (in.Locked), and `aube install` takes no package
			if !aubeResolves(in) {
				return
			}
		default:
			return
		}
		if in.Run || !lifecycleVerb(in) {
			return
		}
	default:
		return
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
	if in.Ecosystem == "npm" && in.FromManifest && !toldNotToFreeze(c) && (rule.root == "" || rule.hasAnyFile(dirs, lockFilesOf[c.Tool])) {
		// nothing is resolved when the lock file decides the versions, and without the repository it is not known
		// whether there is one
		return
	}
	if st := rule.releaseAgeAt(dirs, step, c, s.Source[:c.Offset]); st.set && st.on {
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
	case "aube":
		msg = quote(name) + " " + why + "aube waits one day for a release by default (`minimumReleaseAge` of 1440 minutes), but the settings of this install turn that off. set `minimumReleaseAge` (in minutes) to more than 0 in aube-workspace.yaml, or remove the setting that makes it 0"
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

// releaseAgeAt returns the state of the release age for the install: the places in the order of the comment of this
// file, the last assignment winning. before is the part of the script of the step that comes before the install.
func (rule *RuleUnlockedInstall) releaseAgeAt(dirs []string, step *Step, c *runscript.Command, before string) ageState {
	tool := ageToolOf(c.Tool)
	var st ageState
	// the default of the version of the tool, and the files
	if rule.ageDefaultOn(dirs, step, c, before) {
		st = ageState{set: true, on: true}
	}
	npmrcMode := pnpmNpmrcIgnored
	if c.Tool == "pnpm" {
		npmrcMode = rule.pnpmNpmrcMode(dirs, step, before)
	}
	for _, d := range dirs {
		for _, as := range rule.ageFiles(d, c, npmrcMode) {
			st = st.after(as)
		}
	}
	// the environment, from the workflow to the step
	for _, env := range []*Env{rule.workflowEnv(), rule.jobEnv(), step.Env} {
		st = st.after(tool.envAssignments(env))
	}
	// the scripts that ran before the install
	var scripts strings.Builder
	if rule.job != nil {
		for _, sp := range rule.job.Steps {
			if sp == step {
				break
			}
			if r, ok := sp.Exec.(*ExecRun); ok && r.Run != nil {
				scripts.WriteString(r.Run.Value)
				scripts.WriteByte('\n')
			}
		}
	}
	scripts.WriteString(before)
	st = st.after(tool.textAssignments(scripts.String()))
	// the options of the command
	return st.after(tool.flagAssignments(c))
}

// ageDefaultOn reports whether the version of the tool waits for a release by default: pnpm 11 (one day), Yarn
// 4.15 (one day) and aube (one day, in every version). A Yarn 2+ project of a version that is not known is taken as such too, which keeps the install
// unreported: the default depends on the version.
func (rule *RuleUnlockedInstall) ageDefaultOn(dirs []string, step *Step, c *runscript.Command, before string) bool {
	switch c.Tool {
	case "aube":
		return true
	case "pnpm":
		if rule.pnpmVersionInJob(step, before) >= 11 {
			return true
		}
		for _, d := range dirs {
			pkg, _ := rule.files.packageJSON(d)
			if major, _, ok := pkg.managerVersion("pnpm"); ok && major >= 11 {
				return true
			}
		}
	case "yarn":
		for _, d := range dirs {
			pkg, _ := rule.files.packageJSON(d)
			_, hasRC := rule.files.read(filepath.Join(d, ".yarnrc.yml"))
			if major, minor, ok := pkg.managerVersion("yarn"); ok {
				if major > 4 || major == 4 && minor >= 15 {
					return true
				}
			} else if hasRC {
				return true
			}
		}
	}
	return false
}

// pnpmNpmrcMode says what the `minimum-release-age` of a .npmrc means for pnpm.
type pnpmNpmrcMode int

const (
	// pnpmNpmrcIgnored: the version of pnpm does not read the key from .npmrc (11 and later read only
	// pnpm-workspace.yaml and their config file; before 10.16 there is no such setting).
	pnpmNpmrcIgnored pnpmNpmrcMode = iota
	// pnpmNpmrcRead: pnpm 10.16 up to 11 read it.
	pnpmNpmrcRead
	// pnpmNpmrcUnknown: the version is not known, so a value that turns the age on is believed and one that turns it
	// off is not, because the version may be 11 and keep its default.
	pnpmNpmrcUnknown
)

// pnpmNpmrcMode decides the mode from the version of pnpm that package.json (packageManager) or the job
// (pnpm/action-setup, pnpm@N in a script) names.
func (rule *RuleUnlockedInstall) pnpmNpmrcMode(dirs []string, step *Step, before string) pnpmNpmrcMode {
	major, minor, known := 0, 0, false
	for _, d := range dirs {
		pkg, _ := rule.files.packageJSON(d)
		if ma, mi, ok := pkg.managerVersion("pnpm"); ok {
			major, minor, known = ma, mi, true
		}
	}
	if !known {
		if ma := rule.pnpmVersionInJob(step, before); ma > 0 {
			// only the major version is named: 10 is taken as one that has the setting
			major, minor, known = ma, 16, true
		}
	}
	switch {
	case !known:
		return pnpmNpmrcUnknown
	case major == 10 && minor >= 16:
		return pnpmNpmrcRead
	}
	return pnpmNpmrcIgnored
}

// ageFiles returns the assignments of the files of the directory that the tool reads, one slice for each file, the
// weakest first.
func (rule *RuleUnlockedInstall) ageFiles(dir string, c *runscript.Command, npmrcMode pnpmNpmrcMode) [][]ageAssignment {
	read := func(name string) (string, bool) { return rule.files.read(filepath.Join(dir, name)) }
	var out [][]ageAssignment
	switch c.Tool {
	case "uv", "uvx":
		if c.HasFlag("--no-config") { // uv reads no uv.toml and no pyproject.toml
			return nil
		}
		if text, ok := read("pyproject.toml"); ok {
			out = append(out, uvAssignments(text, "tool.uv", "tool.uv.pip"))
		}
		if text, ok := read("uv.toml"); ok {
			out = append(out, uvAssignments(text, "", "pip"))
		}
	case "npm":
		rc := rule.files.npmrc(dir)
		var as []ageAssignment
		for _, k := range []string{"min-release-age", "before"} {
			if v, ok := rc[k]; ok {
				as = append(as, ageAssignment{k, v})
			}
		}
		out = append(out, as)
	case "pnpm":
		// up to pnpm 10 .npmrc, in this spelling; pnpm-workspace.yaml wins
		if v, ok := rule.files.npmrc(dir)["minimum-release-age"]; ok && (npmrcMode == pnpmNpmrcRead || npmrcMode == pnpmNpmrcUnknown && ageSettingOn(v)) {
			out = append(out, []ageAssignment{{"minimum-release-age", v}})
		}
		if text, ok := read("pnpm-workspace.yaml"); ok {
			var as []ageAssignment
			for _, m := range pnpmWorkspaceAgeRe.FindAllStringSubmatch(text, -1) {
				as = append(as, ageAssignment{"minimum-release-age", m[1]})
			}
			out = append(out, as)
		}
	case "aube":
		// .npmrc (both spellings of the key), then the workspace file, which wins: aube-workspace.yaml, and the
		// pnpm-workspace.yaml only when there is none
		rc := rule.files.npmrc(dir)
		var as []ageAssignment
		for _, k := range []string{"minimum-release-age", "minimumReleaseAge"} {
			if v, ok := rc[k]; ok {
				as = append(as, ageAssignment{"minimum-release-age", v})
			}
		}
		out = append(out, as)
		text, ok := read("aube-workspace.yaml")
		if !ok {
			text, ok = read("pnpm-workspace.yaml")
		}
		if ok {
			var ws []ageAssignment
			for _, m := range pnpmWorkspaceAgeRe.FindAllStringSubmatch(text, -1) {
				ws = append(ws, ageAssignment{"minimum-release-age", m[1]})
			}
			out = append(out, ws)
		}
	case "yarn":
		if text, ok := read(".yarnrc.yml"); ok {
			var as []ageAssignment
			for _, m := range yarnrcGateRe.FindAllStringSubmatch(text, -1) {
				as = append(as, ageAssignment{"npmMinimalAgeGate", m[1]})
			}
			out = append(out, as)
		}
	case "bun":
		// bunfig.toml only, and only its [install] table
		if text, ok := read("bunfig.toml"); ok {
			var as []ageAssignment
			table := ""
			for _, line := range strings.Split(text, "\n") {
				if m := tomlHeaderRe.FindStringSubmatch(line); m != nil {
					table = m[1]
				} else if m := bunfigAgeRe.FindStringSubmatch(line); m != nil && table == "install" {
					as = append(as, ageAssignment{"minimumReleaseAge", strings.ReplaceAll(m[1], "_", "")})
				}
			}
			out = append(out, as)
		}
	}
	return out
}

// uvAssignments finds the exclude-newer keys of the given tables of a TOML file ("" is the top level).
func uvAssignments(text string, tables ...string) []ageAssignment {
	var out []ageAssignment
	table := ""
	for _, line := range strings.Split(text, "\n") {
		if m := tomlHeaderRe.FindStringSubmatch(line); m != nil {
			table = m[1]
		} else if m := tomlExcludeNewerRe.FindStringSubmatch(line); m != nil {
			for _, t := range tables {
				if table == t {
					out = append(out, ageAssignment{"exclude-newer", strings.Trim(m[1], `"' `)})
				}
			}
		}
	}
	return out
}

// toldNotToFreeze reports whether the command switches the freeze of the lock file off (`--no-frozen-lockfile`,
// `--frozen-lockfile=false`): it then resolves the versions again although the repository has the lock file.
func toldNotToFreeze(c *runscript.Command) bool {
	if c.HasFlag("--no-frozen-lockfile") {
		return true
	}
	// aube freezes by default in CI. These two resolve again, the whole lock file or the entries that drifted
	if c.Tool == "aube" && c.HasFlag("--prefer-frozen-lockfile", "--fix-lockfile") {
		return true
	}
	for _, f := range c.Flags {
		switch f.Name {
		case "--frozen-lockfile", "--immutable", "--ci":
			if f.Value != nil && !f.Value.Dynamic() && !ageSettingOn(f.Value.Value) {
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

// pnpmVersionInJob returns the major version of pnpm that the job names: `pnpm/action-setup` with a version, or a
// script that installs `pnpm@11`. It is 0 when none does. For the step itself only the script before the install
// counts.
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

// aubeResolves reports whether the aube command is one that resolves versions: `aube install` and `aube add`
// (`aube ci` and `aube dlx` are not installs of the project, and the first is frozen).
func aubeResolves(in *runscript.Install) bool {
	switch in.Verb {
	case "install", "i", "add", "a":
		return true
	}
	return false
}
