# jactionlint and zizmor

[zizmor](https://docs.zizmor.sh/audits/) is a static analyzer for GitHub Actions security. jactionlint covers much of the same territory
with rules of the same names, and it is **not trying to be zizmor**. Strict compatibility is not a goal: where jactionlint can be
better for its users (fewer false positives, safer fixes, a way to adopt the checks gradually, composite actions, resilient online
checks) it differs on purpose, and where zizmor is ahead (more audits' edge cases, a precomputed advisory snapshot) it says so below.
This page records where jactionlint stands relative to zizmor, audit by audit, with the measurements behind each statement.

**The honesty rule.** A statement about parity needs a measurement: a differential run against zizmor over a corpus of real
repositories, a unit test, or a fixture, and the page says which. A claim without one is marked *not measured*. A rule with no finding
on the corpus has no measured false-positive rate, only the synthetic cases. The measurements are from zizmor 1.30.1 (the latest
release when they were taken, 2026-10-08) and from a corpus of local repositories, not from GitHub as a whole, and they date from the
batch named in each section; a rule that changed since is re-measured only where a section says so.

How to read the table:

- **jactionlint rule** is the stable [rule ID](rules.md) that reports the audit. IDs reuse zizmor's audit names so users can map them.
- **Profile** is where the rule runs: `correctness`, `default` or `pedantic` (see [profiles](config.md#profiles) and the table of
  [personas](#profiles-and-personas)), `online` for the rules that run only with the opt-in `-online` flag whatever the profile, and
  `configured` for a rule that does nothing until you configure an allow or deny list.
- **Status** says how the rule compares with the audit: `full` (same findings on the measured corpus or on every synthetic case),
  `partial` (with what is missing), `different` (same ID, another meaning), or `not measured`. The sections that measure a batch name
  the profiles of their time, see [profiles and personas](#profiles-and-personas).

| zizmor audit | jactionlint rule | Profile | Status |
| --- | --- | --- | --- |
| `adhoc-packages` | `adhoc-packages` | default | partial: 7 of the 10 findings of zizmor on the corpus. The 3 others are 2 installs of local tarballs (`npm install ../pkg/*.tgz`), which jactionlint does not call ad hoc, and a position difference in a folded block. jactionlint reports installs behind `sudo` that zizmor misses. No `pwsh` scripts. See [the check](checks.md#check-adhoc-packages) |
| `anonymous-definition` | `anonymous-definition` | pedantic | full for workflows and jobs; not applicable to `action.yml` (GitHub requires its `name`); fixable |
| `archived-uses` | `archived-uses` | online | partial: all archived repositories are found; see [online audits](#online-audits) |
| `artipacked` | `artipacked` | default | full on the corpus, with a fix. [batch B measurements](#batch-b-corpus-measurements) |
| `bot-conditions` | `bot-conditions` | default | partial: `github.actor`, `github.triggering_actor`, `github.actor_id` and `github.event.sender.*` compared with a bot (`==`, `contains()`, `startsWith()`, `endsWith()`) in job and step `if:`. `!=` and negated tests are not reported, as in zizmor. Measured, see [batch C](#batch-c-measurements). `-fix=unsafe` only for workflows with `pull_request`/`pull_request_target` events. Bot names without `[bot]` are known by ID and prefix only. Composite actions: the fix needs every local caller to run on a pull request event |
| `cache-poisoning` | `cache-poisoning` | default | partial: a fixed table of cache actions, `tags-ignore` is not a release trigger. [batch B measurements](#batch-b-corpus-measurements) |
| `concurrency-limits` | `concurrency-limits` | default | full: only a missing `concurrency:` and the bare group form are reported; see [measurements](#batch-a-measurements) |
| `dangerous-triggers` | `dangerous-triggers` | default | full; adds `issue_comment`, which zizmor flags from 1.31; reported at the trigger, zizmor reports at `on:` |
| `dependabot-cooldown` | `dependabot-cooldown` | default | full on the corpus: 386 of 386 zizmor findings in 193 repositories, plus 3 true positives zizmor 1.30.1 misses (it stops at the first update which satisfies the minimum). `-fix` needs the `default-days` option; zizmor has a built-in 7. The `semver-*-days` keys are checked by neither |
| `dependabot-execution` | `dependabot-execution` | default | not measured: no `allow` in the 193 repositories of the corpus, so both tools report 0. Covered by unit tests only. The fix is unsafe (zizmor offers it too) |
| `excessive-permissions` | `excessive-permissions` | default | partial: write scopes, `write-all`, `read-all`, workflow-level default permissions (option) and, with `missing-permissions`, job-level default permissions. [batch B measurements](#batch-b-corpus-measurements), [missing-permissions measurements](#measured-missing-permissions-and-missing-timeout) |
| `forbidden-uses` | `forbidden-uses` | configured | partial: patterns follow zizmor documentation but were not measured against zizmor |
| `github-app` | `github-app` | default | partial: `actions/create-github-app-token` only, no dependabot side yet. [batch B measurements](#batch-b-corpus-measurements) |
| `github-env` | `github-env` | default | partial: zizmor and jactionlint both have no finding on the corpus. jactionlint accepts values of trusted contexts (`github.sha`, `runner.*`) and `mktemp`/`date` substitutions that zizmor reports in `pull_request_target` and `workflow_run` workflows, and reports untrusted input under every trigger. `pwsh` and `cmd` are matched line by line. See [the check](checks.md#check-github-env) |
| `hardcoded-container-credentials` | `hardcoded-container-credentials`, the check of actionlint ([Hardcoded credentials](checks.md#check-hardcoded-credentials)) | correctness | not measured against zizmor |
| `impostor-commit` | `impostor-commit` | online | partial: same findings on the corpus; a repository with more than 1000 branches gives no verdict (100 without a token); see [online audits](#online-audits) |
| `insecure-commands` | `insecure-commands` | default | partial: workflow, job and step `env` (step `env` in `action.yml` too); fixable (unsafe) |
| `insecure-url-scheme` | `insecure-url-scheme` | default | none to compare: zizmor 1.30.1 applies it to the `repo:` URLs of `.pre-commit-config.yaml` only, which jactionlint does not lint. jactionlint's rule is a different one with the same ID: `http://`, `ftp://` and `git://` locations in `run:` scripts and `with:` inputs. [batch K measurements](#batch-k-measurements) |
| `known-vulnerable-actions` | `known-vulnerable-actions` | online | partial: same findings on the corpus; branch refs are not judged; see [online audits](#online-audits) |
| `misfeature` | `misfeature` | default | partial: `pip-install` of setup-python and `shell: cmd` (`misfeature`, 18 of 18 zizmor findings of the corpus). Shells that are not well known are reported with the option `pedantic` of `misfeature`, which zizmor does in the auditor persona only and which is not compared. Composite actions: checked |
| `obfuscation` | `obfuscation` | default | partial: redundant segments at `uses:`, constant expressions and `format()` of literals outside `if:`, `fromJSON(toJSON(x))` and computed indices. All zizmor findings of the corpus are reported (see [batch C](#batch-c-measurements)). Constants in `if:` are `constant-condition`; `fromJSON(toJSON(context))` is deliberately not reported; the `uses:` fix is unsafe. Composite actions: checked |
| `overprovisioned-secrets` | `overprovisioned-secrets` | default | full on the synthetic cases; 0 findings on the corpus |
| `ref-confusion` | `ref-confusion` | online | partial: no finding on the corpus by either tool; see [online audits](#online-audits) |
| `ref-version-mismatch` | `ref-version-mismatch` | online | partial: every mismatch of a tag comment found; a missing or version-less comment and a branch comment are not reported; see [online audits](#online-audits) |
| `secrets-inherit` | `secrets-inherit` | default | full |
| `secrets-outside-env` | `secrets-outside-env` | pedantic | full; zizmor reports it only for the auditor persona |
| `self-hosted-runner` | `self-hosted-runner` | pedantic (info) | partial: literal `self-hosted` labels and matrix values; labels from other expressions are not resolved |
| `self-repository` | `self-repository` | pedantic (info) | full on the corpus, with an unsafe fix. [batch B measurements](#batch-b-corpus-measurements) |
| `stale-action-refs` | `stale-action-refs` | online | partial: same findings except repositories with more than 1000 tags; see [online audits](#online-audits) |
| `superfluous-actions` | `superfluous-actions` | default | full: the same 40 findings as zizmor on the corpus, split like its personas (7 regular, and the 33 pedantic ones with the option `pedantic`, which the `pedantic` profile turns on). jactionlint adds the archived `actions/create-release` and `actions/upload-release-asset`. See [the check](checks.md#check-superfluous-actions) |
| `template-injection` | `template-injection` | correctness | partial: attacker controlled contexts, objects holding them and env variables set from them in `run:`, github-script and the code inputs of well-known actions, every expression of a script. Every other expansion (free text, and values like `github.repository`) is reported with the option `pedantic`, which is zizmor's pedantic persona. See [batch C](#batch-c-measurements) for the numbers. Not covered: knowledge about the outputs of popular actions, severity by trigger (the level is per rule ID). `-fix` moves a simple reference into `env:` for bash and sh. Batch J adds sinks that zizmor lacks: `container.options` and `services.<id>.options` ([zizmor#1128](https://github.com/zizmorcore/zizmor/issues/1128), still open), their image, entrypoint, command and volumes, `args` and `entrypoint` of `docker://` steps, more code inputs of well-known actions, and the prompt, arguments and settings of AI agent actions; see [batch J](#batch-j-measurements) |
| `typosquat-uses` | `typosquat-uses` | default | partial: one typo of the slug, not all of the transformations of zizmor |
| `undocumented-permissions` | `undocumented-permissions` | pedantic | partial: a comment above a scope counts, `include-read` for zizmor's read scopes. [batch B measurements](#batch-b-corpus-measurements) |
| `unpinned-images` | `unpinned-images` | default | partial: no per-persona split, images in `docker://` are `unpinned-uses`. [batch B measurements](#batch-b-corpus-measurements) |
| `unpinned-tools` | `unpinned-tools` | default | full for the 4 actions zizmor knows (no finding on the corpus in either tool). The option `pedantic` (on under the `pedantic` profile) adds `run:` installs without an exact version and three more actions. An input that is an expression is not reported, zizmor reports it with low confidence. See [the check](checks.md#check-unpinned-tools) |
| `unpinned-uses` | `unpinned-uses` | default | partial: `hash-pin`, `ref-pin`, `any` policies with a subset of zizmor's patterns. [batch B measurements](#batch-b-corpus-measurements) |
| `unredacted-secrets` | `unredacted-secrets` | default | full on the synthetic cases; 0 findings on the corpus |
| `unsound-condition` | `if-always-true` (existing) | correctness | full for workflows: `if-always-true` reports every `if:` with characters around `${{ }}`, a block scalar's trailing newline included. Matches all 5 zizmor findings of the corpus. No new rule. Composite action steps are checked too (not measured) |
| `unsound-contains` | `unsound-contains` | default | full on the synthetic cases; 0 findings on the corpus |
| `unsound-ternary` | `unsound-ternary` | correctness | not measured: no corpus run against zizmor yet, covered by fixtures only |
| `use-trusted-publishing` | `use-trusted-publishing` | default | partial: 2 of the 4 findings of zizmor on the corpus. The 2 others are in reusable workflows without `permissions:`, which jactionlint skips because the caller may grant `id-token: write`. No `pwsh` scripts and no `npm run publish`. See [the check](checks.md#check-use-trusted-publishing) |

## Profiles and personas

zizmor gives every finding a persona (`regular`, `pedantic` or `auditor`), read from the source of v1.30.1 (`Persona::` in
`crates/zizmor/src/audit`). jactionlint has three [profiles](config.md#profiles), each including the one before it. The `regular`
persona is the `default` profile, `pedantic` and `auditor` are the `pedantic` profile, and an audit with findings of both is one
rule with the option `pedantic` for the noisier findings.

The sections below that measure a batch were written while the profiles were still `default`, `strict` and `all`: read `strict` as the
`default` profile of today for the security rules and `all` as `pedantic`. Findings are counted with the rules and options of that
time; the rules that were merged into one ID since (`template-injection-expansion`, `template-injection-trusted`,
`misfeature-custom-shell`) are the option `pedantic` of `template-injection` and `misfeature`.

| zizmor audit | Persona | jactionlint |
| --- | --- | --- |
| `anonymous-definition`, `undocumented-permissions` | pedantic | `pedantic` profile |
| `self-hosted-runner`, `secrets-outside-env` | auditor | `pedantic` profile |
| `concurrency-limits` | pedantic | `default` profile, a decision of the maintainer; it has a safe fix for workflows that only pull requests start |
| `template-injection`, `superfluous-actions`, `unpinned-tools`, `unlocked-install` | regular and pedantic | `default` profile, the pedantic findings behind the option `pedantic` |
| `misfeature` | regular, the custom shells auditor | `default` profile, custom shells behind the option `pedantic` |
| `artipacked`, `excessive-permissions`, `obfuscation`, `unpinned-images`, `unpinned-uses`, `insecure-commands` | regular, with pedantic or auditor findings for special cases | `default` profile; the special cases are not separate findings yet |
| `ref-version-mismatch`, `stale-action-refs` | pedantic | online (`-online`) |
| every other audit | regular | `default` profile, or online |

## Lessons applied

The issue tracker of zizmor is a list of false positives and false negatives that a rule of the same shape can have. Each item below
was reproduced with a fixture in `pitfalls_test.go` against jactionlint before anything was changed, so it is measured by a test, not on
the corpus. "Fixed" means that the fixture failed on the code before; "Not reproduced" means that it passed, and it is kept as a fixture.

| zizmor | What went wrong there | jactionlint |
| --- | --- | --- |
| [#2219](https://github.com/zizmorcore/zizmor/issues/2219) | `github-app` flags `owner` without `repositories` although the token only has organization permissions | Fixed: no finding when every `permission-*` input is an organization permission |
| [#1914](https://github.com/zizmorcore/zizmor/issues/1914) | `bot-conditions` flags `github.actor == 'dependabot[bot]'` next to a check of the pull request author | Fixed: a conjunction with a check of the author of the pull request is not reported |
| [#2059](https://github.com/zizmorcore/zizmor/issues/2059) | findings in steps with `if: false` | Fixed for every rule outside the correctness group, in one place (the findings in a job or a step whose `if:` is the literal false are dropped) |
| [#1098](https://github.com/zizmorcore/zizmor/issues/1098), [#1043](https://github.com/zizmorcore/zizmor/issues/1043) | `artipacked` flags a checkout that a later `git push` needs, and `actions/checkout@v1` | Fixed: neither is reported (a push next to an upload of the workspace still is). Two `# zizmor: ignore[artipacked]` comments of the fix corpus became stale |
| [#2320](https://github.com/zizmorcore/zizmor/issues/2320) | `cache-poisoning` on `astral-sh/setup-uv` v10 with `enable-cache: auto` | Fixed: version aware, the version is read from the comment of a pinned commit too. The guard `if: github.event_name != 'release'` on a step was already honored |
| [#1848](https://github.com/zizmorcore/zizmor/issues/1848) | `use-trusted-publishing` is silent when `id-token: write` is granted (for provenance or a cloud login), and misses PowerShell | Fixed: a long-lived credential next to `id-token: write` is reported, and `pwsh` scripts are searched line by line |
| [#1479](https://github.com/zizmorcore/zizmor/issues/1479) | `dependabot-cooldown` on OpenTofu | Not a flaw: the options reference of GitHub lists `default-days` for every ecosystem, OpenTofu included, and only `semver-*-days` is limited. A test keeps every ecosystem reported |
| [#1481](https://github.com/zizmorcore/zizmor/issues/1481) | `copilot-setup-steps.yml` is not an ordinary workflow | Fixed for `concurrency-limits` and `anonymous-definition`. `missing-timeout` still applies, because the file accepts `timeout-minutes` |
| [#1619](https://github.com/zizmorcore/zizmor/issues/1619) | `concurrency-limits` skips a caller whose jobs all call reusable workflows | Fixed: the caller is reported, because the called workflow cannot hold the limit without deadlocking |
| [#1865](https://github.com/zizmorcore/zizmor/issues/1865) | `superfluous-actions` on self-hosted runners | Fixed: not reported there |
| [#2433](https://github.com/zizmorcore/zizmor/issues/2433) and the other `ref-version-mismatch` issues | a second comment after the version, prereleases, sibling tags | Not reproduced: the version is the first word of the comment. Kept as fixtures |
| [#2321](https://github.com/zizmorcore/zizmor/issues/2321) | `ref-confusion` ignores hash-pinned refs, but a branch can be named like a hash | Fixed for abbreviated SHAs, which are names to GitHub. Full SHAs are read as commits and are not looked up |
| [#2130](https://github.com/zizmorcore/zizmor/issues/2130) | `impostor-commit` and annotated tag objects | Not reproduced: the client dereferences annotated tags, and a SHA that GitHub cannot compare is no verdict, not an impostor |
| [#1673](https://github.com/zizmorcore/zizmor/issues/1673) | `secrets: inherit` under `on.workflow_call` | Open: the documentation and the workflow schema of GitHub show only a mapping there, so the syntax error stays. Whether GitHub runs such a file was not tested |
| [#2210](https://github.com/zizmorcore/zizmor/issues/2210) and others | one 403, 404 or 5xx stops the online run | Not reproduced: a failed lookup is skipped with one warning (`-verbose` lists them). A test covers 404, 403, 500 and 502 |
| [GHSA-f42p-wjw5-97qh](https://github.com/zizmorcore/zizmor/security/advisories/GHSA-f42p-wjw5-97qh) | credentials in the debug log | Not reproduced: a test runs `-online -debug -verbose` with a token and finds it nowhere |

## Intentional differences: where jactionlint goes beyond zizmor

<a id="beyond-zizmor"></a>

These are choices, not gaps. Each says what is different and where the evidence is.

### Rules of our own

Rules that zizmor 1.30.1 has no audit for. They are not in the table above, `scripts/zizmor-diff` has no mapping for them, and a
finding of one of them is always "only jactionlint" in its report. Their false-positive reviews are in the measurement sections named
in the last column; the idea behind some of them is an open issue of zizmor.

| Rule | Profile | What it reports | Evidence |
| --- | --- | --- | --- |
| `missing-timeout`, `timeout-too-long` | default, configured | a job without `timeout-minutes`; a value above `max`. zizmor [#2166](https://github.com/zizmorcore/zizmor/pull/2166) is open | [missing-timeout](#measured-missing-permissions-and-missing-timeout) |
| `unlocked-install` | default | `cargo install` without `--locked` (fixable, unsafe), and `npm install`, `yarn` and `pnpm install --no-frozen-lockfile` when the repository has the lock file the install ignores. With the option `pedantic`: the same without a lock file, `bun install` without `--frozen-lockfile` and `pip install -r` without hashes or constraints. See [the check](checks.md#check-unlocked-install) | not measured: unit tests and fixtures |
| `dependabot-missing-actions-update` | pedantic | `dependabot.yml` has no `github-actions` update although `.github/workflows` uses actions; skipped when the repository has a Renovate configuration | 20 findings in 193 repositories, all true (a Go module with only a `gomod` update). [The check](checks.md#check-dependabot-missing-actions-update) |
| `pipeline-without-pipefail` | default | a failure in a pipeline of a `run:` script is hidden because the default shell (`bash -e {0}`) and `shell: sh` do not enable pipefail. zizmor [#288](https://github.com/zizmorcore/zizmor/issues/288) is the open idea | unit tests and fixtures; not measured on the corpus |
| `agentic-actions` | default | AI agent actions ([zizmor#1605](https://github.com/zizmorcore/zizmor/issues/1605) is a proposal): an agent outsiders can steer without a check of the user, an open gate (`allowed_non_write_users: '*'`), settings that turn safeguards off, an agent on the code of a pull request. [The check](checks.md#check-agentic-actions) | [batch J](#batch-j-measurements) |
| `unverified-download` | default | a download piped into a shell or an interpreter, a downloaded file made executable and run without a checksum or signature check, TLS verification turned off. The idea is zizmor [#711](https://github.com/zizmorcore/zizmor/issues/711) | [batch K](#batch-k-measurements) |
| `insecure-ssh-keyscan` | default | `ssh-keyscan` output written to a `known_hosts` file. The idea is zizmor [#2012](https://github.com/zizmorcore/zizmor/issues/2012) | [batch K](#batch-k-measurements) |
| `checkout-static-credentials` | default | `actions/checkout` given an `ssh-key` or a literal `token`; with the option `secret-tokens` (on under the `pedantic` profile) also a token from a secret other than `GITHUB_TOKEN`. The idea is zizmor [#1118](https://github.com/zizmorcore/zizmor/issues/1118) | [batch K](#batch-k-measurements) |
| `insecure-url-scheme` | default | `http://`, `ftp://` and `git://` locations in `run:` downloads and `with:` inputs. zizmor's audit of this name looks at `.pre-commit-config.yaml` only | [batch K](#batch-k-measurements) |
| `invisible-characters` | default | invisible and bidirectional control characters ([zizmor#914](https://github.com/zizmorcore/zizmor/issues/914)), in workflows and `dependabot.yml`, also in a file that does not parse. Safe fix | [batch L](#batch-l-measurements): no true positive in the corpus, unit tests only |
| `unsound-prefix-match` | default | `startsWith()`, `endsWith()` and `contains()` on the name of an account or repository ([zizmor#1533](https://github.com/zizmorcore/zizmor/issues/1533)); `unsound-contains` is the sibling for a literal haystack | [batch L](#batch-l-measurements) |
| `concurrency-cancels-prs` | default | a `cancel-in-progress` group that all pull requests share | [batch H](#batch-h-measurements) |
| `concurrency-cancels-release` | default | `cancel-in-progress` on tag pushes, releases and jobs that publish or deploy; recommends `cancel-in-progress: false`, a group per ref or tag and `queue: max` (fixable, unsafe) | [batch H](#batch-h-measurements) |
| `gate-job-skipped-on-failure` | default | a job that reads `needs.*.result` but is skipped when a needed job fails | [batch H](#batch-h-measurements): no finding on the corpus |
| `untrusted-checkout` | default | `pull_request_target` and `workflow_run` workflows that check out the pull request and run it. `dangerous-triggers` reports the trigger alone | [batch H](#batch-h-measurements) |
| `untrusted-artifact` | default | `workflow_run` workflows that run, extract or export the artifact of the triggering run unchecked (zizmor [#195](https://github.com/zizmorcore/zizmor/issues/195) is the open idea) | [batch H](#batch-h-measurements) |
| `unused-job-output`, `duplicate-triggers` | default | a job output nobody reads; `push` without a branch filter next to `pull_request` | [batch H](#batch-h-measurements) |
| `unused-workflow-input`, `unused-needs`, `continue-on-error`, `mutable-runner-label` | pedantic | an input never read; a redundant `needs` entry; `continue-on-error: true`; `ubuntu-latest` and other moving labels (fixable with the `pin` option) | [batch H](#batch-h-measurements) |
| `require-shell`, `require-expression-wrapping`, `max-run-lines` | pedantic | conventions: explicit `shell:`, `${{ }}` around `if:` expressions, script length | unit tests; opt-in conventions with no zizmor counterpart |
| `unused-ignore`, `expired-ignore`, `unused-baseline-entry` | pedantic, correctness | housekeeping of ignores and of the baseline | below |

Sinks that zizmor lacks, as part of `template-injection`: `container.options` and `services.<id>.options`
([zizmor#1128](https://github.com/zizmorcore/zizmor/issues/1128), still open), their image, entrypoint, command and volumes, `args`
and `entrypoint` of `docker://` steps, more code inputs of well-known actions, and the prompt, arguments and settings of AI agent
actions ([batch J](#batch-j-measurements)).

### Features

- **Composite actions with a caller-aware context.** zizmor audits `action.yml` files but has no knowledge of who calls them
  ([#1424](https://github.com/zizmorcore/zizmor/issues/1424), [#678](https://github.com/zizmorcore/zizmor/issues/678)).
  jactionlint takes the triggers of an action from the local workflows that call it, and judges a published action that no local
  workflow calls by its own steps only. [Measured](#composite-actions) on the action files of the corpus.
- **A baseline for gradual adoption** ([zizmor#2282](https://github.com/zizmorcore/zizmor/issues/2282), declined there).
  `jactionlint -baseline-write` records the current findings and `-baseline` hides them, so a repository can adopt the stricter
  default and fail only on new findings. Entries are keyed by file, rule ID and a fingerprint, not by line numbers;
  `-baseline-check` lists the entries that match nothing any more, `-format summary` counts findings per rule and file, and SARIF marks
  baselined results as suppressed. Tested with unit tests and by hand; no comparison is possible. See [the usage document](usage.md#baseline).
- **Durable ignores** (`ignores:` in the config file, rules `expired-ignore` and `unused-ignore`). zizmor ignores a finding with a
  `# zizmor: ignore[audit]` comment or a `file:line:col` entry in `zizmor.yml`, and both break when a tool rewrites the line or when
  lines shift ([zizmor#1086](https://github.com/zizmorcore/zizmor/issues/1086)). jactionlint matches by rule plus file glob, job ID,
  step ID or name and the `uses:` value (so a Renovate bump of the SHA keeps the entry working), with an optional reason and expiry
  date. Not measured against zizmor, which has no equivalent. See [durable ignores](config.md#durable-ignores).
- **Ignore comments at the end of a line** (`# jactionlint ignore=...`) cover the whole step when they are on its first line. A
  `# zizmor: ignore[...]` comment is honored as well ([measured below](#ignore-comments)).
- **Fixes that converge.** `-fix` lints, fixes and lints again until nothing is left, writes each file once and atomically, and checks
  every pass (valid YAML, only the edits changed); a fix that breaks a file is refused. `-diff` shows the change, `-fix -rules` limits
  it, and SARIF carries the safe fixes so [hk](usage.md#hk) can apply them. A fixer never invents a value (`missing-timeout` and
  `dependabot-cooldown` need `default-minutes` and `default-days`), where zizmor's `dependabot-cooldown` fix has a built-in 7.
  Measured with `scripts/fix-corpus` on 955 files: [missing-timeout and missing-permissions](#measured-missing-permissions-and-missing-timeout).
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

From the 2026-10-08 survey of the zizmor tracker, these ideas are **not built**: a check of `allow-unsafe-pr-checkout` of
`actions/checkout` and of code that a pull request controls (zizmor [#2134](https://github.com/zizmorcore/zizmor/issues/2134)), TOCTOU
refs ([#935](https://github.com/zizmorcore/zizmor/issues/935)), shell syntax errors without shellcheck
([#2322](https://github.com/zizmorcore/zizmor/issues/2322)), a rule for a SHA without a version comment
([#2316](https://github.com/zizmorcore/zizmor/issues/2316)), immutable releases under `-online`
([#766](https://github.com/zizmorcore/zizmor/issues/766)), credentials in the `registries` of `dependabot.yml`
([#1222](https://github.com/zizmorcore/zizmor/issues/1222)), a repojacking check by owner lookup
([#479](https://github.com/zizmorcore/zizmor/issues/479)), and zizmor's precomputed snapshot of tags and advisories, which would cut the
first-run cost of `-online`. The registry of rules, [rules.md](rules.md), is the list of what exists.

## Batch B corpus measurements

Batch B (permissions, pinning, checkout) was measured against zizmor 1.30.1 (`--offline --persona pedantic`) over 38
repositories with 378 workflow files: the jdx repositories checked out under `~/src` plus entirecli, entiredb, peregrine,
renovatebot/renovate and oxc-project/oxc, so that not all of them are written by the same people. jactionlint ran with the
`all` profile of the time (today `pedantic`), `excessive-permissions: {require-workflow-permissions: true}` and `undocumented-permissions: {include-read: true}`.
A finding matches when it is in the same file and on the line of the primary location of the zizmor finding. `artipacked` is
matched within three lines below it, because zizmor points at the first line of a step and jactionlint at its `uses:`. At
the time these rules were in the `strict` or `all` profile. Today all of them are in `default`, except `undocumented-permissions` and
`self-repository` (`pedantic`); the counts do not depend on the profile.

| zizmor audit | zizmor findings | jactionlint findings | shared | zizmor only | jactionlint only |
| --- | --: | --: | --: | --: | --: |
| `excessive-permissions`, write scopes, `write-all`, `read-all` | 76 | 76 | 76 | 0 | 0 |
| `excessive-permissions`, no `permissions:` at the workflow level | 81 | 81 | 81 | 0 | 0 |
| `excessive-permissions`, no `permissions:` for a job (`missing-permissions`) | 88 | 88 | 88 | 0 | 0 |
| `undocumented-permissions` (with `include-read`) | 433 | 410 | 410 | 23 | 0 |
| `unpinned-uses` | 123 | 123 | 123 | 0 | 0 |
| `unpinned-images` | 8 | 8 | 8 | 0 | 0 |
| `self-repository` | 93 | 93 | 93 | 0 | 0 |
| `github-app` | 14 | 14 | 14 | 0 | 0 |
| `artipacked` | 125 | 130 | 125 | 0 | 5 |
| `cache-poisoning` | 18 | 20 | 16 | 2 | 4 |

What the differences are:

- `undocumented-permissions`, 23 zizmor only: every one of them has a comment on the line above the scope. zizmor accepts only
  a comment at the end of the line, jactionlint accepts both. This is deliberate.
- `unpinned-images`: zizmor points at `container: ${{ matrix.image }}`, jactionlint at the matrix value that is not pinned, so
  one finding is counted as zizmor only and jactionlint only on the same job. They are the same finding.
- `artipacked`, 5 jactionlint only: all five checkouts carry `# zizmor: ignore[artipacked]`, which jactionlint does not read.
  They are true findings that a human decided to keep (the job pushes with the credential). The same comments are the reason
  for one `cache-poisoning` finding.
- `cache-poisoning`, 2 zizmor only: zizmor treats a workflow with `push: {tags-ignore: ["*"]}` as a release workflow, which is
  a workflow that says the opposite, so jactionlint does not (judged as a false positive of zizmor). 3 jactionlint only: a job
  that runs `gh release upload` or pushes an image restores a cache (`jdx/mise-action` with `cache_save: false`, which only
  stops saving, and `docker/build-push-action` with `cache-from: type=gha`). zizmor does not look at jobs, only at triggers, or
  does not know the Docker action. Judged true findings.

Known gaps of batch B, which the table does not show because the corpus does not exercise them:

- zizmor reports `excessive-permissions` with a severity per finding and per persona. jactionlint has one level per rule, so
  all findings of the rule share it (`error`).
- `unpinned-uses` supports the patterns `*`, `owner/*`, `owner/repo` and `owner/repo/path`. Other forms of zizmor's repository
  patterns are rejected when the configuration is read. Docker images follow the `*` pattern only.
- `unpinned-images` does not distinguish regular and pedantic findings (an image without a tag against a tag without a
  digest). The `require-digest` option switches between the two behaviors.
- `github-app` knows `actions/create-github-app-token` only.
- `cache-poisoning` knows the cache actions in a fixed table. Cache modes of the dangerous-write half (`cache-mode: write` on a
  privileged trigger) were added after zizmor 1.30.1, so that half has no zizmor measurement.

## Online audits

The six online audits (batch G) run only with `-online`. They were measured on 2026-10-08 against zizmor 1.30.1 (`--persona
pedantic`, with a token, so not `--offline`) with `go run ./scripts/zizmor-diff --online`, and by running both tools on every
workflow file of the corpus: 38 repositories under `~/src` (the jdx repositories, `oxc-project/oxc` and `renovatebot/renovate`
among them) plus this repository's worktree. A finding is shared when both tools report the same audit in the same file within two
lines. The numbers are for the corpus, not for GitHub as a whole.

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

The same comparison over the 181 other checkouts of those repositories (git worktrees and submodules under `.claude/worktrees`,
which hold older workflows and so more vulnerable actions) found 80 `known-vulnerable-actions` findings of zizmor, all also found
by jactionlint once the checkouts were linted from their own directory, and none that only jactionlint reports; and 67
`stale-action-refs` findings, 64 shared, the 3 others being the `taiki-e/install-action` case below.

What the differences are:

- **jactionlint only, `impostor-commit` (1).** `taiki-e/install-action@c030abd...` in one workflow. zizmor reports it too; the
  workflow has a `# zizmor: ignore[impostor-commit]` comment, which only zizmor reads. Counted as a true finding.
- **zizmor only, `stale-action-refs` (1)** and the same three in the older checkouts: `taiki-e/install-action` has more than 1000
  tags, jactionlint reads 1000 per repository, and says nothing about a commit it did not find in a truncated list.
- **`ref-version-mismatch`, missing or version-less comment (42).** zizmor reports a hash-pinned action without a version comment
  (a pedantic finding). jactionlint does not: a missing comment is not a mismatch. This is a gap, and a candidate for a separate
  offline rule.
- **`ref-version-mismatch`, comment names a branch (2).** zizmor also compares a comment such as `# stable` with the branch of
  that name. jactionlint only reads comments that start with a version.
- **No finding for either tool** for `ref-confusion` and `archived-uses` on this corpus, and `known-vulnerable-actions` on the
  current workflows: the rules are exercised by the fixtures, and by the older checkouts for `known-vulnerable-actions`, not by
  measurement of `ref-confusion` and `archived-uses` against zizmor.

Gaps of the online rules that the corpus does not show, so they are not measured:

- `impostor-commit` treats a commit as legitimate when a tag points at it or a branch contains it. A commit which is only in the
  history of a tag (a deleted release branch) is reported.
- Without a token jactionlint cannot use GraphQL, compares at most 100 branches one request at a time, and says nothing for a larger
  repository where no branch has the commit.
- Reusable workflows are checked like actions; composite action files (`action.yml`) are checked too, with the steps of a composite action treated like workflow steps.

Cost: for the 173 `uses:` lines of this repository the first run makes 91 requests, a run within the hour none, and a later run
only conditional requests (a 304 answer is free of rate limit for authenticated clients).

### Online client: where it differs from zizmor

Not compatible by design; the client answers zizmor issues [#1350](https://github.com/zizmorcore/zizmor/issues/1350) and
[#2210](https://github.com/zizmorcore/zizmor/issues/2210) (one failed lookup aborts the audit) and the credentials in debug logs of
[GHSA-f42p-wjw5-97qh](https://github.com/zizmorcore/zizmor/security/advisories/GHSA-f42p-wjw5-97qh). Measured by unit tests against an
`httptest` server for every failure mode, not against zizmor.

- A lookup which fails (404, 403, 5xx, timeout, DNS) is skipped for that `uses:` only. One warning per kind of failure, exit status
  unchanged unless `-online=strict`.
- Rate limits are read from `X-RateLimit-*` and `Retry-After`; a reset within 30 seconds is waited for, a later one skips with the
  reset time in the message. 5xx and secondary limits are retried with exponential backoff and jitter, at most twice.
- `allow` and `deny` lists of `owner/repo` patterns, a GitHub Enterprise Server host from `GITHUB_API_URL`, `GITHUB_SERVER_URL`, `GH_HOST`
  or `-online-api-url`, and a token from a named variable, a file, the usual variables or `gh auth token`.
- The token is sent to the API host only, is redacted from all output, and is never sent to a host that a repository's own config file chose.
- `-online=cache` answers from the disk cache only, without the network.

Not done: zizmor's precomputed snapshot of tags and advisories, which would cut the first-run cost further.

## Ignore comments

jactionlint honors zizmor's `# zizmor: ignore[...]` comments for the audits that map onto one of its rules, so a repository that
already triaged its zizmor findings keeps them triaged. See [the usage document](usage.md#zizmor-ignore-comments) and
[the alias table](v2-migration.md#zizmor-ignore-comments).

Measured on 2026-10-09 over the workflows under `~/src` (1000 distinct files by content; this includes git worktrees, which hold
older versions of the workflows): 175 files carry a zizmor comment, 353 names in total: `cache-poisoning` 229, `dangerous-triggers` 58,
`artipacked` 23, `use-trusted-publishing` 22, `adhoc-packages` 10, `impostor-commit` 9 and `template-injection` 2. Every one of these
audits is a jactionlint rule now. Linting each of the 175 files with the `pedantic` profile (shellcheck and pyflakes off), with the
comments and with the comments stripped, the comments hide 97 findings (`dangerous-triggers` 58, `cache-poisoning` 14,
`use-trusted-publishing` 13, `artipacked` 8, `template-injection` 4), and `unused-ignore` reports 249 of the 353 comments as stale,
215 of them `cache-poisoning`. Stale means that jactionlint reports nothing there; it is not necessarily a gap: for the one file checked
by hand, zizmor 1.30.1 reports no `cache-poisoning` finding either once the comments are removed (the comments are older than the
current rules of both tools). The other 214 were not compared with zizmor, and `impostor-commit` comments are only exercised
with `-online`, which this run did not use.

## Measured: missing-permissions and missing-timeout

Batch F measured two existing rules on the local corpus: the checkouts of 35 repositories under `~/src` (the "main" numbers) and
all 955 distinct workflow files of the 139 worktrees there (the "all" numbers). The zizmor numbers are from
`zizmor --offline --persona pedantic` 1.30.1 through `scripts/zizmor-diff`.

| Rule | Profile | Findings (main / all) | Overlap with zizmor | Only jactionlint | Only zizmor |
| --- | --- | --: | --- | --- | --- |
| `missing-timeout` | default | 270 / 1010, in 27 of 35 repositories | none: zizmor 1.30.1 has no audit for it | all of them are jobs without `timeout-minutes` and without `uses:`; a separate count of such jobs in the YAML gives the same 1010, so there are no false positives by that definition | not applicable |
| `missing-permissions` | default | 74 / 372, in 14 of 35 repositories | 74 of 74 are `excessive-permissions` findings of zizmor at the same job line | none | 45 are zizmor's workflow-level (line 1) report of the same missing block; 54 are `contents: write`, `id-token: write` and other write scopes at the workflow level, which `missing-permissions` does not look at |

`missing-timeout` and `missing-permissions` are in the `default` profile by decision of the maintainer. `missing-timeout` has no false
positives by the definition above, but it is noisy for a repository that never set the key (270 findings in 27 of the 35
repositories); `jactionlint -fix` with `default-minutes` clears all of them. (`missing-permissions` was in `strict` when it was
measured.)

The fixers were measured on the 955 files: `jactionlint -fix` (with `default-minutes: 30` configured for the measurement) adds exactly 1010 `timeout-minutes: 30` lines and 28 `permissions:` blocks
(the safe ones) and changes no other line (`diff -r` shows added lines only); `-fix=unsafe` adds 137 blocks; after either run
the rules report nothing that has a fix, and a second run changes nothing.
## Batch C measurements

<a id="batch-c-measurements"></a>

The rules of batch C (`template-injection` and its tiers, `bot-conditions`, `obfuscation`, `misfeature`, `unsound-condition`) were
compared with zizmor 1.30.1 (`--offline --persona pedantic`) with the differential harness of `scripts/zizmor-diff`. Two corpora:

- **jdx**: 35 workflow repositories of the author (Rust, Go and TypeScript projects that already pass zizmor's regular persona).
- **OSS**: 1,424 third-party repositories found on disk (Go modules, crates, npm packages and a set of large projects such as
  cilium, airflow, cpython, renovate and deno).

A finding is matched when both tools report the same file and line. Counts are lines of `.github/workflows` (a line with several
findings counts once on the zizmor side, so jactionlint's own count of findings can be higher).

| Rule | Corpus | zizmor lines | jactionlint findings | Same line | Only jactionlint | Only zizmor |
| --- | --- | --: | --: | --: | --: | --: |
| `template-injection` | jdx | 213 | 0 | 0 | 0 | 213 |
| `template-injection` | OSS | 7,056 | 7 | 6 | 0 | 7,050 |
| `template-injection` + `-expansion` | jdx | 213 | 62 | 57 | 1 | 156 |
| `template-injection` + `-expansion` | OSS | 7,056 | 2,056 | 1,874 | 85 | 5,182 |
| `template-injection` + `-expansion` + `-trusted` | jdx | 213 | 247 | 213 | 11 | 0 |
| `template-injection` + `-expansion` + `-trusted` | OSS | 7,056 | 8,441 | 7,056 | 639 | 0 |
| `bot-conditions` | jdx | 0 | 2 | 0 | 2 | 0 |
| `bot-conditions` | OSS | 13 | 13 | 13 | 0 | 0 |
| `obfuscation` | jdx | 8 | 10 | 8 | 2 | 0 |
| `obfuscation` | OSS | 14 | 16 | 14 | 2 | 0 |
| `misfeature` | jdx | 0 | 0 | 0 | 0 | 0 |
| `misfeature` | OSS | 18 | 18 | 18 | 0 | 0 |
| `unsound-condition` (`if-always-true`) | jdx | 0 | 0 | 0 | 0 | 0 |
| `unsound-condition` (`if-always-true`) | OSS | 5 | 5 | 5 | 0 | 0 |

How to read the template injection rows (`-expansion` and `-trusted` were rules then and are the option `pedantic` of `template-injection` now):

- **zizmor-only in the first rows** is not a miss: the pedantic persona of zizmor reports every expansion in a script, which is what
  the `-expansion` and `-trusted` tiers are for. The last row of each corpus is the comparison with the whole of zizmor.
- **Only jactionlint** (`-trusted` and `-expansion` rows) is not a false positive of a security finding. The lines are of
  four kinds: `matrix.*` whose values are literals and `needs.*.result` (`-trusted`; zizmor treats them as safe and does not report
  them in the pedantic persona), values of `matrix.*` that come from `fromJSON`, `env.*` set from `$GITHUB_ENV` and `steps.*.outputs.*`
  of actions that zizmor knows to return safe values (`-expansion`; jactionlint does not have that knowledge), and
  `github.ref_name`/`github.event.inputs.*` (`-expansion`).
- **The default rule** (`template-injection`): all 7 findings in the OSS corpus are true positives. They are on 6 lines, and zizmor reports all of
  the 6 lines. There is no finding in the jdx corpus. No false positive was seen. Before this batch the rule stopped at the first untrusted expression of a script; the rule now reports every
  expression and the new findings that came from this (objects like `toJSON(github)`, env variables set from untrusted input,
  and the code inputs of actions) are true positives in the sample.
- zizmor's *regular* persona is not what the numbers compare: the corpus was too clean to say how the default rule compares with
  zizmor's high-confidence findings.

Rows of other rules:

- **`bot-conditions`**, jdx: both findings are `github.event.sender.id == 29139614` (the ID of Renovate), which zizmor does not know.
  It is the same spoofable property as `github.actor`, so they are true positives.
- **`obfuscation`**: the two extra findings (in both corpora) are `if: vars[matrix.var_name] != 'off'`. zizmor reports the same
  construct only inside `${{ }}`. `fromJSON(toJSON(matrix.container))` (Homebrew) is not reported by either tool. The constant
  expressions in `if:` that zizmor reports (`${{ false }}`) are reported by `constant-condition` and `if-always-true`.
- **`misfeature`** and **`unsound-condition`** match exactly.

Known differences that are not measured: the position of a finding in a double-quoted multi-line YAML string is the first line.
Composite actions are measured in [composite actions](#composite-actions-measurements).
<a id="batch-a-measurements"></a>
## Batch A measurements

Measured with `scripts/zizmor-diff`-style runs: zizmor 1.30.1 (`--offline --persona auditor`, so every audit is on) and
jactionlint with `profile: all` over two corpora, after removing the `# zizmor: ignore[...]` comments of the repositories
(otherwise zizmor hides the findings the maintainers already accepted):

- **jdx corpus**: the workflows of 35 repositories under `~/src` (the `*-jactionlint` worktrees and the other jdx repositories).
- **OSS corpus**: the workflows of 39 well-known projects (actions/runner, astral-sh/uv, cli/cli, grafana/grafana,
  home-assistant/core, apache/airflow, nodejs/node, python/cpython, and others), about 860 files.

A finding matches when both tools report the same audit in the same file on the same line. Primary locations are compared.
Counts are `zizmor / jactionlint / matched`:

| Audit | jdx corpus | OSS corpus | Differences |
| --- | --- | --- | --- |
| `anonymous-definition` | 457 / 457 / 457 | 562 / 562 / 562 | none. jobs that call a reusable workflow are skipped like zizmor does |
| `concurrency-limits` | 133 / 133 / 133 | 373 / 373 / 373 | none. zizmor flags a missing `concurrency:` and the bare `concurrency: group` form, not `cancel-in-progress: false` |
| `secrets-inherit` | 17 / 17 / 17 | 142 / 142 / 142 | none. reported at the job's `uses:` like zizmor |
| `dangerous-triggers` | 36 / 38 / 36 by file | 81 / 96 / 78 by line, 81 by file | the extra findings are all `issue_comment`, which zizmor 1.30.1 does not flag. zizmor reports at `on:` and jactionlint at the trigger, so lines differ for 3 `workflow_run` hits more than 6 lines apart. `pull_request_target` with only `actions/labeler` is exempt in both |
| `secrets-outside-env` | 353 / 353 / 353 | 649 / 648 / 647 | the two zizmor-only lines are inside a folded `>-` scalar, where jactionlint reports the line of the scalar start. reusable (`workflow_call`) workflows are skipped like zizmor does |
| `self-hosted-runner` | 19 / 19 / 19 | 12 / 12 / 12 | none. only the `self-hosted` label is reported: labels of hosted-runner providers such as namespace or blacksmith are not |
| `insecure-commands` | 0 / 0 / 0 | 0 / 0 / 0 | no finding in 74 repositories. on the synthetic fixture zizmor flags `true` at workflow and step level, and so do we; a job-level `"1"` is flagged by neither (the runner parses the variable as a boolean) |
| `overprovisioned-secrets` | 0 / 0 / 0 | 0 / 0 / 0 | synthetic fixture: 2 / 2 / 2 |
| `unredacted-secrets` | 0 / 0 / 0 | 0 / 0 / 0 | synthetic fixture: 3 / 3 / 3 (any `fromJSON(secrets.X)` is reported, also without a field access) |
| `unsound-contains` | 0 / 0 / 0 | 0 / 0 / 0 | synthetic fixture: 2 / 2 / 2 |
| `typosquat-uses` | 0 / 0 / 0 | 0 / 0 / 0 | synthetic fixture: 2 / 2 / 2. no `uses:` of the 74 repositories is reported. zizmor (typomania) also looks for other transformations that jactionlint does not |
| `forbidden-uses` | not measured | not measured | needs configuration, which zizmor and jactionlint read from different files. the pattern semantics are covered by unit tests |

The jactionlint-only findings on the corpora are all reviewed: none is a false positive. Rules with `0` findings
have no measured false-positive rate, only the synthetic cases above. When this was measured, `unredacted-secrets`,
`overprovisioned-secrets` and `typosquat-uses` stayed in `strict` for that reason. Today all of the regular-persona audits of this batch
are in the `default` profile, by decision of the maintainer and not because the corpus showed their false-positive rate: those three
rules are **unmeasured** in `default`.

Behaviors of zizmor that were found only by this comparison, and are now reproduced: job names count for `anonymous-definition`;
`concurrency-limits` skips workflows that only call reusable workflows; `secrets-outside-env` skips `workflow_call` workflows;
`self-hosted-runner` reports the label only; `dangerous-triggers` exempts `actions/labeler` for `pull_request_target`.

## Batch J measurements

<a id="batch-j-measurements"></a>

Batch J extends `template-injection` to the places that read a string as a command line or as the instructions of an agent, and adds
`agentic-actions`. Measured with zizmor 1.30.1 (`--offline --persona pedantic`) and jactionlint on two corpora:

- **jdx**: the workflows of 266 repository checkouts under `~/src` (worktrees included). Eight distinct workflow files use an AI agent action
  (hk's `claude.yml` and copies of it) and 977 files have `container:` or `services:`.
- **AI agent workflows**: 237 workflow files of 151 public repositories that use an agent action, found with GitHub code search for the
  actions (`anthropics/claude-code-action`, `run-gemini-cli`, `gemini-cli-action`, `openai/codex-action`, `actions/ai-inference`, opencode
  and others). It is a sample of what is on GitHub, not a random one.

| Rule | jdx corpus | AI agent corpus | zizmor | Judgement |
| --- | --- | --- | --- | --- |
| `template-injection`: prompt, arguments and settings of agents | 0 | 78 findings in 33 files | 0 in the same files | all 78 are attacker controlled contexts (`github.event.issue.title`, `.body`, `pull_request.title`, `comment.body`) in a prompt; no false positive found |
| `template-injection`: container and service options, image, entrypoint, command, volumes; args of a `docker://` step | 0 in 977 files | 0 (10 files found by code search for `options: ${{`) | 0 | the pattern is rare. Covered by golden tests, since no real finding was available |
| `agentic-actions` | 0 | 13 findings in 9 workflows | no such audit | all 13 are true: 6 agents without a check of the user (no `if:`, `environment:` or permission step) with a token that can write contents, 4 agents that run on a pull request checkout, 3 allowed tools that give any code (`Bash(npx:*)`, `Bash(bunx:*)`, `Bash(xargs:*)`) |

Findings of `agentic-actions` that the first version produced and that were false positives, and what was changed (the counts are of the
same corpus): 60 findings fell to 13. A triage job with a read-only token and a Codex `sandbox: read-only` or a Gemini `tools.core`
list is the architecture the vendors advise, and is no longer reported; `workflow_run` is no longer an outsider trigger for the checks of
the user and of the settings (it is as safe as the workflow it follows); settings files with trailing commas (the example of the Gemini
CLI action has one) or an expression in a detail of the JSON are read; Codex on Windows needs `safety-strategy: unsafe`.

Differences from zizmor, on purpose: zizmor reports no `${{ }}` in the prompt of an agent, and nothing for container options; the
rule reports at `error` level whatever the trigger (decision: every rule of the default profile does), where the proposal for zizmor
would grade by trigger and persona; the actions are a table (`agent_actions.go`) that must be kept up to date.

## Rules of batch H

These rules have no equivalent audit in zizmor 1.30.1. They are rules of jactionlint's own, listed above under [rules of our own](#rules-of-our-own), so they are not part of the parity
table, and the differential harness (`scripts/zizmor-diff`) has no mapping for them. A finding of one of them is always
"only jactionlint" in its report.

| Rule | Profile | Level | What it finds | Closest zizmor audit |
| --- | --- | --- | --- | --- |
| [`concurrency-cancels-prs`](checks.md#check-concurrency-cancels-prs) | default | error | a `cancel-in-progress` group that all pull requests share | `concurrency-limits` asks for a missing `concurrency`, not for a wrong group |
| [`concurrency-cancels-release`](checks.md#check-concurrency-cancels-release) | default | error | `cancel-in-progress` on tag pushes, releases and jobs that publish or deploy (fixable, unsafe) | none |
| [`gate-job-skipped-on-failure`](checks.md#check-gate-job-skipped-on-failure) | default | error | a job that reads `needs.*.result` but is skipped when a needed job fails | none |
| [`untrusted-checkout`](checks.md#check-untrusted-checkout) | default | error | `pull_request_target` and `workflow_run` workflows that check out the pull request and run it | `dangerous-triggers` reports the trigger alone, not what the workflow does with the code |
| [`untrusted-artifact`](checks.md#check-untrusted-artifact) | default | error | `workflow_run` workflows that run, extract or export the artifact of the triggering run unchecked | none |
| [`unused-job-output`](checks.md#check-unused-job-output) | default | error | a job output that no job or `workflow_call` output reads | none |
| [`unused-workflow-input`](checks.md#check-unused-workflow-input) | pedantic | warn | an input of `workflow_dispatch` or `workflow_call` that is never read | none |
| [`unused-needs`](checks.md#check-unused-needs) | pedantic | info | a `needs` entry that is neither read nor needed for the order of jobs | none |
| [`duplicate-triggers`](checks.md#check-duplicate-triggers) | default | error | `push` without a branch filter next to `pull_request` | none |
| [`continue-on-error`](checks.md#check-continue-on-error) | pedantic | info | a job with `continue-on-error: true` | none |
| [`mutable-runner-label`](checks.md#check-mutable-runner-label) | pedantic | warn | `ubuntu-latest` and the other labels that GitHub moves (fixable with the `pin` option) | none |

The profile and level columns are those of today: every rule of `default` is an `error`, and the `pedantic` ones keep their own level.
When they were measured the profiles were `default` and `strict`; `duplicate-triggers` was `strict` and was moved to `default` by the
maintainer. It had 512 findings in 512 files of the OSS corpus; 12 were read by hand and all were real, and the others were judged
real as the same pattern without being read.

## Batch H measurements

The rules above ran with `profile: all` over two corpora, without network access: 231 directories of `~/src` with
`.github/workflows` (jdx repositories and their worktrees: 804 distinct workflow files, found by their content) and the
workflows of 1158 distinct repositories in the Go module cache, the cargo registry and the other caches of the machine (2649
distinct files). A finding of a file that occurs in several worktrees is counted once. zizmor 1.30.1 has no audit to compare with,
so nothing is claimed about parity; the table says what jactionlint found and how the findings were judged by reading the
workflow.

| Rule | jdx: findings (files) | OSS: findings (files) | True | False |
| --- | --- | --- | --- | --- |
| `concurrency-cancels-prs` | 0 | 1 (1) | 1 | 0 |
| `concurrency-cancels-release` | 11 (11) | 4 (4) | 15 | 0 |
| `gate-job-skipped-on-failure` | 0 | 0 | 0 | 0 |
| `untrusted-checkout` | 0 | 1 (1) | 1 | 0 |
| `untrusted-artifact` | 0 | 2 (1) | 2 | 0 |
| `unused-job-output` | 6 (6) | 70 (10) | 76 | 0 |
| `unused-workflow-input` | 0 | 7 (6) | 6 | 1 file |
| `unused-needs` | 37 (18) | 45 (17) | 82 | 0 |
| `duplicate-triggers` | 0 | 512 (512) | 512 | 0 |
| `continue-on-error` | 18 (18) | 17 (15) | 35 | 0 |
| `mutable-runner-label` | 1885 (638) | 6089 (2264) | all | 0 |

What the review found:

- Several versions of the rules reported false positives and were changed before the table was made. The first
  `concurrency-cancels-release` took the name of a workflow or a job ("release-window", "minimum_release_age", "Check for a new
  release") as a sign of a release, flagged CI workflows that run for tags, and flagged `goreleaser --snapshot` and `goreleaser check`
  jobs. Names are no longer a signal, a tag only counts when the group does not name the ref, and a job or step whose `if:` is
  false for the trigger is ignored. It also reported the `${{ github.event_name != 'release' && inputs.tag == '' }}` idiom of the
  aube workflows; an expression is now reported only when the event and the ref decide that it is true.
  `untrusted-checkout` reported a release workflow whose job runs only when an earlier job found that the commit has a version tag
  (`needs.*.outputs`); conditions on `needs` and `steps` are guards now.
- `unused-workflow-input` is in the `pedantic` profile because of the one file that is a false positive by design: an input that a
  dispatching tool requires (`distinct_id` for `return-dispatch`). GitHub rejects a dispatch with an input the workflow does not
  declare, so an unused input can be needed. The other five files are real: a `webhook-url` input that the workflow documents
  but never passes on, and a `branch` input that nothing reads.
- `untrusted-checkout`: the one finding is a `pull_request_target` workflow that checks out the head of the pull request into
  `PR_BRANCH` and runs `cargo check` in it. zizmor reports `dangerous-triggers` on the same file, for the trigger.
- `untrusted-artifact`: both findings are the `unzip artifact.zip` step of the pattern in GitHub's own documentation, in a workflow
  that then reads a pull request number. They are true by the definition of the rule; the impact is low, the number is
  converted with `Number()`.
- `gate-job-skipped-on-failure` found nothing: the fan-in jobs of the jdx repositories (`final`, `test-ci`) use
  `always() && !cancelled()`.
- `unused-job-output`: 66 of the 70 OSS findings are in the generated `*.lock.yml` workflows of one repository. They are true, and
  exclude generated files with `paths: ... ignore:` if you do not want them.
- `duplicate-triggers`: 12 findings read by hand, all real (`on: [push, pull_request]` for a test workflow).
- `continue-on-error` finds jobs that are advisory on purpose (`api-stability`, `enhance-release`). That is why it is info.

Gaps, honestly:

- `untrusted-checkout` follows `actions/checkout`, `gh pr checkout` and `git` commands with a literal reference. A ref that comes
  from an output of an earlier step is not followed, and "runs it" is a list of commands, so an unusual build tool that is not
  in the list of commands that only read files is counted as running code, while an action that builds the workspace and is not
  in the short list is not.
- `untrusted-artifact` links the use to the download by the `path` of the artifact, or its `name` (a file in the root of the
  workspace that has another name is not linked), and it takes any `=~`, numeric test or checksum as validation.
- `concurrency-cancels-release` knows a fixed list of publish and deploy actions and commands. A release done by a script of the
  repository (`./scripts/release.sh`) is not recognized; the workflow is only reported for tag pushes and `release` events then.
- `unused-needs` finds only the entries that another needed job makes redundant. An entry that was forgotten and is not redundant
  is indistinguishable from one that sets the order, so it is left out on purpose.
- None of the rules reads the called reusable workflows: a job output or input that a caller in another repository uses is not
  known (for `unused-job-output` it cannot be used from outside, for `unused-workflow-input` it can).

## Batch K measurements

Batch K (downloads and credentials) was run over the workflows of the jdx repositories under `~/src` (146 distinct sets of
workflows, 826 distinct files, many of them worktrees of the same repositories) and 1794 distinct workflow files of open source Go
modules and other repositories. zizmor 1.30.1 (`--offline --persona pedantic`) was run over the first set too: it has no audit
that overlaps with these rules (its `insecure-url-scheme` looks at `.pre-commit-config.yaml` only), so there is nothing to
compare and no zizmor-only finding. All numbers are findings with the rules at their defaults, every finding was read.

| Rule | jdx repositories (distinct) | other repositories | judged true | judged false |
| --- | --: | --: | --: | --: |
| `unverified-download` | 8 (`mise.run \| sh` 4, `setup.deb.sh \| bash`, a pinned `yq` and a `hugo.deb` without a checksum) | 29 (installer scripts piped into a shell, `releases/latest` binaries, `.deb` files installed without a checksum) | all | 0 |
| `insecure-ssh-keyscan` | 0 | 2 (both `ssh-keyscan >> ~/.ssh/known_hosts`) | all | 0 |
| `insecure-url-scheme` | 0 | 0 | n/a | n/a |
| `checkout-static-credentials`, default | 0 | 0 | n/a | n/a |
| `checkout-static-credentials`, `secret-tokens: true` | 12 (the `token:` of the release-plz and release workflows) | 0 | all, as designed | 0 |

`checkout-static-credentials` reports a token from a secret only with `secret-tokens`, which is off under the default profile and on
under the `pedantic` profile: every one of the 12 distinct findings is a release workflow that has to push with a
personal access token so that the push starts other workflows, which is deliberate. They are correct findings of the rule, but
reporting them for everybody would be noise for workflows whose alternative is a GitHub App or a fine-grained token in a protected
environment, so that part is behind the option and `ssh-key` is not.

jactionlint-only means all of them here. Gaps: archives that are downloaded, extracted and run, `go install` or `npx` of a remote
package, `ssh -o StrictHostKeyChecking=no`, and a download whose verification is in another step are not reported.
## Batch L measurements

Both rules were run (default profile, `-shellcheck= -pyflakes=`) over two corpora of unique files (by content hash): the 1020
workflows and Dependabot configurations of every checkout under `~/src` (the `*-jactionlint` worktrees, other worktrees and
repositories), and 1315 files of third party repositories (the Go module cache checkouts in the scratchpad corpus). zizmor has no
equivalent audit, so there is nothing to match against; the numbers are findings and reviewed false positives.

| Rule | jdx corpus | OSS corpus | Judgement |
| --- | --- | --- | --- |
| `invisible-characters` | 0 | 0 | An independent scan of the raw bytes (every format, control, variation selector and filler character) finds such characters in 30 files, and all of them are `U+FE0F` after `ℹ`, `✏`, `⚠` or `🌡` (emoji, mostly in CodeQL templates and comments). The first version of the rule reported `ℹ️` (the information symbol is a letter for Unicode, not a symbol), which the corpus found; it is fixed and tested. No true positive exists in the corpus, so the detection is covered by the unit tests and fixtures only |
| `unsound-prefix-match` | 0 | 1 | `GOOS: ${{ contains(github.repository, 'windows_exporter') && 'windows' || '' }}`: a real partial match, but it selects a build setting and not a credential. The first version reported it. The rule now reports an expression outside `if:` only when it selects a secret, `github.token` or a self-hosted runner, so this is a non-report. No condition in 2335 files tests a name by a prefix, a suffix or a part (the `endsWith(.., '[bot]')` and `!contains(github.actor, '[bot]')` tests are not reported on purpose) |
| `unsound-prefix-match` option `refs` | not run | not run | opt-in, so not measured; a prefix test of a ref is usually meant |

<a id="composite-actions-measurements"></a>
## Composite actions

zizmor audits `action.yml` files with the same audits as workflows, but it has no knowledge of who calls an action
([#1424](https://github.com/zizmorcore/zizmor/issues/1424), [#678](https://github.com/zizmorcore/zizmor/issues/678)).
jactionlint checks the steps with the same rules, takes the trigger of an action from the local workflows that call it, and
documents which rules apply in [composite actions](checks.md#check-composite-actions).

Measured with zizmor 1.30.1 (`--offline --persona pedantic`) and jactionlint with `profile: all` on the action files of
the jdx repositories (35 distinct files; the `*-jactionlint` and other worktrees were de-duplicated by content) and of the 39 OSS projects
of the corpus above (103 action files). A finding matches when both tools report the same audit in the same file within 8 lines (a
multi-line expression is reported at its first line by jactionlint and at another line by zizmor):

| Audit | jdx corpus | OSS corpus | Differences |
| --- | --- | --- | --- |
| `template-injection` | 12 / 11 / 11 | 263 / 295 / 263 | jactionlint reports the findings of the option `pedantic` (they were the rules `template-injection-expansion` and `template-injection-trusted`), which are zizmor's pedantic persona; the extra OSS findings are expansions of values zizmor does not list. The one jdx miss is a second line of one multi-line expression |
| `unpinned-uses` | 4 / 3 / 3 | 0 / 7 / 0 | the extra findings are `actions/*@tag` refs, which zizmor's default policy allows |
| `github-app` | 1 / 1 / 1 | 0 / 0 / 0 | none |
| `self-repository` | 0 / 0 / 0 | 15 / 21 / 15 | the six extra findings are the repeated `uses: ./.github/actions/...` of the same local action in two Apache Airflow files, of which zizmor reported none |
| `github-env`, `adhoc-packages` | 6 / 0 / 0 | 13 / 0 / 0 | zizmor-only when these were measured, before batch D (`github-env`, `adhoc-packages`) was in the stack; both rules now run on composite actions (`github-env` with the events of the callers) and this row was not measured again |

The jactionlint-only findings were reviewed: `action-syntax` (7 jdx / 3 OSS) are `type:` keys on action inputs, which GitHub ignores;
`undefined-property` (2, cilium) is a step of the calling job read through `steps`, which a composite action cannot see; `cache-poisoning`
(2 jdx / 1 OSS) is a cache restored by an action called from a workflow on pushed tags. The first run found a real false positive,
the `${{ github.token }}` default of an input (context availability of defaults), which is fixed. No false positive remains in the
reviewed sets.
