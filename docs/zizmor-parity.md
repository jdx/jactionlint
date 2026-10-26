# jactionlint and zizmor

jactionlint has a rule for each audit of zizmor 1.30.1, under the same ID in every case but one, and adds rules of its own. This page is for people who run zizmor and want to run jactionlint beside it or in its place: what maps, what differs on purpose and what is missing. It does not claim identical results: `scripts/zizmor-diff` compares jactionlint with that version, and the dated figures are in [the measurements ledger](./zizmor-measurements.md).

## At a glance

- Every audit of zizmor has a rule. All keep the ID except `unsound-condition`, which `if-always-true` reports. [Audit by audit](#audit-by-audit) says where the results differ.
- A rule has one level (`error`, `warn` or `info`), where zizmor grades a finding by severity, confidence and persona. The personas become [profiles](#profiles-and-personas), and the noisier findings of an audit are the option `pedantic` of the same rule.
- Nothing uses the network unless you ask. zizmor runs its online audits whenever it finds a token. jactionlint needs `--online`, `online: true` or `JACTIONLINT_ONLINE` (see [online checks compared with zizmor](#online-checks-compared-with-zizmor)).
- `# zizmor: ignore[...]` comments carry over. `zizmor.yml` does not (see [move from zizmor](#move-from-zizmor-to-jactionlint)).
- Exit statuses differ, so a CI job that tests them changes (see [what changes in a CI pipeline](#what-changes-in-a-ci-pipeline)).
- Both tools check workflow files, `action.yml` files and `dependabot.yml`. jactionlint does not read pre-commit configurations or audit a repository by its slug.
- jactionlint has [rules and features that zizmor does not have](#rules-and-features-that-zizmor-does-not-have). zizmor has things that jactionlint lacks, listed under [known gaps](#known-gaps).

## Move from zizmor to jactionlint

Run both tools for a while, then switch the jobs. Each step stands on its own.

1. **Run jactionlint beside zizmor.** Compare what each reports on the same repository. jactionlint also runs the checks of actionlint (syntax, expressions, shellcheck), which zizmor does not. `--select` reports only the rules you name.

   ```sh
   zizmor --persona pedantic .
   jactionlint --profile pedantic
   jactionlint --select unpinned-uses,artipacked
   ```

2. **Choose the profile.** The `regular` persona is the `default` profile, and `pedantic` and `auditor` are the `pedantic` profile. The [table of personas](#profiles-and-personas) lists the exceptions.

3. **Keep your ignores.** `# zizmor: ignore[...]` comments keep working, and `jactionlint --migrate-ignores` rewrites them into `# jactionlint ignore=` comments. See [zizmor ignore comments](./ignore.md#zizmor-ignore-comments).

4. **Recreate `zizmor.yml`.** jactionlint reads neither `zizmor.yml` nor `.github/zizmor.yml`. [Map each setting](#map-zizmor-yml-to-the-config-file) to a key of `.github/jactionlint.yaml`.

5. **Adopt the stricter default gradually.** `jactionlint --baseline-write` records the current findings, and `jactionlint --baseline` fails only on new ones. See [the baseline](./baseline.md#baseline).

6. **Change the CI job.** The next section lists what differs, and [the CI methods](./ci.md#the-action) show the jobs.

7. **Review the fixes.** `jactionlint --diff` prints what `--fix` would change and writes nothing. See [fix findings automatically](./fix.md#fix-errors-automatically) and [fixes compared with zizmor](#fixes).

8. **Retire zizmor.** When zizmor no longer runs on the repository, set the option `zizmor` of `unused-ignore` (`rules: {unused-ignore: {zizmor: true}}`) to find the `# zizmor: ignore[...]` comments that suppress nothing.

### Map `zizmor.yml` to the config file

Each setting maps as follows:

| `zizmor.yml` | `.github/jactionlint.yaml` |
| --- | --- |
| `rules.<audit>.disable: true` | `rules: {<rule>: off}` |
| `rules.<audit>.ignore` with `file.yml` | an entry of `ignores:` with `rule` and `file`; add `job`, `step` or `uses` to narrow it |
| `rules.<audit>.ignore` with `file.yml:line:column` | no key: put an ignore comment on that line, or describe the place with `job`, `step` or `uses` |
| `rules.<audit>.config` | the options of the rule under `rules:`: `allow` and `deny` of `forbidden-uses`, `policies` of `unpinned-uses`, `days` of `dependabot-cooldown`, `allow` of `secrets-outside-env` and of `known-vulnerable-actions` |
| `rules.<audit>.remap.severity` | `rules: {<rule>: <level>}`, one level per rule |

For example, this `zizmor.yml`:

```yaml
rules:
  template-injection:
    disable: true
  unpinned-uses:
    config:
      policies:
        actions/*: ref-pin
        "*": hash-pin
  dangerous-triggers:
    ignore:
      - release.yml
```

becomes this `.github/jactionlint.yaml`:

```yaml
rules:
  template-injection: off
  unpinned-uses:
    policies:
      actions/*: ref-pin
      "*": hash-pin
ignores:
  - rule: dangerous-triggers
    file: .github/workflows/release.yml
    reason: carried over from zizmor.yml
```

jactionlint rejects a pattern of `policies` that has an `@ref` (zizmor accepts `owner/repo@ref`) when it reads the configuration. See [the rules key](./config.md#rules) and [durable ignores](./config.md#durable-ignores).

## What changes in a CI pipeline

The table maps what a zizmor job uses to its jactionlint counterpart. [Command line](./cli.md#command-line-options) lists every option, and [exit status](./cli.md#exit-status) the statuses.

| | zizmor | jactionlint |
| --- | --- | --- |
| Sensitivity | `--persona regular`, `pedantic`, `auditor` or `--pedantic` | `--profile default` or `pedantic` (`profile:` in the config file) |
| Network | online audits run when a token is found; `--offline` or `ZIZMOR_OFFLINE` forces offline mode, `--no-online-audits` or `ZIZMOR_NO_ONLINE_AUDITS` only skips the online audits | offline unless `--online`, `online: true` or `JACTIONLINT_ONLINE=1`; `--no-online` forces offline mode |
| Token | `--gh-token`, `GH_TOKEN`, `GITHUB_TOKEN`, `ZIZMOR_GITHUB_TOKEN` | `--online-token-env`, `--online-token-file`, `GITHUB_TOKEN`, `GH_TOKEN`, then `gh auth token` |
| GitHub Enterprise Server | `--gh-hostname`, `GH_HOST` | `--online-api-url`, `GITHUB_API_URL`, `GITHUB_SERVER_URL`, `GH_HOST` |
| Exit status | 0 no findings, 1 failure, 2 invalid arguments, 3 no inputs, 11 to 14 by the highest severity (informational, low, medium, high); `--no-exit-codes` and `--format sarif` use 0 and the failure codes only | 0 no findings, 1 findings of level `error`, 2 invalid command line, 3 failure; `--strict-exit` makes `warn` and `info` findings exit 1 too; `--format sarif` exits 1 on findings; no option turns the finding status off |
| Severity filter | `--min-severity informational`, `low`, `medium` or `high`; `--min-confidence` | `--min-severity info`, `warn` or `error`; the level of a rule is set under `rules:`; no confidence |
| Output | `--format plain`, `json`, `json-v1`, `sarif`, `github` | `--format text`, `oneline`, `json`, `jsonl`, `sarif`, `gcc`, `github`, `summary` or a Go template, see [output formats](./output.md#format-error-messages) |
| Fixes | `--fix[=safe]`, `--fix=unsafe-only`, `--fix=all` | `--fix` (safe), `--fix=unsafe` (safe and unsafe), `--diff`, `--fix-rules` |
| Inputs | files, directories, `-`, a `owner/repo[@ref]` slug; `--collect` picks workflows, actions, Dependabot and pre-commit files | files and `-`; with no file, what [a run checks](./usage.md#what-a-run-checks): workflow files, composite actions, `dependabot.yml` and the Actions lock file |
| Config file | `zizmor.yml` (`.github/zizmor.yml`), `--config`, `--no-config` | `.github/jactionlint.yaml`, `--config-file` |
| Ignores | `# zizmor: ignore[audit]`, `ignore:` in `zizmor.yml`, `--no-ignores` | `# jactionlint ignore=RULE` (zizmor comments are honored too), `ignores:`, `paths:`, a baseline, `--no-ignores` |

## Profiles and personas

zizmor grades every finding with a persona: `regular`, `pedantic` or `auditor`. jactionlint has three [profiles](./config.md#profiles), and each includes the one before it.

| zizmor persona | jactionlint |
| --- | --- |
| `regular` | `default` profile |
| `pedantic` | `pedantic` profile |
| `auditor` | `pedantic` profile |

An audit with findings of more than one persona is one rule with one ID. The option `pedantic` of the rule turns on the noisier findings: it is on under the `pedantic` profile and off otherwise, and `rules: {template-injection: {pedantic: true}}` turns it on for one rule under the `default` profile. `jactionlint --explain RULE` lists the options of a rule. These audits are the exceptions to the table above:

| zizmor audit | zizmor persona | jactionlint |
| --- | --- | --- |
| `concurrency-limits` | pedantic | `default` profile; a workflow that only a schedule or a manual run starts is reported with the option `pedantic` |
| `self-repository` | regular | `pedantic` profile, level `info` |
| `template-injection` | regular, with pedantic findings | `correctness` profile (so also `default`); the pedantic findings are the option `pedantic` |
| `unpinned-images` | regular for an image without a tag or with the `latest` tag, pedantic for another tag without a digest | `default` profile; the option `require-digest` chooses between the two |
| `misfeature` | regular, with custom shells in the auditor persona | `default` profile; the option `pedantic` adds custom shells |
| `superfluous-actions` | regular, with `dtolnay/rust-toolchain` in the pedantic persona | `default` profile; the option `pedantic` adds `dtolnay/rust-toolchain` |
| `hardcoded-container-credentials`, `unsound-condition`, `unsound-ternary` | regular | `correctness` profile (so also `default`) |
| `forbidden-uses` | regular | no profile: it runs only when `allow` or `deny` is set |
| `ref-version-mismatch` | regular for a wrong version comment, pedantic for a missing one | online rule for a wrong comment; a missing or version-less comment is the rule [`missing-version-comment`](./rules.md#missing-version-comment), which needs no network and is in the `pedantic` profile |
| `stale-action-refs` | pedantic | online rule |
| the other online audits | regular | online rules |

The online rules belong to no profile: they run, at their own level, when the online checks are on.

## Audit by audit

One row per audit of zizmor. The status says how the results compare:

- `same`: both report the same findings in the cases that were compared, positions aside.
- `differs`: jactionlint reports fewer, more or other findings, for the reason in the last column.
- `not comparable`: the two rules check different things, or no run has compared them.

The last column holds the difference that matters most; [where a rule differs on purpose](#where-a-rule-differs-on-purpose) holds the rest. [The rules](./rules.md) give the profile and level of each rule, and [the measurements ledger](./zizmor-measurements.md) the numbers.

| zizmor audit | jactionlint rule | Status | The difference that matters |
| --- | --- | --- | --- |
| `adhoc-packages` | [`adhoc-packages`](./rules.md#adhoc-packages) | differs | `--prefix` values and local tarballs are not ad hoc; installs behind `sudo` are reported. |
| `anonymous-definition` | [`anonymous-definition`](./rules.md#anonymous-definition) | differs | Skipped in generated files and for `copilot-setup-steps.yml`; not applied to `action.yml`, whose `name` GitHub requires. |
| `archived-uses` | [`archived-uses`](./rules.md#archived-uses) | differs | Asks GitHub, so it needs the online checks. zizmor ships a list of archived repositories and reports offline. |
| `artipacked` | [`artipacked`](./rules.md#artipacked) | differs | Not reported when a later step pushes, for `actions/checkout@v1`, for `actions/checkout@v6` and later (the credential is no longer in the workspace) or for an expression value. The fix is safe, zizmor's is unsafe. |
| `bot-conditions` | [`bot-conditions`](./rules.md#bot-conditions) | differs | Judged by the events that reach the job, also through local reusable workflows; a negated test and a conjunction with a check of the pull request author are not reported. The fix is unsafe, zizmor's is safe. |
| `cache-poisoning` | [`cache-poisoning`](./rules.md#cache-poisoning) | differs | Fewer findings by default, because `tags-ignore` is not a release trigger; the option `pedantic` adds zizmor's heuristics. See [the check](./checks.md#check-cache-poisoning). |
| `concurrency-limits` | [`concurrency-limits`](./rules.md#concurrency-limits) | differs | In `default`, not pedantic. A workflow that only `schedule` or `workflow_dispatch` starts is reported with the option `pedantic`, and the advice for it never cancels runs. Not reported: `copilot-setup-steps.yml`, generated files, a `concurrency:` that is one expression. For a release workflow it advises `cancel-in-progress: false`, as `concurrency-cancels-release` does. |
| `dangerous-triggers` | [`dangerous-triggers`](./rules.md#dangerous-triggers) | differs | Also reports `issue_comment`, at the trigger instead of `on:`; a workflow that cannot write and reads no secret needs the option `pedantic`. |
| `dependabot-cooldown` | [`dependabot-cooldown`](./rules.md#dependabot-cooldown) | differs | Reports every update below the minimum, where zizmor stops at the first update that meets it; an update that proposes nothing is not reported. `--fix` writes the value of the option `default-days`, where zizmor writes 7. |
| `dependabot-execution` | [`dependabot-execution`](./rules.md#dependabot-execution) | same | The fix is unsafe in both. |
| `excessive-permissions` | [`excessive-permissions`](./rules.md#excessive-permissions), [`missing-permissions`](./rules.md#missing-permissions) | differs | Two rules: the workflow level and jobs without `permissions:`. See [permissions](#where-a-rule-differs-on-purpose). |
| `forbidden-uses` | [`forbidden-uses`](./rules.md#forbidden-uses) | not comparable | Opt-in; `allow` and `deny` have the names of zizmor's, and no run compared them. |
| `github-app` | [`github-app`](./rules.md#github-app) | differs | `actions/create-github-app-token`: `skip-token-revoke`, `owner` without `repositories`, missing `permission-*` inputs. Also checks `tibdex/github-app-token` and `peter-murray/workflow-application-token-action`, for which zizmor reports nothing. |
| `github-env` | [`github-env`](./rules.md#github-env) | differs | Accepts values it can show are not attacker controlled (runner paths, `date`, a checked number); reports at the line that writes. See [the check](./checks.md#check-github-env). |
| `hardcoded-container-credentials` | [`hardcoded-container-credentials`](./rules.md#hardcoded-container-credentials) | same | The credentials check of actionlint reports it, at the password. |
| `impostor-commit` | [`impostor-commit`](./rules.md#impostor-commit) | same | Online; no verdict beyond `max-branches` branches. |
| `insecure-commands` | [`insecure-commands`](./rules.md#insecure-commands) | same | Workflow, job and step `env`; the fix is unsafe in both. |
| `insecure-url-scheme` | [`insecure-url-scheme`](./rules.md#insecure-url-scheme) | not comparable | zizmor checks the `repo:` URLs of pre-commit configurations; jactionlint checks locations in scripts and inputs. |
| `known-vulnerable-actions` | [`known-vulnerable-actions`](./rules.md#known-vulnerable-actions) | same | Online; a branch ref is not judged. |
| `misfeature` | [`misfeature`](./rules.md#misfeature) | differs | Custom shells need the option `pedantic` (zizmor: auditor persona). |
| `obfuscation` | [`obfuscation`](./rules.md#obfuscation) | differs | Not reported: a constant `if:` (`constant-condition` reports it), an index chosen by the matrix, a trailing slash in `owner/repo/@sha`. |
| `overprovisioned-secrets` | [`overprovisioned-secrets`](./rules.md#overprovisioned-secrets) | differs | Also reports a bare `${{ secrets }}` and `secrets` passed to another function such as `format()`; zizmor reports `toJSON(secrets)` and a computed name. |
| `ref-confusion` | [`ref-confusion`](./rules.md#ref-confusion) | not comparable | Online; neither tool had a finding on the compared corpus. |
| `ref-version-mismatch` | [`ref-version-mismatch`](./rules.md#ref-version-mismatch), [`missing-version-comment`](./rules.md#missing-version-comment) | differs | Two rules, as for permissions: the online rule compares the comment with the tags, and a missing or version-less comment is `missing-version-comment`, which needs no network and is in the `pedantic` profile. A comment that names a branch is not compared. See [version comments](#where-a-rule-differs-on-purpose). |
| `secrets-inherit` | [`secrets-inherit`](./rules.md#secrets-inherit) | same | Reported at the `uses:` of the job. |
| `secrets-outside-env` | [`secrets-outside-env`](./rules.md#secrets-outside-env) | differs | Reported at the expression that reads the secret, as zizmor does; also a secret passed with `secrets:` to a reusable workflow. `toJSON(secrets)` and a computed name are left to `overprovisioned-secrets`. |
| `self-hosted-runner` | [`self-hosted-runner`](./rules.md#self-hosted-runner) | differs | Also reads matrix values, groups and `self-hosted-runner.labels`; options `allow` and `pull-request-only`. |
| `self-repository` | [`self-repository`](./rules.md#self-repository) | same | The fix is unsafe in both. |
| `stale-action-refs` | [`stale-action-refs`](./rules.md#stale-action-refs) | same | Online; no verdict beyond 1,000 tags of a repository. |
| `superfluous-actions` | [`superfluous-actions`](./rules.md#superfluous-actions) | differs | Not reported when the runner may be self-hosted. A `runs-on:` expression counts as self-hosted unless the repository tells every label it can have, see [the runner](#where-a-rule-differs-on-purpose). Adds two archived release actions. |
| `template-injection` | [`template-injection`](./rules.md#template-injection) | differs | See [template injection](#template-injection). |
| `typosquat-uses` | [`typosquat-uses`](./rules.md#typosquat-uses) | differs | Both report an omitted, repeated, swapped or look-alike character (`doker/login-action`, `acti0ns/checkout`); jactionlint also reports any other added or replaced character (`dockers/login-action`). |
| `undocumented-permissions` | [`undocumented-permissions`](./rules.md#undocumented-permissions) | differs | A comment above a scope counts; each scope is a finding. |
| `unpinned-images` | [`unpinned-images`](./rules.md#unpinned-images) | differs | No persona split (`require-digest` chooses); a matrix image is reported at the matrix value; also reports `docker run` images. An image in a `docker://` step is an `unpinned-uses` finding. |
| `unpinned-tools` | [`unpinned-tools`](./rules.md#unpinned-tools) | differs | Same for the actions zizmor knows; an input that is an expression is not reported; the option `pedantic` adds more. |
| `unpinned-uses` | [`unpinned-uses`](./rules.md#unpinned-uses) | differs | Skipped in generated files; adds the policy `immutable-release` and the Actions lock file (the option `actions-lock`, see [the check](./checks.md#check-actions-lock)). |
| `unredacted-secrets` | [`unredacted-secrets`](./rules.md#unredacted-secrets) | same | The option `pedantic` adds a printed value cut from a secret; zizmor has no counterpart. |
| `unsound-condition` | [`if-always-true`](./rules.md#if-always-true) | differs | The rule has another ID; composite action steps are checked too. A block scalar (`if: \|`) with one expression is not reported, because GitHub trims its trailing newline, where zizmor reports `if: >` and `if: \|` for it; see [the check](./checks.md#if-cond-constant). |
| `unsound-contains` | [`unsound-contains`](./rules.md#unsound-contains) | same | |
| `unsound-ternary` | [`unsound-ternary`](./rules.md#unsound-ternary) | same | Reported at the same positions. |
| `use-trusted-publishing` | [`use-trusted-publishing`](./rules.md#use-trusted-publishing) | differs | Skips registries without trusted publishing; reports a long-lived credential next to `id-token: write`. |

## Where a rule differs on purpose

These differences are choices, not gaps. Each entry gives the effect you see first, and [the checks page](./checks.md) holds the behavior of each rule in full.

- **Generated files and steps that never run.** A file whose first comment says that a tool wrote it (`# This file was automatically generated by gh-aw`, `# generated by praktika`, `# Generated by projen`) is not checked by the rules of the `policy` and `style` groups, because its author fixes the generator. `generated-files: report` ends that, see [generated files](./config.md#generated-files). Findings in a job or a step whose `if:` is constant false (`false`, `${{ !true }}`, `false && github.event_name == 'push'`) are dropped for every rule outside the `correctness` group. zizmor skips the first two but reports the third.
- **Permissions.** zizmor reports a workflow without a top-level `permissions:` and a job without permissions as `excessive-permissions`. Here the workflow-level finding is the option `require-workflow-permissions` of `excessive-permissions`, and a job without permissions is `missing-permissions`. That policy rule skips generated files and leaves the jobs of a reusable-only workflow (`on: workflow_call` alone) to its callers, because the callee gets at most what the caller grants. `undocumented-permissions` reports each scope without a comment, where zizmor reports one finding per `permissions:` block.
- **`use-trusted-publishing`** does not report a publish to a registry without trusted publishing (GitHub Packages, MyGet, a local registry, the Visual Studio marketplace), a script that is not a publish command (`pnpm run publish`) or a reusable workflow without `permissions:`, because the caller may grant `id-token: write`. The option `pedantic` adds a job that publishes with the OIDC token and also builds (zizmor#1968), a job without `environment:` (zizmor#237) and a job that switches attestations off (zizmor#938).
- **`self-hosted-runner` and `secrets-outside-env`.** `self-hosted-runner` does not resolve labels from other expressions (zizmor reports `runs-on: ["${{ inputs.runner }}"]`), and `self-hosted-runner.providers` lists the hosted-runner providers, such as namespace or blacksmith, whose labels `unknown-runner-label` accepts. `secrets-outside-env` skips `workflow_call` workflows, as zizmor does.
- **`concurrency-limits`.** A workflow that only a `schedule` or a `workflow_dispatch` starts is reported only with the option `pedantic` (on under the `pedantic` profile), because nothing supersedes such a run. The advice there is a top-level `concurrency:` with a `group:` and not `cancel-in-progress: true`, because a cancelled scheduled run is not started again and can lose its work. A bare group name (`concurrency: nightly`) is reported with the advice to write `cancel-in-progress: false` explicitly, and neither finding has a fix. zizmor reports both shapes in these workflows like in any other. Neither tool reports `cancel-in-progress: false`. A `concurrency:` that is one expression (`${{ github.workflow }}`) is not taken for a group name, because an expression can be a whole configuration; zizmor reports it. The default profile does not report the scheduled and manual workflows, so it does not change.
- **`superfluous-actions` and the runner.** A job whose runner may be self-hosted is not reported, because the tools of the hosted images are not there. A `runs-on:` expression is resolved when the repository tells every label it can have: matrix values, a condition between labels (`cond && 'a' || 'b'`, without evaluating the condition), `fromJSON()` of a text, and `inputs.*` of a reusable workflow that only `workflow_call` starts, when every caller in the repository passes a known value. Any other expression counts as self-hosted: `${{ vars.RUNNER }}`, `${{ inputs.runner }}` with no caller or with a caller that passes a variable, a runner group, the labels of a provider. A resolved label is judged like a written one, so a label other than `self-hosted` (the name of a larger runner) counts as GitHub-hosted. zizmor reports the job whatever its `runs-on:` holds.
- **Version comments.** The missing-comment finding of zizmor's `ref-version-mismatch` is the separate rule `missing-version-comment`, because `ref-version-mismatch` is an online rule and this one needs no network. It reports a hash-pinned action without a comment that names its version, and a comment that names none (`# the checkout action`). It also reports a pin whose commit no release tag points to, where zizmor stays silent because it has no version to suggest. It accepts a comment that has a version anywhere (`# v4.2.2 (source v4)`, `# Release v4.2.2`), which zizmor calls version-less, and a comment that starts with a branch such as `# main`. `--online --fix` adds the version of the tag that points at the commit, and the fix is safe because it only adds a comment, where zizmor's fix is unsafe. A pin with no tag, or with a comment of other text, gets no fix.

<a id="where-zizmor-is-wrong"></a>
### Findings that zizmor reports and jactionlint does not

jactionlint judges these findings of zizmor not to be findings:

- `adhoc-packages` on `npm install --prefix ./assets`: the value of `--prefix` is a directory, not a package.
- `use-trusted-publishing` on `pnpm dlx pkg-pr-new publish './packages/*'`: `pkg-pr-new` uploads a preview to pkg.pr.new and publishes to no registry.
- `concurrency-limits` and `anonymous-definition` on `copilot-setup-steps.yml`: GitHub runs the file to prepare the environment of the Copilot agent (zizmor#1481).
- `artipacked` on `actions/checkout@v1` (zizmor#1098, #1043): the version always leaves the credential, so nobody can act on the finding.
- `dependabot-cooldown` on an update that proposes nothing (`open-pull-requests-limit: 0`, or an `ignore` of every dependency): there is nothing for a cooldown to delay.
- `github-env` on `PR_NUMBER=$(cat pr_number)` written to `$GITHUB_ENV` after the workflow checked the value against `^[0-9]+$`.
- `bot-conditions` on `github.actor == 'dependabot[bot]' && github.event.pull_request.user.login == 'dependabot[bot]'` (zizmor#1914): the author of the pull request is checked as well.
- Any audit on a step or a job whose `if:` is `false && ...` (zizmor skips the plain `false`, zizmor#2059): the code does not run.

### Template injection

`template-injection` reports expressions that an outsider controls in `run:`, in github-script and in the code inputs of well-known actions. The `default` profile reports free text that the workflow does not validate (`inputs.*` of type string, `github.event.inputs.*`, `github.event.client_payload.*`, `github.ref_name` and the like). Booleans, numbers and choices are fixed vocabularies, so they are reported only with the option `pedantic`, which adds every other expansion too, such as `github.actor`, `matrix.*` and step outputs. [The check](./checks.md#check-template-injection-inputs) says what each expansion means. Compared with zizmor:

- Both tools report `github.ref_name`, string inputs and `github.event.inputs.*` by default.
- zizmor's regular persona also reports `github.actor` (a name that GitHub restricts to letters, digits and hyphens), `env.*`, `vars.*` and step outputs. jactionlint reports them only with the option `pedantic`, and it has no knowledge of the outputs of popular actions.
- jactionlint adds sinks that zizmor lacks: `container.options` and `services.<id>.options` (zizmor#1128), the image, entrypoint, command and volumes of containers and services, `args` and `entrypoint` of `docker://` steps, more code inputs of well-known actions, and the prompt, arguments and settings of AI agent actions.
- The level is per rule, so jactionlint does not grade by trigger. `--fix` moves a quoted reference into `env:` for bash and sh, and that fix is safe; an unquoted value of an assignment needs `--fix=unsafe`.

### Composite actions

zizmor audits `action.yml` files with the same audits as workflows but has no knowledge of who calls an action (zizmor#1424, #678). jactionlint checks the steps with the same rules, takes the triggers of an action from the local workflows that call it, and judges a published action that no local workflow calls by its own steps only. [Composite actions](./checks.md#check-composite-actions) says which rules apply.

<a id="rules-of-our-own"></a>
<a id="beyond-zizmor"></a>
<a id="intentional-differences-where-jactionlint-goes-beyond-zizmor"></a>
## Rules and features that zizmor does not have

These rules have no audit in zizmor. Some follow an idea from its issue tracker, named in the last column. [The rules](./rules.md) give the profile, level and summary of each. The `correctness` profile also adds the checks of actionlint, see [coming from actionlint](./actionlint.md).

| Rules | What they find | zizmor idea |
| --- | --- | --- |
| [`missing-timeout`](./rules.md#missing-timeout), [`timeout-too-long`](./rules.md#timeout-too-long) | a job without `timeout-minutes`; a value above `max` (opt-in) | [#2166](https://github.com/zizmorcore/zizmor/pull/2166) |
| [`unlocked-install`](./rules.md#unlocked-install) | an install that ignores or lacks a lock file | [#1405](https://github.com/zizmorcore/zizmor/issues/1405), [#2044](https://github.com/zizmorcore/zizmor/issues/2044) |
| [`actions-lock`](./rules.md#actions-lock), [`actions-lock-drift`](./rules.md#actions-lock-drift) | an Actions lock file that does not match the workflows or GitHub | [#2371](https://github.com/zizmorcore/zizmor/issues/2371) |
| [`pipeline-without-pipefail`](./rules.md#pipeline-without-pipefail) | a failure in a pipeline that the default shell hides | [#288](https://github.com/zizmorcore/zizmor/issues/288) |
| [`agentic-actions`](./rules.md#agentic-actions) | an AI agent action that outsiders can steer | [#1605](https://github.com/zizmorcore/zizmor/issues/1605) |
| [`unverified-download`](./rules.md#unverified-download), [`insecure-ssh-keyscan`](./rules.md#insecure-ssh-keyscan), [`checkout-static-credentials`](./rules.md#checkout-static-credentials) | a download run unverified, `ssh-keyscan` output trusted, a static credential for `actions/checkout` | [#711](https://github.com/zizmorcore/zizmor/issues/711), [#2012](https://github.com/zizmorcore/zizmor/issues/2012), [#1118](https://github.com/zizmorcore/zizmor/issues/1118) |
| [`workflow-secret-scope`](./rules.md#workflow-secret-scope), [`unused-workflow-secret`](./rules.md#unused-workflow-secret) | a workflow-level secret that reaches several jobs; a `workflow_call` secret nothing reads | [#1816](https://github.com/zizmorcore/zizmor/issues/1816), [#1044](https://github.com/zizmorcore/zizmor/issues/1044) |
| [`comment-trigger-authorization`](./rules.md#comment-trigger-authorization) | a job that starts on the text of a comment and checks no commenter | [#459](https://github.com/zizmorcore/zizmor/issues/459) |
| [`invisible-characters`](./rules.md#invisible-characters) | invisible and bidirectional control characters and private use characters (emoji variation selectors are not reported) | [#914](https://github.com/zizmorcore/zizmor/issues/914) |
| [`unsound-prefix-match`](./rules.md#unsound-prefix-match) | `startsWith()`, `endsWith()` or `contains()` on the name of an account or repository | [#1533](https://github.com/zizmorcore/zizmor/issues/1533) |
| [`untrusted-checkout`](./rules.md#untrusted-checkout), [`untrusted-artifact`](./rules.md#untrusted-artifact) | a privileged workflow that runs the code of a pull request or uses the artifact of the triggering run unchecked | [#195](https://github.com/zizmorcore/zizmor/issues/195) |
| [`repojacking`](./rules.md#repojacking), [`unpinnable-action`](./rules.md#unpinnable-action) | an action whose owner name anyone can register; a pinned action that pulls in another by a tag | [#479](https://github.com/zizmorcore/zizmor/issues/479) |
| [`shell-syntax`](./rules.md#shell-syntax) | a syntax error in a shell script of a `run:` step, found by a parser built into jactionlint when shellcheck does not run (see [the check](./checks.md#check-shell-syntax)) | [#2322](https://github.com/zizmorcore/zizmor/issues/2322) |
| [`hardcoded-registry-credentials`](./rules.md#hardcoded-registry-credentials) | a `password`, `token`, `key` or `auth-key` of a `registries` entry of `dependabot.yml` written in plain text, and a password in its `url` (see [the check](./checks.md#check-hardcoded-registry-credentials)) | [#1222](https://github.com/zizmorcore/zizmor/issues/1222) |
| [`dependabot-missing-actions-update`](./rules.md#dependabot-missing-actions-update) | no `github-actions` update in `dependabot.yml`, or one that does not name the directories of the local composite actions that use actions (the fix of that case is unsafe) | [#1806](https://github.com/zizmorcore/zizmor/issues/1806) |

Rules with no zizmor issue behind them, by what they check:

- **Concurrency and jobs.** [`concurrency-cancels-prs`](./rules.md#concurrency-cancels-prs), [`concurrency-cancels-release`](./rules.md#concurrency-cancels-release), [`gate-job-skipped-on-failure`](./rules.md#gate-job-skipped-on-failure), [`unused-job-output`](./rules.md#unused-job-output), [`duplicate-triggers`](./rules.md#duplicate-triggers), [`unused-needs`](./rules.md#unused-needs), [`continue-on-error`](./rules.md#continue-on-error), [`unused-workflow-input`](./rules.md#unused-workflow-input), [`concurrency-deadlock`](./rules.md#concurrency-deadlock).
- **Scripts and inputs.** [`command-injection`](./rules.md#command-injection), [`output-clobbering`](./rules.md#output-clobbering), [`argument-injection`](./rules.md#argument-injection), [`fail-fast-action-inputs`](./rules.md#fail-fast-action-inputs), [`macos-gnu-flags`](./rules.md#macos-gnu-flags).
- **Runners, permissions and actions.** [`mutable-runner-label`](./rules.md#mutable-runner-label) (fixable with the option `pin`), [`deprecated-node-runtime`](./rules.md#deprecated-node-runtime), [`deprecated-runner-label`](./rules.md#deprecated-runner-label), [`insufficient-permissions`](./rules.md#insufficient-permissions), [`ubuntu-slim-limits`](./rules.md#ubuntu-slim-limits), [`required-actions`](./rules.md#required-actions) (opt-in), [`action-release-cooldown`](./rules.md#action-release-cooldown) (online, opt-in).
- **Conventions and triggers.** [`require-shell`](./rules.md#require-shell), [`require-expression-wrapping`](./rules.md#require-expression-wrapping), [`max-run-lines`](./rules.md#max-run-lines), [`unmatched-paths-filter`](./rules.md#unmatched-paths-filter), [`event-name-never-received`](./rules.md#event-name-never-received).
- **Ignores.** [`unused-ignore`](./rules.md#unused-ignore), [`expired-ignore`](./rules.md#expired-ignore) and [`unused-baseline-entry`](./rules.md#unused-baseline-entry) report an ignore that suppresses nothing, an entry past its date and a baseline entry that matches no finding.

### Agentic actions

`agentic-actions` reports AI agent actions that outsiders can steer: an agent without a check of the user that has a token that can write contents, an open gate (`allowed_non_write_users: '*'`), settings that turn safeguards off, an agent on the code of a pull request, and allowed tools that give any code (`Bash(npx:*)`). The rule reports at `error` level whatever the trigger. It leaves out, on purpose, a triage job with a read-only token and a sandboxed agent, the architecture the vendors advise. See [the check](./checks.md#check-agentic-actions).

### Downloads and credentials

`checkout-static-credentials` reports an `ssh-key` by default and a token from a secret only with `secret-tokens`, which is off under the `default` profile and on under the `pedantic` profile: release workflows push with a personal access token so that the push starts other workflows. `insecure-url-scheme` is not one of the rules without an audit: zizmor checks the `repo:` URLs of pre-commit configurations, where jactionlint checks locations in scripts and inputs, so the two do not report the same finding (see [audit by audit](#audit-by-audit)).

### Features

- **A baseline for gradual adoption.** `--baseline-write` records the current findings and `--baseline` hides them, so a repository can adopt the stricter default and fail only on new findings. See [the baseline](./baseline.md#baseline).
- **Durable ignores.** zizmor ignores a finding with a comment or a `file:line:column` entry of `zizmor.yml`, and both break when a tool rewrites the line or when lines shift (zizmor#1086). The `ignores:` list matches by rule plus file glob, job ID, step ID or name and the `uses:` value, so a Renovate bump of a SHA keeps the entry working, and it takes a reason and an expiry date. See [durable ignores](./config.md#durable-ignores).
- **Ignore comments at the end of a line.** `# jactionlint ignore=RULE` on the first line of a step covers the whole step. See [ignore a finding](./ignore.md#ignore-some-errors).
- **Fixes that converge.** `--fix` lints, fixes and lints again until nothing is left, writes each file once and atomically, and refuses a fix that breaks the file. A fix that needs a number does not guess it: `missing-timeout` and `dependabot-cooldown` need `default-minutes` and `default-days`. See [fix findings automatically](./fix.md#fix-errors-automatically).
- **Failed online lookups are skipped, not fatal.** See [failures, rate limits and tokens](#online-client-where-it-differs-from-zizmor).
- **Composite actions judged by their callers.** See [composite actions](#composite-actions).

<a id="online-audits"></a>
## Online checks compared with zizmor

The online rules ask GitHub about the actions a workflow uses. [Online checks](./online.md#online-checks) owns their behavior, flags and options, and [the online options](./config.md#online-options) their config keys. This section lists what differs from zizmor.

- **When they run.** zizmor runs its online audits whenever it finds a token. jactionlint never uses the network unless `--online`, `online: true` or `JACTIONLINT_ONLINE` asks, so a token in the environment is not enough: pass `--online` or set `JACTIONLINT_ONLINE=1` in the CI job. `--no-online` turns the checks off even when the configuration file turns them on.
- **Which rules.** The online rules with a zizmor counterpart are `impostor-commit`, `known-vulnerable-actions`, `ref-confusion`, `stale-action-refs`, `archived-uses` and `ref-version-mismatch`. `repojacking`, `unpinnable-action` and `actions-lock-drift` have none.
- **`archived-uses`.** zizmor ships a fixed list of archived repositories and reports with `--offline`. jactionlint asks GitHub, so the answer is always current and needs `--online`; without it the rule reports nothing.
- **`ref-version-mismatch`.** jactionlint reads comments that start with a version, and ratchet comments (`# ratchet:actions/checkout@v4`). It does not compare a comment that names a branch with that branch, which zizmor does (see [known gaps](#known-gaps)). A missing or version-less comment is the rule `missing-version-comment`, which needs no network (see [version comments](#where-a-rule-differs-on-purpose)). `--fix` rewrites the version comment and is unsafe.
- **Large repositories.** `stale-action-refs` reads up to 1,000 tags per repository and says nothing about a commit it did not find in a truncated list. `impostor-commit` compares a commit with up to `max-branches` branches (default 1000, at most 100 without a token) and gives no verdict beyond that.

<a id="online-client-where-it-differs-from-zizmor"></a>
### Failures, rate limits and tokens

The online client is built differently from the one of zizmor. Its tests cover the failing lookups that zizmor#2210 and #1350 describe and the credentials in debug logs of GHSA-f42p-wjw5-97qh. Unit tests against a local `httptest` server cover each failure mode; no run compared the client with zizmor.

- A lookup that fails is skipped for that `uses:` only. A rate limit, a rejected token, a DNS or connection failure and three failed lookups in a row stop the remaining lookups of the run. jactionlint prints one warning per kind of failure and the exit status does not change, unless `--online=strict` makes a skipped lookup exit 3. See [when GitHub does not answer](./online.md#when-github-does-not-answer).
- A rate limit that resets soon is waited for, and server errors are retried with backoff. See [rate limits and retries](./online.md#rate-limits-and-retries).
- The token goes to the API host only and is redacted from all output. A host that a repository's own config file names gets no token.

## Ignores and fixes compared

### Ignore comments

jactionlint honors `# zizmor: ignore[...]` comments for the audits that map to one of its rules, so a repository that has triaged its zizmor findings keeps them triaged. The comment has to be spelled `# zizmor: ignore[rule-a,rule-b]`; a name with no rule in jactionlint is skipped, and `# zizmor: ignore` without a list does nothing, as in zizmor. Two audits are reported under another ID: `excessive-permissions` also covers `missing-permissions`, and `unpinned-images` also covers the Docker image findings of `unpinned-uses`. [The alias table](./ignore.md#audit-names-that-differ) has the details.

`unused-ignore` does not report zizmor comments unless its option `zizmor` is on, because zizmor may still run on the repository and decide what its comments suppress. The `impostor-commit` comments take effect only with `--online`.

<a id="fixers"></a>
### Fixes

zizmor marks each fix safe or unsafe, and jactionlint does the same: `--fix` applies the safe fixes and `--fix=unsafe` adds the unsafe ones. zizmor's `--fix=unsafe-only` has no counterpart. The table compares the audits that zizmor can fix with the fix of the rule here:

| zizmor audit | zizmor fix | jactionlint fix |
| --- | --- | --- |
| `artipacked` | unsafe | safe |
| `bot-conditions` | safe | unsafe |
| `cache-poisoning` | unsafe | none |
| `dependabot-cooldown` | safe | safe; needs the option `default-days` |
| `dependabot-execution` | unsafe | unsafe |
| `insecure-commands` | unsafe | unsafe |
| `known-vulnerable-actions` | unsafe | none |
| `obfuscation` | safe | unsafe |
| `ref-version-mismatch` | unsafe | unsafe; needs `--online` |
| `ref-version-mismatch`, a missing comment | unsafe | safe, as the rule `missing-version-comment`; needs `--online` |
| `self-repository` | unsafe | unsafe |
| `template-injection` | unsafe | safe for a quoted reference in bash and sh; unsafe for an unquoted value of an assignment |
| `unpinned-uses` | unsafe | safe; needs `--online` |
| `unsound-condition` | safe | none |

jactionlint also fixes rules that zizmor has no audit for, such as `missing-timeout`, `anonymous-definition`, `concurrency-limits`, `mutable-runner-label`, `invisible-characters` and `unlocked-install`, and pins an image to a digest with `--online --fix=unsafe` (`unpinned-images`). [Fix findings automatically](./fix.md#fix-errors-automatically) lists them.

<a id="what-is-not-done"></a>
## Known gaps

jactionlint does not do these things that zizmor does or proposes:

- **Branch version comments.** `ref-version-mismatch` does not compare a comment that names a branch (`# main`) with that branch.
- **Archived repositories offline.** zizmor reports `archived-uses` without the network, from a list of archived repositories that it ships.
- **Grading.** There is no `--min-confidence`, and a rule has one level whatever the trigger or the confidence.
- **Inputs.** jactionlint does not audit pre-commit configurations, a repository by its slug (`zizmor owner/repo`) or in a language server mode (`--lsp`).
- **`zizmor.yml`.** It is not read, see [move from zizmor](#move-from-zizmor-to-jactionlint).
- **A negated bot test.** `bot-conditions` does not report `!(github.actor == 'dependabot[bot]')`.
- **Policy patterns with `@ref`.** `unpinned-uses` rejects them when it reads the configuration.
- **Positions.** Some rules report at another line than zizmor (`dangerous-triggers` at the trigger, `unpinned-images` at the matrix value, `github-env` at the write), so a comparison by line shows differences that are not missing findings.

### Limits of the workflow rules

These rules do not cover every case:

- **`untrusted-checkout`** follows `actions/checkout`, `gh pr checkout` and `git` commands with a literal reference. It does not follow a ref from the output of an earlier step or a head passed through a composite action that calls another one. A gate that holds at one moment in front of a branch the author can still push to is a time-of-check finding of the same rule (zizmor#935), and `allow-unsafe-pr-checkout: true` is a finding of its own (zizmor#2134). See [the check](./checks.md#check-untrusted-checkout).
- **`untrusted-artifact`** links the use to the download by the `path` of the artifact or its `name`, and takes any `=~`, numeric test or checksum as validation. See [the check](./checks.md#check-untrusted-artifact).
- **`concurrency-cancels-release`** knows a fixed list of publish and deploy actions and commands. A release done by a script of the repository (`./scripts/release.sh`) is not recognized, so only tag pushes and `release` events are reported then.
- **`unused-needs`** finds only the entries that another needed job makes redundant, because a forgotten entry that is not redundant looks like one that sets the order.
- **Called workflows of other repositories.** None of these rules reads one, so a job output or input that a caller in another repository uses is not known.

## Next steps

- [Ignore a finding](./ignore.md#zizmor-ignore-comments): how zizmor comments and `--migrate-ignores` work.
- [Adopt jactionlint with a baseline](./baseline.md#baseline): record today's findings and fail on new ones.
- [Online checks](./online.md#online-checks): flags, tokens, cache and Enterprise Server.
- [Run jactionlint in CI](./ci.md#the-action): jobs for the action, Docker and other methods.
- [The measurements ledger](./zizmor-measurements.md): the dated numbers behind this comparison.
