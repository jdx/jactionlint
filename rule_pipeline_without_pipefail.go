package jactionlint

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RulePipelineWithoutPipefail is a rule to detect pipelines in "run:" scripts whose failures are hidden because
// the shell does not enable pipefail.
//
// The default shell of Linux and macOS runners is `bash -e {0}`, and `shell: sh` is `sh -e {0}`. Without
// pipefail the exit status of `cmd1 | cmd2` is the one of `cmd2`, so when `cmd1` fails the step still succeeds.
// An explicit `shell: bash` runs `bash --noprofile --norc -eo pipefail {0}`, which does not have the problem.
//
// A pipeline is reported when all of the following hold:
//
//   - the shell has no pipefail: the default shell on a runner which is known not to be Windows (the default shell
//     of Windows is pwsh), `shell: sh`, or a custom template for bash or sh without "pipefail" which turns errexit
//     on (a template without errexit does not stop the step at the failure anyway).
//     The shell is the one of the step, of "defaults.run.shell" of the job or of the workflow.
//   - the script does not turn pipefail on before the pipeline (`set -o pipefail`, `set -eo pipefail`,
//     `set -euxo pipefail`, `set -o errexit -o pipefail`, an assignment of SHELLOPTS with pipefail) or turns it off
//     again with `set +o pipefail`.
//   - a stage in front of the last one is a command whose failure matters. Commands which cannot meaningfully fail
//     do not count (see pipefailNoFail, e.g. echo and printf, cat without a file, true and yes), and neither do
//     filters in the middle of a pipeline (see pipefailFilters, e.g. sed and sort: the failure of what feeds them
//     is reported at that command). Neither do grep, rg and diff anywhere: their non-zero status is an answer
//     ("no match", "files differ"), and pipefail would turn an expected "no match" into a failed step.
//   - the script does not handle the status itself: the pipeline is not negated with `!`, not the condition of `if`,
//     `elif`, `while` or `until` and not an operand of `&&` or `||` other than the last one of a list, where `set -e`
//     does not stop the script either. The commands of the first stages are held to the same rule, so
//     `{ git notes show || true; cat note; } | sort` does not blame `git`.
//   - no later stage stops reading early: `head`, `grep -q`, `grep -m`, `read`, `sed ...q` or `awk ... exit` make the
//     producer die with SIGPIPE. Pipefail would then make a working pipeline fail, so enabling it is not a fix.
//     Such pipelines are not reported at all.
//
// The fix inserts `set -o pipefail` as the first line of a literal block script. It is unsafe because failures which
// were hidden now fail the step. There is no fix for `shell: sh` (dash has no pipefail), for custom shells other than
// bash and for scripts which are not a literal block (`run: cmd1 | cmd2`).
type RulePipelineWithoutPipefail struct {
	RuleBase
	src           []byte
	starts        []int // the line starts of src, see buildLineStarts
	workflowShell *String
	jobShell      *String
	nonWindows    bool
	matrix        *Matrix
}

// NewRulePipelineWithoutPipefail creates a new RulePipelineWithoutPipefail instance. src is the content of the
// file, which is needed to attach fixes. It can be nil, then findings have no fix.
func NewRulePipelineWithoutPipefail(src []byte) *RulePipelineWithoutPipefail {
	return &RulePipelineWithoutPipefail{
		RuleBase: RuleBase{
			name: "pipeline-without-pipefail",
			desc: "Checks that failures of commands in pipelines in \"run:\" are not hidden by a shell without pipefail",
		},
		src: src,
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RulePipelineWithoutPipefail) VisitWorkflowPre(n *Workflow) error {
	rule.workflowShell = nil
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.workflowShell = n.Defaults.Run.Shell
	}
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RulePipelineWithoutPipefail) VisitJobPre(n *Job) error {
	rule.jobShell = nil
	if n.Defaults != nil && n.Defaults.Run != nil {
		rule.jobShell = n.Defaults.Run.Shell
	}
	rule.matrix = nil
	if n.Strategy != nil {
		rule.matrix = n.Strategy.Matrix
	}
	rule.nonWindows = rule.runnerIsNotWindows(n.RunsOn)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RulePipelineWithoutPipefail) VisitJobPost(n *Job) error {
	rule.jobShell, rule.matrix, rule.nonWindows = nil, nil, false
	return nil
}

