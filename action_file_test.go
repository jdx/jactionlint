package jactionlint

import (
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// copyActionFixture copies the fixture repository testdata/actions/<name> to a temporary directory
// and returns it. The directory "github" becomes ".github" (so that hk, which lints every real
// ".github" directory, does not see the fixtures as workflows), and config.yaml is not copied.
func copyActionFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", "actions", name)
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		if rel == "config.yaml" {
			return nil
		}
		if rel == "github" || strings.HasPrefix(rel, "github"+string(filepath.Separator)) {
			rel = "." + rel
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func lintActionFixture(t *testing.T, name string) (string, []*Error) {
	t.Helper()
	root := copyActionFixture(t, name)

	cfg := &Config{}
	if cf := filepath.Join("testdata", "actions", name, "config.yaml"); fileExists(cf) {
		c, err := ReadConfigFile(cf)
		if err != nil {
			t.Fatal(err)
		}
		cfg = c
	}
	l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withFixtureRules(cfg)

	proj := &Project{root: root}
	files, err := walkWorkflowFiles(proj.WorkflowsDir())
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, l.callGraphOf(proj).actionPaths()...)
	errs, err := l.LintFiles(files, proj)
	if err != nil {
		t.Fatal(err)
	}
	return root, errs
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

const (
	// compositeFixturePrefix is the prefix of the fixtures which are the metadata of an action. The linter
	// chooses the rules by the file name so they are linted as if they were this file.
	compositeFixturePrefix = "composite_"
	compositeFixturePath   = ".github/actions/example/action.yml"
)

func TestCompositeActionFixtures(t *testing.T) {
	for _, subdir := range []string{"ok", "err", "examples"} {
		files, err := filepath.Glob(filepath.Join("testdata", subdir, compositeFixturePrefix+"*.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no composite action fixture in testdata/%s", subdir)
		}
		for _, f := range files {
			base := strings.TrimSuffix(f, ".yaml")
			t.Run(subdir+"/"+filepath.Base(base), func(t *testing.T) {
				src, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				l, err := NewLinter(io.Discard, &LinterOptions{})
				if err != nil {
					t.Fatal(err)
				}
				l.defaultConfig = fixtureConfig()
				errs, err := l.Lint(compositeFixturePath, src, &Project{root: filepath.Dir(f)})
				if err != nil {
					t.Fatal(err)
				}
				if subdir == "ok" {
					if len(errs) > 0 {
						t.Fatal(errs)
					}
					return
				}
				if *updateActionGoldens {
					writeErrorsGolden(t, base+".out", errs)
				}
				checkErrors(t, base+".out", errs)
			})
		}
	}
}

// writeErrorsGolden writes the errors in the format of the expected files of testdata.
func writeErrorsGolden(t *testing.T, file string, errs []*Error) {
	t.Helper()
	slices.SortFunc(errs, compareErrors)
	var b strings.Builder
	for _, e := range errs {
		e.Filepath = filepath.ToSlash(e.Filepath)
		b.WriteString(e.Error() + "\n")
	}
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// updateActionGoldens rewrites the expected outputs in testdata/actions.
var updateActionGoldens = flag.Bool("update-action-goldens", false, "rewrite the expected files of testdata/actions")

func TestLintActionFixtures(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "actions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			_, errs := lintActionFixture(t, name)
			if *updateActionGoldens {
				slices.SortFunc(errs, compareErrors)
				var b strings.Builder
				for _, e := range errs {
					e.Filepath = filepath.ToSlash(e.Filepath)
					b.WriteString(e.Error() + "\n")
				}
				if err := os.WriteFile(filepath.Join("testdata", "actions", name+".out"), []byte(b.String()), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			checkErrors(t, filepath.Join("testdata", "actions", name+".out"), errs)
		})
	}
}

func TestParseActionComposite(t *testing.T) {
	src := `name: x
description: y
inputs:
  Who:
    description: who
    required: true
    default: world
outputs:
  out:
    description: o
    value: ${{ steps.a.outputs.v }}
runs:
  using: composite
  steps:
    - id: a
      run: echo hi
      shell: bash
    - uses: actions/checkout@v4
branding:
  icon: x
`
	w, errs := ParseAction([]byte(src))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if w.Action == nil || !w.IsComposite() {
		t.Fatalf("not a composite action: %#v", w.Action)
	}
	if len(w.On) != 0 || w.Permissions != nil {
		t.Errorf("an action has no workflow-level configuration: %#v", w)
	}
	job := w.Jobs[compositeJobID]
	if job == nil || !job.Composite || len(job.Steps) != 2 {
		t.Fatalf("steps are not in the synthetic job: %#v", w.Jobs)
	}
	if job.RunsOn != nil || job.Permissions != nil || job.TimeoutMinutes != nil || len(job.Needs) != 0 {
		t.Errorf("the synthetic job has job settings: %#v", job)
	}
	a := w.Action
	if len(a.Inputs) != 1 || a.Inputs[0].ID.Value != "Who" || a.Inputs[0].Required == nil || a.Inputs[0].Default.Value != "world" {
		t.Errorf("inputs: %#v", a.Inputs)
	}
	if len(a.Outputs) != 1 || a.Outputs[0].Value.Value != "${{ steps.a.outputs.v }}" {
		t.Errorf("outputs: %#v", a.Outputs)
	}
	if w.Comments == nil || w.Source == nil {
		t.Error("comments and source must be set")
	}
}

func TestParseActionKinds(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		composite bool
		errs      []string // substrings of the expected messages
	}{
		{
			name: "node",
			src:  "name: x\nruns:\n  using: node24\n  main: index.js\n",
		},
		{
			name: "node future runtime",
			src:  "name: x\nruns:\n  using: node30\n  main: index.js\n",
		},
		{
			name: "docker",
			src:  "name: x\nruns:\n  using: docker\n  image: docker://alpine:3\n",
		},
		{
			name:      "composite",
			src:       "name: x\nruns:\n  using: composite\n  steps:\n    - run: echo\n      shell: sh\n",
			composite: true,
		},
		{
			name: "missing runs",
			src:  "name: x\n",
			errs: []string{`"runs" section is missing`},
		},
		{
			name: "missing using",
			src:  "name: x\nruns:\n  main: index.js\n",
			errs: []string{`"using" is missing`},
		},
		{
			name: "invalid using",
			src:  "name: x\nruns:\n  using: node\n  main: index.js\n",
			errs: []string{`"using" "node" is invalid`},
		},
		{
			name: "node without main",
			src:  "name: x\nruns:\n  using: node20\n",
			errs: []string{`"main" is missing`},
		},
		{
			name: "docker without image",
			src:  "name: x\nruns:\n  using: docker\n",
			errs: []string{`"image" is missing`},
		},
		{
			name:      "composite without steps",
			src:       "name: x\nruns:\n  using: composite\n",
			composite: true,
			errs:      []string{`"steps" is missing`},
		},
		{
			name:      "composite run without shell",
			src:       "name: x\nruns:\n  using: composite\n  steps:\n    - run: echo\n",
			composite: true,
			errs:      []string{`"shell" is required for a "run" step of a composite action`},
		},
		{
			name:      "composite shell in parallel steps",
			src:       "name: x\nruns:\n  using: composite\n  steps:\n    - parallel:\n        - run: echo\n",
			composite: true,
			errs:      []string{`"shell" is required`},
		},
		{
			name: "steps in a node action",
			src:  "name: x\nruns:\n  using: node20\n  main: a.js\n  steps:\n    - run: echo\n      shell: sh\n",
			errs: []string{`"steps" is only available in the composite action`},
		},
		{
			name:      "main in a composite action",
			src:       "name: x\nruns:\n  using: composite\n  main: a.js\n  steps:\n    - run: echo\n      shell: sh\n",
			composite: true,
			errs:      []string{`"main" is not available in "runs" section of the composite action`},
		},
		{
			name: "unknown key",
			src:  "name: x\nfoo: bar\nruns:\n  using: node20\n  main: a.js\n",
			errs: []string{`unexpected key "foo"`},
		},
		{
			name: "unknown input key",
			src:  "name: x\ninputs:\n  a:\n    descriptions: a\nruns:\n  using: node20\n  main: a.js\n",
			errs: []string{`unexpected key "descriptions"`},
		},
		{
			name: "empty",
			src:  "",
			errs: []string{"action metadata is empty"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, errs := ParseAction([]byte(tc.src))
			if w == nil {
				t.Fatal("no workflow")
			}
			if w.IsComposite() != tc.composite {
				t.Errorf("IsComposite() = %v, want %v", w.IsComposite(), tc.composite)
			}
			if !tc.composite && len(w.Jobs) != 0 {
				t.Errorf("an action which is not composite has no job: %v", w.Jobs)
			}
			for _, e := range errs {
				if e.ID != actionSyntaxID {
					t.Errorf("ID of %q is %q, want %q", e.Message, e.ID, actionSyntaxID)
				}
			}
			if len(errs) != len(tc.errs) {
				t.Fatalf("%d errors are expected but got %d: %v", len(tc.errs), len(errs), errs)
			}
			for i, want := range tc.errs {
				if !strings.Contains(errs[i].Message, want) {
					t.Errorf("error %d is %q, want it to contain %q", i, errs[i].Message, want)
				}
			}
		})
	}
}

func TestParseActionInvalidYAML(t *testing.T) {
	w, errs := ParseAction([]byte("name: [\n"))
	if w != nil || len(errs) == 0 || errs[0].ID != "yaml-syntax" {
		t.Fatalf("want a yaml-syntax error: %v %v", w, errs)
	}
}

func TestIsActionPath(t *testing.T) {
	tests := map[string]bool{
		"action.yml":                           true,
		"action.yaml":                          true,
		"./action.yml":                         true,
		".github/actions/setup/action.yml":     true,
		`.github\actions\setup\action.yaml`:    true,
		"/repo/some/dir/action.yml":            true,
		".github/workflows/action.yml":         false,
		"/repo/.github/workflows/action.yaml":  false,
		".github/workflows/sub/action.yml":     false,
		"action.yml.bak":                       false,
		"my-action.yml":                        false,
		".github/workflows/ci.yml":             false,
		"workflows/action.yml":                 true,
		".github/actions/workflows/action.yml": true,
	}
	for p, want := range tests {
		if have := IsActionPath(p); have != want {
			t.Errorf("IsActionPath(%q) = %v, want %v", p, have, want)
		}
	}
}

func TestActionScopeIsComplete(t *testing.T) {
	factories := map[string]bool{}
	for _, f := range ruleFactories {
		factories[f.name] = true
		if _, ok := actionRuleScope[f.name]; !ok {
			t.Errorf("rule %q has no entry in actionRuleScope: decide whether it applies to action.yml files, see rule_action_scope.go", f.name)
		}
	}
	for name, s := range actionRuleScope {
		if !factories[name] {
			t.Errorf("actionRuleScope has an entry for %q which is not a rule", name)
		}
		for _, id := range s.except {
			if _, ok := ruleIndex[id]; !ok {
				t.Errorf("actionRuleScope[%q] excludes %q which is not a rule ID", name, id)
			}
		}
	}
}

func TestActionScopeIsDocumented(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	i := strings.Index(doc, "<a id=\"check-composite-actions\"></a>")
	if i < 0 {
		t.Fatal("docs/checks.md has no section for composite actions")
	}
	sec := doc[i:]
	if j := strings.Index(sec[1:], "<a id=\""); j >= 0 {
		sec = sec[:j+1]
	}
	for _, r := range Rules() {
		if !strings.Contains(sec, "`"+r.ID+"`") {
			t.Errorf("the table of composite actions in docs/checks.md does not mention the rule %q", r.ID)
		}
	}
}
