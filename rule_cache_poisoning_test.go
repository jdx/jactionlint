package jactionlint

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// lintCacheWorkflow lints the workflow as a file of the repository at root ("" for no repository).
func lintCacheWorkflow(t *testing.T, root, src string) []*Error {
	t.Helper()
	l, err := NewLinter(io.Discard, &LinterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.defaultConfig = withoutMissingTimeout(fixtureConfig("cache-poisoning"))
	var proj *Project
	if root != "" {
		proj = &Project{root: root}
	}
	errs, err := l.Lint("test.yaml", []byte(src), proj)
	if err != nil {
		t.Fatal(err)
	}
	return policyErrorsOf(errs, "cache-poisoning")
}

func TestCachePoisoningTagFilters(t *testing.T) {
	const tail = "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: Swatinem/rust-cache@v2\n"
	tests := []struct {
		name string
		on   string
		want int
	}{
		{"a tag pattern", "on:\n  push:\n    tags: ['v*']\n", 1},
		{"every tag excluded", "on:\n  push:\n    tags: ['!**']\n", 0},
		{"negative patterns only", "on:\n  push:\n    tags: ['!v1', '!v2']\n", 0},
		{"a pattern and then everything excluded", "on:\n  push:\n    tags: ['v*', '!**']\n", 0},
		{"a pattern with an exception", "on:\n  push:\n    tags: ['v*', '!v1-beta']\n", 1},
		{"an exception before the pattern", "on:\n  push:\n    tags: ['!v1-beta', 'v*']\n", 1},
		{"the release event next to excluded tags", "on:\n  release:\n    types: [published]\n  push:\n    tags: ['!**']\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lintCacheWorkflow(t, "", tc.on+tail); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningChecksThatCannotPublish(t *testing.T) {
	const on = "on:\n  push:\n    tags: ['v*']\n"
	const rc = "      - uses: Swatinem/rust-cache@v2\n"
	job := func(extra, steps string) string {
		return "jobs:\n  j:\n    runs-on: ubuntu-latest\n" + extra + "    steps:\n" + rc + steps
	}
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"no permissions: the token may write", on + job("", ""), 1},
		{"read-all", on + "permissions: read-all\n" + job("", ""), 0},
		{"contents read", on + "permissions:\n  contents: read\n" + job("", ""), 0},
		{"no permissions at all", on + "permissions: {}\n" + job("", ""), 0},
		{"contents write", on + "permissions:\n  contents: write\n" + job("", ""), 1},
		{"write-all", on + "permissions: write-all\n" + job("", ""), 1},
		{"a job that writes", on + "permissions: read-all\n" + job("    permissions:\n      packages: write\n", ""), 1},
		{"a job that drops the write permission of the workflow", on + "permissions: write-all\n" + job("    permissions: read-all\n", ""), 0},
		{"an environment", on + "permissions: read-all\n" + job("    environment: production\n", ""), 1},
		{"a secret of the repository", on + "permissions: read-all\n" + job("", "      - run: ./deploy\n        env:\n          TOKEN: ${{ secrets.DEPLOY_TOKEN }}\n"), 1},
		{"a secret of the workflow env", on + "permissions: read-all\nenv:\n  T: ${{ secrets.NPM_TOKEN }}\n" + job("", ""), 1},
		{"an input with a secret", on + "permissions: read-all\n" + job("", "      - uses: some/action@v1\n        with:\n          t: ${{ secrets.X }}\n"), 1},
		{"all the secrets", on + "permissions: read-all\n" + job("", "      - run: echo '${{ toJSON(secrets) }}'\n"), 1},
		{"only the token of the workflow", on + "permissions: read-all\n" + job("", "      - run: ./check\n        env:\n          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n"), 0},
		{"only the token of the workflow, in brackets", on + "permissions: read-all\n" + job("", "      - run: ./check\n        env:\n          GH_TOKEN: ${{ secrets['GITHUB_TOKEN'] }}\n"), 0},
		{"another secret, in brackets", on + "permissions: read-all\n" + job("", "      - run: ./check\n        env:\n          GH_TOKEN: ${{ secrets['NPM_TOKEN'] }}\n"), 1},
		{"a publishing job is judged with the tag of the run", on + "permissions: read-all\n" + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm publish\n      - uses: actions/setup-node@v4\n        if: github.ref_type == 'tag'\n        with:\n          cache: npm\n", 1},
		{"a publishing command", on + "permissions: read-all\n" + job("", "      - run: cargo publish\n"), 1},
		{"a publishing action", on + "permissions: read-all\n" + job("", "      - uses: pypa/gh-action-pypi-publish@release/v1\n"), 1},
		{"the release event always counts", "on:\n  release:\n    types: [published]\npermissions: read-all\n" + job("", ""), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lintCacheWorkflow(t, "", tc.src); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningAutomaticCaches(t *testing.T) {
	const head = "on:\n  release:\n    types: [published]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const sha = "49933ea5288caeca8642d1e84afbd3f7d6820020"
	tests := []struct {
		name string
		pkg  string // package.json of the repository, "-" for a repository without one, "" for no repository
		step string
		want int
	}{
		{"no repository to look at", "", "      - uses: actions/setup-node@v5\n", 1},
		{"v4 has no automatic cache", "", "      - uses: actions/setup-node@v4\n", 0},
		{"opted out", "", "      - uses: actions/setup-node@v5\n        with:\n          package-manager-cache: false\n", 0},
		{"a package manager is named", `{"packageManager": "pnpm@9.0.0"}`, "      - uses: actions/setup-node@v5\n", 1},
		{"v6 caches npm only", `{"packageManager": "pnpm@9.0.0"}`, "      - uses: actions/setup-node@v6\n", 0},
		{"v6 and npm", `{"packageManager": "npm@10.0.0"}`, "      - uses: actions/setup-node@v6\n", 1},
		{"devEngines", `{"devEngines": {"packageManager": {"name": "npm"}}}`, "      - uses: actions/setup-node@v6\n", 1},
		{"v5 reads only the top-level field", `{"devEngines": {"packageManager": {"name": "npm"}}}`, "      - uses: actions/setup-node@v5\n", 0},
		{"v6 reads devEngines before the top-level field", `{"packageManager": "pnpm@9.0.0", "devEngines": {"packageManager": {"name": "npm"}}}`, "      - uses: actions/setup-node@v6\n", 1},
		{"v6 and a list in devEngines", `{"devEngines": {"packageManager": [{"name": "pnpm"}, {"name": "npm"}]}}`, "      - uses: actions/setup-node@v6\n", 1},
		{"v6 and a caret in the top-level field", `{"packageManager": "^npm@10"}`, "      - uses: actions/setup-node@v6\n", 1},
		{"v5 needs a version after the name", `{"packageManager": "pnpm"}`, "      - uses: actions/setup-node@v5\n", 0},
		{"no package manager in package.json", `{"name": "x"}`, "      - uses: actions/setup-node@v5\n", 0},
		{"no package.json", "-", "      - uses: actions/setup-node@v5\n", 0},
		{"a commit with the version in its comment", `{"packageManager": "npm@10.0.0"}`, "      - uses: actions/setup-node@" + sha + " # v4.4.0\n", 0},
		{"buildx", "", "      - uses: docker/setup-buildx-action@v3\n", 1},
		{"buildx without the binary cache", "", "      - uses: docker/setup-buildx-action@v3\n        with:\n          cache-binary: false\n", 0},
		{"buildx v2", "", "      - uses: docker/setup-buildx-action@v2\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := ""
			if tc.pkg != "" {
				root = t.TempDir()
				if tc.pkg != "-" {
					if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(tc.pkg), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if got := lintCacheWorkflow(t, root, head+tc.step); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningGatesOfRestoreSwitches(t *testing.T) {
	const head = "on:\n  push:\n    tags: ['v*']\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	off := "${{ !startsWith(github.ref, 'refs/tags/') }}"
	tests := []struct {
		name string
		step string
		want int
	}{
		{"buildx binary cached", "      - uses: docker/setup-buildx-action@v3\n", 1},
		{"buildx binary cache off on tags", "      - uses: docker/setup-buildx-action@v3\n        with:\n          cache-binary: " + off + "\n", 0},
		{"buildx binary cache on", "      - uses: docker/setup-buildx-action@v3\n        with:\n          cache-binary: ${{ startsWith(github.ref, 'refs/tags/') }}\n", 1},
		{"node automatic cache off on tags", "      - uses: actions/setup-node@v5\n        with:\n          package-manager-cache: " + off + "\n", 0},
		{"node automatic cache on", "      - uses: actions/setup-node@v5\n        with:\n          package-manager-cache: ${{ startsWith(github.ref, 'refs/tags/') }}\n", 1},
		{"node automatic cache off next to cache: false", "      - uses: actions/setup-node@v5\n        with:\n          cache: false\n          package-manager-cache: " + off + "\n", 0},
		{"node automatic cache on next to cache: false", "      - uses: actions/setup-node@v5\n        with:\n          cache: false\n          package-manager-cache: ${{ startsWith(github.ref, 'refs/tags/') }}\n", 1},
		{"the ref of a pushed tag from the payload", "      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.event.ref, 'refs/tags/')\n", 1},
		{"the ref of a pushed tag from the payload, negated", "      - uses: Swatinem/rust-cache@v2\n        if: ${{ !startsWith(github.event.ref, 'refs/tags/') }}\n", 0},
		{"node explicit cache is not switched off by it", "      - uses: actions/setup-node@v5\n        with:\n          cache: npm\n          package-manager-cache: " + off + "\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lintCacheWorkflow(t, "", head+tc.step); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

func TestCachePoisoningUnknownPayloadRefDoesNotHideTheCache(t *testing.T) {
	// the payload of a release has no ref, so the condition cannot be evaluated: it must not count as a gate
	const head = "on:\n  release:\n    types: [published]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	for _, cond := range []string{"startsWith(github.event.ref, 'refs/tags/')", "github.event.ref == 'refs/tags/v1'"} {
		src := head + "      - uses: Swatinem/rust-cache@v2\n        if: " + cond + "\n"
		if got := lintCacheWorkflow(t, "", src); len(got) != 1 {
			t.Errorf("%s: want 1 finding but got %v", cond, got)
		}
	}
}

func TestCachePoisoningTagOnlyPushHasNoBranchRun(t *testing.T) {
	// the only run of this workflow is a tag push, so a step kept away from tags never restores the cache
	const head = "on:\n  push:\n    tags: ['v*']\npermissions: read-all\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	const tail = "      - run: docker push example/img\n"
	step := "      - uses: Swatinem/rust-cache@v2\n        if: ${{ !startsWith(github.event.ref, 'refs/tags/') }}\n"
	if got := lintCacheWorkflow(t, "", head+step+tail); len(got) != 0 {
		t.Errorf("tag-only push: want no finding but got %v", got)
	}
	// with a branch filter a branch is pushed too, and the step runs there
	branches := "on:\n  push:\n    branches: [main]\n    tags: ['v*']\n" + head[len("on:\n  push:\n    tags: ['v*']\n"):]
	if got := lintCacheWorkflow(t, "", branches+step+tail); len(got) != 1 {
		t.Errorf("branches and tags: want 1 finding but got %v", got)
	}
}

func TestCachePoisoningBranchPushKnowsItsRefIsNotATag(t *testing.T) {
	// a branch push has a ref of refs/heads/*: a condition which asks for a tag is false there, even though the
	// name of the branch is not known
	const head = "on:\n  push:\n    branches: [main]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    permissions: read-all\n    steps:\n"
	const tail = "      - run: docker push example/img\n"
	tests := []struct {
		name string
		step string
		want int
	}{
		{"step for tags only, payload ref", "      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.event.ref, 'refs/tags/')\n", 0},
		{"step for tags only, ref", "      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.ref, 'refs/tags/')\n", 0},
		{"input for tags only", "      - uses: docker/setup-buildx-action@v3\n        with:\n          cache-binary: ${{ startsWith(github.event.ref, 'refs/tags/') }}\n", 0},
		{"step for tags only, equality", "      - uses: Swatinem/rust-cache@v2\n        if: github.event.ref == 'refs/tags/v1'\n", 0},
		{"step for branches", "      - uses: Swatinem/rust-cache@v2\n        if: startsWith(github.event.ref, 'refs/heads/')\n", 1},
		{"step for one branch", "      - uses: Swatinem/rust-cache@v2\n        if: github.event.ref == 'refs/heads/main'\n", 1},
		{"step not for tags", "      - uses: Swatinem/rust-cache@v2\n        if: ${{ !startsWith(github.event.ref, 'refs/tags/') }}\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lintCacheWorkflow(t, "", head+tc.step+tail); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}
