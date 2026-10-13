package jactionlint

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// This file exports the facts the rules reason about, as one versioned document (--export-facts): the events,
// permissions, concurrency, calls between workflows and actions, and the value of each input, with the position in
// the source and an explicit word wherever a value is not known (a ${{ }} expression decides it, or the repository,
// or nobody in this repository calls the workflow). It is for tools that apply their own policy to the same model
// as jactionlint (Conftest, Lintnet, a script), without parsing the YAML and the semantics of GitHub Actions again.
//
// The document says what a file declares and what the call graph of the repository knows. It does not say what a
// run does: a fact that depends on the run is "unknown", never a guess. The facts are not findings, so the
// configuration, the profile and the ignores do not change them.

// FactsVersion is the version of the facts document. A field is not removed and does not change its meaning within a
// version; fields can be added, so a reader ignores the ones it does not know. The version changes only when a field
// changes meaning.
const FactsVersion = 1

// Facts is the document of --export-facts.
type Facts struct {
	Version     int    `json:"version"`
	Tool        string `json:"tool"`
	ToolVersion string `json:"tool_version"`
	// Workflows are the workflow files that were exported, in the order of their paths.
	Workflows []*WorkflowFacts `json:"workflows"`
	// Actions are the action.yml files that were exported.
	Actions []*ActionFacts `json:"actions"`
	// Calls are the local calls of the exported files: a job that calls a reusable workflow of the repository, and a
	// step that runs a local action. The callee need not be in Workflows or Actions.
	Calls []*CallFacts `json:"calls"`
}

