package jactionlint

import (
	"io"
	"strings"
	"testing"
)

func lintShellSyntax(t *testing.T, src string, opts *LinterOptions, cfg *Config) []*Error {
	t.Helper()
	if opts == nil {
		opts = &LinterOptions{}
	}
	l, err := NewLinter(io.Discard, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		cfg = fixtureConfig("shell-syntax")
	}
	l.defaultConfig = cfg
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	var ret []*Error
	for _, e := range errs {
		if e.ID == "shell-syntax" {
			ret = append(ret, e)
		}
	}
	return ret
}

// syntaxJob is a workflow with one job on the runner and one step: the lines of the step are given after "- ".
func syntaxJob(runsOn, step string) string {
	return "on: push\njobs:\n  j:\n    runs-on: " + runsOn + "\n    steps:\n      - " + step
}

func TestRuleShellSyntaxReported(t *testing.T) {
	tests := []struct {
		what string
		src  string
		line int
		want string
	}{
		{"unclosed double quote", syntaxJob("ubuntu-latest", "run: |\n          echo hi\n          echo \"oops\n"), 8, "reached EOF without closing quote"},
		{"if without fi", syntaxJob("ubuntu-latest", "run: |\n          if true; then\n            echo hi\n"), 7, "`if` statement must end with `fi`"},
		{"stray fi", syntaxJob("ubuntu-latest", "run: |\n          echo a\n          fi\n"), 8, "`fi` can only be used to end an `if`"},
		{"for without done", syntaxJob("macos-latest", "run: |\n          for i in 1 2; do\n            echo $i\n"), 7, "`for` statement must end with `done`"},
		{"unclosed subshell", syntaxJob("ubuntu-latest", "run: echo $(date\n"), 6, ""},
		{"explicit bash on windows (Git Bash)", syntaxJob("windows-latest", "shell: bash\n        run: |\n          if true; then\n"), 8, "`then` must be followed"},
		{"explicit sh", syntaxJob("ubuntu-latest", "shell: sh\n        run: |\n          fi\n"), 8, "can only be used"},
		{"custom template of bash", syntaxJob("ubuntu-latest", "shell: bash -el {0}\n        run: |\n          fi\n"), 8, "can only be used"},
		{"default shell of a workflow on a matrix runner", "on: push\ndefaults:\n  run:\n    shell: bash\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    steps:\n      - run: |\n          fi\n", 10, "can only be used"},
		{"after an expression in the script", syntaxJob("ubuntu-latest", "run: |\n          echo ${{ github.sha }}\n          if [ -z \"${{ github.ref }}\" ]; then\n            echo a\n"), 8, "must end with `fi`"},
		{"after a multi-line expression", syntaxJob("ubuntu-latest", "run: |\n          echo ${{ format('{0}',\n             github.sha) }}\n          done\n"), 9, "can only be used"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintShellSyntax(t, tc.src, nil, nil)
			if len(errs) != 1 {
				t.Fatalf("want one finding, got %d: %v\n%s", len(errs), errs, tc.src)
			}
			if !strings.Contains(errs[0].Message, tc.want) {
				t.Errorf("message %q should contain %q", errs[0].Message, tc.want)
			}
			if errs[0].Line != tc.line {
				t.Errorf("line = %d, want %d: %v", errs[0].Line, tc.line, errs[0])
			}
		})
	}
}

