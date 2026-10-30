package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

// dryRunCell prints what a cell would run, with the expressions replaced, and runs nothing. Outputs of steps and jobs
// that did not run are empty, so a command that uses one shows an empty string there.
func (e *Engine) dryRunCell(ctx context.Context, pj *PlannedJob, jr *jobRun, where string) *jobResult {
	j := pj.Job
	jr.ctx = ctx
	jr.work = e.cfg.Root
	jr.baseEnv = map[string]string{}
	for k, v := range e.cfg.EnvFile {
		jr.baseEnv[k] = v
	}
	for _, env := range []*jactionlint.Env{e.wf.Env, j.Env} {
		if err := jr.addEnv(jr.baseEnv, env, jr.env); err != nil {
			jr.pr.Note(1, "error: %v", err)
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
	jr.pr.JobStart(jr.name + "  [dry run, " + where + "]")
	for _, st := range j.Steps {
		jr.dryStep(st)
	}
	jr.pr.JobEnd(jr.name, "success", 0)
	e.addSummary(jr.name, "success", "dry run")
	return &jobResult{Result: "success", Outputs: map[string]any{}}
}

func (jr *jobRun) dryStep(step *jactionlint.Step) {
	n := jr.nextLabel()
	ev := jr.stepEnv(step)
	pr := jr.pr
	m := jr.e.masker.Mask
	pr.StepStart(n, jr.stepName(step, ev))
	ind := n.indent() + 1
	if step.If != nil {
		ok, err := ev.Condition(step.If.Value)
		switch {
		case err != nil:
			pr.Note(ind, "if: %s  (cannot be evaluated: %v)", oneLine(step.If.Value), err)
		default:
			pr.Note(ind, "if: %s  => %v (as if every earlier step succeeded and no output existed)", oneLine(step.If.Value), ok)
		}
	}
	envs := map[string]string{}
	if err := jr.addEnv(envs, step.Env, ev); err == nil && len(envs) > 0 {
		keys := make([]string, 0, len(envs))
		for k := range envs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			pr.Note(ind, "env %s=%s", k, m(envs[k]))
		}
	}
	switch ex := step.Exec.(type) {
	case *jactionlint.ExecRun:
		script, err := jr.interp(ev, ex.Run.Value)
		if err != nil {
			pr.Note(ind, "error: %v", err)
			return
		}
		shell := jr.defaults.shell
		if ex.Shell != nil {
			shell = ex.Shell.Value
		}
		if spec, err := resolveShell(shell, onPath(sandbox.PathDirs(jr.pathWith(jr.parentPath())))); err == nil {
			pr.Note(ind, "shell: %s", strings.Join(spec.argv, " "))
		}
		wd := jr.defaults.workdir
		if ex.WorkingDirectory != nil {
			wd = ex.WorkingDirectory.Value
		}
		if wd, err = jr.interp(ev, wd); err == nil && wd != "" {
			pr.Note(ind, "working-directory: %s", wd)
		}
		pr.Note(ind, "run:")
		for _, l := range strings.Split(strings.TrimRight(m(script), "\n"), "\n") {
			pr.Note(ind+1, "%s", l)
		}
	case *jactionlint.ExecAction:
		pr.Note(ind, "uses: %s", ex.Uses.Value)
		keys := make([]string, 0, len(ex.Inputs))
		for k := range ex.Inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, err := jr.interp(ev, ex.Inputs[k].Value.Value)
			if err != nil {
				v = "<" + err.Error() + ">"
			}
			pr.Note(ind, "with %s=%s", ex.Inputs[k].Name.Value, m(v))
		}
	default:
		pr.Note(ind, "(not shown)")
	}
}

func (jr *jobRun) parentPath() string {
	for _, kv := range jr.parentEnv() {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			return v
		}
	}
	return ""
}

var _ = fmt.Sprint
