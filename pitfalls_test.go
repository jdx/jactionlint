package jactionlint

import (
	"strings"
	"testing"
)

// The tests of this file are the lessons of the issue tracker of zizmor that apply to the rules of jactionlint, see
// "Lessons applied" in docs/zizmor-parity.md. The name of each test names the issue.

func pitfallConfig(ids ...string) *Config {
	c := &Config{Profile: ProfileDefault, Rules: map[string]RuleConfig{}}
	for _, id := range ids {
		c.Rules[id] = RuleConfig{Level: SeverityError}
	}
	return withoutMissingTimeout(c)
}

func pitfallJob(steps string) string {
	return "on: push\njobs:\n  j:\n    runs-on: ubuntu-24.04\n    steps:\n" + steps
}

// zizmor#2219: an app token for the organization has no repositories.
func TestPitfallGitHubAppOrganizationPermissions(t *testing.T) {
	step := func(perms string) string {
		return pitfallJob("      - uses: actions/create-github-app-token@a8d616148505b5069dccd32f177bb87d7f39123b # v2.1.1\n        with:\n          app-id: 1\n          private-key: x\n          owner: foo\n" + perms)
	}
	tests := []struct {
		what  string
		perms string
		want  int
	}{
		{"organization permission only", "          permission-members: read\n", 0},
		{"several organization permissions", "          permission-members: read\n          permission-organization-projects: write\n", 0},
		{"an organization and a repository permission", "          permission-members: read\n          permission-contents: read\n", 1},
		{"a repository permission", "          permission-contents: read\n", 1},
		{"no permission", "", 2}, // the missing repositories and the missing permissions
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := errsWithID(lintFileWithConfig(t, pitfallConfig("github-app"), "ci.yaml", step(tc.perms)), "github-app"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

// zizmor#1914: the author of the pull request is what proves that a bot made the change.
func TestPitfallBotConditionsNeutralizedByTheAuthor(t *testing.T) {
	tests := []struct {
		cond string
		want int
	}{
		{"github.actor == 'dependabot[bot]'", 1},
		{"github.actor == 'dependabot[bot]' && github.event.pull_request.user.login == 'dependabot[bot]'", 0},
		{"github.event.pull_request.user.login == 'dependabot[bot]' && github.actor == 'dependabot[bot]'", 0},
		{"github.actor == 'dependabot[bot]' && github.event.pull_request.user.id == 49699333 && github.event_name == 'pull_request'", 0},
		{"github.actor == 'dependabot[bot]' || github.event.pull_request.user.login == 'dependabot[bot]'", 1},
		{"github.actor == 'dependabot[bot]' && github.event.pull_request.user.login == 'someone'", 1},
		{"github.actor == 'dependabot[bot]' && github.event.pull_request.user.login != 'dependabot[bot]'", 1},
	}
	for _, tc := range tests {
		t.Run(tc.cond, func(t *testing.T) {
			src := "on: pull_request\njobs:\n  j:\n    runs-on: ubuntu-24.04\n    if: " + tc.cond + "\n    steps:\n      - run: echo\n"
			if got := errsWithID(lintFileWithConfig(t, pitfallConfig("bot-conditions"), "ci.yaml", src), "bot-conditions"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

// zizmor#2059: what never runs is not judged by the rules about security and policy.
func TestPitfallStepsWhichNeverRun(t *testing.T) {
	cfg := pitfallConfig("adhoc-packages", "unpinned-uses")
	tests := []struct {
		what string
		src  string
		want int // findings of adhoc-packages and unpinned-uses
	}{
		{"a live step", pitfallJob("      - run: npm install foo\n      - uses: some/tool@v1\n"), 2},
		{"if false", pitfallJob("      - run: npm install foo\n        if: false\n      - uses: some/tool@v1\n        if: false\n"), 0},
		{"if with an expression of false", pitfallJob("      - run: npm install foo\n        if: ${{ false }}\n"), 0},
		{"if not true", pitfallJob("      - run: npm install foo\n        if: ${{ !true }}\n"), 0},
		{"false and something", pitfallJob("      - run: npm install foo\n        if: ${{ false && github.event_name == 'push' }}\n"), 0},
		{"true or something is live", pitfallJob("      - run: npm install foo\n        if: ${{ true || github.event_name == 'push' }}\n"), 1},
		{"unknown condition is live", pitfallJob("      - run: npm install foo\n        if: github.event_name == 'push'\n"), 1},
		{"a job which never runs", "on: push\njobs:\n  j:\n    if: false\n    runs-on: ubuntu-24.04\n    steps:\n      - run: npm install foo\n      - uses: some/tool@v1\n", 0},
		{"the live step next to a dead one", pitfallJob("      - run: npm install foo\n        if: false\n      - run: npm install bar\n"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs := lintFileWithConfig(t, cfg, "ci.yaml", tc.src)
			n := len(errsWithID(errs, "adhoc-packages")) + len(errsWithID(errs, "unpinned-uses"))
			if n != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, errs)
			}
		})
	}
	// The rules of the correctness group still look at it: a mistake in a step which never runs breaks the file
	errs := lintFileWithConfig(t, pitfallConfig(), "ci.yaml", pitfallJob("      - run: echo ${{ github.nope }}\n        if: false\n"))
	if len(errsWithID(errs, "undefined-property")) != 1 {
		t.Errorf("the syntax rules must still see a step which never runs: %v", errs)
	}
}

// zizmor#1098 and #1043: a checkout whose credential a later push uses, and the first version of the action.
func TestPitfallArtipacked(t *testing.T) {
	up := "      - uses: actions/upload-artifact@v4\n        with:\n          path: .\n"
	tests := []struct {
		what string
		src  string
		want int
	}{
		{"plain", pitfallJob("      - uses: actions/checkout@v4\n"), 1},
		{"a later push", pitfallJob("      - uses: actions/checkout@v4\n      - run: git push origin HEAD\n"), 0},
		{"a later push in a longer script", pitfallJob("      - uses: actions/checkout@v4\n      - run: |\n          git add .\n          git commit -m x\n          git push\n"), 0},
		{"a later pushing action", pitfallJob("      - uses: actions/checkout@v4\n      - uses: stefanzweifel/git-auto-commit-action@v5\n"), 0},
		{"a push before the checkout does not count", pitfallJob("      - run: git push\n      - uses: actions/checkout@v4\n"), 1},
		{"a commit alone is no push", pitfallJob("      - uses: actions/checkout@v4\n      - run: git commit -am x\n"), 1},
		{"a push and an upload of the workspace", pitfallJob("      - uses: actions/checkout@v4\n      - run: git push\n" + up), 1},
		{"a push and an upload of dist", pitfallJob("      - uses: actions/checkout@v4\n      - run: git push\n      - uses: actions/upload-artifact@v4\n        with:\n          path: dist\n"), 0},
		{"checkout v1 has no such input", pitfallJob("      - uses: actions/checkout@v1\n"), 0},
		{"checkout v1.0.0", pitfallJob("      - uses: actions/checkout@v1.0.0\n"), 0},
		{"checkout v2", pitfallJob("      - uses: actions/checkout@v2\n"), 1},
		{"a push that never runs", pitfallJob("      - uses: actions/checkout@v4\n      - if: false\n        run: git push\n"), 1},
		{"a push that runs next to one that does not", pitfallJob("      - uses: actions/checkout@v4\n      - if: false\n        run: git push\n      - run: git push\n"), 0},
		{"checkout v1 pinned to a commit", pitfallJob("      - uses: actions/checkout@544eadc6bf3d226fd7a7a9f0dc5b5bf7ca0675b9 # v1\n"), 0},
		{"checkout v1.0.0 pinned to a commit", pitfallJob("      - uses: actions/checkout@544eadc6bf3d226fd7a7a9f0dc5b5bf7ca0675b9 # v1.0.0\n"), 0},
		{"checkout v4 pinned to a commit", pitfallJob("      - uses: actions/checkout@544eadc6bf3d226fd7a7a9f0dc5b5bf7ca0675b9 # v4.1.0\n"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := errsWithID(lintFileWithConfig(t, pitfallConfig("artipacked"), "ci.yaml", tc.src), "artipacked"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

// zizmor#2320: setup-uv caches only off the events that are open to cache poisoning, from v10 on.
func TestPitfallCachePoisoningSetupUV(t *testing.T) {
	const head = "on:\n  release:\n    types: [published]\njobs:\n  j:\n    runs-on: ubuntu-24.04\n    steps:\n"
	const sha = "ae62891fec2bb8e7d6c99fc78c9fec3a63790f8d"
	tests := []struct {
		what string
		step string
		want int
	}{
		{"v9 default", "      - uses: astral-sh/setup-uv@v9\n", 1},
		{"v10 default", "      - uses: astral-sh/setup-uv@v10\n", 0},
		{"v10.0.0 auto", "      - uses: astral-sh/setup-uv@v10.0.0\n        with:\n          enable-cache: auto\n", 0},
		{"v10 true", "      - uses: astral-sh/setup-uv@v10\n        with:\n          enable-cache: true\n", 1},
		{"v9 auto", "      - uses: astral-sh/setup-uv@v9\n        with:\n          enable-cache: auto\n", 1},
		{"v10 false", "      - uses: astral-sh/setup-uv@v10\n        with:\n          enable-cache: false\n", 0},
		{"sha with the version in the comment", "      - uses: astral-sh/setup-uv@" + sha + " # v10.0.0\n", 0},
		{"sha with an older version in the comment", "      - uses: astral-sh/setup-uv@" + sha + " # v8.1.0\n", 1},
		{"sha without a comment", "      - uses: astral-sh/setup-uv@" + sha + "\n", 1},
		{"a guard on the step is still honored", "      - uses: astral-sh/setup-uv@v9\n        if: github.event_name != 'release'\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := errsWithID(lintFileWithConfig(t, pitfallConfig("cache-poisoning"), "ci.yaml", head+tc.step), "cache-poisoning"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

// zizmor#1848: id-token: write is asked for by many jobs that publish with a token.
func TestPitfallUseTrustedPublishing(t *testing.T) {
	perms := "permissions:\n  id-token: write\n"
	job := func(env, run, shell string) string {
		s := "on: push\n" + perms + "jobs:\n  j:\n    runs-on: ubuntu-24.04\n    steps:\n      - "
		if shell != "" {
			s += "shell: " + shell + "\n        "
		}
		s += "run: " + run + "\n"
		if env != "" {
			s += "        env:\n          " + env + "\n"
		}
		return s
	}
	tests := []struct {
		what string
		src  string
		want int
	}{
		{"oidc publish", job("", "npm publish --provenance", ""), 0},
		{"a token next to id-token", job("NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}", "npm publish --provenance", ""), 1},
		{"cargo token next to id-token", job("CARGO_REGISTRY_TOKEN: ${{ secrets.T }}", "cargo publish", ""), 1},
		{"twine password next to id-token", job("TWINE_PASSWORD: ${{ secrets.T }}", "twine upload dist/*", ""), 1},
		{"pwsh without a token and without id-token", strings.Replace(job("", "cargo publish", "pwsh"), perms, "", 1), 1},
		{"pwsh with id-token", job("", "cargo publish", "pwsh"), 0},
		{"pwsh with a token next to id-token", job("CARGO_REGISTRY_TOKEN: ${{ secrets.T }}", "cargo publish", "pwsh"), 1},
		{"pwsh nuget", strings.Replace(job("", "dotnet nuget push x.nupkg", "pwsh"), perms, "", 1), 1},
		{"pwsh dry run", strings.Replace(job("", "cargo publish --dry-run", "pwsh"), perms, "", 1), 0},
		{"pwsh comment", strings.Replace(job("", "\"echo hi # cargo publish\"", "pwsh"), perms, "", 1), 0},
		{"pwsh other registry", strings.Replace(job("", "cargo publish --registry mine", "pwsh"), perms, "", 1), 0},
		{"pwsh twine", strings.Replace(job("", "python -m twine upload dist/*", "pwsh"), perms, "", 1), 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := errsWithID(lintFileWithConfig(t, pitfallConfig("use-trusted-publishing"), "ci.yaml", tc.src), "use-trusted-publishing"); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

// zizmor#1479: GitHub documents default-days for every ecosystem of the options reference, OpenTofu included, so
// a missing cooldown is reported for all of them.
func TestPitfallDependabotCooldownEcosystems(t *testing.T) {
	for _, eco := range []string{"opentofu", "terraform", "docker", "docker-compose", "devcontainers", "github-actions", "pre-commit", "npm"} {
		src := "version: 2\nupdates:\n  - package-ecosystem: " + eco + "\n    directory: /\n    schedule:\n      interval: weekly\n"
		errs := lintDependabot(t, src, cooldownConfig(map[string]any{"default-days": 7}), nil)
		if len(errs) != 1 || errs[0].ID != "dependabot-cooldown" || errs[0].Fix == nil {
			t.Errorf("%s: %v", eco, errs)
		}
	}
}

// zizmor#2433: a second comment after the version comment.
func TestPitfallCommentVersionWithAnotherComment(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"v7.0.1  # ryl disable-line rule:line-length", "v7.0.1"},
		{"v7.0.1 # ryl disable-line", "v7.0.1"},
		{"v1.0.0-rc.1", "v1.0.0-rc.1"},
		{"v1", "v1"},
		{"1.2.3", "1.2.3"},
		{"tag=v1.2.3", "v1.2.3"},
		{"version: v2.9.2", "v2.9.2"},
		{"v2.9.2 (latest)", "v2.9.2"},
	} {
		if got, ok := commentVersion(tc.in); !ok || got != tc.want {
			t.Errorf("commentVersion(%q) = %q, %v. want %q", tc.in, got, ok, tc.want)
		}
	}
	for _, in := range []string{"keep in sync with v4", "zizmor: ignore[cache-poisoning] v2", "pinned", ""} {
		if got, ok := commentVersion(in); ok {
			t.Errorf("commentVersion(%q) = %q, but the comment is not about a version", in, got)
		}
	}
}

func TestPitfallRefVersionMismatchOnline(t *testing.T) {
	sha := strings.Repeat("a", 40)
	other := strings.Repeat("b", 40)
	fx := `{"repos":{"o/r":{"repo":{"default_branch":"main"},
	  "tags":{"tags":[{"name":"v7.0.1","sha":"` + sha + `"},{"name":"v1.0.0-rc.1","sha":"` + other + `"},{"name":"save/v1.0.0","sha":"` + other + `"}]},
	  "refs":{"tags/v2":{"found":false},"tags/2":{"found":false}}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}, "impostor-commit": {Level: SeverityOff, levelSet: true}, "known-vulnerable-actions": {Level: SeverityOff, levelSet: true}, "ref-confusion": {Level: SeverityOff, levelSet: true}, "archived-uses": {Level: SeverityOff, levelSet: true}}}
	tests := []struct {
		what    string
		comment string
		sha     string
		want    int
	}{
		{"matching version", "# v7.0.1", sha, 0},
		{"a second comment follows", "# v7.0.1  # ryl disable-line rule:line-length", sha, 0},
		{"a prerelease", "# v1.0.0-rc.1", other, 0},
		{"a version that is another commit", "# v7.0.1", other, 1},
		{"a tag that does not exist", "# v2", sha, 1},
		{"a comment about something else", "# pinned for the release", sha, 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			errs, _ := lintOnline(t, c, cfg, workflowWith("uses: o/r@"+tc.sha+" "+tc.comment))
			n := 0
			for _, e := range errs {
				if e.ID == "ref-version-mismatch" {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d ref-version-mismatch but got %v", tc.want, lineIDsOf(errs))
			}
		})
	}
}

// zizmor#2321: a hash is also a valid name for a branch or a tag, and GitHub prefers the branch.
func TestPitfallRefConfusionOnHashLookingRefs(t *testing.T) {
	short := "abcdef1"
	fx := `{"repos":{"o/r":{"repo":{"default_branch":"main"},"tags":{"tags":[{"name":"v1","sha":"` + strings.Repeat("e", 40) + `"}]},
	  "refs":{"heads/` + short + `":{"sha":"` + strings.Repeat("c", 40) + `","found":true},"tags/` + short + `":{"found":false},
	          "heads/v1":{"sha":"` + strings.Repeat("d", 40) + `","found":true},"tags/v1":{"sha":"` + strings.Repeat("e", 40) + `","found":true}}}}}`
	c, err := NewFixtureGitHubClient([]byte(fx))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Rules: map[string]RuleConfig{"stale-action-refs": {Level: SeverityOff, levelSet: true}, "impostor-commit": {Level: SeverityOff, levelSet: true}, "known-vulnerable-actions": {Level: SeverityOff, levelSet: true}, "ref-version-mismatch": {Level: SeverityOff, levelSet: true}, "archived-uses": {Level: SeverityOff, levelSet: true}}}
	count := func(ref string) int {
		errs, _ := lintOnline(t, c, cfg, workflowWith("uses: o/r@"+ref))
		n := 0
		for _, e := range errs {
			if e.ID == "ref-confusion" {
				n++
			}
		}
		return n
	}
	if got := count("v1"); got != 1 {
		t.Errorf("a branch and a tag of the same name: %d findings", got)
	}
	if got := count(short); got != 1 {
		t.Errorf("a branch named like a short hash: %d findings", got)
	}
}

// zizmor#1865: the tools of the GitHub-hosted images are not on a self-hosted runner.
func TestPitfallSuperfluousActionsOnSelfHostedRunners(t *testing.T) {
	cfg := pitfallConfig()
	cfg.Rules["superfluous-actions"] = RuleConfig{Level: SeverityError, Options: map[string]any{"pedantic": true}}
	job := func(runsOn string) string {
		return "on: push\njobs:\n  j:\n    runs-on: " + runsOn + "\n    steps:\n      - uses: dtolnay/rust-toolchain@stable\n      - uses: softprops/action-gh-release@v2\n"
	}
	if got := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", job("ubuntu-24.04")), "superfluous-actions"); len(got) != 2 {
		t.Errorf("hosted: %v", got)
	}
	for _, runsOn := range []string{"[self-hosted, linux]", "self-hosted", "[Self-Hosted, x64]"} {
		if got := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", job(runsOn)), "superfluous-actions"); len(got) != 0 {
			t.Errorf("%s: %v", runsOn, got)
		}
	}
}

// A matrix entry that is self-hosted makes the job a self-hosted one.
func TestPitfallSuperfluousActionsOnMatrixRunners(t *testing.T) {
	cfg := pitfallConfig()
	cfg.Rules["superfluous-actions"] = RuleConfig{Level: SeverityError, Options: map[string]any{"pedantic": true}}
	job := func(rows string) string {
		return "on: push\njobs:\n  j:\n    strategy:\n      matrix:\n        r: " + rows + "\n    runs-on: ${{ matrix.r }}\n    steps:\n      - uses: dtolnay/rust-toolchain@stable\n"
	}
	if got := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", job("[ubuntu-24.04, macos-14]")), "superfluous-actions"); len(got) != 1 {
		t.Errorf("hosted matrix: %v", got)
	}
	if got := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", job("[self-hosted, ubuntu-24.04]")), "superfluous-actions"); len(got) != 0 {
		t.Errorf("matrix with a self-hosted entry: %v", got)
	}
}

// A runner that cannot be shown to be hosted is not reported: matrix arrays, include, nested lists and values
// that are unknown statically.
func TestPitfallSuperfluousActionsMayBeSelfHosted(t *testing.T) {
	cfg := pitfallConfig()
	cfg.Rules["superfluous-actions"] = RuleConfig{Level: SeverityError, Options: map[string]any{"pedantic": true}}
	wf := func(strategy, runsOn string) string {
		return "on: push\njobs:\n  j:\n    strategy:\n" + strategy + "    runs-on: " + runsOn + "\n    steps:\n      - uses: dtolnay/rust-toolchain@stable\n"
	}
	cases := []struct {
		name, strategy, runsOn string
		want                   int
	}{
		{"array row value", "      matrix:\n        r: [[self-hosted, linux], ubuntu-24.04]\n", "${{ matrix.r }}", 0},
		{"nested array", "      matrix:\n        r: [[[self-hosted]]]\n", "${{ matrix.r }}", 0},
		{"include string", "      matrix:\n        r: [ubuntu-24.04]\n        include:\n          - r: self-hosted\n", "${{ matrix.r }}", 0},
		{"include array", "      matrix:\n        r: [ubuntu-24.04]\n        include:\n          - r: [self-hosted, linux]\n", "${{ matrix.r }}", 0},
		{"include only", "      matrix:\n        include:\n          - r: [self-hosted, linux]\n", "${{ matrix.r }}", 0},
		{"fromJSON matrix", "      matrix: ${{ fromJSON(needs.x.outputs.m) }}\n", "${{ matrix.r }}", 0},
		{"fromJSON row", "      matrix:\n        r: ${{ fromJSON(needs.x.outputs.r) }}\n", "${{ matrix.r }}", 0},
		{"expression in a value", "      matrix:\n        r: [ubuntu-24.04, \"${{ inputs.r }}\"]\n", "${{ matrix.r }}", 0},
		{"label list with a matrix entry", "      matrix:\n        r: [[self-hosted, linux]]\n", "[${{ matrix.r }}]", 0},
		{"self-hosted next to a matrix entry", "      matrix:\n        x: [ubuntu-24.04]\n", "[self-hosted, \"${{ matrix.x }}\"]", 0},
		{"undefined property", "      matrix:\n        x: [ubuntu-24.04]\n", "${{ matrix.r }}", 0},
		{"hosted arrays", "      matrix:\n        r: [[ubuntu-24.04, x64], macos-14]\n        include:\n          - r: windows-2025\n", "${{ matrix.r }}", 1},
	}
	for _, c := range cases {
		if got := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", wf(c.strategy, c.runsOn)), "superfluous-actions"); len(got) != c.want {
			t.Errorf("%s: want %d findings, got %v", c.name, c.want, got)
		}
	}
}

// zizmor GHSA-f42p-wjw5-97qh: the token must not reach the logs, whatever the verbosity.
func TestPitfallTokenNeverInTheLogs(t *testing.T) {
	const token = "ghp_PitfallSecretToken0123456789abcdef"
	f := newFakeGitHub(t)
	githubForActionsCheckout(f, false)
	githubFailingFor(f, "corp/private", 403)
	setOnlineEnv(t, f, token)
	_, wf := onlineProject(t, workflowWith("uses: corp/private@v1", "uses: actions/checkout@v4"), "")
	_, stdout, stderr := runOnlineCommand(t, "--online", "--debug", "--verbose", wf)
	for name, text := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(text, token) {
			t.Errorf("the token is in %s", name)
		}
	}
	if !strings.Contains(stderr, "corp/private") {
		t.Errorf("the debug log should mention the skipped lookup:\n%s", stderr)
	}
}

// zizmor#1673: "secrets: inherit" belongs to the job which calls a reusable workflow. The documentation of GitHub and
// its workflow schema show only a mapping under on.workflow_call.secrets, so a scalar there stays a syntax error.
func TestPitfallSecretsInheritOnWorkflowCall(t *testing.T) {
	callee := "on:\n  workflow_call:\n    secrets: inherit\njobs:\n  j:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: echo\n"
	if got := errsWithID(lintFileWithConfig(t, pitfallConfig(), "ci.yaml", callee), "workflow-syntax"); len(got) != 1 || !strings.Contains(got[0].Message, `"secrets" section is scalar node but mapping node is expected`) {
		t.Errorf("the callee: %v", got)
	}
	caller := "on: push\njobs:\n  j:\n    uses: o/r/.github/workflows/w.yaml@v1\n    secrets: inherit\n"
	if got := errsWithID(lintFileWithConfig(t, pitfallConfig(), "ci.yaml", caller), "workflow-syntax"); len(got) != 0 {
		t.Errorf("the caller: %v", got)
	}
}
