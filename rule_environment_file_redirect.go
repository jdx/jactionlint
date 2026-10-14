package jactionlint

import (
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// RuleEnvironmentFileRedirect detects a `run:` script which writes to the environment files of the runner in a way
// that loses the data. It reports, with the one ID "environment-file-redirect":
//
//   - a redirect or `tee` to the bare word GITHUB_OUTPUT, GITHUB_ENV, GITHUB_PATH, GITHUB_STEP_SUMMARY or GITHUB_STATE
//     (`echo "a=b" >> GITHUB_OUTPUT`). The word is a file name, not the variable, so the shell writes a file of that
//     name in the working directory and the runner never sees it: the output, the variable or the path is silently
//     missing. The fix writes `"$GITHUB_OUTPUT"`.
//   - a `>` (or `tee` without `-a`) to the file of one of them after the same script has already written to it:
//     `>` truncates the file, so what the script wrote before is lost. Each step gets new files, so a lone `>` is
//     harmless and is not reported. The fix (unsafe, because the reset may be meant) writes `>>`.
//
// PowerShell (`$env:GITHUB_OUTPUT`) and cmd (`%GITHUB_OUTPUT%`) scripts are not analyzed.
type RuleEnvironmentFileRedirect struct {
	RuleBase
	runContext
	src *sourceIndex
}

// NewRuleEnvironmentFileRedirect creates a new RuleEnvironmentFileRedirect instance. The source of the file lets the
// rule attach fixes to its findings. It can be nil.
func NewRuleEnvironmentFileRedirect(src []byte) *RuleEnvironmentFileRedirect {
	r := &RuleEnvironmentFileRedirect{
		RuleBase: NewRuleBase("environment-file-redirect", "Checks for writes to GITHUB_OUTPUT and the other environment files that lose the data at \"run:\""),
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleEnvironmentFileRedirect) VisitWorkflowPre(n *Workflow) error {
	rule.enterWorkflow(n)
	return nil
}

// VisitJobPre is callback when visiting Job node before visiting its children.
func (rule *RuleEnvironmentFileRedirect) VisitJobPre(n *Job) error {
	rule.enterJob(n)
	return nil
}

// VisitJobPost is callback when visiting Job node after visiting its children.
func (rule *RuleEnvironmentFileRedirect) VisitJobPost(n *Job) error {
	rule.leaveJob()
	return nil
}

// runnerFiles are the environment files of the runner, which a step finds in the variable of the same name.
var runnerFiles = map[string]bool{
	"GITHUB_OUTPUT": true, "GITHUB_ENV": true, "GITHUB_PATH": true, "GITHUB_STEP_SUMMARY": true, "GITHUB_STATE": true,
}

// VisitStep is callback when visiting Step node.
func (rule *RuleEnvironmentFileRedirect) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil || !strings.Contains(run.Run.Value, "GITHUB_") {
		return nil
	}
	s, origin := rule.script(run)
	if s == nil {
		return nil
	}
	// The variables the script sets itself have another meaning there
	assigned := map[string]bool{}
	for _, a := range s.Assignments {
		if runnerFiles[a.Name] {
			assigned[a.Name] = true
		}
	}
	// the files the script has certainly written to so far
	written := map[string]bool{}
	for _, r := range s.Redirects {
		if !r.Write || r.Target == nil {
			continue
		}
		if r.Target.Literal() && runnerFiles[r.Target.Value] {
			rule.bareWord(s, origin, r)
			continue
		}
		name, ok := r.Target.Var()
		if !ok || !runnerFiles[name] || assigned[name] {
			continue
		}
		if written[name] && truncates(r) {
			rule.truncation(s, origin, r, name)
		}
		// A write which may not happen (in a branch, a loop, a function) or happens in another process does not
		// make the later `>` lose data for sure
		if r.Cmd != nil && !r.Cmd.Cond {
			written[name] = true
		}
	}
	return nil
}

// truncates reports whether the redirect empties the file before it writes.
func truncates(r *runscript.Redirect) bool {
	if r.Tee {
		return !r.Append
	}
	switch r.Op {
	case ">", ">|", "&>":
		return true
	}
	return false
}

// bareWord reports a redirect to the word GITHUB_OUTPUT, which is a file of that name.
func (rule *RuleEnvironmentFileRedirect) bareWord(s *runscript.Script, origin runscript.Origin, r *runscript.Redirect) {
	name := r.Target.Value
	start, end := scriptPos(s, origin, r.Target.Offset), scriptPos(s, origin, r.Target.End)
	e := rule.errorIDAt("environment-file-redirect", start, "writing to "+quote(name)+" creates a file with that name in the working directory, because without a `$` it is not the variable: the runner never sees it, so nothing is "+runnerFileEffect(name)+". write `\"$"+name+"\"`").endAt(end)
	// the fix adds the `$` in front of the name, inside the quotes when the word is double quoted
	w := r.Target
	if w.Raw != name && w.Raw != `"`+name+`"` {
		e.NoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the name is quoted in a way the fix does not edit, add the $ by hand"}
		return
	}
	prefix := 0
	if w.Raw != name {
		prefix = 1
	}
	p := s.Position(origin, w.Offset+prefix)
	if rule.src == nil || !p.Exact {
		e.NoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the position of the name in the file is not exact, add the $ by hand"}
		return
	}
	off, ok := rule.src.offset(p.Line, p.Col)
	if !ok || !rule.src.matches(off, name) {
		e.NoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the position of the name in the file is not exact, add the $ by hand"}
		return
	}
	site, ok := rule.src.sites().at(off)
	var dollar string
	if ok {
		dollar, ok = site.Insert("$")
	}
	if !ok {
		e.NoFix = &NoFix{Code: NoFixUnsupportedShape, Reason: "the scalar cannot hold the text of the fix, add the $ by hand"}
		return
	}
	e.Fix = &Fix{Description: "Write $" + name, Edits: []TextEdit{{Start: off, End: off, NewText: dollar}}}
}

func runnerFileEffect(name string) string {
	switch name {
	case "GITHUB_OUTPUT":
		return "set as an output of the step"
	case "GITHUB_ENV":
		return "set as an environment variable of the next steps"
	case "GITHUB_PATH":
		return "added to the PATH of the next steps"
	case "GITHUB_STEP_SUMMARY":
		return "shown in the summary of the job"
	}
	return "saved as the state of the step"
}

// truncation reports a `>` to a file which the script has written to before.
func (rule *RuleEnvironmentFileRedirect) truncation(s *runscript.Script, origin runscript.Origin, r *runscript.Redirect, name string) {
	start, end := scriptPos(s, origin, r.Offset), scriptPos(s, origin, r.End)
	if r.Tee {
		start, end = scriptPos(s, origin, r.Target.Offset), scriptPos(s, origin, r.Target.End)
	}
	op := "`>`"
	if r.Tee {
		op = "`tee` without `-a`"
	}
	e := rule.errorIDAt("environment-file-redirect", start, op+" empties $"+name+" before it writes, so what this script wrote to it before is lost. append with "+map[bool]string{true: "`tee -a`", false: "`>>`"}[r.Tee]).endAt(end)
	// Only a plain `>` of the script is edited; anything else is appended to by hand
	noFix := &NoFix{Code: NoFixUnsupportedShape, Reason: "the redirect is not a plain `>` that the fix can edit exactly, append with `>>` (or `tee -a`) by hand"}
	if r.Tee || r.Op != ">" || r.Fd != "" || rule.src == nil {
		e.NoFix = noFix
		return
	}
	p := s.Position(origin, r.Offset)
	if !p.Exact {
		e.NoFix = noFix
		return
	}
	off, ok := rule.src.offset(p.Line, p.Col)
	if !ok || !rule.src.matches(off, ">") || rule.src.matches(off, ">>") || rule.src.matches(off, ">|") || rule.src.matches(off, ">&") {
		e.NoFix = noFix
		return
	}
	// the reset may be meant, so the fix is not applied unless it is asked for
	e.Fix = &Fix{Description: "Append with >>", Unsafe: true, Edits: []TextEdit{{Start: off, End: off + 1, NewText: ">>"}}}
}

func init() {
	registerRules(
		RuleInfo{ID: "environment-file-redirect", Group: RuleGroupCorrectness, Summary: "A script writes to GITHUB_OUTPUT or another environment file of the runner without the $, or empties it with > after it wrote to it.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, Fixable: true, DocsAnchor: "check-environment-file-redirect"},
	)
	registerRuleFactory("environment-file-redirect", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleEnvironmentFileRedirect(env.src)}
	})
}