func TestRuleShellSyntaxNotReported(t *testing.T) {
	tests := []struct {
		what string
		src  string
	}{
		{"heredoc with an expression", syntaxJob("ubuntu-latest", "run: |\n          cat <<EOF > out.txt\n          sha ${{ github.sha }}\n          it's \"quoted\n          EOF\n")},
		{"quoted heredoc", syntaxJob("ubuntu-latest", "run: |\n          cat <<'EOF'\n          $(not run `at all\n          EOF\n")},
		{"indented heredoc", syntaxJob("ubuntu-latest", "run: |\n          if true; then\n            cat <<-EOF\n          \tindented\n          \tEOF\n          fi\n")},
		{"double bracket", syntaxJob("ubuntu-latest", "run: |\n          if [[ \"$A\" == b* && -n $C ]]; then echo ok; fi\n          [[ $x =~ ^(a|b)+$ ]] || true\n")},
		{"process substitution", syntaxJob("ubuntu-latest", "run: |\n          diff <(sort a) <(sort b)\n          tee >(cat) < /dev/null\n")},
		{"arrays and associative arrays in bash", syntaxJob("ubuntu-latest", "run: |\n          arr=(a b c)\n          declare -A map=([k]=v)\n          echo \"${arr[@]}\" \"${#arr[@]}\" \"${map[k]}\"\n")},
		{"here-string", syntaxJob("ubuntu-latest", "run: |\n          read -r a b <<< \"x y\"\n          grep -q x <<<\"$a\"\n")},
		{"case with fallthrough", syntaxJob("ubuntu-latest", "run: |\n          case \"$X\" in\n            a) echo a ;;&\n            b) echo b ;&\n            *) echo other ;;\n          esac\n")},
		{"expressions as words", syntaxJob("ubuntu-latest", "run: |\n          if ${{ contains(github.ref, 'x') }}; then\n            echo \"${{ matrix.os }}\" > ${{ env.OUT }}\n          fi\n          for f in ${{ inputs.files }}; do echo $f; done\n")},
		{"expression with quotes and braces", syntaxJob("ubuntu-latest", "run: echo '${{ toJSON(github.event) }}' \"${{ format('{0}}', 'a') }}\"\n")},
		{"expression inside arithmetic", syntaxJob("ubuntu-latest", "run: echo $(( ${{ github.run_number }} + 1 ))\n")},
		{"arrays in sh (only valid in bash, and a bash on some runners)", syntaxJob("ubuntu-latest", "shell: sh\n        run: |\n          arr=(1 2)\n          [[ -n $arr ]]\n")},
		{"pwsh script", syntaxJob("ubuntu-latest", "shell: pwsh\n        run: |\n          if ($true) { Write-Host \"x\n")},
		{"powershell script", syntaxJob("windows-latest", "shell: powershell\n        run: |\n          fi \"\n")},
		{"cmd script", syntaxJob("windows-latest", "shell: cmd\n        run: |\n          echo \"unclosed\n")},
		{"python script", syntaxJob("ubuntu-latest", "shell: python\n        run: |\n          print(\"a\"\n")},
		{"custom shell", syntaxJob("ubuntu-latest", "shell: perl {0}\n        run: |\n          print \"unclosed;\n")},
		{"dynamic shell", syntaxJob("ubuntu-latest", "shell: ${{ matrix.shell }}\n        run: |\n          fi\n")},
		{"default shell of a Windows runner is pwsh", syntaxJob("windows-latest", "run: |\n          if ($x) { Write-Host \"a\" }\n          fi \"\n")},
		{"default shell of a runner chosen by an expression", syntaxJob("${{ matrix.os }}", "run: |\n          fi\n")},
		{"default shell of a matrix with Windows", syntaxJob("[ubuntu-latest, windows-latest]", "run: |\n          fi\n")},
		{"default shell of a runner of unknown system", syntaxJob("self-hosted", "run: |\n          fi\n")},
		{"carriage return", syntaxJob("ubuntu-latest", "run: \"echo a\\r\\nif true; then\\r\\n\"\n")},
		{"unclosed expression is not read", syntaxJob("ubuntu-latest", "run: |\n          echo ${{ github.sha\n          fi\n")},
		{"step that uses an action", syntaxJob("ubuntu-latest", "uses: actions/checkout@v4\n")},
		{"empty script", syntaxJob("ubuntu-latest", "run: \"\"\n")},
		{"valid script", syntaxJob("ubuntu-latest", "run: |\n          set -euo pipefail\n          f() { echo \"$1\"; }\n          f a | while read -r l; do echo \"$l\"; done\n")},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if errs := lintShellSyntax(t, tc.src, nil, nil); len(errs) != 0 {
				t.Fatalf("want no finding, got %v\n%s", errs, tc.src)
			}
		})
	}
}

func TestRuleShellSyntaxIsOffInTheCorrectnessProfile(t *testing.T) {
	src := syntaxJob("ubuntu-latest", "run: |\n          fi\n")
	if errs := lintShellSyntax(t, src, nil, &Config{Profile: ProfileCorrectness}); len(errs) != 0 {
		t.Fatalf("the rule is not part of the correctness profile: %v", errs)
	}
	if errs := lintShellSyntax(t, src, nil, &Config{Profile: ProfileDefault}); len(errs) != 1 {
		t.Fatalf("the rule is part of the default profile: %v", errs)
	}
}

// shellcheck reports its SC1xxx syntax errors itself, so the built-in parser stays silent when it runs.
// withFakeShellcheck puts a shellcheck that finds nothing on PATH and returns the options which use it.
func withFakeShellcheck(t *testing.T) *LinterOptions {
	t.Helper()
	fakeShellcheck(t)
	return &LinterOptions{Shellcheck: "shellcheck"}
}

func TestRuleShellSyntaxIsSilentWhenShellcheckRuns(t *testing.T) {
	src := syntaxJob("ubuntu-latest", "run: |\n          fi\n")
	if errs := lintShellSyntax(t, src, withFakeShellcheck(t), nil); len(errs) != 0 {
		t.Fatalf("shellcheck ran, so there must be no second finding: %v", errs)
	}
}

func TestRuleShellSyntaxRunsWithoutShellcheck(t *testing.T) {
	src := syntaxJob("ubuntu-latest", "run: |\n          fi\n")
	for name, opts := range map[string]*LinterOptions{
		"turned off with an empty command": {Shellcheck: ""},
		"command not found":                {Shellcheck: "/nonexistent/dir/shellcheck"},
	} {
		t.Run(name, func(t *testing.T) {
			if errs := lintShellSyntax(t, src, opts, nil); len(errs) != 1 {
				t.Fatalf("want one finding, got %v", errs)
			}
		})
	}
	t.Run("the shellcheck rule is off in the configuration", func(t *testing.T) {
		cfg := fixtureConfig("shell-syntax")
		cfg.Rules["shellcheck"] = RuleConfig{Level: SeverityOff}
		if errs := lintShellSyntax(t, src, withFakeShellcheck(t), cfg); len(errs) != 1 {
			t.Fatalf("want one finding, got %v", errs)
		}
	})
}

func TestRuleShellSyntaxCompositeAction(t *testing.T) {
	src := "name: a\ndescription: d\nruns:\n  using: composite\n  steps:\n    - shell: bash\n      run: |\n        if true; then\n    - shell: pwsh\n      run: |\n        if ($true) { \"x\n"
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig("shell-syntax")
	errs, err := l.Lint("action.yml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range errs {
		if e.ID == "shell-syntax" {
			n++
			if e.Line != 8 {
				t.Errorf("line = %d, want 8: %v", e.Line, e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want one finding, got %d: %v", n, errs)
	}
}
