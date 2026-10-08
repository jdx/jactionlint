package jactionlint

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// This file checks a fix before it is written. After the edits of a pass are applied, the result
// must be valid YAML and the parsed document must equal the original document except for the places
// the edits touch. A fixer which builds YAML by hand and gets a quote or an indentation wrong then
// fails here, and the file is not written.
//
// The check compares the parsed trees, not the text:
//
//   - Comments, the order of the keys of a mapping and the way a scalar is written (plain, quoted,
//     block) do not matter, but its type and value do: 'true' and true differ.
//   - Mappings are matched by key and sequences by the longest common subsequence of their items, so
//     inserting a step is one added item and not a change of every step after it.
//   - Every difference must lie where an edit is: the old node must overlap the old range of an edit,
//     or the new node must overlap the new range of an edit.
//
// Limits: the YAML parser does not tell where a node ends, so the extent of a node is taken to run up to
// the start of the next node. That is generous: a difference next to an edit is accepted even if a
// different edit caused it. The check cannot tell that an edit changed the meaning of the document in
// the place it was meant to (a wrong value in the right key), only that it changed something somewhere
// else or broke the syntax. Line breaks other than LF, CRLF and CR (NEL, LS, PS) make the offsets
// approximate. Anchors are compared by the aliases which use them, not by name.

// parseYAMLDocs parses all documents of the source.
func parseYAMLDocs(src []byte) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var docs []*yaml.Node
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &n)
	}
}

// nodeTable maps the nodes of parsed documents to byte offsets of the source.
type nodeTable struct {
	src       []byte
	lineStart []int
	start     map[*yaml.Node]int
	// end is the offset where the subtree of the node ends: the start of the next node in document
	// order which is not in the subtree, or the end of the source.
	end    map[*yaml.Node]int
	inFlow map[*yaml.Node]bool
}

func newNodeTable(src []byte, docs []*yaml.Node) *nodeTable {
	t := &nodeTable{src: src, start: map[*yaml.Node]int{}, end: map[*yaml.Node]int{}, inFlow: map[*yaml.Node]bool{}}
	t.lineStart = append(t.lineStart, 0)
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			t.lineStart = append(t.lineStart, i+1)
		case '\r':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
			t.lineStart = append(t.lineStart, i+1)
		}
	}
	var order []*yaml.Node
	subEnd := map[*yaml.Node]int{} // index in order after the last node of the subtree
	var walk func(n *yaml.Node, flow bool) bool
	walk = func(n *yaml.Node, flow bool) bool {
		if n.Kind != yaml.DocumentNode {
			off, ok := t.offset(n.Line, n.Column)
			if !ok {
				return false
			}
			t.start[n] = off
			t.inFlow[n] = flow
			order = append(order, n)
		}
		if n.Kind == yaml.AliasNode {
			subEnd[n] = len(order)
			return true
		}
		f := flow || n.Style&yaml.FlowStyle != 0
		for _, c := range n.Content {
			if !walk(c, f) {
				return false
			}
		}
		subEnd[n] = len(order)
		return true
	}
	for _, d := range docs {
		if !walk(d, false) {
			return nil
		}
	}
	var fill func(n *yaml.Node)
	fill = func(n *yaml.Node) {
		if n.Kind != yaml.DocumentNode {
			if i := subEnd[n]; i < len(order) {
				t.end[n] = max(t.start[order[i]], t.start[n])
			} else {
				t.end[n] = len(src)
			}
		}
		if n.Kind == yaml.AliasNode {
			return
		}
		for _, c := range n.Content {
			fill(c)
		}
	}
	for _, d := range docs {
		fill(d)
	}
	return t
}

// offset converts a 1-based line and a 1-based column in code points to a byte offset.
func (t *nodeTable) offset(line, col int) (int, bool) {
	if line < 1 || line > len(t.lineStart) || col < 1 {
		return 0, false
	}
	off := t.lineStart[line-1]
	for i := 1; i < col; i++ {
		if off >= len(t.src) || t.src[off] == '\n' || t.src[off] == '\r' {
			return 0, false
		}
		_, w := utf8.DecodeRune(t.src[off:])
		off += w
	}
	return off, true
}

// extent returns the offsets from the start of the first node to the end of the subtree of the last.
func (t *nodeTable) extent(first, last *yaml.Node) (int, int) {
	return t.start[first], t.end[last]
}

// eachScalar calls fn for the scalar nodes in the tree.
func (t *nodeTable) eachScalar(n *yaml.Node, fn func(*yaml.Node)) {
	if n.Kind == yaml.ScalarNode {
		fn(n)
	}
	if n.Kind == yaml.AliasNode {
		return
	}
	for _, c := range n.Content {
		t.eachScalar(c, fn)
	}
}

