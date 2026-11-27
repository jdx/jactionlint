package jactionlint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

// The fixes of the rule "actions-lock" delete the entries of the lock file which nothing uses: a workflow which does
// not exist any more, a pin which a workflow does not use any more, and a dependency which no workflow reaches. A
// fix which deletes a workflow or a pin deletes with it the dependencies which only that entry reached, as
// `gh actions-lock` prunes them. Nothing else is written: an entry which is missing is resolved on GitHub, with the
// actions which a composite action runs, which is what `gh actions-lock` is for.
//
// Every edit deletes whole lines, so the rest of the file stays byte for byte the same. A collection which loses
// all its entries is written empty ("[]" or "{}"), as gh actions-lock writes it. An entry which is not written
// in the block style that gh actions-lock writes (a flow mapping or list, an entry which shares its line) has no
// fix. The fixes are only offered for a lock file whose version is known (see actionsLock.usable), as the checks
// which report the entries are.

var (
	lockNoFixWrite = &NoFix{Code: NoFixNeedsJudgment, Reason: "an entry of the lock file is resolved on GitHub, with the actions which a composite action runs, and jactionlint does not write one: run `gh actions-lock`"}
	lockNoFixEdit  = &NoFix{Code: NoFixNeedsJudgment, Reason: "the entry does not follow the format and which value is right is not known: correct it by hand, or write the file again with `gh actions-lock --relock`"}
	lockNoFixShape = &NoFix{Code: NoFixUnsupportedShape, Reason: "the entry is not written in the block style of gh actions-lock, so the fix cannot delete it exactly: remove it by hand or run `gh actions-lock`"}
)

// reLockKeyLine is a line which has only a key of a block mapping and its colon: its value is on the lines below.
var reLockKeyLine = regexp.MustCompile(`^[ ]*(?:'(?:[^']|'')*'|"(?:[^"\\]|\\.)*"|[^'"#\s][^#]*?)[ ]*:[ ]*$`)

// reLockItemPrefix is the text before the value of an item of a block sequence.
var reLockItemPrefix = regexp.MustCompile(`^[ ]*-[ ]+$`)

// lockEditor builds the edits which delete entries of a lock file.
type lockEditor struct {
	src   []byte
	lines *lineIndex
}

func newLockEditor(src []byte) *lockEditor {
	return &lockEditor{src: src, lines: newLineIndex(src)}
}

// line returns the text of the 1-based line without its line break.
func (e *lockEditor) line(n int) string {
	s := e.lines.startOf(n)
	if s < 0 {
		return ""
	}
	t := e.src[s:]
	if i := strings.IndexByte(string(t), '\n'); i >= 0 {
		t = t[:i]
	}
	return strings.TrimSuffix(string(t), "\r")
}

// lineEnd returns the offset just after the line break of the 1-based line, or the end of the source.
func (e *lockEditor) lineEnd(n int) int {
	if s := e.lines.startOf(n + 1); s >= 0 {
		return s
	}
	return len(e.src)
}

// contentEnd returns the offset of the line break which ends the 1-based line, or the end of the source.
func (e *lockEditor) contentEnd(n int) int {
	return e.lines.startOf(n) + len(e.line(n))
}

// lockBlock is the lines of an entry: [first, last].
type lockBlock struct{ first, last int }

// block returns the lines of the entry whose key (or item) starts at the 1-based line and column. The text before
// the column must be the indentation of a block mapping, or of a block sequence and its "-" when item is true. The
// entry goes on as long as the lines are indented deeper than it starts. Blank lines and comments at its end belong
// to what follows.
func (e *lockEditor) block(line, col int, item bool) (lockBlock, bool) {
	text := e.line(line)
	pre, ok := columnInLine(text, col)
	if !ok {
		return lockBlock{}, false
	}
	indent := pre
	if item {
		if !reLockItemPrefix.MatchString(text[:pre]) {
			return lockBlock{}, false
		}
		indent = strings.IndexByte(text, '-')
	} else if strings.TrimLeft(text[:pre], " ") != "" {
		return lockBlock{}, false
	}
	b := lockBlock{line, line}
	for n := line + 1; e.lines.startOf(n) >= 0 && e.lines.startOf(n) < len(e.src); n++ {
		t := e.line(n)
		body := strings.TrimLeft(t, " ")
		if body == "" || body[0] == '#' {
			continue
		}
		if len(t)-len(body) <= indent {
			break
		}
		b.last = n
	}
	return b, true
}

