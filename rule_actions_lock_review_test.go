package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

const lockDep = "        ref: '%s'\n        commit: '%s'\n        owner_id: 44036562\n        repo_id: 178564337\n"

// A local action which several workflows run is judged for each of them: a caller which fails the lock check is not hidden
// by the one before it, and the message names the workflow. The version comment is one finding for the line.
func TestActionsLockEveryCallerOfALocalAction(t *testing.T) {
	const sha = "49933ea5288caeca8642d1e84afbd3f7d6820020"
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "actions", "setup", "action.yml"),
		"name: setup\ndescription: setup\nruns:\n  using: composite\n  steps:\n    - uses: actions/setup-node@"+sha+" # v4.0.0\n")
	for _, n := range []string{"a", "b", "c", "d"} {
		writeFile(t, filepath.Join(root, ".github", "workflows", n+".yml"), workflowRunningSetup("build"))
	}
	pin := func(name, key string) string {
		return "    '.github/workflows/" + name + ".yml':\n        - '" + key + "'\n"
	}
	lock := "version: 'v0.0.3'\nworkflows:\n" +
		pin("a", "actions/setup-node@"+sha) + // lists it
		pin("b", "actions/checkout@v4") + // the repository is not listed at all... another repository is
		pin("c", "actions/setup-node@v3") + // lists the repository at another ref
		pin("d", "actions/setup-node@"+sha) + // lists it too
		"dependencies:\n" +
		"    'actions/checkout@v4':\n" + strings.Replace(strings.Replace(lockDep, "%s", "v4", 1), "%s", "sha1-1111111111111111111111111111111111111111", 1) +
		"    'actions/setup-node@v3':\n" + strings.Replace(strings.Replace(lockDep, "%s", "v3", 1), "%s", "sha1-2222222222222222222222222222222222222222", 1) +
		"    'actions/setup-node@" + sha + "':\n" + strings.Replace(strings.Replace(lockDep, "%s", "v4.0.2", 1), "%s", "sha1-"+sha, 1)
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"), lock)

	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfigFixture(t, "rules:\n  actions-lock:\n    level: error\n    pedantic: true\n")
	errs, err := l.LintFile(filepath.Join(root, ".github", "actions", "setup", "action.yml"), &Project{root: root})
	if err != nil {
		t.Fatal(err)
	}
	var missing, other, comment int
	for _, e := range errs {
		switch {
		case strings.Contains(e.Message, "workflow \".github/workflows/b.yml\"") && strings.Contains(e.Message, "is not in"):
			missing++
		case strings.Contains(e.Message, "workflow \".github/workflows/c.yml\"") && strings.Contains(e.Message, "which pins"):
			other++
		case strings.Contains(e.Message, "version comment"):
			comment++
		}
	}
	if missing != 1 || other != 1 || comment != 1 || len(errs) != 3 {
		t.Errorf("want one finding for b (missing), one for c (other ref) and one for the comment: %v", errs)
	}
}

// A SHA-256 commit is a full commit hash too: a key which is one is not a mutable ref, and the identity of the repository
// is checked online for its pin.
func TestActionsLockSHA256(t *testing.T) {
	const h = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/setup-node@"+h+"\n")
	good := "    'actions/setup-node@" + h + "':\n        ref: 'v4.0.2'\n        commit: 'sha256-" + h + "'\n        owner_id: 44036562\n        repo_id: 1\n"
	bad := strings.Replace(good, "ref: 'v4.0.2'", "ref: 'v4.0.2'", 1)
	bad = strings.Replace(bad, "commit: 'sha256-"+h, "commit: 'sha256-"+strings.Repeat("b", 64), 1)
	writeFile(t, filepath.Join(root, ".github", "workflows", "actions.lock"),
		"version: 'v0.0.3'\nworkflows:\n    '.github/workflows/ci.yml':\n        - 'actions/setup-node@"+h+"'\ndependencies:\n"+bad)

	cfg := mustParseConfigFixture(t, "rules:\n  actions-lock: error\n  actions-lock-drift: error\n  impostor-commit: off\n  stale-action-refs: off\n  ref-version-mismatch: off\n  ref-confusion: off\n  archived-uses: off\n  known-vulnerable-actions: off\n")
	errs := lintLockRepository(t, root, cfg, LinterOptions{Online: true, GitHubClient: onlineFixtureClient(t), LogWriter: io.Discard})
	var mismatch, replaced int
	for _, e := range errs {
		switch {
		case strings.Contains(e.Message, "is another commit"):
			mismatch++
		case strings.Contains(e.Message, "not the repository which was locked"):
			replaced++
		default:
			t.Errorf("unexpected: %v", e)
		}
	}
	if mismatch != 1 || replaced != 1 {
		t.Errorf("want a key/commit mismatch and a replaced repository, got %v", errs)
	}
}
