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
| Durable ignores | New: `ignores:` in the config file accepts findings by rule, file, job, step and `uses:` instead of by line, with an optional `reason` and `expires` date. Unlike an ignore comment it survives Renovate or Dependabot rewriting the `uses:` line. An expired entry stops suppressing and is reported as `expired-ignore`; an entry that never matched is reported as `unused-ignore` (`strict` profile). See [durable ignores](config.md#durable-ignores). | planned |
| Ignore comments at the end of a line | A `# jactionlint ignore=...` comment after YAML content (`- uses: a/b@v1 # jactionlint ignore=unpinned-uses`) is now a directive covering that line, and the whole step when it is on the step's first line. In v1 and actionlint only a comment on its own line worked and this form was silently ignored, so an old comment of this shape that was a no-op now starts suppressing. | planned |

## Config migration

The old boolean options keep working for one minor release of v2 and map onto rule IDs, printing a deprecation warning.
A `migrate` command is planned to rewrite an existing configuration file:

```sh
# planned, not available yet
jactionlint -migrate-config
```

## Planned: autofix and hk

v2 plans an autofix mode, `jactionlint -fix [files]`, which applies only safe, mechanical fixes, is idempotent and re-lints
afterwards. `-format sarif` will carry the same fixes (rule ID, level, start and end positions, replacements), so
[hk](https://hk.jdx.dev) can use jactionlint as a fixer: `hk check` shows diagnostics with rule IDs, and `hk fix` applies the
patch derived from the SARIF fixes, falling back to `jactionlint -fix` when some findings have no fix. Unsafe fixes are
never part of the SARIF output and need an explicit `-fix=unsafe`.

## Related

- [zizmor parity](zizmor-parity.md): the per-audit tracking of planned security and policy rules.
- [Configuration](config.md): how configuration works today.
