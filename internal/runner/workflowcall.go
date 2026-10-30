package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
)

// maxWorkflowDepth is how deep reusable workflows may call reusable workflows (GitHub allows 10 levels).
const maxWorkflowDepth = 10

// runReusable runs a job that calls a reusable workflow (`uses:` on the job): the jobs of the called workflow run with
// the inputs and secrets the caller passes, and its outputs become the outputs of the calling job.
func (e *Engine) runReusable(ctx context.Context, pj *PlannedJob, cell *Cell, jr *jobRun) *jobResult {
	j := pj.Job
	wc := j.WorkflowCall
	start := time.Now()
	jr.pr.JobStart(jr.name + " (reusable workflow " + wc.Uses.Value + ")")
	fail := func(msg string) *jobResult {
		jr.pr.Note(1, "error: %s", msg)
		jr.pr.JobEnd(jr.name, "failure", durationOf(start))
		e.addSummary(jr.name, "failure", msg)
		return &jobResult{Result: "failure", Outputs: map[string]any{}}
	}
	if e.depth+1 > maxWorkflowDepth {
		return fail("reusable workflows are nested too deeply")
	}

	callee, path, err := e.loadCalledWorkflow(ctx, wc.Uses.Value)
	if err != nil {
		return fail(err.Error())
	}

	// inputs: what the caller passes in with:, then the defaults of the called workflow
	env := jr.env
	passed := map[string]any{}
	names := make([]*jactionlint.WorkflowCallInput, 0, len(wc.Inputs))
	for _, in := range wc.Inputs {
		names = append(names, in)
	}
	sort.Slice(names, func(a, b int) bool { return posLess(names[a].Name.Pos, names[b].Name.Pos) })
	for _, in := range names {
		v, err := env.Value(in.Value.Value)
		if err != nil {
			return fail(err.Error())
		}
		passed[strings.ToLower(in.Name.Value)] = v
	}
	inputs := map[string]any{}
	var declared *jactionlint.WorkflowCallEvent
	if ce, ok := callee.FindWorkflowCallEvent(); ok {
		declared = ce
	}
	if declared == nil {
		return fail(path + " does not have the workflow_call trigger, so it cannot be called")
	}
	for _, d := range declared.Inputs {
		key := strings.ToLower(d.Name.Value)
		v, given := passed[key]
		if !given && d.Default != nil {
			v = typedDefault(d)
			given = true
		}
		if !given {
			if d.Required != nil && d.Required.Value {
				return fail(fmt.Sprintf("the input %s of %s is required", d.Name.Value, path))
			}
			if d.Type == jactionlint.WorkflowCallEventInputTypeBoolean {
				v = false
			} else if d.Type == jactionlint.WorkflowCallEventInputTypeNumber {
				v = 0.0
			} else {
				v = ""
			}
		}
		inputs[d.Name.Value] = coerceInput(d.Type, v)
	}
	for k := range passed {
		found := false
		for _, d := range declared.Inputs {
			if strings.EqualFold(d.Name.Value, k) {
				found = true
			}
		}
		if !found {
			return fail(fmt.Sprintf("%s does not declare the input %s", path, k))
		}
	}

	// secrets
	secrets := map[string]string{}
	if wc.InheritSecrets {
		for k, v := range e.cfg.Secrets {
			secrets[k] = v
		}
	}
	for _, s := range wc.Secrets {
		v, err := env.Interpolate(s.Value.Value)
		if err != nil {
			return fail(err.Error())
		}
		secrets[s.Name.Value] = v
		e.masker.Add(v)
	}

	cfg := *e.cfg
	cfg.Secrets = secrets
	in := *e.cfg.In
	in.Inputs = inputs
	in.Env = nil
	cfg.In = &in
	plan, err := BuildPlan(callee, path, nil, BaseEnv(&in))
	if err != nil {
		return fail(fmt.Sprintf("%s: %v", path, err))
	}
	sub := &Engine{
		cfg: &cfg, plan: plan, wf: callee, rd: e.rd, masker: e.masker, pr: e.pr,
		results: map[string]*jobResult{}, warned: map[string]bool{}, node: e.node,
		parent: e, prefix: jr.name + " / ", depth: e.depth + 1, cacheSvc: e.cacheSvc, cacheStore: e.cacheStore,
	}
	ok := sub.runJobs(ctx)

	// outputs of the called workflow are expressions over `jobs`
	jobs := map[string]any{}
	result := "skipped"
	sub.mu.Lock()
	for id, r := range sub.results {
		jobs[id] = map[string]any{"result": r.Result, "outputs": r.Outputs}
		switch {
		case r.Result == "failure" || r.Result == "cancelled":
			result = "failure"
		case r.Result == "success" && result == "skipped":
			result = "success"
		}
	}
	sub.mu.Unlock()
	if !ok {
		result = "failure"
	}
	out := map[string]any{}
	oenv := BaseEnv(&in)
	oenv.Contexts["jobs"] = jobs
	oenv.Contexts["inputs"] = inputs
	for name, o := range declared.Outputs {
		if o.Value == nil {
			continue
		}
		v, err := oenv.Interpolate(o.Value.Value)
		if err != nil {
			jr.pr.Note(1, "warning: output %s: %v", name, err)
			continue
		}
		out[name] = v
	}
	jr.pr.JobEnd(jr.name, result, durationOf(start))
	e.addSummary(jr.name, result, "reusable workflow")
	return &jobResult{Result: result, Outputs: out}
}

