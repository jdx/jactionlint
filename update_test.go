package jactionlint

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var updateNow = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

const (
	shaV130 = "1111111111111111111111111111111111111111"
	shaV140 = "2222222222222222222222222222222222222222"
	shaV150 = "3333333333333333333333333333333333333333"
	shaV200 = "4444444444444444444444444444444444444444"
	shaOld  = "5555555555555555555555555555555555555555"
)

type fixtureRel struct {
	tag        string
	sha        string
	days       int // age at updateNow
	immutable  bool
	prerelease bool
	noRelease  bool
}

// updateFixtureClient serves the releases of example/action.
func updateFixtureClient(t *testing.T, rels []fixtureRel, aliases map[string]string) *FixtureGitHubClient {
	t.Helper()
	repo := &fixtureRepo{
		Tags:         &GitHubTagList{},
		Releases:     map[string]*fixtureRelease{},
		ReleaseDates: map[string]*GitHubReleaseDate{},
	}
	for _, r := range rels {
		repo.Tags.Tags = append(repo.Tags.Tags, GitHubTag{Name: r.tag, SHA: r.sha})
		if r.noRelease {
			repo.Releases[r.tag] = &fixtureRelease{}
			repo.ReleaseDates[fixtureReleaseKey(r.tag, r.sha)] = &GitHubReleaseDate{CommittedAt: updateNow.Add(-time.Duration(r.days) * 24 * time.Hour)}
			continue
		}
		repo.Releases[r.tag] = &fixtureRelease{Found: true, Immutable: r.immutable, Prerelease: r.prerelease}
		repo.ReleaseDates[fixtureReleaseKey(r.tag, r.sha)] = &GitHubReleaseDate{PublishedAt: updateNow.Add(-time.Duration(r.days) * 24 * time.Hour)}
	}
	for alias, sha := range aliases {
		repo.Tags.Tags = append(repo.Tags.Tags, GitHubTag{Name: alias, SHA: sha})
	}
	b, err := json.Marshal(gitHubFixtures{Repos: map[string]*fixtureRepo{"example/action": repo}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFixtureGitHubClient(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func standardReleases() []fixtureRel {
	return []fixtureRel{
		{tag: "v1.3.0", sha: shaV130, days: 90, immutable: true},
		{tag: "v1.4.0", sha: shaV140, days: 30, immutable: true},
		{tag: "v1.5.0", sha: shaV150, days: 10, immutable: false},
	}
}

func standardAliases() map[string]string {
	return map[string]string{"v1": shaV150}
}

// runUpdate writes the workflow to a repository, runs the update and returns the result and the workflow afterwards.
func runUpdate(t *testing.T, client GitHubClient, workflow string, uo UpdateOptions, cfg string) (*UpdateResult, string, string) {
	t.Helper()
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(wfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if cfg != "" {
		if err := os.WriteFile(filepath.Join(dir, ".github", "jactionlint.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(wfDir, "ci.yaml")
	if err := os.WriteFile(path, []byte(workflow), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, log bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{GitHubClient: client, Online: true, WorkingDir: dir, LogWriter: &log, Format: "oneline", Color: ColorOptionKindNever})
	if err != nil {
		t.Fatal(err)
	}
	uo.now = func() time.Time { return updateNow }
	res, err := l.Update(nil, uo)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	return res, string(after), out.String() + log.String()
}

func wf(uses string) string {
	return "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n"
}

func TestUpdateFreezePinsTheLatestRelease(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	freeze := true
	res, after, _ := runUpdate(t, c, wf("example/action@v1.3.0"), UpdateOptions{Mode: UpdateApply, Freeze: &freeze}, "update:\n  require-immutable-releases: true\n")
	want := wf("example/action@" + shaV140 + " # v1.4.0")
	if after != want {
		t.Fatalf("got:\n%s\nwant:\n%s", after, want)
	}
	if res.Updates != 1 || len(res.Files) != 1 {
		t.Errorf("updates = %d, files = %v", res.Updates, res.Files)
	}
}

func TestUpdateSHAKeepsTheRestOfTheComment(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	_, after, _ := runUpdate(t, c, wf("example/action@"+shaV130+" # v1.3.0 (see #12)"), UpdateOptions{Mode: UpdateApply}, "")
	want := wf("example/action@" + shaV150 + " # v1.5.0 (see #12)")
	if after != want {
		t.Fatalf("got:\n%s\nwant:\n%s", after, want)
	}
}

func TestUpdateDiffAndReportWriteNothing(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	for _, mode := range []UpdateMode{UpdateReport, UpdateCheck, UpdateDiff} {
		src := wf("example/action@v1.3.0")
		res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: mode}, "")
		if after != src {
			t.Errorf("mode %d changed the file:\n%s", mode, after)
		}
		if res.Updates != 1 || !strings.Contains(out, "example/action can be updated from v1.3.0 to v1.5.0") {
			t.Errorf("mode %d: updates = %d, output:\n%s", mode, res.Updates, out)
		}
		if !strings.Contains(res.Diff, "+      - uses: example/action@v1.5.0") {
			t.Errorf("mode %d: diff:\n%s", mode, res.Diff)
		}
	}
}

func TestUpdateCooldownNeverDowngradesAndReportsTheHeldRelease(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := wf("example/action@v1.4.0")
	days := 30
	res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, MinAge: &days}, "")
	if after != src || res.Updates != 0 {
		t.Fatalf("a reference was changed: %d updates\n%s", res.Updates, after)
	}
	if !strings.Contains(out, "v1.5.0 was released 10 days ago (cooldown 30 days)") {
		t.Errorf("the held release is not reported:\n%s", out)
	}
	// v1.4.0 is itself 30 days old; a 60 day cooldown makes the reference a violation but it is not downgraded
	days = 60
	res, after, out = runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, MinAge: &days}, "")
	if after != src || res.Violations != 1 || !strings.Contains(out, "was released 30 days ago (cooldown 60 days)") {
		t.Errorf("violations = %d\n%s\n%s", res.Violations, after, out)
	}
}

func TestUpdateCooldownFromConfigPicksTheNewestOldEnoughRelease(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	_, after, _ := runUpdate(t, c, wf("example/action@v1.3.0"), UpdateOptions{Mode: UpdateApply}, "update:\n  cooldown-days: 14\n")
	if want := wf("example/action@v1.4.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateReleaseMetadata(t *testing.T) {
	rels := []fixtureRel{
		{tag: "v1.3.0", sha: shaV130, days: 90, immutable: true},
		{tag: "v1.4.0", sha: shaV140, days: 30, immutable: true},
		{tag: "v1.5.0", sha: shaV150, days: 20, immutable: true, prerelease: true},
		{tag: "v1.6.0", sha: shaOld, days: 15, immutable: false},
		{tag: "v2.0.0", sha: shaV200, noRelease: true, days: 10},
	}
	cases := []struct {
		name, cfg, want string
	}{
		{"prereleases excluded by metadata, mutable and tag-only releases accepted", "update:\n  exclude-prereleases: true\n", "v2.0.0"},
		{"immutable releases only", "update:\n  require-immutable-releases: true\n", "v1.5.0"},
		{"both", "update:\n  require-immutable-releases: true\n  exclude-prereleases: true\n", "v1.4.0"},
		{"tag filter", "update:\n  exclude-tags: \"v{2,1.6}*\"\n  exclude-prereleases: true\n", "v1.4.0"},
		{"include", "update:\n  include-tags: [\"v1.[0-4].*\"]\n", "v1.4.0"},
		{"exclusion wins", "update:\n  include-tags: v1*\n  exclude-tags: v1.6.0\n", "v1.5.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := updateFixtureClient(t, rels, nil)
			_, after, _ := runUpdate(t, client, wf("example/action@v1.3.0"), UpdateOptions{Mode: UpdateApply}, c.cfg)
			if want := wf("example/action@" + c.want); after != want {
				t.Fatalf("got:\n%s\nwant %s", after, c.want)
			}
		})
	}
}

func TestUpdateMovingTagsAreNotReleases(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	// v1.5.0 is held by the cooldown; the moving tag v1 points at the same commit and must not be a way around it
	days := 14
	_, after, _ := runUpdate(t, c, wf("example/action@v1.3.0"), UpdateOptions{Mode: UpdateApply, MinAge: &days}, "")
	if want := wf("example/action@v1.4.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateKeepsTheGranularityOfAMovingTag(t *testing.T) {
	rels := append(standardReleases(), fixtureRel{tag: "v2.0.0", sha: shaV200, days: 40, immutable: true})
	c := updateFixtureClient(t, rels, map[string]string{"v1": shaV150, "v2": shaV200})
	_, after, _ := runUpdate(t, c, wf("example/action@v1"), UpdateOptions{Mode: UpdateApply}, "")
	if want := wf("example/action@v2"); after != want {
		t.Fatalf("got:\n%s", after)
	}
	// v1 is current within its major version
	c = updateFixtureClient(t, standardReleases(), standardAliases())
	src := wf("example/action@v1")
	res, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if after != src || res.Updates != 0 {
		t.Fatalf("v1 was changed:\n%s", after)
	}
}

func TestUpdateVerifiesTheVersionComment(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := wf("example/action@" + shaV130 + " # v1.4.0")
	res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if after != src || res.Violations != 1 || !strings.Contains(out, "is not the commit of the version in its comment (v1.4.0)") {
		t.Fatalf("violations = %d\n%s\n%s", res.Violations, after, out)
	}
	// A commit that no tag points at has no known version
	src = wf("example/action@" + shaOld)
	res, after, out = runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if after != src || res.Violations != 1 || !strings.Contains(out, "no release tag points at the commit") {
		t.Fatalf("violations = %d\n%s\n%s", res.Violations, after, out)
	}
}

func TestUpdateAcceptsALessSpecificComment(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	freeze := true
	days := 14
	// "# v1" is true for v1.3.0's major, and stays true when the pin moves to v1.4.0
	src := wf("example/action@" + shaV130 + " # v1")
	res, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, Freeze: &freeze, MinAge: &days}, "")
	if want := wf("example/action@" + shaV140 + " # v1"); after != want || res.Updates != 1 {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdatePinOnlyKeepsTheVersion(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	_, after, _ := runUpdate(t, c, wf("example/action@v1.3.0"), UpdateOptions{Mode: UpdateApply, PinOnly: true}, "")
	if want := wf("example/action@" + shaV130 + " # v1.3.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
	// Already pinned with a version comment: nothing to do
	src := wf("example/action@" + shaV130 + " # v1.3.0")
	res, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, PinOnly: true}, "")
	if after != src || res.Updates != 0 {
		t.Fatalf("got:\n%s", after)
	}
	// Pinned without a comment: the comment is added
	_, after, _ = runUpdate(t, c, wf("example/action@"+shaV130), UpdateOptions{Mode: UpdateApply, PinOnly: true}, "")
	if want := wf("example/action@" + shaV130 + " # v1.3.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateKeepsAPinPinnedAndQuotes(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	_, after, _ := runUpdate(t, c, wf(`"example/action@`+shaV130+`" # v1.3.0`), UpdateOptions{Mode: UpdateApply}, "")
	if want := wf(`"example/action@` + shaV150 + `" # v1.5.0`); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateBranchToTag(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	freeze := true
	src := wf("example/action@main")
	if _, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, ""); after != src {
		t.Fatalf("a branch was converted without --branch-to-tag:\n%s", after)
	}
	_, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, BranchToTag: []string{"^(main|master)$"}}, "")
	if want := wf("example/action@v1.5.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
	_, after, _ = runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, BranchToTag: []string{"^main$"}, Freeze: &freeze}, "")
	if want := wf("example/action@" + shaV150 + " # v1.5.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateActionFilters(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := wf("example/action@v1.3.0")
	for name, uo := range map[string]UpdateOptions{
		"excluded":          {Mode: UpdateApply, ExcludeActions: []string{`^example/`}},
		"not included":      {Mode: UpdateApply, IncludeActions: []string{`^other/`}},
		"ignore rule":       {Mode: UpdateApply, ActionRules: []string{`^example/action@v1\.3=ignore`}},
		"ignore by comment": {Mode: UpdateApply, ActionRules: []string{`# v1.3.0$=ignore`}},
	} {
		s := src
		if name == "ignore by comment" {
			s = wf("example/action@" + shaV130 + " # v1.3.0")
		}
		if _, after, _ := runUpdate(t, c, s, uo, ""); after != s {
			t.Errorf("%s: the reference was changed:\n%s", name, after)
		}
	}
	_, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, IncludeActions: []string{`^example/action$`}}, "")
	if want := wf("example/action@v1.5.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
	// A cooldown rule applies to the actions it matches only
	_, after, _ = runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, ActionRules: []string{`^example/action@=cooldown:14`}}, "")
	if want := wf("example/action@v1.4.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateSkipsWhatItCannotUpdate(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := "on: push\njobs:\n  a:\n    uses: ./.github/workflows/other.yaml\n  b:\n    runs-on: x\n    steps:\n      - uses: ./local\n      - uses: docker://alpine:3\n      - run: |\n          echo uses: example/action@v1.3.0\n      - uses: example/action@${{ matrix.ref }}\n"
	res, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if after != src || res.Updates != 0 || res.Violations != 0 {
		t.Fatalf("updates = %d, violations = %d:\n%s", res.Updates, res.Violations, after)
	}
}

