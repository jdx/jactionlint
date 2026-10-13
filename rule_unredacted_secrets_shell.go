package jactionlint

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// The pedantic finding of unredacted-secrets about values that a script derives from a secret (sisakulint's
// secret-in-log, CodeQL's UnmaskedSecretExposure).
//
// The runner masks a secret in the log by its exact value. A value that the script extracts from it is a different
// string: `KEY=$(jq -r .private_key <<<"$CREDENTIALS")` makes a value that the runner does not know, and `echo "$KEY"`
// or the trace of `set -x` prints it in clear text. `::add-mask::` before the print tells the runner about it.
//
// The analysis follows the variables of one script, in the order of the source:
//
//   - a variable is a secret when the `env:` of the step, the job or the workflow gives it a value with `secrets.X`,
//     and so is a copy of one (`A="$SECRET"`: the exact value stays masked);
//   - a variable is derived when it is set from a command substitution which reads a secret (as an argument, a here-string
//     or the other end of a pipe) with a command that changes the value (jq, cut, sed, awk, tr, base64, python, ...), or from a
//     parameter expansion that cuts it (`${SECRET#prefix}`, `${SECRET:0:8}`), or is a copy of a derived one;
//   - it is printed by `echo`, `printf` or `cat` of the variable (or of the substitution itself), unless stdout goes to a
//     file or into a pipe, which is what `echo "$KEY" | base64 -d > key.pem` does (sisakulint#537);
//   - with `set -x` on, the shell prints the assignment and every later command that uses the value;
//   - `echo "::add-mask::$KEY"` after the assignment and before the print makes it safe.
//
// What is not reported: the value as an argument of another command (`curl -H "Authorization: $TOKEN"`, sisakulint#594), a
// value written to a file or to $GITHUB_ENV or $GITHUB_OUTPUT, hashes and the answer of a service (`aws ... --secret "$KEY"`,
// `curl`): a value that a program computes from the secret is not a cut of it.

// derivationTools change the text of a secret into another text that contains a part of it.
var derivationTools = []string{"jq", "yq", "cut", "awk", "sed", "tr", "base64", "grep", "head", "tail", "xxd", "rev", "fold", "python", "python3", "node", "perl", "ruby"}

var (
	secretExprRe     = regexp.MustCompile(`secrets(?:\.([A-Za-z0-9_-]+)|\[\s*['"]([^'"]+)['"]\s*\])`)
	paramCutRe       = `(?:#{1,2}|%{1,2}|/|\^{1,2}|,{1,2}|:[0-9]|: +-?[0-9])`
	setCommandRe     = regexp.MustCompile(`^set$`)
	xtraceOptionRe   = regexp.MustCompile(`^[-+][a-zA-Z]*x[a-zA-Z]*$`)
	pythonToolNameRe = regexp.MustCompile(`^python[0-9.]*$`)
)

// derived is a variable that holds a value cut from a secret.
type derived struct {
	secret, how string
	masked      bool
	reported    bool
}

type shellSecretFlow struct {
	rule    *RuleUnredactedSecrets
	script  *runscript.Script
	origin  runscript.Origin
	secrets map[string]string   // variable -> secret
	derived map[string]*derived // variable -> how it was derived
	xtrace  bool
	inSub   map[*runscript.Command]bool
	// redirects are the redirects attached to each command, and groups the redirects of compound commands
	redirects map[*runscript.Command][]*runscript.Redirect
	groups    []*runscript.Redirect
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleUnredactedSecrets) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleUnredactedSecrets) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleUnredactedSecrets) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || !rule.pedantic("unredacted-secrets") {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	f := &shellSecretFlow{rule: rule, script: s, origin: origin, secrets: map[string]string{}, derived: map[string]*derived{}, inSub: map[*runscript.Command]bool{}}
	for _, env := range []*Env{rule.workflowEnv(), rule.jobEnv(), n.Env} {
		if env == nil {
			continue
		}
		for _, v := range env.Vars {
			if v == nil || v.Name == nil || v.Value == nil {
				continue
			}
			if refs, ok := v.Value.exprRefs(); ok {
				for _, r := range refs {
					if len(r.chain) >= 2 && strings.EqualFold(r.chain[0], "secrets") {
						f.secrets[v.Name.Value] = strings.ToUpper(r.chain[1])
						break
					}
				}
			}
		}
	}
	f.run()
	return nil
}

