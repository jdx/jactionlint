// Package runner runs the jobs of a GitHub Actions workflow on the local machine.
package runner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2"
)

// Cell is one run of a job: a job without a matrix has one, a matrix job has one per combination.
type Cell struct {
	// Name is the name that the job gets on GitHub when it does not set name: the ID, then the scalar matrix values.
	Name string
	// Matrix holds the matrix values (nil for a job without a matrix).
	Matrix map[string]any
	// Keys is the order of the keys of Matrix.
	Keys []string
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
func BuildPlan(w *jactionlint.Workflow, path string, only []string) (*Plan, error) {
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
				pj.Dynamic = j.Strategy.Expression.Value
			}
		}
		if pj.Dynamic == "" {
			combos, dyn := expandMatrix(m)
			pj.Dynamic = dyn
			for _, c := range combos {
				pj.Cells = append(pj.Cells, newCell(pj.ID, j, m != nil, c))
			}
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