func TestUpdateReportsAnUnreachableRepository(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := wf("unknown/thing@v1.0.0")
	res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if after != src || res.Violations != 1 || !strings.Contains(out, "unknown/thing") {
		t.Fatalf("violations = %d\n%s\n%s", res.Violations, after, out)
	}
}

func TestUpdateRefusesALineItCannotEditSafely(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	freeze := true
	src := wf("example/action@v1.3.0 # pinned on purpose")
	res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply, Freeze: &freeze}, "")
	if after != src || res.Violations != 1 || !strings.Contains(out, "does not name a version") {
		t.Fatalf("violations = %d\n%s\n%s", res.Violations, after, out)
	}
	// Without freeze the comment is left alone and the tag is updated
	_, after, _ = runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if want := wf("example/action@v1.5.0 # pinned on purpose"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateReusableWorkflowsAndActionFiles(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := "on: push\njobs:\n  call:\n    uses: example/action/.github/workflows/build.yaml@v1.3.0\n"
	_, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if want := strings.Replace(src, "v1.3.0", "v1.5.0", 1); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateConfig(t *testing.T) {
	cfg, err := ParseConfig([]byte("update:\n  cooldown-days: 7\n  freeze: true\n  include-tags: \"v*\"\n  exclude-tags: [\"*-{alpha,beta,rc}*\"]\n  exclude-prereleases: true\n  require-immutable-releases: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	u := cfg.Update
	if *u.CooldownDays != 7 || !*u.Freeze || !*u.ExcludePrereleases || !*u.RequireImmutableReleases {
		t.Errorf("%+v", u)
	}
	if len(u.IncludeTags) != 1 || u.IncludeTags[0] != "v*" || len(u.ExcludeTags) != 1 {
		t.Errorf("tags: %v %v", u.IncludeTags, u.ExcludeTags)
	}
	for _, bad := range []string{"update:\n  include-tags: true\n", "update:\n  exclude-tags: [false]\n", "update:\n  include-tags: 1.2\n", "update:\n  cooldown-days: 100000\n", "update:\n  cooldown: 7\n", "update:\n  cooldown-days: -1\n", "update:\n  include-tags: \"v{1\"\n"} {
		if _, err := ParseConfig([]byte(bad)); err == nil {
			t.Errorf("%q must be an error", bad)
		}
	}
}

func TestTagPatterns(t *testing.T) {
	for _, c := range []struct {
		pat, tag string
		want     bool
	}{
		{"v*", "v1.2.3", true},
		{"v*", "1.2.3", false},
		{"*-{alpha,beta,rc}*", "v1.0.0-rc.1", true},
		{"*-{alpha,beta,rc}*", "v1.0.0-beta", true},
		{"*-{alpha,beta,rc}*", "v1.0.0", false},
		{"v{1,2}.*", "v2.4.0", true},
		{"v{1,2}.*", "v3.4.0", false},
		{"v{1,{2,3}}", "v3", true},
		{"v1.?.0", "v1.4.0", true},
		{"v1.[0-4].0", "v1.5.0", false},
		{"v1.[!0-4].0", "v1.5.0", true},
		{`v1\*`, "v1*", true},
		{"v1.0", "v1x0", false},
		{`v\{1,2\}`, "v2", false},
		{`v\{1,2\}`, "v{1,2}", true},
	} {
		p, err := compileTagPattern(c.pat)
		if err != nil {
			t.Errorf("%q: %v", c.pat, err)
			continue
		}
		if got := p.match(c.tag); got != c.want {
			t.Errorf("%q matches %q: %v, want %v", c.pat, c.tag, got, c.want)
		}
	}
	for _, bad := range []string{"", "v{1,2", "v1}", "v[1", `v2\`, strings.Repeat("{1,2}", 20)} {
		if _, err := compileTagPattern(bad); err == nil {
			t.Errorf("%q must be an error", bad)
		}
	}
}

func TestUpdateCommandUsage(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"update", "--check", "--apply"}, "cannot be combined"},
		{[]string{"update", "--no-online"}, "--no-online"},
		{[]string{"update", "--min-age=x"}, "--min-age"},
		{[]string{"update", "--fix"}, "belong to the linter"},
		{[]string{"update", "--pin-only", "--min-age=3"}, "--pin-only"},
		{[]string{"update", "--action-rule=x"}, "--action-rule"},
		{[]string{"update", "--include-tag=v{1"}, "--include-tag"},
		{[]string{"update", "--exclude-action=("}, "--exclude-action"},
	} {
		var out, errb bytes.Buffer
		cmd := Command{Stdout: &out, Stderr: &errb, Stdin: strings.NewReader("")}
		if got := cmd.Main(append([]string{"jactionlint"}, c.args...)); got != ExitStatusInvalidCommandOption {
			t.Errorf("%v: exit status %d, stderr %q", c.args, got, errb.String())
		}
		if !strings.Contains(errb.String(), c.want) {
			t.Errorf("%v: stderr %q lacks %q", c.args, errb.String(), c.want)
		}
	}
	var out bytes.Buffer
	cmd := Command{Stdout: &out, Stderr: &bytes.Buffer{}}
	if got := cmd.Main([]string{"jactionlint", "update", "--help"}); got != 0 || !strings.Contains(out.String(), "jactionlint update") {
		t.Errorf("--help: %d %q", got, out.String())
	}
}

func TestUpdateFindingsHaveEveryOutputFormat(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	for _, format := range []string{"sarif", "json", "github", "text"} {
		dir := t.TempDir()
		wfDir := filepath.Join(dir, ".github", "workflows")
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(wfDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wfDir, "ci.yaml"), []byte(wf("example/action@v1.3.0")), 0o644); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		l, err := NewLinter(&out, &LinterOptions{GitHubClient: c, Online: true, WorkingDir: dir, Format: format, Color: ColorOptionKindNever})
		if err != nil {
			t.Fatal(format, err)
		}
		if _, err := l.Update(nil, UpdateOptions{now: func() time.Time { return updateNow }}); err != nil {
			t.Fatal(format, err)
		}
		if !strings.Contains(out.String(), "update-available") {
			t.Errorf("%s output lacks the finding:\n%s", format, out.String())
		}
	}
}

func TestUpdateIgnoresUsesInsideWith(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: example/action@v1.5.0\n        with:\n          uses: example/action@v1.3.0\n          other: { uses: example/action@v1.3.0 }\n"
	res, after, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
	if after != src || res.Updates != 0 || res.Violations != 0 {
		t.Fatalf("an input was treated as a reference:\n%s", after)
	}
}

func TestUpdateReportsReferencesItCannotEdit(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	for _, uses := range []string{">-\n          example/action@v1.3.0", "|\n          example/action@v1.3.0", "\"example/action\\\n            @v1.3.0\""} {
		src := "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: " + uses + "\n"
		res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateApply}, "")
		if after != src || res.Violations != 1 || !strings.Contains(out, "not a plain or quoted scalar on one line") {
			t.Errorf("%q: violations = %d\n%s", uses, res.Violations, out)
		}
	}
}

func TestUpdateStopsOnAnUnusableChangedSince(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".github", "workflows", "ci.yaml")
	src := wf("example/action@v1.3.0")
	os.WriteFile(path, []byte(src), 0o644)
	if out, err := runGit(dir, "init", "-q"); err != nil {
		t.Skipf("git is not usable: %v %s", err, out)
	}
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{GitHubClient: c, Online: true, WorkingDir: dir, ChangedSince: "no-such-revision"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Update([]string{path}, UpdateOptions{Mode: UpdateApply, now: func() time.Time { return updateNow }}); err == nil {
		t.Error("an unknown revision must stop the update")
	}
	if after, _ := os.ReadFile(path); string(after) != src {
		t.Errorf("the file was written:\n%s", after)
	}
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestUpdateReportsLookupsThatFailForTheCurrentRelease(t *testing.T) {
	// v1.5.0 is the latest, so no candidate is looked at; its own release must still be checked
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	delete(c.data.Repos["example/action"].ReleaseDates, "tags/v1.5.0")
	days := 7
	src := wf("example/action@v1.5.0")
	res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateCheck, MinAge: &days}, "")
	if after != src || res.Violations != 1 || !strings.Contains(out, "update-unverified") {
		t.Fatalf("violations = %d\n%s", res.Violations, out)
	}
}

func TestUpdateWithATruncatedTagList(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	repo := c.data.Repos["example/action"]
	var kept []GitHubTag
	for _, tg := range repo.Tags.Tags {
		if tg.Name != "v1.3.0" {
			kept = append(kept, tg)
		}
	}
	repo.Tags.Tags, repo.Tags.Truncated = kept, true
	repo.Refs = map[string]*fixtureRef{"tags/v1.3.0": {SHA: shaV130, Found: true}}
	repo.Branches = &GitHubBranchList{}
	_, after, _ := runUpdate(t, c, wf("example/action@"+shaV130+" # v1.3.0"), UpdateOptions{Mode: UpdateApply}, "")
	if want := wf("example/action@" + shaV150 + " # v1.5.0"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateRewritesAVersionCommentThatStopsBeingTrue(t *testing.T) {
	rels := append(standardReleases(), fixtureRel{tag: "v2.0.0", sha: shaV200, days: 40, immutable: true})
	c := updateFixtureClient(t, rels, map[string]string{"v1": shaV150, "v2": shaV200})
	_, after, _ := runUpdate(t, c, wf("example/action@v1 # v1.5.0"), UpdateOptions{Mode: UpdateApply}, "")
	if want := wf("example/action@v2 # v2"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateNeverWritesAMovingTagThePolicyRejects(t *testing.T) {
	rels := append(standardReleases(), fixtureRel{tag: "v2.0.0", sha: shaV200, days: 40, immutable: true})
	c := updateFixtureClient(t, rels, map[string]string{"v1": shaV150, "v2": shaV200})
	_, after, _ := runUpdate(t, c, wf("example/action@v1"), UpdateOptions{Mode: UpdateApply}, "update:\n  require-immutable-releases: true\n")
	if want := wf("example/action@v2.0.0"); after != want {
		t.Fatalf("immutable only: got:\n%s", after)
	}
	_, after, _ = runUpdate(t, c, wf("example/action@v1"), UpdateOptions{Mode: UpdateApply}, "update:\n  exclude-tags: v2\n")
	if want := wf("example/action@v2.0.0"); after != want {
		t.Fatalf("excluded alias: got:\n%s", after)
	}
}

func TestUpdateActionRuleIgnoresAnUneditableReference(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := "on: push\njobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: >-\n          example/action@v1.3.0\n"
	res, _, _ := runUpdate(t, c, src, UpdateOptions{Mode: UpdateCheck, ActionRules: []string{`^example/action@=ignore`}}, "")
	if res.Violations != 0 || res.Updates != 0 {
		t.Fatalf("an ignored reference was reported: %d %d", res.Updates, res.Violations)
	}
}

func TestUpdateEditsEachFileOnce(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	path := filepath.Join(dir, ".github", "workflows", "ci.yaml")
	os.WriteFile(path, []byte(wf("example/action@v1.3.0")), 0o644)
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{GitHubClient: c, Online: true, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	res, err := l.Update([]string{path, path, filepath.Join(filepath.Dir(path), ".", "ci.yaml")}, UpdateOptions{Mode: UpdateApply, now: func() time.Time { return updateNow }})
	if err != nil || res.Updates != 1 {
		t.Fatalf("err = %v, updates = %v", err, res)
	}
}

func TestUpdateFindsCompositeActions(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755)
	os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yaml"), []byte("on: push\njobs: {}\n"), 0o644)
	action := "name: x\ndescription: y\nruns:\n  using: composite\n  steps:\n    - uses: example/action@v1.3.0\n      shell: bash\n"
	os.MkdirAll(filepath.Join(dir, "tools", "x"), 0o755)
	path := filepath.Join(dir, "tools", "x", "action.yml")
	os.WriteFile(path, []byte(action), 0o644)
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{GitHubClient: c, Online: true, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Update(nil, UpdateOptions{Mode: UpdateApply, now: func() time.Time { return updateNow }}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if want := strings.Replace(action, "v1.3.0", "v1.5.0", 1); string(after) != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateFixedReleasesAreNotAliases(t *testing.T) {
	rels := []fixtureRel{{tag: "v1.2", sha: shaV130, days: 90, immutable: true}, {tag: "v1.2.1", sha: shaV140, days: 30, immutable: true}}
	c := updateFixtureClient(t, rels, nil)
	_, after, _ := runUpdate(t, c, wf("example/action@v1.2"), UpdateOptions{Mode: UpdateApply}, "")
	if want := wf("example/action@v1.2.1"); after != want {
		t.Fatalf("got:\n%s", after)
	}
}

func TestUpdateCurrentAliasIsCheckedAgainstItsRelease(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	src := wf("example/action@v1")
	res, after, out := runUpdate(t, c, src, UpdateOptions{Mode: UpdateCheck}, "update:\n  require-immutable-releases: false\n  exclude-prereleases: true\n")
	if after != src || res.Violations != 0 {
		t.Fatalf("violations = %d\n%s", res.Violations, out)
	}
	rels := []fixtureRel{{tag: "v1.5.0", sha: shaV150, days: 10, immutable: true}}
	c = updateFixtureClient(t, rels, map[string]string{"v1": shaV150})
	res, _, out = runUpdate(t, c, src, UpdateOptions{Mode: UpdateCheck}, "update:\n  require-immutable-releases: true\n")
	if res.Violations != 0 {
		t.Fatalf("v1 was checked as a release of its own:\n%s", out)
	}
}

func TestUpdateCurrentPrereleaseBreaksThePolicy(t *testing.T) {
	rels := []fixtureRel{{tag: "v2.0.0-rc.1", sha: shaV200, days: 10, prerelease: true}}
	c := updateFixtureClient(t, rels, nil)
	res, _, out := runUpdate(t, c, wf("example/action@v2.0.0-rc.1"), UpdateOptions{Mode: UpdateCheck}, "update:\n  exclude-prereleases: true\n")
	if res.Violations != 1 || !strings.Contains(out, "is a prerelease") {
		t.Fatalf("violations = %d\n%s", res.Violations, out)
	}
}

func TestUpdateSaysWhenTheSearchWasCutShort(t *testing.T) {
	rels := []fixtureRel{{tag: "v1.0.0", sha: shaV130, days: 400, immutable: true}}
	for i := 1; i <= maxUpdateCandidates+1; i++ {
		rels = append(rels, fixtureRel{tag: "v2." + strconv.Itoa(i) + ".0", sha: strings.Repeat(strconv.Itoa(i%10), 40), days: 1, immutable: true})
	}
	c := updateFixtureClient(t, rels, nil)
	days := 30
	res, _, out := runUpdate(t, c, wf("example/action@v1.0.0"), UpdateOptions{Mode: UpdateCheck, MinAge: &days}, "")
	if res.Violations != 1 || !strings.Contains(out, "whether an update exists is unknown") {
		t.Fatalf("violations = %d\n%s", res.Violations, out)
	}
}

func TestUpdateRefusesAnOverflowingCooldown(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), nil)
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755)
	os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yaml"), []byte(wf("example/action@v1.3.0")), 0o644)
	l, err := NewLinter(&bytes.Buffer{}, &LinterOptions{GitHubClient: c, Online: true, WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	days := 106752
	if _, err := l.Update(nil, UpdateOptions{MinAge: &days}); err == nil {
		t.Error("a cooldown that overflows must be refused")
	}
}

func TestUpdateMinSeverityHidesOutputOnly(t *testing.T) {
	c := updateFixtureClient(t, standardReleases(), standardAliases())
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755)
	path := filepath.Join(dir, ".github", "workflows", "ci.yaml")
	os.WriteFile(path, []byte(wf("example/action@v1.3.0")), 0o644)
	var out bytes.Buffer
	l, err := NewLinter(&out, &LinterOptions{GitHubClient: c, Online: true, WorkingDir: dir, MinSeverity: SeverityError, Format: "oneline"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := l.Update(nil, UpdateOptions{Mode: UpdateApply, now: func() time.Time { return updateNow }})
	if err != nil || res.Updates != 1 || strings.Contains(out.String(), "update-available") {
		t.Fatalf("err = %v, updates = %d, output %q", err, res.Updates, out.String())
	}
	if after, _ := os.ReadFile(path); !strings.Contains(string(after), "v1.5.0") {
		t.Errorf("the edit was lost:\n%s", after)
	}
}

func TestUpdateCommandRefusesWhatItCannotDo(t *testing.T) {
	t.Setenv(onlineEnvName, "0")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"update"}, "JACTIONLINT_ONLINE=0"},
		{[]string{"update", "--online-report=r.json"}, "--online-report"},
	} {
		var errb bytes.Buffer
		cmd := Command{Stdout: &bytes.Buffer{}, Stderr: &errb}
		if got := cmd.Main(append([]string{"jactionlint"}, c.args...)); got != ExitStatusInvalidCommandOption || !strings.Contains(errb.String(), c.want) {
			t.Errorf("%v: %d %q", c.args, got, errb.String())
		}
	}
}