// treeDiff is one difference between the old and the new tree. Old is a range of nodes of the old
// tree (nil when the difference is an addition) and New the same for the new tree.
type treeDiff struct {
	path               string
	oldFirst, oldLast  *yaml.Node
	newFirst, newLast  *yaml.Node
	oldIsKey, newIsKey bool
}

type differ struct {
	hashes map[*yaml.Node]string
	diffs  []treeDiff
}

func (d *differ) hash(n *yaml.Node) string {
	if h, ok := d.hashes[n]; ok {
		return h
	}
	var b strings.Builder
	switch n.Kind {
	case yaml.ScalarNode:
		fmt.Fprintf(&b, "s%s\x00%s", n.ShortTag(), n.Value)
	case yaml.AliasNode:
		fmt.Fprintf(&b, "a%s", n.Value)
	case yaml.SequenceNode:
		b.WriteString("q")
		for _, c := range n.Content {
			b.WriteString(d.hash(c))
			b.WriteByte(0x01)
		}
	case yaml.MappingNode:
		b.WriteString("m")
		var entries []string
		for i := 0; i+1 < len(n.Content); i += 2 {
			entries = append(entries, d.hash(n.Content[i])+"\x02"+d.hash(n.Content[i+1]))
		}
		sort.Strings(entries)
		for _, e := range entries {
			b.WriteString(e)
			b.WriteByte(0x01)
		}
	case yaml.DocumentNode:
		b.WriteString("d")
		for _, c := range n.Content {
			b.WriteString(d.hash(c))
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	h := string(sum[:])
	d.hashes[n] = h
	return h
}

func (d *differ) add(path string, f treeDiff) {
	f.path = path
	d.diffs = append(d.diffs, f)
}

// compare records the differences between two nodes at the same place.
func (d *differ) compare(path string, a, b *yaml.Node) {
	if d.hash(a) == d.hash(b) {
		return
	}
	if a.Kind != b.Kind {
		d.add(path, treeDiff{oldFirst: a, oldLast: a, newFirst: b, newLast: b})
		return
	}
	switch a.Kind {
	case yaml.DocumentNode:
		if len(a.Content) == 1 && len(b.Content) == 1 {
			d.compare(path, a.Content[0], b.Content[0])
			return
		}
	case yaml.MappingNode:
		d.compareMappings(path, a, b)
		return
	case yaml.SequenceNode:
		d.compareSequences(path, a, b)
		return
	}
	d.add(path, treeDiff{oldFirst: a, oldLast: a, newFirst: b, newLast: b})
}

func (d *differ) compareMappings(path string, a, b *yaml.Node) {
	type entry struct{ k, v *yaml.Node }
	index := func(n *yaml.Node) (map[string]entry, []string) {
		m := map[string]entry{}
		var order []string
		seen := map[string]int{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := d.hash(n.Content[i])
			seen[k]++
			id := fmt.Sprintf("%s#%d", k, seen[k])
			m[id] = entry{n.Content[i], n.Content[i+1]}
			order = append(order, id)
		}
		return m, order
	}
	am, aorder := index(a)
	bm, border := index(b)
	for _, id := range aorder {
		ae := am[id]
		sub := path + "." + keyText(ae.k)
		be, ok := bm[id]
		if !ok {
			d.add(sub, treeDiff{oldFirst: ae.k, oldLast: ae.v, oldIsKey: true})
			continue
		}
		d.compare(sub, ae.v, be.v)
	}
	for _, id := range border {
		if _, ok := am[id]; !ok {
			be := bm[id]
			d.add(path+"."+keyText(be.k), treeDiff{newFirst: be.k, newLast: be.v, newIsKey: true})
		}
	}
}

func keyText(k *yaml.Node) string {
	if k.Kind == yaml.ScalarNode {
		return k.Value
	}
	return "?"
}

func (d *differ) compareSequences(path string, a, b *yaml.Node) {
	ac, bc := a.Content, b.Content
	pre := 0
	for pre < len(ac) && pre < len(bc) && d.hash(ac[pre]) == d.hash(bc[pre]) {
		pre++
	}
	suf := 0
	for suf < len(ac)-pre && suf < len(bc)-pre && d.hash(ac[len(ac)-1-suf]) == d.hash(bc[len(bc)-1-suf]) {
		suf++
	}
	am, bm := ac[pre:len(ac)-suf], bc[pre:len(bc)-suf]
	// Longest common subsequence of the middle, by dynamic programming
	n, m := len(am), len(bm)
	type gap struct{ a, b []int }
	var gaps []gap
	if n*m > 4_000_000 {
		gaps = []gap{{a: seq(0, n), b: seq(0, m)}}
	} else {
		l := make([][]int, n+1)
		for i := range l {
			l[i] = make([]int, m+1)
		}
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if d.hash(am[i]) == d.hash(bm[j]) {
					l[i][j] = l[i+1][j+1] + 1
				} else {
					l[i][j] = max(l[i+1][j], l[i][j+1])
				}
			}
		}
		i, j := 0, 0
		cur := gap{}
		for i < n || j < m {
			switch {
			case i < n && j < m && d.hash(am[i]) == d.hash(bm[j]):
				if len(cur.a) > 0 || len(cur.b) > 0 {
					gaps = append(gaps, cur)
					cur = gap{}
				}
				i++
				j++
			case j >= m || (i < n && l[i+1][j] >= l[i][j+1]):
				cur.a = append(cur.a, i)
				i++
			default:
				cur.b = append(cur.b, j)
				j++
			}
		}
		if len(cur.a) > 0 || len(cur.b) > 0 {
			gaps = append(gaps, cur)
		}
	}
	for _, g := range gaps {
		k := min(len(g.a), len(g.b))
		for x := 0; x < k; x++ {
			ia, ib := g.a[x], g.b[x]
			d.compare(fmt.Sprintf("%s[%d]", path, pre+ia), am[ia], bm[ib])
		}
		for _, ia := range g.a[k:] {
			d.add(fmt.Sprintf("%s[%d]", path, pre+ia), treeDiff{oldFirst: am[ia], oldLast: am[ia]})
		}
		for _, ib := range g.b[k:] {
			d.add(fmt.Sprintf("%s[%d]", path, pre+ib), treeDiff{newFirst: bm[ib], newLast: bm[ib]})
		}
	}
}

