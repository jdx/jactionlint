package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// Config is what a run needs besides the plan.
type Config struct {
	Root string // root of the repository
	In   *Inputs

	Secrets map[string]string
	EnvFile map[string]string // default env of the workflow (--env-file)

	Sandbox    bool // false: --sandbox=off
	Net        sandbox.Net
	InPlace    bool
	PassEnv    []string
	AllowRead  []string
	AllowWrite []string

	TrustActions bool     // run actions that are not pinned to a commit SHA
	NoShim       []string // actions/checkout etc. whose built-in replacement is turned off

	Debug     bool
	Timings   bool
	Keep      bool // keep .jactionlint/run/<id>
	ParentEnv []string

	Out io.Writer
}

// jobResult is what later jobs see in `needs`.
type jobResult struct {
	Result  string // success, failure, skipped, cancelled
	Outputs map[string]any
}

// Engine runs a plan.
type Engine struct {
	cfg    *Config
	plan   *Plan
	wf     *jactionlint.Workflow
	rd     *RunDir
	masker *Masker
	pr     *Printer

	mu      sync.Mutex
	results map[string]*jobResult // by lower-case job ID
	summary []summaryLine
	jobSeq  int
	warned  map[string]bool
	node    *nodeProvider

	// parent and prefix are set for the engine of a reusable workflow: it shares the run directory, the printer and
	// the summary with the engine of the caller, and its job names start with the name of the calling job.
	parent *Engine
	prefix string
	depth  int
}

type summaryLine struct {
	name, result, note string
}

// NewEngine prepares a run of the plan.
func NewEngine(cfg *Config, plan *Plan) *Engine {
	e := &Engine{cfg: cfg, plan: plan, wf: plan.Workflow, masker: &Masker{}, results: map[string]*jobResult{}, warned: map[string]bool{}}
	e.pr = newPrinter(cfg.Out, cfg.Timings)
	for _, v := range cfg.Secrets {
		e.masker.Add(v)
	}
	return e
}

// Run runs the selected jobs in order and returns whether every one of them succeeded or was legitimately skipped.
func (e *Engine) Run(ctx context.Context) (bool, error) {
	rd, err := NewRunDir(e.cfg.Root)
	if err != nil {
		return false, err
	}
	e.rd = rd
	e.node = newNodeProvider(rd.ToolCache)
	if !e.cfg.Keep {
		defer rd.Remove()
	}

	ok := e.runJobs(ctx)
	e.writeSummary()
	e.keepArtifacts()
	if e.cfg.Keep {
		e.pr.Note(0, "run directory kept: %s", rd.Root)
	}
	return ok, nil
}

// runJobs runs the selected jobs of the plan in order.
func (e *Engine) runJobs(ctx context.Context) bool {
	ok := true
	for _, pj := range e.plan.Jobs {
		if !pj.Selected {
			continue
		}
		res := e.runJob(ctx, pj)
		e.mu.Lock()
		e.results[strings.ToLower(pj.ID)] = res
		e.mu.Unlock()
		if res.Result == "failure" || res.Result == "cancelled" {
			ok = false
		}
	}
	return ok
}

func (e *Engine) root() *Engine {
	for e.parent != nil {
		e = e.parent
	}
	return e
}

func (e *Engine) writeSummary() {
	e.pr.Note(0, "")
	e.pr.Note(0, "Summary")
	for _, s := range e.summary {
		note := ""
		if s.note != "" {
			note = "  (" + s.note + ")"
		}
		e.pr.Note(1, "%-9s %s%s", paint(s.result, s.result), s.name, note)
	}
}

func (e *Engine) addSummary(name, result, note string) {
	r := e.root()
	r.mu.Lock()
	r.summary = append(r.summary, summaryLine{name, result, note})
	r.mu.Unlock()
}

func (e *Engine) warnOnce(key, format string, a ...any) {
	r := e.root()
	r.mu.Lock()
	seen := r.warned[key]
	r.warned[key] = true
	r.mu.Unlock()
	if !seen {
		e.pr.Note(0, "warning: "+format, a...)
	}
}

// jobEnv returns the expression environment before any step of a job: the contexts that exist once the needs of
// the job are done.
func (e *Engine) jobEnv(pj *PlannedJob) *eval.Env {
	env := BaseEnv(e.cfg.In)
	env.HashFiles = nil
	secrets := map[string]any{}
	for k, v := range e.cfg.Secrets {
		secrets[k] = v
	}
	if e.cfg.Debug {
		secrets["ACTIONS_STEP_DEBUG"] = "true"
	}
	env.Contexts["secrets"] = secrets
	needs := map[string]any{}
	e.mu.Lock()
	for _, n := range pj.Needs {
		r := e.results[strings.ToLower(n)]
		if r == nil {
			continue
		}
		needs[n] = map[string]any{"result": r.Result, "outputs": r.Outputs}
	}
	e.mu.Unlock()
	env.Contexts["needs"] = needs
	return env
}

// needsStatus returns the state of the needs of a job for the status functions.
func (e *Engine) needsStatus(pj *PlannedJob) (allOK, anyFailed bool) {
	allOK = true
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range pj.Needs {
		r := e.results[strings.ToLower(n)]
		if r == nil || r.Result != "success" {
			allOK = false
		}
		if r != nil && (r.Result == "failure" || r.Result == "cancelled") {
			anyFailed = true
		}
	}
	return allOK, anyFailed
}

