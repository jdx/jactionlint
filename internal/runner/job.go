package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// jobRun is the state of one cell of a job while its steps run.
// jobShared is the state that the steps of a job, and the steps of the composite actions inside it, share.
type jobShared struct {
	ghEnv   map[string]string // set by GITHUB_ENV, in order of arrival
	pathAdd []string          // set by GITHUB_PATH, newest first
	summary []string
	failed  bool
	posts   []*postStep // post steps of actions, in the order their main steps ran
}

type jobRun struct {
	*jobShared
	e    *Engine
	pr   *Printer // where this job writes; a buffer when jobs run in parallel
	ctr  *jobContainer
	pj   *PlannedJob
	cell *Cell
	name string
	rd   *RunDir // the per-job scratch area
	work string  // GITHUB_WORKSPACE
	ctx  context.Context

	env      *eval.Env
	baseEnv  map[string]string // workflow env < job env, evaluated
	steps    map[string]any
	bg       map[string]*bgStep
	bgOrder  []*bgStep
	stepNo   int
	prefix   string // "" for the steps of the job, "step 3" inside the composite action of step 3
	depth    int    // nesting of composite actions
	action   *actionScope
	defaults struct{ shell, workdir string }
}

// cancelGrace is how long steps may run after the run was cancelled (GitHub's cancel-timeout-minutes default).
const cancelGrace = 5 * time.Minute

type stepResult struct {
	Outcome    string // success, failure, skipped
	Conclusion string
	Note       string
}

