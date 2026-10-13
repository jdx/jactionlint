package jactionlint

import (
	"regexp"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// destOutput is the destination of the data flow of this rule: a record of the file $GITHUB_OUTPUT.
const destOutput = "GITHUB_OUTPUT"

// RuleOutputClobbering detects input that an outsider controls and that is written to $GITHUB_OUTPUT in a way that
// lets it end the record and add others.
//
// The file is read line by line. `name=value` sets an output, and a value that holds a newline ends the record: the
// rest is read as the next one. A comment with `x\napproved=true` writes `approved=true` after the `approved=false`
// that the step wrote on purpose, and the later `if: steps.check.outputs.approved == 'true'` believes it. The
// multi-line form `name<<DELIMITER` ... `DELIMITER` has the same flaw when the delimiter is a constant, because the
// value can hold the delimiter on a line of its own: the one thing that makes the form safe is a delimiter that
// nobody can guess (`$(uuidgen)`). See CodeQL's actions/output-clobbering/high.
//
// The rule judges what is written with the data flow of github-env (rule_github_env.go): the value of a variable is
// its last assignment, a value that was validated or cut down to one line is trusted. What differs:
//
//   - only input that can hold a line break counts: the title of an issue, a branch name or the name of an author is
//     one line, a body, a comment or a commit message is not;
//   - the step needs an `id` and the job (or, in an action, its outputs) must read an output of the step other than
//     the one that is written, as `steps.id.outputs.approved`: the attacker can only change what is read. A step
//     with one output that nothing else reads is not reported;
//   - `inputs.*` of a composite action does not count (an action can pass its input on as an output on purpose).
type RuleOutputClobbering struct {
	RuleBase
	runContext
	// flow judges the values; its fields hold the step and the position that is judged.
	flow *RuleGitHubEnv
	refs *refSet
}

// NewRuleOutputClobbering creates a new RuleOutputClobbering instance.
func NewRuleOutputClobbering() *RuleOutputClobbering {
	return &RuleOutputClobbering{
		RuleBase: NewRuleBase("output-clobbering", "Checks for input that an outsider controls and that is written to GITHUB_OUTPUT as a record"),
		flow:     &RuleGitHubEnv{dest: destOutput, skipInputs: true, multilineOnly: true},
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleOutputClobbering) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	rule.refs = nil
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleOutputClobbering) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	rule.refs = nil
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleOutputClobbering) VisitJobPost(n *Job) error {
	rule.leaveJob()
	rule.refs = nil
	return nil
}

// VisitStep is callback when visiting Step node.
func (rule *RuleOutputClobbering) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || n.ID == nil || n.ID.Value == "" || n.ID.ContainsExpression() {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	writes := s.WritesTo("GITHUB_OUTPUT")
	if len(writes) == 0 || rule.gated(n, run) {
		return nil
	}
	rule.flow.runContext = rule.runContext
	rule.flow.step = n
	rule.flow.errexit = rule.shellErrexit(run)
	rule.flow.dest = destOutput
	var open *outputDelimiter
	for _, w := range writes {
		for _, c := range rule.chunksOf(s, w) {
			t := strings.TrimSpace(c.text)
			// a text that is a whole line is judged strictly, except inside a multi-line record (its delimiter decides)
			if open == nil && isWholeLine(t) && !reRecordStart.MatchString(t) {
				c.d = c.dLine
			}
			if open != nil {
				// the closer is compared as the shell expands it: printf '%s\n' "$d" prints the delimiter held by $d
				if printfDelimiter(c.cmd, t, t) == open.text {
					open = nil
					continue
				}
				if c.d.kind == dataUntrusted && c.d.mayHaveNewline() && !open.random {
					rule.report(s, origin, n, w, c, open.name, open.text, "delimiter")
				}
				continue
			}
			if m := reRecordStart.FindStringSubmatch(t); m != nil {
				at := w.Redirect.Offset
				if c.cmd != nil {
					at = c.cmd.Offset
				}
				delim := printfDelimiter(c.cmd, t, m[2])
				open = &outputDelimiter{name: m[1], text: delim, random: rule.randomDelimiter(s, n, delim, at)}
				// a printf or an echo that writes the delimiter and the value in one command
				if c.d.kind == dataUntrusted && c.d.mayHaveNewline() && !open.random {
					rule.report(s, origin, n, w, c, open.name, open.text, "delimiter")
				}
				continue
			}
			if c.d.kind == dataUntrusted && c.d.mayHaveNewline() {
				name := ""
				if len(c.names) > 0 {
					name = c.names[0] // the record whose value is untrusted, not the first of the command
				} else if m := reRecordName.FindStringSubmatch(t); m != nil {
					name = m[1]
				}
				rule.report(s, origin, n, w, c, name, "", "record")
			}
		}
	}
	return nil
}

