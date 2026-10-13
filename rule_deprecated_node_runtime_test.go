package jactionlint

import (
	"io"
	"regexp"
	"strings"
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

func lintWorkflowForRuntime(t *testing.T, steps string) []*Error {
	t.Helper()
	src := "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n" + steps
	cfg := &Config{Rules: map[string]RuleConfig{"deprecated-node-runtime": {Level: SeverityWarning}}}
	return deprecatedNodeRuntimeFindings(lintWithConfig(t, cfg, src))
}

func TestDeprecatedNodeRuntimeOfPopularActions(t *testing.T) {
	const sha = "11bd71901bbe5b1630ceea73d27597364c9af683"
	tests := []struct {
		name  string
		uses  string
		want  string // a substring of the message, or "" when nothing is reported
		notIn string // a substring that the message must not have
	}{
		{name: "a node20 major", uses: "actions/checkout@v4", want: `update the action to "actions/checkout@v5" or newer, which declares "node24"`},
		{name: "a full version of a node20 major", uses: "actions/checkout@v4.2.2", want: `(checked as "actions/checkout@v4", the major version of the tag)`},
		{name: "a commit with a version comment", uses: "actions/checkout@" + sha + " # v4.2.2", want: `the version comment "# v4.2.2" says so`},
		{name: "an action in a subdirectory", uses: "actions/cache/restore@v4", want: `"actions/cache/restore@v5" or newer`},
		{name: "a newer major which is also node20 is skipped", uses: "actions/download-artifact@v5", want: `"actions/download-artifact@v7" or newer`},
		{name: "a path of the CodeQL action", uses: "github/codeql-action/analyze@v3", want: `"github/codeql-action/analyze@v4" or newer`},
		{name: "a key which is not a major version", uses: "actions/download-artifact@v3-node20", want: `update the action to a release that declares "node24"`, notIn: "or newer"},
		{name: "the node24 major", uses: "actions/checkout@v5"},
		{name: "the latest major", uses: "actions/checkout@v7"},
		{name: "a runtime older than node20 is outdated-action-runner's", uses: "actions/checkout@v3"},
		{name: "a commit without a version comment", uses: "actions/checkout@" + sha},
		{name: "a branch", uses: "actions/checkout@main"},
		{name: "an action that is not known", uses: "someone/unknown-action@v1"},
		{name: "a local action", uses: "./.github/actions/setup"},
		{name: "a docker image", uses: "docker://alpine:3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			found := lintWorkflowForRuntime(t, "      - uses: "+tc.uses+"\n")
			if tc.want == "" {
				if len(found) != 0 {
					t.Fatalf("unexpected: %v", found)
				}
				return
			}
			if len(found) != 1 || !strings.Contains(found[0].Message, tc.want) {
				t.Fatalf("want one finding with %q, got %v", tc.want, found)
			}
			if tc.notIn != "" && strings.Contains(found[0].Message, tc.notIn) {
				t.Errorf("the message must not have %q: %v", tc.notIn, found[0])
			}
			if found[0].Line != 8 || found[0].Severity != SeverityWarning {
				t.Errorf("want a warning at line 8, got %v", found[0])
			}
		})
	}
}

// The steps of a composite action use popular actions like the steps of a job do.
func TestDeprecatedNodeRuntimeInCompositeSteps(t *testing.T) {
	src := "name: x\ndescription: x\nruns:\n  using: composite\n  steps:\n    - uses: actions/setup-node@v4\n"
	cfg := &Config{Rules: map[string]RuleConfig{"deprecated-node-runtime": {Level: SeverityWarning}}}
	found := deprecatedNodeRuntimeFindings(lintActionWithConfig(t, cfg, src))
	if len(found) != 1 || found[0].Line != 6 {
		t.Fatalf("want one finding at line 6, got %v", found)
	}
}

// The data of the runtimes is generated next to the inputs of the actions: it names known actions, and an outdated action
// is not in it.
func TestPopularActionRuntimesData(t *testing.T) {
	isNode := regexp.MustCompile(`^node[0-9]+$`)
	for spec, rt := range PopularActionRuntimes {
		if _, ok := PopularActions[spec]; !ok {
			t.Errorf("%s has a runtime but no metadata", spec)
		}
		if _, ok := OutdatedPopularActionSpecs[spec]; ok {
			t.Errorf("%s is outdated and has a runtime", spec)
		}
		if !isNode.MatchString(rt) {
			t.Errorf("%s has the runtime %q", spec, rt)
		}
	}
	// A released major keeps its runtime
	for spec, want := range map[string]string{"actions/checkout@v4": "node20", "actions/checkout@v5": "node24", "github/codeql-action/init@v3": "node20"} {
		if got := PopularActionRuntimes[spec]; got != want {
			t.Errorf("%s: want %s, got %q", spec, want, got)
		}
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
