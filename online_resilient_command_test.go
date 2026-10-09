//go:build !js

package jactionlint

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// githubFailingFor makes the fake answer every path of a repository with the status.
func githubFailingFor(f *fakeGitHub, repo string, status int) {
	h := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, `{"message":"nope"}`)
	}
	for _, p := range []string{"/repos/" + repo, "/repos/" + repo + "/tags"} {
		f.handle(p, h)
	}
}

func TestCommandOneFailingActionDoesNotHideTheOthers(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newFakeGitHub(t)
			githubForActionsCheckout(f, true)
			githubFailingFor(f, "corp/private", status)
			setOnlineEnv(t, f, "tok")
			_, wf := onlineProject(t, workflowWith("uses: corp/private@v1", "uses: actions/checkout@v4", "uses: corp/private@v2"), "")
			got, stdout, stderr := runOnlineCommand(t, "--online", "--rule-ids", wf)
			if got != ExitStatusSuccessNoProblem {
				t.Errorf("a failed lookup does not change the exit status: %d\n%s%s", got, stdout, stderr)
			}
			if !strings.Contains(stdout, "[archived-uses]") {
				t.Errorf("the finding for the other action is missing:\n%s", stdout)
			}
			if n := strings.Count(stderr, "warning:"); n != 1 || !strings.Contains(stderr, "--verbose") {
				t.Errorf("want exactly one warning:\n%s", stderr)
			}
			if strings.Contains(stderr, "tok") && strings.Contains(stderr, "Bearer") {
				t.Errorf("token in the output:\n%s", stderr)
			}
		})
	}
}

func TestCommandVerboseListsEverySkippedLookup(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	githubFailingFor(f, "corp/one", http.StatusForbidden)
	githubFailingFor(f, "corp/two", http.StatusForbidden)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: corp/one@v1", "uses: corp/two@v1"), "")
	_, _, stderr := runOnlineCommand(t, "--online", "--verbose", wf)
	for _, want := range []string{"skipped corp/one", "skipped corp/two"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("--verbose should say %q:\n%s", want, stderr)
		}
	}
	if n := strings.Count(stderr, "warning:"); n != 1 {
		t.Errorf("still one warning:\n%s", stderr)
	}
}

func TestCommandStrictModeFailsWhenALookupIsSkipped(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	githubFailingFor(f, "corp/private", http.StatusInternalServerError)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")
	status, _, stderr := runOnlineCommand(t, "--online=strict", wf)
	if status != ExitStatusFailure || !strings.Contains(stderr, "online=strict: ") {
		t.Errorf("status %d\n%s", status, stderr)
	}

	// Without the failing action strict passes
	_, wf = onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	status, stdout, stderr := runOnlineCommand(t, "--online=strict", wf)
	if status != ExitStatusSuccessNoProblem {
		t.Errorf("status %d\n%s%s", status, stdout, stderr)
	}
	// Excluded lookups are not failures
	_, wf = onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")
	status, stdout, stderr = runOnlineCommand(t, "--online=strict", "--online-deny=corp/*", wf)
	if status != ExitStatusSuccessNoProblem || stderr != "" {
		t.Errorf("status %d\n%s%s", status, stdout, stderr)
	}
}

func TestCommandAllowAndDenyListsKeepRequestsAway(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")
	_, stdout, stderr := runOnlineCommand(t, "--online", "--rule-ids", "--online-deny", "actions/checkout", wf)
	if f.count("/repos/actions/checkout") != 0 || f.count("/repos/corp/private") == 0 || strings.Contains(stdout, "archived") {
		// corp/private is not in the fake: it is a 404. actions/checkout must not be asked
		t.Errorf("%d requests, stdout:\n%s%s", f.total(), stdout, stderr)
	}

	f2 := newFakeGitHub(t)
	githubForActionsCheckout(f2, true)
	setOnlineEnv(t, f2, "tok")
	_, wf = onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")
	_, _, stderr = runOnlineCommand(t, "--online", "--online-allow", "actions/*", wf)
	if f2.count("/repos/corp/private") != 0 || stderr != "" {
		t.Errorf("corp/private is outside the allow list: %d requests\n%s", f2.count("/repos/corp/private"), stderr)
	}

	// The same from the configuration file
	f3 := newFakeGitHub(t)
	githubForActionsCheckout(f3, true)
	setOnlineEnv(t, f3, "tok")
	_, wf = onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "online: true\nonline-options:\n  deny: [\"corp/*\"]\n")
	runOnlineCommand(t, wf)
	if f3.count("/repos/corp/private") != 0 || f3.count("/repos/actions/checkout") == 0 {
		t.Errorf("the config file deny list: %d requests for corp/private", f3.count("/repos/corp/private"))
	}
}

