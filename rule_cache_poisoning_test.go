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
