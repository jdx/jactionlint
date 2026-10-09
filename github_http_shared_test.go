package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// An answer fetched with one token serves another token when the repository is public, and only then.
func TestHTTPClientSharesPublicAnswersAcrossTokens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		repo   string
		shared bool
	}{
		{"public", `{"private":false,"visibility":"public","default_branch":"main"}`, true},
		{"private", `{"private":true,"visibility":"private","default_branch":"main"}`, false},
		{"internal", `{"private":true,"visibility":"internal","default_branch":"main"}`, false},
		{"unknown", `{"default_branch":"main"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			f.json("/repos/o/r", tc.repo)
			f.json("/repos/o/r/tags", `[{"name":"v1","commit":{"sha":"`+strings.Repeat("a", 40)+`"}}]`)
			dir := t.TempDir()
			ctx := context.Background()

			a := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "token-a"})
			if _, err := a.Tags(ctx, "o", "r"); err != nil {
				t.Fatal(err)
			}
			// The token of the next CI job, offline: only what the first one left in the cache
			b := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "token-b", Offline: true})
			tags, err := b.Tags(ctx, "o", "r")
			switch {
			case tc.shared && (err != nil || len(tags.Tags) != 1):
				t.Errorf("the answer for a public repository must serve another token: %+v, %v", tags, err)
			case !tc.shared && !errors.Is(err, ErrGitHubNotCached):
				t.Errorf("a private answer must never reach another token: %+v, %v", tags, err)
			}
			// Nor an anonymous run
			c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true})
			_, err = c.Tags(ctx, "o", "r")
			if tc.shared != (err == nil) {
				t.Errorf("anonymous: shared=%v but err=%v", tc.shared, err)
			}
		})
	}
}

// A repository that was public and is private now stops being shared at the next fetch.
func TestHTTPClientStopsSharingWhenARepositoryTurnsPrivate(t *testing.T) {
	f := newFakeGitHub(t)
	body := `{"private":false,"default_branch":"main"}`
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
	f.handle("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `[]`) })
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"}).Tags(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	body = `{"private":true,"default_branch":"main"}`
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "a", TTL: 0}).Repository(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	// ttl 0: the tags are fetched again with a new client that now knows the repository is private
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"}).Tags(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "b", Offline: true}).Tags(ctx, "o", "r"); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("the repository is private now: %v", err)
	}
}

// The GraphQL scan follows the same wait and stale policy as the REST requests when the limit is used up.
func TestGraphQLHonorsARecordedRateLimit(t *testing.T) {
	sha := strings.Repeat("a", 40)
	body := `{"data":{"repository":{"refs":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"name":"b","compare":{"status":"DIVERGED"}}]}}}}`
	run := func(t *testing.T, reset time.Duration, seed bool) (GitHubBranchScan, *fakeGitHub, int, error) {
		f := newFakeGitHub(t)
		posts := 0
		f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) { posts++; fmt.Fprint(w, body) })
		dir := t.TempDir()
		if seed {
			// An expired answer in the cache of the token
			c := f.client(httpGitHubOptions{CacheDir: dir, Token: "tok", TTL: time.Hour})
			if _, err := c.CommitOnAnyBranch(context.Background(), "o", "r", sha, 10); err != nil {
				t.Fatal(err)
			}
			posts = 0
		}
		c := f.client(httpGitHubOptions{CacheDir: dir, Token: "tok", TTL: time.Hour})
		now := time.Now().Add(2 * time.Hour) // the cached answer is stale
		c.now = func() time.Time { return now }
		c.setRateLimited(now.Add(reset), true)
		scan, err := c.CommitOnAnyBranch(context.Background(), "o", "r", sha, 10)
		return scan, f, posts, err
	}

	// Far away: the stale answer, no request
	scan, _, posts, err := run(t, time.Hour, true)
	if err != nil || !scan.Complete || posts != 0 {
		t.Errorf("stale answer expected without a request: %+v, %v, %d requests", scan, err, posts)
	}
	// Far away and nothing cached: the error, no request
	_, _, posts, err = run(t, time.Hour, false)
	if !errors.Is(err, ErrGitHubRateLimited) || posts != 0 {
		t.Errorf("rate limit error expected without a request: %v, %d requests", err, posts)
	}
	// Soon: it waits, then asks
	scan, f, posts, err := run(t, 3*time.Second, false)
	if err != nil || !scan.Complete || posts != 1 || len(f.sleptFor()) != 1 {
		t.Errorf("want a wait and one request: %+v, %v, %d requests, waits %v", scan, err, posts, f.sleptFor())
	}
}