// gated reports whether the step runs only for people that the workflow trusts: its condition or the condition of the
// job reads who the user is (the association of the author, the actor, a label), or the script checks the permission
// of the user. The rules cannot judge whether the check is enough, so they do not report.
func (rule *RuleOutputClobbering) gated(n *Step, run *ExecRun) bool {
	if (n.If != nil && reUserCond.MatchString(n.If.Value)) || (rule.job != nil && rule.job.If != nil && reUserCond.MatchString(rule.job.If.Value)) {
		return true
	}
	return reUserCheck.MatchString(run.Run.Value)
}

var (
	// reUserCheck matches the check of the permission or of the association of a user in a script.
	reUserCond  = regexp.MustCompile(`author_association|\.user\.(login|type)\b|github\.(triggering_)?actor\b|github\.event\.label\b|\.labels\b|\.head\.repo\b`)
	reUserCheck = regexp.MustCompile(`collaborators/[^\s"]*/permission|author_association|get_?[cC]ollaborator_?[pP]ermission|getCollaboratorPermissionLevel`)
	// reRecordStart matches the first line of a multi-line record: `name<<DELIMITER`.
	reRecordStart = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*)<<(.+)$`)
	// reRecordName matches the name of a `name=value` record.
	reRecordName = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*)=`)
	// reRandomSource matches what makes the delimiter a value that nobody can guess: $RANDOM, a command that prints a random
	// value, a random device. A command is not matched as the name of a variable ($uuid, ${mktemp}).
	reRandomSource = regexp.MustCompile(`\$\{?S?RANDOM\b|\$\(\([^)]*\bS?RANDOM\b|\$\[[^\]]*\bS?RANDOM\b|(?:^|[^$\w{])(?:uuidgen|uuid|mktemp|shuf|secrets)(?:$|\W)|/dev/u?random|\bopenssl\s+rand\b|\bdate\s[^)]*%N`)
	reVarRef       = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`)
)

// outputDelimiter is the open multi-line record: `name<<text`.
type outputDelimiter struct {
	name, text string
	random     bool
}

// outputChunk is a piece of what a script writes to the file: one command or one line of a here document.
type outputChunk struct {
	// text is the first line of the chunk as written (`$BODY`, `body=$BODY`, `body<<EOF`).
	text string
	// d is the judgement of the text as the value of a record with a constant name; dLine the judgement when the text is a
	// whole line (nothing constant in front of it), which only a strict cut-down makes safe.
	d, dLine data
	cmd      *runscript.Command
	// names are the names of the records of the lines that hold a value (a variable or a conversion of printf): what the
	// attacker writes there is not what he adds.
	names []string
}

// chunksOf splits a write into its pieces in the order they are written: the commands that print, then the lines of
// the here document that feeds them.
func (rule *RuleOutputClobbering) chunksOf(s *runscript.Script, w *runscript.Write) []outputChunk {
	f := rule.flow
	producers := append([]*runscript.Command(nil), w.Producers...)
	sort.SliceStable(producers, func(i, j int) bool { return producers[i].Offset < producers[j].Offset })
	f.loop = false
	var out []outputChunk
	judge := func(dest string, fn func() data) data {
		f.dest = dest
		defer func() { f.dest = destOutput }()
		return fn()
	}
	for _, p := range producers {
		f.at = p.Offset
		f.loop = p.LoopBody || p.InFunc
		reachV, reachL := reachesFile(p, w.Redirect, destOutput), reachesFile(p, w.Redirect, destOutputLine)
		if !reachV && !reachL {
			continue
		}
		text := printedText(p)
		if text == "" {
			// `tee -a "$GITHUB_OUTPUT" <<< "body=$BODY"`: the here string is what is written
			for _, r := range p.Redirects {
				if r.Op == "<<<" && r.Target != nil {
					text = r.Target.Value
				}
			}
		}
		c := outputChunk{text: text, cmd: p, names: rule.untrustedRecordNames(s, p)}
		if reachV {
			c.d = judge(destOutput, func() data { return f.judgeCommand(s, p) })
		}
		if reachL {
			c.dLine = judge(destOutputLine, func() data { return f.judgeCommand(s, p) })
		}
		out = append(out, c)
	}
	if h := w.Heredoc; h != nil {
		f.at = w.Redirect.Offset
		for _, line := range strings.Split(strings.TrimSuffix(h.Body, "\n"), "\n") {
			one := func() data {
				if !h.Quoted {
					return f.judgeHeredoc(s, &runscript.Heredoc{Quoted: h.Quoted, Body: line, Exprs: exprsIn(line)})
				}
				return f.judgeExprList(exprsIn(line), 0)
			}
			out = append(out, outputChunk{text: line, d: judge(destOutput, one), dLine: judge(destOutputLine, one)})
		}
	}
	return out
}

// isWholeLine reports whether the first line of a text has no constant record name in front of it (`name=` or `name<<`): an
// outsider who controls it controls the whole record, and one line is enough to forge another output.
func isWholeLine(text string) bool {
	t := strings.TrimSpace(text)
	return !reRecordStart.MatchString(t) && !reRecordName.MatchString(t)
}

// transparentCommands pass on the text that they read, perhaps changed: what an earlier stage of a pipeline printed
// is still in the output.
var transparentCommands = map[string]bool{
	"cat": true, "tee": true, "sed": true, "tr": true, "cut": true, "sort": true, "uniq": true, "head": true, "tail": true,
	"grep": true, "egrep": true, "fgrep": true, "awk": true, "gawk": true, "fold": true, "nl": true, "rev": true, "tac": true,
	"column": true, "paste": true, "jq": true, "yq": true, "envsubst": true,
}

// reachesFile reports whether what the command prints goes into the file: it is not the test of an `if` or a loop, its
// output is not redirected elsewhere, and every later stage of its pipeline passes the text on without cutting it
// down to something that cannot add a record: for a value after a constant name a later stage that leaves one line is
// enough, for a whole line (dest destOutputLine) it has to cut the text down to a strict set. A program that is not in transparentCommands (a script that parses the
// text) decides what is written, and the rule does not know what that is.
func reachesFile(c *runscript.Command, file *runscript.Redirect, dest string) bool {
	if c.Tested || c.LoopCond {
		return false
	}
	for _, r := range c.Redirects {
		if r == file || (r.Fd != "" && r.Fd != "1") {
			continue
		}
		switch r.Op {
		case ">", ">>", ">|", "&>", "&>>", ">&":
			return false
		}
	}
	if c.Pipeline == nil {
		return true
	}
	for _, st := range c.Pipeline.Stages[c.Stage+1:] {
		for _, sc := range st.Commands {
			if !transparentCommands[sc.Name] || commandSanitizes(sc, dest).satisfies(dest) || discardsOutput(sc) {
				return false
			}
		}
	}
	return true
}

// discardsOutput reports whether the command prints nothing: `grep -q`.
func discardsOutput(c *runscript.Command) bool {
	switch c.Name {
	case "grep", "egrep", "fgrep":
		return c.HasFlag("-q", "--quiet", "--silent", "-l", "-L", "--files-with-matches", "--files-without-match")
	}
	return false
}

// printedText is the text a printing command writes, as it is written in the script: the first argument of
// echo (without its options) or the format of printf. It is "" for any other command.
func printedText(c *runscript.Command) string {
	if c.Name != "echo" && c.Name != "printf" {
		return ""
	}
	escapes := c.Name == "printf"
	for _, a := range c.Args {
		if c.Name == "echo" && strings.HasPrefix(a.Value, "-") && !a.Dynamic() {
			escapes = escapes || strings.Contains(a.Value, "e") // `echo -e` reads \n as a line break
			continue
		}
		v := a.Value
		if escapes {
			v = strings.TrimSuffix(strings.ReplaceAll(v, `\n`, "\n"), "\n")
			if i := strings.IndexByte(v, '\n'); i >= 0 {
				v = v[:i]
			}
		}
		return v
	}
	return ""
}

// printedLines is printedText for all the lines that the command prints: the text of echo -e and printf has `\n` for a
// line break, and a quoted string can hold real ones.
func printedLines(c *runscript.Command) []string {
	if c.Name != "echo" && c.Name != "printf" {
		return nil
	}
	escapes := c.Name == "printf"
	for _, a := range c.Args {
		if c.Name == "echo" && strings.HasPrefix(a.Value, "-") && !a.Dynamic() {
			escapes = escapes || strings.Contains(a.Value, "e")
			continue
		}
		v := a.Value
		if escapes {
			v = strings.ReplaceAll(v, `\n`, "\n")
		}
		return strings.Split(strings.TrimRight(v, "\n"), "\n")
	}
	return nil
}

var (
	// reConversion matches a conversion of the format of printf; %% is none.
	reConversion = regexp.MustCompile(`%[-+ #0-9.*]*[a-zA-Z]|%%`)
)

