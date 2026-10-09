package jactionlint

import (
	"strings"
	"testing"
)

func prefixSrc(cond string) string {
	return "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        if: " + cond + "\n"
}

func TestUnsoundPrefixMatch(t *testing.T) {
	tests := []struct {
		cond string
		want int
	}{
		// Reported
		{"startsWith(github.actor, 'jdx')", 1},
		{"${{ startsWith(github.actor, 'jdx') }}", 1},
		{"STARTSWITH(github.actor, 'jdx')", 1},
		{"startsWith(github.triggering_actor, 'jdx')", 1},
		{"startsWith(github.event.sender.login, 'jdx')", 1},
		{"startsWith(github.repository_owner, 'jdx')", 1},
		{"startsWith(github.event.pull_request.user.login, 'jdx')", 1},
		{"endsWith(github.actor, 'jdx')", 1},
		{"contains(github.actor, 'jdx')", 1},
		{"startsWith(github.repository, 'jdx')", 1},
		{"endsWith(github.repository, '/mise')", 1},
		{"endsWith(github.repository, 'jdx/mise')", 1},
		{"contains(github.repository, 'jdx')", 1},
		{"startsWith(github.event.pull_request.head.repo.full_name, 'jdx')", 1},
		{"github.event_name == 'push' && startsWith(github.actor, 'a')", 1},
		{"startsWith(github.actor, 'a') || startsWith(github.actor, 'b')", 2},
		{"github.actor == 'jdx' || startsWith(github.actor, 'jdx-')", 1}, // the exact test is one alternative
		{"startsWith(github.actor, 'jdx') && github.actor != 'x'", 1},
		{"startsWith(github.actor, 'dependabot')", 1}, // bot-conditions is not enabled

		// Sound
		{"github.actor == 'jdx'", 0},
		{"startsWith(github.repository, 'jdx/')", 0},
		{"startsWith(github.repository, 'jdx/mise')", 0},
		{"startsWith(github.actor, 'dependabot[bot]')", 0},
		{"endsWith(github.actor, '[bot]')", 0},
		{"!startsWith(github.actor, 'jdx')", 0},
		{"!(startsWith(github.actor, 'jdx') || startsWith(github.actor, 'x'))", 0},
		{"!(!startsWith(github.actor, 'jdx'))", 1},
		{"startsWith(github.actor, '')", 0},
		{"startsWith(github.actor, inputs.user)", 0},
		{"startsWith(inputs.user, 'jdx')", 0},
		{"startsWith(github.event.head_commit.message, 'jdx')", 0},
		{"contains(github.event.head_commit.message, 'jdx')", 0},
		{"contains('jdx jdy', github.actor)", 0}, // unsound-contains reports it
		{"contains(fromJSON('[\"jdx\", \"jdy\"]'), github.actor)", 0},
		{"startsWith(github.ref, 'refs/tags/v')", 0}, // refs are not checked by default
		{"startsWith(github.head_ref, 'renovate')", 0},

		// An exact comparison of the same value in the same && chain decides who passes
		{"github.actor == 'jdx' && startsWith(github.actor, 'j')", 0},
		{"startsWith(github.actor, 'j') && github.actor == 'jdx'", 0},
		{"'jdx' == github.actor && (startsWith(github.actor, 'j') || github.event_name == 'push')", 0},
		{"contains(fromJSON('[\"jdx\"]'), github.actor) && startsWith(github.actor, 'j')", 0},
		{"github.actor == 'jdx' && github.event_name == 'push' && startsWith(github.actor, 'j')", 0},
		{"github.actor != 'jdx' && startsWith(github.actor, 'j')", 1},
		{"github.repository_owner == 'jdx' && startsWith(github.actor, 'j')", 1}, // a different value
		{"github.actor == github.repository_owner && startsWith(github.actor, 'j')", 1},
	}
	for _, tc := range tests {
		t.Run(tc.cond, func(t *testing.T) {
			cond := tc.cond
			if strings.HasPrefix(cond, "!") {
				cond = "${{ " + cond + " }}" // "!" starts a YAML tag
			}
			src := prefixSrc(cond)
			if strings.Contains(cond, `"`) { // keep the YAML valid
				src = prefixSrc("|\n          " + cond)
			}
			errs := errsWithID(lintFileWithConfig(t, ruleConfig("unsound-prefix-match"), "ci.yaml", src), "unsound-prefix-match")
			if len(errs) != tc.want {
				t.Errorf("want %d errors but got %d: %v", tc.want, len(errs), errs)
			}
			for _, e := range errs {
				if e.Fix != nil {
					t.Errorf("an unsound match has no mechanical fix: %v", e.Fix)
				}
			}
		})
	}
}

