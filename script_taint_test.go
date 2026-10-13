package jactionlint

import (
	"strings"
	"testing"
)

func countScriptTaint(t *testing.T, id, src string) int {
	t.Helper()
	cfg := &Config{Rules: map[string]RuleConfig{
		"github-env": {Level: SeverityError}, "command-injection": {Level: SeverityError}, "output-clobbering": {Level: SeverityError},
	}}
	n := 0
	for _, e := range lintBatchD(t, "test.yaml", src, cfg) {
		if e.ID == id {
			n++
		}
	}
	return n
}

// TestScriptTaintSources: the output of gh and git that holds text of outsiders reaches the same sinks as an input of
// the event. rule is the rule that is asked, and the script has the sink after the line that is under test.
func TestScriptTaintSources(t *testing.T) {
	const head = "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const checkout = "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.head_ref }}\n"
	const env = `echo "V=$V" >> "$GITHUB_ENV"`
	tests := []struct {
		what, pre, script string
		rule              string
		want              int
	}{
		// --- gh ---
		{"the title of a pull request", "", "V=$(gh pr view \"$N\" --json title --jq .title)\n" + env, "github-env", 1},
		{"the title with -q and a template", "", "V=$(gh pr view \"$N\" --json title -q '.title')\n" + env, "github-env", 1},
		{"in the write", "", `echo "V=$(gh pr view "$N" --json title --jq .title)" >> "$GITHUB_ENV"`, "github-env", 1},
		{"the branch of a pull request", "", "V=$(gh pr view \"$N\" --json headRefName --jq .headRefName)\n" + env, "github-env", 1},
		{"the body of an issue", "", "V=$(gh issue view \"$N\" --json title,body --jq .body)\n" + env, "github-env", 1},
		{"the comments", "", "V=$(gh pr view \"$N\" --json comments --jq '.comments[].body')\n" + env, "github-env", 1},
		{"the files", "", "V=$(gh pr view \"$N\" --json files --jq '.files[].path')\n" + env, "github-env", 1},
		{"the author", "", "V=$(gh pr view \"$N\" --json author --jq .author.login)\n" + env, "github-env", 1},
		{"without a filter", "", "V=$(gh pr view \"$N\" --json title,body)\n" + env, "github-env", 1},
		{"the api, a title", "", "V=$(gh api \"repos/$R/pulls/$N\" --jq .title)\n" + env, "github-env", 1},
		{"the api, the head refs of the pulls of a commit, the first line", "", "V=$(gh api -H 'Accept: application/vnd.github+json' \"/repos/$R/commits/$S/pulls\" --jq '.[].head.ref' | head -n 1)\n" + env, "github-env", 0}, // one line is fine for the environment ...
		{"the same, evaluated", "", "V=$(gh api -H 'Accept: application/vnd.github+json' \"/repos/$R/commits/$S/pulls\" --jq '.[].head.ref' | head -n 1)\neval \"git checkout $V\"", "command-injection", 1},                    // ... but not for a shell
		{"the api, the head ref of a pull request", "", "V=$(gh api \"repos/$R/pulls/$N\" --jq .head.ref)\n" + env, "github-env", 1},
		{"the api, the body of an issue", "", "V=$(gh api repos/$R/issues/$N --jq .body)\n" + env, "github-env", 1},
		{"the api, the comments", "", "V=$(gh api repos/$R/issues/$N/comments --jq '.[].body')\n" + env, "github-env", 1},
		{"the api, the whole object", "", "V=$(gh api repos/$R/pulls/$N)\n" + env, "github-env", 1},
		{"a variable that holds it, read by another command", "", "J=$(gh pr view \"$N\" --json title --jq .title)\nV=\"x-$J\"\n" + env, "github-env", 1},
		// --- not a source ---
		{"a number", "", "V=$(gh pr view \"$N\" --json number --jq .number)\n" + env, "github-env", 0},
		{"the state", "", "V=$(gh pr view \"$N\" --json state,mergeable -q .state)\n" + env, "github-env", 0},
		{"a filter that selects a number out of the text fields", "", "V=$(gh pr view \"$N\" --json title,body --jq .number)\n" + env, "github-env", 0},
		{"the api, a number", "", "V=$(gh api repos/$R/pulls/$N --jq .number)\n" + env, "github-env", 0},
		{"the api, the runs", "", "V=$(gh api repos/$R/actions/runs --jq '.workflow_runs[0].name')\n" + env, "github-env", 0},
		{"a release", "", "V=$(gh release view --json tagName --jq .tagName)\n" + env, "github-env", 0},
		{"the repository", "", "V=$(gh repo view --json name --jq .name)\n" + env, "github-env", 0},
		{"cut down to a set", "", "V=$(gh pr view \"$N\" --json title --jq .title | tr -cd 'a-z0-9-')\n" + env, "github-env", 0},
		{"cut down to one line", "", "V=$(gh pr view \"$N\" --json title --jq .title | tr -d '\\n')\n" + env, "github-env", 0},
		{"printed only", "", "V=$(gh pr view \"$N\" --json title --jq .title)\necho \"$V\"", "github-env", 0},
		{"a checked value", "", "V=$(gh pr view \"$N\" --json title --jq .title)\n[[ \"$V\" =~ ^[a-z]+$ ]] || exit 1\n" + env, "github-env", 0},
		// --- git, once the pull request is checked out ---
		{"the subject of the commit", checkout, "V=$(git log -1 --format=%s)\n" + env, "github-env", 1},
		{"the body of the commit", checkout, "V=$(git log -1 --pretty=%B)\n" + env, "github-env", 1},
		{"the author of the commit", checkout, "V=$(git log -1 --pretty=format:'%an')\n" + env, "github-env", 1},
		{"show", checkout, "V=$(git show -s --format=%B)\n" + env, "github-env", 1},
		{"the changed files", checkout, "V=$(git diff --name-only HEAD~1)\n" + env, "github-env", 1},
		{"diff-tree", checkout, "V=$(git diff-tree --no-commit-id --name-only HEAD -r)\n" + env, "github-env", 1},
		{"the branch", checkout, "V=$(git branch --show-current)\n" + env, "github-env", 1},
		{"the abbreviated ref", checkout, "V=$(git rev-parse --abbrev-ref HEAD)\n" + env, "github-env", 1},
		{"after gh pr checkout in the script", "", "gh pr checkout \"$N\"\nV=$(git log -1 --format=%s)\n" + env, "github-env", 1},
		{"after a fetch of the pull request", "", "git fetch origin refs/pull/$N/head:pr\nV=$(git log -1 --format=%s pr)\n" + env, "github-env", 1},
		{"after gh pr checkout in an earlier step", "      - run: gh pr checkout \"$N\"\n", "V=$(git log -1 --format=%s)\n" + env, "github-env", 1},
		{"without a checkout of the pull request", "", "V=$(git log -1 --format=%s)\n" + env, "github-env", 0},
		{"a checkout of the repository", "      - uses: actions/checkout@v4\n", "V=$(git log -1 --format=%s)\n" + env, "github-env", 0},
		{"the sha is not free text", checkout, "V=$(git rev-parse HEAD)\n" + env, "github-env", 0},
		{"the commit date", checkout, "V=$(git log -1 --format=%cd)\n" + env, "github-env", 0},
		{"the checkout comes later", "", "V=$(git log -1 --format=%s)\n" + env, "github-env", 0},
		// --- the sinks of the other rules ---
		{"eval of a branch", "", "V=$(gh pr view \"$N\" --json headRefName --jq .headRefName)\neval \"git checkout $V\"", "command-injection", 1},
		{"bash -c of a title", "", "V=$(gh pr view \"$N\" --json title --jq .title)\nbash -c \"echo $V\"", "command-injection", 1},
		{"a quoted argument is fine", "", "V=$(gh pr view \"$N\" --json title --jq .title)\ngit checkout \"$V\"", "command-injection", 0},
		{"a number is fine", "", "V=$(gh pr view \"$N\" --json number --jq .number)\neval \"echo $V\"", "command-injection", 0},
		{"eval of the commit message", checkout, "V=$(git log -1 --format=%B)\neval \"echo $V\"", "command-injection", 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head + tc.pre
			src += "      - env:\n          N: ${{ github.event.issue.number }}\n          R: ${{ github.repository }}\n          S: ${{ github.sha }}\n        run: |\n"
			for _, l := range strings.Split(tc.script, "\n") {
				src += "          " + l + "\n"
			}
			if got := countScriptTaint(t, tc.rule, src); got != tc.want {
				t.Errorf("%d findings of %s, want %d\n%s", got, tc.rule, tc.want, src)
			}
		})
	}
}

