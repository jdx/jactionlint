package jactionlint

import (
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleCommandInjection detects input that an outsider controls and that a script runs as code after it was moved
// out of the `${{ }}` expression into an environment variable.
//
// The advice for template injection is to pass the input to the script as `env:` and to read it as `"$BODY"`. That
// is safe as long as the shell only expands the variable. It is not when the script parses the value again:
//
//   - `eval "$BODY"` and `source <(echo "$BODY")` run the value;
//   - `bash -c "echo $BODY"` (any shell with -c, also behind docker, sudo or xargs) parses the command string
//     after the variable was expanded into it, so a `;` or `$(...)` in the value runs. `bash -c 'echo "$1"' _ "$BODY"`
//     passes the value as data and is fine;
//   - `$CMD` as the name of a command runs the program that the value names;
//   - `echo "$BODY" | bash` and `bash <<< "$BODY"` run the value from the input of the shell;
//   - `xargs -I{} sh -c 'echo {}'` puts each line of its input into the command string of the shell;
//   - `python -c "...$BODY..."`, `node -e` and `perl -e` run an inline program with the value in its text;
//   - in the script of actions/github-script, `execSync(`echo ${process.env.BODY}`)`, `exec.exec(process.env.X)`
//     and the other calls of child_process that go through a shell (see command_injection_javascript.go).
//
// A plain `"$BODY"` as an argument of a command is not reported (that is argument injection at most), and neither
// is a value that the script validated or cut down to harmless characters before, or quoted with `printf %q` or
// `${v@Q}`. The judgement of the value is the one of github-env (rule_github_env_flow.go, see the invariant in
// rule_github_env.go): the variable has the value of its last assignment, whatever the step's `env:` gave it, and the
// source of an untrusted value is the table of untrusted inputs of the expression checks. What differs is the
// destination. A value is safe for a shell only when it is cut down to a character set (destShell): a value without
// newlines can still run `$(id)`. An expression written in the script itself is not judged: template-injection
// reports it. `inputs.*` of a composite action is not judged either, as an action may take a command on purpose.
type RuleCommandInjection struct {
	RuleBase
	runContext
	// flow judges the values; its fields hold the step and the position that is judged.
	flow *RuleGitHubEnv
}

// NewRuleCommandInjection creates a new RuleCommandInjection instance.
func NewRuleCommandInjection() *RuleCommandInjection {
	return &RuleCommandInjection{
		RuleBase: NewRuleBase("command-injection", "Checks for input that an outsider controls and that a script runs as shell or JavaScript code"),
		flow:     &RuleGitHubEnv{dest: destShell, varsOnly: true, skipInputs: true},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleCommandInjection) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleCommandInjection) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleCommandInjection) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleCommandInjection) VisitStep(n *Step) error {
	rule.flow.runContext = rule.runContext
	rule.flow.step = n
	switch e := n.Exec.(type) {
	case *ExecRun:
		if e.Run != nil {
			rule.checkScript(e)
		}
	case *ExecAction:
		rule.checkGitHubScript(n, e)
	}
	return nil
}

// ciSink is a command that runs text as code, and the word of that text.
type ciSink struct {
	cmd *runscript.Command
	// word is the word that holds the code, or the input that is run.
	word *runscript.Word
	d    data
	// kind says how the code reaches the shell and chooses the message.
	kind string
	// what names the command in the message: `eval`, `bash -c`.
	what string
}

func (rule *RuleCommandInjection) checkScript(run *ExecRun) {
	s, origin := rule.script(run)
	if s == nil {
		return
	}
	rule.flow.errexit = rule.shellErrexit(run)
	for _, c := range s.Commands {
		if c.Decl {
			continue
		}
		sink := rule.sinkOf(s, c)
		if sink == nil {
			continue
		}
		end := sink.cmd.End
		if sink.word != nil {
			end = sink.word.End
		}
		rule.errorIDAt("command-injection", scriptPos(s, origin, c.Offset), ciMessage(sink)).endAt(scriptPos(s, origin, end))
	}
}

