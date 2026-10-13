package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleArgumentInjection detects free text that an outsider writes and that a script puts into the program text or
// the options of a command that can run code or write files with them.
//
//   - `sed "s/PLACEHOLDER/$BODY/g"`: a value with a slash ends the command and the rest is a program of its own.
//     GNU sed runs a command with the `e` command and the `e` flag of `s`, and `w` and `r` write and read files, so
//     `a/g;1e id;#` runs `id`. The text of an `awk` program is the same (`system()`), and so is the command that
//     `find -exec` runs, and an unquoted value that the shell splits into words can add `-exec` to a `find`.
//   - `wget`, `curl` and `git clone` take a quoted value as one word, and a value that starts with a dash is an option:
//     `-o`, `--output`, `--upload-pack`. The value must be the whole word or start it, it must be a word that is not the
//     value of an option, and nothing may end the options with `--` before it. Commands whose options the rule does not
//     know (tar, zip, rsync) are left out: it cannot tell their operands from the values of their options.
//
// Only free text counts: a body, a comment, a title, a commit message, a name an author chose. The name of a branch,
// of a label or of an account is left out, as in the models of CodeQL (actions/ql/lib/ext/config/argument_injection_sinks.yml
// has the commands): git refuses a branch that starts with a dash, and the characters of an account name are fixed.
// The value is judged like in command-injection, with the strictest charset (letters, digits and a dash) as the only
// safe shape, because a dot and a slash are enough to break a sed program.
type RuleArgumentInjection struct {
	RuleBase
	runContext
	flow *RuleGitHubEnv
}

// NewRuleArgumentInjection creates a new RuleArgumentInjection instance.
func NewRuleArgumentInjection() *RuleArgumentInjection {
	return &RuleArgumentInjection{
		RuleBase: NewRuleBase("argument-injection", "Checks for free text of an outsider in the program text or the options of sed, awk, find, curl, wget and git clone"),
		// GITHUB_PATH: only a value cut down to letters, digits, "_" and "-" is safe (a dot and a slash are not)
		flow: &RuleGitHubEnv{dest: "GITHUB_PATH", varsOnly: true, skipInputs: true, freeTextOnly: true},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleArgumentInjection) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleArgumentInjection) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleArgumentInjection) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleArgumentInjection) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	rule.flow.runContext = rule.runContext
	rule.flow.step = n
	rule.flow.errexit = rule.shellErrexit(run)
	for _, c := range s.Commands {
		if c.Decl || c.Name == "" {
			continue
		}
		w, d, kind := rule.sinkOf(s, c)
		if w == nil {
			continue
		}
		rule.errorIDAt("argument-injection", scriptPos(s, origin, c.Offset), argumentMessage(c, d, kind)).endAt(scriptPos(s, origin, w.End))
	}
	return nil
}

// judge judges a word of the command at its place. Only free text of an outsider counts.
func (rule *RuleArgumentInjection) judge(s *runscript.Script, c *runscript.Command, w *runscript.Word) data {
	f := rule.flow
	f.at = c.Offset
	f.loop = c.LoopBody || c.InFunc
	d := f.judgeWord(s, w, 0)
	if d.kind != dataUntrusted {
		return data{}
	}
	return d
}

// gitGlobalValueFlags are the options of git before the subcommand that take the next word as their value.
var gitGlobalValueFlags = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--super-prefix": true, "--config-env": true, "--exec-path": false}

// gitCloneValueFlags are the options of git clone that take the next word as their value.
var gitCloneValueFlags = map[string]bool{
	"-b": true, "--branch": true, "-o": true, "--origin": true, "-c": true, "--config": true, "--depth": true, "-j": true, "--jobs": true,
	"--reference": true, "--reference-if-able": true, "--separate-git-dir": true, "--template": true, "-u": true, "--upload-pack": true,
	"--server-option": true, "--filter": true, "--shallow-since": true, "--shallow-exclude": true, "--bundle-uri": true,
	"--ref-format": true, "--revision": true,
}

