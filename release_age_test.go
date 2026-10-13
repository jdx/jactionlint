package jactionlint

import (
	"strings"
	"testing"
)

// ageSettingOn is the one place that decides whether the value of a release-age setting is a cooldown.
func TestAgeSettingOn(t *testing.T) {
	off := []string{"", " ", "false", "False", "FALSE", "none", "off", "null", "no", "''", `""`, "0", "00", "0.0", "0d", "0m", "0h", "0s", "0w",
		"0 days", "0 day", "0 hours", "0days", "P0D", "p0d", "PT0S", "PT0H", "P0W", "P0DT0H", "P0Y", "0.0d"}
	for _, v := range off {
		if ageSettingOn(v) {
			t.Errorf("%q must be off", v)
		}
	}
	on := []string{"1", "7", "7d", "3d", "7 days", "1 week", "24 hours", "259200", "1440", "P1D", "P7D", "PT1H", "P0DT1H", "P1DT0H", "0.5d", "10",
		"2026-01-01", "2026-01-01T00:00:00Z", "true", "$AGE", "${AGE}", "${{ vars.AGE }}", "$(cat age)", "set", "P", "-1"}
	for _, v := range on {
		if !ageSettingOn(v) {
			t.Errorf("%q must be on", v)
		}
	}
}

// Property: what a value means does not depend on case, white space or quotes around it, and a zero is off with every
// unit while any other number is on with every unit.
func TestAgeSettingOnProperties(t *testing.T) {
	wrap := []func(string) string{
		func(s string) string { return s },
		strings.ToUpper,
		func(s string) string { return " " + s + "  " },
		func(s string) string { return `"` + s + `"` },
		func(s string) string { return `'` + s + `'` },
		func(s string) string { return ` "` + strings.ToUpper(s) + `" ` },
	}
	units := []string{"", "d", "m", "h", "s", "w", "y", " days", " day", " hours", " minutes", " weeks", "days"}
	for _, zero := range []string{"0", "00", "000", "0.0"} {
		for _, n := range []string{"1", "7", "30", "1440", "259200", "2.5"} {
			for _, u := range units {
				for i, w := range wrap {
					if ageSettingOn(w(zero + u)) {
						t.Errorf("wrap %d: %q must be off", i, w(zero+u))
					}
					if !ageSettingOn(w(n + u)) {
						t.Errorf("wrap %d: %q must be on", i, w(n+u))
					}
				}
			}
		}
	}
	// ISO 8601 durations: every component zero is off, one component that is not is on
	for _, c := range []struct {
		v  string
		on bool
	}{{"P0D", false}, {"P0W", false}, {"P0Y0M0D", false}, {"PT0S", false}, {"PT0H0M0S", false}, {"P0DT0S", false},
		{"P1D", true}, {"P0Y0M1D", true}, {"PT1S", true}, {"P0DT1H", true}, {"PT0H1M", true}} {
		for i, w := range wrap {
			if got := ageSettingOn(w(c.v)); got != c.on {
				t.Errorf("wrap %d: %q is %v, want %v", i, w(c.v), got, c.on)
			}
		}
	}
	// a variable or an expression is on, wherever it is
	for _, v := range []string{"$X", "${X}", "${{ x }}", "0${{ x }}", "$(echo 0)"} {
		if !ageSettingOn(v) {
			t.Errorf("%q is not known here and must be on", v)
		}
	}
}

// The last assignment of a key wins inside one place, the age is on when any key of the place is on, and a later place
// replaces the state of an earlier one.
func TestAgeStateAfter(t *testing.T) {
	a := func(kv ...string) []ageAssignment {
		var out []ageAssignment
		for i := 0; i < len(kv); i += 2 {
			out = append(out, ageAssignment{kv[i], kv[i+1]})
		}
		return out
	}
	tests := []struct {
		what   string
		places [][]ageAssignment
		want   ageState
	}{
		{"nothing", nil, ageState{}},
		{"an empty place changes nothing", [][]ageAssignment{a("k", "7"), nil}, ageState{true, true}},
		{"on", [][]ageAssignment{a("k", "7")}, ageState{true, true}},
		{"off", [][]ageAssignment{a("k", "0")}, ageState{true, false}},
		{"the last of a key wins: on then off", [][]ageAssignment{a("k", "7", "k", "0")}, ageState{true, false}},
		{"the last of a key wins: off then on", [][]ageAssignment{a("k", "0", "k", "7")}, ageState{true, true}},
		{"three assignments", [][]ageAssignment{a("k", "7", "k", "0", "k", "P1D")}, ageState{true, true}},
		{"two keys: one on", [][]ageAssignment{a("min-release-age", "0", "before", "2026-01-01")}, ageState{true, true}},
		{"two keys: both off", [][]ageAssignment{a("min-release-age", "0", "before", "")}, ageState{true, false}},
		{"two keys: the last of each", [][]ageAssignment{a("min-release-age", "7", "before", "2026-01-01", "before", "")}, ageState{true, true}},
		{"two keys: the last of each is off", [][]ageAssignment{a("min-release-age", "7", "min-release-age", "0", "before", "")}, ageState{true, false}},
		{"a later place wins: off over on", [][]ageAssignment{a("k", "7"), a("k", "0")}, ageState{true, false}},
		{"a later place wins: on over off", [][]ageAssignment{a("k", "0"), a("k", "7")}, ageState{true, true}},
		{"a later place wins whatever the key", [][]ageAssignment{a("before", "2026-01-01"), a("min-release-age", "0")}, ageState{true, false}},
		{"a variable is on", [][]ageAssignment{a("k", "$AGE")}, ageState{true, true}},
		{"a variable after off", [][]ageAssignment{a("k", "0"), a("k", "${{ vars.AGE }}")}, ageState{true, true}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			var st ageState
			for _, p := range tc.places {
				st = st.after(p)
			}
			if st != tc.want {
				t.Errorf("got %+v, want %+v", st, tc.want)
			}
		})
	}
	// Property: applying the places one by one gives the state of the last non-empty place, whatever came before
	for _, last := range [][]ageAssignment{a("k", "7"), a("k", "0"), a("k", "$X")} {
		for _, first := range [][]ageAssignment{nil, a("k", "7"), a("k", "0"), a("j", "9")} {
			want := ageState{}.after(last)
			if got := (ageState{}).after(first).after(last); got != want {
				t.Errorf("%v then %v: got %+v, want %+v", first, last, got, want)
			}
		}
	}
}
