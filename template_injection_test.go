package jactionlint

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// tiConfig returns a configuration that turns on all template injection findings and nothing else
// that would add noise to the fixtures.
func tiConfig(t *testing.T) *Config {
	t.Helper()
	return mustParseConfig(t, "profile: pedantic\nrules:\n  missing-permissions: off\n  missing-timeout: off\n  anonymous-definition: off\n  require-shell: off\n  require-expression-wrapping: off\n  unpinned-uses: off\n")
}

func tiWorkflow(runs string) string {
	return "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + runs
}

func idsOf(errs []*Error, prefix string) []string {
	var ret []string
	for _, e := range errs {
		if strings.HasPrefix(e.ID, prefix) {
			ret = append(ret, e.ID)
		}
	}
	return ret
}

func TestTemplateInjectionTiers(t *testing.T) {
	tests := []struct {
		name string
		step string
		want []string
	}{
		{"direct", "      - run: echo '${{ github.event.issue.title }}'\n", []string{"template-injection"}},
		{"dispatch input named id", "      - run: echo '${{ github.event.inputs.id }}'\n", []string{"template-injection-expansion"}},
		{"dispatch input named sha", "      - run: echo '${{ github.event.inputs.sha }}'\n", []string{"template-injection-expansion"}},
		{"dispatch input named number", "      - run: echo '${{ github.event.inputs.number }}'\n", []string{"template-injection-expansion"}},
		{"client payload named sha", "      - run: echo '${{ github.event.client_payload.sha }}'\n", []string{"template-injection-expansion"}},
		{"event id", "      - run: echo '${{ github.event.pull_request.id }}'\n", []string{"template-injection-trusted"}},
		{"bracket access of the context", "      - run: echo '${{ github['event'].issue.title }}'\n", []string{"template-injection"}},
		{"bracket access all the way", "      - run: echo \"${{ github['event']['issue']['Title'] }}\"\n", []string{"template-injection"}},
		{"mixed-case property", "      - run: echo '${{ github.event.issue.Title }}'\n", []string{"template-injection"}},
		{"mixed-case index", "      - run: echo \"${{ github.event.issue['Title'] }}\"\n", []string{"template-injection"}},
		{"index", "      - run: echo \"${{ github.event.issue['title'] }}\"\n", []string{"template-injection"}},
		// Index access with a number or an expression is not a filter, and the case of names does not matter
		{"index with a number", "      - run: echo \"${{ github.event.commits[0].message }}\"\n", []string{"template-injection"}},
		{"index with a number and a mixed-case property", "      - run: echo \"${{ github.event.commits[0].Message }}\"\n", []string{"template-injection"}},
		{"index with an expression", "      - run: echo \"${{ github.event.commits[strategy.job-index].message }}\"\n", []string{"template-injection"}},
		{"index with an expression and a mixed-case property", "      - run: echo \"${{ github.event.commits[strategy.job-index].Message }}\"\n", []string{"template-injection"}},
		{"index with a matrix value", "      - run: echo \"${{ github.event.commits[matrix.i].Author.Email }}\"\n", []string{"template-injection"}},
		{"filter", "      - run: echo \"${{ github.event.commits.*.message }}\"\n", []string{"template-injection"}},
		{"filter with a mixed-case property", "      - run: echo \"${{ github.event.commits.*.Message }}\"\n", []string{"template-injection"}},
		{"mixed-case context and properties", "      - run: echo \"${{ GitHub.Event.Pull_Request.Head.REF }}\"\n", []string{"template-injection"}},
		{"mixed-case bracket after an index", "      - run: echo \"${{ github.event.commits[0]['Message'] }}\"\n", []string{"template-injection"}},
		{"every untrusted expression of a script", "      - run: |\n          echo '${{ github.head_ref }}'\n          echo '${{ github.event.issue.body }}'\n", []string{"template-injection", "template-injection"}},
		{"github-script", "      - uses: actions/github-script@v7\n        with:\n          script: console.log('${{ github.head_ref }}')\n", []string{"template-injection"}},
		{"code input of another action", "      - uses: nick-fields/retry@v3\n        with:\n          command: echo ${{ github.head_ref }}\n", []string{"template-injection"}},
		{"plain input of an action", "      - uses: actions/stale@v9\n        with:\n          stale-pr-message: ${{ github.head_ref }}\n", nil},
		{"object holding untrusted properties", "      - run: echo '${{ toJSON(github.event) }}'\n", []string{"template-injection"}},
		{"env set from untrusted", "      - run: echo '${{ env.TITLE }}'\n        env:\n          TITLE: ${{ github.event.issue.title }}\n", []string{"template-injection"}},
		{"undefined env variable", "      - run: echo '${{ env.TITLE }}'\n", []string{"template-injection-expansion"}},
		{"safe functions", "      - run: echo '${{ contains(github.event.issue.title, 'x') }}'\n", []string{"template-injection-trusted"}},
		{"shell variable", "      - run: echo \"$TITLE\"\n        env:\n          TITLE: ${{ github.event.issue.title }}\n", nil},
		{"free text input", "      - run: echo '${{ inputs.name }}'\n", []string{"template-injection-expansion"}},
		{"step output", "      - id: s\n        run: echo x\n      - run: echo '${{ steps.s.outputs.v }}'\n", []string{"template-injection-expansion"}},
		{"branch name", "      - run: echo '${{ github.ref_name }}'\n", []string{"template-injection-expansion"}},
		{"trusted", "      - run: echo '${{ github.repository }} ${{ runner.os }} ${{ secrets.TOKEN }}'\n", []string{"template-injection-trusted", "template-injection-trusted", "template-injection-trusted"}},
		{"tested only", "      - run: echo '${{ github.event_name == 'push' && 'a' || 'b' }}'\n", []string{"template-injection-trusted"}},
		{"and keeps only its right operand", "      - run: echo '${{ inputs.x && 'a' || 'b' }}'\n", []string{"template-injection-trusted"}},
		{"or can return its left operand", "      - run: echo '${{ inputs.x || 'b' }}'\n", []string{"template-injection-expansion"}},
		{"unknown context", "      - run: echo '${{ unknown.x }}'\n", nil},
		{"not a script", "      - name: ${{ github.event.issue.title }}\n        run: echo\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintWithConfig(t, tiConfig(t), tiWorkflow(tc.step))
			var got []string
			for _, e := range errs {
				if strings.HasPrefix(e.ID, "template-injection") {
					got = append(got, e.ID)
				}
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				for _, e := range errs {
					t.Log(e)
				}
				t.Errorf("(-want +got): %s", diff)
			}
		})
	}
}