// optionOperands returns the words of the command that are neither an option nor the value of one and come before `--`:
// the words where a value that starts with a dash is read as an option. Only the commands whose options are known are
// covered: curl and wget (the tables of the run-script analyzer) and `git clone`; for any other command it returns
// nothing, because a word that is the value of an option that the rule does not know would look like an operand.
func optionOperands(c *runscript.Command) []*runscript.Word {
	var words []*runscript.Word
	switch {
	case c.Name == "curl" || c.Name == "wget":
		words = c.Positional
	case c.Name == "git":
		words = gitCloneOperands(c)
	}
	end := -1
	for _, a := range c.Args {
		if !a.Dynamic() && a.Value == "--" {
			end = a.Offset
			break
		}
	}
	var out []*runscript.Word
	for _, w := range words {
		if end < 0 || w.Offset < end {
			out = append(out, w)
		}
	}
	return out
}

// gitCloneOperands returns the operands of `git clone`: the repository and the directory, after the options of git and
// of clone with their values. It returns nothing for another subcommand.
func gitCloneOperands(c *runscript.Command) []*runscript.Word {
	args := c.Args
	i := 0
	for ; i < len(args); i++ {
		v := args[i].Value
		if args[i].Dynamic() {
			return nil
		}
		if gitGlobalValueFlags[v] {
			i++
			continue
		}
		if strings.HasPrefix(v, "-") {
			continue
		}
		break
	}
	if i >= len(args) || args[i].Value != "clone" {
		return nil
	}
	var out []*runscript.Word
	for i++; i < len(args); i++ {
		w := args[i]
		v := w.Value
		switch {
		case !w.Dynamic() && v == "--":
			out = append(out, args[i+1:]...) // the operands after -- are cut off by the caller
			return out
		case !w.Dynamic() && strings.HasPrefix(v, "-"):
			if gitCloneValueFlags[v] {
				i++
			}
		default:
			out = append(out, w)
		}
	}
	return out
}

// sinkOf returns the word of the command that holds free text of an outsider in a place that can run code, with the
// kind of the place.
func (rule *RuleArgumentInjection) sinkOf(s *runscript.Script, c *runscript.Command) (*runscript.Word, data, string) {
	switch c.Name {
	case "sed", "gsed":
		for _, w := range sedPrograms(c) {
			if d := rule.judge(s, c, w); d.kind == dataUntrusted {
				return w, d, "sed"
			}
		}
	case "awk", "gawk", "mawk", "nawk":
		for _, w := range awkPrograms(c) {
			if d := rule.judge(s, c, w); d.kind == dataUntrusted {
				return w, d, "awk"
			}
		}
	case "find":
		for i, w := range c.Args {
			switch w.Value {
			case "-exec", "-execdir", "-ok", "-okdir":
				if i+1 < len(c.Args) {
					if d := rule.judge(s, c, c.Args[i+1]); d.kind == dataUntrusted {
						return c.Args[i+1], d, "find-exec"
					}
				}
			}
			if isUnquotedExpansion(w) {
				if d := rule.judge(s, c, w); d.kind == dataUntrusted {
					return w, d, "find-split"
				}
			}
		}
	}
	for _, w := range optionOperands(c) {
		if !startsWithVar(w) || !rule.isEnvValue(s, c, w) {
			continue
		}
		if d := rule.judge(s, c, w); d.kind == dataUntrusted {
			return w, d, "option"
		}
	}
	return nil, data{}, ""
}

// nameProps are the last properties of the untrusted inputs that are the name of a branch, of a label or of an account.
var nameProps = map[string]bool{"ref": true, "head_ref": true, "head_branch": true, "default_branch": true, "label": true, "login": true}

