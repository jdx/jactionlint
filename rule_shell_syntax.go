package jactionlint

import (
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleShellSyntax reports a syntax error in the script of a "run:" step without running shellcheck. The script is
// parsed with the bash parser of mvdan.cc/sh, offline, and only a genuine syntax error is reported (an unclosed quote,
// a missing "fi" or "done", an unexpected token). It is the part of shellcheck that needs no installation (zizmor#2322).
//
// The rule is silent when:
//
//   - shellcheck runs. It reports the same mistakes with its SC1xxx codes, and a second finding for the same line is
//     noise. The rule is for runs where shellcheck was not found, was turned off with an empty command, or where
//     the "shellcheck" rule is switched off in the configuration.
//   - the shell is not bash or sh. Only these two names (and a custom template that starts with one of them, such as
//     "bash -el {0}") are read; pwsh, powershell, cmd, python and every other custom shell are not parsed at all. A
//     step without "shell:" is read only when the runner is known to be Linux or macOS: the default shell of a Windows
//     runner is pwsh, and a runner chosen with an expression can be one.
//   - the script is not read faithfully: it has a carriage return, or the parser stops at a construct of another
//     shell instead of at a syntax error.
//
// "sh" is read with the bash grammar, too, and reports only what is an error in bash. The /bin/sh of a runner is
// dash on Ubuntu but bash on macOS and Git Bash, so a script that is only valid in bash ("[[ ]]", arrays, process
// substitution) is not a syntax error of "shell: sh" on every runner, and the rule does not call it one.
//
// ${{ }} expressions are replaced by underscores of the same length before parsing (see sanitizeScriptExpressions: the
// end of an expression is found by the expression lexer, and a script with an expression that cannot be delimited is
// not read), so the offset of an error is also an offset of the script in the workflow.
type RuleShellSyntax struct {
	RuleBase
	runContext
}

// NewRuleShellSyntax creates a new RuleShellSyntax instance.
func NewRuleShellSyntax() *RuleShellSyntax {
	return &RuleShellSyntax{
		RuleBase: NewRuleBase("shell-syntax", "Checks for syntax errors in shell scripts at \"run:\" without shellcheck"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleShellSyntax) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleShellSyntax) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleShellSyntax) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleShellSyntax) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || run.Run.Value == "" {
		return nil
	}
	if !rule.readsShell(run) {
		return nil
	}

	src := run.Run.Value
	// A carriage return ends a line for the YAML parser but is a character of the script for bash, which then fails
	// in other ways than the parser here would say.
	if strings.Contains(src, "\r") || strings.IndexByte(src, 0) >= 0 {
		return nil
	}
	src, ok = sanitizeScriptExpressions(run.Run)
	if !ok {
		return nil // an expression which cannot be delimited: the text is not what the shell gets
	}

	perr := runscript.SyntaxError(src)
	if perr == nil {
		return nil
	}

	pos := run.RunPos
	if pos == nil {
		pos = run.Run.Pos
	}
	if pos == nil {
		return nil
	}
	loc := *pos
	// The line is counted in the script as written: the placeholders have the length of the expressions but not their
	// line breaks, so the sanitized text can have fewer lines
	line := strings.Count(run.Run.Value[:perr.Offset], "\n") + 1
	if l, c, ok := run.Run.valueAt(perr.Offset); ok {
		loc = Pos{Line: l, Col: c}
	} else if run.Run.Literal && run.Run.Pos != nil {
		loc.Line = run.Run.Pos.Line + line
	}
	rule.ReportIDf("shell-syntax", &loc, "syntax error in the %s script of this step (line %d of the script): %s. found by the built-in parser because shellcheck did not run", rule.shellKind(run), line, perr.Text)
	return nil
}

