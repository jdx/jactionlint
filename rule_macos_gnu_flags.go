package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleMacosGnuFlags reports options of the commands in `run:` scripts that exist in the GNU version of the command
// and not in the BSD one that macOS has, in a job that runs on macOS.
//
// The runners of GitHub (and the machines of developers) have the BSD sed, grep, date, stat, find, cp and tee. They
// are rejected by the macOS command with a message such as `tee: illegal option -- -`, which a workflow tested only on
// Linux does not show until a macOS job is added to the matrix. Only forms are reported for which the sources of the
// commands that Apple publishes (apple-oss-distributions: shell_cmds, file_cmds and text_cmds) show that macOS fails:
//
//   - `tee --append` (tee has only -a and -i, and no long option)
//   - `sed -i` followed by the script: the option takes the next word as the suffix of the backup, so the script is
//     taken for the suffix and the file for the script. `sed -i ”` or `sed -i.bak` is what BSD understands. Long
//     options (`--in-place`) do not exist
//   - `date -d` and `date --date` (date has no such option; BSD date reads a time with -f and -v)
//   - `grep -P` and `grep --perl-regexp` (the options of the macOS grep have no P)
//   - `stat -c` and `stat --format` (the format of BSD stat is -f)
//   - `find -printf`, `-regextype` and `-xtype` (they are not in the table of primaries of find)
//   - `cp --parents` (cp has no long options)
//
// Forms that look as GNU-only and work on macOS are not reported: `xargs -r` (a no-op that BSD accepts for
// compatibility), `sort -V`, and `readlink -f` since macOS 12.3.
//
// The job must run on macOS: a `runs-on` label with `macos`, or a matrix value of `runs-on: ${{ matrix.x }}` that is.
// A job on a self-hosted runner is not looked at (its owner can have installed the GNU tools), and neither is a
// command whose failure the script handles (`date -d ... || date -v ...`). A step is skipped
// when it may be meant for another system: its `if:` reads the runner or the matrix, or its script mentions the
// system or GNU tools (`RUNNER_OS`, `uname`, `gsed`, `brew`, ...).
type RuleMacosGnuFlags struct {
	RuleBase
	runContext
	macos string // the label that makes the job run on macOS, "" when the job does not
}

