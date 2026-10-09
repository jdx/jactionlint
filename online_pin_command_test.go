//go:build !js

package jactionlint

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandOnlineFixPinsTags(t *testing.T) {
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	f.handle("/repos/actions/checkout/git/ref/heads/v4", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	setOnlineEnv(t, f, "tok")
	root, wf := onlineProject(t, workflowWith("uses: actions/checkout@v4", `uses: "actions/checkout@v4"   `, "uses: actions/checkout@v4 # v4"), "rules:\n  unpinned-uses: error\n")

	// Without --online nothing can be resolved: the error has no fix
	status, _, _ := runOnlineCommand(t, "--fix", wf)
	if status != ExitStatusSuccessProblemFound {
		t.Errorf("offline --fix leaves the unpinned actions (status %d)", status)
	}
	if b, _ := os.ReadFile(wf); !strings.Contains(string(b), "@v4\n") {
		t.Errorf("offline --fix must not change the file:\n%s", b)
	}

	status, stdout, stderr := runOnlineCommand(t, "--online", "--fix", wf)
	if status != ExitStatusSuccessNoProblem {
		t.Errorf("status %d\n%s%s", status, stdout, stderr)
	}
	b, _ := os.ReadFile(wf)
	want := workflowWith(
		"uses: actions/checkout@"+testSHAv4+" # v4",
		`uses: "actions/checkout@`+testSHAv4+`" # v4`,
		"uses: actions/checkout@"+testSHAv4+" # v4",
	)
	if string(b) != want {
		t.Errorf("unexpected result:\n%s\nwant:\n%s", b, want)
	}

	// Running it again changes nothing and the workflow lints clean, online too
	status, stdout, _ = runOnlineCommand(t, "--online", "--fix", wf)
	if status != ExitStatusSuccessNoProblem {
		t.Errorf("second run: status %d\n%s", status, stdout)
	}
	if b2, _ := os.ReadFile(wf); !bytes.Equal(b, b2) {
		t.Error("--fix is not idempotent")
	}
	status, stdout, _ = runOnlineCommand(t, "--online", "--config-file", filepath.Join(root, ".github", "jactionlint.yaml"), wf)
	if status != ExitStatusSuccessNoProblem {
		t.Errorf("lint after the fix: status %d\n%s", status, stdout)
	}
}
