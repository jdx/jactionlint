package jactionlint

import (
	"io"
	"slices"
	"strings"
	"testing"
)

// The schema of GitHub lets a whole mapping (or list) be written as one expression where it gives the key a list of
// contexts: `with: ${{ fromJSON(vars.INPUTS) }}`, `strategy: ${{ ... }}`, the "run" defaults of a job, the ports of
// a container and the keys `${{ insert }}`. These tests pin how the parser represents them, that no rule reports what
// it cannot know about them, and that the rules which look for something missing do not look at such a step.

func parseWholeValueWorkflow(t *testing.T, src string) *Workflow {
	t.Helper()
	w, errs := Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatalf("unexpected syntax errors: %v", errs)
	}
	return w
}

func TestParseWholeValueExpressions(t *testing.T) {
	w := parseWholeValueWorkflow(t, `on: push
env:
  ${{ insert }}: ${{ fromJSON(vars.WORKFLOW_ENV) }}
  PLAIN: x
jobs:
  a:
    runs-on: ubuntu-latest
    strategy: ${{ fromJSON(vars.STRATEGY) }}
    defaults:
      run: ${{ fromJSON(vars.RUN) }}
    concurrency: ${{ fromJSON(vars.C) }}
    environment: ${{ fromJSON(vars.E) }}
    container: ${{ fromJSON(vars.CONTAINER) }}
    services:
      s: ${{ fromJSON(vars.S) }}
      t:
        image: redis
        ports: ${{ fromJSON(vars.PORTS) }}
        volumes: ${{ fromJSON(vars.VOLUMES) }}
    steps:
      - uses: actions/cache@v4
        with: ${{ fromJSON(vars.W) }}
      - uses: actions/checkout@v4
        with:
          ${{ insert }}: ${{ fromJSON(vars.MORE) }}
          ${{ insert }}:
            fetch-depth: 0
          ref: main
      - run: echo
        env:
          ${{ insert }}: ${{ fromJSON(vars.E) }}
`)
	if v := w.Env.Inserts; len(v) != 1 || v[0].Value != "${{ fromJSON(vars.WORKFLOW_ENV) }}" || len(w.Env.Vars) != 1 || !w.Env.Dynamic() {
		t.Errorf("workflow env: %+v", w.Env)
	}
	j := w.Jobs["a"]
	if j.Strategy.Expression == nil || j.Strategy.Matrix == nil || j.Strategy.Matrix.Expression != j.Strategy.Expression {
		t.Errorf("strategy: %+v", j.Strategy)
	}
	if r := j.Defaults.Run; r.Expression == nil || r.Shell != r.Expression || r.WorkingDirectory != r.Expression {
		t.Errorf("defaults.run: %+v", r)
	}
	if j.Concurrency.Expression == nil || j.Concurrency.Bare {
		t.Errorf("concurrency: %+v", j.Concurrency)
	}
	if j.Environment.Expression == nil || j.Environment.Name != j.Environment.Expression {
		t.Errorf("environment: %+v", j.Environment)
	}
	if j.Container.Expression == nil || j.Container.Image != j.Container.Expression {
		t.Errorf("container: %+v", j.Container)
	}
	if s := j.Services.Value["s"].Container; s.Expression == nil {
		t.Errorf("service s: %+v", s)
	}
	if s := j.Services.Value["t"].Container; s.PortsExpression == nil || s.VolumesExpression == nil || s.Ports != nil || s.Volumes != nil || s.Expression != nil {
		t.Errorf("service t: %+v", s)
	}

	cache := j.Steps[0].Exec.(*ExecAction)
	if cache.InputsExpression == nil || len(cache.Inputs) != 0 || !cache.InputsUnknown() {
		t.Errorf("cache step: %+v", cache)
	}
	co := j.Steps[1].Exec.(*ExecAction)
	if len(co.InputInserts) != 1 || !co.InputsUnknown() || co.Inputs["ref"] == nil || co.Inputs["fetch-depth"] == nil {
		t.Errorf("checkout step: %+v", co)
	}
	if e := j.Steps[2].Env; len(e.Inserts) != 1 || !e.Dynamic() {
		t.Errorf("step env: %+v", e)
	}
	if (&ExecAction{}).InputsUnknown() || (*Env)(nil).Dynamic() || (&Env{}).Dynamic() {
		t.Error("nothing is dynamic without an expression")
	}
}

