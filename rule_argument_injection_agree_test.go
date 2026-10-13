package jactionlint

import "testing"

// TestArgumentInjectionNamesAgree: what counts as the name of a branch, of a label or of an account is the same for an
// expression of the workflow and for the output of a command of the script. The display name of an author or of a
// committer is free text on both sides.
func TestArgumentInjectionNamesAgree(t *testing.T) {
	const head = "on: issue_comment\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const checkout = "      - uses: actions/checkout@v4\n        with:\n          ref: ${{ github.event.pull_request.head.sha || github.head_ref }}\n"
	const sed = "          sed \"s/x/$V/\" file\n"
	exprStep := func(expr string) string {
		return "      - env:\n          V: ${{ " + expr + " }}\n        run: |\n" + sed
	}
	scriptStep := func(cmd string) string {
		return checkout + "      - env:\n          N: ${{ github.event.issue.number }}\n          GH_TOKEN: ${{ github.token }}\n        run: |\n          V=$(" + cmd + ")\n" + sed
	}
	tests := []struct {
		what, steps string
		want        int
	}{
		// an expression of the workflow
		{"expression: the name of the head branch", exprStep("github.head_ref"), 0},
		{"expression: the login of the author", exprStep("github.event.pull_request.user.login"), 0},
		{"expression: the name of a label", exprStep("github.event.label.name"), 0},
		{"expression: the name of the author of the head commit", exprStep("github.event.head_commit.author.name"), 1},
		{"expression: the name of the committer of the head commit", exprStep("github.event.head_commit.committer.name"), 1},
		{"expression: the title", exprStep("github.event.issue.title"), 1},
		// the output of a command
		{"script: the current branch", scriptStep("git branch --show-current"), 0},
		{"script: the abbreviated ref", scriptStep("git rev-parse --abbrev-ref HEAD"), 0},
		{"script: the login of the author of a pull request", scriptStep("gh pr view \"$N\" --json author --jq .author.login"), 0},
		{"script: the head branch of a pull request", scriptStep("gh pr view \"$N\" --json headRefName --jq .headRefName"), 0},
		{"script: the name of the author of a commit", scriptStep("git log -1 --format=%an"), 1},
		{"script: the name of the committer of a commit", scriptStep("git log -1 --pretty=format:%cn"), 1},
		{"script: the name of the author of a pull request", scriptStep("gh pr view \"$N\" --json author --jq .author.name"), 1},
		{"script: the author of a pull request without a filter", scriptStep("gh pr view \"$N\" --json author"), 1},
		{"script: the title of a pull request", scriptStep("gh pr view \"$N\" --json title --jq .title"), 1},
		{"script: the subject of a commit", scriptStep("git log -1 --format=%s"), 1},
	}
	cfg := &Config{Rules: map[string]RuleConfig{"argument-injection": {Level: SeverityError}}}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head + tc.steps
			n := 0
			for _, e := range lintBatchD(t, "test.yaml", src, cfg) {
				if e.ID == "argument-injection" {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("%d findings, want %d\n%s", n, tc.want, src)
			}
		})
	}
}