// exprIsName reports whether every untrusted input that the expression reads is the name of a branch, of a label or of
// an account.
func exprIsName(text string) bool {
	n := parseExprText(text)
	if n == nil {
		return false
	}
	var refs []exprRef
	collectExprRefs(n, nil, &refs)
	untrusted := false
	for _, r := range refs {
		if !nodeReadsUntrustedInput(r.node) {
			continue
		}
		untrusted = true
		if !nameProps[r.chain[len(r.chain)-1]] {
			return false
		}
	}
	return untrusted
}

var reLeadingVar = regexp.MustCompile(`^["']*\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// isEnvValue reports whether the first characters of the word are the value that the step's environment gave to a variable
// that the script did not change: a value that went through a command (`grep -o 'https://...'`) can start with
// something else than what the outsider typed, and the rule does not know what.
func (rule *RuleArgumentInjection) isEnvValue(s *runscript.Script, c *runscript.Command, w *runscript.Word) bool {
	m := reLeadingVar.FindStringSubmatch(w.Raw)
	if m == nil {
		return false
	}
	for _, a := range s.Assignments {
		// `VAR=value cmd` only sets the environment of cmd: it does not change what a later command expands
		if a.Name == m[1] && a.Offset < c.Offset && (a.Cmd == nil || a.Cmd.Decl) {
			return false
		}
	}
	_, ok := rule.flow.envValue(rule.flow.step, m[1])
	return ok
}

// startsWithVar reports whether the word is a variable, or starts with one: the value is then the first characters
// of the word, where an option has its dash.
func startsWithVar(w *runscript.Word) bool {
	r := strings.TrimLeft(w.Raw, `"'`)
	return strings.HasPrefix(r, "$") && !strings.HasPrefix(r, "$(") && !strings.HasPrefix(r, "$((")
}

// isUnquotedExpansion reports whether some expansion of the word is outside of double quotes, so that the shell splits its
// value into words. Quoting is judged for each fragment: `"$X"` and `"a$X"` are quoted, `$X""`, `"a"$X` and `${X}”` are not.
func isUnquotedExpansion(w *runscript.Word) bool {
	if len(w.Vars) == 0 && !w.Subst {
		return false
	}
	raw := w.Raw
	dq, sq := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case sq:
			sq = c != '\''
		case c == '\\':
			i++ // an escaped character is no expansion and no quote
		case dq:
			dq = c != '"'
		case c == '\'':
			sq = true
		case c == '"':
			dq = true
		case c == '$' && i+1 < len(raw) && (raw[i+1] == '{' || raw[i+1] == '(' || raw[i+1] == '_' || (raw[i+1] >= 'a' && raw[i+1] <= 'z') || (raw[i+1] >= 'A' && raw[i+1] <= 'Z')):
			return true
		}
	}
	return false
}

// sedPrograms returns the words of the command that are a sed program: the values of -e and --expression, or the
// first argument when there is none (and no script file).
func sedPrograms(c *runscript.Command) []*runscript.Word {
	return expanding(programWords(c, "-e --expression", "-f --file", ""))
}

// awkPrograms is like sedPrograms for awk, where a program file is -f and gawk also has --source and -e.
func awkPrograms(c *runscript.Command) []*runscript.Word {
	return expanding(programWords(c, "-e --source", "-f --file -E", "-v -F --assign --field-separator -i --include -l --load"))
}

