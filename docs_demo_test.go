package jactionlint

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The README and the home page of the site show the same demo: a broken workflow, what jactionlint reports for it and
// what --fix changes. The blocks are marked with an HTML comment on the line directly above the code fence:
//
//	<!-- demo:input -->               the workflow; it is saved as test.yaml
//	<!-- demo:output -->              the output of `jactionlint --no-color --no-hints test.yaml` (default profile)
//	<!-- demo:output-correctness -->  optional: the same with `--profile correctness`
//	<!-- demo:diff -->                optional: the standard output of the same command with `--diff`
//
// TestDocsDemoIsCurrent runs the command on the input and compares, so the demo cannot go stale. (It did: it claimed
// 7 errors, which is what the correctness profile reports, while a run without configuration reports more.) When it
// fails after a change of a rule, regenerate the output blocks of both files by running the command above in an empty
// Git repository. The demo avoids data that changes without a code change (the list of runner labels, the inputs of
// popular actions), so that this test does not fail whenever the generated data is refreshed. The home page also draws
// its picture of the findings from these blocks (docs/.vitepress/home.data.ts).
func TestDocsDemoIsCurrent(t *testing.T) {
	for _, tool := range []string{"shellcheck", "pyflakes"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed: the output of a run depends on it", tool)
		}
	}

	// The documented run has no configuration, so it uses the default profile; the other tests run with the correctness
	// profile (see TestMain)
	withDefaultProfile(t)

	files := []string{"README.md", filepath.Join("docs", "index.md")}
	var input string
	outputs := map[string]string{}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := strings.ReplaceAll(string(b), "\r\n", "\n") // a Windows checkout can have CRLF
		in, ok := demoBlock(src, "demo:input")
		if !ok {
			t.Fatalf("%s has no <!-- demo:input --> block", file)
		}
		if input == "" {
			input = in
		} else if in != input {
			t.Errorf("the demo input of %s differs from the one of %s", file, files[0])
		}
		for _, name := range []string{"demo:output", "demo:output-correctness", "demo:diff"} {
			out, ok := demoBlock(src, name)
			if !ok {
				if name == "demo:output" {
					t.Fatalf("%s has no <!-- demo:output --> block", file)
				}
				continue
			}
			if prev, seen := outputs[name]; seen && prev != out {
				t.Errorf("the %s block of %s differs from the one of %s", name, file, files[0])
			}
			outputs[name] = out
		}
	}

	for name, want := range outputs {
		args := []string{"--no-color", "--no-hints"}
		switch name {
		case "demo:output-correctness":
			args = append(args, "--profile", "correctness")
		case "demo:diff":
			args = append(args, "--diff") // the diff goes to stdout, the findings that are left to stderr
		}
		got := runDemo(t, input, args)
		if got != want {
			t.Errorf("the %s block of the README and the home page is out of date.\n--- documented\n%s\n--- jactionlint %s test.yaml\n%s",
				name, want, strings.Join(args, " "), got)
		}
	}
}

// demoBlock returns the content of the code fence that follows the line "<!-- marker -->".
func demoBlock(src, marker string) (string, bool) {
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "<!-- "+marker+" -->" {
			continue
		}
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "```") {
			return "", false
		}
		var b strings.Builder
		for _, c := range lines[i+2:] {
			if strings.HasPrefix(c, "```") {
				return b.String(), true
			}
			b.WriteString(c + "\n")
		}
	}
	return "", false
}

// runDemo saves src as test.yaml in an empty Git repository without any configuration and returns what jactionlint
// prints for it.
func runDemo(t *testing.T, src string, args []string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// A configuration file of the machine that runs the test must not change the output
	for _, env := range []string{"XDG_CONFIG_HOME", "HOME", "APPDATA"} {
		t.Setenv(env, t.TempDir())
	}
	t.Chdir(dir)

	var out, errOut bytes.Buffer
	cmd := &Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	cmd.Main(append(append([]string{"jactionlint"}, args...), "test.yaml"))
	if errOut.Len() > 0 {
		t.Logf("stderr: %s", errOut.String())
	}
	return out.String()
}
