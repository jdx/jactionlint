package jactionlint

import "testing"

func TestScriptSourceReplacesExpressions(t *testing.T) {
	tests := []struct {
		src, want string // want "" is a script which is not checked
	}{
		{"return 1", "return 1"},
		{"const s = '${{ a }}'", "const s = '________'"},
		{"const s = '${{a}}'; f(${{ b }})", "const s = '______'; f(________)"},
		{"obj.${{ k }}", "obj.________"},
		{"x = ${{ a }} + ${{ b }}", "x = ________ + ________"},
		// An expression that touches a name or a number: the value can continue it
		{"const n = 1${{ inputs.a }};", ""},
		{"const n = ${{ inputs.a }}2;", ""},
		{"const n = 0x${{ inputs.a }};", ""},
		{"const n = a${{ x }}b;", ""},
		{"const n = ${{ x }}b;", ""},
		{"const n = $${{ x }};", ""},
		// The end of the expression depends on a string
		{"const n = ${{ format('{{}}', 1) }};", ""},
		// A string with `{{` alone does not end the expression early and is replaced as a whole
		{"const n = ${{ format('{{', 1) }} + 1;", "const n = ______________________ + 1;"},
		// Not closed, and over several lines
		{"const n = ${{ x", ""},
		{"const n = ${{\n x }};", ""},
	}
	for _, tc := range tests {
		if have := githubScriptSource(tc.src); have != tc.want {
			t.Errorf("githubScriptSource(%q) = %q, want %q", tc.src, have, tc.want)
		}
	}
}
