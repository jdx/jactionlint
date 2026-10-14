package jactionlint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	freshA = "example/fresh@" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // v1.2.0, released 2 days before the clock of the tests
	freshB = "example/fresh@" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" // v1.1.0, released 42 days before
	freshC = "example/fresh@" + "cccccccccccccccccccccccccccccccccccccccc" // v2.0.0, no release, committed 28 hours before
	freshE = "example/fresh@" + "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" // no tag, committed 6 hours before
)

func cooldownCfg(t *testing.T, extra string) *Config {
	t.Helper()
	return mustParseConfig(t, "rules:\n  archived-uses: off\n  impostor-commit: off\n  known-vulnerable-actions: off\n  ref-confusion: off\n  ref-version-mismatch: off\n  stale-action-refs: off\n"+extra)
}

func cooldownFindings(t *testing.T, client GitHubClient, cfg *Config, steps ...string) []*Error {
	t.Helper()
	errs, _ := lintOnline(t, client, cfg, workflowWith(steps...))
	var ret []*Error
	for _, e := range errs {
		if e.ID == "action-release-cooldown" {
			ret = append(ret, e)
		}
	}
	return ret
}

func TestActionReleaseCooldownIsOptIn(t *testing.T) {
	steps := []string{"uses: " + freshA}
	if got := cooldownFindings(t, onlineFixtureClient(t), cooldownCfg(t, ""), steps...); len(got) != 0 {
		t.Errorf("the rule runs only when the configuration has an entry for it: %v", got)
	}
	if got := cooldownFindings(t, onlineFixtureClient(t), cooldownCfg(t, "  action-release-cooldown: warning\n"), steps...); len(got) != 1 {
		t.Errorf("enabled by its name: %v", got)
	}
	if got := cooldownFindings(t, onlineFixtureClient(t), cooldownCfg(t, "  action-release-cooldown: off\n"), steps...); len(got) != 0 {
		t.Errorf("turned off: %v", got)
	}
	// Only with the online checks
	if got := lintBatchH(t, "rules:\n  action-release-cooldown: warning\n", workflowWith(steps...), "action-release-cooldown"); len(got) != 0 {
		t.Errorf("offline there is nothing to ask: %v", got)
	}
}

func TestActionReleaseCooldownDays(t *testing.T) {
	tests := []struct {
		days string
		step string
		want bool
	}{
		{"", "uses: " + freshA, true}, // 2 days old, 7 days by default
		{"", "uses: " + freshB, false},
		{"days: 1", "uses: " + freshA, false}, // 2 days old
		{"days: 2", "uses: " + freshA, false},
		{"days: 3", "uses: " + freshA, true},
		{"days: 60", "uses: " + freshB, true},
		{"days: 1", "uses: " + freshC, false}, // 28 hours
		{"days: 2", "uses: " + freshC, true},
		{"days: 1", "uses: " + freshE, true}, // 6 hours
		{"days: 365", "uses: example/fresh@main", false},
	}
	for _, tc := range tests {
		t.Run(tc.days+" "+tc.step, func(t *testing.T) {
			extra := "  action-release-cooldown:\n    level: warning\n"
			if tc.days != "" {
				extra += "    " + tc.days + "\n"
			}
			got := cooldownFindings(t, onlineFixtureClient(t), cooldownCfg(t, extra), tc.step)
			if (len(got) == 1) != tc.want || len(got) > 1 {
				t.Errorf("want report=%v, got %v", tc.want, got)
			}
		})
	}
}

