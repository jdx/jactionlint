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
	"go.yaml.in/yaml/v4"
)

// maxComposite is how deep composite actions may call composite actions.
const maxComposite = 10

type inputDef struct {
	Default    string
	HasDefault bool
	Required   bool
}

// actionMeta is the content of an action.yml that the runner needs.
type actionMeta struct {
	Name    string
	Inputs  map[string]inputDef
	Outputs map[string]string // composite actions: the expression of each output
	Using   string
	Main    string
	Pre     string
	Post    string
	PreIf   string
	PostIf  string
	Image   string
	Steps   []*jactionlint.Step // composite
}

type rawAction struct {
	Name   string `yaml:"name"`
	Inputs map[string]struct {
		Default  any  `yaml:"default"`
		Required bool `yaml:"required"`
	} `yaml:"inputs"`
	Outputs map[string]struct {
		Value any `yaml:"value"`
	} `yaml:"outputs"`
	Runs struct {
		Using  string `yaml:"using"`
		Main   string `yaml:"main"`
		Pre    string `yaml:"pre"`
		Post   string `yaml:"post"`
		PreIf  any    `yaml:"pre-if"`
		PostIf any    `yaml:"post-if"`
		Image  string `yaml:"image"`
	} `yaml:"runs"`
}

func scalarString(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprint(v)
	case int64:
		return fmt.Sprint(v)
	case uint64:
		return fmt.Sprint(v)
	case float64:
		return eval.FormatNumber(v)
	}
	return fmt.Sprint(v)
}

// loadActionMeta reads action.yml or action.yaml from a directory.
func loadActionMeta(dir string) (*actionMeta, error) {
	var b []byte
	var err error
	for _, name := range []string{"action.yml", "action.yaml"} {
		if b, err = os.ReadFile(filepath.Join(dir, name)); err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%s has no action.yml or action.yaml", dir)
	}
	var raw rawAction
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("action.yml: %w", err)
	}
	m := &actionMeta{
		Name: raw.Name, Inputs: map[string]inputDef{}, Outputs: map[string]string{},
		Using: raw.Runs.Using, Main: raw.Runs.Main, Pre: raw.Runs.Pre, Post: raw.Runs.Post, Image: raw.Runs.Image,
		PreIf: scalarString(raw.Runs.PreIf), PostIf: scalarString(raw.Runs.PostIf),
	}
	for k, v := range raw.Inputs {
		m.Inputs[strings.ToLower(k)] = inputDef{Default: scalarString(v.Default), HasDefault: v.Default != nil, Required: v.Required}
	}
	for k, v := range raw.Outputs {
		m.Outputs[k] = scalarString(v.Value)
	}
	if strings.EqualFold(m.Using, "composite") {
		w, errs := jactionlint.ParseAction(b)
		if w == nil || len(errs) > 0 {
			msg := "invalid composite action"
			if len(errs) > 0 {
				msg = errs[0].Message
			}
			return nil, fmt.Errorf("action.yml: %s", msg)
		}
		for _, j := range w.Jobs {
			if j.Composite {
				m.Steps = j.Steps
			}
		}
	}
	return m, nil
}

// resolvedAction is an action whose files are on disk.
type resolvedAction struct {
	dir   string // directory with action.yml
	local bool
	owner string
	repo  string
	ref   string
	sha   string
	spec  string
}

// actionScope is the state of the composite action that the steps of a scope belong to.
type actionScope struct {
	ra *resolvedAction
	id string
}

// postStep is the post: of a JavaScript action, run at the end of the job.
type postStep struct {
	n      label
	name   string
	step   *jactionlint.Step
	ra     *resolvedAction
	meta   *actionMeta
	inputs map[string]string
	state  map[string]string
	// run, when set, is the whole post step (the native cache shim); the fields above are for JavaScript actions.
	run func(ctx context.Context) (stepResult, *effects)
}

