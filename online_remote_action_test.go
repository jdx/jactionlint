package jactionlint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// remoteMetadataConfig is a configuration which keeps the other online rules quiet, so that a test looks at the
// metadata lookups of the action rule and nothing else.
func remoteMetadataConfig() *Config {
	c := &Config{Rules: map[string]RuleConfig{}}
	for _, id := range []string{"archived-uses", "known-vulnerable-actions", "ref-confusion", "stale-action-refs", "impostor-commit", "ref-version-mismatch", "repojacking"} {
		c.Rules[id] = RuleConfig{Level: SeverityOff}
	}
	return c
}

// filesClient serves files of repositories; everything else is not found. It counts the file lookups.
type filesClient struct {
	GitHubClient
	files map[string]string // "owner/repo@ref:path" in lower case owner and repo
	calls atomic.Int32
	mu    sync.Mutex
	asked []string
	err   error // fails every lookup when set
	// errFor fails the lookups of the keys ("o/r@ref:path")
	errFor map[string]error
}

func (c *filesClient) FileContent(_ context.Context, owner, repo, ref, path string) ([]byte, error) {
	c.calls.Add(1)
	key := strings.ToLower(owner+"/"+repo) + "@" + ref + ":" + path
	c.mu.Lock()
	c.asked = append(c.asked, key)
	c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	if err, ok := c.errFor[key]; ok {
		return nil, err
	}
	if s, ok := c.files[key]; ok {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("%s: %w", key, ErrGitHubNotFound)
}

const remoteComposite = `name: tool
description: d
inputs:
  name:
    description: n
    required: true
  old:
    description: o
    deprecationMessage: use name
outputs:
  result:
    description: r
    value: ${{ steps.x.outputs.r }}
runs:
  using: composite
  steps:
    - run: echo
      shell: bash
`

func newFilesClient(t testing.TB, files map[string]string) *filesClient {
	t.Helper()
	return &filesClient{GitHubClient: onlineFixtureClient(t), files: files}
}

func failingFilesClient(t testing.TB, err error) *filesClient {
	t.Helper()
	return &filesClient{GitHubClient: onlineFixtureClient(t), err: err}
}

func lintRemote(t *testing.T, c GitHubClient, src string) ([]*Error, *Linter) {
	t.Helper()
	return lintOnline(t, c, remoteMetadataConfig(), src)
}

func TestRemoteActionInputsAreCheckedAtThePinnedRef(t *testing.T) {
	c := newFilesClient(t, map[string]string{"o/tool@v1:action.yml": remoteComposite})
	errs, l := lintRemote(t, c, workflowWith(
		"uses: o/tool@v1",
		"uses: o/tool@v1\n        with:\n          name: x\n          old: y\n          typo: z",
		"uses: o/tool@v1\n        with:\n          name: x",
	))
	got := lineIDsOf(errs)
	wantIDs := []string{"6:missing-action-input", "10:deprecated-action-input", "11:unknown-action-input"}
	if strings.Join(got, " ") != strings.Join(wantIDs, " ") {
		t.Errorf("findings = %v, want %v\n%v", got, wantIDs, errs)
	}
	if n := c.calls.Load(); n != 1 {
		t.Errorf("one action at one ref is read once, got %d lookups: %v", n, c.asked)
	}
	if l.OnlineSkipped() != 0 {
		t.Errorf("skipped = %d", l.OnlineSkipped())
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, `"o/tool@v1"`) || !strings.Contains(e.Message, "read from GitHub") {
			t.Errorf("the message must name the action and say where the metadata came from: %s", e.Message)
		}
	}
}

func TestRemoteActionLookupTriesActionYamlAndSubdirectories(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/tool@v1:sub/dir/action.yaml": remoteComposite,
		"o/tool@v1:action.yaml":         remoteComposite,
	})
	errs, _ := lintRemote(t, c, workflowWith("uses: o/tool/sub/dir@v1", "uses: o/tool/sub/dir/@v1", "uses: o/tool@v1"))
	if got := lineIDsOf(errs); len(got) != 3 {
		t.Errorf("findings = %v", got)
	}
	want := []string{"o/tool@v1:sub/dir/action.yml", "o/tool@v1:sub/dir/action.yaml", "o/tool@v1:action.yml", "o/tool@v1:action.yaml"}
	if strings.Join(c.asked, " ") != strings.Join(want, " ") {
		t.Errorf("asked %v, want %v", c.asked, want)
	}
}

