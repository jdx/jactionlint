//go:build !js

package jactionlint

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/sys/execabs"
)

// actionlintExceptions are the expected errors of the original fixtures of actionlint that the correctness profile
// does not report the same way, with the reason. Nothing is in it because it was hard: every entry is a decision.
// The key is the fixture, the value maps a prefix of the expected line to the reason.
var actionlintExceptions = map[string]map[string]string{}

// TestCorrectnessProfileKeepsEveryErrorOfActionlint runs the fixtures that came from rhysd/actionlint (listed in
// testdata/actionlint_originals.txt: the files of testdata/err and testdata/examples that were not added by the
// jactionlint project) with `profile: correctness` and no other setting, and asserts that every error that their
// .out file expects is still reported. The profile may report more (the bug detectors of jactionlint), so errors
// that are not expected are not looked at. This is the promise of "Coming from actionlint" in docs/actionlint.md.
func TestCorrectnessProfileKeepsEveryErrorOfActionlint(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "actionlint_originals.txt"))
	if err != nil {
		t.Fatal(err)
	}
	shellcheck, pyflakes := "", ""
	if p, err := execabs.LookPath("shellcheck"); err == nil {
		shellcheck = p
	}
	if p, err := execabs.LookPath("pyflakes"); err == nil {
		pyflakes = p
	}
	var files []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	if len(files) < 100 {
		t.Fatalf("the list of the original fixtures is too short: %d", len(files))
	}
	proj := func(f string) *Project { return &Project{root: filepath.Dir(f)} }
	for _, f := range files {
		name := filepath.ToSlash(f)
		t.Run(strings.TrimSuffix(strings.TrimPrefix(name, "testdata/"), ".yaml"), func(t *testing.T) {
			base := strings.TrimSuffix(f, ".yaml")
			o := LinterOptions{}
			if strings.Contains(base, "shellcheck") {
				if shellcheck == "" {
					t.Skip("shellcheck is not installed")
				}
				o.Shellcheck = shellcheck
			}
			if strings.Contains(base, "pyflakes") {
				if pyflakes == "" {
					t.Skip("pyflakes is not installed")
				}
				o.Pyflakes = pyflakes
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			l, err := NewLinter(io.Discard, &o)
			if err != nil {
				t.Fatal(err)
			}
			cfg := &Config{Profile: ProfileCorrectness}
			// What an actionlint config of that fixture says (labels, variables) is not a profile setting
			if c := fixtureConfigFile(t, base+".config"); c != nil {
				cfg = c
				cfg.Profile = ProfileCorrectness
			}
			l.defaultConfig = cfg
			errs, err := l.Lint("test.yaml", src, proj(f))
			if err != nil {
				t.Fatal(err)
			}
			have := map[string]bool{}
			var haveLines []string
			for _, e := range errs {
				s := e.Error()
				have[s] = true
				haveLines = append(haveLines, s)
			}

			out, err := os.Open(base + ".out")
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			sc := bufio.NewScanner(out)
			for sc.Scan() {
				want := sc.Text()
				if want == "" {
					continue
				}
				if reason := exceptionFor(name, want); reason != "" {
					continue
				}
				if strings.HasPrefix(want, "/") && strings.HasSuffix(want, "/") {
					re := regexp.MustCompile(want[1 : len(want)-1])
					found := false
					for _, h := range haveLines {
						if re.MatchString(h) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("the correctness profile does not report what actionlint does\n  want: /%s/\n  have: %q", re, haveLines)
					}
					continue
				}
				if !have[want] {
					t.Errorf("the correctness profile does not report what actionlint does\n  want: %q\n  have: %q", want, haveLines)
				}
			}
		})
	}
}

func exceptionFor(fixture, line string) string {
	for prefix, reason := range actionlintExceptions[fixture] {
		if strings.HasPrefix(line, prefix) {
			return reason
		}
	}
	return ""
}
