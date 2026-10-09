package jactionlint

import (
	"strings"
	"testing"
)

func TestVerifyFix(t *testing.T) {
	const base = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo one\n      - run: echo two\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n"
	edit := func(src, from, to string) (string, []TextEdit) {
		i := strings.Index(src, from)
		if i < 0 {
			t.Fatalf("%q not found", from)
		}
		return src[:i] + to + src[i+len(from):], []TextEdit{{Start: i, End: i + len(from), NewText: to}}
	}
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr string
	}{
		{"change a value", "echo one", "echo 1", ""},
		{"change the quoting only", "echo one", "echo one", ""},
		{"add a key", "    runs-on: ubuntu-latest\n    steps:\n      - run: echo one", "    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo one", ""},
		{"insert a step before another", "      - run: echo two", "      - run: echo 1.5\n      - run: echo two", ""},
		{"remove a step", "      - run: echo two\n", "", ""},
		{"add a comment", "on: push", "on: push # trigger", ""},
		{"invalid yaml", "echo one", "echo one\n  broken: [", "not valid YAML"},
		{"add a sibling key", "echo one", "echo one\n        env: x", ""},
		{"a dedent which takes the following lines with it", "      - run: echo two\n  b:", "    - run: echo two\n  b:", "not valid YAML"},
		{"a new top level key", "on: push", "on: push\nx: true", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, edits := edit(base, tc.from, tc.to)
			err := verifyFix([]byte(base), []byte(out), edits)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("want error containing %q but got %v", tc.wantErr, err)
			}
		})
	}
}

// A change which is not where the edits are is refused, though the edits themselves are fine.
func TestVerifyFixRefusesChangesAwayFromEdits(t *testing.T) {
	old := "a: 1\nb: 2\nc: 3\nd:\n  - x\n  - y\n"
	// The edits claim to change a, but the text changes c
	newSrc := "a: 1\nb: 2\nc: 4\nd:\n  - x\n  - y\n"
	err := verifyFix([]byte(old), []byte(newSrc), []TextEdit{{Start: 3, End: 4, NewText: "1"}})
	if err == nil || !strings.Contains(err.Error(), "away from the edits") {
		t.Errorf("want an error about a difference away from the edits but got %v", err)
	}
	// The same text with the right edit passes
	if err := verifyFix([]byte(old), []byte(newSrc), []TextEdit{{Start: 13, End: 14, NewText: "4"}}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// 'true' is a string and true is a boolean
	if err := verifyFix([]byte("a: 'true'\nb: 1\n"), []byte("a: true\nb: 1\n"), []TextEdit{{Start: 0, End: 0, NewText: ""}}); err == nil {
		t.Error("a change of the type must be found")
	}
	if err := verifyFix([]byte("a: 'true'\nb: 1\n"), []byte("a: \"true\"\nb: 1\n"), []TextEdit{{Start: 3, End: 9, NewText: `"true"`}}); err != nil {
		t.Errorf("only the quoting differs: %v", err)
	}
	// Keys may move, comments may change
	if err := verifyFix([]byte("a: 1\nb: 2\n"), []byte("b: 2 # x\na: 1\n"), []TextEdit{{Start: 0, End: 10, NewText: "b: 2 # x\na: 1\n"}}); err != nil {
		t.Errorf("reordering inside an edit is fine: %v", err)
	}
	// Documents
	if err := verifyFix([]byte("a: 1\n"), []byte("a: 1\n---\nb: 2\n"), []TextEdit{{Start: 5, End: 5, NewText: "---\nb: 2\n"}}); err == nil {
		t.Error("a new document must be refused")
	}
}

func TestVerifyFixFlowAndMultiline(t *testing.T) {
	old := "on: [push, pull_request]\njobs:\n  a:\n    steps:\n      - run: |\n          echo ${{ github.x }}\n          echo done\n"
	newSrc := "on: [push, pull_request]\njobs:\n  a:\n    steps:\n      - run: |\n          echo \"${X}\"\n          echo done\n        env:\n          X: ${{ github.x }}\n"
	i := strings.Index(old, "${{")
	j := strings.Index(old, "}}") + 2
	edits := []TextEdit{
		{Start: i, End: j, NewText: `"${X}"`},
		{Start: len(old), End: len(old), NewText: "        env:\n          X: ${{ github.x }}\n"},
	}
	if got := string(applyEdits([]byte(old), edits)); got != newSrc {
		t.Fatalf("test setup: %q", got)
	}
	if err := verifyFix([]byte(old), []byte(newSrc), edits); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
