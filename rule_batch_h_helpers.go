package jactionlint

import (
	"reflect"
	"sort"
	"strings"
)

// This file has the helpers which the rules of batch H (concurrency, gate jobs, untrusted code and
// dead code) share. They read the expression syntax tree instead of matching text, so that quoting,
// aliases and whitespace in the YAML do not matter.

// exprRef is a reference to a context in an expression, such as `needs.build.outputs.version`.
type exprRef struct {
	// chain are the lower-case segments of the reference. A segment is "*" for an array filter
	// (`needs.*.result`), an array index or an index that is not a string literal (`needs[matrix.job]`).
	chain []string
	// node is the outermost node of the reference and parent is the node that contains it. parent is
	// nil when the reference is the whole expression.
	node, parent ExprNode
}

func (r exprRef) String() string { return strings.Join(r.chain, ".") }

// chainOf returns the segments of a context reference and the expressions that compute dynamic
// indexes in it. ok is false when the chain does not start with a context (e.g. `fromJSON(x).y`).
func chainOf(n ExprNode) (chain []string, dynamic []ExprNode, ok bool) {
	switch n := n.(type) {
	case *VariableNode:
		return []string{strings.ToLower(n.Name)}, nil, true
	case *ObjectDerefNode:
		c, d, ok := chainOf(n.Receiver)
		if !ok {
			return nil, d, false
		}
		return append(c, strings.ToLower(n.Property)), d, true
	case ObjectDerefNode:
		return chainOf(&n)
	case *ArrayDerefNode:
		c, d, ok := chainOf(n.Receiver)
		if !ok {
			return nil, d, false
		}
		return append(c, "*"), d, true
	case ArrayDerefNode:
		return chainOf(&n)
	case *IndexAccessNode:
		c, d, ok := chainOf(n.Operand)
		if !ok {
			return nil, append(d, n.Index), false
		}
		if s, isStr := n.Index.(*StringNode); isStr {
			return append(c, strings.ToLower(s.Value)), d, true
		}
		return append(c, "*"), append(d, n.Index), true
	}
	return nil, nil, false
}

// collectExprRefs appends every context reference in the expression to out. A reference is reported
// once as its outermost chain, so `github.event.pull_request.number` is not also reported as
// `github.event`.
func collectExprRefs(n, parent ExprNode, out *[]exprRef) {
	switch n := n.(type) {
	case *VariableNode, *ObjectDerefNode, ObjectDerefNode, *ArrayDerefNode, ArrayDerefNode, *IndexAccessNode:
		chain, dynamic, ok := chainOf(n)
		if ok {
			*out = append(*out, exprRef{chain: chain, node: n, parent: parent})
		}
		for _, d := range dynamic {
			collectExprRefs(d, n, out)
		}
		if !ok {
			// The receiver is not a context, e.g. fromJSON(needs.x.outputs.y).z
			switch n := n.(type) {
			case *ObjectDerefNode:
				collectExprRefs(n.Receiver, n, out)
			case ObjectDerefNode:
				collectExprRefs(n.Receiver, &n, out)
			case *ArrayDerefNode:
				collectExprRefs(n.Receiver, n, out)
			case ArrayDerefNode:
				collectExprRefs(n.Receiver, &n, out)
			case *IndexAccessNode:
				collectExprRefs(n.Operand, n, out)
			}
		}
	case *NotOpNode:
		collectExprRefs(n.Operand, n, out)
	case *CompareOpNode:
		collectExprRefs(n.Left, n, out)
		collectExprRefs(n.Right, n, out)
	case *LogicalOpNode:
		collectExprRefs(n.Left, n, out)
		collectExprRefs(n.Right, n, out)
	case *FuncCallNode:
		for _, a := range n.Args {
			collectExprRefs(a, n, out)
		}
	}
}

// parseTemplateExprs parses every `${{ }}` expression of the string. ok is false when one of them
// cannot be parsed. The expressions parsed before the failure are returned either way.
func parseTemplateExprs(value string) (nodes []ExprNode, ok bool) {
	s := value
	for {
		i := strings.Index(s, "${{")
		if i < 0 {
			return nodes, true
		}
		s = s[i+3:]
		l := NewExprLexer(s)
		e, err := NewExprParser().Parse(l)
		if err != nil || l.Offset() == 0 {
			return nodes, false
		}
		nodes = append(nodes, e)
		s = s[l.Offset():]
	}
}

// parseConditionExpr parses an `if:` condition that is not wrapped in `${{ }}`.
func parseConditionExpr(value string) (ExprNode, bool) {
	e, err := NewExprParser().Parse(NewExprLexer(value + "}}"))
	if err != nil {
		return nil, false
	}
	return e, true
}

