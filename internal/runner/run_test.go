package runner

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/jdx/jactionlint/v2/internal/sandbox"
)

var update = flag.Bool("update", false, "rewrite the golden transcripts")

// normalize removes what differs between machines from a transcript.
func normalize(s, repo, home string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if r, err := filepath.EvalSymlinks(repo); err == nil {
		s = strings.ReplaceAll(s, r, "<REPO>")
	}
	s = strings.ReplaceAll(s, repo, "<REPO>")
	s = strings.ReplaceAll(s, home, "<HOME>")
	return regexp.MustCompile(`<REPO>/\.jactionlint/run/[0-9-]+/jobs/\d+`).ReplaceAllString(s, "<RUN>")
}

func header(t *testing.T, src string) (args []string, exit int) {
	t.Helper()
	for _, line := range strings.Split(src, "\n") {
		switch {
		case strings.HasPrefix(line, "# args:"):
			args = strings.Fields(strings.TrimPrefix(line, "# args:"))
		case strings.HasPrefix(line, "# exit:"):
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "# exit:")))
			if err != nil {
				t.Fatal(err)
			}
			exit = n
		}
	}
	return args, exit
}

// TestGoldenTranscripts runs the workflows of testdata/run in the real sandbox and compares what is printed.
func TestGoldenTranscripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("steps do not run on Windows")
	}
	if err := sandbox.Available(sandbox.NetOpen); err != nil {
		t.Skipf("the sandbox is not available: %v", err)
	}
	files, _ := filepath.Glob("testdata/run/*.yml")
	if len(files) == 0 {
		t.Fatal("no workflows")
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yml")
		t.Run(name, func(t *testing.T) {
			srcB, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			src := string(srcB)
			args, wantExit := header(t, src)

			home, _ := os.UserHomeDir()
			secretFile := filepath.Join(home, ".jactionlint-golden-secret")
			if err := os.WriteFile(secretFile, []byte("secret"), 0o600); err != nil {
				t.Skipf("cannot write under the home directory: %v", err)
			}
			defer os.Remove(secretFile)
			t.Setenv("REAL_HOME_FILE", secretFile)
			t.Setenv("AWS_SECRET_ACCESS_KEY", "leaked-aws")
			t.Setenv("SSH_AUTH_SOCK", "/tmp/leaked-agent")
			t.Setenv("GITHUB_TOKEN", "leaked-token")
			t.Setenv("JACTIONLINT_CACHE_DIR", t.TempDir())

			// Not under /tmp: the sandbox lets steps write there, so a repository in it would be writable.
			base, err := os.MkdirTemp(home, ".jactionlint-golden-")
			if err != nil {
				t.Skipf("cannot create a directory under the home directory: %v", err)
			}
			t.Cleanup(func() { os.RemoveAll(base) })
			dir := filepath.Join(base, "repo")
			wf := filepath.Join(dir, ".github", "workflows")
			if err := os.MkdirAll(wf, 0o755); err != nil {
				t.Fatal(err)
			}
			os.Mkdir(filepath.Join(dir, ".git"), 0o755)
			os.WriteFile(filepath.Join(wf, name+".yml"), []byte(src), 0o644)
			os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644)
			os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
			os.WriteFile(filepath.Join(dir, "sub", ".keep"), nil, 0o644)

			var out, errOut bytes.Buffer
			code := Main(append([]string{"-C", dir, name, "--no-lint"}, args...), strings.NewReader(""), &out, &errOut)
			if code != wantExit {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, wantExit, out.String(), errOut.String())
			}
			got := normalize(out.String(), dir, home) + normalize(errOut.String(), dir, home)
			golden := strings.TrimSuffix(file, ".yml") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
				t.Fatalf("transcript differs\n--- got ---\n%s\n--- want ---\n%s", got, w)
			}
		})
	}
}