func TestRemoteActionIsNotLookedUpWhenThereIsNothingToCheckAgainst(t *testing.T) {
	for name, uses := range map[string]string{
		"popular action":        "uses: actions/checkout@v4",
		"popular, pinned":       "uses: actions/checkout@" + strings.Repeat("a", 40) + " # v4.2.2",
		"local":                 "uses: ./local",
		"docker":                "uses: docker://alpine:3",
		"expression":            "uses: o/tool@${{ vars.REF }}",
		"expression in a path":  "uses: o/${{ vars.NAME }}@v1",
		"short sha":             "uses: o/tool@abcdef1",
		"not a plain name":      "uses: o/tool/a b@v1",
		"dot dot":               "uses: o/tool/../x@v1",
		"percent in the path":   "uses: o/tool/a%2fb@v1",
		"no ref":                "uses: o/tool",
		"owner with underscore": "uses: o_x/tool@v1",
	} {
		t.Run(name, func(t *testing.T) {
			c := newFilesClient(t, map[string]string{})
			lintRemote(t, c, workflowWith(uses))
			if n := c.calls.Load(); n != 0 {
				t.Errorf("%d lookups for %q: %v", n, uses, c.asked)
			}
		})
	}
}

// Nothing is reported when the metadata cannot be read, whatever the reason, and the other findings stay.
func TestRemoteActionReportsNothingWhenTheMetadataIsUnknown(t *testing.T) {
	steps := []string{"uses: o/tool@v1\n        with:\n          typo: z", "uses: actions/checkout@v4\n        with:\n          typo: z"}
	for name, c := range map[string]*filesClient{
		"not found":     newFilesClient(t, nil),
		"rate limit":    failingFilesClient(t, &GitHubRateLimitError{}),
		"server error":  failingFilesClient(t, &GitHubStatusError{Status: 502}),
		"forbidden":     failingFilesClient(t, &GitHubStatusError{Status: 403}),
		"invalid":       newFilesClient(t, map[string]string{"o/tool@v1:action.yml": "inputs: [a, b]\n"}),
		"not even yaml": newFilesClient(t, map[string]string{"o/tool@v1:action.yml": "a: [\n"}),
	} {
		t.Run(name, func(t *testing.T) {
			errs, l := lintRemote(t, c, workflowWith(steps...))
			// The popular action is still checked against the data set, the other one is not checked
			for _, e := range errs {
				if strings.Contains(e.Message, "o/tool") {
					t.Errorf("nothing is known about o/tool: %v", e)
				}
			}
			if len(errs) != 1 {
				t.Errorf("want the finding about the popular action: %v", errs)
			}
			if l.OnlineSkipped() == 0 {
				t.Errorf("a lookup which cannot be used is skipped, which --online=strict and the coverage report")
			}
		})
	}
}

func TestRemoteActionHonorsTheAllowAndDenyLists(t *testing.T) {
	c := newFilesClient(t, map[string]string{"corp/tool@v1:action.yml": remoteComposite})
	var out strings.Builder
	l, err := NewLinter(&out, &LinterOptions{Online: true, GitHubClient: c, OnlineOptions: OnlineOptions{Deny: []string{"corp/*"}}})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(remoteMetadataConfig())
	errs, err := l.Lint("test.yaml", []byte(workflowWith("uses: corp/tool@v1")), &Project{root: t.TempDir()})
	if err != nil || len(errs) != 0 {
		t.Fatalf("%v %v", errs, err)
	}
	if c.calls.Load() != 0 || l.OnlineSkipped() != 0 {
		t.Errorf("an excluded repository is not asked and is not a failure: %d lookups, %d skipped", c.calls.Load(), l.OnlineSkipped())
	}
	if cov := l.OnlineCoverage(); cov == nil || cov.ExcludedRepositories != 1 || cov.Status != "complete" {
		t.Errorf("coverage = %+v", cov)
	}
}

// A client which cannot read files (a GitHubClient written before the interface existed) makes the lookup
// unavailable, which is neither a finding nor a skipped lookup.
func TestRemoteActionNeedsAClientWhichReadsFiles(t *testing.T) {
	c := &countingClient{GitHubClient: onlineFixtureClient(t)}
	errs, l := lintRemote(t, c, workflowWith("uses: o/tool@v1\n        with:\n          typo: z"))
	if len(errs) != 0 || l.OnlineSkipped() != 0 || c.n.Load() != 0 {
		t.Errorf("%v skipped=%d calls=%d", errs, l.OnlineSkipped(), c.n.Load())
	}
}

// A repository whose files fail to be read is asked once however many refs the workflow uses, so that the
// failures of one repository do not add up to what the session takes for an outage of the API.
func TestRemoteActionFailureOfARepositoryIsNotRepeated(t *testing.T) {
	c := failingFilesClient(t, &GitHubStatusError{Status: 500})
	_, l := lintRemote(t, c, workflowWith("uses: o/tool@v1", "uses: o/tool@v2", "uses: o/tool@v3", "uses: o/tool/sub@v1", "uses: p/other@v1"))
	if n := c.calls.Load(); n != 2 {
		t.Errorf("want one failed lookup per repository, got %d: %v", n, c.asked)
	}
	if l.OnlineSkipped() != 5 {
		t.Errorf("every action which could not be checked counts: %d", l.OnlineSkipped())
	}
}

