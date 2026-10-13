package jactionlint

import (
	"sort"
	"strconv"
	"strings"
)

// This file implements the rule "concurrency-deadlock". A reusable workflow runs inside the run of the workflow
// which calls it, and the `github` context in it is the one of the caller: `github.workflow` is the name of the caller,
// `github.ref` is its ref. So a group which the caller and the called workflow both write as
// `${{ github.workflow }}-${{ github.ref }}` is one group. The caller holds it while it waits for the called workflow,
// and the called workflow waits for it: GitHub detects the deadlock and cancels the run ("a deadlock for concurrency
// group ... was detected", actionlint#538).
//
// The rule looks at the workflows of the repository that this workflow calls (`uses: ./.github/workflows/x.yaml`).
// A group is compared only when it is written with text and with the parts of the `github` context that have the same
// value in the caller and in the called workflow (see sameInCallee). Whatever else it reads (`inputs`, `matrix`,
// `github.job`, a function of another kind) is not compared, so an expression that is not known is silent.

// groupRef is the concurrency group of a workflow or of one of its jobs, in the form which is compared.
type groupRef struct {
	// norm is the group in canonical form: lower case (group names are case insensitive), the expressions without spaces.
	norm string
	// raw is the group as written.
	raw  string
	line int
	// where says where the group is set: "workflow" or `job "id"`.
	where string
}

// workflowGroups are the groups of a workflow which can be compared.
type workflowGroups struct {
	top  *groupRef
	jobs []*groupRef
}

// sameInCallee are the properties of the github context which a called workflow sees with the value of the caller.
// `github.job`, `github.action` and the other properties which belong to the job or the step that runs are not in the list.
var sameInCallee = map[string]bool{
	"workflow": true, "ref": true, "ref_name": true, "ref_type": true, "sha": true, "event_name": true, "event": true,
	"head_ref": true, "base_ref": true, "repository": true, "repository_owner": true, "run_id": true, "run_number": true,
	"run_attempt": true, "actor": true, "triggering_actor": true, "server_url": true, "ref_protected": true,
}

// sameInCalleeFuncs are the functions which keep a group comparable.
var sameInCalleeFuncs = map[string]bool{"format": true, "contains": true, "startswith": true, "endswith": true, "join": true}

// groupOf returns the group in the form which is compared, and false when the group is not known: it has an expression
// which the caller and the called workflow may evaluate differently, or it does not parse.
func groupOf(c *Concurrency, where string) (*groupRef, bool) {
	if c == nil || c.Group == nil || c.Expression != nil {
		return nil, false
	}
	s := c.Group
	if s.placeholders().failed {
		return nil, false
	}
	var sb strings.Builder
	last := 0
	for _, sp := range scanExprs(s) {
		sb.WriteString(strings.ToLower(s.Value[last:sp.Start]))
		canon, ok := canonicalExpression(sp.Src, sp.Node)
		if !ok {
			return nil, false
		}
		sb.WriteString("${{" + canon + "}}")
		last = sp.End
	}
	sb.WriteString(strings.ToLower(s.Value[last:]))
	line := 0
	if s.Pos != nil {
		line = s.Pos.Line
	}
	return &groupRef{norm: strings.TrimSpace(sb.String()), raw: s.Value, line: line, where: where}, true
}

// canonicalExpression returns the text of the expression without spaces and in lower case outside of strings, and
// false when the expression reads anything that is not the same in a caller and in the workflow it calls.
func canonicalExpression(src string, node ExprNode) (string, bool) {
	ok := true
	VisitExprNode(node, func(n, _ ExprNode, entering bool) {
		if !entering || !ok {
			return
		}
		switch n := n.(type) {
		case *VariableNode:
			if !strings.EqualFold(n.Name, "github") {
				ok = false
			}
		case *ObjectDerefNode:
			// The first property of github.<property>
			if v, isVar := n.Receiver.(*VariableNode); isVar && strings.EqualFold(v.Name, "github") && !sameInCallee[strings.ToLower(n.Property)] {
				ok = false
			}
		case *FuncCallNode:
			if !sameInCalleeFuncs[strings.ToLower(n.Callee)] {
				ok = false
			}
		case *ArrayDerefNode:
			ok = false
		case *IndexAccessNode:
			// github['ref'] is the same as github.ref, but it is not worth following
			if v, isVar := n.Operand.(*VariableNode); isVar && strings.EqualFold(v.Name, "github") {
				ok = false
			}
		}
	})
	if !ok {
		return "", false
	}
	var sb strings.Builder
	inString := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\'':
			inString = !inString
			sb.WriteByte(c)
		case inString:
			sb.WriteByte(c)
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		default:
			sb.WriteString(strings.ToLower(string(c)))
		}
	}
	return sb.String(), true
}

