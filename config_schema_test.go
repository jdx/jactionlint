package jactionlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v4"
)

// --- a validator for the subset of JSON Schema the schemas use -------------------------------------------------

// schemaValidator validates a decoded document (JSON or YAML) against a decoded schema. It supports what the
// generated schemas use: type, enum, properties, additionalProperties, required, items, minItems, minimum,
// pattern, oneOf and local $ref. Keywords it does not know make the test fail, so a new keyword is not ignored.
type schemaValidator struct {
	root map[string]any
}

var knownSchemaKeywords = map[string]bool{
	"$schema": true, "$id": true, "$ref": true, "title": true, "description": true, "default": true, "deprecated": true, "format": true,
	"type": true, "enum": true, "properties": true, "additionalProperties": true, "required": true, "items": true, "minItems": true,
	"minimum": true, "pattern": true, "oneOf": true, "definitions": true,
}

func (v schemaValidator) validate(schema map[string]any, doc any, path string) []string {
	for k := range schema {
		if !knownSchemaKeywords[k] {
			return []string{fmt.Sprintf("%s: the test validator does not know the keyword %q", path, k)}
		}
	}
	if ref, ok := schema["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/definitions/")
		defs, _ := v.root["definitions"].(map[string]any)
		target, ok := defs[name].(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: unresolved $ref %s", path, ref)}
		}
		return v.validate(target, doc, path)
	}
	var errs []string
	if t, ok := schema["type"]; ok {
		var types []string
		switch t := t.(type) {
		case string:
			types = []string{t}
		case []any:
			for _, x := range t {
				types = append(types, x.(string))
			}
		}
		if !slices.ContainsFunc(types, func(t string) bool { return typeMatches(t, doc) }) {
			return []string{fmt.Sprintf("%s: %v is not of type %v", path, doc, types)}
		}
	}
	if e, ok := schema["enum"].([]any); ok {
		if !slices.ContainsFunc(e, func(x any) bool { return fmt.Sprint(x) == fmt.Sprint(doc) }) {
			errs = append(errs, fmt.Sprintf("%s: %v is not one of %v", path, doc, e))
		}
	}
	if p, ok := schema["pattern"].(string); ok {
		if s, ok := doc.(string); ok && !regexp.MustCompile(p).MatchString(s) {
			errs = append(errs, fmt.Sprintf("%s: %q does not match %s", path, s, p))
		}
	}
	if m, ok := schema["minimum"].(float64); ok {
		if f, ok := toFloat(doc); ok && f < m {
			errs = append(errs, fmt.Sprintf("%s: %v is less than %v", path, doc, m))
		}
	}
	if one, ok := schema["oneOf"].([]any); ok {
		n := 0
		for _, s := range one {
			if len(v.validate(s.(map[string]any), doc, path)) == 0 {
				n++
			}
		}
		if n != 1 {
			errs = append(errs, fmt.Sprintf("%s: %v matches %d of the oneOf alternatives", path, doc, n))
		}
	}
	switch d := doc.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, ok := d[r.(string)]; !ok {
					errs = append(errs, fmt.Sprintf("%s: %q is required", path, r))
				}
			}
		}
		keys := make([]string, 0, len(d))
		for k := range d {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if ps, ok := props[k].(map[string]any); ok {
				errs = append(errs, v.validate(ps, d[k], path+"."+k)...)
				continue
			}
			switch ap := schema["additionalProperties"].(type) {
			case bool:
				if !ap {
					errs = append(errs, fmt.Sprintf("%s: unknown property %q", path, k))
				}
			case map[string]any:
				errs = append(errs, v.validate(ap, d[k], path+"."+k)...)
			}
		}
	case []any:
		if m, ok := schema["minItems"].(float64); ok && float64(len(d)) < m {
			errs = append(errs, fmt.Sprintf("%s: fewer than %v items", path, m))
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, x := range d {
				errs = append(errs, v.validate(items, x, fmt.Sprintf("%s[%d]", path, i))...)
			}
		}
	}
	return errs
}

func toFloat(doc any) (float64, bool) {
	switch d := doc.(type) {
	case int:
		return float64(d), true
	case int64:
		return float64(d), true
	case uint64:
		return float64(d), true
	case float64:
		return d, true
	}
	return 0, false
}

func typeMatches(t string, doc any) bool {
	switch t {
	case "string":
		_, ok := doc.(string)
		return ok
	case "boolean":
		_, ok := doc.(bool)
		return ok
	case "null":
		return doc == nil
	case "object":
		_, ok := doc.(map[string]any)
		return ok
	case "array":
		_, ok := doc.([]any)
		return ok
	case "integer":
		f, ok := toFloat(doc)
		return ok && f == math.Trunc(f)
	case "number":
		_, ok := toFloat(doc)
		return ok
	}
	return false
}