// judge judges a word of the command at the place of the command. Only an untrusted input counts: a value that is
// merely unknown (the output of a command, a variable of the script that nothing set) is not reported.
func (rule *RuleCommandInjection) judge(s *runscript.Script, c *runscript.Command, w *runscript.Word) data {
	if w == nil {
		return data{}
	}
	f := rule.flow
	f.at = c.Offset
	f.loop = c.LoopBody || c.InFunc
	f.dest = destShell
	d := f.judgeWord(s, w, 0)
	if d.kind != dataUntrusted {
		return data{}
	}
	return d
}

// sinkOf returns the way the command runs untrusted text as code, nil if it does not.
func (rule *RuleCommandInjection) sinkOf(s *runscript.Script, c *runscript.Command) *ciSink {
	switch {
	case c.Name == "eval":
		for _, a := range c.Args {
			if d := rule.judge(s, c, a); d.kind == dataUntrusted {
				return &ciSink{cmd: c, word: a, d: d, kind: "code", what: "eval"}
			}
		}
		return nil
	case c.Name == "source" || c.Name == ".":
		return rule.sourceSink(s, c)
	}
	if shell, str := c.ShellString(); str != nil {
		if sink := rule.shellStringSink(s, c, shell, str); sink != nil {
			return sink
		}
		return nil
	}
	if c.IsShell() {
		return rule.shellInputSink(s, c)
	}
	if interp, code := c.InlineCode(); code != nil {
		if d := rule.judge(s, c, code); d.kind == dataUntrusted {
			return &ciSink{cmd: c, word: code, d: d, kind: "inline", what: interp + " " + inlineFlagOf(c, code)}
		}
		return nil
	}
	if c.NameWord != nil && c.Name == "" {
		return rule.commandPositionSink(s, c)
	}
	return nil
}

// inlineFlagOf returns the option that precedes the program text.
func inlineFlagOf(c *runscript.Command, code *runscript.Word) string {
	for i, w := range c.Words {
		if w == code {
			if i > 0 && strings.HasPrefix(c.Words[i-1].Value, "-") {
				return c.Words[i-1].Value
			}
			return strings.SplitN(w.Value, "=", 2)[0]
		}
	}
	return "-c"
}

// shellStringSink: the command string of `sh -c STRING` is parsed by the shell after the outer shell expanded it.
// For xargs, the replacement string `{}` of -I puts a line of the input into it.
func (rule *RuleCommandInjection) shellStringSink(s *runscript.Script, c *runscript.Command, shell string, str *runscript.Word) *ciSink {
	what := shell + " -c"
	if c.Name == "xargs" {
		what = "xargs " + shell + " -c"
	}
	if d := rule.judge(s, c, str); d.kind == dataUntrusted {
		return &ciSink{cmd: c, word: str, d: d, kind: "string", what: what}
	}
	if c.Name == "xargs" {
		if repl := c.ReplaceString(); repl != "" && strings.Contains(str.Value, repl) {
			if d := rule.stdinInput(s, c); d.kind == dataUntrusted {
				return &ciSink{cmd: c, word: str, d: d, kind: "xargs", what: what}
			}
		}
	}
	return nil
}

// sourceSink: `source <(echo "$BODY")` and `. /dev/stdin <<< "$BODY"`.
func (rule *RuleCommandInjection) sourceSink(s *runscript.Script, c *runscript.Command) *ciSink {
	for _, w := range c.Positional {
		if w.ProcSubst {
			if d := rule.judge(s, c, w); d.kind == dataUntrusted {
				return &ciSink{cmd: c, word: w, d: d, kind: "code", what: c.Name}
			}
		}
	}
	if c.ReadsStdin() {
		if d := rule.stdinInput(s, c); d.kind == dataUntrusted {
			return &ciSink{cmd: c, d: d, kind: "stdin", what: c.Name + " /dev/stdin"}
		}
	}
	return nil
}

