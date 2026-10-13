package jactionlint

import (
	"strings"
	"testing"
)

// The lines which end with "# want" are the ones reported, as missing-action-input or conflicting-action-inputs.
func TestRuleActionInputDependencies(t *testing.T) {
	const head = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		what string
		src  string
	}{
		// download-artifact: run-id and repository need github-token
		{"run-id without a token", head + "      - uses: actions/download-artifact@v4\n        with:\n          name: a\n          run-id: ${{ github.event.workflow_run.id }} # want\n"},
		{"repository without a token", head + "      - uses: actions/download-artifact@v4\n        with:\n          repository: o/r # want\n"},
		{"both without a token", head + "      - uses: actions/download-artifact@v5\n        with:\n          run-id: 123 # want\n          repository: o/r # want\n"},
		{"pinned with a comment", head + "      - uses: actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093 # v4.3.0\n        with:\n          run-id: 123 # want\n"},
		{"full version", head + "      - uses: actions/download-artifact@v4.1.8\n        with:\n          run-id: 123 # want\n"},
		{"with a token", head + "      - uses: actions/download-artifact@v4\n        with:\n          run-id: 123\n          github-token: ${{ secrets.PAT }}\n"},
		{"the default run", head + "      - uses: actions/download-artifact@v4\n        with:\n          run-id: ${{ github.run_id }}\n          repository: ${{ github.repository }}\n"},
		{"neither", head + "      - uses: actions/download-artifact@v4\n        with:\n          name: a\n"},
		{"an old version has no such inputs", head + "      - uses: actions/download-artifact@v3\n        with:\n          run-id: 123\n"},
		{"a with that is an expression", head + "      - uses: actions/download-artifact@v4\n        with: ${{ fromJSON(inputs.with) }}\n"},
		{"a with with an insert key may give the token", head + "      - uses: actions/download-artifact@v4\n        with:\n          run-id: 123\n          ${{ insert }}: ${{ matrix.inputs }}\n"},
		{"a retry with an insert key may give the timeout", head + "      - uses: nick-fields/retry@v3\n        with:\n          command: make\n          ${{ insert }}: ${{ matrix.inputs }}\n"},
		{"a retry with a with that is one expression", head + "      - uses: nick-fields/retry@v3\n        with: ${{ fromJSON(inputs.with) }}\n"},
		{"a branch is not judged", head + "      - uses: actions/download-artifact@main\n        with:\n          run-id: 123\n"},
		{"name and artifact ids", head + "      - uses: actions/download-artifact@v4\n        with:\n          name: a\n          artifact-ids: 1,2 # want\n"},
		{"only artifact ids", head + "      - uses: actions/download-artifact@v4\n        with:\n          artifact-ids: 1,2\n"},

		// nick-fields/retry: exactly one of the timeouts
		{"no timeout", head + "      - uses: nick-fields/retry@v3 # want\n        with:\n          max_attempts: 3\n          command: make\n"},
		{"no with", head + "      - uses: nick-fields/retry@v2 # want\n"},
		{"minutes", head + "      - uses: nick-fields/retry@v3\n        with:\n          timeout_minutes: 5\n          command: make\n"},
		{"seconds", head + "      - uses: nick-fields/retry@v3\n        with:\n          timeout_seconds: 30\n          command: make\n"},
		{"both", head + "      - uses: nick-fields/retry@v3\n        with:\n          timeout_minutes: 5\n          timeout_seconds: 30 # want\n          command: make\n"},
		{"a timeout from an expression", head + "      - uses: nick-fields/retry@v3\n        with:\n          timeout_minutes: ${{ inputs.minutes }}\n          timeout_seconds: ${{ inputs.seconds }}\n          command: make\n"},
		{"a timeout from a variable and none other", head + "      - uses: nick-fields/retry@v3\n        with:\n          timeout_minutes: ${{ vars.T }}\n          command: make\n"},
		{"a pinned retry without a comment is not judged", head + "      - uses: nick-fields/retry@7152eba30c6575329ac0576536151aca5a72780e\n        with:\n          command: make\n"},

		// google-github-actions/auth: exactly one of
		{"neither identity", head + "      - uses: google-github-actions/auth@v2 # want\n        with:\n          project_id: p\n"},
		{"both identities", head + "      - uses: google-github-actions/auth@v2\n        with:\n          workload_identity_provider: projects/1/x\n          credentials_json: ${{ secrets.K }}\n"},
		{"both identities written out", head + "      - uses: google-github-actions/auth@v2\n        with:\n          workload_identity_provider: projects/1/x\n          credentials_json: '{\"a\": 1}' # want\n"},
		{"workload identity", head + "      - uses: google-github-actions/auth@v3\n        with:\n          workload_identity_provider: projects/1/x\n"},
		{"a key", head + "      - uses: google-github-actions/auth@v3\n        with:\n          credentials_json: ${{ secrets.K }}\n"},
		{"auth v1 is judged", head + "      - uses: google-github-actions/auth@v1 # want\n"},

		// dependency-review-action: allow and deny exclude each other
		{"allow and deny", head + "      - uses: actions/dependency-review-action@v4\n        with:\n          allow-licenses: MIT\n          deny-licenses: GPL-3.0 # want\n"},
		{"allow", head + "      - uses: actions/dependency-review-action@v4\n        with:\n          allow-licenses: MIT\n"},
		{"deny", head + "      - uses: actions/dependency-review-action@v4\n        with:\n          deny-licenses: GPL-3.0\n"},
		{"one is empty", head + "      - uses: actions/dependency-review-action@v4\n        with:\n          allow-licenses: MIT\n          deny-licenses: ''\n"},

		// Not in the table
		{"another action", head + "      - uses: actions/cache@v4\n        with:\n          path: a\n          key: b\n"},
		{"a local action", head + "      - uses: ./.github/actions/x\n        with:\n          run-id: 1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			got := append(lintBatchH(t, "", tc.src, "missing-action-input"), lintBatchH(t, "", tc.src, "conflicting-action-inputs")...)
			checkLines(t, got, markedWantLines(tc.src)...)
		})
	}
}