func TestParseWholeValueExpressionsAreErrorsWhereTheSchemaSaysSo(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"defaults of a workflow", "on: push\ndefaults:\n  run: ${{ fromJSON(vars.D) }}\njobs: {}\n", `"run" section is scalar node but mapping node is expected`},
		{"with of a reusable workflow call", "on: push\njobs:\n  a:\n    uses: o/r/.github/workflows/x.yaml@v1\n    with: ${{ fromJSON(vars.I) }}\n", `"with" section is scalar node but mapping node is expected`},
		{"plain text for with", "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: a/b@v1\n        with: plain\n", `"with" section is scalar node but mapping node is expected`},
		{"plain text for strategy", "on: push\njobs:\n  a:\n    runs-on: x\n    strategy: plain\n    steps:\n      - run: x\n", `"strategy" section is scalar node but mapping node is expected`},
		{"two expressions are not one", "on: push\njobs:\n  a:\n    runs-on: x\n    strategy: ${{ a }}${{ b }}\n    steps:\n      - run: x\n", `"strategy" section is scalar node but mapping node is expected`},
		{"insert with text", "on: push\nenv:\n  ${{ insert }}: text\njobs: {}\n", `expecting a single ${{...}} expression or mapping value for "${{ insert }}" key`},
		{"insert with a list", "on: push\nenv:\n  ${{ insert }}: [a]\njobs: {}\n", `the value of the "${{ insert }}" key must be a mapping or an expression`},
		{"insert is not a key elsewhere", "on: push\n${{ insert }}: ${{ vars.X }}\njobs: {}\n", `unexpected key "${{ insert }}" for "workflow" section`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.src))
			if !slices.ContainsFunc(errs, func(e *Error) bool { return strings.Contains(e.Message, tc.want) }) {
				t.Errorf("want an error with %q but got %v", tc.want, errs)
			}
		})
	}
}

