package jactionlint

import "testing"

const delimID = "malformed-expression-delimiters"

func TestExpressionDelimiters(t *testing.T) {
	cfg := ruleConfig(delimID)
	step := func(rest string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - " + rest
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		// reported
		{"single brace in env", "on: push\nenv:\n  REF: ${ github.ref }\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", []int{3}},
		{"single brace in with", step("uses: actions/setup-node@v4\n        with:\n          node-version: ${ matrix.node }\n"), []int{8}},
		{"single brace with spaces inside", step("uses: actions/setup-node@v4\n        with:\n          node-version: ${matrix.node}\n"), []int{8}},
		{"single brace in a condition", step("run: echo\n        if: ${ github.ref == 'refs/heads/main' }\n"), []int{7}},
		{"single brace in a name", step("name: Deploy ${ github.sha }\n        run: echo\n"), []int{6}},
		{"single brace in a job output", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    outputs:\n      v: ${ steps.x.outputs.v }\n    steps:\n      - run: echo\n", []int{6}},
		{"single brace in a function call", step("run: echo\n        env:\n          A: ${ format('{0}-x', github.sha) }\n"), []int{8}},
		{"two in a value", step("run: echo\n        env:\n          A: ${ github.sha }-${ github.ref }\n"), []int{8, 8}},
		{"space after the dollar", step("run: echo\n        env:\n          T: $ {{ github.sha }}\n"), []int{8}},
		{"space between the braces", step("run: echo\n        env:\n          T: ${ { github.sha }}\n"), []int{8}},
		{"no dollar", step("run: echo\n        env:\n          T: \"{{ github.sha }}\"\n"), []int{8}},
		{"no dollar inside text", step("run: echo\n        env:\n          T: image-{{ github.sha }}-x\n"), []int{8}},
		{"no dollar with a function", step("run: echo\n        env:\n          T: image-{{ format('{0}', github.sha) }}\n"), []int{8}},
		{"in a multi-line plain value", step("uses: a/b@v1\n        with:\n          x: one\n            two ${ github.sha }\n"), []int{9}},
		{"reusable workflow input", "on: push\njobs:\n  a:\n    uses: ./.github/workflows/w.yaml\n    with:\n      x: ${ github.sha }\n", []int{6}},

		// not reported
		{"a normal expression", step("run: echo\n        env:\n          A: ${{ github.sha }}\n"), nil},
		{"a normal expression next to braces", step("run: echo\n        env:\n          A: \"{${{ github.sha }}}\"\n"), nil},
		{"shell variable in env", step("run: echo\n        env:\n          A: ${HOME}/x ${PATH}\n"), nil},
		{"shell syntax in run", step("run: echo ${ github.sha } ${HOME} {{ github.sha }} $ {{ x }}\n"), nil},
		{"shell syntax in a block of run", step("run: |\n          echo \"${github.sha}\"\n          echo {{ github.sha }}\n"), nil},
		{"go template", step("run: echo\n        env:\n          A: \"{{ .Id }} {{ .Values.x }} {{ range .Items }}\"\n"), nil},
		{"helm values", step("run: echo\n        env:\n          A: \"{{ Values.x }}\"\n"), nil},
		{"unknown name", step("run: echo\n        env:\n          A: ${ foo.bar } ${ github }\n"), nil},
		{"an expression in a string of an expression", step("run: echo\n        env:\n          A: ${{ format('{{ github.sha }}', 1) }}\n"), nil},
		{"script of github-script", step("uses: actions/github-script@v7\n        with:\n          script: |\n            const x = `${ github.sha } {{ github.sha }}`\n"), nil},
		{"camelCase code input", step("uses: a/b@v1\n        with:\n          afterBuild: echo ${ github.sha }\n"), nil},
		{"camelCase input that is not code", step("uses: a/b@v1\n        with:\n          runId: ${ github.run_id }\n"), []int{8}},
		{"command input", step("uses: a/b@v1\n        with:\n          run-command: echo ${ github.sha }\n"), nil},
		{"three braces", step("run: echo\n        env:\n          A: \"{{{ github.sha }}}\"\n"), nil},
		{"without closing braces", step("run: echo\n        env:\n          A: \"{{ github.sha\"\n"), nil},
		{"run-id is not code", step("uses: a/b@v1\n        with:\n          run-id: ${ github.run_id }\n"), []int{8}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), delimID, tc.lines...)
		})
	}
}

