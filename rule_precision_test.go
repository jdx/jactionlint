package jactionlint

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// precisionStep wraps the steps in a workflow with the given trigger.
func precisionWorkflow(on, steps string) string {
	return "on: " + on + "\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + steps
}

func TestAdhocPackagesArraysAndLerna(t *testing.T) {
	tests := []struct {
		what string
		run  string
		want int
	}{
		{"filters from an array", "filters=()\n          pnpm install --frozen-lockfile \"${filters[@]}\"", 0},
		{"all arguments", "pnpm install --frozen-lockfile \"$@\"", 0},
		{"a package next to an array", "npm install left-pad \"${flags[@]}\"", 1},
		{"yarn lerna add", "yarn lerna add left-pad --scope web", 1},
		{"lerna add with an expression", "yarn lerna add pkg@${{ matrix.v }}", 1},
		{"npx lerna add", "npx lerna add left-pad", 1},
		{"echo of a lerna add", "echo lerna add left-pad", 0},
		{"grep for lerna add", "grep lerna add README.md", 0},
		{"lerna add of a local path", "lerna add ./packages/a", 0},
		{"lerna add of a local path with --scope", "lerna add ./packages/a --scope web", 0},
		{"lerna add of a local path with --dev and --peer", "lerna add ./packages/a --dev --peer", 0},
		{"lerna add with --scope before a local path", "lerna add --scope web ./packages/a", 0},
		{"lerna add with --scope before a package", "lerna add --scope web left-pad", 1},
		{"lerna add with a flag value after a registry package", "lerna add left-pad --scope web --dev", 1},
		{"lerna add with --scope=value before a package", "lerna add --scope=web left-pad", 1},
		{"lerna bootstrap", "yarn lerna bootstrap", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := precisionWorkflow("push", "      - run: |\n          "+tc.run+"\n")
			if got := countBatchD(t, "adhoc-packages", src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, src)
			}
		})
	}
}

