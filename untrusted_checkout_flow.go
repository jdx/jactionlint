package jactionlint

import (
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// This file follows a revision of a pull request through shell variables and script files for untrusted-checkout.
//
// What is attacker-controlled: the head of a pull request or of the triggering run (untrustedValue), `refs/pull/...`,
// and anything that a variable holds after one of those was assigned to it. The variables are the ones of the step's
// own script, the environment of the step, the job and the workflow, the variables an earlier step wrote to
// $GITHUB_ENV (`NAME=<untrusted>`), and, inside a script file that a step starts, the positional parameters that the
// step passes an untrusted word for and the variables it exports or puts in front of the call.
//
// What "reaches the working tree" means: a command that puts a revision in it (gitCheckoutVerbs, `gh pr checkout`)
// and names a tainted word, or a `git fetch` of a tainted word followed by a command that changes the working tree.
// A script file that does that counts at the command that starts it.
//
// What "runs code from it" means is unchanged (stepRunsCode): a later command that can run workspace code. When the
// script file itself runs code after the checkout, the step is reported at once.
//
// What stays unknown, and is not reported: values that come out of a command (`REF=$(cmd)` is followed only into the
// words of the substitution), outputs of earlier steps, `read`, `eval`, arrays, a script whose path is computed or that
// is missing, too large or outside the project, and scripts nested more than maxScriptDepth deep. A later
// unconditional assignment of a clean value ends the taint of a variable; a conditional one does not.

// maxScriptDepth is how many levels of script files that start script files are followed.
const maxScriptDepth = 3

// revTaint tells which shell variables of a script hold a revision of a pull request.
type revTaint struct {
	s *runscript.Script
	// base is what the variables hold when the script starts, with a description of where it comes from.
	base map[string]string
}

func newRevTaint(s *runscript.Script, base map[string]string) *revTaint {
	return &revTaint{s: s, base: base}
}

// names returns the variables that hold a revision of a pull request at the offset.
func (t *revTaint) names(off int) map[string]bool {
	out := make(map[string]bool, len(t.base))
	for n := range t.base {
		out[n] = true
	}
	if len(t.s.Assignments) == 0 && len(t.s.ForVars) == 0 {
		return out
	}
	cands := map[string]bool{}
	for _, a := range t.s.Assignments {
		if a.Offset < off {
			cands[a.Name] = true
		}
	}
	for _, fv := range t.s.ForVars {
		if fv.Offset < off {
			cands[fv.Name] = true
		}
	}
	for n := range cands {
		// the value at this position replaces what the variable held when the script started
		if t.varAt(n, off, 0) != "" {
			out[n] = true
		} else {
			delete(out, n)
		}
	}
	return out
}

// varAt returns what the variable holds at the offset when it is untrusted ("" when it is not or when that is not
// known). The assignments before the offset are walked backwards: an unconditional assignment decides the value, a
// conditional one may or may not have happened, so the walk goes on past it.
func (t *revTaint) varAt(name string, off, depth int) string {
	if depth > 4 {
		return ""
	}
	type event struct {
		off int
		a   *runscript.Assignment
		fv  *runscript.ForVar
	}
	var evs []event
	for _, a := range t.s.Assignments {
		if a.Name == name && a.Offset < off && (a.Cmd == nil || a.Cmd.Decl) {
			evs = append(evs, event{off: a.Offset, a: a})
		}
	}
	for _, fv := range t.s.ForVars {
		if fv.Name == name && fv.Offset < off {
			evs = append(evs, event{off: fv.Offset, fv: fv})
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].off < evs[j].off })
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		switch {
		case e.fv != nil:
			for _, w := range e.fv.Items {
				if d := t.wordAt(w, e.off, depth+1); d != "" {
					return d
				}
			}
		case e.a.Array:
			// the elements are not followed
		default:
			if e.a.Value != nil {
				if d := t.wordAt(e.a.Value, e.a.Offset, depth+1); d != "" {
					return d
				}
			}
			if !e.a.Cond && !e.a.Append && (e.a.Cmd == nil || e.a.Value != nil) {
				return "" // set to a value that is not untrusted
			}
		}
	}
	return t.base[name]
}

// wordAt returns what makes the word untrusted at the offset, "" if nothing does.
func (t *revTaint) wordAt(w *runscript.Word, off, depth int) string {
	if w == nil {
		return ""
	}
	for _, e := range w.Exprs {
		if what, bad := untrustedValue("${{ " + e + " }}"); bad {
			return what
		}
	}
	if strings.Contains(w.Value, "refs/pull/") {
		return w.Value
	}
	for _, v := range w.Vars {
		if d := t.varAt(v, off, depth); d != "" {
			return d
		}
	}
	for _, c := range w.Subs {
		for _, sw := range c.Words {
			if d := t.wordAt(sw, off, depth+1); d != "" {
				return d
			}
		}
	}
	return ""
}

// readScript reads the script file that a command of the step starts, nil when it is not one the rule may follow:
// it is missing, too large, not a regular file, outside the project or does not parse.
func (rule *RuleUntrustedCheckout) readScript(s *Step, rel string) *runscript.Script {
	if rule.project == nil || rule.project.root == "" || rel == "" {
		return nil
	}
	path := filepath.Join(rule.project.root, filepath.FromSlash(stepWorkdir(s)), filepath.FromSlash(rel))
	if sc, ok := rule.scripts[path]; ok {
		return sc
	}
	sc := rule.loadScript(path)
	if rule.scripts == nil {
		rule.scripts = map[string]*runscript.Script{}
	}
	rule.scripts[path] = sc // a missing or unusable file is remembered as well
	return sc
}

