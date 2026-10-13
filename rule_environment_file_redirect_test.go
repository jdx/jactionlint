package jactionlint

import "testing"

const envFileID = "environment-file-redirect"

func TestEnvironmentFileRedirect(t *testing.T) {
	cfg := ruleConfig(envFileID)
	step := func(run string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + run
	}
	block := func(lines ...string) string {
		s := "|\n"
		for _, l := range lines {
			s += "          " + l + "\n"
		}
		return s
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		// the word is a file name
		{"append to GITHUB_OUTPUT", step("echo \"a=b\" >> GITHUB_OUTPUT\n"), []int{6}},
		{"truncate GITHUB_ENV", step("echo \"a=b\" > GITHUB_ENV\n"), []int{6}},
		{"GITHUB_PATH", step("echo /x >> GITHUB_PATH\n"), []int{6}},
		{"GITHUB_STEP_SUMMARY", step("echo '# t' >> GITHUB_STEP_SUMMARY\n"), []int{6}},
		{"GITHUB_STATE", step("echo a=b >> GITHUB_STATE\n"), []int{6}},
		{"quoted word", step("echo a=b >> \"GITHUB_OUTPUT\"\n"), []int{6}},
		{"single quoted word", step("echo a=b >> 'GITHUB_OUTPUT'\n"), []int{6}},
		{"stderr", step("echo a=b 2>> GITHUB_OUTPUT\n"), []int{6}},
		{"tee", step("echo a=b | tee -a GITHUB_OUTPUT\n"), []int{6}},
		{"tee without -a", step("echo a=b | tee GITHUB_ENV\n"), []int{6}},
		{"group", step(block("{ echo a=b; echo c=d; } >> GITHUB_OUTPUT")), []int{7}},
		{"heredoc", step(block("cat <<EOF >> GITHUB_OUTPUT", "a=b", "EOF")), []int{7}},
		{"in a condition", step(block("if true; then", "  echo a=b >> GITHUB_OUTPUT", "fi")), []int{8}},
		{"two", step(block("echo a=b >> GITHUB_OUTPUT", "echo c=d >> GITHUB_ENV")), []int{7, 8}},
		{"explicit sh", "on: push\njobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - shell: bash\n        run: echo a=b >> GITHUB_OUTPUT\n", []int{7}},

		// the variable
		{"variable", step("echo a=b >> $GITHUB_OUTPUT\n"), nil},
		{"quoted variable", step("echo a=b >> \"$GITHUB_OUTPUT\"\n"), nil},
		{"braces", step("echo a=b >> \"${GITHUB_OUTPUT}\"\n"), nil},
		{"every file", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo a=b >> \"$GITHUB_ENV\"", "echo /x >> \"$GITHUB_PATH\"", "echo t >> \"$GITHUB_STEP_SUMMARY\"", "echo a=b >> \"$GITHUB_STATE\"")), nil},
		{"another name", step("echo a=b >> GITHUB_OUTPUT.txt\n"), nil},
		{"a path", step("echo a=b >> ./GITHUB_OUTPUT\n"), nil},
		{"argument, not a redirect", step("echo GITHUB_OUTPUT >> out.txt\n"), nil},
		{"comment", step(block("# echo a=b >> GITHUB_OUTPUT", "echo x")), nil},
		{"text", step("echo \"a >> GITHUB_OUTPUT\"\n"), nil},
		{"read", step("cat < GITHUB_OUTPUT\n"), nil},
		{"powershell", "on: push\njobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - run: echo \"a=b\" >> $env:GITHUB_OUTPUT\n", nil},
		{"pwsh with the bare name", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - shell: pwsh\n        run: echo a=b >> GITHUB_OUTPUT\n", nil},
		{"cmd", "on: push\njobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - shell: cmd\n        run: echo a=b >> %GITHUB_OUTPUT%\n", nil},

		// a > that loses what was written
		{"truncation after an append", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d > \"$GITHUB_OUTPUT\"")), []int{8}},
		{"truncation after a truncation", step(block("echo a=b > \"$GITHUB_ENV\"", "echo c=d > \"$GITHUB_ENV\"")), []int{8}},
		{"truncation with braces", step(block("echo a=b >> ${GITHUB_OUTPUT}", "echo c=d > ${GITHUB_OUTPUT}")), []int{8}},
		{"truncation in a branch after an append", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "if [ -n \"$X\" ]; then", "  echo c=d > \"$GITHUB_OUTPUT\"", "fi")), []int{9}},
		{"tee truncates", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d | tee \"$GITHUB_OUTPUT\"")), []int{8}},
		{"group truncates", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "{ echo c=d; echo e=f; } > \"$GITHUB_OUTPUT\"")), []int{8}},
		{"three, the second truncates and the third too", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d > \"$GITHUB_OUTPUT\"", "echo e=f > \"$GITHUB_OUTPUT\"")), []int{8, 9}},

		// the same, which is not a loss
		{"lone truncation", step("echo a=b > \"$GITHUB_OUTPUT\"\n"), nil},
		{"appends", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d >> \"$GITHUB_OUTPUT\"")), nil},
		{"tee -a", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d | tee -a \"$GITHUB_OUTPUT\"")), nil},
		{"other files", step(block("echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d > \"$GITHUB_ENV\"")), nil},
		{"the truncation comes first", step(block("echo a=b > \"$GITHUB_OUTPUT\"", "echo c=d >> \"$GITHUB_OUTPUT\"")), nil},
		{"exclusive branches", step(block("if [ -n \"$X\" ]; then", "  echo a=b >> \"$GITHUB_OUTPUT\"", "else", "  echo c=d > \"$GITHUB_OUTPUT\"", "fi")), nil},
		{"the earlier write is in a function", step(block("f() { echo a=b >> \"$GITHUB_OUTPUT\"; }", "echo c=d > \"$GITHUB_OUTPUT\"")), nil},
		{"the variable is set by the script", step(block("GITHUB_OUTPUT=out.txt", "echo a=b >> \"$GITHUB_OUTPUT\"", "echo c=d > \"$GITHUB_OUTPUT\"")), nil},
		{"two steps", step(block("echo a=b >> \"$GITHUB_OUTPUT\"") + "      - run: echo c=d > \"$GITHUB_OUTPUT\"\n"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), envFileID, tc.lines...)
		})
	}
}

