package jactionlint

import (
	"strings"
	"testing"
)

func TestScanExprsPositions(t *testing.T) {
	tests := []struct {
		name string
		step string
	}{
		{"plain", "      - run: echo ${{ github.head_ref }}\n"},
		{"double quoted", "      - run: \"echo ${{ github.head_ref }}\"\n"},
		{"single quoted", "      - run: 'echo ${{ github.head_ref }}'\n"},
		{"literal block", "      - run: |\n          echo a\n          echo ${{ github.head_ref }}\n"},
		{"literal block with blank lines", "      - run: |\n\n          echo a\n\n          echo ${{ github.head_ref }}\n"},
		{"folded block", "      - run: >\n          echo a\n          echo ${{ github.head_ref }}\n"},
		{"folded block strip", "      - run: >-\n          echo a\n          echo ${{ github.head_ref }} b\n"},
		{"plain multi-line", "      - run: echo a\n          echo ${{ github.head_ref }}\n"},
		{"plain multi-line with a placeholder on the first line", "      - run: echo ${{ github.sha }}\n          echo ${{ github.head_ref }}\n"},
		{"expression across lines of a folded block", "      - run: >-\n          echo ${{\n            github.head_ref }}\n"},
		{"multi-byte characters", "      - run: echo \u3042\u3044 ${{ github.head_ref }}\n"},
		{"multi-byte characters in a literal block", "      - run: |\n          echo \u3042\u3044 ${{ github.head_ref }}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + tc.step
			w, errs := Parse([]byte(src))
			if w == nil || len(errs) > 0 {
				t.Fatalf("does not parse: %v", errs)
			}
			var run *String
			for _, j := range w.Jobs {
				run = j.Steps[0].Exec.(*ExecRun).Run
			}
			idx := newSourceIndex([]byte(src))
			var found *exprSpan
			spans := idx.scanExprs(run)
			for i := range spans {
				if strings.Contains(spans[i].Src, "head_ref") {
					found = &spans[i]
				}
			}
			if found == nil {
				t.Fatalf("placeholder not found in %q", run.Value)
			}
			// The token is where the text is in the source
			want := strings.LastIndex(src, "github.head_ref")
			wantLine, wantCol := idx.lineCol(want)
			tok := found.Node.Token()
			got := found.TokPos(tok)
			if got.Line != wantLine || got.Col != wantCol {
				t.Errorf("position of the token: want %d:%d but got %d:%d", wantLine, wantCol, got.Line, got.Col)
			}
			// The placeholder maps back to its bytes in the source
			if text := found.Text(run); !strings.Contains(text, "\n") {
				off, ok := idx.valueOffset(run, found.Start)
				if !ok || !idx.matches(off, text) {
					t.Errorf("offset of the placeholder: %d, %v (%q)", off, ok, text)
				}
			}
		})
	}
}
