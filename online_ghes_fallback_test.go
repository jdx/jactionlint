//go:build !js

package jactionlint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// twoHosts makes a fake Enterprise Server and a fake github.com, and points the client at them. The Server has the
// repository corp/internal and, when both is set, actions/checkout too. github.com has actions/checkout and
// example/archived.
type twoHosts struct {
	ghes, dotcom *fakeGitHub
}

const (
	ghesToken   = "ghes-token"
	dotcomToken = "dotcom-token"
)

func newTwoHosts(t *testing.T, both bool) *twoHosts {
	t.Helper()
	h := &twoHosts{ghes: newFakeGitHub(t), dotcom: newFakeGitHub(t)}
	// The Server
	h.ghes.json("/repos/corp/internal", `{"archived":true,"default_branch":"main","private":true}`)
	h.ghes.json("/repos/corp/internal/tags", `[]`)
	if both {
		h.ghes.json("/repos/actions/checkout", `{"archived":false,"default_branch":"main"}`)
		h.ghes.json("/repos/actions/checkout/tags", `[]`)
	}
	h.ghes.json("/advisories", `[]`)
	// github.com
	h.dotcom.json("/repos/actions/checkout", `{"archived":false,"default_branch":"main"}`)
	h.dotcom.json("/repos/actions/checkout/tags", `[]`)
	h.dotcom.json("/repos/example/archived", `{"archived":true,"default_branch":"main"}`)
	h.dotcom.json("/repos/example/archived/tags", `[]`)
	h.dotcom.json("/advisories", `[]`)
	setOnlineEnv(t, h.ghes, ghesToken) // GITHUB_API_URL is the Server
	t.Setenv("GITHUB_COM_TOKEN", dotcomToken)
	old := gitHubComAPIURL
	gitHubComAPIURL = h.dotcom.srv.URL
	t.Cleanup(func() { gitHubComAPIURL = old })
	return h
}

const fallbackConfig = "rules:\n  impostor-commit: off\n  ref-confusion: off\n  ref-version-mismatch: off\n  stale-action-refs: off\n  unpinned-uses: off\n"

func fallbackRun(t *testing.T, args ...string) (stdout, stderr string, report map[string]any) {
	t.Helper()
	_, wf := onlineProject(t, workflowWith("uses: corp/internal@v1", "uses: actions/checkout@v4", "uses: example/archived@v1", "uses: nowhere/none@v1"), fallbackConfig)
	rpath := filepath.Join(t.TempDir(), "report.json")
	_, stdout, stderr = runOnlineCommand(t, append([]string{"--online", "--online-report", rpath}, append(args, wf)...)...)
	b, err := os.ReadFile(rpath)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	return stdout, stderr, report
}

func TestGitHubComFallbackRoutesByRepository(t *testing.T) {
	h := newTwoHosts(t, false)
	stdout, stderr, report := fallbackRun(t, "--online-github-com-fallback")

	// What each host has is looked at, and only there
	for _, want := range []string{`"corp/internal@v1" is in the archived repository corp/internal`, `"example/archived@v1" is in the archived repository example/archived`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("%q is missing in\n%s%s", want, stdout, stderr)
		}
	}
	if n := strings.Count(stdout, "[archived-uses]"); n != 2 {
		t.Errorf("%d archived-uses findings:\n%s", n, stdout)
	}
	// The Server is asked about every repository once, and nothing more for the ones it does not have
	for path, want := range map[string]int{"/repos/corp/internal": 1, "/repos/actions/checkout": 1, "/repos/example/archived": 1, "/repos/nowhere/none": 1} {
		if got := h.ghes.count(path); got != want {
			t.Errorf("Server: %d requests for %s, want %d", got, path, want)
		}
	}
	for _, p := range []string{"/repos/actions/checkout/tags", "/repos/example/archived/tags", "/repos/corp/internal/tags"} {
		if h.ghes.count(p) != 0 && p != "/repos/corp/internal/tags" {
			t.Errorf("Server: a request for %s, which it does not have", p)
		}
	}
	// github.com is asked about the repositories the Server answered 404 for, and for the advisories
	for path, want := range map[string]int{"/repos/actions/checkout": 1, "/repos/example/archived": 1, "/repos/nowhere/none": 1, "/repos/corp/internal": 0} {
		if got := h.dotcom.count(path); got != want {
			t.Errorf("github.com: %d requests for %s, want %d", got, path, want)
		}
	}
	// The repository which neither has is skipped and counted
	if report["skippedLookups"] != float64(1) {
		t.Errorf("skipped lookups: %v", report["skippedLookups"])
	}
	// Both hosts' answers are in the report
	ans, _ := report["answers"].(map[string]any)
	if ans == nil || ans["network"] != float64(h.ghes.total()+h.dotcom.total()) {
		t.Errorf("answers %v, requests %d + %d", ans, h.ghes.total(), h.dotcom.total())
	}

	// A second run is answered by the cache, also what is not found
	before1, before2 := h.ghes.total(), h.dotcom.total()
	fallbackRun(t, "--online-github-com-fallback")
	if h.ghes.total() != before1 || h.dotcom.total() != before2 {
		t.Errorf("a second run made %d and %d requests", h.ghes.total()-before1, h.dotcom.total()-before2)
	}
}

