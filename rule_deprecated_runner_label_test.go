package jactionlint

import (
	"io"
	"strings"
	"testing"
	"time"
)

func lintAt(t *testing.T, day, cfg, src string) []*Error {
	t.Helper()
	now, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	l, err := NewLinter(io.Discard, &LinterOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	c := &Config{}
	if cfg != "" {
		c = mustParseConfig(t, cfg)
	}
	l.defaultConfig = withoutMissingTimeout(c)
	errs, err := l.Lint("test.yaml", []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	return errs
}

func runsOn(v string) string {
	return "on: push\njobs:\n  j:\n    runs-on: " + v + "\n    steps:\n      - run: echo\n"
}

func TestRunnerLabelRetirementTable(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range runnerLabelRetirements {
		if r.label != strings.ToLower(r.label) || seen[r.label] {
			t.Errorf("%q must be unique and in lower case", r.label)
		}
		seen[r.label] = true
		retired := retirementDay(r.retired) // panics on a malformed date
		if r.deprecated != "" && !retirementDay(r.deprecated).Before(retired) {
			t.Errorf("%s: deprecated on %s is not before retired on %s", r.label, r.deprecated, r.retired)
		}
		if len(r.use) == 0 || !strings.HasPrefix(r.source, "https://github.com/actions/runner-images/issues/") {
			t.Errorf("%s: needs successors and the announcement", r.label)
		}
		for _, u := range r.use {
			if _, ok := defaultRunnerOSCompats[u]; !ok {
				t.Errorf("%s: the successor %q is not a known label", r.label, u)
			}
			if o := runnerLabelRetirementOf(u); o != nil {
				t.Errorf("%s: the successor %q is retired or deprecated too", r.label, u)
			}
		}
		// A label that is still in the table of images is one that has not been retired when the table was written
		if _, ok := defaultRunnerOSCompats[r.label]; ok != (r.deprecated != "") {
			t.Errorf("%s: it is in the table of labels exactly while it is deprecated, not yet retired (deprecated=%q)", r.label, r.deprecated)
		}
	}
}

func TestDeprecatedRunnerLabel(t *testing.T) {
	const cfg = "profile: pedantic\n"
	for _, tc := range []struct {
		what, day, src string
		want           []string // substrings of the messages of deprecated-runner-label
	}{
		{"macos-14 before its day", "2026-10-13", runsOn("macos-14"), []string{`"macos-14" is deprecated: GitHub retires it on 2026-11-02 and fails jobs on it on scheduled brownout days in October 2026. use "macos-15" or "macos-26"`}},
		{"the day before", "2026-11-01", runsOn("macos-14"), []string{"retires it on 2026-11-02"}},
		{"macos-14-large", "2026-10-13", runsOn("[macos-14-large]"), []string{`use "macos-15-large" or "macos-26-large"`}},
		{"macos-14-xlarge in upper case", "2026-10-13", runsOn("MACOS-14-XLARGE"), []string{`use "macos-15-xlarge" or "macos-26-xlarge"`}},
		{"ubuntu-22.04", "2026-10-13", runsOn("ubuntu-22.04"), []string{`"ubuntu-22.04" is deprecated: GitHub retires it on 2027-04-17`}},
		{"ubuntu-22.04-arm", "2026-10-13", runsOn("ubuntu-22.04-arm"), []string{`use "ubuntu-24.04-arm" or "ubuntu-26.04-arm"`}},
		{"before the deprecation started", "2026-06-01", runsOn("macos-14"), []string{"retires it on 2026-11-02"}},
		{"matrix values", "2026-10-13", "on: push\njobs:\n  j:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-24.04, ubuntu-22.04]\n        include:\n          - os: macos-14\n    steps:\n      - run: echo\n", []string{"ubuntu-22.04", "macos-14"}},
		{"expression of the matrix in a list", "2026-10-13", "on: push\njobs:\n  j:\n    runs-on: ['${{ matrix.os }}']\n    strategy:\n      matrix:\n        os: [macos-14]\n    steps:\n      - run: echo\n", []string{"macos-14"}},

		// Not reported
		{"on the day of the retirement unknown-runner-label has it", "2026-11-02", runsOn("macos-14"), nil},
		{"after the retirement", "2027-01-01", runsOn("macos-14"), nil},
		{"ubuntu-22.04 on its day", "2027-04-17", runsOn("ubuntu-22.04"), nil},
		{"the pool of self-hosted runners", "2026-10-13", runsOn("[self-hosted, macos-14]"), nil},
		{"a label of today", "2026-10-13", runsOn("macos-15"), nil},
		{"a moving label", "2026-10-13", runsOn("macos-latest"), nil},
		{"an already retired label", "2026-10-13", runsOn("macos-13"), nil},
		{"an expression", "2026-10-13", runsOn("${{ vars.RUNNER }}"), nil},
		{"a larger runner named by its owner", "2026-10-13", runsOn("macos-14-custom"), nil},
	} {
		t.Run(tc.what, func(t *testing.T) {
			var got []string
			for _, e := range lintAt(t, tc.day, cfg, tc.src) {
				if e.ID == "deprecated-runner-label" {
					got = append(got, e.Message)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("want %d findings but got %v", len(tc.want), got)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("%q is not in %q", w, got[i])
				}
			}
		})
	}
}

func TestDeprecatedRunnerLabelProfile(t *testing.T) {
	// The warning is for the pedantic profile only: a label that still works must not fail a build of the default profile
	for _, cfg := range []string{"profile: correctness\n", "profile: default\n"} {
		for _, e := range lintAt(t, "2026-10-13", cfg, runsOn("macos-14")) {
			if e.ID == "deprecated-runner-label" {
				t.Errorf("%q reported it: %v", cfg, e)
			}
		}
	}
	errs := lintAt(t, "2026-10-13", "profile: pedantic\n", runsOn("macos-14"))
	n := 0
	for _, e := range errs {
		if e.ID == "deprecated-runner-label" {
			n++
			if e.Severity != SeverityWarning {
				t.Errorf("want a warning but got %v", e.Severity)
			}
			if e.Fix != nil {
				t.Error("a fix would invent the image to move to")
			}
		}
	}
	if n != 1 {
		t.Errorf("want 1 finding but got %v", errs)
	}
}

func TestRetiredRunnerLabelIsUnknown(t *testing.T) {
	// From its day on a label is unknown, in the profile that actionlint's checks have, naming the date and the successor
	for _, tc := range []struct {
		what, day, label string
		want             string // "" when the label is accepted
	}{
		{"macos-14 the day before", "2026-11-01", "macos-14", ""},
		{"macos-14 on its day", "2026-11-02", "macos-14", `label "macos-14" is unknown. GitHub retired it on 2026-11-02, so a job on it never gets a runner. use "macos-15" or "macos-26" instead (https://github.com/actions/runner-images/issues/13518)`},
		{"macos-14-large after", "2027-01-01", "macos-14-large", `use "macos-15-large" or "macos-26-large"`},
		{"ubuntu-22.04 before", "2027-04-16", "ubuntu-22.04", ""},
		{"ubuntu-22.04 on its day", "2027-04-17", "ubuntu-22.04", `GitHub retired it on 2027-04-17`},
		{"macos-13", "2026-10-13", "macos-13", `label "macos-13" is unknown. GitHub retired it on 2025-12-04, so a job on it never gets a runner. use "macos-15" or "macos-26" instead`},
		{"ubuntu-20.04", "2026-10-13", "ubuntu-20.04", `GitHub retired it on 2025-04-15`},
		{"windows-2019", "2026-10-13", "windows-2019", `use "windows-2022" or "windows-2025" instead`},
		{"an unknown label keeps the list", "2026-10-13", "macos-27", `available labels are`},
	} {
		t.Run(tc.what, func(t *testing.T) {
			var got []string
			for _, e := range lintAt(t, tc.day, "profile: correctness\n", runsOn(tc.label)) {
				if e.ID == "unknown-runner-label" {
					got = append(got, e.Message)
				}
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no finding but got %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want %q in %v", tc.want, got)
			}
			if tc.want != "available labels are" && strings.Contains(got[0], "available labels are") {
				t.Errorf("the list of every label is noise next to the successor: %q", got[0])
			}
		})
	}

	t.Run("a retired label is dropped from the list of available labels", func(t *testing.T) {
		got := lintAt(t, "2026-12-01", "profile: correctness\n", runsOn("macos-27"))
		if len(got) != 1 || strings.Contains(got[0].Message, `"macos-14"`) || !strings.Contains(got[0].Message, `"macos-15"`) {
			t.Errorf("unexpected %v", got)
		}
	})

	t.Run("a label of the pool of self-hosted runners", func(t *testing.T) {
		if got := lintAt(t, "2027-01-01", "profile: correctness\n", runsOn("[self-hosted, macos-14]")); len(got) != 0 {
			t.Errorf("%v", got)
		}
		if got := lintAt(t, "2027-01-01", "profile: correctness\n", runsOn("[self-hosted, macos-13]")); len(got) != 0 {
			t.Errorf("%v", got)
		}
	})

	t.Run("a label listed in the configuration", func(t *testing.T) {
		cfg := "profile: correctness\nself-hosted-runner:\n  labels: [macos-13]\n"
		if got := lintAt(t, "2026-10-13", cfg, runsOn("macos-13")); len(got) != 0 {
			t.Errorf("%v", got)
		}
	})

	t.Run("a retired label does not conflict", func(t *testing.T) {
		for _, e := range lintAt(t, "2027-01-01", "profile: correctness\n", runsOn("[macos-14, windows-2025]")) {
			if e.ID == "conflicting-runner-labels" {
				t.Errorf("%v", e)
			}
		}
	})
}
