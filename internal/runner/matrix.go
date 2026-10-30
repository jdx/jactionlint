package runner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jdx/jactionlint/v2"
	"github.com/jdx/jactionlint/v2/internal/runner/eval"
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

// posLess orders by position in the source, so that declaration order survives the maps of the AST.
func posLess(a, b *jactionlint.Pos) bool {
	if a == nil || b == nil {
		return a != nil
	}
	return a.IsBefore(b)
}

type kv struct {
	key string
	val any
}

type rowSpec struct {
	name string
	vals []any
}

// matrixSpec is a matrix with every expression resolved.
type matrixSpec struct {
	rows    []rowSpec
	include [][]kv
	exclude [][]kv
}

func sortedKV(m map[string]any) []kv {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]kv, len(keys))
	for i, k := range keys {
		out[i] = kv{k, m[k]}
	}
	return out
}

func assignsOf(c *jactionlint.MatrixCombination) []kv {
	type entry struct {
		kv
		pos *jactionlint.Pos
	}
	es := make([]entry, 0, len(c.Assigns))
	for k, a := range c.Assigns {
		name := k
		var pos *jactionlint.Pos
		if a.Key != nil {
			name, pos = a.Key.Value, a.Key.Pos
		}
		es = append(es, entry{kv{name, rawToAny(a.Value)}, pos})
	}
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].pos != nil && es[j].pos != nil {
			return es[i].pos.IsBefore(es[j].pos)
		}
		return es[i].key < es[j].key
	})
	out := make([]kv, len(es))
	for i, e := range es {
		out[i] = e.kv
	}
	return out
}

// resolver evaluates the expressions of a matrix. It is nil when nothing can be evaluated.
type resolver func(s *jactionlint.String) (any, bool, error)

func envResolver(env *eval.Env) resolver {
	if env == nil {
		return nil
	}
	return func(s *jactionlint.String) (any, bool, error) {
		if !env.CanEval(s.Value) {
			return nil, false, nil
		}
		v, err := env.Value(s.Value)
		return v, true, err
	}
}

func asList(v any) ([]any, error) {
	switch v := v.(type) {
	case []any:
		return v, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("expected a list, got %s", valueText(v))
}

func asEntries(v any) ([][]kv, error) {
	l, err := asList(v)
	if err != nil {
		return nil, err
	}
	var out [][]kv
	for _, e := range l {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected a list of mappings, got %s", valueText(e))
		}
		out = append(out, sortedKV(m))
	}
	return out, nil
}

func specFromMap(m map[string]any) (*matrixSpec, error) {
	sp := &matrixSpec{}
	var err error
	for _, e := range sortedKV(m) {
		switch strings.ToLower(e.key) {
		case "include":
			sp.include, err = asEntries(e.val)
		case "exclude":
			sp.exclude, err = asEntries(e.val)
		default:
			var vals []any
			vals, err = asList(e.val)
			sp.rows = append(sp.rows, rowSpec{e.key, vals})
		}
		if err != nil {
			return nil, fmt.Errorf("matrix %s: %w", e.key, err)
		}
	}
	return sp, nil
}