func runIn(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Main(append([]string{"-C", dir}, args...), strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func homeRepo(t *testing.T, workflow string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	base, err := os.MkdirTemp(home, ".jactionlint-test-")
	if err != nil {
		t.Skipf("cannot create a directory under the home directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	dir := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.Mkdir(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".github", "workflows", "w.yml"), []byte(workflow), 0o644)
	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original\n"), 0o644)
	t.Setenv("JACTIONLINT_CACHE_DIR", t.TempDir())
	return dir
}

func needSteps(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("steps do not run on Windows")
	}
}

func TestWorkspaceCopyAndInPlace(t *testing.T) {
	needSteps(t)
	if err := sandbox.Available(sandbox.NetOpen); err != nil {
		t.Skip(err)
	}
	dir := homeRepo(t, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo changed > tracked.txt && echo new > created.txt\n")
	if code, out, errOut := runIn(t, dir, "w", "--no-lint"); code != 0 {
		t.Fatalf("code %d\n%s\n%s", code, out, errOut)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "tracked.txt")); string(b) != "original\n" {
		t.Fatalf("the checkout was changed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "created.txt")); err == nil {
		t.Fatal("a file leaked into the checkout")
	}
	if _, err := os.Stat(filepath.Join(dir, ".jactionlint", "run")); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(filepath.Join(dir, ".jactionlint", "run")); len(ents) != 0 {
		t.Errorf("the run directory was not removed: %v", ents)
	}
	// --in-place works on the checkout itself
	if code, out, errOut := runIn(t, dir, "w", "--no-lint", "--in-place"); code != 0 {
		t.Fatalf("code %d\n%s\n%s", code, out, errOut)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "tracked.txt")); string(b) != "changed\n" {
		t.Fatalf("--in-place did not change the checkout: %q", b)
	}
	// --keep leaves the run directory
	if code, _, _ := runIn(t, dir, "w", "--no-lint", "--keep"); code != 0 {
		t.Fatal(code)
	}
	if ents, _ := os.ReadDir(filepath.Join(dir, ".jactionlint", "run")); len(ents) != 1 {
		t.Errorf("--keep: %v", ents)
	}
}

func TestSandboxOffAndNet(t *testing.T) {
	needSteps(t)
	dir := homeRepo(t, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          if echo x > \"$GITHUB_WORKSPACE/../../outside\"; then echo write-outside-allowed; fi\n")
	t.Setenv("JACTIONLINT_CACHE_DIR", t.TempDir())
	code, out, errOut := runIn(t, dir, "w", "--no-lint", "--sandbox=off")
	if code != 0 || !strings.Contains(out, "running without the sandbox") || !strings.Contains(out, "write-outside-allowed") {
		t.Fatalf("--sandbox=off: code %d\n%s\n%s", code, out, errOut)
	}
	if err := sandbox.Available(sandbox.NetOpen); err != nil {
		t.Skip(err)
	}
	// with the sandbox the same write is denied
	_, out, _ = runIn(t, dir, "w", "--no-lint")
	if strings.Contains(out, "write-outside-allowed") {
		t.Fatalf("the sandbox allowed a write outside the workspace:\n%s", out)
	}
	if code, _, errOut := runIn(t, dir, "w", "--no-lint", "--net=bogus"); code != ExitUsage || !strings.Contains(errOut, "--net must be") {
		t.Fatalf("code %d %q", code, errOut)
	}
}

func TestLintBeforeRun(t *testing.T) {
	needSteps(t)
	// ${{ matrix.nope }} is a type error that jactionlint reports and GitHub rejects
	dir := homeRepo(t, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ unknown.thing }}\n")
	code, out, errOut := runIn(t, dir, "w", "--sandbox=off")
	if code != ExitError || out != "" || !strings.Contains(errOut, `undefined variable "unknown"`) || !strings.Contains(errOut, "pass --no-lint") {
		t.Fatalf("lint did not stop the run: code %d\n%s\n%s", code, out, errOut)
	}
	code, out, _ = runIn(t, dir, "w", "--sandbox=off", "--no-lint")
	if code != 0 || !strings.Contains(out, "[step 1]") {
		t.Fatalf("--no-lint: code %d\n%s", code, out)
	}
}

func TestFailClosedWhenSandboxUnavailable(t *testing.T) {
	needSteps(t)
	if err := sandbox.Available(sandbox.NetOpen); err == nil {
		t.Skip("the sandbox is available here")
	}
	dir := homeRepo(t, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n")
	code, out, errOut := runIn(t, dir, "w", "--no-lint")
	if code != ExitError || out != "" || !strings.Contains(errOut, "nothing was run") || !strings.Contains(errOut, "--sandbox=off") {
		t.Fatalf("code %d\n%s\n%s", code, out, errOut)
	}
}
