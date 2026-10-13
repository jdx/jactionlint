//go:build !js

package jactionlint

import (
	"net/http"
	"strings"
	"testing"
)

// onlyMetadataConfig keeps the other online rules from asking about the repositories of the test.
const onlyMetadataConfig = "rules:\n  archived-uses: off\n  known-vulnerable-actions: off\n  ref-confusion: off\n  stale-action-refs: off\n  impostor-commit: off\n  ref-version-mismatch: off\n"

const remoteTestSHA = "0123456789abcdef0123456789abcdef01234567"

func githubWithAction(f *fakeGitHub, repo string) {
	f.json("/repos/"+repo+"/contents/action.yml", contentsJSON(remoteComposite))
}

func TestCommandRemoteActionIsCheckedAndCached(t *testing.T) {
	f := newFakeGitHub(t)
	githubWithAction(f, "corp/tool")
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith(
		"uses: corp/tool@v1\n        with:\n          typo: x",
		"uses: corp/tool@v1\n        with:\n          name: x",
		"uses: corp/tool@"+remoteTestSHA+" # v1.0.0\n        with:\n          name: x\n          typo: y",
	), onlyMetadataConfig)

	_, stdout, stderr := runOnlineCommand(t, "--online", wf)
	for _, want := range []string{"[missing-action-input]", "[unknown-action-input]", `input "typo" is not defined in action "corp/tool@v1"`, `"corp/tool@` + remoteTestSHA + `"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("%q is missing in\n%s", want, stdout)
		}
	}
	if strings.Count(stdout, "[unknown-action-input]") != 2 {
		t.Errorf("want one finding for each use:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("a token is set and everything was found, so nothing should be on stderr:\n%s", stderr)
	}
	// One request for the tag and one for the commit: the same file is not asked for twice
	if n := f.count("/repos/corp/tool/contents/action.yml"); n != 2 {
		t.Errorf("%d requests for the metadata, want 2 (one for each ref)", n)
	}
	first := f.total()

	// The second run is answered by the cache
	_, again, _ := runOnlineCommand(t, "--online", wf)
	if f.total() != first || again != stdout {
		t.Errorf("a second run made %d requests, and the output changed:\n%s", f.total()-first, again)
	}

	// The cache mode needs no network and gives the same findings
	_, cached, _ := runOnlineCommand(t, "--online=cache", wf)
	if f.total() != first || cached != stdout {
		t.Errorf("--online=cache made %d requests:\n%s", f.total()-first, cached)
	}

	// With a TTL of zero every answer is checked again, the commit too (a 304 costs no quota)
	runOnlineCommand(t, "--online", "--online-cache-ttl=0", wf)
	if n := f.count("/repos/corp/tool/contents/action.yml"); n != 4 {
		t.Errorf("--online-cache-ttl=0 revalidates both refs: %d requests", n)
	}
}

func TestCommandRemoteActionWithoutOnlineNeverAsks(t *testing.T) {
	f := newFakeGitHub(t)
	githubWithAction(f, "corp/tool")
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: corp/tool@v1\n        with:\n          typo: x"), onlyMetadataConfig)
	_, stdout, _ := runOnlineCommand(t, wf)
	if f.total() != 0 || strings.Contains(stdout, "typo") {
		t.Errorf("%d requests, output:\n%s", f.total(), stdout)
	}
}

func TestCommandRemoteActionOfflineModeNeedsTheCache(t *testing.T) {
	f := newFakeGitHub(t)
	githubWithAction(f, "corp/tool")
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: corp/tool@v1\n        with:\n          typo: x"), onlyMetadataConfig)
	// Nothing cached: no finding, a skipped lookup, and strict fails on it
	status, stdout, stderr := runOnlineCommand(t, "--online=cache,strict", wf)
	if f.total() != 0 || strings.Contains(stdout, "typo") || status != ExitStatusFailure || !strings.Contains(stderr, "run once with --online") {
		t.Errorf("status %d, %d requests\n%s%s", status, f.total(), stdout, stderr)
	}
}

// An action which cannot be read is not an error of the run: one warning, no finding, an incomplete coverage
// which strict turns into a failure, and a deny list which leaves it alone.
func TestCommandRemoteActionWhichCannotBeReadIsSkipped(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"not found": nil,
		"forbidden": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) },
		"invalid": func(w http.ResponseWriter, r *http.Request) {
			f := contentsJSON("inputs: [a]\n")
			_, _ = w.Write([]byte(f))
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeGitHub(t)
			if handler != nil {
				f.handle("/repos/corp/tool/contents/action.yml", handler)
			}
			setOnlineEnv(t, f, "tok")
			_, wf := onlineProject(t, workflowWith("uses: corp/tool@v1\n        with:\n          typo: x", "run: echo hi"), onlyMetadataConfig)
			status, stdout, stderr := runOnlineCommand(t, "--online", wf)
			if status != ExitStatusSuccessNoProblem || stdout != "" || strings.Count(stderr, "warning:") != 1 || !strings.Contains(stderr, "corp/tool") {
				t.Errorf("status %d\n%s%s", status, stdout, stderr)
			}
			rep, _, status := runWithReport(t, wf, "--online")
			if rep["status"] != "incomplete" || rep["skippedLookups"] != float64(1) || status != ExitStatusSuccessNoProblem {
				t.Errorf("coverage: status %d %v", status, rep)
			}
			if status, _, _ := runOnlineCommand(t, "--online=strict", wf); status != ExitStatusFailure {
				t.Errorf("strict must fail on a skipped lookup, got %d", status)
			}
			before := f.total()
			status, _, stderr = runOnlineCommand(t, "--online=strict", "--online-deny", "corp/*", wf)
			if status != ExitStatusSuccessNoProblem || stderr != "" || f.total() != before {
				t.Errorf("a denied repository is not asked and is not a failure: status %d, %d new requests\n%s", status, f.total()-before, stderr)
			}
		})
	}
}

// The coverage of the run counts the lookups of the metadata with the other answers.
func TestCommandRemoteActionIsInTheCoverageOfTheRun(t *testing.T) {
	f := newFakeGitHub(t)
	githubWithAction(f, "corp/tool")
	setOnlineEnv(t, f, "tok")
	_, wf := onlineProject(t, workflowWith("uses: corp/tool@"+remoteTestSHA+" # v1\n        with:\n          name: x"), onlyMetadataConfig)
	rep, _, _ := runWithReport(t, wf, "--online")
	ans, _ := rep["answers"].(map[string]any)
	// The token is for a repository which is not known to be public yet, so the answer is followed by one for the repository
	if rep["status"] != "complete" || ans["network"].(float64) < 1 || ans["immutable"] != nil {
		t.Errorf("first run: %v", rep)
	}
	rep, _, _ = runWithReport(t, wf, "--online")
	ans, _ = rep["answers"].(map[string]any)
	if ans["cache"].(float64) < 1 || ans["network"] != float64(0) {
		t.Errorf("the second run is within the TTL: %v", rep)
	}
}