func (e *Engine) runCell(ctx context.Context, pj *PlannedJob, cell *Cell, idx, total int, base *eval.Env, slot int) *jobResult {
	j := pj.Job
	jr := &jobRun{jobShared: &jobShared{ghEnv: map[string]string{}}, e: e, pj: pj, cell: cell, steps: map[string]any{}, bg: map[string]*bgStep{}}
	jr.pr = e.pr
	if e.root().slots != nil {
		jr.pr = e.pr.Buffered()
		defer jr.pr.Flush()
	}
	jr.env = cellEnv(base, cell)
	jr.env.Status = nil
	jr.env.Contexts["strategy"] = map[string]any{
		"fail-fast": true, "job-index": float64(idx), "job-total": float64(total), "max-parallel": float64(total),
	}
	jr.env.Contexts["job"] = map[string]any{"status": "success"}
	jr.env.Contexts["steps"] = jr.steps
	jr.env.HashFiles = nil

	jr.name = cell.Name
	if j.Name != nil {
		if s, err := jr.env.Interpolate(j.Name.Value); err == nil && s != "" {
			jr.name = s
		}
	}
	jr.name = e.prefix + jr.name
	skip := func(why string) *jobResult {
		jr.pr.JobSkip(jr.name, why)
		e.addSummary(jr.name, "skipped", why)
		return &jobResult{Result: "skipped", Outputs: map[string]any{}}
	}

	if j.If != nil {
		cs, _ := eval.StringContexts(ifSource(j.If.Value))
		for _, c := range cs {
			if c == "matrix" {
				allOK, anyFailed := e.needsStatus(pj)
				jr.env.Status = func(name string) bool {
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
				ok, err := jr.env.Condition(j.If.Value)
				jr.env.Status = nil
				if err != nil || !ok {
					return skip("its if condition is false")
				}
				break
			}
		}
	}

	// Where the job would run: in a container when it asks for one (container:, services:) or under --container, on this
	// machine when the labels fit its operating system, otherwise nowhere.
	var labels []string
	if j.RunsOn != nil {
		if j.RunsOn.LabelsExpr != nil {
			if v, err := jr.env.Value(j.RunsOn.LabelsExpr.Value); err == nil {
				if l, ok := v.([]any); ok {
					for _, x := range l {
						labels = append(labels, eval.ToString(x))
					}
				} else {
					labels = append(labels, eval.ToString(v))
				}
			}
		}
		for _, l := range j.RunsOn.Labels {
			if s, err := jr.env.Interpolate(l.Value); err == nil {
				labels = append(labels, s)
			}
		}
	}
	wantsContainer := j.Container != nil || (j.Services != nil && (len(j.Services.Value) > 0 || j.Services.Expression != nil))
	target := targetOS(labels)
	rootMode := false
	ctrImage := ""
	switch {
	case j.WorkflowCall != nil:
	case wantsContainer:
	case e.cfg.ContainerMode && (target == "linux" || target == "" && len(labels) > 0 && e.cfg.ContainerImage != ""):
		img, err := imageForLabels(labels, e.cfg.ContainerImage, e.cfg.Images)
		if err != nil {
			return skip(err.Error())
		}
		ctrImage, rootMode, wantsContainer = img, true, true
	case target != "" && target != hostOS():
		why := fmt.Sprintf("runs-on %s needs %s, this machine is %s", strings.Join(labels, ", "), hostName[target], hostName[hostOS()])
		if target == "linux" {
			why += "; --container runs it in a Linux container"
		}
		return skip(why + explainRef("runs-on-other-os"))
	}
	if e.cfg.DryRun && j.WorkflowCall == nil {
		where := "on this machine"
		if wantsContainer {
			where = "in a container"
			if ctrImage != "" {
				where = "in " + ctrImage
			}
		}
		return e.dryRunCell(ctx, pj, jr, where)
	}
	if wantsContainer && j.WorkflowCall == nil {
		if e.cfg.NoContainers {
			return skip("it needs a container and --no-containers is set" + explainRef("no-container-engine"))
		}
		if _, err := e.containerEngine(ctx); err != nil {
			return skip(err.Error() + explainRef("no-container-engine"))
		}
		if ctrImage == "" && j.Container != nil {
			switch {
			case j.Container.Expression != nil:
				v, err := jr.env.Value(j.Container.Expression.Value)
				if err != nil {
					return skip("container: " + err.Error())
				}
				if m, ok := v.(map[string]any); ok {
					if img, ok := m["image"].(string); ok {
						ctrImage = img
					}
				} else {
					ctrImage = eval.ToString(v)
				}
			case j.Container.Image != nil:
				img, err := jr.interp(jr.env, j.Container.Image.Value)
				if err != nil {
					return skip("container: " + err.Error())
				}
				ctrImage = img
			}
			if ctrImage == "" {
				return skip("container: has no image")
			}
		}
		if ctrImage == "" && !rootMode {
			// services next to a job that runs on this machine: no job container, only the services
			ctrImage = ""
		}
	}

	if j.WorkflowCall != nil {
		return e.runReusable(ctx, pj, cell, jr)
	}

	jr.pr.JobStart(jr.name)
	start := time.Now()

	root := e.root()
	root.mu.Lock()
	root.jobSeq++
	seq := root.jobSeq
	root.mu.Unlock()
	rd, release, err := e.rd.Job(seq, slot)
	if err != nil {
		return e.failJob(jr, start, fmt.Sprintf("cannot create the run directory: %v", err))
	}
	defer release()
	jr.rd = rd
	jr.work = rd.Work
	if e.cfg.InPlace {
		jr.work = e.cfg.Root
	} else if err := CopyWorkspace(e.cfg.Root, rd.Work); err != nil {
		return e.failJob(jr, start, fmt.Sprintf("cannot copy the workspace: %v", err))
	}

	if wantsContainer {
		jr.pr.Note(1, "starting the %s", containerWhat(ctrImage, j))
		jc, err := jr.startContainer(ctx, ctrImage, j.Container, rootMode)
		if err != nil {
			return e.failJob(jr, start, err.Error())
		}
		jr.ctr = jc
		defer jc.stop(context.WithoutCancel(ctx))
		jr.env.Contexts["job"] = jc.jobContext("success")
	}

	timeout := 360.0
	if j.TimeoutMinutes != nil && j.TimeoutMinutes.Expression == nil {
		timeout = j.TimeoutMinutes.Value
	}
	jctx, cancel := context.WithTimeout(ctx, time.Duration(timeout*float64(time.Minute)))
	defer cancel()
	jr.ctx = jctx

	// env: workflow < job, evaluated now
	jr.baseEnv = map[string]string{}
	for k, v := range e.cfg.EnvFile {
		jr.baseEnv[k] = v
	}
	for _, env := range []*jactionlint.Env{e.wf.Env, j.Env} {
		if err := jr.addEnv(jr.baseEnv, env, jr.env); err != nil {
			return e.failJob(jr, start, err.Error())
		}
		jr.env.Contexts["env"] = toAnyMap(jr.baseEnv)
	}
	for _, d := range []*jactionlint.Defaults{e.wf.Defaults, j.Defaults} {
		if d != nil && d.Run != nil {
			if d.Run.Shell != nil {
				jr.defaults.shell = d.Run.Shell.Value
			}
			if d.Run.WorkingDirectory != nil {
				jr.defaults.workdir = d.Run.WorkingDirectory.Value
			}
		}
	}

	failedBefore := false
	_ = failedBefore
	for _, st := range j.Steps {
		jr.runStepEntry(st)
	}
	jr.joinAll()
	jr.runPosts()
	if jctx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		jr.failed = true
		jr.pr.Note(1, "the job timed out after %s minutes", eval.FormatNumber(timeout))
	}

	result := "success"
	switch {
	case ctx.Err() != nil:
		result = "cancelled"
	case jr.failed:
		result = "failure"
	}
	if jr.ctr != nil {
		jr.env.Contexts["job"] = jr.ctr.jobContext(result)
	} else {
		jr.env.Contexts["job"] = map[string]any{"status": result}
	}

	out := map[string]any{}
	for name, o := range j.Outputs {
		key := name
		if o.Name != nil {
			key = o.Name.Value
		}
		if o.Value == nil {
			continue
		}
		v, err := jr.interp(jr.env, o.Value.Value)
		if err != nil {
			jr.pr.Note(1, "warning: output %s: %v", key, err)
			continue
		}
		out[key] = v
	}
	if len(jr.summary) > 0 {
		jr.pr.Note(1, "step summary:")
		for _, l := range jr.summary {
			jr.pr.Note(2, "%s", l)
		}
	}
	jr.pr.JobEnd(jr.name, result, durationOf(start))
	e.addSummary(jr.name, result, "")
	return &jobResult{Result: result, Outputs: out}
}