func TestTemplateInjectionTrustedValues(t *testing.T) {
	src := `on:
  workflow_dispatch:
    inputs:
      flag:
        type: boolean
      pick:
        type: choice
        options: [a, b]
      text:
        type: string
      loose:
jobs:
  j:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        os: [a, b]
        dyn: ${{ fromJSON('[1]') }}
        include:
          - os: c
            extra: d
    env:
      LITERAL: value
      DYNAMIC: ${{ github.sha }}
    steps:
      - run: echo '${{ inputs.flag }} ${{ inputs.pick }} ${{ github.event.inputs.flag }}'
      - run: echo '${{ inputs.text }} ${{ inputs.loose }} ${{ inputs.undeclared }}'
      - run: echo '${{ matrix.os }} ${{ matrix.extra }}'
      - run: echo '${{ matrix.dyn }}'
      - run: echo '${{ env.LITERAL }}'
      - run: echo '${{ env.DYNAMIC }}'
      - run: echo '${{ needs.x.result }} ${{ steps.x.outcome }}'
`
	errs := lintWithConfig(t, tiConfig(t), src)
	var got []string
	for _, e := range errs {
		if strings.HasPrefix(e.ID, "template-injection") {
			got = append(got, e.ID+" "+strings.SplitN(e.Message, `"`, 3)[1])
		}
	}
	want := []string{
		"template-injection-trusted inputs.flag", "template-injection-trusted inputs.pick", "template-injection-trusted github.event.inputs.flag",
		"template-injection-expansion inputs.text", "template-injection-expansion inputs.loose", "template-injection-expansion inputs.undeclared",
		"template-injection-trusted matrix.os", "template-injection-trusted matrix.extra",
		"template-injection-expansion matrix.dyn",
		"template-injection-trusted env.LITERAL",
		"template-injection-expansion env.DYNAMIC",
		"template-injection-trusted needs.x.result", "template-injection-trusted steps.x.outcome",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		for _, e := range errs {
			t.Log(e)
		}
		t.Errorf("(-want +got): %s", diff)
	}
}

