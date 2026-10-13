package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// maxLevels is the number of levels of objects of a payload which are typed. The payload is level 1, the objects
// in it (pull_request) are level 2 and the objects in them (pull_request.head) are level 3. An object of a deeper
// level is untyped, so that an unknown property of it is not reported. Keep it small: the schemas lag behind
// the payloads GitHub sends, and the deeper an object is, the more likely a property of it is missing.
const maxLevels = 3

// looseNames are properties which are untyped wherever they appear: their content is arbitrary or changes with
// the action ("changes" holds the old values of what was edited).
var looseNames = map[string]bool{
	"changes":        true,
	"client_payload": true,
	"inputs":         true,
	"payload":        true,
}

// accountNames are properties which hold an account, an app, an installation or a licence. They are known, but
// what is in them is not: GitHub adds a property to these objects every now and then, in every payload at once, and
// a workflow which reads a new one would be reported until the sources know it. Nobody mistypes what is in them
// often enough to make up for it.
var accountNames = map[string]bool{
	"actor":                    true,
	"app":                      true,
	"assignee":                 true,
	"assignees":                true,
	"creator":                  true,
	"enterprise":               true,
	"installation":             true,
	"license":                  true,
	"merged_by":                true,
	"organization":             true,
	"owner":                    true,
	"performed_via_github_app": true,
	"pusher":                   true,
	"requested_reviewer":       true,
	"requested_reviewers":      true,
	"sender":                   true,
	"triggering_actor":         true,
	"user":                     true,
}

type kind int

const (
	kNull   kind = iota // the schema only allows null
	kAny                // a scalar or something untyped
	kObject             // an object with known properties
	kArray
)

// node is a shape of a value.
type node struct {
	kind  kind
	props map[string]*node // kObject
	elem  *node            // kArray; nil for an array of anything
	src   string           // the name of the schema the node comes from, for naming shared objects
}

var anyNode = &node{kind: kAny}

// merge returns the shape of a value which has the shape of a or of b. A property is known when it is in a or in b.
func merge(a, b *node) *node {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.kind == kNull:
		return b
	case b.kind == kNull:
		return a
	case a.kind == kObject && b.kind == kObject:
		ret := &node{kind: kObject, props: map[string]*node{}}
		if a.src == b.src {
			ret.src = a.src
		}
		for k, v := range a.props {
			ret.props[k] = v
		}
		for k, v := range b.props {
			ret.props[k] = merge(ret.props[k], v)
		}
		return ret
	case a.kind == kArray && b.kind == kArray:
		ret := &node{kind: kArray}
		if a.elem != nil && b.elem != nil {
			ret.elem = merge(a.elem, b.elem)
		}
		return ret
	}
	return anyNode
}

// resolver finds the schema a reference points to.
type resolver interface {
	// resolve returns the schema which ref points to from the schema named base, and the name of that schema.
	resolve(base, ref string) (map[string]any, string, error)
}

type memoKey struct {
	name  string
	level int
}

type converter struct {
	res   resolver
	memo  map[memoKey]*node
	stack map[memoKey]bool
}

func newConverter(res resolver) *converter {
	return &converter{res: res, memo: map[memoKey]*node{}, stack: map[memoKey]bool{}}
}

// convertRef converts the schema which ref points to.
func (c *converter) convertRef(base, ref string, level int) (*node, error) {
	s, name, err := c.res.resolve(base, ref)
	if err != nil {
		return nil, err
	}
	return c.convertNamed(s, name, level)
}

func (c *converter) convertNamed(s map[string]any, name string, level int) (*node, error) {
	key := memoKey{name, level}
	if n, ok := c.memo[key]; ok {
		return n, nil
	}
	if c.stack[key] {
		return nil, fmt.Errorf("the schema %s refers to itself", name)
	}
	c.stack[key] = true
	defer delete(c.stack, key)
	n, err := c.convert(s, name, level)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	cp := *n
	stem := path.Base(name)
	cp.src = strings.TrimSuffix(stem, ".schema.json")
	c.memo[key] = &cp
	return &cp, nil
}

