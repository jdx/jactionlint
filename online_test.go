package jactionlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingClient wraps a GitHubClient and counts the calls.
type countingClient struct {
	GitHubClient
	n atomic.Int32
}

func (c *countingClient) Repository(ctx context.Context, o, r string) (*GitHubRepo, error) {
	c.n.Add(1)
	return c.GitHubClient.Repository(ctx, o, r)
}
func (c *countingClient) Tags(ctx context.Context, o, r string) (*GitHubTagList, error) {
	c.n.Add(1)
	return c.GitHubClient.Tags(ctx, o, r)
}
func (c *countingClient) ResolveRef(ctx context.Context, o, r string, ns GitHubRefNamespace, name string) (string, bool, error) {
	c.n.Add(1)
	return c.GitHubClient.ResolveRef(ctx, o, r, ns, name)
}
func (c *countingClient) Branches(ctx context.Context, o, r string, l int) (*GitHubBranchList, error) {
	c.n.Add(1)
	return c.GitHubClient.Branches(ctx, o, r, l)
}
func (c *countingClient) Compare(ctx context.Context, o, r, b, h string) (GitHubCompareStatus, error) {
	c.n.Add(1)
	return c.GitHubClient.Compare(ctx, o, r, b, h)
}
func (c *countingClient) Advisories(ctx context.Context, o, r string) ([]GitHubAdvisory, error) {
	c.n.Add(1)
	return c.GitHubClient.Advisories(ctx, o, r)
}

// failingClient fails every call with the error.
type failingClient struct {
	err error
	n   atomic.Int32
}

func (c *failingClient) Repository(context.Context, string, string) (*GitHubRepo, error) {
	c.n.Add(1)
	return nil, c.err
}
func (c *failingClient) Tags(context.Context, string, string) (*GitHubTagList, error) {
	c.n.Add(1)
	return nil, c.err
}
func (c *failingClient) ResolveRef(context.Context, string, string, GitHubRefNamespace, string) (string, bool, error) {
	c.n.Add(1)
	return "", false, c.err
}
func (c *failingClient) Branches(context.Context, string, string, int) (*GitHubBranchList, error) {
	c.n.Add(1)
	return nil, c.err
}
func (c *failingClient) Compare(context.Context, string, string, string, string) (GitHubCompareStatus, error) {
	c.n.Add(1)
	return "", c.err
}
func (c *failingClient) Advisories(context.Context, string, string) ([]GitHubAdvisory, error) {
	c.n.Add(1)
	return nil, c.err
}