func TestActionReleaseCooldownDaysIsValidated(t *testing.T) {
	for _, bad := range []string{"days: 0", "days: -1", "days: soon", "days: 1.5"} {
		if _, err := ParseConfig([]byte("rules:\n  action-release-cooldown:\n    " + bad + "\n")); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestActionReleaseCooldownMessages(t *testing.T) {
	cfg := cooldownCfg(t, "  action-release-cooldown: warning\n")
	msg := func(step string) string {
		got := cooldownFindings(t, onlineFixtureClient(t), cfg, step)
		if len(got) != 1 {
			t.Fatalf("%s: %v", step, got)
		}
		return got[0].Message
	}
	for step, want := range map[string]string{
		"uses: " + freshA:                    "v1.2.0 was released 2 days ago, on 2026-10-11",
		"uses: example/fresh@v1":             "v1.2.0 was released 2 days ago", // the most specific tag of the commit
		"uses: " + freshC:                    "v2.0.0 has no release on GitHub, and its commit cccccccccccc was made 28 hours ago",
		"uses: " + freshE:                    "the commit eeeeeeeeeeee was made 6 hours ago",
		"uses: example/fresh@v2.0.0 # keep ": "a tag moved to an older commit looks older than it is",
	} {
		if m := msg(step); !strings.Contains(m, want) {
			t.Errorf("%s:\n%s\nwant %q", step, m, want)
		}
	}
}

// onlyGitHubClient hides the optional interfaces of the client it wraps.
type onlyGitHubClient struct{ GitHubClient }

func TestActionReleaseCooldownWithoutDates(t *testing.T) {
	cfg := cooldownCfg(t, "  action-release-cooldown: warning\n")
	// A client which cannot tell the date of a release: nothing is reported and nothing breaks
	if got := cooldownFindings(t, onlyGitHubClient{onlineFixtureClient(t)}, cfg, "uses: "+freshA); len(got) != 0 {
		t.Errorf("a client without dates: %v", got)
	}
	// A client whose lookup fails
	c := &releaseFailingClient{GitHubClient: onlineFixtureClient(t), err: errors.New("boom")}
	if got := cooldownFindings(t, c, cfg, "uses: "+freshA); len(got) != 0 {
		t.Errorf("a failing lookup: %v", got)
	}
	// An answer without any date
	c = &releaseFailingClient{GitHubClient: onlineFixtureClient(t), empty: true}
	if got := cooldownFindings(t, c, cfg, "uses: "+freshA); len(got) != 0 {
		t.Errorf("an answer without a date: %v", got)
	}
}

type releaseFailingClient struct {
	GitHubClient
	err   error
	empty bool
}

func (c *releaseFailingClient) ReleaseDate(context.Context, string, string, string, string) (*GitHubReleaseDate, error) {
	if c.empty {
		return &GitHubReleaseDate{}, nil
	}
	return nil, c.err
}

func TestActionReleaseCooldownRespectsTheDenyList(t *testing.T) {
	cfg := mustParseConfig(t, "online-options:\n  deny: [\"example/*\"]\nrules:\n  action-release-cooldown: warning\n")
	c := &countingReleaseClient{GitHubClient: onlineFixtureClient(t)}
	errs, _ := lintOnline(t, c, cfg, workflowWith("uses: "+freshA))
	for _, e := range errs {
		if e.ID == "action-release-cooldown" {
			t.Errorf("a denied repository is not looked up: %v", e)
		}
	}
	if c.n != 0 {
		t.Errorf("%d release lookups for a denied repository", c.n)
	}
}

type countingReleaseClient struct {
	GitHubClient
	n int
}

func (c *countingReleaseClient) ReleaseDate(context.Context, string, string, string, string) (*GitHubReleaseDate, error) {
	c.n++
	return &GitHubReleaseDate{}, nil
}

func TestActionReleaseCooldownAsksOncePerVersion(t *testing.T) {
	c := &countingReleaseClient{GitHubClient: onlineFixtureClient(t)}
	cooldownFindings(t, c, cooldownCfg(t, "  action-release-cooldown: warning\n"), "uses: "+freshA, "uses: "+freshA, "uses: example/fresh@v1", "uses: example/fresh@v1.2.0")
	if c.n != 1 {
		t.Errorf("%d lookups for one version", c.n)
	}
}

func TestReleaseDateTime(t *testing.T) {
	pub := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	com := pub.Add(-48 * time.Hour)
	if at, release, ok := (GitHubReleaseDate{PublishedAt: pub, CommittedAt: com}).Time(); !ok || !release || !at.Equal(pub) {
		t.Errorf("the release wins: %v %v %v", at, release, ok)
	}
	if at, release, ok := (GitHubReleaseDate{CommittedAt: com}).Time(); !ok || release || !at.Equal(com) {
		t.Errorf("the commit date is the fallback: %v %v %v", at, release, ok)
	}
	if _, _, ok := (GitHubReleaseDate{}).Time(); ok {
		t.Error("no date")
	}
}

// The example of the document is what the linter prints.
func TestActionReleaseCooldownDocsExample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("docs", "checks.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	start := strings.Index(doc, `<a id="check-action-release-cooldown"></a>`)
	if start < 0 {
		t.Fatal("no section")
	}
	section := doc[start+1:]
	if i := strings.Index(section, `<a id="`); i >= 0 {
		section = section[:i]
	}
	input, ok := docsBlock(section, "Example input:\n\n```yaml\n")
	if !ok {
		t.Fatal("no example input")
	}
	output, ok := docsBlock(section, "Output:\n<!-- Skip update output -->\n```\n")
	if !ok {
		t.Fatal("no output")
	}
	cfgText, ok := docsBlock(section, "```yaml\nrules:\n")
	if !ok {
		t.Fatal("no configuration")
	}
	cfg, err := parseConfig([]byte("rules:\n" + cfgText + "  archived-uses: off\n  impostor-commit: off\n  known-vulnerable-actions: off\n  ref-confusion: off\n  ref-version-mismatch: off\n  stale-action-refs: off\n"))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	l, err := NewLinter(&out, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.online = onlineSettings{enabled: true, client: onlineFixtureClient(t)}
	l.defaultConfig = withFixtureRules(cfg)
	if _, err := l.Lint("test.yaml", []byte(input), nil); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimRight(out.String(), "\n"), strings.TrimRight(output, "\n"); got != want {
		t.Errorf("the output in docs/checks.md is outdated.\ngot:\n%s\nwant:\n%s", got, want)
	}
}
