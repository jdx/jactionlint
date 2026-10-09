package jactionlint

import (
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleGitHubEnv detects dangerous writes to the files $GITHUB_ENV and $GITHUB_PATH in `run:` scripts.
//
// What is written to $GITHUB_ENV becomes the environment of every later step of the job, so a value an outsider
// controls can set LD_PRELOAD or NODE_OPTIONS (a newline in the value is enough to add another variable) and run
// code. A directory added to $GITHUB_PATH can shadow the executables of the runner. The rule reports
//
//   - "github-env": a write of a value that is not a literal in a workflow started by pull_request_target or
//     workflow_run, which run with secrets and a write token while the event may come from a fork, and
//   - a write of a value that is known to come from an outsider (an untrusted
//     context or an environment variable holding one), whatever the trigger.
//
// Scripts of bash and sh are analyzed with the run-script analyzer, scripts of pwsh, powershell and cmd by
// matching their lines.
type RuleGitHubEnv struct {
	RuleBase
	runContext
	step *Step
}

// NewRuleGitHubEnv creates a new RuleGitHubEnv instance.
func NewRuleGitHubEnv() *RuleGitHubEnv {
	return &RuleGitHubEnv{
		RuleBase: NewRuleBase("github-env", "Checks for dangerous writes to GITHUB_ENV and GITHUB_PATH at \"run:\""),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleGitHubEnv) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleGitHubEnv) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleGitHubEnv) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleGitHubEnv) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil {
		return nil
	}
	rule.step = n
	switch rule.shellName(run) {
	case "bash", "sh":
		rule.checkBash(run)
	case "pwsh", "powershell", "cmd":
		rule.checkLines(run)
	}
	return nil
}

// privileged reports whether the workflow runs with secrets and a write token for events that may come from a fork,
// and the event.
func (rule *RuleGitHubEnv) privileged() (string, bool) {
	for _, t := range []string{"pull_request_target", "workflow_run"} {
		if rule.hasTrigger(t) {
			return t, true
		}
	}
	return "", false
}

func (rule *RuleGitHubEnv) checkBash(run *ExecRun) {
	s, origin := rule.script(run)
	if s == nil {
		return
	}
	for _, w := range s.WritesTo("GITHUB_ENV", "GITHUB_PATH") {
		d := rule.judgeWrite(s, w)
		rule.report(s, origin, w, d)
	}
}

func (rule *RuleGitHubEnv) report(s *runscript.Script, origin runscript.Origin, w *runscript.Write, d data) {
	r := w.Redirect
	rule.emit(d, "$"+w.Var, scriptPos(s, origin, r.Offset), scriptPos(s, origin, r.End))
}

// emit reports the write to dest when it is dangerous in this workflow.
func (rule *RuleGitHubEnv) emit(d data, dest string, start, end *Pos) {
	what := "a value"
	effect := "an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps"
	if dest == "$GITHUB_PATH" {
		what = "a directory"
		effect = "an attacker who controls the directory can shadow executables such as ssh and run code in the next steps"
	}
	switch d.kind {
	case dataUntrusted:
		src := quote(d.input)
		if d.via != "" {
			src = "from the variable " + d.via + " (" + d.input + ")"
		}
		e := rule.errorIDAt("github-env", start, "untrusted input "+src+" is written to "+dest+". "+effect+". do not write input that an outsider controls to "+dest+"; validate it first or pass it to the next step with $GITHUB_OUTPUT").endAt(end)
		e.RetiredID = "github-env-untrusted-input"
	case dataUnknown:
		trigger, ok := rule.privileged()
		if !ok {
			return
		}
		rule.errorIDAt("github-env", start, what+" that is not a literal is written to "+dest+" in a workflow triggered by "+quote(trigger)+", which runs with secrets and a write token for events that may come from a fork. "+effect+". write only literal values and values computed from trusted sources, or pass state with $GITHUB_OUTPUT").endAt(end)
	}
}

func quote(s string) string { return `"` + s + `"` }

// judgeWrite judges the data written to the file.
func (rule *RuleGitHubEnv) judgeWrite(s *runscript.Script, w *runscript.Write) data {
	d := data{}
	for _, c := range w.Producers {
		d = d.worse(rule.judgeCommand(s, c))
		if d.kind == dataUntrusted {
			return d
		}
	}
	if w.Heredoc != nil {
		d = d.worse(rule.judgeHeredoc(s, w.Heredoc))
	}
	return d
}

// neutralCommands do not produce the data that is written: they only write what they receive (tee) or are tests.
var neutralCommands = map[string]bool{"tee": true, "test": true, "[": true, "true": true, ":": true, "false": true}