func (e *Engine) failJob(jr *jobRun, start time.Time, msg string) *jobResult {
	jr.pr.Note(1, "error: %s", msg)
	jr.pr.JobEnd(jr.name, "failure", durationOf(start))
	e.addSummary(jr.name, "failure", msg)
	return &jobResult{Result: "failure", Outputs: map[string]any{}}
}

func toAnyMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// addEnv evaluates the variables of an env section into dst.
func (jr *jobRun) addEnv(dst map[string]string, env *jactionlint.Env, ev *eval.Env) error {
	if env == nil {
		return nil
	}
	if env.Expression != nil || len(env.Inserts) > 0 {
		var srcs []*jactionlint.String
		if env.Expression != nil {
			srcs = append(srcs, env.Expression)
		}
		srcs = append(srcs, env.Inserts...)
		for _, s := range srcs {
			v, err := ev.Value(s.Value)
			if err != nil {
				return err
			}
			if m, ok := v.(map[string]any); ok {
				for k, x := range m {
					dst[k] = eval.ToString(x)
				}
			}
		}
	}
	names := make([]*jactionlint.EnvVar, 0, len(env.Vars))
	for _, v := range env.Vars {
		names = append(names, v)
	}
	sort.Slice(names, func(a, b int) bool { return posLess(names[a].Name.Pos, names[b].Name.Pos) })
	for _, v := range names {
		s, err := jr.interp(ev, v.Value.Value)
		if err != nil {
			return err
		}
		dst[v.Name.Value] = s
	}
	return nil
}

