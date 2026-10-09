package jactionlint

import "testing"

func TestParseAdvisoryVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"1.2.3", "1.2.3", true},
		{"v4", "4", true},
		{"V4.1", "4.1", true},
		{"v1.0.0-rc.1", "1.0.0-rc.1", true},
		{"1.2.3+build5", "1.2.3", true},
		{" 2 ", "2", true},
		{"", "", false},
		{"main", "", false},
		{"v", "", false},
		{"1..2", "", false},
		{"1.x", "", false},
		{"-1", "", false},
		{"+1", "", false},
		{"1.2.3-", "", false},
		{"releases/v1", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			v, ok := parseAdvisoryVersion(tc.in)
			if ok != tc.ok || (ok && v.String() != tc.want) {
				t.Errorf("parseAdvisoryVersion(%q) = %q, %v. want %q, %v", tc.in, v, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAdvisoryVersionCompare(t *testing.T) {
	// Each line is in ascending order
	order := []string{"0.9", "1", "1.0.1", "1.1", "1.10", "2.0.0-alpha", "2.0.0-alpha.1", "2.0.0-alpha.beta", "2.0.0-beta", "2.0.0-beta.2", "2.0.0-beta.11", "2.0.0-rc.1", "2", "2.0.1", "10"}
	for i, a := range order {
		for j, b := range order {
			va, _ := parseAdvisoryVersion(a)
			vb, _ := parseAdvisoryVersion(b)
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			// "1" equals "1.0": the table has no such pair
			if got := va.compare(vb); got != want {
				t.Errorf("compare(%q, %q) = %d. want %d", a, b, got, want)
			}
		}
	}
	a, _ := parseAdvisoryVersion("1.2")
	b, _ := parseAdvisoryVersion("1.2.0")
	if a.compare(b) != 0 {
		t.Error("1.2 should equal 1.2.0")
	}
}

func TestVersionRange(t *testing.T) {
	tests := []struct {
		rng     string
		version string
		want    bool
	}{
		{"<= 45.0.7", "45.0.7", true},
		{"<= 45.0.7", "45.0.8", false},
		{"<= 45.0.7", "44", true},
		{"< 41", "40.9", true},
		{"< 41", "41", false},
		{">= 1.0.0, < 1.2.3", "1.2.2", true},
		{">= 1.0.0, < 1.2.3", "1.2.3", false},
		{">= 1.0.0, < 1.2.3", "0.9", false},
		{"= 2.0.0", "2", true},
		{"== 2.0.0", "2.0.1", false},
		{"> 3", "3.0.1", true},
		{">= 0", "0.0.1", true},
		{"< 2.0.0", "2.0.0-rc.1", true},
	}
	for _, tc := range tests {
		t.Run(tc.rng+" "+tc.version, func(t *testing.T) {
			r, err := parseVersionRange(tc.rng)
			if err != nil {
				t.Fatal(err)
			}
			v, ok := parseAdvisoryVersion(tc.version)
			if !ok {
				t.Fatal("bad version")
			}
			if got := r.contains(v); got != tc.want {
				t.Errorf("%q contains %q = %v. want %v", tc.rng, tc.version, got, tc.want)
			}
		})
	}
}

func TestVersionRangeErrors(t *testing.T) {
	for _, s := range []string{"", ",", "1.0", "~> 1.0", "< ", "< x", ">= 1, ", "! 1"} {
		if _, err := parseVersionRange(s); err == nil {
			t.Errorf("parseVersionRange(%q) should fail", s)
		}
	}
}

func FuzzParseVersionRange(f *testing.F) {
	for _, s := range []string{"<= 45.0.7", ">= 1.0.0, < 1.2.3", "= 2", "", ",,", "< 1.0-rc.1"} {
		f.Add(s, "1.2.3")
	}
	f.Fuzz(func(t *testing.T, rng, ver string) {
		r, err := parseVersionRange(rng)
		v, ok := parseAdvisoryVersion(ver)
		if err == nil && ok {
			r.contains(v)
			v.compare(v)
		}
	})
}
