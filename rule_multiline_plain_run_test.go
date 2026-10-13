package jactionlint

import "testing"

const multilineID = "multiline-plain-run"

func TestMultilinePlainRun(t *testing.T) {
	cfg := ruleConfig(multilineID)
	step := func(run string) string {
		return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - " + run
	}
	tests := []struct {
		what  string
		src   string
		lines []int // the line of the first line which is a command of its own
	}{
		// the lines are commands of their own
		{"two commands", step("run: echo a\n          echo b\n"), []int{7}},
		{"the example of the issue", step("run: echo a\n          x=1\n          echo $x\n"), []int{7}},
		{"three commands", step("run: npm ci\n          npm run build\n          npm test\n"), []int{7}},
		{"cd then a command", step("run: cd app\n          npm ci\n"), []int{7}},
		{"on the next line of the key", step("name: x\n        run:\n          echo a\n          echo b\n"), []int{9}},
		{"double quoted", step("run: \"echo a\n          echo b\"\n"), []int{7}},
		{"single quoted", step("run: 'echo a\n          echo b'\n"), []int{7}},
		{"after a wrapped line", step("run: git commit -m\n          'x'\n          git push\n"), []int{8}},
		{"a command after the join", step("run: pip install\n          -r requirements.txt\n          pytest\n"), []int{8}},
		{"an assignment after echo", step("run: echo a\n          X=1\n"), []int{7}},
		{"on a windows runner with bash", "on: push\njobs:\n  a:\n    runs-on: windows-latest\n    steps:\n      - shell: bash\n        run: echo a\n          echo b\n", []int{8}},
		{"two groups, only the second is a bug", step("run: echo a\n\n          echo b\n          cat x\n"), []int{9}},

		// one command wrapped over several lines
		{"options", step("run: docker build\n          --tag x\n          --file Dockerfile\n          .\n"), nil},
		{"after a pipe", step("run: echo x |\n          sort\n"), nil},
		{"after an and", step("run: cd x &&\n          npm ci &&\n          npm test\n"), nil},
		{"after a backslash", step("run: echo x \\\n          && echo y\n"), nil},
		{"before an operator", step("run: echo x\n          && echo y\n"), nil},
		{"a redirect", step("run: echo x\n          >> $GITHUB_OUTPUT\n"), nil},
		{"make with variables", step("run: make test\n          CC=gcc exec=no\n"), nil},
		{"an expression over lines", step("run: echo ${{ a &&\n          b }}\n"), nil},
		{"an expression over lines starting with a context", step("run: node x.js \"${{\n          github.sha }}\"\n"), nil},
		{"a quote left open", step("run: echo \"one\n          echo two\"\n"), nil},
		{"a bracket left open", step("run: echo $(cat\n          git x)\n"), nil},
		{"a launcher", step("run: docker run --rm image\n          git status\n"), nil},
		{"ssh", step("run: ssh host\n          git status\n"), nil},
		{"sudo", step("run: sudo\n          apt-get install x\n"), nil},
		{"find", step("run: find . -name x\n          rm y\n"), nil},
		{"arguments that are paths", step("run: cp\n          a b\n"), nil},
		{"an unknown first word", step("run: ./build.sh\n          echo x\n"), nil},
		{"a script as the first word", step("run: scripts/x\n          git\n"), nil},
		{"a flag", step("run: git log\n          -n 3\n"), nil},
		{"a command in an argument", step("run: echo a b\n          c\n"), nil},
		{"make with a target", step("run: make\n          test\n"), nil},

		// not folded
		{"literal block", step("run: |\n          echo a\n          echo b\n"), nil},
		{"folded block", step("run: >\n          echo a\n          echo b\n"), nil},
		{"one line", step("run: echo a; echo b\n"), nil},
		{"python", step("run: echo a\n          echo b\n        shell: python\n"), nil},
		{"blank line keeps the line break", step("run: echo a\n\n          echo b\n"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), multilineID, tc.lines...)
		})
	}
}

func TestMultilinePlainRunFix(t *testing.T) {
	cfg := ruleConfig(multilineID)
	tests := []struct {
		what string
		in   string // the steps
		want string // "" when there is no fix
	}{
		{"plain", "      - run: echo a\n          x=1\n          echo $x\n", "      - run: |\n          echo a\n          x=1\n          echo $x\n"},
		{"deeper lines are aligned", "      - run: npm ci\n            npm test\n", "      - run: |\n          npm ci\n          npm test\n"},
		{"key without a dash", "      - name: x\n        run: echo a\n          echo b\n", "      - name: x\n        run: |\n          echo a\n          echo b\n"},
		{"double quoted", "      - run: \"echo a\n          echo b\"\n", "      - run: |\n          echo a\n          echo b\n"},
		{"single quoted with a quote", "      - run: 'echo ''a\n          echo b'\n", "      - run: |\n          echo 'a\n          echo b\n"},
		{"followed by a key", "      - run: echo a\n          echo b\n        env:\n          X: y\n", "      - run: |\n          echo a\n          echo b\n        env:\n          X: y\n"},
		{"more steps", "      - run: echo a\n          echo b\n      - run: echo c\n", "      - run: |\n          echo a\n          echo b\n      - run: echo c\n"},

		{"the value is on the next line", "      - run:\n          echo a\n          echo b\n", ""},
		{"a comment follows", "      - run: echo a\n          echo b # c\n", ""},
		{"escape in double quotes", "      - run: \"echo a\\tx\n          echo b\"\n", ""},
		{"blank line", "      - run: echo a\n          echo b\n\n          echo c\n", ""},
		{"flow mapping", "      - {run: \"echo a\n          echo b\"}\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			const head = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n"
			src := head + tc.in
			errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
			if len(errs) != 1 {
				t.Fatalf("want one finding: %v", errs)
			}
			if tc.want == "" {
				if errs[0].Fix != nil || errs[0].NoFix == nil {
					t.Fatalf("want no fix and a reason: %+v", errs[0])
				}
				return
			}
			out, left := fixAndLint(t, cfg, "ci.yaml", src, FixModeSafe)
			if out != head+tc.want {
				t.Fatalf("fixed to\n%s\nwant\n%s", out, head+tc.want)
			}
			wantLines(t, left, multilineID)
			// the value of the script has the lines of the scalar
			w, errs2 := Parse([]byte(out))
			if w == nil {
				t.Fatalf("the result does not parse: %v", errs2)
			}
		})
	}
}

func TestMultilinePlainRunPosition(t *testing.T) {
	cfg := ruleConfig(multilineID)
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"é\"\n          echo b\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
	if len(errs) != 1 || errs[0].Line != 7 || errs[0].Column != 11 {
		t.Fatalf("unexpected findings: %v", errs)
	}
}
