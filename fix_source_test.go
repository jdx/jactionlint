package jactionlint

import (
	"testing"

	"go.yaml.in/yaml/v4"
)

// A fix never inserts a line inside a value which spans several lines, whatever the continuation lines
// look like. The fixed file parses as the same workflow plus the new keys, and has no finding left; or
// the file is left alone.
func TestFixesAroundMultiLineValues(t *testing.T) {
	steps := "    steps:\n      - run: echo\n"
	tests := []struct {
		what string
		src  string
		// jobName is the expected name of the job a, or "" when it has none
		jobName string
	}{
		{"flow sequence closed at the indent of the key",
			"on: [push,\n  pull_request\n]\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + steps, ""},
		{"flow mapping closed at the indent of the key",
			"on: {push: {branches: [main]},\n  pull_request: {}\n}\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + steps, ""},
		{"flow mapping with items at the indent of the key",
			"on: {\npush: {},\npull_request: {}\n}\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + steps, ""},
		{"runs-on as a flow sequence closed at the indent of the key",
			"on: push\njobs:\n  a:\n    runs-on: [ubuntu-latest,\n      self-hosted\n    ]\n" + steps, ""},
		{"double-quoted name continued at the indent of the key",
			"on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    name: \"first\n    second\"\n" + steps, "first second"},
		{"double-quoted name with an escaped quote",
			"on: push\njobs:\n  a:\n    name: \"say \\\"hi\\\"\n      there\"\n    runs-on: ubuntu-latest\n" + steps, "say \"hi\" there"},
		{"single-quoted name with a doubled quote",
			"on: push\njobs:\n  a:\n    name: 'it''s\n    fine'\n    runs-on: ubuntu-latest\n" + steps, "it's fine"},
		{"quoted continuation that looks like a key",
			"on: push\njobs:\n  a:\n    name: \"first\n    runs-on: x\n    last\"\n    runs-on: ubuntu-latest\n" + steps, "first runs-on: x last"},
		{"quoted continuation that looks like a comment",
			"on: push\njobs:\n  a:\n    name: \"first\n    # not a comment\n    last\"\n    runs-on: ubuntu-latest\n" + steps, "first # not a comment last"},
		{"double-quoted runs-on continued at the indent of the key",
			"on: push\njobs:\n  a:\n    runs-on: \"ubuntu-\n    latest\"\n" + steps, ""},
		{"double-quoted runs-on with escapes",
			"on: push\njobs:\n  a:\n    runs-on: \"ubuntu\\\"\n    -latest\"\n" + steps, ""},
		{"single-quoted runs-on with a doubled quote",
			"on: push\njobs:\n  a:\n    runs-on: 'ubuntu''\n    -latest'\n" + steps, ""},
		{"quoted on",
			"on: \"push\n\"\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + steps, ""},
		{"plain multi-line name",
			"on: push\njobs:\n  a:\n    name: first\n      second\n    runs-on: ubuntu-latest\n" + steps, "first second"},
		{"plain multi-line runs-on",
			"on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n      # c\n" + steps, ""},
		{"on with a multi-line quoted event",
			"\"on\": \"push\"\njobs:\n  a:\n    runs-on: ubuntu-latest\n" + steps, ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tc.src), &node); err != nil {
				t.Skipf("the YAML parser rejects the fixture: %v", err)
			}
			before, perrs := Parse([]byte(tc.src))
			if before == nil || len(perrs) > 0 {
				t.Skipf("the fixture is not a valid workflow: %v", perrs)
			}
			out, n, left := fixWith(t, []byte(tc.src), fixerConfig(t, ""), FixModeUnsafe)
			after, perrs := Parse(out)
			if after == nil || len(perrs) > 0 {
				t.Fatalf("the fixed file does not parse (%d fixes): %v\n%s", n, perrs, out)
			}
			if len(after.On) != len(before.On) {
				t.Errorf("the events changed: %d -> %d\n%s", len(before.On), len(after.On), out)
			}
			a, b := before.Jobs["a"], after.Jobs["a"]
			if (a.Name == nil) != (b.Name == nil) || (a.Name != nil && a.Name.Value != b.Name.Value) {
				t.Errorf("the job name changed: %v -> %v\n%s", a.Name, b.Name, out)
			}
			if tc.jobName != "" && (b.Name == nil || b.Name.Value != tc.jobName) {
				t.Errorf("job name = %v, want %q\n%s", b.Name, tc.jobName, out)
			}
			if n == 0 {
				if string(out) != tc.src {
					t.Errorf("no fix applied but the file changed:\n%s", out)
				}
				return
			}
			// What was fixed is fixed, and nothing else broke
			if len(left) != 0 {
				t.Errorf("findings left after the fixes: %v\n%s", left, out)
			}
			if after.Permissions == nil || b.TimeoutMinutes == nil {
				t.Errorf("permissions or timeout-minutes missing after %d fixes:\n%s", n, out)
			}
		})
	}
}
