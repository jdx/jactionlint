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
	// cooldownFlagRe matches the options which set a release age or a date limit.
	cooldownFlagRe = regexp.MustCompile(`(?i)^--(min(imum)?-release-age|exclude-newer(-package)?|uploaded-prior-to|before|npm-minimal-age-gate|config\.minimum.*)$`)
	// cooldownTextRe finds the setting by name in the text of a script or the value of an option (`--pip-args`).
	cooldownTextRe = regexp.MustCompile(`(?i)(min(imum)?[-_]release[-_]age|exclude[-_]newer|uploaded[-_]prior[-_]to|npmMinimalAgeGate|npm_config_before|config set before\b|--before\b)`)
	// cooldownEnvRe matches the (lower case) name of an environment variable which sets one.
	cooldownEnvRe = regexp.MustCompile(`(min(imum)?_release_age|exclude_newer|uploaded_prior_to|^npm_config_before$|minimal_age_gate)`)

	pnpmWorkspaceAgeRe = regexp.MustCompile(`(?m)^minimumReleaseAge:\s*["']?(\d+)`)
	bunfigAgeRe        = regexp.MustCompile(`(?m)^\s*minimumReleaseAge\s*=\s*(\d[\d_]*)`)
	yarnrcGateRe       = regexp.MustCompile(`(?m)^npmMinimalAgeGate:\s*["']?([^"'\s#]+)`)
	pnpmVersionTextRe  = regexp.MustCompile(`pnpm@\^?(\d+)`)
	tomlHeaderRe       = regexp.MustCompile(`^\s*\[+\s*([^\]]*?)\s*\]+`)
	tomlExcludeNewerRe = regexp.MustCompile(`^\s*exclude-newer\s*=\s*(.+?)\s*(#.*)?$`)
)

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
	for _, f := range c.Flags {
		if cooldownFlagRe.MatchString(f.Name) || f.Value != nil && cooldownTextRe.MatchString(f.Value.Value) {
			return
		}
	}
	if c.HasFlag("--project", "--directory", "--config-file", "--no-config") && c.Tool == "uv" {
		return
	}
	dirs, ok := rule.commandDirs(rule.root, step, s, c)
	if !ok {
		return
	}
	if rule.cooldownSetByWorkflow(step) {
		return
	}
	if in.Ecosystem == "npm" && in.FromManifest && (rule.root == "" || rule.hasAnyFile(dirs, lockFilesOf[c.Tool])) {
		// nothing is resolved when the lock file decides the versions, and without the repository it is not known
		// whether there is one
		return
	}
	if rule.cooldownSetByRepository(dirs, step, c) {
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

// cooldownSetByWorkflow reports whether the environment of the step or an earlier script of the job sets one.
func (rule *RuleUnlockedInstall) cooldownSetByWorkflow(step *Step) bool {
	for _, env := range []*Env{step.Env, rule.jobEnv(), rule.workflowEnv()} {
		if env == nil {
			continue
		}
		for name := range env.Vars {
			if cooldownEnvRe.MatchString(name) {
				return true
			}
		}
	}
	return rule.jobScriptMentions(step, cooldownTextRe)
}

// cooldownSetByRepository reports whether the files of the repository, or the version of the tool, give a cooldown
// to the command.
func (rule *RuleUnlockedInstall) cooldownSetByRepository(dirs []string, step *Step, c *runscript.Command) bool {
	for _, d := range dirs {
		pkg, _ := rule.files.packageJSON(d)
		switch c.Tool {
		case "uv", "uvx":
			if rule.uvExcludesNewer(d) {
				return true
			}
		case "npm":
			if npmrcHasAge(rule.files.npmrc(d)) {
				return true
			}
		case "pnpm":
			if text, ok := rule.files.read(filepath.Join(d, "pnpm-workspace.yaml")); ok {
				if m := pnpmWorkspaceAgeRe.FindStringSubmatch(text); m != nil && m[1] != "0" {
					return true
				}
			}
			if npmrcHasAge(rule.files.npmrc(d)) {
				return true
			}
			if major, _, ok := pkg.managerVersion("pnpm"); ok && major >= 11 {
				return true // pnpm 11 waits one day by default
			}
			if rule.pnpmVersionInJob(step) >= 11 {
				return true
			}
		case "yarn":
			if rule.yarnHasAgeGate(d, pkg) {
				return true
			}
		case "bun":
			if text, ok := rule.files.read(filepath.Join(d, "bunfig.toml")); ok {
				if m := bunfigAgeRe.FindStringSubmatch(text); m != nil && strings.Trim(m[1], "0_") != "" {
					return true
				}
			}
			if npmrcHasAge(rule.files.npmrc(d)) {
				return true
			}
		}
	}
	return false
}

// npmrcHasAge reports whether the .npmrc asks for a number of days, or a date.
func npmrcHasAge(rc map[string]string) bool {
	if v, ok := rc["min-release-age"]; ok {
		if n, err := strconv.ParseFloat(v, 64); err != nil || n > 0 {
			return true
		}
	}
	if v, ok := rc["minimum-release-age"]; ok && v != "" && v != "0" {
		return true
	}
	return rc["before"] != ""
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
func (rule *RuleUnlockedInstall) pnpmVersionInJob(step *Step) int {
	if rule.job == nil {
		return 0
	}
	best := 0
	for _, st := range rule.job.Steps {
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
				for _, m := range pnpmVersionTextRe.FindAllStringSubmatch(e.Run.Value, -1) {
					if n, err := strconv.Atoi(m[1]); err == nil && n > best {
						best = n
					}
				}
			}
		}
		if st == step {
			break
		}
	}
	return best
}
