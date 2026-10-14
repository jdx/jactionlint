package jactionlint

import (
	"encoding/json"
	"strings"
)

// maxRunnerLabels bounds the labels that a runs-on expression is resolved to.
const maxRunnerLabels = 64

// maxRunnerDepth bounds the nesting of fromJSON() and of the values that callers pass down.
const maxRunnerDepth = 4

// runnerScope is where the labels of a `runs-on:` expression are looked up: the matrix of the job, and, for the
// inputs of a reusable workflow, the jobs of the repository that call it.
type runnerScope struct {
	job *Job
	// wf is the workflow of the job. It is nil for the job of a caller, whose own inputs are not looked up (a value
	// that a caller takes from its own inputs is not known).
	wf *Workflow
}

// staticRunnerLabels resolves the expression of a runs-on label to every label it can have, when the repository tells
// them all. ok is false when some value is not known: a context that the file does not define (`vars.RUNNER`, `env`,
// `needs`), an input that a workflow gets from callers that are not all known, a matrix that fromJSON() writes.
//
// It is not an evaluator. A condition is never evaluated: `cond && 'a' || 'b'` can be 'a' or 'b'. It follows what the
// checks of a matrix already know (matrixStrings, the rows and the include list), `inputs.<name>` of a reusable workflow
// that only workflow_call starts, and fromJSON() of a text that is known.
func (sc *runnerScope) staticRunnerLabels(n ExprNode) ([]string, bool) {
	labels, ok := sc.runnerLabels(n, 0)
	if !ok || len(labels) == 0 || len(labels) > maxRunnerLabels {
		return nil, false
	}
	return labels, true
}

func (sc *runnerScope) runnerLabels(n ExprNode, depth int) ([]string, bool) {
	if depth > maxRunnerDepth {
		return nil, false
	}
	switch n := n.(type) {
	case *StringNode:
		return []string{n.Value}, true
	case *LogicalOpNode:
		// `a && b` is b, or a when a is false-ish, which is never a label. `a || b` is a, or b when a is false-ish. The
		// condition `a` of `a && b || c` is not a label, so it has none and stays unresolved: only what can be the
		// result counts.
		switch n.Kind {
		case LogicalOpNodeKindAnd:
			return sc.runnerLabels(n.Right, depth+1)
		case LogicalOpNodeKindOr:
			l, ok := sc.runnerLabels(n.Left, depth+1)
			if !ok {
				return nil, false
			}
			r, ok := sc.runnerLabels(n.Right, depth+1)
			if !ok {
				return nil, false
			}
			return append(l, r...), true
		}
		return nil, false
	case *CompareOpNode, *NotOpNode, *BoolNode, *NullNode:
		return nil, true // a truth value is a condition, not a label
	case *FuncCallNode:
		switch strings.ToLower(n.Callee) {
		case "contains", "startswith", "endswith":
			return nil, true
		case "fromjson":
			if len(n.Args) != 1 {
				return nil, false
			}
			texts, ok := sc.runnerLabels(n.Args[0], depth+1)
			if !ok || len(texts) == 0 {
				return nil, false
			}
			var out []string
			for _, t := range texts {
				labels, ok := labelsOfJSON(t)
				if !ok {
					return nil, false
				}
				out = append(out, labels...)
			}
			return out, true
		}
		return nil, false
	}
	chain, dynamic, ok := chainOf(n)
	if !ok || len(dynamic) > 0 || len(chain) < 2 {
		return nil, false
	}
	switch chain[0] {
	case "matrix":
		return sc.matrixLabels(chain[1:])
	case "inputs":
		if len(chain) == 2 {
			return sc.inputLabels(chain[1], depth)
		}
	}
	return nil, false
}

// labelsOfJSON reads the labels of a text that fromJSON() parses: a string, or a list of strings.
func labelsOfJSON(text string) ([]string, bool) {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, false
	}
	switch v := v.(type) {
	case string:
		return []string{v}, true
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, len(out) > 0
	}
	return nil, false
}

