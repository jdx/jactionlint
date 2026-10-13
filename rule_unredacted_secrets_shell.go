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
	paramCutRe       = `(?:#{1,2}|%{1,2}|/|\^{1,2}|,{1,2}|::?[0-9]|: +-?[0-9])`
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
			// a scope that sets the name again replaces what the scope before it set: a literal or another expression
			// is no secret
			delete(f.secrets, v.Name.Value)
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
	// the assignments inside a command substitution run in a subshell: they do not set a variable of the script
	var subSpans [][2]int
	for _, c := range s.Commands {
		for _, w := range c.Words {
			if len(w.Subs) > 0 {
				subSpans = append(subSpans, [2]int{w.Offset, w.End})
			}
		}
	}
	for _, a := range s.Assignments {
		if a.Value != nil && len(a.Value.Subs) > 0 {
			subSpans = append(subSpans, [2]int{a.Value.Offset, a.Value.End})
		}
	}
	// ... and so do the substitutions of a redirect target (`<<<"$(cmd)"`) and the commands in the body of a heredoc
	for _, r := range s.Redirects {
		if r.Target != nil {
			for _, sub := range r.Target.Subs {
				f.inSub[sub] = true
			}
			if len(r.Target.Subs) > 0 {
				subSpans = append(subSpans, [2]int{r.Target.Offset, r.Target.End})
			}
		}
		if r.Heredoc != nil {
			subSpans = append(subSpans, [2]int{r.Heredoc.Offset, r.Heredoc.End})
			for _, c := range s.Commands {
				if c.Offset >= r.Heredoc.Offset && c.Offset < r.Heredoc.End {
					f.inSub[c] = true
				}
			}
		}
	}
	inSubshell := func(offset int) bool {
		return slices.ContainsFunc(subSpans, func(sp [2]int) bool { return offset >= sp[0] && offset < sp[1] })
	}
	var events []secretFlowEvent
	for _, a := range s.Assignments {
		if (a.Cmd == nil || a.Cmd.Decl) && !inSubshell(a.Offset) {
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

// The model has one question and one answer: what does this word hold, in terms of secrets, once the shell has expanded
// it (wordValue)? Everything else asks it: an assignment gives the variable that value, a print looks at the value of the
// words it writes, and the trace of `set -x` looks at every command that expands one.
//
//   - a variable holds what the script put into it (a secret, or a cut of one);
//   - `${{ secrets.X }}` holds the secret;
//   - `${SECRET#prefix}` and its kind hold a cut;
//   - a command substitution holds what its last command prints: followed back along the pipe, through echo, printf, cat and
//     the tools that cut a text, down to a command that reads a secret (a here-string, a heredoc or, for the tools that run a
//     program, an argument), and each of those reads is a word or a heredoc again. Any other command in the way (curl,
//     wc, sha256sum) holds nothing.
//
// The value is "exact" (how is empty: the runner masks it) or "cut" (how names the tool).

// wval is what a word holds.
type wval struct {
	ok     bool   // it holds a secret, exactly or cut
	secret string // the name of the secret
	how    string // empty for the exact secret, else how it was cut
	name   string // the variable that holds it, when it comes from one
}

func (v wval) derived() bool { return v.ok && v.how != "" }

// better keeps the value that is the cut one, and the first of two that are alike.
func better(a, b wval) wval {
	if !a.ok || a.how == "" && b.ok && b.how != "" {
		return b
	}
	return a
}

// wordValue is what the word holds after it is expanded.
func (f *shellSecretFlow) wordValue(w *runscript.Word) wval {
	var v wval
	for _, name := range w.Vars {
		if s, ok := f.secrets[name]; ok {
			v = better(v, wval{true, s, "", name})
		}
		if d, ok := f.derived[name]; ok {
			if d.masked { // the runner knows the value now, as it knows a secret
				v = better(v, wval{true, d.secret, "", name})
			} else {
				v = better(v, wval{true, d.secret, d.how, name})
			}
		}
	}
	for _, e := range w.Exprs {
		if m := secretExprRe.FindStringSubmatch(e); m != nil {
			v = better(v, wval{true, strings.ToUpper(m[1] + m[2]), "", ""})
		}
	}
	if s, ok := f.cutsSecret(w); ok {
		v = better(v, wval{true, s, "a cut of its value", ""})
	}
	// a process substitution is the name of a file, not its text: it is not followed
	if len(w.Subs) > 0 && !w.ProcSubst {
		v = better(v, f.substitutionValue(w))
	}
	return v
}

// commandInput is the best value that the command reads as data: a here-string, a heredoc that expands, and, with args,
// its arguments (the tools that run a program take the secret there).
func (f *shellSecretFlow) commandInput(c *runscript.Command, args bool) wval {
	var v wval
	for _, r := range f.redirects[c] {
		if r.Op == "<<<" && r.Target != nil {
			v = better(v, f.wordValue(r.Target))
		}
		if r.Heredoc != nil && !r.Heredoc.Quoted {
			for _, name := range varsIn(r.Heredoc.Body) {
				if s, ok := f.secrets[name]; ok {
					v = better(v, wval{true, s, "", name})
				}
				if d, ok := f.derived[name]; ok && !d.masked {
					v = better(v, wval{true, d.secret, d.how, name})
				}
			}
			for _, e := range r.Heredoc.Exprs {
				if m := secretExprRe.FindStringSubmatch(e); m != nil {
					v = better(v, wval{true, strings.ToUpper(m[1] + m[2]), "", ""})
				}
			}
		}
	}
	if args {
		for _, w := range c.Words {
			v = better(v, f.wordValue(w))
		}
	}
	return v
}

// substitutionValue is what the command substitutions of the word hold: what the last command prints, followed back along
// its pipe.
func (f *shellSecretFlow) substitutionValue(w *runscript.Word) wval {
	// the commands that are inside the words or the here-strings of another command of the substitution are its inputs,
	// not its last command
	nested := map[*runscript.Command]bool{}
	for _, sub := range w.Subs {
		for _, sw := range sub.Words {
			for _, n := range sw.Subs {
				nested[n] = true
			}
		}
		for _, r := range f.redirects[sub] {
			if r.Target != nil {
				for _, n := range r.Target.Subs {
					nested[n] = true
				}
			}
			if r.Heredoc != nil { // the commands in the body of a heredoc
				for _, n := range w.Subs {
					if n.Offset >= r.Heredoc.Offset && n.Offset < r.Heredoc.End {
						nested[n] = true
					}
				}
			}
		}
	}
	var last *runscript.Command
	for _, sub := range w.Subs {
		if !nested[sub] && (last == nil || sub.End > last.End) {
			last = sub
		}
	}
	if last == nil {
		return wval{}
	}
	return f.chainValue(f.pipeChain(last, last.Stage))
}

// pipeChain is the commands of the pipeline of c from the stage back to the first, or c itself when it is in no pipeline.
func (f *shellSecretFlow) pipeChain(c *runscript.Command, stage int) []*runscript.Command {
	if c.Pipeline == nil {
		return []*runscript.Command{c}
	}
	var chain []*runscript.Command
	for i := stage; i >= 0; i-- {
		if cs := c.Pipeline.Stages[i].Commands; len(cs) == 1 {
			chain = append(chain, cs[0])
		} else {
			break
		}
	}
	return chain
}

// chainValue is what the output of the first command of the chain holds, the chain going back along a pipe: it is cut from
// a secret when only echo, printf, cat and the tools that cut a text stand between them, and holds nothing when another
// command is in the way (`curl -u "$S" ... | jq -r .id` reads the answer of a service, `... | wc -c` counts).
func (f *shellSecretFlow) chainValue(chain []*runscript.Command) wval {
	how := ""
	for _, c := range chain {
		switch {
		case isDerivationTool(c.Name):
			if how == "" {
				how = c.Name
			}
			if in := f.commandInput(c, codeTools[c.Name] || pythonToolNameRe.MatchString(c.Name)); in.ok {
				return wval{true, in.secret, how, ""}
			}
		case c.Name == "echo" || c.Name == "printf" || c.Name == "cat":
			if in := f.commandInput(c, true); in.ok {
				if how == "" {
					how = in.how // a cut value that is only passed on is still that cut
				}
				return wval{true, in.secret, how, ""}
			}
		default:
			return wval{}
		}
	}
	return wval{}
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

// cutsSecret reports whether the word expands a variable that holds a secret in a way that cuts it
// (`${SECRET#prefix}`, `${SECRET:0:8}`, `${SECRET//x/y}`).
func (f *shellSecretFlow) cutsSecret(w *runscript.Word) (secret string, ok bool) {
	for _, name := range w.Vars {
		s, isSecret := f.secretOf(name)
		if !isSecret {
			continue
		}
		if regexp.MustCompile(`\$\{` + regexp.QuoteMeta(name) + paramCutRe).MatchString(w.Raw) {
			return s, true
		}
	}
	return "", false
}

// secretOf returns the secret that a variable holds or was cut from.
func (f *shellSecretFlow) secretOf(name string) (string, bool) {
	if s, ok := f.secrets[name]; ok {
		return s, true
	}
	if d, ok := f.derived[name]; ok {
		return d.secret, true
	}
	return "", false
}

// set gives a variable the value: an exact secret stays one, a cut value is derived, and anything else is neither. With
// appends the old value stays in it. Under `set -x` the shell shows the assignment with the value expanded.
func (f *shellSecretFlow) set(loc runscript.Loc, name string, v wval, appends bool) {
	if appends {
		if f.derived[name] == nil && v.derived() {
			f.derived[name] = &derived{secret: v.secret, how: v.how}
		}
	} else {
		delete(f.derived, name)
		delete(f.secrets, name)
		switch {
		case v.ok && v.how == "":
			f.secrets[name] = v.secret
		case v.derived():
			f.derived[name] = &derived{secret: v.secret, how: v.how}
		}
	}
	if d := f.derived[name]; d != nil && f.xtrace && !d.masked && !d.reported {
		f.report(loc, name, d, "xtrace")
	}
}

func (f *shellSecretFlow) assign(a *runscript.Assignment) {
	if a.Array {
		delete(f.derived, a.Name)
		delete(f.secrets, a.Name)
		return
	}
	var v wval
	if a.Value != nil {
		v = f.wordValue(a.Value)
	}
	f.set(a.Loc, a.Name, v, a.Append)
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
	// `printf -v NAME FORMAT ARGS` assigns the text to NAME and prints nothing
	if c.Name == "printf" {
		for i, a := range c.Args {
			if a.Value == "-v" && i+2 <= len(c.Args)-1 && !c.Args[i+1].Dynamic() {
				var v wval
				for _, w := range c.Args[i+2:] {
					v = better(v, f.wordValue(w))
				}
				f.set(c.Loc, c.Args[i+1].Value, v, false)
				return
			}
		}
	}
	isMask := false
	for _, w := range c.Args {
		if strings.HasPrefix(w.Value, "::add-mask::") && (c.Name == "echo" || c.Name == "printf") {
			isMask = true
		}
	}
	switch {
	case f.xtrace:
		// the trace shows every command with its expansions: a variable that is cut from a secret, also inside a
		// substitution, and a substitution that cuts one
		for _, name := range f.allVars(c) {
			if d := f.derived[name]; d != nil && !d.masked && !d.reported {
				f.report(c.Loc, name, d, "xtrace")
			}
		}
		for _, w := range c.Words {
			if len(w.Subs) > 0 {
				if v := f.substitutionValue(w); v.derived() {
					f.report(c.Loc, "", &derived{secret: v.secret, how: v.how}, "xtrace")
				}
			}
		}
	case f.prints(c) && !isMask:
		for _, v := range f.printed(c) {
			if !v.derived() {
				continue
			}
			if v.name == "" {
				f.report(c.Loc, "", &derived{secret: v.secret, how: v.how}, "print")
			} else if d := f.derived[v.name]; d != nil && !d.masked && !d.reported {
				f.report(c.Loc, v.name, d, "print")
			}
		}
	}
	// the runner only reads the mask from the output of the step: a mask that is redirected to a file or piped on is none
	if isMask && f.prints(c) {
		for _, w := range c.Args {
			for _, name := range w.Vars {
				if d := f.derived[name]; d != nil {
					d.masked = true
				}
			}
		}
	}
}

// printed returns what the command writes: the words of echo and printf, and the input of cat.
func (f *shellSecretFlow) printed(c *runscript.Command) []wval {
	if c.Name == "cat" {
		v := f.commandInput(c, false)
		if c.Pipeline != nil && c.Stage > 0 { // `echo "$KEY" | cat`: the input comes along the pipe
			v = better(v, f.chainValue(f.pipeChain(c, c.Stage-1)))
		}
		return []wval{v}
	}
	var out []wval
	for _, w := range c.Words {
		out = append(out, f.wordValue(w))
	}
	return out
}

// allVars returns the shell variables that the command expands: in its words, in the commands inside its substitutions, in
// a here-string and in a heredoc that expands them.
func (f *shellSecretFlow) allVars(c *runscript.Command) []string {
	var out []string
	one := func(c *runscript.Command) {
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
	}
	one(c)
	for _, w := range c.Words {
		for _, sub := range w.Subs { // at any depth
			one(sub)
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
