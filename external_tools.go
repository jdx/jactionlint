package jactionlint

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// This file keeps track of the external linters (shellcheck and pyflakes): whether they were found, and how many
// scripts they checked or could not check. Without it a missing shellcheck is silent: the run succeeds and the
// scripts are not checked at all (actionlint#189).

const (
	// ExternalToolRan is the status of an external linter that was found and used for the scripts of the run.
	ExternalToolRan = "ran"
	// ExternalToolNotFound is the status of an external linter that was not found, so its scripts were not checked.
	ExternalToolNotFound = "not found"
	// ExternalToolDisabled is the status of an external linter that was turned off with an empty command. It is not
	// part of ExternalTools.
	ExternalToolDisabled = "disabled"
)

// ExternalToolStatus says what happened to one external linter during a run. It is in the run properties of
// the SARIF log, next to the coverage of the online checks, so a consumer can tell "checked and nothing found"
// from "not checked".
type ExternalToolStatus struct {
	// Status is "ran", "not found" or "disabled".
	Status string `json:"status"`
	// Command is the command the run was configured with (the default is the name of the tool).
	Command string `json:"command,omitempty"`
	// Scripts is the number of scripts the tool checked.
	Scripts int `json:"scripts"`
	// SkippedScripts is the number of scripts that the tool would have checked if it had run. It is omitted when
	// it is zero.
	SkippedScripts int `json:"skippedScripts,omitempty"`
}

// externalTool is the state of one external linter in a run. The rules of every file update it concurrently. All
// methods are safe to call on a nil pointer, which is what the rules of a RuleEnv without tracking have.
type externalTool struct {
	name    string // "shellcheck" or "pyflakes"
	command string
	// required is true when the command was asked for explicitly. Then NewLinter has already failed when it
	// cannot be found, and a missing command is not noted again.
	required bool
	// kind is what the tool checks, for the note: "shell script" or "Python script".
	kind string
	// what names the tool in the note when the bare name is not enough ("node 20 or later"), and off says how to
	// skip it on purpose when there is no command line option for it. Both are optional.
	what, off string

	mu      sync.Mutex
	status  string // "" until a rule factory looked for the command
	scripts atomic.Int64
	skipped atomic.Int64
}

func (t *externalTool) setStatus(s string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.status = s
	t.mu.Unlock()
}

func (t *externalTool) found()    { t.setStatus(ExternalToolRan) }
func (t *externalTool) missing()  { t.setStatus(ExternalToolNotFound) }
func (t *externalTool) disabled() { t.setStatus(ExternalToolDisabled) }

// checked counts a script which the tool is run for.
func (t *externalTool) checked() {
	if t != nil {
		t.scripts.Add(1)
	}
}

// skip counts a script which the tool would have checked, if it were available.
func (t *externalTool) skip() {
	if t != nil {
		t.skipped.Add(1)
	}
}

// snapshot returns the status, or false when no rule has looked for the tool in this run.
func (t *externalTool) snapshot() (ExternalToolStatus, bool) {
	t.mu.Lock()
	s := t.status
	t.mu.Unlock()
	if s == "" {
		return ExternalToolStatus{}, false
	}
	return ExternalToolStatus{Status: s, Command: t.command, Scripts: int(t.scripts.Load()), SkippedScripts: int(t.skipped.Load())}, true
}

// note returns the line for the log when the tool was not found and so some scripts were not checked, or "". A
// tool that was disabled or asked for explicitly says nothing: the user decided, or the run has failed.
func (t *externalTool) note() string {
	s, ok := t.snapshot()
	if !ok || s.Status != ExternalToolNotFound || s.SkippedScripts == 0 || t.required {
		return ""
	}
	what, off := t.name, "pass --"+t.name+"= to skip it on purpose"
	if t.what != "" {
		what = t.what
	}
	if t.off != "" {
		off = t.off
	}
	return fmt.Sprintf("%s was not found, so it did not check %s. install it, or %s", what, countNoun(s.SkippedScripts, t.kind), off)
}

// externalTools are the external linters of a run.
type externalTools struct {
	shellcheck *externalTool
	pyflakes   *externalTool
	node       *externalTool // runs the check of github-script-syntax; it has no command line option

	// seen are the files which the totals already have. --fix and --diff lint a file again for each pass, and the
	// scripts of a file count once per run, as the first check of it saw them.
	mu   sync.Mutex
	seen map[string]bool
}