func TestCommandCacheModeNeverUsesTheNetwork(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@"+testSHAv422+" # v3"), "")
	_, warm, _ := runOnlineCommand(t, "--online", "--rule-ids", wf)
	if !strings.Contains(warm, "[archived-uses]") {
		t.Fatalf("setup:\n%s", warm)
	}
	n := f.total()

	// Even with nothing fresh (TTL 0) and a dead server the cache answers
	f.srv.Close()
	_, stdout, stderr := runOnlineCommand(t, "--online=cache", "--online-cache-ttl=0", "--rule-ids", wf)
	if stdout != warm {
		t.Errorf("the cache gave other findings:\n%s\nwant\n%s", stdout, warm)
	}
	if stderr != "" {
		t.Errorf("a cache hit is not worth a warning:\n%s", stderr)
	}
	if f.total() != n {
		t.Errorf("--online=cache made %d requests", f.total()-n)
	}
}

func TestCommandCacheModeReportsWhatIsNotCached(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	status, stdout, stderr := runOnlineCommand(t, "--online=cache", wf)
	if status != ExitStatusSuccessNoProblem || f.total() != 0 {
		t.Errorf("status %d, %d requests\n%s%s", status, f.total(), stdout, stderr)
	}
	if n := strings.Count(stderr, "warning:"); n != 1 || !strings.Contains(stderr, "no cached answer") || !strings.Contains(stderr, "--online") {
		t.Errorf("want one warning that says to fill the cache:\n%s", stderr)
	}
	status, _, _ = runOnlineCommand(t, "--online=strict", "--online=cache", wf) // The last wins
	if status != ExitStatusSuccessNoProblem {
		t.Errorf("the last --online wins, and cache mode does not fail: %d", status)
	}
	status, _, stderr = runOnlineCommand(t, "--online=cache,strict", wf)
	if status != ExitStatusFailure || f.total() != 0 || !strings.Contains(stderr, "online=strict") {
		t.Errorf("cache,strict fails on what is not cached, without the network: status %d, %d requests\n%s", status, f.total(), stderr)
	}
	_, wf = onlineProject(t, workflowWith("uses: actions/checkout@v4"), "online-options:\n  mode: cache\n")
	status, _, stderr = runOnlineCommand(t, wf)
	if f.total() != 0 || !strings.Contains(stderr, "no cached answer") {
		t.Errorf("online-options.mode: cache turns the checks on, offline: status %d, %d requests\n%s", status, f.total(), stderr)
	}
}

func TestCommandOnlineFlagSpellings(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	runOnlineCommand(t, "--no-online", wf)
	if f.total() != 0 {
		t.Error("--no-online must not use the network")
	}
	for _, flag := range []string{"--online", "--online=true", "--online=strict"} {
		f2 := newFakeGitHub(t)
		githubForActionsCheckout(f2, true)
		setOnlineEnv(t, f2, "tok")
		runOnlineCommand(t, flag, wf)
		if f2.total() == 0 {
			t.Errorf("%s did not enable the online checks", flag)
		}
	}
	status, _, stderr := runOnlineCommand(t, "--online=sometimes", wf)
	if status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "cache") {
		t.Errorf("status %d\n%s", status, stderr)
	}
	status, _, stderr = runOnlineCommand(t, "--online", "--online-deny=nonsense", wf)
	if status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "owner/repo") {
		t.Errorf("status %d\n%s", status, stderr)
	}
}

func TestCommandTokenSources(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	setOnlineEnv(t, f, "env-token-value")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	tokFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokFile, []byte("file-token-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MY_PAT", "named-token-value")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "Bearer env-token-value"},
		{[]string{"--online-token-file", tokFile}, "Bearer file-token-value"},
		{[]string{"--online-token-env", "MY_PAT"}, "Bearer named-token-value"},
	} {
		f.mu.Lock()
		f.auth = nil
		f.mu.Unlock()
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		_, _, stderr := runOnlineCommand(t, append([]string{"--online", "--verbose"}, append(tc.args, wf)...)...)
		if len(f.auth) == 0 || f.auth[0] != tc.want {
			t.Errorf("%v: Authorization %q. want %q", tc.args, f.auth, tc.want)
		}
		if strings.Contains(stderr, "token-value") {
			t.Errorf("%v: the token is in the output:\n%s", tc.args, stderr)
		}
		if !strings.Contains(stderr, "using the token from") {
			t.Errorf("--verbose should say where the token came from:\n%s", stderr)
		}
	}
}

