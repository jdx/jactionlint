package jactionlint

import "testing"

func TestMentionsVersion(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"v4", true},
		{"4.2.2", true},
		{"pin@v7.0.1", true},
		{"Release v4.4.1", true},
		{"codeql-bundle-v2.19.0", true},
		{"v2.x", true},
		{"Claude Code 2.1.280", true},
		{"(v4)", true},
		{"v4.2.2-rc1", true},
		{"pinv4.38.2", false},
		{"v4abc", false},
		{"1.2abc", false},
		{"1.2.3abc", false},
		{"v1.2.3beta", false},
		{"v1.2.3.", true},
		{"1.2.3.4", true},
		{"abc4", false},
		{"needed for the cache", false},
		{"zizmor: ignore[artipacked]", false},
		{"v", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := mentionsVersion(tc.text); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.text, got, tc.want)
		}
	}
}
