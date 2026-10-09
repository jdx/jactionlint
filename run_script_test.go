package jactionlint

import (
	"strings"
	"testing"
)

// TestAnalyzeRunPositions checks that positions of the analyzed script are positions in the workflow file.
func TestAnalyzeRunPositions(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		exact bool
	}{
		{"literal", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo hi\n          echo a=1 >> $GITHUB_ENV\n", true},
		{"literal strip", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: x\n        run: |-\n            echo hi\n\n            echo a=1 >> $GITHUB_ENV\n", true},
		{"literal keep", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |+\n         echo a=1 >> $GITHUB_ENV\n", true},
		{"literal leading blank", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n\n          echo a=1 >> $GITHUB_ENV\n", true},
		{"literal expr", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo ${{\n            github.ref\n          }}\n          echo a=1 >> $GITHUB_ENV\n", true},
		{"plain", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo a=1 >> $GITHUB_ENV\n", true},
		{"double quoted", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: \"echo a=1 >> $GITHUB_ENV\"\n", true},
		{"single quoted", "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: 'echo a=1 >> $GITHUB_ENV'\n", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := Parse([]byte(tc.src))
			if len(errs) > 0 || w == nil {
				t.Fatal(errs)
			}
			run := w.Jobs["test"].Steps[0].Exec.(*ExecRun)
			s, origin := analyzeRun(run)
			if s == nil {
				t.Fatal("not analyzed")
			}
			ws := s.WritesTo("GITHUB_ENV")
			if len(ws) != 1 {
				t.Fatalf("%d writes", len(ws))
			}
			pos := s.Position(origin, ws[0].Redirect.Offset)
			if pos.Exact != tc.exact {
				t.Errorf("exact = %v, want %v", pos.Exact, tc.exact)
			}
			line := strings.Split(tc.src, "\n")[pos.Line-1]
			if !strings.HasPrefix(line[pos.Col-1:], ">> $GITHUB_ENV") {
				t.Errorf("position %+v is at %q", pos, line[pos.Col-1:])
			}
		})
	}
}

func TestAnalyzeRunSkips(t *testing.T) {
	src := "on: push\njobs:\n  test:\n    runs-on: windows-latest\n    steps:\n      - run: Write-Host hi\n        shell: pwsh\n      - run: echo hi\n        shell: python\n      - run: if true; then\n"
	w, errs := Parse([]byte(src))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for i, step := range w.Jobs["test"].Steps {
		if s, _ := analyzeRun(step.Exec.(*ExecRun)); s != nil {
			t.Errorf("step %d must not be analyzed", i)
		}
	}
	if s, _ := analyzeRun(nil); s != nil {
		t.Error("nil exec")
	}
}
