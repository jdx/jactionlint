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
| `excessive-permissions` | `excessive-permissions` | 3 / B | strict | not yet assessed |
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

## Beyond zizmor

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