// convert makes the shape of a schema. base is the name of the schema which holds it, level is the level of the
// object which the schema describes.
func (c *converter) convert(s map[string]any, base string, level int) (*node, error) {
	if ref, ok := s["$ref"].(string); ok {
		return c.convertRef(base, ref, level)
	}

	var ret *node
	for _, k := range []string{"allOf", "oneOf", "anyOf"} {
		l, ok := s[k].([]any)
		if !ok {
			continue
		}
		var m *node
		for _, e := range l {
			es, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s has an element which is not an object", k)
			}
			n, err := c.convert(es, base, level)
			if err != nil {
				return nil, err
			}
			m = merge(m, n)
		}
		ret = merge(ret, m)
	}

	types := map[string]bool{}
	switch t := s["type"].(type) {
	case string:
		types[t] = true
	case []any:
		for _, e := range t {
			if e, ok := e.(string); ok {
				types[e] = true
			}
		}
	}

	var own *node
	switch {
	case types["object"] || s["properties"] != nil:
		props, _ := s["properties"].(map[string]any)
		// An object which allows other properties (a map, or something arbitrary) has no known properties.
		if ap, ok := s["additionalProperties"]; ok && ap != false || props == nil || level > maxLevels {
			own = anyNode
			break
		}
		own = &node{kind: kObject, props: map[string]*node{}}
		for k, v := range props {
			vs, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("property %q is not an object", k)
			}
			n, err := c.convert(vs, base, level+1)
			if err != nil {
				return nil, fmt.Errorf("property %q: %w", k, err)
			}
			// Property names of expressions are case insensitive and the checker looks them up in lower case.
			k = strings.ToLower(k)
			own.props[k] = merge(own.props[k], n)
		}
	case types["array"]:
		own = &node{kind: kArray}
		if items, ok := s["items"].(map[string]any); ok {
			n, err := c.convert(items, base, level)
			if err != nil {
				return nil, fmt.Errorf("items: %w", err)
			}
			if n.kind != kAny && n.kind != kNull {
				own.elem = n
			}
		}
	case len(types) == 1 && types["null"]:
		own = &node{kind: kNull}
	case len(types) > 0:
		own = anyNode
	}
	switch {
	case ret == nil && own == nil:
		return anyNode, nil
	case ret == nil:
		return own, nil
	case own == nil:
		return ret, nil
	}
	return merge(ret, own), nil
}

// isAccount reports whether an object is the account of a user, a bot or an organization, or an app: GitHub names
// such objects differently in many places (author, dismisser, uploader, ...), and adds properties to all of them at once.
func isAccount(n *node) bool {
	_, login := n.props["login"]
	_, avatar := n.props["avatar_url"]
	_, slug := n.props["slug"]
	_, owner := n.props["owner"]
	_, perms := n.props["permissions"]
	return login && avatar || slug && owner && perms
}

// prune makes the loose names and the accounts untyped. It returns a new tree.
func prune(n *node) *node {
	switch n.kind {
	case kObject:
		if isAccount(n) {
			return anyNode
		}
		ret := &node{kind: kObject, props: make(map[string]*node, len(n.props)), src: n.src}
		for k, v := range n.props {
			if looseNames[k] || accountNames[k] {
				ret.props[k] = anyNode
			} else {
				ret.props[k] = prune(v)
			}
		}
		return ret
	case kArray:
		if n.elem == nil {
			return n
		}
		e := prune(n.elem)
		if e.kind == kAny {
			e = nil
		}
		return &node{kind: kArray, elem: e}
	}
	return n
}

