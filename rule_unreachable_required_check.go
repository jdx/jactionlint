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
	workflows *lazyCheckWorkflows
	rel       string // the linted workflow, relative to the root of the repository
	declared  func(*RequiredCheck) string
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleUnreachableRequiredCheck) VisitWorkflowPre(n *Workflow) error {
	rule.workflows.mu.Lock()
	defer rule.workflows.mu.Unlock()
	ws := rule.workflows.ws.with(map[string]*Workflow{rule.rel: n})
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

// checkWorkflowRel returns the path of a workflow file relative to the root of the repository, or false when GitHub
// does not run the file as a workflow (it is not directly in .github/workflows).
func checkWorkflowRel(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	name, ok := strings.CutPrefix(rel, ".github/workflows/")
	if !ok || strings.Contains(name, "/") || !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
		return "", false
	}
	return rel, true
}

// requiredChecksRun records the configs which declare required checks in a run, with their project and the workflows
// linted with them.
type requiredChecksRun struct {
	mu   sync.Mutex
	cfgs map[requiredChecksKey]*requiredChecksConfig
}

// requiredChecksKey is a config and a repository: one config (--config-file) can apply to several repositories, whose
// workflows are separate.
type requiredChecksKey struct {
	cfg  *Config
	root string
}

type requiredChecksConfig struct {
	cfg     *Config
	project *Project
	// linted are the workflows as they were linted, which may differ from the files on the disk (Linter.Lint)
	linted map[string]*Workflow
}

// trackRequiredChecks notes that a file is linted with a config which declares required checks. rel and w are the
// workflow as it was linted, or empty and nil when the file is not a workflow GitHub runs.
func (l *Linter) trackRequiredChecks(cfg *Config, project *Project, rel string, w *Workflow) {
	l.requiredChecks.mu.Lock()
	defer l.requiredChecks.mu.Unlock()
	if l.requiredChecks.cfgs == nil {
		l.requiredChecks.cfgs = map[requiredChecksKey]*requiredChecksConfig{}
	}
	k := requiredChecksKey{cfg, project.RootDir()}
	rc := l.requiredChecks.cfgs[k]
	if rc == nil {
		rc = &requiredChecksConfig{cfg: cfg, project: project, linted: map[string]*Workflow{}}
		l.requiredChecks.cfgs[k] = rc
	}
	if rel != "" && w != nil {
		rc.linted[rel] = w
	}
}

// checkWorkflowsOf returns the workflows of the project, read once per run. Hold their mu while reading them.
func (l *Linter) checkWorkflowsOf(p *Project) *lazyCheckWorkflows {
	v, _ := l.checkWorkflows.LoadOrStore(p.root, &lazyCheckWorkflows{})
	lw := v.(*lazyCheckWorkflows)
	lw.once.Do(func() { lw.ws = readCheckWorkflows(p.root) })
	return lw
}

// declarationErrors are the findings located in one config file, for one repository.
type declarationErrors struct {
	path    string
	cfg     *Config
	project *Project
	errs    []*Error
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
	// Every config file that was judged goes through the baseline, also without findings: an entry of the baseline for
	// a declaration that is fixed now is unused
	byKey := map[[2]string]*declarationErrors{}
	for _, run := range cfgs {
		cfg := run.cfg
		level := cfg.RuleLevel("unreachable-required-check")
		// The declarations are judged against the content that was linted, so that both passes see the same workflows
		lw := l.checkWorkflowsOf(run.project)
		lw.mu.Lock()
		ws := lw.ws.with(run.linted)
		for i := range cfg.RequiredChecks {
			rc := &cfg.RequiredChecks[i]
			if rc.Path == "" {
				continue // the config was not read from a file: there is no place to report at
			}
			k := [2]string{rc.Path, run.project.RootDir()}
			d := byKey[k]
			if d == nil {
				d = &declarationErrors{path: rc.Path, cfg: cfg, project: run.project, errs: []*Error{}}
				byKey[k] = d
			}
			v := analyzeRequiredCheck(ws, rc, declaredAt(run.project.RootDir(), rc))
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
			d.errs = append(d.errs, e)
		}
		lw.mu.Unlock()
	}
	ds := make([]*declarationErrors, 0, len(byKey))
	for _, d := range byKey {
		ds = append(ds, d)
	}
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].path != ds[j].path {
			return ds[i].path < ds[j].path
		}
		return ds[i].project.RootDir() < ds[j].project.RootDir()
	})
	for _, d := range ds {
		p := d.path
		src, _ := os.ReadFile(p)
		display := p
		if l.cwd != "" {
			if r, err := filepath.Rel(l.cwd, l.absFilePath(p)); err == nil {
				display = r
			}
		}
		lines := sourceLines(src)
		for _, e := range d.errs {
			e.Filepath = display
			e.fillRegion(lines)
		}
		// The ignores apply to these findings like to the findings of a workflow: --ignore, "paths" and "ignores" of
		// the config, unless --no-ignores
		errs := d.errs
		var pathConfigs []PathConfig
		if !l.noIgnores {
			l.trackIgnoreConfig(d.cfg, d.project, p, nil)
			errs = dropIgnored(errs, l.matchConfigIgnores(errs, d.cfg, d.project, display, nil))
			// The keys of "paths" are relative to the root of the repository, not to the working directory
			if rel, err := filepath.Rel(d.project.RootDir(), l.absFilePath(p)); err == nil {
				pathConfigs = d.cfg.PathConfigs(filepath.ToSlash(rel))
			}
		}
		errs = l.filterErrors(errs, pathConfigs)
		slices.SortFunc(errs, compareErrors)
		// The baseline sees them like the findings of a workflow: before the minimum severity
		bl, err := l.baselineFor(d.project, d.cfg)
		if err != nil {
			l.debug("The baseline is not applied to %s: %v", display, err)
			bl = nil
		}
		l.baselineStage(display, src, d.project, bl, errs)
		errs = slices.DeleteFunc(errs, func(e *Error) bool { return e.Severity < l.minSeverity })
		visible, baselined := splitBaselined(errs)
		// A config file without findings is still a result while the baseline is written: a partial write replaces
		// the entries of the files that it judged, and keeps the others
		if len(visible) == 0 && len(baselined) == 0 && !l.baseline.writing {
			continue
		}
		if visible == nil {
			visible = []*Error{}
		}
		results = appendFileErrors(results, fileResult{file: p, path: display, src: src, errs: visible, baselined: baselined})
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
		if env.checkRel == "" {
			return nil // not a workflow GitHub runs
		}
		root := env.project.RootDir()
		return []Rule{&RuleUnreachableRequiredCheck{
			RuleBase: RuleBase{
				name: "unreachable-required-check",
				desc: "Checks that the checks listed in required-checks are reported for every change they are required for",
			},
			checks:    env.config.RequiredChecks,
			workflows: env.checkWorkflows,
			rel:       env.checkRel,
			declared:  func(rc *RequiredCheck) string { return declaredAt(root, rc) },
		}}
	})
}
