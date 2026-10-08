package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestApplyFixes(t *testing.T) {
	src := []byte("0123456789")
	fix := func(unsafe bool, edits ...TextEdit) *Fix { return &Fix{Unsafe: unsafe, Edits: edits} }
	tests := []struct {
		name string
		errs []*Error
		mode FixMode
		want string
		n    int
	}{
		{"no error", nil, FixModeSafe, "0123456789", 0},
		{"no fix", []*Error{{}}, FixModeSafe, "0123456789", 0},
		{"one fix", []*Error{{Fix: fix(false, TextEdit{2, 4, "xx-"})}}, FixModeSafe, "01xx-456789", 1},
		{"insert and delete", []*Error{{Fix: fix(false, TextEdit{0, 0, "<"}, TextEdit{9, 10, ""})}}, FixModeSafe, "<012345678", 1},
		{"edits of several errors in any order", []*Error{{Fix: fix(false, TextEdit{6, 7, "S"})}, {Fix: fix(false, TextEdit{1, 2, "F"})}}, FixModeSafe, "0F2345S789", 2},
		{"unsafe is not applied by default", []*Error{{Fix: fix(true, TextEdit{2, 4, "xx"})}}, FixModeSafe, "0123456789", 0},
		{"unsafe is applied on request", []*Error{{Fix: fix(true, TextEdit{2, 4, "xx"})}}, FixModeUnsafe, "01xx456789", 1},
		{"a conflicting fix is skipped as a whole", []*Error{
			{Fix: fix(false, TextEdit{2, 5, "A"})},
			{Fix: fix(false, TextEdit{7, 8, "B"}, TextEdit{4, 6, "C"})},
			{Fix: fix(false, TextEdit{8, 9, "D"})},
		}, FixModeSafe, "01A567D9", 2},
		{"identical edits are applied once", []*Error{{Fix: fix(false, TextEdit{2, 3, "X"})}, {Fix: fix(false, TextEdit{2, 3, "X"})}}, FixModeSafe, "01X3456789", 2},
		{"insertions at the same place conflict", []*Error{{Fix: fix(false, TextEdit{2, 2, "a"})}, {Fix: fix(false, TextEdit{2, 2, "b"})}}, FixModeSafe, "01a23456789", 1},
		{"invalid fix is skipped", []*Error{{Fix: fix(false, TextEdit{2, 99, "X"})}, {Fix: fix(false, TextEdit{0, 1, "Y"})}}, FixModeSafe, "Y123456789", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, n := applyFixes(src, tc.errs, tc.mode)
			if string(got) != tc.want || n != tc.n {
				t.Errorf("want %q (%d fixes) but got %q (%d fixes)", tc.want, tc.n, got, n)
			}
			if string(src) != "0123456789" {
				t.Fatal("the source must not be modified")
			}
		})
	}
}

// replacingRule is a custom rule for tests. It reports every "run:" script which contains one of the keys
// of the replacements and offers a fix replacing it with the value.
type replacingRule struct {
	RuleBase
	path         string
	replacements map[string]string
	unsafe       bool
}

func (r *replacingRule) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok || run.Run == nil {
		return nil
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	// Offset of the value: the position is a line and a column in code points
	lines := strings.SplitAfter(string(b), "\n")
	off := 0
	for i := 0; i < run.Run.Pos.Line-1; i++ {
		off += len(lines[i])
	}
	line := lines[run.Run.Pos.Line-1]
	for from, to := range r.replacements {
		if i := strings.Index(line, from); i >= 0 {
			r.ReportIDf("fixable", run.Run.Pos, "%q should be %q", from, to)
			r.Errs()[len(r.Errs())-1].Fix = &Fix{Description: "replace " + from, Unsafe: r.unsafe, Edits: []TextEdit{{off + i, off + i + len(from), to}}}
		}
	}
	return nil
}

func newFixLinter(t *testing.T, path string, out *bytes.Buffer, replacements map[string]string, unsafe bool, opts LinterOptions) *Linter {
	t.Helper()
	opts.OnRulesCreated = func(rules []Rule) []Rule {
		return append(rules, &replacingRule{RuleBase: NewRuleBase("replacer", ""), path: path, replacements: replacements, unsafe: unsafe})
	}
	l, err := NewLinter(out, &opts)
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig()
	return l
}

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const fixWorkflow = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo aaa\n      - run: echo keep こんにちは bbb\n      - run: echo ${{ undefined_var }}\n"