func TestUseTrustedPublishingPrecision(t *testing.T) {
	const jobHead = "on: push\npermissions:\n  id-token: write\njobs:\n  j:\n    runs-on: %s\n    steps:\n"
	win := strings.Replace(jobHead, "%s", "windows-latest", 1)
	lin := strings.Replace(jobHead, "%s", "ubuntu-latest", 1)
	const noOIDC = "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		what string
		src  string
		want int
	}{
		// false positives
		{"nuget key from the login step on windows", win + `      - id: login
        uses: NuGet/login@v1
        with:
          user: me
      - run: dotnet nuget push a.nupkg -k ${{ steps.login.outputs.NUGET_API_KEY }} -s https://api.nuget.org/v3/index.json
`, 0},
		{"nuget secret key on windows", win + `      - run: dotnet nuget push a.nupkg -k ${{ secrets.NUGET_KEY }}
`, 1},
		{"github packages through setup-node", lin + `      - uses: actions/setup-node@v6
        with:
          registry-url: https://npm.pkg.github.com
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
`, 0},
		{"public registry through setup-node", lin + `      - uses: actions/setup-node@v6
        with:
          registry-url: https://registry.npmjs.org
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
`, 1},
		{"local registry set with yarn config", noOIDC + `      - run: |
          yarn config set npmRegistryServer http://localhost:4873
          yarn npm publish
        env:
          YARN_NPM_AUTH_TOKEN: ${{ secrets.T }}
`, 0},
		{"local registry set with npm config", noOIDC + `      - run: |
          npm config set registry http://localhost:4873
          npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 0},
		{"registry in the environment", noOIDC + `      - run: npm publish
        env:
          NPM_CONFIG_REGISTRY: http://localhost:4873
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 0},
		{"npm config for the public registry", noOIDC + `      - run: |
          npm config set registry https://registry.npmjs.org/
          npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},

		{"reset to the public registry after a private one", noOIDC + `      - run: |
          npm config set registry http://localhost:4873
          npm config set registry https://registry.npmjs.org/
          npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},
		{"private registry set again after the public one", noOIDC + `      - run: |
          npm config set registry https://registry.npmjs.org/
          npm config set registry http://localhost:4873
          npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 0},
		{"setup-node private, then a script reset", lin + `      - uses: actions/setup-node@v6
        with:
          registry-url: https://npm.pkg.github.com
      - run: |
          npm config set registry https://registry.npmjs.org/
          npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},
		{"setup-node private, then setup-node public", lin + `      - uses: actions/setup-node@v6
        with:
          registry-url: https://npm.pkg.github.com
      - uses: actions/setup-node@v6
        with:
          registry-url: https://registry.npmjs.org
      - run: npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},
		{"private registry set only in a branch, public before", noOIDC + `      - run: |
          npm config set registry https://registry.npmjs.org/
          if [ -n "$X" ]; then npm config set registry http://localhost:4873; fi
          npm publish
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 0},
		{"public registry on the publish command wins over a private config", noOIDC + `      - run: |
          npm config set registry http://localhost:4873
          npm publish --registry https://registry.npmjs.org/
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},
		{"private registry on the publish command", noOIDC + `      - run: npm publish --registry http://localhost:4873
        env:
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 0},
		{"step environment resets the workflow's private registry", "on: push\nenv:\n  NPM_CONFIG_REGISTRY: http://localhost:4873\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + `      - run: npm publish
        env:
          NPM_CONFIG_REGISTRY: https://registry.npmjs.org/
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},
		{"public registry in the environment wins over a private registry-url", lin + `      - uses: actions/setup-node@v6
        with:
          registry-url: http://localhost:4873
      - run: npm publish
        env:
          NPM_CONFIG_REGISTRY: https://registry.npmjs.org/
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},
		{"public registry in the environment wins over a private npm config set", noOIDC + `      - run: |
          npm config set registry http://localhost:4873
          npm publish
        env:
          NPM_CONFIG_REGISTRY: https://registry.npmjs.org/
          NODE_AUTH_TOKEN: ${{ secrets.T }}
`, 1},

		// false negatives
		{"cargo mono", noOIDC + `      - run: cargo mono publish --no-verify
        env:
          CARGO_REGISTRY_TOKEN: ${{ secrets.T }}
`, 1},
		{"cargo workspaces", noOIDC + `      - run: cargo workspaces publish --yes
        env:
          CARGO_REGISTRY_TOKEN: ${{ secrets.T }}
`, 1},
		{"cargo workspaces dry run", noOIDC + `      - run: cargo workspaces publish --dry-run
`, 0},
		{"uvx twine", noOIDC + `      - run: uvx twine upload dist/*
        env:
          TWINE_PASSWORD: ${{ secrets.T }}
`, 1},
		{"uv tool run twine", noOIDC + `      - run: uv tool run twine upload dist/*
        env:
          TWINE_PASSWORD: ${{ secrets.T }}
`, 1},
		{"uvx from twine", noOIDC + `      - run: uvx --from twine twine upload dist/*
        env:
          TWINE_PASSWORD: ${{ secrets.T }}
`, 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := countBatchD(t, "use-trusted-publishing", tc.src); got != tc.want {
				t.Errorf("%d findings, want %d\n%s", got, tc.want, tc.src)
			}
		})
	}
}

func TestCachePoisoningMorePublishersAndCaches(t *testing.T) {
	const job = "on:\n  push:\n    branches: [main]\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n"
	tests := []struct {
		what  string
		steps string
		want  int
	}{
		{"cargo mono publish with a cache", "      - uses: Swatinem/rust-cache@v2\n      - run: cargo mono publish --no-verify\n", 1},
		{"cargo workspaces publish with a cache", "      - uses: Swatinem/rust-cache@v2\n      - run: cargo workspaces publish --yes\n", 1},
		{"a build-and-tag release action", "      - uses: actions/cache@v5\n        with:\n          path: p\n          key: k\n      - uses: JasonEtco/build-and-tag-action@v2\n", 1},
		{"release-please", "      - uses: actions/cache@v5\n        with:\n          path: p\n          key: k\n      - uses: googleapis/release-please-action@v4\n", 1},
		{"no publisher", "      - uses: Swatinem/rust-cache@v2\n      - run: cargo test\n", 0},
		{"setup-bun in a publishing job", "      - uses: oven-sh/setup-bun@v2\n      - run: npm publish\n", 1},
		{"setup-bun without its cache", "      - uses: oven-sh/setup-bun@v2\n        with:\n          no-cache: true\n      - run: npm publish\n", 0},
		{"setup-rust-toolchain in a publishing job", "      - uses: actions-rust-lang/setup-rust-toolchain@v1\n      - run: cargo publish\n", 1},
		{"setup-rust-toolchain without its cache", "      - uses: actions-rust-lang/setup-rust-toolchain@v1\n        with:\n          cache: false\n      - run: cargo publish\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			if got := lintCacheWorkflow(t, "", job+tc.steps); len(got) != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, got)
			}
		})
	}
}

func TestSelfRepositoryMessageOfAWorkflowCall(t *testing.T) {
	const src = "on: push\njobs:\n  call:\n    uses: ./.github/workflows/x.yml\n  steps:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: ./.github/actions/y\n"
	cfg := mustParseConfig(t, "rules:\n  self-repository: warn\n")
	var got []string
	for _, e := range lintWithConfig(t, cfg, src) {
		if e.ID == "self-repository" {
			got = append(got, e.Message)
		}
	}
	if len(got) != 2 {
		t.Fatalf("want two findings, got %v", got)
	}
	if strings.Contains(got[0], "earlier step") || !strings.Contains(got[0], "reusable workflow") {
		t.Errorf("the message of a workflow call talks about steps: %s", got[0])
	}
	if !strings.Contains(got[1], "earlier step can replace it") {
		t.Errorf("the message of a step lost its reason: %s", got[1])
	}
}

func TestObfuscationMatrixIndex(t *testing.T) {
	cfg := mustParseConfig(t, "rules:\n  obfuscation: warn\n")
	count := func(expr string) int {
		src := "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        tool: [a]\n        provider: [{env_key: K}]\n    steps:\n      - run: echo " + expr + "\n"
		n := 0
		for _, e := range lintWithConfig(t, cfg, src) {
			if e.ID == "obfuscation" {
				n++
			}
		}
		return n
	}
	for expr, want := range map[string]int{
		"${{ fromJSON(needs.check.outputs.results)[matrix.tool].label }}": 0,
		"${{ secrets[matrix.provider.env_key] }}":                         0,
		"${{ github[inputs.name] }}":                                      1,
		"${{ github[github.ref_name] }}":                                  1,
		"${{ github['sha'] }}":                                            0,
	} {
		if got := count(expr); got != want {
			t.Errorf("%s: %d findings, want %d", expr, got, want)
		}
	}
}

func TestInheritedSecretsAreDefined(t *testing.T) {
	lib := "on:\n  workflow_call:\n    secrets:\n      FOO:\n        required: false\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ secrets.READER_TOKEN }}\n"
	caller := func(secrets string) string {
		return "on: push\njobs:\n  x:\n    uses: ./.github/workflows/lib.yml\n" + secrets + "  y:\n    uses: ./.github/workflows/lib.yml\n    secrets: inherit\n"
	}
	tests := []struct {
		what   string
		caller string
		want   int
	}{
		{"every caller inherits", caller("    secrets: inherit\n"), 0},
		{"a caller passes its secrets one by one", caller("    secrets:\n      FOO: ${{ secrets.FOO }}\n"), 1},
		{"a caller passes none", caller(""), 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{
				".github/workflows/lib.yml":    lib,
				".github/workflows/caller.yml": tc.caller,
			})
			l, err := NewLinter(io.Discard, &LinterOptions{WorkingDir: root})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = withoutMissingTimeout(&Config{})
			errs, err := l.LintFiles([]string{filepath.Join(root, ".github/workflows/lib.yml")}, &Project{root: root})
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, e := range errs {
				if strings.Contains(e.Message, `"reader_token" is not defined`) {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, errs)
			}
		})
	}
}

func TestLocalActionCheckoutOtherSetups(t *testing.T) {
	steps := func(s string) string {
		return "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n" + s
	}
	tests := []struct {
		what   string
		origin string
		src    string
		want   int
	}{
		{"a composite of the repository itself before the local action", "git@github.com:Acme/Widgets.git", steps("      - uses: acme/widgets/.github/actions/preparation@main\n      - uses: ./.github/actions/x\n"), 0},
		{"a composite of the repository with an https remote", "https://github.com/acme/widgets", steps("      - uses: acme/widgets/.github/actions/preparation@main\n      - uses: ./.github/actions/x\n"), 0},
		{"a composite of another repository", "git@github.com:acme/other.git", steps("      - uses: acme/widgets/.github/actions/preparation@main\n      - uses: ./.github/actions/x\n"), 1},
		{"no remote is known", "", steps("      - uses: acme/widgets/.github/actions/preparation@main\n      - uses: ./.github/actions/x\n"), 1},
		{"sources unpacked from an artifact", "", steps("      - uses: actions/download-artifact@v4\n        with:\n          name: source-bundle\n      - run: tar zxf sources.tar.gz\n      - uses: ./sources/repo/.github/actions/build\n"), 0},
		{"sources unpacked and an action of the repository", "", steps("      - run: tar zxf sources.tar.gz\n      - uses: ./.github/actions/build\n"), 1},
		{"a local action outside .github and nothing unpacked", "", steps("      - run: echo hi\n      - uses: ./sources/build\n"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			root := t.TempDir()
			if tc.origin != "" {
				writeTree(t, root, map[string]string{".git/config": "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + tc.origin + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"})
			}
			l, err := NewLinter(io.Discard, &LinterOptions{})
			if err != nil {
				t.Fatal(err)
			}
			l.defaultConfig = withoutMissingTimeout(ruleSwitch("local-action-checkout", true))
			errs, err := l.Lint("test.yaml", []byte(tc.src), &Project{root: root})
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, e := range errs {
				if e.ID == "local-action-checkout" {
					n++
				}
			}
			if n != tc.want {
				t.Errorf("want %d findings but got %v", tc.want, errs)
			}
		})
	}
}