func TestCommandAPIURLOfTheRepositoryConfigGetsNoToken(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	setOnlineEnv(t, f, "env-token-value")
	t.Setenv("GITHUB_API_URL", "") // The host comes from the config file only
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "online: true\nonline-options:\n  api-url: "+f.srv.URL+"\n")
	_, _, stderr := runOnlineCommand(t, wf)
	if f.total() == 0 {
		t.Fatal("the config file URL should be used")
	}
	for _, a := range f.auth {
		if a != "" {
			t.Errorf("the token was sent to a host a repository chose: %q", a)
		}
	}
	if !strings.Contains(stderr, "config file of the repository") {
		t.Errorf("the user should be told:\n%s", stderr)
	}

	// The same host on the command line is the user's choice
	f.mu.Lock()
	f.auth = nil
	f.mu.Unlock()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	runOnlineCommand(t, "--online-api-url", f.srv.URL, wf)
	if len(f.auth) == 0 || f.auth[0] != "Bearer env-token-value" {
		t.Errorf("--online-api-url is trusted: %q", f.auth)
	}
}

func TestCommandHostFromServerURL(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	setOnlineEnv(t, f, "tok")
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("GITHUB_SERVER_URL", f.srv.URL) // http://127.0.0.1:port: the API is at /api/v3
	f.json("/api/v3/repos/actions/checkout", `{"default_branch":"main"}`)
	f.json("/api/v3/repos/actions/checkout/tags", `[]`)
	f.json("/api/v3/advisories", `[]`)
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	runOnlineCommand(t, "--online", wf)
	if f.count("/api/v3/repos/actions/checkout") == 0 {
		t.Errorf("GITHUB_SERVER_URL should select the Enterprise Server API: %v", f.hits)
	}
}

// A bare --online on the command line goes to the network although the configuration pins the cache mode.
func TestCommandOnlineOverridesTheModeOfTheConfig(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@"+testSHAv422+" # v3"), "online-options:\n  mode: cache\n")
	_, stdout, _ := runOnlineCommand(t, "--rule-ids", wf)
	if f.total() != 0 {
		t.Fatalf("the mode of the config is cache, so nothing is asked: %v", f.hits)
	}
	_ = stdout
	_, stdout, _ = runOnlineCommand(t, "--online", "--rule-ids", wf)
	if f.total() == 0 || !strings.Contains(stdout, "[archived-uses]") {
		t.Fatalf("--online must ask GitHub:\n%s", stdout)
	}
}

// --diff must not hide that a lookup of --online=strict was skipped behind the exit status of the diff.
func TestCommandDiffStillFailsWhenStrictSkippedALookup(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	githubFailingFor(f, "corp/private", http.StatusInternalServerError)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")
	var out, errOut bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut, onRulesCreated: newEditRule("ubuntu", "ubuntu-latest", "ubuntu-24.04")}
	status := cmd.Main([]string{"jactionlint", "--no-color", "--shellcheck=", "--pyflakes=", "--diff", "--online=strict", wf})
	if !strings.Contains(out.String(), "+++ ") {
		t.Fatalf("setup: the fix must produce a diff:\n%s\n%s", out.String(), errOut.String())
	}
	if status != ExitStatusFailure || !strings.Contains(errOut.String(), "online=strict: ") {
		t.Errorf("status %d\n%s", status, errOut.String())
	}
}

// --no-online turns the online checks off although the configuration turns them on.
func TestCommandOnlineFalseOverridesTheConfig(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@"+testSHAv422+" # v3"), "online: true\n")
	if _, stdout, _ := runOnlineCommand(t, "--rule-ids", wf); f.total() == 0 || !strings.Contains(stdout, "[archived-uses]") {
		t.Fatalf("setup: the config turns the checks on: %v\n%s", f.hits, stdout)
	}
	n := f.total()
	_, stdout, _ := runOnlineCommand(t, "--no-online", "--rule-ids", wf)
	if f.total() != n || strings.Contains(stdout, "[archived-uses]") {
		t.Errorf("--no-online must keep the network away: %d requests\n%s", f.total()-n, stdout)
	}
}

// A refused fix must not hide that a lookup of --online=strict was skipped: the note is printed and the
// exit status stays the failure of the fix.
func TestCommandStrictNoteIsPrintedWhenAFixIsRefused(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	githubFailingFor(f, "corp/private", http.StatusInternalServerError)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: corp/private@v1"), "")
	var out, errOut bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut, onRulesCreated: newEditRule("bad-rule", "ubuntu-latest", "[ubuntu-latest")}
	status := cmd.Main([]string{"jactionlint", "--no-color", "--shellcheck=", "--pyflakes=", "--fix", "--online=strict", wf})
	if status != ExitStatusFailure || !strings.Contains(errOut.String(), "bad-rule") {
		t.Fatalf("setup: the fix must be refused: %d\n%s", status, errOut.String())
	}
	if !strings.Contains(errOut.String(), "online=strict: ") {
		t.Errorf("the skipped lookup must be reported:\n%s", errOut.String())
	}
}
