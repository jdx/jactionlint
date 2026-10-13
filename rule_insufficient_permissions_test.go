package jactionlint

import (
	"strings"
	"testing"
)

const insufficientConfig = "rules:\n  insufficient-permissions: error\n"
const insufficientPedanticConfig = "rules:\n  insufficient-permissions:\n    level: error\n    pedantic: true\n"

// The lines which end with "# want" are the ones the rule reports.
func TestRuleInsufficientPermissions(t *testing.T) {
	const head = "on: push\n"
	const job = "jobs:\n  t:\n    runs-on: ubuntu-latest\n"
	tests := []struct {
		what string
		src  string
	}{
		// What is reported
		{"deploy-pages without id-token", head + job + "    permissions:\n      pages: write\n    steps:\n      - uses: actions/deploy-pages@v4 # want\n"},
		{"deploy-pages inherits the workflow", head + "permissions:\n  contents: read\n" + job + "    steps:\n      - uses: actions/deploy-pages@v4 # want\n"},
		{"deploy-pages with a token of its own still needs the OIDC token", head + job + "    permissions: {}\n    steps:\n      - uses: actions/deploy-pages@v4 # want\n        with:\n          token: ${{ secrets.PAT }}\n"},
		{"upload-sarif", head + job + "    permissions:\n      pull-requests: write\n    steps:\n      - uses: github/codeql-action/upload-sarif@v3 # want\n        with:\n          sarif_file: a.sarif\n"},
		{"upload-sarif with read", head + job + "    permissions:\n      security-events: read\n    steps:\n      - uses: github/codeql-action/upload-sarif@v3 # want\n"},
		{"analyze uploads by default", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: github/codeql-action/analyze@v3 # want\n"},
		{"attest", head + job + "    permissions:\n      id-token: write\n    steps:\n      - uses: actions/attest-build-provenance@v2 # want\n        with:\n          subject-path: a\n"},
		{"stale", head + "permissions:\n  contents: read\n" + job + "    steps:\n      - uses: actions/stale@v9 # want\n"},
		{"stale with a read scope", head + job + "    permissions:\n      issues: read\n    steps:\n      - uses: actions/stale@v9 # want\n"},
		{"labeler", head + job + "    permissions: {}\n    steps:\n      - uses: actions/labeler@v5 # want\n"},
		{"dependency-review comment", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/dependency-review-action@v4 # want\n        with:\n          comment-summary-in-pr: always\n"},
		{"create-pull-request", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peter-evans/create-pull-request@v7 # want\n"},
		{"create-pull-request with the token written out", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: peter-evans/create-pull-request@v7 # want\n        with:\n          token: ${{ secrets.GITHUB_TOKEN }}\n"},
		{"gh-release", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: softprops/action-gh-release@v2 # want\n"},
		{"gh-release with discussion", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: softprops/action-gh-release@v2 # want\n        with:\n          discussion_category_name: Announcements\n"},
		{"gh-release with the token of the environment", head + job + "    permissions: {}\n    env:\n      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n    steps:\n      - uses: softprops/action-gh-release@v2 # want\n"},
		{"release-action", head + job + "    permissions: read-all\n    steps:\n      - uses: ncipollo/release-action@v1 # want\n"},
		{"pages deploy", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: JamesIves/github-pages-deploy-action@v4 # want\n        with:\n          folder: dist\n"},
		{"gh-pages with github_token", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peaceiris/actions-gh-pages@v4 # want\n        with:\n          github_token: ${{ secrets.GITHUB_TOKEN }}\n"},
		{"release-please", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: googleapis/release-please-action@v4 # want\n"},
		{"release-drafter", head + job + "    permissions:\n      pull-requests: read\n    steps:\n      - uses: release-drafter/release-drafter@v6 # want\n"},
		{"autolabeler", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: release-drafter/release-drafter/autolabeler@v6 # want\n"},
		{"test-reporter", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: dorny/test-reporter@v1 # want\n"},
		{"publish-unit-test-result", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: EnricoMi/publish-unit-test-result-action@v2 # want\n"},
		{"sticky comment", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: marocchino/sticky-pull-request-comment@v2 # want\n"},
		{"aws oidc", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4 # want\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n"},
		{"gcp workload identity", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: google-github-actions/auth@v2 # want\n        with:\n          workload_identity_provider: projects/1/locations/global/workloadIdentityPools/p/providers/g\n"},
		{"azure oidc", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: azure/login@v2 # want\n        with:\n          client-id: ${{ secrets.AZURE_CLIENT_ID }}\n          tenant-id: ${{ secrets.AZURE_TENANT_ID }}\n"},
		{"pypi trusted publishing", head + job + "    permissions: read-all\n    steps:\n      - uses: pypa/gh-action-pypi-publish@release/v1 # want\n"},
		{"rubygems credentials", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: rubygems/configure-rubygems-credentials@main # want\n        with:\n          role-to-assume: rg_oidc_x\n"},
		{"nuget login", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: NuGet/login@v1 # want\n"},
		{"the name is not case sensitive", head + job + "    permissions: {}\n    steps:\n      - uses: Actions/Deploy-Pages@v4 # want\n"},
		{"one finding for each step", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/stale@v9 # want\n      - uses: actions/labeler@v5 # want\n"},
		{"matrix job", head + "jobs:\n  t:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        n: [1, 2]\n    permissions: {}\n    steps:\n      - uses: NuGet/login@v1 # want\n"},
		{"gh release create", head + job + "    permissions:\n      contents: read\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: |\n          gh release create v1 a.zip # want\n"},
		{"gh release upload with the token of the step", head + job + "    permissions:\n      contents: read\n    steps:\n      - run: gh release upload v1 a.zip # want\n        env:\n          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n"},
		{"gh pr create", head + job + "    permissions:\n      contents: write\n    env:\n      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n    steps:\n      - run: gh pr create --fill # want\n"},
		{"gh issue create", head + job + "    permissions:\n      pull-requests: write\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh issue create --title x --body y # want\n"},
		{"gh workflow run", head + job + "    permissions:\n      contents: read\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh workflow run other.yaml # want\n"},
		{"gh in the workflow env", head + "env:\n  GH_TOKEN: ${{ github.token }}\npermissions:\n  contents: read\n" + job + "    steps:\n      - run: gh release delete v1 # want\n"},
		{"git push after checkout", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/checkout@v4\n      - run: |\n          git commit -am x\n          git push # want\n"},
		{"git push origin main", head + job + "    permissions:\n      pull-requests: write\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push origin HEAD:main # want\n"},
		{"git push with config of the user", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/checkout@v4\n      - run: |\n          git config user.name bot\n          git config user.email bot@example.com\n          git push # want\n"},
		{"auto commit", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/checkout@v4\n      - uses: stefanzweifel/git-auto-commit-action@v5 # want\n"},

		{"git push after a checkout with persist-credentials: true", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: true\n      - run: git push # want\n"},
		{"auto commit after a checkout with persist-credentials: true", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: true\n      - uses: stefanzweifel/git-auto-commit-action@v5 # want\n"},
		{"test results with check_run: true", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: EnricoMi/publish-unit-test-result-action@v2 # want\n        with:\n          check_run: true\n"},
		{"azure with the auth type written out", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: azure/login@v2 # want\n        with:\n          auth-type: SERVICE_PRINCIPAL\n          client-id: ${{ secrets.ID }}\n"},
		{"attest with a token of its own still needs the OIDC token", head + job + "    permissions:\n      attestations: write\n    steps:\n      - uses: actions/attest-build-provenance@v2 # want\n        with:\n          github-token: ${{ secrets.PAT }}\n          subject-path: a\n"},
		{"attest without a token of its own", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/attest@v2 # want\n"},

		{"create-pull-request with a branch token still opens the pull request with the GITHUB_TOKEN", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peter-evans/create-pull-request@v7 # want\n        with:\n          branch-token: ${{ secrets.PAT }}\n"},
		{"stale which is not a debug run", head + job + "    permissions: {}\n    steps:\n      - uses: actions/stale@v9 # want\n        with:\n          debug-only: false\n"},
		{"release-please with the skip options off", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: googleapis/release-please-action@v4 # want\n        with:\n          skip-github-release: false\n          skip-github-pull-request: false\n          fork: false\n"},
		{"release-drafter with the options off", head + job + "    permissions:\n      pull-requests: read\n    steps:\n      - uses: release-drafter/release-drafter@v6 # want\n        with:\n          disable-releaser: false\n          dry-run: false\n"},
		{"aws with the options off", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4 # want\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          role-chaining: false\n          force-skip-oidc: false\n"},

		// What is not reported: the permissions are not explicit or are enough
		{"attest with a token of its own and the OIDC token", head + job + "    permissions:\n      id-token: write\n    steps:\n      - uses: actions/attest-build-provenance@v2\n        with:\n          github-token: ${{ secrets.PAT }}\n          subject-path: a\n"},
		{"test results with check_run: false quoted", head + job + "    permissions: {}\n    steps:\n      - uses: EnricoMi/publish-unit-test-result-action@v2\n        with:\n          check_run: 'false'\n"},
		{"test results with check_run from an expression", head + job + "    permissions: {}\n    steps:\n      - uses: EnricoMi/publish-unit-test-result-action@v2\n        with:\n          check_run: ${{ inputs.check }}\n"},
		{"azure with the auth type from an expression", head + job + "    permissions: {}\n    steps:\n      - uses: azure/login@v2\n        with:\n          auth-type: ${{ inputs.type }}\n          client-id: ${{ secrets.ID }}\n"},

		{"no permissions", head + job + "    steps:\n      - uses: actions/deploy-pages@v4\n"},
		{"write-all", head + job + "    permissions: write-all\n    steps:\n      - uses: actions/deploy-pages@v4\n      - uses: github/codeql-action/upload-sarif@v3\n"},
		{"enough at the job", head + "permissions: {}\n" + job + "    permissions:\n      pages: write\n      id-token: write\n    steps:\n      - uses: actions/deploy-pages@v4\n"},
		{"enough at the workflow", head + "permissions:\n  security-events: write\n" + job + "    steps:\n      - uses: github/codeql-action/upload-sarif@v3\n"},
		{"the job replaces the permissions of the workflow", head + "permissions: {}\n" + job + "    permissions:\n      security-events: write\n    steps:\n      - uses: github/codeql-action/upload-sarif@v3\n"},
		{"write includes read", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: actions/checkout@v4\n"},
		{"any of the scopes", head + job + "    permissions:\n      pull-requests: write\n    steps:\n      - uses: actions/stale@v9\n"},
		{"other scope of the same action", head + job + "    permissions:\n      issues: write\n    steps:\n      - uses: actions/stale@v9\n      - uses: actions/labeler@v5\n"},
		{"checkout is only for private repositories", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n"},
		{"an action which is not in the table", head + job + "    permissions: {}\n    steps:\n      - uses: actions/cache@v4\n        with:\n          path: a\n          key: b\n"},
		{"a local action", head + job + "    permissions: {}\n    steps:\n      - uses: ./.github/actions/deploy\n"},
		{"an expression in uses", head + job + "    permissions: {}\n    steps:\n      - uses: ${{ matrix.action }}\n"},
		{"a reusable workflow call", head + "jobs:\n  t:\n    permissions: {}\n    uses: ./.github/workflows/a.yaml\n"},
		{"a step that never runs", head + job + "    permissions: {}\n    steps:\n      - uses: actions/deploy-pages@v4\n        if: false\n"},
		{"a job that never runs", head + "jobs:\n  t:\n    if: false\n    runs-on: ubuntu-latest\n    permissions: {}\n    steps:\n      - uses: actions/deploy-pages@v4\n"},
		{"a step that may fail", head + job + "    permissions: {}\n    steps:\n      - uses: github/codeql-action/upload-sarif@v3\n        continue-on-error: true\n"},

		// The environment may come from an expression: it may set the token
		{"gh-release with an env that is an expression", head + job + "    permissions: {}\n    steps:\n      - uses: softprops/action-gh-release@v2\n        env: ${{ fromJSON(inputs.env) }}\n"},
		{"gh-release with an insert key in the env of the job", head + job + "    permissions: {}\n    env:\n      ${{ insert }}: ${{ fromJSON(vars.ENV) }}\n    steps:\n      - uses: softprops/action-gh-release@v2\n"},
		{"gh with an insert key in the env of the step", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh release create v1\n        env:\n          ${{ insert }}: ${{ fromJSON(vars.ENV) }}\n"},
		{"gh with an env of the workflow that is an expression", "on: push\nenv: ${{ fromJSON(vars.ENV) }}\njobs:\n  t:\n    runs-on: ubuntu-latest\n    permissions: {}\n    steps:\n      - run: gh release create v1\n"},
		{"analyze with a token of its own", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: github/codeql-action/analyze@v3\n        with:\n          token: ${{ secrets.PAT }}\n"},
		{"analyze with the token written out", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: github/codeql-action/analyze@v3 # want\n        with:\n          token: ${{ secrets.GITHUB_TOKEN }}\n"},

		// The inputs may come from an expression: they may give another token
		{"a with that is one expression", head + job + "    permissions: {}\n    steps:\n      - uses: github/codeql-action/upload-sarif@v3\n        with: ${{ fromJSON(inputs.with) }}\n"},
		{"a with with an insert key", head + job + "    permissions: {}\n    steps:\n      - uses: softprops/action-gh-release@v2\n        with:\n          files: a\n          ${{ insert }}: ${{ matrix.inputs }}\n"},
		{"aws with an insert key", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          ${{ insert }}: ${{ matrix.inputs }}\n"},
		{"a push after a checkout whose inputs are an expression", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with: ${{ fromJSON(inputs.checkout) }}\n      - run: git push\n"},
		{"auto commit after a checkout with an insert key", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ${{ insert }}: ${{ matrix.inputs }}\n      - uses: stefanzweifel/git-auto-commit-action@v5\n"},

		// Another credential than the GITHUB_TOKEN
		{"upload-sarif with another token", head + job + "    permissions: {}\n    steps:\n      - uses: github/codeql-action/upload-sarif@v3\n        with:\n          token: ${{ secrets.PAT }}\n"},
		{"create-pull-request with a token", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peter-evans/create-pull-request@v7\n        with:\n          token: ${{ secrets.PAT }}\n"},
		{"create-pull-request with a branch token and the permission of the pull request", head + job + "    permissions:\n      pull-requests: write\n    steps:\n      - uses: peter-evans/create-pull-request@v7\n        with:\n          branch-token: ${{ secrets.PAT }}\n"},
		{"stale which only logs, as a flag", head + job + "    permissions: {}\n    steps:\n      - uses: actions/stale@v9\n        with:\n          debug-only: true\n"},
		{"stale whose debug flag is an expression", head + job + "    permissions: {}\n    steps:\n      - uses: actions/stale@v9\n        with:\n          debug-only: ${{ inputs.debug }}\n"},
		{"release-please which skips the release", head + job + "    permissions: {}\n    steps:\n      - uses: googleapis/release-please-action@v4\n        with:\n          skip-github-release: true\n"},
		{"release-please from a fork", head + job + "    permissions: {}\n    steps:\n      - uses: googleapis/release-please-action@v4\n        with:\n          fork: true\n"},
		{"release-please with a skip from an expression", head + job + "    permissions: {}\n    steps:\n      - uses: googleapis/release-please-action@v4\n        with:\n          skip-github-pull-request: ${{ inputs.skip }}\n"},
		{"release-drafter in a dry run", head + job + "    permissions:\n      pull-requests: read\n    steps:\n      - uses: release-drafter/release-drafter@v6\n        with:\n          dry-run: true\n"},
		{"aws which skips the OIDC login", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          force-skip-oidc: true\n"},
		{"aws with a role chaining from an expression", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          role-chaining: ${{ inputs.chain }}\n"},
		{"create-pull-request with a token from an expression", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peter-evans/create-pull-request@v7\n        with:\n          token: ${{ steps.app.outputs.token }}\n"},
		{"gh-release with a token", head + job + "    permissions: {}\n    steps:\n      - uses: softprops/action-gh-release@v2\n        with:\n          token: ${{ secrets.RELEASE_TOKEN }}\n"},
		{"gh-release with a token in the environment", head + job + "    permissions: {}\n    steps:\n      - uses: softprops/action-gh-release@v2\n        env:\n          GITHUB_TOKEN: ${{ secrets.RELEASE_TOKEN }}\n"},
		{"stale with a token", head + job + "    permissions: {}\n    steps:\n      - uses: actions/stale@v9\n        with:\n          repo-token: ${{ secrets.BOT_TOKEN }}\n"},
		{"stale which only logs", head + job + "    permissions: {}\n    steps:\n      - uses: actions/stale@v9\n        with:\n          debug-only: true\n"},
		{"pages deploy with an ssh key", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: JamesIves/github-pages-deploy-action@v4\n        with:\n          ssh-key: ${{ secrets.KEY }}\n"},
		{"gh-pages with a deploy key", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peaceiris/actions-gh-pages@v4\n        with:\n          deploy_key: ${{ secrets.KEY }}\n"},
		{"gh-pages with a personal token", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: peaceiris/actions-gh-pages@v4\n        with:\n          personal_token: ${{ secrets.PAT }}\n"},
		{"release-please which skips the pull request", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: googleapis/release-please-action@v4\n        with:\n          skip-github-pull-request: true\n"},
		{"release-drafter which only labels", head + job + "    permissions:\n      pull-requests: write\n    steps:\n      - uses: release-drafter/release-drafter@v6\n        with:\n          disable-releaser: true\n"},
		{"test results without a check run", head + job + "    permissions: {}\n    steps:\n      - uses: EnricoMi/publish-unit-test-result-action@v2\n        with:\n          check_run: false\n"},
		{"checkout with a token", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          token: ${{ secrets.PAT }}\n"},
		{"checkout with an ssh key", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ssh-key: ${{ secrets.KEY }}\n"},
		{"checkout of another repository", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          repository: other/repo\n"},

		// The configuration of the step decides
		{"analyze which does not upload", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: github/codeql-action/analyze@v3\n        with:\n          upload: never\n"},
		{"analyze with an upload from an expression", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: github/codeql-action/analyze@v3\n        with:\n          upload: ${{ inputs.upload }}\n"},
		{"dependency-review without a comment", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/dependency-review-action@v4\n"},
		{"dependency-review which never comments", head + job + "    permissions:\n      contents: read\n    steps:\n      - uses: actions/dependency-review-action@v4\n        with:\n          comment-summary-in-pr: never\n"},
		{"aws with an access key", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          aws-access-key-id: ${{ secrets.K }}\n          aws-secret-access-key: ${{ secrets.S }}\n"},
		{"aws without a role", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          aws-access-key-id: ${{ secrets.K }}\n          aws-secret-access-key: ${{ secrets.S }}\n"},
		{"aws with role chaining", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          role-chaining: true\n"},
		{"aws with a web identity token file", head + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n          web-identity-token-file: /tmp/t\n"},
		{"aws with credentials of an earlier step", head + job + "    permissions: {}\n    steps:\n      - run: echo \"AWS_ACCESS_KEY_ID=x\" >> \"$GITHUB_ENV\"\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n"},
		{"aws with credentials in the environment", head + "env:\n  AWS_ACCESS_KEY_ID: ${{ secrets.K }}\n" + job + "    permissions: {}\n    steps:\n      - uses: aws-actions/configure-aws-credentials@v4\n        with:\n          role-to-assume: arn:aws:iam::1:role/x\n"},
		{"gcp with a key", head + job + "    permissions: {}\n    steps:\n      - uses: google-github-actions/auth@v2\n        with:\n          credentials_json: ${{ secrets.KEY }}\n"},
		{"azure with a secret", head + job + "    permissions: {}\n    steps:\n      - uses: azure/login@v2\n        with:\n          creds: ${{ secrets.AZURE_CREDENTIALS }}\n"},
		{"azure with a managed identity", head + job + "    permissions: {}\n    steps:\n      - uses: azure/login@v2\n        with:\n          auth-type: IDENTITY\n          client-id: ${{ secrets.ID }}\n"},
		{"pypi with a password", head + job + "    permissions: {}\n    steps:\n      - uses: pypa/gh-action-pypi-publish@release/v1\n        with:\n          password: ${{ secrets.PYPI }}\n"},
		{"rubygems with a token", head + job + "    permissions: {}\n    steps:\n      - uses: rubygems/configure-rubygems-credentials@main\n        with:\n          api-token: ${{ secrets.K }}\n"},

		// Commands of scripts
		{"gh release view only reads", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh release view v1\n"},
		{"gh without a token in the environment", head + job + "    permissions: {}\n    steps:\n      - run: gh release create v1\n"},
		{"gh with a token of its own", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ secrets.PAT }}\n    steps:\n      - run: gh release create v1\n"},
		{"gh with a token of the step", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh release create v1\n        env:\n          GH_TOKEN: ${{ secrets.PAT }}\n"},
		{"gh with the token written in front", head + job + "    permissions: {}\n    steps:\n      - run: GH_TOKEN=${{ secrets.PAT }} gh release create v1\n"},
		{"gh with the token set in the script", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: |\n          export GH_TOKEN=\"$OTHER\"\n          gh release create v1\n"},
		{"gh for another repository", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh release create v1 --repo other/repo\n"},
		{"gh which may fail", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: gh release create v1 || true\n"},
		{"gh in a condition", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: if ! gh release create v1; then echo failed; fi\n"},
		{"gh with enough permissions", head + job + "    permissions:\n      contents: write\n      pull-requests: write\n      issues: write\n      actions: write\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - run: |\n          gh release create v1\n          gh pr create --fill\n          gh issue create -t x -b y\n          gh workflow run a.yaml\n"},
		{"git push without a checkout", head + job + "    permissions: {}\n    steps:\n      - run: git push\n"},
		{"git push after a checkout without credentials", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          persist-credentials: false\n      - run: git push\n"},
		{"git push after a checkout with a token", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          token: ${{ secrets.PAT }}\n      - run: git push\n"},
		{"git push after a checkout with an ssh key", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          ssh-key: ${{ secrets.KEY }}\n      - run: git push\n"},
		{"git push before the checkout", head + job + "    permissions: {}\n    steps:\n      - run: git push\n      - uses: actions/checkout@v4\n"},
		{"git push to a url", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push https://x:${{ secrets.PAT }}@github.com/o/r.git HEAD\n"},
		{"git push to another remote", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push heroku main\n"},
		{"git push after a remote was changed", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: |\n          git remote set-url origin https://x:${{ secrets.PAT }}@github.com/o/r.git\n          git push\n"},
		{"git push after the credential helper was set", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git config --global credential.helper store\n      - run: git push\n"},
		{"git push in another directory", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: |\n          cd other\n          git push\n"},
		{"git push with -C", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git -C other push\n"},
		{"git push with a working directory", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push\n        working-directory: other\n"},
		{"git push dry run", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push --dry-run\n"},
		{"git push which may fail", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push || echo skipped\n"},
		{"git push with enough permissions", head + job + "    permissions:\n      contents: write\n    steps:\n      - uses: actions/checkout@v4\n      - run: git push\n"},
		{"auto commit with a checkout with a token", head + job + "    permissions: {}\n    steps:\n      - uses: actions/checkout@v4\n        with:\n          token: ${{ secrets.PAT }}\n      - uses: stefanzweifel/git-auto-commit-action@v5\n"},
		{"auto commit without a checkout", head + job + "    permissions: {}\n    steps:\n      - uses: stefanzweifel/git-auto-commit-action@v5\n"},
		{"powershell is not judged", head + job + "    permissions: {}\n    env:\n      GH_TOKEN: ${{ github.token }}\n    steps:\n      - shell: pwsh\n        run: gh release create v1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.what, func(t *testing.T) {
			want := markedWantLines(tc.src)
			checkLines(t, lintBatchH(t, insufficientConfig, tc.src, "insufficient-permissions"), want...)
		})
	}
}

func TestRuleInsufficientPermissionsCheckoutIsPedantic(t *testing.T) {
	const src = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    permissions:\n      pull-requests: write\n    steps:\n      - uses: actions/checkout@v4\n      - uses: actions/checkout@v4\n        with:\n          path: other\n"
	checkLines(t, lintBatchH(t, insufficientConfig, src, "insufficient-permissions"))
	checkLines(t, lintBatchH(t, insufficientPedanticConfig, src, "insufficient-permissions"), "8", "9")
	if got := lintBatchH(t, insufficientPedanticConfig, src, "insufficient-permissions"); len(got) == 0 || !strings.Contains(got[0].Message, `"contents: read"`) || !strings.Contains(got[0].Message, `"contents: none"`) {
		t.Errorf("message: %v", got)
	}
	// With contents: read the checkout is fine, whatever the other scopes
	const ok = "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    permissions:\n      contents: read\n    steps:\n      - uses: actions/checkout@v4\n"
	checkLines(t, lintBatchH(t, insufficientPedanticConfig, ok, "insufficient-permissions"))
}

func TestRuleInsufficientPermissionsMessage(t *testing.T) {
	const src = "on: push\npermissions:\n  contents: read\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/deploy-pages@v4\n  u:\n    runs-on: ubuntu-latest\n    permissions:\n      issues: read\n    steps:\n      - uses: actions/stale@v9\n"
	errs := lintBatchH(t, insufficientConfig, src, "insufficient-permissions")
	if len(errs) != 2 {
		t.Fatalf("want 2 findings: %v", errs)
	}
	want := []string{
		`actions/deploy-pages needs "pages: write" and "id-token: write", but the permissions of the workflow (this job sets none) give "pages: none" and "id-token: none", so the step fails. grant "pages: write" and "id-token: write" in the "permissions:" of the job, which replace the ones of the workflow, so list every scope the job needs`,
		`actions/stale needs "issues: write" or "pull-requests: write", but the permissions of this job give "issues: read" and "pull-requests: none", so the step fails. grant "issues: write" or "pull-requests: write" in the "permissions:" of the job`,
	}
	for i, e := range errs {
		if e.Message != want[i] {
			t.Errorf("message %d:\n got %s\nwant %s", i, e.Message, want[i])
		}
	}
}

// The table is written in lower case, and every entry names the documentation it comes from.
func TestActionPermissionTable(t *testing.T) {
	for name, e := range actionPermissionTable {
		if name != strings.ToLower(name) {
			t.Errorf("%s: the name must be lower case", name)
		}
		if !strings.HasPrefix(e.Source, "https://") || e.Needs == nil {
			t.Errorf("%s: no source or no needs", name)
		}
	}
}

// The rule is off in the correctness profile and on in the default profile.
func TestInsufficientPermissionsProfile(t *testing.T) {
	const src = "on: push\npermissions: {}\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: NuGet/login@v1\n"
	if got := lintBatchH(t, "profile: correctness\n", src, "insufficient-permissions"); len(got) != 0 {
		t.Errorf("correctness: %v", got)
	}
	if got := lintBatchH(t, "profile: default\n", src, "insufficient-permissions"); len(got) != 1 {
		t.Errorf("default: %v", got)
	}
}
