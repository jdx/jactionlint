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
| `anonymous-definition` | `anonymous-definition` | 3 / A | strict | full for workflows and jobs (action.yml is not linted); fixable |
| `archived-uses` | `archived-uses` | 3 / G | online | not yet assessed |
| `artipacked` | `artipacked` | 3 / B | strict | not yet assessed |
| `bot-conditions` | `bot-conditions` | 3 / C | strict | not yet assessed |
| `cache-poisoning` | `cache-poisoning` | 3 / B | strict | not yet assessed |
| `concurrency-limits` | `concurrency-limits` | 3 / A | strict | full: only a missing `concurrency:` and the bare group form are reported; see [measurements](#batch-a-measurements) |
| `dangerous-triggers` | `dangerous-triggers` | 3 / A | strict | full; adds `issue_comment`, which zizmor flags from 1.31; reported at the trigger, zizmor reports at `on:` |
| `dependabot-cooldown` | `dependabot-cooldown` | 3 / E | strict | not yet assessed |
| `dependabot-execution` | `dependabot-execution` | 3 / E | strict | not yet assessed |
| `excessive-permissions` | `excessive-permissions` | 3 / B | strict | not yet assessed |
| `forbidden-uses` | `forbidden-uses` | 3 / A | opt-in (allow/deny config) | partial: patterns follow zizmor documentation but were not measured against zizmor |
| `github-app` | `github-app` | 3 / B and E | strict | not yet assessed |
| `github-env` | `github-env` | 3 / D | default | not yet assessed |
| `hardcoded-container-credentials` | existing check, [Hardcoded credentials](checks.md#check-hardcoded-credentials); ID assigned in Phase 1 | exists today, ID in Phase 1 | default | not yet assessed |
| `impostor-commit` | `impostor-commit` | 3 / G | online | not yet assessed |
| `insecure-commands` | `insecure-commands` | 3 / A | default | partial: workflow, job and step `env` (not action.yml); fixable (unsafe) |
| `insecure-url-scheme` | `insecure-url-scheme` (where applicable to dependabot.yml) | 3 / E | strict | not yet assessed |
| `known-vulnerable-actions` | `known-vulnerable-actions` | 3 / G | online | not yet assessed |
| `misfeature` | `misfeature` | 3 / C | strict | not yet assessed |
| `obfuscation` | `obfuscation` | 3 / C | strict | not yet assessed |
| `overprovisioned-secrets` | `overprovisioned-secrets` | 3 / A | strict | full on the synthetic cases; 0 findings on the corpus |
| `ref-confusion` | `ref-confusion` | 3 / G | online | not yet assessed |
| `ref-version-mismatch` | `ref-version-mismatch` | 3 / G | online | not yet assessed |
| `secrets-inherit` | `secrets-inherit` | 3 / A | default | full |
| `secrets-outside-env` | `secrets-outside-env` | 3 / A | all | full; zizmor reports it only for the auditor persona |
| `self-hosted-runner` | `self-hosted-runner` | 3 / A | all (info) | partial: literal `self-hosted` labels and matrix values; labels from other expressions are not resolved |
| `self-repository` | `self-repository` | 3 / B | strict | not yet assessed |
| `stale-action-refs` | `stale-action-refs` | 3 / G | online | not yet assessed |
| `superfluous-actions` | `superfluous-actions` | 3 / D | strict | not yet assessed |
| `template-injection` | `template-injection` | 3 / C | default | not yet assessed |
| `typosquat-uses` | `typosquat-uses` | 3 / A | strict | partial: one typo of the slug, not all of the transformations of zizmor |
| `undocumented-permissions` | `undocumented-permissions` | 3 / B (needs YAML comments, Phase 2) | strict | not yet assessed |
| `unpinned-images` | `unpinned-images` | 3 / B | strict | not yet assessed |
| `unpinned-tools` | `unpinned-tools` | 3 / D | strict | not yet assessed |
| `unpinned-uses` | `unpinned-uses` | 3 / B | strict | not yet assessed |
| `unredacted-secrets` | `unredacted-secrets` | 3 / A | strict | full on the synthetic cases; 0 findings on the corpus |
| `unsound-condition` | `unsound-condition` | 3 / C | default | not yet assessed |
| `unsound-contains` | `unsound-contains` | 3 / A | default | full on the synthetic cases; 0 findings on the corpus |
| `unsound-ternary` | `unsound-ternary` | 3 / C | default | not yet assessed |
| `use-trusted-publishing` | `use-trusted-publishing` | 3 / D | strict | not yet assessed |

See [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features) for the
criteria a rule must meet before it is added.

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
