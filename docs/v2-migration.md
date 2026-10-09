# Migrating to v2

> [!WARNING]
> **Planned, in progress.** jactionlint v2 has not been released. This page lists the breaking changes that are planned so
> you can see what is coming. Details will change, and the final text is written as each change lands. Nothing described
> here is available yet unless it says so.

v2 turns jactionlint from a tool that finds mistakes into one that organizes its checks in three tiers (correctness,
security and policy, see [the policy](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features))
with stable rule IDs, severities and profiles. These changes are breaking, which is why this is a major version.

## Planned breaking changes

| Change | What it means for you | Status |
| --- | --- | --- |
| Module path `github.com/jdx/jactionlint/v2` | Go API users update their imports. `go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest` replaces the old path. Command line users are not affected. | planned |
| Stable rule IDs | Every diagnostic carries an ID (for example `unpinned-uses`) that never changes, replacing matching on message text and the coarse `kind`. `-ignore`, `paths.*.ignore` and inline ignore comments accept IDs as well as message regular expressions. | planned |
| Strict config parsing | An unknown key in `jactionlint.yaml` becomes an error, with a suggestion for the closest known key. Today unknown keys are silently ignored. | planned |
| Exit codes by severity | Findings of severity `error` exit with 1. `warn` and `info` findings do not fail the run unless `-strict-exit` is given. `-min-severity` hides lower severities. | planned |
| `-format` changes | Built-in formats `text` (default), `oneline`, `json`, `jsonl`, `sarif`, `gcc` and `github`. Go templates keep working. `allKinds` still works and `allRules` is added to list every rule ID. SARIF output carries rule metadata, levels and fixes. | planned |
| Online checks, `-online` | New, opt-in: six checks that query the GitHub API (impostor commits, known vulnerable actions, ref confusion, stale refs, archived repositories, version comments) and `-online -fix` pinning tags to commits. Nothing changes unless you pass the flag. See [the usage document](usage.md#online-checks). | planned |
| Config booleans become `rules:` | Options such as `require-permissions` become entries in a `rules:` map (`<id>: off\|info\|warn\|error`), together with a `profile:` (`default`, `strict` or `all`) and `extends:` for shared config. | planned |
| `missing-timeout` is on by default | A job without `timeout-minutes` is now reported by the default profile. It used to be opt-in. Set `rules.missing-timeout.default-minutes` and run `jactionlint -fix` to add that `timeout-minutes` to every job (there is no built-in number, so without the option the finding has no fix), or set `rules: {missing-timeout: off}` to keep the old behavior. A config that has the old `timeout-minutes:` key keeps its old meaning. | planned |
| AI agent actions are checked by default | A workflow that gives the text of outsiders to an AI agent action (Claude Code Action, Gemini CLI, Codex and others) is reported: `${{ }}` of an attacker controlled context in a prompt is `template-injection`, and an agent that outsiders can steer without a check of the user, with its safeguards off, or on the code of a pull request is `agentic-actions`. Both report at `error` level in the default profile. `template-injection` also reports the options, image, entrypoint, command and volumes of `container:` and `services:`. Use `rules: {agentic-actions: off}` or an ignore comment for a workflow you accept. | planned |

## Config migration

The old boolean options keep working for one minor release of v2 and map onto rule IDs, printing a deprecation warning.
`timeout-minutes: {max: N}` maps to `timeout-too-long` only: `missing-timeout` stays as the profile sets it. Only a written `required: true` or `required: false` turns `missing-timeout` on or off.
A `migrate` command is planned to rewrite an existing configuration file:

```sh
# planned, not available yet
jactionlint -migrate-config
```

## zizmor ignore comments

jactionlint honors zizmor's `# zizmor: ignore[...]` comments (see [the usage document](usage.md#zizmor-ignore-comments)) so a
repository moving from zizmor does not get its triaged findings back. A name in the list stands for the jactionlint rule of
the same ID. These audits are reported under another ID and are mapped:

| zizmor audit | jactionlint rule | Note |
| --- | --- | --- |
| `excessive-permissions` | `missing-permissions` (and the rule of the same name) | the job-level report of zizmor is `missing-permissions` here |
| `unpinned-images` | `unpinned-uses` | only the Docker image findings of the rule |

Any other name is used as is when jactionlint has a rule with that ID and ignored otherwise. Run `jactionlint -migrate-ignores`
to turn the comments into `# jactionlint ignore=` comments. A name with both a rule of its own and an alias is migrated to all of them.

## Planned: autofix and hk

v2 plans an autofix mode, `jactionlint -fix [files]`, which applies only safe, mechanical fixes, is idempotent and re-lints
afterwards. `-format sarif` will carry the same fixes (rule ID, level, start and end positions, replacements), so
[hk](https://hk.jdx.dev) can use jactionlint as a fixer: `hk check` shows diagnostics with rule IDs, and `hk fix` applies the
patch derived from the SARIF fixes, falling back to `jactionlint -fix` when some findings have no fix. Unsafe fixes are
never part of the SARIF output and need an explicit `-fix=unsafe`.

## Related

- [zizmor parity](zizmor-parity.md): the per-audit tracking of planned security and policy rules.
- [Configuration](config.md): how configuration works today.