// matrixLabels returns the labels that the matrix of the job has for `matrix.<path>`: every value of the row and of
// the include entries, a list of labels counting as its labels.
func (sc *runnerScope) matrixLabels(path []string) ([]string, bool) {
	if sc.job == nil || sc.job.Strategy == nil {
		return nil, false
	}
	m := sc.job.Strategy.Matrix
	if m == nil || m.Expression != nil {
		return nil, false
	}
	if len(path) != 1 {
		return nil, false // the key of an object value (`matrix.target.os`) is read by matrixImages
	}
	prop := path[0]
	var out []string
	found := false
	for name, row := range m.Rows {
		if !strings.EqualFold(name, prop) {
			continue
		}
		found = true
		if row == nil || row.Expression != nil {
			return nil, false
		}
		for _, v := range row.Values {
			labels, ok := labelsOfRaw(v)
			if !ok {
				return nil, false
			}
			out = append(out, labels...)
		}
	}
	if m.Include != nil {
		if m.Include.Expression != nil {
			return nil, false
		}
		for _, c := range m.Include.Combinations {
			if c.Expression != nil {
				return nil, false
			}
			for key, a := range c.Assigns {
				if !strings.EqualFold(key, prop) {
					continue
				}
				found = true
				labels, ok := labelsOfRaw(a.Value)
				if !ok {
					return nil, false
				}
				out = append(out, labels...)
			}
		}
	}
	// a property that the matrix does not define has no value to show that the runner is hosted
	return out, found
}

// labelsOfRaw returns the labels of a matrix value: a string without an expression, or a list of them at any depth.
func labelsOfRaw(v RawYAMLValue) ([]string, bool) {
	switch v := v.(type) {
	case *RawYAMLString:
		if ContainsExpression(v.Value) {
			return nil, false
		}
		return []string{v.Value}, true
	case *RawYAMLArray:
		var out []string
		for _, e := range v.Elems {
			labels, ok := labelsOfRaw(e)
			if !ok {
				return nil, false
			}
			out = append(out, labels...)
		}
		return out, true
	}
	return nil, false
}

// inputLabels returns the labels that every call of a reusable workflow in the repository passes for the input. It
// needs a workflow that only workflow_call starts (a manual run or another event would let a person choose), calls
// that are all known (see Workflow.callerEvents) and a value in each of them that is known too, which is the default of
// the input when the call leaves it out. A caller that takes the value from its own inputs is not followed. The
// repository cannot tell what a workflow of another repository passes, so a workflow that is called from outside only
// is not resolved either, as it has no caller here.
func (sc *runnerScope) inputLabels(name string, depth int) ([]string, bool) {
	w := sc.wf
	if w == nil || len(w.callerJobs) == 0 || w.callerEvents == nil || !w.callerEvents.Known || !onlyWorkflowCall(w) {
		return nil, false
	}
	var input *WorkflowCallEventInput
	for _, e := range w.On {
		if ev, ok := e.(*WorkflowCallEvent); ok {
			for _, in := range ev.Inputs {
				if in != nil && strings.EqualFold(in.ID, name) {
					input = in
				}
			}
		}
	}
	if input == nil {
		return nil, false
	}
	key := strings.ToLower(name)
	var out []string
	for _, call := range w.callerJobs {
		if call == nil || call.WorkflowCall == nil {
			return nil, false
		}
		var labels []string
		var ok bool
		if v := call.WorkflowCall.Inputs[key]; v != nil && v.Value != nil {
			labels, ok = (&runnerScope{job: call}).stringLabels(v.Value, depth+1)
		} else if d := input.Default; d != nil {
			labels, ok = (&runnerScope{}).stringLabels(d, depth+1)
		}
		if !ok {
			return nil, false
		}
		out = append(out, labels...)
		if len(out) > maxRunnerLabels {
			return nil, false
		}
	}
	return out, true
}

// stringLabels returns the labels that a string can have: its text, or the text and matrix values of the job.
func (sc *runnerScope) stringLabels(s *String, depth int) ([]string, bool) {
	if !s.ContainsExpression() {
		return []string{s.Value}, s.Value != ""
	}
	if sc.job != nil {
		if labels, ok := matrixImages(sc.job, s.Value); ok {
			return labels, true
		}
	}
	if !s.IsExpressionAssigned() {
		return nil, false
	}
	exprs, ok := s.templateExprs()
	if !ok || len(exprs) != 1 {
		return nil, false
	}
	labels, ok := sc.runnerLabels(exprs[0], depth)
	return labels, ok && len(labels) > 0
}
