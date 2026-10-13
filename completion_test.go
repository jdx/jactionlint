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
try 2 jactionlint --fix-rules missing-timeout,unpinned-uses,missing-t
try 2 jactionlint --fix-rules a,b,c,missing-t
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
		"missing-timeout,unpinned-uses,missing-timeout", // the middle elements stay
		"a,b,c,missing-timeout",
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

// TestZshSpecsAreBalanced checks the quotes of the specs of _arguments. Each spec is one single quoted word, so a quote
// inside it ends it early and the rest becomes words of their own (zsh -n does not see that).
func TestZshSpecsAreBalanced(t *testing.T) {
	script := completionScript(t, "zsh")
	_, body, _ := strings.Cut(script, "_arguments -s -S \\\n")
	body, _, _ = strings.Cut(body, "}\n")
	n := 0
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(line, " \\")
		n++
		if !strings.HasPrefix(line, "'") || !strings.HasSuffix(line, "'") {
			t.Errorf("a spec is not one quoted word: %s", line)
			continue
		}
		// A single quote of the text is written as '\'' (end, a quoted quote, start)
		inner := strings.ReplaceAll(line, `'\''`, "")
		if c := strings.Count(inner, "'"); c != 2 {
			t.Errorf("%d unescaped single quotes in the spec, want 2: %s", c, line)
		}
	}
	if n < len(cliOptions) {
		t.Errorf("%d specs for %d options", n, len(cliOptions))
	}
}

// TestZshArgumentsWords runs the zsh function with an _arguments which lists its words: every word is an option spec,
// so none of them was split by a quote.
func TestZshArgumentsWords(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("zsh is not available")
	}
	f := writeTemp(t, "c.zsh", completionScript(t, "zsh"))
	out, err := exec.Command(zsh, "-f", "-c", `_arguments() { print -rl -- "$@"; }; compdef() { :; }; source "$1"; _jactionlint`, "zsh", f).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	specs := 0
	for _, w := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		switch {
		case w == "-s" || w == "-S":
		case strings.HasPrefix(w, "-"), strings.HasPrefix(w, "(- *)"), strings.HasPrefix(w, "*:"):
			specs++
		default:
			t.Errorf("a stray word given to _arguments: %q", w)
		}
	}
	if specs < len(cliOptions) {
		t.Errorf("%d specs for %d options", specs, len(cliOptions))
	}
	if !strings.Contains(string(out), `:rules:{_values -s , "rule ID" $rules}`) {
		t.Errorf("the list of rule IDs is not completed as a comma separated list:\n%s", out)
	}
}

// TestPowerShellCompleterText checks what the PowerShell completer knows about the options.
func TestPowerShellCompleterText(t *testing.T) {
	s := completionScript(t, "powershell")
	for _, want := range []string{
		"Name = '--color'; Short = ''; Arg = 'WHEN'; Optional = $true;",
		"Name = '--fix'; Short = ''; Arg = 'unsafe'; Optional = $true;",
		"Name = '--fix-rules'; Short = ''; Arg = 'RULES'; Optional = $false; File = $false; List = $true;",
		"Name = '--config-file'; Short = '-c'; Arg = 'FILE'; Optional = $false; File = $true;",
		"CompleteFilename",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the PowerShell script does not contain %q", want)
		}
	}
}

// TestPowerShellCompletion drives the completer with TabExpansion2 when a working pwsh is available. The shim of a
// version manager does not run outside of its project, so JACTIONLINT_TEST_PWSH names the real one.
func TestPowerShellCompletion(t *testing.T) {
	name := os.Getenv("JACTIONLINT_TEST_PWSH")
	if name == "" {
		name = "pwsh"
	}
	pwsh, err := exec.LookPath(name)
	if err != nil {
		t.Skip("pwsh is not installed")
	}
	if err := exec.Command(pwsh, "-NoProfile", "-Command", "1").Run(); err != nil {
		t.Skipf("pwsh does not run: %v", err)
	}
	script := writeTemp(t, "c.ps1", completionScript(t, "powershell"))
	cases := []struct {
		line string
		want []string // some completion has this text
		not  []string // no completion has this text
	}{
		{"jactionlint --format ", []string{"sarif"}, nil},
		{"jactionlint --fix-rules missing-timeout,unpinned-u", []string{"unpinned-uses"}, nil},
		{"jactionlint --fix-rules=missing-timeout,unpinned-u", []string{"unpinned-uses"}, nil},
		{"jactionlint --fix-rules=a,missing-timeout,unpinned-u", []string{"unpinned-uses"}, nil},
		{"jactionlint --color=al", []string{"--color=always"}, nil},
		{"jactionlint --color ", nil, []string{"always", "never", "auto"}},
		{"jactionlint --fix ", nil, []string{"unsafe"}},
		{"jactionlint --fix=un", []string{"--fix=unsafe"}, nil},
		// A finished list is a complete word: the file names follow, not the values of the option
		{"jactionlint --fix-rules=missing-timeout,unpinned-uses ", []string{"./docs"}, []string{"missing-timeout", "unpinned-uses"}},
		{"jactionlint --online=cache,strict ", []string{"./docs"}, []string{"cache", "strict", "cache,strict"}},
		{"jactionlint --fix-rules missing-timeout,unpinned-uses ", []string{"./docs"}, []string{"missing-timeout"}},
		{"jactionlint --config-file=docs/us", []string{"--config-file=./docs/usage.md"}, nil},
		{"jactionlint --config-file docs/us", []string{"./docs/usage.md"}, nil},
		{"jactionlint docs/us", []string{"./docs/usage.md"}, nil},
	}
	var cmds strings.Builder
	cmds.WriteString(". $env:JACTIONLINT_TEST_PS1\n")
	for _, c := range cases {
		cmds.WriteString("$l = '" + c.line + "'; $r = TabExpansion2 -inputScript $l -cursorColumn $l.Length; '=== ' + $l; $r.CompletionMatches | ForEach-Object { $_.CompletionText }\n")
	}
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", cmds.String())
	cmd.Env = append(os.Environ(), "JACTIONLINT_TEST_PS1="+script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	results := map[string][]string{}
	var cur string
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(l, "=== ") {
			cur = strings.TrimPrefix(l, "=== ")
			continue
		}
		if l != "" {
			results[cur] = append(results[cur], l)
		}
	}
	for _, c := range cases {
		got := results[c.line]
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("%q: no completion %q in %v", c.line, w, got[:min(len(got), 5)])
			}
		}
		for _, n := range c.not {
			if slices.Contains(got, n) {
				t.Errorf("%q: completion %q must not be offered (%v)", c.line, n, got[:min(len(got), 5)])
			}
		}
	}
}
