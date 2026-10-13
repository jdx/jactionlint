package jactionlint

import (
	"sort"
	"strings"
)

// RuleEventNameNeverReceived detects a comparison of `github.event_name` with the name of an event that the workflow
// never receives, which is always false:
//
//	on: [push, workflow_dispatch]
//	jobs:
//	  deploy:
//	    if: github.event_name == 'pull_request'   # never true
//
// The job or the step never runs, or the comparison is a dead branch of a larger expression: a wrong trigger, a
// typo in the name of the event, or code which is left over from a trigger that was removed. It also reports two
// different events that must both be equal to the event of the run (`github.event_name == 'push' &&
// github.event_name == 'pull_request'`).
//
// Only a workflow whose events are all known is looked at: one with `workflow_call` is skipped (the event is the one of
// its caller), and so is a composite action and a workflow with the internal event `image_version`. The names are
// compared without regard to case, as GitHub compares strings. `!=` and the other ways to test the event (`contains`,
// a `switch` by a matrix) are not looked at.
type RuleEventNameNeverReceived struct {
	RuleBase
	src *sourceIndex
}

// NewRuleEventNameNeverReceived creates a new RuleEventNameNeverReceived instance. The source of the file makes the
// positions exact in values that span several lines. It can be nil.
func NewRuleEventNameNeverReceived(src []byte) *RuleEventNameNeverReceived {
	r := &RuleEventNameNeverReceived{
		RuleBase: NewRuleBase("event-name-never-received", "Checks for \"github.event_name\" compared with an event that the workflow never receives"),
	}
	if src != nil {
		r.src = newSourceIndex(src)
	}
	return r
}

// VisitWorkflowPre is callback when visiting Workflow node before visiting its children.
func (rule *RuleEventNameNeverReceived) VisitWorkflowPre(n *Workflow) error {
	if n.Action != nil || len(n.On) == 0 {
		return nil
	}
	events := map[string]bool{}
	for _, e := range n.On {
		switch e.(type) {
		case *WorkflowCallEvent, *ImageVersionEvent:
			return nil // the event of the run is not known from this file
		}
		events[strings.ToLower(e.EventName())] = true
	}
	names := make([]string, 0, len(events))
	for name := range events {
		names = append(names, name)
	}
	sort.Strings(names)
	list := strings.Join(names, ", ")

	check := func(root ExprNode, pos func(*Token) *Pos) {
		rule.checkExpr(root, events, list, pos)
	}
	// every ${{ }} of every string. The parser can keep one string in two fields (a mapping that is one expression), so a
	// string is looked at once for each place and text.
	type site struct {
		line, col int
		value     string
	}
	seen := map[site]bool{}
	walkStrings(n, func(s *String) {
		if s == nil || !strings.Contains(s.Value, "${{") {
			return
		}
		if s.Pos != nil {
			k := site{s.Pos.Line, s.Pos.Col, s.Value}
			if seen[k] {
				return
			}
			seen[k] = true
		}
		for _, sp := range rule.src.scanExprs(s) {
			sp := sp
			check(sp.Node, sp.TokPos)
		}
	})
	// the conditions which are not wrapped
	cond := func(s *String) {
		// A condition may have spaces around it, or be a block scalar: the lexer skips them, and the offsets of the tokens
		// are the offsets of the value
		if s == nil || strings.Contains(s.Value, "${{") || strings.TrimSpace(s.Value) == "" {
			return
		}
		if e, err := s.condExpr(); err == nil && e != nil {
			check(e, func(t *Token) *Pos {
				line, col := rule.src.valuePosition(s, t.Offset)
				return &Pos{Line: line, Col: col}
			})
		}
	}
	for _, j := range n.Jobs {
		if j == nil {
			continue
		}
		cond(j.If)
		if j.Snapshot != nil {
			cond(j.Snapshot.If)
		}
		walkSteps(j.Steps, func(s *Step) { cond(s.If) })
	}
	return nil
}

// eventNameCompare returns the event written in a comparison of github.event_name with a string.
func eventNameCompare(n ExprNode) (event string, ok bool) {
	c, isCmp := n.(*CompareOpNode)
	if !isCmp || c.Kind != CompareOpNodeKindEq {
		return "", false
	}
	l, r := c.Left, c.Right
	if isEventName(r) {
		l, r = r, l
	}
	s, isStr := r.(*StringNode)
	if !isStr || !isEventName(l) {
		return "", false
	}
	return s.Value, true
}

func isEventName(n ExprNode) bool {
	chain, _, ok := chainOf(n)
	return ok && len(chain) == 2 && chain[0] == "github" && chain[1] == "event_name"
}

func (rule *RuleEventNameNeverReceived) checkExpr(root ExprNode, events map[string]bool, list string, pos func(*Token) *Pos) {
	if root == nil {
		return
	}
	seen := map[*CompareOpNode]bool{}
	VisitExprNode(root, func(n, parent ExprNode, entering bool) {
		if !entering {
			return
		}
		// a comparison with an event that never happens
		if ev, ok := eventNameCompare(n); ok && !events[strings.ToLower(ev)] {
			c := n.(*CompareOpNode)
			seen[c] = true
			rule.ReportIDf("event-name-never-received", pos(c.Token()), "\"github.event_name == %s\" is always false: this workflow is never run by the event %q, it runs on %s", quoteExprString(ev), ev, list)
			return
		}
		// two different events at once: the operands of a chain of &&
		if l, ok := n.(*LogicalOpNode); ok && l.Kind == LogicalOpNodeKindAnd {
			if p, isAnd := parent.(*LogicalOpNode); isAnd && p.Kind == LogicalOpNodeKindAnd {
				return // looked at from the start of the chain
			}
			var operands []ExprNode
			flattenAnd(l, &operands)
			first := ""
			var firstNode *CompareOpNode
			for _, op := range operands {
				ev, ok := eventNameCompare(op)
				if !ok || !events[strings.ToLower(ev)] {
					continue // an event which never happens is reported above
				}
				c := op.(*CompareOpNode)
				if firstNode == nil {
					first, firstNode = ev, c
					continue
				}
				if !strings.EqualFold(ev, first) {
					rule.ReportIDf("event-name-never-received", pos(c.Token()), "\"github.event_name == %s\" is always false here: the same condition needs the event to be %q as well, and a run has one event", quoteExprString(ev), first)
					return
				}
			}
		}
	})
}

func flattenAnd(n ExprNode, out *[]ExprNode) {
	if l, ok := n.(*LogicalOpNode); ok && l.Kind == LogicalOpNodeKindAnd {
		flattenAnd(l.Left, out)
		flattenAnd(l.Right, out)
		return
	}
	*out = append(*out, n)
}

// quoteExprString writes a string as a literal of expressions.
func quoteExprString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func init() {
	registerRules(
		RuleInfo{ID: "event-name-never-received", Group: RuleGroupCorrectness, Summary: "github.event_name is compared with an event that the workflow never receives, so the comparison is always false.", DefaultLevel: SeverityWarning, Profile: ProfilePedantic, DocsAnchor: "check-event-name-never-received"},
	)
	registerRuleFactory("event-name-never-received", func(env *RuleEnv) []Rule {
		return []Rule{NewRuleEventNameNeverReceived(env.src)}
	})
}