// lintOnline lints the workflow with the online checks served by the client and returns the errors.
func lintOnline(t *testing.T, client GitHubClient, cfg *Config, src string) ([]*Error, *Linter) {
	t.Helper()
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{Online: true, GitHubClient: client, LogWriter: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		cfg = &Config{}
	}
	l.defaultConfig = withoutMissingTimeout(cfg)
	errs, err := l.Lint("test.yaml", []byte(src), &Project{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return errs, l
}

func workflowWith(steps ...string) string {
	var b strings.Builder
	b.WriteString("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n")
	for _, s := range steps {
		b.WriteString("      - " + s + "\n")
	}
	return b.String()
}

func lineIDsOf(errs []*Error) []string {
	var ret []string
	for _, e := range errs {
		ret = append(ret, fmt.Sprintf("%d:%s", e.Line, e.ID))
	}
	return ret
}

func TestSessionAsksOncePerRepository(t *testing.T) {
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	src := workflowWith(
		"uses: actions/checkout@v4",
		"uses: actions/checkout@v4",
		"uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2",
		"uses: Actions/Checkout@v4", // GitHub is case-insensitive
	)
	// Every file of a run shares the session
	_, l := lintOnline(t, c, nil, src)
	first := c.n.Load()
	if first == 0 {
		t.Fatal("the online rules asked nothing")
	}
	if _, err := l.Lint("other.yaml", []byte(src), &Project{root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if got := c.n.Load(); got != first {
		t.Errorf("a second file of the same run asked again: %d calls then %d", first, got)
	}
	// The repository, its tags, the refs, no advisory lookups beyond one: nowhere near one request per use
	if first > 5 {
		t.Errorf("%d requests for one repository", first)
	}
}

func TestOnlineRulesNeedTheFlag(t *testing.T) {
	c := &failingClient{err: errors.New("the network must not be used")}
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{GitHubClient: c, LogWriter: io.Discard}) // Online is false
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{Profile: ProfileAll}
	src := workflowWith("uses: actions/checkout@v4", "uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529 # v1")
	if _, err := l.Lint("test.yaml", []byte(src), &Project{root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if c.n.Load() != 0 {
		t.Errorf("the client was used %d times without Online", c.n.Load())
	}
}

func TestOnlineConfigKey(t *testing.T) {
	cfg, err := ParseConfig([]byte("online: true\n"))
	if err != nil || !cfg.Online {
		t.Fatalf("online: true was not parsed: %+v, %v", cfg, err)
	}
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{GitHubClient: c, LogWriter: io.Discard}) // No Online option
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(cfg)
	errs, err := l.Lint("test.yaml", []byte(workflowWith("uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529")), &Project{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := lineIDsOf(errs); len(got) != 2 || got[0] != "6:impostor-commit" {
		t.Errorf("the config key should turn the online checks on: %v", got)
	}

	cfg, err = ParseConfig([]byte("online: false\n"))
	if err != nil || cfg.Online {
		t.Fatalf("online: false: %+v, %v", cfg, err)
	}
	if _, err := ParseConfig([]byte("online: sometimes\n")); err == nil {
		t.Error("online takes a boolean")
	}
}

func TestOnlineRulesFollowRuleLevels(t *testing.T) {
	c := onlineFixtureClient(t)
	src := workflowWith("uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529")
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}, "impostor-commit": {Level: SeverityWarning, levelSet: true}}}
	errs, _ := lintOnline(t, c, cfg, src)
	if len(errs) != 1 || errs[0].ID != "impostor-commit" || errs[0].Severity != SeverityWarning {
		t.Errorf("want only impostor-commit as a warning: %v", lineIDsOf(errs))
	}
	// Profiles do not matter for online rules
	for _, p := range []Profile{ProfileDefault, ProfileStrict, ProfileAll} {
		errs, _ := lintOnline(t, c, &Config{Profile: p}, src)
		var online []*Error
		for _, e := range errs {
			if e.ID == "impostor-commit" || e.ID == "stale-action-refs" {
				online = append(online, e)
			}
		}
		if len(online) != 2 {
			t.Errorf("profile %s: want both online findings but got %v", p, lineIDsOf(errs))
		}
	}
}

func TestKnownVulnerableActionsAllowOption(t *testing.T) {
	src := workflowWith("uses: tj-actions/changed-files@v44")
	cfg, err := ParseConfig([]byte("rules:\n  known-vulnerable-actions:\n    allow: [GHSA-mrrh-fwg8-r2c3]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if errs, _ := lintOnline(t, onlineFixtureClient(t), cfg, src); len(errs) != 0 {
		t.Errorf("the advisory is allowed: %v", errs)
	}
	if errs, _ := lintOnline(t, onlineFixtureClient(t), nil, src); len(errs) != 1 {
		t.Errorf("want the advisory reported: %v", errs)
	}
	for _, bad := range []string{"allow: GHSA-1", "allow: [1, 2]", "allow: {a: b}"} {
		if _, err := ParseConfig([]byte("rules:\n  known-vulnerable-actions:\n    " + bad + "\n")); err == nil || !strings.Contains(err.Error(), "list of strings") {
			t.Errorf("%q should be refused: %v", bad, err)
		}
	}
}

func TestImpostorCommitMaxBranches(t *testing.T) {
	src := workflowWith("uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529")
	cfg, err := ParseConfig([]byte("rules:\n  stale-action-refs: off\n  impostor-commit:\n    max-branches: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	// The fixture has two branches, the default one and releases/v1. With a limit of one branch the verdict is unknown.
	if errs, _ := lintOnline(t, onlineFixtureClient(t), cfg, src); len(errs) != 0 {
		t.Errorf("without all branches there is no verdict: %v", lineIDsOf(errs))
	}
	cfg, _ = ParseConfig([]byte("rules:\n  stale-action-refs: off\n  impostor-commit:\n    max-branches: 2\n"))
	if errs, _ := lintOnline(t, onlineFixtureClient(t), cfg, src); len(errs) != 1 {
		t.Errorf("with all branches the commit is an impostor: %v", lineIDsOf(errs))
	}
}

func TestOnlineSkipsWhatItCannotJudge(t *testing.T) {
	c := onlineFixtureClient(t)
	src := workflowWith(
		"uses: ./local",
		"uses: docker://alpine:3.19",
		"uses: actions/checkout@${{ matrix.ref }}",
		"uses: actions/checkout", // No ref
		"uses: ../../x/y@v1",
		"uses: a b/c@v1",
		"uses: example/forgotten@v1",
		"uses: actions/checkout@abc1234", // Short SHA
	)
	errs, _ := lintOnline(t, c, nil, src)
	for _, e := range errs {
		switch e.ID {
		case "impostor-commit", "known-vulnerable-actions", "ref-confusion", "stale-action-refs", "archived-uses", "ref-version-mismatch":
			t.Errorf("unexpected finding %s: %s", e.ID, e.Message)
		}
	}
}

func TestSessionSkipsFailedLookupsAndWarnsOncePerKind(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
		// blocks is whether the session stops asking after the failures (the API cannot answer)
		blocks bool
		calls  int32
	}{
		{"server errors", &GitHubStatusError{Status: 502}, "502", true, maxConsecutiveGitHubFailures},
		{"forbidden", &GitHubStatusError{Status: 403, Message: "blocked"}, "403", false, 20},
		{"timeout", &net.OpError{Op: "dial", Err: timeoutErr{}}, "did not answer in time", true, maxConsecutiveGitHubFailures},
		{"dns", &net.DNSError{Err: "no such host", Name: "api.github.com"}, "no such host", true, 1},
		{"unauthorized", &GitHubStatusError{Status: 401}, "rejected the token", true, 1},
		{"rate limit", &GitHubRateLimitError{Reset: time.Now().Add(time.Hour)}, "rate limit", true, 1},
		{"not found", fmt.Errorf("x: %w", ErrGitHubNotFound), "not found on GitHub", false, 20},
		{"not cached", fmt.Errorf("x: %w", ErrGitHubNotCached), "no cached answer", false, 20},
		{"unknown error", errors.New("boom"), "boom", true, maxConsecutiveGitHubFailures},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &failingClient{err: tc.err}
			var warnings, details []string
			s := newOnlineSession(context.Background(), c, func(m string) { warnings = append(warnings, m) })
			s.detail = func(f string, a ...any) { details = append(details, fmt.Sprintf(f, a...)) }
			for i := 0; i < 20; i++ {
				s.Repository("o", fmt.Sprintf("r%d", i))
			}
			if got := c.n.Load(); got != tc.calls {
				t.Errorf("the client was called %d times. want %d", got, tc.calls)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.wantMsg) {
				t.Errorf("want one warning containing %q but got %q", tc.wantMsg, warnings)
			}
			if got := s.skippedLookups(); got != 20 {
				t.Errorf("every lookup of the 20 is skipped and counted. got %d", got)
			}
			if len(details) != 20 {
				t.Errorf("-verbose names every skipped lookup: got %d lines", len(details))
			}
			if (s.blockedErr() != nil) != tc.blocks {
				t.Errorf("blocked = %v. want %v", s.blockedErr() != nil, tc.blocks)
			}
			if s.stopped() != nil {
				t.Error("a failed lookup does not stop the run")
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestSessionCanceledStops(t *testing.T) {
	c := &failingClient{err: context.Canceled}
	var warnings []string
	s := newOnlineSession(context.Background(), c, func(m string) { warnings = append(warnings, m) })
	for i := 0; i < 5; i++ {
		s.Repository("o", fmt.Sprintf("r%d", i))
	}
	if c.n.Load() != 1 || len(warnings) != 1 || !strings.Contains(warnings[0], "stopped") || s.stopped() == nil {
		t.Errorf("calls %d, warnings %q", c.n.Load(), warnings)
	}
}

func TestSessionOneFailureDoesNotHideTheOthers(t *testing.T) {
	// The repository "bad" fails in every way a single repository can; the others are answered
	for _, bad := range []error{
		fmt.Errorf("x: %w", ErrGitHubNotFound),
		&GitHubStatusError{Status: 403},
		&GitHubStatusError{Status: 500},
		&net.OpError{Op: "read", Err: timeoutErr{}},
	} {
		good := onlineFixtureClient(t)
		c := &selectiveClient{GitHubClient: good, failFor: "bad", err: bad}
		var warnings []string
		s := newOnlineSession(context.Background(), c, func(m string) { warnings = append(warnings, m) })
		if _, err := s.Repository("o", "bad"); err == nil {
			t.Fatal("the bad lookup must fail")
		}
		for i := 0; i < 5; i++ {
			if _, err := s.Repository("actions", "checkout"); err != nil {
				t.Errorf("%v: a good lookup after a failed one must work: %v", bad, err)
			}
			s.Tags("actions", "checkout")
		}
		if len(warnings) != 1 {
			t.Errorf("%v: want one warning but got %q", bad, warnings)
		}
		if s.skippedLookups() != 1 {
			t.Errorf("%v: want one skipped lookup but got %d", bad, s.skippedLookups())
		}
	}
}

type selectiveClient struct {
	GitHubClient
	failFor string
	err     error
}

func (c *selectiveClient) Repository(ctx context.Context, o, r string) (*GitHubRepo, error) {
	if r == c.failFor {
		return nil, c.err
	}
	return c.GitHubClient.Repository(ctx, o, r)
}

func TestSessionRateLimitBlocksUntilReset(t *testing.T) {
	reset := time.Now().Add(50 * time.Millisecond)
	c := &failingClient{err: &GitHubRateLimitError{Reset: reset}}
	s := newOnlineSession(context.Background(), c, nil)
	s.Repository("o", "a")
	s.Repository("o", "b")
	if c.n.Load() != 1 {
		t.Fatalf("the session must not ask while the limit holds: %d calls", c.n.Load())
	}
	time.Sleep(80 * time.Millisecond)
	s.Repository("o", "c")
	if c.n.Load() != 2 {
		t.Errorf("the session should ask again after the reset: %d calls", c.n.Load())
	}
}

func TestSessionAllowAndDenyLists(t *testing.T) {
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	var warnings []string
	s := newOnlineSession(context.Background(), c, func(m string) { warnings = append(warnings, m) })
	s.allow = []string{"actions/*", "Docker/Login-Action"}
	s.deny = []string{"actions/setup-*"}
	for _, tc := range []struct {
		owner, repo string
		want        bool
	}{
		{"actions", "checkout", true},
		{"Actions", "Checkout", true},
		{"actions", "setup-node", false}, // deny wins
		{"mycorp", "private-action", false},
	} {
		_, err := s.Repository(tc.owner, tc.repo)
		if excluded := errors.Is(err, errOnlineExcluded); excluded == tc.want {
			t.Errorf("%s/%s: excluded = %v", tc.owner, tc.repo, excluded)
		}
	}
	if !s.allows("docker", "login-action") || s.allows("docker", "other") {
		t.Error("patterns match case-insensitively and exactly")
	}
	if len(warnings) != 0 || s.skippedLookups() != 0 {
		t.Errorf("an excluded lookup is not a failure: %q, %d", warnings, s.skippedLookups())
	}
	if _, _, err := s.TagCommit("mycorp", "x", "v1"); !errors.Is(err, errOnlineExcluded) {
		t.Errorf("TagCommit: %v", err)
	}
	if _, err := s.Advisories("mycorp", "x", ""); !errors.Is(err, errOnlineExcluded) {
		t.Errorf("Advisories: %v", err)
	}
}

func TestSessionRecoversBetweenFailures(t *testing.T) {
	// Two failures and a success: the failures do not add up
	var warnings []string
	var n atomic.Int32
	mixed := &flakyClient{GitHubClient: onlineFixtureClient(t), fail: func() bool { return n.Add(1)%3 != 0 }}
	s := newOnlineSession(context.Background(), mixed, func(m string) { warnings = append(warnings, m) })
	for i := 0; i < 9; i++ {
		s.Tags("actions", fmt.Sprintf("checkout%d", i)) // Different repositories: no memoization
	}
	if len(warnings) != 1 || s.blockedErr() != nil {
		t.Errorf("failures separated by successes must not stop the session: %q", warnings)
	}
	if s.skippedLookups() != 6 {
		t.Errorf("six of nine lookups failed: %d", s.skippedLookups())
	}
}

type flakyClient struct {
	GitHubClient
	fail func() bool
}

func (c *flakyClient) Tags(ctx context.Context, o, r string) (*GitHubTagList, error) {
	if c.fail() {
		return nil, &GitHubStatusError{Status: 500}
	}
	return c.GitHubClient.Tags(ctx, "actions", "checkout")
}

func TestSessionStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	var warnings []string
	s := newOnlineSession(ctx, c, func(m string) { warnings = append(warnings, m) })
	if _, err := s.Repository("actions", "checkout"); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := s.Repository("actions", "setup-node"); err == nil {
		t.Error("a canceled session must not answer")
	}
	if c.n.Load() != 1 || len(warnings) != 1 {
		t.Errorf("calls: %d, warnings: %q", c.n.Load(), warnings)
	}
}

func TestSessionIsSafeForConcurrentUse(t *testing.T) {
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	s := newOnlineSession(context.Background(), c, nil)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Repository("actions", "checkout")
			s.Tags("actions", "checkout")
			s.TagCommit("actions", "checkout", "v4")
			s.BranchCommit("actions", "checkout", "main")
			s.Advisories("actions", "checkout", "")
		}()
	}
	wg.Wait()
	// Repository, tags, the ref of the branch and the advisories. The tag came from the list.
	if got := c.n.Load(); got != 4 {
		t.Errorf("want 4 calls but got %d", got)
	}
}

func TestTagCommitFallsBackToTheRefWhenTheListIsTruncated(t *testing.T) {
	fx := []byte(`{"repos":{"o/r":{"tags":{"tags":[{"name":"v1","sha":"` + strings.Repeat("a", 40) + `"}],"truncated":true},
	  "refs":{"tags/v0":{"sha":"` + strings.Repeat("b", 40) + `","found":true},"tags/nope":{"found":false}}}}}`)
	c, err := NewFixtureGitHubClient(fx)
	if err != nil {
		t.Fatal(err)
	}
	s := newOnlineSession(context.Background(), c, nil)
	if sha, ok, err := s.TagCommit("o", "r", "v1"); err != nil || !ok || sha != strings.Repeat("a", 40) {
		t.Errorf("v1: %q, %v, %v", sha, ok, err)
	}
	if sha, ok, err := s.TagCommit("o", "r", "v0"); err != nil || !ok || sha != strings.Repeat("b", 40) {
		t.Errorf("v0: %q, %v, %v", sha, ok, err)
	}
	if _, ok, err := s.TagCommit("o", "r", "nope"); err != nil || ok {
		t.Errorf("nope: %v, %v", ok, err)
	}
}

func TestStaleActionRefsIsSilentWhenTagsAreTruncated(t *testing.T) {
	fx := `{"repos":{"o/r":{"repo":{"default_branch":"main"},"tags":{"tags":[],"truncated":true}}}}`
	c, _ := NewFixtureGitHubClient([]byte(fx))
	errs, _ := lintOnline(t, c, &Config{Rules: map[string]RuleConfig{"impostor-commit": {Level: SeverityOff, levelSet: true}, "known-vulnerable-actions": {Level: SeverityOff, levelSet: true}}},
		workflowWith("uses: o/r@"+strings.Repeat("c", 40)))
	for _, e := range errs {
		t.Errorf("unexpected %s: %s", e.ID, e.Message)
	}
}

func TestVersionOf(t *testing.T) {
	s := newOnlineSession(context.Background(), onlineFixtureClient(t), nil)
	tests := []struct {
		spec string
		want string // "" when there is no version
	}{
		{"tj-actions/changed-files@v45.0.7", "45.0.7"},
		{"tj-actions/changed-files@v45", "45.0.2"}, // The most specific tag of the commit
		{"tj-actions/changed-files@48d8f15b2aaa3d255ca5af3eba4870f807ce6b3c", "45.0.2"},
		{"tj-actions/changed-files@v46", "46"},
		{"tj-actions/changed-files@main", ""},
		{"tj-actions/changed-files@" + strings.Repeat("e", 40), ""},
		{"actions/checkout@v4", "4"},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			v, ok, err := s.versionOf(ParseUses(tc.spec))
			if err != nil {
				t.Fatal(err)
			}
			if got := v.String(); (tc.want == "") == ok || (ok && got != tc.want) {
				t.Errorf("versionOf = %q, %v. want %q", got, ok, tc.want)
			}
		})
	}
}

func TestCommentVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"v4.2.2", "v4.2.2", true},
		{" 4.2.2 ", "4.2.2", true},
		{"tag=v4", "v4", true},
		{"v1.2.3 (bumped by a bot)", "v1.2.3", true},
		{"v4.2.2-rc.1", "v4.2.2-rc.1", true},
		{"pinned for ci", "", false},
		{"see issue #12", "", false},
		{"keep in sync with v4", "", false}, // The version is not what the comment is about
		{"zizmor: ignore[cache-poisoning] v2", "", false},
		{"Tag: v4", "v4", true},
		{"version=1.2", "1.2", true},
		{"", "", false},
		{"v", "", false},
		{"version: 3", "3", true},
	}
	for _, tc := range tests {
		got, ok := commentVersion(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("commentVersion(%q) = %q, %v. want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func FuzzCommentVersion(f *testing.F) {
	for _, s := range []string{"v4.2.2", "tag=v4", "", "(((", "v1.2.3-"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if v, ok := commentVersion(s); ok && v == "" {
			t.Errorf("an empty version for %q", s)
		}
	})
}

// With more tags than were read, the commit may be a tagged release whose branch is gone. There is no
// verdict then, as stale-action-refs does, unless a branch has the commit.
func TestImpostorCommitIsSilentWhenTagsAreTruncated(t *testing.T) {
	sha := strings.Repeat("c", 40)
	head := strings.Repeat("a", 40)
	fx := func(truncated bool, compare string) string {
		return `{"repos":{"o/r":{"repo":{"default_branch":"main"},
		  "tags":{"tags":[],"truncated":` + fmt.Sprint(truncated) + `},
		  "branches":{"branches":[{"name":"main","sha":"` + head + `"}]},
		  "refs":{"heads/main":{"sha":"` + head + `","found":true}},
		  "compare":{"` + head + `...` + sha + `":"` + compare + `"}}}}`
	}
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}, "known-vulnerable-actions": {Level: SeverityOff, levelSet: true}}}
	run := func(truncated bool, compare string) []*Error {
		c, err := NewFixtureGitHubClient([]byte(fx(truncated, compare)))
		if err != nil {
			t.Fatal(err)
		}
		errs, _ := lintOnline(t, c, cfg, workflowWith("uses: o/r@"+sha))
		return errs
	}
	if errs := run(false, "diverged"); len(errs) != 1 || errs[0].ID != "impostor-commit" {
		t.Fatalf("with all the tags read the commit is an impostor: %v", lineIDsOf(errs))
	}
	if errs := run(true, "diverged"); len(errs) != 0 {
		t.Errorf("the tag list is truncated, so there is no verdict: %v", lineIDsOf(errs))
	}
	// A commit that a branch has is the repository's own whatever the tags say
	if errs := run(true, "behind"); len(errs) != 0 {
		t.Errorf("the commit is on the default branch: %v", lineIDsOf(errs))
	}
}