// lockCollection is a mapping or a list of the lock file which entries are deleted from.
type lockCollection struct {
	key   *yaml.Node // the key whose value it is, for "[]" and "{}"
	node  *yaml.Node
	empty string // what an empty one is written as
}

func (c lockCollection) size() int {
	if c.node.Kind == yaml.MappingNode {
		return len(c.node.Content) / 2
	}
	return len(c.node.Content)
}

// delete returns the edits which delete the entries of the collection which start at the positions. It returns false
// when one of them cannot be deleted exactly.
func (e *lockEditor) delete(c lockCollection, starts []*yaml.Node) ([]TextEdit, bool) {
	if c.node == nil || c.node.Style&yaml.FlowStyle != 0 || (c.node.Kind != yaml.MappingNode && c.node.Kind != yaml.SequenceNode) {
		return nil, false
	}
	item := c.node.Kind == yaml.SequenceNode
	blocks := make([]lockBlock, 0, len(starts))
	seen := map[int]bool{}
	for _, n := range starts {
		if seen[n.Line] {
			continue
		}
		seen[n.Line] = true
		b, ok := e.block(n.Line, n.Column, item)
		if !ok {
			return nil, false
		}
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return nil, false
	}
	if len(blocks) == c.size() && c.key != nil && reLockKeyLine.MatchString(e.line(c.key.Line)) {
		// Nothing is left: the value is written empty after the key, from its colon to the end of the last entry
		first, last := blocks[0], blocks[0]
		for _, b := range blocks {
			if b.first < first.first {
				first = b
			}
			if b.last > last.last {
				last = b
			}
		}
		if first.first > c.key.Line {
			keyLine := e.line(c.key.Line)
			start := e.lines.startOf(c.key.Line) + len(strings.TrimRight(keyLine, " "))
			return []TextEdit{{Start: start, End: e.contentEnd(last.last), NewText: " " + c.empty}}, true
		}
	}
	edits := make([]TextEdit, 0, len(blocks))
	for _, b := range blocks {
		edits = append(edits, TextEdit{Start: e.lines.startOf(b.first), End: e.lineEnd(b.last)})
	}
	return edits, true
}

// workflows is the collection "workflows" of the lock file.
func (l *actionsLock) workflowsCollection() lockCollection {
	return lockCollection{key: l.workflowsKey, node: l.workflowsNode, empty: "{}"}
}

func (l *actionsLock) depsCollection() lockCollection {
	return lockCollection{key: l.depsKey, node: l.depsNode, empty: "{}"}
}

func (w *lockWorkflow) pinsCollection() lockCollection {
	return lockCollection{key: &yaml.Node{Line: w.Line, Column: w.Col}, node: w.value, empty: "[]"}
}

// depKeyNodes returns the keys of "dependencies" whose canonical key is one of the keys: every spelling of them.
func (l *actionsLock) depKeyNodes(keys map[string]bool) []*yaml.Node {
	if l.depsNode == nil || l.depsNode.Kind != yaml.MappingNode {
		return nil
	}
	var out []*yaml.Node
	for i := 0; i+1 < len(l.depsNode.Content); i += 2 {
		k := l.depsNode.Content[i]
		if p, ok := parseLockPin(k.Value, k.Line, k.Column); ok && keys[p.Key] {
			out = append(out, k)
		}
	}
	return out
}

