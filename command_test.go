package jactionlint

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandMain(t *testing.T) {
	var output bytes.Buffer

	// Create command instance populating stdin/stdout/stderr
	cmd := Command{
		Stdin:  os.Stdin,
		Stdout: &output,
		Stderr: &output,
	}

	// Run the command end-to-end. Note that given args should contain program name
	workflow := filepath.Join("testdata", "examples", "main.yaml")
	status := cmd.Main([]string{"jactionlint", "-shellcheck=", "-pyflakes=", "-ignore", `label .+ is unknown\.`, workflow})

	if status != 1 {
		t.Fatal("exit status should be 1 but got", status)
	}

	out := output.String()

	for _, s := range []string{
		"main.yaml:3:5:",
		"unexpected key \"branch\" for \"push\" section",
		"^~~~~~~~~~~~~~~",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("output should contain %q: %q", s, out)
		}
	}

	if strings.Contains(out, "[runner-label]") {
		t.Errorf("runner-label rule should be ignored by -ignore but it is included in output: %q", out)
	}
}

func TestCommandMigrateIgnores(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "w.yaml")
	src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      - run: echo ${{ github.event.issue.title }} # zizmor: ignore[template-injection] checked\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := Command{Stdin: os.Stdin, Stdout: &output, Stderr: &output}
	if status := cmd.Main([]string{"jactionlint", "-migrate-ignores", file}); status != 0 {
		t.Fatalf("exit status should be 0 but got %d: %s", status, output.String())
	}
	if !strings.Contains(output.String(), "migrated zizmor ignore comments: template-injection") {
		t.Errorf("unexpected output: %q", output.String())
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    timeout-minutes: 5\n    steps:\n      # checked\n      # jactionlint ignore=template-injection\n      - run: echo ${{ github.event.issue.title }}\n"
	if string(b) != want {
		t.Errorf("unexpected content: %q", b)
	}
	// The migrated file lints clean
	output.Reset()
	if status := cmd.Main([]string{"jactionlint", "-shellcheck=", "-pyflakes=", file}); status != 0 {
		t.Errorf("the migrated file must be clean but got %d: %s", status, output.String())
	}
}
