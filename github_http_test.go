//go:build !js

package jactionlint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGitHub is a minimal GitHub API for the tests of the HTTP client. Handlers are registered by path.
type fakeGitHub struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	handlers map[string]http.HandlerFunc
	hits     map[string]int // by path and query
	auth     []string       // Authorization headers seen
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{t: t, handlers: map[string]http.HandlerFunc{}, hits: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		key := r.URL.Path
		f.hits[key]++
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		h := f.handlers[key]
		f.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) handle(path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[path] = h
}

// json registers a path which answers the JSON with an ETag, and revalidates it.
func (f *fakeGitHub) json(path, body string) {
	etag := fmt.Sprintf("%q", fmt.Sprint(len(body)))
	f.handle(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	})
}

func (f *fakeGitHub) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func (f *fakeGitHub) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.hits {
		n += c
	}
	return n
}

func (f *fakeGitHub) client(opts httpGitHubOptions) *httpGitHubClient {
	f.t.Helper()
	opts.BaseURL = f.srv.URL
	if opts.CacheDir == "" {
		opts.CacheDir = f.t.TempDir()
	}
	c, err := newHTTPGitHubClient(opts)
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

func TestHTTPClientTTLAndETag(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r", `{"archived":true,"default_branch":"trunk"}`)
	dir := t.TempDir()
	ctx := context.Background()

	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	for i := 0; i < 3; i++ {
		r, err := c.Repository(ctx, "o", "r")
		if err != nil || !r.Archived || r.DefaultBranch != "trunk" {
			t.Fatalf("Repository() = %+v, %v", r, err)
		}
	}
	if n := f.count("/repos/o/r"); n != 1 {
		t.Errorf("within the TTL one request is enough but got %d", n)
	}

	// A new process (client) with the same cache directory sees the entry too
	c2 := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	if _, err := c2.Repository(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/repos/o/r"); n != 1 {
		t.Errorf("the cache should be shared between clients but got %d requests", n)
	}

	// With a TTL of zero the answer is revalidated with its ETag, and the 304 answer is served from the cache
	c3 := f.client(httpGitHubOptions{CacheDir: dir, TTL: 0})
	r, err := c3.Repository(ctx, "o", "r")
	if err != nil || r.DefaultBranch != "trunk" {
		t.Fatalf("Repository() = %+v, %v", r, err)
	}
	if n := f.count("/repos/o/r"); n != 2 {
		t.Errorf("want a revalidation request but got %d requests", n)
	}
}

func TestHTTPClientCacheIsPerToken(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r", `{"default_branch":"main"}`)
	dir := t.TempDir()
	ctx := context.Background()
	for _, tok := range []string{"token-a", "token-b", ""} {
		c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: tok})
		if _, err := c.Repository(ctx, "o", "r"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("/repos/o/r"); n != 3 {
		t.Errorf("each credential has its own cache entries but got %d requests", n)
	}
	if f.auth[0] != "Bearer token-a" || f.auth[2] != "" {
		t.Errorf("unexpected Authorization headers: %q", f.auth)
	}
}

func TestHTTPClientNotFoundIsRememberedAndTyped(t *testing.T) {
	f := newFakeGitHub(t)
	c := f.client(httpGitHubOptions{TTL: time.Hour})
	for i := 0; i < 2; i++ {
		if _, err := c.Repository(context.Background(), "o", "gone"); !errors.Is(err, ErrGitHubNotFound) {
			t.Fatalf("want ErrGitHubNotFound but got %v", err)
		}
	}
	if n := f.count("/repos/o/gone"); n != 1 {
		t.Errorf("a missing repository should be asked for once but got %d requests", n)
	}
	sha, found, err := c.ResolveRef(context.Background(), "o", "gone", GitHubRefTags, "v1")
	if err != nil || found || sha != "" {
		t.Errorf("a missing ref is not an error: %q, %v, %v", sha, found, err)
	}
}

func TestHTTPClientRateLimitStopsRequests(t *testing.T) {
	f := newFakeGitHub(t)
	reset := time.Now().Add(time.Hour).Unix()
	f.json("/repos/o/cached", `{"default_branch":"main"}`)
	f.handle("/repos/o/limited", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded for 1.2.3.4."}`)
	})
	dir := t.TempDir()
	// Fill the cache, then expire it
	warm := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "tok"})
	if _, err := warm.Repository(context.Background(), "o", "cached"); err != nil {
		t.Fatal(err)
	}
	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: 0, Token: "tok"})

	_, err := c.Repository(context.Background(), "o", "limited")
	var rl *GitHubRateLimitError
	if !errors.Is(err, ErrGitHubRateLimited) || !errors.As(err, &rl) || rl.Reset.Unix() != reset || !rl.Authenticated {
		t.Fatalf("want a rate limit error resetting at %d but got %v", reset, err)
	}
	before := f.total()
	for i := 0; i < 5; i++ {
		if _, err := c.Repository(context.Background(), "o", "other"); !errors.Is(err, ErrGitHubRateLimited) {
			t.Fatalf("want a rate limit error but got %v", err)
		}
	}
	if f.total() != before {
		t.Errorf("no request must be made once the limit is reached but got %d", f.total()-before)
	}
	// An expired entry is better than nothing
	r, err := c.Repository(context.Background(), "o", "cached")
	if err != nil || r.DefaultBranch != "main" {
		t.Errorf("a stale cache entry should be used when rate limited: %+v, %v", r, err)
	}
	if !strings.Contains(err2s(c.Repository(context.Background(), "o", "other")), "resets at") {
		t.Error("the error should say when the limit resets")
	}
}

