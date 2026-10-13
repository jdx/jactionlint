package jactionlint

import (
	"sort"
	"strings"
	"testing"
)

// survivors parses a matrix and returns the values of the row `name` that certainly have a job.
func survivors(t *testing.T, matrix, name string) []string {
	t.Helper()
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n" + matrix + "    steps:\n      - run: echo\n"
	w, _ := Parse([]byte(src))
	if w == nil || w.Jobs["a"] == nil || w.Jobs["a"].Strategy == nil || w.Jobs["a"].Strategy.Matrix == nil {
		t.Fatalf("no matrix in %q", src)
	}
	var out []string
	for v, ok := range survivingRowValues(w.Jobs["a"].Strategy.Matrix, name) {
		if ok {
			out = append(out, v.Value)
		}
	}
	sort.Strings(out)
	return out
}

func TestSurvivingRowValues(t *testing.T) {
	tests := []struct {
		what   string
		matrix string
		want   string // the values, joined by ","
	}{
		{"no exclude", "        os: [a, b]\n", "a,b"},
		{"an entry with only the key removes the value", "        os: [a, b]\n        exclude:\n          - os: a\n", "b"},
		{"an entry with another key removes only some combinations", "        os: [a, b]\n        n: [1, 2]\n        exclude:\n          - os: a\n            n: 1\n", "a,b"},
		{"the entries together remove every combination", "        os: [a, b]\n        n: [1, 2]\n        exclude:\n          - os: a\n            n: 1\n          - os: a\n            n: 2\n", "b"},
		{"a number is the same as its text", "        os: [a, b]\n        n: [1, 2]\n        exclude:\n          - os: a\n            n: 1\n          - os: a\n            n: '2'\n", "b"},
		{"the case of a value is not told", "        os: [A, b]\n        exclude:\n          - os: a\n", "b"},
		{"an expression on the key may match any value", "        os: [a, b]\n        exclude:\n          - os: ${{ inputs.x }}\n", ""},
		{"an expression on another key may match any value of it", "        os: [a, b]\n        n: [1, 2]\n        exclude:\n          - os: a\n            n: ${{ inputs.x }}\n", "b"},
		{"an entry that is an expression may remove anything", "        os: [a, b]\n        exclude: ${{ fromJSON(inputs.x) }}\n", ""},
		{"a row that is an expression stands for any value", "        os: [a, b]\n        n: ${{ fromJSON(inputs.x) }}\n        exclude:\n          - os: a\n            n: 1\n", "b"},
		{"the row itself is an expression", "        os: ${{ fromJSON(inputs.x) }}\n", ""},
		{"an array never equals a string", "        os: [a, b]\n        exclude:\n          - os: [a]\n", "a,b"},
		{"an entry with a key that is not in the base cannot match", "        os: [a, b]\n        include:\n          - extra: 1\n        exclude:\n          - os: a\n            extra: 1\n", "a,b"},
		{"a string filter never equals an object value", "        os: [a]\n        cfg:\n          - {x: 1}\n        exclude:\n          - os: a\n            cfg: x\n", "a"},
		{"an object filter may equal an object value", "        os: [a]\n        cfg:\n          - {x: 1, y: 2}\n        exclude:\n          - os: a\n            cfg: {x: 1}\n", ""},
		{"an object filter never equals a string value", "        os: [a]\n        cfg: [x]\n        exclude:\n          - os: a\n            cfg: {x: 1}\n", "a"},
		{"an array filter never equals a string value", "        os: [a]\n        cfg: [x]\n        exclude:\n          - os: a\n            cfg: [x]\n", "a"},
		{"an array filter may equal an array value", "        os: [a]\n        cfg:\n          - [x]\n        exclude:\n          - os: a\n            cfg: [x]\n", ""},
		{"an object value and a string value in one row", "        os: [a]\n        cfg:\n          - {x: 1}\n          - y\n        exclude:\n          - os: a\n            cfg: y\n", "a"},
		{"1 and 1.0 are the same number", "        os: [a]\n        n: [1]\n        exclude:\n          - os: a\n            n: 1.0\n", ""},
		{"1 and the string 1 may be the same", "        os: [a]\n        n: [1]\n        exclude:\n          - os: a\n            n: '1'\n", ""},
		{"1 and 2 are not", "        os: [a]\n        n: [1]\n        exclude:\n          - os: a\n            n: 2\n", "a"},
		{"null is a value like the others", "        os: [a]\n        n: [null, 1]\n        exclude:\n          - os: a\n            n: null\n", "a"},
		{"include does not bring a value back to the row", "        os: [a, b]\n        exclude:\n          - os: a\n        include:\n          - os: a\n", "b"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			// The lines of the tests are indented for the rows of a matrix
			got := strings.Join(survivors(t, tc.matrix, "os"), ",")
			if got != tc.want {
				t.Errorf("have %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSurvivingRowValuesBound(t *testing.T) {
	// 13^4 combinations are more than the bound: nothing is certain when there is an exclude
	m := "        os: [a, b]\n"
	for _, k := range []string{"p", "q", "r", "s"} {
		m += "        " + k + ": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13]\n"
	}
	m += "        exclude:\n          - os: a\n            p: 1\n"
	if got := survivors(t, m, "os"); len(got) != 0 {
		t.Errorf("have %v, want none", got)
	}
}
