package jactionlint

import (
	"strings"
	"testing"
)

// With more tags than were read, the most specific tag of the commit may be missing: the comment names the tag as written
// and does not pick a less specific one from the partial list.
func TestPinFixCommentWithTruncatedTags(t *testing.T) {
	sha := strings.Repeat("7", 40)
	fx := `{"repos":{"example/many":{"tags":{"tags":[{"name":"v4","sha":"` + sha + `"},{"name":"v4.2","sha":"` + sha + `"}],"truncated":true},` +
		`"refs":{"heads/v4":{"found":false}}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	src := workflowWith("uses: example/many@v4")
	errs, _ := lintOnline(t, c, unpinnedConfig(), src)
	got, n := applyFixes([]byte(src), errs, FixModeSafe)
	if want := workflowWith("uses: example/many@" + sha + " # v4"); n != 1 || string(got) != want {
		t.Errorf("got %d fixes:\n%s\nwant:\n%s", n, got, want)
	}
}
