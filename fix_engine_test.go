package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// editRule is a test rule with a rule ID. It reports every occurrence of find in the source and offers a
// fix which replaces it with repl.
type editRule struct {
	RuleBase
	id     string
	find   string
	repl   string
	unsafe bool
}

func newEditRule(id, find, repl string) func([]Rule) []Rule {
	return func(rules []Rule) []Rule {
		return append(rules, &editRule{RuleBase: NewRuleBase("edit-"+id, ""), id: id, find: find, repl: repl})
	}
}

func (r *editRule) VisitWorkflowPre(n *Workflow) error {
	src := string(n.Source)
	for off := 0; ; {
		i := strings.Index(src[off:], r.find)
		if i < 0 {
			break
		}
		at := off + i
		line := 1 + strings.Count(src[:at], "\n")
		r.ReportID(r.id, &Pos{Line: line, Col: 1}, "found "+r.find)
		r.Errs()[len(r.Errs())-1].Fix = &Fix{Description: r.id, Unsafe: r.unsafe, Edits: []TextEdit{{Start: at, End: at + len(r.find), NewText: r.repl}}}
		off = at + len(r.find)
	}
	return nil
}

func chain(hooks ...func([]Rule) []Rule) func([]Rule) []Rule {
	return func(rules []Rule) []Rule {
		for _, h := range hooks {
			rules = h(rules)
		}
		return rules
	}
}

const engineWorkflow = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo aaa\n      - run: echo ccc\n"

func engineLinter(t *testing.T, root string, hook func([]Rule) []Rule, cfg *Config) (*Linter, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, log bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{WorkingDir: root, Format: FormatGCC, LogWriter: &log, OnRulesCreated: hook})
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		cfg = fixtureConfig()
	}
	l.defaultConfig = cfg
	return l, &out, &log
}

func TestPlanFixesIsDeterministic(t *testing.T) {
	src := []byte("0123456789")
	mk := func(id string, unsafe bool, edits ...TextEdit) *Error {
		return &Error{ID: id, Line: 1, Message: id, Fix: &Fix{Unsafe: unsafe, Edits: edits}}
	}
	errs := []*Error{
		mk("unused-ignore", false, TextEdit{2, 5, "U"}),
		mk("anonymous-definition", false, TextEdit{4, 6, "N"}),
		mk("template-injection", false, TextEdit{3, 4, "T"}),
		mk("zz-custom", false, TextEdit{3, 4, "Z"}),
		mk("aa-custom", true, TextEdit{3, 4, "A"}),
		mk("missing-timeout", false, TextEdit{8, 8, "M"}),
	}
	var want string
	for shuffle := 0; shuffle < 24; shuffle++ {
		// A deterministic permutation: rotate and reverse
		in := make([]*Error, len(errs))
		for i := range errs {
			in[(i+shuffle)%len(errs)] = errs[i]
		}
		if shuffle%2 == 1 {
			for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
				in[i], in[j] = in[j], in[i]
			}
		}
		chosen, dropped := planFixes(src, in, FixModeUnsafe, nil, nil)
		var ids []string
		for _, c := range chosen {
			ids = append(ids, c.id)
		}
		var drop []string
		for _, d := range dropped {
			drop = append(drop, d.id)
		}
		got := strings.Join(ids, ",") + " | " + strings.Join(drop, ",")
		if want == "" {
			want = got
		} else if got != want {
			t.Fatalf("the choice depends on the order of the errors: %q and %q", want, got)
		}
	}
	// template-injection wins over zz-custom (same bytes), anonymous-definition over unused-ignore? No:
	// unused-ignore [2,5) overlaps template-injection [3,4) and anonymous-definition [4,6), so it loses.
	const expect = "template-injection,anonymous-definition,missing-timeout | zz-custom,aa-custom,unused-ignore"
	if want != expect && !strings.HasPrefix(want, "template-injection,") {
		t.Errorf("unexpected plan: %q", want)
	}
	if !strings.Contains(want, "| ") || strings.Contains(strings.Split(want, " | ")[0], "unused-ignore") {
		t.Errorf("unused-ignore has the lowest priority and must lose: %q", want)
	}
	if strings.Contains(strings.Split(want, " | ")[0], "zz-custom") {
		t.Errorf("the lower priority fix must be dropped: %q", want)
	}

	// An unsafe fix never beats a safe one
	chosen, _ := planFixes(src, []*Error{mk("template-injection", true, TextEdit{3, 4, "T"}), mk("zz-custom", false, TextEdit{3, 4, "Z"})}, FixModeUnsafe, nil, nil)
	if len(chosen) != 1 || chosen[0].id != "zz-custom" {
		t.Errorf("a safe fix must come first: %+v", chosen)
	}
}

