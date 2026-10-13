package jactionlint

import (
	"strings"
	"testing"
)

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

		// the end of the line is a word, not a keyword
		{"a line that ends with in", step("run: git checkout main\n          npm ci\n"), []int{7}},
		{"a path that ends with in", step("run: ls /usr/bin\n          cat x\n"), []int{7}},
		{"a line that ends with do", step("run: git push origin\n          npm test\n"), []int{7}},
		{"assignments before a command", step("run: FOO=1 npm ci\n          npm test\n"), []int{7}},
		{"a wrapper before a command", step("run: time npm ci\n          npm test\n"), []int{7}},

		// the command is run by a launcher
		{"an assignment before sudo", step("run: DEBIAN_FRONTEND=noninteractive sudo\n          apt-get install x\n"), nil},
		{"an assignment before docker", step("run: FOO=1 docker run --rm image\n          git status\n"), nil},
		{"env before ssh", step("run: env FOO=1 ssh host\n          git status\n"), nil},
		{"time before docker", step("run: time docker run x\n          git status\n"), nil},
		{"export takes assignments", step("run: export A=1\n          B=2\n"), nil},
		{"unset takes names", step("run: unset A\n          B=2\n"), nil},

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
		{"plain", "      - run: echo a\n          npm ci\n          npm test\n", "      - run: |\n          echo a\n          npm ci\n          npm test\n"},
		{"a quote on a line of its own", "      - run: \"\n          echo a\n          npm ci\n          \"\n", "      - run: |\n          echo a\n          npm ci\n"},
		{"deeper lines are aligned", "      - run: npm ci\n            npm test\n", "      - run: |\n          npm ci\n          npm test\n"},
		{"key without a dash", "      - name: x\n        run: echo a\n          echo b\n", "      - name: x\n        run: |\n          echo a\n          echo b\n"},
		{"double quoted", "      - run: \"echo a\n          echo b\"\n", "      - run: |\n          echo a\n          echo b\n"},
		{"single quoted with a quote", "      - run: 'echo ''a\n          echo b'\n", "      - run: |\n          echo 'a\n          echo b\n"},
		{"followed by a key", "      - run: echo a\n          echo b\n        env:\n          X: y\n", "      - run: |\n          echo a\n          echo b\n        env:\n          X: y\n"},
		{"more steps", "      - run: echo a\n          echo b\n      - run: echo c\n", "      - run: |\n          echo a\n          echo b\n      - run: echo c\n"},

		{"a declaration with more assignments", "      - run: export A=1\n          B=2\n          echo x\n", ""},
		{"a wrapped line next to commands", "      - run: make test\n          CC=gcc\n          echo x\n", ""},
		{"a wrapped argument next to commands", "      - run: git commit -m\n          'x'\n          git push\n", ""},
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

func TestMultilinePlainRunMessage(t *testing.T) {
	cfg := ruleConfig(multilineID)
	tests := []struct {
		what    string
		run     string
		want    string
		notWant string
	}{
		{"folded text", "echo a\n          echo b", "runs the single command `echo a echo b`", ""},
		{"single quoted quote", "'echo ''a'' b\n          echo c'", "`echo 'a' b echo c`", "''"},
		{"blank line", "echo a\n\n          echo b\n          echo c", "joins the lines of each group between blank lines into one line", "folds its"},
		{"escape", "\"echo a\\tb\n          echo c\"", "the double quoted scalar has escapes", "blank line"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + tc.run + "\n"
			errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
			if len(errs) != 1 {
				t.Fatalf("want one finding: %v", errs)
			}
			if !strings.Contains(errs[0].Message, tc.want) || (tc.notWant != "" && strings.Contains(errs[0].Message, tc.notWant)) {
				t.Errorf("message is %q", errs[0].Message)
			}
		})
	}
}

func TestMultilinePlainRunNoFixReasons(t *testing.T) {
	cfg := ruleConfig(multilineID)
	tests := []struct {
		what string
		run  string
		want string
	}{
		{"blank line", "echo a\n\n          echo b\n          echo c", "the script has a blank line, which YAML keeps as a line break, so the fix is not exact. add `|` by hand"},
		{"lines that continue", "make test\n          CC=gcc\n          echo x", fixShapeReason},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + tc.run + "\n"
			errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
			if len(errs) != 1 || errs[0].Fix != nil || errs[0].NoFix == nil || errs[0].NoFix.Reason != tc.want {
				t.Fatalf("want no fix with the reason %q: %+v", tc.want, errs)
			}
		})
	}
}

