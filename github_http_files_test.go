//go:build !js

package jactionlint

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// contentsJSON is the answer of the contents API for a file. The API wraps the base64 at 60 columns.
func contentsJSON(content string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(content))
	var lines []string
	for len(enc) > 60 {
		lines = append(lines, enc[:60])
		enc = enc[60:]
	}
	lines = append(lines, enc)
	return fmt.Sprintf(`{"type":"file","encoding":"base64","size":%d,"content":%q}`, len(content), strings.Join(lines, "\n")+"\n")
}

func TestHTTPClientFileContent(t *testing.T) {
	const action = "name: x\ndescription: y\ninputs:\n  a:\n    description: the a input\n    required: true\nruns:\n  using: composite\n  steps: []\n"
	f := newFakeGitHub(t)
	f.json("/repos/o/r/contents/action.yml", contentsJSON(action))
	f.json("/repos/o/r/contents/sub/dir/action.yml", contentsJSON(action))
	f.json("/repos/o/r/contents/dir", `[{"type":"file","name":"action.yml"}]`)
	f.json("/repos/o/r/contents/link", `{"type":"symlink","target":"x"}`)
	f.json("/repos/o/r/contents/huge", `{"type":"file","encoding":"none","size":2000000,"content":""}`)
	var gotRef string
	f.handle("/repos/o/r/contents/ref.yml", func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.URL.Query().Get("ref")
		fmt.Fprint(w, contentsJSON("x: 1\n"))
	})
	c := f.client(httpGitHubOptions{TTL: time.Hour})
	ctx := context.Background()

	got, err := c.FileContent(ctx, "o", "r", "v1", "action.yml")
	if err != nil || string(got) != action {
		t.Fatalf("FileContent() = %q, %v", got, err)
	}
	if got, err := c.FileContent(ctx, "o", "r", "main", "sub/dir/action.yml"); err != nil || string(got) != action {
		t.Fatalf("FileContent() of a path = %q, %v", got, err)
	}
	if _, err := c.FileContent(ctx, "o", "r", "feature/x y"[:9], "ref.yml"); err != nil || gotRef != "feature/x" {
		t.Errorf("the ref goes in the query: %q, %v", gotRef, err)
	}
	// Nothing which is not a regular file with its content is a file
	for _, p := range []string{"missing.yml", "dir", "link"} {
		if _, err := c.FileContent(ctx, "o", "r", "v1", p); !errors.Is(err, ErrGitHubNotFound) {
			t.Errorf("%s: want ErrGitHubNotFound, got %v", p, err)
		}
	}
	if _, err := c.FileContent(ctx, "o", "r", "v1", "huge"); err == nil || errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("a file which is too large is an error, not a missing file: %v", err)
	}
}

func TestHTTPClientFileContentRefusesInvalidNames(t *testing.T) {
	f := newFakeGitHub(t)
	c := f.client(httpGitHubOptions{})
	ctx := context.Background()
	for _, p := range []string{"", "../x", "a/../x", "./x", "a//b", "/abs", "a b", "a\\b", "a%2fb", "a%2e%2e", "a?b", "a#b", "a\x00b", strings.Repeat("a/", 200)} {
		if _, err := c.FileContent(ctx, "o", "r", "v1", p); err == nil {
			t.Errorf("path %q should be refused", p)
		}
	}
	for _, ref := range []string{"", "..", "a..b", "a b", "a%2fb", "a\x00b", "x/"} {
		if _, err := c.FileContent(ctx, "o", "r", ref, "action.yml"); err == nil {
			t.Errorf("ref %q should be refused", ref)
		}
	}
	for _, o := range [][2]string{{"..", "r"}, {"o", ".."}, {"o/x", "r"}} {
		if _, err := c.FileContent(ctx, o[0], o[1], "v1", "action.yml"); err == nil {
			t.Errorf("repository %v should be refused", o)
		}
	}
	if n := f.total(); n != 0 {
		t.Errorf("%d requests were sent for invalid names", n)
	}
}

