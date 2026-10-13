package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/execabs"
)

func TestShellcheckConfigParse(t *testing.T) {
	c, err := ParseConfig([]byte("shellcheck:\n  rcfile: .shellcheckrc\n  args: [--exclude=SC2129, -eSC2086, --severity=warning]\n  enable: [avoid-nullary-conditions, all]\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := ShellcheckConfig{RCFile: ".shellcheckrc", Args: []string{"--exclude=SC2129", "-eSC2086", "--severity=warning"}, Enable: []string{"avoid-nullary-conditions", "all"}}
	if c.Shellcheck.RCFile != want.RCFile || !slices.Equal(c.Shellcheck.Args, want.Args) || !slices.Equal(c.Shellcheck.Enable, want.Enable) {
		t.Errorf("got %+v, want %+v", c.Shellcheck, want)
	}
	if got := c.Shellcheck.extraArgs(); !slices.Equal(got, []string{"--exclude=SC2129", "-eSC2086", "--severity=warning", "--enable=avoid-nullary-conditions,all"}) {
		t.Errorf("extraArgs = %q", got)
	}

	for _, tc := range []struct {
		name, src, want string
	}{
		{"unknown key", "shellcheck:\n  rcfil: x\n", `unknown key "rcfil" in "shellcheck" at line 2`},
		{"format", "shellcheck:\n  args: [--format=gcc]\n", `invalid argument "--format=gcc" in "shellcheck.args": jactionlint reads the JSON output`},
		{"short format", "shellcheck:\n  args: [-fgcc]\n", `invalid argument "-fgcc" in "shellcheck.args"`},
		{"shell", "shellcheck:\n  args: [--shell=sh]\n", `jactionlint passes the shell of each script`},
		{"norc", "shellcheck:\n  args: [--norc]\n", `use "rcfile"`},
		{"rcfile in args", "shellcheck:\n  args: [--rcfile=x]\n", `use "rcfile"`},
		{"file name", "shellcheck:\n  args: [script.sh]\n", `invalid argument "script.sh"`},
		{"stdin", "shellcheck:\n  args: ['-']\n", `invalid argument "-"`},
		{"separate value", "shellcheck:\n  args: ['-e', 'SC2086']\n", `invalid argument "SC2086"`},
		{"enable name", "shellcheck:\n  enable: [Avoid_Nullary]\n", `invalid check name "Avoid_Nullary" in "shellcheck.enable"`},
		{"enable list", "shellcheck:\n  enable: [a, b c]\n", `invalid check name "b c"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestShellcheckConfigExtends(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("base.yaml", "shellcheck:\n  rcfile: base.rc\n  args: [--exclude=SC1]\n  enable: [all]\n")
	child := write("child.yaml", "extends: [base.yaml]\nshellcheck:\n  args: [--exclude=SC2]\n")
	c, err := ReadConfigFile(child)
	if err != nil {
		t.Fatal(err)
	}
	// Written keys replace, the others are inherited
	if c.Shellcheck.RCFile != "base.rc" || !slices.Equal(c.Shellcheck.Args, []string{"--exclude=SC2"}) || !slices.Equal(c.Shellcheck.Enable, []string{"all"}) {
		t.Errorf("merged = %+v", c.Shellcheck)
	}
}

func TestShellcheckRCFileResolution(t *testing.T) {
	root, cwd := t.TempDir(), t.TempDir()
	for _, p := range []string{filepath.Join(root, ".shellcheckrc"), filepath.Join(cwd, "flag.rc"), filepath.Join(filepath.Dir(root), "outside.rc")} {
		if err := os.WriteFile(p, []byte("disable=SC2086\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &Config{Shellcheck: ShellcheckConfig{RCFile: ".shellcheckrc"}}

	if p, err := shellcheckRCFile("", cwd, nil, root); p != "" || err != nil {
		t.Errorf("no rcfile: %q, %v", p, err)
	}
	if p, err := shellcheckRCFile("", cwd, cfg, root); err != nil || p != filepath.Join(root, ".shellcheckrc") {
		t.Errorf("config relative to the repository root: %q, %v", p, err)
	}
	if p, err := shellcheckRCFile("flag.rc", cwd, cfg, root); err != nil || p != filepath.Join(cwd, "flag.rc") {
		t.Errorf("the flag wins and is relative to the working directory: %q, %v", p, err)
	}
	if _, err := shellcheckRCFile("missing.rc", cwd, cfg, root); err == nil || !strings.Contains(err.Error(), "--shellcheck-rcfile") {
		t.Errorf("a missing file of the flag: %v", err)
	}
	if _, err := shellcheckRCFile("", cwd, &Config{Shellcheck: ShellcheckConfig{RCFile: "missing.rc"}}, root); err == nil || !strings.Contains(err.Error(), `"shellcheck.rcfile"`) {
		t.Errorf("a missing file of the config: %v", err)
	}
	if _, err := shellcheckRCFile("", cwd, &Config{Shellcheck: ShellcheckConfig{RCFile: root}}, root); err == nil {
		t.Error("a directory is no rcfile")
	}
	// A config of the repository stays inside the repository, the user's own config does not
	outside := &Config{Shellcheck: ShellcheckConfig{RCFile: "../" + filepath.Base(filepath.Dir(root)) + "/outside.rc"}}
	outside.Shellcheck.RCFile = filepath.Join(filepath.Dir(root), "outside.rc")
	if _, err := shellcheckRCFile("", cwd, outside, root); err == nil || !strings.Contains(err.Error(), "outside of the repository") {
		t.Errorf("a repository config must not read a file outside: %v", err)
	}
	outside.userOwned = true
	if p, err := shellcheckRCFile("", cwd, outside, root); err != nil || p != filepath.Join(filepath.Dir(root), "outside.rc") {
		t.Errorf("the user's config can: %q, %v", p, err)
	}
}

func TestShellcheckArgs(t *testing.T) {
	r := newRuleShellcheck(nil)
	const defaults = "-e SC1091,SC2194,SC2050,SC2153,SC2154,SC2157,SC2043"
	if got := strings.Join(r.shellcheckArgs("bash"), " "); got != "--norc -f json -x --shell bash "+defaults+" -" {
		t.Errorf("default arguments changed: %s", got)
	}
	r.rcfile = "/x/.shellcheckrc"
	r.extraArgs = []string{"--exclude=SC2129", "--enable=all"}
	if got := strings.Join(r.shellcheckArgs("sh"), " "); got != "--rcfile=/x/.shellcheckrc -f json -x --shell sh "+defaults+" --exclude=SC2129 --enable=all -" {
		t.Errorf("arguments = %s", got)
	}
}

// shellcheckProject writes a project with a workflow whose script is the given text, and the config file
// written to .github/jactionlint.yaml when config is not empty. It returns the path of the workflow.
func shellcheckProject(t *testing.T, script, config string) (root, workflow string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: " + script + "\n"
	workflow = filepath.Join(root, ".github", "workflows", "ci.yaml")
	if err := os.WriteFile(workflow, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, workflow
}

// fakeShellcheck puts a shellcheck on PATH which records its arguments in a file and finds nothing.
func fakeShellcheck(t *testing.T) (argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake shellcheck is a shell script")
	}
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' \"$*\" > '" + argsFile + "'\necho '[]'\n"
	if err := os.WriteFile(filepath.Join(dir, "shellcheck"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return argsFile
}

func runCmd(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	return cmd.Main(append([]string{"jactionlint", "--pyflakes="}, args...)), out.String()
}

func TestShellcheckReceivesTheConfiguredArguments(t *testing.T) {
	argsFile := fakeShellcheck(t)
	root, wf := shellcheckProject(t, "echo one", "shellcheck:\n  rcfile: sc.rc\n  args: [--exclude=SC2129]\n  enable: [avoid-nullary-conditions]\n")
	rc := filepath.Join(root, "sc.rc")
	if err := os.WriteFile(rc, []byte("disable=SC2086\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		b, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(argsFile)
		return strings.TrimSpace(string(b))
	}

	if status, out := runCmd(t, wf); status != 0 {
		t.Fatalf("status = %d: %s", status, out)
	}
	want := "--rcfile=" + rc + " -f json -x --shell bash -e SC1091,SC2194,SC2050,SC2153,SC2154,SC2157,SC2043 --exclude=SC2129 --enable=avoid-nullary-conditions -"
	if got := read(); got != want {
		t.Errorf("from the config:\n got %s\nwant %s", got, want)
	}

	// The flag wins over the config, and is relative to the working directory
	flagRC := filepath.Join(t.TempDir(), "flag.rc")
	if err := os.WriteFile(flagRC, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := runCmd(t, "--shellcheck-rcfile="+flagRC, wf); status != 0 {
		t.Fatalf("status = %d: %s", status, out)
	}
	if got := read(); !strings.HasPrefix(got, "--rcfile="+flagRC+" ") {
		t.Errorf("from the flag: %s", got)
	}

	// Without either: --norc as before
	_, wf2 := shellcheckProject(t, "echo one", "")
	if status, out := runCmd(t, wf2); status != 0 {
		t.Fatalf("status = %d: %s", status, out)
	}
	if got := read(); got != "--norc -f json -x --shell bash -e SC1091,SC2194,SC2050,SC2153,SC2154,SC2157,SC2043 -" {
		t.Errorf("the default changed: %s", got)
	}
}

func TestShellcheckRCFileMustExist(t *testing.T) {
	fakeShellcheck(t)
	_, wf := shellcheckProject(t, "echo one", "shellcheck:\n  rcfile: nowhere.rc\n")
	status, out := runCmd(t, wf)
	if status != ExitStatusFailure || !strings.Contains(out, `"shellcheck.rcfile": the shellcheckrc`) {
		t.Errorf("config: status = %d: %s", status, out)
	}
	_, wf = shellcheckProject(t, "echo one", "")
	status, out = runCmd(t, "--shellcheck-rcfile=nowhere.rc", wf)
	if status != ExitStatusFailure || !strings.Contains(out, "--shellcheck-rcfile: the shellcheckrc") {
		t.Errorf("flag: status = %d: %s", status, out)
	}
}

// TestShellcheckConfigWithRealShellcheck runs the real command: a rc file that disables a check, an argument
// that excludes it and an optional check that adds a finding.
func TestShellcheckConfigWithRealShellcheck(t *testing.T) {
	if _, err := execabs.LookPath("shellcheck"); err != nil {
		t.Skip("shellcheck is not found")
	}
	count := func(out, code string) int { return strings.Count(out, "SC"+code+":") }

	root, wf := shellcheckProject(t, `echo $FOO "$BAR"`, "")
	status, out := runCmd(t, wf)
	if status != 1 || count(out, "2086") != 1 {
		t.Fatalf("baseline: status = %d: %s", status, out)
	}
	// A .shellcheckrc in the repository is not read by default (--norc)
	if err := os.WriteFile(filepath.Join(root, ".shellcheckrc"), []byte("disable=SC2086\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, out := runCmd(t, wf); count(out, "2086") != 1 {
		t.Errorf("the rc file must not be read without configuration: %s", out)
	}
	// With the file named by a flag it applies
	if status, out := runCmd(t, "--shellcheck-rcfile="+filepath.Join(root, ".shellcheckrc"), wf); status != 0 || count(out, "2086") != 0 {
		t.Errorf("flag: status = %d: %s", status, out)
	}
	// ... or named by the config
	if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte("shellcheck:\n  rcfile: .shellcheckrc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := runCmd(t, wf); status != 0 || count(out, "2086") != 0 {
		t.Errorf("config rcfile: status = %d: %s", status, out)
	}
	// args
	if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte("shellcheck:\n  args: [--exclude=SC2086]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := runCmd(t, wf); status != 0 || count(out, "2086") != 0 {
		t.Errorf("config args: status = %d: %s", status, out)
	}
	// An optional check: SC2250 asks for braces around variables
	if err := os.WriteFile(filepath.Join(root, ".github", "jactionlint.yaml"), []byte("shellcheck:\n  args: [--exclude=SC2086]\n  enable: [require-variable-braces]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if status, out := runCmd(t, wf); status != 1 || count(out, "2250") == 0 {
		t.Errorf("config enable: status = %d: %s", status, out)
	}
}
