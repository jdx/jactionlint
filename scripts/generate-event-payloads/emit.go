package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A shape is a node whose objects are shared when equal: equal objects have the same pointer.
type shape struct {
	kind  kind
	names []string // kObject: the sorted names of the properties
	subs  []*shape // kObject: their shapes; kArray: the element, if any
	key   string   // the canonical text, which identifies the shape
	src   string
	refs  int // how many objects use it
	name  string
}

type interner struct {
	byKey map[string]*shape
}

func newInterner() *interner {
	return &interner{byKey: map[string]*shape{}}
}

// intern returns the shape of a node. hint names the object when its schema does not: it is the name of the
// property which holds the object.
func (in *interner) intern(n *node, hint string) *shape {
	switch n.kind {
	case kObject:
		names := make([]string, 0, len(n.props))
		for k := range n.props {
			names = append(names, k)
		}
		sort.Strings(names)
		s := &shape{kind: kObject, names: names, src: n.src}
		if s.src == "" {
			s.src = hint
		}
		var b strings.Builder
		b.WriteString("{")
		for i, k := range names {
			sub := in.intern(n.props[k], k)
			s.subs = append(s.subs, sub)
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(strconv.Quote(k))
			if sub.kind != kAny {
				b.WriteString(":")
				b.WriteString(sub.key)
			}
		}
		b.WriteString("}")
		s.key = b.String()
		if old, ok := in.byKey[s.key]; ok {
			if old.src == "" {
				old.src = s.src
			}
			return old
		}
		in.byKey[s.key] = s
		return s
	case kArray:
		s := &shape{kind: kArray}
		if n.elem != nil {
			sub := in.intern(n.elem, hint)
			s.subs = []*shape{sub}
			s.key = "[" + sub.key + "]"
		} else {
			s.key = "[]"
		}
		return s
	}
	return &shape{kind: kAny, key: "*"}
}

// countRefs counts the parents of every object shape once per distinct parent.
func countRefs(s *shape, seen map[*shape]bool) {
	if s.kind == kObject {
		s.refs++
		if seen[s] {
			return
		}
		seen[s] = true
	}
	for _, sub := range s.subs {
		countRefs(sub, seen)
	}
}

const reserved = "{}[],:@=\"` \t\n"

func checkName(k string) error {
	if k == "" || strings.ContainsAny(k, reserved) {
		return fmt.Errorf("the property name %q cannot be written in the output", k)
	}
	return nil
}

// emit writes the text of a shape. Shared objects are written as references.
func emit(b *strings.Builder, s *shape, top bool) error {
	switch s.kind {
	case kObject:
		if !top && s.name != "" {
			b.WriteString("@" + s.name)
			return nil
		}
		b.WriteString("{")
		for i, k := range s.names {
			if err := checkName(k); err != nil {
				return err
			}
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(k)
			if sub := s.subs[i]; sub.kind != kAny {
				b.WriteString(":")
				if err := emit(b, sub, false); err != nil {
					return err
				}
			}
		}
		b.WriteString("}")
	case kArray:
		b.WriteString("[")
		if len(s.subs) > 0 {
			if err := emit(b, s.subs[0], false); err != nil {
				return err
			}
		}
		b.WriteString("]")
	}
	return nil
}

const lineWidth = 110

// wrap breaks a text after commas so that no line gets much longer than lineWidth. The text has no spaces, and
// the parser of the output ignores white space.
func wrap(s, indent string) string {
	var b strings.Builder
	col := 0
	for i := 0; i < len(s); i++ {
		b.WriteByte(s[i])
		col++
		if s[i] == ',' && col >= lineWidth {
			b.WriteString("\n" + indent)
			col = 0
		}
	}
	return b.String()
}

// nameShared names the objects which are used more than once and long enough to be worth a name. The name is the
// name of the schema of the object, and a number when several objects have the same name.
func nameShared(in *interner) []*shape {
	var shared []*shape
	for _, s := range in.byKey {
		if s.refs > 1 && len(s.key) > 60 {
			shared = append(shared, s)
		}
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i].key < shared[j].key })
	used := map[string]bool{}
	for _, s := range shared {
		base := strings.NewReplacer("-", "_", ".", "_").Replace(s.src)
		if base == "" {
			h := sha256.Sum256([]byte(s.key))
			base = "x" + hex.EncodeToString(h[:3])
		}
		name := base
		for i := 2; used[name]; i++ {
			name = base + strconv.Itoa(i)
		}
		used[name] = true
		s.name = name
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i].name < shared[j].name })
	return shared
}
