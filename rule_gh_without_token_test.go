package jactionlint

import "testing"

func TestGhWithoutToken(t *testing.T) {
	cfg := ruleConfig("gh-without-token")
	job := func(runsOn, rest string) string {
		return "on: push\njobs:\n  a:\n    runs-on: " + runsOn + "\n" + rest
	}
	tests := []struct {
		what  string
		src   string
		lines []int
	}{
		{"plain command", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n"), []int{6}},
		{"second line of a script", job("ubuntu-latest", "    steps:\n      - run: |\n          echo hi\n          gh release create v1\n"), []int{8}},
		{"one finding for each step", job("ubuntu-latest", "    steps:\n      - run: |\n          gh pr list\n          gh pr view 1\n"), []int{7}},
		{"one finding for each step, two steps", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n      - run: gh pr view 1\n"), []int{6, 7}},
		{"sudo", job("ubuntu-latest", "    steps:\n      - run: sudo gh pr list\n"), []int{6}},
		{"in a condition", job("ubuntu-latest", "    steps:\n      - run: if gh pr view 1; then echo y; fi\n"), []int{6}},
		{"gh api", job("ubuntu-latest", "    steps:\n      - run: gh api repos/a/b\n"), []int{6}},
		{"windows with bash", job("windows-latest", "    steps:\n      - run: gh pr list\n        shell: bash\n"), []int{6}},
		{"matrix of hosted runners", job("${{ matrix.os }}", "    strategy:\n      matrix:\n        os: [ubuntu-latest, macos-latest]\n    steps:\n      - run: gh pr list\n"), []int{9}},
		{"larger runner label", job("ubuntu-latest-8-cores", "    steps:\n      - run: gh pr list\n"), []int{6}},

		// tokens
		{"step env", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n        env:\n          GH_TOKEN: ${{ github.token }}\n"), nil},
		{"step env GITHUB_TOKEN", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n        env:\n          GITHUB_TOKEN: ${{ github.token }}\n"), nil},
		{"enterprise token", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n        env:\n          GH_HOST: ghe.example.com\n          GH_ENTERPRISE_TOKEN: ${{ secrets.T }}\n"), nil},
		{"job env", job("ubuntu-latest", "    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh pr list\n"), nil},
		{"workflow env", "on: push\nenv:\n  GH_TOKEN: ${{ github.token }}\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: gh pr list\n", nil},
		{"container env", job("ubuntu-latest", "    container:\n      image: alpine\n      env:\n        GH_TOKEN: x\n    steps:\n      - run: gh pr list\n"), nil},
		{"env of another step does not count", job("ubuntu-latest", "    steps:\n      - run: echo\n        env:\n          GH_TOKEN: ${{ github.token }}\n      - run: gh pr list\n"), []int{9}},
		{"env as an expression", job("ubuntu-latest", "    env: ${{ fromJSON(vars.ENV) }}\n    steps:\n      - run: gh pr list\n"), nil},
		{"prefix assignment", job("ubuntu-latest", "    steps:\n      - run: GH_TOKEN=${{ github.token }} gh pr list\n"), nil},
		{"env wrapper", job("ubuntu-latest", "    steps:\n      - run: env GH_TOKEN=${{ github.token }} gh pr list\n"), nil},
		{"exported in the script", job("ubuntu-latest", "    steps:\n      - run: |\n          export GH_TOKEN=${{ github.token }}\n          gh pr list\n"), nil},
		{"token passed on the command line", job("ubuntu-latest", "    steps:\n      - run: echo ${{ github.token }} | gh auth login --with-token\n"), nil},
		{"github env of an earlier step", job("ubuntu-latest", "    steps:\n      - run: echo \"GH_TOKEN=${{ steps.t.outputs.token }}\" >> \"$GITHUB_ENV\"\n      - run: gh pr list\n"), nil},
		{"github env of a later step", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n      - run: echo \"GH_TOKEN=x\" >> \"$GITHUB_ENV\"\n"), []int{6}},
		{"exportVariable of an earlier step", job("ubuntu-latest", "    steps:\n      - uses: actions/github-script@v7\n        with:\n          script: core.exportVariable('GH_TOKEN', 'x')\n      - run: gh pr list\n"), nil},
		{"local action before", job("ubuntu-latest", "    steps:\n      - uses: ./.github/actions/setup\n      - run: gh pr list\n"), nil},
		{"third party action before", job("ubuntu-latest", "    steps:\n      - uses: actions/checkout@v4\n      - run: gh pr list\n"), []int{7}},
		{"auth login in an earlier step", job("ubuntu-latest", "    steps:\n      - run: gh auth login --with-token < token.txt\n      - run: gh pr list\n"), nil},
		{"auth status", job("ubuntu-latest", "    steps:\n      - run: gh auth status\n"), nil},
		{"hosts.yml", job("ubuntu-latest", "    steps:\n      - run: |\n          cp hosts.yml ~/.config/gh/hosts.yml\n          gh pr list\n"), nil},

		// not a gh command, or one that needs no token
		{"version", job("ubuntu-latest", "    steps:\n      - run: gh --version\n"), nil},
		{"help flag", job("ubuntu-latest", "    steps:\n      - run: gh pr list --help\n"), nil},
		{"help", job("ubuntu-latest", "    steps:\n      - run: gh help\n"), nil},
		{"completion", job("ubuntu-latest", "    steps:\n      - run: gh completion -s bash\n"), nil},
		{"config", job("ubuntu-latest", "    steps:\n      - run: gh config set prompt disabled\n"), nil},
		{"extension", job("ubuntu-latest", "    steps:\n      - run: gh extension install owner/gh-x\n"), nil},
		{"command of an extension", job("ubuntu-latest", "    steps:\n      - run: gh my-extension foo\n"), nil},
		{"no arguments", job("ubuntu-latest", "    steps:\n      - run: gh\n"), nil},
		{"echo", job("ubuntu-latest", "    steps:\n      - run: echo \"gh pr list\"\n"), nil},
		{"comment", job("ubuntu-latest", "    steps:\n      - run: |\n          # gh pr list\n          echo x\n"), nil},
		{"heredoc", job("ubuntu-latest", "    steps:\n      - run: |\n          cat <<EOF\n          gh pr list\n          EOF\n"), nil},
		{"command -v", job("ubuntu-latest", "    steps:\n      - run: command -v gh\n"), nil},
		{"brew install", job("ubuntu-latest", "    steps:\n      - run: brew install gh\n"), nil},
		{"argument of ssh", job("ubuntu-latest", "    steps:\n      - run: ssh host gh pr list\n"), nil},
		{"a script of gh in the name of a file", job("ubuntu-latest", "    steps:\n      - run: ./gh-wrapper.sh pr list\n"), nil},
		{"variable", job("ubuntu-latest", "    steps:\n      - run: $GH pr list\n"), nil},
		{"dynamic subcommand", job("ubuntu-latest", "    steps:\n      - run: gh $SUB list\n"), nil},

		// runners that may be logged in
		{"self-hosted", job("[self-hosted, linux]", "    steps:\n      - run: gh pr list\n"), nil},
		{"self-hosted label only", job("self-hosted", "    steps:\n      - run: gh pr list\n"), nil},
		{"custom label", job("my-runners", "    steps:\n      - run: gh pr list\n"), nil},
		{"hosted and custom label", job("[ubuntu-latest, gpu]", "    steps:\n      - run: gh pr list\n"), nil},
		{"runner group", job("\n      group: my-group", "    steps:\n      - run: gh pr list\n"), nil},
		{"expression", job("${{ inputs.runner }}", "    steps:\n      - run: gh pr list\n"), nil},
		{"matrix with a self-hosted runner", job("${{ matrix.os }}", "    strategy:\n      matrix:\n        os: [ubuntu-latest, self-hosted]\n    steps:\n      - run: gh pr list\n"), nil},
		{"matrix with an expression", job("${{ matrix.os }}", "    strategy:\n      matrix:\n        os: ${{ fromJSON(vars.OS) }}\n    steps:\n      - run: gh pr list\n"), nil},
		{"matrix with a value that has no row", job("${{ matrix.os }}", "    strategy:\n      matrix:\n        v: [1]\n    steps:\n      - run: gh pr list\n"), nil},

		// shells
		{"windows default shell is pwsh", job("windows-latest", "    steps:\n      - run: gh pr list\n"), nil},
		{"matrix with windows and the default shell", job("${{ matrix.os }}", "    strategy:\n      matrix:\n        os: [ubuntu-latest, windows-latest]\n    steps:\n      - run: gh pr list\n"), nil},
		{"pwsh", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n        shell: pwsh\n"), nil},
		{"python", job("ubuntu-latest", "    steps:\n      - run: gh pr list\n        shell: python\n"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			wantLines(t, lintFileWithConfig(t, cfg, "ci.yaml", tc.src), "gh-without-token", tc.lines...)
		})
	}
}

func TestGhWithoutTokenMessageAndPosition(t *testing.T) {
	cfg := ruleConfig("gh-without-token")
	src := "on: push\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n          echo \"éé\"; gh pr list --state open\n"
	errs := errsWithID(lintFileWithConfig(t, cfg, "ci.yaml", src), "gh-without-token")
	if len(errs) != 1 {
		t.Fatalf("want one finding: %v", errs)
	}
	e := errs[0]
	if e.Line != 7 || e.Column != 22 {
		t.Errorf("position is %d:%d", e.Line, e.Column)
	}
	if want := `command "gh pr list" stops with`; len(e.Message) < len(want) || e.Message[:len(want)] != want {
		t.Errorf("message is %q", e.Message)
	}
	if e.Fix != nil {
		t.Errorf("a fixer must not invent a token")
	}
}