// conditionExprs returns the expressions of an `if:` condition, wrapped in `${{ }}` or not.
func conditionExprs(s *String) ([]ExprNode, bool) {
	if s == nil {
		return nil, true
	}
	if s.ContainsExpression() {
		return parseTemplateExprs(s.Value)
	}
	e, ok := parseConditionExpr(s.Value)
	if !ok {
		return nil, false
	}
	return []ExprNode{e}, true
}

// stringExprRefs returns the references of the `${{ }}` expressions of a string. ok is false when an
// expression cannot be parsed.
func stringExprRefs(value string) (refs []exprRef, ok bool) {
	nodes, ok := parseTemplateExprs(value)
	for _, n := range nodes {
		collectExprRefs(n, nil, &refs)
	}
	return refs, ok
}

// refCovers reports whether the reference reads the target context path, which means that the
// reference is a prefix of the target (`toJSON(needs)` reads `needs.build.result`), the target is a
// prefix of the reference (`needs.build.result.x`), or they only differ where one of them has a
// wildcard segment. The comparison ignores case.
func refCovers(chain, target []string) bool {
	n := min(len(chain), len(target))
	for i := 0; i < n; i++ {
		if chain[i] == "*" || target[i] == "*" {
			continue
		}
		if !strings.EqualFold(chain[i], target[i]) {
			return false
		}
	}
	return true
}

// refSet is the set of context references found in the strings of a workflow.
type refSet struct {
	refs []exprRef
	// unknown is true when an expression could not be parsed. A reference may be hidden in it, so rules
	// that report the absence of a reference must not report anything.
	unknown bool
}

func (rs *refSet) addString(value string) {
	if !strings.Contains(value, "${{") {
		return
	}
	refs, ok := stringExprRefs(value)
	rs.refs = append(rs.refs, refs...)
	if !ok {
		rs.unknown = true
	}
}

// addCondition adds the references of an `if:` condition.
func (rs *refSet) addCondition(s *String) {
	if s == nil || s.ContainsExpression() {
		return // already added with the other strings
	}
	e, ok := parseConditionExpr(s.Value)
	if !ok {
		rs.unknown = true
		return
	}
	collectExprRefs(e, nil, &rs.refs)
}

// reads reports whether some reference reads the context path.
func (rs *refSet) reads(target ...string) bool {
	for _, r := range rs.refs {
		if refCovers(r.chain, target) {
			return true
		}
	}
	return false
}

var stringPtrType = reflect.TypeOf((*String)(nil))
var rawYAMLStringPtrType = reflect.TypeOf((*RawYAMLString)(nil))

// walkStrings calls f for every string of the syntax tree below root: names, values, conditions,
// the expressions of booleans and numbers and the scalars of matrices. It reads the tree with
// reflection so that a field added to the syntax tree is never forgotten by the rules that need to
// see every expression.
func walkStrings(root any, f func(*String)) {
	walkValue(reflect.ValueOf(root), f)
}

func walkValue(v reflect.Value, f func(*String)) {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return
		}
		switch v.Type() {
		case stringPtrType:
			f(v.Interface().(*String))
			return
		case rawYAMLStringPtrType:
			s := v.Interface().(*RawYAMLString)
			f(&String{Value: s.Value, Pos: s.Pos()})
			return
		case reflect.TypeOf((*CommentIndex)(nil)):
			return
		}
		walkValue(v.Elem(), f)
	case reflect.Interface:
		if !v.IsNil() {
			walkValue(v.Elem(), f)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if t.Field(i).IsExported() {
				walkValue(v.Field(i), f)
			}
		}
	case reflect.Slice, reflect.Array:
		switch v.Type().Elem().Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Struct, reflect.Slice, reflect.Map:
			for i := 0; i < v.Len(); i++ {
				walkValue(v.Index(i), f)
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			walkValue(it.Value(), f)
		}
	}
}

// workflowRefs collects the context references of every string of the workflow, including the
// `if:` conditions that are not wrapped in `${{ }}`.
func workflowRefs(w *Workflow) *refSet {
	rs := &refSet{}
	walkStrings(w, func(s *String) { rs.addString(s.Value) })
	for _, j := range w.Jobs {
		rs.addCondition(j.If)
		if j.Snapshot != nil {
			rs.addCondition(j.Snapshot.If)
		}
		for _, s := range flattenSteps(j.Steps) {
			rs.addCondition(s.If)
		}
	}
	return rs
}