// Location is a position in a file: 1-based, the column counted in Unicode code points.
type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// StringFact is a string of the file. Dynamic is true when it contains a ${{ }} expression, so its value is known
// only when the workflow runs: Value is then the text as written.
type StringFact struct {
	Value    string    `json:"value"`
	Dynamic  bool      `json:"dynamic,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// NumberFact is a number of the file, or an expression (Dynamic) that decides it.
type NumberFact struct {
	// Value is the number. It is 0 when Dynamic is true.
	Value    float64   `json:"value"`
	Dynamic  bool      `json:"dynamic,omitempty"`
	Expr     string    `json:"expression,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// InputFact is one input of a call: a key of "with:" of a step or of a job that calls a reusable workflow.
type InputFact struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Dynamic  bool      `json:"dynamic,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// EventFacts is an event of "on:".
type EventFacts struct {
	Name     string    `json:"name"`
	Location *Location `json:"location,omitempty"`
	// Types are the activity types of the event. Empty means the default types of the event.
	Types []string `json:"types,omitempty"`
	// Filters are the branch, tag and path filters by their key ("branches", "paths-ignore", ...).
	Filters map[string][]string `json:"filters,omitempty"`
	// Schedules are the cron expressions of the event "schedule".
	Schedules []string `json:"schedules,omitempty"`
	// Inputs are the names of the inputs of workflow_dispatch and workflow_call.
	Inputs []string `json:"inputs,omitempty"`
}

// PermissionFacts are the permissions a file declares for the token.
type PermissionFacts struct {
	// Declared is false when the file has no "permissions:" at this level.
	Declared bool `json:"declared"`
	// Scopes maps a scope to "read", "write" or "none", or to "unknown" when an expression decides it. The scope "*"
	// is read-all or write-all, and "none" for an empty mapping.
	Scopes   map[string]string `json:"scopes,omitempty"`
	Location *Location         `json:"location,omitempty"`
}

// EffectivePermissions are the permissions the token of a job has.
type EffectivePermissions struct {
	// Source is "job" (the job declares them), "workflow" (it inherits those of the workflow) or "repository-setting":
	// neither declares them, so the default token of the repository applies, which the file does not say. Scopes is empty
	// then, and that means unknown, not none.
	Source string            `json:"source"`
	Scopes map[string]string `json:"scopes,omitempty"`
}

// ConcurrencyFacts is a "concurrency:" of a workflow or a job.
type ConcurrencyFacts struct {
	// Form is "mapping" (group and cancel-in-progress), "group" (only a group name, which cannot cancel) or
	// "expression" (the whole key is an expression, so the group and the flag are unknown).
	Form  string      `json:"form"`
	Group *StringFact `json:"group,omitempty"`
	// CancelInProgress is "true", "false", "unset" (not written: nothing is cancelled) or "unknown" (an expression).
	CancelInProgress string `json:"cancel_in_progress"`
	// Queue is the queue strategy, when written.
	Queue    string    `json:"queue,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// WorkflowFacts are the facts of one workflow file.
type WorkflowFacts struct {
	// Path is relative to the root of the repository, with "/" as the separator (relative to the working directory when
	// the file belongs to no repository).
	Path string `json:"path"`
	// Parsed is false when the file is not valid YAML. It has no other fact then.
	Parsed bool `json:"parsed"`
	// SyntaxErrors counts the syntax errors of the file. The facts are what could be read.
	SyntaxErrors int           `json:"syntax_errors,omitempty"`
	Name         *StringFact   `json:"name,omitempty"`
	Events       []*EventFacts `json:"events"`
	// Callable is true when the workflow has the event workflow_call.
	Callable bool `json:"callable"`
	// CalledBy says who calls a callable workflow in this repository. It is absent for a workflow that is not callable
	// and when the file belongs to no repository.
	CalledBy    *CalledByFacts    `json:"called_by,omitempty"`
	Permissions PermissionFacts   `json:"permissions"`
	Concurrency *ConcurrencyFacts `json:"concurrency,omitempty"`
	CacheMode   *StringFact       `json:"cache_mode,omitempty"`
	Jobs        []*JobFacts       `json:"jobs"`
}

// CalledByFacts are the local callers of a reusable workflow.
type CalledByFacts struct {
	// Callers are the files that call it directly, sorted: workflows and composite actions of the repository.
	Callers []string `json:"callers"`
	// Events are the events of the workflows that reach it, directly or through other reusable workflows, without
	// workflow_call. Sorted.
	Events []string `json:"events"`
	// Known is true when every chain of calls ends in a workflow of this repository with events of its own. It is false
	// when nothing in the repository calls the workflow, or a caller is itself only called: another repository may then
	// call it on any event, and Events is not all of them.
	Known bool `json:"known"`
}

// JobFacts are the facts of one job.
type JobFacts struct {
	ID       string      `json:"id"`
	Location *Location   `json:"location,omitempty"`
	Name     *StringFact `json:"name,omitempty"`
	Needs    []string    `json:"needs,omitempty"`
	// RunsOn are the labels of runs-on. DynamicRunsOn is true when an expression decides them.
	RunsOn        []string `json:"runs_on,omitempty"`
	DynamicRunsOn bool     `json:"runs_on_dynamic,omitempty"`
	// Permissions are those the job declares; EffectivePermissions are those its token has.
	Permissions          PermissionFacts      `json:"permissions"`
	EffectivePermissions EffectivePermissions `json:"effective_permissions"`
	Concurrency          *ConcurrencyFacts    `json:"concurrency,omitempty"`
	Environment          *StringFact          `json:"environment,omitempty"`
	TimeoutMinutes       *NumberFact          `json:"timeout_minutes,omitempty"`
	CacheMode            *StringFact          `json:"cache_mode,omitempty"`
	// If is the condition as written, without "${{ }}". It is not evaluated.
	If *string `json:"if,omitempty"`
	// Uses is set when the job calls a reusable workflow.
	Uses  *JobUsesFacts `json:"uses,omitempty"`
	Steps []*StepFacts  `json:"steps,omitempty"`
}

// JobUsesFacts is a job that calls a reusable workflow.
type JobUsesFacts struct {
	UsesFacts
	// SecretsInherit is true for "secrets: inherit".
	SecretsInherit bool `json:"secrets_inherit"`
	// Secrets are the names of the secrets that the call passes by name.
	Secrets []string    `json:"secrets,omitempty"`
	Inputs  []InputFact `json:"inputs,omitempty"`
}

// UsesFacts is a "uses:" value.
type UsesFacts struct {
	Raw string `json:"uses"`
	// Kind is "action", "reusable-workflow", "docker", "local" or "invalid".
	Kind string `json:"kind"`
	// Dynamic is true when the value has an expression.
	Dynamic bool `json:"dynamic,omitempty"`
	// Repository is "owner/repo" of an action or a reusable workflow of another repository.
	Repository string `json:"repository,omitempty"`
	// Ref and RefKind are what follows "@". RefKind is "full-sha", "short-sha", "semver-tag", "other-tag-or-branch",
	// "digest" or "none".
	Ref     string `json:"ref,omitempty"`
	RefKind string `json:"ref_kind,omitempty"`
	// Path is the canonical "./path" of a local action or workflow.
	Path string `json:"path,omitempty"`
}

// StepFacts is a step. A step of a "parallel:" group is listed after the group, in the order of the source.
type StepFacts struct {
	// Index is the position of the step in the job, from 0.
	Index    int         `json:"index"`
	Location *Location   `json:"location,omitempty"`
	ID       string      `json:"id,omitempty"`
	Name     *StringFact `json:"name,omitempty"`
	// Kind is "run", "action" or "parallel".
	Kind string  `json:"kind"`
	If   *string `json:"if,omitempty"`
	// Shell is the shell of a run step, when written.
	Shell *StringFact `json:"shell,omitempty"`
	Uses  *UsesFacts  `json:"uses,omitempty"`
	// Inputs are the keys of "with:" of an action step. InputsDynamic is true when the whole section is an expression,
	// so the list is not all of them.
	Inputs        []InputFact `json:"inputs,omitempty"`
	InputsDynamic bool        `json:"inputs_dynamic,omitempty"`
}

// ActionFacts are the facts of an action.yml.
type ActionFacts struct {
	// Path is the file, relative to the root of the repository with "/" as the separator.
	Path   string `json:"path"`
	Parsed bool   `json:"parsed"`
	// SyntaxErrors counts the syntax errors of the file.
	SyntaxErrors int         `json:"syntax_errors,omitempty"`
	Name         *StringFact `json:"name,omitempty"`
	// Using is the runtime: "composite", "docker" or "node20" and the like.
	Using  string   `json:"using,omitempty"`
	Inputs []string `json:"inputs,omitempty"`
	// CalledBy says which workflows of the repository run it. It is absent when the file belongs to no repository.
	CalledBy *ActionCalledByFacts `json:"called_by,omitempty"`
	// Steps are the steps of a composite action.
	Steps []*StepFacts `json:"steps,omitempty"`
}

// ActionCalledByFacts are the local workflows that run an action.
type ActionCalledByFacts struct {
	// Known is false when no workflow of the repository runs the action: it may be published for other repositories, and
	// its events are unknown.
	Known   bool                 `json:"known"`
	Callers []*ActionCallerFacts `json:"callers"`
}

// ActionCallerFacts is one workflow that runs an action.
type ActionCallerFacts struct {
	Workflow string `json:"workflow"`
	// Via are the local actions and reusable workflows between the workflow and the action, outermost first. It is
	// empty when a step of the workflow runs the action.
	Via []string `json:"via,omitempty"`
	// Events are the events of the workflow, without workflow_call. Empty means they are not known.
	Events []string `json:"events"`
	// Inputs are what the steps of the workflow pass in "with:". They are given only when a step of the workflow runs the
	// action itself (Via is empty).
	Inputs []InputFact `json:"inputs,omitempty"`
}

// CallFacts is a call of a local reusable workflow or action.
type CallFacts struct {
	// Kind is "reusable-workflow" or "action".
	Kind string `json:"kind"`
	// From is the file with the call, Job the job, and Step the index of the step (absent for a job that calls a workflow).
	From     string    `json:"from"`
	Job      string    `json:"job,omitempty"`
	Step     *int      `json:"step,omitempty"`
	Location *Location `json:"location,omitempty"`
	// To is the file or the directory of the callee, relative to the root of the repository.
	To string `json:"to"`
	// Permissions are the permissions of the token of the calling job: what the callee can have at most.
	Permissions *EffectivePermissions `json:"permissions,omitempty"`
	// Events are the events of the calling workflow (without workflow_call). Empty means that no event of its own is
	// known.
	Events []string `json:"events"`
}

func locationOf(p *Pos) *Location {
	if p == nil || p.Line <= 0 {
		return nil
	}
	return &Location{Line: p.Line, Column: p.Col}
}

func stringFact(s *String) *StringFact {
	if s == nil {
		return nil
	}
	return &StringFact{Value: s.Value, Dynamic: s.ContainsExpression(), Location: locationOf(s.Pos)}
}

func stringValues(ss []*String) []string {
	var ret []string
	for _, s := range ss {
		if s != nil {
			ret = append(ret, s.Value)
		}
	}
	return ret
}

func permissionFacts(p *Permissions) PermissionFacts {
	if p == nil {
		return PermissionFacts{}
	}
	return PermissionFacts{Declared: true, Scopes: permissionMap(p), Location: locationOf(p.Pos)}
}

func effectiveFacts(w, j PermissionFacts) EffectivePermissions {
	switch {
	case j.Declared:
		return EffectivePermissions{Source: "job", Scopes: j.Scopes}
	case w.Declared:
		return EffectivePermissions{Source: "workflow", Scopes: w.Scopes}
	}
	return EffectivePermissions{Source: "repository-setting"}
}

func concurrencyFacts(c *Concurrency) *ConcurrencyFacts {
	if c == nil {
		return nil
	}
	f := &ConcurrencyFacts{Form: "mapping", Group: stringFact(c.Group), CancelInProgress: "unset", Location: locationOf(c.Pos)}
	switch {
	case c.Expression != nil:
		f.Form = "expression"
		f.CancelInProgress = "unknown"
	case c.Bare:
		f.Form = "group"
	}
	if c.CancelInProgress != nil {
		switch {
		case c.CancelInProgress.Expression != nil:
			f.CancelInProgress = "unknown"
		case c.CancelInProgress.Value:
			f.CancelInProgress = "true"
		default:
			f.CancelInProgress = "false"
		}
	}
	if c.Queue != nil {
		f.Queue = c.Queue.Value
	}
	return f
}

func eventFacts(e Event) *EventFacts {
	f := &EventFacts{Name: e.EventName()}
	switch ev := e.(type) {
	case *WebhookEvent:
		f.Location = locationOf(ev.Pos)
		f.Types = stringValues(ev.Types)
		for _, flt := range []*WebhookEventFilter{ev.Branches, ev.BranchesIgnore, ev.Tags, ev.TagsIgnore, ev.Paths, ev.PathsIgnore} {
			if flt == nil || flt.Name == nil {
				continue
			}
			if f.Filters == nil {
				f.Filters = map[string][]string{}
			}
			f.Filters[flt.Name.Value] = stringValues(flt.Values)
		}
		if len(ev.Workflows) > 0 {
			if f.Filters == nil {
				f.Filters = map[string][]string{}
			}
			f.Filters["workflows"] = stringValues(ev.Workflows)
		}
	case *ScheduledEvent:
		f.Location = locationOf(ev.Pos)
		for _, s := range ev.Schedules {
			if s != nil && s.Cron != nil {
				f.Schedules = append(f.Schedules, s.Cron.Value)
			}
		}
	case *WorkflowDispatchEvent:
		f.Location = locationOf(ev.Pos)
		for name := range ev.Inputs {
			f.Inputs = append(f.Inputs, name)
		}
		sort.Strings(f.Inputs)
	case *RepositoryDispatchEvent:
		f.Location = locationOf(ev.Pos)
		f.Types = stringValues(ev.Types)
	case *WorkflowCallEvent:
		f.Location = locationOf(ev.Pos)
		for _, in := range ev.Inputs {
			if in != nil && in.Name != nil {
				f.Inputs = append(f.Inputs, in.Name.Value)
			}
		}
	}
	return f
}

func eventNames(evs []Event) []string {
	var ret []string
	for _, e := range evs {
		if e == nil {
			continue
		}
		if n := e.EventName(); n != "" && n != "workflow_call" && !slices.Contains(ret, n) {
			ret = append(ret, n)
		}
	}
	sort.Strings(ret)
	return ret
}

func usesFacts(raw string) UsesFacts {
	u := ParseUses(raw)
	f := UsesFacts{Raw: raw, Kind: u.Kind.String(), Dynamic: u.Dynamic}
	switch u.Kind {
	case UsesAction, UsesReusableWorkflow:
		f.Repository = u.Owner + "/" + u.Repo
		f.Ref = u.Ref
		f.RefKind = u.RefKind.String()
	case UsesLocal:
		f.Path = u.Path
	case UsesDocker:
		f.Ref = u.Digest
		f.RefKind = u.RefKind.String()
	}
	return f
}

func inputFacts(m map[string]*Input) []InputFact {
	var ret []InputFact
	for name, in := range m {
		if in == nil || in.Value == nil {
			continue
		}
		if in.Name != nil {
			name = in.Name.Value
		}
		ret = append(ret, InputFact{Name: name, Value: in.Value.Value, Dynamic: in.Value.ContainsExpression(), Location: locationOf(in.Value.Pos)})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Name < ret[j].Name })
	return ret
}

