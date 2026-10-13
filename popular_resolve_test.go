package jactionlint

import "testing"

func TestResolvePopularSpec(t *testing.T) {
	const sha = "0400d5f644dc74513175e3cd8d07132dd4860809"
	known := func(s string) bool { return s == "actions/cache@v4" || s == "actions/checkout/sub@v3" }
	tests := []struct {
		uses    string
		comment string // text of the comment after the value, if any
		want    string // "" for no resolution
		source  popularRefSource
	}{
		{"actions/cache@v4", "", "actions/cache@v4", popularExact},
		{"actions/cache@v4.2.4", "", "actions/cache@v4", popularFromTag},
		{"actions/cache@v4.2", "", "actions/cache@v4", popularFromTag},
		{"actions/cache@4.2.4", "", "actions/cache@v4", popularFromTag},
		{"actions/checkout/sub@v3.1.0", "", "actions/checkout/sub@v3", popularFromTag},
		{"actions/cache@4", "", "", 0},
		{"actions/cache@v4.2.4-rc.1", "", "", 0},
		{"actions/cache@v04.2", "", "", 0},
		{"actions/cache@v5.0.0", "", "", 0},
		{"actions/cache@main", "", "", 0},
		{"actions/cache@" + sha, "", "", 0},
		{"actions/cache@" + sha, "v4.2.4", "actions/cache@v4", popularFromComment},
		{"actions/cache@" + sha, "v4", "actions/cache@v4", popularFromComment},
		{"actions/cache@" + sha, "tag=v4.2.4", "actions/cache@v4", popularFromComment},
		{"actions/cache@" + sha, "v4.2.4 (pinned)", "actions/cache@v4", popularFromComment},
		{"actions/cache@" + sha, "4.2.4", "actions/cache@v4", popularFromComment},
		{"actions/cache@" + sha, "4", "", 0},
		{"actions/cache@" + sha, "v4 -> v5", "", 0},
		{"actions/cache@" + sha, "v4 (was v3.9.0)", "", 0},
		{"actions/cache@" + sha, "keep in sync with v4", "", 0},
		{"actions/cache@" + sha, "v4.2.4-beta.1", "", 0},
		{"actions/cache@" + sha, "v5.1.0", "", 0},
		{"actions/cache@" + sha[:12], "v4.2.4", "", 0},
		{"example/other@" + sha, "v4.2.4", "", 0},
		{"actions/cache@${{ matrix.v }}", "", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.uses+"#"+tc.comment, func(t *testing.T) {
			src := "uses: " + tc.uses
			if tc.comment != "" {
				src += " # " + tc.comment
			}
			idx := NewCommentIndex([]byte(src + "\n"))
			got, ok := resolvePopularSpec(ParseUses(tc.uses), &String{Value: tc.uses, Pos: &Pos{Line: 1, Col: 7}}, idx, known)
			if tc.want == "" {
				if ok {
					t.Fatalf("resolved to %+v, want nothing", got)
				}
				return
			}
			if !ok || got.Key != tc.want || got.Source != tc.source {
				t.Fatalf("got %+v, %v; want %q (source %d)", got, ok, tc.want, tc.source)
			}
		})
	}
}
