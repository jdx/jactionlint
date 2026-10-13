package jactionlint

import "testing"

const eventNameID = "event-name-never-received"

func TestEventNameNeverReceived(t *testing.T) {
	cfg := ruleConfig(eventNameID)
	wf := func(on, rest string) string {
		return "on: " + on + "\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + rest
	}
	steps := func(cond string) string {
		return "    steps:\n      - run: echo\n        if: " + cond + "\n"
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		// never true
		{"step condition", wf("push", steps("github.event_name == 'pull_request'")), []int{7}},
		{"wrapped in an expression", wf("push", steps("${{ github.event_name == 'pull_request' }}")), []int{7}},
		{"job condition", wf("push", "    if: github.event_name == 'release'\n    steps:\n      - run: echo\n"), []int{5}},
		{"the string on the left", wf("push", steps("${{ 'schedule' == github.event_name }}")), []int{7}},
		{"another case", wf("push", steps("github.event_name == 'Pull_Request'")), []int{7}},
		{"a list of events", wf("[push, workflow_dispatch]", steps("github.event_name == 'schedule'")), []int{7}},
		{"a map of events", wf("\n  push:\n    branches: [main]\n  pull_request:", steps("github.event_name == 'issues'")), []int{10}},
		{"one branch of an or", wf("push", steps("github.event_name == 'push' || github.event_name == 'pull_request'")), []int{7}},
		{"in an and", wf("push", steps("github.ref == 'x' && github.event_name == 'release'")), []int{7}},
		{"in an env value", wf("push", "    env:\n      PR: ${{ github.event_name == 'pull_request' }}\n    steps:\n      - run: echo\n"), []int{6}},
		{"in a run script", wf("push", "    steps:\n      - run: echo ${{ github.event_name == 'pull_request' }}\n"), []int{6}},
		{"bracket access", wf("push", steps("github['event_name'] == 'release'")), []int{7}},
		{"two events at once", wf("[push, pull_request]", steps("github.event_name == 'push' && github.event_name == 'pull_request'")), []int{7}},
		{"two events at once in a longer and", wf("[push, pull_request]", steps("github.ref && github.event_name == 'push' && github.run_id && github.event_name == 'pull_request'")), []int{7}},
		{"in a matrix include", wf("push", "    strategy:\n      matrix:\n        x:\n          - ${{ github.event_name == 'release' }}\n    steps:\n      - run: echo\n"), []int{8}},

		// conditions with spaces around them, or in a block
		{"a condition in a literal block", wf("push", "    steps:\n      - run: echo\n        if: |\n          github.event_name == 'release'\n"), []int{8}},
		{"a condition in a folded block", wf("push", "    steps:\n      - run: echo\n        if: >-\n          github.ref == 'x' &&\n          github.event_name == 'release'\n"), []int{9}},
		{"a condition in quotes with spaces", wf("push", "    steps:\n      - run: echo\n        if: \"  github.event_name == 'release'  \"\n"), []int{7}},
		{"a job condition in a block", wf("push", "    if: |\n      github.event_name == 'release'\n    steps:\n      - run: echo\n"), []int{6}},

		// possible
		{"the event", wf("push", steps("github.event_name == 'push'")), nil},
		{"one of the events", wf("[push, pull_request]", steps("github.event_name == 'pull_request'")), nil},
		{"the case of the name", wf("push", steps("github.event_name == 'PUSH'")), nil},
		{"or of events", wf("[push, pull_request]", steps("github.event_name == 'push' || github.event_name == 'pull_request'")), nil},
		{"the same event twice", wf("push", steps("github.event_name == 'push' && github.event_name == 'push'")), nil},
		{"not equal", wf("push", steps("github.event_name != 'pull_request'")), nil},
		{"schedule", wf("\n  schedule:\n    - cron: '0 0 * * *'", steps("github.event_name == 'schedule'")), nil},
		{"workflow_dispatch", wf("workflow_dispatch", steps("github.event_name == 'workflow_dispatch'")), nil},
		{"repository_dispatch", wf("repository_dispatch", steps("github.event_name == 'repository_dispatch'")), nil},
		{"not an event name", wf("push", steps("github.ref == 'refs/heads/main'")), nil},
		{"a variable", wf("push", steps("github.event_name == matrix.event")), nil},
		{"two events on or branches", wf("[push, pull_request]", steps("(github.event_name == 'push' && github.ref == 'x') || github.event_name == 'pull_request'")), nil},

		// workflows which are not looked at
		{"workflow_call", wf("[push, workflow_call]", steps("github.event_name == 'pull_request'")), nil},
		{"only workflow_call", "on:\n  workflow_call:\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        if: github.event_name == 'pull_request'\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), eventNameID, tc.lines...)
		})
	}
}

func TestEventNameNeverReceivedMessage(t *testing.T) {
	cfg := ruleConfig(eventNameID)
	src := "on: [push, workflow_dispatch]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        if: ${{ github.ref != 'é' && github.event_name == 'pull_request' }}\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), eventNameID)
	if len(errs) != 1 || errs[0].Line != 7 || errs[0].Column != 38 {
		t.Fatalf("unexpected findings: %v", errs)
	}
	want := `"github.event_name == 'pull_request'" is always false: this workflow is never run by the event "pull_request", it runs on push, workflow_dispatch`
	if errs[0].Message != want {
		t.Errorf("message is %q", errs[0].Message)
	}
}

// A string that the parser keeps in two fields (a mapping that is one expression) is reported once by the rule itself.
func TestEventNameNeverReceivedNoDuplicates(t *testing.T) {
	cond := "${{ github.event_name == 'release' && 'a' || 'b' }}"
	src := "on: push\nconcurrency: " + cond + "\njobs:\n  a:\n    runs-on: ubuntu-latest\n    container: " + cond + "\n    strategy: " + cond + "\n    steps:\n      - run: echo\n"
	w, errs := Parse([]byte(src))
	if w == nil {
		t.Fatal(errs)
	}
	rule := NewRuleEventNameNeverReceived([]byte(src))
	if err := rule.VisitWorkflowPre(w); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, e := range rule.Errs() {
		seen[e.Error()]++
	}
	if len(seen) != 3 {
		t.Errorf("want one finding for each of the three places: %v", seen)
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("reported %d times: %s", n, k)
		}
	}
}
