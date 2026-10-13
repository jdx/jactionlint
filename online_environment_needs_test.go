//go:build !js

package jactionlint

import (
	"strings"
	"testing"
)

const environmentNeedsWorkflow = `on: pull_request_target
jobs:
  approval:
    runs-on: ubuntu-latest
    environment: release
    steps:
      - run: echo approved
  deploy:
    needs: approval
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci && npm test
`

// A job that needs a job with an environment is held back with it, unless the online checks find that the environment has no
// required reviewers: the same answer as for the environment of the job itself.
func TestEnvironmentOfNeededJob(t *testing.T) {
	const reviewers = `{"id":1,"type":"required_reviewers","reviewers":[{"type":"User","reviewer":{"login":"octocat"}}]}`
	for _, tc := range []struct {
		name    string
		rules   string
		online  bool
		finding bool
	}{
		{"reviewers", reviewers, true, false},
		{"no reviewers", `{"id":2,"type":"branch_policy"}`, true, true},
		{"offline", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitHub(t)
			f.json("/repos/o/r/environments/release", environmentJSON(tc.rules))
			setOnlineEnv(t, f, "tok")
			t.Setenv("GITHUB_REPOSITORY", "o/r")
			_, wf := onlineProject(t, environmentNeedsWorkflow, environmentConfig)
			args := []string{wf}
			if tc.online {
				args = append([]string{"--online"}, args...)
			}
			_, stdout, stderr := runOnlineCommand(t, args...)
			if got := strings.Contains(stdout, "[untrusted-checkout]"); got != tc.finding {
				t.Errorf("untrusted-checkout reported: %v, want %v\n%s%s", got, tc.finding, stdout, stderr)
			}
		})
	}
}