func (jr *jobRun) actionEnv() map[string]string {
	if jr.action == nil {
		return nil
	}
	env := map[string]string{"GITHUB_ACTION_PATH": jr.action.ra.dir, "GITHUB_ACTION": jr.action.id}
	if !jr.action.ra.local {
		env["GITHUB_ACTION_REPOSITORY"] = jr.action.ra.owner + "/" + jr.action.ra.repo
		env["GITHUB_ACTION_REF"] = jr.action.ra.ref
	}
	return env
}

// actionDirs are the directories with the code of downloaded actions, which steps may read and run.
func (jr *jobRun) actionDirs() []string {
	if st, err := StateDir(); err == nil {
		return []string{filepath.Join(st, "actions")}
	}
	return nil
}

// resolveAction finds the files of an action. A repository action is downloaded at its commit into the cache, and
// only when the ref is a full SHA, unless --trust-actions allows resolving a tag or branch.
func (jr *jobRun) resolveAction(ctx context.Context, ref *jactionlint.UsesRef, say func(string)) (*resolvedAction, error) {
	e := jr.e
	switch ref.Kind {
	case jactionlint.UsesLocal:
		p := ref.Path
		if p == "" {
			p = strings.TrimPrefix(ref.Raw, "./")
		}
		dir := filepath.Join(jr.work, filepath.FromSlash(strings.TrimPrefix(p, "./")))
		if !inDir(jr.work, dir) {
			return nil, fmt.Errorf("%s points outside the repository", ref.Raw)
		}
		return &resolvedAction{dir: dir, local: true, spec: ref.Raw}, nil
	case jactionlint.UsesAction:
		sha := ref.Ref
		if !reSHA.MatchString(ref.Ref) {
			if !e.cfg.TrustActions {
				return nil, fmt.Errorf("%s is not pinned to a commit SHA. jactionlint run executes the code of an action, so it only runs pinned actions by default. Pin it to a full commit SHA, or pass --trust-actions to run the commit that %q points to now", ref.Raw, ref.Ref)
			}
			var err error
			if sha, err = resolveRef(ctx, ref.Owner, ref.Repo, ref.Ref); err != nil {
				return nil, err
			}
			say(fmt.Sprintf("%s/%s@%s is %s", ref.Owner, ref.Repo, ref.Ref, sha))
		}
		state, err := StateDir()
		if err != nil {
			return nil, err
		}
		dir, cached, err := fetchRepo(ctx, state, ref.Owner, ref.Repo, sha)
		if err != nil {
			return nil, err
		}
		if !cached {
			say(fmt.Sprintf("downloaded %s/%s@%s", ref.Owner, ref.Repo, sha[:12]))
		}
		if ref.Subpath != "" {
			dir = filepath.Join(dir, filepath.FromSlash(ref.Subpath))
			if !inDir(filepath.Dir(dir), dir) {
				return nil, fmt.Errorf("%s points outside the repository", ref.Raw)
			}
		}
		return &resolvedAction{dir: dir, owner: ref.Owner, repo: ref.Repo, ref: ref.Ref, sha: sha, spec: ref.Raw}, nil
	}
	return nil, fmt.Errorf("%s is not an action reference", ref.Raw)
}

