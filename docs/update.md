# Update actions

`jactionlint update` checks the external actions and reusable workflows that your workflows and composite actions use, finds the latest release that your policy allows, and shows, previews or writes the new references. It pins them to a commit SHA with a matching version comment when you ask. It is separate from `--fix`: `--fix` repairs findings, `update` changes dependencies, so a lint run never moves a version.

```sh
jactionlint update              # list the available updates, change nothing
jactionlint update --check      # exit 1 when a reference has an update or breaks the policy
jactionlint update --diff       # the changes as a unified diff, change nothing
jactionlint update --apply      # write the changes
```

`update` reads releases from GitHub, so it always uses the [online options](./online.md#online-checks): the same token, cache, GitHub Enterprise Server settings and github.com fallback as `--online`. It writes nothing unless you pass `--apply`. Dependabot and Renovate stay the choice when you want pull requests; `update` is for a local run, a hook or a CI check.

## What it reads

Without operands, `update` reads the workflows in `.github/workflows` and every `action.yml` and `action.yaml` of the repository, like a lint run does. File operands restrict it to those files, and `--changed-since REV` (with `--changed-scope`) restricts it to the references on the changed lines.

It handles `uses:` values of actions (`owner/repo@ref`, `owner/repo/path@ref`) and of reusable workflows (`owner/repo/.github/workflows/x.yaml@ref`). It leaves local paths (`./action`), `docker://` images and values with `${{ }}` alone.

## How a release is chosen

1. The stable versions among the tags of the repository are the candidates. A tag such as `v4` that points at the commit of `v4.2.2` is a moving alias, not a release.
2. `include-tags` keeps the tags that match one of its glob patterns, then `exclude-tags` removes those that match, so an exclusion wins over an inclusion. Braces expand: `*-{alpha,beta,rc}*`.
3. From the highest version down, a candidate must pass the release checks:
   - `exclude-prereleases` rejects a release that GitHub marks as a prerelease, whatever its tag says.
   - `require-immutable-releases` accepts only a published release marked immutable. A tag without a release is rejected, and there is no fallback to tags. A draft is not a release.
   - `cooldown-days` rejects a release younger than that. The age is the publication date of the release, or the date of the tag or its commit for a tag without a release.
4. The first candidate that passes and is newer than the current version wins. **A reference is never moved to an older version**, not to satisfy the cooldown and not for any other reason: when only older releases pass, the reference stays and a note says which newer release is held back and why.

If GitHub cannot answer (the repository is gone, the rate limit is reached, the date of a release is unknown, the client cannot read release metadata), the reference is reported as `update-unverified` with the reason. It is not treated as up to date, and `--check` fails.

A reference that is already at the newest release, but whose release breaks the policy (a cooldown that is not over, a prerelease, a mutable release) is reported as `update-policy`.

## Pin to a commit

With `freeze: true` or `--freeze`, the new reference is the full commit SHA of the chosen release, with the version in a comment:

```yaml
# before
- uses: example/action@0123456789abcdef0123456789abcdef01234567 # v1.3.0
# after
- uses: example/action@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v1.4.0
```

A reference that is already a commit SHA stays a commit SHA when you do not freeze. `--pin-only` pins the release that is referenced and does not upgrade it, `uses: example/action@v1.3.0` becoming `@<sha> # v1.3.0`.

Before it updates a pinned reference, `update` verifies it: a tag of the repository must point at the commit, and the version in the comment must name that tag (or be a less specific spelling of it, such as `# v1` for `v1.3.0`). A commit that no tag points at, or a comment that names another commit, is reported as `update-unverified` and left alone.

The edit changes only the ref and the version in the comment. The rest of the comment, the quotes, the indentation and the line endings stay. A comment such as `# v1` that is still true for the new version is kept. A line it cannot edit without guessing (a comment that names no version, or other content after the value) is reported and left as it is.

A workflow that the [Actions lock file](./checks.md#check-actions-lock) lists is not rewritten with a commit SHA, because that can fail the lock check. `update` reports the update and tells you to add the entry with `gh actions-lock`.

## Branches

`--branch-to-tag REGEX` converts a reference whose ref is a branch matching the expression, such as `@main`, to the latest eligible release. With `--freeze` it is pinned to the commit of that release. Branch references that do not match are left alone.

## The Actions lock file

When `.github/workflows/actions.lock` exists, `--apply` ends by running `gh actions-lock --relock` from the repository root, so the lock file follows the workflows. It runs even when no reference changed, which refreshes mutable references. The [gh-actions-lock](https://github.com/github/gh-actions-lock) extension writes the file; `jactionlint` does not. Without `--apply` (the listing, `--check` and `--diff`) nothing runs. If `gh` or the extension is missing, or the relock fails, `update` prints the error and exits `3`; the workflow edits are already written.

## Options

| Option | Config key | Effect |
| --- | --- | --- |
| `--check` | | Exit 1 when a reference has an update, breaks the policy or cannot be verified. |
| `--diff` | | Print the changes as a unified diff. |
| `--apply` | | Write the changes. Then, when `.github/workflows/actions.lock` exists, run `gh actions-lock --relock`. |
| `--min-age DAYS` | `update.cooldown-days` | The age a release must have. `0` turns the cooldown off. |
| `--freeze` | `update.freeze` | Pin to the commit SHA with a version comment. |
| `--pin-only` | | Pin the referenced release without upgrading it. |
| `--include-tag GLOB`, `--exclude-tag GLOB` | `update.include-tags`, `update.exclude-tags` | Tag filters. Repeatable. |
| | `update.exclude-prereleases` | Leave out releases that GitHub marks as prereleases. |
| | `update.require-immutable-releases` | Accept only immutable releases. |
| `--branch-to-tag REGEX` | | Convert matching branches to release tags. Repeatable. |
| `--include-action REGEX`, `--exclude-action REGEX` | | Limit the actions by name: `owner/repo`, `owner/repo/path` or the path of a reusable workflow. Repeatable. |
| `--action-rule REGEX=ACTION` | | Per-action policy. `ignore` skips matching references, `cooldown:DAYS` sets their cooldown. The expression is matched against the value of `uses:` followed by its comment (`actions/checkout@v4 # v4.1.0`) and the first rule that matches applies. Repeatable. |

A command-line option wins over the config file. The other options are those of the linter: `--format` (with `sarif`, `json` and the others), `--changed-since`, `--config-file`, `--color`, `--online=cache`, `--online=strict`, `--online-api-url`, `--online-token-env`, `--online-github-com-fallback` and so on.

```yaml
# .github/jactionlint.yaml
update:
  cooldown-days: 7
  freeze: true
  include-tags: "v*"
  exclude-tags: ["*-{alpha,beta,rc}*"]
  exclude-prereleases: true
  require-immutable-releases: true
```

## Output and exit status

Each reference with something to report is a finding with a rule-like ID, so every output format works, SARIF included:

| ID | Level | Meaning |
| --- | --- | --- |
| `update-available` | warn | A newer eligible release exists, or the policy wants the reference pinned. |
| `update-policy` | warn | The reference breaks the policy and no eligible release replaces it. |
| `update-unverified` | warn | The version could not be verified, or the lookup failed. |
| `update-held` | info | A newer release exists but is not eligible yet. |

Exit status: `0` unless one of the cases below applies. `1` for `--check` when a finding of `update-available`, `update-policy` or `update-unverified` remains (the other modes exit `0` with those findings, so a listing or an `--apply` does not fail a job). `2` for an invalid command line. `3` for a failure such as an unreadable file or, with `--online=strict`, a skipped lookup.

These IDs are not rules of the linter. `--ignore`, `ignores` and ignore comments do not apply to them; use `--exclude-action`, `--action-rule` or `--changed-since` to leave references out.