func TestExpressionDelimitersPedantic(t *testing.T) {
	cfg := ruleConfig(delimID)
	pedantic := ruleConfig(delimID)
	pedantic.Rules[delimID] = RuleConfig{Level: SeverityError, Options: map[string]any{"pedantic": true}}
	step := func(rest string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: a/b@v1\n        with:\n" + rest
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"variable", step("          path: $HOME/x\n"), []int{8}},
		{"braces", step("          path: ${HOME}/x\n"), []int{8}},
		{"workspace", step("          path: $GITHUB_WORKSPACE/x\n"), []int{8}},
		{"two", step("          path: $A:${B}\n"), []int{8, 8}},
		{"escaped dollar", step("          path: $$HOME \\$HOME\n"), nil},
		{"a number or a special variable", step("          path: $1 $? $(pwd) ${HOME:-x} $\n"), nil},
		{"expression", step("          path: ${{ env.HOME }}/x\n"), nil},
		{"input that holds code", step("          script: echo $HOME\n          args: $HOME\n          pre-run: $HOME\n"), nil},
		{"env of a step is not checked", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          P: $HOME\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), delimID)
			wantLines(t, lintFileWithConfig(t, pedantic, "ci.yaml", tc.src), delimID, tc.lines...)
		})
	}
}

func TestExpressionDelimitersFix(t *testing.T) {
	cfg := ruleConfig(delimID)
	const head = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n"
	tests := []struct {
		what string
		env  string
		want string // "" when there is no fix
	}{
		{"single brace", "          A: ${ github.ref }\n", "          A: ${{ github.ref }}\n"},
		{"single brace without spaces", "          A: x-${github.sha}-y\n", "          A: x-${{github.sha}}-y\n"},
		{"function", "          A: ${ format('{0}}', github.sha) }\n", "          A: ${{ format('{0}}', github.sha) }}\n"},
		{"double quoted", "          A: \"v-${ github.sha }\"\n", "          A: \"v-${{ github.sha }}\"\n"},
		{"single quoted", "          A: 'v-${ github.sha }'\n", "          A: 'v-${{ github.sha }}'\n"},
		{"space after the dollar", "          A: $ {{ github.sha }}\n", "          A: ${{ github.sha }}\n"},
		{"spaces after the dollar", "          A: $  {{ github.sha }}\n", "          A: ${{ github.sha }}\n"},
		{"space between the braces", "          A: ${ { github.sha }}\n", "          A: ${{ github.sha }}\n"},
		{"no dollar", "          A: x-{{ github.sha }}\n", "          A: x-${{ github.sha }}\n"},
		{"no dollar quoted", "          A: \"{{ github.sha }}\"\n", "          A: \"${{ github.sha }}\"\n"},
		{"escape in the text", "          A: \"\\u0041 ${ github.sha }\"\n", "          A: \"\\u0041 ${{ github.sha }}\"\n"},
		{"folded lines in the expression", "          A: x ${ github.sha ==\n            'a' }\n", ""},
		{"invalid expression", "          A: ${ github.sha ==}\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head + tc.env
			errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), delimID)
			if tc.want == "" {
				for _, e := range errs {
					if e.Fix != nil {
						t.Fatalf("a fix is not expected: %v", e.Fix)
					}
				}
				return
			}
			out, left := fixAndLint(t, cfg, "ci.yaml", src, FixModeSafe)
			if want := head + tc.want; out != want {
				t.Fatalf("fixed to\n%s\nwant\n%s", out, want)
			}
			wantLines(t, left, delimID)
		})
	}
}

func TestExpressionDelimitersNoFixReasons(t *testing.T) {
	cfg := ruleConfig(delimID)
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          A: ${ github.sha ==}\n          B: x ${ github.sha ==\n            'a' }\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), delimID)
	if len(errs) != 2 {
		t.Fatalf("want two findings: %v", errs)
	}
	for _, e := range errs {
		if e.Fix != nil || e.NoFix == nil || e.NoFix.Code != NoFixUnsupportedShape || e.NoFix.Reason == "" {
			t.Errorf("want a reason why there is no fix: %+v", e)
		}
	}
}

func TestExpressionDelimitersPositionWithNonASCII(t *testing.T) {
	cfg := ruleConfig(delimID)
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          A: \"é-é ${ github.sha }\"\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), delimID)
	if len(errs) != 1 || errs[0].Line != 8 || errs[0].Column != 19 {
		t.Fatalf("unexpected findings: %v", errs)
	}
}
