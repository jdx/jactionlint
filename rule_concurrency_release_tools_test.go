package jactionlint

import (
	"strings"
	"testing"
)

const releaseToolsTestConfig = "profile: default\nrules:\n  missing-permissions: off\n  missing-timeout: off\n  unpinned-uses: off\n  excessive-permissions: off\n  artipacked: off\n  mutable-runner-label: off\n  unlocked-install: off\n  unpinned-tools: off\n"

// Any job that runs a publish command or pushes tags is a release job, whatever tool does it: the advice is
// cancel-in-progress false for it (issue 113: release-plz, semantic-release, changesets and so on).
func TestConcurrencyLimitsAdviceForReleaseToolsIsNotKeyedOnOneTool(t *testing.T) {
	cfg := mustParseConfig(t, releaseToolsTestConfig)
	tests := []struct {
		what    string
		run     string
		release bool
	}{
		{"release-plz release", "release-plz release", true},
		{"release-plz through a launcher", "npx release-plz release", true},
		{"release-plz release-pr only opens a pull request", "release-plz release-pr", false},
		{"semantic-release", "npx semantic-release", true},
		{"semantic-release dry run", "npx semantic-release --dry-run", false},
		{"changesets publish", "pnpm changeset publish", true},
		{"changesets version only edits files", "pnpm changeset version", false},
		{"cargo publish", "cargo publish", true},
		{"cargo release executes", "cargo release patch --execute", true},
		{"cargo release is a dry run without --execute", "cargo release patch", false},
		{"lerna publish", "npx lerna publish from-package", true},
		{"maven deploy", "mvn -B deploy", true},
		{"nuget push", "dotnet nuget push pkg.nupkg", true},
		{"git push --tags", "git push --tags", true},
		{"git push --follow-tags", "git push --follow-tags origin main", true},
		{"git push of a tag name", "git push origin v1.2.3", true},
		{"git push of a tag variable", "git push origin \"$TAG\"", true},
		{"git push of a tag refspec", "git push origin refs/tags/v1", true},
		{"git global option before push", "git -C sub push origin --tags", true},
		{"creating a tag and pushing the branch", "git tag \"$VERSION\"\ngit push origin HEAD", true},
		{"git push of a branch", "git push origin main", false},
		{"listing tags", "git tag -l\ngit push origin main", false},
		{"build", "make build", false},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := "on:\n  push:\n    branches: [main]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          " + strings.ReplaceAll(tc.run, "\n", "\n          ") + "\n"
			errs := errsWithID(lintWithConfig(t, cfg, src), "concurrency-limits")
			if len(errs) != 1 {
				t.Fatalf("want one finding but got %v", errs)
			}
			e := errs[0]
			cancelling := strings.Replace(src, "jobs:\n", "concurrency:\n  group: g\n  cancel-in-progress: true\njobs:\n", 1)
			got := errsWithID(lintWithConfig(t, cfg, cancelling), "concurrency-cancels-release")
			if tc.release {
				if !strings.Contains(e.Message, "cancel-in-progress: false") || strings.Contains(e.Message, "cancel-in-progress: true") || e.Fix != nil {
					t.Errorf("a release job must be advised cancel-in-progress false and get no fix: %v", e)
				}
				if len(got) != 1 {
					t.Errorf("cancel-in-progress true on a release job: %v", got)
				}
				return
			}
			if !strings.Contains(e.Message, "cancel-in-progress: true") {
				t.Errorf("a workflow that releases nothing is advised to cancel: %v", e)
			}
			if len(got) != 0 {
				t.Errorf("cancelling is fine for a job that releases nothing: %v", got)
			}
		})
	}
}

func TestConcurrencyLimitsAdviceForReleaseActions(t *testing.T) {
	cfg := mustParseConfig(t, releaseToolsTestConfig)
	for _, action := range []string{"release-plz/action@v0.5", "changesets/action@v1", "cycjimmy/semantic-release-action@v4"} {
		src := "on:\n  push:\n    branches: [main]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + action + "\n"
		errs := errsWithID(lintWithConfig(t, cfg, src), "concurrency-limits")
		if len(errs) != 1 || !strings.Contains(errs[0].Message, "cancel-in-progress: false") || errs[0].Fix != nil {
			t.Errorf("%s: %v", action, errs)
		}
	}
}

// A workflow with uses: jobs gets a group that cannot equal the group of the workflow it calls, where
// github.workflow is the name of the caller (issue 113).
func TestConcurrencyLimitsGroupOfACallerDiffersFromItsCallee(t *testing.T) {
	cfg := withoutMissingTimeout(mustParseConfig(t, "rules:\n  concurrency-limits: error\n"))
	const callerGroup = "${{ github.workflow }}-caller-${{ github.event.pull_request.number || github.ref }}"
	call := "jobs:\n  ci:\n    uses: ./.github/workflows/ci-impl.yml\n"

	pr := "on: pull_request\n" + call
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", pr), "concurrency-limits")
	if len(errs) != 1 || !strings.Contains(errs[0].Message, callerGroup) || errs[0].Fix == nil {
		t.Fatalf("a pull request caller: %v", errs)
	}
	got, n := applyFixes([]byte(pr), errs, FixModeSafe)
	want := "on: pull_request\n\nconcurrency:\n  group: " + callerGroup + "\n  cancel-in-progress: true\n" + call
	if n != 1 || string(got) != want {
		t.Errorf("fixed:\n%q\nwant:\n%q", got, want)
	}

	// A push workflow gets the same hint without a fix
	push := "on: push\n" + call
	errs = errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", push), "concurrency-limits")
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "-caller") || !strings.Contains(errs[0].Message, "called workflow") {
		t.Errorf("a push caller: %v", errs)
	}

	// A workflow without uses: jobs keeps the plain group and says nothing about callees
	plain := "on: pull_request\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n"
	errs = errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", plain), "concurrency-limits")
	if len(errs) != 1 || strings.Contains(errs[0].Message, "-caller") || strings.Contains(errs[0].Message, "called workflow") {
		t.Errorf("a workflow without reusable calls: %v", errs)
	}
}
