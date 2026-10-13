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

const environmentWorkflow = `on: pull_request_target
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ENVIRONMENT
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci && npm test
`

// environmentConfig leaves the online rules off, so that the requests of the tests are the ones for the environments
const environmentConfig = "profile: default\nrules:\n  missing-timeout: off\n  impostor-commit: off\n  known-vulnerable-actions: off\n  ref-confusion: off\n  stale-action-refs: off\n  archived-uses: off\n  ref-version-mismatch: off\n  unpinned-uses: off\n  repojacking: off\n"

// environmentRun lints the workflow of a job in the environment with the online checks and returns the output and
// the skipped lookups of the report.
func environmentRun(t *testing.T, f *fakeGitHub, env string, args ...string) (stdout string, skipped float64) {
	t.Helper()
	_, wf := onlineProject(t, strings.ReplaceAll(environmentWorkflow, "ENVIRONMENT", env), environmentConfig)
	report := filepath.Join(t.TempDir(), "report.json")
	status, stdout, stderr := runOnlineCommand(t, append([]string{"--online", "--online-report", report}, append(args, wf)...)...)
	if status > ExitStatusSuccessProblemFound {
		t.Fatalf("status %d\n%s%s", status, stdout, stderr)
	}
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

func environmentJSON(rules string) string {
	return fmt.Sprintf(`{"id":1,"name":"x","protection_rules":[%s],"deployment_branch_policy":null}`, rules)
}

// The environment of the job decides when it has required reviewers, and is only a name when it has none.
func TestEnvironmentGateRequests(t *testing.T) {
	const reviewers = `{"id":1,"type":"required_reviewers","reviewers":[{"type":"User","reviewer":{"login":"octocat"}}]}`
	const branchPolicy = `{"id":2,"type":"branch_policy"}`
	for _, tc := range []struct {
		name     string
		env      string
		answer   func(f *fakeGitHub)
		finding  bool
		skipped  float64
		requests map[string]int
	}{
		{
			name: "required reviewers",
			env:  "release",
			answer: func(f *fakeGitHub) {
				f.json("/repos/o/r/environments/release", environmentJSON(branchPolicy+","+reviewers))
			},
			requests: map[string]int{"/repos/o/r/environments/release": 1, "/repos/o/r": 1},
		},
		{
			name:     "no required reviewers",
			env:      "release",
			answer:   func(f *fakeGitHub) { f.json("/repos/o/r/environments/release", environmentJSON(branchPolicy)) },
			finding:  true,
			requests: map[string]int{"/repos/o/r/environments/release": 1, "/repos/o/r": 1},
		},
		{
			name:     "no rules at all",
			env:      "release",
			answer:   func(f *fakeGitHub) { f.json("/repos/o/r/environments/release", environmentJSON("")) },
			finding:  true,
			requests: map[string]int{"/repos/o/r/environments/release": 1, "/repos/o/r": 1},
		},
		{
			// The environment does not exist in a public repository: GitHub creates it empty
			name: "missing in a public repository",
			env:  "relase",
			answer: func(f *fakeGitHub) {
				f.json("/repos/o/r", `{"private":false,"default_branch":"main"}`)
			},
			finding:  true,
			requests: map[string]int{"/repos/o/r/environments/relase": 1, "/repos/o/r": 1},
		},
		{
			// The token may not see the environment of a private repository: nothing is known
			name: "not found in a private repository",
			env:  "release",
			answer: func(f *fakeGitHub) {
				f.json("/repos/o/r", `{"private":true,"default_branch":"main"}`)
			},
			skipped:  1,
			requests: map[string]int{"/repos/o/r/environments/release": 1, "/repos/o/r": 1},
		},
		{
			// An internal repository, or an answer which does not say that the repository is public, is not public
			name: "not found, the repository does not say it is public",
			env:  "release",
			answer: func(f *fakeGitHub) {
				f.json("/repos/o/r", `{"default_branch":"main"}`)
			},
			skipped:  1,
			requests: map[string]int{"/repos/o/r/environments/release": 1, "/repos/o/r": 1},
		},
		{
			name: "not found in an internal repository",
			env:  "release",
			answer: func(f *fakeGitHub) {
				f.json("/repos/o/r", `{"private":true,"visibility":"internal","default_branch":"main"}`)
			},
			skipped:  1,
			requests: map[string]int{"/repos/o/r/environments/release": 1, "/repos/o/r": 1},
		},
		{
			name: "forbidden",
			env:  "release",
			answer: func(f *fakeGitHub) {
				f.handle("/repos/o/r/environments/release", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
				})
			},
			skipped:  1,
			requests: map[string]int{"/repos/o/r/environments/release": 2}, // the second one is without the token, see do
		},
		{
			// A name that is computed is not looked up
			name:     "expression",
			env:      "${{ github.event.pull_request.base.ref }}",
			answer:   func(f *fakeGitHub) {},
			requests: map[string]int{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			tc.answer(f)
			setOnlineEnv(t, f, "tok")
			t.Setenv("GITHUB_REPOSITORY", "o/r")
			stdout, skipped := environmentRun(t, f, tc.env)
			if got := strings.Contains(stdout, "[untrusted-checkout]"); got != tc.finding {
				t.Errorf("untrusted-checkout reported: %v, want %v\n%s", got, tc.finding, stdout)
			}
			if tc.finding && !strings.Contains(stdout, "no one has to approve the run") {
				t.Errorf("the message does not say that the environment is no gate:\n%s", stdout)
			}
			if skipped != tc.skipped {
				t.Errorf("skipped lookups: %v, want %v", skipped, tc.skipped)
			}
			for path, n := range tc.requests {
				if got := f.count(path); got != n {
					t.Errorf("%d requests for %s, want %d", got, path, n)
				}
			}
			if f.total() != sum(tc.requests) {
				t.Errorf("%d requests in all, want %d: %v", f.total(), sum(tc.requests), f.hits)
			}

			// The second run is answered by the cache: no request at all
			before := f.total()
			environmentRun(t, f, tc.env)
			if f.total() != before && tc.name != "forbidden" {
				t.Errorf("a second run made %d requests", f.total()-before)
			}
		})
	}
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// Without the online checks nothing is asked, and the environment is trusted.
func TestEnvironmentIsTrustedWithoutOnline(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/environments/release", environmentJSON(""))
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	_, wf := onlineProject(t, strings.ReplaceAll(environmentWorkflow, "ENVIRONMENT", "release"), environmentConfig)
	_, stdout, stderr := runOnlineCommand(t, wf)
	if strings.Contains(stdout, "[untrusted-checkout]") || f.total() != 0 {
		t.Errorf("%d requests, output:\n%s%s", f.total(), stdout, stderr)
	}
}

// The repository comes from the remote "origin" of the clone when GITHUB_REPOSITORY is not set.
func TestEnvironmentRepositoryFromTheRemote(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/acme/widgets/environments/release", environmentJSON(""))
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "")
	root, wf := onlineProject(t, strings.ReplaceAll(environmentWorkflow, "ENVIRONMENT", "release"), environmentConfig)
	cfg := "[remote \"origin\"]\n\turl = git@github.com:acme/widgets.git\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stdout, _ := runOnlineCommand(t, "--online", wf)
	if !strings.Contains(stdout, "[untrusted-checkout]") || f.count("/repos/acme/widgets/environments/release") != 1 {
		t.Errorf("%v\n%s", f.hits, stdout)
	}
}