// reached returns the dependencies which the pins of the workflows reach, directly or through the "uses" of
// other dependencies. skip tells which pins are left out.
func (l *actionsLock) reached(skip func(w *lockWorkflow, i int) bool) map[string]bool {
	reached := map[string]bool{}
	var reach func(key string)
	reach = func(key string) {
		if reached[key] {
			return
		}
		reached[key] = true
		if d := l.deps[key]; d != nil {
			for _, u := range d.Uses {
				reach(u.Key)
			}
		}
	}
	for _, w := range l.Workflows {
		for i, p := range w.Pins {
			if !skip(w, i) {
				reach(p.Key)
			}
		}
	}
	return reached
}

// orphans returns the dependencies which the pins reach and which nothing reaches when the pins which skip tells
// are deleted.
func (l *actionsLock) orphans(skip func(w *lockWorkflow, i int) bool) map[string]bool {
	before := l.reached(func(*lockWorkflow, int) bool { return false })
	after := l.reached(skip)
	out := map[string]bool{}
	for k := range before {
		if !after[k] && l.deps[k] != nil {
			out[k] = true
		}
	}
	return out
}

// lockPinOf is the i-th pin of a workflow.
type lockPinOf struct {
	w *lockWorkflow
	i int
}

// lockPrune is a finding of an entry which nothing uses, and the entry: a workflow, a pin of a workflow or a
// dependency.
type lockPrune struct {
	err       *Error
	desc      string
	workflows []*lockWorkflow
	pins      []lockPinOf
	deps      []string // canonical keys
}

// lockPlan is what one fix deletes: the entries of its findings, and the dependencies which nothing reaches without
// them.
type lockPlan struct {
	workflows map[*lockWorkflow]bool
	pins      map[*lockWorkflow]map[int]bool
	deps      map[string]bool
}

func (l *actionsLock) plan(ps []*lockPrune) *lockPlan {
	p := &lockPlan{workflows: map[*lockWorkflow]bool{}, pins: map[*lockWorkflow]map[int]bool{}, deps: map[string]bool{}}
	for _, x := range ps {
		for _, w := range x.workflows {
			p.workflows[w] = true
		}
		for _, pin := range x.pins {
			if p.pins[pin.w] == nil {
				p.pins[pin.w] = map[int]bool{}
			}
			p.pins[pin.w][pin.i] = true
		}
		for _, k := range x.deps {
			p.deps[k] = true
		}
	}
	for k := range l.orphans(func(w *lockWorkflow, i int) bool { return p.workflows[w] || p.pins[w][i] }) {
		p.deps[k] = true
	}
	return p
}

// entries returns the entries which the plan deletes by collection: "W" for "workflows", "D" for "dependencies" and
// "P" and the path of a workflow for its pins. An entry is the line where it starts.
func (l *actionsLock) entries(p *lockPlan) map[string]map[int]bool {
	out := map[string]map[int]bool{}
	add := func(c string, line int) {
		if out[c] == nil {
			out[c] = map[int]bool{}
		}
		out[c][line] = true
	}
	for w := range p.workflows {
		add("W", w.Line)
	}
	for w, items := range p.pins {
		if p.workflows[w] {
			continue
		}
		for i := range items {
			add("P"+w.Key, w.Pins[i].Line)
		}
	}
	for _, k := range l.depKeyNodes(p.deps) {
		add("D", k.Line)
	}
	return out
}

// size returns the number of entries of the collection which entries names.
func (l *actionsLock) size(c string) int {
	switch {
	case c == "W" && l.workflowsNode != nil:
		return lockCollection{node: l.workflowsNode}.size()
	case c == "D" && l.depsNode != nil:
		return lockCollection{node: l.depsNode}.size()
	case strings.HasPrefix(c, "P"):
		if w := l.workflows[c[1:]]; w != nil && w.value != nil {
			return lockCollection{node: w.value}.size()
		}
	}
	return -1
}