// forCheck returns the tracking for the check of one file: the same tools with counters of their own, which record
// puts into the totals when the check is over.
func (ts *externalTools) forCheck() *externalTools {
	if ts == nil {
		return nil
	}
	cp := func(t *externalTool) *externalTool {
		return &externalTool{name: t.name, command: t.command, required: t.required, kind: t.kind, what: t.what, off: t.off}
	}
	return &externalTools{shellcheck: cp(ts.shellcheck), pyflakes: cp(ts.pyflakes), node: cp(ts.node)}
}

// record adds what the check of the file saw to the totals, unless an earlier check of the same file did.
func (ts *externalTools) record(path string, check *externalTools) {
	if ts == nil || check == nil {
		return
	}
	ts.mu.Lock()
	if ts.seen[path] {
		ts.mu.Unlock()
		return
	}
	if ts.seen == nil {
		ts.seen = map[string]bool{}
	}
	ts.seen[path] = true
	ts.mu.Unlock()
	for _, p := range [][2]*externalTool{{ts.shellcheck, check.shellcheck}, {ts.pyflakes, check.pyflakes}, {ts.node, check.node}} {
		total, one := p[0], p[1]
		if s, ok := one.snapshot(); ok {
			total.setStatus(s.Status)
			total.scripts.Add(int64(s.Scripts))
			total.skipped.Add(int64(s.SkippedScripts))
		}
	}
}

func newExternalTools(opts *LinterOptions) *externalTools {
	return &externalTools{
		shellcheck: &externalTool{name: "shellcheck", command: opts.Shellcheck, required: opts.ShellcheckRequired, kind: "shell script"},
		pyflakes:   &externalTool{name: "pyflakes", command: opts.Pyflakes, required: opts.PyflakesRequired, kind: "Python script"},
		node: &externalTool{name: "node", command: "node", kind: "JavaScript script", what: "node 20 or later",
			off: `turn the check off with "rules: {github-script-syntax: off}" in the configuration`},
	}
}

// tool returns the tracking of the external linter, or nil.
func (ts *externalTools) tool(name string) *externalTool {
	if ts == nil {
		return nil
	}
	switch name {
	case "shellcheck":
		return ts.shellcheck
	case "pyflakes":
		return ts.pyflakes
	case "node":
		return ts.node
	}
	return nil
}

// statuses returns the status of the tools which some script needed, or nil when none did: a tool with nothing to
// check (a workflow of "uses:" steps needs no pyflakes) is left out, like the note leaves it out, since the log would
// say "not found" about a tool the run never wanted. A tool that was turned off on purpose with an empty command is
// left out too: the user chose that, and it would add a property to every log of a run that never meant to use it.
func (ts *externalTools) statuses() map[string]ExternalToolStatus {
	if ts == nil {
		return nil
	}
	var ret map[string]ExternalToolStatus
	for _, t := range []*externalTool{ts.shellcheck, ts.pyflakes, ts.node} {
		if s, ok := t.snapshot(); ok && s.Status != ExternalToolDisabled && s.Scripts+s.SkippedScripts > 0 {
			if ret == nil {
				ret = map[string]ExternalToolStatus{}
			}
			ret[t.name] = s
		}
	}
	return ret
}

// notes returns the notes about the tools that were not found.
func (ts *externalTools) notes() []string {
	if ts == nil {
		return nil
	}
	var ret []string
	for _, t := range []*externalTool{ts.shellcheck, ts.pyflakes, ts.node} {
		if n := t.note(); n != "" {
			ret = append(ret, n)
		}
	}
	return ret
}

// checkRequired fails when a command that was asked for explicitly cannot be run, so that a typo in the path of
// --shellcheck does not turn the integration off without a word.
func (ts *externalTools) checkRequired() error {
	for _, t := range []*externalTool{ts.shellcheck, ts.pyflakes} {
		if !t.required || t.command == "" {
			continue
		}
		if _, _, err := resolveExternalCommand(t.command); err != nil {
			return fmt.Errorf("--%s=%s: the command was not found or is not executable (%w). Fix the path, or pass --%s= to turn the %s integration off", t.name, t.command, err, t.name, t.name)
		}
	}
	return nil
}
