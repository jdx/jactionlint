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

const (
	testSHAv4   = "11d5960a326750d5838078e36cf38b85af677262"
	testSHAv422 = "11bd71901bbe5b1630ceea73d27597364c9af683"
)

// githubForActionsCheckout makes the fake serve the data of actions/checkout the way the API does.
func githubForActionsCheckout(f *fakeGitHub, archived bool) {
	f.json("/repos/actions/checkout", fmt.Sprintf(`{"archived":%v,"default_branch":"main"}`, archived))
	f.json("/repos/actions/checkout/tags", fmt.Sprintf(`[{"name":"v4","commit":{"sha":%q}},{"name":"v4.2.2","commit":{"sha":%q}}]`, testSHAv4, testSHAv422))
	f.json("/advisories", `[]`)
	f.json("/repos/actions/checkout/git/ref/tags/v4", fmt.Sprintf(`{"object":{"type":"commit","sha":%q}}`, testSHAv4))
}

// onlineProject creates a repository with one workflow and returns the workflow path.
func onlineProject(t *testing.T, workflow string, config string) (root, wf string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	wf = filepath.Join(dir, "ci.yaml")
	if err := os.WriteFile(wf, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	// The workflows of the tests do not set timeout-minutes, which the default profile reports
	if config == "" {
		config = "rules:\n  missing-timeout: off\n"
	} else if strings.HasPrefix(config, "rules:\n") {
		config = "rules:\n  missing-timeout: off\n" + strings.TrimPrefix(config, "rules:\n")
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, wf
}

func runOnlineCommand(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status = cmd.Main(append([]string{"jactionlint", "-no-color", "-shellcheck=", "-pyflakes="}, args...))
	return status, out.String(), errOut.String()
}

func setOnlineEnv(t *testing.T, f *fakeGitHub, token string) {
	t.Helper()
	t.Setenv("GITHUB_API_URL", f.srv.URL)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", token)
	t.Setenv("GH_TOKEN", "")
}

func TestCommandNeverUsesTheNetworkWithoutOnline(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", "uses: actions/checkout@"+testSHAv422+" # v3"), "profile: all\n")
	status, stdout, stderr := runOnlineCommand(t, "-rule-ids", wf)
	if n := f.total(); n != 0 {
		t.Fatalf("%d requests were made without -online", n)
	}
	if strings.Contains(stdout+stderr, "archived") {
		t.Errorf("online findings without -online:\n%s%s", stdout, stderr)
	}
	_ = status
}

func TestCommandOnlineFindsAndCaches(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, true)
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@"+testSHAv422+" # v3"), "")

	status, stdout, stderr := runOnlineCommand(t, "-online", "-rule-ids", wf)
	if status != ExitStatusSuccessProblemFound && status != ExitStatusSuccessNoProblem {
		t.Fatalf("status %d\n%s%s", status, stdout, stderr)
	}
	for _, id := range []string{"[archived-uses]", "[ref-version-mismatch]"} {
		if !strings.Contains(stdout, id) {
			t.Errorf("%s is missing in\n%s", id, stdout)
		}
	}
	if stderr != "" {
		t.Errorf("a token is set, so nothing should be on stderr:\n%s", stderr)
	}
	first := f.total()
	if first == 0 {
		t.Fatal("-online made no request")
	}

	// A second run reads the cache (the default TTL is one hour)
	runOnlineCommand(t, "-online", "-rule-ids", wf)
	if f.total() != first {
		t.Errorf("a second run made %d requests", f.total()-first)
	}

	// With a TTL of zero everything is revalidated, and nothing changed (304)
	_, stdout2, _ := runOnlineCommand(t, "-online", "-online-cache-ttl=0", "-rule-ids", wf)
	if f.total() <= first {
		t.Error("-online-cache-ttl=0 should revalidate")
	}
	if !strings.Contains(stdout2, "[archived-uses]") {
		t.Errorf("the findings must be the same:\n%s", stdout2)
	}
	// The ETag revalidation uses If-None-Match, so the fake answered 304 without a body. The output is stable.
	if !strings.Contains(stdout2, "ref-version-mismatch") {
		t.Errorf("missing finding after revalidation:\n%s", stdout2)
	}
}

func TestCommandOnlineRateLimitedWarnsOnce(t *testing.T) {
	f := newFakeGitHub(t)
	limit := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(4102444800)) // 2100
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	}
	for _, p := range []string{"/repos/actions/checkout", "/repos/actions/checkout/tags", "/advisories"} {
		f.handle(p, limit)
	}
	setOnlineEnv(t, f, "tok")
	steps := []string{}
	for i := 0; i < 8; i++ {
		steps = append(steps, fmt.Sprintf("uses: actions/checkout@%040x", i+1))
	}
	_, wf := onlineProject(t, workflowWith(steps...), "")
	status, stdout, stderr := runOnlineCommand(t, "-online", wf)
	if status != ExitStatusSuccessNoProblem {
		t.Errorf("a rate limit is not a failure of the run (status %d)\n%s%s", status, stdout, stderr)
	}
	if n := strings.Count(stderr, "warning:"); n != 1 || !strings.Contains(stderr, "rate limit") {
		t.Errorf("want exactly one warning about the rate limit:\n%s", stderr)
	}
	if n := f.total(); n > maxGitHubInFlight+1 {
		t.Errorf("%d requests were made after the limit was reached", n)
	}
}

func TestCommandOnlineUnauthenticatedSaysSo(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	setOnlineEnv(t, f, "")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	_, _, stderr := runOnlineCommand(t, "-online", wf)
	if n := strings.Count(stderr, "unauthenticated"); n != 1 {
		t.Errorf("want one notice about the missing token:\n%s", stderr)
	}
	for _, a := range f.auth {
		if a != "" {
			t.Errorf("no Authorization header expected but got %q", a)
		}
	}
}

func TestCommandOnlineWarningsGoToStructuredOutput(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	setOnlineEnv(t, f, "")
	_, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4"), "")
	_, stdout, stderr := runOnlineCommand(t, "-online", "-format", "sarif", wf)
	if stderr != "" {
		t.Errorf("hk parses stdout and stderr together, so stderr must stay empty with -format sarif:\n%s", stderr)
	}
	if !strings.Contains(stdout, "unauthenticated") {
		t.Errorf("the notice should be in the SARIF notifications:\n%s", stdout)
	}
}

func TestCommandInvalidOnlineFlags(t *testing.T) {
	status, _, stderr := runOnlineCommand(t, "-online-cache-ttl=soon")
	if status != ExitStatusInvalidCommandOption {
		t.Errorf("status %d\n%s", status, stderr)
	}
}