// pipefailShellKind tells what a shell does with pipelines.
type pipefailShellKind int

const (
	pipefailShellOK         pipefailShellKind = iota // pipefail is on, or the shell is not analyzed
	pipefailShellDefault                             // the default shell: bash -e {0}
	pipefailShellSh                                  // sh: sh -e {0}
	pipefailShellTemplate                            // a custom template of bash without pipefail
	pipefailShellShTemplate                          // a custom template of sh, which has no pipefail to turn on
)

// VisitStep is callback when visiting Step node.
func (rule *RulePipelineWithoutPipefail) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || run.Run.Value == "" {
		return nil
	}
	shell := run.Shell
	if shell == nil {
		shell = rule.jobShell
	}
	if shell == nil {
		shell = rule.workflowShell
	}
	kind, value, tmplBash := rule.classifyShell(shell)
	if kind == pipefailShellOK {
		return nil
	}
	if kind == pipefailShellDefault && !rule.nonWindows {
		return nil // the default shell of Windows is pwsh, and the runner is not known to be anything else
	}

	script, err := runscript.Analyze(run.Run.Value, value)
	if err != nil || len(script.Pipelines) == 0 {
		return nil
	}
	if (kind == pipefailShellTemplate || kind == pipefailShellShTemplate) && !templateErrexit(value) && !scriptSets(script, "errexit", 'e') {
		return nil // a failing command does not stop the step, so there is nothing to hide
	}

	origin := run.Run.scriptOrigin()
	events := pipefailEvents(script)
	// The fix turns pipefail on for the whole script. A pipeline that ends in a consumer which quits early (head,
	// grep -q) is not reported because pipefail would make it die with SIGPIPE, so a script that has one is left alone.
	fixable := (kind == pipefailShellDefault || (kind == pipefailShellTemplate && tmplBash)) && !hasExprInShell(shell) && !anyPipelineQuitsEarly(script)
	var fix *Fix
	fixBuilt := false

	discarded := discardedSubstitutions(script)
	for _, p := range script.Pipelines {
		if p.Negated || p.Tested || pipefailOnAt(events, p.Offset) || pipelineIsDiscarded(p, discarded) {
			continue
		}
		c := hiddenFailure(p)
		if c == nil {
			continue
		}
		pos := script.Position(origin, p.Offset)
		rule.ReportIDf(
			"pipeline-without-pipefail",
			&Pos{Line: pos.Line, Col: pos.Col},
			"failure of %s is hidden in the pipeline %q because %s. %s",
			quoteCommand(c),
			pipelineSnippet(script.Source, p),
			shellWithoutPipefail(kind, value),
			pipefailAdvice(kind),
		)
		if !fixable {
			continue
		}
		if !fixBuilt {
			fix, fixBuilt = rule.buildFix(run.Run), true
		}
		rule.errs[len(rule.errs)-1].Fix = fix
	}
	return nil
}

// discardedSubstitutions returns the commands inside the command substitutions in the arguments of commands, like the
// `sha256sum f | cut -d' ' -f1` of `echo "hash=$(sha256sum f | cut -d' ' -f1)"`. The status of such a substitution is
// not the status of anything: the command that gets the argument runs and has its own status, with or without
// pipefail, so a failure in the pipeline cannot stop the step. The value of an assignment (`x=$(a | b)`) is different, it
// is the status of the assignment, and so are the declarations (`local x=$(...)` has the status of local, though).
func discardedSubstitutions(s *runscript.Script) map[*runscript.Command]bool {
	var m map[*runscript.Command]bool
	for _, c := range s.Commands {
		if c.Name == "" {
			continue // assignments only
		}
		for _, w := range c.Args {
			for _, sub := range w.Subs {
				if m == nil {
					m = map[*runscript.Command]bool{}
				}
				m[sub] = true
			}
		}
	}
	return m
}

// pipelineIsDiscarded reports whether the pipeline is one inside a substitution of discardedSubstitutions.
func pipelineIsDiscarded(p *runscript.Pipeline, discarded map[*runscript.Command]bool) bool {
	if discarded == nil {
		return false
	}
	for _, st := range p.Stages {
		for _, c := range st.Commands {
			if discarded[c] {
				return true
			}
		}
	}
	return false
}

