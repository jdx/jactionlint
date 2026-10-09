package jactionlint

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleUnlockedInstall detects installations in `run:` scripts that do not use a lock file. Without one, the
// dependencies are resolved anew on every run, so a new (or compromised) release of any transitive dependency
// reaches the workflow.
//
// It reports, with the one ID "unlocked-install":
//
//   - `cargo install` without --locked. The flag is the documented way to use the Cargo.lock that the crate was
//     published with. This is the one finding that has a fix.
//   - installs from a manifest that are not bound to the lock file of the repository: `npm install` instead of
//     `npm ci`, `yarn install` without --immutable and `pnpm install --no-frozen-lockfile`. They are reported
//     when the repository has the lock file (the project root, or the working-directory of the step, has
//     package-lock.json, yarn.lock or pnpm-lock.yaml), because only then the lock file is being ignored. Without
//     the lock file there is nothing to bind to.
//   - with the option "pedantic", the same installs without a lock file in the repository, `bun install` without a
//     frozen lock file, and `pip install -r` without hashes or constraints.
type RuleUnlockedInstall struct {
	RuleBase
	runContext
	src  *fileOffsets
	root string // the root directory of the project, empty when the file is not in one
	step *Step
}

// NewRuleUnlockedInstall creates a new RuleUnlockedInstall instance. The source is the content of the file; it is
// needed to build the fix and can be nil, in which case no fix is attached. The root is the root directory of the
// project, where the lock files are looked up; it can be empty.
func NewRuleUnlockedInstall(src []byte, root string) *RuleUnlockedInstall {
	r := &RuleUnlockedInstall{
		RuleBase: NewRuleBase("unlocked-install", "Checks for installations that are not bound to a lock file at \"run:\""),
	}
	if src != nil {
		r.src = newFileOffsets(src)
	}
	r.root = root
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnlockedInstall) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnlockedInstall) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleUnlockedInstall) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleUnlockedInstall) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	rule.step = n
	for _, c := range s.Commands {
		in := c.Installs()
		if in == nil {
			continue
		}
		rule.checkCargo(s, origin, c, in)
		rule.checkManifest(s, origin, c, in)
	}
	return nil
}

func (rule *RuleUnlockedInstall) checkCargo(s *runscript.Script, origin runscript.Origin, c *runscript.Command, in *runscript.Install) {
	if in.Ecosystem != "crates" || in.Verb != "install" || in.Locked || len(in.Packages) == 0 {
		return
	}
	start, end := commandRange(s, origin, c)
	e := rule.errorIDAt("unlocked-install", start, `"cargo install" without --locked builds with the newest dependencies that match the crate instead of the ones in its Cargo.lock, so a new release of any dependency reaches the workflow. add --locked`).endAt(end)
	if edit, ok := rule.insertAfter(s, origin, c.Positional[0], "install", " --locked"); ok {
		e.Fix = &Fix{
			// The build can fail where it succeeded before, when the lock file of the crate is out of date, so
			// the fix is not applied unless it is asked for.
			Description: "Add --locked",
			Unsafe:      true,
			Edits:       []TextEdit{edit},
		}
	}
}

// insertAfter returns the edit which inserts text right after the word in the file. The word must be a plain,
// unquoted word with the given value, and the text of the file at the computed position must be that word:
// positions inside scalars that are not literal blocks can be off, and a wrong edit is worse than none.
func (rule *RuleUnlockedInstall) insertAfter(s *runscript.Script, origin runscript.Origin, w *runscript.Word, value, text string) (TextEdit, bool) {
	if rule.src == nil || w.Raw != value || w.Value != value {
		return TextEdit{}, false
	}
	p := s.Position(origin, w.End)
	off, ok := rule.src.offset(p.Line, p.Col)
	if !ok || off < len(value) || string(rule.src.src[off-len(value):off]) != value {
		return TextEdit{}, false
	}
	return TextEdit{Start: off, End: off, NewText: text}, true
}