// interp interpolates the ${{ }} of a string and warns about secrets that were not supplied.
func (jr *jobRun) interp(ev *eval.Env, s string) (string, error) {
	if !eval.HasExpr(s) {
		return s, nil
	}
	if sec, ok := ev.Contexts["secrets"].(map[string]any); ok {
		for _, name := range eval.Refs(s, "secrets") {
			found := false
			for k := range sec {
				if strings.EqualFold(k, name) {
					found = true
				}
			}
			if !found {
				jr.e.warnOnce("secret:"+name, "secret %s is not set, so ${{ secrets.%s }} is empty (pass --secret %s=VALUE)%s", strings.ToUpper(name), strings.ToUpper(name), strings.ToUpper(name), explainRef("secret-unset"))
			}
		}
	}
	return ev.Interpolate(s)
}

// status answers success(), failure(), cancelled() and always() for the steps of the job.
func (jr *jobRun) status(name string) bool {
	switch name {
	case "success":
		return !jr.failed && jr.ctx.Err() == nil
	case "failure":
		return jr.failed
	case "cancelled":
		return jr.ctx.Err() != nil
	}
	return true
}

func (jr *jobRun) stepEnv(step *jactionlint.Step) *eval.Env {
	ev := &eval.Env{Contexts: map[string]any{}, Status: jr.status}
	for k, v := range jr.env.Contexts {
		ev.Contexts[k] = v
	}
	merged := map[string]string{}
	for k, v := range jr.baseEnv {
		merged[k] = v
	}
	for k, v := range jr.ghEnv {
		merged[k] = v
	}
	ev.Contexts["env"] = toAnyMap(merged)
	ev.HashFiles = func(p []string) (string, error) { return hashFiles(jr.work, p) }
	return ev
}

// runStepEntry handles one step of the job, including its condition.
func (jr *jobRun) runStepEntry(step *jactionlint.Step) {
	n := jr.nextLabel()
	ev := jr.stepEnv(step)
	name := jr.stepName(step, ev)

	cond := true
	if step.If != nil {
		ok, err := ev.Condition(step.If.Value)
		if err != nil {
			jr.pr.StepStart(n, name)
			jr.pr.Note(n.indent()+1, "error: if: %v", err)
			jr.recordStep(step, stepResult{Outcome: "failure", Conclusion: "failure"})
			jr.failed = true
			jr.pr.StepEnd(n, "failure", "bad if", 0)
			return
		}
		cond = ok
	} else {
		cond = jr.status("success")
	}
	if !cond {
		why := "if is false"
		if step.If == nil {
			why = "an earlier step failed"
			if jr.ctx.Err() != nil {
				why = "the run was cancelled"
			}
		}
		jr.pr.StepSkip(n, name, why)
		jr.recordStep(step, stepResult{Outcome: "skipped", Conclusion: "skipped"})
		return
	}

	switch ex := step.Exec.(type) {
	case *jactionlint.ExecRun:
		background := step.Background != nil && step.Background.Value && step.Background.Expression == nil
		if background {
			jr.startBackground(n, name, step, ex, ev)
			return
		}
		jr.pr.StepStart(n, name)
		start := time.Now()
		// A step that still runs after a cancel (if: always()) gets a fresh context and the grace period of GitHub.
		runCtx := jr.ctx
		if jr.ctx.Err() != nil {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(context.WithoutCancel(jr.ctx), cancelGrace)
			defer cancel()
		}
		res, eff := jr.execRun(n, step, ex, ev, func(l string) { jr.pr.StepOutput(n, l) }, runCtx)
		jr.finishStep(n, step, res, eff, start)
	case *jactionlint.ExecAction:
		jr.runAction(n, name, step, ex, ev)
	case *jactionlint.ExecWait:
		jr.wait(n, name, ex)
	case *jactionlint.ExecCancel:
		jr.cancelBG(n, name, ex)
	case *jactionlint.ExecParallel:
		jr.parallel(n, name, step, ex)
	default:
		jr.pr.StepSkip(n, name, "this kind of step is not supported yet")
	}
}

