package jactionlint

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"
)

// The tests of the one mapping from the value of a scalar to the place in the file (scalar_map.go) and of
// the rules which report inside scalars.

const posHead = "on: push\npermissions: {}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n"

// findingsOf lints the source with the pedantic profile and returns the findings of the rule.
func findingsOf(t *testing.T, id, src string) []ErrorTemplateFields {
	t.Helper()
	var list []ErrorTemplateFields
	if err := json.Unmarshal([]byte(lintFormatted(t, FormatJSON, "w.yaml", []byte(src))), &list); err != nil {
		t.Fatal(err)
	}
	var ret []ErrorTemplateFields
	for _, e := range list {
		if e.ID == id {
			ret = append(ret, e)
		}
	}
	return ret
}

// textAt returns the text of the file from the line and the column (in code points).
func textAt(src string, line, col int) string {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return "<outside>"
	}
	l := strings.TrimSuffix(lines[line-1], "\r")
	b, ok := columnInLine(l, col)
	if !ok {
		return "<outside>"
	}
	return l[b:]
}

// A finding is on the line of the text it is about, at the first character of it, whatever the style of
// the scalar: escapes, quotes written twice, folded lines and blocks.
func TestFindingsPointAtTheirTextInScalars(t *testing.T) {
	tests := []struct {
		name string
		id   string
		body string // the steps, or the lines which follow the job header
		want string // what the text at the position starts with
		n    int    // number of findings, 1 when 0
	}{
		{
			name: "double quoted with newline escapes (R2-3-1, R2-1-4)",
			id:   "pipeline-without-pipefail",
			body: "    steps:\n      - run: \"echo hi\\necho there\\nwc -l < f | tr -d ' '\\necho done\\n\"\n",
			want: "wc -l",
		},
		{
			name: "multi-line single quoted",
			id:   "pipeline-without-pipefail",
			body: "    steps:\n      - run: 'x=1\n\n          cat f | sort > g'\n",
			want: "cat f",
		},
		{
			name: "folded env value, secrets (R2-2-3, R2-3-7, R2-4-7)",
			id:   "secrets-outside-env",
			body: "    env:\n      KEY: >-\n        ${{ secrets.FOO }}-${{ secrets.BAR }}\n    steps:\n      - run: echo\n",
			want: "secrets.FOO",
			n:    2,
		},
		{
			name: "folded with value, secrets",
			id:   "secrets-outside-env",
			body: "    steps:\n      - uses: some/action@abc\n        with:\n          key: >-\n            x\n            ${{ secrets.FOO }}\n",
			want: "secrets.FOO",
		},
		{
			name: "folded run, secrets (R2-1-7)",
			id:   "secrets-outside-env",
			body: "    steps:\n      - run: >\n          ./sign code\n          --tenant \"${{ secrets.TENANT }}\" --publish\n",
			want: "secrets.TENANT",
		},
		{
			name: "folded if, unsound-prefix-match (R2-3-7)",
			id:   "unsound-prefix-match",
			body: "    steps:\n      - if: >\n          vars.SKIP != 'true' &&\n          (github.actor == 'a' || contains(github.actor, 'x-bot'))\n        run: echo\n",
			want: "contains(github.actor, 'x-bot')",
		},
		{
			name: "folded run, pipeline",
			id:   "pipeline-without-pipefail",
			body: "    steps:\n      - run: >\n          cat a\n          | grep x\n          | sort\n",
			want: "cat a",
		},
		{
			name: "escapes before the expression (R2-1-5, R2-3-8)",
			id:   "template-injection",
			body: "    steps:\n      - run: \"echo \\\"x\\\" \\\"${{ github.head_ref }}\\\" y\"\n",
			want: "github.head_ref",
		},
		{
			name: "tab escape and unicode before the expression (R2-3-8)",
			id:   "template-injection",
			body: "    steps:\n      - run: \"echo é \\t ${{ github.head_ref }}\"\n",
			want: "github.head_ref",
		},
		{
			name: "single quote escapes before the expression (R2-1-5)",
			id:   "template-injection",
			body: "    steps:\n      - run: 'echo ''x'' \"${{ github.head_ref }}\"'\n",
			want: "github.head_ref",
		},
		{
			name: "unicode escapes before the expression",
			id:   "template-injection",
			body: "    steps:\n      - run: \"echo \\u00e9\\U0001F600\\x41 ${{ github.head_ref }}\"\n",
			want: "github.head_ref",
		},
		{
			name: "line continuation in a double quoted scalar",
			id:   "template-injection",
			body: "    steps:\n      - run: \"echo \\\n          hi ${{ github.head_ref }}\"\n",
			want: "github.head_ref",
		},
		{
			name: "folded plain scalar with a blank line",
			id:   "template-injection",
			body: "    steps:\n      - run: echo one\n\n          two ${{ github.head_ref }}\n",
			want: "github.head_ref",
		},
		{
			name: "folded block with more indented line",
			id:   "template-injection",
			body: "    steps:\n      - run: >\n          echo one\n\n            indented\n          echo ${{ github.head_ref }}\n",
			want: "github.head_ref",
		},
		{
			name: "literal block keep",
			id:   "template-injection",
			body: "    steps:\n      - run: |+\n          echo one\n\n          echo ${{ github.head_ref }}\n\n",
			want: "github.head_ref",
		},
		{
			name: "expression written without spaces (R2-1-6)",
			id:   "template-injection",
			body: "    steps:\n      - run: echo ${{github.head_ref}}\n",
			want: "github.head_ref",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := posHead + tc.body
			got := findingsOf(t, tc.id, src)
			n := max(tc.n, 1)
			if len(got) != n {
				t.Fatalf("want %d finding(s) of %s, got %d: %+v", n, tc.id, len(got), got)
			}
			if text := textAt(src, got[0].Line, got[0].Column); !strings.HasPrefix(text, tc.want) {
				t.Errorf("%s is at %d:%d, on %q, want a position on %q", tc.id, got[0].Line, got[0].Column, text, tc.want)
			}
		})
	}
}

