package jactionlint

import (
	"flag"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var ruleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

func TestRuleRegistryIsWellFormed(t *testing.T) {
	checks, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	prev := ""
	for _, r := range ruleRegistry {
		if !ruleIDPattern.MatchString(r.ID) {
			t.Errorf("rule ID %q must be kebab-case", r.ID)
		}
		if seen[r.ID] {
			t.Errorf("rule ID %q is registered twice", r.ID)
		}
		seen[r.ID] = true
		if r.ID <= prev {
			t.Errorf("rule ID %q must be sorted after %q", r.ID, prev)
		}
		prev = r.ID

		switch r.Group {
		case RuleGroupCorrectness, RuleGroupSecurity, RuleGroupPolicy, RuleGroupStyle:
		default:
			t.Errorf("rule %q has invalid group %q", r.ID, r.Group)
		}
		if r.Summary == "" || !strings.HasSuffix(r.Summary, ".") {
			t.Errorf("rule %q must have a summary which ends with a period: %q", r.ID, r.Summary)
		}
		if r.DefaultLevel < SeverityInfo || r.DefaultLevel > SeverityError {
			t.Errorf("rule %q has invalid default level %v", r.ID, r.DefaultLevel)
		}
		if r.Profile != "" {
			if _, ok := profileRank[r.Profile]; !ok {
				t.Errorf("rule %q has invalid profile %q", r.ID, r.Profile)
			}
		}
		if r.Online && r.Profile != "" {
			t.Errorf("online rule %q must be independent of the profile", r.ID)
		}
		if r.DocsAnchor != "" && !strings.Contains(string(checks), `<a id="`+r.DocsAnchor+`"></a>`) {
			t.Errorf("rule %q refers to anchor %q which does not exist in docs/checks.md", r.ID, r.DocsAnchor)
		}
		for _, o := range r.Options {
			if o.Name == "" || o.Name == "level" {
				t.Errorf("rule %q has an option with invalid name %q", r.ID, o.Name)
			}
		}
		if got := RuleDocURL(r.ID); !strings.HasSuffix(got, "/rules#"+r.ID) {
			t.Errorf("unexpected doc URL %q", got)
		}
	}

	if !slices.IsSortedFunc(Rules(), func(a, b RuleInfo) int { return strings.Compare(a.ID, b.ID) }) {
		t.Error("Rules() must be sorted")
	}
	if _, ok := LookupRule("unpinned-uses"); !ok {
		t.Error("unpinned-uses must be registered")
	}
	if _, ok := LookupRule("no-such-rule"); ok {
		t.Error("unknown rule was found")
	}
}

var updateRuleIDs = flag.String("update-rule-ids", "", "write the rule IDs which are in no testdata/rule_ids.d/*.txt to testdata/rule_ids.d/<value>.txt")

// Rule IDs are public API: they are written in configuration files, ignore comments and CI
// annotations. The files testdata/rule_ids.d/*.txt are the snapshot of all IDs which have been
// released. Removing or renaming an ID breaks users so it fails this test. Newly registered IDs are
// added with: go test -run TestRuleIDsAreStable -update-rule-ids=<name> (writes rule_ids.d/<name>.txt,
// so every batch of rules owns a separate file).
func TestRuleIDsAreStable(t *testing.T) {
	dir := filepath.Join("testdata", "rule_ids.d")
	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no snapshot files in %s", dir)
	}

	snapshot := map[string]string{}
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if id := strings.TrimSpace(line); id != "" {
				snapshot[id] = name
			}
		}
	}

	for id, file := range snapshot {
		if _, ok := LookupRule(id); !ok {
			t.Errorf("rule ID %q (listed in %s) was removed or renamed. IDs are stable; keep it registered", id, file)
		}
	}

	var added []string
	for _, r := range ruleRegistry {
		if _, ok := snapshot[r.ID]; !ok {
			added = append(added, r.ID)
		}
	}
	if len(added) == 0 {
		return
	}
	if *updateRuleIDs != "" {
		name := *updateRuleIDs
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(name) {
			t.Fatalf("invalid -update-rule-ids name %q", name)
		}
		dst := filepath.Join(dir, name+".txt")
		var prev []byte
		if b, err := os.ReadFile(dst); err == nil {
			prev = b
		}
		out := string(prev)
		for _, id := range added { // ruleRegistry is sorted by ID
			out += id + "\n"
		}
		if err := os.WriteFile(dst, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d rule IDs to %s", len(added), dst)
		return
	}
	for _, id := range added {
		t.Errorf("rule ID %q is in no testdata/rule_ids.d/*.txt. run: go test -run TestRuleIDsAreStable -update-rule-ids=<name>", id)
	}
}