func (rule *RuleGitHubEnv) judgeCommand(s *runscript.Script, c *runscript.Command) data {
	d := data{}
	for _, a := range c.Assigns {
		if a.Value != nil {
			d = d.worse(rule.judgeWord(s, a.Value, 0))
		}
	}
	for _, r := range c.Redirects {
		if r.Op == "<<<" && r.Target != nil {
			d = d.worse(rule.judgeWord(s, r.Target, 0))
		}
	}
	switch {
	case c.Name == "":
		return d.worse(data{kind: dataUnknown})
	case neutralCommands[c.Name]:
		return d
	case c.Name == "echo" || c.Name == "printf":
		for _, a := range c.Args {
			d = d.worse(rule.judgeWord(s, a, 0))
		}
		return d
	case c.Name == "cat" && len(c.Positional) == 0:
		// `cat <<EOF >> $GITHUB_ENV` is a literal when its here document is
		return d
	}
	return d.worse(data{kind: dataUnknown})
}

// reExpressionSpan matches a ${{ }} expression with any spacing, `${{github.sha}}` included.
var reExpressionSpan = regexp.MustCompile(`(?s)\$\{\{.*?\}\}`)

func (rule *RuleGitHubEnv) judgeHeredoc(s *runscript.Script, h *runscript.Heredoc) data {
	d := rule.judgeExprList(h.Exprs, 0)
	if h.Quoted || (!strings.Contains(h.Body, "$") && !strings.Contains(h.Body, "`")) {
		return d
	}
	// The body is expanded by the shell: `${{ }}` is already judged. A plain variable ($NAME or ${NAME}) is judged
	// by its value like in the argument of echo; any other expansion is not a literal.
	body := reExpressionSpan.ReplaceAllString(h.Body, "")
	if strings.Contains(body, "`") {
		d = d.worse(data{kind: dataUnknown})
	}
	rest := reHeredocVar.ReplaceAllStringFunc(body, func(m string) string {
		name := strings.Trim(m, "${}")
		d = d.worse(rule.judgeVar(s, name, 0))
		return ""
	})
	if strings.Contains(rest, "$") {
		d = d.worse(data{kind: dataUnknown})
	}
	return d
}

// reHeredocVar matches a plain reference to a variable in the body of a here document.
var reHeredocVar = regexp.MustCompile(`\$(?:[A-Za-z_][A-Za-z0-9_]*|\{[A-Za-z_][A-Za-z0-9_]*\})`)

// benignSubstitutionCommands are commands whose output does not depend on anything an outsider controls when their
// arguments do not: they print a fresh name, the time, the working directory or a property of the machine.
var benignSubstitutionCommands = map[string]bool{
	"mktemp": true, "date": true, "pwd": true, "uname": true, "nproc": true, "hostname": true, "whoami": true,
	"arch": true, "dirname": true, "basename": true, "realpath": true,
}

// benignSubstitution reports whether the word has command substitutions and all their commands are benign. An
// arithmetic expansion has no commands and is not judged.
func benignSubstitution(w *runscript.Word) bool {
	if len(w.Subs) == 0 {
		return false
	}
	for _, c := range w.Subs {
		if !benignSubstitutionCommands[c.Name] || c.Name == "" {
			return false
		}
	}
	return true
}

// judgeWordArgs judges the expressions and variables of an argument of a command in a substitution; nested
// substitutions are judged on their own, as the commands of Word.Subs are listed at any depth.
func (rule *RuleGitHubEnv) judgeWordArgs(s *runscript.Script, w *runscript.Word, depth int) data {
	d := rule.judgeExprList(w.Exprs, depth)
	for _, v := range w.Vars {
		d = d.worse(rule.judgeVar(s, v, depth))
	}
	return d
}

// judgeWord judges a word of the script: its expressions, and the environment variables and variables of the
// script it expands.
func (rule *RuleGitHubEnv) judgeWord(s *runscript.Script, w *runscript.Word, depth int) data {
	d := rule.judgeExprList(w.Exprs, depth)
	if d.kind == dataUntrusted {
		return d
	}
	if w.Subst && !benignSubstitution(w) {
		d = d.worse(data{kind: dataUnknown})
	}
	if w.Glob {
		d = d.worse(data{kind: dataUnknown})
	}
	if w.Subst {
		// the arguments of the commands of a substitution decide what they print
		for _, c := range w.Subs {
			for _, a := range c.Args {
				d = d.worse(rule.judgeWordArgs(s, a, depth))
			}
		}
	}
	if len(w.Vars) == 0 && !w.Subst && strings.Contains(w.Raw, "$") && len(w.Exprs) == 0 {
		return d.worse(data{kind: dataUnknown}) // $1, $@, $$ ...
	}
	for _, v := range w.Vars {
		d = d.worse(rule.judgeVar(s, v, depth))
		if d.kind == dataUntrusted {
			return d
		}
	}
	return d
}

// judgeExprList judges the expressions of a value. An expression that is just a variable of the `env` context is
// judged by the value it was given in the workflow file.
func (rule *RuleGitHubEnv) judgeExprList(exprs []string, depth int) data {
	d := data{}
	for _, e := range exprs {
		if name, ok := envContextVar(e); ok && depth <= 3 {
			if v, ok := rule.envContextValue(rule.step, name); ok {
				ed := rule.judgeExprList(exprsIn(v.Value), depth+1)
				if ed.kind == dataUntrusted && ed.via == "" {
					ed.via = name
				}
				d = d.worse(ed)
				continue
			}
		}
		d = d.worse(rule.judgeExpr(e))
	}
	return d
}

