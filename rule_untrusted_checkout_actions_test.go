package jactionlint

import "testing"

// Actions that run what is in the workspace count as running the code of a pull request that an earlier step checked out.
func TestRuleUntrustedCheckoutWorkspaceActions(t *testing.T) {
	const head = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2\n        with:\n          ref: ${{ github.event.pull_request.head.sha }} # want\n"
	const headNoWant = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n"
	tests := []struct {
		what string
		step string
		want bool
	}{
		// actions that run the workspace for every use
		{"megalinter", "oxsecurity/megalinter@v8", true},
		{"a flavor of megalinter", "oxsecurity/megalinter/flavors/python@v8", true},
		{"super-linter", "super-linter/super-linter@v7", true},
		{"slim super-linter", "super-linter/super-linter/slim@v7", true},
		{"checkov", "bridgecrewio/checkov-action@v12", true},
		{"sonarcloud", "SonarSource/sonarcloud-github-action@v3", true},
		{"sonarqube", "sonarsource/sonarqube-scan-action@v5", true},
		{"terraform plan", "dflook/terraform-plan@v1", true},
		{"tofu apply", "dflook/tofu-apply@v1", true},
		{"reviewdog eslint", "reviewdog/action-eslint@v1", true},
		{"reviewdog rubocop", "reviewdog/action-rubocop@v2", true},
		{"vale", "errata-ai/vale-action@v2", true},
		{"npm publish", "JS-DevTools/npm-publish@v3", true},
		{"cibuildwheel", "pypa/cibuildwheel@v2", true},
		{"maturin", "PyO3/maturin-action@v1", true},
		{"trunk", "trunk-io/trunk-action@v1", true},
		{"build-and-inspect-python-package", "hynek/build-and-inspect-python-package@v2", true},
		{"a spelling in another case", "Pypa/CIBuildWheel@v2", true},
		{"the actions of the first version", "golangci/golangci-lint-action@v6", true},
		// actions that run the workspace for some inputs
		{"setup-ruby with a bundler cache", "ruby/setup-ruby@v1\n        with:\n          bundler-cache: true", true},
		{"setup-ruby without it", "ruby/setup-ruby@v1\n        with:\n          ruby-version: '3.3'", false},
		{"setup-ruby with the cache off", "ruby/setup-ruby@v1\n        with:\n          bundler-cache: false", false},
		{"setup-ruby with a cache that is not known", "ruby/setup-ruby@v1\n        with:\n          bundler-cache: ${{ inputs.cache }}", false},
		{"setup-node with a cache", "actions/setup-node@v4\n        with:\n          cache: yarn", true},
		{"setup-node without one", "actions/setup-node@v4\n        with:\n          node-version: 22", false},
		{"build-push-action with a path as the context", "docker/build-push-action@v6\n        with:\n          context: .", true},
		{"build-push-action with a directory as the context", "docker/build-push-action@v6\n        with:\n          context: ./app\n          push: false", true},
		{"build-push-action with the default context", "docker/build-push-action@v6\n        with:\n          push: false", false},
		{"build-push-action with a git context", "docker/build-push-action@v6\n        with:\n          context: https://github.com/example/repo.git#main", false},
		{"build-push-action with the default git context of the template", "docker/build-push-action@v6\n        with:\n          context: \"{{defaultContext}}:app\"", false},
		{"goreleaser release", "goreleaser/goreleaser-action@v6\n        with:\n          args: release --clean", true},
		{"goreleaser build", "goreleaser/goreleaser-action@v6\n        with:\n          args: build --snapshot", true},
		{"goreleaser check", "goreleaser/goreleaser-action@v6\n        with:\n          args: check", false},
		{"goreleaser only installed", "goreleaser/goreleaser-action@v6\n        with:\n          install-only: true", false},
		{"goreleaser installed and told to release", "goreleaser/goreleaser-action@v6\n        with:\n          args: release\n          install-only: true", false},
		{"goreleaser without arguments", "goreleaser/goreleaser-action@v6", false},
		{"mage with a target", "magefile/mage-action@v3\n        with:\n          args: build", true},
		{"mage without arguments runs the default target", "magefile/mage-action@v3", true},
		{"mage with arguments that are not known", "magefile/mage-action@v3\n        with:\n          args: ${{ inputs.target }}", true},
		{"mage only installed", "magefile/mage-action@v3\n        with:\n          install-only: true", false},
		{"gradle build action with arguments", "gradle/gradle-build-action@v3\n        with:\n          arguments: build", true},
		{"gradle build action without arguments", "gradle/gradle-build-action@v3", false},
		{"setup-gradle prepares a later step", "gradle/actions/setup-gradle@v4", false},
		{"setup-gradle v3 with arguments runs gradle", "gradle/actions/setup-gradle@v3\n        with:\n          arguments: build", true},
		{"setup-gradle with arguments that are not known", "gradle/actions/setup-gradle@v3\n        with:\n          arguments: ${{ inputs.args }}", false},
		{"powershell that runs a script of the workspace", "azure/powershell@v2\n        with:\n          azPSVersion: latest\n          inlineScript: ./scripts/deploy.ps1", true},
		{"powershell that runs an inline script", "azure/powershell@v2\n        with:\n          azPSVersion: latest\n          inlineScript: Get-AzResourceGroup", false},
		{"cypress", "cypress-io/github-action@v6", true},
		{"cypress that neither installs nor runs", "cypress-io/github-action@v6\n        with:\n          install: false\n          runTests: false", false},
		// actions that do not run it
		{"setup-buildx does not", "docker/setup-buildx-action@v3", false},
		{"upload-artifact does not", "actions/upload-artifact@v4\n        with:\n          path: out", false},
		{"pre-commit lite action only commits", "pre-commit-ci/lite-action@v1", false},
		{"an unrelated action of the same owner", "oxsecurity/other@v1", false},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			src := headNoWant + "      - uses: " + tc.step + "\n"
			if tc.want {
				src = head + "      - uses: " + tc.step + "\n"
			}
			errs := onlyID(lintInProject(t, nil, src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
			checkLines(t, errs, markedWantLines(src)...)
		})
	}
}

// With a checkout in a subdirectory an action that works on the root of the workspace does not run it.
func TestRuleUntrustedCheckoutWorkspaceActionInDirectory(t *testing.T) {
	const src = "on: pull_request_target\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2\n        with:\n          ref: ${{ github.event.pull_request.head.sha }}\n          path: pr\n      - uses: reviewdog/action-eslint@v1\n"
	errs := onlyID(lintInProject(t, nil, src, "rules:\n  untrusted-checkout: error\n"), "untrusted-checkout")
	checkLines(t, errs)
}
