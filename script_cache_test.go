package jactionlint

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// TestStringSharesAnalyzedScript checks that the rules which ask a string for its shell analysis get the one
// analysis, whatever bash or sh is named, and that it is dropped when the value changes.
func TestStringSharesAnalyzedScript(t *testing.T) {
	s := &String{Value: "echo ${{ github.sha }} | tee out"}
	first, err := s.analyzeScript("bash")
	if err != nil || first == nil {
		t.Fatalf("analyzeScript(bash) = %v, %v", first, err)
	}
	for _, shell := range []string{"", "bash", "sh", "bash -e {0}", "/usr/bin/bash", `C:\git\bash.exe`} {
		again, err := s.analyzeScript(shell)
		if err != nil || again != first {
			t.Errorf("analyzeScript(%q) did not share the analysis: %v, %v", shell, again == first, err)
		}
	}
	for _, shell := range []string{"pwsh", "python", "cmd", "powershell {0}"} {
		if got, err := s.analyzeScript(shell); got != nil || !errors.Is(err, runscript.ErrUnsupportedShell) {
			t.Errorf("analyzeScript(%q) = %v, %v; want the error for a shell which is not analyzed", shell, got, err)
		}
	}
	if again, _ := s.analyzeScript("bash"); again != first {
		t.Error("a shell which is not analyzed dropped the analysis")
	}

	fresh, err := runscript.Analyze(s.Value, "bash")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh, first) {
		t.Error("the shared analysis differs from a fresh one")
	}

	s.Value = "echo changed"
	changed, err := s.analyzeScript("bash")
	if err != nil || changed == first || changed.Source != "echo changed" {
		t.Errorf("the analysis of the old value was kept: %v, %v", changed, err)
	}
}

// TestStringSharesScriptParseError checks that a script which does not parse is not parsed again either.
func TestStringSharesScriptParseError(t *testing.T) {
	s := &String{Value: "if then fi ("}
	script, err := s.analyzeScript("bash")
	if script != nil || err == nil {
		t.Fatalf("analyzeScript = %v, %v; want a parse error", script, err)
	}
	var perr *runscript.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("the error is %T, not a *runscript.ParseError", err)
	}
	if _, again := s.analyzeScript("sh"); again != err {
		t.Error("the parse error was not kept")
	}
}

// TestReadSiblings checks the one read of the workflows of a project which gives the calls and the facts about the events.
func TestReadSiblings(t *testing.T) {
	const callee = "on:\n  workflow_call:\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	caller := func(on string) string {
		return "name: Caller " + on + "\non: " + on + "\njobs:\n  call:\n    uses: ./.github/workflows/callee.yml\n  act:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n      - uses: ./.github/actions/x\n"
	}
	files := map[string]string{
		".github/workflows/callee.yml": callee,
		".github/workflows/a-push.yml": caller("push"),
		".github/workflows/b-pr.yml":   caller("pull_request"),
		".github/workflows/c-prt.yaml": caller("pull_request_target"),
		".github/workflows/notes.txt":  "not a workflow",
	}
	wantRefs := map[string][]callRef{
		"w:.github/workflows/callee.yml": {
			{file: ".github/workflows/a-push.yml", job: "call", step: -1},
			{file: ".github/workflows/b-pr.yml", job: "call", step: -1},
			{file: ".github/workflows/c-prt.yaml", job: "call", step: -1},
		},
		"a:.github/actions/x": {
			{file: ".github/workflows/a-push.yml", job: "act", step: 1},
			{file: ".github/workflows/b-pr.yml", job: "act", step: 1},
			{file: ".github/workflows/c-prt.yaml", job: "act", step: 1},
		},
	}

	root := writeProject(t, files)
	project, err := NewProject(root)
	if err != nil {
		t.Fatal(err)
	}
	// a nil value reads on every call, a value reads once: both give the same
	for name, sib := range map[string]*siblingWorkflows{"shared": {}, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			sc := sib.scanned(project)
			if !reflect.DeepEqual(sc.refs, wantRefs) {
				t.Errorf("refs = %v, want %v", sc.refs, wantRefs)
			}
			if got := sib.callRefs(project, "w:.github/workflows/callee.yml"); !reflect.DeepEqual(got, wantRefs["w:.github/workflows/callee.yml"]) {
				t.Errorf("callRefs = %v", got)
			}
			byName, ok := sib.parsed(project)
			if !ok {
				t.Fatal("the facts could not be determined")
			}
			want := map[string]upstreamFacts{
				"caller push":                upstreamTrusted,
				"caller pull_request":        upstreamPROnly,
				"caller pull_request_target": upstreamPROnly,
				// a workflow with only workflow_call is neither trusted nor pull request only
				".github/workflows/callee.yml": 0,
			}
			if !reflect.DeepEqual(byName, want) {
				t.Errorf("facts = %v, want %v", byName, want)
			}
		})
	}
	shared := &siblingWorkflows{}
	if a, b := shared.scanned(project), shared.scanned(project); a != b {
		t.Error("a shared value read the workflows twice")
	}
}