func (e *Engine) runJob(ctx context.Context, pj *PlannedJob) *jobResult {
	env := e.jobEnv(pj)
	allOK, anyFailed := e.needsStatus(pj)
	env.Status = func(name string) bool {
		switch name {
		case "success":
			return allOK && ctx.Err() == nil
		case "failure":
			return anyFailed
		case "cancelled":
			return ctx.Err() != nil
		}
		return true
	}

	skip := func(why string) *jobResult {
		e.pr.JobSkip(e.prefix+pj.ID, why)
		e.addSummary(e.prefix+pj.ID, "skipped", why)
		return &jobResult{Result: "skipped", Outputs: map[string]any{}}
	}

	// A job that does not run because of its needs or its condition.
	if pj.Job.If != nil {
		// matrix is not known yet: the condition is evaluated per cell below when it reads it
		cs, _ := eval.StringContexts(ifSource(pj.Job.If.Value))
		readsMatrix := false
		for _, c := range cs {
			if c == "matrix" {
				readsMatrix = true
			}
		}
		if !readsMatrix {
			ok, err := env.Condition(pj.Job.If.Value)
			if err != nil {
				e.pr.Note(0, "error: job %s: if: %v", pj.ID, err)
				e.addSummary(pj.ID, "failure", "bad if")
				return &jobResult{Result: "failure", Outputs: map[string]any{}}
			}
			if !ok {
				return skip(skipReason(allOK, ctx))
			}
		}
	} else if !allOK || ctx.Err() != nil {
		return skip(skipReason(allOK, ctx))
	}

	cells := pj.Cells
	if pj.Dynamic != "" || hasUnresolvedMatrix(pj) {
		var err error
		cells, err = e.expandCells(pj, env)
		if err != nil {
			e.pr.Note(0, "error: job %s: %v", pj.ID, err)
			e.addSummary(pj.ID, "failure", err.Error())
			return &jobResult{Result: "failure", Outputs: map[string]any{}}
		}
	}
	if len(cells) == 0 {
		return skip("the matrix has no combinations")
	}

	res := &jobResult{Result: "skipped", Outputs: map[string]any{}}
	failFast := true
	if s := pj.Job.Strategy; s != nil && s.FailFast != nil && s.FailFast.Expression == nil {
		failFast = s.FailFast.Value
	}
	stop := false
	for i, c := range cells {
		if stop {
			e.pr.JobSkip(e.prefix+c.jobName(pj), "an earlier matrix cell failed (fail-fast)")
			e.addSummary(e.prefix+c.jobName(pj), "skipped", "fail-fast")
			continue
		}
		cr := e.runCell(ctx, pj, c, i, len(cells), env)
		switch {
		case cr.Result == "failure" || cr.Result == "cancelled":
			res.Result = cr.Result
			if failFast {
				stop = true
			}
		case cr.Result == "success" && res.Result == "skipped":
			res.Result = "success"
		}
		for k, v := range cr.Outputs {
			if s, ok := v.(string); !ok || s != "" || res.Outputs[k] == nil {
				res.Outputs[k] = v
			}
		}
	}
	return res
}

func skipReason(allOK bool, ctx context.Context) string {
	switch {
	case ctx.Err() != nil:
		return "the run was cancelled"
	case !allOK:
		return "a job it needs did not succeed"
	}
	return "its if condition is false"
}

func hasUnresolvedMatrix(pj *PlannedJob) bool { return pj.Dynamic != "" }

// expandCells builds the cells of a job whose matrix needs the contexts that exist only now.
func (e *Engine) expandCells(pj *PlannedJob, env *eval.Env) ([]*Cell, error) {
	j := pj.Job
	var m *jactionlint.Matrix
	if j.Strategy != nil {
		m = j.Strategy.Matrix
		if j.Strategy.Expression != nil {
			m = &jactionlint.Matrix{Expression: strategyMatrixExpr(j.Strategy.Expression)}
		}
	}
	combos, dyn, err := expandMatrix(m, envResolver(env))
	if err != nil {
		return nil, err
	}
	if dyn != "" {
		return nil, fmt.Errorf("the matrix %s reads data that does not exist", strings.TrimSpace(dyn))
	}
	var cells []*Cell
	for _, c := range combos {
		cells = append(cells, newCell(pj.ID, j, m != nil, c))
	}
	return cells, nil
}

func (c *Cell) jobName(pj *PlannedJob) string {
	if c.Display != "" {
		return c.Display
	}
	return c.Name
}

// durationOf is time.Since for the printer, kept apart so tests can run without a clock.
func durationOf(start time.Time) time.Duration { return time.Since(start) }

func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}

func inDir(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// keepArtifacts moves the artifacts of the run out of the run directory, which is removed, into
// .jactionlint/artifacts/<run>.
func (e *Engine) keepArtifacts() {
	src := e.artifactsDir()
	ents, err := os.ReadDir(src)
	if err != nil || len(ents) == 0 {
		return
	}
	dst := filepath.Join(e.cfg.Root, ".jactionlint", "artifacts", filepath.Base(e.rd.Root))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	if err := os.Rename(src, dst); err != nil {
		e.pr.Note(0, "warning: could not keep the artifacts: %v", err)
		return
	}
	e.pr.Note(0, "artifacts kept in %s", relPath(e.cfg.Root, dst))
}
