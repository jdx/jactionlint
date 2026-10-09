package jactionlint

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// lintZ lints the source with the config and returns "line id" of each error.
func lintZ(t *testing.T, cfg *Config, src string) []string {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		cfg = &Config{}
	}
	l.defaultConfig = withoutMissingTimeout(cfg)
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, e := range errs {
		have = append(have, fmt.Sprintf("%d %s", e.Line, e.ID))
	}
	return have
}

// zWorkflow builds a workflow with one step per line of the steps.
func zWorkflow(steps ...string) string {
	return "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + strings.Join(steps, "")
}

const zInj = "echo ${{ github.event.issue.title }}"

func TestZizmorIgnorePlacements(t *testing.T) {
	tests := []struct {
		name string
		step string
		want []string
	}{
		{"no comment", "      - run: " + zInj + "\n", []string{"6 template-injection"}},
		{"trailing", "      - run: " + zInj + " # zizmor: ignore[template-injection]\n", nil},
		{"trailing with reason", "      - run: " + zInj + " # zizmor: ignore[template-injection] it is safe here\n", nil},
		{"after another comment", "      - run: " + zInj + " # v1 # zizmor: ignore[template-injection]\n", nil},
		{"multiple rules", "      - run: " + zInj + " # zizmor: ignore[artipacked,template-injection]\n", nil},
		{"spaces around names", "      - run: " + zInj + " # zizmor: ignore[artipacked,  template-injection ,x]\n", nil},
		{"duplicates and empty names", "      - run: " + zInj + " # zizmor: ignore[,template-injection,,template-injection]\n", nil},
		{"other rule", "      - run: " + zInj + " # zizmor: ignore[artipacked]\n", []string{"6 template-injection"}},
		{"unknown audit is no error", "      - run: " + zInj + " # zizmor: ignore[no-such-audit]\n", []string{"6 template-injection"}},
		{"no rule list", "      - run: " + zInj + " # zizmor: ignore\n", []string{"6 template-injection"}},
		{"empty rule list", "      - run: " + zInj + " # zizmor: ignore[]\n", []string{"6 template-injection"}},
		{"unclosed", "      - run: " + zInj + " # zizmor: ignore[template-injection\n", []string{"6 template-injection"}},
		{"names need a comma", "      - run: " + zInj + " # zizmor: ignore[artipacked template-injection]\n", []string{"6 template-injection"}},
		{"text glued to the bracket", "      - run: " + zInj + " # zizmor: ignore[template-injection]reason\n", []string{"6 template-injection"}},
		{"spacing is fixed 1", "      - run: " + zInj + " # zizmor:ignore[template-injection]\n", []string{"6 template-injection"}},
		{"spacing is fixed 2", "      - run: " + zInj + " #zizmor: ignore[template-injection]\n", []string{"6 template-injection"}},
		{"spacing is fixed 3", "      - run: " + zInj + " #  zizmor: ignore[template-injection]\n", []string{"6 template-injection"}},
		// zizmor honors a comment only inside the span of the finding, so a comment on a line of its
		// own above the line is not enough
		{"whole line above", "      # zizmor: ignore[template-injection]\n      - run: " + zInj + "\n", []string{"7 template-injection"}},
		{"on the next step", "      - run: " + zInj + "\n      - run: echo # zizmor: ignore[template-injection]\n", []string{"6 template-injection"}},
		{"header of a block scalar", "      - run: | # zizmor: ignore[template-injection]\n          " + zInj + "\n          echo b\n", nil},
		{"header of a block scalar after another comment", "      - run: | # note # zizmor: ignore[template-injection]\n          " + zInj + "\n", nil},
		{"header of a block scalar with indicator after another comment", "      - run: |- # keep # zizmor: ignore[template-injection]\n          " + zInj + "\n", nil},
		{"header of a block scalar with indicator", "      - run: |- # zizmor: ignore[template-injection]\n          " + zInj + "\n", nil},
		{"header of a block scalar is not the next step", "      - run: | # zizmor: ignore[template-injection]\n          echo a\n      - run: " + zInj + "\n", []string{"8 template-injection"}},
		{"header of a step key", "      - env: # zizmor: ignore[template-injection]\n          A: b\n        run: " + zInj + "\n", []string{"8 template-injection"}},
		// The anchor is unused, which is another finding
		{"anchor", "      - run: &cmd " + zInj + " # zizmor: ignore[template-injection]\n", []string{"6 unused-anchor"}},
		{"quoted value", "      - run: \"" + zInj + "\" # zizmor: ignore[template-injection]\n", nil},
		{"flow mapping", "      - {run: \"" + zInj + "\"} # zizmor: ignore[template-injection]\n", nil},
		{"hash in a quoted value is no comment", "      - run: \"" + zInj + " # zizmor: ignore[template-injection]\"\n", []string{"6 template-injection"}},
		// The text is part of the script, not a comment
		{"inside a block scalar", "      - run: |\n          " + zInj + " # zizmor: ignore[template-injection]\n", []string{"7 template-injection"}},
		{"line of its own inside a block scalar", "      - run: |\n          # zizmor: ignore[template-injection]\n          " + zInj + "\n", []string{"8 template-injection"}},
		{"after a block scalar", "      - run: |\n          " + zInj + "\n        # zizmor: ignore[template-injection]\n", []string{"7 template-injection"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, lintZ(t, nil, zWorkflow(tc.step))); diff != "" {
				t.Errorf("(-want +got): %s", diff)
			}
		})
	}
}

