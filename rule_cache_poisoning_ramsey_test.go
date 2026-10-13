package jactionlint

import (
	"io"
	"path/filepath"
	"testing"
)

// ramsey/composer-install reads ignore-cache with a list of values of its own (bin/should_cache.sh: 1, yes, Yes, y, Y, true,
// True), in the case it has, and that list decides in a composite action as well as in a step (Bugbot on #216).
func TestCachePoisoningRamseyIgnoreCacheInACompositeAction(t *testing.T) {
	const action = "name: x\ndescription: d\ninputs:\n  ignore:\n    description: c\nruns:\n  using: composite\n  steps:\n    - uses: ramsey/composer-install@v3\n      with:\n        ignore-cache: ${{ inputs.ignore }}\n"
	const on = "on:\n  release:\n    types: [published]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/x\n"
	with := func(v string) string { return on + "        with:\n          ignore: " + v + "\n" }
	tests := []struct {
		name   string
		caller string
		want   int // findings: the action restores the cache unless every release caller turns it off
	}{
		{"yes turns it off", with("yes"), 0},
		{"Yes turns it off", with("'Yes'"), 0},
		{"1 turns it off", with("1"), 0},
		{"y turns it off", with("y"), 0},
		{"true turns it off", with("'true'"), 0},
		{"True turns it off", with("'True'"), 0},
		{"TRUE does not: the action reads true and True", with("'TRUE'"), 1},
		{"no does not", with("'no'"), 1},
		{"false does not", with("'false'"), 1},
		{"an expression which is true on a release", with("${{ github.event_name == 'release' }}"), 0},
		{"an expression which is false on a release", with("${{ github.event_name != 'release' }}"), 1},
		{"an expression of the string yes", with("${{ 'yes' }}"), 0},
		{"an expression of the string TRUE", with("${{ 'TRUE' }}"), 1},
		{"an expression which is unknown", with("${{ vars.IGNORE }}"), 1},
		{"the default of the action", on, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeProject(t, map[string]string{
				".github/workflows/ci.yml":     tc.caller,
				".github/actions/x/action.yml": action,
			})
			l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  cache-poisoning: warn\n")
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "actions", "x", "action.yml")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := policyErrorsOf(errs, "cache-poisoning"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}

	// in a step of a workflow the expression of the input is judged by the same list
	steps := []struct {
		name  string
		value string
		want  int
	}{
		{"a step with an expression of yes", "${{ 'yes' }}", 0},
		{"a step with an expression of TRUE", "${{ 'TRUE' }}", 1},
		{"a step with an expression which is true on the release", "${{ github.event_name == 'release' }}", 0},
	}
	for _, tc := range steps {
		t.Run(tc.name, func(t *testing.T) {
			src := "on:\n  release:\n    types: [published]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ramsey/composer-install@v3\n        with:\n          ignore-cache: " + tc.value + "\n"
			if got := cacheCount(t, false, src); got != tc.want {
				t.Errorf("want %d findings, got %d", tc.want, got)
			}
		})
	}
}