// untrustedRecordNames returns the names of the records, among the lines that an echo or a printf prints, whose value is
// untrusted text that can hold a line break: a variable or an expression of the line, or the argument that a conversion of
// printf takes. A record whose value is a trusted variable or a literal is not in the list.
func (rule *RuleOutputClobbering) untrustedRecordNames(s *runscript.Script, p *runscript.Command) []string {
	if p.Name != "echo" && p.Name != "printf" {
		return nil
	}
	var words []*runscript.Word
	for _, a := range p.Args {
		if p.Name == "echo" && strings.HasPrefix(a.Value, "-") && !a.Dynamic() {
			continue
		}
		words = append(words, a)
	}
	if len(words) == 0 {
		return nil
	}
	first, rest := words[0], words[1:]
	f := rule.flow
	f.at = p.Offset
	f.loop = p.LoopBody || p.InFunc
	hostile := func(d data) bool { return d.kind == dataUntrusted && d.mayHaveNewline() }
	conv := 0
	var names []string
	for _, l := range printedLines(p) {
		tainted := false
		whole := isWholeLine(l)
		f.dest = destOutput
		if whole {
			f.dest = destOutputLine
		}
		for _, m := range reVarRef.FindAllStringSubmatch(l, -1) {
			for _, v := range first.Vars {
				if v == m[1] && hostile(f.judgeVar(s, v, 0)) {
					tainted = true
				}
			}
		}
		if len(first.Exprs) > 0 && strings.Contains(l, "${{") && hostile(f.judgeExprList(exprsIn(l), 0)) {
			tainted = true
		}
		if p.Name == "printf" {
			for _, cv := range reConversion.FindAllString(l, -1) {
				if cv == "%%" {
					continue
				}
				if conv < len(rest) && hostile(f.judgeWord(s, rest[conv], 0)) {
					tainted = true
				}
				conv++
			}
		}
		if !tainted {
			continue
		}
		if m := reRecordStart.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			names = append(names, m[1])
		} else if m := reRecordName.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			names = append(names, m[1])
		} else {
			names = append(names, "") // a whole line: the attacker writes the name too
		}
	}
	f.dest = destOutput
	return names
}