// jobRefs is like workflowRefs for one job.
func jobRefs(j *Job) *refSet {
	rs := &refSet{}
	walkStrings(j, func(s *String) { rs.addString(s.Value) })
	rs.addCondition(j.If)
	for _, s := range flattenSteps(j.Steps) {
		rs.addCondition(s.If)
	}
	return rs
}

// webhookEvents returns the webhook events of the workflow by name.
func webhookEvents(w *Workflow, names ...string) []*WebhookEvent {
	var ret []*WebhookEvent
	for _, e := range w.On {
		we, ok := e.(*WebhookEvent)
		if !ok || we.Hook == nil {
			continue
		}
		for _, n := range names {
			if strings.EqualFold(we.Hook.Value, n) {
				ret = append(ret, we)
				break
			}
		}
	}
	return ret
}

// hasEvent reports whether the workflow is triggered by one of the events.
func hasEvent(w *Workflow, names ...string) bool {
	for _, e := range w.On {
		for _, n := range names {
			if e.EventName() == n {
				return true
			}
		}
	}
	return false
}

// condTruth is a three-valued truth: an expression may be known to be true, known to be false or depend on
// something that the rule cannot know.
type condTruth int8

const (
	condUnknown condTruth = iota
	condTrue
	condFalse
)

func triNot(t condTruth) condTruth {
	switch t {
	case condTrue:
		return condFalse
	case condFalse:
		return condTrue
	}
	return condUnknown
}

// scenario describes what is known about a workflow run when a condition is evaluated: the event
// and, for events on a Git ref, what the ref looks like.
type scenario struct {
	event string
	// ref is the full ref (`refs/heads/main`) when it is known exactly. refPrefix is what the ref is
	// known to start with (`refs/tags/`) when it is not.
	ref, refPrefix string
}

// stringValue is a string that an expression evaluates to, as far as the scenario tells.
type stringValue struct {
	exact  string
	known  bool   // exact is the value
	prefix string // the value starts with prefix (when known is false)
}

func (sc scenario) refName() stringValue {
	if sc.ref != "" {
		for _, p := range []string{"refs/heads/", "refs/tags/"} {
			if strings.HasPrefix(sc.ref, p) {
				return stringValue{exact: strings.TrimPrefix(sc.ref, p), known: true}
			}
		}
	}
	return stringValue{}
}

func (sc scenario) refType() stringValue {
	r := sc.ref
	if r == "" {
		r = sc.refPrefix
	}
	switch {
	case strings.HasPrefix(r, "refs/heads/"):
		return stringValue{exact: "branch", known: true}
	case strings.HasPrefix(r, "refs/tags/"):
		return stringValue{exact: "tag", known: true}
	}
	return stringValue{}
}

// value returns the string that the node evaluates to in the scenario.
func (sc scenario) value(n ExprNode) stringValue {
	switch n := n.(type) {
	case *StringNode:
		return stringValue{exact: n.Value, known: true}
	case *BoolNode:
		if n.Value {
			return stringValue{exact: "true", known: true}
		}
		return stringValue{exact: "false", known: true}
	}
	chain, _, ok := chainOf(n)
	if !ok || len(chain) != 2 || chain[0] != "github" {
		return stringValue{}
	}
	switch chain[1] {
	case "event_name":
		if sc.event != "" {
			return stringValue{exact: sc.event, known: true}
		}
	case "ref":
		if sc.ref != "" {
			return stringValue{exact: sc.ref, known: true}
		}
		return stringValue{prefix: sc.refPrefix}
	case "ref_name":
		return sc.refName()
	case "ref_type":
		return sc.refType()
	case "head_ref", "base_ref":
		// They are only set for pull requests and empty otherwise
		if sc.event != "" && !isPullRequestEvent(sc.event) {
			return stringValue{exact: "", known: true}
		}
	}
	return stringValue{}
}

func isPullRequestEvent(e string) bool {
	switch e {
	case "pull_request", "pull_request_target", "pull_request_review", "pull_request_review_comment":
		return true
	}
	return false
}

// equals compares the value with a literal like GitHub does, ignoring case.
func (v stringValue) equals(lit string) condTruth {
	if v.known {
		if strings.EqualFold(v.exact, lit) {
			return condTrue
		}
		return condFalse
	}
	if v.prefix != "" && !strings.HasPrefix(strings.ToLower(lit), strings.ToLower(v.prefix)) {
		return condFalse
	}
	return condUnknown
}

