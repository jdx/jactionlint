package jactionlint

import "testing"

func TestScalarsMayBeEqual(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// The same text, in any case
		{"linux", "linux", true},
		{"Linux", "linux", true},
		{"linux", "windows", false},
		// Numbers of the YAML 1.2 core schema, by value
		{"1", "1.0", true},
		{"1", "1.00", true},
		{"0x10", "16", true},
		{"0o20", "16", true},
		{"0x10", "0o20", true},
		{"16", "17", false},
		{"0x10", "17", false},
		{"1e1", "10", true},
		{".5", "0.5", true},
		{"+1", "1", true},
		{".inf", "+.inf", true},
		{".inf", "-.inf", false},
		{".inf", "5", false},
		// NaN and a number that cannot be read may be anything
		{".nan", "1", true},
		{"0xFFFFFFFFFFFFFFFFFF", "1", true},
		// Booleans and null
		{"true", "True", true},
		{"true", "TRUE", true},
		{"true", "false", false},
		{"null", "~", true},
		{"Null", "NULL", true},
		{"null", "false", false},
		{"null", "0", false},
		// A number is not a string that looks like another number, and a version is a string
		{"1.2.3", "1.2.4", false},
		{"1.2.3", "1.2.3", true},
		{"22.04-arm", "22.04", false},
		{"1_000", "1000", false},
		{"yes", "true", false},
		{"on", "true", false},
		// The same text is the same whatever the type: "1" and 1
		{"1", "1", true},
	}
	for _, tc := range tests {
		for _, p := range [][2]string{{tc.a, tc.b}, {tc.b, tc.a}} {
			if have := scalarsMayBeEqual(p[0], p[1]); have != tc.want {
				t.Errorf("scalarsMayBeEqual(%q, %q) = %v, want %v", p[0], p[1], have, tc.want)
			}
		}
	}
}