type secretFlowEvent struct {
	offset int
	a      *runscript.Assignment
	c      *runscript.Command
}

func (f *shellSecretFlow) run() {
	s := f.script
	if len(f.secrets) == 0 && !slices.ContainsFunc(s.Exprs, func(e *runscript.Expr) bool { return secretExprRe.MatchString(e.Text) }) {
		return // no secret in reach: nothing to derive a value from
	}
	f.redirects = map[*runscript.Command][]*runscript.Redirect{}
	for _, r := range s.Redirects {
		switch {
		case r.Cmd != nil:
			f.redirects[r.Cmd] = append(f.redirects[r.Cmd], r)
		case r.Group:
			f.groups = append(f.groups, r)
		}
	}
	for _, c := range s.Commands {
		for _, w := range c.Words {
			for _, sub := range w.Subs {
				f.inSub[sub] = true
			}
		}
	}
	for _, a := range s.Assignments {
		if a.Value != nil {
			for _, sub := range a.Value.Subs {
				f.inSub[sub] = true
			}
		}
	}
	var events []secretFlowEvent
	for _, a := range s.Assignments {
		if a.Cmd == nil || a.Cmd.Decl {
			events = append(events, secretFlowEvent{offset: a.Offset, a: a})
		}
	}
	for _, c := range s.Commands {
		if !f.inSub[c] {
			events = append(events, secretFlowEvent{offset: c.Offset, c: c})
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].offset < events[j].offset })
	for _, e := range events {
		if e.a != nil {
			f.assign(e.a)
		} else {
			f.command(e.c)
		}
	}
}

// sourcesOf returns the secret and the variable names that the word reads, and the tool that changes them when the
// word is a command substitution.
func (f *shellSecretFlow) sourcesOf(w *runscript.Word) (secret, how string, ok bool) {
	// the word itself: variables and expressions
	read := func(w *runscript.Word) (string, bool) {
		for _, name := range w.Vars {
			if s, ok := f.secrets[name]; ok {
				return s, true
			}
			if d, ok := f.derived[name]; ok {
				return d.secret, true
			}
		}
		for _, e := range w.Exprs {
			if m := secretExprRe.FindStringSubmatch(e); m != nil {
				return strings.ToUpper(m[1] + m[2]), true
			}
		}
		return "", false
	}
	if len(w.Subs) == 0 {
		return "", "", false
	}
	// stdinSecret is the secret that a command reads as data: a here-string or heredoc, or (for the tools that run
	// code) an argument.
	reads := func(c *runscript.Command, args bool) (string, bool) {
		for _, r := range f.redirects[c] {
			if r.Op == "<<<" && r.Target != nil {
				if s, ok := read(r.Target); ok {
					return s, true
				}
			}
			if r.Heredoc != nil && !r.Heredoc.Quoted {
				for _, name := range varsIn(r.Heredoc.Body) {
					if s, ok := f.secrets[name]; ok {
						return s, true
					}
				}
			}
		}
		if args {
			for _, sw := range c.Words {
				if s, ok := read(sw); ok {
					return s, true
				}
			}
		}
		return "", false
	}
	// The value of the substitution is what its last command prints. Follow it back along the pipeline: it is cut from
	// the secret when only echo, printf, cat and the tools that cut a text stand between them (`echo "$S" | base64 -d |
	// jq -r .key`), and not when another command is in the way (`curl -u "$S" ... | jq -r .id` reads the answer of a
	// service, `... | wc -c` counts).
	var last *runscript.Command
	for _, sub := range w.Subs {
		if last == nil || sub.End > last.End {
			last = sub
		}
	}
	chain := []*runscript.Command{last}
	if last.Pipeline != nil {
		chain = chain[:0]
		for i := last.Stage; i >= 0; i-- {
			if cs := last.Pipeline.Stages[i].Commands; len(cs) == 1 {
				chain = append(chain, cs[0])
			} else {
				break
			}
		}
	}
	for _, c := range chain {
		switch {
		case isDerivationTool(c.Name):
			if how == "" {
				how = c.Name
			}
			if s, ok := reads(c, codeTools[c.Name] || pythonToolNameRe.MatchString(c.Name)); ok {
				return s, how, true
			}
		case c.Name == "echo" || c.Name == "printf" || c.Name == "cat":
			if s, ok := reads(c, true); ok {
				return s, how, how != ""
			}
		default:
			return "", "", false
		}
	}
	return "", "", false
}