// randomDelimiter reports whether the delimiter of a record is a value that nobody can guess. A delimiter that the
// rule cannot resolve is taken as random: the rule reports only what it knows.
func (rule *RuleOutputClobbering) randomDelimiter(s *runscript.Script, step *Step, text string, at int) bool {
	return rule.randomText(s, step, strings.Trim(text, `"'`), at, 0)
}

// randomText reports whether the text, as the shell expands it at the offset, holds something that nobody can guess: a
// random source, a variable of the script that was assigned one (its value is followed), or an environment value of the
// workflow that holds a secret or the token.
func (rule *RuleOutputClobbering) randomText(s *runscript.Script, step *Step, text string, at, depth int) bool {
	if !strings.ContainsAny(text, "$`") {
		return false // EOF, END_OF_OUTPUT, ghadelimiter: a constant
	}
	if reRandomSource.MatchString(text) {
		return true
	}
	if depth > 4 {
		return true // too deep to follow: do not report
	}
	for _, m := range reVarRef.FindAllStringSubmatch(text, -1) {
		name := m[1]
		// the assignment in effect for the command: the last plain one before it (`VAR=x cmd` only sets the
		// environment of cmd, and a later assignment has not happened yet)
		var last *runscript.Assignment
		for _, a := range s.Assignments {
			if a.Name != name || a.Value == nil || a.Offset >= at || (a.Cmd != nil && !a.Cmd.Decl) {
				continue
			}
			if last == nil || a.Offset > last.Offset {
				last = a
			}
		}
		if last != nil {
			if rule.randomText(s, step, last.Value.Raw, last.Offset, depth+1) {
				return true
			}
			continue
		}
		if v, ok := rule.flow.envValue(step, name); ok {
			// what the workflow set is a constant, an expression that has a value somebody can read or choose (the id of the
			// run, text of the event), or a secret or the token
			if exprsUnguessable(v.Value) {
				return true
			}
			continue
		}
		return true // not known: do not report
	}
	return false
}