func err2s(_ *GitHubRepo, err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestHTTPClientSecondaryRateLimitAndOtherForbidden(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/slow", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"You have exceeded a secondary rate limit."}`)
	})
	f.handle("/repos/o/blocked", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Repository access blocked"}`)
	})
	c := f.client(httpGitHubOptions{})
	if _, err := c.Repository(context.Background(), "o", "blocked"); err == nil || errors.Is(err, ErrGitHubRateLimited) {
		t.Errorf("a blocked repository is not a rate limit: %v", err)
	} else {
		var se *GitHubStatusError
		if !errors.As(err, &se) || se.Status != 403 || !strings.Contains(err.Error(), "blocked") {
			t.Errorf("want a status error but got %v", err)
		}
	}
	_, err := c.Repository(context.Background(), "o", "slow")
	var rl *GitHubRateLimitError
	if !errors.As(err, &rl) || time.Until(rl.Reset) < 20*time.Second {
		t.Errorf("want a rate limit error honoring Retry-After but got %v", err)
	}
}

func TestHTTPClientRejectedTokenFallsBackOnce(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
			return
		}
		fmt.Fprint(w, `{"default_branch":"main"}`)
	})
	var notices []string
	c := f.client(httpGitHubOptions{Token: "stale", Notify: func(m string) { notices = append(notices, m) }})
	if _, err := c.Repository(context.Background(), "o", "r"); err != nil {
		t.Fatalf("the request should succeed without the rejected token: %v", err)
	}
	f.handle("/repos/o/r2", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) })
	if _, err := c.Repository(context.Background(), "o", "r2"); err != nil {
		t.Fatal(err)
	}
	rejected := 0
	for _, n := range notices {
		if strings.Contains(n, "rejected") {
			rejected++
		}
	}
	if rejected != 1 {
		t.Errorf("the rejected token should be mentioned once: %q", notices)
	}
	// 1 attempt with the token, then only anonymous requests
	withAuth := 0
	for _, a := range f.auth {
		if a != "" {
			withAuth++
		}
	}
	if withAuth != 1 {
		t.Errorf("the rejected token must not be sent again: %d requests had it", withAuth)
	}
}

func TestHTTPClientUnauthenticatedNoticeOnce(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/a", `{}`)
	f.json("/repos/o/b", `{}`)
	var notices []string
	c := f.client(httpGitHubOptions{Notify: func(m string) { notices = append(notices, m) }})
	c.Repository(context.Background(), "o", "a")
	c.Repository(context.Background(), "o", "b")
	if len(notices) != 1 || !strings.Contains(notices[0], "unauthenticated") {
		t.Errorf("want one notice about missing token: %q", notices)
	}
	c = f.client(httpGitHubOptions{Token: "x", Notify: func(m string) { notices = append(notices, m) }})
	c.Repository(context.Background(), "o", "a")
	if len(notices) != 1 {
		t.Errorf("no notice with a token: %q", notices)
	}
}