// Without a known repository the environment is trusted and nothing is asked.
func TestEnvironmentWithoutRepositoryIsTrusted(t *testing.T) {
	f := newFakeGitHub(t)
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "")
	_, wf := onlineProject(t, strings.ReplaceAll(environmentWorkflow, "ENVIRONMENT", "release"), environmentConfig)
	_, stdout, _ := runOnlineCommand(t, "--online", wf)
	if strings.Contains(stdout, "[untrusted-checkout]") || f.total() != 0 {
		t.Errorf("%v\n%s", f.hits, stdout)
	}
}

// The environments of a public repository are shown to everybody, so an anonymous run works.
func TestHTTPClientEnvironment(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r/environments/RTD deploys", environmentJSON(`{"id":3,"type":"branch_policy"}`))
	f.json("/repos/o/r/environments/PyPI", environmentJSON(`{"id":3,"type":"branch_policy"},{"id":4,"type":"required_reviewers","reviewers":[]}`))
	c := f.client(httpGitHubOptions{})
	ctx := context.Background()

	env, err := c.Environment(ctx, "o", "r", "RTD deploys")
	if err != nil || env.RequiredReviewers {
		t.Errorf("RTD deploys: %+v, %v", env, err)
	}
	env, err = c.Environment(ctx, "o", "r", "PyPI")
	if err != nil || !env.RequiredReviewers {
		t.Errorf("PyPI: %+v, %v", env, err)
	}
	if _, err := c.Environment(ctx, "o", "r", "other"); !errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("missing: %v", err)
	}
	// A name which changes the path is not sent
	n := f.total()
	for _, name := range []string{"a/b", "..", ".", "", "a%2Fb", "a\nb", "${{ matrix.env }}"} {
		if _, err := c.Environment(ctx, "o", "r", name); !errors.Is(err, errInvalidGitHubName) {
			t.Errorf("%q: %v", name, err)
		}
	}
	if f.total() != n {
		t.Errorf("%d requests for invalid names", f.total()-n)
	}
}

// A client which cannot look up environments leaves them trusted without counting a skipped lookup.
func TestEnvironmentGateClientWithoutLookup(t *testing.T) {
	sess := newOnlineSession(context.Background(), &failingClient{err: errors.New("x")}, nil)
	if g, err := sess.EnvironmentGate("o", "r", "release"); g != gateUnknown || !errors.Is(err, errNoEnvironmentLookup) {
		t.Errorf("%v, %v", g, err)
	}
	if sess.skippedLookups() != 0 {
		t.Errorf("%d skipped lookups", sess.skippedLookups())
	}
}