func hasExprInShell(s *String) bool { return s != nil && s.ContainsExpression() }

// classifyShell tells whether scripts of the step run in a shell without pipefail. value is the shell to pass to
// the analyzer. bash is whether a custom template runs bash.
func (rule *RulePipelineWithoutPipefail) classifyShell(s *String) (kind pipefailShellKind, value string, bash bool) {
	if s == nil || strings.TrimSpace(s.Value) == "" {
		return pipefailShellDefault, "", true
	}
	if s.ContainsExpression() {
		return pipefailShellOK, "", false
	}
	f := strings.Fields(s.Value)
	name := strings.ToLower(f[0])
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".exe")
	if name != "bash" && name != "sh" {
		return pipefailShellOK, "", false
	}
	if !strings.Contains(s.Value, "{0}") {
		// "bash" and "sh" are the names GitHub knows: bash runs with pipefail, sh does not.
		if len(f) == 1 {
			if name == "bash" {
				return pipefailShellOK, "", false
			}
			return pipefailShellSh, "sh", false
		}
		// Not a template and not a name: invalid, reported by invalid-shell-name
		return pipefailShellOK, "", false
	}
	if strings.Contains(strings.ToLower(s.Value), "pipefail") {
		return pipefailShellOK, "", false
	}
	if name == "sh" {
		return pipefailShellShTemplate, s.Value, false
	}
	return pipefailShellTemplate, s.Value, true
}

// templateErrexit returns whether the custom shell template turns errexit on (`-e`, `-eu`, `-o errexit`).
func templateErrexit(tmpl string) bool {
	f := strings.Fields(tmpl)
	for i := 1; i < len(f); i++ {
		a := f[i]
		if a == "{0}" {
			break
		}
		if a == "-o" && i+1 < len(f) && f[i+1] == "errexit" {
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.Contains(a, "e") {
			return true
		}
	}
	return false
}

func shellWithoutPipefail(kind pipefailShellKind, value string) string {
	switch kind {
	case pipefailShellSh:
		return `"shell: sh" runs "sh -e {0}" without pipefail`
	case pipefailShellTemplate, pipefailShellShTemplate:
		return fmt.Sprintf("the custom shell %q does not enable pipefail", value)
	default:
		return `the default shell runs "bash -e {0}" without pipefail`
	}
}

func pipefailAdvice(kind pipefailShellKind) string {
	switch kind {
	case pipefailShellSh, pipefailShellShTemplate:
		return `use "shell: bash", which runs with pipefail`
	case pipefailShellTemplate:
		return `add "-o pipefail" to the shell or "set -o pipefail" before the pipeline`
	default:
		return `add "set -o pipefail" before the pipeline or use "shell: bash", which runs with pipefail`
	}
}

func quoteCommand(c *runscript.Command) string {
	if c.Name != "" {
		return fmt.Sprintf("%q", c.Name)
	}
	if c.NameWord != nil {
		return fmt.Sprintf("%q", c.NameWord.Raw)
	}
	return "a command"
}

// pipelineSnippet is the first line of the pipeline, shortened.
func pipelineSnippet(src string, p *runscript.Pipeline) string {
	if p.Offset < 0 || p.End > len(src) || p.Offset >= p.End {
		return ""
	}
	s := src[p.Offset:p.End]
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " ..."
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "\\"))
	const max = 60
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "..."
	}
	return s
}

// pipefailEvent is a command which turns pipefail on or off.
type pipefailEvent struct {
	offset int
	on     bool
}

// pipefailEvents returns the places where the script changes pipefail, in source order.
func pipefailEvents(s *runscript.Script) []pipefailEvent {
	var ev []pipefailEvent
	for _, c := range s.Commands {
		if c.Name != "set" {
			continue
		}
		if on, ok := setOption(c.Args, "pipefail", 0); ok {
			ev = append(ev, pipefailEvent{c.Offset, on})
		}
	}
	for _, a := range s.Assignments {
		if a.Name == "SHELLOPTS" && a.Value != nil && strings.Contains(a.Value.Value, "pipefail") {
			ev = append(ev, pipefailEvent{a.Offset, true})
		}
	}
	// commands and assignments are each in source order, merge them
	for i := 1; i < len(ev); i++ {
		for j := i; j > 0 && ev[j].offset < ev[j-1].offset; j-- {
			ev[j], ev[j-1] = ev[j-1], ev[j]
		}
	}
	return ev
}