// A file at a full commit SHA cannot change, so it is used from the cache whatever its age. A file at a tag or a
// branch follows the TTL like every other answer, and a TTL of zero asks again for both.
func TestHTTPClientFileAtACommitIsKeptForEver(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	f := newFakeGitHub(t)
	f.json("/repos/o/r/contents/action.yml", contentsJSON("name: x\n"))
	dir := t.TempDir()
	ctx := context.Background()
	t0 := time.Now()
	later := func(opts httpGitHubOptions, d time.Duration) *httpGitHubClient {
		opts.CacheDir = dir
		c := f.client(opts)
		c.now = func() time.Time { return t0.Add(d) }
		return c
	}

	first := later(httpGitHubOptions{TTL: time.Hour}, 0)
	for _, ref := range []string{sha, "v1"} {
		if _, err := first.FileContent(ctx, "o", "r", ref, "action.yml"); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("/repos/o/r/contents/action.yml"); n != 2 {
		t.Fatalf("want 2 requests, got %d", n)
	}

	// A year later: the commit is served from the cache and counted apart, the tag is revalidated (a 304)
	next := later(httpGitHubOptions{TTL: time.Hour}, 24*365*time.Hour)
	if _, err := next.FileContent(ctx, "o", "r", sha, "action.yml"); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/repos/o/r/contents/action.yml"); n != 2 {
		t.Errorf("a file at a full SHA needs no request: %d requests", n)
	}
	if a := next.answerStats(); a.Immutable != 1 || a.Cache != 0 || a.Network != 0 || a.OldestFetched != "" {
		t.Errorf("answers = %+v", a)
	}
	if _, err := next.FileContent(ctx, "o", "r", "v1", "action.yml"); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/repos/o/r/contents/action.yml"); n != 3 {
		t.Errorf("a file at a tag follows the TTL: %d requests", n)
	}
	if a := next.answerStats(); a.Immutable != 1 || a.Network != 1 {
		t.Errorf("answers = %+v", a)
	}

	// Within the TTL it is an ordinary cache answer
	soon := later(httpGitHubOptions{TTL: time.Hour}, time.Minute)
	if _, err := soon.FileContent(ctx, "o", "r", sha, "action.yml"); err != nil {
		t.Fatal(err)
	}
	if a := soon.answerStats(); a.Cache != 1 || a.Immutable != 0 {
		t.Errorf("answers = %+v", a)
	}

	// A TTL of zero asks for every answer to be checked, immutable or not
	zero := later(httpGitHubOptions{TTL: 0}, time.Minute)
	before := f.count("/repos/o/r/contents/action.yml")
	if _, err := zero.FileContent(ctx, "o", "r", sha, "action.yml"); err != nil {
		t.Fatal(err)
	}
	if n := f.count("/repos/o/r/contents/action.yml"); n != before+1 {
		t.Errorf("--online-cache-ttl=0 revalidates: %d requests, had %d", n, before)
	}

	// And the offline mode serves it, as it serves everything
	off := later(httpGitHubOptions{TTL: time.Hour, Offline: true}, 24*365*time.Hour)
	if got, err := off.FileContent(ctx, "o", "r", sha, "action.yml"); err != nil || string(got) != "name: x\n" {
		t.Errorf("offline: %q, %v", got, err)
	}
	if _, err := off.FileContent(ctx, "o", "r", "v2", "action.yml"); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("offline, not cached: %v", err)
	}
}

// A missing file is remembered for the TTL like any other 404, also for a commit: whether a token may see the
// repository can change.
func TestHTTPClientMissingFileFollowsTheTTL(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	f := newFakeGitHub(t)
	dir := t.TempDir()
	ctx := context.Background()
	c := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	for i := 0; i < 3; i++ {
		if _, err := c.FileContent(ctx, "o", "r", sha, "action.yml"); !errors.Is(err, ErrGitHubNotFound) {
			t.Fatal(err)
		}
	}
	if n := f.count("/repos/o/r/contents/action.yml"); n != 1 {
		t.Errorf("a missing file is asked for once within the TTL: %d requests", n)
	}
	later := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour})
	later.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := later.FileContent(ctx, "o", "r", sha, "action.yml"); !errors.Is(err, ErrGitHubNotFound) {
		t.Fatal(err)
	}
	if n := f.count("/repos/o/r/contents/action.yml"); n != 2 {
		t.Errorf("a missing file is asked for again after the TTL: %d requests", n)
	}
}

// The contents of an action are cached for the token that fetched them, like every answer.
func TestHTTPClientFileContentIsPerToken(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	f := newFakeGitHub(t)
	f.json("/repos/o/r", `{"private":true,"visibility":"private","default_branch":"main"}`)
	f.json("/repos/o/r/contents/action.yml", contentsJSON("name: private\n"))
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "a"}).FileContent(ctx, "o", "r", sha, "action.yml"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client(httpGitHubOptions{CacheDir: dir, TTL: time.Hour, Token: "b", Offline: true}).FileContent(ctx, "o", "r", sha, "action.yml"); !errors.Is(err, ErrGitHubNotCached) {
		t.Errorf("the file of a private repository must not reach another token: %v", err)
	}
}