// nextLabel numbers the next step of this scope.
func (jr *jobRun) nextLabel() label {
	jr.stepNo++
	if jr.prefix == "" {
		return label(fmt.Sprintf("step %d", jr.stepNo))
	}
	return label(fmt.Sprintf("%s.%d", jr.prefix, jr.stepNo))
}

func (jr *jobRun) stepName(step *jactionlint.Step, ev *eval.Env) string {
	if step.Name != nil {
		if s, err := ev.Interpolate(step.Name.Value); err == nil && s != "" {
			return s
		}
		return step.Name.Value
	}
	switch ex := step.Exec.(type) {
	case *jactionlint.ExecRun:
		first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(ex.Run.Value), "\n", 2)[0])
		if len(first) > 80 {
			first = first[:77] + "..."
		}
		return "Run " + first
	case *jactionlint.ExecAction:
		return "Run " + ex.Uses.Value
	case *jactionlint.ExecWait:
		return "wait"
	case *jactionlint.ExecCancel:
		return "cancel"
	case *jactionlint.ExecParallel:
		return "parallel"
	}
	return "step"
}

// finishStep records the outcome of a step and applies what it wrote through file commands.
func (jr *jobRun) finishStep(n label, step *jactionlint.Step, res stepResult, eff *effects, start time.Time) {
	e := jr.e
	coe := false
	if step.ContinueOnError != nil {
		if step.ContinueOnError.Expression != nil {
			coe, _ = jr.stepEnv(step).Condition("${{ " + trimExpr(step.ContinueOnError.Expression.Value) + " }}")
		} else {
			coe = step.ContinueOnError.Value
		}
	}
	res.Conclusion = res.Outcome
	if res.Outcome == "failure" && coe {
		res.Conclusion = "success"
		if res.Note != "" {
			res.Note += ", "
		}
		res.Note += "continue-on-error"
	}
	if eff != nil {
		jr.apply(eff, step)
		eff.report(e.pr, n.indent()+1)
	}
	if res.Conclusion == "failure" {
		jr.failed = true
	}
	jr.recordStep(step, res)
	if eff != nil {
		jr.setOutputs(step, eff.outputs)
	}
	shown := res.Outcome
	jr.pr.StepEnd(n, shown, res.Note, durationOf(start))
	if res.Outcome == "failure" && eff != nil && eff.hint != "" {
		jr.pr.Note(n.indent()+1, "hint: %s", eff.hint)
	}
}

func trimExpr(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "${{")
	s = strings.TrimSuffix(s, "}}")
	return strings.TrimSpace(s)
}

func (jr *jobRun) recordStep(step *jactionlint.Step, res stepResult) {
	if step.ID == nil || step.ID.Value == "" {
		return
	}
	prev, _ := jr.steps[step.ID.Value].(map[string]any)
	outputs := map[string]any{}
	if prev != nil {
		if o, ok := prev["outputs"].(map[string]any); ok {
			outputs = o
		}
	}
	jr.steps[step.ID.Value] = map[string]any{"outputs": outputs, "outcome": res.Outcome, "conclusion": res.Conclusion}
}

func (jr *jobRun) setOutputs(step *jactionlint.Step, out map[string]string) {
	if step.ID == nil || step.ID.Value == "" || len(out) == 0 {
		return
	}
	m, _ := jr.steps[step.ID.Value].(map[string]any)
	if m == nil {
		return
	}
	o, _ := m["outputs"].(map[string]any)
	if o == nil {
		o = map[string]any{}
		m["outputs"] = o
	}
	for k, v := range out {
		o[k] = v
	}
}