// runAction runs a `uses:` step.
func (jr *jobRun) runAction(n label, name string, step *jactionlint.Step, ex *jactionlint.ExecAction, ev *eval.Env) {
	e := jr.e
	e.pr.StepStart(n, name)
	start := time.Now()
	ind := n.indent() + 1
	say := func(s string) { e.pr.Note(ind, "%s", s) }
	emit := func(l string) { e.pr.StepOutput(n, l) }

	fail := func(note string) {
		emit("error: " + note)
		jr.finishStep(n, step, stepResult{Outcome: "failure", Note: note}, nil, start)
	}
	skip := func(why string) {
		e.pr.StepEnd(n, "skipped", why, 0)
		jr.recordStep(step, stepResult{Outcome: "skipped", Conclusion: "skipped"})
	}

	ref := jactionlint.ParseUses(ex.Uses.Value)
	if ref.Kind == jactionlint.UsesInvalid {
		fail(fmt.Sprintf("invalid uses %q: %s", ex.Uses.Value, ref.Problem))
		return
	}
	if ref.Kind == jactionlint.UsesDocker {
		skip("docker actions need a container engine, which is not supported yet")
		return
	}
	if ref.Kind == jactionlint.UsesReusableWorkflow {
		fail("a reusable workflow can only be used as a job (uses: at the job level)")
		return
	}

	// with:
	inputs := map[string]string{}
	names := make([]*jactionlint.Input, 0, len(ex.Inputs))
	for _, in := range ex.Inputs {
		names = append(names, in)
	}
	sort.Slice(names, func(a, b int) bool { return posLess(names[a].Name.Pos, names[b].Name.Pos) })
	for _, in := range names {
		v, err := jr.interp(ev, in.Value.Value)
		if err != nil {
			fail(err.Error())
			return
		}
		inputs[strings.ToLower(in.Name.Value)] = v
	}

	ctx := jr.ctx
	// shims for the actions that a local run replaces
	if ref.Kind == jactionlint.UsesAction {
		if sh := e.shimFor(ref); sh != nil {
			if res, eff, handled := sh(jr, n, step, inputs, emit, say); handled {
				jr.finishStep(n, step, res, eff, start)
				return
			}
		}
	}

	ra, err := jr.resolveAction(ctx, ref, say)
	if err != nil {
		fail(err.Error())
		return
	}
	meta, err := loadActionMeta(ra.dir)
	if err != nil {
		fail(err.Error())
		return
	}
	for k, d := range meta.Inputs {
		if _, ok := inputs[k]; !ok && d.HasDefault {
			dv, err := jr.interp(ev, d.Default)
			if err != nil {
				fail(err.Error())
				return
			}
			inputs[k] = dv
		}
	}

	id := "__" + strings.ReplaceAll(strings.ReplaceAll(ra.spec, "/", "_"), "@", "_")
	if step.ID != nil && step.ID.Value != "" {
		id = step.ID.Value
	}

	var res stepResult
	var eff *effects
	switch {
	case strings.EqualFold(meta.Using, "composite"):
		res, eff = jr.runComposite(n, step, ra, meta, inputs, id, ev)
	case strings.HasPrefix(strings.ToLower(meta.Using), "node"):
		res, eff = jr.runNode(n, step, ra, meta, inputs, id, ev, emit, say, ctx)
	case strings.EqualFold(meta.Using, "docker"):
		skip("docker actions need a container engine, which is not supported yet")
		return
	default:
		fail(fmt.Sprintf("runs.using %q is not supported", meta.Using))
		return
	}
	jr.finishStep(n, step, res, eff, start)
}

// inputEnv converts inputs to the INPUT_* variables of a JavaScript action.
func inputEnv(inputs map[string]string) map[string]string {
	env := map[string]string{}
	for k, v := range inputs {
		env["INPUT_"+strings.ToUpper(strings.ReplaceAll(k, " ", "_"))] = v
	}
	return env
}