func loadSchema(t *testing.T, b []byte) schemaValidator {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	return schemaValidator{root}
}

func validateConfigYAML(t *testing.T, v schemaValidator, src string) []string {
	t.Helper()
	var doc any
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("not YAML: %v", err)
	}
	if doc == nil {
		return nil // an empty file is a valid config
	}
	// YAML 1.1 reads 2027-06-30 as a timestamp; the parser of jactionlint and YAML 1.2 read a string
	doc = timestampsToStrings(doc)
	return v.validate(v.root, doc, "$")
}

func timestampsToStrings(doc any) any {
	switch d := doc.(type) {
	case time.Time:
		return d.Format("2006-01-02")
	case map[string]any:
		for k, x := range d {
			d[k] = timestampsToStrings(x)
		}
	case []any:
		for i, x := range d {
			d[i] = timestampsToStrings(x)
		}
	}
	return doc
}

// --- tests -----------------------------------------------------------------------------------------------------

func TestSchemasAreUpToDate(t *testing.T) {
	for name, want := range map[string][]byte{
		"jactionlint-config.json": ConfigSchema(),
		"jactionlint-output.json": OutputSchema(),
	} {
		have, err := os.ReadFile(filepath.Join("schema", name))
		if err != nil {
			t.Fatal(err)
		}
		have = bytes.ReplaceAll(have, []byte("\r\n"), []byte("\n"))
		if !bytes.Equal(have, want) {
			t.Errorf("schema/%s is outdated. regenerate it with `mise run schema` (go run ./scripts/generate-config-schema)", name)
		}
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sorted(ss []string) []string {
	ss = slices.Clone(ss)
	sort.Strings(ss)
	return ss
}

func sub(t *testing.T, m map[string]any, path ...string) map[string]any {
	t.Helper()
	for _, p := range path {
		n, ok := m[p].(map[string]any)
		if !ok {
			t.Fatalf("the schema has no %s", strings.Join(path, "."))
		}
		m = n
	}
	return m
}

// TestConfigSchemaKnowsTheKeysOfTheParser keeps the schema and the parser (which rejects unknown keys) in step: a
// key added to one has to be added to the other.
func TestConfigSchemaKnowsTheKeysOfTheParser(t *testing.T) {
	root := loadSchema(t, ConfigSchema()).root
	props := sub(t, root, "properties")
	if got, want := keysOf(props), sorted(configTopKeys); !slices.Equal(got, want) {
		t.Errorf("top-level keys of the schema differ from configTopKeys:\n got %v\nwant %v", got, want)
	}
	for _, c := range []struct {
		name string
		path []string
		keys []string
	}{
		{"self-hosted-runner", []string{"properties", "self-hosted-runner", "properties"}, selfHostedRunnerKeys},
		{"fix", []string{"properties", "fix", "properties"}, fixConfigKeys},
		{"shellcheck", []string{"properties", "shellcheck", "properties"}, shellcheckConfigKeys},
		{"online-options", []string{"properties", "online-options", "properties"}, onlineOptionsKeys},
		{"paths", []string{"properties", "paths", "additionalProperties", "properties"}, pathConfigKeys},
		{"ignores", []string{"properties", "ignores", "items", "properties"}, configIgnoreKeys},
		{"required-actions", []string{"properties", "required-actions", "items", "properties"}, requiredActionKeys},
		{"timeout-minutes", []string{"properties", "timeout-minutes", "properties"}, legacyTimeoutMinutesKy},
	} {
		if got, want := keysOf(sub(t, root, c.path...)), sorted(c.keys); !slices.Equal(got, want) {
			t.Errorf("%s: keys of the schema differ from the parser:\n got %v\nwant %v", c.name, got, want)
		}
	}
}

func TestConfigSchemaHasEveryRuleAndOption(t *testing.T) {
	root := loadSchema(t, ConfigSchema()).root
	rules := sub(t, root, "properties", "rules", "properties")
	if got, want := keysOf(rules), ruleIDs(); !slices.Equal(got, want) {
		t.Fatalf("the rules of the schema differ from the registry")
	}
	for _, r := range ruleRegistry {
		s := rules[r.ID].(map[string]any)
		if len(r.Options) == 0 {
			if s["$ref"] != "#/definitions/level" {
				t.Errorf("%s has no options and must be a level: %v", r.ID, s)
			}
			continue
		}
		alt := s["oneOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
		want := []string{"level"}
		for _, o := range r.Options {
			want = append(want, o.Name)
		}
		if got := keysOf(alt); !slices.Equal(got, sorted(want)) {
			t.Errorf("%s: options of the schema %v, of the registry %v", r.ID, got, sorted(want))
		}
	}
	// The IDs for fix.rules and ignores
	ids := sub(t, root, "properties", "fix", "properties", "rules", "items")["enum"].([]any)
	if len(ids) != len(ruleRegistry) {
		t.Errorf("fix.rules lists %d IDs, the registry has %d", len(ids), len(ruleRegistry))
	}
}

// TestSchemaPropertiesAreDescribed makes the next key explain itself: a property without a description shows
// nothing when an editor hovers over it.
func TestSchemaPropertiesAreDescribed(t *testing.T) {
	for name, b := range map[string][]byte{"config": ConfigSchema(), "output": OutputSchema()} {
		root := loadSchema(t, b).root
		var walk func(path string, n any)
		walk = func(path string, n any) {
			m, ok := n.(map[string]any)
			if !ok {
				return
			}
			if props, ok := m["properties"].(map[string]any); ok {
				for k, p := range props {
					pm := p.(map[string]any)
					if d, _ := pm["description"].(string); strings.TrimSpace(d) == "" {
						t.Errorf("%s schema: %s.%s has no description", name, path, k)
					}
					walk(path+"."+k, p)
				}
			}
			for _, k := range []string{"items", "additionalProperties"} {
				walk(path+"."+k, m[k])
			}
			if one, ok := m["oneOf"].([]any); ok {
				for i, o := range one {
					walk(fmt.Sprintf("%s.oneOf[%d]", path, i), o)
				}
			}
		}
		walk("$", root)
	}
}

func TestConfigSchemaAcceptsRealConfigs(t *testing.T) {
	v := loadSchema(t, ConfigSchema())
	var files []string
	err := filepath.WalkDir("testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch d.Name() {
		case "jactionlint.yaml", "jactionlint.yml", "actionlint.yaml", "actionlint.yml", "ok.yml":
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 8 {
		t.Fatalf("found only %d config files", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseConfig(b); err != nil {
			continue // a config the parser rejects (it needs "extends" or is a negative test)
		}
		if errs := validateConfigYAML(t, v, string(b)); len(errs) > 0 {
			t.Errorf("the parser accepts %s but the schema does not:\n%s", f, strings.Join(errs, "\n"))
		}
	}

	// The example of docs/config.md, which uses most keys
	b, err := os.ReadFile("docs/config.md")
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) // a Windows checkout can have CRLF
	m := regexp.MustCompile("(?s)## Configuration file\n.*?```yaml\n(.*?)```").FindSubmatch(b)
	if m == nil {
		t.Fatal("the example of docs/config.md is not found")
	}
	if errs := validateConfigYAML(t, v, string(m[1])); len(errs) > 0 {
		t.Errorf("the example of docs/config.md does not validate:\n%s", strings.Join(errs, "\n"))
	}
}

func TestConfigSchemaRejectsMistakes(t *testing.T) {
	v := loadSchema(t, ConfigSchema())
	for _, tc := range []struct{ name, src, want string }{
		{"unknown key", "profil: default\n", `unknown property "profil"`},
		{"profile", "profile: strictest\n", "is not one of"},
		{"unknown rule", "rules:\n  unpinned-use: error\n", `unknown property "unpinned-use"`},
		{"level", "rules:\n  unpinned-uses: fatal\n", "oneOf"},
		{"unknown option", "rules:\n  max-run-lines:\n    level: warn\n    maxx: 80\n", "oneOf"},
		{"option type", "rules:\n  max-run-lines:\n    max: lots\n", "oneOf"},
		{"option on a rule without options", "rules:\n  unpinned-uses:\n    max: 3\n", "oneOf"},
		{"ignores need a rule", "ignores:\n  - file: a.yaml\n", `"rule" is required`},
		{"ignores unknown rule", "ignores:\n  - rule: nope\n    file: a.yaml\n", "oneOf"},
		{"ignores date", "ignores:\n  - rule: unpinned-uses\n    file: a\n    expires: tomorrow\n", "does not match"},
		{"online-options", "online-options:\n  mode: sometimes\n", "is not one of"},
		{"fix rule", "fix:\n  rules: [nope]\n", "is not one of"},
		{"generated-files", "generated-files: maybe\n", "is not one of"},
		{"required-actions", "required-actions:\n  - version: v4\n", `"action" is required`},
		{"shellcheck key", "shellcheck:\n  rcfil: x\n", `unknown property "rcfil"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateConfigYAML(t, v, tc.src)
			if !strings.Contains(strings.Join(errs, "\n"), tc.want) {
				t.Errorf("errors %v do not contain %q", errs, tc.want)
			}
		})
	}
	for _, src := range []string{
		"rules:\n  unpinned-uses: warn\n  missing-permissions: false\n  require-shell: true\n  max-run-lines:\n    level: warn\n    max: 30\n",
		"config-variables: null\nconfig-secrets: []\nbaseline: true\nprofile: strict\n",
		"ignores:\n  - rule: [unpinned-uses, missing-timeout]\n    job: build\n",
	} {
		if errs := validateConfigYAML(t, v, src); len(errs) > 0 {
			t.Errorf("%q: %v", src, errs)
		}
	}
}

func TestInitConfigWritesTheSchemaComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.GenerateDefaultConfig(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".github", "jactionlint.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "# yaml-language-server: $schema="+ConfigSchemaURL+"\n") {
		t.Errorf("the generated file does not start with the schema comment: %.120q", b)
	}
	if _, err := ReadConfigFile(filepath.Join(dir, ".github", "jactionlint.yaml")); err != nil {
		t.Errorf("the generated file is not a valid config: %v", err)
	}
	if errs := validateConfigYAML(t, loadSchema(t, ConfigSchema()), string(b)); len(errs) > 0 {
		t.Errorf("the generated file does not validate: %v", errs)
	}
}

func TestPrintSchemaFlags(t *testing.T) {
	for flag, want := range map[string][]byte{"--print-config-schema": ConfigSchema(), "--print-output-schema": OutputSchema()} {
		var out, errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		if status := cmd.Main([]string{"jactionlint", flag}); status != 0 {
			t.Fatalf("%s: status %d: %s", flag, status, errOut.String())
		}
		if !bytes.Equal(out.Bytes(), want) {
			t.Errorf("%s did not print the schema", flag)
		}
	}
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	if status := cmd.Main([]string{"jactionlint", "--print-config-schema", "--print-output-schema"}); status != ExitStatusInvalidCommandOption {
		t.Errorf("both flags: status %d", status)
	}
}

// TestOutputSchemaDescribesTheJSONOutput lints files which give findings with fixes and with reasons for having
// none, and validates what --format json and jsonl print.
func TestOutputSchemaDescribesTheJSONOutput(t *testing.T) {
	v := loadSchema(t, OutputSchema())
	finding := sub(t, v.root, "definitions", "finding")

	files := []string{
		filepath.Join("testdata", "examples", "main.yaml"),
		filepath.Join("testdata", "fix", "missing_timeout.yaml"),
	}
	files = slices.DeleteFunc(files, func(f string) bool { _, err := os.Stat(f); return err != nil })
	if matches, _ := filepath.Glob(filepath.Join("testdata", "fix", "*.yaml")); len(matches) > 0 {
		files = append(files, matches[:min(len(matches), 12)]...)
	}
	sawFix, sawNoFix := false, false
	for _, f := range files {
		var out, errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		cmd.Main([]string{"jactionlint", "--shellcheck=", "--pyflakes=", "--profile", "pedantic", "--format", "json", f})
		var doc any
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("%s: not JSON: %v: %s", f, err, out.String())
		}
		if errs := v.validate(v.root, doc, "$"); len(errs) > 0 {
			t.Errorf("%s: --format json does not validate:\n%s", f, strings.Join(errs, "\n"))
		}
		for _, item := range doc.([]any) {
			m := item.(map[string]any)
			_, hasFix := m["fix"]
			_, hasNoFix := m["no_fix"]
			sawFix, sawNoFix = sawFix || hasFix, sawNoFix || hasNoFix
		}

		out.Reset()
		cmd = Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		cmd.Main([]string{"jactionlint", "--shellcheck=", "--pyflakes=", "--profile", "pedantic", "--format", "jsonl", f})
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			if line == "" {
				continue
			}
			var item any
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				t.Fatalf("%s: jsonl line is not JSON: %v", f, err)
			}
			if errs := v.validate(finding, item, "$"); len(errs) > 0 {
				t.Errorf("%s: a jsonl line does not validate:\n%s", f, strings.Join(errs, "\n"))
			}
		}
	}
	if !sawFix || !sawNoFix {
		t.Errorf("the files must produce a fix (%v) and a no_fix (%v) to cover them", sawFix, sawNoFix)
	}

	// The schema is not vacuous
	bad := map[string]any{"message": "x", "line": 0.0}
	if errs := v.validate(finding, bad, "$"); len(errs) == 0 {
		t.Error("an incomplete finding must not validate")
	}
}