// TestScriptTaintOutputClobbering: only text that can hold several lines adds a record to $GITHUB_OUTPUT.
func TestScriptTaintOutputClobbering(t *testing.T) {
	mk := func(script string) string {
		src := "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - id: gate\n        env:\n          N: ${{ github.event.issue.number }}\n        run: |\n"
		for _, l := range strings.Split(script, "\n") {
			src += "          " + l + "\n"
		}
		return src + "      - if: steps.gate.outputs.approved == 'true'\n        run: echo x\n"
	}
	const out = `>> "$GITHUB_OUTPUT"`
	tests := []struct {
		what, script string
		want         int
	}{
		{"the body of a pull request", "V=$(gh pr view \"$N\" --json body --jq .body)\necho \"body=$V\" " + out, 1},
		{"the comments", "V=$(gh pr view \"$N\" --json comments --jq '.comments[].body')\necho \"c=$V\" " + out, 1},
		{"the files", "V=$(gh pr view \"$N\" --json files --jq '.files[].path')\necho \"f=$V\" " + out, 1},
		{"the title is one line", "V=$(gh pr view \"$N\" --json title --jq .title)\necho \"t=$V\" " + out, 0},
		{"the branch is one line", "V=$(gh pr view \"$N\" --json headRefName --jq .headRefName)\necho \"b=$V\" " + out, 0},
		{"the first line of the body", "V=$(gh pr view \"$N\" --json body --jq .body | head -n 1)\necho \"b=$V\" " + out, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := mk(tc.script)
			if got := countScriptTaint(t, "output-clobbering", src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}
