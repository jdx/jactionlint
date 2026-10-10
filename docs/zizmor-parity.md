# jactionlint and zizmor

[zizmor](https://docs.zizmor.sh/audits/) is a static analyzer for GitHub Actions security. jactionlint covers much of the same territory
with rules of the same names, and it is **not trying to be zizmor**. Strict compatibility is not a goal: where jactionlint can be
better for its users (fewer false positives, safer fixes, a way to adopt the checks gradually, composite actions, resilient online
checks) it differs on purpose, and where zizmor is ahead (more edge cases of some audits, a precomputed advisory snapshot) this page
says so. It records where jactionlint stands relative to zizmor 1.30.1, audit by audit.

**The honesty rule.** A statement about parity rests on a differential run against zizmor over a corpus of real repositories, on a
unit test or on a fixture. A statement without one is marked *not measured*. A rule with no finding on the corpus has no measured
false-positive rate, only its synthetic cases. Comparisons use a corpus of local repositories, not GitHub as a whole.

How to read the table:

- **jactionlint rule** is the stable [rule ID](rules.md) that reports the audit. IDs reuse zizmor's audit names so users can map them.
- **Profile** is where the rule runs: `correctness`, `default` or `pedantic` (see [profiles](config.md#profiles) and the table of
  [personas](#profiles-and-personas)), `online` for the rules that run only with the opt-in `--online` flag whatever the profile, and
  `configured` for a rule that does nothing until you configure an allow or deny list.
- **Status** says how the rule compares with the audit: `full` (same findings on the measured corpus or on every synthetic case),
  `partial` (with what is missing), `different` (same ID, another meaning), or `not measured`.

| zizmor audit | jactionlint rule | Profile | Status |
| --- | --- | --- | --- |
| `adhoc-packages` | `adhoc-packages` | default | partial: installs of local tarballs (`npm install ../pkg/*.tgz`) are not called ad hoc, and positions in a folded block can differ. jactionlint also reports installs behind `sudo`. No `pwsh` scripts. See [the check](checks.md#check-adhoc-packages) |
| `anonymous-definition` | `anonymous-definition` | pedantic | full for workflows and jobs; not applicable to `action.yml` (GitHub requires its `name`); fixable |
| `archived-uses` | `archived-uses` | online | partial: all archived repositories are found; see [online audits](#online-audits) |
| `artipacked` | `artipacked` | default | full on the corpus, with a fix. A checkout that a later `git push` needs and `actions/checkout@v1` are not reported |
| `bot-conditions` | `bot-conditions` | default | partial: `github.actor`, `github.triggering_actor`, `github.actor_id` and `github.event.sender.*` compared with a bot (`==`, `contains()`, `startsWith()`, `endsWith()`) in job and step `if:`. `!=` and negated tests are not reported, as in zizmor. `--fix=unsafe` only for workflows with `pull_request`/`pull_request_target` events. Bot names without `[bot]` are known by ID and prefix only. Judged by the events that reach the job, also through local reusable workflows: not reported when only `push`-like events (`push`, `schedule`, `workflow_dispatch`, ...) can run it, and the suggestion says that the author field is missing on the other events of a mixed workflow. Composite actions: the fix needs every local caller to run on a pull request event |
| `cache-poisoning` | `cache-poisoning` | default | partial: a fixed table of cache actions, `tags-ignore` is not a release trigger, and neither is a `tags:` filter that lets no tag through or a tag-triggered workflow whose token is read-only and whose job publishes nothing (zizmor reports both). `setup-node` v5 and later and `docker/setup-buildx-action` are covered; the automatic cache of `setup-node` is judged by the `package.json` of the repository. A step guarded by `if: !startsWith(github.ref, 'refs/tags/')` is evaluated for the release scenarios, where zizmor reports it |
| `concurrency-limits` | `concurrency-limits` | default | full: only a missing `concurrency:` and the bare group form are reported, not `cancel-in-progress: false` |
| `dangerous-triggers` | `dangerous-triggers` | default | full; adds `issue_comment`, which zizmor flags from 1.31; reported at the trigger, zizmor reports at `on:`. `pull_request_target` with only `actions/labeler` is exempt in both |
| `dependabot-cooldown` | `dependabot-cooldown` | default | full on the corpus, plus true positives that zizmor 1.30.1 misses (it stops at the first update which satisfies the minimum). `--fix` needs the `default-days` option; zizmor has a built-in 7. The `semver-*-days` keys are checked by neither |
| `dependabot-execution` | `dependabot-execution` | default | not measured: no `allow` on the corpus, so both tools report nothing. Covered by unit tests only. The fix is unsafe (zizmor offers it too) |
| `excessive-permissions` | `excessive-permissions` | default | partial: write scopes, `write-all`, `read-all`, workflow-level default permissions (option) and, with `missing-permissions`, job-level default permissions. zizmor grades findings by persona and confidence, jactionlint has one level per rule |
| `forbidden-uses` | `forbidden-uses` | configured | partial: patterns follow zizmor documentation but were not measured against zizmor |
| `github-app` | `github-app` | default | partial: `actions/create-github-app-token` only, no dependabot side yet |
| `github-env` | `github-env` | default | partial: no finding on the corpus in either tool. jactionlint accepts values of trusted contexts (`github.sha`, `runner.*`) and `mktemp`/`date` substitutions that zizmor reports in `pull_request_target` and `workflow_run` workflows, and reports untrusted input under every trigger. Writes of values from step outputs or computed in the script are reported only for `pull_request_target` and `workflow_run`. `pwsh` and `cmd` are matched line by line. `inputs.*` of a composite action counts as an outsider's input. See [the check](checks.md#check-github-env) |
| `hardcoded-container-credentials` | `hardcoded-container-credentials`, the check of actionlint ([Hardcoded credentials](checks.md#check-hardcoded-credentials)) | correctness | not measured against zizmor |
| `impostor-commit` | `impostor-commit` | online | partial: same findings on the corpus; a repository with more than 1000 branches gives no verdict (100 without a token); see [online audits](#online-audits) |
| `insecure-commands` | `insecure-commands` | default | partial: workflow, job and step `env` (step `env` in `action.yml` too); fixable (unsafe). A job-level `"1"` is flagged by neither tool, the runner parses the variable as a boolean |
| `insecure-url-scheme` | `insecure-url-scheme` | default | none to compare: zizmor 1.30.1 applies it to the `repo:` URLs of `.pre-commit-config.yaml` only, which jactionlint does not lint. jactionlint's rule is a different one with the same ID: `http://`, `ftp://` and `git://` locations in `run:` scripts and `with:` inputs |
| `known-vulnerable-actions` | `known-vulnerable-actions` | online | partial: same findings on the corpus; branch refs are not judged; see [online audits](#online-audits) |
| `misfeature` | `misfeature` | default | partial: `pip-install` of setup-python and `shell: cmd`. Shells that are not well known are reported with the option `pedantic` of `misfeature`, which zizmor does in the auditor persona only and which is not compared. Composite actions: checked |
| `obfuscation` | `obfuscation` | default | partial: redundant segments at `uses:`, constant expressions and `format()` of literals outside `if:`, `fromJSON(toJSON(x))` and computed indices. Constants in `if:` are `constant-condition`; `fromJSON(toJSON(context))` is deliberately not reported; `if: vars[matrix.var_name] != 'off'` is reported without `${{ }}`; the `uses:` fix is unsafe. Composite actions: checked |
| `overprovisioned-secrets` | `overprovisioned-secrets` | default | full on the synthetic cases; no finding on the corpus |
| `ref-confusion` | `ref-confusion` | online | partial: no finding on the corpus by either tool; see [online audits](#online-audits) |
| `ref-version-mismatch` | `ref-version-mismatch` | online | partial: every mismatch of a tag comment is found; a missing or version-less comment and a branch comment are not reported; see [online audits](#online-audits) |
| `secrets-inherit` | `secrets-inherit` | default | full, reported at the `uses:` of the job like zizmor |
| `secrets-outside-env` | `secrets-outside-env` | pedantic | full; zizmor reports it only for the auditor persona. `workflow_call` workflows are skipped like zizmor does. Inside a folded `>-` scalar jactionlint reports the line of the scalar start |
| `self-hosted-runner` | `self-hosted-runner` | pedantic (info) | partial: literal `self-hosted` labels and matrix values; labels from other expressions are not resolved. Labels of hosted-runner providers such as namespace or blacksmith are not reported |
| `self-repository` | `self-repository` | pedantic (info) | full on the corpus, with an unsafe fix |
| `stale-action-refs` | `stale-action-refs` | online | partial: same findings except repositories with more than 1000 tags; see [online audits](#online-audits) |
| `superfluous-actions` | `superfluous-actions` | default | full: the same findings as zizmor, split like its personas (regular, and the pedantic ones with the option `pedantic`, which the `pedantic` profile turns on). jactionlint adds the archived `actions/create-release` and `actions/upload-release-asset`. See [the check](checks.md#check-superfluous-actions) |
| `template-injection` | `template-injection` | correctness | partial: see [template injection](#template-injection) |
| `typosquat-uses` | `typosquat-uses` | default | partial: one typo of the slug, not all of the transformations of zizmor |
| `undocumented-permissions` | `undocumented-permissions` | pedantic | partial: a comment above a scope counts, where zizmor accepts only a comment at the end of the line; `include-read` for zizmor's read scopes |
| `unpinned-images` | `unpinned-images` | default | partial: no per-persona split (an image without a tag against a tag without a digest; the `require-digest` option switches between them), images in `docker://` are `unpinned-uses`. For `container: ${{ matrix.image }}` zizmor points at the container and jactionlint at the matrix value that is not pinned |
| `unpinned-tools` | `unpinned-tools` | default | full for the 4 actions zizmor knows. The option `pedantic` (on under the `pedantic` profile) adds `run:` installs without an exact version and three more actions. An input that is an expression is not reported, zizmor reports it with low confidence. See [the check](checks.md#check-unpinned-tools) |
| `unpinned-uses` | `unpinned-uses` | default | partial: `hash-pin`, `ref-pin`, `any` policies with the patterns `*`, `owner/*`, `owner/repo` and `owner/repo/path`; other pattern forms are rejected when the configuration is read. Docker images follow the `*` pattern only |
| `unredacted-secrets` | `unredacted-secrets` | default | full on the synthetic cases (any `fromJSON(secrets.X)` is reported, also without a field access); no finding on the corpus |
| `unsound-condition` | `if-always-true` (existing) | correctness | full for workflows: `if-always-true` reports every `if:` with characters around `${{ }}`, a block scalar's trailing newline included. No new rule. Composite action steps are checked too (not measured) |
| `unsound-contains` | `unsound-contains` | default | full on the synthetic cases; no finding on the corpus |
| `unsound-ternary` | `unsound-ternary` | correctness | not measured: no corpus run against zizmor, covered by fixtures only |
| `use-trusted-publishing` | `use-trusted-publishing` | default | partial: reusable workflows without `permissions:` are skipped because the caller may grant `id-token: write`. No `pwsh` npm publish and no `npm run publish`. A blank `NODE_AUTH_TOKEN` and a token from `rust-lang/crates-io-auth-action` or `NuGet/login` are trusted publishing and are not reported; `cargo publish -p` is understood. A long-lived credential next to `id-token: write` is reported. See [the check](checks.md#check-use-trusted-publishing) |

`unused-ignore` does not report `# zizmor: ignore[...]` comments unless its `zizmor` option is on; see [ignore comments](#ignore-comments).

## Profiles and personas

zizmor gives every finding a persona (`regular`, `pedantic` or `auditor`). jactionlint has three [profiles](config.md#profiles), each
including the one before it. The `regular` persona is the `default` profile, `pedantic` and `auditor` are the `pedantic` profile, and an
audit with findings of both is one rule with the option `pedantic` for the noisier findings.

| zizmor audit | Persona | jactionlint |
| --- | --- | --- |
| `anonymous-definition`, `undocumented-permissions` | pedantic | `pedantic` profile |
| `self-hosted-runner`, `secrets-outside-env` | auditor | `pedantic` profile |
| `concurrency-limits` | pedantic | `default` profile, a decision of the maintainer; it has a safe fix for workflows that only pull requests start |
| `template-injection`, `superfluous-actions`, `unpinned-tools`, `unlocked-install` | regular and pedantic | `default` profile, the pedantic findings behind the option `pedantic` |
| `misfeature` | regular, the custom shells auditor | `default` profile, custom shells behind the option `pedantic` |
| `artipacked`, `excessive-permissions`, `obfuscation`, `unpinned-images`, `unpinned-uses`, `insecure-commands` | regular, with pedantic or auditor findings for special cases | `default` profile; the special cases are not separate findings |
| `ref-version-mismatch`, `stale-action-refs` | pedantic | online (`--online`) |
| every other audit | regular | `default` profile, or online |

`unredacted-secrets`, `overprovisioned-secrets` and `typosquat-uses` are in `default` by decision of the maintainer, not because a
corpus showed their false-positive rate: they are **unmeasured** there.

## Lessons applied

The issue tracker of zizmor is a list of false positives and false negatives that a rule of the same shape can have. Each item below
has a fixture in `pitfalls_test.go`, so it is covered by a test, not by the corpus. "Handled" means jactionlint avoids the problem;
"Not affected" means the fixture passes without a special case and is kept.

| zizmor | What went wrong there | jactionlint |
| --- | --- | --- |
| [#2219](https://github.com/zizmorcore/zizmor/issues/2219) | `github-app` flags `owner` without `repositories` although the token only has organization permissions | Handled: no finding when every `permission-*` input is an organization permission |
| [#1914](https://github.com/zizmorcore/zizmor/issues/1914) | `bot-conditions` flags `github.actor == 'dependabot[bot]'` next to a check of the pull request author | Handled: a conjunction with a check of the author of the pull request is not reported |
| [#2059](https://github.com/zizmorcore/zizmor/issues/2059) | findings in steps with `if: false` | Handled for every rule outside the correctness group, in one place: findings in a job or a step whose `if:` is the literal false are dropped |
| [#1098](https://github.com/zizmorcore/zizmor/issues/1098), [#1043](https://github.com/zizmorcore/zizmor/issues/1043) | `artipacked` flags a checkout that a later `git push` needs, and `actions/checkout@v1` | Handled: neither is reported (a push next to an upload of the workspace still is) |
| [#2320](https://github.com/zizmorcore/zizmor/issues/2320) | `cache-poisoning` on `astral-sh/setup-uv` v10 with `enable-cache: auto` | Handled: version aware, the version is read from the comment of a pinned commit too. The guard `if: github.event_name != 'release'` on a step is honored |
| [#1848](https://github.com/zizmorcore/zizmor/issues/1848) | `use-trusted-publishing` is silent when `id-token: write` is granted (for provenance or a cloud login), and misses PowerShell | Handled: a long-lived credential next to `id-token: write` is reported, and `pwsh` scripts are searched line by line |
| [#1479](https://github.com/zizmorcore/zizmor/issues/1479) | `dependabot-cooldown` on OpenTofu | Not a flaw: the options reference of GitHub lists `default-days` for every ecosystem, OpenTofu included, and only `semver-*-days` is limited. A test keeps every ecosystem reported |
| [#1481](https://github.com/zizmorcore/zizmor/issues/1481) | `copilot-setup-steps.yml` is not an ordinary workflow | Handled for `concurrency-limits` and `anonymous-definition`. `missing-timeout` still applies, because the file accepts `timeout-minutes` |
| [#1619](https://github.com/zizmorcore/zizmor/issues/1619) | `concurrency-limits` skips a caller whose jobs all call reusable workflows | Handled: the caller is reported, because the called workflow cannot hold the limit without deadlocking |
| [#1865](https://github.com/zizmorcore/zizmor/issues/1865) | `superfluous-actions` on self-hosted runners | Handled: not reported there |
| [#2433](https://github.com/zizmorcore/zizmor/issues/2433) and the other `ref-version-mismatch` issues | a second comment after the version, prereleases, sibling tags | Not affected: the version is the first word of the comment |
| [#2321](https://github.com/zizmorcore/zizmor/issues/2321) | `ref-confusion` ignores hash-pinned refs, but a branch can be named like a hash | Handled for abbreviated SHAs, which are names to GitHub. Full SHAs are read as commits and are not looked up |
| [#2130](https://github.com/zizmorcore/zizmor/issues/2130) | `impostor-commit` and annotated tag objects | Not affected: the client dereferences annotated tags, and a SHA that GitHub cannot compare is no verdict, not an impostor |
| [#1673](https://github.com/zizmorcore/zizmor/issues/1673) | `secrets: inherit` under `on.workflow_call` | Open: the documentation and the workflow schema of GitHub show only a mapping there, so the syntax error stays. Whether GitHub runs such a file is not tested |
| [#2210](https://github.com/zizmorcore/zizmor/issues/2210) and others | one 403, 404 or 5xx stops the online run | Not affected: a failed lookup is skipped with one warning (`--verbose` lists them). A test covers 404, 403, 500 and 502 |
| [GHSA-f42p-wjw5-97qh](https://github.com/zizmorcore/zizmor/security/advisories/GHSA-f42p-wjw5-97qh) | credentials in the debug log | Not affected: a test runs `--online --debug --verbose` with a token and finds it nowhere |

## Differential harness

`mise run zizmor-diff` compares jactionlint with zizmor 1.30.1 on the repositories of `scripts/zizmor-diff/corpus.txt`, with the
`pedantic` profile of `scripts/zizmor-diff/pedantic.yaml` and zizmor `--offline --persona pedantic`. A finding matches when both tools
report the same audit in the same file on the same line, or within a few lines where one tool points at the first line of a step and the
other at its `uses:`. The online audits (`--online`) are measured separately, see [online audits](#online-audits). Only workflow files
are compared. Rules with no entry in the mapping of the harness are always "only jactionlint" in its report.

On the 23 repositories of the corpus that are checked out, zizmor reports 999 findings and jactionlint reports 831 of them. Audits
with a finding:

| zizmor audit | zizmor | also reported by jactionlint | missed |
| --- | --: | --: | --: |
| `anonymous-definition` | 323 | 323 | 0 |
| `template-injection` | 172 | 172 | 0 |
| `undocumented-permissions` | 166 | 145 | 21 |
| `excessive-permissions` | 163 | 33 | 130 |
| `concurrency-limits` | 75 | 74 | 1 |
| `artipacked` | 64 | 50 | 14 |
| `self-repository` | 17 | 17 | 0 |
| `cache-poisoning` | 10 | 8 | 2 |
| `dependabot-cooldown` | 4 | 4 | 0 |
| `superfluous-actions` | 2 | 2 | 0 |
| `unpinned-images`, `unpinned-uses`, `use-trusted-publishing` | 1 each | 1 each | 0 |

Every other mapped audit has no finding on either side. The 168 misses are not analyzed: `excessive-permissions` is `partial` by design
(and the harness maps `missing-permissions` to it too), but the rest may be gaps worth a look.

jactionlint reports findings that zizmor does not on this corpus: `require-shell`, `require-expression-wrapping`,
`mutable-runner-label`, `secrets-outside-env`, `missing-timeout`, `pipeline-without-pipefail`, `unknown-runner-label`,
`checkout-static-credentials`, `self-hosted-runner`, `workflow-secret-scope`, `unverified-download`, `shellcheck`, `max-run-lines`,
`continue-on-error`, `concurrency-cancels-release`, `unused-job-output` and `unused-needs`, plus surplus findings of mapped rules
(`undocumented-permissions`, `concurrency-limits`, `template-injection`, `cache-poisoning`, `artipacked`, `bot-conditions`,
`unpinned-tools`). The surplus findings are not reviewed one by one.

Known causes of differences between the tools:

- `undocumented-permissions`: zizmor accepts only a comment at the end of the line, jactionlint accepts one above the scope too.
- `artipacked` and `cache-poisoning`: a checkout or a step with a `# zizmor: ignore[...]` comment is hidden for zizmor and reported
  by jactionlint unless the comment maps onto a rule; see [ignore comments](#ignore-comments).
- `cache-poisoning`: zizmor treats a workflow with `push: {tags-ignore: ["*"]}` as a release workflow, which says the opposite, so
  jactionlint does not. jactionlint also reports a job that runs `gh release upload` or pushes an image and restores a cache
  (`jdx/mise-action` with `cache_save: false`, which only stops saving, and `docker/build-push-action` with `cache-from: type=gha`),
  where zizmor looks at triggers and not at jobs.
- `dangerous-triggers`: jactionlint also reports `issue_comment`, and reports at the trigger where zizmor reports at `on:`.

## Intentional differences: where jactionlint goes beyond zizmor

<a id="beyond-zizmor"></a>

These are choices, not gaps.

### Rules of our own

Rules that zizmor 1.30.1 has no audit for. They are not in the table above (`insecure-url-scheme` has the name of an audit of zizmor
that checks `.pre-commit-config.yaml`; the workflow URL check has no counterpart there), and a finding of one of them is always "only
jactionlint" in the harness report. The idea behind some of them is an open issue of zizmor. Every rule of `default` reports at
`error`; the `pedantic` ones keep their own level.

| Rule | Profile | What it reports | Notes |
| --- | --- | --- | --- |
| `missing-timeout`, `timeout-too-long` | default, configured | a job without `timeout-minutes`; a value above `max`. zizmor [#2166](https://github.com/zizmorcore/zizmor/pull/2166) is open | `missing-timeout` is noisy for a repository that never set the key; `--fix` with `default-minutes` clears it |
| `unlocked-install` | default | `cargo install` without `--locked` (fixable, unsafe), and `npm install`, `yarn` and `pnpm install --no-frozen-lockfile` when the repository has the lock file the install ignores. With the option `pedantic`: the same without a lock file, `bun install` without `--frozen-lockfile` and `pip install -r` without hashes or constraints. See [the check](checks.md#check-unlocked-install) | unit tests and fixtures |
| `dependabot-missing-actions-update` | pedantic | `dependabot.yml` has no `github-actions` update although `.github/workflows` uses actions; skipped when the repository has a Renovate configuration. [The check](checks.md#check-dependabot-missing-actions-update) | |
| `pipeline-without-pipefail` | default | a failure in a pipeline of a `run:` script is hidden because the default shell (`bash -e {0}`) and `shell: sh` do not enable pipefail. zizmor [#288](https://github.com/zizmorcore/zizmor/issues/288) is the open idea | unit tests and fixtures |
| `agentic-actions` | default | AI agent actions ([zizmor#1605](https://github.com/zizmorcore/zizmor/issues/1605) is a proposal): an agent outsiders can steer without a check of the user, an open gate (`allowed_non_write_users: '*'`), settings that turn safeguards off, an agent on the code of a pull request. [The check](checks.md#check-agentic-actions) | [see below](#agentic-actions) |
| `unverified-download` | default | a download piped into a shell or an interpreter, a downloaded file made executable and run without a checksum or signature check, TLS verification turned off. The idea is zizmor [#711](https://github.com/zizmorcore/zizmor/issues/711) | [see below](#downloads-and-credentials) |
| `insecure-ssh-keyscan` | default | `ssh-keyscan` output written to a `known_hosts` file. The idea is zizmor [#2012](https://github.com/zizmorcore/zizmor/issues/2012) | [see below](#downloads-and-credentials) |
| `checkout-static-credentials` | default | `actions/checkout` given an `ssh-key` or a literal `token`; with the option `secret-tokens` (on under the `pedantic` profile) also a token from a secret other than `GITHUB_TOKEN`. The idea is zizmor [#1118](https://github.com/zizmorcore/zizmor/issues/1118) | [see below](#downloads-and-credentials) |
| `workflow-secret-scope` | pedantic | a secret assigned to the workflow-level `env` that reaches several jobs | [the check](checks.md#check-workflow-secret-scope) |
| `insecure-url-scheme` | default | `http://`, `ftp://` and `git://` locations in `run:` downloads and `with:` inputs. zizmor's audit of this name looks at `.pre-commit-config.yaml` only | |
| `invisible-characters` | default | invisible and bidirectional control characters ([zizmor#914](https://github.com/zizmorcore/zizmor/issues/914)), in workflows and `dependabot.yml`, also in a file that does not parse. Safe fix | no true positive on the corpus: unit tests and fixtures. Emoji variation selectors (`U+FE0F` after `ℹ`, `✏`, `⚠`) are not reported |
| `unsound-prefix-match` | default | `startsWith()`, `endsWith()` and `contains()` on the name of an account or repository ([zizmor#1533](https://github.com/zizmorcore/zizmor/issues/1533)); `unsound-contains` is the sibling for a literal haystack | an expression outside `if:` is reported only in a runner, environment, container, service or workflow-call-secret context, or when it reads a secret or `github.token`; `endsWith(.., '[bot]')` and `!contains(github.actor, '[bot]')` are not reported on purpose. The option `refs` is opt-in |
| `concurrency-cancels-prs` | default | a `cancel-in-progress` group that all pull requests share. `concurrency-limits` asks for a missing `concurrency`, not for a wrong group | |
| `concurrency-cancels-release` | default | `cancel-in-progress` on tag pushes, releases and jobs that publish or deploy; recommends `cancel-in-progress: false`, a group per ref or tag and `queue: max` (fixable, unsafe) | see [limits of the workflow rules](#limits-of-the-workflow-rules) |
| `gate-job-skipped-on-failure` | default | a job that reads `needs.*.result` but is skipped when a needed job fails | no finding on the corpus; the fan-in jobs there use `always() && !cancelled()` |
| `untrusted-checkout` | default | `pull_request_target` and `workflow_run` workflows that check out the pull request and run it. `dangerous-triggers` reports the trigger alone | see [limits of the workflow rules](#limits-of-the-workflow-rules) |
| `untrusted-artifact` | default | `workflow_run` workflows that run, extract or export the artifact of the triggering run unchecked (zizmor [#195](https://github.com/zizmorcore/zizmor/issues/195) is the open idea) | see [limits of the workflow rules](#limits-of-the-workflow-rules) |
| `unused-job-output`, `duplicate-triggers` | default | a job output nobody reads; `push` without a branch filter next to `pull_request` | `unused-job-output` also reports generated `*.lock.yml` workflows; exclude them with `paths:` and `ignore:` if you do not want them |
| `unused-workflow-input`, `unused-needs`, `continue-on-error`, `mutable-runner-label` | pedantic | an input never read; a redundant `needs` entry; `continue-on-error: true`; `ubuntu-latest` and other moving labels (fixable with the `pin` option) | `unused-workflow-input` is pedantic because a dispatching tool can require an input the workflow never reads (GitHub rejects a dispatch with an undeclared input); `continue-on-error` finds jobs that are advisory on purpose, which is why it is info |
| `require-shell`, `require-expression-wrapping`, `max-run-lines` | pedantic | conventions: explicit `shell:`, `${{ }}` around `if:` expressions, script length | unit tests; opt-in conventions with no zizmor counterpart |
| `unused-ignore`, `expired-ignore`, `unused-baseline-entry` | pedantic, correctness | housekeeping of ignores and of the baseline | |

Sinks that zizmor lacks, as part of `template-injection`: `container.options` and `services.<id>.options`
([zizmor#1128](https://github.com/zizmorcore/zizmor/issues/1128), still open), their image, entrypoint, command and volumes, `args`
and `entrypoint` of `docker://` steps, more code inputs of well-known actions, and the prompt, arguments and settings of AI agent
actions.

### Features

- **Composite actions with a caller-aware context.** zizmor audits `action.yml` files but has no knowledge of who calls them
  ([#1424](https://github.com/zizmorcore/zizmor/issues/1424), [#678](https://github.com/zizmorcore/zizmor/issues/678)).
  jactionlint takes the triggers of an action from the local workflows that call it, and judges a published action that no local
  workflow calls by its own steps only. See [composite actions](#composite-actions).
- **A baseline for gradual adoption** ([zizmor#2282](https://github.com/zizmorcore/zizmor/issues/2282), declined there).
  `jactionlint --baseline-write` records the current findings and `--baseline` hides them, so a repository can adopt the stricter
  default and fail only on new findings. Entries are keyed by file, rule ID and a fingerprint, not by line numbers;
  `--baseline-check` lists the entries that match nothing any more, `--format summary` counts findings per rule and file, and SARIF marks
  baselined results as suppressed. Tested with unit tests and by hand; no comparison is possible. See [the usage document](usage.md#baseline).
- **Durable ignores** (`ignores:` in the config file, rules `expired-ignore` and `unused-ignore`). zizmor ignores a finding with a
  `# zizmor: ignore[audit]` comment or a `file:line:col` entry in `zizmor.yml`, and both break when a tool rewrites the line or when
  lines shift ([zizmor#1086](https://github.com/zizmorcore/zizmor/issues/1086)). jactionlint matches by rule plus file glob, job ID,
  step ID or name and the `uses:` value (so a Renovate bump of the SHA keeps the entry working), with an optional reason and expiry
  date. No equivalent in zizmor to measure against. See [durable ignores](config.md#durable-ignores).
- **Ignore comments at the end of a line** (`# jactionlint ignore=...`) cover the whole step when they are on its first line. A
  `# zizmor: ignore[...]` comment is honored as well, see [ignore comments](#ignore-comments).
- **Fixes that converge.** `--fix` lints, fixes and lints again until nothing is left, writes each file once and atomically, and checks
  every pass (valid YAML, only the edits changed); a fix that breaks a file is refused. `--diff` shows the change, `--fix --fix-rules` limits
  it, and SARIF carries the safe fixes so [hk](usage.md#hk) can apply them. A fixer never invents a value (`missing-timeout` and
  `dependabot-cooldown` need `default-minutes` and `default-days`), where zizmor's `dependabot-cooldown` fix has a built-in 7. See
  [fixers](#fixers).
- **An online client that survives failures.** [Where it differs from zizmor](#online-client-where-it-differs-from-zizmor).
- **One ID per audit and three profiles.** zizmor's personas become [profiles](#profiles-and-personas), and the pedantic findings of an
  audit are one option of the same rule, not a second ID.

### Where a rule differs on purpose

- `dangerous-triggers` also reports `issue_comment` (zizmor flags it from the version after 1.30.1) and reports at the trigger, where
  zizmor reports at `on:`.
- `undocumented-permissions` accepts a comment above a scope as well as at its end of line (zizmor: end of line only).
- `cache-poisoning` does not treat `push: {tags-ignore: ["*"]}` as a release workflow.
- `ref-version-mismatch` does not report a missing or version-less comment (zizmor does, as a pedantic finding): a missing comment is
  not a mismatch. This is a gap, and a candidate for a separate offline rule.
- `obfuscation` does not report `fromJSON(toJSON(context))`; constants in `if:` are `constant-condition` and `if-always-true`.
- A rule has one level, where zizmor grades a finding by confidence and persona. Every rule of `correctness` and `default` is an
  `error`; the `pedantic` findings of an audit share the level of the rule.

## What is not done

These ideas from the zizmor tracker are **not built**: a check of `allow-unsafe-pr-checkout` of `actions/checkout` and of code that a
pull request controls (zizmor [#2134](https://github.com/zizmorcore/zizmor/issues/2134)), TOCTOU refs
([#935](https://github.com/zizmorcore/zizmor/issues/935)), shell syntax errors without shellcheck
([#2322](https://github.com/zizmorcore/zizmor/issues/2322)), a rule for a SHA without a version comment
([#2316](https://github.com/zizmorcore/zizmor/issues/2316)), immutable releases under `--online`
([#766](https://github.com/zizmorcore/zizmor/issues/766)), credentials in the `registries` of `dependabot.yml`
([#1222](https://github.com/zizmorcore/zizmor/issues/1222)), a repojacking check by owner lookup
([#479](https://github.com/zizmorcore/zizmor/issues/479)), and zizmor's precomputed snapshot of tags and advisories, which would cut the
first-run cost of `--online`. The registry of rules, [rules.md](rules.md), is the list of what exists.

## Template injection

`template-injection` reports attacker controlled contexts, objects holding them and env variables set from them in `run:`,
github-script and the code inputs of well-known actions, and every expression of a script, not only the first. Free text that is not
validated and chosen from outside the workflow is reported by the `default` profile too: `inputs.*` of type string,
`github.event.inputs.*`, `github.event.client_payload.*`, `github.event.release.tag_name` and the like, `github.ref_name` and
`github.base_ref`. Booleans, numbers and choices are fixed vocabularies and are not reported. Every other expansion (step outputs,
matrix values, and values like `github.repository`) is reported with the option `pedantic`, which is zizmor's pedantic persona.
`--fix` moves a simple reference into `env:` for bash and sh.

On the corpora compared with zizmor:

- The `default` rule reports no false positive seen, and zizmor reports every line it reports. The corpus was too clean to say how it
  compares with zizmor's high-confidence findings.
- With the option `pedantic` the rule reports every line zizmor does, plus lines zizmor does not: `matrix.*` whose values are
  literals and `needs.*.result`, values of `matrix.*` from `fromJSON`, `env.*` set from `$GITHUB_ENV`, `steps.*.outputs.*` of actions
  that zizmor knows to return safe values (jactionlint does not have that knowledge), and `github.ref_name` and
  `github.event.inputs.*`. These are not false positives of a security finding.
- The sinks for agent prompts, container options and `docker://` steps (see above) report no false positive on the sampled
  workflows; the container and service sinks are rare and are covered by golden tests.
- Left unreported on purpose: `github.actor` (a name GitHub restricts to letters, digits and hyphens) and inputs of the type
  `choice`, `boolean` and `number` (fixed vocabularies).
- Not covered: knowledge about the outputs of popular actions, and severity by trigger (the level is per rule ID).

## Agentic actions

`agentic-actions` reports AI agent actions that outsiders can steer. On a sample of public workflows that use agent actions, the
findings were all true: agents without a check of the user (no `if:`, `environment:` or permission step) with a token that can write
contents, agents that run on a pull request checkout, and allowed tools that give any code (`Bash(npx:*)`, `Bash(bunx:*)`,
`Bash(xargs:*)`). What the rule does not report, on purpose:

- A triage job with a read-only token and a Codex `sandbox: read-only` or a Gemini `tools.core` list, the architecture the vendors
  advise.
- `workflow_run` as an outsider trigger for the checks of the user and of the settings (it is as safe as the workflow it follows).

It reads settings files with trailing commas or an expression in a detail of the JSON, and knows that Codex on Windows needs
`safety-strategy: unsafe`. zizmor reports no `${{ }}` in the prompt of an agent. The rule reports at `error` level whatever the
trigger, where the proposal for zizmor would grade by trigger and persona. The actions are a table (`agent_actions.go`) that must be
kept up to date.

## Downloads and credentials

`unverified-download`, `insecure-ssh-keyscan`, `insecure-url-scheme` and `checkout-static-credentials` have no audit in zizmor to
compare with. Every finding on the corpora was read and judged true.

`checkout-static-credentials` reports a token from a secret only with `secret-tokens`, which is off under the `default` profile and on
under the `pedantic` profile: the findings are release workflows that have to push with a personal access token so that the push starts
other workflows, which is deliberate. They are correct findings, but reporting them for everybody would be noise for workflows whose
alternative is a GitHub App or a fine-grained token in a protected environment, so that part is behind the option and `ssh-key` is not.

Not reported: archives that are downloaded, extracted and run, `go install` or `npx` of a remote package,
`ssh -o StrictHostKeyChecking=no`, and a download whose verification is in another step.

## Limits of the workflow rules

- `untrusted-checkout` follows `actions/checkout`, `gh pr checkout` and `git` commands with a literal reference. A ref that comes from
  an output of an earlier step is not followed. "Runs it" is a list of commands: an unusual build tool that is not in the list of
  commands that only read files counts as running code, while an action that builds the workspace and is not in the short list does
  not. Conditions on `needs` and `steps` are guards.
- `untrusted-artifact` links the use to the download by the `path` of the artifact, or its `name` (a file in the root of the workspace
  that has another name is not linked), and takes any `=~`, numeric test or checksum as validation.
- `concurrency-cancels-release` knows a fixed list of publish and deploy actions and commands. A release done by a script of the
  repository (`./scripts/release.sh`) is not recognized; the workflow is only reported for tag pushes and `release` events then. Names
  of workflows and jobs are not a signal, a tag only counts when the group does not name the ref, a job or step whose `if:` is false for
  the trigger is ignored, and an expression is reported only when the event and the ref decide that it is true.
- `unused-needs` finds only the entries that another needed job makes redundant. A forgotten entry that is not redundant is
  indistinguishable from one that sets the order, so it is left out on purpose.
- None of these rules reads the called reusable workflows: a job output or input that a caller in another repository uses is not
  known (for `unused-job-output` it cannot be used from outside, for `unused-workflow-input` it can).

## Fixers

`jactionlint --fix` with `default-minutes` configured adds exactly one `timeout-minutes` line per job without it, adds the safe
`permissions:` blocks and changes no other line (`diff -r` shows added lines only); `--fix=unsafe` adds the unsafe blocks. After either
run the rules report nothing that has a fix, and a second run changes nothing. This is exercised with `scripts/fix-corpus` over the
distinct workflow files of the local corpus.

## Online audits

The six online audits run only with `--online`. They are compared with zizmor with `go run ./scripts/zizmor-diff --online` (zizmor
`--persona pedantic`, with a token), and by running both tools on every workflow file of the corpus. A finding is shared when both
tools report the same audit in the same file within two lines. The numbers are for the corpus, not for GitHub as a whole.

| audit | zizmor | shared | zizmor only | jactionlint only |
| --- | --: | --: | --: | --: |
| `impostor-commit` | 2 | 2 | 0 | 1 |
| `known-vulnerable-actions` | 0 | 0 | 0 | 0 |
| `ref-confusion` | 0 | 0 | 0 | 0 |
| `stale-action-refs` | 7 | 6 | 1 | 0 |
| `archived-uses` | 0 | 0 | 0 | 0 |
| `ref-version-mismatch`, tag comment does not match | 182 | 182 | 0 | 0 |
| `ref-version-mismatch`, comment names a branch | 2 | 1 | 1 | 0 |
| `ref-version-mismatch`, missing or version-less comment | 42 | 0 | 42 | 0 |

Older checkouts of the same repositories, which hold older workflows and so more vulnerable actions, add `known-vulnerable-actions`
and `stale-action-refs` findings that jactionlint reports as well, except for the case below.

What the differences are:

- **`impostor-commit`, jactionlint only.** A workflow with a `# zizmor: ignore[impostor-commit]` comment, which only zizmor reads. A
  true finding.
- **`stale-action-refs`, zizmor only.** `taiki-e/install-action` has more than 1000 tags, jactionlint reads 1000 per repository, and
  says nothing about a commit it did not find in a truncated list.
- **`ref-version-mismatch`, missing or version-less comment.** zizmor reports a hash-pinned action without a version comment (a
  pedantic finding). jactionlint does not: a missing comment is not a mismatch. This is a gap, and a candidate for a separate offline
  rule.
- **`ref-version-mismatch`, comment names a branch.** zizmor also compares a comment such as `# stable` with the branch of that name.
  jactionlint only reads comments that start with a version.
- **No finding for either tool** for `ref-confusion` and `archived-uses`, and for `known-vulnerable-actions` on current workflows:
  those rules are exercised by fixtures, not by a measurement against zizmor.

Gaps of the online rules that the corpus does not show, so they are not measured:

- `impostor-commit` treats a commit as legitimate when a tag points at it or a branch contains it. A commit which is only in the
  history of a tag (a deleted release branch) is reported.
- Without a token jactionlint cannot use GraphQL, compares at most 100 branches one request at a time, and says nothing for a larger
  repository where no branch has the commit.
- Reusable workflows are checked like actions; composite action files (`action.yml`) are checked too, with the steps of a composite
  action treated like workflow steps.

Cost: for a repository with 173 `uses:` lines the first run makes about 91 requests, a run within the hour none, and a later run
only conditional requests (a 304 answer is free of rate limit for authenticated clients).

### Online client: where it differs from zizmor

Not compatible by design; the client answers zizmor issues [#1350](https://github.com/zizmorcore/zizmor/issues/1350) and
[#2210](https://github.com/zizmorcore/zizmor/issues/2210) (one failed lookup aborts the audit) and the credentials in debug logs of
[GHSA-f42p-wjw5-97qh](https://github.com/zizmorcore/zizmor/security/advisories/GHSA-f42p-wjw5-97qh). Covered by unit tests against an
`httptest` server for every failure mode, not compared with zizmor.

- A lookup which fails (404, 403, 5xx, timeout, DNS) is skipped for that `uses:` only. One warning per kind of failure, exit status
  unchanged unless `--online=strict`.
- Rate limits are read from `X-RateLimit-*` and `Retry-After`; a reset within 30 seconds is waited for, a later one skips with the
  reset time in the message. 5xx and secondary limits are retried with exponential backoff and jitter, at most twice.
- `allow` and `deny` lists of `owner/repo` patterns, a GitHub Enterprise Server host from `GITHUB_API_URL`, `GITHUB_SERVER_URL`, `GH_HOST`
  or `--online-api-url`, and a token from a named variable, a file, the usual variables or `gh auth token`.
- The token is sent to the API host only, is redacted from all output, and is never sent to a host that a repository's own config file chose.
- `--online=cache` answers from the disk cache only, without the network.

Not done: zizmor's precomputed snapshot of tags and advisories, which would cut the first-run cost further.

## Ignore comments

jactionlint honors zizmor's `# zizmor: ignore[...]` comments for the audits that map onto one of its rules, so a repository that
already triaged its zizmor findings keeps them triaged. See [the usage document](usage.md#zizmor-ignore-comments) and
[the alias table](v2-migration.md#zizmor-ignore-comments).

On a corpus of workflows that carry such comments, the audits named most often are `cache-poisoning`, `dangerous-triggers`,
`artipacked`, `use-trusted-publishing`, `adhoc-packages`, `impostor-commit` and `template-injection`, and every one of them is a
jactionlint rule. Many of the comments are stale (`unused-ignore` reports them, mostly for `cache-poisoning`). Stale means that
jactionlint reports nothing there, not necessarily a gap: zizmor reports no `cache-poisoning` finding on the one file checked by hand
once the comments are removed. `impostor-commit` comments are only exercised with `--online`.

## Composite actions

zizmor audits `action.yml` files with the same audits as workflows, but it has no knowledge of who calls an action
([#1424](https://github.com/zizmorcore/zizmor/issues/1424), [#678](https://github.com/zizmorcore/zizmor/issues/678)).
jactionlint checks the steps with the same rules, takes the trigger of an action from the local workflows that call it, and
documents which rules apply in [composite actions](checks.md#check-composite-actions).

On the action files of the corpus, compared with zizmor within 8 lines (a multi-line expression is reported at its first line by
jactionlint and at another line by zizmor):

| Audit | Differences |
| --- | --- |
| `template-injection` | jactionlint reports the findings of the option `pedantic`, which are zizmor's pedantic persona; the extra findings are expansions of values zizmor does not list. A second line of one multi-line expression is missed |
| `unpinned-uses` | the extra findings are `actions/*@tag` refs, which zizmor's default policy allows |
| `github-app` | none |
| `self-repository` | the extra findings are the repeated `uses: ./.github/actions/...` of the same local action in two files, of which zizmor reported none |
| `github-env`, `adhoc-packages` | not compared on composite actions; both rules run on them (`github-env` with the events of the callers) |

The jactionlint-only findings were reviewed:

- `action-syntax`: `type:` keys on action inputs, which GitHub ignores.
- `undefined-property`: a step of the calling job read through `steps`, which a composite action cannot see.
- `cache-poisoning`: a cache restored by an action called from a workflow on pushed tags. The caller's input expression is
  evaluated for the release scenarios.
- The `${{ github.token }}` default of an input is valid (context availability of defaults).

No false positive remains in the reviewed sets. `matrix.*` and `needs.*` in a composite action are not reported as undefined, because
GitHub does not document that the runner withholds them.

## Decisions kept

These are reported or left unreported on purpose:

- `bot-conditions` on `github.event.sender.id` is reported: the account of the last event is not the author of the change, with an id
  as well as with a login. It is not reported where no pull request event can reach the job, as in a reusable workflow that only a
  `push` workflow calls.
- `invalid-local-action` never reports a missing local action in a composite action (actionlint issues #25 and #40), because it can be
  a checkout of a private repository.
- `self-repository` is a zizmor regular finding that the profile keeps in `pedantic`.
- `concurrency-limits` changes its advice to `cancel-in-progress: false` for workflows that release or deploy, so that it does not
  contradict `concurrency-cancels-release`; the fixer only applies to workflows of pull requests.
- `workflow-call-permissions` without `assume-default-permissions` reports only what no default token has (`id-token`).
