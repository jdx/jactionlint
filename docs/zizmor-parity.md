# zizmor parity

> [!WARNING]
> This is a tracking document for planned work. None of the planned rules below exist yet unless the status says so. The six online
> audits exist and were measured, see [Online audits](#online-audits).

jactionlint v2 aims to cover the security and policy territory that [zizmor](https://docs.zizmor.sh/audits/) covers, so that
a repository does not need to run two tools. Parity is tracked **per audit**. jactionlint makes **no blanket claim** that
it replaces zizmor: an audit is only marked as covered after a differential run against zizmor over a corpus of real
repositories shows comparable findings and no false positives for rules enabled by default.

How to read the table:

- **Planned ID** is the stable rule ID jactionlint intends to use. IDs reuse zizmor's audit names so users can map them.
- **Phase / batch** refers to the v2 plan: the batch letters are Phase 3 rule batches. A is checks on data that is already
  parsed, B permissions and pinning, C expression analysis, D run-script analysis, E `dependabot.yml`, G online checks.
  Online checks only run with the opt-in `-online` flag and never in the playground.
- **Profile** is the profile the rule is proposed for (`default`, `strict` or `all`, see [the v2 migration](v2-migration.md)).
  It is not final until the false-positive review of the corpus. Two rows use an activation mode instead of a profile:
  `online` rules run only with the opt-in `-online` flag, whatever the profile, and `opt-in (allow/deny config)` rules do
  nothing until you configure an allow or deny list.
- **Parity** will become `full`, `partial` or `not planned` once the differential corpus test exists. Until then every
  row says `not yet assessed`.

| zizmor audit | Planned ID | Phase / batch | Profile | Parity |
| --- | --- | --- | --- | --- |
| `adhoc-packages` | `adhoc-packages` | 3 / D | default | partial: 7 of the 10 findings of zizmor on the corpus. The 3 others are 2 installs of local tarballs (`npm install ../pkg/*.tgz`), which jactionlint does not call ad hoc, and a position difference in a folded block. jactionlint reports installs behind `sudo` that zizmor misses. No `pwsh` scripts. See [the check](checks.md#check-adhoc-packages) |
| `anonymous-definition` | `anonymous-definition` | 3 / A | strict | full for workflows and jobs (action.yml is not linted); fixable |
| `archived-uses` | `archived-uses` | 3 / G | online | partial: all archived repositories are found; see [online audits](#online-audits) |
| `artipacked` | `artipacked` | 3 / B | strict | full on the corpus, with a fix. [batch B measurements](#batch-b-corpus-measurements) |
| `bot-conditions` | `bot-conditions` | 3 / C | strict | partial: `github.actor`, `github.triggering_actor`, `github.actor_id` and `github.event.sender.*` compared with a bot (`==`, `contains()`, `startsWith()`, `endsWith()`) in job and step `if:`. `!=` and negated tests are not reported, as in zizmor. Measured, see [batch C](#batch-c-measurements). `-fix=unsafe` only for workflows with `pull_request`/`pull_request_target` events. Bot names without `[bot]` are known by ID and prefix only. No composite actions yet |
| `cache-poisoning` | `cache-poisoning` | 3 / B | strict | partial: a fixed table of cache actions, `tags-ignore` is not a release trigger. [batch B measurements](#batch-b-corpus-measurements) |
| `concurrency-limits` | `concurrency-limits` | 3 / A | strict | full: only a missing `concurrency:` and the bare group form are reported; see [measurements](#batch-a-measurements) |
| `dangerous-triggers` | `dangerous-triggers` | 3 / A | strict | full; adds `issue_comment`, which zizmor flags from 1.31; reported at the trigger, zizmor reports at `on:` |
| `dependabot-cooldown` | `dependabot-cooldown` | 3 / E | default | full on the corpus: 386 of 386 zizmor findings in 193 repositories, plus 3 true positives zizmor 1.30.1 misses (it stops at the first update which satisfies the minimum). `-fix` needs the `default-days` option; zizmor has a built-in 7. The `semver-*-days` keys are checked by neither |
| `dependabot-execution` | `dependabot-execution` | 3 / E | default | not measured: no `allow` in the 193 repositories of the corpus, so both tools report 0. Covered by unit tests only. The fix is unsafe (zizmor offers it too) |
| `excessive-permissions` | `excessive-permissions` | 3 / B | strict | partial: write scopes, `write-all`, `read-all`, workflow-level default permissions (option) and, with `missing-permissions`, job-level default permissions. [batch B measurements](#batch-b-corpus-measurements), [missing-permissions measurements](#measured-missing-permissions-and-missing-timeout) |
| `forbidden-uses` | `forbidden-uses` | 3 / A | opt-in (allow/deny config) | partial: patterns follow zizmor documentation but were not measured against zizmor |
| `github-app` | `github-app` | 3 / B and E | strict | partial: `actions/create-github-app-token` only, no dependabot side yet. [batch B measurements](#batch-b-corpus-measurements) |
| `github-env` | `github-env`, `github-env-untrusted-input` | 3 / D | default | partial: zizmor and jactionlint both have no finding on the corpus. jactionlint accepts values of trusted contexts (`github.sha`, `runner.*`) and `mktemp`/`date` substitutions that zizmor reports in `pull_request_target` and `workflow_run` workflows, and reports untrusted input under every trigger. `pwsh` and `cmd` are matched line by line. See [the check](checks.md#check-github-env) |
| `hardcoded-container-credentials` | existing check, [Hardcoded credentials](checks.md#check-hardcoded-credentials); ID assigned in Phase 1 | exists today, ID in Phase 1 | default | not yet assessed |
| `impostor-commit` | `impostor-commit` | 3 / G | online | partial: same findings on the corpus; a repository with more than 1000 branches gives no verdict (100 without a token); see [online audits](#online-audits) |
| `insecure-commands` | `insecure-commands` | 3 / A | default | partial: workflow, job and step `env` (not action.yml); fixable (unsafe) |
| `insecure-url-scheme` | `insecure-url-scheme` | 3 / K | default | none to compare: zizmor 1.30.1 applies it to the `repo:` URLs of `.pre-commit-config.yaml` only, which jactionlint does not lint. jactionlint's rule is a different one with the same ID: `http://`, `ftp://` and `git://` locations in `run:` scripts and `with:` inputs. [batch K measurements](#batch-k-measurements) |
| `known-vulnerable-actions` | `known-vulnerable-actions` | 3 / G | online | partial: same findings on the corpus; branch refs are not judged; see [online audits](#online-audits) |
| `misfeature` | `misfeature`, `misfeature-custom-shell` | 3 / C | strict, all | partial: `pip-install` of setup-python and `shell: cmd` (`misfeature`, 18 of 18 zizmor findings of the corpus). Shells that are not well known are `misfeature-custom-shell` (`all`, info), which zizmor reports in the auditor persona only and which is not compared. No composite actions yet |
| `obfuscation` | `obfuscation` | 3 / C | strict | partial: redundant segments at `uses:`, constant expressions and `format()` of literals outside `if:`, `fromJSON(toJSON(x))` and computed indices. All zizmor findings of the corpus are reported (see [batch C](#batch-c-measurements)). Constants in `if:` are `constant-condition`; `fromJSON(toJSON(context))` is deliberately not reported; the `uses:` fix is unsafe. No composite actions yet |
| `overprovisioned-secrets` | `overprovisioned-secrets` | 3 / A | strict | full on the synthetic cases; 0 findings on the corpus |
| `ref-confusion` | `ref-confusion` | 3 / G | online | partial: no finding on the corpus by either tool; see [online audits](#online-audits) |
| `ref-version-mismatch` | `ref-version-mismatch` | 3 / G | online | partial: every mismatch of a tag comment found; a missing or version-less comment and a branch comment are not reported; see [online audits](#online-audits) |
| `secrets-inherit` | `secrets-inherit` | 3 / A | default | full |
| `secrets-outside-env` | `secrets-outside-env` | 3 / A | all | full; zizmor reports it only for the auditor persona |
| `self-hosted-runner` | `self-hosted-runner` | 3 / A | all (info) | partial: literal `self-hosted` labels and matrix values; labels from other expressions are not resolved |
| `self-repository` | `self-repository` | 3 / B | strict (info) | full on the corpus, with an unsafe fix. [batch B measurements](#batch-b-corpus-measurements) |
| `stale-action-refs` | `stale-action-refs` | 3 / G | online | partial: same findings except repositories with more than 1000 tags; see [online audits](#online-audits) |
| `superfluous-actions` | `superfluous-actions` | 3 / D | default | full: the same 40 findings as zizmor on the corpus, split like its personas (7 regular, and the 33 pedantic ones with the option `pedantic`, which the `strict` profile turns on). jactionlint adds the archived `actions/create-release` and `actions/upload-release-asset`. See [the check](checks.md#check-superfluous-actions) |
| `template-injection` | `template-injection` (default), `template-injection-expansion` (strict), `template-injection-trusted` (all) | 3 / C | default, strict, all | partial: attacker controlled contexts, objects holding them and env variables set from them in `run:`, github-script and the code inputs of well-known actions, every expression of a script (default). Every other expansion is `template-injection-expansion` (free text) or `template-injection-trusted` (values like `github.repository`), which together are zizmor's pedantic persona. See [batch C](#batch-c-measurements) for the numbers. Not covered: `action.yml`, knowledge about the outputs of popular actions, severity by trigger (the level is per rule ID). `-fix` moves a simple reference into `env:` for bash and sh. Batch J adds sinks that zizmor lacks: `container.options` and `services.<id>.options` ([zizmor#1128](https://github.com/zizmorcore/zizmor/issues/1128), still open), their image, entrypoint, command and volumes, `args` and `entrypoint` of `docker://` steps, more code inputs of well-known actions, and the prompt, arguments and settings of AI agent actions; see [batch J](#batch-j-measurements) |
| `typosquat-uses` | `typosquat-uses` | 3 / A | strict | partial: one typo of the slug, not all of the transformations of zizmor |
| `undocumented-permissions` | `undocumented-permissions` | 3 / B (needs YAML comments, Phase 2) | all | partial: a comment above a scope counts, `include-read` for zizmor's read scopes. [batch B measurements](#batch-b-corpus-measurements) |
| `unpinned-images` | `unpinned-images` | 3 / B | strict | partial: no per-persona split, images in `docker://` are `unpinned-uses`. [batch B measurements](#batch-b-corpus-measurements) |
| `unpinned-tools` | `unpinned-tools` | 3 / D | default | full for the 4 actions zizmor knows (no finding on the corpus in either tool). The option `pedantic` (on under `strict`) adds `run:` installs without an exact version and three more actions. An input that is an expression is not reported, zizmor reports it with low confidence. See [the check](checks.md#check-unpinned-tools) |
| `unpinned-uses` | `unpinned-uses` | 3 / B | strict | partial: `hash-pin`, `ref-pin`, `any` policies with a subset of zizmor's patterns. [batch B measurements](#batch-b-corpus-measurements) |
| `unredacted-secrets` | `unredacted-secrets` | 3 / A | strict | full on the synthetic cases; 0 findings on the corpus |
| `unsound-condition` | `if-always-true` (existing) | 3 / C | default | full for workflows: `if-always-true` reports every `if:` with characters around `${{ }}`, a block scalar's trailing newline included. Matches all 5 zizmor findings of the corpus. No new rule. Composite action steps are not checked yet |
| `unsound-contains` | `unsound-contains` | 3 / A | default | full on the synthetic cases; 0 findings on the corpus |
| `unsound-ternary` | `unsound-ternary` | 3 / C | default | not yet assessed |
| `use-trusted-publishing` | `use-trusted-publishing` | 3 / D | default | partial: 2 of the 4 findings of zizmor on the corpus. The 2 others are in reusable workflows without `permissions:`, which jactionlint skips because the caller may grant `id-token: write`. No `pwsh` scripts and no `npm run publish`. See [the check](checks.md#check-use-trusted-publishing) |

## Beyond zizmor

<a id="beyond-zizmor"></a>

Rules of jactionlint which zizmor 1.30.1 has no audit for:

| ID | Profile | What it reports |
| --- | --- | --- |
| `unlocked-install` | default | `cargo install` without `--locked`. Fixable (unsafe). With the option `pedantic` (on under `strict`): `npm install` instead of `npm ci`, `yarn` and `bun install` without a frozen lock file, `pnpm install --no-frozen-lockfile`, `pip install -r` without hashes or constraints. See [the check](checks.md#check-unlocked-install) |
| `dependabot-missing-actions-update` | strict | `dependabot.yml` has no `github-actions` update although `.github/workflows` uses actions. It is skipped when the repository has a Renovate configuration. 20 findings in 193 repositories, all true positives (a Go module with only a `gomod` update). See [the check](checks.md#check-dependabot-missing-actions-update) |
| `pipeline-without-pipefail` | default | A failure of a command in a pipeline of a `run:` script is hidden because the default shell (`bash -e {0}`) and `shell: sh` do not enable pipefail. |
| `agentic-actions` | default | AI agent actions ([zizmor#1605](https://github.com/zizmorcore/zizmor/issues/1605) is a proposal): an agent that outsiders can steer without a check of the user, an open gate (`allowed_non_write_users: '*'`), settings that turn the safeguards off, and an agent that runs on the code of a pull request. See [AI agent actions](checks.md#check-agentic-actions) |
| `unverified-download` | default | a download piped into a shell or an interpreter, a downloaded file made executable and run without a checksum or signature check, and TLS verification turned off for a download (error). Closest zizmor audit: none; the idea is zizmor [#711](https://github.com/zizmorcore/zizmor/issues/711) |
| `insecure-ssh-keyscan` | default | `ssh-keyscan` output written to a `known_hosts` file (error). Closest zizmor audit: none; the idea is zizmor [#2012](https://github.com/zizmorcore/zizmor/issues/2012) |
| `checkout-static-credentials` | default | `actions/checkout` given an `ssh-key` or a literal `token` (and, with `secret-tokens` (on under `strict` and `all`), a token from a secret other than `GITHUB_TOKEN`) (error). Closest zizmor audit: none; the idea is zizmor [#1118](https://github.com/zizmorcore/zizmor/issues/1118) |
| `insecure-url-scheme` | default | `http://`, `ftp://` and `git://` locations in `run:` downloads and in `with:` inputs (error). Closest zizmor audit: `insecure-url-scheme`, which only checks `repo:` URLs of `.pre-commit-config.yaml` |

## Batch B corpus measurements

Batch B (permissions, pinning, checkout) was measured against zizmor 1.30.1 (`--offline --persona pedantic`) over 38
repositories with 378 workflow files: the jdx repositories checked out under `~/src` plus entirecli, entiredb, peregrine,
renovatebot/renovate and oxc-project/oxc, so that not all of them are written by the same people. jactionlint ran with the
`all` profile, `excessive-permissions: {require-workflow-permissions: true}` and `undocumented-permissions: {include-read: true}`.
A finding matches when it is in the same file and on the line of the primary location of the zizmor finding. `artipacked` is
matched within three lines below it, because zizmor points at the first line of a step and jactionlint at its `uses:`. All
rules are in the `strict` or `all` profile, so none of this changes what the default profile reports.

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
  all findings of the rule share it (`warn`).
- `unpinned-uses` supports the patterns `*`, `owner/*`, `owner/repo` and `owner/repo/path`. Other forms of zizmor's repository
  patterns are rejected when the configuration is read. Docker images follow the `*` pattern only.
- `unpinned-images` does not distinguish regular and pedantic findings (an image without a tag against a tag without a
  digest). The `require-digest` option switches between the two behaviors.
- `github-app` knows `actions/create-github-app-token` only.
- `cache-poisoning` knows the cache actions in a fixed table. Cache modes of the dangerous-write half (`cache-mode: write` on a
  privileged trigger) were added after zizmor 1.30.1, so that half has no zizmor measurement.
- Findings are not suppressed by `# zizmor: ignore[...]` comments.

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
- Reusable workflows are checked like actions; composite action files (`action.yml`) are not linted by jactionlint yet.

Cost: for the 173 `uses:` lines of this repository the first run makes 91 requests, a run within the hour none, and a later run
only conditional requests (a 304 answer is free of rate limit for authenticated clients).

## Ignore comments

jactionlint honors zizmor's `# zizmor: ignore[...]` comments for the audits that map onto one of its rules, so a repository
that already triaged its zizmor findings keeps them triaged. This is measured only for the rules that exist: on the corpus of
520 workflow files with a zizmor comment, 2 name a mapped audit today (`template-injection`) and the rest name audits (such as
`cache-poisoning` and `dangerous-triggers`) that have no jactionlint rule yet, so they are inert until those rules land. For
`template-injection`, jactionlint reports fewer contexts than zizmor, so a comment can be stale for jactionlint (the
`unused-ignore` rule says so). See [the usage document](usage.md#zizmor-ignore-comments) and
[the alias table](v2-migration.md#zizmor-ignore-comments).

See [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features) for the
criteria a rule must meet before it is added.

## Measured: missing-permissions and missing-timeout

Batch F measured two existing rules on the local corpus: the checkouts of 35 repositories under `~/src` (the "main" numbers) and
all 955 distinct workflow files of the 139 worktrees there (the "all" numbers). The zizmor numbers are from
`zizmor --offline --persona pedantic` 1.30.1 through `scripts/zizmor-diff`.

| Rule | Profile | Findings (main / all) | Overlap with zizmor | Only jactionlint | Only zizmor |
| --- | --- | --: | --- | --- | --- |
| `missing-timeout` | default | 270 / 1010, in 27 of 35 repositories | none: zizmor 1.30.1 has no audit for it | all of them are jobs without `timeout-minutes` and without `uses:`; a separate count of such jobs in the YAML gives the same 1010, so there are no false positives by that definition | not applicable |
| `missing-permissions` | strict | 74 / 372, in 14 of 35 repositories | 74 of 74 are `excessive-permissions` findings of zizmor at the same job line | none | 45 are zizmor's workflow-level (line 1) report of the same missing block; 54 are `contents: write`, `id-token: write` and other write scopes at the workflow level, which `missing-permissions` does not look at |

`missing-timeout` is in the default profile because the user asked for it, not because it passed the false-positive bar in the
sense of the other default rules: it has no false positives, but it is noisy for a repository that never set the key (270
findings in 27 of the 35 repositories). `jactionlint -fix` clears all of them. `missing-permissions` stays in `strict`.

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

How to read the template injection rows:

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

Known differences that are not measured: the position of a finding in a double-quoted multi-line YAML string is the first line, and
composite actions (`action.yml`) are not analyzed at all.
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
have no measured false-positive rate, only the synthetic cases above, so `unredacted-secrets`, `overprovisioned-secrets` and
`typosquat-uses` stay in the `strict` profile and `insecure-commands`, `unsound-contains` and `secrets-inherit` are the only batch A rules
in `default`.

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

These rules have no equivalent audit in zizmor 1.30.1. They are rules of jactionlint's own, so they are not part of the parity
table above, and the differential harness (`scripts/zizmor-diff`) has no mapping for them. A finding of one of them is always
"only jactionlint" in its report.

| Rule | Profile | Level | What it finds | Closest zizmor audit |
| --- | --- | --- | --- | --- |
| [`concurrency-cancels-prs`](checks.md#check-concurrency-cancels-prs) | default | warn | a `cancel-in-progress` group that all pull requests share | `concurrency-limits` asks for a missing `concurrency`, not for a wrong group |
| [`concurrency-cancels-release`](checks.md#check-concurrency-cancels-release) | default | warn | `cancel-in-progress` on tag pushes, releases and jobs that publish or deploy (fixable, unsafe) | none |
| [`gate-job-skipped-on-failure`](checks.md#check-gate-job-skipped-on-failure) | default | error | a job that reads `needs.*.result` but is skipped when a needed job fails | none |
| [`untrusted-checkout`](checks.md#check-untrusted-checkout) | default | error | `pull_request_target` and `workflow_run` workflows that check out the pull request and run it | `dangerous-triggers` reports the trigger alone, not what the workflow does with the code |
| [`untrusted-artifact`](checks.md#check-untrusted-artifact) | default | error | `workflow_run` workflows that run, extract or export the artifact of the triggering run unchecked | none |
| [`unused-job-output`](checks.md#check-unused-job-output) | default | warn | a job output that no job or `workflow_call` output reads | none |
| [`unused-workflow-input`](checks.md#check-unused-workflow-input) | strict | warn | an input of `workflow_dispatch` or `workflow_call` that is never read | none |
| [`unused-needs`](checks.md#check-unused-needs) | strict | info | a `needs` entry that is neither read nor needed for the order of jobs | none |
| [`duplicate-triggers`](checks.md#check-duplicate-triggers) | strict | warn | `push` without a branch filter next to `pull_request` | none |
| [`continue-on-error`](checks.md#check-continue-on-error) | strict | info | a job with `continue-on-error: true` | none |
| [`mutable-runner-label`](checks.md#check-mutable-runner-label) | strict | warn | `ubuntu-latest` and the other labels that GitHub moves (fixable with the `pin` option) | none |

The profile column follows the policy of the maintainer: `default` for what has (almost) no false positives, and `strict` (the
pedantic tier) for the opinionated checks, whatever the volume.

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
- `unused-workflow-input` is in the pedantic tier because of the one file that is a false positive by design: an input that a
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
under `strict` and `all` (a pedantic check): every one of the 12 distinct findings is a release workflow that has to push with a
personal access token so that the push starts other workflows, which is deliberate. They are correct findings of the rule, but
reporting them for everybody would be noise for workflows whose alternative is a GitHub App or a fine-grained token in a protected
environment, so that part is behind the option and `ssh-key` is not.

jactionlint-only means all of them here. Gaps: archives that are downloaded, extracted and run, `go install` or `npx` of a remote
package, `ssh -o StrictHostKeyChecking=no`, and a download whose verification is in another step are not reported.
