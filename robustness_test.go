package jactionlint

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The inputs which the bug bash used to look for crashes and hangs. None of them is a valid workflow
// that a person would write; each is a shape which broke some parser or made some rule slow: huge
// files, deep nesting, aliases which expand, odd encodings and line breaks. Linting must finish with
// findings or an error, never a panic and never an endless run. The sizes are the ones of the bug
// bash, reduced where the size only made the run longer.

const robustBase = "name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"

func robustCases() map[string]func() []byte {
	rep := strings.Repeat
	b := func(s string) func() []byte { return func() []byte { return []byte(s) } }
	f := func(n int, format func(i int) string) string {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteString(format(i))
		}
		return sb.String()
	}
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	utf16 := func(s string) []byte {
		out := []byte{0xFF, 0xFE}
		for _, r := range s {
			out = append(out, byte(r), byte(r>>8))
		}
		return out
	}
	random := make([]byte, 5000)
	rand.New(rand.NewSource(1)).Read(random)

	return map[string]func() []byte{
		"empty":        b(""),
		"only newline": b("\n"),
		"binary":       func() []byte { return random },
		"nul bytes":    b(robustBase + "\x00\x00\x00"),
		"invalid utf8": b(strings.Replace(robustBase, "hi", "h\xff\xfe\xc3\x28i", 1)),
		"bom":          b("\xef\xbb\xbf" + robustBase),
		"bom only":     b("\xef\xbb\xbf"),
		"utf16le":      func() []byte { return utf16(robustBase) },
		"utf32le":      func() []byte { return append([]byte{0xFF, 0xFE, 0, 0}, []byte("n\x00\x00\x00")...) },
		"crlf":         b(crlf(robustBase)),
		"crlf multiline run": b(crlf("name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n" +
			"      - run: |\n          echo ${{ github.event.issue.title }}\n          ls | wc -l\n")),
		"cr only":            b(strings.ReplaceAll(robustBase, "\n", "\r")),
		"nel ls ps":          b(strings.Replace(robustBase, "echo hi", "echo \"a\u0085b\u2028c\u2029d\"", 1)),
		"nel in key":         b("name: t\u0085x\non: push\njobs:\n  a\u2028b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n"),
		"long line":          b(robustBase + "      - run: echo " + rep("a", 500_000) + "\n"),
		"long expression":    b(robustBase + "      - if: ${{ " + strings.TrimSuffix(rep("true && ", 20000), " && ") + " }}\n        run: x\n"),
		"deep expression":    b(robustBase + "      - if: ${{ " + rep("(", 20000) + "1" + rep(")", 20000) + " }}\n        run: x\n"),
		"deep flow nesting":  b("name: t\non: push\njobs: " + rep("[", 20000) + rep("]", 20000) + "\n"),
		"deep block nesting": b(f(1000, func(i int) string { return rep("  ", i) + fmt.Sprintf("k%d:\n", i) })),
		"anchor bomb": b("a: &a [x,x,x,x,x,x,x,x,x]\n" + f(15, func(i int) string {
			p, c := string(rune('a'+i)), string(rune('b'+i))
			return fmt.Sprintf("%s: &%s [%s]\n", c, c, strings.TrimSuffix(rep("*"+p+",", 9), ","))
		}) + "name: t\non: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: y\n    env: *q\n"),
		"merge keys": b("x: &x {a: 1}\n" + f(1000, func(i int) string {
			prev := "x"
			if i > 0 {
				prev = fmt.Sprintf("y%d", i-1)
			}
			return fmt.Sprintf("y%d: &y%d {<<: *%s, b%d: 1}\n", i, i, prev, i)
		}) + "name: t\non: push\njobs:\n  a:\n    runs-on: x\n    <<: *y999\n    steps:\n      - run: y\n"),
		"recursive alias": b("name: t\non: push\njobs:\n  a: &a\n    runs-on: x\n    self: *a\n    steps:\n      - run: y\n"),
		"recursive merge": b("a: &a\n  <<: *a\nname: t\non: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: y\n"),
		"many steps":      b(robustBase + f(8000, func(i int) string { return fmt.Sprintf("      - name: s%d\n        run: echo %d\n", i, i) })),
		"many steps with expressions": b(robustBase + f(3000, func(i int) string {
			return fmt.Sprintf("      - name: s%d\n        id: i%d\n        run: echo ${{ steps.i%d.outputs.x }}\n", i, i, max(0, i-1))
		})),
		"many jobs": b("name: t\non: push\njobs:\n" + f(1500, func(i int) string {
			needs := ""
			if i > 0 {
				needs = fmt.Sprintf("j%d", i-1)
			}
			return fmt.Sprintf("  j%d:\n    runs-on: ubuntu-latest\n    needs: [%s]\n    steps:\n      - run: echo hi\n", i, needs)
		})),
		"cyclic needs": b("name: t\non: push\njobs:\n" + f(1000, func(i int) string {
			return fmt.Sprintf("  j%d:\n    runs-on: ubuntu-latest\n    needs: [j%d]\n    steps:\n      - run: echo hi\n", i, (i+1)%1000)
		})),
		"huge matrix": b("name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n" + f(30, func(i int) string {
			var vs []string
			for j := 0; j < 50; j++ {
				vs = append(vs, fmt.Sprint(j))
			}
			return fmt.Sprintf("        k%d: [%s]\n", i, strings.Join(vs, ","))
		}) + "    steps:\n      - run: echo ${{ matrix.k29 }}\n"),
		"huge matrix include": b("name: t\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        include:\n" +
			f(15000, func(i int) string { return fmt.Sprintf("          - {a: %d, b: %d}\n", i, i) }) + "    steps:\n      - run: echo hi\n"),
		"long script":          b(robustBase + "      - run: |\n" + f(4000, func(i int) string { return fmt.Sprintf("          echo $x%d | grep a | wc -l\n", i) })),
		"emoji and rtl":        b("name: 🚀\non: push\njobs:\n  ünï:\n    name: 日本語 \u202e rtl\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo \u200b\u202e'hi'\n"),
		"tabs":                 b("name: t\non: push\njobs:\n\ta:\n\t\truns-on: x\n"),
		"duplicate keys":       b("name: t\nname: u\non: push\njobs:\n  a:\n    runs-on: x\n    runs-on: y\n    steps:\n      - run: y\n"),
		"multi document":       b(robustBase + "---\n" + robustBase),
		"null everywhere":      b("name:\non:\njobs:\n  a:\n    runs-on:\n    steps:\n      -\n      - null\n      - ~\n"),
		"weird tags":           b("name: !!binary aGk=\non: !!set {push}\njobs: !!omap\n  - a: {runs-on: x}\n"),
		"weird expressions":    b(robustBase + "      - run: ${{ '" + rep("'", 1001) + " }} ${{ ${{ }} }} ${{ fromJSON('" + rep("[", 1000) + "') }} ${{ \n"),
		"no trailing newline":  b(strings.TrimSuffix(robustBase, "\n")),
		"big integer":          b(robustBase + "      - timeout-minutes: 99999999999999999999999999999999\n        run: x\n"),
		"ignore comment flood": b(robustBase + rep("      # jactionlint ignore="+rep("x", 50)+"\n", 5000)),
		"ignore comment on every step": b(robustBase + f(5000, func(i int) string {
			return fmt.Sprintf("      - run: echo %d # jactionlint ignore=shellcheck\n", i)
		})),
		"zizmor comment on every step": b(robustBase + f(5000, func(int) string {
			return "      - uses: actions/checkout@v4 # zizmor: ignore[unpinned-uses]\n"
		})),
	}
}

