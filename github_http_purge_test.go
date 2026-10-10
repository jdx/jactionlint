package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A 401 on the repository endpoint (the token was revoked) purges the shared copy through the same fail-closed
// route as a 403 or 404: afterTokenRequest sees that the token was dropped.
func TestHTTPClientRepositoryUnauthorizedPurgesTheSharedCopy(t *testing.T) {
	for _, tc := range []struct {
		name string
		anon func(w http.ResponseWriter) // what the repository endpoint answers once the token is dropped
	}{
		{"anonymous retry is refused too", func(w http.ResponseWriter) { http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized) }},
		{"anonymous retry says not found", func(w http.ResponseWriter) { http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			revoked := false
			f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
				if revoked {
					if r.Header.Get("Authorization") != "" {
						http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
						return
					}
					tc.anon(w)
					return
				}
				fmt.Fprint(w, `{"private":false,"default_branch":"main"}`)
			})
			f.json("/repos/o/r/tags", `[{"name":"v1","commit":{"sha":"`+strings.Repeat("a", 40)+`"}}]`)
			dir := t.TempDir()
			ctx := context.Background()
			a := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"})
			if _, err := a.Repository(ctx, "o", "r"); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Tags(ctx, "o", "r"); err != nil {
				t.Fatal(err)
			}
			if _, err := f.client(httpGitHubOptions{CacheDir: dir, Offline: true}).Tags(ctx, "o", "r"); err != nil {
				t.Fatalf("a fresh public copy is shared: %v", err)
			}
			if _, err := f.client(httpGitHubOptions{CacheDir: dir, Offline: true}).Repository(ctx, "o", "r"); err != nil {
				t.Fatalf("the repository is shared too: %v", err)
			}

			revoked = true
			b := f.client(httpGitHubOptions{CacheDir: dir, Token: "a"})
			_, _ = b.Repository(ctx, "o", "r") // the answer is not the point: what stays in the cache is
			if b.currentToken() != "" {
				t.Fatal("the 401 must drop the token")
			}
			for _, tok := range []string{"", "other"} {
				// A not found that the anonymous retry cached is an anonymous answer, not the shared copy
				c := f.client(httpGitHubOptions{CacheDir: dir, Token: tok, Offline: true})
				if repo, err := c.Repository(ctx, "o", "r"); err == nil {
					t.Errorf("token %q: the shared repository copy survived a 401: %+v", tok, repo)
				}
			}
			// The shared copies of the other requests are blocked for the rest of the run that saw the 401...
			b.offline = true
			if _, err := b.Tags(ctx, "o", "r"); !errors.Is(err, ErrGitHubNotCached) {
				t.Errorf("the shared tags were served after the 401: %v", err)
			}
			// ... and expire with their confirmation in the runs that did not
			c := f.client(httpGitHubOptions{CacheDir: dir, Offline: true})
			c.now = func() time.Time { return time.Now().Add(sharedVisibilityTTL + time.Minute) }
			if _, err := c.Tags(ctx, "o", "r"); !errors.Is(err, ErrGitHubNotCached) {
				t.Errorf("the shared tags outlived their confirmation: %v", err)
			}
		})
	}
}

// An answer shared by an earlier version sits in the anonymous slot, carries no confirmation time and is not
// migrated: it is an anonymous entry like any other, served within the TTL and expired through the freshness
// check (the TTL), after which the server is asked again.
func TestHTTPClientAnonymousSlotEntryExpiresThroughTheFreshnessCheck(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/tags", `[{"name":"new","commit":{"sha":"`+strings.Repeat("b", 40)+`"}}]`)
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Now()

	// Plant what an earlier commit of the stack wrote: a token's answer in the anonymous slot
	probe := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	u := probe.base.String() + "/repos/o/r/tags?per_page=100"
	probe.cache.put(cacheKey(probe.scope(""), u), &cacheEntry{
		URL: u, Fetched: t0, Status: http.StatusOK,
		Body: []byte(`[{"name":"old","commit":{"sha":"` + strings.Repeat("a", 40) + `"}}]`),
	})
	name := func(d time.Duration) string {
		t.Helper()
		c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
		c.now = func() time.Time { return t0.Add(d) }
		tags, err := c.Tags(ctx, "o", "r")
		if err != nil {
			t.Fatal(err)
		}
		return tags.Tags[0].Name
	}
	if got := name(time.Minute); got != "old" || f.count("/repos/o/r/tags") != 0 {
		t.Fatalf("within the TTL the entry is served: %q, %d requests", got, f.count("/repos/o/r/tags"))
	}
	if got := name(2 * time.Hour); got != "new" || f.count("/repos/o/r/tags") != 1 {
		t.Errorf("an expired entry must be fetched again: %q, %d requests", got, f.count("/repos/o/r/tags"))
	}
}

// A GraphQL scan whose request is answered with a rate limit that ends later than the client waits follows the
// REST policy: the expired answer of the cache is used when there is one, the error otherwise.
func TestGraphQLRateLimitedAnswerFallsBackToStale(t *testing.T) {
	sha := strings.Repeat("a", 40)
	body := `{"data":{"repository":{"refs":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"name":"b","compare":{"status":"DIVERGED"}}]}}}}`
	for _, seed := range []bool{true, false} {
		t.Run(fmt.Sprintf("seeded=%v", seed), func(t *testing.T) {
			f := newFakeGitHub(t)
			limited := false
			var posts atomic.Int32
			f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				if limited {
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
					return
				}
				fmt.Fprint(w, body)
			})
			dir := t.TempDir()
			ctx := context.Background()
			if seed {
				if _, err := f.client(httpGitHubOptions{CacheDir: dir, Token: "tok", TTL: time.Hour}).CommitOnAnyBranch(ctx, "o", "r", sha, 10); err != nil {
					t.Fatal(err)
				}
			}
			limited = true
			c := f.client(httpGitHubOptions{CacheDir: dir, Token: "tok", TTL: time.Hour})
			now := time.Now().Add(2 * time.Hour) // the cached answer is stale
			c.now = func() time.Time { return now }
			posts.Store(0)
			scan, err := c.CommitOnAnyBranch(ctx, "o", "r", sha, 10)
			if seed && (err != nil || !scan.Complete) {
				t.Errorf("the stale answer is better than none: %+v, %v", scan, err)
			}
			if !seed && !errors.Is(err, ErrGitHubRateLimited) {
				t.Errorf("nothing cached: want the rate limit error, got %+v, %v", scan, err)
			}
			if posts.Load() != 1 || len(f.sleptFor()) != 0 {
				t.Errorf("want one request and no wait: %d requests, waits %v", posts.Load(), f.sleptFor())
			}
			// The limit is recorded: the next scan does not ask again
			if _, err := c.CommitOnAnyBranch(ctx, "o", "r", sha, 10); seed != (err == nil) || posts.Load() != 1 {
				t.Errorf("second scan: %v after %d requests", err, posts.Load())
			}
		})
	}
}
