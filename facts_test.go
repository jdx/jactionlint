package jactionlint

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var updateFactsGoldens = flag.Bool("update-facts-goldens", false, "rewrite testdata/facts/expected.json")

func factsRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("testdata", "facts", "repo"))
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func exportTestFacts(t *testing.T) *Facts {
	t.Helper()
	repo := factsRepo(t)
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, pat := range []string{".github/workflows/*.yaml", ".github/actions/*/action.yml"} {
		m, _ := filepath.Glob(filepath.Join(repo, filepath.FromSlash(pat)))
		files = append(files, m...)
	}
	f, err := l.ExportFacts(files, &Project{root: repo})
	if err != nil {
		t.Fatal(err)
	}
	f.ToolVersion = "test"
	return f
}

// TestExportFactsGolden pins the document. Regenerate with go test -run TestExportFactsGolden -update-facts-goldens .
// and review the diff: a fact that changes meaning is a new version of the document.
func TestExportFactsGolden(t *testing.T) {
	b, err := exportTestFacts(t).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "facts", "expected.json")
	if *updateFactsGoldens {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(strings.ReplaceAll(string(want), "\r\n", "\n"), string(b)); diff != "" {
		t.Errorf("facts (-want +got): %s", diff)
	}
}

func workflowFactsOf(t *testing.T, f *Facts, path string) *WorkflowFacts {
	t.Helper()
	for _, w := range f.Workflows {
		if w.Path == path {
			return w
		}
	}
	t.Fatalf("no workflow %s in the facts", path)
	return nil
}

// The unknowns are said, not left out: an expression is dynamic, a token that nobody declares is the setting of the
// repository, a workflow nothing calls has callers that are not known.
func TestExportFactsUnknownsAreExplicit(t *testing.T) {
	f := exportTestFacts(t)
	ci := workflowFactsOf(t, f, ".github/workflows/ci.yaml")
	if c := ci.Concurrency; c == nil || c.Form != "mapping" || c.Group == nil || !c.Group.Dynamic || c.CancelInProgress != "unknown" {
		t.Errorf("concurrency of ci: %+v", c)
	}
	test, release := ci.Jobs[0], ci.Jobs[1]
	if !test.DynamicRunsOn || test.TimeoutMinutes == nil || !test.TimeoutMinutes.Dynamic {
		t.Errorf("job test: runs-on dynamic %v, timeout %+v", test.DynamicRunsOn, test.TimeoutMinutes)
	}
	if test.EffectivePermissions.Source != "workflow" || test.EffectivePermissions.Scopes["contents"] != "read" {
		t.Errorf("job test inherits the permissions of the workflow: %+v", test.EffectivePermissions)
	}
	if release.EffectivePermissions.Source != "job" || release.EffectivePermissions.Scopes["contents"] != "write" || !release.Uses.SecretsInherit {
		t.Errorf("job release: %+v %+v", release.EffectivePermissions, release.Uses)
	}

	rel := workflowFactsOf(t, f, ".github/workflows/release.yaml")
	if rel.Permissions.Declared || rel.Jobs[0].EffectivePermissions.Source != "repository-setting" || len(rel.Jobs[0].EffectivePermissions.Scopes) != 0 {
		t.Errorf("a token nobody declares is the repository setting, not none: %+v", rel.Jobs[0].EffectivePermissions)
	}
	if rel.CalledBy == nil || !rel.CalledBy.Known || strings.Join(rel.CalledBy.Events, ",") != "pull_request,push" || strings.Join(rel.CalledBy.Callers, ",") != ".github/workflows/ci.yaml" {
		t.Errorf("release called by: %+v", rel.CalledBy)
	}
	if rel.Jobs[0].Steps[0].Uses.RefKind != "full-sha" {
		t.Errorf("a pin: %+v", rel.Jobs[0].Steps[0].Uses)
	}

	orphan := workflowFactsOf(t, f, ".github/workflows/orphan.yaml")
	if orphan.CalledBy == nil || orphan.CalledBy.Known || len(orphan.CalledBy.Callers) != 0 {
		t.Errorf("a workflow nothing calls has callers that are not known: %+v", orphan.CalledBy)
	}

	if b := workflowFactsOf(t, f, ".github/workflows/broken.yaml"); b.Parsed || len(b.Jobs) != 0 {
		t.Errorf("a file that is not YAML has no facts: %+v", b)
	}

	if len(f.Actions) != 1 || f.Actions[0].CalledBy == nil || !f.Actions[0].CalledBy.Known {
		t.Fatalf("actions: %+v", f.Actions)
	}
	var caller *ActionCallerFacts
	for _, c := range f.Actions[0].CalledBy.Callers {
		caller = c
	}
	if caller == nil || strings.Join(caller.Events, ",") != "pull_request,push" {
		t.Fatalf("callers of the action: %+v", f.Actions[0].CalledBy.Callers)
	}
	got := map[string]bool{}
	for _, in := range caller.Inputs {
		got[in.Name] = in.Dynamic
	}
	if d, ok := got["cache"]; !ok || !d {
		t.Errorf("the cache input the workflow passes is an expression: %+v", caller.Inputs)
	}
	if d, ok := got["version"]; !ok || d {
		t.Errorf("the version input is a literal: %+v", caller.Inputs)
	}

	var edges []string
	for _, c := range f.Calls {
		edges = append(edges, c.Kind+" "+c.From+" "+c.Job+" -> "+c.To+" "+c.Permissions.Source)
	}
	want := []string{
		"reusable-workflow .github/workflows/ci.yaml release -> .github/workflows/release.yaml job",
		"action .github/workflows/ci.yaml test -> .github/actions/setup workflow",
	}
	if diff := cmp.Diff(want, edges); diff != "" {
		t.Errorf("calls (-want +got): %s", diff)
	}
}