// codeTools are the derivation tools that take a program as an argument, which can hold the secret as data.
var codeTools = map[string]bool{"jq": true, "yq": true, "awk": true, "node": true, "perl": true, "ruby": true}

var shellVarsRe = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

func varsIn(text string) []string {
	var out []string
	for _, m := range shellVarsRe.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

func isDerivationTool(name string) bool {
	return slices.Contains(derivationTools, name) || pythonToolNameRe.MatchString(name)
}

// cutsSecret reports whether the word expands a secret variable in a way that cuts it
// (`${SECRET#prefix}`, `${SECRET:0:8}`, `${SECRET//x/y}`).
func (f *shellSecretFlow) cutsSecret(w *runscript.Word) (secret string, ok bool) {
	for _, name := range w.Vars {
		s, isSecret := f.secrets[name]
		if d, isDerived := f.derived[name]; isDerived && !isSecret {
			s, isSecret = d.secret, true
		}
		if !isSecret {
			continue
		}
		if regexp.MustCompile(`\$\{` + regexp.QuoteMeta(name) + paramCutRe).MatchString(w.Raw) {
			return s, true
		}
	}
	return "", false
}

func (f *shellSecretFlow) assign(a *runscript.Assignment) {
	name := a.Name
	delete(f.derived, name)
	if a.Value == nil || a.Array {
		delete(f.secrets, name)
		return
	}
	w := a.Value
	if v, ok := w.Var(); ok {
		// a copy: the exact value of a secret is still masked, a derived one still is not
		if d := f.derived[v]; d != nil {
			f.derived[name] = &derived{secret: d.secret, how: d.how, masked: d.masked}
			return
		}
		if s, ok := f.secrets[v]; ok {
			f.secrets[name] = s
			return
		}
	}
	delete(f.secrets, name)
	if secret, how, ok := f.sourcesOf(w); ok {
		f.derived[name] = &derived{secret: secret, how: how}
	} else if secret, ok := f.cutsSecret(w); ok {
		f.derived[name] = &derived{secret: secret, how: "a cut of its value"}
	} else {
		return
	}
	if f.xtrace {
		f.report(a.Loc, name, f.derived[name], "xtrace")
	}
}

func (f *shellSecretFlow) command(c *runscript.Command) {
	if c.Name == "set" {
		for _, w := range c.Args {
			switch {
			case xtraceOptionRe.MatchString(w.Value):
				f.xtrace = w.Value[0] == '-'
			case w.Value == "-o" || w.Value == "+o":
				// `set -o xtrace`
				for i, a := range c.Args {
					if a == w && i+1 < len(c.Args) && c.Args[i+1].Value == "xtrace" {
						f.xtrace = w.Value == "-o"
					}
				}
			}
		}
		return
	}
	isMask := false
	for _, w := range c.Args {
		if strings.HasPrefix(w.Value, "::add-mask::") && (c.Name == "echo" || c.Name == "printf") {
			isMask = true
		}
	}
	prints := f.prints(c)
	for _, name := range f.usedVars(c) {
		d := f.derived[name]
		if d == nil || d.masked || d.reported {
			continue
		}
		switch {
		case f.xtrace:
			f.report(c.Loc, name, d, "xtrace")
		case prints && !isMask:
			f.report(c.Loc, name, d, "print")
		}
	}
	if prints && !isMask && !f.xtrace {
		for _, w := range c.Words {
			if len(w.Subs) == 0 {
				continue
			}
			if secret, how, ok := f.sourcesOf(w); ok {
				f.report(c.Loc, "", &derived{secret: secret, how: how}, "print")
			}
		}
	}
	if isMask {
		for _, w := range c.Args {
			for _, name := range w.Vars {
				if d := f.derived[name]; d != nil {
					d.masked = true
				}
			}
		}
	}
}

// usedVars returns the shell variables that the command expands: in its words, in a here-string and in a heredoc
// that expands them.
func (f *shellSecretFlow) usedVars(c *runscript.Command) []string {
	var out []string
	for _, w := range c.Words {
		out = append(out, w.Vars...)
	}
	for _, r := range f.redirects[c] {
		if r.Op == "<<<" && r.Target != nil {
			out = append(out, r.Target.Vars...)
		}
		if r.Heredoc != nil && !r.Heredoc.Quoted {
			out = append(out, varsIn(r.Heredoc.Body)...)
		}
	}
	return out
}

// prints reports whether the command writes its arguments (or its input) to the log: echo, printf and cat with no
// redirection of stdout and not as the stage of a pipeline that feeds another command.
func (f *shellSecretFlow) prints(c *runscript.Command) bool {
	switch c.Name {
	case "echo", "printf", "cat":
	default:
		return false
	}
	if c.Pipeline != nil && c.Stage < len(c.Pipeline.Stages)-1 {
		return false
	}
	stdoutRedirect := func(r *runscript.Redirect) bool {
		return r.Write && (r.Fd == "" || r.Fd == "1" || r.Op == "&>" || r.Op == "&>>")
	}
	for _, r := range f.redirects[c] {
		if stdoutRedirect(r) {
			return false
		}
	}
	for _, r := range f.groups {
		if stdoutRedirect(r) && slices.Contains(r.Inner, c) {
			return false
		}
	}
	for _, w := range c.Args {
		// workflow commands that are not printed back
		if strings.HasPrefix(w.Value, "::set-output") || strings.HasPrefix(w.Value, "::save-state") {
			return false
		}
	}
	if c.Name == "cat" {
		// `cat <<<"$KEY"` and a heredoc that expands it
		for _, r := range f.redirects[c] {
			if r.Op == "<<<" && r.Target != nil {
				for _, name := range r.Target.Vars {
					if d := f.derived[name]; d != nil && !d.masked {
						return true
					}
				}
			}
			if r.Heredoc != nil && !r.Heredoc.Quoted {
				for _, name := range varsIn(r.Heredoc.Body) {
					if d := f.derived[name]; d != nil && !d.masked {
						return true
					}
				}
			}
		}
		return false
	}
	return true
}

func (f *shellSecretFlow) report(loc runscript.Loc, name string, d *derived, kind string) {
	if name != "" {
		d.reported = true
	}
	subject := "the value that this command prints"
	if name != "" {
		subject = "the variable " + quote(name)
	}
	var msg string
	switch kind {
	case "xtrace":
		msg = subject + " is extracted from the secret " + d.secret + " with " + d.how + ", and `set -x` prints it, because the shell shows the assignment and every command with the values expanded. the runner masks only the exact value of a secret, not a part of it, so the value is in the log in clear text. turn tracing off with `set +x` first, and mask the value with `echo \"::add-mask::$VALUE\"`"
	default:
		msg = subject + " is extracted from the secret " + d.secret + " with " + d.how + " and printed here. the runner masks only the exact value of a secret, not a part of it, so the value is in the log in clear text. mask it first with `echo \"::add-mask::$VALUE\"`"
	}
	pos := scriptPos(f.script, f.origin, loc.Offset)
	f.rule.errorIDAt("unredacted-secrets", pos, msg)
}
