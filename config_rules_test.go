package jactionlint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConfigRulesParse(t *testing.T) {
	c := mustParseConfig(t, `
profile: strict
rules:
  unpinned-uses: warn
  require-shell: off
  missing-timeout: error
  max-run-lines:
    level: info
    max: 42
  timeout-too-long:
    max: 12.5
  invalid-glob: true
  matrix-duplicate-value: false
`)
	if c.Profile != ProfileStrict {
		t.Errorf("profile: %q", c.Profile)
	}
	levels := map[string]Severity{
		"unpinned-uses":          SeverityWarning,
		"require-shell":          SeverityOff,
		"missing-timeout":        SeverityError,
		"max-run-lines":          SeverityInfo,
		"timeout-too-long":       SeverityError, // options only: the default level
		"invalid-glob":           SeverityError,
		"matrix-duplicate-value": SeverityOff,
		"missing-permissions":    SeverityError, // the profile
		"unused-ignore-unknown":  SeverityError, // unknown IDs are custom rules
	}
	for id, want := range levels {
		if got := c.RuleLevel(id); got != want {
			t.Errorf("RuleLevel(%q) = %v, want %v", id, got, want)
		}
	}
	if v, ok := c.RuleOption("max-run-lines", "max"); !ok || v != 42 {
		t.Errorf("max-run-lines max = %v, %v", v, ok)
	}
	if v, ok := c.RuleOption("timeout-too-long", "max"); !ok || v != 12.5 {
		t.Errorf("timeout-too-long max = %v, %v", v, ok)
	}
	if _, ok := c.RuleOption("require-shell", "max"); ok {
		t.Error("unknown option must not be found")
	}
	if len(c.Deprecations) != 0 {
		t.Errorf("no deprecated key is used: %v", c.Deprecations)
	}
}

func TestConfigRuleLevelByProfile(t *testing.T) {
	ids := []string{"expression-type", "unsound-ternary", "unpinned-uses", "missing-permissions", "require-shell", "max-run-lines", "timeout-too-long", "required-actions"}
	tests := []struct {
		profile string
		want    map[string]bool
	}{
		{"", map[string]bool{"expression-type": true, "unsound-ternary": true}},
		{"default", map[string]bool{"expression-type": true, "unsound-ternary": true}},
		{"strict", map[string]bool{"expression-type": true, "unsound-ternary": true, "unpinned-uses": true, "missing-permissions": true}},
		{"all", map[string]bool{"expression-type": true, "unsound-ternary": true, "unpinned-uses": true, "missing-permissions": true, "require-shell": true, "max-run-lines": true}},
	}
	for _, tc := range tests {
		src := ""
		if tc.profile != "" {
			src = "profile: " + tc.profile + "\n"
		}
		c := mustParseConfig(t, src)
		for _, id := range ids {
			if got := c.RuleEnabled(id); got != tc.want[id] {
				t.Errorf("profile %q: RuleEnabled(%q) = %v, want %v", tc.profile, id, got, tc.want[id])
			}
		}
	}

	// A nil config behaves like an empty one
	var nilCfg *Config
	if !nilCfg.RuleEnabled("expression-type") || nilCfg.RuleEnabled("unpinned-uses") {
		t.Error("a nil config must use the default profile")
	}
	if _, ok := nilCfg.RuleOption("max-run-lines", "max"); !ok {
		t.Error("the default of an option is available without a config")
	}

	// max-run-lines enabled by the "all" profile uses the default maximum
	c := mustParseConfig(t, "profile: all\n")
	if m, ok := c.ruleOptionNumber("max-run-lines", "max"); !ok || m != DefaultMaxRunLines {
		t.Errorf("default max = %v, %v", m, ok)
	}

	// required-actions is enabled by the list, not by a profile
	c = mustParseConfig(t, "required-actions:\n  - action: actions/checkout\n")
	if !c.RuleEnabled("required-actions") {
		t.Error("required-actions must be enabled by the list")
	}
	c = mustParseConfig(t, "required-actions:\n  - action: actions/checkout\nrules:\n  required-actions: warn\n")
	if c.RuleLevel("required-actions") != SeverityWarning {
		t.Error("required-actions level must be configurable")
	}
}

func TestConfigParseStrictErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"unknown top-level key", "foo: 1\n", []string{`unknown key "foo" in the configuration at line:1,col:1`, "available keys are"}},
		{"did you mean a key", "self-hosted-runnr:\n  labels: []\n", []string{`unknown key "self-hosted-runnr"`, `did you mean "self-hosted-runner"?`}},
		{"did you mean a legacy key", "\nrequire-shel: true\n", []string{`line:2,col:1`, `did you mean "require-shell"?`}},
		{"unknown nested key", "self-hosted-runner:\n  label: [a]\n", []string{`unknown key "label" in "self-hosted-runner" at line:2,col:3`, `did you mean "labels"?`}},
		{"unknown key of path config", "paths:\n  a.yaml:\n    ignor: [x]\n", []string{`unknown key "ignor" in "paths"`, `did you mean "ignore"?`}},
		{"unknown key of required action", "required-actions:\n  - action: a/b\n    ver: v1\n", []string{`unknown key "ver" in "required-actions"`, `did you mean "version"?`}},
		{"unknown key of legacy timeout", "timeout-minutes:\n  require: true\n", []string{`unknown key "require" in "timeout-minutes"`}},
		{"unknown rule", "rules:\n  unpinned-use: error\n", []string{`unknown rule ID "unpinned-use" in "rules" at line:2,col:3`, `did you mean "unpinned-uses"?`, "https://jactionlint.jdx.dev/rules"}},
		{"unknown rule without suggestion", "rules:\n  zzzzzzzz: error\n", []string{`unknown rule ID "zzzzzzzz"`, "https://jactionlint.jdx.dev/rules"}},
		{"unknown rule option", "rules:\n  max-run-lines:\n    maxx: 3\n", []string{`unknown key "maxx" in the options of rule "max-run-lines"`, `did you mean "max"?`}},
		{"rule without options", "rules:\n  require-shell:\n    max: 3\n", []string{`unknown key "max" in the options of rule "require-shell"`}},
		{"invalid level", "rules:\n  require-shell: fatal\n", []string{`invalid severity "fatal"`, "line:2,col:18"}},
		{"invalid level in mapping", "rules:\n  require-shell: {level: fatal}\n", []string{`"level" must be one of`}},
		{"rule is a list", "rules:\n  require-shell: [error]\n", []string{"a rule must be configured with a level or a mapping"}},
		{"negative option", "rules:\n  max-run-lines: {max: -1}\n", []string{`invalid value -1 for option "max" of rule "max-run-lines"`, "non-negative integer"}},
		{"fractional integer option", "rules:\n  max-run-lines: {max: 1.5}\n", []string{`option "max"`, "non-negative integer"}},
		{"string option", "rules:\n  timeout-too-long: {max: abc}\n", []string{`option "max"`, "non-negative number"}},
		{"nan option", "rules:\n  timeout-too-long: {max: .nan}\n", []string{`option "max"`, "non-negative number"}},
		{"invalid profile", "profile: paranoid\n", []string{`invalid profile "paranoid"`, `in "profile"`}},
		{"extends needs a file", "extends: [a.yaml]\n", []string{`"extends" can be used only in a config file`}},
		{"negative max-run-lines", "max-run-lines: -1\n", []string{`"max-run-lines" must not be negative`}},
		{"broken yaml", "rules: [\n", []string{"yaml"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.in))
			if err == nil {
				t.Fatal("no error occurred")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err.Error(), w)
				}
			}
			if strings.Contains(err.Error(), "\n") {
				t.Errorf("error must be one line: %q", err.Error())
			}
		})
	}
}

func TestConfigLegacyKeysAreTranslated(t *testing.T) {
	c := mustParseConfig(t, `
require-commit-hash: true
require-permissions: true
require-checkout-before-local-action: false
require-expression-wrapping: true
check-falsy-ternary: false
check-workflow-run-names: true
require-shell: true
max-run-lines: 30
timeout-minutes:
  required: true
  max: 45
`)
	want := map[string]Severity{
		"unpinned-uses":               SeverityError,
		"missing-permissions":         SeverityError,
		"local-action-checkout":       SeverityOff, // a default rule can be turned off by the old key
		"require-expression-wrapping": SeverityError,
		"unsound-ternary":             SeverityOff,
		"workflow-run-names":          SeverityError,
		"require-shell":               SeverityError,
		"max-run-lines":               SeverityError,
		"missing-timeout":             SeverityError,
		"timeout-too-long":            SeverityError,
	}
	for id, lv := range want {
		if got := c.RuleLevel(id); got != lv {
			t.Errorf("RuleLevel(%q) = %v, want %v", id, got, lv)
		}
	}
	if m, _ := c.ruleOptionNumber("max-run-lines", "max"); m != 30 {
		t.Errorf("max-run-lines = %v", m)
	}
	if m, _ := c.ruleOptionNumber("timeout-too-long", "max"); m != 45 {
		t.Errorf("timeout max = %v", m)
	}
	if len(c.Deprecations) != 9 {
		t.Errorf("want one deprecation per key: %v", c.Deprecations)
	}
	for _, d := range c.Deprecations {
		if !strings.Contains(d, "is deprecated") || !strings.Contains(d, "-migrate-config") {
			t.Errorf("unexpected deprecation message %q", d)
		}
	}

	// timeout-minutes without required: only the maximum is set, missing-timeout is left to the profile
	c = mustParseConfig(t, "timeout-minutes:\n  max: 5\n")
	if _, set := c.Rules["missing-timeout"]; set || !c.RuleEnabled("timeout-too-long") {
		t.Errorf("unexpected rules %+v", c.Rules)
	}

	// max-run-lines: 0 means disabled, also under the "all" profile
	c = mustParseConfig(t, "profile: all\nmax-run-lines: 0\n")
	if c.RuleEnabled("max-run-lines") {
		t.Error("max-run-lines: 0 must disable the rule")
	}

	// Explicit rules win over the deprecated keys
	c = mustParseConfig(t, "require-shell: true\nrules:\n  require-shell: warn\n")
	if c.RuleLevel("require-shell") != SeverityWarning {
		t.Error("rules must win over the deprecated key")
	}
}

