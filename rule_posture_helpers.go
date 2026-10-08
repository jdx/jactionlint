package jactionlint

import (
	"strings"
)

// This file has the helpers which the permission, pinning and checkout rules (excessive-permissions,
// unpinned-images, artipacked, cache-poisoning, ...) share.

// flattenSteps returns the steps in the order they appear, including the steps of `parallel:` groups
// (the group step itself is not returned).
func flattenSteps(steps []*Step) []*Step {
	ret := make([]*Step, 0, len(steps))
	for _, s := range steps {
		if p, ok := s.Exec.(*ExecParallel); ok {
			ret = append(ret, flattenSteps(p.Steps)...)
			continue
		}
		ret = append(ret, s)
	}
	return ret
}

// stepAction returns the action which the step runs and its parsed `uses:` value. It returns nil
// for a step which does not run an action, has no usable `uses:` or whose `uses:` is an expression.
func stepAction(s *Step) (*ExecAction, *UsesRef) {
	a, ok := s.Exec.(*ExecAction)
	if !ok || a.Uses == nil || a.Uses.Value == "" || a.Uses.ContainsExpression() {
		return nil, nil
	}
	return a, ParseUses(a.Uses.Value)
}

// isRepoAction reports whether the reference is the repository action with the canonical name
// "owner/repo[/subpath]" (lower case), whatever its ref.
func (u *UsesRef) isRepoAction(name string) bool {
	return u.Kind == UsesAction && u.CanonicalName() == name
}

// input returns the value of the input of the action. Names are case-insensitive. An input which is
// not set returns false.
func (e *ExecAction) input(name string) (string, bool) {
	i, ok := e.Inputs[strings.ToLower(name)]
	if !ok || i == nil || i.Value == nil {
		return "", false
	}
	return strings.TrimSpace(i.Value.Value), true
}

// isFalseLiteral reports whether the value is the YAML/Actions spelling of false. An expression is
// never a literal false.
func isFalseLiteral(v string) bool {
	return strings.EqualFold(v, "false")
}

// isTrueLiteral reports whether the value is the YAML/Actions spelling of true.
func isTrueLiteral(v string) bool {
	return strings.EqualFold(v, "true")
}

// privilegedTriggers are the events which run in the context of the base repository while their
// input is controlled by whoever opened a pull request, issue or comment.
var privilegedTriggers = []string{"pull_request_target", "workflow_run", "issue_comment"}
