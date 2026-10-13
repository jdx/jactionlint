//go:build !js

package jactionlint

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientReleaseDate(t *testing.T) {
	sha := strings.Repeat("a", 40)
	ctx := context.Background()
	published := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	committed := time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)

	t.Run("a release", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.json("/repos/o/r/releases/tags/v1.2.0", `{"tag_name":"v1.2.0","draft":false,"published_at":"2026-10-11T09:00:00Z"}`)
		d, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "v1.2.0", sha)
		if err != nil {
			t.Fatal(err)
		}
		if !d.PublishedAt.Equal(published) || !d.CommittedAt.IsZero() {
			t.Errorf("got %+v", d)
		}
		if f.count("/repos/o/r/git/commits/"+sha) != 0 {
			t.Error("the commit is not asked for when the release has a date")
		}
	})

	t.Run("a tag without a release has the date of the commit", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.json("/repos/o/r/git/commits/"+sha, `{"sha":"`+sha+`","committer":{"date":"2026-10-01T08:30:00Z"},"author":{"date":"2026-09-01T08:30:00Z"}}`)
		d, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "v1.2.0", sha)
		if err != nil {
			t.Fatal(err)
		}
		if !d.PublishedAt.IsZero() || !d.CommittedAt.Equal(committed) {
			t.Errorf("got %+v", d)
		}
	})

	t.Run("a draft is not a release", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.json("/repos/o/r/releases/tags/v1.2.0", `{"draft":true,"published_at":"2026-10-11T09:00:00Z"}`)
		f.json("/repos/o/r/git/commits/"+sha, `{"committer":{"date":"2026-10-01T08:30:00Z"}}`)
		d, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "v1.2.0", sha)
		if err != nil || !d.PublishedAt.IsZero() || !d.CommittedAt.Equal(committed) {
			t.Errorf("got %+v, %v", d, err)
		}
	})

	t.Run("a commit without a tag", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.json("/repos/o/r/git/commits/"+sha, `{"committer":{"date":"2026-10-01T08:30:00Z"}}`)
		d, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "", sha)
		if err != nil || !d.CommittedAt.Equal(committed) {
			t.Errorf("got %+v, %v", d, err)
		}
		if n := f.total(); n != 1 {
			t.Errorf("%d requests", n)
		}
	})

	t.Run("a tag with a slash is looked up by its commit", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.json("/repos/o/r/git/commits/"+sha, `{"committer":{"date":"2026-10-01T08:30:00Z"}}`)
		d, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "save/v1.0.0", sha)
		if err != nil || !d.CommittedAt.Equal(committed) {
			t.Errorf("got %+v, %v", d, err)
		}
		if n := f.total(); n != 1 {
			t.Errorf("%d requests: the tag with a slash must not be put in a path", n)
		}
	})

	t.Run("unknown things have no date, and are no failure", func(t *testing.T) {
		f := newFakeGitHub(t)
		d, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "v9", sha)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, ok := d.Time(); ok {
			t.Errorf("got %+v", d)
		}
	})

	t.Run("a failure is an error", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.handle("/repos/o/r/releases/tags/v1", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) })
		if _, err := f.client(httpGitHubOptions{}).ReleaseDate(ctx, "o", "r", "v1", sha); err == nil {
			t.Error("a server error must not look like a tag without a release")
		}
	})

	t.Run("names are validated", func(t *testing.T) {
		f := newFakeGitHub(t)
		c := f.client(httpGitHubOptions{})
		if _, err := c.ReleaseDate(ctx, "o/x", "r", "v1", sha); err == nil {
			t.Error("an invalid owner")
		}
		d, err := c.ReleaseDate(ctx, "o", "r", "../v1", "not-a-sha")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, ok := d.Time(); ok || f.total() != 0 {
			t.Errorf("an invalid tag and commit send nothing: %d requests", f.total())
		}
	})
}

func TestRecordedReleaseDatesAreServed(t *testing.T) {
	published := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	fx := `{"repos":{"o/r":{"release_dates":{"tags/v1":{"published_at":"2026-10-11T09:00:00Z"},"commits/` + strings.Repeat("a", 40) + `":{"committed_at":"2026-10-01T08:30:00Z"}}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	rec := NewRecordingGitHubClient(c)
	ctx := context.Background()
	d, err := rec.ReleaseDate(ctx, "o", "r", "v1", strings.Repeat("b", 40))
	if err != nil || !d.PublishedAt.Equal(published) {
		t.Fatalf("got %+v, %v", d, err)
	}
	if _, err := rec.ReleaseDate(ctx, "o", "r", "", strings.Repeat("A", 40)); err != nil {
		t.Fatalf("the key of a commit is lower case: %v", err)
	}
	if _, err := rec.ReleaseDate(ctx, "o", "r", "v2", strings.Repeat("b", 40)); err == nil {
		t.Error("a date which was not recorded must fail the test")
	}
	// What was recorded is served again
	b, err := rec.Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	again, err := NewFixtureGitHubClient(b)
	if err != nil {
		t.Fatal(err)
	}
	d, err = again.ReleaseDate(ctx, "o", "r", "v1", "")
	if err != nil || !d.PublishedAt.Equal(published) {
		t.Errorf("got %+v, %v", d, err)
	}
}