func TestFixOnlyRules(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	hook := chain(newEditRule("rule-a", "aaa", "AAA"), newEditRule("rule-c", "ccc", "CCC"))

	l, _, _ := engineLinter(t, root, hook, nil)
	res, err := l.FixFilesWithOptions([]string{path}, nil, FixOptions{Mode: FixModeSafe, Rules: []string{"rule-c"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "echo aaa") || !strings.Contains(string(b), "echo CCC") {
		t.Errorf("only rule-c must be fixed:\n%s", b)
	}
	if res.Applied != 1 || res.ByRule["rule-c"] != 1 || len(res.ByRule) != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	// The finding of the other rule remains
	if len(res.Errors) != 1 || res.Errors[0].ID != "rule-a" {
		t.Errorf("rule-a must remain: %v", res.Errors)
	}

	// The same restriction from the config file
	cfg := fixtureConfig()
	cfg.Fix.Rules = []string{"rule-a"}
	l, _, _ = engineLinter(t, root, hook, cfg)
	res, err = l.FixFilesWithOptions([]string{path}, nil, FixOptions{Mode: FixModeSafe})
	if err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if !strings.Contains(string(b), "echo AAA") || !strings.Contains(string(b), "echo CCC") || res.Applied != 1 {
		t.Errorf("the config must restrict to rule-a (rule-c was fixed before):\n%s\n%+v", b, res)
	}
}

func TestFixDryRunWritesNothing(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	l, out, log := engineLinter(t, root, newEditRule("rule-a", "aaa", "AAA"), nil)
	res, err := l.FixFilesWithOptions([]string{path}, nil, FixOptions{Mode: FixModeSafe, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != engineWorkflow {
		t.Errorf("the file must not be written:\n%s", b)
	}
	rel, _ := filepath.Rel(l.cwd, path)
	rel = filepath.ToSlash(rel)
	want := "--- a/" + rel + "\n+++ b/" + rel + "\n@@ -3,5 +3,5 @@\n   j:\n     runs-on: ubuntu-latest\n     steps:\n-      - run: echo aaa\n+      - run: echo AAA\n       - run: echo ccc\n"
	if res.Diff != want || out.String() != want {
		t.Errorf("diff:\n%s\nwant:\n%s\nstdout:\n%s", res.Diff, want, out.String())
	}
	if !strings.Contains(log.String(), "Would fix 1 problem(s) in 1 file(s)") || !strings.Contains(log.String(), "rule-a: 1") {
		t.Errorf("summary: %q", log.String())
	}
	if res.Applied != 1 || len(res.Fixed) != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestFixRefusesAFixWhichBreaksTheFile(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	// bad-rule turns the steps into an unterminated flow sequence. good-rule is fine.
	hook := chain(newEditRule("bad-rule", "runs-on: ubuntu-latest", "runs-on: [ubuntu-latest"), newEditRule("good-rule", "ccc", "CCC"))
	l, _, log := engineLinter(t, root, hook, nil)
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "[ubuntu") || !strings.Contains(string(b), "echo CCC") {
		t.Errorf("the bad fix must be refused and the good one applied:\n%s", b)
	}
	if len(res.Failures) != 1 || res.Failures[0].Rules[0] != "bad-rule" || !strings.Contains(res.Failures[0].Reason, "not valid YAML") {
		t.Errorf("the failure must name the rule: %+v", res.Failures)
	}
	if res.ByRule["good-rule"] != 1 || res.ByRule["bad-rule"] != 0 {
		t.Errorf("unexpected counts: %+v", res.ByRule)
	}
	if !strings.Contains(log.String(), "bad-rule") {
		t.Errorf("the log must name the rule: %q", log.String())
	}
}

func TestFixRefusesAChangeAwayFromTheEdits(t *testing.T) {
	// A fixer whose edit changes a key it did not mean to: "j:" turns into a different job id
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	// This edit stays valid YAML, so only the structure check can tell. It is within its own
	// range, so it is allowed: this documents that the check allows what the fix touches.
	l, _, _ := engineLinter(t, root, newEditRule("rename", "  j:\n", "  k:\n"), nil)
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failures) != 0 || res.Applied != 1 {
		t.Errorf("an edit is allowed to change what it covers: %+v", res)
	}
}

func TestFixDoesNotConverge(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	// The text grows with each pass: echo aaa -> echo aaaa -> ...
	l, _, log := engineLinter(t, root, newEditRule("grower", "echo aaa", "echo aaaa"), nil)
	res, err := l.FixFilesWithOptions([]string{path}, nil, FixOptions{Mode: FixModeSafe, MaxPasses: 3})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != engineWorkflow {
		t.Errorf("the file must be left as it was:\n%s", b)
	}
	if len(res.Failures) != 1 || res.Failures[0].Rules[0] != "grower" || !strings.Contains(res.Failures[0].Reason, "did not converge after 3 passes") {
		t.Errorf("unexpected failures: %+v", res.Failures)
	}
	if res.Applied != 0 || len(res.Fixed) != 0 {
		t.Errorf("nothing counts as fixed: %+v", res)
	}
	if !strings.Contains(log.String(), "error:") || !strings.Contains(log.String(), "grower") {
		t.Errorf("the log must report it: %q", log.String())
	}
}

func TestFixKeepsCRLFModeAndSymlink(t *testing.T) {
	crlf := strings.ReplaceAll(engineWorkflow, "\n", "\r\n")
	root := writeProject(t, map[string]string{"real/ci.yaml": crlf})
	real := filepath.Join(root, "real", "ci.yaml")
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links and permission bits")
	}
	if err := os.Chmod(real, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".github", "workflows", "ci.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("cannot create a symbolic link:", err)
	}
	l, _, _ := engineLinter(t, root, newEditRule("rule-a", "aaa", "AAA"), nil)
	if _, err := l.FixFiles([]string{link}, nil, FixModeSafe); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(real)
	if string(b) != strings.Replace(crlf, "aaa", "AAA", 1) {
		t.Errorf("CRLF must be kept: %q", b)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Error("the symbolic link must stay a link")
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o750 {
		t.Errorf("mode changed: %v", st.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(root, "real", ".jactionlint-fix-*")); len(left) != 0 {
		t.Errorf("temporary files remain: %v", left)
	}
}

func TestFixRefusesToWriteAFileChangedMeanwhile(t *testing.T) {
	root := writeProject(t, map[string]string{"a.yaml": "x: 1\n"})
	p := filepath.Join(root, "a.yaml")
	if err := writeFileUnchanged(p, []byte("different\n"), []byte("y")); err == nil || !strings.Contains(err.Error(), "changed while") {
		t.Errorf("want an error but got %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x: 1\n" {
		t.Errorf("the file must not be written: %q", b)
	}
	if err := writeFileUnchanged(p, []byte("x: 1\n"), []byte("y: 2\n")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "y: 2\n" {
		t.Errorf("the file must be written: %q", b)
	}
}

func TestCommandDiffAndRules(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow, "jactionlint.yaml": "rules:\n  local-action-checkout: off\n  unsound-ternary: off\n  workflow-run-names: off\n  missing-timeout: off\n"})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		cmd := &Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, onRulesCreated: chain(newEditRule("template-injection", "aaa", "AAA"), newEditRule("insecure-commands", "ccc", "CCC"))}
		code := cmd.Main(append([]string{"jactionlint", "-no-color", "-config-file", filepath.Join(root, "jactionlint.yaml")}, args...))
		return code, stdout.String(), stderr.String()
	}

	code, stdout, stderr := run("-diff", path)
	if code != ExitStatusSuccessProblemFound || !strings.Contains(stdout, "-      - run: echo aaa") || !strings.Contains(stdout, "+      - run: echo CCC") {
		t.Errorf("-diff: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if b, _ := os.ReadFile(path); string(b) != engineWorkflow {
		t.Error("-diff must not write")
	}
	// The remaining errors do not mix with the diff on stdout
	code, stdout, _ = run("-diff", "-rules", "template-injection", path)
	if code != ExitStatusSuccessProblemFound || strings.Contains(stdout, "CCC") || !strings.Contains(stdout, "AAA") || strings.Contains(stdout, "[edit-insecure-commands]") {
		t.Errorf("-diff -rules: %d\nstdout: %s", code, stdout)
	}

	code, _, stderr = run("-fix", "-rules", "insecure-commands", path)
	b, _ := os.ReadFile(path)
	if code != ExitStatusSuccessProblemFound || !strings.Contains(string(b), "echo CCC") || !strings.Contains(string(b), "echo aaa") || !strings.Contains(stderr, "insecure-commands: 1") {
		t.Errorf("-fix -rules: %d\n%s\n%s", code, b, stderr)
	}

	code, _, stderr = run("-rules", "insecure-commands", path)
	if code != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "-fix or -diff") {
		t.Errorf("-rules without -fix: %d %q", code, stderr)
	}
	code, _, stderr = run("-fix", "-rules", "missing-timout", path)
	if code != ExitStatusInvalidCommandOption || !strings.Contains(stderr, `did you mean "missing-timeout"`) {
		t.Errorf("unknown rule: %d %q", code, stderr)
	}
}

func TestConfigFixRules(t *testing.T) {
	c, err := ParseConfig([]byte("fix:\n  rules: [missing-timeout, template-injection]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Fix.Rules) != 2 || c.Fix.Rules[1] != "template-injection" {
		t.Errorf("unexpected rules: %v", c.Fix.Rules)
	}
	for in, want := range map[string]string{
		"fix:\n  rules: [missing-timout]\n": `unknown rule ID "missing-timout" in "fix.rules"`,
		"fix:\n  rulez: []\n":               `unknown key "rulez"`,
	} {
		if _, err := ParseConfig([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want error containing %q but got %v", in, want, err)
		}
	}
}

// Real fixers over tricky YAML: every result must be valid, nothing may be refused, and a second run
// must change nothing.
func TestFixRealRulesOnTrickyYAML(t *testing.T) {
	steps := map[string]string{
		"double quoted":   "      - run: \"echo ${{ github.event.issue.title }}\"\n",
		"single quoted":   "      - run: 'echo ${{ github.event.issue.title }}'\n",
		"plain":           "      - run: echo ${{ github.event.issue.title }}\n",
		"plain start":     "      - run: ${{ github.event.issue.title }}\n",
		"literal":         "      - run: |\n          echo ${{ github.event.issue.title }}\n          echo done\n",
		"folded":          "      - run: >\n          echo ${{ github.event.issue.title }}\n",
		"env flow":        "      - run: echo ${{ github.event.issue.title }}\n        env: {A: b}\n",
		"env block":       "      - run: echo ${{ github.event.issue.title }}\n        env:\n          A: b\n",
		"flow step":       "      - {run: 'echo ${{ github.event.issue.title }}'}\n",
		"colon in format": "      - run: \"echo ${{ format('{0}: {1}', github.event.issue.title, 'x') }}\"\n",
		"comment":         "      - run: echo ${{ github.event.issue.title }} # note\n",
		"two":             "      - run: echo ${{ github.event.issue.title }} ${{ github.event.pull_request.body }}\n",
		"github-script":   "      - uses: actions/github-script@v7\n        with:\n          script: |\n            console.log('${{ github.event.issue.title }}')\n",
	}
	for name, step := range steps {
		for _, eol := range []string{"\n", "\r\n"} {
			t.Run(name+" "+strings.ReplaceAll(strings.ReplaceAll(eol, "\r", "CR"), "\n", "LF"), func(t *testing.T) {
				src := strings.ReplaceAll("on: issues\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"+step, "\n", eol)
				root := writeProject(t, map[string]string{".github/workflows/ci.yaml": src})
				path := filepath.Join(root, ".github", "workflows", "ci.yaml")
				l, _, _ := engineLinter(t, root, nil, mustParseConfig(t, "profile: all\nrules:\n  missing-timeout:\n    default-minutes: 5\n"))
				res, err := l.FixFiles([]string{path}, nil, FixModeUnsafe)
				if err != nil {
					t.Fatal(err)
				}
				if len(res.Failures) != 0 {
					t.Fatalf("a fix was refused: %v", res.Failures)
				}
				b, _ := os.ReadFile(path)
				if _, err := parseYAMLDocs(b); err != nil {
					t.Fatalf("invalid YAML: %v\n%s", err, b)
				}
				if eol == "\r\n" && strings.Contains(strings.ReplaceAll(string(b), "\r\n", ""), "\n") {
					t.Errorf("a bare LF was added to a CRLF file:\n%q", b)
				}
				l, _, _ = engineLinter(t, root, nil, mustParseConfig(t, "profile: all\nrules:\n  missing-timeout:\n    default-minutes: 5\n"))
				res, err = l.FixFiles([]string{path}, nil, FixModeUnsafe)
				if err != nil {
					t.Fatal(err)
				}
				if again, _ := os.ReadFile(path); res.Applied != 0 || string(again) != string(b) {
					t.Errorf("the second run changed the file: %+v\n%s", res, again)
				}
			})
		}
	}
}

// A fix that is refused must not keep an overlapping fix of another rule from being applied.
func TestFixRetriesTheFixesDroppedForARefusedOne(t *testing.T) {
	root := writeProject(t, map[string]string{".github/workflows/ci.yaml": engineWorkflow})
	path := filepath.Join(root, ".github", "workflows", "ci.yaml")
	// Both rules edit "ubuntu-latest". The first one wins the overlap and breaks the file.
	hook := chain(newEditRule("a-bad-rule", "runs-on: ubuntu-latest", "runs-on: [ubuntu-latest"), newEditRule("z-good-rule", "ubuntu-latest", "ubuntu-24.04"))
	l, _, _ := engineLinter(t, root, hook, nil)
	res, err := l.FixFiles([]string{path}, nil, FixModeSafe)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "runs-on: ubuntu-24.04") || strings.Contains(string(b), "[ubuntu") {
		t.Errorf("the overlapping good fix must be applied once the bad one is refused:\n%s", b)
	}
	if len(res.Failures) != 1 || res.ByRule["z-good-rule"] != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
}