// The facts do not depend on the configuration: a rule that is off still has its facts.
func TestExportFactsIgnoresTheConfiguration(t *testing.T) {
	repo := factsRepo(t)
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: repo, Config: mustParseConfig(t, "profile: correctness\n")})
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.ExportFacts([]string{filepath.Join(repo, ".github", "workflows", "ci.yaml")}, &Project{root: repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Workflows) != 1 || len(f.Workflows[0].Jobs) != 2 {
		t.Errorf("%+v", f.Workflows)
	}
}

// The schema lists exactly the fields of the types, and requires those which are not omitted when empty.
func TestFactsSchemaMatchesTypes(t *testing.T) {
	var schema struct {
		Props       map[string]json.RawMessage `json:"properties"`
		Required    []string                   `json:"required"`
		Definitions map[string]struct {
			Props    map[string]json.RawMessage `json:"properties"`
			Required []string                   `json:"required"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(FactsSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	check := func(name string, typ reflect.Type, props map[string]json.RawMessage, required []string) {
		t.Helper()
		fields := map[string]bool{} // name -> always written
		var walk func(typ reflect.Type)
		walk = func(typ reflect.Type) {
			for i := 0; i < typ.NumField(); i++ {
				sf := typ.Field(i)
				tag := sf.Tag.Get("json")
				if sf.Anonymous && tag == "" {
					walk(sf.Type)
					continue
				}
				n, opts, _ := strings.Cut(tag, ",")
				fields[n] = !strings.Contains(opts, "omitempty")
			}
		}
		walk(typ)
		for n := range fields {
			if _, ok := props[n]; !ok {
				t.Errorf("%s: the schema has no property %q", name, n)
			}
		}
		for n := range props {
			if _, ok := fields[n]; !ok {
				t.Errorf("%s: the schema has %q, which the type does not", name, n)
			}
		}
		req := map[string]bool{}
		for _, r := range required {
			req[r] = true
		}
		for n, always := range fields {
			if always && !req[n] {
				t.Errorf("%s: %q is always written but not required", name, n)
			}
			if !always && req[n] {
				t.Errorf("%s: %q is required but omitted when empty", name, n)
			}
		}
	}
	check("facts", reflect.TypeOf(Facts{}), schema.Props, schema.Required)
	defs := map[string]reflect.Type{
		"location": reflect.TypeOf(Location{}), "stringFact": reflect.TypeOf(StringFact{}), "numberFact": reflect.TypeOf(NumberFact{}),
		"inputFact": reflect.TypeOf(InputFact{}), "event": reflect.TypeOf(EventFacts{}), "permissions": reflect.TypeOf(PermissionFacts{}),
		"effectivePermissions": reflect.TypeOf(EffectivePermissions{}), "concurrency": reflect.TypeOf(ConcurrencyFacts{}),
		"uses": reflect.TypeOf(UsesFacts{}), "step": reflect.TypeOf(StepFacts{}), "job": reflect.TypeOf(JobFacts{}),
		"calledBy": reflect.TypeOf(CalledByFacts{}), "workflow": reflect.TypeOf(WorkflowFacts{}), "actionCaller": reflect.TypeOf(ActionCallerFacts{}),
		"call": reflect.TypeOf(CallFacts{}), "action": reflect.TypeOf(ActionFacts{}),
	}
	for name, typ := range defs {
		d, ok := schema.Definitions[name]
		if !ok {
			t.Errorf("no definition %q", name)
			continue
		}
		check(name, typ, d.Props, d.Required)
	}
}

// copyFactsRepo copies the repository of the fixture to a directory with a .git, so that it is a project of its own.
func copyFactsRepo(t *testing.T) string {
	t.Helper()
	src := factsRepo(t)
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dst, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestExportFactsCommand(t *testing.T) {
	repo := copyFactsRepo(t)
	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		status := cmd.Main(append([]string{"jactionlint"}, args...))
		return status, out.String(), errOut.String()
	}

	status, out, stderr := run("--export-facts", filepath.Join(repo, ".github", "workflows", "release.yaml"))
	if status != 0 || stderr != "" {
		t.Fatalf("status %d: %s", status, stderr)
	}
	var f Facts
	if err := json.Unmarshal([]byte(out), &f); err != nil {
		t.Fatalf("the output is not JSON: %v\n%s", err, out)
	}
	if f.Version != FactsVersion || f.Tool != "jactionlint" || len(f.Workflows) != 1 || f.Workflows[0].Path != ".github/workflows/release.yaml" {
		t.Errorf("%+v", f)
	}
	// The callers come from the whole repository although one file was asked for
	if cb := f.Workflows[0].CalledBy; cb == nil || !cb.Known || len(cb.Events) == 0 {
		t.Errorf("the callers of a file are those of the repository: %+v", cb)
	}

	if status, _, stderr := run("--export-facts", "-"); status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "stdin") {
		t.Errorf("stdin: status %d: %s", status, stderr)
	}
	if status, _, stderr := run("--export-facts", "--fix", filepath.Join(repo, ".github", "workflows", "ci.yaml")); status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "only reads") {
		t.Errorf("--fix: status %d: %s", status, stderr)
	}

	status, out, _ = run("--print-facts-schema")
	if status != 0 || out != string(FactsSchema()) {
		t.Errorf("--print-facts-schema: status %d", status)
	}
	if status, _, stderr := run("--print-facts-schema", "--print-config-schema"); status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "cannot be combined") {
		t.Errorf("two schemas: status %d: %s", status, stderr)
	}
}

// The committed schema is the one the binary prints.
func TestFactsSchemaIsCommitted(t *testing.T) {
	have, err := os.ReadFile(filepath.Join("schema", "jactionlint-facts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes.ReplaceAll(have, []byte("\r\n"), []byte("\n"))) != string(FactsSchema()) {
		t.Error("schema/jactionlint-facts.json is outdated. run `mise run schema`")
	}
}

// Without files the facts are those of the repository: its workflows and its actions, and not the Dependabot file.
func TestExportFactsRepository(t *testing.T) {
	repo := copyFactsRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".github", "dependabot.yml"), []byte("version: 2\nupdates: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.ExportFacts(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, w := range f.Workflows {
		paths = append(paths, w.Path)
	}
	want := []string{".github/workflows/broken.yaml", ".github/workflows/ci.yaml", ".github/workflows/orphan.yaml", ".github/workflows/release.yaml"}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("workflows (-want +got): %s", diff)
	}
	if len(f.Actions) != 1 || f.Actions[0].Path != ".github/actions/setup/action.yml" {
		t.Errorf("actions: %+v", f.Actions)
	}
	// Reading is all it does
	if _, err := os.Stat(filepath.Join(repo, ".github", "jactionlint-baseline.json")); err == nil {
		t.Error("a baseline was written")
	}
}

// The facts of about 1500 real workflows are made without a panic, and every parsed workflow has what the schema requires.
func TestExportFactsRealWorldCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("reads about 1500 files")
	}
	names, srcs := realWorldCorpus(t)
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range names {
		if err := os.WriteFile(filepath.Join(dir, "w"+strconv.Itoa(i)+".yaml"), srcs[i], 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.ExportFacts(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Workflows) != len(names) {
		t.Fatalf("%d workflows for %d files", len(f.Workflows), len(names))
	}
	jobs, parsed := 0, 0
	for _, w := range f.Workflows {
		if w.Parsed {
			parsed++
		}
		for _, j := range w.Jobs {
			jobs++
			if j.EffectivePermissions.Source == "" {
				t.Fatalf("%s: job %s has no source of permissions", w.Path, j.ID)
			}
		}
	}
	t.Logf("%d workflows, %d parsed, %d jobs, %d local calls", len(names), parsed, jobs, len(f.Calls))
	if parsed < len(names)*9/10 || jobs < len(names) {
		t.Errorf("parsed %d of %d workflows with %d jobs", parsed, len(names), jobs)
	}
	if _, err := f.Marshal(); err != nil {
		t.Fatal(err)
	}
}

func TestExportFactsInvalidPermissionIsUnknown(t *testing.T) {
	repo := copyFactsRepo(t)
	file := filepath.Join(repo, ".github", "workflows", "bad.yaml")
	src := "on: push\npermissions:\n  contents: Write\n  issues: ${{ vars.LEVEL }}\n  checks: read\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.ExportFacts([]string{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"contents": "unknown", "issues": "unknown", "checks": "read"}
	if diff := cmp.Diff(want, f.Workflows[0].Permissions.Scopes); diff != "" {
		t.Errorf("scopes (-want +got): %s", diff)
	}
}

// validateAgainst checks a decoded JSON value against the part of JSON Schema (draft-07) that FactsSchema uses: type,
// enum, required, properties, additionalProperties, items, allOf and local $ref.
func validateAgainst(schema map[string]any, defs map[string]any, v any, path string) []string {
	if ref, ok := schema["$ref"].(string); ok {
		return validateAgainst(defs[strings.TrimPrefix(ref, "#/definitions/")].(map[string]any), defs, v, path)
	}
	var errs []string
	if all, ok := schema["allOf"].([]any); ok {
		for _, sub := range all {
			errs = append(errs, validateAgainst(sub.(map[string]any), defs, v, path)...)
		}
	}
	if typ, ok := schema["type"].(string); ok {
		okType := false
		switch typ {
		case "string":
			_, okType = v.(string)
		case "boolean":
			_, okType = v.(bool)
		case "number":
			_, okType = v.(float64)
		case "integer":
			f, isNum := v.(float64)
			okType = isNum && f == float64(int64(f))
		case "array":
			_, okType = v.([]any)
		case "object":
			_, okType = v.(map[string]any)
		}
		if !okType {
			return append(errs, fmt.Sprintf("%s: %v is not of type %s", path, v, typ))
		}
	}
	if enum, ok := schema["enum"].([]any); ok && !slices.Contains(enum, v) {
		errs = append(errs, fmt.Sprintf("%s: %v is not one of %v", path, v, enum))
	}
	if obj, ok := v.(map[string]any); ok {
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, has := obj[r.(string)]; !has {
					errs = append(errs, fmt.Sprintf("%s: %q is required", path, r))
				}
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for k, val := range obj {
			switch {
			case props[k] != nil:
				errs = append(errs, validateAgainst(props[k].(map[string]any), defs, val, path+"."+k)...)
			case schema["additionalProperties"] != nil:
				if ap, ok := schema["additionalProperties"].(map[string]any); ok {
					errs = append(errs, validateAgainst(ap, defs, val, path+"."+k)...)
				}
			}
		}
	}
	if arr, ok := v.([]any); ok {
		if items, ok := schema["items"].(map[string]any); ok {
			for i, e := range arr {
				errs = append(errs, validateAgainst(items, defs, e, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	}
	return errs
}

func validateFactsDocument(t *testing.T, doc []byte) {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(FactsSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatal(err)
	}
	for _, e := range validateAgainst(schema, schema["definitions"].(map[string]any), v, "$") {
		t.Errorf("the document does not match the schema: %s", e)
	}
}

// Every document is valid against the schema, also for the workflows that have no event of their own, whose lists
// are empty and must be [] and not null.
func TestExportFactsMatchesTheSchema(t *testing.T) {
	repo := copyFactsRepo(t)
	only := "on: workflow_call\njobs:\n  c:\n    uses: ./.github/workflows/release.yaml\n  d:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/setup\n"
	if err := os.WriteFile(filepath.Join(repo, ".github", "workflows", "only-called.yaml"), []byte(only), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{WorkingDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.ExportFacts(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(doc, []byte("null")) {
		t.Errorf("the document has a null:\n%s", doc)
	}
	validateFactsDocument(t, doc)
	// The golden fixture too
	validateFactsDocument(t, mustMarshalFacts(t, exportTestFacts(t)))
}

func mustMarshalFacts(t *testing.T, f *Facts) []byte {
	t.Helper()
	b, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Every type that implements Event has a case in eventFacts: a trigger must not export as a name only.
func TestEveryEventTypeHasFacts(t *testing.T) {
	pos := &Pos{Line: 3, Col: 5}
	str := func(v string) []*String { return []*String{{Value: v}} }
	samples := map[string]Event{
		"WebhookEvent":            &WebhookEvent{Hook: &String{Value: "push"}, Types: str("x"), Pos: pos},
		"ScheduledEvent":          &ScheduledEvent{Pos: pos},
		"WorkflowDispatchEvent":   &WorkflowDispatchEvent{Pos: pos},
		"RepositoryDispatchEvent": &RepositoryDispatchEvent{Pos: pos},
		"WorkflowCallEvent":       &WorkflowCallEvent{Pos: pos},
		"ImageVersionEvent":       &ImageVersionEvent{Types: str("ready"), Names: str("ubuntu"), Versions: str("1.*"), Pos: pos},
	}
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "ast.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "EventName" {
			continue
		}
		name := fmt.Sprint(fn.Recv.List[0].Type.(*ast.StarExpr).X)
		ev, ok := samples[name]
		if !ok {
			t.Errorf("%s implements Event: add a case to eventFacts and a sample to this test", name)
			continue
		}
		got := eventFacts(ev)
		if got.Location == nil || got.Location.Line != 3 {
			t.Errorf("%s: eventFacts has no location: %+v", name, got)
		}
	}
	got := eventFacts(samples["ImageVersionEvent"])
	if diff := cmp.Diff(map[string][]string{"names": {"ubuntu"}, "versions": {"1.*"}}, got.Filters); diff != "" || len(got.Types) != 1 {
		t.Errorf("image_version: %+v %s", got, diff)
	}
}
