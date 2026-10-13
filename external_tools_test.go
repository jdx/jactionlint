package jactionlint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// toolProject creates a project with one workflow that has a bash script and a Python script, and returns the
// path of the workflow.
func toolProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	wd := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `on: push
permissions: {}
concurrency:
  group: ci
  cancel-in-progress: true
jobs:
  j:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - run: echo one
      - run: echo two
        shell: bash
      - run: print("py")
        shell: python
`
	f := filepath.Join(wd, "ci.yaml")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// emptyPath makes the commands shellcheck and pyflakes unavailable, and returns the directory it put on PATH.
func emptyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("JACTIONLINT_NO_HINTS", "")
	return dir
}

func runToolCommand(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status = cmd.Main(append([]string{"jactionlint"}, args...))
	return status, out.String(), errOut.String()
}

func TestExplicitToolMustExist(t *testing.T) {
	f := toolProject(t)
	for _, tool := range []string{"shellcheck", "pyflakes"} {
		t.Run("missing "+tool, func(t *testing.T) {
			status, _, stderr := runToolCommand(t, "--"+tool+"=/nonexistent/dir/"+tool, f)
			if status != ExitStatusFailure {
				t.Fatalf("status = %d, want %d. stderr: %s", status, ExitStatusFailure, stderr)
			}
			for _, want := range []string{"--" + tool + "=/nonexistent/dir/" + tool, "not found or is not executable", "--" + tool + "= to turn"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr should contain %q: %q", want, stderr)
				}
			}
		})
	}

	t.Run("a command name which is not in PATH", func(t *testing.T) {
		emptyPath(t)
		status, _, stderr := runToolCommand(t, "--shellcheck=shellcheck", f)
		if status != ExitStatusFailure || !strings.Contains(stderr, "--shellcheck=shellcheck") {
			t.Errorf("status = %d, stderr: %s", status, stderr)
		}
	})

	t.Run("a file which is not executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no executable bit")
		}
		p := filepath.Join(t.TempDir(), "shellcheck")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status, _, stderr := runToolCommand(t, "--shellcheck="+p, f)
		if status != ExitStatusFailure || !strings.Contains(stderr, "not executable") {
			t.Errorf("status = %d, stderr: %s", status, stderr)
		}
	})

	t.Run("an empty command turns the tool off without a word", func(t *testing.T) {
		emptyPath(t)
		status, stdout, stderr := runToolCommand(t, "--shellcheck=", "--pyflakes=", f)
		if status != 0 || stderr != "" {
			t.Errorf("status = %d, stdout: %s, stderr: %s", status, stdout, stderr)
		}
	})
}