func ifFact(s *String) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(s.Value)
	if isExprAssigned(v) {
		v = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v, "${{"), "}}"))
	}
	return &v
}

func stepFacts(steps []*Step) []*StepFacts {
	var ret []*StepFacts
	walkSteps(steps, func(s *Step) {
		f := &StepFacts{Index: len(ret), Location: locationOf(s.Pos), If: ifFact(s.If), Name: stringFact(s.Name)}
		if s.ID != nil {
			f.ID = s.ID.Value
		}
		switch e := s.Exec.(type) {
		case *ExecRun:
			f.Kind = "run"
			f.Shell = stringFact(e.Shell)
		case *ExecAction:
			f.Kind = "action"
			if e.Uses != nil {
				u := usesFacts(e.Uses.Value)
				f.Uses = &u
			}
			f.Inputs = inputFacts(e.Inputs)
			f.InputsDynamic = e.InputsExpression != nil || len(e.InputInserts) > 0
		case *ExecParallel:
			f.Kind = "parallel"
		}
		ret = append(ret, f)
	})
	return ret
}

func jobFacts(j *Job, wf PermissionFacts) *JobFacts {
	f := &JobFacts{
		Location:    locationOf(j.Pos),
		Name:        stringFact(j.Name),
		Permissions: permissionFacts(j.Permissions),
		Concurrency: concurrencyFacts(j.Concurrency),
		CacheMode:   stringFact(j.CacheMode),
		If:          ifFact(j.If),
		Steps:       stepFacts(j.Steps),
	}
	if j.ID != nil {
		f.ID = j.ID.Value
	}
	f.EffectivePermissions = effectiveFacts(wf, f.Permissions)
	f.Needs = stringValues(j.Needs)
	if j.Environment != nil {
		switch {
		case j.Environment.Expression != nil:
			f.Environment = stringFact(j.Environment.Expression)
		default:
			f.Environment = stringFact(j.Environment.Name)
		}
	}
	if j.TimeoutMinutes != nil {
		n := &NumberFact{Location: locationOf(j.TimeoutMinutes.Pos)}
		if j.TimeoutMinutes.Expression != nil {
			n.Dynamic, n.Expr = true, j.TimeoutMinutes.Expression.Value
		} else {
			n.Value = j.TimeoutMinutes.Value
		}
		f.TimeoutMinutes = n
	}
	if r := j.RunsOn; r != nil {
		switch {
		case r.LabelsExpr != nil:
			f.DynamicRunsOn = true
		default:
			for _, l := range r.Labels {
				if l == nil {
					continue
				}
				if l.ContainsExpression() {
					f.DynamicRunsOn = true
				}
				f.RunsOn = append(f.RunsOn, l.Value)
			}
		}
	}
	if wc := j.WorkflowCall; wc != nil && wc.Uses != nil {
		u := &JobUsesFacts{UsesFacts: usesFacts(wc.Uses.Value), SecretsInherit: wc.InheritSecrets}
		for _, in := range wc.Inputs {
			if in != nil && in.Name != nil && in.Value != nil {
				u.Inputs = append(u.Inputs, InputFact{Name: in.Name.Value, Value: in.Value.Value, Dynamic: in.Value.ContainsExpression(), Location: locationOf(in.Value.Pos)})
			}
		}
		sort.Slice(u.Inputs, func(a, b int) bool { return u.Inputs[a].Name < u.Inputs[b].Name })
		for _, s := range wc.Secrets {
			if s != nil && s.Name != nil {
				u.Secrets = append(u.Secrets, s.Name.Value)
			}
		}
		sort.Strings(u.Secrets)
		f.Uses = u
	}
	return f
}