func TestHTTPClientServerErrorIsAnError(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	c := f.client(httpGitHubOptions{})
	_, err := c.Repository(context.Background(), "o", "r")
	var se *GitHubStatusError
	if !errors.As(err, &se) || se.Status != 502 {
		t.Errorf("want a status error but got %v", err)
	}
	if n := f.count("/repos/o/r"); n != 1 {
		t.Errorf("a failed request must not be retried but got %d requests", n)
	}
}

func TestHTTPClientTagsPaginationAndTruncation(t *testing.T) {
	f := newFakeGitHub(t)
	page := func(n int, next bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if next {
				w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/tags?per_page=100&page=%d>; rel="next", <%s/x>; rel="last"`, f.srv.URL, n+1, f.srv.URL))
			}
			fmt.Fprintf(w, `[{"name":"v%d","commit":{"sha":"%040d"}}]`, n, n)
		}
	}
	f.handle("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			page(1, true)(w, r)
		case "2":
			page(2, true)(w, r)
		default:
			page(3, false)(w, r)
		}
	})
	c := f.client(httpGitHubOptions{})
	l, err := c.Tags(context.Background(), "o", "r")
	if err != nil || len(l.Tags) != 3 || l.Truncated {
		t.Fatalf("Tags() = %+v, %v", l, err)
	}
	if l.Tags[2].Name != "v3" || l.Tags[2].SHA != fmt.Sprintf("%040d", 3) {
		t.Errorf("unexpected tags: %+v", l.Tags)
	}

	// A list that never ends is cut
	f2 := newFakeGitHub(t)
	f2.handle("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/tags?page=9>; rel="next"`, f2.srv.URL))
		fmt.Fprint(w, `[{"name":"v1","commit":{"sha":"abc"}}]`)
	})
	l, err = f2.client(httpGitHubOptions{}).Tags(context.Background(), "o", "r")
	if err != nil || !l.Truncated || len(l.Tags) != maxGitHubPages {
		t.Errorf("want a truncated list of %d tags but got %d, truncated=%v, %v", maxGitHubPages, len(l.Tags), l != nil && l.Truncated, err)
	}
}

func TestHTTPClientRefusesForeignNextLink(t *testing.T) {
	var foreign atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreign.Add(1)
		fmt.Fprint(w, `[]`)
	}))
	defer other.Close()
	f := newFakeGitHub(t)
	f.handle("/repos/o/r/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/steal>; rel="next"`, other.URL))
		fmt.Fprint(w, `[]`)
	})
	c := f.client(httpGitHubOptions{Token: "secret"})
	if _, err := c.Tags(context.Background(), "o", "r"); err == nil {
		t.Error("a next link to another host should be refused")
	}
	if foreign.Load() != 0 {
		t.Error("the token must not be sent to another host")
	}
}

func TestHTTPClientResolveRefDereferencesAnnotatedTags(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/git/ref/tags/v1", `{"object":{"type":"tag","sha":"t1"}}`)
	f.json("/repos/o/r/git/tags/t1", `{"object":{"type":"tag","sha":"t2"}}`)
	f.json("/repos/o/r/git/tags/t2", `{"object":{"type":"commit","sha":"c0ffee"}}`)
	f.json("/repos/o/r/git/ref/tags/light", `{"object":{"type":"commit","sha":"deadbeef"}}`)
	f.json("/repos/o/r/git/ref/tags/tree", `{"object":{"type":"tree","sha":"abc"}}`)
	f.json("/repos/o/r/git/ref/heads/feature/x", `{"object":{"type":"commit","sha":"f00"}}`)
	c := f.client(httpGitHubOptions{})
	for _, tc := range []struct {
		ns    GitHubRefNamespace
		name  string
		sha   string
		found bool
	}{
		{GitHubRefTags, "v1", "c0ffee", true},
		{GitHubRefTags, "light", "deadbeef", true},
		{GitHubRefTags, "tree", "", false},
		{GitHubRefTags, "nope", "", false},
		{GitHubRefHeads, "feature/x", "f00", true},
	} {
		sha, found, err := c.ResolveRef(context.Background(), "o", "r", tc.ns, tc.name)
		if err != nil || sha != tc.sha || found != tc.found {
			t.Errorf("ResolveRef(%s/%s) = %q, %v, %v. want %q, %v", tc.ns, tc.name, sha, found, err, tc.sha, tc.found)
		}
	}
}