// pipefailOnAt returns whether pipefail is on at the offset, as set by the events before it.
func pipefailOnAt(ev []pipefailEvent, offset int) bool {
	on := false
	for _, e := range ev {
		if e.offset >= offset {
			break
		}
		on = e.on
	}
	return on
}

// scriptSets returns whether the script turns the option on, given by its long name or its letter.
func scriptSets(s *runscript.Script, long string, letter byte) bool {
	for _, c := range s.Commands {
		if c.Name == "set" {
			if on, ok := setOption(c.Args, long, letter); ok && on {
				return true
			}
		}
	}
	return false
}

// setOption interprets the arguments of `set`. It returns whether the option, given by its long name (`-o name`)
// and optionally by a letter (`-e`), is turned on or off by them. ok is false when they do not mention it.
func setOption(args []*runscript.Word, long string, letter byte) (on, ok bool) {
	for i := 0; i < len(args); i++ {
		a := args[i].Value
		if a == "--" {
			break
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			continue
		}
		enable := a[0] == '-'
		flags := a[1:]
		if letter != 0 && strings.IndexByte(flags, letter) >= 0 {
			on, ok = enable, true
		}
		if strings.IndexByte(flags, 'o') >= 0 && i+1 < len(args) {
			i++
			if args[i].Value == long {
				on, ok = enable, true
			}
		}
	}
	return on, ok
}

// pipefailNoFail are commands which cannot meaningfully fail, so a pipeline starting with them has nothing to hide.
// yes only ever fails with SIGPIPE.
var pipefailNoFail = map[string]bool{
	"echo": true, "printf": true, "true": true, "false": true, ":": true, "yes": true, "pwd": true, "date": true,
	"uname": true, "whoami": true, "hostname": true, "seq": true, "id": true, "printenv": true, "env": true, "[": true, "test": true,
}

// pipefailAnswers are commands which use a non-zero status as an answer ("no match", "files differ"), not as a
// failure. They are never blamed as the producer of a pipeline: `grep -c x file | cut` would fail for a file without
// a match once pipefail is on.
var pipefailAnswers = map[string]bool{
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "diff": true, "cmp": true,
}

// pipefailFilters are the commands which only transform their input. In the middle of a pipeline they are not
// reported: they signal "no match" with a non-zero status, and a failure of the producer is reported at the producer.
var pipefailFilters = map[string]bool{
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "sed": true, "awk": true, "gawk": true, "cut": true,
	"tr": true, "sort": true, "uniq": true, "wc": true, "head": true, "tail": true, "tee": true, "cat": true,
	"column": true, "fold": true, "rev": true, "nl": true, "paste": true,
}

// anyPipelineQuitsEarly reports whether a pipeline of the script has a stage after the first which can exit before its
// input ends.
func anyPipelineQuitsEarly(s *runscript.Script) bool {
	for _, p := range s.Pipelines {
		for _, st := range p.Stages[min(1, len(p.Stages)):] {
			for _, c := range st.Commands {
				if stopsReadingEarly(c) {
					return true
				}
			}
		}
	}
	return false
}

// hiddenFailure returns the command whose failure the pipeline hides, nil if there is none to report.
func hiddenFailure(p *runscript.Pipeline) *runscript.Command {
	var found *runscript.Command
	foundStage := 0 // c.Stage is the stage in the innermost pipeline of the command, not in p
	for i, st := range p.Stages[:len(p.Stages)-1] {
		if found != nil {
			break
		}
		for _, c := range st.Commands {
			// Only assignments, or the script handles the failure itself (`cmd || true`). The left side of `&&` in a
			// group is not handled: the group fails with it and the next stage hides that.
			if (c.Name == "" && c.NameWord == nil) || (c.Tested && !c.AndOnly) {
				continue
			}
			if failureMatters(c, i) {
				found, foundStage = c, i
				break
			}
		}
	}
	if found == nil {
		return nil
	}
	// A later stage which quits early makes everything in front of it die with SIGPIPE
	for _, st := range p.Stages[foundStage+1:] {
		for _, c := range st.Commands {
			// A command in a loop or in a pipeline of its own reads something else, or only a part of the input
			// (a stage of this pipeline is not one of them, even when the pipeline is in the body of a loop:
			// `for s in a b; do cmd "$s" | grep -q x; done` stops at the first match of every round)
			if (c.LoopBody && c.Pipeline != p) || (c.Pipeline != nil && c.Pipeline != p) {
				continue
			}
			if stopsReadingEarly(c) {
				return nil
			}
		}
	}
	return found
}

