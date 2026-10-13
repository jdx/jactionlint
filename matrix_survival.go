package jactionlint

import (
	"math"
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
//   - a row that is an expression, or a value of a row with an expression, stands for any value of that row; an object or
//     an array value is equal only to an object or an array filter;
//   - an entry that is itself an expression, or `exclude` as a whole written as an expression, may match any
//     combination, so no value of a row is certain (the values that `include` adds still are);
//   - a string filter never equals an object or an array value and the other way round (`[macos]` is not `macos`), and
//     an entry with a key that is not a row cannot match a combination of the base.
//
// A matrix with more combinations than matrixSurvivalMaxCombinations is not enumerated, and nothing is certain when
// it has an `exclude`.

// matrixSurvivalMaxCombinations bounds the cross product that survivingRowValues enumerates.
const matrixSurvivalMaxCombinations = 4096

// rowSlot is one value of a row for the enumeration: a known string, an object or an array (which only an object or an
// array filter can equal), or "any" (unknown).
type rowSlot struct {
	s   string
	obj bool
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
				switch x := v.(type) {
				case *RawYAMLString:
					if ContainsExpression(x.Value) {
						ss = append(ss, rowSlot{any: true})
					} else {
						ss = append(ss, rowSlot{s: x.Value})
					}
				default:
					ss = append(ss, rowSlot{obj: true})
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

// filterMayEqual reports whether the value of an `exclude` entry may equal the value of a combination. A string
// equals only a string (a scalar, by value), and an object or an array only an object or an array (an object filter
// matches when all of its keys match, which is not looked at: it may match). An expression may be anything.
func filterMayEqual(filter RawYAMLValue, slot rowSlot) bool {
	if slot.any {
		return true
	}
	s, ok := filter.(*RawYAMLString)
	if !ok {
		return slot.obj
	}
	if ContainsExpression(s.Value) {
		return true
	}
	if slot.obj {
		return false
	}
	return scalarsMayBeEqual(s.Value, slot.s)
}

// scalarsMayBeEqual is true for texts that GitHub may take as the same value. The scalars are read like its parser does
// (YAML 1.2 core schema): null (null, Null, NULL, ~), booleans (true, True, TRUE and the false ones), numbers (decimal,
// 0o octal, 0x hexadecimal, floats with an exponent, .inf and .nan) and strings. Two texts may be equal when they are the
// same text in any case, or are the same kind of scalar with the same value (0x10 and 16, 1.0 and 1, .inf and +.inf,
// True and true, null and ~). A number that cannot be read, or a NaN, may equal any number: the answer is only "no" when
// the two values are known to differ, since the callers stay silent on "may".
func scalarsMayBeEqual(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	ka, va, oka := yamlCoreScalar(a)
	kb, vb, okb := yamlCoreScalar(b)
	switch {
	case ka != kb:
		return false
	case ka == scalarString:
		return false
	case ka == scalarNumber:
		return !oka || !okb || va == vb
	case ka == scalarBool:
		return va == vb
	}
	return true // null and null
}

type scalarKind int

const (
	scalarString scalarKind = iota
	scalarNull
	scalarBool
	scalarNumber
)

// yamlCoreScalar returns the kind of the scalar and, for a number, its value. ok is false for a number that could not be
// read, or that is NaN.
func yamlCoreScalar(s string) (scalarKind, float64, bool) {
	switch s {
	case "null", "Null", "NULL", "~":
		return scalarNull, 0, true
	case "true", "True", "TRUE":
		return scalarBool, 1, true
	case "false", "False", "FALSE":
		return scalarBool, 0, true
	}
	switch {
	case yamlCoreSpecialNumberRe.MatchString(s):
		if strings.Contains(strings.ToLower(s), "nan") {
			return scalarNumber, 0, false
		}
		if strings.HasPrefix(s, "-") {
			return scalarNumber, math.Inf(-1), true
		}
		return scalarNumber, math.Inf(1), true
	case strings.HasPrefix(s, "0x"), strings.HasPrefix(s, "0o"):
		if yamlCoreNumberRe.MatchString(s) {
			if n, err := strconv.ParseInt(s[2:], map[string]int{"0x": 16, "0o": 8}[s[:2]], 64); err == nil {
				return scalarNumber, float64(n), true
			}
			return scalarNumber, 0, false
		}
	case yamlCoreNumberRe.MatchString(s):
		f, err := strconv.ParseFloat(s, 64)
		return scalarNumber, f, err == nil
	}
	return scalarString, 0, true
}
