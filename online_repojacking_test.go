//go:build !js

package jactionlint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repojackingConfig leaves the other online rules off, so that the requests of the tests are the ones of the rule.
const repojackingConfig = "rules:\n  impostor-commit: off\n  known-vulnerable-actions: off\n  ref-confusion: off\n  stale-action-refs: off\n  archived-uses: off\n  ref-version-mismatch: off\n  unpinned-uses: off\n"

// movedRepository makes the fake answer like GitHub does for a repository that was renamed or transferred: 301 to
// /repositories/{id}.
func movedRepository(f *fakeGitHub, from, to string) {
	f.handle("/repos/"+from, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/repositories/42", http.StatusMovedPermanently)
	})
	f.json("/repositories/42", fmt.Sprintf(`{"id":42,"full_name":%q,"private":false,"archived":false,"default_branch":"main"}`, to))
}

// ownerLookupHost makes the fake the host whose answers about owners count, as api.github.com does.
func ownerLookupHost(t *testing.T, f *fakeGitHub) {
	t.Helper()
	old := ownerLookupAPIHost
	ownerLookupAPIHost = strings.TrimPrefix(f.srv.URL, "http://")
	t.Cleanup(func() { ownerLookupAPIHost = old })
}

func ownerAnswer(f *fakeGitHub, login string, exists bool) {
	if exists {
		f.json("/users/"+login, fmt.Sprintf(`{"login":%q,"type":"User"}`, login))
	}
}