func TestFixFiles(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": fixWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	if err := os.Chmod(path, 0o755); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)

	var out bytes.Buffer
	var log strings.Builder
	l := newFixLinter(t, path, &out, map[string]string{"aaa": "AAA", "bbb": "BBB"}, false, LinterOptions{WorkingDir: root, LogWriter: &log, Format: FormatGCC})
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(path)
	want := strings.NewReplacer("echo aaa", "echo AAA", "keep こんにちは bbb", "keep こんにちは BBB").Replace(fixWorkflow)
	if diff := cmp.Diff(want, string(b)); diff != "" {
		t.Errorf("file (-want +got): %s", diff)
	}
	if st2, _ := os.Stat(path); runtime.GOOS != "windows" && st2.Mode().Perm() != st.Mode().Perm() {
		t.Errorf("permission changed from %v to %v", st.Mode().Perm(), st2.Mode().Perm())
	}
	if res.Applied != 2 || len(res.Fixed) != 1 || res.Fixed[0] != path {
		t.Errorf("unexpected result: %+v", res)
	}
	// Only the error which cannot be fixed remains, and it is the only output
	if len(res.Errors) != 1 || res.Errors[0].ID != "undefined-property" {
		t.Errorf("unexpected remaining errors: %v", res.Errors)
	}
	if !strings.Contains(out.String(), "undefined-property") || strings.Contains(out.String(), "fixable") || strings.Count(out.String(), "\n") != 1 {
		t.Errorf("only remaining errors are printed: %q", out.String())
	}
	if !strings.Contains(log.String(), "Fixed 2 problem(s) in 1 file(s)") {
		t.Errorf("summary must be in the log: %q", log.String())
	}

	// Idempotent: nothing changes the second time and the file is not touched
	before, _ := os.Stat(path)
	out.Reset()
	log.Reset()
	l = newFixLinter(t, path, &out, map[string]string{"aaa": "AAA", "bbb": "BBB"}, false, LinterOptions{WorkingDir: root, LogWriter: &log, Format: FormatGCC})
	res, err = l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 0 || len(res.Fixed) != 0 || len(res.Errors) != 1 || strings.Contains(log.String(), "Fixed") {
		t.Errorf("the second run must not fix anything: %+v %q", res, log.String())
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("a file without fixes must not be rewritten")
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".jactionlint-fix-*")); len(left) != 0 {
		t.Errorf("temporary files remain: %v", left)
	}
}

func TestFixFilesUnsafe(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": fixWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	repl := map[string]string{"aaa": "AAA"}

	var out bytes.Buffer
	l := newFixLinter(t, path, &out, repl, true, LinterOptions{WorkingDir: root, Format: FormatGCC})
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != fixWorkflow || res.Applied != 0 || len(res.Errors) != 2 {
		t.Errorf("an unsafe fix must not be applied by default: %+v\n%s", res, b)
	}

	out.Reset()
	l = newFixLinter(t, path, &out, repl, true, LinterOptions{WorkingDir: root, Format: FormatGCC})
	res, err = l.FixFiles([]string{path}, nil, FixModeUnsafe)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if !strings.Contains(string(b), "echo AAA") || res.Applied != 1 || len(res.Errors) != 1 {
		t.Errorf("an unsafe fix must be applied on request: %+v\n%s", res, b)
	}
}

