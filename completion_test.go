package jactionlint

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func completionScript(t *testing.T, shell string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	if status := cmd.Main([]string{"jactionlint", "--completion=" + shell}); status != 0 {
		t.Fatalf("--completion=%s: status %d: %s", shell, status, errOut.String())
	}
	return out.String()
}

func TestCompletionScriptsMentionEveryOptionAndRule(t *testing.T) {
	for _, shell := range completionShells {
		t.Run(shell, func(t *testing.T) {
			s := completionScript(t, shell)
			if s != completionScript(t, shell) {
				t.Error("the script is not deterministic")
			}
			for _, o := range cliOptions {
				if !strings.Contains(s, o.Long) {
					t.Errorf("the script does not mention --%s", o.Long)
				}
			}
			for _, id := range []string{"unpinned-uses", "missing-timeout", "template-injection"} {
				if !strings.Contains(s, id) {
					t.Errorf("the script does not complete the rule ID %s", id)
				}
			}
			for _, v := range []string{"correctness", "pedantic", "sarif", "jsonl", "unsafe", "cache,strict", "powershell"} {
				if !strings.Contains(s, v) {
					t.Errorf("the script does not complete the value %s", v)
				}
			}
		})
	}
	// every rule ID, not a sample
	s := completionScript(t, "bash")
	_, rest, _ := strings.Cut(s, "local rules='")
	list, _, _ := strings.Cut(rest, "'")
	var ids []string
	for _, r := range Rules() {
		ids = append(ids, r.ID)
	}
	if got := strings.Fields(list); !slices.Equal(got, ids) {
		t.Errorf("the bash script lists %d rule IDs, the registry has %d", len(got), len(ids))
	}
}

func TestCompletionValuesAreAccepted(t *testing.T) {
	for _, o := range completionOptions() {
		for _, v := range o.choices {
			var err error
			switch o.long {
			case "profile":
				_, err = ParseProfile(v)
			case "min-severity":
				_, err = ParseSeverity(v)
			case "format":
				_, err = newPrinter(v, false, false, nil)
			case "online":
				_, err = ParseOnlineMode(v)
			case "color":
				var f colorFlag
				err = f.Set(v)
			case "fix":
				var f fixFlag
				err = f.Set(v)
			case "completion":
				err = writeCompletion(&bytes.Buffer{}, v)
			}
			if err != nil {
				t.Errorf("--%s=%s is offered but not accepted: %v", o.long, v, err)
			}
		}
	}
}

func TestCompletionUnknownShell(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status := cmd.Main([]string{"jactionlint", "--completion=tcsh"})
	if status != ExitStatusInvalidCommandOption || !strings.Contains(errOut.String(), `invalid value "tcsh" for --completion`) || out.Len() != 0 {
		t.Errorf("status %d, stdout %q, stderr %q", status, out.String(), errOut.String())
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestCompletionScriptsParse checks the syntax of each script with the shell itself, when it is installed.
func TestCompletionScriptsParse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shells are not expected on Windows")
	}
	for _, c := range []struct {
		shell, file string
		bin         string
		args        func(string) []string
	}{
		{"bash", "c.bash", "bash", func(f string) []string { return []string{"-n", f} }},
		{"zsh", "c.zsh", "zsh", func(f string) []string { return []string{"-n", f} }},
		{"fish", "c.fish", "fish", func(f string) []string { return []string{"--no-execute", f} }},
	} {
		t.Run(c.shell, func(t *testing.T) {
			bin, err := exec.LookPath(c.bin)
			if err != nil {
				t.Skipf("%s is not installed", c.bin)
			}
			f := writeTemp(t, c.file, completionScript(t, c.shell))
			if out, err := exec.Command(bin, c.args(f)...).CombinedOutput(); err != nil {
				t.Errorf("%s does not accept the script: %v\n%s", c.bin, err, out)
			}
		})
	}
}

// TestBashCompletionBehavior drives the generated function with the variables bash sets for a completion.
func TestBashCompletionBehavior(t *testing.T) {
	bin, err := exec.LookPath("bash")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("bash is not available")
	}
	script := completionScript(t, "bash")
	driver := `
source "$1"
try() { COMP_CWORD=$1; shift; COMP_WORDS=("$@"); COMPREPLY=(); _jactionlint; echo "${COMPREPLY[*]}"; }
try 2 jactionlint --format ''
try 3 jactionlint --format = js
try 2 jactionlint --profile pe
try 2 jactionlint --ignore unpinned-u
try 2 jactionlint --fix-rules missing-timeout,unpinned-u
try 1 jactionlint --pro
try 1 jactionlint --col
try 2 jactionlint --completion ''
try 2 jactionlint --explain missing-t
try 2 jactionlint --min-severity ''
try 2 jactionlint --config-file nothing-here
`
	cmd := exec.Command(bin, "-c", driver, "bash", writeTemp(t, "c.bash", script))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	want := []string{
		"text oneline json jsonl sarif gcc github summary",
		"json jsonl",
		"pedantic",
		"unpinned-uses",
		"missing-timeout,unpinned-uses",
		"--profile",
		"--color=",
		"bash zsh fish powershell",
		"missing-timeout",
		"info warn error",
		"",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if !slices.Contains(strings.Fields(got[0]), "sarif") {
		t.Error("sarif must be offered for --format")
	}
}