// attachPruneFixes gives the findings of the entries which nothing uses their fixes. Each finding has the fix which
// deletes its entry, so that a tool which applies some of the fixes of a SARIF log gets a valid file, but the fixes
// which together delete every entry of a collection are one: it is written empty ("[]" or "{}") rather than left
// without a value. That fix is attached to the first of its findings, and the others say so.
func (l *actionsLock) attachPruneFixes(e *lockEditor, prunes []*lockPrune) {
	if len(prunes) == 0 || e == nil {
		return
	}
	group := make([]int, len(prunes))
	for i := range group {
		group[i] = i
	}
	root := func(i int) int {
		for group[i] != i {
			i = group[i]
		}
		return i
	}
	members := func() map[int][]*lockPrune {
		m := map[int][]*lockPrune{}
		for i, p := range prunes {
			m[root(i)] = append(m[root(i)], p)
		}
		return m
	}
	// Merge the groups whose fixes together delete every entry of a collection, until no other collection is emptied
	for range prunes {
		deleted := map[string]map[int]bool{}
		touch := map[string][]int{}
		for r, ps := range members() {
			for c, lines := range l.entries(l.plan(ps)) {
				if deleted[c] == nil {
					deleted[c] = map[int]bool{}
				}
				for n := range lines {
					deleted[c][n] = true
				}
				touch[c] = append(touch[c], r)
			}
		}
		merged := false
		for c, rs := range touch {
			if len(rs) < 2 || len(deleted[c]) != l.size(c) {
				continue
			}
			for _, r := range rs[1:] {
				if a, b := root(rs[0]), root(r); a != b {
					group[max(a, b)] = min(a, b)
					merged = true
				}
			}
		}
		if !merged {
			break
		}
	}
	for _, ps := range members() {
		slices.SortFunc(ps, func(a, b *lockPrune) int { return compareErrors(a.err, b.err) })
		fix, why := l.planFix(e, l.plan(ps))
		if fix != nil {
			fix.Description = ps[0].desc
			if len(ps) > 1 {
				fix.Description = "Remove the entries of the lock file which nothing uses"
			}
		}
		for i, p := range ps {
			switch {
			case fix == nil:
				p.err.NoFix = why
			case i == 0:
				p.err.Fix = fix
			default:
				p.err.NoFix = &NoFix{Code: NoFixEditConflict, Reason: fmt.Sprintf("the fix of the finding at line %d removes this entry too: together the entries are all of their list or mapping, which the fix writes empty", ps[0].err.Line)}
			}
		}
	}
}

// planFix returns the fix which deletes what the plan says.
func (l *actionsLock) planFix(e *lockEditor, p *lockPlan) (*Fix, *NoFix) {
	var edits []TextEdit
	add := func(c lockCollection, starts []*yaml.Node) bool {
		if len(starts) == 0 {
			return true
		}
		more, ok := e.delete(c, starts)
		edits = append(edits, more...)
		return ok
	}
	var gone []*yaml.Node
	for _, w := range l.Workflows {
		if p.workflows[w] {
			gone = append(gone, &yaml.Node{Line: w.Line, Column: w.Col})
		}
	}
	if !add(l.workflowsCollection(), gone) {
		return nil, lockNoFixShape
	}
	for _, w := range l.Workflows {
		if p.workflows[w] || len(p.pins[w]) == 0 {
			continue
		}
		var items []*yaml.Node
		for i, pin := range w.Pins {
			if p.pins[w][i] {
				items = append(items, &yaml.Node{Line: pin.Line, Column: pin.Col})
			}
		}
		if !add(w.pinsCollection(), items) {
			return nil, lockNoFixShape
		}
	}
	if !add(l.depsCollection(), l.depKeyNodes(p.deps)) {
		return nil, lockNoFixShape
	}
	if len(edits) == 0 {
		return nil, lockNoFixShape
	}
	return &Fix{Description: "Remove the entries of the lock file which nothing uses", Edits: edits}, nil
}
