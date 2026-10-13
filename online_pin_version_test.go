package jactionlint

import "testing"

func TestSpecificVersionTag(t *testing.T) {
	tests := []struct {
		name  string
		names []string
		given string
		want  string // "" when the given tag is kept
	}{
		{"most specific", []string{"v4", "v4.2", "v4.2.4"}, "v4", "v4.2.4"},
		{"from the middle", []string{"v4", "v4.2"}, "v4", "v4.2"},
		{"already the most specific", []string{"v4", "v4.2.4"}, "v4.2.4", "v4.2.4"},
		{"no longer tag", []string{"v4"}, "v4", "v4"},
		{"without v", []string{"4", "4.2.4"}, "4", "4.2.4"},
		{"the spelling of the author", []string{"v4.2.4", "4.2.4"}, "v4", "v4.2.4"},
		{"the spelling of the author without v", []string{"v4.2.4", "4.2.4"}, "4", "4.2.4"},
		{"not a version in the way", []string{"v4", "latest", "stable-4", "v4.2.4"}, "v4", "v4.2.4"},
		{"a version which does not refine the tag", []string{"v4", "v5.0.0"}, "v4", "v4"},
		{"two versions of the same specificity", []string{"v4", "v4.2.3", "v4.2.4"}, "v4", ""},
		{"a release over its prerelease", []string{"v4", "v4.2.4-rc.1", "v4.2.4"}, "v4", "v4.2.4"},
		{"a prerelease is not more specific than a release", []string{"v4", "v4.3.0-rc.1"}, "v4", "v4"},
		{"build metadata is not used", []string{"v4", "v4.2.4+build.5"}, "v4", "v4"},
		{"a prerelease as given is kept", []string{"v4.0.0-rc.1", "v4.0.0-rc.1.1"}, "v4.0.0-rc.1", ""},
		{"a tag which is not a version", []string{"release", "v4.2.4"}, "release", ""},
		{"four numbers", []string{"v4", "v4.2.4.1"}, "v4", "v4"},
		{"no given tag", []string{"v4", "v4.2.4", "foo"}, "", "v4.2.4"},
		{"no given tag, none is a version", []string{"foo"}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := specificVersionTag(tc.names, tc.given)
			if tc.want == "" {
				if ok {
					t.Errorf("got %q, want the tag kept", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("got %q, %v. want %q", got, ok, tc.want)
			}
		})
	}
}

const (
	tagsSHA1 = "1111111111111111111111111111111111111111"
	tagsSHA2 = "2222222222222222222222222222222222222222"
	tagsSHA3 = "3333333333333333333333333333333333333333"
)

// tagsClient serves example/tags: v1 is also v1.2 and v1.2.3, v2 is v2.0.1 and v2.0.2 (which version
// is meant is not known), and "release" is a tag which is not a version.
func tagsClient(t *testing.T) *FixtureGitHubClient {
	t.Helper()
	tags := `{"name":"v1","sha":"` + tagsSHA1 + `"},{"name":"v1.2","sha":"` + tagsSHA1 + `"},{"name":"v1.2.3","sha":"` + tagsSHA1 + `"},` +
		`{"name":"v2","sha":"` + tagsSHA2 + `"},{"name":"v2.0.1","sha":"` + tagsSHA2 + `"},{"name":"v2.0.2","sha":"` + tagsSHA2 + `"},` +
		`{"name":"release","sha":"` + tagsSHA3 + `"},{"name":"v3","sha":"` + tagsSHA3 + `"},{"name":"v3.0.0","sha":"` + tagsSHA3 + `"}`
	b := []byte(`{"repos":{"example/tags":{"tags":{"tags":[` + tags + `]},"refs":{"heads/v1":{"found":false},"heads/v1.2":{"found":false},"heads/v1.2.3":{"found":false},"heads/v2":{"found":false},"heads/release":{"found":false},"heads/v3":{"found":false}}}}}`)
	c, err := NewFixtureGitHubClient(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPinFixCommentNamesTheMostSpecificTag(t *testing.T) {
	run := func(t *testing.T, cfg *Config, step string) string {
		t.Helper()
		src := workflowWith(step)
		errs, _ := lintOnline(t, tagsClient(t), cfg, src)
		got, _ := applyFixes([]byte(src), errs, FixModeSafe)
		return string(got)
	}
	tests := []struct{ name, step, want string }{
		{"short tag gets the exact version", "uses: example/tags@v1", "uses: example/tags@" + tagsSHA1 + " # v1.2.3"},
		{"a minor tag", "uses: example/tags@v1.2", "uses: example/tags@" + tagsSHA1 + " # v1.2.3"},
		{"exact tag as written", "uses: example/tags@v1.2.3", "uses: example/tags@" + tagsSHA1 + " # v1.2.3"},
		{"two versions on the commit: as written", "uses: example/tags@v2", "uses: example/tags@" + tagsSHA2 + " # v2"},
		{"not a version: as written", "uses: example/tags@release", "uses: example/tags@" + tagsSHA3 + " # release"},
		{"quoted", `uses: "example/tags@v1"`, `uses: "example/tags@` + tagsSHA1 + `" # v1.2.3`},
		{"an existing comment is left alone", "uses: example/tags@v1 # v1", "uses: example/tags@" + tagsSHA1 + " # v1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := run(t, unpinnedConfig(), tc.step), workflowWith(tc.want); got != want {
				t.Errorf("got:\n%swant:\n%s", got, want)
			}
		})
	}

	t.Run("short-pin-comment keeps the tag as written", func(t *testing.T) {
		cfg := unpinnedConfig()
		rc := cfg.Rules["unpinned-uses"]
		rc.Options = map[string]any{"short-pin-comment": true}
		cfg.Rules["unpinned-uses"] = rc
		got := run(t, cfg, "uses: example/tags@v1")
		if want := workflowWith("uses: example/tags@" + tagsSHA1 + " # v1"); got != want {
			t.Errorf("got:\n%swant:\n%s", got, want)
		}
	})

	t.Run("the result passes the online checks", func(t *testing.T) {
		errs, _ := lintOnline(t, tagsClient(t), unpinnedConfig(), run(t, unpinnedConfig(), "uses: example/tags@v1"))
		for _, e := range errs {
			t.Errorf("%s: %s", e.ID, e.Message)
		}
	})
}
