package jactionlint

import (
	"strings"
	"testing"
)

func lintInvisible(t *testing.T, path, src string) []*Error {
	t.Helper()
	return errsWithID(lintFileWithConfig(t, ruleConfig("invisible-characters"), path, src), "invisible-characters")
}

func wfWithRun(run string) string {
	return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + run + "\n"
}

func TestInvisibleCharactersReported(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		line  int
		col   int
		place string
	}{
		{"zero width space in a run", wfWithRun("echo a\u200bb"), 6, 20, "a run: script"},
		{"bidi override in a run", wfWithRun("echo \u202eevil"), 6, 19, "a run: script"},
		{"isolate in a block run", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo hi\n          echo \u2066x\n", 8, 16, "a run: script"},
		{"word joiner in uses", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout\u2060@v4\n", 6, 31, "a uses: reference"},
		{"in an expression", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    if: github.actor == 'a\u200bb'\n    steps:\n      - run: echo\n", 5, 27, "a condition"},
		{"in an expression inside a run", wfWithRun("echo ${{ github.ref\u200b }}"), 6, 33, "an expression"},
		{"in a comment", "# v4\u202e\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", 1, 5, "a comment"},
		{"in an inline comment", wfWithRun("echo hi # ok\u200b"), 6, 26, "a comment"},
		{"in a name", "name: a\u200bb\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", 1, 8, `the value of "name"`},
		{"in a key", "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - na\u200bme: x\n        run: echo\n", 6, 11, "the key"},
		{"byte order mark in the middle", wfWithRun("echo a\ufeffb"), 6, 20, "a run: script"},
		{"variation selector after a latin letter", wfWithRun("echo a\ufe01"), 6, 20, "a run: script"},
		{"tag characters after a latin letter", wfWithRun("echo a\U000e0041\U000e007f"), 6, 20, "a run: script"},
		{"direction mark in latin text", wfWithRun("echo a\u200fb"), 6, 20, "a run: script"},
		{"joiner between latin letters", wfWithRun("echo a\u200db"), 6, 20, "a run: script"},
		{"escape character", wfWithRun("'echo \x1b[0m'"), 6, 20, "a run: script"},
		{"soft hyphen", wfWithRun("echo a\u00adb"), 6, 20, "a run: script"},
		{"hangul filler", wfWithRun("echo a\u3164b"), 6, 20, "a run: script"},
		{"line separator", "name: a\u2028b\non: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", 1, 8, `the value of "name"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := lintInvisible(t, "ci.yaml", tc.src)
			if len(errs) != 1 {
				t.Fatalf("want 1 error but got %d: %v", len(errs), errs)
			}
			e := errs[0]
			if e.Line != tc.line || e.Column != tc.col {
				t.Errorf("want %d:%d but got %d:%d (%s)", tc.line, tc.col, e.Line, e.Column, e.Message)
			}
			if !strings.Contains(e.Message, tc.place) {
				t.Errorf("want the place %q in %q", tc.place, e.Message)
			}
			if e.Severity != SeverityError {
				t.Errorf("want error severity but got %v", e.Severity)
			}
			if e.Fix == nil || e.Fix.Unsafe {
				t.Fatalf("want a safe fix but got %v", e.Fix)
			}
			fixed, remaining := fixAndLint(t, ruleConfig("invisible-characters"), "ci.yaml", tc.src, FixModeSafe)
			if got := errsWithID(remaining, "invisible-characters"); len(got) != 0 {
				t.Errorf("errors remain after the fix: %v", got)
			}
			for _, r := range fixed {
				if isInvisible(r) && r != '\t' {
					t.Errorf("the fixed source still has %U", r)
				}
			}
		})
	}
}

func TestInvisibleCharactersAllowed(t *testing.T) {
	tests := map[string]string{
		"ascii":                        wfWithRun("echo hi"),
		"byte order mark at the start": "\ufeff" + wfWithRun("echo hi"),
		"japanese":                     "# \u30c6\u30b9\u30c8\u3067\u3059\n" + wfWithRun("echo \u3053\u3093\u306b\u3061\u306f"),
		"chinese":                      wfWithRun("echo \u4f60\u597d\uff0c\u4e16\u754c"),
		"russian":                      wfWithRun("echo \u041f\u0440\u0438\u0432\u0435\u0442"),
		"accented":                     wfWithRun("echo caf\u00e9 e\u0301"),
		"hangul":                       wfWithRun("echo \ud55c\uae00"),
		"devanagari with a joiner":     wfWithRun("echo \u0915\u094d\u200d\u0937"),
		"persian with a zwnj":          wfWithRun("echo \u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645"),
		"hebrew with an rlm":           wfWithRun("echo \u05e9\u05dc\u05d5\u05dd\u200f!"),
		"arabic with an alm":           wfWithRun("echo \u0645\u0631\u062d\u0628\u0627\u061c!"),
		"arabic prefix format char":    wfWithRun("echo \u0600\u0661\u0662"),
		"emoji":                        wfWithRun("echo \U0001f680"),
		"emoji with a variation":       wfWithRun("echo \u2764\ufe0f \u2714\ufe0f"),
		"emoji zwj family":             wfWithRun("echo \U0001f468\u200d\U0001f469\u200d\U0001f467"),
		"emoji zwj with skin tone":     wfWithRun("echo \U0001f9d1\U0001f3fd\u200d\U0001f4bb"),
		"emoji zwj with a variation":   wfWithRun("echo \u2764\ufe0f\u200d\U0001f525"),
		"information source":           wfWithRun("echo \u2139\ufe0f \u2194\ufe0f \u00a9\ufe0f"),
		"keycap":                       wfWithRun("echo 1\ufe0f\u20e3"),
		"subdivision flag":             wfWithRun("echo \U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f"),
		"cjk variation sequence":       wfWithRun("echo \u845b\U000e0100"),
		"mongolian":                    wfWithRun("echo \u1820\u180b\u1821"),
		"hash keycap":                  wfWithRun("echo #\ufe0f\u20e3 *\ufe0f\u20e3"),
		"heart with text selector":     wfWithRun("echo \u2764\ufe0e \U0001f600\ufe0f"),
		"tab and crlf":                 strings.ReplaceAll(wfWithRun("echo\thi"), "\n", "\r\n"),
		"invalid utf-8":                wfWithRun("echo \xff\xfe"),
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if errs := lintInvisible(t, "ci.yaml", src); len(errs) != 0 {
				t.Errorf("want no error but got %v", errs)
			}
		})
	}
}

func TestInvisibleCharactersGrouping(t *testing.T) {
	// Adjacent characters are one finding with one fix
	errs := lintInvisible(t, "ci.yaml", wfWithRun("echo a\u200b\u200b\u202eb"))
	if len(errs) != 1 {
		t.Fatalf("want 1 error but got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "3 invisible characters") || errs[0].EndColumn != errs[0].Column+3 {
		t.Errorf("unexpected error: %+v", errs[0])
	}
	if got := errs[0].Fix.Edits; len(got) != 1 || got[0].End-got[0].Start != 9 {
		t.Errorf("unexpected edits: %v", got)
	}
	// A finding on every line
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo a\u200b\n          echo b\u200b\n"
	errs = lintInvisible(t, "ci.yaml", src)
	if len(errs) != 2 || errs[0].Line != 7 || errs[1].Line != 8 {
		t.Errorf("unexpected errors: %v", errs)
	}
	// Many characters are summarized
	errs = lintInvisible(t, "ci.yaml", wfWithRun("echo \u200b\u200b\u200b\u200b\u200b"))
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "and 2 more") {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestInvisibleCharactersCRLFAndPositions(t *testing.T) {
	src := strings.ReplaceAll(wfWithRun("echo \u00e9\u200b"), "\n", "\r\n")
	errs := lintInvisible(t, "ci.yaml", src)
	if len(errs) != 1 || errs[0].Line != 6 || errs[0].Column != 20 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	fixed, _ := fixAndLint(t, ruleConfig("invisible-characters"), "ci.yaml", src, FixModeSafe)
	if strings.ContainsRune(fixed, '\u200b') || !strings.Contains(fixed, "\u00e9\r\n") {
		t.Errorf("unexpected fixed source %q", fixed)
	}
}

func TestInvisibleCharactersBOMTwice(t *testing.T) {
	errs := lintInvisible(t, "ci.yaml", "\ufeff\ufeff"+wfWithRun("echo"))
	if len(errs) != 1 || errs[0].Line != 1 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestInvisibleCharactersOtherFiles(t *testing.T) {
	// A file which does not parse is still checked
	errs := lintInvisible(t, "ci.yaml", "on: [push\n# \u202e\n")
	if len(errs) != 1 || errs[0].Line != 2 {
		t.Errorf("unexpected errors: %v", errs)
	}
	// Dependabot configurations
	dep := "version: 2\nupdates:\n  - package-ecosystem: \"github-actions\u200b\"\n    directory: \"/\"\n    schedule:\n      interval: weekly\n"
	errs = lintInvisible(t, ".github/dependabot.yml", dep)
	if len(errs) != 1 || errs[0].Line != 3 {
		t.Errorf("unexpected errors: %v", errs)
	}
	// The rule can be turned off, and an inline ignore works
	if got := errsWithID(lintFileWithConfig(t, mustParseConfig(t, "rules:\n  invisible-characters: off\n"), "ci.yaml", wfWithRun("echo \u200b")), "invisible-characters"); len(got) != 0 {
		t.Errorf("the rule is off but reported %v", got)
	}
	// It is on by default
	l := lintFileWithConfig(t, defaultProfileConfig(), "ci.yaml", wfWithRun("echo \u200b"))
	if len(errsWithID(l, "invisible-characters")) != 1 {
		t.Errorf("want a finding with the default configuration: %v", l)
	}
	// An inline ignore comment above silences the finding
	ign := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      # jactionlint ignore=invisible-characters\n      - run: echo \u200b\n"
	if got := lintInvisible(t, "ci.yaml", ign); len(got) != 0 {
		t.Errorf("want the finding ignored but got %v", got)
	}
}

// A variation selector next to a character that is no emoji hides in a script: the backtick and the caret
// are modifier symbols, the digits, "#" and "*" take the selector only in a keycap sequence.
func TestInvisibleCharactersVariationSelectorAfterNoEmoji(t *testing.T) {
	for name, text := range map[string]string{
		"backtick and VS16": "echo `\ufe0f",
		"caret and VS15":    "echo ^\ufe0e",
		"digit and VS16":    "echo 5\ufe0f",
		"hash and VS16":     "echo #\ufe0f",
		"star and VS16":     "echo *\ufe0f",
		"diaeresis":         "echo \u00a8\ufe0f",
		"letter and ZWJ":    "echo a\u200d\U0001f525",
	} {
		t.Run(name, func(t *testing.T) {
			if errs := lintInvisible(t, "ci.yaml", wfWithRun(text)); len(errs) == 0 {
				t.Error("the invisible character must be reported")
			}
		})
	}
}
