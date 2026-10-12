package jactionlint

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The invariants pinned here, over every workflow in testdata/fix (real workflows with fixable findings):
//
//  1. A run that only reports (text, oneline, json, jsonl, sarif, summary, and --diff, which plans
//     the fixes) never touches the source: not its bytes, its mtime, or the files next to it. It
//     does not matter whether the file has LF or CRLF line endings, or a UTF-8 BOM.
//  2. Applying the fixes keeps every `uses:` line, with the SHA/version comment after it, and every
//     comment line. LF files stay LF, CRLF files stay CRLF, and a BOM stays the first three bytes.

// sourceVariants returns the line-ending and BOM variants of a fixture.
func sourceVariants(src string) map[string]string {
	lf := strings.ReplaceAll(src, "\r\n", "\n")
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	const bom = "\xef\xbb\xbf"
	return map[string]string{"lf": lf, "crlf": crlf, "bom-lf": bom + lf, "bom-crlf": bom + crlf}
}

func fixtureWorkflows(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "fix", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	files = slices.DeleteFunc(files, func(f string) bool {
		return strings.HasSuffix(f, ".fixed.yaml") || strings.HasSuffix(f, ".unsafe.yaml")
	})
	if len(files) < 30 {
		t.Fatalf("testdata/fix must have the workflows but got %d", len(files))
	}
	return files
}

func TestReportingNeverWritesSourcesForEveryFixture(t *testing.T) {
	outputs := [][]string{
		nil,
		{"--format", "oneline"},
		{"--format", "json"},
		{"--format", "jsonl"},
		{"--format", "sarif"},
		{"--format", "summary"},
		{"--diff"},
		{"--fix=unsafe", "--diff"},
		{"--diff", "--format", "sarif"},
	}
	for _, f := range fixtureWorkflows(t) {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		variants := sourceVariants(string(raw))
		// The extreme variants cover the other two.
		for _, vname := range []string{"lf", "bom-crlf"} {
			content := variants[vname]
			t.Run(filepath.Base(f)+"/"+vname, func(t *testing.T) {
				dir, path := writeWorkflow(t, content)
				before := dirSnapshot(t, dir)
				for _, args := range outputs {
					// 0 is a clean run and 1 a run with findings; 2 and up is a command that failed before it linted anything
					code, stdout, stderr := runRegression(t, append(slices.Clone(args), path)...)
					if code != ExitStatusSuccessNoProblem && code != ExitStatusSuccessProblemFound {
						t.Fatalf("%v: exit status %d\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
					}
					after := dirSnapshot(t, dir)
					if len(after) != len(before) {
						t.Fatalf("%v: files created or removed: %d -> %d", args, len(before), len(after))
					}
					for p, want := range before {
						if after[p] != want {
							t.Fatalf("%v modified %s (bytes or mtime)", args, p)
						}
					}
				}
				if b, _ := os.ReadFile(path); string(b) != content {
					t.Fatalf("source bytes changed")
				}
			})
		}
	}
}

// ignoreDirectiveLine matches a comment-only line which is an ignore directive: `# jactionlint ignore=`, its older
// spelling `# actionlint ignore=`, or `# zizmor: ignore[`.
var ignoreDirectiveLine = regexp.MustCompile(`^#\s*(j?actionlint\s+ignore=|zizmor:\s*ignore\[)`)

// keptLines are the lines of src that a fix must leave as they are: every uses: line (with its
// version comment) and every comment-only line except the ignore comments. The BOM is not part of the first line.
func keptLines(src string) []string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		line = strings.TrimPrefix(line, "\ufeff")
		trim := strings.TrimSpace(line)
		// unused-ignore removes an ignore directive which suppresses nothing: that is its fix. Ordinary comments
		// that merely contain the word, such as "the guard ignores the non-matching hour", stay checked.
		if ignoreDirectiveLine.MatchString(trim) {
			continue
		}
		if strings.Contains(trim, "uses:") || strings.HasPrefix(trim, "#") {
			kept = append(kept, line)
		}
	}
	return kept
}

func TestFixKeepsCommentsUsesAndLineEndingsForEveryFixture(t *testing.T) {
	cfg := fixerConfig(t, "")
	modes := map[string]FixMode{"safe": FixModeSafe, "unsafe": FixModeUnsafe}
	for _, f := range fixtureWorkflows(t) {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for vname, content := range sourceVariants(string(raw)) {
			for mname, mode := range modes {
				t.Run(filepath.Base(f)+"/"+vname+"/"+mname, func(t *testing.T) {
					out, _, _ := fixWith(t, []byte(content), cfg, mode)
					got := string(out)

					have := map[string]int{}
					for _, l := range strings.Split(strings.ReplaceAll(got, "\r\n", "\n"), "\n") {
						have[strings.TrimPrefix(l, "\ufeff")]++
					}
					for _, l := range keptLines(content) {
						if have[l] == 0 {
							t.Errorf("the fix lost or edited this line: %q", l)
						} else {
							have[l]--
						}
					}
					if strings.HasPrefix(content, "\xef\xbb\xbf") != strings.HasPrefix(got, "\xef\xbb\xbf") {
						t.Errorf("the BOM was added or removed")
					}
					lf := strings.Count(got, "\n")
					crlf := strings.Count(got, "\r\n")
					switch {
					case strings.Contains(content, "\r\n") && lf != crlf:
						t.Errorf("a CRLF file got %d bare LF line endings", lf-crlf)
					case !strings.Contains(content, "\r\n") && crlf != 0:
						t.Errorf("an LF file got %d CRLF line endings", crlf)
					}
				})
			}
		}
	}
}

// --migrate-ignores rewrites sources, so it has to keep what it does not migrate: the version comment
// on the uses: line, other comments, the BOM and the line endings of every variant.
func TestMigrateIgnoresKeepsSourceShapeInEveryVariant(t *testing.T) {
	base := "on: push\n" +
		"jobs:\n" +
		"  j:\n" +
		"    runs-on: ubuntu-latest\n" +
		"    timeout-minutes: 5\n" +
		"    steps:\n" +
		"      # unrelated\n" +
		"      - uses: actions/checkout@" + regressionSHA + " # v4.2.2 # zizmor: ignore[artipacked]\n" +
		"      - run: echo hi # plain trailing comment\n"
	for vname, content := range sourceVariants(base) {
		t.Run(vname, func(t *testing.T) {
			_, path := writeWorkflow(t, content)
			var out strings.Builder
			cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
			if code := cmd.Main([]string{"jactionlint", "--migrate-ignores", path}); code != 0 {
				t.Fatalf("exit %d: %s", code, out.String())
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(b)
			if strings.Contains(got, "zizmor") {
				t.Errorf("the zizmor comment should be migrated: %q", got)
			}
			if strings.HasPrefix(content, "\xef\xbb\xbf") != strings.HasPrefix(got, "\xef\xbb\xbf") {
				t.Errorf("the BOM was added or removed: %q", got)
			}
			eol := "\n"
			if strings.Contains(content, "\r\n") {
				eol = "\r\n"
			}
			for _, s := range []string{
				"uses: actions/checkout@" + regressionSHA + " # v4.2.2" + eol,
				"# unrelated" + eol,
				"# plain trailing comment" + eol,
				"# jactionlint ignore=artipacked" + eol,
			} {
				if !strings.Contains(got, s) {
					t.Errorf("result should contain %q: %q", s, got)
				}
			}
			if strings.Count(got, "\n") != strings.Count(got, eol) {
				t.Errorf("line endings changed: %q", got)
			}
		})
	}
}
