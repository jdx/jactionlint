package jactionlint

import (
	"io"
	"strings"
	"testing"
)

func lintTemplateInjection(t *testing.T, name, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = mustParseConfig(t, "profile: correctness\nrules:\n  template-injection: error\n  local-action-checkout: off\n")
	errs, err := l.Lint(name, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return policyErrorsOf(errs, "template-injection")
}

// The fix reads the variable which the runner exports, and the runner exports the key as it is spelled.
func TestTemplateInjectionFixUsesTheSpellingOfTheEnvKey(t *testing.T) {
	const head = "on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n"
	tests := []struct {
		name string
		src  string
		want string // text of the fixed run, "" for no fix
	}{
		{"the key spelled in mixed case", head + "    env:\n      Foo: ${{ github.event.issue.title }}\n    steps:\n      - run: X=${{ env.FOO }}\n", `X="${Foo}"`},
		{"the key spelled in upper case", head + "    env:\n      FOO: ${{ github.event.issue.title }}\n    steps:\n      - run: X=${{ env.foo }}\n", `X="${FOO}"`},
		{"levels which spell the key differently", head + "    env:\n      Foo: ${{ github.event.issue.title }}\n    steps:\n      - run: X=${{ env.FOO }}\n        env:\n          FOO: ${{ github.event.issue.title }}\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintTemplateInjection(t, "test.yaml", tc.src)
			out, n := applyFixes([]byte(tc.src), errs, FixModeUnsafe)
			if tc.want == "" {
				if n != 0 {
					t.Errorf("want no fix but got\n%s", out)
				}
				return
			}
			if !strings.Contains(string(out), tc.want) {
				t.Errorf("want %s in\n%s", tc.want, out)
			}
		})
	}
}

// In a composite action every input is text: the whole object is reported like inputs.title is.
func TestTemplateInjectionReportsTheInputsObjectOfAnAction(t *testing.T) {
	const action = "name: x\ndescription: d\ninputs:\n  title:\n    description: t\nruns:\n  using: composite\n  steps:\n    - run: echo \"${{ inputs }}\"\n      shell: bash\n"
	if got := lintTemplateInjection(t, "action.yml", action); len(got) != 1 {
		t.Errorf("want 1 finding but got %v", got)
	}
}

// A composite action which declares no input has nothing in inputs that the caller controls.
func TestTemplateInjectionIgnoresTheInputsObjectOfAnActionWithoutInputs(t *testing.T) {
	const action = "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - run: echo \"${{ inputs }}\"\n      shell: bash\n"
	if got := lintTemplateInjection(t, "action.yml", action); len(got) != 0 {
		t.Errorf("want no finding but got %v", got)
	}
}
