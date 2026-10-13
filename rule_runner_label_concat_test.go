package jactionlint

import "testing"

func TestIsStableMatrixString(t *testing.T) {
	tests := []struct {
		v      RawYAMLString
		stable bool
	}{
		{RawYAMLString{Value: "linux"}, true},
		{RawYAMLString{Value: "x64"}, true},
		{RawYAMLString{Value: "22.04"}, true},
		{RawYAMLString{Value: "22.04-arm"}, true},
		{RawYAMLString{Value: "20.10"}, false}, // GitHub prints 20.1
		{RawYAMLString{Value: "20.10", StringTag: true}, true},
		{RawYAMLString{Value: "0x18"}, false},
		{RawYAMLString{Value: "1_000"}, false},
		{RawYAMLString{Value: ".inf"}, false},
		{RawYAMLString{Value: "null"}, false},
		{RawYAMLString{Value: "~"}, false},
		{RawYAMLString{Value: ""}, false},
		{RawYAMLString{Value: "${{ inputs.os }}", StringTag: true}, false},
	}
	for _, tc := range tests {
		if have := isStableMatrixString(&tc.v); have != tc.stable {
			t.Errorf("isStableMatrixString(%q, tag=%v) = %v, want %v", tc.v.Value, tc.v.StringTag, have, tc.stable)
		}
	}
}

func TestConcatenatedRunnerLabelsNeedsOneMatrixName(t *testing.T) {
	row := func(vs ...string) *MatrixRow {
		r := &MatrixRow{}
		for _, v := range vs {
			r.Values = append(r.Values, &RawYAMLString{Value: v})
		}
		return r
	}
	m := &Matrix{Rows: map[string]*MatrixRow{"os": row("a", "b"), "arch": row("x")}}
	for _, tc := range []struct {
		label string
		want  []string
	}{
		{"${{ matrix.os }}-latest", []string{"a-latest", "b-latest"}},
		{"${{matrix.OS}}-${{ matrix.os }}", []string{"a-a", "b-b"}},
		{"${{ matrix.os }}-${{ matrix.arch }}", nil},
		{"${{ matrix.os }}-${{ github.ref }}", nil},
		{"${{ matrix['os'] }}-latest", nil},
		{"${{ matrix.os }-latest", nil},
		{"${{ matrix.nothing }}-latest", nil},
		{"-latest", nil},
	} {
		var have []string
		for _, l := range concatenatedRunnerLabels(&String{Value: tc.label}, m) {
			have = append(have, l.Value)
		}
		if len(have) != len(tc.want) {
			t.Errorf("%q: have %v, want %v", tc.label, have, tc.want)
			continue
		}
		for i := range have {
			if have[i] != tc.want[i] {
				t.Errorf("%q: have %v, want %v", tc.label, have, tc.want)
				break
			}
		}
	}
}
