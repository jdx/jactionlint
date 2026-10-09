package jactionlint

import (
	"strings"
	"testing"
)

func TestRuleConcurrencyCancelsRelease(t *testing.T) {
	const steps = "    steps:\n      - run: echo\n"
	job := "jobs:\n  t:\n    runs-on: ubuntu-latest\n" + steps
	cancel := "concurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\n"
	tests := []struct {
		what string
		src  string
		want []string
	}{
		{"tag push with a group that is the same for all tags", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: release\n  cancel-in-progress: true\n" + job, []string{"6"}},
		{"tag push with the tag in the group", "on:\n  push:\n    tags: ['v*']\n" + cancel + job, nil},
		{"tag push with the tag in the group and a publish", "on:\n  push:\n    tags: ['v*']\n" + cancel + "jobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish\n", nil},
		{"release event", "on:\n  release:\n    types: [published]\nconcurrency:\n  group: ${{ github.workflow }}\n  cancel-in-progress: true\n" + job, []string{"6"}},
		{"push to a release branch of a test workflow", "on:\n  push:\n    branches: [release/**]\n" + cancel + job, nil},
		{"push to main of a test workflow", "on:\n  push:\n    branches: [main]\n" + cancel + job, nil},
		{"bare push of a test workflow", "on: push\n" + cancel + job, nil},
		{"pull request only", "on: pull_request\n" + cancel + "jobs:\n  t:\n    runs-on: ubuntu-latest\n    environment: preview\n" + steps, nil},
		{"cancel false", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: release\n  cancel-in-progress: false\n" + job, nil},
		{"no cancel", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: release\n" + job, nil},
		{"mixed idiom on tags", "on:\n  pull_request:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\n" + job, nil},
		{"mixed idiom on main with an environment", "on:\n  pull_request:\n  push:\n    branches: [main]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: ${{ github.event_name == 'pull_request' }}\njobs:\n  d:\n    runs-on: ubuntu-latest\n    environment: production\n" + steps, nil},
		{"not on main", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}\njobs:\n  d:\n    runs-on: ubuntu-latest\n    environment: production\n" + steps, nil},
		{"not a tag", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: ${{ !startsWith(github.ref, 'refs/tags/') }}\n" + job, nil},
		{"not a tag by ref_type", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: ${{ github.ref_type != 'tag' }}\n" + job, nil},
		{"expression that depends on something unknown", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: ${{ github.workflow }}\n  cancel-in-progress: ${{ vars.CANCEL }}\n" + job, nil},
		{"expression that is true for tags", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: ${{ github.workflow }}\n  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}\n" + job, []string{"6"}},
		{"reusable workflow with a literal cancel", "on: workflow_call\nconcurrency:\n  group: x\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", []string{"4"}},
		{"the event of the caller is not known", "on: workflow_call\nconcurrency:\n  group: x\n  cancel-in-progress: ${{ github.event_name != 'release' }}\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", nil},
		{"input decides", "on:\n  workflow_call:\n    inputs:\n      tag:\n        type: string\nconcurrency:\n  group: ${{ inputs.tag }}\n  cancel-in-progress: ${{ github.event_name != 'release' && inputs.tag == '' }}\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", nil},
		{"environment job on main", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  d:\n    runs-on: ubuntu-latest\n    environment:\n      name: production\n" + steps, []string{"6"}},
		{"job level environment", "on:\n  push:\n    branches: [main]\njobs:\n  d:\n    runs-on: ubuntu-latest\n    environment: production\n    concurrency:\n      group: deploy\n      cancel-in-progress: true\n" + steps, []string{"10"}},
		{"job level without a signal in a tag workflow", "on:\n  push:\n    tags: ['v*']\njobs:\n  t:\n    runs-on: ubuntu-latest\n    concurrency:\n      group: test\n      cancel-in-progress: true\n" + steps, nil},
		{"goreleaser release with another skip", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip=validate\n", []string{"6"}},
		{"goreleaser release with skip-validate", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip-validate\n", []string{"6"}},
		{"goreleaser release skipping the changelog and the sign step", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip=changelog,sign\n", []string{"6"}},
		{"goreleaser release skipping publish", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip=publish\n", nil},
		{"goreleaser release skipping publish among others", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip=validate,publish\n", nil},
		{"goreleaser release --skip publish", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip publish\n", nil},
		{"goreleaser release --skip-publish", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip-publish\n", nil},
		{"goreleaser release snapshot", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --snapshot\n", nil},
		{"goreleaser action with another skip", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: goreleaser/goreleaser-action@v6\n        with:\n          args: release --clean --skip=validate\n", []string{"6"}},
		{"goreleaser action skipping publish", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: goreleaser/goreleaser-action@v6\n        with:\n          args: release --clean --skip=publish\n", nil},
		{"inputs do not scope a tag push", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: release-${{ inputs.tag }}\n  cancel-in-progress: true\n" + job, []string{"6"}},
		{"event inputs do not scope a tag push", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: release-${{ github.event.inputs.tag }}\n  cancel-in-progress: true\n" + job, []string{"6"}},
		{"inputs scope a manual run", "on:\n  workflow_dispatch:\n    inputs:\n      tag:\n        type: string\nconcurrency:\n  group: release-${{ inputs.tag }}\n  cancel-in-progress: true\n" + "jobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish\n", nil},
		{"the release of the event scopes a release event", "on:\n  release:\n    types: [published]\nconcurrency:\n  group: ${{ github.event.release.tag_name }}\n  cancel-in-progress: true\n" + job, nil},
		{"the release of the event does not scope a tag push", "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: ${{ github.event.release.tag_name }}\n  cancel-in-progress: true\n" + job, []string{"6"}},
		{"cargo publish", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish\n", []string{"6"}},
		{"cargo publish dry run", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: cargo publish --dry-run\n", nil},
		{"release action", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: softprops/action-gh-release@v2\n", []string{"6"}},
		{"docker push", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v6\n        with:\n          push: true\n", []string{"6"}},
		{"docker build without push", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v6\n", nil},
		{"names are not signals", "name: Release\non:\n  workflow_dispatch:\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  deploy-docs:\n    runs-on: ubuntu-latest\n" + steps, nil},
		{"wrangler deploy", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: deploy\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: bunx wrangler deploy\n", []string{"6"}},
		{"docker push", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: docker push ghcr.io/x/y\n", []string{"6"}},
		{"a command that only mentions deploy", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo wrangler deploy\n", nil},
		{"dispatch without a signal", "on: workflow_dispatch\n" + cancel + job, nil},
		{"manual run with the ref in the group", "on: workflow_dispatch\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", nil},
		{"manual run with one group for all", "on: workflow_dispatch\nconcurrency:\n  group: deploy\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", []string{"4"}},
		{"group of the release input", "on:\n  workflow_dispatch:\n    inputs:\n      tag:\n        type: string\nconcurrency:\n  group: release-${{ inputs.tag }}\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n", nil},
		{"release step only for tags on a branch push", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: softprops/action-gh-release@v2\n        if: startsWith(github.ref, 'refs/tags/')\n", nil},
		{"release step for every push", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ${{ github.workflow }}-${{ github.ref }}\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: softprops/action-gh-release@v2\n", []string{"6"}},
		{"job only for pushes", "on:\n  merge_group:\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ github.event_name == 'merge_group' }}\njobs:\n  d:\n    if: github.event_name == 'push'\n    runs-on: ubuntu-latest\n    environment: production\n" + steps, nil},
		{"job for merge groups", "on:\n  merge_group:\nconcurrency:\n  group: ci\n  cancel-in-progress: ${{ github.event_name == 'merge_group' }}\njobs:\n  d:\n    if: github.event_name == 'merge_group'\n    runs-on: ubuntu-latest\n    environment: production\n" + steps, []string{"5"}},
		{"goreleaser snapshot", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: goreleaser/goreleaser-action@v6\n        with:\n          args: release --snapshot --clean\n", nil},
		{"goreleaser check", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: goreleaser/goreleaser-action@v6\n        with:\n          args: check\n", nil},
		{"goreleaser release command", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --clean\n", []string{"6"}},
		{"goreleaser dry run command", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: goreleaser release --skip=publish --snapshot --clean\n", nil},
		{"goreleaser release", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: goreleaser/goreleaser-action@v6\n        with:\n          args: release --clean\n", []string{"6"}},
		{"docker push by an expression", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: docker/build-push-action@v6\n        with:\n          push: ${{ github.event_name != 'pull_request' }}\n", []string{"6"}},
		{"windows default shell is not bash", "on:\n  push:\n    branches: [main]\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  t:\n    runs-on: windows-latest\n    steps:\n      - run: cargo publish\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			checkLines(t, lintBatchH(t, "", tc.src, "concurrency-cancels-release"), tc.want...)
		})
	}
}

func TestRuleConcurrencyCancelsReleaseFix(t *testing.T) {
	const tail = "jobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n"
	head := "on:\n  push:\n    tags: ['v*']\nconcurrency:\n  group: release\n"
	tests := []struct {
		what   string
		cancel string
		want   string // the replaced line, "" if there is no fix
	}{
		{"literal", "cancel-in-progress: true", "cancel-in-progress: false"},
		{"upper case", "cancel-in-progress: True", "cancel-in-progress: false"},
		{"comment is kept", "cancel-in-progress: true # keep", "cancel-in-progress: false # keep"},
		{"expression has no fix", "cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}", ""},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := head + "  " + tc.cancel + "\n" + tail
			errs := lintBatchH(t, "", src, "concurrency-cancels-release")
			if len(errs) != 1 {
				t.Fatalf("want 1 error but got %v", errs)
			}
			f := errs[0].Fix
			if tc.want == "" {
				if f != nil {
					t.Fatalf("want no fix but got %+v", f)
				}
				return
			}
			if f == nil || !f.Unsafe {
				t.Fatalf("want an unsafe fix but got %+v", f)
			}
			if out, n := applyFixes([]byte(src), errs, FixModeSafe); n != 0 || string(out) != src {
				t.Errorf("a safe fix run applied an unsafe fix")
			}
			out, n := applyFixes([]byte(src), errs, FixModeUnsafe)
			if n != 1 {
				t.Fatalf("applied %d fixes", n)
			}
			want := head + "  " + tc.want + "\n" + tail
			if string(out) != want {
				t.Errorf("want\n%s\nbut got\n%s", want, out)
			}
			if again := lintBatchH(t, "", string(out), "concurrency-cancels-release"); len(again) != 0 {
				t.Errorf("the fixed workflow is still reported: %v", again)
			}
			if strings.Contains(string(out), "true") && !strings.Contains(tc.want, "true") {
				t.Errorf("true is still there:\n%s", out)
			}
		})
	}
}

func TestRuleConcurrencyCancelsReleaseFixFlowAndCRLF(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src := strings.ReplaceAll("on:\n  release:\nconcurrency: {group: release, cancel-in-progress: true}\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo\n", "\n", nl)
		errs := lintBatchH(t, "", src, "concurrency-cancels-release")
		if len(errs) != 1 || errs[0].Fix == nil {
			t.Fatalf("want one fixable error but got %v", errs)
		}
		out, _ := applyFixes([]byte(src), errs, FixModeUnsafe)
		want := strings.Replace(src, "cancel-in-progress: true", "cancel-in-progress: false", 1)
		if string(out) != want {
			t.Errorf("want %q but got %q", want, out)
		}
	}
}