func TestHTTPClientTagDerefLoopIsBounded(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/git/ref/tags/loop", `{"object":{"type":"tag","sha":"t"}}`)
	f.json("/repos/o/r/git/tags/t", `{"object":{"type":"tag","sha":"t"}}`)
	c := f.client(httpGitHubOptions{})
	_, found, err := c.ResolveRef(context.Background(), "o", "r", GitHubRefTags, "loop")
	if err != nil || found {
		t.Errorf("a tag that points to itself resolves to nothing: %v, %v", found, err)
	}
	if n := f.count("/repos/o/r/git/tags/t"); n > maxTagDerefs {
		t.Errorf("followed %d tag objects", n)
	}
}

func TestHTTPClientCompareBranchesAndAdvisories(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r/compare/base...head", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "1" {
			t.Error("the comparison should ask for one commit only")
		}
		fmt.Fprint(w, `{"status":"behind"}`)
	})
	f.handle("/repos/o/r/compare/base...weird", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"sideways"}`)
	})
	f.handle("/repos/o/r/branches", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "2" {
			t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/branches?page=2>; rel="next"`, f.srv.URL))
		fmt.Fprint(w, `[{"name":"main","commit":{"sha":"a"}},{"name":"dev","commit":{"sha":"b"}}]`)
	})
	f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("ecosystem") != "actions" || q.Get("affects") != "o/r" {
			t.Errorf("unexpected query %v", q)
		}
		fmt.Fprint(w, `[
		 {"ghsa_id":"GHSA-1","summary":"s","severity":"high","html_url":"u","vulnerabilities":[
		   {"package":{"ecosystem":"actions","name":"o/r"},"vulnerable_version_range":"< 2","first_patched_version":"2"},
		   {"package":{"ecosystem":"npm","name":"o/r"},"vulnerable_version_range":"< 9"}]},
		 {"ghsa_id":"GHSA-2","withdrawn_at":"2024-01-01T00:00:00Z","vulnerabilities":[]},
		 {"ghsa_id":"GHSA-3","vulnerabilities":[{"package":{"ecosystem":"actions","name":"o/r"},"vulnerable_version_range":"<= 1","first_patched_version":{"identifier":"2"}}]}]`)
	})
	c := f.client(httpGitHubOptions{})
	ctx := context.Background()

	st, err := c.Compare(ctx, "o", "r", "base", "head")
	if err != nil || st != GitHubCompareBehind {
		t.Errorf("Compare() = %q, %v", st, err)
	}
	if _, err := c.Compare(ctx, "o", "r", "base", "weird"); err == nil {
		t.Error("an unknown status should be an error")
	}
	if _, err := c.Compare(ctx, "o", "r", "base", "unknown"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("an unknown commit is not found: %v", err)
	}
	bs, err := c.Branches(ctx, "o", "r", 2)
	if err != nil || len(bs.Branches) != 2 || !bs.Truncated || bs.Branches[1].Name != "dev" {
		t.Errorf("Branches() = %+v, %v", bs, err)
	}
	advs, err := c.Advisories(ctx, "o", "r")
	if err != nil || len(advs) != 2 {
		t.Fatalf("Advisories() = %+v, %v", advs, err)
	}
	if advs[0].ID != "GHSA-1" || len(advs[0].Vulnerabilities) != 1 || advs[0].Vulnerabilities[0].FirstPatched != "2" {
		t.Errorf("unexpected advisory %+v", advs[0])
	}
	if advs[1].Vulnerabilities[0].FirstPatched != "" {
		t.Errorf("an object as first_patched_version is not understood and must be empty: %+v", advs[1])
	}
}