// fixAll applies fixes like -fix does: it lints, applies and lints again until nothing changes.
func fixAll(t *testing.T, cfg *Config, src string, mode FixMode) (string, []*Error) {
	t.Helper()
	for i := 0; i < 10; i++ {
		errs := lintWithConfig(t, cfg, src)
		out, n := applyFixes([]byte(src), errs, mode)
		if n == 0 {
			return src, errs
		}
		src = string(out)
	}
	t.Fatalf("fixing does not converge:\n%s", src)
	return "", nil
}

func TestTemplateInjectionFixes(t *testing.T) {
	tests := []struct {
		name   string
		step   string
		safe   string // the step after the safe fixes
		unsafe string // the step after all the fixes; empty when it is the same as safe
	}{
		{
			name: "double quotes with a new env",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "default environment variable",
			step: "      - run: echo \"${{ github.head_ref }} ${{ runner.temp }}\"\n",
			safe: "      - run: echo \"${GITHUB_HEAD_REF} ${RUNNER_TEMP}\"\n",
		},
		{
			name: "single quotes around the whole word",
			step: "      - run: echo '${{ github.event.issue.title }}'\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "single quotes in a longer word",
			step: "      - run: echo 'title=${{ github.event.issue.title }}!'\n",
			safe: "      - run: echo 'title='\"${ISSUE_TITLE}\"'!'\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name:   "unquoted needs the unsafe fix",
			step:   "      - run: echo ${{ github.event.issue.title }}\n",
			safe:   "      - run: echo ${{ github.event.issue.title }}\n",
			unsafe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "block scalar with several expressions and one variable",
			step: "      - name: x\n        run: |\n          echo \"${{ github.event.issue.title }}\"\n          echo \"title is ${{ github.event.issue.title }}\" \"${{ github.event.issue.body }}\"\n",
			safe: "      - name: x\n        run: |\n          echo \"${ISSUE_TITLE}\"\n          echo \"title is ${ISSUE_TITLE}\" \"${ISSUE_BODY}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n          ISSUE_BODY: ${{ github.event.issue.body }}\n",
		},
		{
			name: "yaml single quoted",
			step: "      - run: 'echo \"${{ github.event.issue.title }}\"'\n",
			safe: "      - run: 'echo \"${ISSUE_TITLE}\"'\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "workspace and runner paths",
			step: "      - run: echo \"${{ github.workspace }} ${{ runner.os }}\"\n",
			safe: "      - run: echo \"${GITHUB_WORKSPACE} ${RUNNER_OS}\"\n",
		},
		{
			name: "multi-byte characters before the expression",
			step: "      - run: |\n          echo \"\u3042\u3044 ${{ github.event.issue.title }}\"\n",
			safe: "      - run: |\n          echo \"\u3042\u3044 ${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "existing env block",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        env:\n          A: b\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n          A: b\n",
		},
		{
			name: "existing variable for the same expression",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        env:\n          T: ${{ github.event.issue.title }}\n",
			safe: "      - run: echo \"${T}\"\n        env:\n          T: ${{ github.event.issue.title }}\n",
		},
		{
			name: "name taken by another value",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        env:\n          ISSUE_TITLE: other\n",
			safe: "      - run: echo \"${ISSUE_TITLE_2}\"\n        env:\n          ISSUE_TITLE_2: ${{ github.event.issue.title }}\n          ISSUE_TITLE: other\n",
		},
		{
			name: "env variable read through the shell",
			step: "      - run: echo \"${{ env.TITLE }}\"\n        env:\n          TITLE: ${{ github.event.issue.title }}\n",
			safe: "      - run: echo \"${TITLE}\"\n        env:\n          TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "step with other keys after run",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: bash\n        name: x\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n        shell: bash\n        name: x\n",
		},
		{
			name: "a name before run and a following step",
			step: "      - name: a\n        run: |\n          echo \"${{ github.head_ref }} ${{ inputs.x }}\"\n\n      - run: echo ok\n",
			safe: "      - name: a\n        run: |\n          echo \"${GITHUB_HEAD_REF} ${INPUTS_X}\"\n        env:\n          INPUTS_X: ${{ inputs.x }}\n\n      - run: echo ok\n",
		},
		{
			name: "not provable: command substitution",
			step: "      - run: echo \"$(echo ${{ github.event.issue.title }})\"\n",
			safe: "      - run: echo \"$(echo ${{ github.event.issue.title }})\"\n",
		},
		{
			name: "not provable: function call",
			step: "      - run: echo \"${{ toJSON(github.event.issue) }}\"\n",
			safe: "      - run: echo \"${{ toJSON(github.event.issue) }}\"\n",
		},
		{
			name: "not provable: comment",
			step: "      - run: |\n          echo hi # ${{ github.event.issue.title }}\n",
			safe: "      - run: |\n          echo hi # ${{ github.event.issue.title }}\n",
		},
		{
			name: "pwsh is not fixed",
			step: "      - run: Write-Host \"${{ github.event.issue.title }}\"\n        shell: pwsh\n",
			safe: "      - run: Write-Host \"${{ github.event.issue.title }}\"\n        shell: pwsh\n",
		},
		{
			name: "explicit sh",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: sh\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n        shell: sh\n",
		},
		{
			name: "bash template",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: bash {0}\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n        shell: bash {0}\n",
		},
		{
			name: "bash with options",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: bash -eo pipefail {0}\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n        shell: bash -eo pipefail {0}\n",
		},
		{
			name: "bash with long options",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: bash --noprofile --norc -eo pipefail {0}\n",
			safe: "      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n        shell: bash --noprofile --norc -eo pipefail {0}\n",
		},
		{
			name: "a shell that is not understood",
			step: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: bash -c '{0}'\n",
			safe: "      - run: echo \"${{ github.event.issue.title }}\"\n        shell: bash -c '{0}'\n",
		},
		{
			// The quotes of the replacement are escaped for the double quoted YAML scalar
			name:   "yaml quoted string with quotes in the replacement",
			step:   "      - run: \"echo ${{ github.event.issue.title }}\"\n",
			safe:   "      - run: \"echo ${{ github.event.issue.title }}\"\n",
			unsafe: "      - run: \"echo \\\"${ISSUE_TITLE}\\\"\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name:   "single quoted yaml string",
			step:   "      - run: 'echo ${{ github.event.issue.title }}'\n",
			safe:   "      - run: 'echo ${{ github.event.issue.title }}'\n",
			unsafe: "      - run: 'echo \"${ISSUE_TITLE}\"'\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n",
		},
		{
			name: "yaml quoted string keeps working",
			step: "      - run: \"echo \\\"${{ github.event.issue.title }}\\\"\"\n",
			safe: "      - run: \"echo \\\"${{ github.event.issue.title }}\\\"\"\n", // escapes before the expression: left alone
		},
		{
			name:   "bracket test",
			step:   "      - run: '[[ \"${{ github.head_ref }}\" == main ]] && echo hi'\n",
			safe:   "      - run: '[[ \"${{ github.head_ref }}\" == main ]] && echo hi'\n",
			unsafe: "      - run: '[[ \"${GITHUB_HEAD_REF}\" == main ]] && echo hi'\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tiConfig(t)
			src := tiWorkflow(tc.step)
			safe, errs := fixAll(t, cfg, src, FixModeSafe)
			if diff := cmp.Diff(tiWorkflow(tc.safe), safe); diff != "" {
				t.Errorf("safe fix (-want +got): %s", diff)
			}
			for _, e := range errs {
				if e.Fix != nil && !e.Fix.Unsafe {
					t.Errorf("a safe fix remains after fixing: %v", e)
				}
				if e.Kind == "syntax-check" {
					t.Errorf("fixed workflow does not parse: %v", e)
				}
			}
			unsafe, errs := fixAll(t, cfg, src, FixModeUnsafe)
			want := tc.unsafe
			if want == "" {
				want = tc.safe
			}
			if diff := cmp.Diff(tiWorkflow(want), unsafe); diff != "" {
				t.Errorf("unsafe fix (-want +got): %s", diff)
			}
			for _, e := range errs {
				if e.Fix != nil {
					t.Errorf("a fix remains after fixing: %v", e)
				}
				if e.Kind == "syntax-check" {
					t.Errorf("fixed workflow does not parse: %v", e)
				}
			}
		})
	}
}

