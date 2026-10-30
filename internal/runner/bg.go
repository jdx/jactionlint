package runner

import (
	"context"
	"strings"
	"time"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
)

// bgStep is a step that runs in the background (background: true, or a member of parallel:). Its output is held and
// shown when the job waits for it, so that the output of concurrent steps does not interleave.
type bgStep struct {
	n      label
	name   string
	step   *jactionlint.Step
	cancel context.CancelFunc
	done   chan struct{}
	res    stepResult
	eff    *effects
	lines  []string
	start  time.Time
	joined bool
}

func (jr *jobRun) startBackground(n label, name string, step *jactionlint.Step, ex *jactionlint.ExecRun, ev *eval.Env) *bgStep {
	ctx, cancel := context.WithCancel(jr.ctx)
	b := &bgStep{n: n, name: name, step: step, cancel: cancel, done: make(chan struct{}), start: time.Now()}
	jr.e.pr.Note(n.indent(), "[%s] %s (running in the background)", n, name)
	jr.bgOrder = append(jr.bgOrder, b)
	if step.ID != nil && step.ID.Value != "" {
		jr.bg[step.ID.Value] = b
	}
	go func() {
		defer close(b.done)
		defer cancel()
		b.res, b.eff = jr.execRun(n, step, ex, ev, func(l string) { b.lines = append(b.lines, l) }, ctx)
	}()
	return b
}

// join waits for the step and records its result in the job.
func (jr *jobRun) join(b *bgStep) {
	<-b.done
	if b.joined {
		return
	}
	b.joined = true
	jr.e.pr.StepStart(b.n, b.name)
	for _, l := range b.lines {
		jr.e.pr.StepOutput(b.n, l)
	}
	jr.finishStep(b.n, b.step, b.res, b.eff, b.start)
}

func (jr *jobRun) joinAll() {
	for _, b := range jr.bgOrder {
		if !b.joined {
			b.cancel()
			jr.join(b)
		}
	}
}

func (jr *jobRun) wait(n label, name string, ex *jactionlint.ExecWait) {
	e := jr.e
	e.pr.StepStart(n, name)
	var targets []*bgStep
	if ex.All {
		for _, b := range jr.bgOrder {
			if !b.joined {
				targets = append(targets, b)
			}
		}
	} else {
		for _, nm := range ex.Names {
			b := jr.bg[nm.Value]
			if b == nil {
				e.pr.Note(n.indent()+1, "error: no background step with the id %q", nm.Value)
				jr.failed = true
				e.pr.StepEnd(n, "failure", "unknown step", 0)
				return
			}
			targets = append(targets, b)
		}
	}
	for _, b := range targets {
		jr.join(b)
	}
	e.pr.StepEnd(n, "success", "", 0)
}

func (jr *jobRun) cancelBG(n label, name string, ex *jactionlint.ExecCancel) {
	e := jr.e
	e.pr.StepStart(n, name)
	b := jr.bg[ex.Name.Value]
	if b == nil {
		e.pr.Note(n.indent()+1, "error: no background step with the id %q", ex.Name.Value)
		jr.failed = true
		e.pr.StepEnd(n, "failure", "unknown step", 0)
		return
	}
	b.cancel()
	<-b.done
	e.pr.StepEnd(n, "success", "cancelled "+ex.Name.Value, 0)
}

func (jr *jobRun) parallel(n label, name string, step *jactionlint.Step, ex *jactionlint.ExecParallel) {
	e := jr.e
	e.pr.StepStart(n, name+" ("+itoa(len(ex.Steps))+" steps)")
	var started []*bgStep
	for _, sub := range ex.Steps {
		run, ok := sub.Exec.(*jactionlint.ExecRun)
		if !ok {
			e.pr.Note(n.indent()+1, "skipped: only run steps can be part of parallel: yet")
			continue
		}
		sn := jr.nextLabel()
		ev := jr.stepEnv(sub)
		started = append(started, jr.startBackground(sn, jr.stepName(sub, ev), sub, run, ev))
	}
	for _, b := range started {
		jr.join(b)
	}
	e.pr.StepEnd(n, "success", "", 0)
}

func itoa(n int) string { return strings.TrimSpace(eval.FormatNumber(float64(n))) }
