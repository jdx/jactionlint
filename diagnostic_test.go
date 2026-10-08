package jactionlint

import (
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/execabs"
)

// Every error of the built-in rules must carry a registered ID, a severity, the documentation URL
// and a region, whichever rule reported it.
func TestDiagnosticsOfBuiltinRulesAreComplete(t *testing.T) {
	var shellcheck, pyflakes string
	if p, err := execabs.LookPath("shellcheck"); err == nil {
		shellcheck = p
	}
	if p, err := execabs.LookPath("pyflakes"); err == nil {
		pyflakes = p
	}

	total := 0
	ids := map[string]bool{}
	for _, subdir := range []string{"examples", "err"} {
		dir, infiles, err := testFindAllWorkflowsInDir(subdir)
		if err != nil {
			t.Fatal(err)
		}
		proj := &Project{root: dir}
		for _, infile := range infiles {
			b, err := os.ReadFile(infile)
			if err != nil {
				t.Fatal(err)
			}
			l, err := NewLinter(io.Discard, &LinterOptions{Shellcheck: shellcheck, Pyflakes: pyflakes})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = withoutMissingTimeout(&Config{})
			errs, err := l.Lint("test.yaml", b, proj)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range errs {
				total++
				ids[e.ID] = true
				info, ok := LookupRule(e.ID)
				if !ok {
					t.Errorf("%s: error %q has unregistered ID %q (kind %q)", infile, e.Message, e.ID, e.Kind)
					continue
				}
				if e.Severity != info.DefaultLevel {
					t.Errorf("%s: error %q has severity %v", infile, e.Message, e.Severity)
				}
				if e.DocURL != info.DocURL() {
					t.Errorf("%s: error %q has DocURL %q", infile, e.Message, e.DocURL)
				}
				if e.Line > 0 && (e.EndLine < e.Line || (e.EndLine == e.Line && e.EndColumn < e.Column)) {
					t.Errorf("%s: error %q has an invalid region %d:%d-%d:%d", infile, e.Message, e.Line, e.Column, e.EndLine, e.EndColumn)
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("no error was found in testdata")
	}
	t.Logf("%d errors with %d distinct IDs", total, len(ids))
}

func TestRuleBaseReportID(t *testing.T) {
	r := NewRuleBase("dummy", "")
	r.ReportID("invalid-glob", &Pos{1, 2}, "msg1")
	r.ReportIDf("invalid-cron", &Pos{3, 4}, "msg%d", 2)
	r.ReportRange("invalid-glob", &Pos{5, 6}, &Pos{7, 8}, "msg3")
	errs := r.Errs()
	if len(errs) != 3 {
		t.Fatalf("want 3 errors but got %v", errs)
	}
	if errs[0].ID != "invalid-glob" || errs[0].Kind != "dummy" || errs[0].Message != "msg1" {
		t.Errorf("unexpected error %+v", errs[0])
	}
	if errs[1].ID != "invalid-cron" || errs[1].Message != "msg2" || errs[1].Line != 3 || errs[1].Column != 4 {
		t.Errorf("unexpected error %+v", errs[1])
	}
	if e := errs[2]; e.EndLine != 7 || e.EndColumn != 8 {
		t.Errorf("unexpected region of %+v", e)
	}
}

func TestErrorRegionIsFilledFromToken(t *testing.T) {
	src := []byte("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo 'こんにちは' foo\r\n")
	tests := []struct {
		line, col   int
		endLine     int
		endColumn   int
		description string
	}{
		{4, 5, 4, 13, "key"},
		{4, 14, 4, 27, "value"},
		{6, 7, 6, 8, "dash"},
		{6, 9, 6, 13, "run key"},
		{6, 19, 6, 26, "multibyte token"},
		{6, 999, 6, 999, "column after the end of the line"},
		{99, 1, 99, 1, "line out of the source"},
	}
	lines := sourceLines(src)
	for _, tc := range tests {
		e := &Error{Line: tc.line, Column: tc.col}
		e.fillRegion(lines)
		if e.EndLine != tc.endLine || e.EndColumn != tc.endColumn {
			t.Errorf("%s: want end %d:%d but got %d:%d", tc.description, tc.endLine, tc.endColumn, e.EndLine, e.EndColumn)
		}
	}

	e := &Error{Line: 1, Column: 1, EndLine: 2, EndColumn: 3}
	e.fillRegion(lines)
	if e.EndLine != 2 || e.EndColumn != 3 {
		t.Error("an explicit region must not be changed")
	}
	e = &Error{}
	e.fillRegion(lines)
	if e.EndLine != 0 || e.EndColumn != 0 {
		t.Error("an error without position has no region")
	}
}

func TestFixValidFor(t *testing.T) {
	src := []byte("0123456789")
	tests := []struct {
		name string
		fix  *Fix
		want bool
	}{
		{"nil", nil, false},
		{"no edit", &Fix{}, false},
		{"insert", &Fix{Edits: []TextEdit{{3, 3, "x"}}}, true},
		{"delete", &Fix{Edits: []TextEdit{{3, 5, ""}}}, true},
		{"whole file", &Fix{Edits: []TextEdit{{0, 10, "x"}}}, true},
		{"out of range", &Fix{Edits: []TextEdit{{3, 11, "x"}}}, false},
		{"negative", &Fix{Edits: []TextEdit{{-1, 2, "x"}}}, false},
		{"reversed", &Fix{Edits: []TextEdit{{5, 3, "x"}}}, false},
		{"adjacent", &Fix{Edits: []TextEdit{{4, 6, "x"}, {2, 4, "y"}}}, true},
		{"overlap", &Fix{Edits: []TextEdit{{2, 5, "x"}, {4, 6, "y"}}}, false},
	}
	for _, tc := range tests {
		if got := tc.fix.validFor(src); got != tc.want {
			t.Errorf("%s: want %v but got %v", tc.name, tc.want, got)
		}
	}
	if !strings.Contains((&Error{Message: "m", Filepath: "f", Line: 1, Column: 2, Kind: "k"}).Error(), "[k]") {
		t.Error("Error() must keep showing the kind")
	}
}