// apply merges the effects of a finished step into the job: the env and PATH that later steps see.
func (jr *jobRun) apply(eff *effects, step *jactionlint.Step) {
	for _, k := range eff.envKeys {
		jr.ghEnv[k] = eff.env[k]
	}
	for _, p := range eff.path {
		jr.pathAdd = append([]string{p}, jr.pathAdd...)
	}
	if eff.summary != "" {
		jr.summary = append(jr.summary, strings.Split(strings.TrimRight(eff.summary, "\n"), "\n")...)
	}
}

// pathWith returns the PATH of the steps: the additions of GITHUB_PATH in front of the PATH of the caller.
func (jr *jobRun) pathWith(base string) string {
	parts := append([]string(nil), jr.pathAdd...)
	if base != "" {
		parts = append(parts, base)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

// policyFor builds the sandbox policy of a step.
func (jr *jobRun) policyFor(files fileSet, workdir string, finalPath string) *sandbox.Policy {
	cfg := jr.e.cfg
	opts := sandbox.Options{
		Workspace: jr.work, Home: jr.rd.Home, Temp: jr.rd.Temp, ToolCache: jr.rd.ToolCache,
		PathDirs: sandbox.PathDirs(finalPath), RealHome: userHome(), Net: cfg.Net,
		ExtraRead: cfg.AllowRead, ExtraWrite: cfg.AllowWrite,
	}
	if cfg.Net == sandbox.NetProxy {
		// the proxy and the cache service are the only places a step may connect to
		if cfg.Proxy != nil {
			opts.ConnectPorts = append(opts.ConnectPorts, cfg.Proxy.Port())
		}
		if svc := jr.e.root().cacheSvc; svc != nil {
			opts.ConnectPorts = append(opts.ConnectPorts, svc.Port())
		}
	}
	pol := sandbox.DefaultPolicy(opts)
	// The files a step talks to the runner through are writable one by one: it cannot replace them.
	pol.Write = append(pol.Write, files.writable()...)
	pol.Read = append(pol.Read, files.event)
	pol.Read = append(pol.Read, probeFiles(jr.work, workdir, jr.rd.Root)...)
	return pol
}

var _ = filepath.Join
var _ = runtime.GOOS
var _ sync.Mutex

// probeFiles lists the package.json files in the parent directories of the given paths. Node looks for one in every
// parent of the script it runs and fails on a file it is denied, while a file that does not exist is fine, so these
// files (and only these) are made readable.
func probeFiles(dirs ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		for p := d; ; p = filepath.Dir(p) {
			f := filepath.Join(p, "package.json")
			if !seen[f] {
				seen[f] = true
				if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
					out = append(out, f)
				}
			}
			if filepath.Dir(p) == p {
				break
			}
		}
	}
	return out
}

func containerWhat(image string, j *jactionlint.Job) string {
	if image == "" {
		return "services of the job"
	}
	if j.Container == nil {
		return "container " + image + " (--container)"
	}
	return "job container " + image
}

// parentEnv is the environment a step inherits from: the caller's, or the image's inside a container.
func (jr *jobRun) inCtr() bool { return jr.ctr != nil && jr.ctr.inContainer }

func (jr *jobRun) parentEnv() []string {
	if jr.inCtr() {
		return jr.ctr.imageEnv
	}
	return jr.e.cfg.ParentEnv
}

// nodeSearchPath is where a JavaScript action may look for a node binary: the PATH of the step on this machine, but
// inside a container only the tool cache, which is mounted there.
func (jr *jobRun) nodeSearchPath(finalPath string) []string {
	if jr.inCtr() {
		return nil
	}
	return sandboxPathDirs(finalPath)
}