func TestTemplateInjectionFixIsOnlyForFindings(t *testing.T) {
	// The fix of the default findings must not rewrite expansions which the configuration does not report.
	src := tiWorkflow("      - run: echo \"${{ github.event.issue.title }} ${{ inputs.x }}\"\n")
	out, _ := fixAll(t, &Config{}, src, FixModeSafe)
	want := tiWorkflow("      - run: echo \"${ISSUE_TITLE} ${{ inputs.x }}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n")
	if diff := cmp.Diff(want, out); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestTemplateInjectionFixRunsOnWindows(t *testing.T) {
	src := "on: pull_request_target\njobs:\n  j:\n    runs-on: windows-latest\n    steps:\n      - run: echo \"${{ github.event.issue.title }}\"\n"
	out, errs := fixAll(t, tiConfig(t), src, FixModeUnsafe)
	if out != src {
		t.Errorf("a script of the default shell of Windows is changed: %s", out)
	}
	if len(idsOf(errs, "template-injection")) != 1 {
		t.Errorf("want the finding to remain: %v", errs)
	}
	bash := strings.Replace(src, "      - run:", "      - shell: bash\n        run:", 1)
	out, _ = fixAll(t, tiConfig(t), bash, FixModeSafe)
	if !strings.Contains(out, "${ISSUE_TITLE}") {
		t.Errorf("a bash script on Windows is not fixed: %s", out)
	}
}

func TestTemplateInjectionFixCRLF(t *testing.T) {
	src := strings.ReplaceAll(tiWorkflow("      - run: echo \"${{ github.event.issue.title }}\"\n"), "\n", "\r\n")
	out, _ := fixAll(t, tiConfig(t), src, FixModeSafe)
	want := strings.ReplaceAll(tiWorkflow("      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n"), "\n", "\r\n")
	if diff := cmp.Diff(want, out); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestTemplateInjectionFixNoTrailingNewline(t *testing.T) {
	src := strings.TrimSuffix(tiWorkflow("      - run: echo \"${{ github.event.issue.title }}\"\n"), "\n")
	out, _ := fixAll(t, tiConfig(t), src, FixModeSafe)
	want := strings.TrimSuffix(tiWorkflow("      - run: echo \"${ISSUE_TITLE}\"\n        env:\n          ISSUE_TITLE: ${{ github.event.issue.title }}\n"), "\n")
	if diff := cmp.Diff(want, out); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func FuzzTemplateInjectionFix(f *testing.F) {
	for _, s := range []string{
		"echo \"${{ github.event.issue.title }}\"",
		"echo '${{ github.head_ref }}' ${{ inputs.x }}\ncat <<EOF\n${{ github.sha }}\nEOF",
		"x=$(echo ${{ env.A }}) # ${{ github.actor }}",
		"[[ \"${{ github.ref_name }}\" == a ]] && echo '${{ github.event.issue.body }}'",
	} {
		f.Add(s, true)
		f.Add(s, false)
	}
	f.Fuzz(func(t *testing.T, script string, block bool) {
		if strings.ContainsAny(script, "\x00\r\t") || !utf8Valid(script) {
			return
		}
		var step string
		if block {
			step = "      - run: |\n          " + strings.ReplaceAll(script, "\n", "\n          ") + "\n"
		} else {
			if strings.ContainsAny(script, "\n#:'\"[]{},&*!|>%@`\\") {
				return // not a plain YAML scalar
			}
			step = "      - run: " + script + "\n"
		}
		src := tiWorkflow(step)
		cfg := &Config{Profile: ProfilePedantic}
		errs := lintWithConfig(t, cfg, src)
		for _, e := range errs {
			if e.Kind == "syntax-check" {
				return
			}
		}
		out, _ := applyFixes([]byte(src), errs, FixModeUnsafe)
		for _, e := range lintWithConfig(t, cfg, string(out)) {
			if e.Kind == "syntax-check" {
				t.Fatalf("the fix broke the workflow:\n%s\n--- became ---\n%s", src, out)
			}
		}
	})
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestTemplateInjectionFixContainerPaths(t *testing.T) {
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    container: alpine\n    steps:\n      - run: echo \"${{ github.workspace }} ${{ runner.os }}\"\n"
	out, _ := fixAll(t, tiConfig(t), src, FixModeSafe)
	want := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    container: alpine\n    steps:\n      - run: echo \"${WORKSPACE} ${RUNNER_OS}\"\n        env:\n          WORKSPACE: ${{ github.workspace }}\n"
	if diff := cmp.Diff(want, out); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestPosixShellTemplate(t *testing.T) {
	for shell, want := range map[string]bool{
		"bash": true, "sh": true, "BASH": true, "bash {0}": true, "bash -eo pipefail {0}": true,
		"bash --noprofile --norc -eo pipefail {0}": true, "/bin/bash -e {0}": true, "dash {0}": true, "zsh {0}": true,
		"": false, "pwsh": false, "pwsh -command \". '{0}'\"": false, "python {0}": false, "cmd /D /E:ON /V:OFF /S /C \"CALL \"{0}\"\"": false,
		"bash -c '{0}'": false, "bash {0} -e": false, "bash $X {0}": false, "bash; rm {0}": false, "bash | cat {0}": false,
	} {
		if got := posixShellTemplate(shell); got != want {
			t.Errorf("posixShellTemplate(%q) = %v, want %v", shell, got, want)
		}
	}
}

// An expression the fix cannot replace must not get an environment variable that nothing reads.
func TestTemplateInjectionFixAddsOnlyTheVariablesItUses(t *testing.T) {
	src := tiWorkflow("      - run: ${{ github.event.issue.title }} x ${{ github.event.pull_request.title }}\n")
	out, _ := fixAll(t, tiConfig(t), src, FixModeUnsafe)
	if strings.Contains(out, "ISSUE_TITLE:") {
		t.Errorf("ISSUE_TITLE is not read by the script:\n%s", out)
	}
	if !strings.Contains(out, "\"${PULL_REQUEST_TITLE}\"") || !strings.Contains(out, "PULL_REQUEST_TITLE: ${{") {
		t.Errorf("the expression that can be replaced is:\n%s", out)
	}
}