// programWords reads the options of sed or awk and returns the program words: the values of the options in progFlags,
// else the first word that is not an option. A script file (fileFlags) means there is no program on the command line.
// skipFlags are the options that take a value which is not a program.
func programWords(c *runscript.Command, progFlags, fileFlags, skipFlags string) []*runscript.Word {
	prog, file, skip := strings.Fields(progFlags), strings.Fields(fileFlags), strings.Fields(skipFlags)
	has := func(list []string, v string) bool {
		for _, f := range list {
			if f == v {
				return true
			}
		}
		return false
	}
	var out []*runscript.Word
	hasFile := false
	for i := 0; i < len(c.Args); i++ {
		w := c.Args[i]
		v := w.Value
		switch {
		case v == "--":
			if len(out) == 0 && !hasFile && i+1 < len(c.Args) {
				return append(out, c.Args[i+1])
			}
			return out
		case has(prog, v):
			if i+1 < len(c.Args) {
				out = append(out, c.Args[i+1])
				i++
			}
		case has(file, v):
			hasFile = true
			i++
		case strings.HasPrefix(v, "--file=") || attachedFile(v, file):
			hasFile = true
		case v == "-i" && strings.HasSuffix(c.Name, "sed") && i+1 < len(c.Args) && !c.Args[i+1].Dynamic() && c.Args[i+1].Value == "":
			i++ // BSD sed: the suffix of the backup is the next word, and it is empty
		case has(skip, v):
			i++
		case strings.HasPrefix(v, "--expression=") || strings.HasPrefix(v, "--source="):
			out = append(out, w)
		case strings.HasPrefix(v, "-e") && len(v) > 2 && !strings.HasPrefix(v, "--") && has(prog, "-e"):
			out = append(out, w)
		case strings.HasPrefix(v, "-") && len(v) > 1:
			// another option, `-n`, `-i.bak`, `-F:`, `-vx=1`
		default:
			if len(out) == 0 && !hasFile {
				return append(out, w)
			}
			return out
		}
	}
	return out
}

// attachedFile reports whether the word is a short option that names a script file with the name attached (`-fprog.awk`).
func attachedFile(v string, fileFlags []string) bool {
	for _, f := range fileFlags {
		if len(f) == 2 && f[0] == '-' && f[1] != '-' && strings.HasPrefix(v, f) && len(v) > 2 && !strings.HasPrefix(v, "--") {
			return true
		}
	}
	return false
}

// expanding keeps the words that expand a variable or a command substitution: a single quoted program holds none.
func expanding(ws []*runscript.Word) []*runscript.Word {
	var out []*runscript.Word
	for _, w := range ws {
		if w != nil && (len(w.Vars) > 0 || w.Subst) {
			out = append(out, w)
		}
	}
	return out
}

func argumentMessage(c *runscript.Command, d data, kind string) string {
	name := c.Name
	if name == "git" {
		name = "git clone"
	}
	src := "free text of an outsider " + describeUntrusted(d)
	switch kind {
	case "sed":
		return src + " is expanded into the program of sed: a value with a slash ends the command and the rest is a program of its own. GNU sed runs commands with `e` (`a/g;1e id;#`) and writes files with `w`. keep the value out of the program text: read it from the environment in awk (ENVIRON[\"BODY\"]) or perl ($ENV{BODY}), or cut it down to letters and digits first"
	case "awk":
		return src + " is expanded into the program of " + name + ", where it can close a string and call system(). pass the value with -v name=\"$VALUE\" and read it in the program as name, or cut it down to letters and digits first"
	case "find-exec":
		return src + " is the command that find runs with -exec: an attacker who controls the value chooses what runs. use a fixed command"
	case "find-split":
		return src + " is expanded without quotes in find: the shell splits it into words, so it can add an option such as -exec. quote it, and give it as the argument of an option such as -name"
	}
	return src + " starts a word of " + name + ": a value that starts with a dash is an option, and options of this command can run code or write files (--output, -o, --upload-pack). put -- before the value, or give the value as the argument of an option"
}

func init() {
	registerRules(
		RuleInfo{ID: "argument-injection", Group: RuleGroupSecurity, Summary: "Free text that an outsider writes is expanded into the program text or the options of sed, awk, find, curl, wget and git clone.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-argument-injection"},
	)
	registerRuleFactory("argument-injection", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("argument-injection") {
			return nil
		}
		return []Rule{NewRuleArgumentInjection()}
	})
}