func TestRuleActionInputDependenciesMessages(t *testing.T) {
	const head = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		src, id, want string
	}{
		{head + "      - uses: actions/download-artifact@v4\n        with:\n          run-id: 1\n", "missing-action-input",
			`input "run-id" of action "actions/download-artifact@v4" needs "github-token", without it the action ignores it and downloads from the current run. add "github-token"`},
		{head + "      - uses: nick-fields/retry@v3\n        with:\n          command: make\n", "missing-action-input",
			`missing input: action "nick-fields/retry@v3" needs "timeout_minutes" or "timeout_seconds", the action fails with "Must specify either timeout_minutes or timeout_seconds inputs". give one of them`},
		{head + "      - uses: actions/dependency-review-action@v4\n        with:\n          allow-licenses: MIT\n          deny-licenses: GPL-3.0\n", "conflicting-action-inputs",
			`inputs "allow-licenses" and "deny-licenses" of action "actions/dependency-review-action@v4" exclude each other, the action raises an error. give only one of "allow-licenses" and "deny-licenses"`},
	}
	for _, tc := range tests {
		errs := lintBatchH(t, "", tc.src, tc.id)
		if len(errs) != 1 || errs[0].Message != tc.want {
			t.Errorf("want %q, got %v", tc.want, errs)
		}
	}
}

// The rules of the table are off in nothing: they are in the correctness profile and can be turned off one by one.
func TestActionInputDependenciesConfig(t *testing.T) {
	const src = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: nick-fields/retry@v3\n        with:\n          command: make\n"
	if got := lintBatchH(t, "profile: correctness\n", src, "missing-action-input"); len(got) != 1 {
		t.Errorf("correctness: %v", got)
	}
	if got := lintBatchH(t, "rules:\n  missing-action-input: off\n", src, "missing-action-input"); len(got) != 0 {
		t.Errorf("off: %v", got)
	}
}

func TestInputDependencyTable(t *testing.T) {
	for name, e := range inputDependencyTable {
		if name != strings.ToLower(name) || e.MinMajor < 1 || !strings.HasPrefix(e.Source, "https://") || len(e.Rules) == 0 {
			t.Errorf("%s: incomplete entry", name)
		}
		for _, r := range e.Rules {
			if len(r.Inputs) < 2 || r.Effect == "" {
				t.Errorf("%s: incomplete rule %+v", name, r)
			}
			for _, i := range r.Inputs {
				if i != strings.ToLower(i) {
					t.Errorf("%s: input %q must be lower case", name, i)
				}
			}
		}
	}
	for key, want := range map[string]bool{"actions/download-artifact@v4": true, "Actions/Download-Artifact@v8": true, "actions/download-artifact@v3": false, "actions/download-artifact@main": false, "actions/cache@v4": false} {
		if got := knownInputDependencies(key); got != want {
			t.Errorf("%s: want %v", key, want)
		}
	}
}
