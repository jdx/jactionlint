package runner

import (
	"sort"

	"github.com/jdx/jactionlint/v2"
)

// combo is one combination of a matrix: the values that `matrix.*` holds in the job. Keys keeps the order in which
// the workflow declares them (the rows first, then the keys that an include entry adds), which is the order of the
// values in the default job name.
type combo struct {
	Keys   []string
	Values map[string]any
}

func (c *combo) set(k string, v any) {
	if _, ok := c.Values[k]; !ok {
		c.Keys = append(c.Keys, k)
	}
	c.Values[k] = v
}

func (c *combo) clone() *combo {
	n := &combo{Keys: append([]string(nil), c.Keys...), Values: make(map[string]any, len(c.Values))}
	for k, v := range c.Values {
		n.Values[k] = v
	}
	return n
}

// nameValues lists the scalar values of the combination, which is what GitHub puts in the default name of a job.
func (c *combo) nameValues() []string {
	var vs []string
	for _, k := range c.Keys {
		if s, ok := formatScalar(c.Values[k]); ok {
			vs = append(vs, s)
		}
	}
	return vs
}

// matrixKeyLess orders by position in the source, so that declaration order survives the maps of the AST.
func posLess(a, b *jactionlint.Pos) bool {
	if a == nil || b == nil {
		return a != nil
	}
	return a.IsBefore(b)
}

type entry struct {
	key string
	pos *jactionlint.Pos
	val any
}

func assignsOf(c *jactionlint.MatrixCombination) []entry {
	es := make([]entry, 0, len(c.Assigns))
	for k, a := range c.Assigns {
		name := k
		var pos *jactionlint.Pos
		if a.Key != nil {
			name, pos = a.Key.Value, a.Key.Pos
		}
		es = append(es, entry{name, pos, rawToAny(a.Value)})
	}
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].pos != nil && es[j].pos != nil {
			return es[i].pos.IsBefore(es[j].pos)
		}
		return es[i].key < es[j].key
	})
	return es
}

// expandMatrix returns the combinations that a strategy.matrix produces, the way the runner builds them: the
// product of the rows, minus the combinations which an exclude entry matches, then every include entry. dynamic is
// the source of an expression when the matrix cannot be known without running (a fromJSON of a job output).
func expandMatrix(m *jactionlint.Matrix) (combos []*combo, dynamic string) {
	if m == nil {
		return []*combo{{Values: map[string]any{}}}, ""
	}
	if m.Expression != nil {
		return nil, m.Expression.Value
	}
	if m.Include != nil && m.Include.Expression != nil {
		return nil, m.Include.Expression.Value
	}
	if m.Exclude != nil && m.Exclude.Expression != nil {
		return nil, m.Exclude.Expression.Value
	}
	rows := make([]*jactionlint.MatrixRow, 0, len(m.Rows))
	for _, r := range m.Rows {
		if r.Expression != nil {
			return nil, r.Expression.Value
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool { return posLess(rows[i].Name.Pos, rows[j].Name.Pos) })
	for _, c := range includeList(m.Include) {
		if c.Expression != nil {
			return nil, c.Expression.Value
		}
	}

	original := map[string]bool{}
	for _, r := range rows {
		original[r.Name.Value] = true
	}
	combos = product(rows)
	if len(rows) == 0 {
		combos = nil
	}

	if m.Exclude != nil {
		var kept []*combo
	next:
		for _, c := range combos {
			for _, x := range m.Exclude.Combinations {
				if excludes(c, assignsOf(x)) {
					continue next
				}
			}
			kept = append(kept, c)
		}
		combos = kept
	}

	// An include entry joins the combinations of the rows, never the ones that other include entries made.
	base := combos
	for _, inc := range includeList(m.Include) {
		es := assignsOf(inc)
		added := false
		for _, c := range base {
			ok := true
			for _, e := range es {
				if original[e.key] && !equalValue(c.Values[e.key], e.val) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			added = true
			for _, e := range es {
				if !original[e.key] {
					c.set(e.key, e.val)
				}
			}
		}
		if !added {
			c := &combo{Values: map[string]any{}}
			for _, e := range es {
				c.set(e.key, e.val)
			}
			combos = append(combos, c)
		}
	}
	return combos, ""
}

func includeList(c *jactionlint.MatrixCombinations) []*jactionlint.MatrixCombination {
	if c == nil {
		return nil
	}
	return c.Combinations
}

func product(rows []*jactionlint.MatrixRow) []*combo {
	if len(rows) == 0 {
		return nil
	}
	out := []*combo{{Values: map[string]any{}}}
	for _, r := range rows {
		var next []*combo
		for _, c := range out {
			for _, v := range r.Values {
				n := c.clone()
				n.set(r.Name.Value, rawToAny(v))
				next = append(next, n)
			}
		}
		out = next
	}
	return out
}

func excludes(c *combo, x []entry) bool {
	if len(x) == 0 {
		return false
	}
	for _, e := range x {
		v, ok := c.Values[e.key]
		if !ok || !matchValue(v, e.val) {
			return false
		}
	}
	return true
}