// groupsOfWorkflow collects the groups of a workflow which can be compared.
func groupsOfWorkflow(w *Workflow) *workflowGroups {
	g := &workflowGroups{}
	if r, ok := groupOf(w.Concurrency, "workflow"); ok {
		g.top = r
	}
	ids := make([]string, 0, len(w.Jobs))
	for id := range w.Jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		j := w.Jobs[id]
		if j == nil {
			continue
		}
		name := id
		if j.ID != nil {
			name = j.ID.Value
		}
		if r, ok := groupOf(j.Concurrency, "job "+strconv.Quote(name)); ok {
			g.jobs = append(g.jobs, r)
		}
	}
	if g.top == nil && len(g.jobs) == 0 {
		return nil
	}
	return g
}

// RuleConcurrencyDeadlock reports the group of a workflow which a workflow that it calls has too.
type RuleConcurrencyDeadlock struct {
	RuleBase
}

// NewRuleConcurrencyDeadlock creates a new RuleConcurrencyDeadlock instance.
func NewRuleConcurrencyDeadlock() *RuleConcurrencyDeadlock {
	return &RuleConcurrencyDeadlock{
		RuleBase: NewRuleBase("concurrency-deadlock", "Checks for a concurrency group which a workflow and the reusable workflow it calls share"),
	}
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleConcurrencyDeadlock) VisitWorkflowPre(n *Workflow) error {
	if n.Action != nil || len(n.calleeGroups) == 0 {
		return nil
	}
	top, ok := groupOf(n.Concurrency, "workflow")
	if !ok || n.Concurrency.Group.Pos == nil {
		return nil
	}
	ids := make([]string, 0, len(n.Jobs))
	for id := range n.Jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		j := n.Jobs[id]
		if j == nil || j.WorkflowCall == nil || j.WorkflowCall.Uses == nil {
			continue
		}
		target, ok := localTarget(j.WorkflowCall.Uses.Value)
		if !ok || target.kind != 'w' {
			continue
		}
		callee := n.calleeGroups[target.path]
		if callee == nil {
			continue
		}
		for _, g := range append([]*groupRef{callee.top}, callee.jobs...) {
			if g == nil || g.norm != top.norm {
				continue
			}
			jobName := id
			if j.ID != nil {
				jobName = j.ID.Value
			}
			rule.ReportIDf("concurrency-deadlock", n.Concurrency.Group.Pos, "the concurrency group %q is the same as the group of the %s of %s (line %d), which job %q calls. the called workflow runs inside this one with its `github` context, so both groups are one: this run holds the group while it waits for the called workflow, which waits for the group. GitHub detects the deadlock and cancels the run. give one of the groups a part of its own, for example a suffix such as \"-caller\"", top.raw, g.where, target.path, g.line, jobName)
			break
		}
	}
	return nil
}

func init() {
	registerRules(
		RuleInfo{ID: "concurrency-deadlock", Group: RuleGroupCorrectness, Summary: "The concurrency group of a workflow is the same as the group of a local reusable workflow it calls, so GitHub detects a deadlock and cancels the run.", DefaultLevel: SeverityError, Profile: ProfileCorrectness, DocsAnchor: "check-concurrency-deadlock"},
	)
	registerRuleFactory("concurrency-deadlock", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleConcurrencyDeadlock()}
	})
}
