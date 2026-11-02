package jactionlint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

// RuleUnreachableRequiredCheck reports the trigger filters of a workflow which keep GitHub from starting the job that
// reports a check declared in "required-checks", so that the check stays pending (see required_checks.go). A declared
// name that no job reports is a problem of the declaration: the linter reports it in the config file at the end of the
// run (Linter.finishRequiredChecksRun).
type RuleUnreachableRequiredCheck struct {
	RuleBase
	checks    []RequiredCheck
	workflows checkWorkflows
	rel       string // the linted workflow, relative to the root of the repository
	declared  func(*RequiredCheck) string
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnreachableRequiredCheck) VisitWorkflowPre(n *Workflow) error {
	ws := rule.workflows.with(rule.rel, n)
	for i := range rule.checks {
		rc := &rule.checks[i]
		for _, p := range analyzeRequiredCheck(ws, rc, rule.declared(rc)).problems {
			if p.file == rule.rel {
				rule.ReportID("unreachable-required-check", p.pos, p.msg)
			}
		}
	}
	return nil
}

// declaredAt returns where the declaration is, relative to the root of the repository when it is inside.
func declaredAt(root string, rc *RequiredCheck) string {
	p := rc.Path
	if p == "" {
		return "\"required-checks\"" // the config was not read from a file, so its lines mean nothing to the reader
	}
	if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
		p = filepath.ToSlash(r)
	}
	return fmt.Sprintf("%s:%d", p, rc.Line)
}

// requiredChecksRun records the configs which declare required checks in a run, with their project.
type requiredChecksRun struct {
	mu   sync.Mutex
	cfgs map[*Config]*Project
}

func (l *Linter) trackRequiredChecks(cfg *Config, project *Project) {
	if cfg == nil || project == nil || len(cfg.RequiredChecks) == 0 || !cfg.RuleEnabled("unreachable-required-check") {
		return
	}
	l.requiredChecks.mu.Lock()
	defer l.requiredChecks.mu.Unlock()
	if l.requiredChecks.cfgs == nil {
		l.requiredChecks.cfgs = map[*Config]*Project{}
	}
	l.requiredChecks.cfgs[cfg] = project
}

// checkWorkflowsOf returns the workflows of the project, read once per run.
func (l *Linter) checkWorkflowsOf(p *Project) checkWorkflows {
	v, _ := l.checkWorkflows.LoadOrStore(p.root, &lazyCheckWorkflows{})
	lw := v.(*lazyCheckWorkflows)
	lw.once.Do(func() { lw.ws = readCheckWorkflows(p.root) })
	return lw.ws
}

// finishRequiredChecksRun reports the declarations of "required-checks" which no job can report, at the declaration in
// the config file, and the ones the analysis cannot decide, at level info. The problems located in a workflow are
// reported when that workflow is linted. The run state is reset.
func (l *Linter) finishRequiredChecksRun(results []fileResult) []fileResult {
	l.requiredChecks.mu.Lock()
	cfgs := l.requiredChecks.cfgs
	l.requiredChecks.cfgs = nil
	l.requiredChecks.mu.Unlock()
	if len(cfgs) == 0 {
		return results
	}
	byPath := map[string][]*Error{}
	for cfg, project := range cfgs {
		level := cfg.RuleLevel("unreachable-required-check")
		ws := l.checkWorkflowsOf(project)
		for i := range cfg.RequiredChecks {
			rc := &cfg.RequiredChecks[i]
			if rc.Path == "" {
				continue // the config was not read from a file: there is no place to report at
			}
			v := analyzeRequiredCheck(ws, rc, declaredAt(project.RootDir(), rc))
			var msg string
			sev := level
			switch {
			case v.unresolved:
				msg = fmt.Sprintf("no job of the workflows in .github/workflows reports the required check %q, so GitHub waits for it forever. a check is named by the \"name:\" of its job (or its ID), with the values of a matrix job in parentheses, or \"caller / callee\" for a job of a reusable workflow", rc.Name)
				if len(v.suggestions) > 0 {
					msg += ". did you mean " + strings.Join(quoteEach(v.suggestions), " or ") + "?"
				}
			case v.unknown != "" && len(v.problems) == 0:
				msg = fmt.Sprintf("cannot determine whether the required check %q is reported for every change in its scope: %s", rc.Name, v.unknown)
				sev = min(level, SeverityInfo)
			default:
				continue
			}
			e := &Error{ID: "unreachable-required-check", Kind: "unreachable-required-check", Message: msg, Filepath: rc.Path, Line: max(rc.Line, 1), Column: max(rc.Column, 1), Severity: sev}
			e.DocURL = ruleIndex["unreachable-required-check"].DocURL()
			byPath[rc.Path] = append(byPath[rc.Path], e)
		}
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		errs := byPath[p]
		slices.SortFunc(errs, compareErrors)
		src, _ := os.ReadFile(p)
		display := p
		if l.cwd != "" {
			if r, err := filepath.Rel(l.cwd, l.absFilePath(p)); err == nil {
				display = r
			}
		}
		lines := sourceLines(src)
		for _, e := range errs {
			e.Filepath = display
			e.fillRegion(lines)
		}
		errs = l.filterErrors(errs, nil) // --ignore applies to these findings like to any other
		errs = slices.DeleteFunc(errs, func(e *Error) bool { return e.Severity < l.minSeverity })
		if len(errs) == 0 {
			continue
		}
		merged := false
		for i := range results {
			if results[i].path == display {
				results[i].errs = append(results[i].errs, errs...)
				slices.SortFunc(results[i].errs, compareErrors)
				merged = true
				break
			}
		}
		if !merged {
			results = append(results, fileResult{file: p, path: display, src: src, errs: errs})
		}
	}
	return results
}

func quoteEach(ss []string) []string {
	ret := make([]string, len(ss))
	for i, s := range ss {
		ret[i] = fmt.Sprintf("%q", s)
	}
	return ret
}

func init() {
	registerRules(
		RuleInfo{ID: "unreachable-required-check", Group: RuleGroupCorrectness, Summary: "A check listed in required-checks is reported by no job, or a trigger filter keeps its workflow from starting, so the check stays pending.", DefaultLevel: SeverityError, DocsAnchor: "check-unreachable-required-check"},
	)
	registerRuleFactory("unreachable-required-check", func(env *RuleEnv) []Rule {
		if env.config == nil || len(env.config.RequiredChecks) == 0 || !env.config.RuleEnabled("unreachable-required-check") {
			return nil
		}
		if env.project == nil || env.checkWorkflows == nil {
			env.Skip("the workflow does not belong to a project, so the other workflows are not known")
			return nil
		}
		root := env.project.RootDir()
		abs := env.path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(env.cwd, abs)
		}
		rel, err := filepath.Rel(root, absPath(abs))
		rel = filepath.ToSlash(rel)
		if err != nil || !strings.HasPrefix(rel, ".github/workflows/") || strings.Contains(strings.TrimPrefix(rel, ".github/workflows/"), "/") {
			return nil // not a workflow GitHub runs
		}
		return []Rule{&RuleUnreachableRequiredCheck{
			RuleBase: RuleBase{
				name: "unreachable-required-check",
				desc: "Checks that the checks listed in required-checks are reported for every change they are required for",
			},
			checks:    env.config.RequiredChecks,
			workflows: env.checkWorkflows,
			rel:       rel,
			declared:  func(rc *RequiredCheck) string { return declaredAt(root, rc) },
		}}
	})
}