// printfDelimiter resolves the conversions of the delimiter of a record that printf writes (`printf 'body<<%s\n' "$d"`) to
// the words they take, so that the delimiter is judged like the one of echo. It returns the text unchanged for any other
// command, and "$" (an unknown, never reported) when an argument is missing.
func printfDelimiter(c *runscript.Command, line, delim string) string {
	if c == nil || c.Name != "printf" || !strings.Contains(delim, "%") {
		return delim
	}
	words := append([]*runscript.Word(nil), c.Args...)
	if len(words) == 0 {
		return delim
	}
	args := words[1:]
	idx := len(reConversion.FindAllString(strings.TrimSuffix(line, delim), -1))
	return reConversion.ReplaceAllStringFunc(delim, func(cv string) string {
		if cv == "%%" {
			return "%"
		}
		if idx < len(args) {
			idx++
			return strings.Trim(args[idx-1].Raw, `"'`)
		}
		idx++
		return "$UNKNOWN_PRINTF_ARGUMENT"
	})
}

// exprsUnguessable reports whether a value of the workflow holds an expression that nobody can guess: a secret or the token.
// The expressions are read as expressions, with or without spaces inside `${{ }}`; the text around them is a literal. What
// reRandomSource matches is shell text, not YAML.
func exprsUnguessable(value string) bool {
	for _, e := range exprsIn(value) {
		n := parseExprText(e)
		if n == nil {
			continue
		}
		var refs []exprRef
		collectExprRefs(n, nil, &refs)
		for _, r := range refs {
			if r.chain[0] == "secrets" || (len(r.chain) == 2 && r.chain[0] == "github" && r.chain[1] == "token") {
				return true
			}
		}
	}
	return false
}

// outputReads returns the references of the job (or of the action) to outputs of the steps.
func (rule *RuleOutputClobbering) outputRefs() *refSet {
	if rule.refs == nil {
		switch {
		case rule.wf != nil && rule.wf.Action != nil:
			rule.refs = workflowRefs(rule.wf) // the outputs of the action
		case rule.job != nil:
			rule.refs = jobRefs(rule.job)
		case rule.wf != nil:
			rule.refs = workflowRefs(rule.wf)
		default:
			rule.refs = &refSet{unknown: true}
		}
	}
	return rule.refs
}

// clobberable reports whether the job reads an output of the step other than name: that is what an attacker can set.
func (rule *RuleOutputClobbering) clobberable(id string, names ...string) bool {
	rs := rule.outputRefs()
	if rs.unknown {
		return false // a reference may be in an expression that does not parse; the rule reports only what it knows
	}
	for _, r := range rs.refs {
		c := r.chain
		if len(c) < 3 || c[0] != "steps" || !strings.EqualFold(c[1], id) || c[2] != "outputs" {
			continue
		}
		if len(c) == 3 {
			return true // the whole object: toJSON(steps.id.outputs)
		}
		written := false
		for _, n := range names {
			if c[3] == strings.ToLower(n) {
				written = true
			}
		}
		if !written {
			return true
		}
	}
	return false
}