func (jr *jobRun) runNode(n label, step *jactionlint.Step, ra *resolvedAction, meta *actionMeta, inputs map[string]string, id string, ev *eval.Env, emit func(string), say func(string), ctx context.Context) (stepResult, *effects) {
	e := jr.e
	major, _ := majorOf(meta.Using)
	scope := &actionScope{ra: ra, id: id}
	merged := &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}

	phase := func(script string, state map[string]string, lab label, em func(string), cx context.Context) (stepResult, *effects) {
		extra := inputEnv(inputs)
		for k, v := range (&jobRun{jobShared: jr.jobShared, action: scope}).actionEnv() {
			extra[k] = v
		}
		for k, v := range state {
			extra["STATE_"+k] = v
		}
		pr, err := jr.prepareEnv(lab, step, ev, extra)
		if err != nil {
			return stepResult{Outcome: "failure", Note: err.Error()}, &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}
		}
		bin, err := e.node.ensure(cx, major, sandboxPathDirs(pr.finalPath), say)
		if err != nil {
			em("error: " + err.Error())
			return stepResult{Outcome: "failure", Note: err.Error()}, &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}
		}
		file := filepath.Join(ra.dir, filepath.FromSlash(script))
		if !inDir(ra.dir, file) {
			return stepResult{Outcome: "failure", Note: "script outside the action"}, &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}
		}
		return jr.launch(lab, step, pr, []string{bin, file}, jr.work, append([]string{ra.dir}, probeFiles(ra.dir)...), em, cx)
	}

	state := map[string]string{}
	run := func(script, condSrc, defCond string, lab label, em func(string), cx context.Context) (stepResult, bool) {
		if script == "" {
			return stepResult{Outcome: "success"}, false
		}
		cond := condSrc
		if cond == "" {
			cond = defCond
		}
		// pre-if and post-if default to always(); an explicit one is a normal condition
		if ok, err := jr.stepEnv(step).Condition(cond); err != nil || !ok {
			return stepResult{Outcome: "success"}, false
		}
		res, eff := phase(script, state, lab, em, cx)
		for k, v := range eff.state {
			state[k] = v
		}
		jr.apply(eff, step)
		eff.report(e.pr, lab.indent()+1)
		for k, v := range eff.outputs {
			merged.outputs[k] = v
		}
		merged.warnings = append(merged.warnings, nil...)
		if eff.hint != "" {
			merged.hint = eff.hint
		}
		return res, true
	}

	if meta.Pre != "" {
		if res, ran := run(meta.Pre, meta.PreIf, "always()", n, emit, ctx); ran && res.Outcome != "success" {
			return res, merged
		}
	}
	if meta.Main == "" {
		return stepResult{Outcome: "failure", Note: "the action has no runs.main"}, merged
	}
	res, _ := run(meta.Main, "", "always()", n, emit, ctx)
	// the post step is scheduled whether or not main succeeded, as the runner does, when main started
	if meta.Post != "" {
		jr.posts = append(jr.posts, &postStep{n: label("post " + string(n)), name: "Post " + actionLabel(step, ra), step: step, ra: ra, meta: meta, inputs: inputs, state: state})
	}
	return res, merged
}

func actionLabel(step *jactionlint.Step, ra *resolvedAction) string {
	if step.Name != nil && step.Name.Value != "" {
		return step.Name.Value
	}
	return ra.spec
}

func sandboxPathDirs(p string) []string {
	var out []string
	for _, d := range strings.Split(p, string(os.PathListSeparator)) {
		if d != "" && filepath.IsAbs(d) {
			out = append(out, d)
		}
	}
	return out
}

// runPosts runs the post steps of the actions of the job, last first.
func (jr *jobRun) runPosts() {
	e := jr.e
	for i := len(jr.posts) - 1; i >= 0; i-- {
		p := jr.posts[i]
		cond := ""
		if p.meta != nil {
			cond = p.meta.PostIf
		}
		if cond == "" {
			cond = "always()"
		}
		if ok, err := jr.stepEnv(p.step).Condition(cond); err != nil || !ok {
			continue
		}
		if p.run != nil {
			e.pr.StepStart(p.n, p.name)
			pstart := time.Now()
			rctx := jr.ctx
			if jr.ctx.Err() != nil {
				rctx = context.WithoutCancel(jr.ctx)
			}
			res, eff := p.run(rctx)
			if eff != nil {
				eff.report(e.pr, p.n.indent()+1)
			}
			if res.Outcome == "failure" {
				jr.failed = true
			}
			e.pr.StepEnd(p.n, res.Outcome, res.Note, durationOf(pstart))
			continue
		}
		ev := jr.stepEnv(p.step)
		e.pr.StepStart(p.n, p.name)
		start := time.Now()
		runCtx := jr.ctx
		if jr.ctx.Err() != nil {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(context.WithoutCancel(jr.ctx), cancelGrace)
			defer cancel()
		}
		emit := func(l string) { e.pr.StepOutput(p.n, l) }
		scope := &actionScope{ra: p.ra, id: "post"}
		extra := inputEnv(p.inputs)
		for k, v := range (&jobRun{jobShared: jr.jobShared, action: scope}).actionEnv() {
			extra[k] = v
		}
		for k, v := range p.state {
			extra["STATE_"+k] = v
		}
		pr, err := jr.prepareEnv(p.n, p.step, ev, extra)
		var res stepResult
		var eff *effects
		if err != nil {
			res = stepResult{Outcome: "failure", Note: err.Error()}
		} else {
			major, _ := majorOf(p.meta.Using)
			bin, nerr := e.node.ensure(runCtx, major, sandboxPathDirs(pr.finalPath), func(s string) { e.pr.Note(p.n.indent()+1, "%s", s) })
			if nerr != nil {
				res = stepResult{Outcome: "failure", Note: nerr.Error()}
			} else {
				res, eff = jr.launch(p.n, p.step, pr, []string{bin, filepath.Join(p.ra.dir, filepath.FromSlash(p.meta.Post))}, jr.work, []string{p.ra.dir}, emit, runCtx)
			}
		}
		if eff != nil {
			jr.apply(eff, p.step)
			eff.report(e.pr, p.n.indent()+1)
		}
		res.Conclusion = res.Outcome
		if res.Outcome == "failure" {
			jr.failed = true
		}
		e.pr.StepEnd(p.n, res.Outcome, res.Note, durationOf(start))
	}
}