// readsFile reports whether the command has its input redirected from a file (`cat < file`), which can fail.
func readsFile(c *runscript.Command) bool {
	for _, r := range c.Redirects {
		if r.Op == "<" || r.Op == "<>" {
			return true
		}
	}
	return false
}

// failureMatters returns whether a failure of the command in the stage of the pipeline is worth reporting.
func failureMatters(c *runscript.Command, stage int) bool {
	switch {
	case pipefailNoFail[c.Name], pipefailAnswers[c.Name]:
		return false
	case c.Name == "cat" && len(c.Positional) == 0 && !readsFile(c):
		return false // copies a here document, a here string or the output of the previous stage
	case stage > 0 && pipefailFilters[c.Name]:
		return false
	case c.Name == "read" || c.Name == "cd":
		return false
	}
	return true
}

var sedQuitRe = regexp.MustCompile(`(^|[;{\s/])\d*q\d*\s*($|[;}])`)
var awkExitRe = regexp.MustCompile(`\bexit\b`)

// stopsReadingEarly returns whether the command can exit before its input ends.
func stopsReadingEarly(c *runscript.Command) bool {
	switch c.Name {
	case "head":
		return true
	case "read":
		// `cmd | read x`, `cmd | { read x; ...; }` and `if read x` take one line and leave. The condition of a
		// `while` loop reads to the end.
		return !c.LoopCond
	case "grep", "egrep", "fgrep", "rg":
		for _, a := range c.Args {
			v := a.Value
			if strings.HasPrefix(v, "--") {
				if v == "--quiet" || v == "--silent" || strings.HasPrefix(v, "--max-count") {
					return true
				}
				continue
			}
			if len(v) > 1 && v[0] == '-' && strings.ContainsAny(v[1:], "qm") {
				return true
			}
		}
	case "sed":
		for _, a := range c.Positional {
			if sedQuitRe.MatchString(a.Value) {
				return true
			}
		}
	case "awk", "gawk":
		for _, a := range c.Positional {
			if awkExitRe.MatchString(a.Value) {
				return true
			}
		}
	}
	return false
}

// buildFix makes the fix inserting `set -o pipefail` as the first line of the script. It returns nil when the
// script is not a literal block of which the lines can be found in the source.
func (rule *RulePipelineWithoutPipefail) buildFix(run *String) *Fix {
	if rule.src == nil || !run.Literal || run.Indent <= 0 || run.Pos == nil {
		return nil
	}
	lines := strings.Split(run.Value, "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 {
		return nil
	}
	// The line of "run.Pos" is the one of the "|" header, the content starts at the next line.
	lineNo := run.Pos.Line + 1 + first
	if rule.starts == nil {
		rule.starts = buildLineStarts(rule.src) // once for all the scripts of the file
	}
	start := -1
	if lineNo >= 1 && lineNo <= len(rule.starts) && rule.starts[lineNo-1] < len(rule.src) {
		start = rule.starts[lineNo-1]
	}
	if start < 0 {
		return nil
	}
	end := bytes.IndexByte(rule.src[start:], '\n')
	var line []byte
	nl := "\n"
	if end < 0 {
		line = rule.src[start:]
	} else {
		line = rule.src[start : start+end]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
			nl = "\r\n"
		}
	}
	indent := strings.Repeat(" ", run.Indent)
	if string(line) != indent+strings.TrimSuffix(lines[first], "\r") {
		return nil // the source is not what the parser saw (tabs, comments after the header, ...)
	}
	return &Fix{
		Description: `Add "set -o pipefail" as the first line of the script`,
		Unsafe:      true,
		Edits:       []TextEdit{{Start: start, End: start, NewText: indent + "set -o pipefail" + nl}},
	}
}

