package jactionlint

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const (
	pinSHA1 = "1111111111111111111111111111111111111111"
	pinSHA2 = "2222222222222222222222222222222222222222"
	pinSHA3 = "3333333333333333333333333333333333333333"
	pinDig  = "sha256:abababababababababababababababababababababababababababababababab"
)

func compositeOf(steps ...string) string {
	return "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n" + strings.Join(steps, "") + "    - run: echo\n      shell: bash\n"
}

func stepUses(spec string) string { return "    - uses: " + spec + "\n" }

func pinConfig() *Config {
	c := remoteMetadataConfig()
	c.Rules["unpinnable-action"] = RuleConfig{Level: SeverityWarning, Options: map[string]any{"pedantic": true}}
	return c
}

func TestPinProblemsOfAnAction(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/a@" + pinSHA1 + ":action.yml": compositeOf(
			stepUses("actions/checkout@v4"), // 1: a tag
			stepUses("./local"),             // fine: the same commit
			stepUses("docker://alpine:3"),   // 2: an image tag
			stepUses("docker://alpine@"+pinDig),
			stepUses("o/dep@"+pinSHA2),      // pinned: read, and has 3
			stepUses("o/b@main"),            // 4: a branch
			stepUses("o/c@${{ inputs.x }}"), // not known
			stepUses("o/d@abcdef1"),         // 5: a short SHA is not a pin
		),
		"o/dep@" + pinSHA2 + ":action.yml":  compositeOf(stepUses("actions/setup-node@v4")),
		"o/img@" + pinSHA1 + ":action.yml":  "name: x\nruns:\n  using: docker\n  image: docker://alpine:3\n",
		"o/reg@" + pinSHA1 + ":action.yml":  "name: x\nruns:\n  using: docker\n  image: ghcr.io/o/img:1\n",
		"o/dig@" + pinSHA1 + ":action.yml":  "name: x\nruns:\n  using: docker\n  image: docker://alpine@" + pinDig + "\n",
		"o/file@" + pinSHA1 + ":action.yml": "name: x\nruns:\n  using: docker\n  image: Dockerfile\n",
		"o/js@" + pinSHA1 + ":action.yml":   "name: x\nruns:\n  using: node20\n  main: i.js\n",
	})
	s := newOnlineSession(context.Background(), c, nil)
	got := func(repo string) []string {
		t.Helper()
		ps, err := s.PinProblems("o", repo, "", pinSHA1)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range ps {
			out = append(out, p.String())
		}
		return out
	}
	want := []string{
		`a composite step uses "actions/checkout@v4", a ref which can move and not a commit SHA`,
		`a composite step uses the Docker image "docker://alpine:3", a tag and not a digest`,
		`"o/dep@` + pinSHA2 + `", which it uses, has a composite step which uses "actions/setup-node@v4", a ref which can move and not a commit SHA`,
		`a composite step uses "o/b@main", a ref which can move and not a commit SHA`,
		`a composite step uses "o/d@abcdef1", a ref which can move and not a commit SHA`,
	}
	if g := got("a"); strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems of a:\n%s\nwant\n%s", strings.Join(g, "\n"), strings.Join(want, "\n"))
	}
	if g := got("img"); len(g) != 1 || g[0] != `it runs the Docker image "docker://alpine:3", a tag and not a digest` {
		t.Errorf("img: %v", g)
	}
	if g := got("reg"); len(g) != 1 || !strings.Contains(g[0], `"ghcr.io/o/img:1"`) {
		t.Errorf("an image of a registry without the scheme is an image too: %v", g)
	}
	for _, repo := range []string{"dig", "file", "js", "missing"} {
		if g := got(repo); len(g) != 0 {
			t.Errorf("%s: %v", repo, g)
		}
	}
}

