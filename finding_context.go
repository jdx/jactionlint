package jactionlint

import (
	"slices"
	"strings"
)

// This file adds the context of a finding to the structured formats (--finding-context): the events which trigger
// the workflow and the permissions of the token of the job the finding is in. People who triage the findings of
// many repositories filter by them ("only workflows which run on pull_request_target with a write token"), see
// zizmor#1569.

// findingContext is what the findings of one workflow share.
type findingContext struct {
	events []string
	// workflow is the permissions of the workflow, nil when it declares none
	workflow map[string]string
	jobs     map[string]map[string]string // by lower case job ID: the permissions the job declares
	declared map[string]bool              // by lower case job ID: the job declares its own permissions
	scopes   *scopeIndex
}

// newFindingContext reads the events and the permissions of the workflow. It is nil for a file which is not a workflow.
func newFindingContext(w *Workflow, scopes *scopeIndex) *findingContext {
	if w == nil || w.Action != nil {
		return nil
	}
	fc := &findingContext{scopes: scopes, jobs: map[string]map[string]string{}, declared: map[string]bool{}}
	for _, e := range w.On {
		if e != nil {
			if n := e.EventName(); n != "" && !slices.Contains(fc.events, n) {
				fc.events = append(fc.events, n)
			}
		}
	}
	slices.Sort(fc.events)
	fc.workflow = permissionMap(w.Permissions)
	for id, j := range w.Jobs {
		if j != nil && j.Permissions != nil {
			fc.jobs[strings.ToLower(id)] = permissionMap(j.Permissions)
			fc.declared[strings.ToLower(id)] = true
		}
	}
	return fc
}

// permissionMap turns the permissions of a workflow or a job into the map of scope to level. read-all and write-all
// are the scope "*", and `permissions: {}` is "*" at none. What an expression decides (`permissions: ${{ ... }}`, or
// the value of a scope) is "unknown", also for "*": it is not "none", which is what an empty mapping says. It is nil for
// no permissions.
func permissionMap(p *Permissions) map[string]string {
	if p == nil {
		return nil
	}
	m := map[string]string{}
	if p.All != nil {
		switch {
		case p.All.ContainsExpression():
			m["*"] = "unknown"
		case p.All.Value == "read-all":
			m["*"] = "read"
		case p.All.Value == "write-all":
			m["*"] = "write"
		default:
			m["*"] = "unknown" // a value which GitHub does not know
		}
	}
	for name, s := range p.Scopes {
		switch {
		case s == nil || s.Value == nil || s.Value.ContainsExpression():
			m[name] = "unknown"
		default:
			m[name] = s.Value.Value
		}
	}
	if len(m) == 0 {
		m["*"] = "none"
	}
	return m
}

// apply sets the events and the permissions of the token on the finding: the ones of its job, or else the ones of
// the workflow. No permissions are set for a token which the file does not configure, because then they are a
// setting of the repository.
func (fc *findingContext) apply(e *Error) {
	if fc == nil || e == nil {
		return
	}
	e.Events = slices.Clone(fc.events)
	perms := fc.workflow
	if job := fc.scopes.scopeAt(e.Line).job; job != "" && fc.declared[strings.ToLower(job)] {
		perms = fc.jobs[strings.ToLower(job)]
	}
	if perms != nil {
		e.Permissions = make(map[string]string, len(perms))
		for k, v := range perms {
			e.Permissions[k] = v
		}
	}
}
