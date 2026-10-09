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

// The visibility that decides whether an answer may enter the shared slot must be fresh: a repository
// body that comes out of the cache (still within the TTL) must not say "public" about a repository that
// turned private since, and a shared copy is not served once its confirmation is old.
func TestHTTPClientSharingFailsClosedOnStaleVisibility(t *testing.T) {
	f := newFakeGitHub(t)
	repoBody := `{"private":false,"default_branch":"main"}`
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, repoBody) })
	f.json("/repos/o/r/tags", `[{"name":"v1","commit":{"sha":"`+strings.Repeat("a", 40)+`"}}]`)
	f.json("/repos/o/r/branches", `[{"name":"main","commit":{"sha":"`+strings.Repeat("b", 40)+`"}}]`)
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Now()
	at := func(c *httpGitHubClient, d time.Duration) *httpGitHubClient {
		c.now = func() time.Time { return t0.Add(d) }
		return c
	}

	// Public: the tags are shared
	a := at(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "a"}), 0)
	if _, err := a.Tags(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := at(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true}), time.Minute).Tags(ctx, "o", "r"); err != nil {
		t.Fatalf("a fresh public copy is shared: %v", err)
	}

	// Ten minutes later the repository is private. Its body is still fresh in the cache (TTL one hour).
	repoBody = `{"private":true,"default_branch":"main"}`
	// The shared copy has outlived its confirmation: nobody gets it, not even offline
	for _, tok := range []string{"", "b"} {
		_, err := at(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true, Token: tok}), 10*time.Minute).Tags(ctx, "o", "r")
		if !errors.Is(err, ErrGitHubNotCached) {
			t.Errorf("token %q: a shared copy with an old confirmation must not be served: %v", tok, err)
		}
	}
	// A token which still reads it fetches something new: the visibility is asked again, not taken from the cache
	a2 := at(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "a"}), 10*time.Minute)
	if _, err := a2.Branches(ctx, "o", "r", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := at(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "b", Offline: true}), 10*time.Minute+time.Second).Branches(ctx, "o", "r", 10); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("the branches of a repository that turned private were shared: %v", err)
	}
	if _, err := at(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true}), 10*time.Minute+time.Second).Branches(ctx, "o", "r", 10); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("anonymous: the branches of a repository that turned private were shared: %v", err)
	}
}

// When the visibility cannot be refreshed nothing is shared.
func TestHTTPClientDoesNotShareWhenVisibilityCannotBeRefreshed(t *testing.T) {
	f := newFakeGitHub(t)
	repoOK := true
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		if !repoOK {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"private":false,"default_branch":"main"}`)
	})
	f.json("/repos/o/r/tags", `[]`)
	f.json("/repos/o/r/branches", `[]`)
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Now()
	a := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "a"})
	a.now = func() time.Time { return t0 }
	if _, err := a.Tags(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	repoOK = false
	a2 := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "a"})
	a2.now = func() time.Time { return t0.Add(10 * time.Minute) }
	_, _ = a2.Branches(ctx, "o", "r", 10)
	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true})
	c.now = func() time.Time { return t0.Add(10*time.Minute + time.Second) }
	if _, err := c.Branches(ctx, "o", "r", 10); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("shared although the visibility could not be refreshed: %v", err)
	}
}

// A copy that a token made for everybody must not replace, hide or take along what the server answered to a
// request without a token.
func TestHTTPClientSharingKeepsTheAnonymousSlot(t *testing.T) {
	f := newFakeGitHub(t)
	repoBody := `{"private":false,"default_branch":"main"}`
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, repoBody) })
	f.handle("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			fmt.Fprint(w, `[{"name":"anon","commit":{"sha":"`+strings.Repeat("a", 40)+`"}}]`)
			return
		}
		fmt.Fprint(w, `[{"name":"auth1","commit":{"sha":"`+strings.Repeat("b", 40)+`"}},{"name":"auth2","commit":{"sha":"`+strings.Repeat("c", 40)+`"}}]`)
	})
	dir := t.TempDir()
	ctx := context.Background()
	tagNames := func(c *httpGitHubClient) string {
		t.Helper()
		tags, err := c.Tags(ctx, "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tag := range tags.Tags {
			names = append(names, tag.Name)
		}
		return strings.Join(names, ",")
	}

	if got := tagNames(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})); got != "anon" {
		t.Fatalf("anonymous: %q", got)
	}
	// A token asks again (no TTL) and shares its answer
	if got := tagNames(f.client(httpGitHubOptions{CacheDir: dir, Token: "a"})); got != "auth1,auth2" {
		t.Fatalf("token: %q", got)
	}
	// The anonymous answer is still the anonymous one
	if got := tagNames(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true})); got != "anon" {
		t.Errorf("the answer of a token replaced the anonymous one: %q", got)
	}
	// The repository turns private: the shared copy goes, the anonymous answer stays
	repoBody = `{"private":true,"default_branch":"main"}`
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"}).Repository(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if got := tagNames(f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Offline: true})); got != "anon" {
		t.Errorf("purging the shared copy took the anonymous answer along: %q", got)
	}
}

// Only the status codes that the client reports as errors reach the caller of the repository endpoint: a 403
// (not a rate limit) must purge the shared copy too.
func TestHTTPClientRepositoryForbiddenPurgesTheSharedCopy(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"private":false,"default_branch":"main"}`)
	})
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"}).Repository(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Offline: true}).Repository(ctx, "o", "r"); err != nil {
		t.Fatalf("a fresh public copy is shared: %v", err)
	}
	// Now every request is refused, with or without a token
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Resource not accessible"}`, http.StatusForbidden)
	})
	_, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"}).Repository(ctx, "o", "r")
	if err == nil {
		t.Fatal("want the 403")
	}
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, Offline: true}).Repository(ctx, "o", "r"); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("the shared copy survived a 403: %v", err)
	}
}