// GitHub publishes the advisories of an action in a subdirectory under the full package name, so the
// lookup of the repository alone finds nothing for them.
func TestKnownVulnerableActionsFindsAdvisoriesOfASubdirectory(t *testing.T) {
	adv := `{"id":"GHSA-sub","summary":"setup-gradle leaks","severity":"high","url":"https://github.com/advisories/GHSA-sub",
	  "vulnerabilities":[{"package":"gradle/actions/setup-gradle","range":"< 4.2.0","first_patched":"4.2.0"}]}`
	fx := `{"repos":{"gradle/actions":{"repo":{"default_branch":"main"},
	  "advisories":{"list":[],"packages":{"gradle/actions/setup-gradle":[` + adv + `]}}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}}}
	errs, _ := lintOnline(t, c, cfg, workflowWith("uses: gradle/actions/setup-gradle@v4.1.0", "uses: gradle/actions/wrapper-validation@v4.1.0"))
	if got := lineIDsOf(errs); len(got) != 1 || got[0] != "6:known-vulnerable-actions" {
		t.Fatalf("want the advisory for the first step only: %v", got)
	}
	if !strings.Contains(errs[0].Message, "GHSA-sub") {
		t.Errorf("unexpected message %q", errs[0].Message)
	}
}

// scanFailingClient fails every branch scan like a GraphQL query with errors.
type scanFailingClient struct{ GitHubClient }

func (scanFailingClient) CommitOnAnyBranch(context.Context, string, string, string, int) (GitHubBranchScan, error) {
	return GitHubBranchScan{}, fmt.Errorf("%w: GraphQL: Could not resolve", ErrGitHubBranchScanUnavailable)
}

// A branch scan that fails does not stop the online rules: the branches are compared over REST.
func TestFailingBranchScansDoNotStopTheSession(t *testing.T) {
	head := strings.Repeat("a", 40)
	shas := []string{strings.Repeat("c", 40), strings.Repeat("d", 40), strings.Repeat("e", 40), strings.Repeat("f", 40)}
	compare := ""
	var steps []string
	for i, sha := range shas {
		if i > 0 {
			compare += ","
		}
		compare += `"` + head + `...` + sha + `":"diverged"`
		steps = append(steps, "uses: o/r@"+sha)
	}
	fx := `{"repos":{"o/r":{"repo":{"default_branch":"main"},"tags":{"tags":[]},
	  "branches":{"branches":[{"name":"main","sha":"` + head + `"}]},
	  "refs":{"heads/main":{"sha":"` + head + `","found":true}},
	  "compare":{` + compare + `}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}, "known-vulnerable-actions": {Level: SeverityOff, levelSet: true}}}
	errs, _ := lintOnline(t, scanFailingClient{c}, cfg, workflowWith(steps...))
	n := 0
	for _, e := range errs {
		if e.ID == "impostor-commit" {
			n++
		}
	}
	if n != len(shas) {
		t.Errorf("want %d impostor findings from the REST comparison but got %v", len(shas), lineIDsOf(errs))
	}
}