func TestUnsoundPrefixMatchPositionsAndMessages(t *testing.T) {
	cfg := ruleConfig("unsound-prefix-match")
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    if: |\n      github.event_name == 'push' &&\n      startsWith(github.actor, 'jdx')\n    steps:\n      - if: ${{ endsWith(github.repository, '/mise') }}\n        run: echo\n      - if: ${{ contains(github.repository_owner, 'jdx') }}\n        run: echo\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "unsound-prefix-match")
	if len(errs) != 3 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if errs[0].Line != 7 || errs[0].Column != 7 {
		t.Errorf("unexpected position of the first error: %v", errs[0])
	}
	for i, want := range []string{`"jdx-evil"`, `"evil/mise"`, `only contains "jdx"`} {
		if !strings.Contains(errs[i].Message, want) {
			t.Errorf("want %q in %q", want, errs[i].Message)
		}
	}
	if errs[0].Severity != SeverityError {
		t.Errorf("want error severity but got %v", errs[0].Severity)
	}
}

func TestUnsoundPrefixMatchBotConditions(t *testing.T) {
	// A comparison with a bot in a condition is reported by bot-conditions when it is enabled
	src := prefixSrc("startsWith(github.actor, 'dependabot')")
	both := ruleConfig("unsound-prefix-match", "bot-conditions")
	if got := errsWithID(lintFileWithConfig(t, both, "ci.yaml", src), "unsound-prefix-match"); len(got) != 0 {
		t.Errorf("bot-conditions is on but got %v", got)
	}
	if got := errsWithID(lintFileWithConfig(t, both, "ci.yaml", src), "bot-conditions"); len(got) != 1 {
		t.Errorf("want a bot-conditions error but got %v", got)
	}
	// Other names are still reported
	src = prefixSrc("startsWith(github.actor, 'jdx')")
	if got := errsWithID(lintFileWithConfig(t, both, "ci.yaml", src), "unsound-prefix-match"); len(got) != 1 {
		t.Errorf("want 1 error but got %v", got)
	}
}

func TestUnsoundPrefixMatchRefsOption(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  unsound-prefix-match:\n    level: error\n    refs: true\n")
	tests := map[string]int{
		"startsWith(github.ref, 'refs/tags/v')":                                     1,
		"startsWith(github.ref, 'refs/heads/release/')":                             0,
		"contains(github.head_ref, 'dependabot/')":                                  1,
		"endsWith(github.head_ref, '/dependabot/')":                                 1,
		"startsWith(github.head_ref, 'renovate')":                                   1,
		"endsWith(github.ref_name, '-rc')":                                          1,
		"github.ref == 'refs/heads/main' && startsWith(github.ref, 'refs/heads/m')": 0,
	}
	for cond, want := range tests {
		errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", prefixSrc(cond)), "unsound-prefix-match")
		if len(errs) != want {
			t.Errorf("%s: want %d errors but got %v", cond, want, errs)
		}
	}
}

func TestUnsoundPrefixMatchSites(t *testing.T) {
	cfg := ruleConfig("unsound-prefix-match")
	// Outside conditions, only a test which selects a secret, the token or a self-hosted runner is a
	// trust decision
	src := "on: push\njobs:\n  a:\n    runs-on: ${{ startsWith(github.repository, 'jdx') && 'self-hosted' || 'ubuntu-latest' }}\n    env:\n      GOOS: ${{ contains(github.repository, 'windows_exporter') && 'windows' || '' }}\n      TRUSTED: ${{ startsWith(github.actor, 'jdx') }}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          token: ${{ startsWith(github.actor, 'jdx') && secrets.TOKEN }}\n      - run: echo\n        env:\n          T: ${{ endsWith(github.repository, '/x') && github.token || '' }}\n"
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", src), "unsound-prefix-match", 4, 11, 14)
	// On by default
	if got := errsWithID(lintFileWithConfig(t, defaultProfileConfig(), "ci.yaml", prefixSrc("startsWith(github.actor, 'jdx')")), "unsound-prefix-match"); len(got) != 1 {
		t.Errorf("want a finding with the default configuration: %v", got)
	}
}

// A "self-hosted" inside the pattern being tested does not select a runner; runs-on and a comparison
// with runner.* do.
func TestUnsoundPrefixMatchSelfHostedLiteral(t *testing.T) {
	cfg := ruleConfig("unsound-prefix-match")
	env := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      BUILD: ${{ startsWith(matrix.os, 'self-hosted-') && 'fast' || 'slow' }}\n      B2: ${{ contains(github.repository, 'self-hosted') }}\n    steps:\n      - run: echo\n"
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", env), "unsound-prefix-match")
	runsOn := "on: push\njobs:\n  a:\n    runs-on: ${{ startsWith(github.repository, 'jdx') && matrix.os || 'ubuntu-latest' }}\n    steps:\n      - run: echo\n"
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", runsOn), "unsound-prefix-match", 4)
	cmp := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      X: ${{ startsWith(github.actor, 'jdx') && runner.environment == 'self-hosted' }}\n    steps:\n      - run: echo\n"
	wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", cmp), "unsound-prefix-match", 6)
}

// The credentials reached by an index or as a whole are credentials too.
func TestUnsoundPrefixMatchSelectsCredentialsByIndex(t *testing.T) {
	cfg := ruleConfig("unsound-prefix-match")
	for _, v := range []string{"secrets['TOKEN']", "github['token']", "toJSON(secrets)", "secrets[inputs.name]"} {
		src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n        env:\n          T: ${{ startsWith(github.actor, 'jdx') && " + v + " }}\n"
		wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", src), "unsound-prefix-match", 8)
	}
}
