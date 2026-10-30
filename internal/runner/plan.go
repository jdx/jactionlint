// Package runner runs the jobs of a GitHub Actions workflow on the local machine.
package runner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
)

// Cell is one run of a job: a job without a matrix has one, a matrix job has one per combination.
type Cell struct {
	// Name is the name that the job gets on GitHub when it does not set name: the ID, then the scalar matrix values.
	Name string
	// Matrix holds the matrix values (nil for a job without a matrix).
	Matrix map[string]any
	// Keys is the order of the keys of Matrix.
	Keys []string
	// Display is the name of the job in the cell when the workflow sets name: and the expressions in it are known.
	Display string
	// If is the result of the job's if: when it can be decided before the run (nil when there is no if: or it reads
	// needs, steps or another context that only exists during the run).
	If *bool
	// RunsOn is the runs-on of the job in the cell with its expressions evaluated ("" when it is not known).
	RunsOn string
}

// PlannedJob is a job of the workflow with the cells it expands to.
type PlannedJob struct {
	ID    string
	Job   *jactionlint.Job
	Needs []string
	// Stage is 1 for a job without needs and one more than the deepest need otherwise: jobs of a stage can run together.
	Stage int
	// Selected is false for a job that -j left out together with everything it does not depend on.
	Selected bool
	Cells    []*Cell
	// Dynamic is the source of the matrix expression when the cells depend on data that exists only at run time.
	Dynamic string
}

// Plan is the order in which the jobs of one workflow run.
type Plan struct {
	Workflow *jactionlint.Workflow
	Path     string
	// Jobs is in dependency order; jobs of one stage keep the order of the file.
	Jobs []*PlannedJob
}

// Job returns the planned job with the ID, case-insensitively like GitHub.
func (p *Plan) Job(id string) *PlannedJob {
	for _, j := range p.Jobs {
		if strings.EqualFold(j.ID, id) {
			return j
		}
	}
	return nil
}

// BuildPlan expands the matrices, orders the jobs by needs and marks the ones that only (all jobs when empty) and
// their dependencies select.
//
// env, when not nil, resolves the expressions that only need contexts it has (a matrix built from vars or inputs,
// the name of a job); the others stay as they are written.
func BuildPlan(w *jactionlint.Workflow, path string, only []string, env *eval.Env) (*Plan, error) {
	jobs := make([]*PlannedJob, 0, len(w.Jobs))
	byID := map[string]*PlannedJob{}
	for key, j := range w.Jobs {
		id := key
		if j.ID != nil {
			id = j.ID.Value
		}
		pj := &PlannedJob{ID: id, Job: j}
		for _, n := range j.Needs {
			pj.Needs = append(pj.Needs, n.Value)
		}
		var m *jactionlint.Matrix
		if j.Strategy != nil {
			m = j.Strategy.Matrix
			if j.Strategy.Expression != nil {
				m = &jactionlint.Matrix{Expression: strategyMatrixExpr(j.Strategy.Expression)}
			}
		}
		combos, dyn, err := expandMatrix(m, envResolver(env))
		if err != nil {
			// What cannot be evaluated now (an input that was not given, a value of the wrong kind) is reported when
			// the job's turn comes, not as a failure of the whole plan.
			combos, dyn = nil, "cannot be evaluated yet: "+err.Error()
		}
		pj.Dynamic = dyn
		for _, c := range combos {
			cell := newCell(pj.ID, j, m != nil, c)
			cell.Display = displayName(env, j, cell)
			cell.If = evalIf(env, j, cell)
			cell.RunsOn = evalRunsOn(env, j, cell)
			pj.Cells = append(pj.Cells, cell)
		}
		jobs = append(jobs, pj)
		byID[strings.ToLower(id)] = pj
	}
	sort.SliceStable(jobs, func(a, b int) bool { return posLess(jobs[a].Job.Pos, jobs[b].Job.Pos) })

	for _, pj := range jobs {
		for _, n := range pj.Needs {
			if byID[strings.ToLower(n)] == nil {
				return nil, fmt.Errorf("job %q needs %q, which is not a job of the workflow", pj.ID, n)
			}
		}
	}

	// Stage by depth with cycle detection.
	state := map[*PlannedJob]int{} // 1 visiting, 2 done
	var visit func(pj *PlannedJob, trail []string) error
	visit = func(pj *PlannedJob, trail []string) error {
		switch state[pj] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("jobs form a cycle through needs: %s", strings.Join(append(trail, pj.ID), " -> "))
		}
		state[pj] = 1
		pj.Stage = 1
		for _, n := range pj.Needs {
			d := byID[strings.ToLower(n)]
			if err := visit(d, append(trail, pj.ID)); err != nil {
				return err
			}
			if d.Stage+1 > pj.Stage {
				pj.Stage = d.Stage + 1
			}
		}
		state[pj] = 2
		return nil
	}
	for _, pj := range jobs {
		if err := visit(pj, nil); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(jobs, func(a, b int) bool { return jobs[a].Stage < jobs[b].Stage })

	p := &Plan{Workflow: w, Path: path, Jobs: jobs}
	if len(only) == 0 {
		for _, pj := range jobs {
			pj.Selected = true
		}
		return p, nil
	}
	var mark func(pj *PlannedJob)
	mark = func(pj *PlannedJob) {
		if pj.Selected {
			return
		}
		pj.Selected = true
		for _, n := range pj.Needs {
			mark(byID[strings.ToLower(n)])
		}
	}
	for _, id := range only {
		pj := byID[strings.ToLower(id)]
		if pj == nil {
			ids := make([]string, 0, len(jobs))
			for _, j := range jobs {
				ids = append(ids, j.ID)
			}
			return nil, fmt.Errorf("the workflow has no job %q (jobs: %s)", id, strings.Join(ids, ", "))
		}
		mark(pj)
	}
	return p, nil
}

