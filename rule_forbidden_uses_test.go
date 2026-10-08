package jactionlint

import (
	"strings"
	"testing"
)

func TestUsesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		uses    string
		want    bool
	}{
		{"*", "actions/checkout@v4", true},
		{"*", "octo/repo/.github/workflows/w.yaml@v1", true},
		{"*", "./local", false},
		{"*", "docker://alpine", false},
		{"actions/*", "actions/checkout@v4", true},
		{"actions/*", "actions/cache/save@v4", true},
		{"actions/*", "Actions/Checkout@v4", true},
		{"actions/*", "actionsx/checkout@v4", false},
		{"actions/*", "octo/actions/x@v1", false},
		{"actions/cache", "actions/cache@v4", true},
		{"actions/cache", "actions/cache/save@v4", false},
		{"actions/cache", "Actions/Cache@v4", true},
		{"actions/cache/*", "actions/cache@v4", true},
		{"actions/cache/*", "actions/cache/save@v4", true},
		{"actions/cache/*", "actions/cache2@v4", false},
		{"actions/cache/save", "actions/cache/save@v4", true},
		{"actions/cache/save", "actions/cache@v4", false},
		{"actions/checkout@v4", "actions/checkout@v4", true},
		{"actions/checkout@v4", "actions/checkout@v3", false},
		{"actions/checkout@v4", "actions/checkout@V4", false},
		{"actions/*@v4", "actions/cache@v4", true},
		{"actions/*@v4", "actions/cache@v5", false},
		{"actions/*@v*", "actions/cache@v5", true},
		{"actions/setup-*", "actions/setup-node@v4", true},
		{"actions/setup-*", "actions/checkout@v4", false},
		{"*/checkout", "actions/checkout@v4", true},
		{"*/checkout", "actions/checkout/sub@v4", false},
		{"*/checkout/*", "actions/checkout/sub@v4", true},
		{"octo/shared/.github/workflows/ci.yaml@v1", "octo/shared/.github/workflows/ci.yaml@v1", true},
		{"octo/shared/.github/workflows/*", "octo/shared/.github/workflows/ci.yaml@v1", true},
		{" actions/* ", "actions/checkout@v4", true},
		{"actions/*/", "actions/checkout@v4", true}, // A trailing slash is ignored
		{"", "actions/checkout@v4", false},
	}
	for _, tc := range tests {
		got := ParseUsesPattern(tc.pattern).Match(ParseUses(tc.uses))
		if got != tc.want {
			t.Errorf("pattern %q on %q: want %v but got %v", tc.pattern, tc.uses, tc.want, got)
		}
	}
	if ParseUsesPattern("a/b").Match(nil) {
		t.Error("nil never matches")
	}
}

func FuzzUsesPattern(f *testing.F) {
	for _, s := range []string{"*", "a/*", "a/b@v1", "a/b/c*d@*", "**", "@", "/", "a/*/", ""} {
		f.Add(s, "actions/checkout@v4")
		f.Add(s, "a/b/c@v1")
	}
	f.Fuzz(func(t *testing.T, pattern, uses string) {
		p := ParseUsesPattern(pattern)
		_ = p.String()
		u := ParseUses(uses)
		got := p.Match(u)
		if got && !u.IsRepo() {
			t.Fatalf("pattern %q matches %q which is not a repository reference", pattern, uses)
		}
		// A reference always matches the pattern made of its own name and ref
		if u.IsRepo() && !strings.ContainsAny(u.Owner+u.Repo+u.Subpath+u.Ref, "*@ ") && u.Ref != "" {
			own := ParseUsesPattern(u.CanonicalName() + "@" + u.Ref)
			if !own.Match(u) {
				t.Fatalf("%q does not match its own pattern", uses)
			}
		}
	})
}

func TestForbiddenUsesConfig(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - uses: octo/repo@v1\n"
	tests := []struct {
		what  string
		cfg   string
		lines []int
	}{
		{"no config does nothing", "", nil},
		{"enabled without a list does nothing", "rules:\n  forbidden-uses: error\n", nil},
		{"empty lists do nothing", "rules:\n  forbidden-uses: {allow: [], deny: []}\n", nil},
		{"allow", "rules:\n  forbidden-uses: {allow: ['actions/*']}\n", []int{7}},
		{"deny", "rules:\n  forbidden-uses: {deny: ['actions/*']}\n", []int{6}},
		{"deny wins", "rules:\n  forbidden-uses: {allow: ['*'], deny: ['octo/*']}\n", []int{7}},
		{"turned off", "rules:\n  forbidden-uses: {level: off, deny: ['*']}\n", nil},
		{"ref", "rules:\n  forbidden-uses: {allow: ['actions/checkout@v4', 'octo/repo@v2']}\n", []int{7}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, mustParseConfig(t, tc.cfg), "ci.yaml", src), "forbidden-uses", tc.lines...)
		})
	}

	for _, bad := range []string{"rules:\n  forbidden-uses: {allow: 1}\n", "rules:\n  forbidden-uses: {allow: [1]}\n", "rules:\n  forbidden-uses: {allow: x}\n", "rules:\n  forbidden-uses: {nope: []}\n"} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Errorf("config %q should be rejected", bad)
		}
	}

	t.Run("dynamic uses are not judged", func(t *testing.T) {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: octo/repo@${{ inputs.ref }}\n"
		wantLines(t, lintFileWithConfig(t, mustParseConfig(t, "rules:\n  forbidden-uses: {deny: ['*']}\n"), "ci.yaml", src), "forbidden-uses")
	})
}