// runnerIsNotWindows returns whether the job is known to run on Linux or macOS: no label can be Windows and at
// least one label says Linux or macOS. Anything it cannot see through (an expression which is not a plain matrix
// reference, a runner group, a custom label) counts as unknown.
func (rule *RulePipelineWithoutPipefail) runnerIsNotWindows(r *Runner) bool {
	if r == nil {
		return false
	}
	var labels []string
	exprs := []*String{}
	for _, l := range r.Labels {
		if l.ContainsExpression() {
			exprs = append(exprs, l)
		} else {
			labels = append(labels, l.Value)
		}
	}
	if r.LabelsExpr != nil {
		exprs = append(exprs, r.LabelsExpr)
	}
	positive := false
	for _, l := range labels {
		switch classifyRunnerLabel(l) {
		case runnerLabelWindows:
			return false
		case runnerLabelUnix:
			positive = true
		}
	}
	for _, e := range exprs {
		vals, ok := rule.matrixValues(e.Value)
		if !ok {
			continue // unknown: a Linux label next to it still decides, as labels are combined with AND
		}
		all := len(vals) > 0
		for _, v := range vals {
			switch classifyRunnerLabel(v) {
			case runnerLabelWindows:
				return false
			case runnerLabelUnix:
			default:
				all = false
			}
		}
		if all {
			positive = true
		}
	}
	return positive
}

type runnerLabelKind int

const (
	runnerLabelUnknown runnerLabelKind = iota
	runnerLabelWindows
	runnerLabelUnix
)

var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

// classifyRunnerLabel guesses the OS of a runner label, GitHub-hosted (`ubuntu-latest`) or custom
// (`blacksmith-4vcpu-ubuntu-2404`, `jdx-win-ci`).
func classifyRunnerLabel(l string) runnerLabelKind {
	toks := nonAlnumRe.Split(strings.ToLower(l), -1)
	unix := false
	for _, t := range toks {
		switch {
		case strings.HasPrefix(t, "win"):
			return runnerLabelWindows
		case strings.HasPrefix(t, "ubuntu"), t == "linux", strings.HasPrefix(t, "macos"), t == "osx", t == "darwin",
			t == "mac", t == "xcode", t == "debian", t == "alpine", t == "fedora":
			unix = true
		}
	}
	if unix {
		return runnerLabelUnix
	}
	return runnerLabelUnknown
}

var matrixRefRe = regexp.MustCompile(`^\$\{\{\s*matrix\.([A-Za-z_][A-Za-z0-9_-]*)\s*\}\}$`)

// matrixValues returns all values the label `${{ matrix.key }}` can take, false if they are not all known.
func (rule *RulePipelineWithoutPipefail) matrixValues(label string) ([]string, bool) {
	m := matrixRefRe.FindStringSubmatch(strings.TrimSpace(label))
	if m == nil || rule.matrix == nil || rule.matrix.Expression != nil {
		return nil, false
	}
	key := strings.ToLower(m[1])
	var out []string
	add := func(v RawYAMLValue) bool {
		s, ok := v.(*RawYAMLString)
		if !ok {
			return false
		}
		out = append(out, s.Value)
		return true
	}
	if row, ok := rule.matrix.Rows[key]; ok {
		if row.Expression != nil {
			return nil, false
		}
		for _, v := range row.Values {
			if !add(v) {
				return nil, false
			}
		}
	}
	if inc := rule.matrix.Include; inc != nil {
		if inc.Expression != nil {
			return nil, false
		}
		for _, c := range inc.Combinations {
			if c.Expression != nil {
				return nil, false
			}
			for k, a := range c.Assigns {
				if strings.ToLower(k) == key && !add(a.Value) {
					return nil, false
				}
			}
		}
	}
	for _, v := range out {
		if ContainsExpression(v) {
			return nil, false
		}
	}
	return out, len(out) > 0
}

func init() {
	registerRules(
		RuleInfo{ID: "pipeline-without-pipefail", Group: RuleGroupCorrectness, Summary: "A failing command in a pipeline of a run: script is hidden because the shell does not enable pipefail.", DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-pipeline-without-pipefail"},
	)
	registerRuleFactory("pipeline-without-pipefail", func(env *RuleEnv) []Rule {
		return []Rule{NewRulePipelineWithoutPipefail(env.src)}
	})
}