func newCell(id string, j *jactionlint.Job, hasMatrix bool, c *combo) *Cell {
	cell := &Cell{Name: id}
	if !hasMatrix {
		return cell
	}
	cell.Matrix, cell.Keys = c.Values, c.Keys
	if vs := c.nameValues(); len(vs) > 0 {
		cell.Name = id + " (" + strings.Join(vs, ", ") + ")"
	}
	return cell
}

// detail describes the matrix values that the name does not show (mappings and lists), so that cells with the same
// name can be told apart.
func (c *Cell) detail() string {
	var parts []string
	hidden := false
	for _, k := range c.Keys {
		if _, ok := formatScalar(c.Matrix[k]); !ok {
			hidden = true
		}
	}
	if !hidden {
		return ""
	}
	for _, k := range c.Keys {
		parts = append(parts, k+"="+valueText(c.Matrix[k]))
	}
	return "  [" + strings.Join(parts, " ") + "]"
}

// strategyMatrixExpr turns `strategy: ${{ fromJSON(x) }}` into the expression of the matrix inside it.
func strategyMatrixExpr(s *jactionlint.String) *jactionlint.String {
	inner := *s
	inner.Value = "${{ (" + strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s.Value), "${{"), "}}") + ").matrix }}"
	return &inner
}

// displayName evaluates the name: of a job in a cell. It is "" when the job has no name or an expression in it needs
// data that does not exist yet.
func displayName(env *eval.Env, j *jactionlint.Job, c *Cell) string {
	if j.Name == nil {
		return ""
	}
	if env == nil || !eval.HasExpr(j.Name.Value) {
		return j.Name.Value
	}
	e := cellEnv(env, c)
	if !e.CanEval(j.Name.Value) {
		return ""
	}
	if s, err := e.Interpolate(j.Name.Value); err == nil {
		return s
	}
	return ""
}

// cellEnv returns env with the matrix context of the cell.
func cellEnv(env *eval.Env, c *Cell) *eval.Env {
	e := &eval.Env{Contexts: map[string]any{}, Status: env.Status, HashFiles: env.HashFiles}
	for k, v := range env.Contexts {
		e.Contexts[k] = v
	}
	e.Contexts["matrix"] = orEmpty(c.Matrix)
	return e
}

func evalIf(env *eval.Env, j *jactionlint.Job, c *Cell) *bool {
	if env == nil || j.If == nil {
		return nil
	}
	e := cellEnv(env, c)
	// A condition is written with or without ${{ }}; look at the contexts of the expression either way.
	cs, err := eval.StringContexts(ifSource(j.If.Value))
	if err != nil {
		return nil
	}
	for _, n := range cs {
		known := false
		for k := range e.Contexts {
			if strings.EqualFold(k, n) {
				known = true
			}
		}
		if !known {
			return nil // reads needs, steps or another context that exists only during the run
		}
	}
	// With needs, the status of the job is not known before they ran: the answer counts only when it is the same
	// whether they succeed, one fails or the run is cancelled (failure() || true is definite, success() is not).
	statuses := []string{"success"}
	if len(j.Needs) > 0 {
		statuses = []string{"success", "failure", "cancelled"}
	}
	var result *bool
	for _, st := range statuses {
		st := st
		e.Status = func(name string) bool {
			switch name {
			case "always":
				return true
			default:
				return name == st
			}
		}
		v, err := e.Condition(j.If.Value)
		if err != nil {
			return nil
		}
		if result != nil && *result != v {
			return nil
		}
		result = &v
	}
	return result
}

func evalRunsOn(env *eval.Env, j *jactionlint.Job, c *Cell) string {
	if env == nil || j.RunsOn == nil {
		return ""
	}
	e := cellEnv(env, c)
	var parts []string
	if j.RunsOn.LabelsExpr != nil {
		if !e.CanEval(j.RunsOn.LabelsExpr.Value) {
			return ""
		}
		v, err := e.Value(j.RunsOn.LabelsExpr.Value)
		if err != nil {
			return ""
		}
		if l, ok := v.([]any); ok {
			for _, x := range l {
				parts = append(parts, eval.ToString(x))
			}
		} else {
			parts = append(parts, eval.ToString(v))
		}
		return strings.Join(parts, ", ")
	}
	for _, l := range j.RunsOn.Labels {
		if !e.CanEval(l.Value) {
			return ""
		}
		s, err := e.Interpolate(l.Value)
		if err != nil {
			return ""
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// ifSource wraps a condition that is written without ${{ }} so that its expressions can be found.
func ifSource(v string) string {
	t := strings.TrimSpace(v)
	if !eval.HasExpr(t) {
		t = "${{ " + t + " }}"
	}
	return t
}