// robustLint lints the source with the pedantic profile, the strictest, and prints it as SARIF, which
// computes the position of every fix. It fails the test when it takes more than the budget or panics.
func robustLint(t *testing.T, name string, src []byte, format string) {
	t.Helper()
	type result struct {
		panicked any
		err      error
	}
	done := make(chan result, 1)
	go func() {
		var r result
		defer func() {
			r.panicked = recover()
			done <- r
		}()
		l, err := NewLinter(io.Discard, &LinterOptions{Format: format})
		if err != nil {
			r.err = err
			return
		}
		l.defaultConfig = &Config{Profile: ProfilePedantic}
		_, r.err = l.Lint("w.yaml", src, nil)
	}()
	select {
	case r := <-done:
		if r.panicked != nil {
			t.Fatalf("%s: panic: %v", name, r.panicked)
		}
		// An error is fine: the file may not be workflow syntax at all
	case <-time.After(90 * time.Second):
		t.Fatalf("%s: linting did not finish within 90 s", name)
	}
}

func TestRobustness(t *testing.T) {
	for name, gen := range robustCases() {
		t.Run(strings.ReplaceAll(name, " ", "_"), func(t *testing.T) {
			t.Parallel()
			src := gen()
			if testing.Short() && len(src) > 500_000 {
				t.Skip("large input")
			}
			robustLint(t, name, src, FormatSARIF)
			if len(src) < 100_000 {
				robustLint(t, name, src, FormatText)
			}
		})
	}
}

// Files of a repository which are not workflows: the metadata of an action and a Dependabot configuration.
func TestRobustnessOfOtherFiles(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{".git", ".github/workflows"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		".github/workflows/w.yml": robustBase + "      - uses: ./\n",
		".github/dependabot.yml":  "version: 2\nupdates:\n  - package-ecosystem: [x]\n    directory: 5\n    schedule: {interval: [a]}\n    groups: ~\n",
		"action.yml":              "name: x\nruns:\n  using: composite\n  steps:\n    - run: ${{ \n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			done <- err
			return
		}
		l.defaultConfig = &Config{Profile: ProfilePedantic}
		_, _ = l.LintRepository(dir)
	}()
	select {
	case p := <-done:
		if p != nil {
			t.Fatalf("panic: %v", p)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("linting did not finish within 60 s")
	}
}

// A file in UTF-16 or UTF-32 gets one finding which names the encoding, not one for each NUL byte.
func TestNonUTF8EncodingIsOneFinding(t *testing.T) {
	cases := map[string][]byte{
		"UTF-16 (little endian)": append([]byte{0xFF, 0xFE}, bytes.Repeat([]byte("a\x00"), 50)...),
		"UTF-16 (big endian)":    append([]byte{0xFE, 0xFF}, bytes.Repeat([]byte("\x00a"), 50)...),
		"UTF-32 (little endian)": append([]byte{0xFF, 0xFE, 0, 0}, bytes.Repeat([]byte("a\x00\x00\x00"), 50)...),
		"UTF-32 (big endian)":    append([]byte{0, 0, 0xFE, 0xFF}, bytes.Repeat([]byte("\x00\x00\x00a"), 50)...),
	}
	for enc, src := range cases {
		errs := checkInvisibleCharacters(src, &Config{Profile: ProfileDefault})
		if len(errs) != 1 || !strings.Contains(errs[0].Message, enc) || errs[0].Line != 1 || errs[0].Fix != nil {
			t.Errorf("%s: want one finding without a fix, got %v", enc, errs)
		}
	}
	// A UTF-8 byte order mark is fine
	if errs := checkInvisibleCharacters([]byte("\xef\xbb\xbfname: t\n"), &Config{Profile: ProfileDefault}); len(errs) != 0 {
		t.Errorf("a UTF-8 BOM is not reported: %v", errs)
	}
}