func typedDefault(d *jactionlint.WorkflowCallEventInput) any {
	v := d.Default.Value
	switch d.Type {
	case jactionlint.WorkflowCallEventInputTypeBoolean:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case jactionlint.WorkflowCallEventInputTypeNumber:
		return eval.ToNumber(v)
	}
	return v
}

func coerceInput(t jactionlint.WorkflowCallEventInputType, v any) any {
	switch t {
	case jactionlint.WorkflowCallEventInputTypeBoolean:
		if s, ok := v.(string); ok {
			return strings.EqualFold(strings.TrimSpace(s), "true")
		}
		return eval.Truthy(v)
	case jactionlint.WorkflowCallEventInputTypeNumber:
		return eval.ToNumber(v)
	}
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return eval.ToString(v)
}

// loadCalledWorkflow reads the workflow that a job calls: a file of this repository or one of another repository at a
// commit (downloaded like an action, and under the same pinning rule).
func (e *Engine) loadCalledWorkflow(ctx context.Context, uses string) (*jactionlint.Workflow, string, error) {
	ref := jactionlint.ParseUses(uses)
	var file, display string
	switch ref.Kind {
	case jactionlint.UsesLocal:
		rel := strings.TrimPrefix(ref.Path, "./")
		file = filepath.Join(e.cfg.Root, filepath.FromSlash(rel))
		if !inDir(e.cfg.Root, file) {
			return nil, "", fmt.Errorf("%s points outside the repository", uses)
		}
		display = rel
	case jactionlint.UsesReusableWorkflow:
		sha := ref.Ref
		if !reSHA.MatchString(sha) {
			if !e.cfg.TrustActions {
				return nil, "", fmt.Errorf("%s is not pinned to a commit SHA. jactionlint run only runs pinned workflows by default; pin it, or pass --trust-actions to run the commit that %q points to now", uses, ref.Ref)
			}
			var err error
			if sha, err = resolveRef(ctx, ref.Owner, ref.Repo, ref.Ref); err != nil {
				return nil, "", err
			}
		}
		state, err := StateDir()
		if err != nil {
			return nil, "", err
		}
		dir, _, err := fetchRepo(ctx, state, ref.Owner, ref.Repo, sha)
		if err != nil {
			return nil, "", err
		}
		file = filepath.Join(dir, filepath.FromSlash(ref.Subpath))
		if !inDir(dir, file) {
			return nil, "", fmt.Errorf("%s points outside the repository", uses)
		}
		display = ref.Owner + "/" + ref.Repo + "/" + ref.Subpath + "@" + shortSHA(sha)
	default:
		return nil, "", fmt.Errorf("%q is not a reusable workflow", uses)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the reusable workflow %s: %w", display, err)
	}
	w, errs := jactionlint.Parse(b)
	if w == nil || len(errs) > 0 {
		msg := "invalid workflow"
		if len(errs) > 0 {
			msg = errs[0].Error()
		}
		return nil, "", fmt.Errorf("%s: %s", display, msg)
	}
	return w, display, nil
}