func TestRepojackingRequests(t *testing.T) {
	for _, tc := range []struct {
		name     string
		uses     string
		answer   func(f *fakeGitHub)
		finding  string // part of the message, or empty for none
		skipped  float64
		requests map[string]int
	}{
		{
			name:     "owner does not exist",
			uses:     "uses: gone/act@v1",
			answer:   func(f *fakeGitHub) {},
			finding:  `has the owner "gone", which does not exist`,
			skipped:  1, // the repository is not found, which every online rule counts
			requests: map[string]int{"/repos/gone/act": 1, "/users/gone": 1},
		},
		{
			name:     "repository not visible, owner exists",
			uses:     "uses: acme/private@v1",
			answer:   func(f *fakeGitHub) { ownerAnswer(f, "acme", true) },
			skipped:  1,
			requests: map[string]int{"/repos/acme/private": 1, "/users/acme": 1},
		},
		{
			name: "moved to another owner, old owner gone",
			uses: "uses: old/act@v1",
			answer: func(f *fakeGitHub) {
				movedRepository(f, "old/act", "new/act")
			},
			finding:  `has moved to new/act and the owner "old" no longer exists`,
			requests: map[string]int{"/repos/old/act": 1, "/repositories/42": 1, "/users/old": 1},
		},
		{
			name: "moved to another owner, old owner exists",
			uses: "uses: old/act@v1",
			answer: func(f *fakeGitHub) {
				movedRepository(f, "old/act", "new/act")
				ownerAnswer(f, "old", true)
			},
			requests: map[string]int{"/repos/old/act": 1, "/repositories/42": 1, "/users/old": 1},
		},
		{
			name: "renamed in the account",
			uses: "uses: same/old@v1",
			answer: func(f *fakeGitHub) {
				movedRepository(f, "same/old", "Same/new")
			},
			requests: map[string]int{"/repos/same/old": 1, "/repositories/42": 1},
		},
		{
			name: "the case of the owner",
			uses: "uses: Actions/Checkout@v4",
			answer: func(f *fakeGitHub) {
				f.json("/repos/Actions/Checkout", `{"full_name":"actions/checkout","private":false,"default_branch":"main"}`)
			},
			requests: map[string]int{"/repos/Actions/Checkout": 1},
		},
		{
			// The owner lookup fails: nothing is known
			name: "owner lookup forbidden",
			uses: "uses: gone/act@v1",
			answer: func(f *fakeGitHub) {
				f.handle("/users/gone", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"blocked"}`)
				})
			},
			skipped:  2,
			requests: map[string]int{"/repos/gone/act": 1, "/users/gone": 2}, // the second one is without the token
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			// The action.yml of the action is readable, so that the test counts only what it is about
			if ref := ParseUses(strings.TrimPrefix(tc.uses, "uses: ")); ref.Kind == UsesAction {
				f.json("/repos/"+ref.Owner+"/"+ref.Repo+"/contents/action.yml", contentsJSON("name: x\ndescription: d\nruns:\n  using: node20\n  main: i.js\n"))
			}
			tc.answer(f)
			setOnlineEnv(t, f, "tok")
			ownerLookupHost(t, f)
			run := func() (string, float64) {
				_, wf := onlineProject(t, workflowWith(tc.uses), repojackingConfig)
				stdout, skipped := runWithSkipped(t, wf)
				return stdout, skipped
			}
			stdout, skipped := run()
			if tc.finding == "" && strings.Contains(stdout, "[repojacking]") || tc.finding != "" && !strings.Contains(stdout, tc.finding) {
				t.Errorf("want %q in\n%s", tc.finding, stdout)
			}
			if skipped != tc.skipped {
				t.Errorf("skipped lookups: %v, want %v", skipped, tc.skipped)
			}
			for path, n := range tc.requests {
				if got := f.count(path); got != n {
					t.Errorf("%d requests for %s, want %d", got, path, n)
				}
			}
			// A repository can be asked for by the other rules too: the requests of the token for its visibility are not the rule's
			for path := range f.hits {
				if _, ok := tc.requests[path]; !ok && !strings.HasPrefix(path, "/repos/") {
					t.Errorf("unexpected request for %s", path)
				}
			}

			// The second run is answered by the cache, except for what failed
			before := f.total()
			run()
			if f.total() != before && !strings.Contains(tc.name, "forbidden") {
				t.Errorf("a second run made %d requests", f.total()-before)
			}
		})
	}
}

// runWithSkipped lints the workflow with the online checks and returns the output and the skipped lookups.
func runWithSkipped(t *testing.T, wf string) (string, float64) {
	t.Helper()
	report := filepath.Join(t.TempDir(), "report.json")
	_, stdout, stderr := runOnlineCommand(t, "--online", "--online-report", report, wf)
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	var rep map[string]any
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	return stdout, rep["skippedLookups"].(float64)
}

// The same owner of several actions is asked for once.
func TestRepojackingAsksOnceForAnOwner(t *testing.T) {
	f := newFakeGitHub(t)
	setOnlineEnv(t, f, "tok")
	ownerLookupHost(t, f)
	_, wf := onlineProject(t, workflowWith("uses: gone/a@v1", "uses: gone/b@v1", "uses: gone/a@v2"), repojackingConfig)
	_, stdout, _ := runOnlineCommand(t, "--online", wf)
	if n := strings.Count(stdout, "[repojacking]"); n != 3 {
		t.Errorf("%d findings:\n%s", n, stdout)
	}
	if n := f.count("/users/gone"); n != 1 {
		t.Errorf("%d requests for the owner", n)
	}
	if f.count("/repos/gone/a") != 1 || f.count("/repos/gone/b") != 1 {
		t.Errorf("%v", f.hits)
	}
}

func TestHTTPClientOwner(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/users/octo-org", `{"login":"Octo-Org","type":"Organization"}`)
	ownerLookupHost(t, f)
	c := f.client(httpGitHubOptions{})
	ctx := context.Background()
	o, err := c.Owner(ctx, "octo-org")
	if err != nil || o.Login != "Octo-Org" || o.Type != "Organization" {
		t.Errorf("%+v, %v", o, err)
	}
	if _, err := c.Owner(ctx, "nobody"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("missing: %v", err)
	}
	n := f.total()
	for _, name := range []string{"", "a/b", "..", "-x", "a b", "x-", "a%2Fb"} {
		if _, err := c.Owner(ctx, name); !errors.Is(err, errInvalidGitHubName) {
			t.Errorf("%q: %v", name, err)
		}
	}
	if f.total() != n {
		t.Errorf("%d requests for invalid names", f.total()-n)
	}
}

// The client follows the redirect of a renamed repository and says what its name is now.
func TestHTTPClientRepositoryFullName(t *testing.T) {
	f := newFakeGitHub(t)
	movedRepository(f, "old/act", "new/act")
	c := f.client(httpGitHubOptions{Token: "tok"})
	r, err := c.Repository(context.Background(), "old", "act")
	if err != nil || r.FullName != "new/act" {
		t.Errorf("%+v, %v", r, err)
	}
	for _, a := range f.auth {
		if a != "Bearer tok" {
			t.Errorf("the token must go with the redirect to the same host: %q", a)
		}
	}
}

// A client which cannot look up owners leaves everything as it is: no finding, and nothing counted for the owner.
func TestRepojackingWithoutOwnerLookup(t *testing.T) {
	sess := newOnlineSession(context.Background(), &failingClient{err: fmt.Errorf("x: %w", ErrGitHubNotFound)}, nil)
	if _, err := sess.Owner("gone"); !errors.Is(err, errNoOwnerLookup) {
		t.Errorf("%v", err)
	}
	if _, err := newOnlineSession(context.Background(), onlineFixtureClient(t), nil).Owner("a/b"); !errors.Is(err, errInvalidOwnerName) {
		t.Errorf("%v", err)
	}
	cfg, err := ParseConfig([]byte(repojackingConfig))
	if err != nil {
		t.Fatal(err)
	}
	errs, l := lintOnline(t, &failingClient{err: fmt.Errorf("x: %w", ErrGitHubNotFound)}, cfg, workflowWith("uses: gone/a@v1"))
	for _, e := range errs {
		if e.ID == "repojacking" {
			t.Errorf("unexpected finding: %s", e.Message)
		}
	}
	if n := l.OnlineSkipped(); n != 1 { // the repository, which is missing
		t.Errorf("%d skipped lookups", n)
	}
}

// The fixtures record and replay the answers about owners and the name a repository has now.
func TestOwnerFixtureRoundTrip(t *testing.T) {
	f := newFakeGitHub(t)
	movedRepository(f, "old/act", "new/act")
	ownerAnswer(f, "new", true)
	ownerLookupHost(t, f)
	rec := NewRecordingGitHubClient(f.client(httpGitHubOptions{}))
	ctx := context.Background()
	if _, err := rec.Repository(ctx, "old", "act"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Owner(ctx, "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Owner(ctx, "Old"); !errors.Is(err, ErrGitHubNotFound) {
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
	if r, err := fx.Repository(ctx, "old", "act"); err != nil || r.FullName != "new/act" {
		t.Errorf("%+v, %v", r, err)
	}
	if _, err := fx.Owner(ctx, "NEW"); err != nil {
		t.Error(err)
	}
	if _, err := fx.Owner(ctx, "old"); !errors.Is(err, ErrGitHubNotFound) {
		t.Error(err)
	}
	if _, err := fx.Owner(ctx, "unknown"); err == nil || errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("a lookup which is not recorded must fail: %v", err)
	}
}

// An Enterprise Server runs the actions of github.com through GitHub Connect: its API has no such owner, and that is no
// finding. Only api.github.com answers.
func TestRepojackingIsForGitHubComOnly(t *testing.T) {
	f := newFakeGitHub(t) // the Server: it answers 404 to everything
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: former/deploy@v1"), repojackingConfig)
	movedRepository(f, "former/deploy", "new/deploy")
	status, stdout, stderr := runOnlineCommand(t, "--online", wf)
	_ = status
	if strings.Contains(stdout, "[repojacking]") {
		t.Errorf("a Server cannot tell:\n%s%s", stdout, stderr)
	}
	if f.count("/users/actions") != 0 || f.count("/users/former") != 0 {
		t.Errorf("owners were asked for on a Server: %v", f.hits)
	}
}