func TestMissingDefaultToolIsNoted(t *testing.T) {
	f := toolProject(t)

	t.Run("one note per tool and run, on stderr", func(t *testing.T) {
		emptyPath(t)
		status, stdout, stderr := runToolCommand(t, f)
		if status != 0 {
			t.Fatalf("a missing tool must not fail the run: status = %d, stdout: %s, stderr: %s", status, stdout, stderr)
		}
		for _, want := range []string{
			"note: shellcheck was not found, so it did not check 2 shell scripts. install it, or pass --shellcheck= to skip it on purpose\n",
			"note: pyflakes was not found, so it did not check 1 Python script. install it, or pass --pyflakes= to skip it on purpose\n",
		} {
			if strings.Count(stderr, want) != 1 {
				t.Errorf("stderr should contain %q once: %q", want, stderr)
			}
		}
		if strings.Contains(stdout, "was not found") {
			t.Errorf("the note belongs to stderr: %q", stdout)
		}
	})

	t.Run("silenced by --no-hints", func(t *testing.T) {
		emptyPath(t)
		status, _, stderr := runToolCommand(t, "--no-hints", f)
		if status != 0 || stderr != "" {
			t.Errorf("status = %d, stderr: %q", status, stderr)
		}
	})

	t.Run("silenced by JACTIONLINT_NO_HINTS", func(t *testing.T) {
		emptyPath(t)
		t.Setenv("JACTIONLINT_NO_HINTS", "1")
		if _, _, stderr := runToolCommand(t, f); stderr != "" {
			t.Errorf("stderr: %q", stderr)
		}
	})

	t.Run("turning only one tool off leaves the other note", func(t *testing.T) {
		emptyPath(t)
		_, _, stderr := runToolCommand(t, "--pyflakes=", f)
		if strings.Contains(stderr, "pyflakes") || !strings.Contains(stderr, "shellcheck was not found") {
			t.Errorf("stderr: %q", stderr)
		}
	})

	t.Run("no script, no note", func(t *testing.T) {
		emptyPath(t)
		g := filepath.Join(filepath.Dir(f), "uses.yaml")
		src := "on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v5.0.0\n        with:\n          persist-credentials: false\n"
		if err := os.WriteFile(g, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		status, stdout, stderr := runToolCommand(t, g)
		if status != 0 || stderr != "" {
			t.Errorf("status = %d, stdout: %s, stderr: %q", status, stdout, stderr)
		}
	})
}

type sarifToolProperties struct {
	Runs []struct {
		Invocations []struct {
			ToolConfigurationNotifications []struct {
				Message struct{ Text string } `json:"message"`
			} `json:"toolConfigurationNotifications"`
		} `json:"invocations"`
		Properties struct {
			ExternalTools map[string]ExternalToolStatus `json:"externalTools"`
		} `json:"properties"`
	} `json:"runs"`
}

func TestSARIFReportsExternalTools(t *testing.T) {
	f := toolProject(t)

	t.Run("not found", func(t *testing.T) {
		emptyPath(t)
		status, stdout, stderr := runToolCommand(t, "--format", "sarif", f)
		if status != 0 {
			t.Fatalf("status = %d: %s", status, stderr)
		}
		if stderr != "" {
			t.Errorf("a structured format keeps the log free of the note: %q", stderr)
		}
		var log sarifToolProperties
		if err := json.Unmarshal([]byte(stdout), &log); err != nil {
			t.Fatal(err)
		}
		got := log.Runs[0].Properties.ExternalTools
		want := map[string]ExternalToolStatus{
			"shellcheck": {Status: "not found", Command: "shellcheck", SkippedScripts: 2},
			"pyflakes":   {Status: "not found", Command: "pyflakes", SkippedScripts: 1},
		}
		if len(got) != len(want) || got["shellcheck"] != want["shellcheck"] || got["pyflakes"] != want["pyflakes"] {
			t.Errorf("externalTools = %+v, want %+v", got, want)
		}
		var notes []string
		for _, inv := range log.Runs[0].Invocations {
			for _, n := range inv.ToolConfigurationNotifications {
				notes = append(notes, n.Message.Text)
			}
		}
		if len(notes) != 2 || !strings.Contains(notes[0]+notes[1], "shellcheck was not found") {
			t.Errorf("notifications = %q", notes)
		}
	})

	t.Run("ran", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the fake commands are shell scripts")
		}
		dir := emptyPath(t)
		for _, name := range []string{"shellcheck", "pyflakes"} {
			script := "#!/bin/sh\ncat >/dev/null\necho '[]'\n"
			if name == "pyflakes" {
				script = "#!/bin/sh\ncat >/dev/null\n"
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		status, stdout, stderr := runToolCommand(t, "--format", "sarif", f)
		if status != 0 || stderr != "" {
			t.Fatalf("status = %d: %s", status, stderr)
		}
		var log sarifToolProperties
		if err := json.Unmarshal([]byte(stdout), &log); err != nil {
			t.Fatal(err)
		}
		got := log.Runs[0].Properties.ExternalTools
		if g := got["shellcheck"]; g.Status != "ran" || g.Scripts != 2 || g.SkippedScripts != 0 {
			t.Errorf("shellcheck = %+v", g)
		}
		if g := got["pyflakes"]; g.Status != "ran" || g.Scripts != 1 {
			t.Errorf("pyflakes = %+v", g)
		}
		for _, inv := range log.Runs[0].Invocations {
			if len(inv.ToolConfigurationNotifications) > 0 {
				t.Errorf("a tool which ran needs no notification: %+v", inv)
			}
		}
	})

	t.Run("turned off on purpose", func(t *testing.T) {
		emptyPath(t)
		status, stdout, _ := runToolCommand(t, "--format", "sarif", "--shellcheck=", "--pyflakes=", f)
		if status != 0 {
			t.Fatalf("status = %d", status)
		}
		if strings.Contains(stdout, "externalTools") {
			t.Errorf("a tool that was turned off is not listed: %s", stdout)
		}
	})
}

func TestExternalToolsOfTheLibrary(t *testing.T) {
	f := toolProject(t)
	emptyPath(t)
	// The library default of Shellcheck is empty: no integration, nothing to note, nothing to fail
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.LintFile(f, nil); err != nil {
		t.Fatal(err)
	}
	if s := l.ExternalTools(); s != nil {
		t.Errorf("ExternalTools() = %+v", s)
	}

	// An option that is not required only skips: the library gives the command and the linter notes it
	l, err = NewLinter(&bytes.Buffer{}, &LinterOptions{Shellcheck: "shellcheck", Pyflakes: "pyflakes"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.LintFile(f, nil); err != nil {
		t.Fatal(err)
	}
	if s := l.ExternalTools()["shellcheck"]; s.Status != ExternalToolNotFound || s.SkippedScripts != 2 {
		t.Errorf("shellcheck = %+v", s)
	}

	// Required: fail in NewLinter
	if _, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Shellcheck: "shellcheck", ShellcheckRequired: true}); err == nil {
		t.Error("a required shellcheck which is missing must fail NewLinter")
	}
	if _, err := NewLinter(&bytes.Buffer{}, &LinterOptions{ShellcheckRequired: true}); err != nil {
		t.Errorf("an empty command is off: %v", err)
	}
}

// TestToolsNobodyNeededAreNotListed: the status of a tool is about the scripts which needed it, like the note is.
func TestToolsNobodyNeededAreNotListed(t *testing.T) {
	emptyPath(t)
	root := t.TempDir()
	for _, d := range []string{filepath.Join(root, ".git"), filepath.Join(root, ".github", "workflows")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	usesOnly := "on: push\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8\n"
	pythonOnly := "on: push\npermissions: {}\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: print(1)\n        shell: python\n"
	files := map[string]string{"uses.yaml": usesOnly, "python.yaml": pythonOnly}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(root, ".github", "workflows", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	lint := func(file string) map[string]ExternalToolStatus {
		l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Shellcheck: "shellcheck", Pyflakes: "pyflakes"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.LintFile(filepath.Join(root, ".github", "workflows", file), nil); err != nil {
			t.Fatal(err)
		}
		return l.ExternalTools()
	}
	if got := lint("uses.yaml"); got != nil {
		t.Errorf("a workflow of uses: steps needs no tool, got %+v", got)
	}
	got := lint("python.yaml")
	if _, ok := got["shellcheck"]; ok || got["pyflakes"].Status != ExternalToolNotFound || got["pyflakes"].SkippedScripts != 1 {
		t.Errorf("only pyflakes was needed: %+v", got)
	}

	// The SARIF log has no property for the first and the second only has pyflakes
	for file, wantProp := range map[string]bool{"uses.yaml": false, "python.yaml": true} {
		var out, errOut bytes.Buffer
		cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
		cmd.Main([]string{"jactionlint", "--format", "sarif", filepath.Join(root, ".github", "workflows", file)})
		has := strings.Contains(out.String(), `"externalTools"`)
		if has != wantProp {
			t.Errorf("%s: externalTools in the SARIF log = %v, want %v\n%s", file, has, wantProp, out.String())
		}
		if wantProp && strings.Contains(out.String(), `"shellcheck": {`) {
			t.Errorf("%s: shellcheck was not needed: %s", file, out.String())
		}
	}
}

// TestToolScriptsAreCountedOncePerFile: --fix lints a file again for each pass, and --diff does too, but the scripts
// of a file count once per run, like in a plain run.
func TestToolScriptsAreCountedOncePerFile(t *testing.T) {
	emptyPath(t)
	root := t.TempDir()
	for _, d := range []string{filepath.Join(root, ".git"), filepath.Join(root, ".github", "workflows")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// artipacked has a safe fix, so the file is linted again after the pass. Three scripts: two bash and one python
	src := "on: push\npermissions: {}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n" +
		"      - uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8\n" +
		"      - run: echo one\n      - run: echo two\n        shell: bash\n      - run: print(1)\n        shell: python\n"
	f := filepath.Join(root, ".github", "workflows", "ci.yaml")
	write := func() {
		if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	newLinter := func() *Linter {
		l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{Shellcheck: "shellcheck", Pyflakes: "pyflakes", Profile: ProfileDefault})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}

	write()
	plain := newLinter()
	if _, err := plain.LintFile(f, nil); err != nil {
		t.Fatal(err)
	}
	want := plain.ExternalTools()
	if want["shellcheck"].SkippedScripts != 2 || want["pyflakes"].SkippedScripts != 1 {
		t.Fatalf("the plain run: %+v", want)
	}

	write()
	fixer := newLinter()
	res, err := fixer.FixFilesWithOptions([]string{f}, nil, FixOptions{Mode: FixModeSafe})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied == 0 {
		t.Fatalf("the file must have been fixed, so that it was linted again: %+v", res)
	}
	if got := fixer.ExternalTools(); !reflect.DeepEqual(got, want) {
		t.Errorf("--fix counted %+v, a plain run %+v", got, want)
	}

	write()
	diff := newLinter()
	if _, err := diff.FixFilesWithOptions([]string{f}, nil, FixOptions{Mode: FixModeSafe, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if got := diff.ExternalTools(); !reflect.DeepEqual(got, want) {
		t.Errorf("--diff counted %+v, a plain run %+v", got, want)
	}
	if n := fixer.tools.notes(); len(n) != 2 || !strings.Contains(n[0], "did not check 2 shell scripts") || !strings.Contains(n[1], "did not check 1 Python script.") {
		t.Errorf("the notes: %q", n)
	}
}