func TestHTTPClientAdvisoriesOfSeveralPackages(t *testing.T) {
	f := newFakeGitHub(t)
	var requests int
	f.handle("/advisories", func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("affects"); got != "o/r,o/r/sub" {
			t.Errorf("affects = %q", got)
		}
		// The same advisory is listed for both packages
		fmt.Fprint(w, `[
		 {"ghsa_id":"GHSA-1","vulnerabilities":[{"package":{"ecosystem":"actions","name":"o/r"},"vulnerable_version_range":"< 2"}]},
		 {"ghsa_id":"GHSA-1","vulnerabilities":[{"package":{"ecosystem":"actions","name":"o/r/sub"},"vulnerable_version_range":"< 2"}]},
		 {"ghsa_id":"GHSA-2","vulnerabilities":[{"package":{"ecosystem":"actions","name":"o/r/sub"},"vulnerable_version_range":"< 3"}]}]`)
	})
	c := f.client(httpGitHubOptions{})
	advs, err := c.AdvisoriesForPackages(context.Background(), []string{"o/r", "o/r/sub"})
	if err != nil || len(advs) != 2 || advs[0].ID != "GHSA-1" || advs[1].ID != "GHSA-2" {
		t.Fatalf("AdvisoriesForPackages() = %+v, %v", advs, err)
	}
	if requests != 1 {
		t.Errorf("want one request but got %d", requests)
	}
}

func TestHTTPClientContextCancellation(t *testing.T) {
	f := newFakeGitHub(t)
	release := make(chan struct{})
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	c := f.client(httpGitHubOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Repository(ctx, "o", "r")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want a deadline error but got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("cancellation should return promptly")
	}
}

func TestHTTPClientLimitsRequestsInFlight(t *testing.T) {
	f := newFakeGitHub(t)
	var cur, peak atomic.Int32
	f.handle("/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
		fmt.Fprint(w, `{}`)
	})
	c := f.client(httpGitHubOptions{CacheDir: filepath.Join(t.TempDir(), "none")})
	c.cache = nil // Every call must reach the server
	var wg sync.WaitGroup
	for i := 0; i < 4*maxGitHubInFlight; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Repository(context.Background(), "o", "r")
		}()
	}
	wg.Wait()
	if p := peak.Load(); p > maxGitHubInFlight {
		t.Errorf("%d requests ran at once. the limit is %d", p, maxGitHubInFlight)
	}
}

func TestHTTPClientInvalidBaseURL(t *testing.T) {
	for _, u := range []string{"ftp://x", "not a url", "https://"} {
		if _, err := newHTTPGitHubClient(httpGitHubOptions{BaseURL: u}); err == nil {
			t.Errorf("%q should be refused", u)
		}
	}
}

func TestEscapeRefName(t *testing.T) {
	if got := escapeRefName("feature/a b#c"); got != "feature/a%20b%23c" {
		t.Errorf("escapeRefName = %q", got)
	}
}

func TestNextLink(t *testing.T) {
	tests := map[string]string{
		``:                                 "",
		`<https://x/a?page=2>; rel="next"`: "https://x/a?page=2",
		`<https://x/a?page=1>; rel="prev", <https://x/a?page=3>; rel="next"`: "https://x/a?page=3",
		`<https://x/a>; rel="last"`: "",
		`garbage`:                   "",
	}
	for in, want := range tests {
		if got := nextLink(in); got != want {
			t.Errorf("nextLink(%q) = %q. want %q", in, got, want)
		}
	}
}

func FuzzNextLink(f *testing.F) {
	f.Add(`<https://x/a?page=2>; rel="next"`)
	f.Add(`;;<>,,`)
	f.Fuzz(func(t *testing.T, s string) { nextLink(s) })
}

func TestDefaultCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	if got := defaultGitHubCacheDir(); got != filepath.Join("/xdg/cache", "jactionlint") && !strings.HasSuffix(got, "jactionlint") {
		t.Errorf("defaultGitHubCacheDir() = %q", got)
	}
	t.Setenv("XDG_CACHE_HOME", "relative/path") // Not absolute: ignored, as the XDG spec says
	if got := defaultGitHubCacheDir(); strings.HasPrefix(got, "relative") {
		t.Errorf("a relative XDG_CACHE_HOME must be ignored: %q", got)
	}
}

func TestGitHubTokenFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	if githubTokenFromEnv() != "" {
		t.Error("no token expected")
	}
	t.Setenv("GH_TOKEN", " gh ")
	if githubTokenFromEnv() != "gh" {
		t.Error("GH_TOKEN should be used")
	}
	t.Setenv("GITHUB_TOKEN", "ghub")
	if githubTokenFromEnv() != "ghub" {
		t.Error("GITHUB_TOKEN wins")
	}
}