func TestConfigExtends(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	write("org/base.yaml", `
profile: strict
rules:
  unpinned-uses: warn
  require-shell: error
self-hosted-runner:
  labels: [org]
  strict-labels: true
config-variables: [A]
paths:
  a.yaml:
    ignore: [org-a]
  b.yaml:
    ignore: [org-b]
`)
	write("org/second.yaml", `
rules:
  require-shell: info
self-hosted-runner:
  labels: [second]
require-checkout-before-local-action: true
`)
	repo := write("repo/.github/jactionlint.yaml", `
extends:
  - ../../org/base.yaml
  - ../../org/second.yaml
rules:
  unpinned-uses: error
paths:
  b.yaml:
    ignore: [repo-b]
`)

	c, err := ReadConfigFile(repo)
	if err != nil {
		t.Fatal(err)
	}
	if c.Profile != ProfileStrict {
		t.Errorf("profile must be inherited: %q", c.Profile)
	}
	if got := c.RuleLevel("unpinned-uses"); got != SeverityError {
		t.Errorf("the file itself wins: %v", got)
	}
	if got := c.RuleLevel("require-shell"); got != SeverityInfo {
		t.Errorf("later extends wins: %v", got)
	}
	if !c.RuleEnabled("local-action-checkout") {
		t.Error("a deprecated key of an inherited file is translated")
	}
	if diff := cmp.Diff([]string{"second"}, c.SelfHostedRunner.Labels); diff != "" {
		t.Errorf("labels are replaced by the later file: %s", diff)
	}
	if !c.SelfHostedRunner.StrictLabels {
		t.Error("strict-labels must be inherited when later files do not set it")
	}
	if diff := cmp.Diff([]string{"A"}, c.ConfigVariables); diff != "" {
		t.Error(diff)
	}
	if len(c.Paths) != 2 {
		t.Fatalf("paths are merged by key: %v", c.Paths)
	}
	if !c.Paths["a.yaml"].Ignore.Match(&Error{Message: "org-a"}) {
		t.Error("path config of the base was lost")
	}
	if pc := c.Paths["b.yaml"]; !pc.Ignore.Match(&Error{Message: "repo-b"}) || pc.Ignore.Match(&Error{Message: "org-b"}) {
		t.Error("the same path is replaced by the file itself")
	}
	if len(c.Deprecations) != 1 || !strings.Contains(c.Deprecations[0], "second.yaml") {
		t.Errorf("deprecation must name the file: %v", c.Deprecations)
	}
	if c.Path != repo {
		t.Errorf("path: %q", c.Path)
	}

	t.Run("missing file", func(t *testing.T) {
		p := write("missing.yaml", "extends: [nothing.yaml]\n")
		_, err := ReadConfigFile(p)
		if err == nil || !strings.Contains(err.Error(), `could not load "nothing.yaml" listed in "extends"`) {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		write("cycle/a.yaml", "extends: [b.yaml]\n")
		write("cycle/b.yaml", "extends: [a.yaml]\n")
		_, err := ReadConfigFile(filepath.Join(dir, "cycle", "a.yaml"))
		if err == nil || !strings.Contains(err.Error(), `makes a cycle`) {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("self", func(t *testing.T) {
		p := write("self.yaml", "extends: [self.yaml]\n")
		_, err := ReadConfigFile(p)
		if err == nil || !strings.Contains(err.Error(), `makes a cycle`) {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("too deep", func(t *testing.T) {
		for i := 0; i < maxExtendsDepth+2; i++ {
			next := "last.yaml"
			if i < maxExtendsDepth+1 {
				next = "deep" + string(rune('a'+i+1)) + ".yaml"
			}
			body := "extends: [" + next + "]\n"
			if next == "last.yaml" {
				body = "profile: all\n"
			}
			write("deep/deep"+string(rune('a'+i))+".yaml", body)
		}
		write("deep/last.yaml", "profile: all\n")
		_, err := ReadConfigFile(filepath.Join(dir, "deep", "deepa.yaml"))
		if err == nil || !strings.Contains(err.Error(), "more than") {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("error in the base names both files", func(t *testing.T) {
		write("broken/base.yaml", "rules:\n  nope: error\n")
		p := write("broken/c.yaml", "extends: [base.yaml]\n")
		_, err := ReadConfigFile(p)
		if err == nil || !strings.Contains(err.Error(), "base.yaml") || !strings.Contains(err.Error(), `unknown rule ID "nope"`) {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("absolute path", func(t *testing.T) {
		abs := write("abs/base.yaml", "profile: all\n")
		p := write("abs/c.yaml", "extends: ['"+filepath.ToSlash(abs)+"']\n")
		c, err := ReadConfigFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if c.Profile != ProfileAll {
			t.Error("absolute extends was not loaded")
		}
	})
}

func TestConfigRulesAffectLinting(t *testing.T) {
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: echo ${{ github.event.issue.title }}\n"

	tests := []struct {
		what string
		cfg  string
		want []Severity
	}{
		{"default", "", []Severity{SeverityError}},
		{"lowered", "rules:\n  template-injection: warn\n", []Severity{SeverityWarning}},
		{"info", "rules:\n  template-injection: info\n", []Severity{SeverityInfo}},
		{"off", "rules:\n  template-injection: off\n", nil},
		{"strict adds the pinning rule", "profile: strict\n", []Severity{SeverityError, SeverityError, SeverityError, SeverityError}},
		{"strict with a lowered rule", "profile: strict\nrules:\n  unpinned-uses: warn\n  missing-permissions: off\n  missing-timeout: off\n", []Severity{SeverityWarning, SeverityError}},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintWithConfig(t, mustParseConfig(t, tc.cfg), src)
			var got []Severity
			for _, e := range errs {
				switch e.ID {
				case "template-injection", "unpinned-uses", "missing-permissions", "missing-timeout":
					got = append(got, e.Severity)
				}
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("severities (-want +got): %s\n%v", diff, errs)
			}
		})
	}
}

func TestLinterReportsDeprecatedConfigKeysOnce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jactionlint.yaml")
	if err := os.WriteFile(cfgPath, []byte("require-shell: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	l, err := NewLinter(io.Discard, &LinterOptions{ConfigFile: cfgPath, LogWriter: &log})
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n")
	for i := 0; i < 3; i++ {
		errs, err := l.Lint("test.yaml", src, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(errs) != 1 || errs[0].ID != "require-shell" {
			t.Fatalf("the deprecated key must still work: %v", errs)
		}
	}
	out := log.String()
	if n := strings.Count(out, "is deprecated"); n != 1 {
		t.Errorf("the deprecation must be reported once but got %d times: %q", n, out)
	}
	if !strings.Contains(out, "warning:") || !strings.Contains(out, fmt.Sprintf("%q", cfgPath)) || !strings.Contains(out, `"require-shell"`) {
		t.Errorf("unexpected warning: %q", out)
	}
}

func TestDidYouMean(t *testing.T) {
	c := []string{"labels", "strict-labels", "ignore"}
	tests := map[string]string{
		"label":        "labels",
		"lables":       "labels",
		"strict-label": "strict-labels",
		"IGNORE":       "ignore",
		"ignor":        "ignore",
		"zzz":          "",
		"":             "",
	}
	for in, want := range tests {
		if got := didYouMean(in, c); got != want {
			t.Errorf("didYouMean(%q) = %q, want %q", in, got, want)
		}
	}
	if d := editDistance("kitten", "sitting"); d != 3 {
		t.Errorf("distance = %d", d)
	}
	if d := editDistance("ab", "ba"); d != 1 {
		t.Errorf("transposition = %d", d)
	}
}

func TestRuleConfigYAML(t *testing.T) {
	c := mustParseConfig(t, "rules:\n  require-shell: \"off\"\n  unpinned-uses: WARNING\n")
	if c.RuleLevel("require-shell") != SeverityOff || c.RuleLevel("unpinned-uses") != SeverityWarning {
		t.Errorf("%+v", c.Rules)
	}
}

func TestInitConfigIsValidAndMigrationFree(t *testing.T) {
	f := filepath.Join(t.TempDir(), "jactionlint.yaml")
	if err := writeDefaultConfigFile(f); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfigFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Deprecations) != 0 || len(c.Rules) != 0 || c.Profile != "" {
		t.Errorf("the generated config must not change the behavior: %+v", c)
	}
	b, _ := os.ReadFile(f)
	for _, want := range []string{"profile:", "rules:", "extends:", "paths:", "ignore:"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("generated config does not mention %q", want)
		}
	}
	// Every commented example must be valid when uncommented
	var uncommented []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "#") && (strings.HasPrefix(l, "#  ") || strings.HasPrefix(l, "#profile") || strings.HasPrefix(l, "#extends") || strings.HasPrefix(l, "#assume")) {
			uncommented = append(uncommented, strings.TrimPrefix(l, "#"))
		} else {
			uncommented = append(uncommented, l)
		}
	}
	dir := filepath.Dir(f)
	if err := os.MkdirAll(filepath.Join(dir, "..", "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "..", "shared", "jactionlint.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	g := filepath.Join(dir, "uncommented.yaml")
	if err := os.WriteFile(g, []byte(strings.Join(uncommented, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfigFile(g); err != nil {
		t.Errorf("examples in the generated config are invalid: %v\n%s", err, strings.Join(uncommented, "\n"))
	}
}

func TestLegacyTimeoutMinutesLeavesMissingTimeoutToTheProfile(t *testing.T) {
	// The profile must decide when "required" is not written, so the level is the same as without the key
	for _, profile := range []string{"default", "strict", "all"} {
		with := mustParseConfig(t, "profile: "+profile+"\ntimeout-minutes:\n  max: 30\n")
		without := mustParseConfig(t, "profile: "+profile+"\n")
		if with.RuleLevel("missing-timeout") != without.RuleLevel("missing-timeout") {
			t.Errorf("profile %s: timeout-minutes {max: 30} changed missing-timeout from %v to %v", profile, without.RuleLevel("missing-timeout"), with.RuleLevel("missing-timeout"))
		}
		if !with.RuleEnabled("timeout-too-long") {
			t.Errorf("profile %s: timeout-too-long should be on", profile)
		}
	}

	off := mustParseConfig(t, "profile: strict\ntimeout-minutes:\n  required: false\n  max: 30\n")
	if off.RuleEnabled("missing-timeout") {
		t.Error("required: false must turn missing-timeout off")
	}
	on := mustParseConfig(t, "timeout-minutes:\n  required: true\n")
	if on.RuleLevel("missing-timeout") != SeverityError {
		t.Errorf("required: true: level %v", on.RuleLevel("missing-timeout"))
	}
	if len(on.Deprecations) != 1 || strings.Contains(mustParseConfig(t, "timeout-minutes:\n  max: 3\n").Deprecations[0], "missing-timeout") {
		t.Errorf("the message for {max} alone must not mention missing-timeout")
	}
}

func TestMigrateConfigTimeoutMinutes(t *testing.T) {
	for _, tc := range []struct {
		in          string
		wantMissing string // "" means the rule must not be written
	}{
		{"timeout-minutes:\n  max: 30\n", ""},
		{"timeout-minutes:\n  required: false\n  max: 30\n", "off"},
		{"timeout-minutes:\n  required: true\n", "error"},
	} {
		out, _, err := MigrateConfig([]byte(tc.in))
		if err != nil {
			t.Fatal(err)
		}
		c := mustParseConfig(t, string(out))
		rc, set := c.Rules["missing-timeout"]
		switch {
		case tc.wantMissing == "" && set:
			t.Errorf("%q: missing-timeout written:\n%s", tc.in, out)
		case tc.wantMissing == "off" && (!set || rc.Level != SeverityOff):
			t.Errorf("%q: want missing-timeout off:\n%s", tc.in, out)
		case tc.wantMissing == "error" && (!set || rc.Level != SeverityError):
			t.Errorf("%q: want missing-timeout error:\n%s", tc.in, out)
		}
		if len(c.Deprecations) != 0 {
			t.Errorf("%q: deprecations remain: %v", tc.in, c.Deprecations)
		}
		before := mustParseConfig(t, tc.in)
		if before.RuleLevel("missing-timeout") != c.RuleLevel("missing-timeout") || before.RuleLevel("timeout-too-long") != c.RuleLevel("timeout-too-long") {
			t.Errorf("%q: the migrated config means something else:\n%s", tc.in, out)
		}
	}
}
