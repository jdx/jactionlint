package jactionlint

import (
	"strconv"
	"strings"
)

// This file models which values of a row of a matrix certainly have a job, for the checks that look at what a job
// runs on (concatenated runner labels). The model follows how GitHub documents the matrix
// (https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax, "Expanding or adding matrix
// configurations" and "Excluding matrix configurations"):
//
//  1. The base is the cross product of the rows, the keys other than include and exclude.
//  2. An `exclude` entry removes every combination of the base which has all the key/value pairs of the entry (a
//     partial match). It is applied before `include`, and it can name only keys of the base: what `include` adds can
//     never be excluded.
//  3. An `include` entry is added afterwards: to the combinations that it does not overwrite, or as a new combination.
//     So a value that an `include` entry names has a job, whatever `exclude` removed.
//
// A value of a row has a job for sure when at least one combination of the base which has the value is matched by no
// `exclude` entry. The unknown is treated as "may match", so the answer is only "yes" when it is certain, and the
// callers stay silent otherwise:
//
//   - a value written with an expression in an entry matches any value of that key;
//   - a row that is an expression, or a value of a row which is not a plain string, stands for any value of that row;
//   - an entry that is itself an expression, or `exclude` as a whole written as an expression, may match any
//     combination, so no value of a row is certain (the values that `include` adds still are);
//   - an entry never matches a string with an object or an array (`[macos]` is not `macos`), and an entry with a key
//     that is not a row cannot match a combination of the base.
//
// A matrix with more combinations than matrixSurvivalMaxCombinations is not enumerated, and nothing is certain when
// it has an `exclude`.

// matrixSurvivalMaxCombinations bounds the cross product that survivingRowValues enumerates.
const matrixSurvivalMaxCombinations = 4096

// rowSlot is one value of a row for the enumeration: a known string, or "any" (unknown).
type rowSlot struct {
	s   string
	any bool
}

// survivingRowValues returns the values of the row `name` that certainly have a job (see the comment of the file).
// It returns the values of the row when nothing is excluded.
func survivingRowValues(m *Matrix, name string) map[*RawYAMLString]bool {
	out := map[*RawYAMLString]bool{}
	row, ok := m.Rows[name]
	if !ok || row.Expression != nil {
		return out // the values of a row which is an expression are not known
	}
	var cands []*RawYAMLString
	for _, v := range row.Values {
		if s, ok := v.(*RawYAMLString); ok && !ContainsExpression(s.Value) {
			cands = append(cands, s)
		}
	}
	if m.Exclude == nil || (len(m.Exclude.Combinations) == 0 && m.Exclude.Expression == nil) {
		for _, s := range cands {
			out[s] = true
		}
		return out
	}
	if m.Exclude.Expression != nil {
		return out
	}
	for _, e := range m.Exclude.Combinations {
		if e.Expression != nil {
			return out
		}
	}

	// The slots of the other rows
	var keys []string
	slots := map[string][]rowSlot{}
	size := 1
	for k, r := range m.Rows {
		if k == name {
			continue
		}
		var ss []rowSlot
		if r.Expression != nil {
			ss = []rowSlot{{any: true}}
		} else {
			for _, v := range r.Values {
				if s, ok := v.(*RawYAMLString); ok && !ContainsExpression(s.Value) {
					ss = append(ss, rowSlot{s: s.Value})
				} else {
					ss = append(ss, rowSlot{any: true})
				}
			}
		}
		if len(ss) == 0 {
			return out // no combination at all
		}
		keys = append(keys, k)
		slots[k] = ss
		if size *= len(ss); size > matrixSurvivalMaxCombinations {
			return out
		}
	}

	for _, cand := range cands {
		combo := make([]int, len(keys))
		for {
			if !entryMayMatch(m.Exclude.Combinations, name, rowSlot{s: cand.Value}, keys, slots, combo) {
				out[cand] = true
				break
			}
			// Next combination of the other rows
			i := 0
			for ; i < len(combo); i++ {
				if combo[i]++; combo[i] < len(slots[keys[i]]) {
					break
				}
				combo[i] = 0
			}
			if i == len(combo) {
				break
			}
		}
	}
	return out
}

// entryMayMatch reports whether one of the `exclude` entries may match the combination: the candidate for the row
// `name` and the slots of the other rows chosen by combo.
func entryMayMatch(entries []*MatrixCombination, name string, cand rowSlot, keys []string, slots map[string][]rowSlot, combo []int) bool {
	for _, e := range entries {
		match := true
		for k, a := range e.Assigns {
			k = strings.ToLower(k)
			var slot rowSlot
			switch {
			case k == name:
				slot = cand
			default:
				found := false
				for i, kk := range keys {
					if kk == k {
						slot, found = slots[kk][combo[i]], true
						break
					}
				}
				if !found {
					match = false // a key that is not in the base cannot match
				}
			}
			if !match {
				break
			}
			if !filterMayEqual(a.Value, slot) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// filterMayEqual reports whether the value of an `exclude` entry may equal the value of a combination.
func filterMayEqual(filter RawYAMLValue, slot rowSlot) bool {
	s, ok := filter.(*RawYAMLString)
	if !ok {
		// An object or an array equals only an object or an array, which a slot that is not a string may be
		return slot.any
	}
	if ContainsExpression(s.Value) || slot.any {
		return true
	}
	return scalarsMayBeEqual(s.Value, slot.s)
}

// scalarsMayBeEqual is true for texts that GitHub may take as the same value: the same text, the same text in another
// case, or the same number.
func scalarsMayBeEqual(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	return errA == nil && errB == nil && fa == fb
}
