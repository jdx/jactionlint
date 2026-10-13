package jactionlint

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func ruffProject(t *testing.T, script string) string {
	t.Helper()
	root := t.TempDir()
	wd := filepath.Join(root, ".github", "workflows")
	for _, d := range []string{wd, filepath.Join(root, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := "on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: |\n" + ruffIndent(script, "          ") + "        shell: python\n"
	f := filepath.Join(wd, "ci.yaml")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func ruffIndent(s, ind string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		b.WriteString(ind + l + "\n")
	}
	return b.String()
}

func runRuffCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status := cmd.Main(append([]string{"jactionlint", "--profile", "default", "--shellcheck="}, args...))
	return status, out.String(), errOut.String()
}

func TestRuffMapsItsDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ruff is a shell script")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	json := `[{"code":"F821","message":"Undefined name ` + "`foo`" + `","location":{"row":2,"column":7},"end_location":{"row":2,"column":10}},` +
		`{"code":"F401","message":"` + "`os`" + ` imported but unused","location":{"row":1,"column":8},"end_location":{"row":1,"column":10}}]`
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat is not found")
	}
	script := "#!/bin/sh\n" + cat + " > " + argsFile + ".stdin\nprintf '%s\\n' \"$*\" > " + argsFile + "\nprintf '%s\\n' '" + json + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ruff"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	f := ruffProject(t, "import os\nprint(foo)\n")
	t.Setenv("PATH", dir)

	status, out, stderr := runRuffCommand(t, "--ruff=ruff", "--pyflakes=", f)
	if status != 1 {
		t.Fatalf("status %d: %s %s", status, out, stderr)
	}
	for _, want := range []string{
		"ruff reported issue in this script: 2:7: F821 Undefined name `foo` [pyflakes]",
		"ruff reported issue in this script: 1:8: F401 `os` imported but unused [pyflakes]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not contain %q:\n%s", want, out)
		}
	}
	args, _ := os.ReadFile(argsFile)
	if got := strings.TrimSpace(string(args)); got != "check --isolated --no-cache --select F --output-format json -" {
		t.Errorf("arguments of ruff: %q", got)
	}
	stdin, _ := os.ReadFile(argsFile + ".stdin")
	if string(stdin) != "import os\nprint(foo)\n" {
		t.Errorf("the script on stdin: %q", stdin)
	}
}

func TestRuffReplacesPyflakes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake commands are shell scripts")
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat is not found")
	}
	dir := t.TempDir()
	// pyflakes would report; ruff finds nothing: only ruff may run
	if err := os.WriteFile(filepath.Join(dir, "pyflakes"), []byte("#!/bin/sh\n"+cat+" >/dev/null\necho '<stdin>:1:1: from pyflakes'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ruff"), []byte("#!/bin/sh\n"+cat+" >/dev/null\necho '[]'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	f := ruffProject(t, "print(1)\n")
	if status, out, _ := runRuffCommand(t, f); status != 1 || !strings.Contains(out, "pyflakes reported issue") {
		t.Fatalf("without --ruff pyflakes runs: %d %s", status, out)
	}
	if status, out, stderr := runRuffCommand(t, "--ruff=ruff", f); status != 0 {
		t.Errorf("with --ruff pyflakes must not run: %d %s %s", status, out, stderr)
	}
}

func TestRuffMustExist(t *testing.T) {
	f := ruffProject(t, "print(1)\n")
	status, _, stderr := runRuffCommand(t, "--ruff=/nonexistent/ruff", f)
	if status != ExitStatusFailure || !strings.Contains(stderr, "--ruff=/nonexistent/ruff: the command was not found") {
		t.Errorf("status %d: %s", status, stderr)
	}
}

func TestRuffBrokenOutputIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ruff is a shell script")
	}
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat is not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ruff"), []byte("#!/bin/sh\n"+cat+" >/dev/null\necho 'not json'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	f := ruffProject(t, "print(1)\n")
	status, _, stderr := runRuffCommand(t, "--ruff=ruff", f)
	if status != ExitStatusFailure || !strings.Contains(stderr, "could not parse JSON output from ruff") {
		t.Errorf("status %d: %s", status, stderr)
	}
}

// TestRuffReal runs the real ruff when there is one.
func TestRuffReal(t *testing.T) {
	name := os.Getenv("JACTIONLINT_TEST_RUFF") // a ruff which the shims of a version manager do not hide
	if name == "" {
		name = "ruff"
	}
	ruff, err := exec.LookPath(name)
	if err != nil {
		t.Skip("ruff is not installed")
	}
	if err := exec.Command(ruff, "--version").Run(); err != nil {
		t.Skipf("ruff does not run: %v", err)
	}
	f := ruffProject(t, "import os\nprint(foo)\n")
	status, out, stderr := runRuffCommand(t, "--ruff="+ruff, f)
	if status != 1 || !strings.Contains(out, "F821 Undefined name `foo`") || !strings.Contains(out, "F401 `os` imported but unused") {
		t.Errorf("status %d: %s %s", status, out, stderr)
	}
	f = ruffProject(t, "print(\n")
	status, out, _ = runRuffCommand(t, "--ruff="+ruff, f)
	if status != 1 || !strings.Contains(out, "ruff reported issue in this script: ") || strings.Contains(out, "invalid-syntax") {
		t.Errorf("a syntax error: status %d: %s", status, out)
	}
	f = ruffProject(t, "x = 1\nprint(x)\n")
	if status, out, _ := runRuffCommand(t, "--ruff="+ruff, f); status != 0 {
		t.Errorf("a clean script: status %d: %s", status, out)
	}
}
