# zizmor parity

> [!WARNING]
> This is a tracking document for planned work. None of the planned rules below exist yet unless the status says so.

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
| `archived-uses` | `archived-uses` | 3 / G | online | not yet assessed |
| `artipacked` | `artipacked` | 3 / B | strict | not yet assessed |
| `bot-conditions` | `bot-conditions` | 3 / C | strict | not yet assessed |
| `cache-poisoning` | `cache-poisoning` | 3 / B | strict | not yet assessed |
| `concurrency-limits` | `concurrency-limits` | 3 / A | strict | not yet assessed |
| `dangerous-triggers` | `dangerous-triggers` | 3 / A | default | not yet assessed |
| `dependabot-cooldown` | `dependabot-cooldown` | 3 / E | strict | not yet assessed |
| `dependabot-execution` | `dependabot-execution` | 3 / E | strict | not yet assessed |
| `excessive-permissions` | `excessive-permissions` | 3 / B | strict | partial: `missing-permissions` (strict) covers the case of no `permissions:` block ([measured](#measured-missing-permissions-and-missing-timeout)); write scopes and `read-all`/`write-all` are not covered yet |
| `forbidden-uses` | `forbidden-uses` | 3 / A | opt-in (allow/deny config) | not yet assessed |
| `github-app` | `github-app` | 3 / B and E | strict | not yet assessed |
| `github-env` | `github-env` | 3 / D | default | not yet assessed |
| `hardcoded-container-credentials` | existing check, [Hardcoded credentials](checks.md#check-hardcoded-credentials); ID assigned in Phase 1 | exists today, ID in Phase 1 | default | not yet assessed |
| `impostor-commit` | `impostor-commit` | 3 / G | online | not yet assessed |
| `insecure-commands` | `insecure-commands` | 3 / A | default | not yet assessed |
| `insecure-url-scheme` | `insecure-url-scheme` (where applicable to dependabot.yml) | 3 / E | strict | not yet assessed |
| `known-vulnerable-actions` | `known-vulnerable-actions` | 3 / G | online | not yet assessed |
| `misfeature` | `misfeature` | 3 / C | strict | not yet assessed |
| `obfuscation` | `obfuscation` | 3 / C | strict | not yet assessed |
| `overprovisioned-secrets` | `overprovisioned-secrets` | 3 / A | strict | not yet assessed |
| `ref-confusion` | `ref-confusion` | 3 / G | online | not yet assessed |
| `ref-version-mismatch` | `ref-version-mismatch` | 3 / G | online | not yet assessed |
| `secrets-inherit` | `secrets-inherit` | 3 / A | default | not yet assessed |
| `secrets-outside-env` | `secrets-outside-env` | 3 / A | strict | not yet assessed |
| `self-hosted-runner` | `self-hosted-runner` | 3 / A | all (info) | not yet assessed |
| `self-repository` | `self-repository` | 3 / B | strict | not yet assessed |
| `stale-action-refs` | `stale-action-refs` | 3 / G | online | not yet assessed |
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

The fixers were measured on the 955 files: `jactionlint -fix` adds exactly 1010 `timeout-minutes: 30` lines and 28 `permissions:` blocks
(the safe ones) and changes no other line (`diff -r` shows added lines only); `-fix=unsafe` adds 137 blocks; after either run
the rules report nothing that has a fix, and a second run changes nothing.