// ExportFacts reads the workflows and actions of the files, or of the repository when there are none, and returns
// their facts. The files of the repository that are neither a workflow nor an action.yml (Dependabot configuration,
// the Actions lock file) are skipped. Nothing is checked and nothing is written. The project may be nil, and then it
// is found from the first file.
func (l *Linter) ExportFacts(files []string, project *Project) (*Facts, error) {
	if len(files) == 0 {
		var err error
		files, project, err = l.repositoryFiles(l.cwd)
		if err != nil {
			return nil, err
		}
	}
	if project == nil {
		p, err := l.projects.At(files[0])
		if err != nil {
			return nil, err
		}
		project = p
	}
	var graph *callGraph
	root := l.cwd
	if project != nil {
		graph = l.callGraphOf(project)
		root = project.RootDir()
	}
	rel := func(p string) string {
		abs := l.absFilePath(p)
		if r, err := filepath.Rel(absPath(root), abs); err == nil && !strings.HasPrefix(r, "..") {
			return filepath.ToSlash(r)
		}
		return filepath.ToSlash(p)
	}

	facts := &Facts{Version: FactsVersion, Tool: "jactionlint", ToolVersion: getCommandVersion(),
		Workflows: []*WorkflowFacts{}, Actions: []*ActionFacts{}, Calls: []*CallFacts{}}
	for _, f := range files {
		if l.isDependabotFile(f) || l.isActionsLockFile(f) {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("could not read %q: %w", f, err)
		}
		path := rel(f)
		if l.isActionFile(f) {
			facts.Actions = append(facts.Actions, l.actionFacts(path, src, graph))
			continue
		}
		wf := l.workflowFacts(path, src, graph, facts)
		facts.Workflows = append(facts.Workflows, wf)
	}
	sort.SliceStable(facts.Workflows, func(i, j int) bool { return facts.Workflows[i].Path < facts.Workflows[j].Path })
	sort.SliceStable(facts.Actions, func(i, j int) bool { return facts.Actions[i].Path < facts.Actions[j].Path })
	sort.SliceStable(facts.Calls, func(i, j int) bool {
		a, b := facts.Calls[i], facts.Calls[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Job != b.Job {
			return a.Job < b.Job
		}
		return stepIndex(a) < stepIndex(b)
	})
	return facts, nil
}

func stepIndex(c *CallFacts) int {
	if c.Step == nil {
		return -1
	}
	return *c.Step
}

func (l *Linter) workflowFacts(path string, src []byte, graph *callGraph, out *Facts) *WorkflowFacts {
	w, errs := Parse(src)
	f := &WorkflowFacts{Path: path, Events: []*EventFacts{}, Jobs: []*JobFacts{}}
	if w == nil {
		return f
	}
	f.Parsed = true
	f.SyntaxErrors = len(errs)
	f.Name = stringFact(w.Name)
	for _, e := range w.On {
		if e != nil {
			f.Events = append(f.Events, eventFacts(e))
		}
	}
	_, f.Callable = w.FindWorkflowCallEvent()
	f.Permissions = permissionFacts(w.Permissions)
	f.Concurrency = concurrencyFacts(w.Concurrency)
	f.CacheMode = stringFact(w.CacheMode)
	if f.Callable && graph != nil {
		f.CalledBy = graph.calledByFacts(path)
	}
	ids := make([]string, 0, len(w.Jobs))
	for id := range w.Jobs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := w.Jobs[ids[i]], w.Jobs[ids[j]]
		if a != nil && b != nil && a.Pos != nil && b.Pos != nil && a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		return ids[i] < ids[j]
	})
	events := eventNames(w.On)
	for _, id := range ids {
		j := w.Jobs[id]
		if j == nil {
			continue
		}
		jf := jobFacts(j, f.Permissions)
		f.Jobs = append(f.Jobs, jf)
		eff := jf.EffectivePermissions
		if jf.Uses != nil && jf.Uses.Kind == UsesLocal.String() {
			out.Calls = append(out.Calls, &CallFacts{Kind: "reusable-workflow", From: path, Job: jf.ID, Location: jf.Location,
				To: strings.TrimPrefix(jf.Uses.Path, "./"), Permissions: &eff, Events: events})
		}
		for _, s := range jf.Steps {
			if s.Uses != nil && s.Uses.Kind == UsesLocal.String() {
				idx := s.Index
				out.Calls = append(out.Calls, &CallFacts{Kind: "action", From: path, Job: jf.ID, Step: &idx, Location: s.Location,
					To: strings.TrimPrefix(s.Uses.Path, "./"), Permissions: &eff, Events: events})
			}
		}
	}
	return f
}