func TestDiskCacheBoundAndConcurrency(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	c := newDiskCache(dir, 20_000)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				url := fmt.Sprintf("http://x/%d/%d", g, i)
				key := cacheKey("s", url)
				c.put(key, &cacheEntry{URL: url, Fetched: time.Now(), Status: 200, Body: []byte(strings.Repeat("x", 1000))})
				if e := c.get(key, url); e != nil && string(e.Body) != strings.Repeat("x", 1000) {
					t.Error("read a torn entry")
				}
			}
		}()
	}
	wg.Wait()
	var total int64
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("leftover temporary file %s", e.Name())
		}
		if i, err := e.Info(); err == nil {
			total += i.Size()
		}
	}
	// The bound is checked when files are added so the directory can exceed it by what was written since
	if total > 20_000+8*1400 {
		t.Errorf("the cache grew to %d bytes. the limit is 20000", total)
	}
	if total == 0 {
		t.Error("the cache is empty")
	}
}

func TestDiskCacheCorruptAndForeignEntries(t *testing.T) {
	dir := t.TempDir()
	c := newDiskCache(dir, 0)
	key := cacheKey("s", "http://x/a")
	os.WriteFile(filepath.Join(dir, key+".json"), []byte("{broken"), 0o600)
	if c.get(key, "http://x/a") != nil {
		t.Error("a corrupt file is a miss")
	}
	c.put(key, &cacheEntry{URL: "http://x/other", Status: 200, Fetched: time.Now()})
	if c.get(key, "http://x/a") != nil {
		t.Error("an entry of another URL is a miss")
	}
	c.put(key, &cacheEntry{URL: "http://x/a", Status: 200, Fetched: time.Now(), Body: make([]byte, maxCachedBodyBytes+1)})
	if e := c.get(key, "http://x/a"); e != nil && len(e.Body) > maxCachedBodyBytes {
		t.Error("a huge answer must not be cached")
	}
	var nilCache *diskCache
	nilCache.put("k", &cacheEntry{})
	if nilCache.get("k", "u") != nil {
		t.Error("a nil cache has nothing")
	}
}

func TestDiskCacheUnwritableDirectoryIsIgnored(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0o600)
	c := newDiskCache(filepath.Join(file, "sub"), 0) // A directory cannot be created below a file
	c.put("k", &cacheEntry{URL: "u", Status: 200, Fetched: time.Now()})
	if c.get("k", "u") != nil {
		t.Error("nothing should have been stored")
	}
}

