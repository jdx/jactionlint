package jactionlint

import (
	"io"
	"strings"
	"testing"

	"golang.org/x/sys/execabs"
)

func lintShellcheckWithIgnores(t *testing.T, src string) []*Error {
	t.Helper()
	sc, err := execabs.LookPath("shellcheck")
	if err != nil {
		t.Skip("skipped because \"shellcheck\" command does not exist in system")
	}
	l, err := NewLinter(io.Discard, &LinterOptions{Shellcheck: sc})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{Profile: ProfileCorrectness})
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

// actionlint reports a shellcheck finding on the line of the `run:` key, jactionlint on the line of the
// script which has the problem. The ignore comments written for actionlint sit at the `run:` key or the
// step, so they have to keep covering the lines of the script.
func TestInlineIgnoreOfRunLineCoversShellcheck(t *testing.T) {
	const head = "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n"
	const script = "          echo hello\n          cat $FILE\n          echo finished\n"
	tests := []struct {
		name  string
		steps string
		want  int // the shellcheck findings that remain
	}{
		{"nothing", "      - run: |\n" + script, 1},
		{"above the step", "      # jactionlint ignore=shellcheck\n      - run: |\n" + script, 0},
		{"above the run key", "      - name: build\n        # jactionlint ignore=shellcheck\n        run: |\n" + script, 0},
		{"trailing on the run line", "      - run: | # jactionlint ignore=shellcheck\n" + script, 0},
		{"trailing on the run key", "      - name: x\n        run: | # jactionlint ignore=shellcheck\n" + script, 0},
		{"above, by message", "      # jactionlint ignore=SC2086\n      - run: |\n" + script, 0},
		{"above, by message in the middle of the step", "      - name: x\n        shell: bash\n        # jactionlint ignore=SC2086\n        run: |\n" + script, 0},
		{"above the run key of the next step", "      # jactionlint ignore=shellcheck\n      - run: echo\n      - run: |\n" + script, 1},
		{"blank lines and comments in the script", "      # jactionlint ignore=shellcheck\n      - run: |\n          echo hello\n\n          # a comment\n          cat $FILE\n", 0},
		{"CRLF", strings.ReplaceAll("      - run: | # jactionlint ignore=shellcheck\n"+script, "\n", "\r\n"), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := head + tc.steps
			if strings.Contains(tc.steps, "\r\n") {
				src = strings.ReplaceAll(head, "\n", "\r\n") + tc.steps
			}
			errs := lintShellcheckWithIgnores(t, src)
			n := 0
			for _, e := range errs {
				if e.ID == "shellcheck" {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d shellcheck findings but got %d: %v", tc.want, n, errs)
			}
		})
	}
}