func (rule *RuleOutputClobbering) report(s *runscript.Script, origin runscript.Origin, step *Step, w *runscript.Write, c outputChunk, name, delim, kind string) {
	// the record of the line that holds the value is the one that the attacker writes; the other lines of the command are
	// records of the script
	names := c.names
	if kind == "delimiter" || len(names) == 0 {
		names = []string{name}
	}
	if !rule.clobberable(step.ID.Value, names...) {
		return
	}
	r := w.Redirect
	src := "untrusted input " + describeUntrusted(c.d)
	var msg string
	switch kind {
	case "delimiter":
		msg = src + " is written to $GITHUB_OUTPUT between the delimiter " + quote(delim) + ", which is a constant, so the value can hold a line with the delimiter and then any other record: an attacker who controls it sets the other outputs of the step (steps." + step.ID.Value + ".outputs.*) that later steps trust. a constant delimiter is not a fix. use a delimiter that nobody can guess (delimiter=$(uuidgen); echo \"" + name + "<<$delimiter\" ...; echo \"$delimiter\") or write a value without line breaks"
	default:
		what := "a record"
		if name != "" {
			what = "the record " + quote(name+"=...")
		}
		msg = src + " is written to $GITHUB_OUTPUT as " + what + " and can hold a line break: an attacker who controls it ends the record and adds others, so it sets the other outputs of the step (steps." + step.ID.Value + ".outputs.*) that later steps trust. write a value without line breaks (validate it, or cut it down with tr -d '\\n'), or write it between a delimiter that nobody can guess (delimiter=$(uuidgen); echo \"" + nameOrValue(name) + "<<$delimiter\" ...; echo \"$delimiter\")"
	}
	rule.errorIDAt("output-clobbering", scriptPos(s, origin, r.Offset), msg).endAt(scriptPos(s, origin, r.End))
}

func nameOrValue(name string) string {
	if name == "" {
		return "name"
	}
	return name
}

// singleLineProps are the last properties of the untrusted inputs that hold one line: a title, the name of a branch
// or of an account. The branch name of a pull request cannot hold a line break, git refuses control characters.
var singleLineProps = map[string]bool{
	"title": true, "ref": true, "head_ref": true, "head_branch": true, "label": true, "name": true, "email": true,
	"login": true, "page_name": true, "default_branch": true, "homepage": true,
}

// exprMayHaveNewline reports whether an untrusted expression can hold a line break: some property that it reads is
// not known to be one line.
func exprMayHaveNewline(text string) bool {
	n := parseExprText(text)
	if n == nil {
		return true
	}
	var refs []exprRef
	collectExprRefs(n, nil, &refs)
	untrusted := false
	for _, r := range refs {
		if !exprReadsUntrustedInput(strings.Join(r.chain, ".")) {
			continue
		}
		untrusted = true
		if !singleLineProps[r.chain[len(r.chain)-1]] {
			return true
		}
	}
	return !untrusted
}

// cannotAddRecords reports whether what the command prints is a number, a hash or base64 on one line, whatever it
// reads: no value of the input can add a record.
func cannotAddRecords(c *runscript.Command) bool {
	switch c.Name {
	case "base64":
		return !c.HasFlag("-d", "-D", "--decode")
	case "wc", "md5sum", "sha1sum", "sha224sum", "sha256sum", "sha384sum", "sha512sum", "b2sum", "cksum", "shasum":
		return true
	case "grep", "egrep", "fgrep":
		return c.HasFlag("-c", "--count")
	}
	return false
}

func init() {
	registerRules(
		RuleInfo{ID: "output-clobbering", Group: RuleGroupSecurity, Summary: "Input that an outsider controls is written to GITHUB_OUTPUT as a record, so a line break in it sets the other outputs of the step.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-output-clobbering"},
	)
	registerRuleFactory("output-clobbering", func(env *RuleEnv) []Rule {
		if !env.config.RuleEnabled("output-clobbering") {
			return nil
		}
		return []Rule{NewRuleOutputClobbering()}
	})
}