// Every token goes to its own host.
func TestGitHubComFallbackTokensStayWithTheirHost(t *testing.T) {
	h := newTwoHosts(t, false)
	t.Setenv("GH_TOKEN", "gh-token-of-the-server")
	fallbackRun(t, "--online-github-com-fallback")
	for _, a := range h.ghes.auth {
		if a != "Bearer "+ghesToken {
			t.Errorf("the Server got %q", a)
		}
	}
	if len(h.dotcom.auth) == 0 {
		t.Fatal("github.com was not asked")
	}
	for _, a := range h.dotcom.auth {
		if a != "Bearer "+dotcomToken {
			t.Errorf("github.com got %q", a)
		}
	}
}

// Without GITHUB_COM_TOKEN github.com is asked without a token, never with the one of the Server.
func TestGitHubComFallbackNeverSendsTheTokenOfTheServer(t *testing.T) {
	h := newTwoHosts(t, false)
	t.Setenv("GITHUB_COM_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	_, stderr, _ := fallbackRun(t, "--online-github-com-fallback")
	if len(h.dotcom.auth) == 0 {
		t.Fatal("github.com was not asked")
	}
	for _, a := range h.dotcom.auth {
		if a != "" {
			t.Errorf("github.com got %q", a)
		}
	}
	if !strings.Contains(stderr, "GITHUB_COM_TOKEN") {
		t.Errorf("no notice about the missing token for github.com:\n%s", stderr)
	}
}

func TestGitHubComFallbackTokenSources(t *testing.T) {
	t.Run("variable of the option", func(t *testing.T) {
		h := newTwoHosts(t, false)
		t.Setenv("GITHUB_COM_TOKEN", "")
		t.Setenv("MY_DOTCOM", "mine")
		fallbackRun(t, "--online-github-com-fallback", "--online-github-com-token-env=MY_DOTCOM")
		for _, a := range h.dotcom.auth {
			if a != "Bearer mine" {
				t.Errorf("github.com got %q", a)
			}
		}
	})
	t.Run("file of the option", func(t *testing.T) {
		h := newTwoHosts(t, false)
		f := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(f, []byte("from-file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		fallbackRun(t, "--online-github-com-fallback", "--online-github-com-token-file="+f)
		for _, a := range h.dotcom.auth {
			if a != "Bearer from-file" {
				t.Errorf("github.com got %q", a)
			}
		}
	})
}

// Without the option the Server is the only host, as before.
func TestGitHubComFallbackIsOptIn(t *testing.T) {
	h := newTwoHosts(t, false)
	stdout, _, report := fallbackRun(t)
	if h.dotcom.total() != 0 {
		t.Errorf("github.com was asked %d times without the option: %v", h.dotcom.total(), h.dotcom.hits)
	}
	if strings.Count(stdout, "[archived-uses]") != 1 || report["skippedLookups"] != float64(3) {
		t.Errorf("skipped %v:\n%s", report["skippedLookups"], stdout)
	}
}

// The Server wins when both have the repository, and github.com is not asked about it.
func TestGitHubComFallbackServerWins(t *testing.T) {
	h := newTwoHosts(t, true)
	fallbackRun(t, "--online-github-com-fallback")
	if n := h.ghes.count("/repos/actions/checkout"); n != 1 {
		t.Errorf("Server: %d requests", n)
	}
	if n := h.dotcom.count("/repos/actions/checkout") + h.dotcom.count("/repos/actions/checkout/tags"); n != 0 {
		t.Errorf("github.com was asked %d times about a repository of the Server", n)
	}
}

// A failure of the Server is no reason to ask github.com: the repository may well be on the Server.
func TestGitHubComFallbackOnlyForNotFound(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			h := newTwoHosts(t, false)
			h.ghes.handle("/repos/actions/checkout", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"no"}`)
			})
			retries := "--online-max-wait=0"
			fallbackRun(t, "--online-github-com-fallback", retries)
			if n := h.dotcom.count("/repos/actions/checkout"); n != 0 {
				t.Errorf("github.com was asked %d times after a %d of the Server", n, status)
			}
		})
	}
}

// The options of the configuration file are the same as the flags, and the fallback means nothing for github.com.
func TestGitHubComFallbackOptions(t *testing.T) {
	cfg, err := ParseConfig([]byte("online-options:\n  github-com-fallback: true\n  github-com-token-env: X\n  github-com-token-file: /f\n"))
	if err != nil {
		t.Fatal(err)
	}
	o := cfg.OnlineOptions
	if o.GitHubComFallback == nil || !*o.GitHubComFallback || o.GitHubComTokenEnv != "X" || o.GitHubComTokenFile != "/f" {
		t.Errorf("%+v", o)
	}
	off := false
	got := o.overlay(OnlineOptions{GitHubComFallback: &off})
	if *got.GitHubComFallback || got.GitHubComTokenEnv != "X" {
		t.Errorf("%+v", got)
	}

	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_API_URL", "")
	gitHubComAPIURL = f.srv.URL
	t.Cleanup(func() { gitHubComAPIURL = defaultGitHubAPIURL })
	// api.github.com is the primary host: nothing to fall back to
	var notes []string
	c, err := newDefaultGitHubClient(defaultClientOptions{Options: OnlineOptions{GitHubComFallback: ptr(true)}, TTL: 0, Notify: func(s string) { notes = append(notes, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*fallbackGitHubClient); ok || len(notes) == 0 || !strings.Contains(strings.Join(notes, "\n"), "no effect") {
		t.Errorf("%T %v", c, notes)
	}
}

func ptr[T any](v T) *T { return &v }

// The routing, without HTTP: the first host which has the repository answers everything about it.
func TestFallbackClientRouting(t *testing.T) {
	mk := func(data string) *FixtureGitHubClient {
		c, err := NewFixtureGitHubClient([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	primary := mk(`{"repos":{"corp/a":{"repo":{"archived":false},"tags":{"tags":[{"name":"v1","sha":"` + strings.Repeat("1", 40) + `"}]}},"corp/gone":{"missing":true},"actions/checkout":{"missing":true},"both/x":{"repo":{"default_branch":"server"},"tags":{"tags":[]}}}}`)
	secondary := mk(`{"repos":{"actions/checkout":{"repo":{"archived":false},"tags":{"tags":[{"name":"v4","sha":"` + strings.Repeat("4", 40) + `"}]},"advisories":{"list":[{"id":"GHSA-x","summary":"s","vulnerabilities":[]}]}},"corp/gone":{"missing":true},"corp/a":{"missing":true},"both/x":{"repo":{"default_branch":"dotcom"},"tags":{"tags":[]}}}}`)
	c := newFallbackGitHubClient(primary, secondary)
	ctx := context.Background()

	tags, err := c.Tags(ctx, "corp", "a")
	if err != nil || len(tags.Tags) != 1 || tags.Tags[0].Name != "v1" {
		t.Errorf("%+v, %v", tags, err)
	}
	tags, err = c.Tags(ctx, "actions", "checkout")
	if err != nil || len(tags.Tags) != 1 || tags.Tags[0].Name != "v4" {
		t.Errorf("%+v, %v", tags, err)
	}
	if r, err := c.Repository(ctx, "both", "x"); err != nil || r.DefaultBranch != "server" {
		t.Errorf("the Server must win: %+v, %v", r, err)
	}
	if _, err := c.Repository(ctx, "corp", "gone"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("on neither host: %v", err)
	}
	if _, err := c.Tags(ctx, "corp", "gone"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("on neither host: %v", err)
	}
	// The advisories are the ones of github.com
	adv, err := c.Advisories(ctx, "corp", "a")
	if err == nil && len(adv) != 0 {
		t.Errorf("%v", adv)
	}
	adv, err = c.AdvisoriesForPackages(ctx, []string{"actions/checkout"})
	if err != nil || len(adv) != 1 {
		t.Errorf("%v, %v", adv, err)
	}
	// Every repository is routed once: a repository asked about again does not ask the hosts again
	calls := primary.Calls() + secondary.Calls()
	if _, err := c.Repository(ctx, "actions", "checkout"); err != nil {
		t.Fatal(err)
	}
	if primary.Calls()+secondary.Calls() != calls {
		t.Error("the route of a repository is not remembered")
	}
}

// The rate limit of github.com (60 an hour without a token) does not stop the lookups on the Server.
func TestGitHubComFallbackRateLimitDoesNotStopTheServer(t *testing.T) {
	h := newTwoHosts(t, false)
	limit := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(4102444800)) // 2100
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	}
	for _, p := range []string{"/repos/actions/checkout", "/repos/example/archived", "/repos/nowhere/none", "/advisories"} {
		h.dotcom.handle(p, limit)
	}
	// The repositories of github.com come first, the one of the Server last
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: example/archived@v1", "uses: nowhere/none@v1", "uses: corp/internal@v1"), fallbackConfig)
	_, stdout, stderr := runOnlineCommand(t, "--online", "--online-github-com-fallback", wf)
	if !strings.Contains(stdout, `"corp/internal@v1" is in the archived repository corp/internal`) {
		t.Errorf("the Server must still answer:\n%s%s", stdout, stderr)
	}
	if n := h.dotcom.total(); n > 2 {
		t.Errorf("%d requests to github.com after its rate limit was reached", n)
	}
	if n := strings.Count(stderr, "rate limit"); n != 1 {
		t.Errorf("want one warning about the rate limit:\n%s", stderr)
	}
}

// gh prints $GH_TOKEN and $GITHUB_TOKEN as the token of github.com, which on a Server are its tokens.
func TestWithoutTokenVariables(t *testing.T) {
	env := []string{"PATH=/bin", "GH_TOKEN=a", "github_token=b", "GITHUB_TOKEN=c", "GH_HOST=x", "GITHUB_TOKEN_OF_SOMEONE=d", "GH_ENTERPRISE_TOKEN=e"}
	got := strings.Join(withoutTokenVariables(env), " ")
	if got != "PATH=/bin GH_HOST=x GITHUB_TOKEN_OF_SOMEONE=d GH_ENTERPRISE_TOKEN=e" {
		t.Errorf("%s", got)
	}
}

// The environments of the workflow are the ones of the Server: github.com is never asked, and the wrapper has no owner lookup.
func TestFallbackClientEnvironmentsAreTheServers(t *testing.T) {
	primary, err := NewFixtureGitHubClient([]byte(`{"repos":{"corp/ci":{"repo":{},"environments":{"release":{"found":true,"required_reviewers":true}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := NewFixtureGitHubClient([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	c := newFallbackGitHubClient(primary, secondary)
	env, err := c.Environment(context.Background(), "corp", "ci", "release")
	if err != nil || !env.RequiredReviewers {
		t.Errorf("%+v, %v", env, err)
	}
	if secondary.Calls() != 0 {
		t.Error("github.com was asked about an environment")
	}
	if _, ok := any(c).(GitHubOwnerClient); ok {
		t.Error("the owner lookup of github.com says nothing about the owners of a Server")
	}
}

// stubClient answers every question of a repository with the error, and counts the questions.
type stubClient struct {
	GitHubClient
	err error
	n   atomic.Int32
}

func (c *stubClient) Repository(context.Context, string, string) (*GitHubRepo, error) {
	c.n.Add(1)
	return nil, c.err
}

// A github.com which does not answer (a firewall which drops the packets, a proxy which hangs) costs the whole timeout of each
// request: after a few timeouts in a row it is not asked again, like the session does for the Server, instead of making every
// action which the Server does not have wait.
func TestFallbackClientStopsAskingAGitHubComWhichTimesOut(t *testing.T) {
	notFound := fmt.Errorf("x: %w", ErrGitHubNotFound)
	primary := &stubClient{err: notFound}
	secondary := &stubClient{err: &net.OpError{Op: "dial", Err: timeoutErr{}}}
	c := newFallbackGitHubClient(primary, secondary)
	for i := 0; i < 10; i++ {
		_, err := c.Repository(context.Background(), "o", fmt.Sprintf("r%d", i))
		var scoped *hostScopedError
		if !errors.As(err, &scoped) {
			t.Fatalf("repository %d: the failure of github.com must be the one of a host: %v", i, err)
		}
	}
	if n := secondary.n.Load(); n != maxConsecutiveGitHubFailures {
		t.Errorf("github.com was asked %d times, want %d", n, maxConsecutiveGitHubFailures)
	}
	if n := primary.n.Load(); n != 10 {
		t.Errorf("the Server was asked %d times, want 10", n)
	}
}

// A failure in between which is not one of an outage starts the count again.
func TestFallbackClientCountsFailuresInARow(t *testing.T) {
	c := newFallbackGitHubClient(&stubClient{err: fmt.Errorf("x: %w", ErrGitHubNotFound)}, &stubClient{})
	timeout := &net.OpError{Op: "dial", Err: timeoutErr{}}
	for i := 0; i < maxConsecutiveGitHubFailures-1; i++ {
		c.fromSecondary(timeout)
	}
	c.fromSecondary(nil)
	for i := 0; i < maxConsecutiveGitHubFailures-1; i++ {
		c.fromSecondary(timeout)
	}
	if c.secondaryDown() != nil {
		t.Error("github.com was given up too early")
	}
	c.fromSecondary(timeout)
	if c.secondaryDown() == nil {
		t.Error("github.com was not given up")
	}
}

// The advice of a rate limit of github.com names the token of github.com, not the one of the Server, and it says that only github.com is skipped.
func TestGitHubComFallbackRateLimitAdviceIsForGitHubCom(t *testing.T) {
	h := newTwoHosts(t, false)
	t.Setenv("GITHUB_COM_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	limit := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(4102444800)) // 2100
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	}
	for _, p := range []string{"/repos/actions/checkout", "/repos/example/archived", "/repos/nowhere/none", "/advisories"} {
		h.dotcom.handle(p, limit)
	}
	_, stderr := func() (string, string) {
		_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/internal@v1"), fallbackConfig)
		_, o, e := runOnlineCommand(t, "--online", "--online-github-com-fallback", wf)
		return o, e
	}()
	var warning string
	for _, l := range strings.Split(stderr, "\n") {
		if strings.Contains(l, "rate limit") {
			warning = l
		}
	}
	if warning == "" {
		t.Fatalf("no warning about the rate limit:\n%s", stderr)
	}
	if strings.Contains(warning, "GITHUB_TOKEN") || strings.Contains(warning, "GH_TOKEN") {
		t.Errorf("the advice names the token of the Server: %s", warning)
	}
	if !strings.Contains(warning, githubComTokenEnv) || !strings.Contains(warning, "api.github.com") {
		t.Errorf("the advice does not name the token and the host of the fallback: %s", warning)
	}
	if strings.Contains(warning, "lookups which need GitHub are skipped") {
		t.Errorf("the lookups on the Server are not skipped: %s", warning)
	}
}

// Without a fallback the advice is the one it always was.
func TestRateLimitAdviceWithoutAFallback(t *testing.T) {
	e := &GitHubRateLimitError{}
	if s := e.Error(); !strings.Contains(s, "GITHUB_TOKEN") {
		t.Errorf("%s", s)
	}
	e = &GitHubRateLimitError{TokenHint: "X or Y"}
	if s := e.Error(); strings.Contains(s, "GITHUB_TOKEN") || !strings.Contains(s, "X or Y") {
		t.Errorf("%s", s)
	}
	if s := (&GitHubRateLimitError{Authenticated: true, TokenHint: "X"}).Error(); strings.Contains(s, "X") {
		t.Errorf("a request with a token needs no advice: %s", s)
	}
}
