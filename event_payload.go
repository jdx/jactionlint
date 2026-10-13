package jactionlint

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:generate go run ./scripts/generate-event-payloads ./event_payloads.go

// payloadShape is what is known about a value of the payload of a webhook event (github.event). A nil shape is
// a value which is not looked into: a scalar, something arbitrary or something unknown.
//
// The type of github.event stays an untyped object: the shapes are used only to check the names of the properties
// of github.event.*, so that the types of the values never change what the other checks report.
type payloadShape struct {
	props map[string]*payloadShape // an object whose properties are all known
	elem  *payloadShape            // an array of objects of this shape. It is nil for an array of anything
	array bool
}

// The shapes are parsed from the text of event_payloads.go on first use.
var (
	payloadShapesOnce sync.Once
	payloadShapes     map[string]*payloadShape
)

// payloadLoader parses the objects of eventPayloadObjects, which refer to each other by name.
type payloadLoader struct {
	objects map[string]*payloadShape
}

func (l *payloadLoader) object(name string) *payloadShape {
	if s, ok := l.objects[name]; ok {
		return s
	}
	text, ok := eventPayloadObjects[name]
	if !ok {
		panic("event payload object is not defined: " + name) // The generated file is wrong
	}
	p := &payloadParser{src: text, object: l.object}
	s := p.shape()
	l.objects[name] = s
	return s
}

func loadPayloadShapes() map[string]*payloadShape {
	payloadShapesOnce.Do(func() {
		l := &payloadLoader{objects: make(map[string]*payloadShape, len(eventPayloadObjects))}
		payloadShapes = make(map[string]*payloadShape, len(eventPayloads))
		for name, text := range eventPayloads {
			if !strings.HasPrefix(text, "=") {
				p := &payloadParser{src: text, object: l.object}
				payloadShapes[name] = p.shape()
			}
		}
		for name, text := range eventPayloads {
			if alias, ok := strings.CutPrefix(text, "="); ok {
				payloadShapes[name] = payloadShapes[alias]
			}
		}
	})
	return payloadShapes
}

// payloadParser parses the text of a shape (see event_payloads.go).
type payloadParser struct {
	src    string
	pos    int
	object func(name string) *payloadShape
}

func (p *payloadParser) skipSpace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *payloadParser) peek() byte {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *payloadParser) expect(c byte) {
	if p.peek() != c {
		panic("event payload shape is malformed at " + string(c)) // The generated file is wrong
	}
	p.pos++
}

func (p *payloadParser) name() string {
	p.skipSpace()
	start := p.pos
	for p.pos < len(p.src) && !strings.ContainsRune("{}[],:@ \t\r\n", rune(p.src[p.pos])) {
		p.pos++
	}
	return p.src[start:p.pos]
}

// shape parses a shape: an object {...}, an array [shape] or a reference @name.
func (p *payloadParser) shape() *payloadShape {
	switch p.peek() {
	case '@':
		p.pos++
		return p.object(p.name())
	case '[':
		p.pos++
		s := &payloadShape{array: true}
		if p.peek() != ']' {
			s.elem = p.shape()
		}
		p.expect(']')
		return s
	}
	p.expect('{')
	s := &payloadShape{props: map[string]*payloadShape{}}
	for p.peek() != '}' {
		name := p.name()
		var sub *payloadShape
		if p.peek() == ':' {
			p.pos++
			sub = p.shape()
		}
		s.props[name] = sub
		if p.peek() == ',' {
			p.pos++
		}
	}
	p.expect('}')
	return s
}

// unionPayloadShapes returns the shape of a value which has the shape of a or of b. A property of an object is
// known when it is known in a or in b. Anything which is not known in one of them is not known.
func unionPayloadShapes(a, b *payloadShape) *payloadShape {
	switch {
	case a == nil || b == nil:
		return nil
	case a.props != nil && b.props != nil:
		if a == b {
			return a
		}
		ret := &payloadShape{props: make(map[string]*payloadShape, len(a.props)+len(b.props))}
		for k, v := range a.props {
			ret.props[k] = v
		}
		for k, v := range b.props {
			if old, ok := ret.props[k]; ok {
				ret.props[k] = unionPayloadShapes(old, v)
			} else {
				ret.props[k] = v
			}
		}
		return ret
	case a.array && b.array:
		return &payloadShape{array: true, elem: unionPayloadShapes(a.elem, b.elem)}
	}
	return nil
}

// payloadUniverse is the name of the payload which has the properties of the payloads of all the events.
const payloadUniverse = "*"

// eventPayload is what is known of github.event of a workflow.
type eventPayload struct {
	shape *payloadShape
	// events are the events of the workflow when shape is the union of their payloads. It is nil when shape is the
	// union of the payloads of all the events.
	events []string
}