// judgeExpr judges one expression. In the metadata of an action, `inputs.*` is whatever the caller passes, and a
// workflow can pass the title of an issue: it is an outsider's input, whichever event runs the action.
func (rule *RuleGitHubEnv) judgeExpr(e string) data {
	if rule.wf != nil && rule.wf.Action != nil && exprReadsContext(e, "inputs") {
		return data{kind: dataUntrusted, input: e}
	}
	return judgeExprs([]string{e})
}

// envContextVar returns NAME of an expression that is exactly `env.NAME`.
func envContextVar(e string) (string, bool) {
	n, ok := parseExprText(e).(*ObjectDerefNode)
	if !ok {
		return "", false
	}
	if v, ok := n.Receiver.(*VariableNode); ok && strings.EqualFold(v.Name, "env") {
		return n.Property, true
	}
	return "", false
}

// judgeVar judges the value of a shell variable: an assignment of the script wins over the environment of the
// step, which wins over the environment of the job and of the workflow.
func (rule *RuleGitHubEnv) judgeVar(s *runscript.Script, name string, depth int) data {
	if depth > 3 {
		return data{kind: dataUnknown}
	}
	assigned := false
	d := data{}
	for _, a := range s.Assignments {
		if a.Name != name {
			continue
		}
		assigned = true
		if a.Value == nil {
			continue
		}
		if a.Append || a.Array {
			d = d.worse(data{kind: dataUnknown})
			continue
		}
		d = d.worse(rule.judgeWord(s, a.Value, depth+1))
	}
	if assigned {
		return d
	}
	if v, ok := rule.envValue(rule.step, name); ok {
		ed := rule.judgeExprList(exprsIn(v.Value), depth+1)
		if ed.kind == dataUntrusted && ed.via == "" {
			ed.via = name
		}
		return ed
	}
	if runnerProvidedVars[name] {
		return data{}
	}
	return data{kind: dataUnknown}
}

// --- pwsh, powershell and cmd ---------------------------------------------------------------------------

var (
	rePwshEnvFile = regexp.MustCompile(`(?i)\$\{?env:GITHUB_(ENV|PATH)\}?`)
	reCmdEnvFile  = regexp.MustCompile(`(?i)%GITHUB_(ENV|PATH)%`)
	rePwshWriter  = regexp.MustCompile(`(?i)(>>?|\bout-file\b|\badd-content\b|\bset-content\b|\btee-object\b)`)
)

// checkLines looks for writes to the environment files line by line in the scripts of shells the analyzer does
// not parse. A line is a write when it mentions the file together with a redirection or a cmdlet that writes.
func (rule *RuleGitHubEnv) checkLines(run *ExecRun) {
	shell := rule.shellName(run)
	re := rePwshEnvFile
	if shell == "cmd" {
		re = reCmdEnvFile
	}
	origin := run.Run.scriptOrigin()
	lineOff := 0
	for i, line := range strings.Split(run.Run.Value, "\n") {
		off := lineOff
		lineOff += len(line) + 1
		line = strings.TrimSuffix(line, "\r")
		m := re.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		if shell == "cmd" {
			if !strings.Contains(line, ">") {
				continue
			}
		} else if !rePwshWriter.MatchString(line) {
			continue
		}
		dest := "$GITHUB_" + strings.ToUpper(line[m[2]:m[3]])
		d := rule.judgeLine(line[:m[0]] + line[m[1]:])
		start := origin.Map(i+1, m[0]+1)
		end := origin.Map(i+1, m[1]+1)
		if l, c, ok := run.Run.valueAt(off + m[0]); ok {
			start = runscript.Position{Line: l, Col: c}
			if l, c, ok := run.Run.valueAt(off + m[1]); ok {
				end = runscript.Position{Line: l, Col: c}
			}
		}
		rule.emit(d, dest, &Pos{Line: start.Line, Col: start.Col}, &Pos{Line: end.Line, Col: end.Col})
	}
}

// judgeLine judges what a line of pwsh or cmd writes: the line is a literal when it has no variable, no
// subexpression and no expression apart from the file it writes to.
func (rule *RuleGitHubEnv) judgeLine(rest string) data {
	d := rule.judgeExprList(exprsIn(rest), 0)
	if d.kind == dataUntrusted {
		return d
	}
	stripped := reExpressionSpan.ReplaceAllString(rest, "")
	if strings.ContainsAny(stripped, "$%`(") {
		d = d.worse(data{kind: dataUnknown})
	}
	return d
}

func init() {
	registerRules(
		RuleInfo{ID: "github-env", Group: RuleGroupSecurity, Summary: "Input that an outsider controls, or a value that is not a literal in a workflow triggered by pull_request_target or workflow_run, is written to GITHUB_ENV or GITHUB_PATH.", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-github-env"},
	)
	registerRuleFactory("github-env", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("github-env") {
			return nil
		}
		return []Rule{NewRuleGitHubEnv()}
	})
}