func (rule *RuleUntrustedCheckout) loadScript(path string) *runscript.Script {
	real, err := filepath.EvalSymlinks(path)
	root, rerr := filepath.EvalSymlinks(rule.project.root)
	if err != nil || rerr != nil || !isPathInDir(root, real) {
		return nil
	}
	if fi, err := os.Stat(real); err != nil || !fi.Mode().IsRegular() || fi.Size() > maxScriptSize {
		return nil
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return nil
	}
	script, err := runscript.Analyze(string(b), "")
	if err != nil {
		return nil
	}
	return script
}

// childBase returns what the variables of the script that the command starts hold: the environment of the process
// (the step's own, the variables the script exported and the ones in front of the call) and the positional
// parameters that get an untrusted word.
func childBase(t *revTaint, c *runscript.Command, script *runscript.Word, process map[string]string) map[string]string {
	base := maps.Clone(process)
	if base == nil {
		base = map[string]string{}
	}
	// A variable the process inherited stays exported when the script assigns it, and `export` exports a new one, so
	// the value at the call replaces the inherited one: a clean assignment ends the taint.
	exported := map[string]bool{}
	for n := range base {
		exported[n] = true
	}
	for _, a := range t.s.Assignments {
		if a.Cmd != nil && a.Cmd.Decl && a.Cmd.Name == "export" && a.Offset < c.Offset {
			exported[a.Name] = true
		}
	}
	for n := range exported {
		if d := t.varAt(n, c.Offset, 0); d != "" {
			base[n] = d
		} else {
			delete(base, n)
		}
	}
	for _, a := range c.Assigns { // NAME=value ./script.sh
		if d := t.wordAt(a.Value, c.Offset, 0); d != "" {
			base[a.Name] = d
		} else {
			delete(base, a.Name)
		}
	}
	n := 0
	for _, w := range c.Words {
		if w.Offset <= script.Offset {
			continue
		}
		n++
		if d := t.wordAt(w, c.Offset, 0); d != "" {
			base[strconv.Itoa(n)], base["@"], base["*"] = d, d, d
		}
	}
	return base
}

// scriptWord returns the word of the command that names the script file it starts.
func scriptWord(c *runscript.Command) *runscript.Word {
	switch c.Name {
	case "bash", "sh":
		if len(c.Positional) > 0 {
			return c.Positional[0]
		}
		return nil
	}
	return c.NameWord
}

// scriptChecksOut reports whether running the script puts untrusted code in the working tree: it describes the
// revision, and says what the script runs afterwards ("" when it runs nothing, so a later step has to).
func (rule *RuleUntrustedCheckout) scriptChecksOut(sc *runscript.Script, base map[string]string, s *Step, depth int) (what, how string, ok bool) {
	t := newRevTaint(sc, base)
	for i, c := range sc.Commands {
		var env map[string]bool
		if c.Name == "git" || c.Name == "gh" {
			env = t.names(c.Offset)
		}
		what, ok = commandChecksOutUntrusted(c, env)
		if !ok && c.Name == "git" && c.Verb() == "fetch" && fetchedThenUsed(sc.Commands[i+1:]) {
			what, ok = commandNamesUntrusted(c, env)
		}
		if ok {
			what = t.describe(what, c.Offset)
		}
		if !ok && depth > 0 {
			what, how, ok = rule.followScript(t, c, base, s, depth-1)
		}
		if !ok {
			continue
		}
		if how == "" {
			for _, d := range sc.Commands[i+1:] {
				if commandRunsCode(d) {
					how = quoteName(d)
					break
				}
			}
		}
		return what, how, true
	}
	return "", "", false
}

// describe turns the "$NAME" that commandNamesUntrusted gives for a variable into where its value comes from.
func (t *revTaint) describe(what string, off int) string {
	if n, ok := strings.CutPrefix(what, "$"); ok {
		if d := t.varAt(n, off, 0); d != "" {
			return d
		}
	}
	return what
}

func quoteName(c *runscript.Command) string {
	name := c.Name
	if name == "" {
		name = "a command"
	}
	return "\"" + name + "\""
}

// followScript follows a command that starts a script file of the project into the file.
func (rule *RuleUntrustedCheckout) followScript(t *revTaint, c *runscript.Command, process map[string]string, s *Step, depth int) (what, how string, ok bool) {
	rel, sw := scriptPath(c), scriptWord(c)
	if rel == "" || sw == nil {
		return "", "", false
	}
	child := rule.readScript(s, rel)
	if child == nil {
		return "", "", false
	}
	return rule.scriptChecksOut(child, childBase(t, c, sw, process), s, depth)
}

// carryEnv remembers the variables that the script writes to $GITHUB_ENV with an untrusted value: the later steps
// of the job see them in their environment.
func (rule *RuleUntrustedCheckout) carryEnv(sc *runscript.Script, t *revTaint) {
	for _, w := range sc.WritesTo("GITHUB_ENV") {
		for _, p := range w.Producers {
			if p.Name != "echo" && p.Name != "printf" {
				continue
			}
			for _, word := range p.Positional {
				name, _, found := strings.Cut(word.Value, "=")
				if !found || name == "" || strings.ContainsAny(name, " $\"'") {
					continue
				}
				if d := t.wordAt(word, p.Offset, 0); d != "" {
					if rule.carried == nil {
						rule.carried = map[string]string{}
					}
					rule.carried[name] = d
				} else if !p.Cond {
					delete(rule.carried, name) // a later write replaces the value
				}
			}
		}
	}
}