func TestZizmorIgnoreCRLF(t *testing.T) {
	src := strings.ReplaceAll(zWorkflow(
		"      - run: "+zInj+" # zizmor: ignore[template-injection]\n",
		"      - run: | # zizmor: ignore[template-injection]\n          "+zInj+"\n",
		"      - run: |\n          "+zInj+" # zizmor: ignore[template-injection]\n",
	), "\n", "\r\n")
	if diff := cmp.Diff([]string{"10 template-injection"}, lintZ(t, nil, src)); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestZizmorIgnoreMultiLineRegion(t *testing.T) {
	// A comment on any line of the region of the error applies, even when it is not the first line
	ig := inlineIgnore{commentLine: 4}
	for _, tc := range []struct {
		line, end int
		want      bool
	}{
		{3, 5, true}, {4, 4, true}, {4, 6, true}, {2, 4, true}, {5, 6, false}, {1, 3, false}, {5, 0, false},
	} {
		if have := ig.covers(&Error{Line: tc.line, EndLine: tc.end}); have != tc.want {
			t.Errorf("lines %d-%d: want %v but have %v", tc.line, tc.end, tc.want, have)
		}
	}
}

func TestZizmorIgnoreHeaderRange(t *testing.T) {
	src := "on: # zizmor: ignore[template-injection]\n" + // 1
		"  push:\n" + // 2
		"    branches: [a]\n" + // 3
		"\n" + // 4
		"# comment\n" + // 5
		"  workflow_dispatch:\n" + // 6
		"jobs:\n" + // 7
		"  j:\n" + // 8
		"    steps: # zizmor: ignore[template-injection]\n" + // 9
		"    - run: a\n" + // 10
		"    - run: |\n" + // 11
		"        b\n" + // 12
		"    timeout-minutes: 1\n" + // 13
		"  k:\n" + // 14
		"    steps:\n" + // 15
		"      - run: | # zizmor: ignore[template-injection]\n" + // 16
		"          b\n" + // 17
		"        shell: bash\n" + // 18
		"      - run: c # zizmor: ignore[template-injection]\n" // 19
	ignores := parseZizmorIgnores([]byte(src))
	var have []string
	for _, ig := range ignores {
		have = append(have, fmt.Sprintf("%d:%d-%d", ig.commentLine, ig.start, ig.end))
	}
	want := []string{"1:1-6", "9:9-12", "16:16-17", "19:0-0"}
	if diff := cmp.Diff(want, have); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}
}

func TestZizmorAliases(t *testing.T) {
	// A zizmor audit name stands for the rule of the same name and for the aliases
	for name, want := range map[string]string{
		"template-injection":    "template-injection",
		"excessive-permissions": "missing-permissions",
		"unpinned-images":       "unpinned-uses",
		"unpinned-uses":         "unpinned-uses",
	} {
		found := false
		for _, a := range zizmorTargets(name) {
			found = found || a.ID == want
		}
		if !found {
			t.Errorf("%s must stand for %s", name, want)
		}
	}
	if ts := zizmorTargets("no-such-audit"); len(ts) != 0 {
		t.Errorf("an unknown audit stands for nothing: %v", ts)
	}
	// Every alias must lead to a rule of jactionlint
	for name, as := range zizmorAliases {
		for _, a := range as {
			if _, ok := LookupRule(a.ID); !ok {
				t.Errorf("the alias %s -> %s is not a rule", name, a.ID)
			}
		}
	}

	src := zWorkflow("      - run: echo\n")
	cfg := ruleConfig("missing-permissions")
	have := lintZ(t, cfg, src)
	if len(have) != 1 || !strings.HasSuffix(have[0], "missing-permissions") {
		t.Fatalf("unexpected errors: %v", have)
	}
	line := strings.Fields(have[0])[0]
	var n int
	fmt.Sscan(line, &n)
	lines := strings.Split(src, "\n")
	lines[n-1] += " # zizmor: ignore[excessive-permissions]"
	if have := lintZ(t, cfg, strings.Join(lines, "\n")); len(have) != 0 {
		t.Errorf("excessive-permissions must suppress missing-permissions: %v", have)
	}
}