// Every ID passed to the reporting functions must be registered, and every registered ID must be
// reported somewhere. This catches typos in IDs which no test input triggers.
func TestRuleIDsInSourceMatchRegistry(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	used := map[string]bool{}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "rule_registry.go" {
			continue
		}
		f, err := goparser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				// The ID of a RuleInfo registers the rule; it is not a report of it.
				if id, ok := n.Type.(*ast.Ident); ok && id.Name == "RuleInfo" {
					return false
				}
			case *ast.CallExpr:
				var fn string
				switch f := n.Fun.(type) {
				case *ast.SelectorExpr:
					fn = f.Sel.Name
				case *ast.Ident:
					fn = f.Name
				}
				idx := -1
				switch fn {
				case "ReportID", "ReportIDf", "ReportRange", "errorfID", "errorID", "errorIDAt", "errorfAtExpr", "errorAtExpr":
					idx = 0
				}
				if idx < 0 || len(n.Args) <= idx {
					return true
				}
				lit, ok := n.Args[idx].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				id, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				used[id] = true
				if _, ok := LookupRule(id); !ok {
					t.Errorf("%s: rule ID %q is reported but not registered", fset.Position(lit.Pos()), id)
				}
			case *ast.KeyValueExpr:
				if k, ok := n.Key.(*ast.Ident); ok && k.Name == "ID" {
					if lit, ok := n.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						id, _ := strconv.Unquote(lit.Value)
						used[id] = true
						if _, ok := LookupRule(id); !ok {
							t.Errorf("%s: rule ID %q is reported but not registered", fset.Position(lit.Pos()), id)
						}
					}
				}
			}
			return true
		})
	}

	for _, r := range ruleRegistry {
		if !used[r.ID] {
			t.Errorf("rule ID %q is registered but no source reports it", r.ID)
		}
	}
}

func TestProfileIncludes(t *testing.T) {
	tests := []struct {
		p, q Profile
		want bool
	}{
		{ProfileCorrectness, ProfileCorrectness, true},
		{ProfileCorrectness, ProfileDefault, false},
		{ProfileCorrectness, ProfilePedantic, false},
		{ProfileDefault, ProfileCorrectness, true},
		{ProfileDefault, ProfileDefault, true},
		{ProfileDefault, ProfilePedantic, false},
		{ProfilePedantic, ProfileCorrectness, true},
		{ProfilePedantic, ProfileDefault, true},
		{ProfilePedantic, ProfilePedantic, true},
		{ProfilePedantic, "", false},
	}
	for _, tc := range tests {
		if got := tc.p.Includes(tc.q); got != tc.want {
			t.Errorf("%q.Includes(%q) = %v, want %v", tc.p, tc.q, got, tc.want)
		}
	}
	for _, name := range []string{"correctness", "default", "pedantic"} {
		if p, err := ParseProfile(name); err != nil || string(p) != name {
			t.Errorf("ParseProfile(%q) = %q, %v", name, p, err)
		}
	}
	// The retired names are read in the config file only
	for _, name := range []string{"strict", "all"} {
		if _, err := ParseProfile(name); err == nil {
			t.Errorf("ParseProfile(%q) must fail: the name is retired", name)
		}
		if p, dep, err := parseConfigProfile(name); err != nil || p != ProfilePedantic || dep == "" {
			t.Errorf("parseConfigProfile(%q) = %q, %q, %v", name, p, dep, err)
		}
	}
	if _, err := ParseProfile("paranoid"); err == nil {
		t.Error("unknown profile must be an error")
	}
}

func TestSeverity(t *testing.T) {
	for _, s := range []Severity{SeverityOff, SeverityInfo, SeverityWarning, SeverityError} {
		got, err := ParseSeverity(s.String())
		if err != nil || got != s {
			t.Errorf("round trip of %v: got %v, %v", s, got, err)
		}
	}
	if s, err := ParseSeverity("warning"); err != nil || s != SeverityWarning {
		t.Errorf("warning must be accepted: %v %v", s, err)
	}
	if _, err := ParseSeverity("fatal"); err == nil {
		t.Error("unknown severity must be an error")
	}
	b, _ := SeverityWarning.MarshalText()
	if string(b) != "warn" {
		t.Errorf("unexpected text %q", b)
	}
}

func TestRegisterRulesPanicsOnDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering an ID twice must panic")
		}
	}()
	registerRules(RuleInfo{ID: "unpinned-uses"})
}

func TestRegisterRuleFactoryPanicsOnDuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering a factory twice must panic")
		}
	}()
	registerRuleFactory("matrix", func(*RuleEnv) []Rule { return nil })
}

// The order of the rules decides the order of errors at the same position, so it must not depend on
// the order of the source files.
func TestRuleFactoryOrderIsStable(t *testing.T) {
	var got []string
	for _, f := range ruleFactories {
		got = append(got, f.name)
	}
	if len(got) < len(legacyRuleOrder) || !slices.Equal(got[:len(legacyRuleOrder)], legacyRuleOrder) {
		t.Errorf("legacy rules must be applied first in the legacy order: %v", got)
	}
	if rest := got[len(legacyRuleOrder):]; !slices.IsSorted(rest) {
		t.Errorf("new rules must be sorted by name: %v", rest)
	}
}