// TestReadSiblingsBrokenWorkflow checks that a workflow which cannot be parsed takes the facts away, as before, and keeps
// the calls of the others.
func TestReadSiblingsBrokenWorkflow(t *testing.T) {
	root := writeProject(t, map[string]string{
		".github/workflows/a.yml":      "on: push\njobs:\n  call:\n    uses: ./.github/workflows/b.yml\n",
		".github/workflows/b.yml":      "on: workflow_call\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
		".github/workflows/broken.yml": "a: [\n",
		".github/workflows/z.yml":      "on: push\njobs:\n  call:\n    uses: ./.github/workflows/b.yml\n",
	})
	project, err := NewProject(root)
	if err != nil {
		t.Fatal(err)
	}
	sib := &siblingWorkflows{}
	byName, ok := sib.parsed(project)
	if ok || byName != nil {
		t.Errorf("parsed = %v, %v; want the facts to be unknown", byName, ok)
	}
	refs := sib.callRefs(project, "w:.github/workflows/b.yml")
	if len(refs) != 2 || refs[0].file != ".github/workflows/a.yml" || refs[1].file != ".github/workflows/z.yml" {
		t.Errorf("callRefs = %v; the workflows after the broken one must still count", refs)
	}
}

// TestReadSiblingsExpressionInName checks that a name with an expression takes the facts away, and not the calls.
func TestReadSiblingsExpressionInName(t *testing.T) {
	root := writeProject(t, map[string]string{
		".github/workflows/a.yml": "name: ${{ github.sha }}\non: push\njobs:\n  call:\n    uses: ./.github/workflows/b.yml\n",
		".github/workflows/b.yml": "on: workflow_call\njobs:\n  x:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n",
		".github/workflows/c.yml": "on: push\njobs:\n  call:\n    uses: ./.github/workflows/b.yml\n",
	})
	project, err := NewProject(root)
	if err != nil {
		t.Fatal(err)
	}
	sib := &siblingWorkflows{}
	if byName, ok := sib.parsed(project); ok || byName != nil {
		t.Errorf("parsed = %v, %v", byName, ok)
	}
	if refs := sib.callRefs(project, "w:.github/workflows/b.yml"); len(refs) != 2 {
		t.Errorf("callRefs = %v", refs)
	}
}

