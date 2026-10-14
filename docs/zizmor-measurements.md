# zizmor comparison measurements

This page is the dated ledger of the comparison between jactionlint and zizmor, for maintainers who decide whether a rule is ready for the default profile. It records the corpus, the commands, the figures and the cause of every difference, and it marks each figure that the repository cannot reproduce. Users who move from zizmor read [jactionlint and zizmor](./zizmor-parity.md) instead.

## Record of the runs

The page holds the figures of two runs. The first ran on 2026-10-13 for this page and is reproducible with the repository and a local corpus. The second is the 1,109-repository run: its figures were committed between 2026-10-09 and 2026-10-12, and neither its corpus nor the jactionlint commit it ran with is recorded.

| Item | 24-repository run | 1,109-repository run |
| --- | --- | --- |
| Date | 2026-10-13 | Committed 2026-10-09 to 2026-10-12 (commits below) |
| jactionlint | main at commit `2baf2cb7` (`v2.1.1-0.20261013205457-2baf2cb76315`) | Not recorded: the figures say only "main" |
| zizmor | 1.30.1, the version that `scripts/zizmor-diff` runs and that `scripts/zizmor-diff/mapping.json` names | The same version |
| Corpus | The 24 entries of `scripts/zizmor-diff/corpus.txt`: 23 `~/src/*-jactionlint` clones and `~/src/mise`. 23 existed on the machine of the run; `omapac-jactionlint` did not | 1,109 local clones of public repositories. The list, the commits and the clone date are not in the repository |
| Matching | `--line-tolerance 3` | `--line-tolerance 3` |
| Provenance | Reproducible: run the command below on the same clones, left clean as described next | Not reproducible from the repository |

`~/src/mise` is a working clone. Its ignored directory `.claude/worktrees` holds other checkouts of the repository, and jactionlint checks the composite actions in any directory of a repository, so a run on the clone adds one finding of `require-expression-wrapping` per worktree that zizmor, which honors `.gitignore`, does not report. The jactionlint figures below are for a clone of the same commit without that directory; the zizmor figures do not depend on it.

The harness runs jactionlint with the profile of `scripts/zizmor-diff/pedantic.yaml` (`profile: pedantic`) instead of each repository's own config file, and zizmor with `--offline --persona pedantic --format sarif`. The tolerance defaults to 0 lines in the harness and `mise run zizmor-diff` passes none, so the figures on this page are not what the mise task prints.

The command of the 24-repository run:

```sh
go build -o jactionlint ./cmd/jactionlint
go run ./scripts/zizmor-diff \
    --jactionlint ./jactionlint \
    --jactionlint-config scripts/zizmor-diff/pedantic.yaml \
    --line-tolerance 3 \
    --markdown report.md --json report.json
```