// resolveFailingClient fails ResolveRef like a lookup that cannot be answered.
type resolveFailingClient struct{ GitHubClient }

func (resolveFailingClient) ResolveRef(context.Context, string, string, GitHubRefNamespace, string) (string, bool, error) {
	return "", false, errors.New("boom")
}

// A tag that cannot be looked up is not a tag which does not exist.
func TestRefVersionMismatchIsSilentWhenTheTagLookupFails(t *testing.T) {
	sha := strings.Repeat("c", 40)
	fx := `{"repos":{"o/r":{"repo":{"default_branch":"main"},"tags":{"tags":[],"truncated":true},
	  "refs":{"tags/v2":{"found":false},"tags/2":{"found":false}}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}, "impostor-commit": {Level: SeverityOff, levelSet: true}, "known-vulnerable-actions": {Level: SeverityOff, levelSet: true}}}
	src := workflowWith("uses: o/r@" + sha + " # v2")
	if errs, _ := lintOnline(t, c, cfg, src); len(errs) != 1 || errs[0].ID != "ref-version-mismatch" {
		t.Fatalf("a tag which does not exist is a mismatch: %v", lineIDsOf(errs))
	}
	for _, e := range func() []*Error { errs, _ := lintOnline(t, resolveFailingClient{c}, cfg, src); return errs }() {
		t.Errorf("unexpected %s: %s", e.ID, e.Message)
	}
}

// An ignore for an online rule is not unused while the online checks are off: the rule did not run.
func TestUnusedIgnoreOfOnlineRules(t *testing.T) {
	src := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      # jactionlint ignore=impostor-commit\n      - uses: actions/checkout@v4\n"
	cfg := mustParseConfig(t, "profile: strict\nrules:\n  unused-ignore: error\n  missing-timeout: off\n  stale-action-refs: off\n")
	unused := func(errs []*Error) int { return len(errsWithID(errs, "unused-ignore")) }

	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	errs, err := l.Lint("test.yaml", []byte(src), &Project{root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := unused(errs); got != 0 {
		t.Errorf("offline: the online rule could not report, so the ignore is not unused: %v", lineIDsOf(errs))
	}
	if errs, _ := lintOnline(t, onlineFixtureClient(t), cfg, src); unused(errs) != 1 {
		t.Errorf("online: the rule ran and found nothing to ignore: %v", lineIDsOf(errs))
	}
}

func TestAffectedByPrefersThePackageOfTheSubdirectory(t *testing.T) {
	adv := func(vs ...GitHubVulnerability) GitHubAdvisory {
		return GitHubAdvisory{ID: "GHSA-test", Vulnerabilities: vs}
	}
	root := GitHubVulnerability{Package: "o/r", VulnerableRange: "< 2.0.0", FirstPatched: "2.0.0"}
	sub := GitHubVulnerability{Package: "o/r/sub", VulnerableRange: "< 1.5.0", FirstPatched: "1.5.0"}
	other := GitHubVulnerability{Package: "o/r/other", VulnerableRange: "< 9.0.0", FirstPatched: "9.0.0"}
	ver := func(s string) advisoryVersion {
		v, ok := parseAdvisoryVersion(s)
		if !ok {
			t.Fatalf("version %q", s)
		}
		return v
	}
	tests := []struct {
		name        string
		a           GitHubAdvisory
		uses        string
		version     string
		wantFound   bool
		wantPatched string
	}{
		{"root advisory, subdirectory action: the root ranges apply", adv(root), "o/r/sub@v1", "1.9.0", true, "2.0.0"},
		{"root advisory, subdirectory action, patched", adv(root), "o/r/sub@v1", "2.1.0", false, ""},
		{"root advisory, root action", adv(root), "o/r@v1", "1.9.0", true, "2.0.0"},
		{"subdirectory advisory, root action: not affected", adv(sub), "o/r@v1", "1.0.0", false, ""},
		{"subdirectory advisory, another subdirectory: not affected", adv(sub), "o/r/else@v1", "1.0.0", false, ""},
		{"subdirectory advisory, its action", adv(sub), "o/r/sub@v1", "1.0.0", true, "1.5.0"},
		{"both listed: the subdirectory ranges and patched version win", adv(root, sub), "o/r/sub@v1", "1.0.0", true, "1.5.0"},
		{"both listed, order does not matter", adv(sub, root), "o/r/sub@v1", "1.0.0", true, "1.5.0"},
		{"both listed: patched for the subdirectory although the root range still covers it", adv(root, sub), "o/r/sub@v1", "1.7.0", false, ""},
		{"both listed, root action uses the root entry", adv(sub, root), "o/r@v1", "1.7.0", true, "2.0.0"},
		{"the entry of another subdirectory does not hide the root entry", adv(other, root), "o/r/sub@v1", "1.9.0", true, "2.0.0"},
		{"package names are compared without case", adv(GitHubVulnerability{Package: "O/R/Sub", VulnerableRange: "< 3", FirstPatched: "3"}), "o/r/sub@v1", "1.0.0", true, "3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref := ParseUses(tc.uses)
			got, found := affectedBy(tc.a, ref, ver(tc.version))
			if found != tc.wantFound || got.FirstPatched != tc.wantPatched {
				t.Errorf("affectedBy = %+v, %v; want found=%v patched=%q", got, found, tc.wantFound, tc.wantPatched)
			}
		})
	}

	// Two advisories for the same action are reported on their own
	a1 := GitHubAdvisory{ID: "GHSA-1", Vulnerabilities: []GitHubVulnerability{root}}
	a2 := GitHubAdvisory{ID: "GHSA-2", Vulnerabilities: []GitHubVulnerability{sub, root}}
	ref := ParseUses("o/r/sub@v1")
	v1, f1 := affectedBy(a1, ref, ver("1.7.0"))
	v2, f2 := affectedBy(a2, ref, ver("1.7.0"))
	if !f1 || v1.FirstPatched != "2.0.0" || f2 {
		t.Errorf("GHSA-1 = %+v %v, GHSA-2 = %+v %v", v1, f1, v2, f2)
	}
}
