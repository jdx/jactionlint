package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMigrateConfig(t *testing.T) {
	in := `# Labels of my runners
self-hosted-runner:
  labels: [mine] # keep

# Pin everything
require-commit-hash: true

# Keep the shell explicit
require-shell: true
check-falsy-ternary: false
max-run-lines: 30
timeout-minutes:
  required: true
  max: 45.5
paths:
  a.yaml:
    ignore: [x]
`
	out, migrated, err := MigrateConfig([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"require-commit-hash", "require-shell", "check-falsy-ternary", "max-run-lines", "timeout-minutes"}
	if diff := cmp.Diff(wantKeys, migrated); diff != "" {
		t.Errorf("migrated keys (-want +got): %s", diff)
	}

	s := string(out)
	for _, deprecated := range legacyConfigKeys {
		if regexp.MustCompile(`(?m)^` + deprecated + `:`).MatchString(s) {
			t.Errorf("%q must be removed:\n%s", deprecated, s)
		}
	}
	for _, kept := range []string{"# Labels of my runners", "labels: [mine] # keep", "# Pin everything", "# Keep the shell explicit", "a.yaml:", "ignore: [x]"} {
		if !strings.Contains(s, kept) {
			t.Errorf("%q must be kept:\n%s", kept, s)
		}
	}

	// The result is valid, has no deprecated key and means the same as the input
	before := mustParseConfig(t, in)
	after := mustParseConfig(t, s)
	if len(after.Deprecations) != 0 {
		t.Errorf("deprecations remain: %v\n%s", after.Deprecations, s)
	}
	for _, r := range Rules() {
		if before.RuleLevel(r.ID) != after.RuleLevel(r.ID) {
			t.Errorf("level of %q changed from %v to %v\n%s", r.ID, before.RuleLevel(r.ID), after.RuleLevel(r.ID), s)
		}
	}
	for _, opt := range [][2]string{{"max-run-lines", "max"}, {"timeout-too-long", "max"}} {
		b, _ := before.RuleOption(opt[0], opt[1])
		a, _ := after.RuleOption(opt[0], opt[1])
		if a != b {
			t.Errorf("option %v changed from %v to %v", opt, b, a)
		}
	}
	if diff := cmp.Diff(before.SelfHostedRunner.Labels, after.SelfHostedRunner.Labels); diff != "" {
		t.Error(diff)
	}

	// Migrating again changes nothing
	again, migrated, err := MigrateConfig(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrated) != 0 || !bytes.Equal(again, out) {
		t.Errorf("migration must be idempotent: %v\n%s", migrated, again)
	}
}

func TestMigrateConfigPlacesRulesWhereTheFirstKeyWas(t *testing.T) {
	out, _, err := MigrateConfig([]byte("config-variables: [A]\nrequire-shell: true\nconfig-secrets: [B]\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "config-variables: [A]\nrules:\n  require-shell: error\nconfig-secrets: [B]\n"
	if diff := cmp.Diff(want, string(out)); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestMigrateConfigMergesIntoExistingRules(t *testing.T) {
	in := "rules:\n  unpinned-uses: warn\n  require-shell: info\nrequire-shell: true\nrequire-permissions: true\n"
	out, migrated, err := MigrateConfig([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrated) != 2 {
		t.Errorf("migrated: %v", migrated)
	}
	c := mustParseConfig(t, string(out))
	if c.RuleLevel("require-shell") != SeverityInfo {
		t.Errorf("an existing rule must not be overwritten:\n%s", out)
	}
	if c.RuleLevel("missing-permissions") != SeverityError || c.RuleLevel("unpinned-uses") != SeverityWarning {
		t.Errorf("unexpected result:\n%s", out)
	}
}

func TestMigrateConfigNothingToMigrate(t *testing.T) {
	for _, in := range []string{"", "profile: pedantic\n", "# only a comment\n", "rules:\n  require-shell: error\n"} {
		out, migrated, err := MigrateConfig([]byte(in))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if len(migrated) != 0 || string(out) != in {
			t.Errorf("%q must not change but got %q %v", in, out, migrated)
		}
	}
}

func TestMigrateConfigError(t *testing.T) {
	for _, in := range []string{"unknown-key: 1\n", "rules: [\n", "max-run-lines: -1\n", "rules:\n  nope: error\n"} {
		if _, _, err := MigrateConfig([]byte(in)); err == nil {
			t.Errorf("%q must be an error", in)
		}
	}
}

func TestMigrateConfigFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "jactionlint.yaml")
	if err := os.WriteFile(p, []byte("require-shell: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	keys, err := MigrateConfigFile(p)
	if err != nil || len(keys) != 1 {
		t.Fatalf("%v %v", keys, err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "rules:\n  require-shell: error\n" {
		t.Errorf("unexpected content %q", b)
	}
	keys, err = MigrateConfigFile(p)
	if err != nil || len(keys) != 0 {
		t.Fatalf("second migration: %v %v", keys, err)
	}

	if _, err := MigrateConfigFile(filepath.Join(t.TempDir(), "none.yaml")); err == nil {
		t.Error("a missing file must be an error")
	}
	if err := os.WriteFile(p, []byte("nope: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateConfigFile(p); err == nil || !strings.Contains(err.Error(), "could not migrate config file") {
		t.Errorf("unexpected error %v", err)
	}
}

func TestLinterMigrateConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	newLinter := func(opts *LinterOptions) *Linter {
		l, err := NewLinter(&out, opts)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}

	if err := newLinter(&LinterOptions{WorkingDir: root}).MigrateConfig(root); err == nil || !strings.Contains(err.Error(), "no config file") {
		t.Errorf("a project without a config file must be an error: %v", err)
	}

	// The config of the project is found. The name used by actionlint is accepted
	cfg := filepath.Join(root, ".github", "actionlint.yaml")
	if err := os.WriteFile(cfg, []byte("require-permissions: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := newLinter(&LinterOptions{WorkingDir: root}).MigrateConfig(root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "was migrated") || !strings.Contains(out.String(), "require-permissions") {
		t.Errorf("unexpected output %q", out.String())
	}
	b, _ := os.ReadFile(cfg)
	if string(b) != "rules:\n  missing-permissions: error\n" {
		t.Errorf("unexpected content %q", b)
	}

	out.Reset()
	if err := newLinter(&LinterOptions{WorkingDir: root}).MigrateConfig(root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no deprecated key") {
		t.Errorf("unexpected output %q", out.String())
	}

	// -config-file wins
	other := filepath.Join(t.TempDir(), "other.yaml")
	if err := os.WriteFile(other, []byte("require-shell: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := newLinter(&LinterOptions{ConfigFile: other, WorkingDir: root}).MigrateConfig(root); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(other)
	if string(b) != "rules:\n  require-shell: error\n" {
		t.Errorf("unexpected content %q", b)
	}

	// Not in a project
	if err := newLinter(&LinterOptions{}).MigrateConfig(t.TempDir()); err == nil {
		t.Error("outside of a project must be an error")
	}
}

func TestCommandMigrateConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("check-workflow-run-names: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if code := cmd.Main([]string{"jactionlint", "-config-file", p, "-migrate-config"}); code != ExitStatusSuccessNoProblem {
		t.Fatalf("exit status %d: %s", code, stderr.String())
	}
	b, _ := os.ReadFile(p)
	if string(b) != "rules:\n  workflow-run-names: error\n" {
		t.Errorf("unexpected content %q", b)
	}
	if !strings.Contains(stdout.String(), "was migrated") {
		t.Errorf("unexpected output %q", stdout.String())
	}
}
