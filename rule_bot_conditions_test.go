package jactionlint

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func botConfig(t *testing.T) *Config {
	t.Helper()
	return mustParseConfig(t, "rules:\n  bot-conditions: warn\n")
}

func TestBotConditionsFix(t *testing.T) {
	tests := []struct {
		name   string
		on     string
		cond   string
		want   string // the condition after the unsafe fix
		unsafe bool   // whether a fix exists at all
	}{
		{"bare", "pull_request_target", "github.actor == 'dependabot[bot]'", "github.event.pull_request.user.login == 'dependabot[bot]'", true},
		{"wrapped", "pull_request", "${{ github.actor == 'dependabot[bot]' && github.repository == 'a/b' }}", "${{ github.event.pull_request.user.login == 'dependabot[bot]' && github.repository == 'a/b' }}", true},
		{"id", "pull_request", "github.actor_id == 49699333", "github.event.pull_request.user.id == 49699333", true},
		{"sender", "pull_request", "${{ 'dependabot[bot]' == github.event.sender.login }}", "${{ 'dependabot[bot]' == github.event.pull_request.user.login }}", true},
		{"other events have no pull request", "[pull_request, push]", "github.actor == 'dependabot[bot]'", "github.actor == 'dependabot[bot]'", false},
		{"push", "push", "github.actor == 'dependabot[bot]'", "github.actor == 'dependabot[bot]'", false},
		{"no replacement for the node ID", "pull_request", "github.event.sender.node_id == 'dependabot[bot]'", "github.event.sender.node_id == 'dependabot[bot]'", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "on: " + tc.on + "\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: " + tc.cond + "\n    steps:\n      - run: echo\n"
			errs := lintWithConfig(t, botConfig(t), src)
			if len(errs) != 1 || errs[0].ID != "bot-conditions" {
				t.Fatalf("want one bot-conditions error but got %v", errs)
			}
			if (errs[0].Fix != nil) != tc.unsafe {
				t.Fatalf("fix = %+v", errs[0].Fix)
			}
			if tc.unsafe && !errs[0].Fix.Unsafe {
				t.Error("the fix must be unsafe")
			}
			// An unsafe fix is not applied by default
			if out, n := applyFixes([]byte(src), errs, FixModeSafe); n != 0 || string(out) != src {
				t.Errorf("unsafe fix was applied in the safe mode")
			}
			want := "on: " + tc.on + "\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: " + tc.want + "\n    steps:\n      - run: echo\n"
			got, rest := fixAll(t, botConfig(t), src, FixModeUnsafe)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("(-want +got): %s", diff)
			}
			if tc.unsafe && len(rest) != 0 {
				t.Errorf("errors remain after fixing: %v", rest)
			}
		})
	}
}

func TestBotConditionsOnlyTheFirstOfACondition(t *testing.T) {
	src := "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.actor == 'dependabot[bot]' || github.actor == 'renovate[bot]'\n    steps:\n      - run: echo\n"
	errs := lintWithConfig(t, botConfig(t), src)
	if len(errs) != 1 {
		t.Fatalf("want one error but got %v", errs)
	}
	got, rest := fixAll(t, botConfig(t), src, FixModeUnsafe)
	want := "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    if: github.event.pull_request.user.login == 'dependabot[bot]' || github.event.pull_request.user.login == 'renovate[bot]'\n    steps:\n      - run: echo\n"
	if got != want || len(rest) != 0 {
		t.Errorf("want %q but got %q (%v)", want, got, rest)
	}
}