// graphqlBranches makes the fake answer branch scans: statuses has one list of compare statuses per page.
func graphqlBranches(f *fakeGitHub, pages [][]string, calls *atomic.Int32) {
	f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			Variables struct {
				After *string `json:"after"`
				SHA   string  `json:"sha"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Variables.SHA == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		page := 0
		if a := req.Variables.After; a != nil {
			page, _ = strconv.Atoi(*a)
		}
		var nodes []string
		for _, s := range pages[page] {
			nodes = append(nodes, fmt.Sprintf(`{"name":"b","compare":{"status":%q}}`, s))
		}
		fmt.Fprintf(w, `{"data":{"repository":{"refs":{"pageInfo":{"hasNextPage":%v,"endCursor":"%d"},"nodes":[%s]}}}}`, page+1 < len(pages), page+1, strings.Join(nodes, ","))
	})
}

func TestHTTPClientCommitOnAnyBranch(t *testing.T) {
	sha := strings.Repeat("a", 40)
	ctx := context.Background()

	t.Run("not available without a token", func(t *testing.T) {
		f := newFakeGitHub(t)
		_, err := f.client(httpGitHubOptions{}).CommitOnAnyBranch(ctx, "o", "r", sha, 100)
		if !errors.Is(err, ErrGitHubBranchScanUnavailable) {
			t.Errorf("got %v", err)
		}
		if f.total() != 0 {
			t.Error("no request without a token")
		}
	})

	t.Run("found on the second page", func(t *testing.T) {
		f := newFakeGitHub(t)
		var n atomic.Int32
		graphqlBranches(f, [][]string{{"DIVERGED", "AHEAD"}, {"DIVERGED", "BEHIND"}, {"BEHIND"}}, &n)
		c := f.client(httpGitHubOptions{Token: "tok", TTL: time.Hour})
		scan, err := c.CommitOnAnyBranch(ctx, "o", "r", sha, 100)
		if err != nil || !scan.Found || n.Load() != 2 {
			t.Errorf("scan = %+v, %v after %d requests", scan, err, n.Load())
		}
		// Asked again within the TTL
		if scan, err = c.CommitOnAnyBranch(ctx, "o", "r", sha, 100); err != nil || !scan.Found || n.Load() != 2 {
			t.Errorf("the answer should come from the cache: %+v, %v, %d requests", scan, err, n.Load())
		}
		if f.auth[0] != "Bearer tok" {
			t.Errorf("the token must be sent: %q", f.auth)
		}
	})

	t.Run("on no branch", func(t *testing.T) {
		f := newFakeGitHub(t)
		var n atomic.Int32
		graphqlBranches(f, [][]string{{"DIVERGED", "AHEAD"}, {"DIVERGED"}}, &n)
		scan, err := f.client(httpGitHubOptions{Token: "tok"}).CommitOnAnyBranch(ctx, "o", "r", sha, 100)
		if err != nil || scan.Found || !scan.Complete || n.Load() != 2 {
			t.Errorf("scan = %+v, %v after %d requests", scan, err, n.Load())
		}
	})

	t.Run("limit", func(t *testing.T) {
		f := newFakeGitHub(t)
		var n atomic.Int32
		graphqlBranches(f, [][]string{{"DIVERGED", "AHEAD", "DIVERGED"}, {"BEHIND"}}, &n)
		scan, err := f.client(httpGitHubOptions{Token: "tok"}).CommitOnAnyBranch(ctx, "o", "r", sha, 2)
		if err != nil || scan.Found || scan.Complete {
			t.Errorf("branches beyond the limit are not looked at: %+v, %v", scan, err)
		}
		if n.Load() != 1 {
			t.Errorf("%d requests", n.Load())
		}
	})

	t.Run("errors", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"data":{"repository":null},"errors":[{"message":"Could not resolve to a Repository"}]}`)
		})
		_, err := f.client(httpGitHubOptions{Token: "tok"}).CommitOnAnyBranch(ctx, "o", "r", sha, 10)
		if !errors.Is(err, ErrGitHubBranchScanUnavailable) || !strings.Contains(err.Error(), "Could not resolve") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("partial data with errors", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"data":{"repository":{"refs":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"name":"b","compare":null}]}}},"errors":[{"message":"Field error"}]}`)
		})
		_, err := f.client(httpGitHubOptions{Token: "tok"}).CommitOnAnyBranch(ctx, "o", "r", sha, 10)
		// A scan with holes cannot give a verdict, and it must not be counted as a failure of the session
		if !errors.Is(err, ErrGitHubBranchScanUnavailable) {
			t.Errorf("got %v", err)
		}
		s := newOnlineSession(ctx, scanErrClient{err}, nil)
		for range 5 {
			s.record(err)
		}
		if s.stopped() != nil {
			t.Error("scans with errors must not stop the session")
		}
	})

	t.Run("rejected token and rate limit", func(t *testing.T) {
		f := newFakeGitHub(t)
		f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
		var notices []string
		c := f.client(httpGitHubOptions{Token: "bad", Notify: func(m string) { notices = append(notices, m) }})
		if _, err := c.CommitOnAnyBranch(ctx, "o", "r", sha, 10); !errors.Is(err, ErrGitHubBranchScanUnavailable) {
			t.Errorf("got %v", err)
		}
		if len(notices) != 1 || !strings.Contains(notices[0], "rejected") {
			t.Errorf("notices: %q", notices)
		}

		f = newFakeGitHub(t)
		f.handle("/graphql", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"secondary rate limit"}`)
		})
		c = f.client(httpGitHubOptions{Token: "tok"})
		if _, err := c.CommitOnAnyBranch(ctx, "o", "r", sha, 10); !errors.Is(err, ErrGitHubRateLimited) {
			t.Errorf("got %v", err)
		}
		if _, err := c.CommitOnAnyBranch(ctx, "o", "r", strings.Repeat("b", 40), 10); !errors.Is(err, ErrGitHubRateLimited) || f.count("/graphql") != 1 {
			t.Errorf("no second request after the limit: %v, %d", err, f.count("/graphql"))
		}
	})

	t.Run("unsafe names and short SHAs are not sent", func(t *testing.T) {
		f := newFakeGitHub(t)
		c := f.client(httpGitHubOptions{Token: "tok"})
		for _, tc := range [][3]string{{"o\"x", "r", sha}, {"o", "r{", sha}, {"o", "r", "abc"}} {
			if _, err := c.CommitOnAnyBranch(ctx, tc[0], tc[1], tc[2], 10); !errors.Is(err, ErrGitHubBranchScanUnavailable) {
				t.Errorf("%v: %v", tc, err)
			}
		}
		if f.total() != 0 {
			t.Error("nothing should be sent")
		}
	})
}