const fixShapeReason = "the fix is offered only when every line is a complete command that starts with a well-known command (no assignment, operator, open quote or bracket, heredoc or backslash), and this script is not like that. add `|` after `run:` and put each command on its own line by hand"

// The finding for each of them is right or not reported; the fix refuses them, which is better than a wrong rewrite.
func TestMultilinePlainRunMustNotBeFixed(t *testing.T) {
	cfg := ruleConfig(multilineID)
	tests := []struct{ what, run string }{
		{"an assignment line", "echo a\n          x=1\n          echo $x"},
		{"an assignment before the command", "FOO=1 npm ci\n          npm test"},
		{"a declaration", "export A=1\n          B=2\n          echo x"},
		{"an open quote in a line", "echo \"a\n          npm ci\n          npm test"},
		{"an open bracket", "echo $(cat\n          npm ci\n          npm test)"},
		{"an open substitution", "echo `cat\n          npm ci`\n          npm test"},
		{"a heredoc", "cat <<EOF\n          npm ci\n          npm test"},
		{"a backslash at the end", "echo a \\\n          npm ci\n          npm test"},
		{"an operator at the end", "echo a &&\n          npm ci\n          npm test"},
		{"a keyword at the end", "echo a; then\n          npm ci\n          npm test"},
		{"a command that is not in the list", "echo a\n          ./build.sh\n          npm test"},
		{"a shell", "echo a\n          bash x.sh\n          npm test"},
		{"a wrapper", "echo a\n          time npm ci\n          npm test"},
		{"an escape", "\"echo a\\tb\n          npm ci\n          npm test\""},
		{"a comment", "echo a\n          npm ci # x\n          npm test"},
		{"a blank line", "echo a\n\n          npm ci\n          npm test"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + tc.run + "\n"
			for _, e := range errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID) {
				if e.Fix != nil {
					t.Fatalf("the fix must be refused: %+v", e.Fix)
				}
				if e.NoFix == nil || e.NoFix.Reason == "" {
					t.Fatalf("a finding without a fix says why: %+v", e)
				}
			}
		})
	}
}

// An assignment-only line gives its variables to the command of the next line, so it does not start a command.
func TestMultilinePlainRunAssignmentOnlyLine(t *testing.T) {
	cfg := ruleConfig(multilineID)
	for _, run := range []string{"FOO=1\n          make", "FOO=1 BAR=2\n          npm ci", "export A=1\n          B=2"} {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + run + "\n"
		wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
	}
}

// What is left open at the end of a line belongs to the next line: a command substitution with backticks or $( ).
func TestMultilinePlainRunOpenSubstitution(t *testing.T) {
	cfg := ruleConfig(multilineID)
	for _, run := range []string{"echo `date\n          npm ci`", "echo $(date\n          npm ci)", "echo ${{ a\n          npm ci }}"} {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + run + "\n"
		wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
	}
}

// A line that only holds the quote of a quoted scalar is folded into a space, it is not a blank line.
func TestMultilinePlainRunQuoteOnlyLine(t *testing.T) {
	cfg := ruleConfig(multilineID)
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"\n          echo a\n          npm ci\n          \"\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
	if len(errs) != 1 {
		t.Fatalf("want one finding: %v", errs)
	}
	if strings.Contains(errs[0].Message, "blank line") || !strings.Contains(errs[0].Message, "`echo a npm ci`") {
		t.Errorf("the message is about a blank line, or does not show the command: %s", errs[0].Message)
	}
	if errs[0].Fix == nil {
		t.Errorf("the fix is expected: %+v", errs[0].NoFix)
	}
	// a real blank line is one
	src = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"echo a\n\n          npm ci\n          npm test\"\n"
	errs = errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), multilineID)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "blank line") || errs[0].Fix != nil {
		t.Errorf("a blank line is described and not fixed: %v", errs)
	}
}