func (v stringValue) startsWith(lit string) condTruth {
	l, p := strings.ToLower(lit), strings.ToLower(v.prefix)
	if v.known {
		return boolTri(strings.HasPrefix(strings.ToLower(v.exact), l))
	}
	if p != "" {
		if strings.HasPrefix(p, l) {
			return condTrue
		}
		if !strings.HasPrefix(l, p) {
			return condFalse
		}
	}
	return condUnknown
}

func boolTri(b bool) condTruth {
	if b {
		return condTrue
	}
	return condFalse
}

// eval evaluates a condition in the scenario. It knows the event name, the ref and string
// comparisons and gives up (condUnknown) on everything else, so a result of condTrue or condFalse is
// certain.
func (sc scenario) eval(n ExprNode) condTruth {
	switch n := n.(type) {
	case *BoolNode:
		return boolTri(n.Value)
	case *NullNode:
		return condFalse
	case *IntNode:
		return boolTri(n.Value != 0)
	case *StringNode:
		return boolTri(n.Value != "")
	case *NotOpNode:
		return triNot(sc.eval(n.Operand))
	case *LogicalOpNode:
		l, r := sc.eval(n.Left), sc.eval(n.Right)
		if n.Kind == LogicalOpNodeKindAnd {
			switch {
			case l == condFalse || r == condFalse:
				return condFalse
			case l == condTrue && r == condTrue:
				return condTrue
			}
			return condUnknown
		}
		switch {
		case l == condTrue || r == condTrue:
			return condTrue
		case l == condFalse && r == condFalse:
			return condFalse
		}
		return condUnknown
	case *CompareOpNode:
		if !n.Kind.IsEqualityOp() {
			return condUnknown
		}
		var t condTruth
		switch {
		case isStringLiteral(n.Right):
			t = sc.value(n.Left).equals(n.Right.(*StringNode).Value)
		case isStringLiteral(n.Left):
			t = sc.value(n.Right).equals(n.Left.(*StringNode).Value)
		default:
			return condUnknown
		}
		if n.Kind == CompareOpNodeKindNotEq {
			return triNot(t)
		}
		return t
	case *FuncCallNode:
		if strings.EqualFold(n.Callee, "startsWith") && len(n.Args) == 2 && isStringLiteral(n.Args[1]) {
			return sc.value(n.Args[0]).startsWith(n.Args[1].(*StringNode).Value)
		}
		return condUnknown
	}
	chain, _, ok := chainOf(n)
	if ok && len(chain) == 2 && chain[0] == "github" && (chain[1] == "head_ref" || chain[1] == "base_ref") {
		if v := sc.value(n); v.known {
			return boolTri(v.exact != "")
		}
	}
	return condUnknown
}

func isStringLiteral(n ExprNode) bool {
	_, ok := n.(*StringNode)
	return ok
}

// isTrue evaluates a boolean workflow field in the scenario and reports whether it is true. A literal is
// taken as it is. An expression is true only when the rule can tell from the event and the ref: one
// that depends on something else (an input, a variable) is not reported, because its author
// handles the cases the rule cannot see.
func (sc scenario) isTrue(b *Bool) bool {
	if b == nil {
		return false
	}
	if b.Expression == nil {
		return b.Value
	}
	exprs, ok := parseTemplateExprs(b.Expression.Value)
	if !ok || len(exprs) != 1 || !b.Expression.IsExpressionAssigned() {
		return false
	}
	return sc.eval(exprs[0]) == condTrue
}

// statusFunctions are the functions that stop GitHub from adding the implicit success() to the
// condition of a job or a step.
var statusFunctions = []string{"always", "cancelled", "failure", "success"}

// callsStatusFunction reports whether the condition calls one of the status check functions,
// possibly negated. success() counts only when countSuccess is set because it is what the
// implicit check already is.
func callsStatusFunction(exprs []ExprNode, countSuccess bool) bool {
	found := false
	for _, e := range exprs {
		VisitExprNode(e, func(n, _ ExprNode, entering bool) {
			f, ok := n.(*FuncCallNode)
			if !ok || !entering {
				return
			}
			for _, s := range statusFunctions {
				if strings.EqualFold(f.Callee, s) && (countSuccess || s != "success") {
					found = true
				}
			}
		})
	}
	return found
}

// jobIDsInOrder returns the keys of Workflow.Jobs in the order the jobs are written.
func jobIDsInOrder(w *Workflow) []string {
	ids := make([]string, 0, len(w.Jobs))
	for id := range w.Jobs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := w.Jobs[ids[i]].Pos, w.Jobs[ids[j]].Pos
		if a != nil && b != nil && *a != *b {
			return a.IsBefore(b)
		}
		return ids[i] < ids[j]
	})
	return ids
}