// runComposite runs the steps of a composite action in a scope of their own: `inputs` and `steps` are the action's.
func (jr *jobRun) runComposite(n label, step *jactionlint.Step, ra *resolvedAction, meta *actionMeta, inputs map[string]string, id string, ev *eval.Env) (stepResult, *effects) {
	e := jr.e
	eff := &effects{env: map[string]string{}, outputs: map[string]string{}, state: map[string]string{}}
	if jr.depth >= maxComposite {
		return stepResult{Outcome: "failure", Note: "composite actions are nested too deeply"}, eff
	}

	child := *jr
	child.depth = jr.depth + 1
	child.steps = map[string]any{}
	child.stepNo = 0
	child.prefix = string(n)
	child.bg = map[string]*bgStep{}
	child.bgOrder = nil
	child.defaults.shell, child.defaults.workdir = "", ""
	child.action = &actionScope{ra: ra, id: id}
	child.env = &eval.Env{Contexts: map[string]any{}, Status: jr.env.Status, HashFiles: jr.env.HashFiles}
	for k, v := range jr.env.Contexts {
		child.env.Contexts[k] = v
	}
	in := map[string]any{}
	for k, v := range inputs {
		in[k] = v
	}
	child.env.Contexts["inputs"] = in
	child.env.Contexts["steps"] = child.steps
	gh := map[string]any{}
	for k, v := range jr.env.Contexts["github"].(map[string]any) {
		gh[k] = v
	}
	gh["action_path"] = ra.dir
	if !ra.local {
		gh["action_repository"] = ra.owner + "/" + ra.repo
		gh["action_ref"] = ra.ref
	}
	child.env.Contexts["github"] = gh
	// the env of the uses step is the env of the inner steps
	child.baseEnv = map[string]string{}
	for k, v := range jr.baseEnv {
		child.baseEnv[k] = v
	}
	stepEnv := map[string]string{}
	if err := jr.addEnv(stepEnv, step.Env, ev); err == nil {
		for k, v := range stepEnv {
			child.baseEnv[k] = v
		}
	}

	prevFailed := jr.failed
	for _, st := range meta.Steps {
		child.runStepEntry(st)
	}
	child.joinAll()
	innerFailed := jr.failed && !prevFailed
	jr.failed = prevFailed

	// outputs: the expressions see the steps and the inputs of the action
	oenv := child.stepEnv(step)
	for name, expr := range meta.Outputs {
		v, err := child.interp(oenv, expr)
		if err != nil {
			eff.warn("output %s: %v", name, err)
			continue
		}
		eff.outputs[name] = v
	}
	_ = e
	if innerFailed {
		return stepResult{Outcome: "failure", Note: "a step of the action failed"}, eff
	}
	return stepResult{Outcome: "success"}, eff
}