func TestFixFilesRepeatsUntilNothingChanges(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo one\n"})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")

	// The fix of the first pass makes the rule report the second one
	var out bytes.Buffer
	l := newFixLinter(t, path, &out, map[string]string{"one": "two", "two": "three"}, false, LinterOptions{WorkingDir: root})
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "echo three") {
		t.Errorf("fixing must repeat: %s", b)
	}
	if res.Applied != 2 || len(res.Fixed) != 1 || len(res.Errors) != 0 {
		t.Errorf("unexpected result: %+v", res)
	}

	// A rule whose fixes undo each other cannot make the loop endless
	if err := os.WriteFile(path, []byte("on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo aaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "aaa" -> "bbb" is applied in a pass and "bbb" -> "aaa" in the next pass
	flip := &flipRule{RuleBase: NewRuleBase("flip", ""), path: path}
	l, err = NewLinter(&out, &LinterOptions{WorkingDir: root, OnRulesCreated: func(rules []Rule) []Rule { return append(rules, flip) }})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig()
	res, err = l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != maxFixPasses {
		t.Errorf("fixing must stop after %d passes but applied %d fixes", maxFixPasses, res.Applied)
	}
}

type flipRule struct {
	RuleBase
	path string
}

func (r *flipRule) VisitStep(n *Step) error {
	b, _ := os.ReadFile(r.path)
	s := string(b)
	for _, p := range [][2]string{{"aaa", "bbb"}, {"bbb", "aaa"}} {
		if i := strings.Index(s, "echo "+p[0]); i >= 0 {
			r.ReportID("fixable", n.Pos, "flip")
			r.Errs()[len(r.Errs())-1].Fix = &Fix{Edits: []TextEdit{{i + 5, i + 8, p[1]}}}
			return nil
		}
	}
	return nil
}

func TestFixRepositoryAndManyFiles(t *testing.T) {
	wf := func(n string) string {
		return "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo " + n + "\n"
	}
	root := writeProject(t, map[string]string{
		".github/workflows/a.yaml": wf("aaa"),
		".github/workflows/b.yaml": wf("zzz"),
		".github/workflows/c.yaml": wf("aaa"),
	})
	// The rule reads the file by its path, so make a rule for each file through a shared lookup
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{WorkingDir: root, Format: FormatGCC, OnRulesCreated: func(rules []Rule) []Rule {
		return append(rules, &wholeFileRule{RuleBase: NewRuleBase("whole", ""), dir: filepath.Join(root, ".github", "workflows")})
	}})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = fixtureConfig()
	res, err := l.FixRepository(root, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 2 || len(res.Fixed) != 2 || len(res.Errors) != 0 {
		t.Errorf("unexpected result: %+v", res)
	}
	for _, n := range []string{"a", "c"} {
		b, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", n+".yaml"))
		if !strings.Contains(string(b), "echo AAA") {
			t.Errorf("%s.yaml was not fixed: %s", n, b)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "b.yaml"))
	if string(b) != wf("zzz") {
		t.Errorf("b.yaml must not change: %s", b)
	}

	// Errors
	if _, err := l.FixRepository(t.TempDir(), FixModeSafe); err == nil {
		t.Error("outside of a project must be an error")
	}
	if _, err := l.FixFiles([]string{filepath.Join(root, "nothing.yaml")}, nil, FixModeSafe); err == nil {
		t.Error("a missing file must be an error")
	}
	if _, err := l.FixFiles([]string{"x"}, nil, 0); err == nil {
		t.Error("an invalid mode must be an error")
	}
	if res, err := l.FixFiles(nil, nil, FixModeSafe); err != nil || res.Applied != 0 {
		t.Errorf("no file: %+v %v", res, err)
	}
}

// wholeFileRule fixes "aaa" in the "run:" of any file in the directory. It finds the file by the position.
type wholeFileRule struct {
	RuleBase
	dir string
}

func (r *wholeFileRule) VisitStep(n *Step) error {
	run, ok := n.Exec.(*ExecRun)
	if !ok {
		return nil
	}
	if !strings.Contains(run.Run.Value, "aaa") {
		return nil
	}
	entries, _ := os.ReadDir(r.dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(r.dir, e.Name()))
		lines := strings.SplitAfter(string(b), "\n")
		if len(lines) < run.Run.Pos.Line || !strings.Contains(lines[run.Run.Pos.Line-1], "aaa") {
			continue
		}
		off := 0
		for i := 0; i < run.Run.Pos.Line-1; i++ {
			off += len(lines[i])
		}
		i := strings.Index(lines[run.Run.Pos.Line-1], "aaa")
		r.ReportID("fixable", n.Pos, "aaa")
		r.Errs()[len(r.Errs())-1].Fix = &Fix{Edits: []TextEdit{{off + i, off + i + 3, "AAA"}}}
		return nil
	}
	return nil
}