// sanitizeScriptExpressions replaces each ${{ }} expression of the script with underscores of the same byte length, so
// that an offset of the result is an offset of the script. The end of an expression is found by the lexer of the
// expressions, as the rule `expression` does, so a "}}" inside a string literal ('}}') does not end it. It returns
// false when an expression cannot be delimited (it is not closed or does not parse): then the text between the
// expressions is not known, and the script is not read. sanitizeExpressionsInScript, which the integration of
// shellcheck uses, closes an expression at the first "}}"; it is left as it is because shellcheck's output depends on it.
func sanitizeScriptExpressions(str *String) (string, bool) {
	value := str.Value
	c := str.placeholders()
	if c.failed {
		return "", false
	}
	if len(c.list) == 0 {
		return value, !strings.Contains(value, "${{")
	}
	b := []byte(value)
	for _, p := range c.list {
		start, end := p.after-len("${{"), p.after+p.n
		if start < 0 || end > len(b) || end-start < len("${{}}") || value[end-2:end] != "}}" {
			return "", false
		}
		for i := start; i < end; i++ {
			b[i] = '_'
		}
	}
	out := string(b)
	if strings.Contains(out, "${{") {
		return "", false
	}
	return out, true
}

// readsShell reports whether the script of the step is read: the shell is bash or sh, and the step is not one that
// runs under pwsh when the runner is not known.
func (rule *RuleShellSyntax) readsShell(run *ExecRun) bool {
	name := rule.shellName(run)
	if name != "bash" && name != "sh" {
		return false
	}
	if rule.hasExplicitShell(run) {
		return true
	}
	return rule.job != nil && knownUnixRunner(rule.job.RunsOn)
}

// hasExplicitShell reports whether the shell comes from "shell:" of the step, "defaults.run.shell" of the job or
// of the workflow, and not from the default of the runner.
func (rule *RuleShellSyntax) hasExplicitShell(run *ExecRun) bool {
	switch {
	case run.Shell != nil:
		return true
	case rule.job != nil && rule.job.Defaults != nil && rule.job.Defaults.Run != nil && rule.job.Defaults.Run.Shell != nil:
		return true
	case rule.wf != nil && rule.wf.Defaults != nil && rule.wf.Defaults.Run != nil && rule.wf.Defaults.Run.Shell != nil:
		return true
	}
	return false
}

func (rule *RuleShellSyntax) shellKind(run *ExecRun) string {
	if n := rule.shellName(run); n != "" {
		return n
	}
	return "bash"
}

// knownUnixRunner reports whether every label of runs-on is written out (no expression and no runner group), none of
// them is a Windows runner and at least one says Linux or macOS. It is stricter than the check of the rule
// pipeline-without-pipefail, which reads a matrix: a script that is no bash is a mistake on Linux, but it is a
// pwsh script on the Windows leg of a matrix, so the rule reads only a job whose runner it can name.
func knownUnixRunner(r *Runner) bool {
	if r == nil || r.LabelsExpr != nil || r.Group != nil || len(r.Labels) == 0 {
		return false
	}
	unix := false
	for _, l := range r.Labels {
		if l == nil || l.ContainsExpression() {
			return false
		}
		switch classifyRunnerLabel(l.Value) {
		case runnerLabelWindows:
			return false
		case runnerLabelUnix:
			unix = true
		}
	}
	return unix
}

func init() {
	registerRules(
		RuleInfo{ID: "shell-syntax", Group: RuleGroupCorrectness, Summary: "A shell script at \"run:\" has a syntax error (found without shellcheck).", DefaultLevel: SeverityError, Profile: ProfileDefault, DocsAnchor: "check-shell-syntax"},
	)
	registerRuleFactory("shell-syntax", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("shell-syntax") {
			return nil
		}
		// shellcheck reports the same mistakes (SC1xxx) when it runs. The factory of shellcheck runs before this one
		// and records that it could not be created; a rule which is switched off in the configuration still runs
		// the command, but its findings are dropped, so the built-in parser is needed then too.
		if !env.skipped["shellcheck"] && env.config.RuleEnabled("shellcheck") {
			return nil
		}
		return []Rule{NewRuleShellSyntax()}
	})
}