func TestRemoteActionOutputsAreTypedOnlyWhenTheyAreKnown(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/composite@v1:action.yml": remoteComposite,
		"o/bare@v1:action.yml":      "name: b\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: echo\n      shell: bash\n",
		"o/node@v1:action.yml":      "name: n\ndescription: d\noutputs:\n  a:\n    description: a\nruns:\n  using: node20\n  main: i.js\n",
		"o/dynamic@v1:action.yml":   "name: n\ndescription: d\nruns:\n  using: node20\n  main: i.js\n",
		"o/docker@v1:action.yml":    "name: n\ndescription: d\nruns:\n  using: docker\n  image: Dockerfile\n",
	})
	src := workflowWith(
		"id: composite\n        uses: o/composite@v1\n        with:\n          name: x",
		"id: bare\n        uses: o/bare@v1",
		"id: node\n        uses: o/node@v1",
		"id: dynamic\n        uses: o/dynamic@v1",
		"id: docker\n        uses: o/docker@v1",
		"id: script\n        uses: actions/github-script@v7\n        with:\n          script: ''",
		"run: echo ${{ steps.composite.outputs.result }} ${{ steps.composite.outputs.other }}",
		"run: echo ${{ steps.bare.outputs.x }}",
		"run: echo ${{ steps.node.outputs.a }} ${{ steps.node.outputs.b }}",
		"run: echo ${{ steps.dynamic.outputs.x }}",
		"run: echo ${{ steps.docker.outputs.x }}",
		"run: echo ${{ steps.script.outputs.x }}",
	)
	errs, _ := lintRemote(t, c, src)
	var got []string
	for _, e := range errs {
		got = append(got, fmt.Sprintf("%d:%s", e.Line, e.Kind))
	}
	// A composite action has only the outputs it declares. A JavaScript or Docker action may set any, declared or not.
	want := []string{"22:expression", "23:expression"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("findings = %v, want %v\n%v", got, want, errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Message, "not defined") {
			t.Errorf("unexpected message %q", e.Message)
		}
	}
}

// Every rule which needs the metadata shares one lookup, and the same commit in another case is the same lookup.
func TestRemoteActionLookupIsSharedByTheRules(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	c := newFilesClient(t, map[string]string{"o/tool@" + sha + ":action.yml": remoteComposite})
	src := workflowWith(
		"id: a\n        uses: o/tool@"+sha+"\n        with:\n          name: x",
		"id: b\n        uses: O/Tool@"+strings.ToUpper(sha)+"\n        with:\n          name: x",
		"run: echo ${{ steps.a.outputs.result }} ${{ steps.b.outputs.result }}",
	)
	if errs, _ := lintRemote(t, c, src); len(errs) != 0 {
		t.Errorf("%v", errs)
	}
	if n := c.calls.Load(); n != 1 {
		t.Errorf("the action rule and the expression rule share one lookup, and a SHA is not case sensitive: %d lookups %v", n, c.asked)
	}
}

func TestSessionRemoteActionIsConcurrencySafe(t *testing.T) {
	c := newFilesClient(t, map[string]string{"o/tool@v1:action.yml": remoteComposite})
	s := newOnlineSession(context.Background(), c, nil)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := s.RemoteAction("o", "tool", "", "v1")
			if err != nil || m == nil || m.Inputs["name"] == nil {
				t.Errorf("%v %v", m, err)
			}
		}()
	}
	wg.Wait()
	if n := c.calls.Load(); n != 1 {
		t.Errorf("%d lookups", n)
	}
	if _, err := s.RemoteAction("o", "missing", "", "v1"); err != nil {
		t.Errorf("a repository without metadata is not an error: %v", err)
	}
	if s.skippedLookups() != 1 {
		t.Errorf("but it is a lookup that could not be used: %d", s.skippedLookups())
	}
	if _, err := s.RemoteAction("o", "tool", "", "v1"); err != nil && errors.Is(err, ErrGitHubNotFound) {
		t.Error(err)
	}
}