func TestInsertDirectiveIsOnlyInsert(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{"${{ insert }}", true},
		{"${{insert}}", true},
		{"  ${{   insert   }}  ", true},
		{"${{ Insert }}", false},
		{"${{ insert foo }}", false},
		{"${{ inserts }}", false},
		{"${{ matrix.key }}", false},
		{"insert", false},
		{"${{ insert }}x", false},
		{"", false},
	} {
		if got := isInsertDirective(&String{Value: tc.key}); got != tc.want {
			t.Errorf("isInsertDirective(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
	if isInsertDirective(nil) {
		t.Error("nil is not a key")
	}
}

// lintWholeValue lints the workflow with a profile and returns the IDs and messages of the findings.
func lintWholeValue(t *testing.T, profile Profile, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{Profile: profile}
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func wholeValueIDs(errs []*Error) []string {
	var ids []string
	for _, e := range errs {
		ids = append(ids, e.ID)
	}
	return ids
}

// The rules which report what a step does not set must not report a step whose inputs come from an expression, and
// must report the same step when the inputs are written down (the control).
func TestRulesSkipStepsWithInputsFromExpression(t *testing.T) {
	const jobs = "jobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		name, id string
		on       string
		step     string // the step without "with"
		with     string // what the control has as "with"
	}{
		{"artipacked", "artipacked", "on: pull_request\n", "      - uses: actions/checkout@v4\n", "        with:\n          fetch-depth: 0\n"},
		{"cache-poisoning", "cache-poisoning", "on:\n  release:\n    types: [published]\n", "      - uses: actions/cache@v4\n", "        with:\n          path: x\n          key: y\n"},
		{"github-app", "github-app", "on: push\n", "      - uses: actions/create-github-app-token@v2\n", "        with:\n          app-id: ${{ vars.APP }}\n          private-key: ${{ secrets.KEY }}\n"},
		{"missing-action-input", "missing-action-input", "on: push\n", "      - uses: actions/cache@v4\n", "        with:\n          path: x\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// the control: the rule reports the step when the inputs are written down
			if !slices.Contains(wholeValueIDs(lintWholeValue(t, ProfilePedantic, tc.on+jobs+tc.step+tc.with)), tc.id) {
				t.Fatalf("the control must report %s", tc.id)
			}
			for _, profile := range []Profile{ProfileCorrectness, ProfileDefault, ProfilePedantic} {
				for _, w := range []string{
					"        with: ${{ fromJSON(vars.INPUTS) }}\n",
					"        with:\n          ${{ insert }}: ${{ fromJSON(vars.INPUTS) }}\n",
				} {
					if slices.Contains(wholeValueIDs(lintWholeValue(t, profile, tc.on+jobs+tc.step+w)), tc.id) {
						t.Errorf("%s: %s must not be reported for %q", profile, tc.id, w)
					}
				}
			}
		})
	}
}

// The workflows have no finding of the rules which make a claim about what the expression stands for. Every rule runs,
// at the most demanding profile.
func TestWholeValueExpressionsHaveNoFalsePositiveFromTheirRules(t *testing.T) {
	const src = `on:
  workflow_dispatch:
env:
  ${{ insert }}: ${{ fromJSON(vars.WORKFLOW_ENV) }}
concurrency: ${{ fromJSON(vars.CONCURRENCY) }}
permissions: {}
jobs:
  a:
    runs-on: ${{ matrix.os }}
    timeout-minutes: 5
    strategy: ${{ fromJSON(vars.STRATEGY) }}
    defaults:
      run: ${{ fromJSON(vars.RUN) }}
    container: ${{ fromJSON(vars.CONTAINER) }}
    services: ${{ fromJSON(vars.SERVICES) }}
    environment: ${{ fromJSON(vars.ENVIRONMENT) }}
    concurrency: ${{ fromJSON(vars.JOB_CONCURRENCY) }}
    env:
      ${{ insert }}: ${{ fromJSON(vars.JOB_ENV) }}
    steps:
      - run: |
          echo "${{ matrix.os }}"
          curl -fsSL https://example.com/install.sh | sh
        env:
          ${{ insert }}: ${{ fromJSON(vars.STEP_ENV) }}
      - run: echo hi
        shell: bash
`
	for _, profile := range []Profile{ProfileCorrectness, ProfileDefault, ProfilePedantic} {
		t.Run(string(profile), func(t *testing.T) {
			for _, e := range lintWholeValue(t, profile, src) {
				switch e.ID {
				case "workflow-syntax", "expression-type", "undefined-property", "context-availability", "concurrency-limits", "require-shell":
					t.Errorf("unexpected %s: %v", e.ID, e)
				}
			}
		})
	}
}

// A job whose shell comes from an expression is not a job with the default shell: the rules which look at the script
// must not judge the script by bash.
func TestDefaultsRunExpressionHidesTheShell(t *testing.T) {
	const src = `on: push
permissions: {}
jobs:
  a:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    defaults:
      run: ${{ fromJSON(vars.RUN) }}
    steps:
      - run: Get-ChildItem | Select-Object -First 1
`
	for _, profile := range []Profile{ProfileCorrectness, ProfileDefault, ProfilePedantic} {
		for _, e := range lintWholeValue(t, profile, src) {
			switch e.ID {
			case "shellcheck", "pipeline-without-pipefail", "require-shell", "workflow-syntax":
				t.Errorf("%s: unexpected %s: %v", profile, e.ID, e)
			}
		}
	}
}

// Expressions of the wrong type or in a place whose contexts do not allow them are still reported
func TestWholeValueExpressionsAreTypeChecked(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n"
	tests := []struct{ name, job, want string }{
		{"with is a string", "      - uses: actions/checkout@v4\n        with: ${{ github.sha }}\n", `type of expression at "with" must be object but found type string`},
		{"insert is a string", "      - run: x\n        env:\n          ${{ insert }}: ${{ github.sha }}\n", `type of expression at "${{ insert }}" must be object but found type string`},
		{"with uses an unavailable context", "      - uses: actions/checkout@v4\n        with: ${{ fromJSON(runner.os) }}\n", ""},
	}
	for _, tc := range tests[:2] {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintWholeValue(t, ProfileCorrectness, src+tc.job)
			if !slices.ContainsFunc(errs, func(e *Error) bool { return strings.Contains(e.Message, tc.want) }) {
				t.Errorf("want %q but got %v", tc.want, errs)
			}
		})
	}

	// the key "strategy" has no matrix and no steps contexts, "with" of a step has all of them
	errs := lintWholeValue(t, ProfileCorrectness, "on: push\njobs:\n  a:\n    runs-on: x\n    strategy: ${{ fromJSON(steps.x.outputs.y) }}\n    steps:\n      - uses: actions/checkout@v4\n        with: ${{ fromJSON(steps.x.outputs.y) }}\n")
	var ctx []int
	for _, e := range errs {
		if e.ID == "context-availability" {
			ctx = append(ctx, e.Line)
		}
	}
	if !slices.Equal(ctx, []int{5}) {
		t.Errorf("the context steps must be rejected in strategy (line 5) only: %v", errs)
	}
}