// The end of the region of an expression written without spaces is the end of its text, not the "}}".
func TestRegionEndsBeforeClosingBraces(t *testing.T) {
	src := posHead + "    steps:\n      - run: echo ${{github.head_ref}}\n      - run: echo ${{ github.head_ref}}\n      - run: echo ${{github.head_ref }}\n"
	got := findingsOf(t, "template-injection", src)
	if len(got) != 3 {
		t.Fatalf("want 3 findings, got %+v", got)
	}
	for _, e := range got {
		line := strings.Split(src, "\n")[e.Line-1]
		if text := line[e.Column-1 : e.EndColumn]; text != "github.head_ref" {
			t.Errorf("line %d: the region is %q", e.Line, text)
		}
	}
}

// The name in workflow_run covers the whole quoted name, in characters and not in bytes.
func TestWorkflowRunNamesRegion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/.github/workflows", 0o755); err != nil {
		t.Fatal(err)
	}
	src := "on:\n  workflow_run:\n    workflows: [\"日本語 aaaa\", 'bbbbbbbb', plain name]\n    types: [completed]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"
	path := root + "/.github/workflows/a.yml"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = &Config{Profile: ProfileCorrectness}
	errs, err := l.Lint(path, []byte(src), &Project{root: root})
	if err != nil {
		t.Fatal(err)
	}
	var got []*Error
	for _, e := range errs {
		if e.ID == "workflow-run-names" {
			got = append(got, e)
		}
	}
	want := []string{`"日本語 aaaa"`, `'bbbbbbbb'`}
	if len(got) < len(want) {
		t.Fatalf("want findings for %v, got %v", want, errs)
	}
	line := []rune(strings.Split(src, "\n")[2])
	for i, w := range want {
		e := got[i]
		if text := string(line[e.Column-1 : e.EndColumn-1]); text != w {
			t.Errorf("the region of finding %d is %q, want %q", i, text, w)
		}
	}
}

