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
	return fmt.Sprintf("%s was not found, so it did not check %s. install it, or pass --%s= to skip it on purpose", t.name, countNoun(s.SkippedScripts, t.kind), t.name)
}

// externalTools are the external linters of a run.
type externalTools struct {
	shellcheck *externalTool
	pyflakes   *externalTool
}

func newExternalTools(opts *LinterOptions) *externalTools {
	return &externalTools{
		shellcheck: &externalTool{name: "shellcheck", command: opts.Shellcheck, required: opts.ShellcheckRequired, kind: "shell script"},
		pyflakes:   &externalTool{name: "pyflakes", command: opts.Pyflakes, required: opts.PyflakesRequired, kind: "Python script"},
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
	for _, t := range []*externalTool{ts.shellcheck, ts.pyflakes} {
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
	for _, t := range []*externalTool{ts.shellcheck, ts.pyflakes} {
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
