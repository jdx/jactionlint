package jactionlint

import (
	"context"
	"errors"
	"testing"
	"time"
)

func releaseDateFixture(t *testing.T, data string) *FixtureGitHubClient {
	t.Helper()
	c, err := NewFixtureGitHubClient([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The date of a release is looked up on the host which has the repository.
func TestFallbackClientReleaseDatesAreOfTheHostWhichHasTheRepository(t *testing.T) {
	primary := releaseDateFixture(t, `{"repos":{"corp/a":{"repo":{},"release_dates":{"tags/v1":{"published_at":"2026-10-01T00:00:00Z"}}},"actions/checkout":{"missing":true},"both/x":{"repo":{},"release_dates":{"tags/v1":{"published_at":"2026-10-02T00:00:00Z"}}}}}`)
	secondary := releaseDateFixture(t, `{"repos":{"actions/checkout":{"repo":{},"release_dates":{"tags/v4.2.2":{"published_at":"2024-10-23T00:00:00Z"}}},"both/x":{"repo":{},"release_dates":{"tags/v1":{"published_at":"2020-01-01T00:00:00Z"}}}}}`)
	c := newFallbackGitHubClient(primary, secondary)
	ctx := context.Background()

	if d, err := c.ReleaseDate(ctx, "corp", "a", "v1", ""); err != nil || d.PublishedAt.Year() != 2026 {
		t.Errorf("the Server has it: %+v, %v", d, err)
	}
	before := primary.Calls()
	if d, err := c.ReleaseDate(ctx, "actions", "checkout", "v4.2.2", ""); err != nil || d.PublishedAt.Year() != 2024 {
		t.Errorf("only github.com has it: %+v, %v", d, err)
	}
	if primary.Calls() != before+1 { // The question where the repository is, and nothing else
		t.Errorf("the Server was asked %d times about a repository of github.com", primary.Calls()-before)
	}
	if d, err := c.ReleaseDate(ctx, "both", "x", "v1", ""); err != nil || d.PublishedAt.Year() != 2026 {
		t.Errorf("the Server must win: %+v, %v", d, err)
	}
}

// A github.com which is down answers no date, as a failure of the host, and the Server goes on.
func TestFallbackClientReleaseDateWithGitHubComDown(t *testing.T) {
	primary := releaseDateFixture(t, `{"repos":{"corp/a":{"repo":{},"release_dates":{"tags/v1":{"published_at":"2026-10-01T00:00:00Z"}}},"actions/checkout":{"missing":true},"actions/setup-go":{"missing":true}}}`)
	secondary := &stubClient{err: &GitHubRateLimitError{Reset: time.Now().Add(time.Hour)}}
	c := newFallbackGitHubClient(primary, secondary)
	ctx := context.Background()
	_, err := c.ReleaseDate(ctx, "actions", "checkout", "v4.2.2", "")
	var scoped *hostScopedError
	if !errors.As(err, &scoped) || !errors.Is(err, ErrGitHubRateLimited) {
		t.Errorf("want the rate limit of the host: %v", err)
	}
	if d, err := c.ReleaseDate(ctx, "corp", "a", "v1", ""); err != nil || d.PublishedAt.IsZero() {
		t.Errorf("the Server must go on: %+v, %v", d, err)
	}
	if _, err := c.ReleaseDate(ctx, "actions", "setup-go", "v7.0.0", ""); !errors.Is(err, ErrGitHubRateLimited) {
		t.Errorf("%v", err)
	}
	if secondary.n.Load() != 1 {
		t.Errorf("github.com was asked %d times", secondary.n.Load())
	}
}

// A client of the pair which cannot tell the date says so.
func TestFallbackClientReleaseDateWithoutLookup(t *testing.T) {
	c := newFallbackGitHubClient(&stubClient{}, &stubClient{})
	c.primary = &stubClient{repo: &GitHubRepo{}}
	if _, err := c.ReleaseDate(context.Background(), "o", "r", "v1", ""); !errors.Is(err, errReleaseDatesUnsupported) {
		t.Errorf("%v", err)
	}
}

// The rule works through the wrapper, and a client which cannot tell dates is a skipped lookup which is said, not silence.
func TestActionReleaseCooldownThroughTheFallbackAndWithoutDates(t *testing.T) {
	cfg := cooldownCfg(t, "  action-release-cooldown: warning\n")
	primary := releaseDateFixture(t, `{"repos":{"corp/other":{"repo":{}},"example/fresh":{"missing":true}}}`)
	secondary := onlineFixtureClient(t)
	errs, _ := lintOnline(t, newFallbackGitHubClient(primary, secondary), cfg, workflowWith("uses: "+freshA))
	var n int
	for _, e := range errs {
		if e.ID == "action-release-cooldown" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("a repository of github.com is checked through the wrapper: %d findings", n)
	}

	errs, l := lintOnline(t, plainGitHubClient{onlineFixtureClient(t)}, cfg, workflowWith("uses: "+freshA))
	for _, e := range errs {
		if e.ID == "action-release-cooldown" {
			t.Errorf("a client without dates reports nothing: %v", e)
		}
	}
	if l.OnlineSkipped() != 1 {
		t.Errorf("the lookup which could not be made is counted: %d", l.OnlineSkipped())
	}
}
