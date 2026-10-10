package jactionlint

import (
	"slices"
	"sort"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// flowKind tells what happens to a variable at an event of the script.
type flowKind int

const (
	flowAssign  flowKind = iota // NAME=value, NAME+=value, NAME=(...), export NAME=value
	flowRead                    // read NAME
	flowForVar                  // for NAME in items
	flowGuard                   // a test after which the script only goes on when NAME has a checked shape
	flowTotal                   // an if with an else or a case with a default that sets NAME on every path
	flowUnknown                 // eval, source, printf -v NAME, mapfile NAME: the value is whatever they make it
)

// flowEvent is one thing that happens to a variable, at an offset of the script.
type flowEvent struct {
	off    int
	kind   flowKind
	assign *runscript.Assignment
	// assigns are the assignments inside a flowTotal.
	assigns []*runscript.Assignment
	cmd     *runscript.Command
	forVar  *runscript.ForVar
	guard   *runscript.Guard
	// maybe is true when the event is not known to happen before the write in every run: it is later in a loop.
	maybe bool
}

// flowEvents lists the events that can change the variable before the offset, in execution order (source order,
// which is execution order for the straight-line script; the branches and loops are marked Cond). When the write
// is in a loop the events after it are listed as well: the next round runs them first.
func flowEvents(s *runscript.Script, name string, at int, loop bool) []flowEvent {
	var evs []flowEvent
	add := func(e flowEvent) {
		if e.off >= at {
			if !loop {
				return
			}
			e.maybe = true
		}
		evs = append(evs, e)
	}
	totals := map[*runscript.Total][]*runscript.Assignment{}
	for _, a := range s.Assignments {
		if a.Name != name {
			continue
		}
		if a.Cmd != nil && !a.Cmd.Decl && !slices.Contains(s.Funcs, a.Cmd.Name) {
			continue // NAME=value cmd: only the environment of cmd, unless cmd is a function of the script
		}
		in := false
		for _, t := range s.Totals {
			// a total is one event at its end; a write inside it sees the assignments before the write one by one
			if t.Name == name && a.Offset >= t.Offset && a.Offset < t.End && t.End <= at {
				totals[t] = append(totals[t], a)
				in = true
			}
		}
		if !in {
			add(flowEvent{off: a.Offset, kind: flowAssign, assign: a})
		}
	}
	for t, as := range totals {
		add(flowEvent{off: t.End, kind: flowTotal, assigns: as})
	}
	for _, fv := range s.ForVars {
		if fv.Name == name {
			add(flowEvent{off: fv.Offset, kind: flowForVar, forVar: fv})
		}
	}
	for _, g := range s.Guards {
		if g.Var == name {
			add(flowEvent{off: g.End, kind: flowGuard, guard: g})
		}
	}
	for _, c := range s.Commands {
		switch {
		case c.Name == "read":
			for _, p := range c.Positional {
				if p.Value == name {
					add(flowEvent{off: c.Offset, kind: flowRead, cmd: c})
				}
			}
		case c.Tool == "eval" || c.Tool == "source" || c.Tool == "." || c.Name == "eval":
			add(flowEvent{off: c.Offset, kind: flowUnknown, cmd: c})
		case c.Name == "printf":
			for i, a := range c.Args {
				if a.Value == "-v" && i+1 < len(c.Args) && (c.Args[i+1].Value == name || c.Args[i+1].Dynamic()) {
					add(flowEvent{off: c.Offset, kind: flowUnknown, cmd: c})
				}
			}
		case c.Name == "mapfile" || c.Name == "readarray":
			for _, p := range c.Positional {
				if p.Value == name {
					add(flowEvent{off: c.Offset, kind: flowUnknown, cmd: c})
				}
			}
		}
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].off < evs[j].off })
	return evs
}

// guardHolds reports whether the guard proves the value harmless for the destination.
func (rule *RuleGitHubEnv) guardHolds(s *runscript.Script, g *runscript.Guard, dest string) bool {
	if g.Bare && !rule.errexit && !setsErrexitBefore(s, g.Offset) {
		return false // a failing test does not stop a shell that runs without -e
	}
	if g.Regex != "" {
		return validatingRegex(g.Regex, dest)
	}
	if len(g.Literals) == 0 {
		return false
	}
	for _, l := range g.Literals {
		if !charsetSafe(l, dest == "GITHUB_PATH") {
			return false
		}
	}
	return true
}

