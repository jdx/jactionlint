package jactionlint

import (
	"fmt"
	"strings"
	"testing"
)

func TestCredentialPath(t *testing.T) {
	yes := []string{
		"~/.docker/config.json", "~/.docker", "$HOME/.docker/config.json", "${HOME}/.docker/", "~/.npmrc", ".npmrc", "./.npmrc",
		"**/.npmrc", "~/.ssh", "~/.ssh/", "~/.ssh/id_ed25519", ".git", "./.git", ".git/", "~/.config/gh", "~/.config/gh/", ".env",
		"**/.env", ".env.local", "~/.aws", "~/.aws/", "~/.kube/config", "~/.netrc", "~/.pypirc", "~/.cargo/credentials.toml",
		"~/.m2/settings.xml", "'~/.npmrc'", `"~/.ssh"`, "  ~/.npmrc  ", "~/.gnupg", "~/.config/gcloud",
	}
	for _, p := range yes {
		if _, ok := credentialPath(p); !ok {
			t.Errorf("%q should be a credential path", p)
		}
	}
	no := []string{
		"", "~/.cache", "~/.npm", "node_modules", "~/.cargo/registry", "~/.cargo", "~/.m2/repository", "~/.ssh/known_hosts", "~/.docker/cli-plugins",
		".docker", ".docker/config.json", ".ssh", ".aws", ".kube/config", ".netrc", "~/.config/gh-cache", ".github", ".github/workflows", ".gitignore", ".git/modules", "!~/.npmrc", "**/*.env", "~/.env.d/x", ".",
		"${{ github.workspace }}/.npmrc", "~", "vendor/.env/x", "~/.config", "dist",
	}
	for _, p := range no {
		if what, ok := credentialPath(p); ok {
			t.Errorf("%q should not be a credential path, got %q", p, what)
		}
	}
}

// With the option pedantic, a cache whose path holds files with credentials is reported (zizmor#723).
func TestCachePoisoningCredentialCaches(t *testing.T) {
	cache := func(path string) string {
		return "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/cache@v4\n        with:\n          path: " + path + "\n          key: k\n"
	}
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"npmrc", cache("~/.npmrc"), 1},
		{"a list", cache("|\n            ~/.npm\n            ~/.docker/config.json\n            .env"), 2},
		{"the git directory with a checkout that persists", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - uses: actions/cache@v4\n        with:\n          path: .git\n          key: k\n", 1},
		{"the git directory with a checkout that does not", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/cache@v4\n        with:\n          path: .git\n          key: k\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n", 0},
		{"the git directory without a checkout", cache(".git"), 0},
		{"a directory of the workspace named like one of the home", cache(".docker"), 0},
		{"the ssh directory", cache("~/.ssh"), 1},
		{"a harmless path", cache("~/.npm"), 0},
		{"a cache directory next to credentials of its tool", cache("~/.cargo/registry"), 0},
		{"save", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/cache/save@v4\n        with:\n          path: ~/.config/gh\n          key: k\n", 1},
		{"restore only", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/cache/restore@v4\n        with:\n          path: ~/.npmrc\n          key: k\n", 0},
		{"another action", "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/upload-artifact@v4\n        with:\n          path: ~/.npmrc\n", 0},
		{"a pull request workflow too", strings.Replace(cache("~/.npmrc"), "on: push", "on: pull_request", 1), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, true, tc.src); got != tc.want {
				t.Errorf("pedantic: want %d findings, got %d", tc.want, got)
			}
			if got := cacheCount(t, false, tc.src); got != 0 {
				t.Errorf("not pedantic: want no finding, got %d", got)
			}
		})
	}
	t.Run("the position is the path", func(t *testing.T) {
		cfg := mustParseConfig(t, "profile: correctness\nrules:\n  cache-poisoning:\n    level: error\n    pedantic: true\n")
		errs := policyErrorsOf(lintWithConfig(t, cfg, cache("~/.npmrc")), "cache-poisoning")
		if len(errs) != 1 || errs[0].Line != 8 {
			t.Fatalf("want one finding on line 8, got %v", errs)
		}
		t.Log(fmt.Sprint(errs[0].Message))
	})
}

// The actions that restore a cache and had no entry.
func TestCachePoisoningMoreCacheActions(t *testing.T) {
	const on = "on:\n  push:\n    tags: ['v*']\n"
	step := func(uses, with string) string {
		s := on + "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n"
		if with != "" {
			s += "        with:\n" + with
		}
		return s
	}
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"pnpm/setup with the cache", step("pnpm/setup@v1", "          cache: true\n"), 1},
		{"pnpm/setup with an expression", step("pnpm/setup@v1", "          cache: ${{ inputs.cache }}\n"), 1},
		{"pnpm/setup without the cache", step("pnpm/setup@v1", ""), 0},
		{"pnpm/setup with the cache off", step("pnpm/setup@v1", "          cache: false\n"), 0},
		{"pnpm/setup that only does not save", step("pnpm/setup@v1", "          cache: true\n          save-cache: false\n"), 1},
		{"graalvm with a cache", step("graalvm/setup-graalvm@v1", "          cache: maven\n"), 1},
		{"graalvm without", step("graalvm/setup-graalvm@v1", "          java-version: 21\n"), 0},
		{"composer", step("ramsey/composer-install@v3", ""), 1},
		{"composer with the cache ignored", step("ramsey/composer-install@v3", "          ignore-cache: yes\n"), 0},
		{"composer with true", step("ramsey/composer-install@v3", "          ignore-cache: 'true'\n"), 0},
		{"composer with no", step("ramsey/composer-install@v3", "          ignore-cache: no\n"), 1},
		{"composer with another word", step("ramsey/composer-install@v3", "          ignore-cache: TRUE\n"), 1},
		{"composer with an expression", step("ramsey/composer-install@v3", "          ignore-cache: ${{ inputs.x }}\n"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheCount(t, false, tc.src); got != tc.want {
				t.Errorf("want %d findings, got %d", tc.want, got)
			}
		})
	}
	// An expression of the trigger in the gate input makes the cache conditional, as for the other actions
	t.Run("a gate on the trigger", func(t *testing.T) {
		src := step("pnpm/setup@v1", "          cache: ${{ !startsWith(github.ref, 'refs/tags/') }}\n")
		if got := cacheCount(t, false, src); got != 0 {
			t.Errorf("want no finding, got %d", got)
		}
		src = step("ramsey/composer-install@v3", "          ignore-cache: ${{ startsWith(github.ref, 'refs/tags/') }}\n")
		if got := cacheCount(t, false, src); got != 0 {
			t.Errorf("composer: want no finding, got %d", got)
		}
	})
}