// The walk reads each pinned action once, goes two levels deep, and ends a cycle.
func TestPinWalkIsDepthLimitedAndCycleSafe(t *testing.T) {
	chain := map[string]string{}
	for i, name := range []string{"a", "b", "c", "d"} {
		next := []string{"b", "c", "d", ""}[i]
		steps := []string{stepUses("actions/setup-" + name + "@v1")}
		if next != "" {
			steps = append(steps, stepUses("o/"+next+"@"+pinSHA1))
		}
		chain["o/"+name+"@"+pinSHA1+":action.yml"] = compositeOf(steps...)
	}
	c := newFilesClient(t, chain)
	s := newOnlineSession(context.Background(), c, nil)
	ps, err := s.PinProblems("o", "a", "", pinSHA1)
	if err != nil {
		t.Fatal(err)
	}
	// a (level 1) and b (level 2): c is not read
	if len(ps) != 2 || ps[0].holder != "" || ps[1].holder != "o/b@"+pinSHA1 {
		t.Errorf("problems = %+v", ps)
	}
	if n := c.calls.Load(); n != 2 {
		t.Errorf("%d lookups for two levels: %v", n, c.asked)
	}

	// Without the depth limit a cycle still ends
	cyc := newFilesClient(t, map[string]string{
		"o/x@" + pinSHA1 + ":action.yml": compositeOf(stepUses("o/y@"+pinSHA1), stepUses("o/x@v1")),
		"o/y@" + pinSHA1 + ":action.yml": compositeOf(stepUses("o/x@"+pinSHA1), stepUses("docker://alpine")),
	})
	w := pinWalk{s: newOnlineSession(context.Background(), cyc, nil), max: 100, seen: map[string]bool{}}
	ps, err = w.walk("o", "x", "", pinSHA1, 1, "")
	if err != nil || len(ps) != 2 {
		t.Errorf("a cycle: %+v, %v", ps, err)
	}
	if n := cyc.calls.Load(); n != 2 {
		t.Errorf("%d lookups for a cycle of two: %v", n, cyc.asked)
	}
}

func TestUnpinnableActionRuleReportsWhatItShould(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/loose@" + pinSHA1 + ":action.yml": compositeOf(stepUses("actions/checkout@v4")),
		"o/tidy@" + pinSHA1 + ":action.yml":  compositeOf(stepUses("actions/checkout@" + pinSHA3)),
		"o/loose@v1:action.yml":              compositeOf(stepUses("actions/checkout@v4")),
	})
	errs, l := lintOnline(t, c, pinConfig(), workflowWith(
		"uses: o/loose@"+pinSHA1,
		"uses: o/loose@"+pinSHA1,
		"uses: o/tidy@"+pinSHA1,
		"uses: o/loose@v1", // not pinned: the other rule's business
		"uses: o/gone@"+pinSHA2,
		"uses: docker://alpine:3",
	))
	if got := lineIDsOf(errs); strings.Join(got, " ") != "6:unpinnable-action 7:unpinnable-action" {
		t.Errorf("findings = %v\n%v", got, errs)
	}
	for _, e := range errs {
		if e.Severity != SeverityWarning {
			t.Errorf("level = %v", e.Severity)
		}
	}
	// Each file once: o/loose and o/tidy at their commit, o/loose at its tag (read for its inputs by the action rule),
	// the checkout which o/tidy pins and o/gone, which are not on GitHub (action.yml and action.yaml)
	if n := c.calls.Load(); n != 7 {
		t.Errorf("%d lookups: %v", n, c.asked)
	}
	// o/gone and the checkout pinned by o/tidy are not on GitHub (fixtures), which the run counts
	if l.OnlineSkipped() == 0 {
		t.Error("an action which cannot be read is a skipped lookup")
	}
}

// The check needs the option pedantic: it reads every pinned action, also the popular ones.
func TestUnpinnableActionIsOnlyPedantic(t *testing.T) {
	// The action rule reads o/loose for its inputs; only this check reads what o/loose pins
	files := map[string]string{
		"o/loose@" + pinSHA1 + ":action.yml": compositeOf(stepUses("actions/checkout@v4"), stepUses("o/dep@"+pinSHA2)),
		"o/dep@" + pinSHA2 + ":action.yml":   compositeOf(),
	}
	src := workflowWith("uses: o/loose@" + pinSHA1)
	for name, tc := range map[string]struct {
		cfg   *Config
		found bool
	}{
		"default":            {remoteMetadataConfig(), false},
		"option":             {pinConfig(), true},
		"pedantic profile":   {withProfile(remoteMetadataConfig(), ProfilePedantic), true},
		"option off":         {withPedanticOff(withProfile(remoteMetadataConfig(), ProfilePedantic)), false},
		"rule off, pedantic": {withRuleOff(withProfile(remoteMetadataConfig(), ProfilePedantic), "unpinnable-action"), false},
	} {
		t.Run(name, func(t *testing.T) {
			c := newFilesClient(t, files)
			errs, _ := lintOnline(t, c, tc.cfg, src)
			found := false
			for _, e := range errs {
				found = found || e.ID == "unpinnable-action"
			}
			if found != tc.found {
				t.Errorf("found = %v: %v", found, errs)
			}
			read := strings.Contains(strings.Join(c.asked, " "), "o/dep@")
			if read != tc.found {
				t.Errorf("what the pinned action pins is read: %v, want %v (%v)", read, tc.found, c.asked)
			}
		})
	}
}