func (l *Linter) actionFacts(file string, src []byte, graph *callGraph) *ActionFacts {
	w, errs := ParseAction(src)
	f := &ActionFacts{Path: file}
	if w == nil || w.Action == nil {
		return f
	}
	f.Parsed = true
	f.SyntaxErrors = len(errs)
	a := w.Action
	f.Name = stringFact(a.Name)
	if a.Runs != nil && a.Runs.Using != nil {
		f.Using = a.Runs.Using.Value
	}
	for _, in := range a.Inputs {
		if in != nil && in.ID != nil {
			f.Inputs = append(f.Inputs, in.ID.Value)
		}
	}
	if a.Runs.IsComposite() {
		f.Steps = stepFacts(a.Runs.Steps)
	}
	if graph != nil {
		callers := graph.callersOf(path.Dir(file)) // the directory of the action file
		f.CalledBy = &ActionCalledByFacts{Known: callers.Known(), Callers: []*ActionCallerFacts{}}
		for _, c := range callers.Callers {
			cf := &ActionCallerFacts{Workflow: c.Workflow, Via: c.Via, Events: eventNames(c.Events)}
			if cf.Events == nil {
				cf.Events = []string{}
			}
			for _, call := range c.calls {
				cf.Inputs = append(cf.Inputs, inputFacts(call.Inputs)...)
			}
			f.CalledBy.Callers = append(f.CalledBy.Callers, cf)
		}
	}
	return f
}

// calledByFacts returns who calls the reusable workflow at the path, relative to the root with "/".
func (g *callGraph) calledByFacts(rel string) *CalledByFacts {
	ce := g.callerEventsOf(rel)
	f := &CalledByFacts{Callers: []string{}, Events: eventNames(ce.Events), Known: ce.Known}
	if f.Events == nil {
		f.Events = []string{}
	}
	for _, from := range g.callers[graphNode{'w', rel}] {
		f.Callers = append(f.Callers, from.path)
	}
	return f
}

// Marshal returns the document as indented JSON with a final newline.
func (f *Facts) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