func TestMatrixOfAWholeStrategyIsUnknown(t *testing.T) {
	src := `on: push
jobs:
  a:
    strategy: ${{ fromJSON('{"matrix":{"os":["ubuntu-latest"]}}') }}
    runs-on: ${{ matrix.os }}
    steps:
      - run: echo "$X"
        env:
          X: ${{ matrix.deep.property }}
`
	if errs := lintWholeValue(t, ProfileCorrectness, src); len(errs) != 0 {
		t.Errorf("want no finding but got %v", errs)
	}
	// without the strategy the matrix is known to be empty
	errs := lintWholeValue(t, ProfileCorrectness, strings.Replace(src, `    strategy: ${{ fromJSON('{"matrix":{"os":["ubuntu-latest"]}}') }}`+"\n", "", 1))
	if !slices.Contains(wholeValueIDs(errs), "undefined-property") {
		t.Errorf("a job without a strategy has no matrix: %v", errs)
	}
}

func TestObjectAtRunsOnAndNames(t *testing.T) {
	src := `on: push
jobs:
  a:
    strategy:
      matrix:
        t:
          - runner: {group: g, labels: [x]}
            c: {group: a, cancel-in-progress: true}
            e: {name: staging}
    runs-on: ${{ matrix.t.runner }}
    concurrency: ${{ matrix.t.c }}
    environment: ${{ matrix.t.e }}
    steps:
      - run: echo
`
	if errs := lintWholeValue(t, ProfileCorrectness, src); len(errs) != 0 {
		t.Errorf("an object is a valid runs-on, concurrency and environment: %v", errs)
	}
	// An array is neither
	errs := lintWholeValue(t, ProfileCorrectness, "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        l: [[a, b]]\n    concurrency: ${{ matrix.l }}\n    steps:\n      - run: echo\n")
	if !slices.Contains(wholeValueIDs(errs), "expression-type") {
		t.Errorf("an array must not be a concurrency: %v", errs)
	}
}