func TestEnvironmentFileRedirectFix(t *testing.T) {
	cfg := ruleConfig(envFileID)
	tests := []struct {
		what   string
		run    string
		safe   string // the result of the safe fixes, "" when there is no fix
		unsafe string // the result of all the fixes, "" when it is the same
	}{
		{"bare word", "echo a=b >> GITHUB_OUTPUT", "echo a=b >> $GITHUB_OUTPUT", ""},
		{"double quoted", "echo a=b >> \"GITHUB_ENV\"", "echo a=b >> \"$GITHUB_ENV\"", ""},
		{"truncation", "|\n          echo a=b >> \"$GITHUB_OUTPUT\"\n          echo c=d > \"$GITHUB_OUTPUT\"", "|\n          echo a=b >> \"$GITHUB_OUTPUT\"\n          echo c=d > \"$GITHUB_OUTPUT\"", "|\n          echo a=b >> \"$GITHUB_OUTPUT\"\n          echo c=d >> \"$GITHUB_OUTPUT\""},
		{"single quoted", "echo a=b >> 'GITHUB_OUTPUT'", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + tc.run + "\n"
			for mode, want := range map[FixMode]string{FixModeSafe: tc.safe, FixModeUnsafe: tc.unsafe} {
				if want == "" {
					want = tc.safe
				}
				if want == "" {
					want = tc.run
				}
				out, left := fixAndLint(t, cfg, "ci.yaml", src, mode)
				wantSrc := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + want + "\n"
				if out != wantSrc {
					t.Errorf("mode %d: fixed to\n%s\nwant\n%s", mode, out, wantSrc)
				}
				if mode == FixModeUnsafe || tc.what != "truncation" {
					if tc.safe != "" || mode == FixModeUnsafe {
						if n := len(errsWithID(left, envFileID)); n != 0 && tc.what != "single quoted" {
							t.Errorf("mode %d: %d findings are left: %v", mode, n, left)
						}
					}
				}
			}
		})
	}
}

func TestEnvironmentFileRedirectPositions(t *testing.T) {
	cfg := ruleConfig(envFileID)
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo \"é\" >> GITHUB_OUTPUT\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), envFileID)
	if len(errs) != 1 || errs[0].Line != 7 || errs[0].Column != 23 || errs[0].EndLine != 7 || errs[0].EndColumn != 36 {
		t.Fatalf("unexpected findings: %v", errs)
	}
	if errs[0].Fix == nil {
		t.Errorf("the fix is expected")
	}
}
