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
| `adhoc-packages` | `adhoc-packages` | 3 / D | strict | not yet assessed |
| `anonymous-definition` | `anonymous-definition` | 3 / A | strict | not yet assessed |
| `archived-uses` | `archived-uses` | 3 / G | online | partial: all archived repositories are found; see [online audits](#online-audits) |
| `artipacked` | `artipacked` | 3 / B | strict | not yet assessed |
| `bot-conditions` | `bot-conditions` | 3 / C | strict | not yet assessed |
| `cache-poisoning` | `cache-poisoning` | 3 / B | strict | not yet assessed |
| `concurrency-limits` | `concurrency-limits` | 3 / A | strict | not yet assessed |
| `dangerous-triggers` | `dangerous-triggers` | 3 / A | default | not yet assessed |
| `dependabot-cooldown` | `dependabot-cooldown` | 3 / E | strict | not yet assessed |
| `dependabot-execution` | `dependabot-execution` | 3 / E | strict | not yet assessed |
| `excessive-permissions` | `excessive-permissions` | 3 / B | strict | not yet assessed |
| `forbidden-uses` | `forbidden-uses` | 3 / A | opt-in (allow/deny config) | not yet assessed |
| `github-app` | `github-app` | 3 / B and E | strict | not yet assessed |
| `github-env` | `github-env` | 3 / D | default | not yet assessed |
| `hardcoded-container-credentials` | existing check, [Hardcoded credentials](checks.md#check-hardcoded-credentials); ID assigned in Phase 1 | exists today, ID in Phase 1 | default | not yet assessed |
| `impostor-commit` | `impostor-commit` | 3 / G | online | partial: same findings on the corpus; a repository with more than 1000 branches gives no verdict (100 without a token); see [online audits](#online-audits) |
| `insecure-commands` | `insecure-commands` | 3 / A | default | not yet assessed |
| `insecure-url-scheme` | `insecure-url-scheme` (where applicable to dependabot.yml) | 3 / E | strict | not yet assessed |
| `known-vulnerable-actions` | `known-vulnerable-actions` | 3 / G | online | partial: same findings on the corpus; branch refs are not judged; see [online audits](#online-audits) |
| `misfeature` | `misfeature` | 3 / C | strict | not yet assessed |
| `obfuscation` | `obfuscation` | 3 / C | strict | not yet assessed |
| `overprovisioned-secrets` | `overprovisioned-secrets` | 3 / A | strict | not yet assessed |
| `ref-confusion` | `ref-confusion` | 3 / G | online | partial: no finding on the corpus by either tool; see [online audits](#online-audits) |
| `ref-version-mismatch` | `ref-version-mismatch` | 3 / G | online | partial: every mismatch of a tag comment found; a missing or version-less comment and a branch comment are not reported; see [online audits](#online-audits) |
| `secrets-inherit` | `secrets-inherit` | 3 / A | default | not yet assessed |
| `secrets-outside-env` | `secrets-outside-env` | 3 / A | strict | not yet assessed |
| `self-hosted-runner` | `self-hosted-runner` | 3 / A | all (info) | not yet assessed |
| `self-repository` | `self-repository` | 3 / B | strict | not yet assessed |
| `stale-action-refs` | `stale-action-refs` | 3 / G | online | partial: same findings except repositories with more than 1000 tags; see [online audits](#online-audits) |
| `superfluous-actions` | `superfluous-actions` | 3 / D | strict | not yet assessed |
| `template-injection` | `template-injection` | 3 / C | default | not yet assessed |
| `typosquat-uses` | `typosquat-uses` | 3 / A | strict | not yet assessed |
| `undocumented-permissions` | `undocumented-permissions` | 3 / B (needs YAML comments, Phase 2) | strict | not yet assessed |
| `unpinned-images` | `unpinned-images` | 3 / B | strict | not yet assessed |
| `unpinned-tools` | `unpinned-tools` | 3 / D | strict | not yet assessed |
| `unpinned-uses` | `unpinned-uses` | 3 / B | strict | not yet assessed |
| `unredacted-secrets` | `unredacted-secrets` | 3 / A | strict | not yet assessed |
| `unsound-condition` | `unsound-condition` | 3 / C | default | not yet assessed |
| `unsound-contains` | `unsound-contains` | 3 / A | default | not yet assessed |
| `unsound-ternary` | `unsound-ternary` | 3 / C | default | not yet assessed |
| `use-trusted-publishing` | `use-trusted-publishing` | 3 / D | strict | not yet assessed |

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

See [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features) for the
criteria a rule must meet before it is added.
