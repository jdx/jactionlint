package jactionlint

import "testing"

func TestInsecureCommands(t *testing.T) {
	cfg := ruleConfig("insecure-commands")
	step := func(env string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n" + env
	}
	tests := []struct {
		what  string
		src   string
		lines []int
		fixed string // the source after -fix=unsafe, empty when there is no fix
	}{
		{"true", step("        env:\n          ACTIONS_ALLOW_UNSECURE_COMMANDS: true\n"), []int{8}, step("")},
		{"1 does not enable the commands", step("        env:\n          ACTIONS_ALLOW_UNSECURE_COMMANDS: '1'\n"), nil, ""},
		{"lower case name and True", step("        env:\n          actions_allow_unsecure_commands: TRUE\n"), []int{8}, step("")},
		{"false", step("        env:\n          ACTIONS_ALLOW_UNSECURE_COMMANDS: false\n"), nil, ""},
		{"an expression is unknown", step("        env:\n          ACTIONS_ALLOW_UNSECURE_COMMANDS: ${{ inputs.x }}\n"), nil, ""},
		{
			"one of several entries",
			step("        env:\n          A: a\n          ACTIONS_ALLOW_UNSECURE_COMMANDS: true # opt in\n          B: b\n"),
			[]int{9},
			step("        env:\n          A: a\n          B: b\n"),
		},
		{
			"workflow and job",
			"on: push\nenv:\n  ACTIONS_ALLOW_UNSECURE_COMMANDS: true\njobs:\n  a:\n    env:\n      ACTIONS_ALLOW_UNSECURE_COMMANDS: true\n      K: v\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
			[]int{3, 7},
			"on: push\njobs:\n  a:\n    env:\n      K: v\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
		},
		{"flow mapping has no fix", step("        env: {ACTIONS_ALLOW_UNSECURE_COMMANDS: true}\n"), []int{7}, ""},
		{"quoted key", step("        env:\n          \"ACTIONS_ALLOW_UNSECURE_COMMANDS\": true\n          K: v\n"), []int{8}, step("        env:\n          K: v\n")},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintFileWithConfig(t, cfg, "ci.yaml", tc.src)
			wantLines(t, errs, "insecure-commands", tc.lines...)
			if tc.fixed == "" {
				for _, e := range errsWithID(errs, "insecure-commands") {
					if e.Fix != nil {
						t.Errorf("unexpected fix: %+v", e.Fix)
					}
				}
				return
			}
			for _, e := range errsWithID(errs, "insecure-commands") {
				if e.Fix == nil || !e.Fix.Unsafe {
					t.Errorf("want an unsafe fix but got %+v", e.Fix)
				}
			}
			if got, _ := fixAndLint(t, cfg, "ci.yaml", tc.src, FixModeSafe); got != tc.src {
				t.Errorf("the safe mode must not apply the fix: %q", got)
			}
			got, after := fixAndLint(t, cfg, "ci.yaml", tc.src, FixModeUnsafe)
			if got != tc.fixed {
				t.Errorf("fixed source mismatch\nwant: %q\nhave: %q", tc.fixed, got)
			}
			if len(after) != 0 {
				t.Errorf("the fixed source is not clean: %v", after)
			}
		})
	}
}
