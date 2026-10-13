package jactionlint

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// tableScopes returns the scopes which the table of action_permissions.go lists, as everything the step needs, for
// the actions of the job: scope to level. It is what the fix of missing-permissions writes for a job that needs
// more than reading the repository. A step of another kind and an action which is not in the table add nothing: the
// fix never writes a scope that no documentation lists. An action which the table knows but does not determine
// (it has more needs in private repositories, or one of several scopes would do) is returned in inexact, because
// "contents: read" would break the job and the fix cannot say what to write instead.
func tableScopes(w *Workflow, j *Job) (scopes map[string]int, inexact []string) {
	ctx := &runContext{wf: w, job: j}
	scopes = map[string]int{}
	for _, s := range flattenSteps(j.Steps) {
		if isStaticallyFalse(s.If) {
			continue
		}
		a, ref := stepAction(s)
		if a == nil || ref.Kind != UsesAction {
			continue
		}
		entry, ok := actionPermissionTable[ref.CanonicalName()]
		if !ok {
			continue
		}
		needs := entry.Needs(actionStep{step: s, act: a, ctx: ctx})
		if needs.Pedantic || len(needs.All)+len(needs.Any) == 0 {
			continue
		}
		if a.InputsUnknown() {
			// the inputs may give another token, so what the action needs is not told
			inexact = append(inexact, ref.CanonicalName())
			continue
		}
		if !needs.Exact {
			inexact = append(inexact, ref.CanonicalName())
			continue
		}
		for _, n := range needs.All {
			scopes[n.Scope] = max(scopes[n.Scope], n.Level)
		}
	}
	return scopes, inexact
}

// jobPermissionEdits returns the edits which add the "permissions:" a job needs, for each job of the workflow that
// sets none and whose actions need more than "contents: read" according to the table. The block repeats
// "contents: read" because the permissions of a job replace the ones of the workflow. ok is false when a job that
// needs a block is not written in a shape the edit understands, or runs an action which the table does not determine:
// writing only the workflow level would then break the job, so there is no fix, and the reason says why.
func jobPermissionEdits(w *Workflow, d *srcDoc, unit string) (edits []TextEdit, why *NoFix) {
	jobs := make([]*Job, 0, len(w.Jobs))
	for _, j := range w.Jobs {
		jobs = append(jobs, j)
	}
	// In the order of the file, so that the reason names the first job
	slices.SortFunc(jobs, func(a, b *Job) int { return a.ID.Pos.Line - b.ID.Pos.Line })
	for _, j := range jobs {
		if j.Permissions != nil || j.WorkflowCall != nil {
			continue
		}
		scopes, inexact := tableScopes(w, j)
		if len(inexact) > 0 {
			return nil, &NoFix{Code: NoFixNeedsJudgment, Reason: fmt.Sprintf("the job %q runs %s, which needs permissions that jactionlint cannot determine exactly (see the documentation of the action), so \"contents: read\" would break it. set the permissions of the job by hand", j.ID.Value, inexact[0])}
		}
		if len(scopes) == 0 {
			continue
		}
		if _, has := scopes["contents"]; !has {
			scopes["contents"] = permLevelRead
		}
		names := make([]string, 0, len(scopes))
		for n := range scopes {
			names = append(names, n)
		}
		sort.Strings(names)
		site, found := d.locateJobWithout(j, "permissions")
		if !found {
			return nil, &NoFix{Code: NoFixUnsupportedShape, Reason: fmt.Sprintf("the job %q needs permissions of its own and is not written in a way the fix can edit exactly, set them by hand", j.ID.Value)}
		}
		pad := strings.Repeat(" ", site.bodyIndent)
		lines := []string{pad + "permissions:"}
		for _, n := range names {
			lines = append(lines, pad+unit+n+": "+permissionLevelName(scopes[n]))
		}
		edits = append(edits, d.insertAfterLine(site.after, lines...))
	}
	slices.SortFunc(edits, func(a, b TextEdit) int { return a.Start - b.Start })
	return edits, nil
}
