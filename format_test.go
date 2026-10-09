package jactionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

func lintFormat(t *testing.T, format string, opts LinterOptions, cfg *Config, path string) (string, []*Error) {
	t.Helper()
	opts.Format = format
	var b strings.Builder
	l, err := NewLinter(&b, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		cfg = &Config{}
	}
	l.defaultConfig = withoutMissingTimeout(cfg)
	errs, err := l.LintFile(path, &Project{root: filepath.Dir(path)})
	if err != nil {
		t.Fatal(err)
	}
	return b.String(), errs
}

var formatTestFile = filepath.Join("testdata", "format", "test.yaml")

func TestNativeJSONFormatsAreTheSameAsTemplates(t *testing.T) {
	for _, tc := range []struct{ format, file string }{
		{FormatJSON, "test.json"},
		{FormatJSONL, "test.jsonl"},
	} {
		want, err := os.ReadFile(filepath.Join("testdata", "format", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		have, _ := lintFormat(t, tc.format, LinterOptions{}, nil, formatTestFile)
		if runtimeIsWindows() {
			have = strings.ReplaceAll(have, `testdata\\format\\`, "testdata/format/")
		}
		if diff := cmp.Diff(string(want), have); diff != "" {
			t.Errorf("%s (-want +got): %s", tc.format, diff)
		}
	}

	// No error is an empty array, not null, and nothing for JSON Lines
	clean := filepath.Join("testdata", "ok", "minimal.yaml")
	if out, errs := lintFormat(t, FormatJSON, LinterOptions{}, nil, clean); out != "[]\n" || len(errs) != 0 {
		t.Errorf("unexpected output for json: %q %v", out, errs)
	}
	if out, _ := lintFormat(t, FormatJSONL, LinterOptions{}, nil, clean); out != "" {
		t.Errorf("unexpected output for jsonl: %q", out)
	}
}

func runtimeIsWindows() bool { return filepath.Separator == '\\' }

func TestJSONFormatFields(t *testing.T) {
	out, _ := lintFormat(t, FormatJSON, LinterOptions{}, mustParseConfig(t, "rules:\n  workflow-syntax: warn\n"), formatTestFile)
	var have []map[string]any
	if err := json.Unmarshal([]byte(out), &have); err != nil {
		t.Fatal(err)
	}
	if len(have) != 3 {
		t.Fatalf("want 3 errors: %s", out)
	}
	e := have[0]
	if e["id"] != "workflow-syntax" || e["severity"] != "warn" || e["kind"] != "syntax-check" ||
		e["doc_url"] != "https://jactionlint.jdx.dev/rules#workflow-syntax" || e["end_line"] != float64(3) {
		t.Errorf("unexpected fields: %v", e)
	}
	if _, ok := e["fix"]; ok {
		t.Error("fix must be omitted when there is no fix")
	}
	if have[1]["id"] != "undefined-property" || have[1]["severity"] != "error" {
		t.Errorf("unexpected fields: %v", have[1])
	}
}

func TestTextFormats(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  workflow-syntax: warn\n  undefined-property: info\n")
	src, err := os.ReadFile(formatTestFile)
	if err != nil {
		t.Fatal(err)
	}
	_ = src

	// The text format of errors keeps the former output
	out, _ := lintFormat(t, "", LinterOptions{}, nil, formatTestFile)
	want := `testdata/format/test.yaml:3:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [workflow-syntax]
  |
3 |     branch: main
  |     ^~~~~~~
testdata/format/test.yaml:9:23: property "msg" is not defined in object type {} [undefined-property]
  |
9 |       - run: echo ${{ matrix.msg }}
  |                       ^~~~~~~~~~
testdata/format/test.yaml:10:9: unexpected key "with" for step to run shell command. expected one of "background", "continue-on-error", "env", "id", "if", "name", "run", "shell", "timeout-minutes", "working-directory" [workflow-syntax]
   |
10 |         with:
   |         ^~~~~
`
	if runtimeIsWindows() {
		out = strings.ReplaceAll(out, `\`, "/")
	}
	if diff := cmp.Diff(want, out); diff != "" {
		t.Errorf("default text (-want +got): %s", diff)
	}
	explicit, _ := lintFormat(t, FormatText, LinterOptions{}, nil, formatTestFile)
	if explicit != out && !runtimeIsWindows() {
		t.Error("--format text must be the same as the default")
	}

	// Levels other than error are prefixed. --rule-ids changes nothing
	out, _ = lintFormat(t, "", LinterOptions{ShowRuleIDs: true}, cfg, formatTestFile)
	for _, w := range []string{
		`: warning: unexpected key "branch"`, `[workflow-syntax]`,
		`: info: property "msg" is not defined`, `[undefined-property]`,
	} {
		if !strings.Contains(out, w) {
			t.Errorf("%q is not in the output:\n%s", w, out)
		}
	}
	if strings.Contains(out, "[syntax-check]") || strings.Contains(out, "[expression]") {
		t.Errorf("kinds must be replaced by IDs:\n%s", out)
	}
	if !strings.Contains(explicit, "[workflow-syntax]") || strings.Contains(explicit, "[syntax-check]") {
		t.Errorf("the text format shows the rule ID without any option:\n%s", explicit)
	}

	// oneline: --oneline and --format oneline
	for _, o := range []struct {
		format string
		opts   LinterOptions
	}{{FormatOneline, LinterOptions{}}, {"", LinterOptions{Oneline: true}}, {FormatText, LinterOptions{Oneline: true}}} {
		out, _ = lintFormat(t, o.format, o.opts, nil, formatTestFile)
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if len(lines) != 3 {
			t.Errorf("format %q: one line per error is expected:\n%s", o.format, out)
		}
	}
}

func TestGCCFormat(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  workflow-syntax: warn\n  undefined-property: info\n")
	out, _ := lintFormat(t, FormatGCC, LinterOptions{}, cfg, formatTestFile)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("unexpected output: %q", out)
	}
	for i, prefix := range []string{
		"testdata/format/test.yaml:3:5: warning: unexpected key ",
		"testdata/format/test.yaml:9:23: note: property \"msg\" is not defined in object type {} [undefined-property]",
		"testdata/format/test.yaml:10:9: warning: unexpected key ",
	} {
		l := filepath.ToSlash(lines[i])
		if !strings.HasPrefix(l, prefix) {
			t.Errorf("line %d: want prefix %q but got %q", i, prefix, l)
		}
	}
	if !strings.HasSuffix(lines[0], " [workflow-syntax]") {
		t.Errorf("the ID must be at the end: %q", lines[0])
	}

	out, _ = lintFormat(t, FormatGCC, LinterOptions{}, nil, formatTestFile)
	if !strings.Contains(out, ": error: ") {
		t.Errorf("errors are printed as error: %q", out)
	}
}

func TestGitHubFormat(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  workflow-syntax: warn\n  undefined-property: info\n")
	out, _ := lintFormat(t, FormatGitHub, LinterOptions{}, cfg, formatTestFile)
	lines := strings.Split(strings.TrimSuffix(filepath.ToSlash(out), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("unexpected output: %q", out)
	}
	want := []string{
		"::warning file=testdata/format/test.yaml,line=3,endLine=3,col=5,endColumn=11,title=workflow-syntax::unexpected key ",
		"::notice file=testdata/format/test.yaml,line=9,endLine=9,col=23,endColumn=32,title=undefined-property::property \"msg\" is not defined in object type {}",
		"::warning file=testdata/format/test.yaml,line=10,endLine=10,col=9,endColumn=13,title=workflow-syntax::unexpected key ",
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("line %d: want prefix %q but got %q", i, w, lines[i])
		}
	}
	out, _ = lintFormat(t, FormatGitHub, LinterOptions{}, nil, formatTestFile)
	if !strings.HasPrefix(out, "::error file=") {
		t.Errorf("unexpected output: %q", out)
	}

	// Escaping of data and properties
	var b bytes.Buffer
	err := githubPrinter{}.print(&b, []fileResult{{errs: []*Error{{
		Filepath: "a,b:c%.yaml", Line: 1, Column: 2, EndLine: 1, EndColumn: 3, ID: "x:y", Severity: SeverityError,
		Message: "100%\nnext\r",
	}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "::error file=a%2Cb%3Ac%25.yaml,line=1,endLine=1,col=2,endColumn=2,title=x%3Ay::100%25%0Anext%0D\n"; b.String() != want {
		t.Errorf("want %q but got %q", want, b.String())
	}
}

func TestInvalidFormat(t *testing.T) {
	for _, f := range []string{"hello", "SARIF", "{{ .", "xml"} {
		_, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Format: f})
		if err == nil {
			t.Errorf("%q must be an error", f)
		}
	}
	_, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Format: "hello"})
	if err == nil || !strings.Contains(err.Error(), `invalid format "hello"`) || !strings.Contains(err.Error(), `"sarif"`) {
		t.Errorf("unexpected error: %v", err)
	}
	// A template which does not parse reports the template error
	_, err = NewLinter(&bytes.Buffer{}, &LinterOptions{Format: "{{ ."})
	if err == nil || !strings.Contains(err.Error(), "could not be parsed") {
		t.Errorf("unexpected error: %v", err)
	}
	// The templates keep working, including the ones using the new functions
	out, _ := lintFormat(t, `{{range $r := allRules}}{{if eq $r.ID "unpinned-uses"}}{{$r.ID}} {{$r.Group}} {{$r.DefaultLevel}} {{$r.Profile}} {{$r.URL}} {{$r.Name}}{{end}}{{end}}`, LinterOptions{}, nil, formatTestFile)
	if want := "unpinned-uses policy error default https://jactionlint.jdx.dev/rules#unpinned-uses UnpinnedUses"; out != want {
		t.Errorf("want %q but got %q", want, out)
	}
	out, _ = lintFormat(t, `{{range .}}{{.ID}}:{{.Severity}}:{{.EndLine}} {{end}}`, LinterOptions{}, nil, formatTestFile)
	if want := "workflow-syntax:error:3 undefined-property:error:9 workflow-syntax:error:10 "; out != want {
		t.Errorf("want %q but got %q", want, out)
	}
}

// --- SARIF ----------------------------------------------------------------------------------

// fixRule is a custom rule which reports an error with a fix on every step of a job.
type fixRule struct {
	RuleBase
	fixes func(s *Step) *Fix
}

func (r *fixRule) VisitStep(n *Step) error {
	r.ReportID("fixable", n.Pos, "this step has a fix")
	r.Errs()[len(r.Errs())-1].Fix = r.fixes(n)
	return nil
}

func lintWithFixes(t *testing.T, src string, format string, fixes func(s *Step) *Fix) (string, []*Error) {
	t.Helper()
	var b strings.Builder
	l, err := NewLinter(&b, &LinterOptions{
		Format: format,
		OnRulesCreated: func(rules []Rule) []Rule {
			return append(rules, &fixRule{RuleBase: NewRuleBase("fixer", ""), fixes: fixes})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(&Config{})
	errs, err := l.Lint("wf.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return b.String(), errs
}

type sarifDoc = map[string]any

func sarifRunOf(t *testing.T, out string) sarifDoc {
	t.Helper()
	var doc sarifDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	runs, ok := doc["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("one run is expected: %s", out)
	}
	return runs[0].(sarifDoc)
}

func TestSARIFFormat(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  workflow-syntax: warn\n  undefined-property: info\n")
	out, _ := lintFormat(t, FormatSARIF, LinterOptions{}, cfg, formatTestFile)

	var doc sarifDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["version"] != "2.1.0" || doc["$schema"] != "https://json.schemastore.org/sarif-2.1.0.json" {
		t.Errorf("unexpected header: %v", doc)
	}
	run := sarifRunOf(t, out)
	if run["columnKind"] != "unicodeCodePoints" {
		t.Errorf("columnKind must be explicit: %v", run["columnKind"])
	}
	driver := run["tool"].(sarifDoc)["driver"].(sarifDoc)
	if driver["name"] != "jactionlint" || driver["informationUri"] != "https://github.com/jdx/jactionlint" {
		t.Errorf("unexpected driver: %v", driver)
	}

	rules := driver["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("only the rules in the results are described: %v", rules)
	}
	r0 := rules[0].(sarifDoc)
	if r0["id"] != "undefined-property" || r0["name"] != "UndefinedProperty" || r0["helpUri"] != "https://jactionlint.jdx.dev/rules#undefined-property" ||
		r0["shortDescription"].(sarifDoc)["text"] != "An undefined variable or property is accessed in an expression." ||
		r0["defaultConfiguration"].(sarifDoc)["level"] != "error" {
		t.Errorf("unexpected rule: %v", r0)
	}
	if tags := r0["properties"].(sarifDoc)["tags"].([]any); len(tags) != 1 || tags[0] != "correctness" {
		t.Errorf("unexpected tags: %v", tags)
	}
	if rules[1].(sarifDoc)["id"] != "workflow-syntax" {
		t.Errorf("rules are sorted by ID: %v", rules)
	}

	results := run["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("want 3 results: %v", results)
	}
	type want struct {
		ruleID, level string
		ruleIndex     float64
		line, col     float64
		endCol        float64
	}
	for i, w := range []want{
		{"workflow-syntax", "warning", 1, 3, 5, 12},
		{"undefined-property", "note", 0, 9, 23, 33},
		{"workflow-syntax", "warning", 1, 10, 9, 14},
	} {
		r := results[i].(sarifDoc)
		loc := r["locations"].([]any)[0].(sarifDoc)["physicalLocation"].(sarifDoc)
		reg := loc["region"].(sarifDoc)
		uri := loc["artifactLocation"].(sarifDoc)["uri"]
		if r["ruleId"] != w.ruleID || r["level"] != w.level || r["ruleIndex"] != w.ruleIndex ||
			reg["startLine"] != w.line || reg["startColumn"] != w.col || reg["endLine"] != w.line || reg["endColumn"] != w.endCol ||
			(uri != "testdata/format/test.yaml" && !runtimeIsWindows()) {
			t.Errorf("result %d: unexpected %v (want %+v)", i, r, w)
		}
		if _, ok := r["fixes"]; ok {
			t.Errorf("result %d must have no fixes", i)
		}
		if r["message"].(sarifDoc)["text"] == "" {
			t.Errorf("result %d has no message", i)
		}
		if r["properties"].(sarifDoc)["kind"] == "" {
			t.Errorf("result %d has no kind", i)
		}
	}

	// No result is an empty array and the log is still valid
	out, _ = lintFormat(t, FormatSARIF, LinterOptions{}, nil, filepath.Join("testdata", "ok", "minimal.yaml"))
	run = sarifRunOf(t, out)
	if r, ok := run["results"].([]any); !ok || len(r) != 0 {
		t.Errorf("results must be an empty array: %v", run["results"])
	}
	if r, ok := run["tool"].(sarifDoc)["driver"].(sarifDoc)["rules"].([]any); !ok || len(r) != 0 {
		t.Errorf("rules must be an empty array: %v", r)
	}
}

func TestSARIFFormatHasNoOutputBesidesTheLog(t *testing.T) {
	// hk parses stdout and stderr together so nothing may be written to stderr in this format. A deprecated key
	// is reported in the log of the SARIF document instead.
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("require-shell: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	code := cmd.Main([]string{"jactionlint", "--format", "sarif", "--config-file", p, formatTestFile, formatTestFile})
	if code != ExitStatusSuccessProblemFound {
		t.Errorf("exit status %d", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must be empty but got %q", stderr.String())
	}
	var doc sarifDoc
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout must be only the SARIF log: %v\n%s", err, stdout.String())
	}
	inv := sarifRunOf(t, stdout.String())["invocations"].([]any)
	if len(inv) != 1 || inv[0].(sarifDoc)["executionSuccessful"] != true {
		t.Fatalf("unexpected invocations: %v", inv)
	}
	notes := inv[0].(sarifDoc)["toolConfigurationNotifications"].([]any)
	if len(notes) != 1 || notes[0].(sarifDoc)["level"] != "warning" ||
		!strings.Contains(notes[0].(sarifDoc)["message"].(sarifDoc)["text"].(string), `"require-shell" is deprecated`) {
		t.Errorf("the deprecation must be in the document once: %v", notes)
	}

	// Other formats keep writing the warning to stderr
	stdout.Reset()
	stderr.Reset()
	cmd.Main([]string{"jactionlint", "--format", "json", "--config-file", p, formatTestFile})
	if !strings.Contains(stderr.String(), `"require-shell" is deprecated`) {
		t.Errorf("stderr: %q", stderr.String())
	}
	var arr []any
	if err := json.Unmarshal(stdout.Bytes(), &arr); err != nil {
		t.Fatalf("stdout must be only the JSON: %v", err)
	}

	// Without deprecations the document has no invocation
	stdout.Reset()
	cmd.Main([]string{"jactionlint", "--format", "sarif", formatTestFile})
	if _, ok := sarifRunOf(t, stdout.String())["invocations"]; ok {
		t.Error("invocations must be omitted when there is nothing to tell")
	}
}

func TestSARIFOrdersFilesByPath(t *testing.T) {
	var b bytes.Buffer
	err := sarifPrinter{}.print(&b, []fileResult{
		{path: "b.yaml", errs: []*Error{{Filepath: "b.yaml", Line: 1, Column: 1, ID: "invalid-glob", Severity: SeverityError, Message: "b"}}},
		{path: "a.yaml", errs: []*Error{{Filepath: "a.yaml", Line: 1, Column: 1, ID: "invalid-glob", Severity: SeverityError, Message: "a"}}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	results := sarifRunOf(t, b.String())["results"].([]any)
	if results[0].(sarifDoc)["message"].(sarifDoc)["text"] != "a" {
		t.Errorf("files must be sorted: %v", results)
	}
}

func TestSARIFURI(t *testing.T) {
	tests := map[string]string{
		".github/workflows/ci.yaml": ".github/workflows/ci.yaml",
		"dir with space/a b.yaml":   "dir%20with%20space/a%20b.yaml",
		"<stdin>":                   "%3Cstdin%3E",
		"日本語.yaml":                  "%E6%97%A5%E6%9C%AC%E8%AA%9E.yaml",
		"a:b/c.yaml":                "./a:b/c.yaml",
	}
	for in, want := range tests {
		if got := sarifURI(in); got != want {
			t.Errorf("sarifURI(%q) = %q, want %q", in, got, want)
		}
	}
	if !runtimeIsWindows() {
		if got := sarifURI("/abs/dir/a.yaml"); got != "file:///abs/dir/a.yaml" {
			t.Errorf("absolute path: %q", got)
		}
	}
}

func TestOffsetPosition(t *testing.T) {
	src := []byte("ab\r\nこ😀d\nlast")
	tests := []struct {
		off       int
		line, col int
	}{
		{0, 1, 1},
		{1, 1, 2},
		{2, 1, 3},  // at \r: the end of the line
		{3, 1, 3},  // at \n: the end of the line, too
		{4, 2, 1},  // start of the line 2
		{7, 2, 2},  // after こ (3 bytes)
		{11, 2, 3}, // after 😀 (4 bytes)
		{12, 2, 4}, // at \n
		{13, 3, 1},
		{17, 3, 5},
	}
	for _, tc := range tests {
		l, c := offsetPosition(src, tc.off)
		if l != tc.line || c != tc.col {
			t.Errorf("offset %d: want %d:%d but got %d:%d", tc.off, tc.line, tc.col, l, c)
		}
	}
}

func TestEditsConflict(t *testing.T) {
	e := func(s, e int, t string) TextEdit { return TextEdit{s, e, t} }
	tests := []struct {
		a, b TextEdit
		want bool
	}{
		{e(0, 2, "x"), e(2, 4, "y"), false},
		{e(0, 3, "x"), e(2, 4, "y"), true},
		{e(2, 4, "y"), e(0, 3, "x"), true},
		{e(1, 2, "x"), e(1, 2, "x"), false}, // identical edits are applied once
		{e(1, 1, "x"), e(1, 1, "y"), true},  // two insertions at the same place
		{e(1, 1, "x"), e(1, 3, "y"), true},
		{e(1, 3, "y"), e(1, 1, "x"), true},
		{e(1, 1, "x"), e(2, 3, "y"), false},
	}
	for i, tc := range tests {
		if got := editsConflict(tc.a, tc.b); got != tc.want {
			t.Errorf("case %d: want %v but got %v", i, tc.want, got)
		}
	}
}

// applySARIFFixes applies the fixes of the log to the source the way "hk util sarif-diff" does: it
// reads the columns according to the columnKind of the run and converts the regions to byte ranges.
// It returns false when a fix cannot be applied or a result has no fix.
func applySARIFFixes(t *testing.T, out string, src string) (string, bool) {
	t.Helper()
	run := sarifRunOf(t, out)
	utf16Columns := run["columnKind"] != "unicodeCodePoints"

	lineStart := func(n int) (string, int, bool) {
		off := 0
		for i, l := range strings.SplitAfter(src, "\n") {
			if i+1 == n {
				text := strings.TrimSuffix(strings.TrimSuffix(l, "\n"), "\r")
				return text, off, true
			}
			off += len(l)
		}
		return "", 0, false
	}
	colOffset := func(text string, col int) (int, bool) {
		count := col - 1
		units := 0
		for off, r := range text {
			if units == count {
				return off, true
			}
			if utf16Columns {
				units += len(utf16.Encode([]rune{r}))
			} else {
				units++
			}
			if units > count {
				return 0, false
			}
		}
		return len(text), units == count
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for _, r := range run["results"].([]any) {
		fixes, ok := r.(sarifDoc)["fixes"].([]any)
		if !ok || len(fixes) == 0 {
			return "", false
		}
		for _, change := range fixes[0].(sarifDoc)["artifactChanges"].([]any) {
			for _, rep := range change.(sarifDoc)["replacements"].([]any) {
				reg := rep.(sarifDoc)["deletedRegion"].(sarifDoc)
				num := func(k string, def int) int {
					if v, ok := reg[k].(float64); ok {
						return int(v)
					}
					return def
				}
				sl, sc := num("startLine", 0), num("startColumn", 1)
				el, ec := num("endLine", sl), num("endColumn", -1)
				st, so, ok1 := lineStart(sl)
				et, eo, ok2 := lineStart(el)
				if !ok1 || !ok2 {
					return "", false
				}
				so2, ok1 := colOffset(st, sc)
				var eo2 int
				ok2 = true
				if ec < 0 {
					eo2 = len(et)
				} else {
					eo2, ok2 = colOffset(et, ec)
				}
				if !ok1 || !ok2 {
					return "", false
				}
				edits = append(edits, edit{so + so2, eo + eo2, rep.(sarifDoc)["insertedContent"].(sarifDoc)["text"].(string)})
			}
		}
	}
	for i := 0; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			a, b := edits[i], edits[j]
			if a == b {
				continue
			}
			if a.start > b.start {
				a, b = b, a
			}
			if b.start < a.end || (b.start == a.start && a.end == a.start) {
				return "", false
			}
		}
	}
	// Apply from the end
	res := src
	for len(edits) > 0 {
		best := 0
		for i, e := range edits {
			if e.start > edits[best].start {
				best = i
			}
		}
		e := edits[best]
		res = res[:e.start] + e.text + res[e.end:]
		edits = append(edits[:best], edits[best+1:]...)
	}
	return res, true
}

func applyFixDirectly(src string, fixes ...*Fix) string {
	var edits []TextEdit
	for _, f := range fixes {
		edits = append(edits, f.Edits...)
	}
	res := []byte(src)
	for len(edits) > 0 {
		best := 0
		for i, e := range edits {
			if e.Start > edits[best].Start {
				best = i
			}
		}
		e := edits[best]
		res = append(res[:e.Start:e.Start], append([]byte(e.NewText), res[e.End:]...)...)
		edits = append(edits[:best], edits[best+1:]...)
	}
	return string(res)
}

const fixSrcLF = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \"こんにちは😀\" # one\n      - run: echo two\n      - run: echo three\n"

func TestSARIFFixesMatchHkContract(t *testing.T) {
	for _, src := range []string{fixSrcLF, strings.ReplaceAll(fixSrcLF, "\n", "\r\n")} {
		name := "LF"
		if strings.Contains(src, "\r") {
			name = "CRLF"
		}
		t.Run(name, func(t *testing.T) {
			// Step 1: replace the multi-byte string. Step 2: insert a line. Step 3: delete a comment-free tail
			off := func(s string) int {
				i := strings.Index(src, s)
				if i < 0 {
					t.Fatalf("%q not found", s)
				}
				return i
			}
			var fixes []*Fix
			fixFor := func(s *Step) *Fix {
				var f *Fix
				switch s.Pos.Line {
				case 6:
					a := off("こんにちは😀")
					f = &Fix{Description: "Replace the greeting", Edits: []TextEdit{{a, a + len("こんにちは😀"), "hello"}, {off(" # one"), off(" # one") + len(" # one"), ""}}}
				case 7:
					a := off("echo two")
					f = &Fix{Description: "Insert text", Edits: []TextEdit{{a, a, "set -e; "}}}
				case 8:
					a := off("three")
					f = &Fix{Description: "Replace to the end of the file's line", Edits: []TextEdit{{a, a + len("three"), "3\n      # added"}}}
				}
				fixes = append(fixes, f)
				return f
			}
			out, errs := lintWithFixes(t, src, FormatSARIF, fixFor)
			if len(errs) != 3 {
				t.Fatalf("want 3 errors: %v", errs)
			}

			run := sarifRunOf(t, out)
			for i, r := range run["results"].([]any) {
				fx := r.(sarifDoc)["fixes"].([]any)
				if len(fx) != 1 {
					t.Fatalf("result %d must have one fix: %v", i, r)
				}
				f := fx[0].(sarifDoc)
				if f["description"].(sarifDoc)["text"] != fixes[i].Description {
					t.Errorf("unexpected description: %v", f["description"])
				}
				changes := f["artifactChanges"].([]any)
				if len(changes) != 1 || changes[0].(sarifDoc)["artifactLocation"].(sarifDoc)["uri"] != "wf.yaml" {
					t.Errorf("unexpected changes: %v", changes)
				}
				for _, rep := range changes[0].(sarifDoc)["replacements"].([]any) {
					reg := rep.(sarifDoc)["deletedRegion"].(sarifDoc)
					for _, k := range []string{"startLine", "startColumn", "endLine", "endColumn"} {
						if _, ok := reg[k].(float64); !ok {
							t.Errorf("deletedRegion must have %s: %v", k, reg)
						}
					}
					if _, ok := rep.(sarifDoc)["insertedContent"].(sarifDoc)["text"].(string); !ok {
						t.Errorf("insertedContent.text is required: %v", rep)
					}
				}
			}

			have, ok := applySARIFFixes(t, out, src)
			if !ok {
				t.Fatalf("hk would not be able to apply the fixes:\n%s", out)
			}
			if want := applyFixDirectly(src, fixes...); have != want {
				t.Errorf("applying the SARIF fixes differs from applying the edits\nwant: %q\nhave: %q", want, have)
			}
			if !utf8.ValidString(have) {
				t.Error("the result is not valid UTF-8")
			}
		})
	}
}

func TestSARIFOmitsFixesWhichCannotBeApplied(t *testing.T) {
	src := fixSrcLF
	a := strings.Index(src, "echo two")
	b := strings.Index(src, "echo three")
	fix := func(s *Step) *Fix {
		switch s.Pos.Line {
		case 6:
			return &Fix{Description: "unsafe", Unsafe: true, Edits: []TextEdit{{a, a, "x"}}}
		case 7:
			return &Fix{Description: "safe", Edits: []TextEdit{{a, a + 4, "ECHO"}}}
		case 8:
			// Overlaps the fix of the previous result
			return &Fix{Description: "conflict", Edits: []TextEdit{{a + 2, b, ""}}}
		}
		return nil
	}
	out, _ := lintWithFixes(t, src, FormatSARIF, fix)
	results := sarifRunOf(t, out)["results"].([]any)
	has := []bool{}
	for _, r := range results {
		_, ok := r.(sarifDoc)["fixes"]
		has = append(has, ok)
	}
	// An unsafe fix and a fix which conflicts with an accepted one are not in the log: the finding stays
	// unfixable so that the step's own fixer runs
	if diff := cmp.Diff([]bool{false, true, false}, has); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}

	for name, f := range map[string]*Fix{
		"out of range": {Edits: []TextEdit{{0, len(src) + 1, ""}}},
		"reversed":     {Edits: []TextEdit{{5, 3, ""}}},
		"no edit":      {},
		"overlapping":  {Edits: []TextEdit{{0, 5, ""}, {3, 6, ""}}},
	} {
		out, _ := lintWithFixes(t, "on: push\njobs: {}\n", FormatSARIF, func(*Step) *Fix { return f })
		_ = out
		// The workflow above has no step, so use the real source
		out, _ = lintWithFixes(t, src, FormatSARIF, func(s *Step) *Fix {
			if s.Pos.Line == 6 {
				return f
			}
			return nil
		})
		if strings.Contains(out, `"fixes"`) {
			t.Errorf("%s: an invalid fix must be left out: %s", name, out)
		}
	}
}

func TestFixInJSONOutput(t *testing.T) {
	out, errs := lintWithFixes(t, fixSrcLF, FormatJSON, func(s *Step) *Fix {
		if s.Pos.Line == 7 {
			return &Fix{Description: "d", Edits: []TextEdit{{1, 2, "x"}}}
		}
		return nil
	})
	if len(errs) != 3 {
		t.Fatal(errs)
	}
	var have []map[string]any
	if err := json.Unmarshal([]byte(out), &have); err != nil {
		t.Fatal(err)
	}
	if fix, ok := have[1]["fix"].(map[string]any); !ok || fix["description"] != "d" {
		t.Errorf("unexpected fix: %v", have[1])
	}
	if _, ok := have[0]["fix"]; ok {
		t.Errorf("fix must be omitted: %v", have[0])
	}
	_ = fmt.Sprint()
}
