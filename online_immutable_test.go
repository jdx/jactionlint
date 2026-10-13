//go:build !js

package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// immutableConfig turns the other online rules off, so that the requests of the tests are the ones for the releases.
const immutableConfig = "rules:\n  impostor-commit: off\n  known-vulnerable-actions: off\n  ref-confusion: off\n  stale-action-refs: off\n  archived-uses: off\n  ref-version-mismatch: off\n  repojacking: off\n  unpinned-uses:\n    level: error\n    policies:\n      o/*: immutable-release\n"

func releaseJSON(tag string, immutable bool) string {
	return fmt.Sprintf(`{"id":1,"tag_name":%q,"immutable":%v,"draft":false}`, tag, immutable)
}

func TestImmutableReleaseRequests(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/releases/tags/v1.2.3", releaseJSON("v1.2.3", true))
	f.json("/repos/o/r/releases/tags/v1.2.2", releaseJSON("v1.2.2", false))
	// v2.0.0 is an immutable release, but a branch has the name too
	f.json("/repos/o/r/releases/tags/v2.0.0", releaseJSON("v2.0.0", true))
	f.json("/repos/o/r/git/ref/heads/v2.0.0", `{"object":{"type":"commit","sha":"`+strings.Repeat("a", 40)+`"}}`)
	// A release which the API refuses to show
	f.handle("/repos/o/r/releases/tags/v3.0.0", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"no"}`)
	})
	setOnlineEnv(t, f, "tok")
	steps := []string{
		"uses: o/r@v1.2.3", // accepted
		"uses: o/r@v1.2.2", // the release is not immutable
		"uses: o/r@v1",     // no release: a moving tag
		"uses: o/r@v2.0.0", // a branch has the name
		"uses: o/r@v3.0.0", // the lookup fails
		"uses: o/r@" + strings.Repeat("b", 40),
		"uses: other/r@v1.2.3", // the policy is hash-pin, no lookup
		"uses: o/r@v1.2.3",     // the same again: no new request
	}
	_, wf := onlineProject(t, workflowWith(steps...), immutableConfig)
	_, stdout, stderr := runOnlineCommand(t, "--online", wf)
	_ = stderr
	reported := map[string]bool{}
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "[unpinned-uses]") {
			for _, s := range []string{"o/r@v1.2.3", "o/r@v1.2.2", "o/r@v1\"", "o/r@v2.0.0", "o/r@v3.0.0", "other/r@v1.2.3"} {
				if strings.Contains(l, `"`+s) {
					reported[strings.TrimSuffix(s, `"`)] = true
				}
			}
		}
	}
	want := map[string]bool{"o/r@v1.2.2": true, "o/r@v1": true, "o/r@v2.0.0": true, "o/r@v3.0.0": true, "other/r@v1.2.3": true}
	for k := range want {
		if !reported[k] {
			t.Errorf("%s must be reported:\n%s", k, stdout)
		}
	}
	if reported["o/r@v1.2.3"] {
		t.Errorf("the tag of an immutable release must be accepted:\n%s", stdout)
	}
	// One request for each tag which the policy leaves open, one for the branch of the immutable ones. The 403 of v3.0.0 is
	// asked again without the token by the client, which is not cached.
	for path, n := range map[string]int{
		"/repos/o/r/releases/tags/v1.2.3": 1, "/repos/o/r/releases/tags/v1.2.2": 1, "/repos/o/r/releases/tags/v1": 1,
		"/repos/o/r/releases/tags/v2.0.0": 1, "/repos/o/r/git/ref/heads/v1.2.3": 1, "/repos/o/r/git/ref/heads/v2.0.0": 1,
		"/repos/other/r/releases/tags/v1.2.3": 0,
	} {
		if got := f.count(path); got != n {
			t.Errorf("%d requests for %s, want %d", got, path, n)
		}
	}
	if got := f.count("/repos/o/r/releases/tags/v3.0.0"); got < 1 {
		t.Errorf("v3.0.0 was not asked for")
	}
	// A commit is never looked up
	for path := range f.hits {
		if strings.Contains(path, strings.Repeat("b", 40)) {
			t.Errorf("a request for a commit: %s", path)
		}
	}

	// The second run is answered by the cache, except for the failure
	before := f.total() - f.count("/repos/o/r/releases/tags/v3.0.0")
	runOnlineCommand(t, "--online", wf)
	if after := f.total() - f.count("/repos/o/r/releases/tags/v3.0.0"); after != before {
		t.Errorf("a second run made %d requests", after-before)
	}
}

// Without --online the policy is hash-pin, and nothing is asked.
func TestImmutableReleasePolicyOfflineIsHashPin(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/releases/tags/v1.2.3", releaseJSON("v1.2.3", true))
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: o/r@v1.2.3"), immutableConfig)
	_, stdout, _ := runOnlineCommand(t, wf)
	if !strings.Contains(stdout, "[unpinned-uses]") || !strings.Contains(stdout, "immutable release") || f.total() != 0 {
		t.Errorf("%d requests:\n%s", f.total(), stdout)
	}
}