func TestZizmorIgnoreMessageFilter(t *testing.T) {
	// unpinned-images covers only the Docker image findings of unpinned-uses
	e := &inlineIgnoreEntry{zizmor: "unpinned-images", targets: zizmorTargets("unpinned-images")}
	if !e.match(&Error{ID: "unpinned-uses", Message: "docker image must be pinned to a digest"}) {
		t.Error("the docker image finding must match")
	}
	if e.match(&Error{ID: "unpinned-uses", Message: "action must be pinned to a full-length commit SHA"}) {
		t.Error("an action finding must not match unpinned-images")
	}
}

func TestZizmorUnusedIgnore(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  unused-ignore: warn\n  unpinned-uses: error\n")
	step := func(comment string) string {
		return zWorkflow("      - run: echo ok " + comment + "\n")
	}

	// A comment for a rule which is on and did not suppress anything is stale
	have := lintZ(t, cfg, step("# zizmor: ignore[template-injection]"))
	if diff := cmp.Diff([]string{"6 unused-ignore"}, have); diff != "" {
		t.Errorf("(-want +got): %s", diff)
	}

	// Never for an audit jactionlint does not have
	if have := lintZ(t, cfg, step("# zizmor: ignore[no-such-audit]")); len(have) != 0 {
		t.Errorf("an unmapped audit must not be reported: %v", have)
	}

	// Never for a rule which is off
	off := mustParseConfig(t, "rules:\n  unused-ignore: warn\n  template-injection: off\n  missing-permissions: off\n")
	for _, c := range []string{"# zizmor: ignore[template-injection]", "# zizmor: ignore[excessive-permissions]"} {
		if have := lintZ(t, off, step(c)); len(have) != 0 {
			t.Errorf("%s: a rule which is off must not be reported: %v", c, have)
		}
	}

	// Never for an online rule while the online checks are off: it could not have reported anything
	offline := mustParseConfig(t, "rules:\n  unused-ignore: warn\n")
	if have := lintZ(t, offline, step("# zizmor: ignore[impostor-commit]")); len(have) != 0 {
		t.Errorf("an online rule must not be reported offline: %v", have)
	}
	// With -online the same comment is stale
	{
		l, err := NewLinter(io.Discard, &LinterOptions{})
		if err != nil {
			t.Fatal(err)
		}
		l.defaultConfig = withoutMissingTimeout(offline)
		l.online = onlineSettings{enabled: true, client: onlineFixtureClient(t)}
		errs, err := l.Lint("test.yaml", []byte(step("# zizmor: ignore[impostor-commit]")), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(errs) != 1 || errs[0].ID != "unused-ignore" {
			t.Errorf("online, the comment is stale: %v", errs)
		}
	}

	// A mix reports only the stale name of the mapped rules, with its position
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = cfg
	src := zWorkflow("      - run: " + zInj + " # zizmor: ignore[no-such-audit, template-injection, unpinned-images]\n")
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 || errs[0].ID != "unused-ignore" || !strings.Contains(errs[0].Message, `zizmor ignore comment for "unpinned-images"`) {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if want := strings.Index(strings.Split(src, "\n")[5], "unpinned-images") + 1; errs[0].Column != want {
		t.Errorf("want column %d but have %d", want, errs[0].Column)
	}
}

func TestMigrateZizmorIgnores(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		out   string
		names string
	}{
		{
			"trailing comment",
			"jobs:\n  j:\n    steps:\n      - uses: a/b@v1 # zizmor: ignore[unpinned-uses]\n",
			"jobs:\n  j:\n    steps:\n      # jactionlint ignore=unpinned-uses\n      - uses: a/b@v1\n",
			"unpinned-uses",
		},
		{
			"keeps another comment",
			"a:\n  - uses: a/b@abc # v2.9.2 # zizmor: ignore[unpinned-uses,cache-poisoning]\n",
			"a:\n  # jactionlint ignore=unpinned-uses,cache-poisoning\n  - uses: a/b@abc # v2.9.2\n",
			"unpinned-uses, cache-poisoning",
		},
		{
			"keeps unpinned-images, which covers only the Docker images of unpinned-uses",
			"a:\n  - uses: docker://alpine:3 # zizmor: ignore[unpinned-images]\n",
			"a:\n  - uses: docker://alpine:3 # zizmor: ignore[unpinned-images]\n",
			"",
		},
		{
			"keeps a name without a counterpart",
			"a:\n  - uses: a/b@abc # v2.9.2 # zizmor: ignore[unpinned-uses,no-such-audit]\n",
			"a:\n  # jactionlint ignore=unpinned-uses\n  - uses: a/b@abc # v2.9.2 # zizmor: ignore[no-such-audit]\n",
			"unpinned-uses",
		},
		{
			"reason and alias",
			"on: # zizmor: ignore[excessive-permissions,unpinned-uses] it is fine\n  push:\n",
			"# it is fine\n# jactionlint ignore=excessive-permissions,missing-permissions,unpinned-uses\non:\n  push:\n",
			"excessive-permissions, unpinned-uses",
		},
		{
			"block scalar header",
			"steps:\n  - run: | # zizmor: ignore[template-injection]\n      echo\n",
			"steps:\n  # jactionlint ignore=template-injection\n  - run: |\n      echo\n",
			"template-injection",
		},
		{
			"CRLF",
			"steps:\r\n  - run: x # zizmor: ignore[template-injection] why\r\n",
			"steps:\r\n  # why\r\n  # jactionlint ignore=template-injection\r\n  - run: x\r\n",
			"template-injection",
		},
		{
			"no trailing newline",
			"a: b # zizmor: ignore[template-injection]",
			"# jactionlint ignore=template-injection\na: b",
			"template-injection",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, names := MigrateZizmorIgnores([]byte(tc.in))
			if diff := cmp.Diff(tc.out, string(out)); diff != "" {
				t.Errorf("(-want +got): %s", diff)
			}
			if have := strings.Join(names, ", "); have != tc.names {
				t.Errorf("want %q but have %q", tc.names, have)
			}
			// Running it again changes nothing
			again, names := MigrateZizmorIgnores(out)
			if string(again) != string(out) || len(names) != 0 {
				t.Errorf("not idempotent: %q %v", again, names)
			}
		})
	}
}

