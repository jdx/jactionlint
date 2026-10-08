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
| `artipacked` | `artipacked` | 3 / B | strict | full on the corpus, with a fix. [batch B measurements](#batch-b-corpus-measurements) |
| `bot-conditions` | `bot-conditions` | 3 / C | strict | not yet assessed |
| `cache-poisoning` | `cache-poisoning` | 3 / B | strict | partial: a fixed table of cache actions, `tags-ignore` is not a release trigger. [batch B measurements](#batch-b-corpus-measurements) |
| `concurrency-limits` | `concurrency-limits` | 3 / A | strict | not yet assessed |
| `dangerous-triggers` | `dangerous-triggers` | 3 / A | default | not yet assessed |
| `dependabot-cooldown` | `dependabot-cooldown` | 3 / E | strict | not yet assessed |
| `dependabot-execution` | `dependabot-execution` | 3 / E | strict | not yet assessed |
| `excessive-permissions` | `excessive-permissions` | 3 / B | strict | partial: write scopes, `write-all`, `read-all`, workflow-level default permissions (option) and, with `missing-permissions`, job-level default permissions. [batch B measurements](#batch-b-corpus-measurements) |
| `forbidden-uses` | `forbidden-uses` | 3 / A | opt-in (allow/deny config) | not yet assessed |
| `github-app` | `github-app` | 3 / B and E | strict | partial: `actions/create-github-app-token` only, no dependabot side yet. [batch B measurements](#batch-b-corpus-measurements) |
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
| `self-repository` | `self-repository` | 3 / B | strict (info) | full on the corpus, with an unsafe fix. [batch B measurements](#batch-b-corpus-measurements) |
| `stale-action-refs` | `stale-action-refs` | 3 / G | online | not yet assessed |
| `superfluous-actions` | `superfluous-actions` | 3 / D | strict | not yet assessed |
| `template-injection` | `template-injection` | 3 / C | default | not yet assessed |
| `typosquat-uses` | `typosquat-uses` | 3 / A | strict | not yet assessed |
| `undocumented-permissions` | `undocumented-permissions` | 3 / B (needs YAML comments, Phase 2) | all | partial: a comment above a scope counts, `include-read` for zizmor's read scopes. [batch B measurements](#batch-b-corpus-measurements) |
| `unpinned-images` | `unpinned-images` | 3 / B | strict | partial: no per-persona split, images in `docker://` are `unpinned-uses`. [batch B measurements](#batch-b-corpus-measurements) |
| `unpinned-tools` | `unpinned-tools` | 3 / D | strict | not yet assessed |
| `unpinned-uses` | `unpinned-uses` | 3 / B | strict | partial: `hash-pin`, `ref-pin`, `any` policies with a subset of zizmor's patterns. [batch B measurements](#batch-b-corpus-measurements) |
| `unredacted-secrets` | `unredacted-secrets` | 3 / A | strict | not yet assessed |
| `unsound-condition` | `unsound-condition` | 3 / C | default | not yet assessed |
| `unsound-contains` | `unsound-contains` | 3 / A | default | not yet assessed |
| `unsound-ternary` | `unsound-ternary` | 3 / C | default | not yet assessed |
| `use-trusted-publishing` | `use-trusted-publishing` | 3 / D | strict | not yet assessed |

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

See [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features) for the
criteria a rule must meet before it is added.
