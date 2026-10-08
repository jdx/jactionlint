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
| `bot-conditions` | `bot-conditions` | 3 / C | strict | partial: `github.actor`, `github.triggering_actor`, `github.actor_id` and `github.event.sender.*` compared with a bot (`==`, `contains()`, `startsWith()`, `endsWith()`) in job and step `if:`. `!=` and negated tests are not reported, as in zizmor. Measured, see [batch C](#batch-c-measurements). `-fix=unsafe` only for workflows with `pull_request`/`pull_request_target` events. Bot names without `[bot]` are known by ID and prefix only. No composite actions yet |
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
| `impostor-commit` | `impostor-commit` | 3 / G | online | not yet assessed |
| `insecure-commands` | `insecure-commands` | 3 / A | default | not yet assessed |
| `insecure-url-scheme` | `insecure-url-scheme` (where applicable to dependabot.yml) | 3 / E | strict | not yet assessed |
| `known-vulnerable-actions` | `known-vulnerable-actions` | 3 / G | online | not yet assessed |
| `misfeature` | `misfeature`, `misfeature-custom-shell` | 3 / C | strict, all | partial: `pip-install` of setup-python and `shell: cmd` (`misfeature`, 18 of 18 zizmor findings of the corpus). Shells that are not well known are `misfeature-custom-shell` (`all`, info), which zizmor reports in the auditor persona only and which is not compared. No composite actions yet |
| `obfuscation` | `obfuscation` | 3 / C | strict | partial: redundant segments at `uses:`, constant expressions and `format()` of literals outside `if:`, `fromJSON(toJSON(x))` and computed indices. All zizmor findings of the corpus are reported (see [batch C](#batch-c-measurements)). Constants in `if:` are `constant-condition`; `fromJSON(toJSON(context))` is deliberately not reported; the `uses:` fix is unsafe. No composite actions yet |
| `overprovisioned-secrets` | `overprovisioned-secrets` | 3 / A | strict | not yet assessed |
| `ref-confusion` | `ref-confusion` | 3 / G | online | not yet assessed |
| `ref-version-mismatch` | `ref-version-mismatch` | 3 / G | online | not yet assessed |
| `secrets-inherit` | `secrets-inherit` | 3 / A | default | not yet assessed |
| `secrets-outside-env` | `secrets-outside-env` | 3 / A | strict | not yet assessed |
| `self-hosted-runner` | `self-hosted-runner` | 3 / A | all (info) | not yet assessed |
| `self-repository` | `self-repository` | 3 / B | strict | not yet assessed |
| `stale-action-refs` | `stale-action-refs` | 3 / G | online | not yet assessed |
| `superfluous-actions` | `superfluous-actions` | 3 / D | strict | not yet assessed |
| `template-injection` | `template-injection` (default), `template-injection-expansion` (strict), `template-injection-trusted` (all) | 3 / C | default, strict, all | partial: attacker controlled contexts, objects holding them and env variables set from them in `run:`, github-script and the code inputs of well-known actions, every expression of a script (default). Every other expansion is `template-injection-expansion` (free text) or `template-injection-trusted` (values like `github.repository`), which together are zizmor's pedantic persona. See [batch C](#batch-c-measurements) for the numbers. Not covered: `action.yml`, knowledge about the outputs of popular actions, severity by trigger (the level is per rule ID). `-fix` moves a simple reference into `env:` for bash and sh |
| `typosquat-uses` | `typosquat-uses` | 3 / A | strict | not yet assessed |
| `undocumented-permissions` | `undocumented-permissions` | 3 / B (needs YAML comments, Phase 2) | strict | not yet assessed |
| `unpinned-images` | `unpinned-images` | 3 / B | strict | not yet assessed |
| `unpinned-tools` | `unpinned-tools` | 3 / D | strict | not yet assessed |
| `unpinned-uses` | `unpinned-uses` | 3 / B | strict | not yet assessed |
| `unredacted-secrets` | `unredacted-secrets` | 3 / A | strict | not yet assessed |
| `unsound-condition` | `if-always-true` (existing) | 3 / C | default | full for workflows: `if-always-true` reports every `if:` with characters around `${{ }}`, a block scalar's trailing newline included. Matches all 5 zizmor findings of the corpus. No new rule. Composite action steps are not checked yet |
| `unsound-contains` | `unsound-contains` | 3 / A | default | not yet assessed |
| `unsound-ternary` | `unsound-ternary` | 3 / C | default | not yet assessed |
| `use-trusted-publishing` | `use-trusted-publishing` | 3 / D | strict | not yet assessed |

See [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features) for the
criteria a rule must meet before it is added.

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