To build another corpus, clone repositories into separate directories and list them in a corpus file (`--corpus-file FILE`: one directory or `label=directory` per line, `~` expands, `#` starts a comment) or pass them with `--repos DIR`. A directory that does not exist is reported as skipped. The flags are in [the harness README](https://github.com/jdx/jactionlint/blob/main/scripts/zizmor-diff/README.md), and `go test ./scripts/zizmor-diff` needs neither zizmor nor the network.

Commits that first recorded the figures of the 1,109-repository run:

| Figures | Commit | Date |
| --- | --- | --- |
| Online comparison table, request cost | `714ba461` (#60) | 2026-10-09 |
| Composite action comparison | `a0f28439` | 2026-10-09 |
| Template injection review | `70e11b93` | 2026-10-09 |
| Agentic actions and downloads reviews, ignore comments, fix corpus | `50dfb7fb` | 2026-10-10 |
| Totals and per-audit table (112,559 findings) | `140b8c35` (#157) | 2026-10-12 |
| Cache poisoning | `c7254590` (#160) | 2026-10-12 |
| Permissions | `016f5888` (#176) | 2026-10-12 |
| Misses by cause, totals without tolerance | `b1ac9f23` (#185) | 2026-10-12 |

## Differential harness

`mise run zizmor-diff` builds jactionlint and compares it with zizmor on the repositories of the corpus file. A zizmor finding counts as also reported when jactionlint reports a rule that `scripts/zizmor-diff/mapping.json` maps to the audit, in the same file, within the line tolerance. Workflow files are compared, and so is `.github/dependabot.yml` for the audits of the mapping: the other findings of zizmor outside workflows, such as those in an `action.yml`, are counted as out of scope. A rule without an entry in the mapping is always "only jactionlint" in the report.

The online rules are compared separately with `--online` (jactionlint runs with `--online`, zizmor without `--offline`; the environment needs `GITHUB_TOKEN` or `GH_TOKEN`). See [online checks measured](#online-checks-measured).

### The 24-repository run

zizmor reports 998 findings on the 23 repositories that exist, and jactionlint also reports 955 of them (95.7%); 43 are missed. The audits that are not listed have no finding on this corpus.

| zizmor audit | zizmor | also reported by jactionlint | missed |
| --- | --: | --: | --: |
| `anonymous-definition` | 323 | 323 | 0 |
| `template-injection` | 172 | 172 | 0 |
| `undocumented-permissions` | 166 | 165 | 1 |
| `excessive-permissions` | 163 | 128 | 35 |
| `concurrency-limits` | 74 | 74 | 0 |
| `artipacked` | 64 | 57 | 7 |
| `self-repository` | 17 | 17 | 0 |
| `cache-poisoning` | 10 | 10 | 0 |
| `dependabot-cooldown` | 4 | 4 | 0 |
| `superfluous-actions` | 2 | 2 | 0 |
| `unpinned-images`, `unpinned-uses`, `use-trusted-publishing` | 1 each | 1 each | 0 |
| Total | 998 | 955 | 43 |

All 43 misses are deliberate differences, sorted per file for this page: 35 are `excessive-permissions` findings in a reusable-only workflow (`on: workflow_call` alone); 7 are `artipacked` findings in workflows that contain `git push` or `git commit` (the file was searched, not each step); 1 is an `undocumented-permissions` scope with a comment on the line above. The causes are explained in [misses by cause](#misses-by-cause).

jactionlint reports 3,540 findings on the same repositories. 2,478 of them, from 29 rules, have no matching zizmor finding. The largest groups are `require-shell` (914), `mutable-runner-label` (432), `require-expression-wrapping` (377), `secrets-outside-env` (337) and `missing-timeout` (216): rules of the `pedantic` profile or rules that zizmor has no audit for, and `secrets-outside-env`, which zizmor reports only for its auditor persona and the harness does not run.

### The 1,109-repository run

zizmor reports 112,559 findings, and jactionlint also reports 104,421 of them (92.8%); 8,138 are missed. Without the line tolerance 97,717 match (86.8%); the difference is mostly `artipacked`, `template-injection` and `dangerous-triggers`, where the tools point at different lines of the same finding. The audits with misses, at a tolerance of 3 lines:

| zizmor audit | zizmor | also reported by jactionlint | missed |
| --- | --: | --: | --: |
| `excessive-permissions` | 12,856 | 12,286 | 570 |
| `undocumented-permissions` | 9,244 | 6,348 | 2,896 |
| `concurrency-limits` | 5,693 | 4,253 | 1,440 |
| `anonymous-definition` | 10,011 | 9,048 | 963 |
| `unpinned-uses` | 21,203 | 20,447 | 756 |
| `artipacked` | 10,703 | 10,004 | 699 |
| `cache-poisoning` | 1,038 | 760 | 278 |
| `obfuscation` | 291 | 223 | 68 |
| `unpinned-images` | 609 | 576 | 33 |
| `archived-uses` | 109 | 0 | 109 |
| `superfluous-actions` | 1,036 | 962 | 74 |
| `dangerous-triggers` | 777 | 705 | 72 |
| `dependabot-cooldown` | 892 | 848 | 44 |
| `adhoc-packages` | 1,071 | 1,041 | 30 |
| `template-injection` | 24,181 | 24,142 | 39 |
| `use-trusted-publishing` | 112 | 82 | 30 |
| `github-env` | 63 | 31 | 32 |
| `bot-conditions` | 38 | 34 | 4 |
| `self-repository` | 10,729 | 10,728 | 1 |

The audits `github-app`, `misfeature`, `overprovisioned-secrets`, `secrets-inherit`, `unpinned-tools`, `unsound-condition`, `unsound-contains` and `unsound-ternary` are recorded with findings and no misses. The record contradicts itself on `overprovisioned-secrets` and `unsound-contains`: one note says that they have no finding on the corpus, another lists them with findings and no misses. Their count on this corpus is unresolved. `archived-uses` is 0 of 109 because it is an online rule: zizmor reports it with `--offline` and jactionlint does not without `--online`.

### What the comparison does not say

- The misses of `concurrency-limits`, `anonymous-definition`, `unpinned-uses`, `artipacked`, `obfuscation`, `unpinned-images`, `archived-uses`, `superfluous-actions`, `dangerous-triggers`, `dependabot-cooldown`, `adhoc-packages`, `template-injection`, `use-trusted-publishing`, `github-env`, `bot-conditions` and `self-repository` were grouped by cause, with at least 15 of each cause read (a smaller group in full). The grouping script is not in the repository. [Misses by cause](#misses-by-cause) holds the counts.
- The misses of `excessive-permissions` and `undocumented-permissions` are accounted for in [permissions against zizmor](#permissions-against-zizmor). Those of `cache-poisoning` are partial by design, see [cache poisoning against zizmor](#cache-poisoning-against-zizmor).
- `cache-poisoning` has two tallies that differ on purpose: the harness row (1,038 findings, 278 missed) compares the whole pedantic run with a tolerance of 3 lines, while the investigation (1,041 findings, 288 not reported) runs zizmor with its auditor persona, enables only this rule in jactionlint and matches on file and line.
- The online rules are not part of these totals.
- The surplus of jactionlint is not reviewed as a whole. Two groups were: 2 `artipacked` findings (checkouts that zizmor reports at the first line of the step) and 14 `unpinned-images` findings (10 that zizmor reports at the `container:` line of the same job, and 4 of the option `pedantic` for a computed matrix image, which zizmor does not report).
- Rules of jactionlint that have no zizmor audit are "only jactionlint" in the report: `require-shell`, `require-expression-wrapping`, `mutable-runner-label`, `missing-timeout`, `pipeline-without-pipefail`, `unknown-runner-label`, `checkout-static-credentials`, `workflow-secret-scope`, `unverified-download`, `shellcheck`, `max-run-lines`, `continue-on-error`, `concurrency-cancels-release`, `unused-job-output` and `unused-needs`. `secrets-outside-env` and `self-hosted-runner` are audits of the auditor persona of zizmor and appear in the same list because the harness runs zizmor with `--persona pedantic`. Mapped rules also report surplus findings (`undocumented-permissions`, `concurrency-limits`, `template-injection`, `cache-poisoning`, `artipacked`, `bot-conditions`, `unpinned-tools`).

Known causes of differences between the tools, on the 1,109-repository run:

- `undocumented-permissions`: zizmor accepts only a comment at the end of the line, jactionlint accepts one above the scope too.
- `cache-poisoning`: zizmor treats a workflow with `push: {tags-ignore: ["*"]}` as a release workflow, which says the opposite, so jactionlint does not. jactionlint also reports a job that runs `gh release upload` or pushes an image and restores a cache (`jdx/mise-action` with `cache_save: false`, which only stops saving, and `docker/build-push-action` with `cache-from: type=gha`), where zizmor looks at triggers and not at jobs.
- `dangerous-triggers`: jactionlint also reports `issue_comment` and reports at the trigger where zizmor reports at `on:`. A workflow that cannot write and reads no secret is reported only with the option `pedantic`; the effect of that on the comparison was not measured, and the pedantic profile of the harness turns the option on.

## Misses by cause

Every row is a cause of misses in the 1,109-repository run, at a tolerance of 3 lines. The kinds are: a position (both tools report the finding, more than 3 lines apart), a difference on purpose (jactionlint decides not to report, see [where a rule differs on purpose](./zizmor-parity.md#where-a-rule-differs-on-purpose)), a zizmor finding that is not one, and a gap (jactionlint does not report what zizmor reports, and no reason is recorded). The counts of each audit add up to its row in the table above.

| Audit | Misses | Cause | Kind |
| --- | --: | --- | --- |
| `excessive-permissions` | 417 | a job of a reusable-only workflow without `permissions:` | difference on purpose |
| `excessive-permissions` | 153 | a job in a generated file | difference on purpose |
| `undocumented-permissions` | 2,678 | a scope in a generated file | difference on purpose |
| `undocumented-permissions` | 184 | a comment on the line above the scope | difference on purpose |
| `undocumented-permissions` | 34 | a scope granted with `none` | difference on purpose |
| `concurrency-limits` | 1,363 | a workflow that only a `schedule` or a `workflow_dispatch` starts | difference on purpose |
| `concurrency-limits` | 37 | a generated file | difference on purpose |
| `concurrency-limits` | 20 | the job of `copilot-setup-steps.yml` | zizmor finding that is not one |
| `concurrency-limits` | 15 | recorded as a `concurrency:` without `cancel-in-progress: true` | unverified: see below |
| `concurrency-limits` | 4 | a workflow in which every job has its own `concurrency:` | difference on purpose |
| `concurrency-limits` | 1 | the `on:` of a workflow that is indented as a whole is not found, so jactionlint reports at line 1 | position |
| `anonymous-definition` | 941 | a generated file | difference on purpose |
| `anonymous-definition` | 22 | the job of `copilot-setup-steps.yml` | zizmor finding that is not one |
| `unpinned-uses` | 755 | a generated file | difference on purpose |
| `unpinned-uses` | 1 | a step that never runs, recorded as `if: false` (see below) | zizmor finding that is not one |
| `artipacked` | 678 | a checkout that a later step pushes with: a `git push` command or an action that pushes | difference on purpose |
| `artipacked` | 8 | `actions/checkout@v1`, which has no `persist-credentials` input | zizmor finding that is not one |
| `artipacked` | 2 | a `persist-credentials` that is an expression | difference on purpose |
| `artipacked` | 11 | zizmor reports at the first line of the step, jactionlint at its `uses:` (more than 3 lines further down with a long `if:` or comments, or at the alias of a step written as a YAML anchor) | position |
| `cache-poisoning` | 278 | partial by design | see [cache poisoning against zizmor](#cache-poisoning-against-zizmor) |
| `obfuscation` | 53 | an index that is a value of the matrix (`secrets[matrix.provider.env_key]`, `fromJSON(...)[matrix.tool]`) | difference on purpose |
| `obfuscation` | 14 | a constant `if:` (`if: ${{ false }}`), reported by `constant-condition` and `if-always-true` | difference on purpose |
| `obfuscation` | 1 | a trailing slash in the path of a remote action (`owner/repo/@sha`) | difference on purpose |
| `unpinned-images` | 31 | an image from the matrix: zizmor reports at the `container:`, jactionlint at the matrix value | position |
| `unpinned-images` | 2 | a generated file | difference on purpose |
| `archived-uses` | 109 | an online rule: reported only with `--online` | online rule |
| `superfluous-actions` | 72 | a job whose runner may be self-hosted: a `runs-on:` that is an expression the rule cannot resolve counts as one | difference on purpose |
| `superfluous-actions` | 2 | a literal `self-hosted` label | difference on purpose |
| `dangerous-triggers` | 72 | the position of the `on:` block | position |
| `dependabot-cooldown` | 40 | an update that proposes nothing: `open-pull-requests-limit: 0` or an `ignore` of every dependency | zizmor finding that is not one |
| `dependabot-cooldown` | 4 | zizmor reports at the `package-ecosystem` key, jactionlint at the first key of the update | position |
| `adhoc-packages` | 18 | the value of `--prefix` (`npm install --prefix ./assets`) is a directory, not a package | zizmor finding that is not one |
| `adhoc-packages` | 12 | an install from a local path or tarball | difference on purpose |
| `template-injection` | 15 | the input of an action that is not code: the payload of a Slack action, the body of an issue (`8398a7/action-slack`, `slackapi/slack-github-action`, `imjohnbo/issue-bot`) | difference on purpose |
| `template-injection` | 11 | zizmor reports at the `run:` line, jactionlint at the expansion | position |
| `template-injection` | 10 | a step or a job that never runs, recorded as `if: false` (see below) | zizmor finding that is not one |
| `template-injection` | 3 | expansions that jactionlint judges to be safe (`toJson(github.event.pull_request.requested_reviewers[*].login)`, a constant string in `run:`) | difference on purpose |
| `use-trusted-publishing` | 10 | a publish to GitHub Packages or MyGet | difference on purpose |
| `use-trusted-publishing` | 8 | `pnpm dlx pkg-pr-new publish`, which uploads a preview to pkg.pr.new and publishes to no registry | zizmor finding that is not one |
| `use-trusted-publishing` | 3 | a reusable workflow without `permissions:` (the caller may grant `id-token: write`) | difference on purpose |
| `use-trusted-publishing` | 2 | the Visual Studio marketplace (`vsce publish`) | difference on purpose |
| `use-trusted-publishing` | 2 | a script that is not a publish command (`pnpm run publish`, `bun upload-npm -- publish`) | difference on purpose |
| `use-trusted-publishing` | 2 | `bun x electron-builder ... --publish never`, which publishes nothing | zizmor finding that is not one |
| `use-trusted-publishing` | 2 | positions | position |
| `use-trusted-publishing` | 1 | a local registry | difference on purpose |
| `github-env` | 17 | zizmor reports at the `run:` of the script, jactionlint at the line that writes | position |
| `github-env` | 15 | a write of a value that is not attacker controlled: a runner path (`${{ runner.temp }}`, `$RUNNER_TEMP`), `date`, `$PWD`, `realpath`, a number that the script checked against `^[0-9]+$`, a variable already in the environment from a secret | difference on purpose |
| `bot-conditions` | 3 | a conjunction with a check of the author of the pull request (zizmor#1914) | zizmor finding that is not one |
| `bot-conditions` | 1 | a negated test: zizmor reports it, jactionlint does not | gap |
| `self-repository` | 1 | a step that never runs, recorded as `if: false` (see below) | zizmor finding that is not one |

The 15 `concurrency-limits` misses recorded as a `concurrency:` without `cancel-in-progress: true` have an unverified cause. A minimal example does not reproduce the label: zizmor reports only a missing `concurrency:` and the bare string form (`concurrency: group-name`), the same shapes jactionlint reports, and stays silent for `cancel-in-progress: false`, a group without `cancel-in-progress` and a string value.

Two causes cut across the audits:

- **Generated files.** A file whose header comment says that a tool wrote it (`# This file was automatically generated by gh-aw`, `# generated by praktika`, `# Generated by projen`) is skipped by the rules of the `policy` and `style` groups under the default `generated-files: skip-policy`. The corpus has 200 such files in 42 repositories (96 by gh-aw, 28 by praktika, 25 "automatically generated", 10 by dist, 5 by modulesync and a few others). They hold 941 of the 963 misses of `anonymous-definition`, 755 of the 756 of `unpinned-uses`, 37 of the 1,440 of `concurrency-limits` and 2 of the 33 of `unpinned-images`. With `generated-files: report` the same run misses 22, 1, 1,403 and 31.
- **Steps that never run.** Findings in a job or a step whose `if:` is constant false (`false`, `${{ !true }}`, `false && github.event_name == 'push'`) are dropped for every rule outside the correctness group (zizmor#2059). That accounts for 12 of the misses: 10 `template-injection`, 1 `unpinned-uses` and 1 `self-repository`. The record says `if: false`, which cannot be reproduced: zizmor 1.30.1 itself skips a step or a job whose `if:` is `false` or `${{ false }}` (since zizmor 1.26.0), and in a minimal workflow it reports only the `false && ...` shape that jactionlint drops. The recorded shape of these 12 is unverified.

## Cache poisoning against zizmor

This section is from the 1,109-repository run, recorded with commit `c7254590` (#160). The corpus has 1,109 repositories. zizmor with `--persona auditor --offline` reports 1,041 `cache-poisoning` findings on 237 of them. Both tools ran per repository, jactionlint with only this rule enabled, and a finding is shared when both report the same file and line (the `uses:` of the step). The rule as of the parent of that commit reported 411 of the 1,041 findings and 553 findings that zizmor does not have; the 288 that remain after the change are all zizmor reporting something that jactionlint has a reason not to.

| | Findings of zizmor | Share |
| --- | --: | --: |
| reported by the rule before the change | 411 | 39% |
| new by default | 39 | 4% |
| new with the option `pedantic` | 303 | 29% |
| not reported | 288 | 28% |

Each finding of zizmor that was not reported was classified by finding out where the rule stopped (a debug trace of the rule) and by reading the action. The classes are **(a)** real and cheap to cover, **(b)** real but needing more analysis, **(c)** a finding of zizmor that is not one. The classification script and the traces are not in the repository.

- **(a), reported by default after the change** (39 findings of zizmor and 36 more of jactionlint, 75 in all):
  - a job that is a check by its permissions, in a workflow on pushed tags, but uploads an artifact for a job that can publish (`needs`, directly or through another job; a job with an `environment`, a publishing command or action, or a write permission for `contents`, `packages`, `attestations`, `pages`, `deployments` or `id-token`). The job builds what the other job publishes;
  - `docker/build-push-action` with `push` set to an expression (`${{ inputs.push }}`, `${{ github.event_name != 'pull_request' }}`) or with `outputs` that push (`type=registry`, `push=true`) is a publisher, where only the literal `push: true` was;
  - the publishers that zizmor knows and the table lacked (`jreleaser/release-action`, `release-drafter/release-drafter`, `aws-actions/aws-cloudformation-github-deploy`, `azure/aci-deploy`, `azure/container-apps-deploy-action`, `azure/sql-action`, `google-github-actions/deploy-cloud-functions`);
  - `PyO3/maturin-action` with `sccache` (or `cache: sccache`) and `Mozilla-Actions/sccache-action` with `SCCACHE_GHA_ENABLED`;
  - the conditions: a tag filter of `preview-*` gives the run the tag `preview-1.0.0` instead of always `v1.0.0`, every positive pattern is a run, and a condition that is open only in a part that says nothing about the trigger (`github.repository == 'o/r' && !startsWith(github.ref_name, 'go/')`, `github.event_name == 'push' && !contains(needs.release.outputs.version, '-')`) does not leave the run out. That recovered release workflows of `apache/fory`, `getsentry/MobileBuildMCP` and others.
- **(a) with the option `pedantic`**: 303 findings of zizmor, 114 of them a workflow on pushed tags with read-only permissions that publishes nothing itself and 189 a `branches:` filter that names a release branch next to others (`[main, 'release/**']`). They are the heuristics of zizmor, and they are not cheap in false positives (see below), so they sit behind the option.
- **(b)**: none that could be separated from (c). What is left is described by the next class, and none could be decided without running the workflow.
- **(c), not reported on purpose** (288):
  - 185 `actions/setup-node` v5 and later in a repository whose `package.json` names no package manager (or has none): the action restores a cache only when it does. zizmor does not read the file;
  - 64 `actions/setup-node` with a package manager that this version does not cache (v4, or pnpm, yarn or bun with v6 and later, which cache npm only). The code of the action was read for v6.3.0, v7.0.0 and v7.1.0;
  - 16 steps with a job or step condition that keeps the run of the release away (`if: github.ref == 'refs/heads/main'`, a manual dispatch, a schedule). zizmor does not evaluate `if:`;
  - 14 others: a step with a condition on the branch that cannot be decided (the release branch is not known), and `docker/build-push-action` without `push` (zizmor counts the default as true; the default is false);
  - 6 `actions/setup-go` before v4, which does not cache, and 3 `Mozilla-Actions/sccache-action` without `SCCACHE_GHA_ENABLED`, which only installs the binary.

### False-positive review

The review of commit `c7254590` covers the 75 findings that are new by default and a sample of the option `pedantic`. The review notes are not in the repository.

All 75 new findings by default were read one by one, with the workflow open, in 34 repositories. 73 are true positives (a release workflow, a job that pushes an image or publishes a release, a build job whose artifact a publishing job downloads) and 2 are doubtful: both are the build job of one gate workflow, where the manual dispatch pushes a test image tagged with the commit that the later jobs of the matrix use. That is 0 clear false positives and 2 doubtful of 75.

The option `pedantic` was sampled with 36 findings of 70 repositories (14 of a tag, 22 of a release branch, one per file, drawn with a fixed seed). 34 are jobs that publish nothing (a test, a lint, a smoke test, a documentation build in a CI workflow); the 2 others are a wheel build and a smoke test of a release workflow. That is a false-positive rate of about 90%, which is why the findings are not in the default.

<a id="effect-on-the-findings-that-were-reported-before"></a>
### Effect on the docker/setup-buildx-action findings

`docker/setup-buildx-action` is reported only with `version` by default, as zizmor does: without it the action uses the buildx of the runner and does not download one, so nothing is restored from the cache (`src/main.ts` of the action: `download` runs when buildx is not available or `version` is set). On the run of commit `c7254590` that removes 242 findings from the 964 of the same run (174 on `ubuntu-*` or `windows-*` runners, 63 on a matrix runner, 1 self-hosted, 4 in composite actions). The option `pedantic` brings them back. A self-hosted runner without buildx is the one case that this does not cover.

## Permissions against zizmor

`excessive-permissions` and `undocumented-permissions` had the most misses in the comparison with zizmor. This section records commit `016f5888` (#176), which reports the pedantic persona's permission findings. Each miss of the 1,109-repository run was sorted by cause with a script that reads the workflow (not in the repository), and examples of each cause were read with both messages and the workflow open. The counts are from the harness run with the `pedantic` profile and zizmor `--persona pedantic`.

### Excessive permissions

zizmor reports a workflow without a top-level `permissions:` once at the start of the document, whatever its jobs set (the pedantic persona always, the regular persona for some workflows with more than one job), and once more for every job without permissions. At the parent of the commit jactionlint reported only the jobs (`missing-permissions`): the workflow-level finding is the option `require-workflow-permissions` of `excessive-permissions`, which was off, and it reported at line 1, which a license header of comments moves away from the first key (a byte order mark does the same). The commit turns the option on under the `pedantic` profile and reports at the first key of the file. Composite actions and workflows without `on:` or without jobs (a `workflow-syntax` finding of their own) are not reported. That accounts for 4,048 of the 4,618 misses. The 570 that are left, from 12,856 findings of zizmor:

| Cause | Findings | Kind |
| --- | --: | --- |
| a job of a reusable-only workflow (`on: workflow_call` alone) without `permissions:` | 417 | deliberate |
| a job in a generated file (`missing-permissions` is a policy rule, which `generated-files: skip-policy` skips) | 153 | deliberate |

A reusable-only workflow is not a place to set the permissions of a job: the callee gets at most what its caller grants, and a value that is more than that makes GitHub reject the call, so no value written in the callee is right for every caller. `missing-permissions` does not report it (the workflow-level finding above still is). zizmor reports the jobs of such a workflow. `generated-files: report` brings the 153 back.

### Undocumented permissions

zizmor reports one finding for a `permissions:` block (at its first scope without a comment) when a scope other than `contents: read` has no comment at the end of its line, and it does that for read scopes too. At the parent of the commit jactionlint asked for a comment on write scopes only unless `include-read` was on, which accounted for 1,170 of the 4,066 misses. The commit turns `include-read` on under the `pedantic` profile, where this rule runs. The 2,896 that are left, from 9,244 findings of zizmor:

| Cause | Findings | Kind |
| --- | --: | --- |
| a scope in a generated file (`generated-files: skip-policy`; mostly `*.lock.yml` of gh-aw and `cargo-dist` workflows) | 2,678 | deliberate |
| a comment on the line above the scope, which zizmor does not count | 184 | deliberate |
| a scope granted with `none` (`contents: none` is the common one) | 34 | deliberate |

A comment above a scope is how most people write it, and a reviewer reads it as the explanation, so it counts. `none` grants nothing: asking why a scope is not granted is not what the audit is for. `generated-files: report` brings the first group back.

### Findings only jactionlint reports

With the commit, the jactionlint-only findings of the three rules `excessive-permissions`, `missing-permissions` and `undocumented-permissions` go from 308 to 546 on the same run: `excessive-permissions` 39 to 94, `missing-permissions` 65 to 65, `undocumented-permissions` 204 to 387. All 238 new ones were read by cause:

- 181 are `undocumented-permissions` for the second and later scopes of a block that zizmor reports once. zizmor reports the block at its first scope without a comment; jactionlint points at each scope without a comment, so a fix can be made scope by scope.
- 55 are `excessive-permissions` for a workflow that zizmor does not audit:
  - 31 findings in 3 repositories where zizmor stops with `fatal: no audit was performed` because it cannot load one file (`failed to load file://./action.yaml as action`), so it reports nothing for the repository (the record also names 4 repositories of the corpus where this happens; the two counts are not reconciled);
  - 12 in `openclaw/openclaw`, whose `zizmor.yml` disables the audit;
  - 9 in files that start with a byte order mark, which zizmor does not report (a copy of `Terrain3D/.github/workflows/linux.yml` without the mark is reported);
  - 2 in `nrwl/nx`, where zizmor never starts because mise cannot install the tools of the repository;
  - 1 in a workflow with a `workflow-syntax` finding.
- 2 are `undocumented-permissions` in `nrwl/nx`, for the same reason.

jactionlint keeps two behaviors that differ from zizmor: it reports one finding per scope, not one per block, and it does not report a `none` scope.

## Online checks measured

The online rules are compared with `go run ./scripts/zizmor-diff --online` (zizmor `--persona pedantic`, with a token) and by running both tools on every workflow file of the corpus. A finding is shared when both tools report the same audit in the same file within two lines. The figures are from the 1,109-repository run, recorded with commit `714ba461` (#60); the harness `--online` mode is in the repository, the corpus and the run are not.

| Audit | zizmor | Shared | zizmor only | jactionlint only |
| --- | --: | --: | --: | --: |
| `impostor-commit` | 2 | 2 | 0 | 1 |
| `known-vulnerable-actions` | 0 | 0 | 0 | 0 |
| `ref-confusion` | 0 | 0 | 0 | 0 |
| `stale-action-refs` | 7 | 6 | 1 | 0 |
| `archived-uses` (zizmor with `--offline`) | 109 | 109 | 0 | 0 |
| `ref-version-mismatch`, tag comment does not match | 182 | 182 | 0 | 0 |
| `ref-version-mismatch`, comment names a branch | 2 | 1 | 1 | 0 |
| `ref-version-mismatch`, missing or version-less comment | 42 | 0 | 42 | 0 |

Older checkouts of the same repositories, which hold older workflows and so more vulnerable actions, add `known-vulnerable-actions` and `stale-action-refs` findings that jactionlint reports as well, except for the case below.

What the differences are:

- **`impostor-commit`, jactionlint only.** A workflow with a `# zizmor: ignore[impostor-commit]` comment, which only zizmor reads. A true finding.
- **`stale-action-refs`, zizmor only.** `taiki-e/install-action` has more than 1000 tags, jactionlint reads 1000 per repository, and says nothing about a commit it did not find in a truncated list.
- **`ref-version-mismatch`, missing or version-less comment.** zizmor reports a hash-pinned action without a version comment (a pedantic finding). jactionlint does not: a missing comment is not a mismatch. This is a gap.
- **`ref-version-mismatch`, comment names a branch.** zizmor also compares a comment such as `# stable` with the branch of that name. jactionlint reads only comments that start with a version.
- **`archived-uses`.** zizmor reports 109 findings in 42 repositories with `--offline` (`actions-rs/toolchain` 22, `actions/upload-release-asset` 18, `8398a7/action-slack` 17, `actions/create-release` 10, `contributor-assistant/github-action` 9, `actions-rs/cargo` 8, `marvinpinto/action-automatic-releases` 7 and a few others). jactionlint with `--online` and a token reports all 109, on the same lines within 3. Without `--online` it reports none.
- **No finding for either tool** for `ref-confusion`, and for `known-vulnerable-actions` on current workflows: those rules are exercised by fixtures, not by a measurement against zizmor.

Gaps of the online rules that the corpus does not show, so they are not measured:

- `impostor-commit` treats a commit as legitimate when a tag points at it or a branch contains it. A commit that is only in the history of a tag (a deleted release branch) is reported.
- Without a token jactionlint cannot use GraphQL, compares at most 100 branches, and says nothing for a larger repository where no branch has the commit. It compares branches in parallel batches of 6 requests (`rule_impostor_commit.go`).
- Reusable workflows are checked like actions, and the steps of a composite action are checked like workflow steps.

Request cost, from the same recording: for a repository with 173 `uses:` lines the first run makes about 91 requests, a run within the hour none, and a later run only conditional requests (a 304 answer is free of rate limit for authenticated clients).

## Composite actions measured

zizmor audits `action.yml` files with the same audits as workflows, and jactionlint checks the steps with the same rules. The harness does not compare `action.yml` files, so this comparison used another procedure that is not in the repository (commit `a0f28439`, 2026-10-09). It ran on the action files of the 1,109-repository corpus, within 8 lines of tolerance (jactionlint reports a multi-line expression at its first line and zizmor at another).

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
- `cache-poisoning`: a cache restored by an action called from a workflow on pushed tags. The input expression of the caller is evaluated for the release scenarios.
- The `${{ github.token }}` default of an input is valid (context availability of defaults).

No false positive remains in the reviewed sets. `matrix.*` and `needs.*` in a composite action are not reported as undefined, because GitHub does not document that the runner withholds them.

## Other reviews

These reviews were recorded with the commits named in each entry. None of them can be reproduced from the repository.

- **Template injection** (commit `70e11b93`). The `default` rule reported no false positive, and zizmor reported every line the rule reported. The corpus was too clean to say how it compares with zizmor's high-confidence findings. With the option `pedantic` the rule reports every line zizmor does, plus `matrix.*` whose values are literals and `needs.*.result`, values of `matrix.*` from `fromJSON`, `env.*` set from `$GITHUB_ENV`, and `steps.*.outputs.*` of actions that zizmor knows to return safe values (jactionlint does not have that knowledge). These are not false positives of a security finding. The record also lists `github.ref_name` and `github.event.inputs.*` among the extra findings, which is wrong: a minimal example run for this page shows that both tools report them in their default runs (see [template injection](./zizmor-parity.md#template-injection)). The sinks for agent prompts, container options and `docker://` steps reported no false positive on the sampled workflows; the container and service sinks are rare and are covered by golden tests.
- **Agentic actions** (commit `50dfb7fb`). On a sample of public workflows that use agent actions, the findings were all true: agents without a check of the user (no `if:`, `environment:` or permission step) with a token that can write contents, agents that run on a pull request checkout, and allowed tools that give any code (`Bash(npx:*)`, `Bash(bunx:*)`, `Bash(xargs:*)`). The sample is not in the repository.
- **Downloads and credentials** (commit `50dfb7fb`). Every finding of `unverified-download`, `insecure-ssh-keyscan`, `insecure-url-scheme` and `checkout-static-credentials` on the corpora was read and judged true. The findings of `checkout-static-credentials` for a token from a secret are release workflows that push with a personal access token so that the push starts other workflows.
- **Later rules** (commits `ebeeb6f0`, #202, and `c9d89447`, #237, both 2026-10-13). `command-injection` had no finding on a corpus of about 500 repositories (6,900 workflow files), so its false-positive rate is not measured. `actions-lock` was run offline on 82 repositories that have a lock file and found no false positive; the repositories are not listed in the repository.
- **Ignore comments** (commit `50dfb7fb`). On a corpus of workflows that carry `# zizmor: ignore[...]` comments, the audits named most often are `cache-poisoning`, `dangerous-triggers`, `artipacked`, `use-trusted-publishing`, `adhoc-packages`, `impostor-commit` and `template-injection`, and every one of them is a jactionlint rule. Many of the comments are stale (`unused-ignore` reports them, mostly for `cache-poisoning`). Stale means that jactionlint reports nothing there, not necessarily a gap: zizmor reports no `cache-poisoning` finding on the one file checked by hand once the comments are removed. The `impostor-commit` comments are exercised only with `--online`.
- **Fix corpus** (commit `50dfb7fb`). `scripts/fix-corpus` applies the fixes to a copy of real workflows and checks that every YAML file is still valid, that a second `--fix` changes nothing, that the findings of no jactionlint rule go up and that the findings of no zizmor audit go up (zizmor `--persona pedantic --offline`). It runs with `mise run fix-corpus`; see [its README](https://github.com/jdx/jactionlint/blob/main/scripts/fix-corpus/README.md). Whether a fix adds only lines is not among its checks.
- **Unmeasured rules in the default profile.** `unredacted-secrets`, `overprovisioned-secrets` and `typosquat-uses` run in `default` with no recorded measurement of their false-positive rate on a corpus.

## Lessons applied

The issue tracker of zizmor is a list of false positives and false negatives that a rule of the same shape can have. Each row names the zizmor issue, what went wrong there, what jactionlint does and the test that pins it. "Handled" means jactionlint avoids the problem, "Not affected" means the fixture passes without a special case and is kept, and "Open" means the behavior of zizmor is not matched. The tests are in [`pitfalls_test.go`](https://github.com/jdx/jactionlint/blob/main/pitfalls_test.go) except where the last column names another file: four rows (#1481, #1619, #2130, #2210) are covered elsewhere.

| zizmor | What went wrong there | jactionlint | Test |
| --- | --- | --- | --- |
| [#2219](https://github.com/zizmorcore/zizmor/issues/2219) | `github-app` flags `owner` without `repositories` although the token has only organization permissions | Handled: no finding when every `permission-*` input is an organization permission | `TestPitfallGitHubAppOrganizationPermissions` |
| [#1914](https://github.com/zizmorcore/zizmor/issues/1914) | `bot-conditions` flags `github.actor == 'dependabot[bot]'` next to a check of the pull request author | Handled: a conjunction with a check of the author of the pull request is not reported | `TestPitfallBotConditionsNeutralizedByTheAuthor` |
| [#2059](https://github.com/zizmorcore/zizmor/issues/2059) | findings in steps with `if: false` (reported as an `unpinned-tools` false positive) | Handled for every rule outside the correctness group, in one place: findings in a job or a step whose `if:` is constant false are dropped (zizmor skips the literal `false` since 1.26.0) | `TestPitfallStepsWhichNeverRun` |
| [#1098](https://github.com/zizmorcore/zizmor/issues/1098), [#1043](https://github.com/zizmorcore/zizmor/issues/1043) | `artipacked` flags a checkout that a later `git push` needs, and `actions/checkout@v1` | Handled: neither is reported (a push next to an upload of the workspace still is) | `TestPitfallArtipacked` |
| [#2320](https://github.com/zizmorcore/zizmor/issues/2320) | `cache-poisoning` on `astral-sh/setup-uv` v10 with `enable-cache: auto` | Handled: version aware, the version is read from the comment of a pinned commit too. The guard `if: github.event_name != 'release'` on a step is honored | `TestPitfallCachePoisoningSetupUV` |
| [#1848](https://github.com/zizmorcore/zizmor/issues/1848) | `use-trusted-publishing` is silent when `id-token: write` is granted (for provenance or a cloud login), and misses PowerShell | Handled: a long-lived credential next to `id-token: write` is reported, and `pwsh` scripts are searched line by line | `TestPitfallUseTrustedPublishing` |
| [#1479](https://github.com/zizmorcore/zizmor/issues/1479) | `dependabot-cooldown` on OpenTofu | Not a flaw: the options reference of GitHub lists `default-days` for every ecosystem, OpenTofu included, and only `semver-*-days` is limited. A test keeps every ecosystem reported | `TestPitfallDependabotCooldownEcosystems` |
| [#1481](https://github.com/zizmorcore/zizmor/issues/1481) | `copilot-setup-steps.yml` is not an ordinary workflow | Handled for `concurrency-limits` and `anonymous-definition`. `missing-timeout` still applies, because the file accepts `timeout-minutes` | `TestPathsWithoutNamesOrConcurrencyForCopilot` in `rule_concurrency_limits_test.go` |
| [#1619](https://github.com/zizmorcore/zizmor/issues/1619) | `concurrency-limits` skips a caller whose jobs all call reusable workflows | Handled: the caller is reported, because the called workflow cannot hold the limit without deadlocking | the case "only reusable workflow calls are limited by the caller" of `TestConcurrencyLimits` in `rule_concurrency_limits_test.go` |
| [#1865](https://github.com/zizmorcore/zizmor/issues/1865) | `superfluous-actions` on self-hosted runners | Handled: not reported there | `TestPitfallSuperfluousActionsOnSelfHostedRunners`, `...OnMatrixRunners`, `...MayBeSelfHosted` |
| [#2433](https://github.com/zizmorcore/zizmor/issues/2433) and the other `ref-version-mismatch` issues | a second comment after the version, prereleases, sibling tags | Not affected: the version is the first word of the comment | `TestPitfallCommentVersionWithAnotherComment`, `TestPitfallRefVersionMismatchOnline` |
| [#2321](https://github.com/zizmorcore/zizmor/issues/2321) | `ref-version-mismatch` gives a confusing message when a tag and a branch share a name, and GitHub gives the branch priority | Handled for the related case of a hash-looking ref: abbreviated SHAs are names to GitHub and are looked up, full SHAs are read as commits and are not. The test does not cover the message of the issue | `TestPitfallRefConfusionOnHashLookingRefs` |
| [#2130](https://github.com/zizmorcore/zizmor/issues/2130) | `impostor-commit` and annotated tag objects | Not affected: the client dereferences annotated tags, and a SHA that GitHub cannot compare is no verdict, not an impostor | `TestHTTPClientResolveRefDereferencesAnnotatedTags` in `github_http_test.go` |
| [#1673](https://github.com/zizmorcore/zizmor/issues/1673) | `secrets: inherit` under `on.workflow_call` | Open: the documentation and the workflow schema of GitHub show only a mapping there, so the syntax finding stays. Whether GitHub runs such a file is not tested | `TestPitfallSecretsInheritOnWorkflowCall` |
| [#2210](https://github.com/zizmorcore/zizmor/issues/2210), [#1350](https://github.com/zizmorcore/zizmor/issues/1350) | one 503 or 500 answer, or a private repository, makes the online run fail or noisy | Not affected: a lookup that answers 404 or 403 is skipped. Three timeouts or server errors in a row, a DNS or connection failure, a rejected token (401) and an exhausted rate limit stop the remaining lookups of the run. One warning per kind of failure, and `--verbose` lists them | `TestSessionSkipsFailedLookupsAndWarnsOncePerKind` in `online_test.go` |
| [GHSA-f42p-wjw5-97qh](https://github.com/zizmorcore/zizmor/security/advisories/GHSA-f42p-wjw5-97qh) | credentials in the debug log | Not affected: a test runs `--online --debug --verbose` with a token and finds it nowhere | `TestPitfallTokenNeverInTheLogs` |

## Decisions kept

These behaviors are reported or left unreported on purpose.

- `bot-conditions` on `github.event.sender.id` is reported: the account of the last event is not the author of the change, with an id as well as with a login. It is not reported where no pull request event can reach the job, as in a reusable workflow that only a `push` workflow calls.
- `invalid-local-action` never reports a missing local action in a composite action (actionlint issues #25 and #40), because it can be a checkout of a private repository.
- `self-repository` is a regular-persona finding of zizmor that the profile keeps in `pedantic`.
- `concurrency-limits` stays in the `default` profile, where zizmor has it in its pedantic persona. It changes its advice to `cancel-in-progress: false` for workflows that release or deploy, so that it does not contradict `concurrency-cancels-release`; the fixer applies only to workflows of pull requests.
- `workflow-call-permissions` without `assume-default-permissions` reports only what no default token has (`id-token`).
- `unredacted-secrets`, `overprovisioned-secrets` and `typosquat-uses` stay in `default` although their false-positive rate on a corpus is not recorded.

## Next steps

- [jactionlint and zizmor](./zizmor-parity.md): what changes for a user who moves from zizmor, and where each rule differs.
- [Profiles](./config.md#profiles): the definition of the profiles that the harness compares.
- [The policy for new checks](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features): the measurement bar that a rule meets before it enters the default profile.
- [The rules](./rules.md): the registry with the profile and level of every rule.