// The JSON output and the SARIF log give the same region (the JSON end is inclusive, the SARIF end is not).
func TestJSONAndSARIFRegionsAgree(t *testing.T) {
	for _, file := range positionFixtures(t) {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var list []ErrorTemplateFields
		if err := json.Unmarshal([]byte(lintFormatted(t, FormatJSON, file, src)), &list); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var doc struct {
			Runs []struct {
				Results []struct {
					RuleID    string `json:"ruleId"`
					Locations []struct {
						PhysicalLocation struct {
							Region *struct{ StartLine, StartColumn, EndLine, EndColumn int }
						}
					}
				}
			}
		}
		if err := json.Unmarshal([]byte(lintFormatted(t, FormatSARIF, file, src)), &doc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		results := doc.Runs[0].Results
		if len(results) != len(list) {
			t.Fatalf("%s: %d JSON findings and %d SARIF results", file, len(list), len(results))
		}
		for i, e := range list {
			r := results[i].Locations[0].PhysicalLocation.Region
			if r == nil || e.Line != r.StartLine || e.Column != r.StartColumn {
				continue
			}
			if e.EndLine == e.Line && r.EndLine == r.StartLine && r.EndColumn > r.StartColumn && e.EndColumn != r.EndColumn-1 {
				t.Errorf("%s:%d:%d %s: end_column is %d in JSON and the SARIF region ends before column %d", file, e.Line, e.Column, e.ID, e.EndColumn, r.EndColumn)
			}
		}
	}
}

func TestSARIFHasStableFingerprints(t *testing.T) {
	const body = "    steps:\n      - run: echo ${{ github.head_ref }}\n      - run: echo ${{ github.head_ref }}\n"
	fingerprints := func(src string) []string {
		var doc struct {
			Runs []struct {
				Artifacts []struct{ Location struct{ URI string } }
				Results   []struct {
					RuleID              string
					PartialFingerprints map[string]string
					Locations           []struct {
						PhysicalLocation struct{ ArtifactLocation struct{ Index *int } }
					}
				}
			}
		}
		if err := json.Unmarshal([]byte(lintFormatted(t, FormatSARIF, "w.yaml", []byte(src))), &doc); err != nil {
			t.Fatal(err)
		}
		var ret []string
		for _, r := range doc.Runs[0].Results {
			fp := r.PartialFingerprints["primaryLocationLineHash"]
			if fp == "" {
				t.Fatalf("%s has no fingerprint", r.RuleID)
			}
			if i := r.Locations[0].PhysicalLocation.ArtifactLocation.Index; i == nil || doc.Runs[0].Artifacts[*i].Location.URI != "w.yaml" {
				t.Errorf("%s: the artifact of the result is not listed", r.RuleID)
			}
			if r.RuleID == "template-injection" {
				ret = append(ret, fp)
			}
		}
		return ret
	}
	a := fingerprints(posHead + body)
	b := fingerprints("# a comment\n\n" + strings.Replace(posHead, "    runs-on", "    # more\n    runs-on", 1) + strings.ReplaceAll(body, "      - run", "        - run"))
	if len(a) != 2 || a[0] == a[1] {
		t.Fatalf("two identical lines must have different fingerprints: %v", a)
	}
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("fingerprints changed when lines were added above and the indentation changed: %v and %v", a, b)
	}
}

// --- the mapping itself -------------------------------------------------------------------------------

// piece is one unit of a generated scalar: the text written in the file, the bytes it gives in the value,
// and whether the place of the first byte of it is the start of the text (false for a folded line break, whose
// place is the end of the line).
type piece struct {
	src, value string
	atStart    bool
}