// reportsInstall reports whether an install of a package manager that is not bound to the lock file is a finding:
// when the repository has one of the lock files, or when the pedantic findings are on.
func (rule *RuleUnlockedInstall) reportsInstall(lockFiles ...string) bool {
	if rule.pedantic("unlocked-install") {
		return true
	}
	if rule.root == "" {
		return false
	}
	dirs := []string{rule.root}
	if rule.step != nil {
		if run, ok := rule.step.Exec.(*ExecRun); ok && run.WorkingDirectory != nil && !run.WorkingDirectory.ContainsExpression() {
			if wd := filepath.FromSlash(strings.TrimSpace(run.WorkingDirectory.Value)); wd != "" && !filepath.IsAbs(wd) && filepath.IsLocal(wd) {
				dirs = append(dirs, filepath.Join(rule.root, wd))
			}
		}
	}
	for _, d := range dirs {
		for _, f := range lockFiles {
			if st, err := os.Stat(filepath.Join(d, f)); err == nil && !st.IsDir() {
				return true
			}
		}
	}
	return false
}

func (rule *RuleUnlockedInstall) checkManifest(s *runscript.Script, origin runscript.Origin, c *runscript.Command, in *runscript.Install) {
	switch in.Ecosystem {
	case "npm":
		if in.Run || in.Global || len(in.Packages) > 0 || len(in.Requirements) > 0 {
			return
		}
		if c.HasFlag("--package-lock-only", "--lockfile-only") {
			return
		}
		name := c.Name + " " + in.Verb
		if in.Verb == "" || c.Tool == "yarn" && c.Sub(0) == "" {
			name = c.Name
		}
		start, end := commandRange(s, origin, c)
		if c.Tool == "pnpm" {
			// pnpm freezes the lock file by default in CI, so only an explicit opt-out is reported
			f := c.Flag("--frozen-lockfile")
			if c.HasFlag("--no-frozen-lockfile") || (f != nil && f.Value != nil && f.Value.Value == "false") {
				if rule.reportsInstall("pnpm-lock.yaml") {
					rule.errorIDAt("unlocked-install", start, `"pnpm install" is told not to freeze the lock file, so it may update pnpm-lock.yaml instead of failing when it is out of date. pass --frozen-lockfile`).endAt(end)
				}
			}
			return
		}
		if in.Locked {
			return
		}
		switch c.Tool {
		case "npm":
			if rule.reportsInstall("package-lock.json", "npm-shrinkwrap.json") {
				rule.errorIDAt("unlocked-install", start, quote(name)+" resolves the dependencies again and may update the package-lock.json instead of failing when it is out of date. use `npm ci`").endAt(end)
			}
		case "yarn":
			if rule.reportsInstall("yarn.lock") {
				rule.errorIDAt("unlocked-install", start, quote(name)+" does not require the yarn.lock to be up to date in Yarn 1, and relies on the CI detection of Yarn 2+. pass `--immutable` (`--frozen-lockfile` for Yarn 1)").endAt(end)
			}
		case "bun":
			if rule.pedantic("unlocked-install") {
				rule.errorIDAt("unlocked-install", start, quote(name)+" may update bun.lock instead of failing when it is out of date. use `bun ci` or pass `--frozen-lockfile`").endAt(end)
			}
		}
	case "pypi":
		start, end := commandRange(s, origin, c)
		if in.Run || in.Locked || len(in.Requirements) == 0 || len(in.Packages) > 0 {
			return
		}
		if c.HasFlag("-c", "--constraint") {
			return
		}
		if in.Verb != "install" {
			return
		}
		cmd := c.Name
		switch {
		case c.Tool == "uv":
			cmd = "uv pip"
		case !strings.HasPrefix(c.Name, "pip"):
			cmd = "python -m pip"
		}
		var req []string
		for _, r := range in.Requirements {
			req = append(req, r.Value)
		}
		if rule.pedantic("unlocked-install") {
			rule.errorIDAt("unlocked-install", start, quote(strings.TrimSpace(cmd+" install -r "+strings.Join(req, " -r ")))+" installs a requirements file without hashes or constraints, so the transitive dependencies are resolved anew on every run. use a lock file with hashes (`pip-compile --generate-hashes`) and pass --require-hashes, or pin them with -c").endAt(end)
		}
	}
}

func init() {
	registerRules(
		RuleInfo{ID: "unlocked-install", Group: RuleGroupSecurity, Summary: "cargo install runs without --locked, or npm, yarn or pnpm install without freezing a lock file that the repository has.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-unlocked-install", Options: []RuleOption{pedanticOption}},
	)
	registerRuleFactory("unlocked-install", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("unlocked-install") {
			return nil
		}
		root := ""
		if env.project != nil {
			root = env.project.RootDir()
		}
		return []Rule{NewRuleUnlockedInstall(env.Source(), root)}
	})
}