func TestMigrateZizmorIgnoresLeavesAlone(t *testing.T) {
	for name, in := range map[string]string{
		"no comment":          "a: b\n",
		"unmapped audit":      "a: b # zizmor: ignore[no-such-audit]\n",
		"whole line":          "# zizmor: ignore[template-injection]\na: b\n",
		"inside block scalar": "a: |\n  x # zizmor: ignore[template-injection]\n",
		"inside quotes":       "a: \"x # zizmor: ignore[template-injection]\"\n",
		"invalid spacing":     "a: b # zizmor:ignore[template-injection]\n",
		"no list":             "a: b # zizmor: ignore\n",
		// Inserting a comment line would end the plain scalar
		"multi-line plain scalar": "a: b\n  c # zizmor: ignore[template-injection]\n",
	} {
		t.Run(name, func(t *testing.T) {
			out, names := MigrateZizmorIgnores([]byte(in))
			if string(out) != in || len(names) != 0 {
				t.Errorf("must not change: %q %v", out, names)
			}
		})
	}
}

func TestMigrateZizmorIgnoresKeepsLintResult(t *testing.T) {
	// The jactionlint comments written by the migration suppress the same findings
	src := zWorkflow(
		"      - run: "+zInj+" # zizmor: ignore[template-injection] checked\n",
		"      - run: | # zizmor: ignore[template-injection]\n          "+zInj+"\n",
		"      - run: "+zInj+"\n",
	)
	before := lintZ(t, nil, src)
	out, names := MigrateZizmorIgnores([]byte(src))
	if len(names) != 2 {
		t.Fatalf("unexpected names: %v", names)
	}
	// Only the third step is reported, and the lines moved down by the inserted comments
	if diff := cmp.Diff([]string{"9 template-injection"}, before); diff != "" {
		t.Errorf("before (-want +got): %s", diff)
	}
	if diff := cmp.Diff([]string{"12 template-injection"}, lintZ(t, nil, string(out))); diff != "" {
		t.Errorf("after (-want +got): %s", diff)
	}
	// The migrated comments are not stale
	cfg := mustParseConfig(t, "rules:\n  unused-ignore: warn\n")
	if have := lintZ(t, cfg, string(out)); len(have) != 1 {
		t.Errorf("unexpected errors: %v", have)
	}
}

func TestMigrateZizmorIgnoreFiles(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/w.yml"
	other := dir + "/o.yml"
	if err := writeFileKeepingMode(p, []byte("a: b # zizmor: ignore[template-injection]\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileKeepingMode(other, []byte("a: b\n")); err != nil {
		t.Fatal(err)
	}
	res, err := MigrateZizmorIgnoreFiles([]string{p, other})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || len(res[p]) != 1 {
		t.Fatalf("unexpected result: %v", res)
	}
	var out strings.Builder
	l, err := NewLinter(&out, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.MigrateIgnores([]string{p, other}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No zizmor ignore comment could be migrated") {
		t.Errorf("a second run must change nothing: %q", out.String())
	}
}