// shellInputSink: a shell that reads its program from a pipe, a here string or document, or a process substitution.
func (rule *RuleCommandInjection) shellInputSink(s *runscript.Script, c *runscript.Command) *ciSink {
	for _, w := range c.Positional {
		if w.ProcSubst {
			if d := rule.judge(s, c, w); d.kind == dataUntrusted {
				return &ciSink{cmd: c, word: w, d: d, kind: "stdin", what: c.Name}
			}
		}
	}
	if !c.ReadsStdin() {
		return nil
	}
	if d := rule.stdinInput(s, c); d.kind == dataUntrusted {
		return &ciSink{cmd: c, d: d, kind: "stdin", what: c.Name}
	}
	return nil
}

// stdinInput judges what the command gets on its standard input: a here string, an unquoted here document, and
// the arguments of the earlier stages of its pipeline up to the last one that cuts the data down to a character set.
func (rule *RuleCommandInjection) stdinInput(s *runscript.Script, c *runscript.Command) data {
	f := rule.flow
	f.at = c.Offset
	f.loop = c.LoopBody || c.InFunc
	f.dest = destShell
	for _, r := range c.Redirects {
		switch {
		case r.Op == "<<<" && r.Target != nil:
			if d := rule.judge(s, c, r.Target); d.kind == dataUntrusted {
				return d
			}
		case r.Heredoc != nil && !r.Heredoc.Quoted:
			if d := f.judgeHeredoc(s, r.Heredoc); d.kind == dataUntrusted {
				return d
			}
		}
	}
	p := c.Pipeline
	if p == nil || c.Stage == 0 {
		return data{}
	}
	for i := c.Stage - 1; i >= 0; i-- {
		for _, sc := range p.Stages[i].Commands {
			if commandSanitizes(sc, destShell).satisfies(destShell) {
				return data{}
			}
		}
		for _, sc := range p.Stages[i].Commands {
			for _, a := range sc.Args {
				if d := rule.judge(s, sc, a); d.kind == dataUntrusted {
					return d
				}
			}
			for _, r := range sc.Redirects {
				switch {
				case r.Op == "<<<" && r.Target != nil:
					if d := rule.judge(s, sc, r.Target); d.kind == dataUntrusted {
						return d
					}
				case r.Heredoc != nil && !r.Heredoc.Quoted:
					// `cat <<EOF | bash`: the text of the here document is what the shell runs
					if d := f.judgeHeredoc(s, r.Heredoc); d.kind == dataUntrusted {
						return d
					}
				}
			}
		}
	}
	return data{}
}

// commandPositionSink: `$CMD` or `"$CMD" args` where the name of the program is a variable (or the output of a
// command) that holds untrusted input. A word that only starts with a variable (`$DIR/tool`) names a program in
// a directory and is not judged.
func (rule *RuleCommandInjection) commandPositionSink(s *runscript.Script, c *runscript.Command) *ciSink {
	w := c.NameWord
	if _, ok := w.Var(); !ok && !(w.Subst && len(w.Vars) == 0 && strings.HasPrefix(w.Raw, "$(")) {
		return nil
	}
	if d := rule.judge(s, c, w); d.kind == dataUntrusted {
		return &ciSink{cmd: c, word: w, d: d, kind: "command", what: w.Raw}
	}
	return nil
}

// describeUntrusted says where the untrusted input comes from, for the messages of github-env and this rule.
func describeUntrusted(d data) string {
	if d.via != "" {
		return "from the variable " + d.via + " (" + d.input + ")"
	}
	return quote(d.input)
}