// payloadOfWorkflow returns what is known of github.event of the workflow, or nil when nothing is. It is known when
// every event of "on:" is a webhook event which has a schema. Nothing is known of the payloads of workflow_call (the
// caller's), workflow_dispatch, repository_dispatch and schedule, nor when "on:" is empty or has something else, and
// not of composite actions and reusable workflows, whose payload depends on the caller.
//
// With strict, the properties are the ones of the payloads of the events of the workflow. Without it, they are the
// ones of the payloads of all the events, so that a workflow which reads a property of another event (such as
// github.event.pull_request.number in a workflow for push, a defensive read which is null) is not reported: only a
// property which no event has, a typo, is.
func payloadOfWorkflow(w *Workflow, strict bool) *eventPayload {
	if w == nil || w.Action != nil || len(w.On) == 0 {
		return nil
	}
	shapes := loadPayloadShapes()
	var shape *payloadShape
	var events []string
	seen := map[string]bool{}
	for _, e := range w.On {
		we, ok := e.(*WebhookEvent)
		if !ok || we.Hook == nil {
			return nil
		}
		name := we.Hook.Value
		s, ok := shapes[name]
		if !ok || s == nil {
			return nil
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		events = append(events, name)
		if shape == nil {
			shape = s
		} else {
			shape = unionPayloadShapes(shape, s)
		}
	}
	if !strict {
		shape, events = shapes[payloadUniverse], nil
	}
	if shape == nil || shape.props == nil {
		return nil
	}
	return &eventPayload{shape: shape, events: events}
}

// strictEventPayloads reports whether the properties of github.event are the ones of the payloads of the events of
// the workflow, and not of the payloads of all the events. An explicit "strict-event-payloads" decides. Unset,
// the pedantic profile turns it on and the others leave it off.
func (c *Config) strictEventPayloads() bool {
	if on, ok := c.ruleOptionBool("undefined-property", "strict-event-payloads"); ok {
		return on
	}
	return c != nil && c.Profile == ProfilePedantic
}

// payloadRef is a place of a value in the payload while an expression is followed down from github.event.
type payloadRef struct {
	// shape is the shape of the value. When filtered is true, the value is the array which `.*` made, and shape is
	// the shape of each of its elements.
	shape    *payloadShape
	filtered bool
	// path is the expression which got here, for messages
	path string
}

// checkEventPayload reports the properties of github.event which the payload of the events of the workflow does not
// have. What is below a value which is not known is not checked.
func (sema *ExprSemanticsChecker) checkEventPayload(expr ExprNode) {
	if sema.payload == nil {
		return
	}
	refs := map[ExprNode]*payloadRef{}
	VisitExprNode(expr, func(n, _ ExprNode, entering bool) {
		if entering {
			return
		}
		var recv ExprNode
		switch n := n.(type) {
		case *ObjectDerefNode:
			recv = n.Receiver
		case *ArrayDerefNode:
			recv = n.Receiver
		case *IndexAccessNode:
			recv = n.Operand
		default:
			return
		}
		if v, ok := recv.(*VariableNode); ok {
			if v.Name == "github" && githubProperty(n) == "event" {
				refs[n] = &payloadRef{shape: sema.payload.shape, path: "github.event"}
			}
			return
		}
		if r := refs[recv]; r != nil {
			if next := sema.stepPayload(n, r); next != nil && next.shape != nil {
				refs[n] = next
			}
		}
	})
}

// githubProperty returns the name of the property of the github variable which n reads: github.event or github['event'].
func githubProperty(n ExprNode) string {
	switch n := n.(type) {
	case *ObjectDerefNode:
		return n.Property
	case *IndexAccessNode:
		if lit, ok := n.Index.(*StringNode); ok {
			return strings.ToLower(lit.Value)
		}
	}
	return ""
}

// stepPayload follows a property, an index or `.*` of n from r. It reports a property which the payload does not
// have. It returns nil when nothing more is known.
func (sema *ExprSemanticsChecker) stepPayload(n ExprNode, r *payloadRef) *payloadRef {
	switch n := n.(type) {
	case *ObjectDerefNode:
		return sema.lookupPayloadProp(n, r, n.Property)
	case *IndexAccessNode:
		if lit, ok := n.Index.(*StringNode); ok {
			return sema.lookupPayloadProp(n, r, strings.ToLower(lit.Value))
		}
		if r.shape.array && !r.filtered {
			return &payloadRef{shape: r.shape.elem, path: r.path + "[]"}
		}
	case *ArrayDerefNode:
		switch {
		case r.shape.array && !r.filtered:
			return &payloadRef{shape: r.shape.elem, filtered: true, path: r.path + ".*"}
		case r.shape.array && r.filtered:
			// The filter of an array of arrays flattens them
			return &payloadRef{shape: r.shape.elem, filtered: true, path: r.path + ".*"}
		}
	}
	return nil
}

func (sema *ExprSemanticsChecker) lookupPayloadProp(n ExprNode, r *payloadRef, prop string) *payloadRef {
	if r.shape.props == nil {
		return nil
	}
	sub, ok := r.shape.props[prop]
	if !ok {
		sema.reportUnknownPayloadProp(n, r.path, prop, r.shape)
		return nil
	}
	return &payloadRef{shape: sub, filtered: r.filtered, path: r.path + "." + prop}
}

// maxListedPayloadProps is the number of properties which a message lists.
const maxListedPayloadProps = 24

func (sema *ExprSemanticsChecker) reportUnknownPayloadProp(n ExprNode, path, prop string, shape *payloadShape) {
	names := make([]string, 0, len(shape.props))
	for k := range shape.props {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("property ")
	b.WriteString(strconv.Quote(prop))
	b.WriteString(" is not defined in ")
	b.WriteString(path)
	if sema.payload.events == nil {
		b.WriteString(": no webhook event has it in its payload")
	} else if len(sema.payload.events) == 1 {
		b.WriteString(", the payload of the event ")
		b.WriteString(strconv.Quote(sema.payload.events[0]))
	} else {
		b.WriteString(", the payload of the events ")
		b.WriteString(sortedQuotes(append([]string(nil), sema.payload.events...)))
	}
	if s := didYouMean(prop, names); s != "" {
		b.WriteString(". did you mean ")
		b.WriteString(strconv.Quote(s))
		b.WriteString("?")
	} else if len(names) <= maxListedPayloadProps {
		b.WriteString(". available properties are ")
		b.WriteString(sortedQuotes(names))
	}
	sema.errorfID("undefined-property", n, "%s", b.String())
}