// addExtra adds the properties of extraProps to the objects of the tree of an event. The tree is changed.
func addExtra(n *node, event, objPath string) {
	switch n.kind {
	case kArray:
		if n.elem != nil {
			addExtra(n.elem, event, objPath)
		}
	case kObject:
		for _, key := range []string{event + ":" + objPath, "*:" + objPath} {
			for _, p := range extraProps[key] {
				if _, ok := n.props[p]; !ok {
					n.props[p] = anyNode
				}
			}
		}
		for k, v := range n.props {
			sub := k
			if objPath != "" {
				sub = objPath + "." + k
			}
			addExtra(v, event, sub)
		}
	}
}

// octokitFiles is the directory payload-schemas/api.github.com of octokit/webhooks, by path relative to it.
// The schema of an event is a file in the directory of the event, and it refers to schemas in common/.
type octokitFiles map[string][]byte

func (f octokitFiles) resolve(base, ref string) (map[string]any, string, error) {
	p := path.Join(path.Dir(base), ref)
	if strings.HasPrefix(ref, "common/") {
		p = ref
	}
	b, ok := f[p]
	if !ok {
		return nil, "", fmt.Errorf("the schema %s does not exist", p)
	}
	s, err := parseJSON(b)
	if err != nil {
		return nil, "", fmt.Errorf("could not parse %s: %w", p, err)
	}
	return s, p, nil
}

// events returns the names of the directories of the events.
func (f octokitFiles) events() []string {
	seen := map[string]bool{}
	for p := range f {
		if dir, _, ok := strings.Cut(p, "/"); ok && dir != "common" {
			seen[dir] = true
		}
	}
	ret := make([]string, 0, len(seen))
	for d := range seen {
		ret = append(ret, d)
	}
	sort.Strings(ret)
	return ret
}

// payload returns the shape of the payload of an event: the union of the payloads of all its actions.
func (f octokitFiles) payload(c *converter, event string) (*node, error) {
	var names []string
	for p := range f {
		if dir, _, ok := strings.Cut(p, "/"); ok && dir == event && strings.HasSuffix(p, ".schema.json") {
			names = append(names, p)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no schema was found for the event %s", event)
	}
	sort.Strings(names)
	var ret *node
	for _, p := range names {
		s, name, err := f.resolve("", p)
		if err != nil {
			return nil, err
		}
		n, err := c.convertNamed(s, name, 1)
		if err != nil {
			return nil, err
		}
		if n.kind != kObject {
			return nil, fmt.Errorf("the schema %s is not an object", p)
		}
		ret = merge(ret, n)
	}
	return ret, nil
}

// openAPI is the OpenAPI description of the REST API of GitHub, which has the webhooks as x-webhooks.
type openAPI map[string]any

func (o openAPI) resolve(_, ref string) (map[string]any, string, error) {
	rest, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return nil, "", fmt.Errorf("the reference %q is not local", ref)
	}
	var cur any = map[string]any(o)
	for _, part := range strings.Split(rest, "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("the reference %q does not exist", ref)
		}
		cur = m[part]
	}
	s, ok := cur.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("the reference %q does not exist", ref)
	}
	return s, ref, nil
}

// webhooks returns the schemas of the payloads of the webhooks by the name of the event.
func (o openAPI) webhooks() (map[string][]map[string]any, error) {
	hooks, ok := o["x-webhooks"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the description has no x-webhooks")
	}
	ret := map[string][]map[string]any{}
	names := make([]string, 0, len(hooks))
	for n := range hooks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		h, _ := hooks[n].(map[string]any)
		post, _ := h["post"].(map[string]any)
		gh, _ := post["x-github"].(map[string]any)
		event, _ := gh["subcategory"].(string)
		body, _ := post["requestBody"].(map[string]any)
		content, _ := body["content"].(map[string]any)
		js, _ := content["application/json"].(map[string]any)
		schema, _ := js["schema"].(map[string]any)
		if event == "" || schema == nil {
			return nil, fmt.Errorf("the webhook %s has no event or no schema", n)
		}
		ret[event] = append(ret[event], schema)
	}
	return ret, nil
}