// A failed lookup keeps the finding and is counted as skipped.
func TestImmutableReleaseFailedLookupIsSkipped(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r/releases/tags/v1.2.3", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"no"}`)
	})
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: o/r@v1.2.3"), immutableConfig)
	rep, _, _ := runWithReport(t, wf, "--online")
	// The lookup of the release, and the one of the tags which the pin fix needs for the finding that is left
	if by, _ := rep["skippedBy"].(map[string]any); by["forbidden"] != float64(1) {
		t.Errorf("%v", rep)
	}
	_, stdout, _ := runOnlineCommand(t, "--online", wf)
	if !strings.Contains(stdout, "[unpinned-uses]") {
		t.Errorf("a lookup which failed must not hide the finding:\n%s", stdout)
	}
}

// Other policies and other values are not affected.
func TestImmutableReleaseOnlyForItsPolicy(t *testing.T) {
	f := newFakeGitHub(t)
	setOnlineEnv(t, f, "tok")
	cfg := "rules:\n  unpinned-uses:\n    level: error\n    policies:\n      o/*: ref-pin\n"
	_, wf := onlineProject(t, workflowWith("uses: o/r@v1.2.3", "uses: x/y@v1"), cfg)
	_, stdout, _ := runOnlineCommand(t, "--online", wf)
	if strings.Count(stdout, "[unpinned-uses]") != 1 || !strings.Contains(stdout, "x/y@v1") || f.count("/repos/o/r/releases/tags/v1.2.3") != 0 {
		t.Errorf("%v\n%s", f.hits, stdout)
	}
}

func TestHTTPClientRelease(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/releases/tags/v1.2.3", releaseJSON("v1.2.3", true))
	f.json("/repos/o/r/releases/tags/v1.2.2", `{"tag_name":"v1.2.2"}`) // an answer of a Server which has no such field
	c := f.client(httpGitHubOptions{})
	ctx := context.Background()
	if r, err := c.Release(ctx, "o", "r", "v1.2.3"); err != nil || !r.Immutable || r.Tag != "v1.2.3" {
		t.Errorf("%+v, %v", r, err)
	}
	if r, err := c.Release(ctx, "o", "r", "v1.2.2"); err != nil || r.Immutable {
		t.Errorf("%+v, %v", r, err)
	}
	if _, err := c.Release(ctx, "o", "r", "v9"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("%v", err)
	}
	n := f.total()
	for _, tag := range []string{"", "a/b", "..", "a b", "a%2Fb", "x\n"} {
		if _, err := c.Release(ctx, "o", "r", tag); !errors.Is(err, errInvalidGitHubName) {
			t.Errorf("%q: %v", tag, err)
		}
	}
	if f.total() != n {
		t.Errorf("%d requests for invalid tags", f.total()-n)
	}
}

func TestReleaseFixtureRoundTrip(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r", `{"private":false,"default_branch":"main"}`)
	f.json("/repos/o/r/releases/tags/v1", releaseJSON("v1", true))
	rec := NewRecordingGitHubClient(f.client(httpGitHubOptions{}))
	ctx := context.Background()
	if _, err := rec.Repository(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Release(ctx, "o", "r", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Release(ctx, "o", "r", "v0"); !errors.Is(err, ErrGitHubNotFound) {
		t.Fatal(err)
	}
	b, err := rec.Fixtures()
	if err != nil {
		t.Fatal(err)
	}
	fx, err := NewFixtureGitHubClient(b)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := fx.Release(ctx, "o", "r", "v1"); err != nil || !r.Immutable {
		t.Errorf("%+v, %v", r, err)
	}
	if _, err := fx.Release(ctx, "o", "r", "v0"); !errors.Is(err, ErrGitHubNotFound) {
		t.Error(err)
	}
	if _, err := fx.Release(ctx, "o", "r", "unknown"); err == nil || errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("a lookup which is not recorded must fail: %v", err)
	}
}

// A client which cannot look up releases keeps the finding, and counts nothing.
func TestImmutableReleaseWithoutLookup(t *testing.T) {
	cfg, err := ParseConfig([]byte(immutableConfig))
	if err != nil {
		t.Fatal(err)
	}
	errs, l := lintOnline(t, &failingClient{err: fmt.Errorf("x: %w", ErrGitHubNotFound)}, cfg, workflowWith("uses: o/r@v1.2.3"))
	found := false
	for _, e := range errs {
		found = found || e.ID == "unpinned-uses"
	}
	if !found {
		t.Errorf("%v", lineIDsOf(errs))
	}
	if n := l.OnlineSkipped(); n > 1 { // the tags which the pin fix of the finding asks for
		t.Errorf("%d skipped lookups", n)
	}
}

func TestUsesRefAt(t *testing.T) {
	src := []byte("steps:\n  - uses: o/r@v1\n  - uses: \"o/r@v2\" # c\n  - uses: 'o/r@v3\n  - uses: ${{ x }}\n  - uses: \xc3\xa9 o/r@v4\n")
	starts := lineStarts(src)
	for _, tc := range []struct {
		line, col int
		want      string
	}{{2, 11, "o/r@v1"}, {3, 11, "o/r@v2"}, {4, 11, ""}, {5, 11, ""}} {
		ref := usesRefAt(src, starts[tc.line-1], tc.col)
		got := ""
		if ref != nil {
			got = ref.Raw
		}
		if got != tc.want {
			t.Errorf("line %d: %q, want %q", tc.line, got, tc.want)
		}
	}
	if usesRefAt(src, starts[1], 200) != nil {
		t.Error("a column outside the line")
	}
}