func TestCommandFix(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": fixWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	hook := func(repl map[string]string, unsafe bool) func([]Rule) []Rule {
		return func(rules []Rule) []Rule {
			return append(rules, &replacingRule{RuleBase: NewRuleBase("replacer", ""), path: path, replacements: repl, unsafe: unsafe})
		}
	}
	run := func(hook func([]Rule) []Rule, args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		cmd := &Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, onRulesCreated: hook}
		code := cmd.Main(append([]string{"jactionlint", "-no-color", "-config-file", filepath.Join(root, "jactionlint.yaml")}, args...))
		return code, stdout.String(), stderr.String()
	}
	if err := os.WriteFile(filepath.Join(root, "jactionlint.yaml"), []byte("rules:\n  local-action-checkout: off\n  unsound-ternary: off\n  workflow-run-names: off\n  missing-timeout: off\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Unfixable errors remain: exit status 1
	code, stdout, stderr := run(hook(map[string]string{"aaa": "AAA"}, false), "-fix", path)
	if code != ExitStatusSuccessProblemFound || !strings.Contains(stdout, "undefined variable") || !strings.Contains(stderr, "Fixed 1 problem(s) in 1 file(s)") {
		t.Errorf("exit status %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "echo AAA") {
		t.Errorf("not fixed: %s", b)
	}

	// Run again: nothing to fix. The exit status stays 1 because of the remaining error
	code, stdout, stderr = run(hook(map[string]string{"aaa": "AAA"}, false), "-fix", path)
	if code != ExitStatusSuccessProblemFound || strings.Contains(stderr, "Fixed") || !strings.Contains(stdout, "undefined variable") {
		t.Errorf("exit status %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// Everything is fixed: exit status 0 and no output
	if err := os.WriteFile(path, []byte(strings.Replace(fixWorkflow, "\n      - run: echo ${{ undefined_var }}", "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = run(hook(map[string]string{"aaa": "AAA"}, false), "-fix", path)
	if code != ExitStatusSuccessNoProblem || stdout != "" || !strings.Contains(stderr, "Fixed 1 problem(s)") {
		t.Errorf("exit status %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	// With SARIF the only thing on stdout is the log of what remains
	if err := os.WriteFile(path, []byte(fixWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = run(hook(map[string]string{"aaa": "AAA"}, false), "-fix", "-format", "sarif", path)
	results := sarifRunOf(t, stdout)["results"].([]any)
	if code != ExitStatusSuccessProblemFound || len(results) != 1 || results[0].(sarifDoc)["ruleId"] != "undefined-property" {
		t.Errorf("exit status %d, results %v", code, results)
	}

	// -fix=unsafe
	if err := os.WriteFile(path, []byte(fixWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, _ = run(hook(map[string]string{"aaa": "AAA"}, true), "-fix", path)
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "echo AAA") || code != ExitStatusSuccessProblemFound {
		t.Errorf("an unsafe fix must not be applied by -fix: %s", b)
	}
	run(hook(map[string]string{"aaa": "AAA"}, true), "-fix=unsafe", path)
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "echo AAA") {
		t.Errorf("-fix=unsafe must apply it: %s", b)
	}

	// Only warnings remain: exit status 0
	if err := os.WriteFile(filepath.Join(root, "jactionlint.yaml"), []byte("rules:\n  undefined-property: warn\n  missing-timeout: off\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ = run(nil, "-fix", path)
	if code != ExitStatusSuccessNoProblem || !strings.Contains(stdout, "warning:") {
		t.Errorf("exit status %d, stdout %s", code, stdout)
	}

	// Without a file, the files of the project are fixed
	if err := os.WriteFile(path, []byte(fixWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout2, stderr2 bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &stdout2, Stderr: &stderr2, onRulesCreated: hook(map[string]string{"aaa": "AAA"}, false)}
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	cmd.Main([]string{"jactionlint", "-no-color", "-fix"})
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "echo AAA") {
		t.Errorf("the project was not fixed: %s\n%s", b, stderr2.String())
	}

	// Invalid usage
	code, _, stderr = run(nil, "-fix", "-")
	if code != ExitStatusFailure || !strings.Contains(stderr, "stdin") {
		t.Errorf("-fix with stdin: %d %q", code, stderr)
	}
	code, _, stderr = run(nil, "-fix=maybe", path)
	if code != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "-fix") {
		t.Errorf("-fix=maybe: %d %q", code, stderr)
	}
	code, _, _ = run(nil, "-fix=false", path)
	if code != ExitStatusSuccessNoProblem && code != ExitStatusSuccessProblemFound {
		t.Errorf("-fix=false: %d", code)
	}
}

func TestFixFlag(t *testing.T) {
	var f fixFlag
	if f.String() != "false" || !f.IsBoolFlag() {
		t.Errorf("unexpected default: %q", f.String())
	}
	for in, want := range map[string]FixMode{"true": FixModeSafe, "safe": FixModeSafe, "UNSAFE": FixModeUnsafe, "false": 0} {
		if err := f.Set(in); err != nil || f.mode != want {
			t.Errorf("Set(%q): %v %v", in, f.mode, err)
		}
	}
	f.Set("false")
	if f.String() != "false" {
		t.Error(f.String())
	}
	f.Set("unsafe")
	if f.String() != "unsafe" {
		t.Error(f.String())
	}
	f.Set("safe")
	if f.String() != "safe" {
		t.Error(f.String())
	}
}
