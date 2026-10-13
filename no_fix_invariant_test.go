package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// adviceOnlyKinds are the findings of rules that can fix others which the rule never fixes. They have neither a Fix
// nor a NoFix on purpose: "advice only" is told from "the fix is unavailable" by the absence of a reason. Every other
// finding of a rule with Fixable set has one or the other.
var adviceOnlyKinds = map[string]func(e *Error, w *Workflow) bool{
	// Only the "uses:" paths are fixed; a constant or computed expression is rewritten by a person
	"obfuscation": func(e *Error, _ *Workflow) bool { return !strings.HasPrefix(e.Message, "path of ") },
	// Only the pedantic finding of a shell variable in the input of an action: the value may be meant as it is
	"malformed-expression-delimiters": func(e *Error, _ *Workflow) bool { return strings.Contains(e.Message, "is not expanded here") },
	// Only the expansions in run: scripts of bash and sh are fixed (a script of github-script, a prompt, a docker option...)
	"template-injection": func(e *Error, w *Workflow) bool { return !inRunScript(e, w) },
	// Only cargo install gets --locked; the other installers are advice
	"unlocked-install": func(e *Error, _ *Workflow) bool { return !strings.HasPrefix(e.Message, `"cargo install"`) },
	// Only the local-actions variant has a fix; the missing or disabled update is for a person to add
	"dependabot-missing-actions-update": func(e *Error, _ *Workflow) bool { return !strings.Contains(e.Message, "local composite actions") },
	// A workflow which is not run for pull requests only: whether a push, tag or manual run may cancel the older ones is
	// left to the person
	"concurrency-limits": func(e *Error, w *Workflow) bool {
		if strings.HasPrefix(e.Message, `"concurrency:" is only a group name`) {
			return true
		}
		for _, ev := range w.On {
			if !slices.Contains(pullRequestEvents, ev.EventName()) {
				return true
			}
		}
		return false
	},
	// The ignores of the config file are not edited by the fixer
	"unused-ignore": func(e *Error, _ *Workflow) bool { return strings.HasPrefix(e.Message, "the ignore for ") },
}

// TestFixableRulesExplainMissingFix pins the contract of Error.NoFix: a finding of a rule which has fixes (Fixable)
// either carries a Fix, or says why it has none, or is one of the kinds the rule never fixes. A new site that reports
// without a fix and without a reason would be advice only by accident.
func TestFixableRulesExplainMissingFix(t *testing.T) {
	cfg := mustParseConfig(t, "profile: pedantic\nrules:\n  missing-timeout: error\n  mutable-runner-label: warn\n")
	check := func(name string, src []byte) {
		l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Config: cfg, NoIgnores: true})
		if err != nil {
			t.Fatal(err)
		}
		errs, _ := l.Lint(name, src, nil)
		for _, e := range errs {
			info, ok := ruleIndex[e.ID]
			if !ok || !info.Fixable || e.Fix != nil || e.NoFix != nil {
				continue
			}
			if advice, ok := adviceOnlyKinds[e.ID]; ok {
				if w, _ := Parse(src); w != nil && advice(e, w) {
					continue
				}
			}
			t.Errorf("%s:%d: %s has neither a fix nor a reason: %s", name, e.Line, e.ID, e.Message)
		}
	}
	for _, d := range []string{"err", "examples", "fix", "ok"} {
		files, _ := filepath.Glob(filepath.Join("testdata", d, "*.y*ml"))
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			check(f, src)
		}
	}
	if testing.Short() {
		return
	}
	names, srcs := realWorldCorpus(t)
	for i := 0; i < len(names); i += 5 {
		check(names[i], srcs[i])
	}
}

// inRunScript reports whether the finding is inside the script of a run: step, the only place where the fix of
// template-injection works.
func inRunScript(e *Error, w *Workflow) bool {
	found := false
	for _, j := range w.Jobs {
		if j == nil {
			continue
		}
		walkSteps(j.Steps, func(s *Step) {
			if r, ok := s.Exec.(*ExecRun); ok && r.Run != nil && r.Run.Pos != nil {
				from := r.Run.Pos.Line
				if e.Line >= from && e.Line <= from+strings.Count(r.Run.Value, "\n")+1 {
					found = true
				}
			}
		})
	}
	return found
}
