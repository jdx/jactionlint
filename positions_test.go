package jactionlint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// The positions of the findings are lines and columns counted in code points. These tests check that
// every region of every finding of the fixtures is a valid region of its file, in the SARIF log and in
// the JSON output, with text that is not ASCII, astral characters, tabs, CRLF and multi-line scalars.

func positionFixtures(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"testdata/positions", "testdata/err", "testdata/examples", "testdata/ok"} {
		m, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) < 100 {
		t.Fatalf("too few fixtures: %d", len(files))
	}
	return files
}

// sourceColumns returns the length in code points of each line, without the line break.
func sourceColumns(src []byte) []int {
	var ret []int
	for _, l := range strings.Split(string(src), "\n") {
		ret = append(ret, utf8.RuneCountInString(strings.TrimSuffix(l, "\r")))
	}
	return ret
}

func lintFormatted(t *testing.T, format string, name string, src []byte) string {
	t.Helper()
	var b strings.Builder
	l, err := NewLinter(&b, &LinterOptions{Format: format})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{Profile: ProfilePedantic}
	if _, err := l.Lint(name, src, nil); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSARIFRegionsAreValid(t *testing.T) {
	for _, file := range positionFixtures(t) {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cols := sourceColumns(src)
		out := lintFormatted(t, FormatSARIF, file, src)
		var doc struct {
			Runs []struct {
				ColumnKind string `json:"columnKind"`
				Results    []struct {
					RuleID    string `json:"ruleId"`
					Locations []struct {
						PhysicalLocation struct {
							Region *struct {
								StartLine, StartColumn, EndLine, EndColumn int
							} `json:"region"`
						} `json:"physicalLocation"`
					} `json:"locations"`
					Fixes []struct {
						ArtifactChanges []struct {
							Replacements []struct {
								DeletedRegion struct {
									StartLine, StartColumn, EndLine, EndColumn int
								} `json:"deletedRegion"`
							} `json:"replacements"`
						} `json:"artifactChanges"`
					} `json:"fixes"`
				} `json:"results"`
			} `json:"runs"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if len(doc.Runs) != 1 || doc.Runs[0].ColumnKind != "unicodeCodePoints" {
			t.Fatalf("%s: unexpected run", file)
		}
		check := func(what string, sl, sc, el, ec int) {
			t.Helper()
			at := func(line int) int {
				if line < 1 || line > len(cols) {
					return -1
				}
				return cols[line-1]
			}
			switch {
			case sl < 1 || sc < 1:
				t.Errorf("%s: %s starts before the file: %d:%d", file, what, sl, sc)
			case at(sl) < 0 || sc > at(sl)+1:
				t.Errorf("%s: %s starts outside the line: %d:%d (the line has %d characters)", file, what, sl, sc, at(sl))
			case el == 0 && ec == 0:
				// no end given
			case el < sl || (el == sl && ec < sc):
				t.Errorf("%s: %s ends before it starts: %d:%d-%d:%d", file, what, sl, sc, el, ec)
			case at(el) < 0 || ec > at(el)+1:
				t.Errorf("%s: %s ends outside the line: %d:%d-%d:%d (the line has %d characters)", file, what, sl, sc, el, ec, at(el))
			}
		}
		for _, r := range doc.Runs[0].Results {
			for _, l := range r.Locations {
				if reg := l.PhysicalLocation.Region; reg != nil {
					check(r.RuleID, reg.StartLine, reg.StartColumn, reg.EndLine, reg.EndColumn)
				}
			}
			for _, f := range r.Fixes {
				for _, c := range f.ArtifactChanges {
					for _, rep := range c.Replacements {
						d := rep.DeletedRegion
						check(r.RuleID+" fix", d.StartLine, d.StartColumn, d.EndLine, d.EndColumn)
					}
				}
			}
		}
	}
}

func TestJSONColumnsAreConsistent(t *testing.T) {
	for _, file := range positionFixtures(t) {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cols := sourceColumns(src)
		var list []ErrorTemplateFields
		if err := json.Unmarshal([]byte(lintFormatted(t, FormatJSON, file, src)), &list); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, e := range list {
			if e.Line < 1 || e.Line > len(cols) {
				continue
			}
			if e.Column < 1 || e.Column > cols[e.Line-1]+1 {
				t.Errorf("%s:%d: %s: column %d is outside the line of %d characters", file, e.Line, e.ID, e.Column, cols[e.Line-1])
			}
			// end_column is the column of the last character of the indicator
			if e.EndLine == e.Line && e.EndColumn < e.Column {
				t.Errorf("%s:%d: %s: end_column %d is before column %d", file, e.Line, e.ID, e.EndColumn, e.Column)
			}
			if e.EndLine == e.Line && e.EndColumn > cols[e.Line-1] && e.Column <= cols[e.Line-1] {
				t.Errorf("%s:%d: %s: end_column %d is after the line of %d characters", file, e.Line, e.ID, e.EndColumn, cols[e.Line-1])
			}
		}
	}
}

// The reproducer of the bug bash: a finding after non-ASCII text had an end_column before its column,
// because the column of the line was used as a byte offset to find the end of the indicator.
func TestJSONEndColumnWithNonASCII(t *testing.T) {
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - run: echo\n        if: github.event.head_commit.message == format('é {0}', 'x') && format('a{0}', 'b') == 'ab'\n"
	var list []ErrorTemplateFields
	if err := json.Unmarshal([]byte(lintFormatted(t, FormatJSON, "w.yaml", []byte(src))), &list); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range list {
		if e.ID != "obfuscation" {
			continue
		}
		n++
		if e.EndColumn < e.Column {
			t.Errorf("end_column %d is before column %d: %+v", e.EndColumn, e.Column, e)
		}
	}
	if n != 2 {
		t.Errorf("want 2 obfuscation findings, got %d in %v", n, list)
	}
}

func TestIndicatorCountsCodePoints(t *testing.T) {
	line := "é x ✓y z"
	e := &Error{Line: 1, Column: 5} // ✓ is the fifth character, the sixth byte
	ind, last := e.indicator(line)
	if ind != "    ^~" || last != 6 {
		t.Errorf("got %q and %d", ind, last)
	}
	// A column after the end of the line has no indicator and does not panic
	for _, col := range []int{9, 10, 100} {
		e := &Error{Line: 1, Column: col}
		if _, ok := columnInLine(line, e.Column); ok != (col == 9) {
			t.Errorf("column %d: ok=%v", col, ok)
		}
		e.indicator(line)
	}
}

// A finding inside an expression of a multi-line scalar is on the line and at the column of the text
// in the file, however the scalar is written and whichever line breaks and characters it has.
func TestPositionsInMultiLineScalars(t *testing.T) {
	scalars := map[string]string{
		"literal":       "|\n        é ✓ 😀 ${{ format('a{0}', 'b') }}\n        next ${{ format('c{0}', 'd') }}\n",
		"literal strip": "|-\n        é\n\n        ✓ 😀 ${{ format('a{0}', 'b') }}\n",
		"folded":        ">\n        é ✓\n        😀 ${{ format('a{0}', 'b') }}\n        x ${{ format('c{0}', 'd') }}\n",
		"folded strip":  ">-\n        é ✓ 😀\n        ${{ format('a{0}', 'b') }}\n",
		"plain":         "é ✓ 😀\n        next ${{ format('a{0}', 'b') }}\n",
		"double quoted": "\"é ✓ 😀\n        next ${{ format('a{0}', 'b') }}\"\n",
		"single quoted": "'é ✓ 😀\n        next ${{ format(''a{0}'', ''b'') }}'\n",
	}
	for name, scalar := range scalars {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(name+" "+strings.NewReplacer("\r", "CR", "\n", "LF").Replace(nl), func(t *testing.T) {
				src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      X: " + scalar
				src = strings.ReplaceAll(src, "\n", nl)
				var list []ErrorTemplateFields
				if err := json.Unmarshal([]byte(lintFormatted(t, FormatJSON, "w.yaml", []byte(src))), &list); err != nil {
					t.Fatal(err)
				}
				var got [][2]int
				for _, e := range list {
					if e.ID == "obfuscation" {
						got = append(got, [2]int{e.Line, e.Column})
					}
				}
				// Where "format(" is, as a line and a column counted in code points
				var want [][2]int
				for i, l := range strings.Split(src, "\n") {
					l = strings.TrimSuffix(l, "\r")
					for from := 0; ; {
						j := strings.Index(l[from:], "format(")
						if j < 0 {
							break
						}
						want = append(want, [2]int{i + 1, utf8.RuneCountInString(l[:from+j]) + 1})
						from += j + 1
					}
				}
				if len(got) != len(want) {
					t.Fatalf("want findings at %v but got %v in\n%s", want, got, src)
				}
				for i := range want {
					if got[i] != want[i] {
						t.Errorf("finding %d is at %v but format( is at %v in\n%s", i, got[i], want[i], src)
					}
				}
			})
		}
	}
}
