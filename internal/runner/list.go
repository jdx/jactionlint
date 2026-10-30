package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jdx/jactionlint/v2"
)

func runsOnText(j *jactionlint.Job) string {
	if j.RunsOn == nil {
		return ""
	}
	if j.RunsOn.LabelsExpr != nil {
		return j.RunsOn.LabelsExpr.Value
	}
	var ls []string
	for _, l := range j.RunsOn.Labels {
		ls = append(ls, l.Value)
	}
	s := strings.Join(ls, ", ")
	if j.RunsOn.Group != nil {
		s = "group " + j.RunsOn.Group.Value + " " + s
	}
	return strings.TrimSpace(s)
}

func writeText(w io.Writer, p *Plan, root string, o *options) {
	title := relPath(root, p.Path)
	if p.Workflow.Name != nil {
		title += "  (" + p.Workflow.Name.Value + ")"
	}
	fmt.Fprintf(w, "%s\n", title)
	if !listensTo(p.Workflow, o.event) {
		fmt.Fprintf(w, "  note: the workflow has no %q trigger, GitHub would not run it for that event\n", o.event)
	}
	stage := 0
	dynamic := false
	for _, pj := range p.Jobs {
		if !pj.Selected {
			continue
		}
		if pj.Dynamic != "" {
			dynamic = true
		}
		if pj.Stage != stage {
			stage = pj.Stage
			fmt.Fprintf(w, "  stage %d\n", stage)
		}
		writeJob(w, pj, o)
	}
	if dynamic {
		fmt.Fprintf(w, "  note: a matrix that depends on another job's output is expanded when its turn comes%s\n", explainRef("dynamic-matrix"))
	}
}

func writeJob(w io.Writer, pj *PlannedJob, o *options) {
	var notes []string
	if len(pj.Needs) > 0 {
		notes = append(notes, "needs: "+strings.Join(pj.Needs, ", "))
	}
	if pj.Job.WorkflowCall != nil {
		notes = append(notes, "calls: "+pj.Job.WorkflowCall.Uses.Value)
	} else if ro := runsOnText(pj.Job); ro != "" {
		notes = append(notes, "runs-on: "+ro)
	}
	if pj.Job.If != nil {
		notes = append(notes, "if: "+oneLine(pj.Job.If.Value))
	}
	suffix := ""
	if len(notes) > 0 {
		suffix = "   " + strings.Join(notes, "   ")
	}
	if pj.Dynamic != "" {
		fmt.Fprintf(w, "    %s (matrix known at run time: %s)%s\n", pj.ID, strings.TrimSpace(pj.Dynamic), suffix)
		return
	}
	if len(pj.Cells) == 0 {
		fmt.Fprintf(w, "    %s (matrix is empty, nothing runs)%s\n", pj.ID, suffix)
		return
	}
	for _, c := range pj.Cells {
		name := c.Name
		extra := ""
		if c.Display != "" && c.Display != name {
			extra = "   name: " + c.Display
		}
		if c.If != nil {
			extra += fmt.Sprintf("   => if is %v", *c.If)
		}
		line := suffix
		if c.RunsOn != "" && pj.Job.WorkflowCall == nil {
			line = strings.Replace(line, "runs-on: "+runsOnText(pj.Job), "runs-on: "+c.RunsOn, 1)
		}
		if where := placement(o, pj, c); where != "" {
			extra = "   => " + where + extra
		}
		fmt.Fprintf(w, "    %s%s%s%s\n", name, c.detail(), line, extra)
	}
}

// oneLine folds the line breaks of a multi-line condition.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func listensTo(w *jactionlint.Workflow, event string) bool {
	for _, e := range w.On {
		if strings.EqualFold(e.EventName(), event) {
			return true
		}
	}
	return false
}

type jsonJob struct {
	ID       string     `json:"id"`
	Stage    int        `json:"stage"`
	Selected bool       `json:"selected"`
	Needs    []string   `json:"needs,omitempty"`
	RunsOn   string     `json:"runs_on,omitempty"`
	Uses     string     `json:"uses,omitempty"`
	If       string     `json:"if,omitempty"`
	Dynamic  string     `json:"dynamic_matrix,omitempty"`
	Cells    []jsonCell `json:"cells"`
}

type jsonCell struct {
	Name   string         `json:"name"`
	Matrix map[string]any `json:"matrix,omitempty"`
}

type jsonPlan struct {
	Workflow string    `json:"workflow"`
	Name     string    `json:"name,omitempty"`
	Event    string    `json:"event"`
	Jobs     []jsonJob `json:"jobs"`
}

func writeJSON(w io.Writer, plans []*Plan, root string, o *options) error {
	out := make([]jsonPlan, 0, len(plans))
	for _, p := range plans {
		jp := jsonPlan{Workflow: relPath(root, p.Path), Event: o.event, Jobs: []jsonJob{}}
		if p.Workflow.Name != nil {
			jp.Name = p.Workflow.Name.Value
		}
		for _, pj := range p.Jobs {
			j := jsonJob{ID: pj.ID, Stage: pj.Stage, Selected: pj.Selected, Needs: pj.Needs, RunsOn: runsOnText(pj.Job), Dynamic: pj.Dynamic, Cells: []jsonCell{}}
			if pj.Job.WorkflowCall != nil {
				j.Uses = pj.Job.WorkflowCall.Uses.Value
			}
			if pj.Job.If != nil {
				j.If = oneLine(pj.Job.If.Value)
			}
			for _, c := range pj.Cells {
				j.Cells = append(j.Cells, jsonCell{Name: c.Name, Matrix: c.Matrix})
			}
			jp.Jobs = append(jp.Jobs, j)
		}
		out = append(out, jp)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// placement says where a cell would run with the options given: on this machine, in a container, or nowhere.
func placement(o *options, pj *PlannedJob, c *Cell) string {
	j := pj.Job
	if j.WorkflowCall != nil {
		return ""
	}
	var labels []string
	if c.RunsOn != "" {
		for _, l := range strings.Split(c.RunsOn, ",") {
			labels = append(labels, strings.TrimSpace(l))
		}
	}
	hasServices := j.Services != nil && (len(j.Services.Value) > 0 || j.Services.Expression != nil)
	target := targetOS(labels)
	switch {
	case j.Container != nil:
		img := ""
		if j.Container.Image != nil && !strings.Contains(j.Container.Image.Value, "${{") {
			img = " " + j.Container.Image.Value
		}
		return "runs in the job container" + img
	case o.ro.container != "" && !o.ro.noContainers && target == "linux":
		img, err := imageForLabels(labels, containerOverride(o), o.imageMap())
		if err != nil {
			return "skipped: " + err.Error()
		}
		return "runs in a container: " + img
	case target != "" && target != hostOS():
		why := "skipped: needs " + hostName[target]
		if target == "linux" {
			why += " (--container runs it in a Linux container)"
		}
		return why
	case hasServices:
		return "runs on this machine with its services in containers"
	}
	return "" // on this machine, which is not worth a note on every line
}

func containerOverride(o *options) string {
	if o.ro.container == "default" {
		return ""
	}
	return o.ro.container
}

func (o *options) imageMap() map[string]string {
	m := map[string]string{}
	for _, kv := range o.ro.images {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[strings.ToLower(k)] = v
		}
	}
	return m
}