// genScalar builds a scalar of the style from random pieces. It returns the text to put after "key: ".
func genScalar(r *rand.Rand, style string) (text string, pieces []piece) {
	chars := []string{"a", "b", "$", "{", "é", "日", "😀", "x", "-", "}"}
	n := 3 + r.Intn(12)
	indent := "        "
	for i := 0; i < n; i++ {
		switch k := r.Intn(10); {
		case k < 5 || i == 0 || i == n-1:
			c := chars[r.Intn(len(chars))]
			pieces = append(pieces, piece{c, c, true})
		case k < 6:
			pieces = append(pieces, piece{" ", " ", true})
		case k < 7 && style == "dq":
			esc := [][2]string{{`\"`, `"`}, {`\\`, `\`}, {`\n`, "\n"}, {`\t`, "\t"}, {`\x41`, "A"}, {`\u00e9`, "é"}, {`\U0001F600`, "😀"}, {`\/`, "/"}}[r.Intn(8)]
			pieces = append(pieces, piece{esc[0], esc[1], true})
		case k < 7 && style == "sq":
			pieces = append(pieces, piece{"''", "'", true})
		case k < 9 && style != "lit":
			// a line break between two words: one space, or newlines when blank lines follow
			if len(pieces) > 0 && pieces[len(pieces)-1].src == " " {
				pieces = append(pieces, piece{"b", "b", true})
			}
			if r.Intn(3) == 0 {
				pieces = append(pieces, piece{"\n\n" + indent, "\n", false})
			} else {
				pieces = append(pieces, piece{"\n" + indent, " ", false})
			}
			pieces = append(pieces, piece{"c", "c", true})
		default:
			pieces = append(pieces, piece{"d", "d", true})
		}
	}
	var b strings.Builder
	for _, p := range pieces {
		b.WriteString(p.src)
	}
	text = b.String()
	switch style {
	case "dq":
		text = `"` + text + `"`
	case "sq":
		text = `'` + text + `'`
	}
	return text, pieces
}

func TestScalarMapAgreesWithConstruction(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	checked := 0
	defer func() {
		if checked < 600 {
			t.Errorf("only %d of the generated scalars were checked", checked)
		}
	}()
	for _, style := range []string{"plain", "dq", "sq"} {
		for iter := 0; iter < 400; iter++ {
			text, pieces := genScalar(r, style)
			if style == "plain" && (strings.HasPrefix(text, "{") || strings.HasPrefix(text, "}") || strings.HasPrefix(text, "-") || strings.HasPrefix(text, "$")) && strings.HasPrefix(text, "{") {
				continue // starts a flow mapping
			}
			src := "a:\n  k: " + text + "\n"
			if checkScalarMap(t, src, 2, pieces) {
				checked++
			}
		}
	}
}

// checkScalarMap parses the document, whose scalar value is on the line, and compares the place of every byte of the
// value with the place the generator knows.
func checkScalarMap(t *testing.T, src string, line int, pieces []piece) bool {
	t.Helper()
	var want strings.Builder
	for _, p := range pieces {
		want.WriteString(p.value)
	}
	w, _ := Parse([]byte("on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      K: " + strings.TrimPrefix(src, "a:\n  k: ")))
	if w == nil {
		return false // not a valid document: the generator made something the parser refuses
	}
	k := w.Jobs["j"].Env.Vars["k"]
	if k == nil || k.Value == nil || k.Value.Value != want.String() {
		if k != nil && k.Value != nil {
			t.Fatalf("the generator and the parser disagree about the value of\n%s\nparser: %q\ngenerator: %q", src, k.Value.Value, want.String())
		}
		return false
	}
	full := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      K: " + strings.TrimPrefix(src, "a:\n  k: ")
	fileLines := strings.Split(full, "\n")
	s := k.Value
	off := 0
	srcOff := strings.Index(full, strings.TrimPrefix(src, "a:\n  k: "))
	if s.Quoted {
		srcOff++
	}
	for _, p := range pieces {
		l, c, ok := s.valueAt(off)
		if !ok {
			t.Fatalf("no place for the byte %d of\n%s", off, full)
		}
		if p.atStart {
			wl := strings.Count(full[:srcOff], "\n") + 1
			wc := utf8.RuneCountInString(full[strings.LastIndex(full[:srcOff], "\n")+1:srcOff]) + 1
			if l != wl || c != wc {
				t.Fatalf("byte %d (%q) is at %d:%d, want %d:%d (%q) in\n%s", off, p.value, l, c, wl, wc, textAt(full, wl, wc), full)
			}
		} else {
			// a folded line break is at the end of the line before it
			if l > len(fileLines) || c != utf8.RuneCountInString(strings.TrimRight(fileLines[l-1], " \t"))+1 {
				t.Fatalf("the folded break at byte %d is at %d:%d in\n%s", off, l, c, full)
			}
		}
		srcOff += len(p.src)
		off += len(p.value)
	}
	if l, c, ok := s.valueAt(off); !ok || l < 1 || c < 1 || l > len(fileLines) || c > utf8.RuneCountInString(fileLines[l-1])+1 {
		t.Fatalf("the end of the value is at %d:%d in\n%s", l, c, full)
	}
	return true
}

// The place of every byte of a block scalar is the byte in the line, after the indentation.
func TestScalarMapBlocks(t *testing.T) {
	for _, hdr := range []string{"|", "|-", "|+", ">", ">-", ">+"} {
		for _, body := range []string{
			"one two\n  three\n",
			"é ✓ 😀\n\n  next line\n  \n  last\n\n",
			"a\n\n\n  b\n    more indented\n  c\n",
		} {
			src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      K: " + hdr + "\n        " + strings.ReplaceAll(strings.TrimRight(body, "\n"), "\n  ", "\n        ") + "\n      L: x\n"
			w, errs := Parse([]byte(src))
			if w == nil {
				t.Fatalf("%v\n%s", errs, src)
			}
			s := w.Jobs["j"].Env.Vars["k"].Value
			lines := strings.Split(src, "\n")
			for off := 0; off < len(s.Value); off++ {
				if s.Value[off] == ' ' || s.Value[off] == '\n' || !utf8.RuneStart(s.Value[off]) {
					continue
				}
				l, c, ok := s.valueAt(off)
				if !ok {
					t.Fatalf("no place for byte %d of %q in\n%s", off, s.Value, src)
				}
				r, _ := utf8.DecodeRuneInString(s.Value[off:])
				if got, _ := utf8.DecodeRuneInString(textAt(src, l, c)); got != r {
					t.Fatalf("%s: byte %d (%q) is at %d:%d on %q in\n%s", hdr, off, r, l, c, lines[l-1], src)
				}
			}
		}
	}
}

// FuzzScalarPositions checks that the place of a byte of the value of any scalar is inside the file, whatever
// the text, and that the characters which are written as they are in the value are found where they are.
func FuzzScalarPositions(f *testing.F) {
	for _, s := range []string{"plain", `"a\nb"`, "'it''s'", ">\n  a\n  b", "|-\n  a\n\n  b", "a\n   b", `"x\
   y"`, `"\u00e9\x41"`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, scalar string) {
		src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    env:\n      K: " + strings.ReplaceAll(scalar, "\n", "\n        ") + "\n"
		w, _ := Parse([]byte(src))
		if w == nil || w.Jobs["j"] == nil || w.Jobs["j"].Env == nil || w.Jobs["j"].Env.Vars["k"] == nil {
			return
		}
		s := w.Jobs["j"].Env.Vars["k"].Value
		if s == nil {
			return
		}
		lines := strings.Split(src, "\n")
		prevL, prevC := 0, 0
		for off := 0; off <= len(s.Value); off++ {
			l, c, ok := s.valueAt(off)
			if !ok {
				return
			}
			if l < 1 || l > len(lines) || c < 1 || c > utf8.RuneCountInString(strings.TrimSuffix(lines[l-1], "\r"))+1 {
				t.Fatalf("byte %d of %q is at %d:%d, outside the file\n%s", off, s.Value, l, c, src)
			}
			if off > 0 && off < len(s.Value) && utf8.RuneStart(s.Value[off]) && (l < prevL || (l == prevL && c < prevC)) {
				t.Fatalf("byte %d of %q is at %d:%d, before the byte %d", off, s.Value, l, c, off-1)
			}
			prevL, prevC = l, c
		}
	})
}

// The shellcheck findings point at the token which shellcheck reports, in any style of the scalar.
func TestShellcheckPointsAtTheToken(t *testing.T) {
	sc, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck is not installed")
	}
	tests := []struct{ name, run, want string }{
		{"literal", "|\n          echo one\n          echo two $FOO bar\n", "$FOO"},
		{"literal with tab", "|\n          echo one\n          \techo $BAR\n", "$BAR"},
		{"folded", ">\n          echo one\n          $BAZ bar\n", "$BAZ"},
		{"double quoted", "\"echo one\\necho é $QUX\"", "$QUX"},
		{"multi-line expression before", "|\n          echo ${{\n            github.sha }}\n          echo $AFTER\n", "$AFTER"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := posHead + "    steps:\n      - run: " + tc.run
			var b strings.Builder
			l, err := NewLinter(&b, &LinterOptions{Format: FormatJSON, Shellcheck: sc})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = &Config{Profile: ProfilePedantic}
			errs, err := l.Lint("w.yaml", []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, e := range errs {
				if e.ID != "shellcheck" || !strings.Contains(e.Message, "SC2086") {
					continue
				}
				n++
				if text := textAt(src, e.Line, e.Column); !strings.HasPrefix(text, tc.want) {
					t.Errorf("SC2086 is at %d:%d on %q, want %q", e.Line, e.Column, text, tc.want)
				}
				if e.EndLine != e.Line || e.EndColumn-e.Column != len([]rune(tc.want)) {
					t.Errorf("the region is %d:%d-%d:%d, want %d characters", e.Line, e.Column, e.EndLine, e.EndColumn, len([]rune(tc.want)))
				}
			}
			if n == 0 {
				t.Errorf("no SC2086 in %v", errs)
			}
		})
	}
}
