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
			if open != nil {
				if t == open.text {
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
				open = &outputDelimiter{name: m[1], text: m[2], random: rule.randomDelimiter(s, n, m[2], at)}
				// a printf or an echo that writes the delimiter and the value in one command
				if c.d.kind == dataUntrusted && c.d.mayHaveNewline() && !open.random {
					rule.report(s, origin, n, w, c, open.name, open.text, "delimiter")
				}
				continue
			}
			if c.d.kind == dataUntrusted && c.d.mayHaveNewline() {
				name := ""
				if m := reRecordName.FindStringSubmatch(t); m != nil {
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
	d    data
	cmd  *runscript.Command
}

// chunksOf splits a write into its pieces in the order they are written: the commands that print, then the lines of
// the here document that feeds them.
func (rule *RuleOutputClobbering) chunksOf(s *runscript.Script, w *runscript.Write) []outputChunk {
	f := rule.flow
	producers := append([]*runscript.Command(nil), w.Producers...)
	sort.SliceStable(producers, func(i, j int) bool { return producers[i].Offset < producers[j].Offset })
	f.loop = false
	var out []outputChunk
	for _, p := range producers {
		f.at = p.Offset
		f.loop = p.LoopBody || p.InFunc
		if neutralCommands[p.Name] || !reachesFile(p, w.Redirect) {
			continue
		}
		out = append(out, outputChunk{text: printedText(p), d: f.judgeCommand(s, p), cmd: p})
	}
	if h := w.Heredoc; h != nil {
		f.at = w.Redirect.Offset
		for _, line := range strings.Split(strings.TrimSuffix(h.Body, "\n"), "\n") {
			d := data{}
			if !h.Quoted {
				d = f.judgeHeredoc(s, &runscript.Heredoc{Quoted: h.Quoted, Body: line, Exprs: exprsIn(line)})
			} else {
				d = f.judgeExprList(exprsIn(line), 0)
			}
			out = append(out, outputChunk{text: line, d: d})
		}
	}
	return out
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
// down to something that cannot add a record. A program that is not in transparentCommands (a script that parses the
// text) decides what is written, and the rule does not know what that is.
func reachesFile(c *runscript.Command, file *runscript.Redirect) bool {
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
			if !transparentCommands[sc.Name] || commandSanitizes(sc, destOutput).satisfies(destOutput) || discardsOutput(sc) {
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

// randomDelimiter reports whether the delimiter of a record is a value that nobody can guess. A delimiter that the
// rule cannot resolve is taken as random: the rule reports only what it knows.
func (rule *RuleOutputClobbering) randomDelimiter(s *runscript.Script, step *Step, text string, at int) bool {
	text = strings.Trim(text, `"'`)
	if strings.Contains(text, "%") {
		return true // a printf format: the delimiter is one of its arguments, which the rule does not follow
	}
	if !strings.ContainsAny(text, "$`") {
		return false // EOF, END_OF_OUTPUT, ghadelimiter: a constant
	}
	if reRandomSource.MatchString(text) {
		return true
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
			if reRandomSource.MatchString(last.Value.Raw) {
				return true
			}
			continue
		}
		if v, ok := rule.flow.envValue(step, name); ok {
			if !strings.Contains(v.Value, "${{") && !reRandomSource.MatchString(v.Value) {
				continue // set to a constant by the workflow
			}
		}
		return true // not known: do not report
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
func (rule *RuleOutputClobbering) clobberable(id, name string) bool {
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
		if c[3] != strings.ToLower(name) {
			return true
		}
	}
	return false
}

func (rule *RuleOutputClobbering) report(s *runscript.Script, origin runscript.Origin, step *Step, w *runscript.Write, c outputChunk, name, delim, kind string) {
	if !rule.clobberable(step.ID.Value, name) {
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
		if !nodeReadsUntrustedInput(r.node) {
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