func withProfile(c *Config, p Profile) *Config { c.Profile = p; return c }
func withRuleOff(c *Config, id string) *Config {
	c.Rules[id] = RuleConfig{Level: SeverityOff}
	return c
}
func withPedanticOff(c *Config) *Config {
	c.Rules["unpinnable-action"] = RuleConfig{Level: SeverityWarning, Options: map[string]any{"pedantic": false}}
	return c
}

func TestUnpinnableActionListsAFewProblems(t *testing.T) {
	var steps []string
	for i := 1; i <= 5; i++ {
		steps = append(steps, stepUses(fmt.Sprintf("o/d%d@v1", i)))
	}
	c := newFilesClient(t, map[string]string{"o/many@" + pinSHA1 + ":action.yml": compositeOf(steps...)})
	errs, _ := lintOnline(t, c, pinConfig(), workflowWith("uses: o/many@"+pinSHA1))
	if len(errs) != 1 || strings.Count(errs[0].Message, "a ref which can move") != maxReportedPinProblems || !strings.Contains(errs[0].Message, "(and 2 more)") {
		t.Errorf("%v", errs)
	}
}

// An action which two steps pin to the same commit is read once and its problems are listed once, so that they do not
// use up the places of the other dependencies.
func TestPinProblemsOfSiblingActionsAreNotRepeated(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/a@" + pinSHA1 + ":action.yml": compositeOf(
			stepUses("o/dep@"+pinSHA2),
			stepUses("o/dep@"+pinSHA2),
			stepUses("o/dep@"+strings.ToUpper(pinSHA2)),
			stepUses("o/other@v1"),
		),
		"o/dep@" + pinSHA2 + ":action.yml": compositeOf(stepUses("actions/setup-node@v4")),
	})
	s := newOnlineSession(context.Background(), c, nil)
	ps, err := s.PinProblems("o", "a", "", pinSHA1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Errorf("want the problem of dep once and the one of o/other, got %d: %v", len(ps), ps)
	}
}

// The steps of a parallel group of a composite action are steps of the action: what they use is not frozen by the
// pin either, and the group of a nested action is read too.
func TestPinProblemsReadParallelGroups(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/a@" + pinSHA1 + ":action.yml": "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n" +
			"    - uses: o/dep@" + pinSHA2 + "\n" +
			"    - parallel:\n" +
			"        - uses: actions/cache@v4\n" +
			"        - uses: docker://alpine:3\n" +
			"        - run: echo\n          shell: bash\n" +
			"        - uses: o/dep@" + pinSHA2 + "\n" +
			"    - uses: o/other@v1\n",
		"o/dep@" + pinSHA2 + ":action.yml": "name: x\ndescription: d\nruns:\n  using: composite\n  steps:\n    - parallel:\n        - uses: actions/setup-node@v4\n",
	})
	s := newOnlineSession(context.Background(), c, nil)
	ps, err := s.PinProblems("o", "a", "", pinSHA1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.target+"|"+p.holder)
	}
	want := []string{
		"actions/setup-node@v4|o/dep@" + pinSHA2,
		"actions/cache@v4|",
		"docker://alpine:3|",
		"o/other@v1|",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// `./path` in a step of a remote composite action is looked up in the workspace of the caller at run time (see
// self-repository), not in the repository of the action, so it cannot be followed and is not reported. An action
// which two steps use is walked once, and one which uses itself ends.
func TestPinProblemsDoNotFollowLocalPathsAndWalkEachActionOnce(t *testing.T) {
	c := newFilesClient(t, map[string]string{
		"o/a@" + pinSHA1 + ":action.yml": compositeOf(
			stepUses("./.github/actions/x"),
			stepUses("$/.github/actions/y"),
			stepUses("o/a@"+pinSHA1), // itself
			stepUses("o/dep@"+pinSHA2),
			stepUses("o/dep@"+pinSHA2),
		),
		"o/dep@" + pinSHA2 + ":action.yml": compositeOf(stepUses("o/a@"+pinSHA1), stepUses("actions/setup-node@v4")),
	})
	w := pinWalk{s: newOnlineSession(context.Background(), c, nil), max: 10, seen: map[string]bool{}}
	ps, err := w.walk("o", "a", "", pinSHA1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].target != "actions/setup-node@v4" {
		t.Errorf("problems = %v", ps)
	}
	if n := c.calls.Load(); n != 2 {
		t.Errorf("each action is read once: %d lookups %v", n, c.asked)
	}
}