func TestLiteralOfAnExpression(t *testing.T) {
	str := func(v string) *String { return &String{Value: v} }
	for _, tc := range []struct {
		v    string
		want string
		ok   bool
	}{
		{"max", "max", true},
		{"${{ 'max' }}", "max", true},
		{"${{'max'}}", "max", true},
		{" ${{ 'single' }} ", "single", true},
		{"${{ vars.Q }}", "", false},
		{"${{ 'a' }}${{ 'b' }}", "", false},
		{"x${{ 'a' }}", "", false},
		{"${{ true }}", "", false},
		{"${{ 'unterminated }}", "", false},
	} {
		got, ok := str(tc.v).literalString()
		if got != tc.want || ok != tc.ok {
			t.Errorf("literalString(%q) = %q, %v; want %q, %v", tc.v, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := (*String)(nil).literalString(); ok {
		t.Error("a nil string has no value")
	}
	if !(&Bool{Value: true}).literalTrue() || (&Bool{Value: false}).literalTrue() || (*Bool)(nil).literalTrue() {
		t.Error("a literal boolean")
	}
	if !(&Bool{Expression: str("${{ true }}")}).literalTrue() || (&Bool{Expression: str("${{ false }}")}).literalTrue() || (&Bool{Expression: str("${{ vars.B }}")}).literalTrue() {
		t.Error("a literal boolean in an expression")
	}
}

// Nothing may crash on the shapes whatever the profile is: the findings are not looked at, only that every rule and
// the fixers run through. The source is fuzzed in a loop by dropping the value of each key, so that a
// section is also a number, a list, an empty value or text.
func TestWholeValueExpressionsDoNotCrashRules(t *testing.T) {
	t.Parallel()
	const base = `on:
  pull_request_target:
env:
  ${{ insert }}: ${{ fromJSON(vars.WORKFLOW_ENV) }}
concurrency: ${{ fromJSON(vars.CONCURRENCY) }}
jobs:
  a:
    runs-on: ${{ fromJSON(vars.RUNNER) }}
    strategy: ${{ fromJSON(vars.STRATEGY) }}
    defaults:
      run: ${{ fromJSON(vars.RUN) }}
    container: ${{ fromJSON(vars.CONTAINER) }}
    services:
      s: ${{ fromJSON(vars.S) }}
      t:
        image: redis
        ports: ${{ fromJSON(vars.PORTS) }}
        volumes: ${{ fromJSON(vars.VOLUMES) }}
        env:
          ${{ insert }}: ${{ fromJSON(vars.SERVICE_ENV) }}
    environment: ${{ fromJSON(vars.ENVIRONMENT) }}
    concurrency: ${{ fromJSON(vars.JOB_CONCURRENCY) }}
    env:
      ${{ insert }}: ${{ fromJSON(vars.JOB_ENV) }}
    steps:
      - uses: actions/checkout@v4
        with: ${{ fromJSON(vars.CHECKOUT) }}
      - uses: actions/checkout@v4
        with:
          ${{ insert }}: ${{ fromJSON(vars.CHECKOUT) }}
          ref: ${{ github.event.pull_request.head.sha }}
      - uses: actions/cache@v4
        with: ${{ fromJSON(vars.CACHE) }}
      - uses: actions/setup-node@v4
        with: ${{ fromJSON(vars.NODE) }}
      - uses: actions/create-github-app-token@v2
        with: ${{ fromJSON(vars.APP) }}
      - uses: anthropics/claude-code-action@v1
        with: ${{ fromJSON(vars.AGENT) }}
      - uses: actions/github-script@v7
        with: ${{ fromJSON(vars.SCRIPT) }}
      - uses: docker://alpine:3.20
        with: ${{ fromJSON(vars.DOCKER) }}
      - run: echo ${{ github.event.pull_request.title }}
        env:
          ${{ insert }}: ${{ fromJSON(vars.STEP_ENV) }}
      - run: echo ${{ github.event.pull_request.title }}
        env: ${{ fromJSON(vars.STEP_ENV) }}
  b:
    uses: ./.github/workflows/none.yaml
    with:
      x: ${{ fromJSON(vars.X) }}
`
	srcs := []string{base}
	// Replace the value of each expression by a shape which is not the one the parser expects
	for _, repl := range []string{"1", "[a, b]", "{a: b}", "", "plain", "''"} {
		for i, line := range strings.Split(base, "\n") {
			// The last expression of the line is the value of the key, also after a "${{ insert }}" key
			k := strings.LastIndex(line, "${{")
			if k < 0 || !strings.HasSuffix(strings.TrimSpace(line[:k]), ":") {
				continue
			}
			lines := strings.Split(base, "\n")
			lines[i] = line[:k] + repl
			srcs = append(srcs, strings.Join(lines, "\n"))
		}
	}
	for _, profile := range []Profile{ProfileCorrectness, ProfileDefault, ProfilePedantic} {
		for _, src := range srcs {
			lintWholeValue(t, profile, src)
			cfg := mustParseConfig(t, "profile: "+string(profile)+"\nrules:\n  missing-permissions: error\n  missing-timeout:\n    default-minutes: 30\n  unused-ignore: error\n")
			fixWith(t, []byte(src), cfg, FixModeUnsafe)
		}
	}
}

// The fix of template-injection adds a variable to the env of the step. It does not when the env has variables which come
// from an expression: one of them can have the name the fix chooses, which GitHub rejects as defined twice.
func TestTemplateInjectionFixIsNotOfferedForDynamicEnv(t *testing.T) {
	const head = "on:\n  issues:\n    types: [opened]\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n"
	const run = "      - run: echo \"${{ github.event.issue.title }}\"\n"
	for name, env := range map[string]string{
		"whole":         "        env: ${{ fromJSON(vars.E) }}\n",
		"insert":        "        env:\n          ${{ insert }}: ${{ fromJSON(vars.E) }}\n",
		"insert, plain": "        env:\n          A: b\n          ${{ insert }}: ${{ fromJSON(vars.E) }}\n",
	} {
		found := false
		for _, e := range lintWholeValue(t, ProfilePedantic, head+run+env) {
			if e.ID == "template-injection" {
				found = true
				if e.Fix != nil {
					t.Errorf("%s: no fix is wanted: %+v", name, e.Fix)
				}
			}
		}
		if !found {
			t.Errorf("%s: the finding is still expected", name)
		}
	}
	// the control: a plain env gets the fix
	hasFix := false
	for _, e := range lintWholeValue(t, ProfilePedantic, head+run+"        env:\n          A: b\n") {
		hasFix = hasFix || (e.ID == "template-injection" && e.Fix != nil)
	}
	if !hasFix {
		t.Error("the control: a plain env gets the fix")
	}
}
