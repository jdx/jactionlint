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
	"github.com/jdx/jactionlint/v2/internal/runner/cache"
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

	// ContainerEngine names docker or podman ("" finds one); NoContainers refuses every container; ContainerMode is
	// --container (run Linux jobs in a container, with ContainerImage for all of them when set, or Images per label).
	ContainerEngine        string
	NoContainers           bool
	ContainerMode          bool
	ContainerImage         string
	Images                 map[string]string
	Platform               string
	TrustImages            bool
	UnsafeContainerOptions bool

	Proxy        *Proxy   // the proxy of --net=proxy, started by the caller
	TrustActions bool     // run actions that are not pinned to a commit SHA
	NoShim       []string // actions/checkout etc. whose built-in replacement is turned off

	// Cache is "on", "read-only" or "off". CacheDir overrides the location, CacheSize the size limit in bytes.
	Cache      string
	CacheDir   string
	CacheSize  int64
	EnableShim []string // opt-in shims, such as actions/cache

	Parallel int  // how many cells run at once (1: one after the other)
	DryRun   bool // print what would run, run nothing

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

	slots      chan int // free slots of the cells that run at once; nil when they run one after the other
	ctrEng     *ctrEngine
	ctrSeq     int
	cacheSvc   *cache.Service
	cacheStore *cache.Store
	cacheEnv   map[string]string
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
	if e.cfg.DryRun {
		ok := e.runJobs(ctx)
		e.writeSummary()
		return ok, nil
	}
	rd, err := NewRunDir(e.cfg.Root)
	if err != nil {
		return false, err
	}
	e.rd = rd
	e.node = newNodeProvider(rd.ToolCache)
	if svc, err := e.startCache(); err != nil {
		e.pr.Note(0, "warning: the cache service did not start, so nothing is cached: %v", err)
	} else if svc != nil {
		e.cacheSvc = svc
		e.cacheEnv = svc.Env()
		defer svc.Close()
	}
	if !e.cfg.Keep {
		defer rd.Remove()
	}

	if e.cfg.Parallel > 1 {
		e.slots = make(chan int, e.cfg.Parallel)
		for i := 0; i < e.cfg.Parallel; i++ {
			e.slots <- i
		}
	}
	ok := e.runJobs(ctx)
	e.writeSummary()
	e.keepArtifacts()
	if e.cfg.Keep {
		e.pr.Note(0, "run directory kept: %s", rd.Root)
	}
	return ok, nil
}