// TestUntrustedCheckoutCallsOfOneWorkflow checks the callers which call one action several times: the calls of a workflow
// are judged one by one, from the one parse of the workflow.
func TestUntrustedCheckoutCallsOfOneWorkflow(t *testing.T) {
	step := func(ref string) string {
		return "      - uses: ./.github/actions/build\n        with:\n          ref: " + ref + "\n"
	}
	pr := "on: pull_request_target\njobs:\n" +
		"  a:\n    runs-on: ubuntu-latest\n    steps:\n" + step("main") + step("v1") +
		"  b:\n    runs-on: ubuntu-latest\n    steps:\n" + step("main") + step(headSHA) + step("main")
	other := "on: pull_request_target\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n" + step("main") + step("release")
	push := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n" + step(headSHA)
	root := writeProject(t, map[string]string{
		".github/actions/build/action.yml": ucAction,
		".github/workflows/a-pr.yml":       pr,
		".github/workflows/b-other.yml":    other,
		".github/workflows/c-push.yml":     push,
	})
	l := newActionLinter(t, root, io.Discard, LinterOptions{}, "rules:\n  untrusted-checkout: error\n")
	errs, err := l.LintFiles([]string{filepath.Join(root, ".github", "actions", "build", "action.yml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	errs = errsWithID(errs, "untrusted-checkout")
	if len(errs) != 1 {
		t.Fatalf("want 1 finding but got %v", errs)
	}
	if m := errs[0].Message; !strings.Contains(m, "given github.event.pull_request.head.sha by .github/workflows/a-pr.yml") || strings.Contains(m, "b-other.yml") || strings.Contains(m, "c-push.yml") {
		t.Errorf("the message does not name the call of a-pr.yml alone: %s", m)
	}
}

func benchmarkScriptSource(n int) string {
	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "if [ -n \"$VAR%d\" ]; then\n  curl -fsSL https://example.com/%d | tar xz -C \"$DIR\"\n  echo \"built %d\" >> \"$GITHUB_OUTPUT\"\nfi\n", i, i, i)
	}
	return b.String()
}

// BenchmarkAnalyzeScript measures the shell analysis of a run script that every rule asks for: Fresh is what each rule used
// to pay, Shared is what the rules after the first pay.
func BenchmarkAnalyzeScript(b *testing.B) {
	src := benchmarkScriptSource(40)
	b.Run("Fresh", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := runscript.Analyze(src, "bash"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Shared", func(b *testing.B) {
		b.ReportAllocs()
		s := &String{Value: src}
		for i := 0; i < b.N; i++ {
			if _, err := s.analyzeScript("bash"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkExprScan measures the scan of the expressions of a string that several rules ask for: Fresh scans the string
// as the rule which comes first does, Shared is what the others pay.
func BenchmarkExprScan(b *testing.B) {
	value := "echo ${{ github.event.pull_request.title }} ${{ matrix.os == 'linux' && format('{0}-{1}', github.sha, inputs.name) || 'x' }} ${{ secrets.TOKEN }}"
	b.Run("Fresh", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s := &String{Value: value}
			if c := s.placeholders(); c.failed || len(c.list) != 3 {
				b.Fatalf("%d placeholders, failed %v", len(c.list), c.failed)
			}
		}
	})
	b.Run("Shared", func(b *testing.B) {
		b.ReportAllocs()
		s := &String{Value: value}
		for i := 0; i < b.N; i++ {
			if c := s.placeholders(); c.failed || len(c.list) != 3 {
				b.Fatalf("%d placeholders, failed %v", len(c.list), c.failed)
			}
		}
	})
}

func perfWorkflowRunScripts(n int) string {
	b := perfHeader()
	script := benchmarkScriptSource(8)
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "      - name: s%d\n        run: |\n", i)
		for _, l := range strings.Split(strings.TrimSuffix(script, "\n"), "\n") {
			fmt.Fprintf(b, "          %s\n", l)
		}
		b.WriteString("          echo ${{ github.sha }}\n")
	}
	return b.String()
}

// BenchmarkLintRunScripts lints steps with long run scripts, which every rule that reads shell parses. Run it with
// -benchmem for the allocation.
func BenchmarkLintRunScripts(b *testing.B) {
	benchmarkLint(b, ProfilePedantic, perfWorkflowRunScripts, 100, 1000)
}
