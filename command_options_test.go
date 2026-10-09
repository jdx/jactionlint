package jactionlint

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/spf13/pflag"
)

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	// --color changes the global switch of the color package
	noColor := color.NoColor
	t.Cleanup(func() { color.NoColor = noColor })
	var stdout, stderr bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &stderr}
	code := cmd.Main(append([]string{"jactionlint"}, args...))
	return code, stdout.String(), stderr.String()
}

const cliWorkflow = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo hi\n"

func cliWorkflowFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "w.yaml")
	if err := os.WriteFile(path, []byte(cliWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIGNUOptionSyntax(t *testing.T) {
	path := cliWorkflowFile(t)
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(cfg, []byte("profile: correctness\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := []string{"--shellcheck=", "--pyflakes="}
	for name, args := range map[string][]string{
		"long equals":           {"--format=json"},
		"long separate":         {"--format", "json"},
		"short separate":        {"-f", "json"},
		"short attached":        {"-fjson"},
		"short bundle":          {"-vfjson"},
		"short config attach":   {"-c" + cfg, "-fjson"},
		"short config sep":      {"-c", cfg, "-f", "json"},
		"options after files":   {"-fjson"},
		"double dash":           {"-f", "json", "--"},
		"profile short":         {"-p", "correctness", "-f", "json"},
		"ignore short":          {"-i", "nothing", "-fjson"},
		"color equals":          {"--color=never", "-fjson"},
		"bare color":            {"--color", "-fjson"},
		"min severity":          {"--min-severity=warn", "-fjson"},
		"no baseline":           {"--no-baseline", "-fjson"},
		"no online":             {"--no-online", "-fjson"},
		"online cache":          {"--online=cache", "--online-cache-ttl=1m", "-fjson"},
		"fix rules without fix": nil,
	} {
		if args == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			full := append(append([]string{}, base...), args...)
			full = append(full, path)
			code, stdout, stderr := runCLI(t, "", full...)
			if code != ExitStatusSuccessNoProblem || !strings.HasPrefix(strings.TrimSpace(stdout), "[") {
				t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
		})
	}
}

func TestCLIDoubleDashEndsOptions(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "-w.yaml")
	if err := os.WriteFile(odd, []byte(cliWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "", "--shellcheck=", "--pyflakes=", "-w.yaml")
	if code != ExitStatusInvalidCommandOption {
		t.Errorf("-w.yaml is a bundle of options, got exit %d: %s", code, stderr)
	}
	code, stdout, stderr := runCLI(t, "", "--shellcheck=", "--pyflakes=", "--", "-w.yaml")
	if code != ExitStatusSuccessNoProblem {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCLILoneDashReadsStdin(t *testing.T) {
	code, stdout, stderr := runCLI(t, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    stepz: 1\n", "--shellcheck=", "--pyflakes=", "--stdin-filename", "wf.yaml", "--oneline", "-")
	if code != ExitStatusSuccessProblemFound || !strings.Contains(stdout, "wf.yaml") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCLIOptionalValuesNeedEquals(t *testing.T) {
	// --fix unsafe is --fix and a file named "unsafe": the value of an optional option is never the next argument
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "", "--fix", "unsafe")
	if code == ExitStatusSuccessNoProblem || !strings.Contains(stderr, "unsafe") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestCLIUsageErrors(t *testing.T) {
	path := cliWorkflowFile(t)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"unknown long":         {[]string{"--frobnicate"}, "unknown option --frobnicate"},
		"unknown short":        {[]string{"-Z"}, "unknown option"},
		"abbreviation":         {[]string{"--form", "json", path}, "unknown option --form"},
		"missing value":        {[]string{"--format"}, "requires a value"},
		"missing short value":  {[]string{"-f"}, "requires a value"},
		"bad color":            {[]string{"--color=sometimes", path}, "always, never or auto"},
		"bad fix":              {[]string{"--fix=maybe", path}, "--fix=unsafe"},
		"bad online":           {[]string{"--online=sometimes", path}, "invalid online mode"},
		"empty baseline":       {[]string{"--baseline=", path}, "must not be empty"},
		"bad duration":         {[]string{"--online-cache-ttl=soon", path}, "online-cache-ttl"},
		"bad profile":          {[]string{"--profile=loud", path}, "--profile"},
		"bad severity":         {[]string{"--min-severity=loud", path}, "--min-severity"},
		"baseline both":        {[]string{"--baseline", "--no-baseline", path}, "cannot be combined"},
		"online both":          {[]string{"--online", "--no-online", path}, "cannot be combined"},
		"boolean with a value": {[]string{"--verbose=maybe", path}, "verbose"},
		"fix-rules alone":      {[]string{"--fix-rules=missing-timeout", path}, "--fix-rules"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, "", append([]string{"--shellcheck=", "--pyflakes="}, tc.args...)...)
			if code != ExitStatusInvalidCommandOption {
				t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr %q does not contain %q", stderr, tc.want)
			}
			if name != "bad profile" && name != "bad severity" && !strings.Contains(stderr, "--help") && strings.Count(strings.TrimSpace(stderr), "\n") > 0 {
				t.Errorf("stderr is not one line: %q", stderr)
			}
		})
	}
}

func TestCLIUnknownOptionIsOneLineWithAHint(t *testing.T) {
	code, stdout, stderr := runCLI(t, "", "--frobnicate")
	if code != ExitStatusInvalidCommandOption || stdout != "" {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
	if stderr != "jactionlint: unknown option --frobnicate (try --help)\n" {
		t.Errorf("unexpected message %q", stderr)
	}
}

func TestCLIHelpAndVersion(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		code, stdout, stderr := runCLI(t, "", arg)
		if code != ExitStatusSuccessNoProblem || stderr != "" {
			t.Errorf("%s: exit %d, stderr %q", arg, code, stderr)
		}
		if !strings.HasPrefix(stdout, "Usage: jactionlint [OPTIONS] [FILES...] [-]") {
			t.Errorf("%s: unexpected help %q", arg, stdout)
		}
		for _, o := range cliOptions {
			if !strings.Contains(stdout, "--"+o.Long) {
				t.Errorf("%s: help does not mention --%s", arg, o.Long)
			}
		}
		for _, line := range strings.Split(stdout, "\n") {
			if len(line) > 110 {
				t.Errorf("help line is too long (%d): %q", len(line), line)
			}
		}
	}
	for _, arg := range []string{"--version", "-V"} {
		code, stdout, _ := runCLI(t, "", arg)
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if code != ExitStatusSuccessNoProblem || len(lines) != 3 || !strings.HasPrefix(lines[2], "built with ") {
			t.Errorf("%s: exit %d, stdout %q", arg, code, stdout)
		}
	}
	// -v is --verbose, not the version
	code, stdout, _ := runCLI(t, "", "-v", "--shellcheck=", "--pyflakes=", "-fjson", cliWorkflowFile(t))
	if code != ExitStatusSuccessNoProblem || strings.Contains(stdout, "built with") {
		t.Errorf("-v: exit %d, stdout %q", code, stdout)
	}
}

// TestCLILegacyOptions: every option of v1 gets an error which names the replacement, before the argument is
// read as a bundle of short options (-fix is not -f -i -x).
func TestCLILegacyOptions(t *testing.T) {
	want := map[string]string{
		"help": "--help", "version": "--version", "verbose": "--verbose", "debug": "--debug", "stdin-filename": "--stdin-filename",
		"format": "--format", "oneline": "--oneline", "rule-ids": "--rule-ids", "color": "--color", "no-color": "--no-color",
		"no-hints": "--no-hints", "min-severity": "--min-severity", "strict-exit": "--strict-exit", "profile": "--profile",
		"config-file": "--config-file", "ignore": "--ignore", "init-config": "--init-config", "migrate-config": "--migrate-config",
		"migrate-ignores": "--migrate-ignores", "fix": "--fix", "diff": "--diff", "rules": "--fix-rules", "baseline": "--baseline",
		"baseline-write": "--baseline-write", "baseline-check": "--baseline-check", "sarif-hide-baselined": "--sarif-hide-baselined",
		"online": "--online", "online-api-url": "--online-api-url", "online-token-env": "--online-token-env",
		"online-token-file": "--online-token-file", "online-allow": "--online-allow", "online-deny": "--online-deny",
		"online-cache-ttl": "--online-cache-ttl", "online-max-wait": "--online-max-wait", "shellcheck": "--shellcheck", "pyflakes": "--pyflakes",
	}
	got := legacyOptions()
	for name, repl := range want {
		if got[name] != repl {
			t.Errorf("legacy -%s is replaced by %q, want %q", name, got[name], repl)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected legacy option -%s", name)
		}
	}
	for name, repl := range want {
		for _, args := range [][]string{{"-" + name}, {"-" + name + "=x"}, {"--shellcheck=", "-" + name, "x"}} {
			code, stdout, stderr := runCLI(t, "", args...)
			wantMsg := "jactionlint: unknown option -" + name + "; did you mean " + repl + "? (try --help)\n"
			if code != ExitStatusInvalidCommandOption || stdout != "" || stderr != wantMsg {
				t.Errorf("%v: exit %d, stdout %q, stderr %q, want %q", args, code, stdout, stderr, wantMsg)
			}
		}
	}
	for arg, repl := range map[string]string{"-online=false": "--no-online", "-baseline=false": "--no-baseline", "-profile=default": "--profile"} {
		if msg := legacyHint(arg); !strings.Contains(msg, "did you mean "+repl+"?") {
			t.Errorf("%s: %q", arg, msg)
		}
	}
}

func TestCLILegacyOptionsAreNotCheckedAsValuesOrAfterDoubleDash(t *testing.T) {
	path := cliWorkflowFile(t)
	// the value of --ignore may look like anything
	code, _, stderr := runCLI(t, "", "--shellcheck=", "--pyflakes=", "--ignore", "-fix", "-i", "-format", path)
	if code != ExitStatusSuccessNoProblem {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
	code, _, stderr = runCLI(t, "", "--shellcheck=", "--pyflakes=", "--", "-fix")
	if code == ExitStatusInvalidCommandOption && strings.Contains(stderr, "did you mean") {
		t.Errorf("an operand after -- was read as an option: %q", stderr)
	}
}

func TestCLIOptionTable(t *testing.T) {
	f := newCommandFlags()
	inTable := map[string]cliOption{}
	shorts := map[string]string{}
	for _, o := range cliOptions {
		if _, dup := inTable[o.Long]; dup {
			t.Errorf("--%s is in the table twice", o.Long)
		}
		inTable[o.Long] = o
		if o.Help == "" || o.Group == "" {
			t.Errorf("--%s needs a group and a help text", o.Long)
		}
		if strings.ToLower(o.Long) != o.Long || strings.Contains(o.Long, "_") {
			t.Errorf("--%s is not kebab-case", o.Long)
		}
		if o.Short != "" {
			if prev, dup := shorts[o.Short]; dup {
				t.Errorf("-%s is used by --%s and --%s", o.Short, prev, o.Long)
			}
			shorts[o.Short] = o.Long
		}
		fl := f.fs.Lookup(o.Long)
		if fl == nil {
			t.Errorf("--%s is in the table but not registered", o.Long)
			continue
		}
		if fl.Shorthand != o.Short {
			t.Errorf("--%s: shorthand %q in the table, %q registered", o.Long, o.Short, fl.Shorthand)
		}
		if o.Optional != (fl.NoOptDefVal != "" && o.ArgName != "") {
			t.Errorf("--%s: optional value mismatch", o.Long)
		}
		if (o.ArgName == "") != (fl.Value.Type() == "bool") {
			t.Errorf("--%s: argument name and type %s disagree", o.Long, fl.Value.Type())
		}
	}
	f.fs.VisitAll(func(fl *pflag.Flag) {
		if _, ok := inTable[fl.Name]; !ok {
			t.Errorf("--%s is registered but not in the table", fl.Name)
		}
	})
	if len(shorts) > 10 {
		t.Errorf("%d short options; keep them to the most used ones", len(shorts))
	}
	known := map[string]bool{}
	for _, g := range cliGroups {
		known[g] = true
	}
	for _, o := range cliOptions {
		if !known[o.Group] {
			t.Errorf("--%s: unknown group %q", o.Long, o.Group)
		}
	}
}

// TestCLIOptionsAreDocumented: the manual and the usage document mention every option.
func TestCLIOptionsAreDocumented(t *testing.T) {
	for _, file := range []string{"docs/usage.md", "man/jactionlint.1.ronn"} {
		b, err := os.ReadFile(filepath.FromSlash(file))
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range cliOptions {
			if !strings.Contains(string(b), "--"+o.Long) {
				t.Errorf("%s does not document --%s", file, o.Long)
			}
			if o.Short != "" && !strings.Contains(string(b), "-"+o.Short) {
				t.Errorf("%s does not mention -%s", file, o.Short)
			}
		}
	}
	b, err := os.ReadFile(filepath.FromSlash("docs/v2-migration.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range cliOptions {
		if o.Replaces != "" && !strings.Contains(string(b), "`--"+o.Long) {
			t.Errorf("docs/v2-migration.md does not list --%s", o.Long)
		}
	}
}

var staleOption = regexp.MustCompile(`(^|[^-A-Za-z0-9_./])-(help|profile|format|fix|no-color|color|oneline|diff|rules|rule-ids|min-severity|strict-exit|no-hints|debug|verbose|version|ignore|shellcheck|pyflakes|config-file|init-config|migrate-config|migrate-ignores|stdin-filename|online|baseline|baseline-write|baseline-check|sarif-hide-baselined|online-[a-z-]+)([^A-Za-z0-9_-]|$)`)

// TestNoStaleV1Options keeps the single-dash long options of v1 out of the documentation, the scripts, the
// workflows and the messages of the command. The migration table in docs/v2-migration.md may name them: it sits
// between the legacy-options markers.
func TestNoStaleV1Options(t *testing.T) {
	wholeFiles := []string{"README.md", "CONTRIBUTING.md", "hk.pkl", "mise.toml", "Dockerfile", ".pre-commit-hooks.yaml"}
	roots := []string{"docs", "man", "scripts", ".github", "testdata", "cmd", "playground", "internal"}
	var files []string
	files = append(files, wholeFiles...)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "generate-popular-actions", "node_modules", ".vitepress", "public", "bench", "fix", "realworld", "fuzz":
					return filepath.SkipDir
				}
				return nil
			}
			p = filepath.ToSlash(p)
			switch {
			case strings.HasSuffix(p, "_test.go"), strings.HasSuffix(p, "package-lock.json"):
				return nil
			case strings.HasPrefix(p, "testdata/") && !strings.HasSuffix(p, ".sh") && !strings.HasSuffix(p, ".md") && !strings.HasPrefix(p, "testdata/rule_ids.d/"):
				return nil
			}
			files = append(files, p)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Go sources of the command and its messages; the table of the options names the v1 spellings on purpose
	goFiles, _ := filepath.Glob("*.go")
	for _, g := range goFiles {
		if !strings.HasSuffix(g, "_test.go") && g != "command_options.go" {
			files = append(files, g)
		}
	}
	sort.Strings(files)
	for _, file := range files {
		b, err := os.ReadFile(filepath.FromSlash(file))
		if err != nil {
			t.Fatal(err)
		}
		inTable := false
		for i, line := range strings.Split(string(b), "\n") {
			switch {
			case strings.Contains(line, "<!-- legacy-options:start -->"):
				inTable = true
			case strings.Contains(line, "<!-- legacy-options:end -->"):
				inTable = false
			}
			if inTable {
				continue
			}
			// the helper tools have options of their own, e.g. `check-checks -fix`
			if strings.Contains(line, "check-checks") && strings.Contains(line, "-fix") {
				continue
			}
			if m := staleOption.FindStringSubmatch(line); m != nil {
				t.Errorf("%s:%d uses the v1 option -%s (write --%s): %s", file, i+1, m[2], m[2], strings.TrimSpace(line))
			}
		}
	}
}

// FuzzCommandLine: no argument list makes the parser panic or return a multi-line error.
func FuzzCommandLine(f *testing.F) {
	for _, s := range []string{"-fix", "--fix=unsafe", "-vfjson", "-c", "--", "-", "--format=", "-i-fix", "--online=cache,strict", "-\x00", "--baseline=--x"} {
		f.Add(s, "x")
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		fl := newCommandFlags()
		if err := fl.parse([]string{a, b}); err != nil && strings.Contains(err.Error(), "\n") && !strings.Contains(a+b, "\n") {
			t.Errorf("multi-line error %q", err)
		}
	})
}