func seq(from, to int) []int {
	var s []int
	for i := from; i < to; i++ {
		s = append(s, i)
	}
	return s
}

// verifyFix checks the source produced by applying the edits to the old source. See the comment at the
// top of the file for what is checked.
func verifyFix(oldSrc, newSrc []byte, edits []TextEdit) error {
	if !utf8.Valid(newSrc) {
		return errors.New("the result is not valid UTF-8")
	}
	newDocs, err := parseYAMLDocs(newSrc)
	if err != nil {
		return fmt.Errorf("the result is not valid YAML: %s", strings.ReplaceAll(err.Error(), "\n", " "))
	}
	oldDocs, err := parseYAMLDocs(oldSrc)
	if err != nil {
		return fmt.Errorf("the original is not valid YAML: %s", strings.ReplaceAll(err.Error(), "\n", " "))
	}
	if len(oldDocs) != len(newDocs) {
		return fmt.Errorf("the number of YAML documents changed from %d to %d", len(oldDocs), len(newDocs))
	}
	oldT, newT := newNodeTable(oldSrc, oldDocs), newNodeTable(newSrc, newDocs)
	if oldT == nil || newT == nil {
		return errors.New("cannot map the YAML nodes to the source")
	}

	// The ranges of the edits in the old and the new source
	sorted := slices.Clone(edits)
	slices.SortFunc(sorted, func(a, b TextEdit) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		return a.End - b.End
	})
	type span struct{ oldS, oldE, newS, newE int }
	spans := make([]span, 0, len(sorted))
	delta := 0
	for _, e := range sorted {
		ns := e.Start + delta
		spans = append(spans, span{e.Start, e.End, ns, ns + len(e.NewText)})
		delta += len(e.NewText) - (e.End - e.Start)
	}
	touches := func(s, e int, lo, hi int) bool { return lo < e && s <= hi }

	d := &differ{hashes: map[*yaml.Node]string{}}
	for i := range oldDocs {
		d.compare(fmt.Sprintf("document %d", i+1), oldDocs[i], newDocs[i])
	}
	for _, df := range d.diffs {
		claimed := false
		if df.oldFirst != nil {
			s, e := oldT.extent(df.oldFirst, df.oldLast)
			for _, sp := range spans {
				if touches(s, e, sp.oldS, sp.oldE) {
					claimed = true
					break
				}
			}
		}
		if !claimed && df.newFirst != nil {
			s, e := newT.extent(df.newFirst, df.newLast)
			for _, sp := range spans {
				if touches(s, e, sp.newS, sp.newE) {
					claimed = true
					break
				}
			}
		}
		if !claimed {
			line := 0
			if n := firstNonNil(df.oldFirst, df.newFirst); n != nil {
				line = n.Line
			}
			return fmt.Errorf("the result differs from the original at %s (line %d), away from the edits", strings.TrimPrefix(df.path, "document 1"), line)
		}
	}
	return nil
}

func firstNonNil(ns ...*yaml.Node) *yaml.Node {
	for _, n := range ns {
		if n != nil {
			return n
		}
	}
	return nil
}
