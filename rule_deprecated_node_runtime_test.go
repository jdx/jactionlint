package jactionlint

import (
	"io"
	"testing"
)

func lintActionWithConfig(t *testing.T, cfg *Config, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(cfg)
	errs, err := l.Lint(".github/actions/x/action.yml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func deprecatedNodeRuntimeFindings(errs []*Error) []*Error {
	var found []*Error
	for _, e := range errs {
		if e.ID == "deprecated-node-runtime" {
			found = append(found, e)
		}
	}
	return found
}

func TestDeprecatedNodeRuntimeInActionMetadata(t *testing.T) {
	const head = "name: x\ndescription: x\nruns:\n  using: "
	tests := []struct {
		name string
		src  string
		want int
		line int
	}{
		{"node20", head + "node20\n  main: index.js\n", 1, 4},
		{"node20 in capitals, which the runner accepts", head + "NODE20\n  main: index.js\n", 1, 4},
		{"node16", head + "node16\n  main: index.js\n", 1, 4},
		{"node12", head + "node12\n  main: index.js\n", 1, 4},
		{"node24 is the runtime that runners have", head + "node24\n  main: index.js\n", 0, 0},
		{"a future runtime", head + "node26\n  main: index.js\n", 0, 0},
		{"composite", head + "composite\n  steps:\n    - run: echo\n      shell: bash\n", 0, 0},
		{"docker", head + "docker\n  image: docker://alpine:3\n", 0, 0},
	}
	cfg := func() *Config {
		return &Config{Rules: map[string]RuleConfig{"deprecated-node-runtime": {Level: SeverityWarning}}}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			found := deprecatedNodeRuntimeFindings(lintActionWithConfig(t, cfg(), tc.src))
			if len(found) != tc.want {
				t.Fatalf("want %d findings, got %v", tc.want, found)
			}
			if tc.want > 0 && (found[0].Line != tc.line || found[0].Severity != SeverityWarning) {
				t.Errorf("want a warning at line %d, got %v", tc.line, found[0])
			}
		})
	}
}

// The runner runs a node20 action (on Node.js 24), so the finding is not part of the profiles that fail a
// build: it needs the pedantic profile or a rule setting.
func TestDeprecatedNodeRuntimeIsPedantic(t *testing.T) {
	src := "name: x\ndescription: x\nruns:\n  using: node20\n  main: index.js\n"
	for _, p := range []Profile{ProfileCorrectness, ProfileDefault} {
		if found := deprecatedNodeRuntimeFindings(lintActionWithConfig(t, &Config{Profile: p}, src)); len(found) != 0 {
			t.Errorf("profile %q must not report it: %v", p, found)
		}
	}
	found := deprecatedNodeRuntimeFindings(lintActionWithConfig(t, &Config{Profile: ProfilePedantic}, src))
	if len(found) != 1 || found[0].Severity != SeverityWarning {
		t.Errorf("the pedantic profile reports a warning: %v", found)
	}
}

// A workflow does not declare a runtime: only the metadata of an action does.
func TestDeprecatedNodeRuntimeIgnoresWorkflows(t *testing.T) {
	src := "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo using node20\n"
	cfg := &Config{Rules: map[string]RuleConfig{"deprecated-node-runtime": {Level: SeverityWarning}}}
	if found := deprecatedNodeRuntimeFindings(lintWithConfig(t, cfg, src)); len(found) != 0 {
		t.Errorf("unexpected: %v", found)
	}
}