func ciMessage(k *ciSink) string {
	src := "untrusted input " + describeUntrusted(k.d)
	const fix = "validate it against a fixed list or a strict pattern first"
	switch k.kind {
	case "code":
		return src + " is run as shell code by \"" + k.what + "\". an attacker who controls the value can run any command in this step: a ; or $(...) in it is enough. do not run input that an outsider controls; pass it to the program as data, or " + fix
	case "string":
		return src + " is expanded into the command string of \"" + k.what + "\", which the shell then parses as code: a value with shell syntax (a ;, a quote or $(...)) runs commands in this step. pass the value as an argument of the command string instead (" + passAsArgument(k.what) + "), or " + fix
	case "xargs":
		return src + " reaches \"" + k.what + "\", which puts each input line into the command string of the shell, where it is parsed as code. pass the line as an argument instead (xargs -I{} sh -c 'cmd \"$1\"' _ {}), or " + fix
	case "command":
		return src + " is run as a command: " + k.what + " is the program that runs, so an attacker who controls the value chooses what runs in this step. do not run input that an outsider controls; " + fix
	case "stdin":
		return src + " is piped into \"" + k.what + "\", which runs it as a script. an attacker who controls the value can run any command in this step. do not run input that an outsider controls; " + fix
	case "inline":
		return src + " is expanded into the program text of \"" + k.what + "\". a value that closes the string or the expression runs code in this step. read the value from the environment inside the program instead (os.environ, process.env), or " + fix
	}
	return src + " is run as code by \"" + k.what + "\""
}

// passAsArgument shows how to give the value to the shell as data.
func passAsArgument(what string) string {
	return strings.TrimSuffix(what, " -c") + " -c 'cmd \"$1\"' _ \"$VALUE\""
}

// checkGitHubScript looks at the script of an actions/github-script step.
func (rule *RuleCommandInjection) checkGitHubScript(n *Step, e *ExecAction) {
	_, u := usesOfStep(n)
	if u == nil || !u.isRepoAction("actions/github-script") {
		return
	}
	in, ok := e.Inputs["script"]
	if !ok || in == nil || in.Value == nil || in.Value.Value == "" {
		return
	}
	str := in.Value
	env := func(name string) (data, bool) {
		rule.flow.dest = destShell
		return rule.flow.judgeInitial(name, 0), true
	}
	for _, h := range scanJSCommandSinks(str.Value, env) {
		start, end := stringPos(str, h.start), stringPos(str, h.end)
		rule.errorIDAt("command-injection", start, jsMessage(h)).endAt(end)
	}
}

// stringPos is the position in the file of an offset of the value of a string.
func stringPos(s *String, off int) *Pos {
	if l, c, ok := s.valueAt(off); ok {
		return &Pos{Line: l, Col: c}
	}
	if s.Pos != nil {
		return &Pos{Line: s.Pos.Line, Col: s.Pos.Col}
	}
	return &Pos{Line: 1, Col: 1}
}

func jsMessage(h jsHit) string {
	src := "untrusted input " + describeUntrusted(h.d)
	const fix = "validate it against a fixed list or a strict pattern first"
	switch h.kind {
	case "command":
		return src + " is the program that \"" + h.call + "\" runs: it is the start of the command line, so an attacker who controls the value chooses what runs in this step. pass a fixed program and give the value as an argument (exec.exec('tool', [value])), or " + fix
	case "shellc":
		return src + " is the script of a shell that \"" + h.call + "\" starts with -c, which parses it as code: a value with shell syntax runs commands in this step. pass the value as an argument of the program instead (execFileSync('tool', [value])), or " + fix
	}
	return src + " is built into the command line that \"" + h.call + "\" runs through a shell: a value with shell syntax (a ;, a quote or $(...)) runs commands in this step. pass the value as an argument instead (execFileSync('tool', [value]) or exec.exec('tool', [value])), or " + fix
}

func init() {
	registerRules(
		RuleInfo{ID: "command-injection", Group: RuleGroupSecurity, Summary: "Input that an outsider controls is run as code by a script after it was moved to an environment variable: eval, sh -c, a variable as a command, a pipe into a shell, an inline program, child_process in github-script.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-command-injection"},
	)
	registerRuleFactory("command-injection", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("command-injection") {
			return nil
		}
		return []Rule{NewRuleCommandInjection()}
	})
}
