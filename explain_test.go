package jactionlint

import (
	"bytes"
	"strings"
	"testing"
)

func explain(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd := Command{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}
	status := cmd.Main(append([]string{"jactionlint"}, args...))
	return status, out.String(), errOut.String()
}

func TestExplain(t *testing.T) {
	status, out, stderr := explain(t, "--explain", "missing-timeout")
	if status != 0 || stderr != "" {
		t.Fatalf("status %d: %s", status, stderr)
	}
	for _, want := range []string{
		"missing-timeout\n===============\n",
		"A job does not set timeout-minutes.",
		"Group:          policy",
		"Profile:        default (so also pedantic)",
		"Default level:  error",
		"Fix:            yes.",
		"https://jactionlint.jdx.dev/rules#missing-timeout",
		"https://jactionlint.jdx.dev/checks#check-timeout-minutes",
		"default-minutes (int, no default)",
		"# jactionlint ignore=missing-timeout",
		"## Timeout minutes of jobs",
		"Example input:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<!--") || strings.Contains(out, `<a id=`) {
		t.Errorf("the markers of the document are in the output:\n%s", out)
	}
	if strings.Contains(out, "Details and examples") || strings.Contains(out, "## Script injection") {
		t.Errorf("the section must end at the next heading:\n%s", out)
	}

	// --explain=ID is the same
	if _, out2, _ := explain(t, "--explain=missing-timeout"); out2 != out {
		t.Error("--explain=ID differs from --explain ID")
	}
}

func TestExplainKinds(t *testing.T) {
	_, out, _ := explain(t, "--explain", "archived-uses")
	if !strings.Contains(out, "only with --online") {
		t.Errorf("an online rule says so:\n%s", out)
	}
	_, out, _ = explain(t, "--explain", "forbidden-uses")
	if !strings.Contains(out, "runs only when the configuration turns it on") {
		t.Errorf("an opt-in rule says so:\n%s", out)
	}
}

func TestExplainErrors(t *testing.T) {
	status, out, stderr := explain(t, "--explain", "missing-timout")
	if status != ExitStatusInvalidCommandOption || out != "" || !strings.Contains(stderr, `unknown rule ID "missing-timout" in --explain`) || !strings.Contains(stderr, `did you mean "missing-timeout"?`) {
		t.Errorf("status %d, stdout %q, stderr %q", status, out, stderr)
	}
	status, _, stderr = explain(t, "--explain", "template-injection-trusted")
	if status != ExitStatusInvalidCommandOption || !strings.Contains(stderr, "was merged into \"template-injection\"") {
		t.Errorf("a retired ID: status %d, stderr %q", status, stderr)
	}
}

// TestEveryRuleCanBeExplained keeps docs/checks.md and the DocsAnchor of the rules in step: --explain prints the section
// that the anchor names, so an anchor which does not exist is a bug of the registry.
func TestEveryRuleCanBeExplained(t *testing.T) {
	for _, r := range Rules() {
		var b bytes.Buffer
		if err := explainRule(&b, r.ID); err != nil {
			t.Errorf("%s: %v", r.ID, err)
			continue
		}
		if r.DocsAnchor == "" {
			continue
		}
		sec := checksSection(checksDoc, r.DocsAnchor)
		if sec == "" {
			t.Errorf("%s: docs/checks.md has no section <a id=%q>", r.ID, r.DocsAnchor)
			continue
		}
		if strings.Contains(sec, `<a id="`) && strings.Index(sec, `<a id="`) == 0 {
			t.Errorf("%s: the section starts with another anchor", r.ID)
		}
		if !strings.Contains(b.String(), strings.SplitN(sec, "\n", 2)[0]) {
			t.Errorf("%s: the section is not in the output", r.ID)
		}
	}
}

func TestChecksSection(t *testing.T) {
	doc := "intro\n<a id=\"one\"></a>\n## One\n\ntext\n```\n## not a heading\n```\n\n### Sub\n\nmore\n\n<a id=\"two\"></a>\n## Two\n\nsecond\n\n## Three without anchor\n\nthird\n"
	if got, want := checksSection(doc, "one"), "## One\n\ntext\n```\n## not a heading\n```\n\n### Sub\n\nmore"; got != want {
		t.Errorf("one:\n%q\nwant\n%q", got, want)
	}
	if got, want := checksSection(doc, "two"), "## Two\n\nsecond"; got != want {
		t.Errorf("two: %q", got)
	}
	if got := checksSection(doc, "three"); got != "" {
		t.Errorf("three: %q", got)
	}
}