// specOf resolves a matrix of the workflow. dynamic is the source of an expression that cannot be resolved yet.
func specOf(m *jactionlint.Matrix, res resolver) (sp *matrixSpec, dynamic string, err error) {
	resolve := func(s *jactionlint.String) (v any, ok bool, err error) {
		if res == nil {
			return nil, false, nil
		}
		return res(s)
	}
	if m.Expression != nil {
		v, ok, err := resolve(m.Expression)
		if !ok || err != nil {
			return nil, m.Expression.Value, err
		}
		mm, isMap := v.(map[string]any)
		if !isMap {
			return nil, "", fmt.Errorf("matrix: the expression %s is not a mapping", strings.TrimSpace(m.Expression.Value))
		}
		sp, err := specFromMap(mm)
		return sp, "", err
	}
	sp = &matrixSpec{}
	type namedRow struct {
		name string
		pos  *jactionlint.Pos
		*jactionlint.MatrixRow
	}
	rows := make([]namedRow, 0, len(m.Rows))
	for key, r := range m.Rows {
		nr := namedRow{name: key, MatrixRow: r}
		switch {
		case r.Name != nil:
			nr.name, nr.pos = r.Name.Value, r.Name.Pos
		case r.Expression != nil:
			nr.pos = r.Expression.Pos
		}
		rows = append(rows, nr)
	}
	sort.SliceStable(rows, func(i, j int) bool { return posLess(rows[i].pos, rows[j].pos) })
	for _, r := range rows {
		if r.Expression != nil {
			v, ok, err := resolve(r.Expression)
			if !ok || err != nil {
				return nil, r.Expression.Value, err
			}
			vals, err := asList(v)
			if err != nil {
				return nil, "", fmt.Errorf("matrix %s: %w", r.name, err)
			}
			sp.rows = append(sp.rows, rowSpec{r.name, vals})
			continue
		}
		vals := make([]any, len(r.Values))
		for i, v := range r.Values {
			vals[i] = rawToAny(v)
		}
		sp.rows = append(sp.rows, rowSpec{r.name, vals})
	}
	for _, side := range []struct {
		c   *jactionlint.MatrixCombinations
		dst *[][]kv
		n   string
	}{{m.Include, &sp.include, "include"}, {m.Exclude, &sp.exclude, "exclude"}} {
		if side.c == nil {
			continue
		}
		if side.c.Expression != nil {
			v, ok, err := resolve(side.c.Expression)
			if !ok || err != nil {
				return nil, side.c.Expression.Value, err
			}
			if *side.dst, err = asEntries(v); err != nil {
				return nil, "", fmt.Errorf("matrix %s: %w", side.n, err)
			}
			continue
		}
		for _, c := range side.c.Combinations {
			if c.Expression != nil {
				v, ok, err := resolve(c.Expression)
				if !ok || err != nil {
					return nil, c.Expression.Value, err
				}
				es, err := asEntries([]any{v})
				if err != nil {
					return nil, "", fmt.Errorf("matrix %s: %w", side.n, err)
				}
				*side.dst = append(*side.dst, es...)
				continue
			}
			*side.dst = append(*side.dst, assignsOf(c))
		}
	}
	return sp, "", nil
}

// expandMatrix returns the combinations that a strategy.matrix produces, the way the runner builds them: the
// product of the rows, minus the combinations which an exclude entry matches, then every include entry. dynamic is
// the source of an expression when the matrix cannot be known yet (a fromJSON of a job output).
func expandMatrix(m *jactionlint.Matrix, res resolver) (combos []*combo, dynamic string, err error) {
	if m == nil {
		return []*combo{{Values: map[string]any{}}}, "", nil
	}
	sp, dynamic, err := specOf(m, res)
	if sp == nil {
		return nil, dynamic, err
	}
	return sp.expand(), "", nil
}

func (sp *matrixSpec) expand() []*combo {
	original := map[string]bool{}
	for _, r := range sp.rows {
		original[r.name] = true
	}
	combos := product(sp.rows)

	if len(sp.exclude) > 0 {
		var kept []*combo
	next:
		for _, c := range combos {
			for _, x := range sp.exclude {
				if excludes(c, x) {
					continue next
				}
			}
			kept = append(kept, c)
		}
		combos = kept
	}

	// An include entry joins the combinations of the rows, never the ones that other include entries made.
	base := combos
	for _, es := range sp.include {
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
	return combos
}

func product(rows []rowSpec) []*combo {
	if len(rows) == 0 {
		return nil
	}
	out := []*combo{{Values: map[string]any{}}}
	for _, r := range rows {
		var next []*combo
		for _, c := range out {
			for _, v := range r.vals {
				n := c.clone()
				n.set(r.name, v)
				next = append(next, n)
			}
		}
		out = next
	}
	return out
}

func excludes(c *combo, x []kv) bool {
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