// NewRuleMacosGnuFlags creates a new RuleMacosGnuFlags instance.
func NewRuleMacosGnuFlags() *RuleMacosGnuFlags {
	return &RuleMacosGnuFlags{
		RuleBase: NewRuleBase("macos-gnu-flags", "Checks for GNU-only options of commands at \"run:\" in jobs that run on macOS"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleMacosGnuFlags) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleMacosGnuFlags) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	rule.macos = macosLabelOf(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleMacosGnuFlags) VisitJobPost(n *Job) error {
	rule.leaveJob()
	rule.macos = ""
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleMacosGnuFlags) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || rule.macos == "" || run.Run == nil || rule.mayBeForAnotherSystem(n, run) {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	portable := portableTools(s)
	for _, c := range s.Commands {
		if (c.Tested && !c.AndOnly) || portable[c.Name] {
			// the script handles the failure (`if grep -P ...`, `date -d ... || date -v ...`: not the left side of &&,
			// whose failure is the status of the list) or has the form of macOS too
			continue
		}
		for _, f := range macosGnuForms(c) {
			start, end := scriptPos(s, origin, f.word.Offset), scriptPos(s, origin, f.word.End)
			e := rule.errorIDAt("macos-gnu-flags", start, f.message+" The job runs on macOS ("+rule.macos+"), whose "+c.Name+" is the BSD one. "+f.hint)
			if end != nil {
				e.endAt(end)
			}
		}
	}
	return nil
}

// macosSystemRe matches what a script says when it picks the system or installs the GNU tools.
var macosSystemRe = regexp.MustCompile(`(?i)\brunner[._]os\b|\buname\b|\bostype\b|\bdarwin\b|\bmacos\b|\bsw_vers\b|\bgnu\b|\bgnu-|\bcoreutils\b|\bfindutils\b|\bg(?:sed|grep|date|stat|find|tee|cp|xargs)\b|\bbrew\b|\bnix\b`)

// mayBeForAnotherSystem reports whether the step may not run on macOS, or may run with GNU tools.
func (rule *RuleMacosGnuFlags) mayBeForAnotherSystem(n *Step, run *ExecRun) bool {
	if n.If != nil {
		c := strings.ToLower(n.If.Value)
		if strings.Contains(c, "runner.") || strings.Contains(c, "matrix.") || strings.Contains(c, "matrix[") || strings.Contains(c, "runner[") {
			return true
		}
	}
	if rule.job != nil && rule.job.Container != nil {
		return true // the steps run in a Linux container
	}
	return macosSystemRe.MatchString(run.Run.Value)
}

// macosLabelOf returns the label of the runner that makes the job run on macOS: a `macos` label of `runs-on`, or a
// value of the matrix that `runs-on: ${{ matrix.x }}` takes. It returns "" for a job that is not known to run on macOS
// and for a job of a self-hosted runner.
func macosLabelOf(j *Job) string {
	if j == nil || j.RunsOn == nil || j.Container != nil {
		return ""
	}
	var m *Matrix
	if j.Strategy != nil {
		m = j.Strategy.Matrix
	}
	var labels []string
	for _, l := range j.RunsOn.Labels {
		if l == nil {
			continue
		}
		if strings.EqualFold(l.Value, "self-hosted") {
			return ""
		}
		if !l.ContainsExpression() {
			labels = append(labels, l.Value)
		} else if vs, ok := matrixLabelValues(l, m); ok {
			labels = append(labels, runnersAfterExclude(vs, l, m)...)
		}
	}
	if j.RunsOn.LabelsExpr != nil {
		if vs, ok := matrixLabelValues(j.RunsOn.LabelsExpr, m); ok {
			labels = append(labels, runnersAfterExclude(vs, j.RunsOn.LabelsExpr, m)...)
		}
	}
	// A runs-on of one label that the matrix chooses makes one job for each value, so a value self-hosted is a job of
	// its own and the GitHub-hosted macOS value is another one. With several labels the values are labels of the same
	// job, and a self-hosted one makes the runner the one of its owner.
	if len(j.RunsOn.Labels)+boolInt(j.RunsOn.LabelsExpr != nil) > 1 {
		for _, l := range labels {
			if strings.EqualFold(l, "self-hosted") {
				return ""
			}
		}
	}
	for _, l := range labels {
		if isMacOSRunnerLabel(l) {
			return l
		}
	}
	return ""
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// matrixKeyOfLabel returns the lower case key of `runs-on: ${{ matrix.key }}` (or `matrix['key']`), read like
// matrixLabelValues does so that the values and the exclude entries are about the same key.
func matrixKeyOfLabel(label *String) (string, bool) {
	if label == nil || !label.IsExpressionAssigned() {
		return "", false
	}
	exprs, ok := label.templateExprs()
	if !ok || len(exprs) != 1 {
		return "", false
	}
	chain, _, ok := chainOf(exprs[0])
	if !ok || len(chain) != 2 || chain[0] != "matrix" {
		return "", false
	}
	return strings.ToLower(chain[1]), true
}

// runnersAfterExclude drops the values of `runs-on: ${{ matrix.key }}` that no job gets: a value of the row which an
// `exclude` entry with only that key removes. A value that `include` adds is a new job and stays, and when an
// `exclude` has an expression the values of the row are not known, so only the ones of `include` are kept.
func runnersAfterExclude(vs []string, label *String, m *Matrix) []string {
	if m == nil || m.Exclude == nil {
		return vs
	}
	key, ok := matrixKeyOfLabel(label)
	if !ok {
		return vs
	}
	removed, included := map[string]bool{}, map[string]bool{}
	unknown := m.Exclude.Expression != nil
	for _, c := range m.Exclude.Combinations {
		if c.Expression != nil {
			unknown = true
			continue
		}
		for k, a := range c.Assigns {
			if strings.ToLower(k) != key {
				continue
			}
			if str, ok := a.Value.(*RawYAMLString); ok {
				if ContainsExpression(str.Value) {
					unknown = true
				} else if len(c.Assigns) == 1 {
					removed[str.Value] = true
				}
			}
		}
	}
	if m.Include != nil {
		for _, c := range m.Include.Combinations {
			for k, a := range c.Assigns {
				if str, ok := a.Value.(*RawYAMLString); ok && strings.ToLower(k) == key {
					included[str.Value] = true
				}
			}
		}
	}
	var out []string
	for _, v := range vs {
		if included[v] || !(unknown || removed[v]) {
			out = append(out, v)
		}
	}
	return out
}

// isMacOSRunnerLabel reports whether the label is a GitHub-hosted macOS runner: macos-latest, macos-14, macos-15-intel,
// macos-26-xlarge, xcode-27, ... A label such as macOS-ARM64 is the label of a self-hosted runner.
func isMacOSRunnerLabel(l string) bool {
	l = strings.ToLower(l)
	return (strings.HasPrefix(l, "macos-") || strings.HasPrefix(l, "xcode-")) && isGitHubHostedLabel(l)
}

// portableTools returns the names of the commands which the script also calls in a form that is the one of macOS, as in
// `date -j -f ... "$x" || date -d "$x"` or when a script prints `stat -c` and `stat -f` to see which one works: the author
// handles both systems, so the GNU form is not a mistake.
func portableTools(s *runscript.Script) map[string]bool {
	out := map[string]bool{}
	for _, c := range s.Commands {
		for i, w := range c.Args {
			if w == nil {
				continue
			}
			switch c.Name {
			case "date":
				if dateBSDOptionRe.MatchString(w.Value) {
					out["date"] = true
				}
			case "stat":
				if w.Value == "-f" || strings.HasPrefix(w.Value, "-f") && len(w.Value) > 2 && w.Value[2] == '%' {
					out["stat"] = true
				}
			case "sed":
				if sedInPlaceRe.MatchString(w.Value) && i+1 < len(c.Args) && c.Args[i+1] != nil && !c.Args[i+1].Dynamic() && isSedSuffix(c.Args[i+1].Value) {
					out["sed"] = true
				}
			case "tee":
				if w.Value == "-a" {
					out["tee"] = true
				}
			}
		}
	}
	return out
}

// isSedSuffix reports whether the word is what BSD sed takes as the suffix of the backup of -i: the empty string,
// `.bak`, `~` or a word of letters (`bak`, `orig`). A word like `1d` is a script.
func isSedSuffix(v string) bool {
	return v == "" || sedSuffixRe.MatchString(v)
}

// macosGnuForm is one GNU-only form in a command.
type macosGnuForm struct {
	word    *runscript.Word
	message string
	hint    string
}

var (
	// sedInPlaceRe is a word that is the in-place option of sed, alone or last of a cluster (-i, -Ei, -ni).
	sedInPlaceRe = regexp.MustCompile(`^-[A-Za-z]*i$`)
	// dateBSDOptionRe is an option that only the date of macOS has: -j (also in a cluster, -jf or -uj) and -v with or
	// without its adjustment (-v, -v-1d, -v+1H).
	dateBSDOptionRe = regexp.MustCompile(`^-(?:[A-Za-z]*j[A-Za-z]*|[A-Za-z]*v(?:[-+].*)?)$`)
	// dateOptionRe is the -d option of GNU date, alone or with its value attached (-d@1700000000, -ud).
	dateOptionRe = regexp.MustCompile(`^-u?d`)
	// statFormatRe is the -c option of GNU stat, alone or with its value attached (-c%s, -Lc).
	statFormatRe = regexp.MustCompile(`^-[LFlnqrsx]*c`)
	// sedSuffixRe is a word that BSD sed takes as the suffix of the backup: ".bak", ".orig", "~", "bak".
	sedSuffixRe = regexp.MustCompile(`^(?:\.[A-Za-z0-9_~.-]*|~|[A-Za-z]{3,})$`)
)

// macosGnuForms returns the GNU-only forms of the command.
func macosGnuForms(c *runscript.Command) []macosGnuForm {
	var out []macosGnuForm
	add := func(w *runscript.Word, msg, hint string) {
		out = append(out, macosGnuForm{word: w, message: msg, hint: hint})
	}
	for i, w := range c.Args {
		if w == nil {
			continue
		}
		v := w.Value // without the quotes: sed "-i" is sed -i
		if c.Name == "grep" && i > 0 && c.Args[i-1] != nil && grepValueFlags[c.Args[i-1].Value] {
			continue // the value of an option: grep -e -P
		}
		switch c.Name {
		case "tee":
			if v == "--append" {
				add(w, `"tee --append" is rejected by the tee of macOS ("illegal option -- -"): it has only short options.`, `Use "tee -a".`)
			}
		case "sed":
			switch {
			case strings.HasPrefix(v, "--in-place") || v == "--in-place":
				add(w, `"sed --in-place" is rejected by the sed of macOS: it has no long options.`, `Use "sed -i.bak" (and remove the backup), which both seds understand.`)
			case sedInPlaceRe.MatchString(v) && i+1 < len(c.Args):
				next := c.Args[i+1]
				if _, plain := next.Var(); plain || strings.HasPrefix(next.Value, "-") || (!next.Dynamic() && (next.Value == "" || sedSuffixRe.MatchString(next.Value))) {
					continue // a suffix (-i '' or -i .bak), a variable that may be one, or an option that makes it unclear what follows
				}
				add(w, `"sed -i" without a suffix takes the script as the suffix of the backup on macOS, so the file is read as the script ("invalid command code").`, `Use "sed -i.bak" (and remove the backup), which both seds understand, or "sed -i ''" on macOS only.`)
			}
		case "date":
			if dateOptionRe.MatchString(v) || v == "--date" || strings.HasPrefix(v, "--date=") {
				add(w, `"date -d" does not exist in the date of macOS ("illegal option -- d").`, `Use "date -j -f FORMAT ..." or "date -v", or install coreutils and call "gdate".`)
			}
		case "grep":
			if v == "--perl-regexp" || (strings.HasPrefix(v, "-") && !strings.HasPrefix(v, "--") && len(v) > 1 && strings.Contains(v[1:], "P") && clusterOfFlags(v)) {
				add(w, `"grep -P" is rejected by the grep of macOS ("invalid option -- P"): it has no Perl regular expressions.`, `Use "grep -E" if the pattern allows it, or "perl -ne", which macOS has too.`)
			}
		case "stat":
			if statFormatRe.MatchString(v) || v == "--format" || strings.HasPrefix(v, "--format=") || v == "--printf" || strings.HasPrefix(v, "--printf=") {
				add(w, `"stat -c" does not exist in the stat of macOS: its format option is -f.`, `Use "stat -f FORMAT" on macOS, or read the value in another way (wc -c < file for the size).`)
			}
		case "find":
			if v == "-printf" || v == "-regextype" || v == "-xtype" {
				add(w, `"find `+v+`" is not a primary of the find of macOS ("unknown primary or operator").`, `Use "-print" with other commands, or "-exec stat -f", or install findutils and call "gfind".`)
			}
		case "cp":
			if v == "--parents" {
				add(w, `"cp --parents" is rejected by the cp of macOS: it has no long options.`, `Use "mkdir -p" for the directory and "cp" for the file.`)
			}
		}
	}
	return out
}

// grepValueFlags are the options of grep which take the next word as a value.
var grepValueFlags = map[string]bool{"-e": true, "-f": true, "-m": true, "-A": true, "-B": true, "-C": true, "-d": true, "-D": true,
	"--regexp": true, "--file": true, "--max-count": true, "--include": true, "--exclude": true}

// clusterOfFlags reports whether a word like -oP or -rnP is a cluster of single letters of grep, not an option with a
// value glued to it (-A3, -m1, -ePATTERN, -fFILE, -d recurse) in which a P can be part of the value.
func clusterOfFlags(w string) bool {
	for _, r := range w[1:] {
		switch r {
		case 'A', 'B', 'C', 'D', 'd', 'e', 'f', 'm':
			// An option with a value: what follows is the value, a P in it is not an option
			return strings.IndexRune(w[1:], 'P') < strings.IndexRune(w[1:], r)
		}
	}
	return true
}

func init() {
	registerRules(
		RuleInfo{ID: "macos-gnu-flags", Group: RuleGroupCorrectness, Summary: "A command at run: has a GNU-only option and the job runs on macOS, which has the BSD command.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-macos-gnu-flags"},
	)
	registerRuleFactory("macos-gnu-flags", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleMacosGnuFlags()}
	})
}