// The fixtures record and replay the answers about environments.
func TestEnvironmentFixtureRoundTrip(t *testing.T) {
	f := newFakeGitHub(t)
	f.json("/repos/o/r", `{"private":false,"default_branch":"main"}`)
	f.json("/repos/o/r/environments/release", environmentJSON(`{"id":4,"type":"required_reviewers"}`))
	rec := NewRecordingGitHubClient(f.client(httpGitHubOptions{}))
	ctx := context.Background()
	if _, err := rec.Repository(ctx, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Environment(ctx, "o", "r", "release"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Environment(ctx, "o", "r", "gone"); !errors.Is(err, ErrGitHubNotFound) {
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
	if env, err := fx.Environment(ctx, "o", "r", "release"); err != nil || !env.RequiredReviewers {
		t.Errorf("%+v, %v", env, err)
	}
	if _, err := fx.Environment(ctx, "o", "r", "gone"); !errors.Is(err, ErrGitHubNotFound) {
		t.Error(err)
	}
	if _, err := fx.Environment(ctx, "o", "r", "unknown"); err == nil || errors.Is(err, ErrGitHubNotFound) {
		t.Errorf("a lookup which is not recorded must fail: %v", err)
	}
}

// A job which a condition guards is not looked up: nothing is asked when the environment would not decide.
func TestEnvironmentIsNotLookedUpForAGuardedAgent(t *testing.T) {
	f := newFakeGitHub(t)
	f.handle("/repos/o/r/environments/release", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"no"}`)
	})
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_REPOSITORY", "o/r")
	wf := `on: issue_comment
jobs:
  guarded:
    runs-on: ubuntu-latest
    environment: release
    if: github.event.comment.author_association == 'OWNER'
    steps:
      - uses: google-github-actions/run-gemini-cli@v0
        with:
          prompt: Answer the comment
`
	_, path := onlineProject(t, wf, environmentConfig+"  agentic-actions: error\n")
	report := filepath.Join(t.TempDir(), "report.json")
	_, stdout, stderr := runOnlineCommand(t, "--online", "--online-report", report, path)
	if f.count("/repos/o/r/environments/release") != 0 || strings.Contains(stdout, "[agentic-actions]") {
		t.Errorf("%v\n%s%s", f.hits, stdout, stderr)
	}
}

const optOutWorkflow = `on: pull_request_target
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: release
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
          allow-unsafe-pr-checkout: true
`

// The step which turns off the protection of actions/checkout goes through the same decision about the environment as
// the checkout which runs the code: the reviewers decide, and only an environment which is no gate brings the finding back.
func TestEnvironmentGateForTheOptOutOfTheCheckoutProtection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  func(f *fakeGitHub)
		offline bool
		finding bool
		skipped float64
	}{
		{"required reviewers", func(f *fakeGitHub) {
			f.json("/repos/o/r/environments/release", environmentJSON(`{"id":1,"type":"required_reviewers"}`))
		}, false, false, 0},
		{"no required reviewers", func(f *fakeGitHub) { f.json("/repos/o/r/environments/release", environmentJSON("")) }, false, true, 0},
		{"the lookup fails", func(f *fakeGitHub) {
			f.handle("/repos/o/r/environments/release", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"message":"no"}`)
			})
		}, false, false, 1},
		{"without --online", func(f *fakeGitHub) {}, true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			tc.answer(f)
			setOnlineEnv(t, f, "tok")
			t.Setenv("GITHUB_REPOSITORY", "o/r")
			_, wf := onlineProject(t, optOutWorkflow, environmentConfig)
			report := filepath.Join(t.TempDir(), "report.json")
			args := []string{"--online", "--online-report", report, wf}
			if tc.offline {
				args = []string{wf}
			}
			_, stdout, stderr := runOnlineCommand(t, args...)
			if got := strings.Contains(stdout, `"allow-unsafe-pr-checkout"`); got != tc.finding {
				t.Errorf("opt-out reported: %v, want %v\n%s%s", got, tc.finding, stdout, stderr)
			}
			if tc.finding && !strings.Contains(stdout, "no required reviewers") {
				t.Errorf("the message does not say that the environment is no gate:\n%s", stdout)
			}
			if tc.offline {
				if f.total() != 0 {
					t.Errorf("%d requests without --online", f.total())
				}
				return
			}
			b, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			var rep map[string]any
			if err := json.Unmarshal(b, &rep); err != nil {
				t.Fatal(err)
			}
			if rep["skippedLookups"] != tc.skipped {
				t.Errorf("skipped lookups %v, want %v", rep["skippedLookups"], tc.skipped)
			}
		})
	}
}