// runJobs runs the selected jobs of the plan: in order, or with --parallel as soon as the jobs they need are done.
func (e *Engine) runJobs(ctx context.Context) bool {
	var jobs []*PlannedJob
	for _, pj := range e.plan.Jobs {
		if pj.Selected {
			jobs = append(jobs, pj)
		}
	}
	record := func(pj *PlannedJob, res *jobResult) {
		e.mu.Lock()
		e.results[strings.ToLower(pj.ID)] = res
		e.mu.Unlock()
	}
	bad := func(res *jobResult) bool { return res.Result == "failure" || res.Result == "cancelled" }
	if e.root().slots == nil {
		ok := true
		for _, pj := range jobs {
			res := e.runJob(ctx, pj)
			record(pj, res)
			if bad(res) {
				ok = false
			}
		}
		return ok
	}

	type done struct {
		pj  *PlannedJob
		res *jobResult
	}
	finished := make(chan done)
	started := map[*PlannedJob]bool{}
	complete := map[string]bool{}
	running := 0
	ok := true
	for len(complete) < len(jobs) {
		for _, pj := range jobs {
			if started[pj] {
				continue
			}
			ready := true
			for _, n := range pj.Needs {
				// a need that is not selected has no result; it never blocks
				if np := e.plan.Job(n); np != nil && np.Selected && !complete[strings.ToLower(n)] {
					ready = false
				}
			}
			if !ready {
				continue
			}
			started[pj] = true
			running++
			go func(pj *PlannedJob) { finished <- done{pj, e.runJob(ctx, pj)} }(pj)
		}
		if running == 0 {
			break // cannot happen: the plan has no cycles
		}
		d := <-finished
		running--
		record(d.pj, d.res)
		complete[strings.ToLower(d.pj.ID)] = true
		if bad(d.res) {
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
	maxParallel := len(cells)
	if s := pj.Job.Strategy; s != nil && s.MaxParallel != nil && s.MaxParallel.Expression == nil && s.MaxParallel.Value > 0 {
		maxParallel = s.MaxParallel.Value
	}

	// With --parallel the cells of a job run at once, up to max-parallel and to the free slots; a failed cell cancels
	// the ones that still run when fail-fast is on.
	cellCtx, cancelCells := context.WithCancel(ctx)
	defer cancelCells()
	results := make([]*jobResult, len(cells))
	var wg sync.WaitGroup
	local := make(chan struct{}, maxParallel)
	slots := e.root().slots
	if pj.Job.WorkflowCall != nil {
		slots = nil // the jobs of a called workflow take the slots; the call itself must not hold one
	}
	concurrent := e.root().slots != nil
	for i, c := range cells {
		local <- struct{}{}
		if cellCtx.Err() != nil && ctx.Err() == nil {
			<-local
			e.pr.JobSkip(e.prefix+c.jobName(pj), "an earlier matrix cell failed (fail-fast)")
			e.addSummary(e.prefix+c.jobName(pj), "skipped", "fail-fast")
			continue
		}
		slot := 0
		if slots != nil {
			slot = <-slots
		}
		run := func(i int, c *Cell, slot int) {
			defer func() {
				<-local
				if slots != nil {
					slots <- slot
				}
				wg.Done()
			}()
			cr := e.runCell(cellCtx, pj, c, i, len(cells), env, slot)
			results[i] = cr
			if failFast && (cr.Result == "failure" || cr.Result == "cancelled") {
				cancelCells()
			}
		}
		wg.Add(1)
		if !concurrent {
			run(i, c, slot)
		} else {
			go run(i, c, slot)
		}
	}
	wg.Wait()
	for _, cr := range results {
		if cr == nil {
			continue
		}
		switch {
		case cr.Result == "failure" || cr.Result == "cancelled":
			res.Result = cr.Result
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

// cacheScopes are the scopes a restore reads: the current ref, then the default branch (GitHub's fallback), so that a
// throwaway branch cannot poison the cache of main.
func (e *Engine) cacheScopes() []string {
	gh := BaseEnv(e.cfg.In).Contexts["github"].(map[string]any)
	ref, _ := gh["ref"].(string)
	scopes := []string{ref}
	if def := e.cfg.In.Repo.DefaultBranch; def != "" && "refs/heads/"+def != ref {
		scopes = append(scopes, "refs/heads/"+def)
	}
	return scopes
}

// repoCacheDir is the directory of the cache of this repository.
func (cfg *Config) repoCacheDir() (string, error) {
	base := cfg.CacheDir
	if base == "" {
		state, err := StateDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(state, "cache")
	}
	id := cfg.In.Repo.Owner + "-" + cfg.In.Repo.Name
	return filepath.Join(base, sanitizeName(id)), nil
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// startCache starts the local cache service that actions/cache and the actions built on it talk to.
func (e *Engine) startCache() (*cache.Service, error) {
	mode := e.cfg.Cache
	if mode == "off" {
		return nil, nil
	}
	dir, err := e.cfg.repoCacheDir()
	if err != nil {
		return nil, err
	}
	st, err := cache.Open(dir)
	if err != nil {
		return nil, err
	}
	if e.cfg.CacheSize > 0 {
		st.MaxBytes = e.cfg.CacheSize
	}
	st.ReadOnly = mode == "read-only"
	svc, err := cache.NewService(st, e.cacheScopes())
	if err != nil {
		return nil, err
	}
	if e.cfg.Debug {
		svc.Log = func(format string, a ...any) { e.pr.Note(0, "debug: "+format, a...) }
	}
	e.cacheStore = st
	return svc, nil
}

// containerEngine finds the engine once and checks that it answers.
func (e *Engine) containerEngine(ctx context.Context) (*ctrEngine, error) {
	r := e.root()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctrEng != nil {
		return r.ctrEng, nil
	}
	if r.cfg.NoContainers {
		return nil, fmt.Errorf("this needs a container, and --no-containers is set")
	}
	eng, err := detectEngine(r.cfg.ContainerEngine)
	if err != nil {
		return nil, err
	}
	if err := eng.available(ctx); err != nil {
		return nil, err
	}
	r.ctrEng = eng
	return eng, nil
}

func (e *Engine) nextContainerSeq() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ctrSeq++
	return e.ctrSeq
}
