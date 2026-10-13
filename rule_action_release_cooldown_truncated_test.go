package jactionlint

import (
	"strings"
	"testing"
)

func TestActionReleaseCooldownWithTruncatedTags(t *testing.T) {
	sha := strings.Repeat("9", 40)
	fx := `{"repos":{"example/trunc":{"tags":{"tags":[{"name":"v9","sha":"` + sha + `"}],"truncated":true},` +
		`"refs":{"tags/v9.1.0":{"sha":"` + sha + `","found":true}},` +
		`"release_dates":{"tags/v9.1.0":{"published_at":"2026-10-12T09:00:00Z"},"tags/v9":{"committed_at":"2026-10-12T09:00:00Z"},"commits/` + sha + `":{"committed_at":"2026-10-12T09:00:00Z"}}}}}`
	client := func() GitHubClient {
		c, err := NewFixtureGitHubClient([]byte(fx))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	cfg := cooldownCfg(t, "  action-release-cooldown: warning\n")
	tests := []struct {
		name string
		step string
		want bool
	}{
		{"a commit whose version tag may be missing from the list", "uses: example/trunc@" + sha, false},
		{"a moving tag which may hide a more specific one", "uses: example/trunc@v9", false},
		{"a comment that the repository confirms", "uses: example/trunc@" + sha + " # v9.1.0", true},
		{"a tag written out in full", "uses: example/trunc@v9.1.0", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cooldownFindings(t, client(), cfg, tc.step)
			if (len(got) == 1) != tc.want {
				t.Errorf("want report=%v, got %v", tc.want, got)
			}
			for _, e := range got {
				if !strings.Contains(e.Message, "v9.1.0 was released") {
					t.Errorf("judged by the release: %s", e.Message)
				}
			}
		})
	}
}