// judgeVar judges the value a shell variable has at rule.at. The events that can set it are walked backwards, from
// the last one before the point:
//
//   - an unconditional assignment or read decides the value, whatever came before; the walk stops there;
//   - a conditional one (in a branch, a loop, a function, a pipeline) may or may not have happened: its value
//     counts, and the walk goes on to what was there before it;
//   - `+=` adds to the value there was: it counts and the walk goes on;
//   - a guard that holds, with no assignment after it, decides the value: it is harmless;
//   - anything the analysis cannot see (eval, source, printf -v) makes the value unknown.
//
// When the walk does not stop, the variable may still hold what the step's environment gave it, which is
// judged last. An assignment in a loop that comes after the write counts when the write is in the loop.
func (rule *RuleGitHubEnv) judgeVar(s *runscript.Script, name string, depth int) data {
	if depth > 3 {
		return data{kind: dataUnknown}
	}
	at := rule.at
	d := data{}
	decided := false
	evs := flowEvents(s, name, at, rule.loop)
	for i := len(evs) - 1; i >= 0 && !decided; i-- {
		e := evs[i]
		switch e.kind {
		case flowGuard:
			if !e.maybe && (e.guard.ScopeEnd == 0 || at < e.guard.ScopeEnd) && rule.guardHolds(s, e.guard, rule.dest) {
				decided = true
			}
		case flowAssign:
			a := e.assign
			switch {
			case a.Array:
				d = d.worse(data{kind: dataUnknown})
				decided = !a.Cond && !e.maybe
			case a.Value == nil:
				decided = a.Cmd == nil && !a.Cond && !e.maybe // `A=` empties it, `declare A` leaves it
			default:
				d = d.worse(rule.judgeWordAt(s, a.Value, a.Offset, depth+1))
				decided = !a.Append && !a.Cond && !e.maybe
			}
		case flowTotal:
			for _, a := range e.assigns {
				if a.Value != nil {
					d = d.worse(rule.judgeWordAt(s, a.Value, a.Offset, depth+1))
				}
			}
			decided = !e.maybe
		case flowRead:
			d = d.worse(rule.judgeRead(s, e.cmd, depth+1))
			decided = !e.cmd.Cond && !e.maybe
		case flowForVar:
			for _, w := range e.forVar.Items {
				d = d.worse(rule.judgeWordAt(s, w, e.off, depth+1))
			}
			for _, w := range e.forVar.Items {
				if w.Glob {
					d = d.worse(data{kind: dataUnknown})
				}
			}
			if len(e.forVar.Items) == 0 {
				d = d.worse(data{kind: dataUnknown})
			}
			decided = at < e.forVar.End // in the loop the variable is one of the items
		case flowUnknown:
			d = d.worse(data{kind: dataUnknown})
		}
		if d.kind == dataUntrusted {
			return d
		}
	}
	if decided {
		return d
	}
	return d.worse(rule.judgeInitial(name, depth))
}

// judgeWordAt judges a word as it is at an offset of the script.
func (rule *RuleGitHubEnv) judgeWordAt(s *runscript.Script, w *runscript.Word, at, depth int) data {
	saved := rule.at
	rule.at = at
	defer func() { rule.at = saved }()
	return rule.judgeWord(s, w, depth)
}

// judgeRead judges what `read` puts in its variables: the here string, or an unknown input.
func (rule *RuleGitHubEnv) judgeRead(s *runscript.Script, c *runscript.Command, depth int) data {
	for _, r := range c.Redirects {
		if r.Op == "<<<" && r.Target != nil {
			return rule.judgeWordAt(s, r.Target, c.Offset, depth)
		}
	}
	return data{kind: dataUnknown}
}

// judgeInitial judges what a variable holds when the script has not set it: the environment of the step, of the
// job, of the workflow, or the runner.
func (rule *RuleGitHubEnv) judgeInitial(name string, depth int) data {
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

// setsErrexitBefore reports whether the script turns errexit on (`set -e`, `set -o errexit`) before the offset.
func setsErrexitBefore(s *runscript.Script, off int) bool {
	on := false
	for _, c := range s.Commands {
		if c.Name != "set" || c.Offset >= off || c.Cond {
			continue
		}
		if v, ok := setOption(c.Args, "errexit", 'e'); ok {
			on = v
		}
	}
	return on
}