func TestRemoteActionLookupAcceptsOnlyPlainNames(t *testing.T) {
	for uses, want := range map[string]bool{
		"o/r@v1":                         true,
		"o/r/sub@v1":                     true,
		"o/r/sub/dir/@main":              true,
		"o/r.x/.github/a@v1.2.3":         true,
		"o/r@" + strings.Repeat("a", 40): true,
		"o/r@feature/x":                  true,
		"o/r@abcdef1":                    false,
		"o/r/a b@v1":                     false,
		"o/r/..@v1":                      false,
		"o/r/a/../b@v1":                  false,
		"o/r@v1..2":                      false,
		"o/r@${{ x }}":                   false,
		"./o":                            false,
		"docker://o:1":                   false,
		"o/r/x.yml@v1":                   false, // a workflow, not an action
		"-o/r@v1":                        false,
		"o/r@":                           false,
	} {
		if got := remoteActionLookup(ParseUses(uses)); got != want {
			t.Errorf("remoteActionLookup(%q) = %v, want %v", uses, got, want)
		}
	}
}

// Many actions write `type: string` in an input, which GitHub does not document. The metadata of another repository is
// read as far as it goes; the file of the repository being linted is still reported.
func TestDecodeActionMetadataLenient(t *testing.T) {
	const withType = "name: x\ninputs:\n  a:\n    type: string\n    required: true\n  b:\n    description: d\nruns:\n  using: node20\n  main: i.js\n"
	if _, err := decodeActionMetadata([]byte(withType), false); err == nil || !strings.Contains(err.Error(), `unexpected key "type"`) {
		t.Errorf("a local action with an undocumented key is reported: %v", err)
	}
	m, err := decodeActionMetadata([]byte(withType), true)
	if err != nil || len(m.Inputs) != 2 || !m.Inputs["a"].Required || m.Runs.Using != "node20" {
		t.Errorf("%+v, %v", m, err)
	}
	for _, bad := range []string{"inputs: [a]\n", "a: [\n", "name: x\ninputs:\n  a: 1\n", "outputs: [x]\n"} {
		if m, err := decodeActionMetadata([]byte(bad), true); err == nil {
			t.Errorf("%q is not valid metadata: %+v", bad, m)
		}
	}
}

// In the cache mode a ref which is not in the cache says nothing about the other refs of the repository.
func TestRemoteActionNotCachedDoesNotSkipTheOtherRefs(t *testing.T) {
	c := newFilesClient(t, map[string]string{"o/tool@v2:action.yml": remoteComposite})
	c.errFor = map[string]error{"o/tool@v1:action.yml": fmt.Errorf("o/tool: %w", ErrGitHubNotCached)}
	s := newOnlineSession(context.Background(), c, nil)
	if _, err := s.RemoteAction("o", "tool", "", "v1"); !errors.Is(err, ErrGitHubNotCached) {
		t.Fatalf("v1: %v", err)
	}
	m, err := s.RemoteAction("o", "tool", "", "v2")
	if err != nil || m == nil {
		t.Errorf("v2 is cached and must be read: %v, %v", m, err)
	}
}

// The files of a repository which fails are asked for once, also when many refs are looked up at the same time,
// so that the failures of one repository do not add up to an outage of the API.
func TestRemoteActionFailureOfARepositoryIsKnownToConcurrentLookups(t *testing.T) {
	c := failingFilesClient(t, &GitHubStatusError{Status: 500})
	s := newOnlineSession(context.Background(), c, nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.RemoteAction("o", "tool", "", fmt.Sprintf("v%d", i)); err == nil {
				t.Error("want an error")
			}
		}()
	}
	wg.Wait()
	if n := c.calls.Load(); n != 1 {
		t.Errorf("%d lookups for a repository which fails: %v", n, c.asked)
	}
	if s.skippedLookups() != 16 {
		t.Errorf("every action which could not be checked counts: %d", s.skippedLookups())
	}
	if err := s.blockedErr(); err != nil {
		t.Errorf("one repository must not look like an outage: %v", err)
	}
}

// An action of the data set is not read from GitHub, however it is pinned: the version comment of a commit names
// the entry of the data set, as the action rule reads it. The outputs of the step must not ask either.
func TestPopularActionPinnedToACommitIsNotRead(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for name, tc := range map[string]struct {
		uses string
		want int // lookups: action.yml and action.yaml of a commit which the data set does not know
	}{
		"major":              {"actions/checkout@v4", 0},
		"tag":                {"actions/checkout@v4.2.2", 0},
		"commit, comment":    {"actions/checkout@" + sha + " # v4.2.2", 0},
		"commit, major":      {"actions/checkout@" + sha + " # v4", 0},
		"commit, no comment": {"actions/checkout@" + sha, 2},
	} {
		t.Run(name, func(t *testing.T) {
			c := newFilesClient(t, map[string]string{})
			lintRemote(t, c, workflowWith("id: co\n        uses: "+tc.uses, "run: echo ${{ steps.co.outputs.ref }}"))
			if n := c.calls.Load(); int(n) != tc.want {
				t.Errorf("%d lookups, want %d: %v", n, tc.want, c.asked)
			}
		})
	}
}