func TestGraphQLURL(t *testing.T) {
	for base, want := range map[string]string{
		"https://api.github.com":          "https://api.github.com/graphql",
		"https://ghe.example.com/api/v3":  "https://ghe.example.com/api/graphql",
		"https://ghe.example.com/api/v3/": "https://ghe.example.com/api/graphql",
	} {
		c, err := newHTTPGitHubClient(httpGitHubOptions{BaseURL: base})
		if err != nil {
			t.Fatal(err)
		}
		if got := c.graphqlURL(); got != want {
			t.Errorf("%s: %s. want %s", base, got, want)
		}
	}
	c, _ := newHTTPGitHubClient(httpGitHubOptions{GraphQLURL: "https://x/graphql"})
	if c.graphqlURL() != "https://x/graphql" {
		t.Error("the explicit URL wins")
	}
}

// scanningClient is a GitHubClient with a branch scanner on top of the fixtures.
type scanningClient struct {
	*FixtureGitHubClient
	scan GitHubBranchScan
	err  error
	n    atomic.Int32
}

func (c *scanningClient) CommitOnAnyBranch(context.Context, string, string, string, int) (GitHubBranchScan, error) {
	c.n.Add(1)
	return c.scan, c.err
}

func TestImpostorCommitUsesTheBranchScanner(t *testing.T) {
	src := workflowWith("uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529")
	cfg, _ := ParseConfig([]byte("rules:\n  stale-action-refs: off\n"))
	tests := []struct {
		name string
		scan GitHubBranchScan
		err  error
		want []string
	}{
		{"on a branch", GitHubBranchScan{Found: true}, nil, nil},
		{"on no branch", GitHubBranchScan{Complete: true}, nil, []string{"6:impostor-commit"}},
		{"too many branches", GitHubBranchScan{}, nil, nil},
		// The fixtures say the commit is on none of the two branches, so comparing one by one finds an impostor
		{"unavailable", GitHubBranchScan{}, ErrGitHubBranchScanUnavailable, []string{"6:impostor-commit"}},
		{"failed", GitHubBranchScan{}, &GitHubStatusError{Status: 500}, []string{"6:impostor-commit"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &scanningClient{FixtureGitHubClient: onlineFixtureClient(t), scan: tc.scan, err: tc.err}
			errs, _ := lintOnline(t, c, cfg, src)
			if got := lineIDsOf(errs); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v. want %v", got, tc.want)
			}
			if c.n.Load() != 1 {
				t.Errorf("the scanner was asked %d times", c.n.Load())
			}
		})
	}
}

type scanErrClient struct{ error }

func (scanErrClient) Repository(context.Context, string, string) (*GitHubRepo, error) {
	return nil, nil
}
func (scanErrClient) Tags(context.Context, string, string) (*GitHubTagList, error) { return nil, nil }
func (scanErrClient) ResolveRef(context.Context, string, string, GitHubRefNamespace, string) (string, bool, error) {
	return "", false, nil
}
func (scanErrClient) Branches(context.Context, string, string, int) (*GitHubBranchList, error) {
	return nil, nil
}
func (scanErrClient) Compare(context.Context, string, string, string, string) (GitHubCompareStatus, error) {
	return "", nil
}
func (scanErrClient) Advisories(context.Context, string, string) ([]GitHubAdvisory, error) {
	return nil, nil
}
