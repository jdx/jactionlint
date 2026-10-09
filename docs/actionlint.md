# Coming from actionlint

jactionlint is a maintained fork of [actionlint][actionlint]. It reads the same workflows, takes the same options, spelled the POSIX/GNU way (see the [migration table](v2-migration.md#command-line-options)) and finds the same
mistakes, and it adds checks for security and policy on top. This page is for a repository that used actionlint and wants either
the same result as before or to see what else it can have.

## Get the behavior of actionlint

Choose the `correctness` [profile](config.md#profiles). It has what actionlint checks and the bug detectors of jactionlint, and no
rule about security posture or policy. Put it in `.github/jactionlint.yaml`:

```yaml
profile: correctness
```

or pass it on the command line, which wins over the file:

```sh
jactionlint --profile correctness
```

Without a profile you get the `default` one, which also has rules such as `unpinned-uses`, `missing-permissions`,
`missing-timeout` and `concurrency-limits`. A repository that never asked for these will see new errors until it chooses one.

The promise of the profile is tested: `go test` runs the fixtures that came from actionlint with `profile: correctness` and no other
setting, and requires every error their `.out` files expect to be reported. The profile can report more than actionlint does, see
[what is stricter](#what-is-stricter-than-actionlint).

### Turn rules off on top of it

A rule is off with its [rule ID](rules.md) in the `rules` mapping, and a finding can be ignored by ID with `--ignore` and
`# jactionlint ignore=` comments, like actionlint ignores by regular expression (regular expressions work too):

```yaml
profile: correctness
rules:
  unsound-ternary: off
  workflow-run-names: off
  shellcheck: warn # lower the level: warnings do not fail the run
```

Every rule of the profile reports at `error`, so the exit status is 1 when anything is found, as with actionlint. A rule you set to
`warn` or `info` does not fail the run unless `--strict-exit` is given.

## A config file written for actionlint

`.github/actionlint.yaml` and `.github/actionlint.yml` are still read when there is no `.github/jactionlint.yaml`, and so are the
global files under `$XDG_CONFIG_HOME/actionlint`. The keys `self-hosted-runner`, `config-variables`, `config-secrets` and
`paths` mean the same. Such a file has no `profile`, so the `default` profile applies, which has more rules than actionlint. jactionlint
says so once per run:

```
note: config file ".github/actionlint.yaml" was read as a jactionlint config. it sets no "profile", so the default profile applies, which has more rules than actionlint. add "profile: correctness" to the file or run with --profile correctness for the checks of actionlint. see https://jactionlint.jdx.dev/actionlint
```

Add `profile: correctness` to the file, or move it to `.github/jactionlint.yaml` and add the key there. actionlint itself ignores
a key it does not know, so a file shared by both tools can keep `profile: correctness`.

## From the checks of actionlint to rule IDs

| Check of actionlint | Rule IDs of jactionlint |
| --- | --- |
| Unexpected keys, missing required keys, duplicate keys, empty mappings, mapping values | `workflow-syntax`, `duplicate-key`, `yaml-syntax`, `merge-key`, `recursive-alias`, `unused-anchor` |
| Syntax of `${{ }}` | `expression-syntax` |
| Types, contexts, built-in functions, contextual typing of `steps`, `matrix` and `needs`, comparisons | `expression-type`, `undefined-property`, `undefined-function`, `invalid-function-call` |
| Availability of contexts and special functions | `context-availability` |
| Constant conditions and `if:` that are always true | `constant-condition`, `if-always-true` |
| shellcheck and pyflakes for `run:` | `shellcheck`, `pyflakes` |
| Script injection by untrusted inputs | `template-injection` |
| Job dependencies | `undefined-job-needs`, `cyclic-job-needs`, `duplicate-job-needs` |
| Matrix values | `matrix-duplicate-value`, `matrix-invalid-exclude` |
| Webhook events and `workflow_dispatch` | `unknown-event`, `invalid-activity-type`, `invalid-event-filter`, `invalid-event-config`, `invalid-workflow-dispatch-input` |
| Glob patterns of filters | `invalid-glob` |
| `cron` and time zones | `invalid-cron`, `cron-too-frequent`, `invalid-timezone` |
| Runner labels | `unknown-runner-label`, `conflicting-runner-labels`, `invalid-label-pattern` |
| Format of `uses:` | `invalid-uses` |
| Inputs of local and popular actions | `missing-action-input`, `unknown-action-input`, `invalid-local-action`, `deprecated-action-input` |
| Outdated popular actions | `outdated-action-runner` |
| Reusable workflows (`workflow_call`) | `invalid-workflow-call`, `invalid-workflow-call-input`, `missing-workflow-input`, `unknown-workflow-input`, `missing-workflow-secret`, `unknown-workflow-secret`, `workflow-input-type`, `workflow-call-permissions`, `invalid-local-workflow` |
| Shell names | `invalid-shell-name` |
| Job and step IDs | `duplicate-job-id`, `duplicate-step-id`, `invalid-id` |
| Hardcoded credentials of containers | `hardcoded-container-credentials` |
| Environment variable names | `invalid-env-var-name` |
| Permissions | `invalid-permissions` |
| Deprecated workflow commands | `deprecated-commands` |
| Ignore comments | `invalid-ignore-comment`, `expired-ignore` |

[The list of rules](rules.md) has the group, level and profile of each. A finding prints its ID at the end of the line, and the
`id` field of `--format json` and the `ruleId` of `--format sarif` carry it. `--format` templates written for actionlint keep working:
the fields of an error are the same, and `kind` is still there.

## Where shellcheck findings are reported

actionlint reports a shellcheck finding at the `run:` key of the step. jactionlint reports it at the line of the script that has
the problem when `run:` is a literal block (`|`, `|-`, `|+`) and no `${{ }}` in the script spans several lines, which is more precise
and puts the annotation of a pull request on the right line. For the other ways to write a script (`>`, a plain or quoted
scalar) it still reports the `run:` line, like actionlint. The column is always that of `run:`, because the indentation of the
script is not known to the parser; the script line and column from shellcheck are in the message (`SC2086:info:2:5:`).

Things which key on the line of a finding see this difference when moving from actionlint:

- **An ignore comment keeps working.** A `# jactionlint ignore=shellcheck` comment on the step (above it, or at the end of its first
  line), above the `run:` key, or at the end of the `run: |` line covers every line of the script, because a comment covers the
  line it belongs to and everything nested under it. A comment you moved to a script line to ignore one finding covers that line only.
- **An `ignore:` pattern or `--ignore` is not tied to a line.** It matches the message or the rule ID, so it is not affected.
- **A line-based tool needs the new lines.** A problem matcher or a script that keeps a list of `file:line` pairs written for
  actionlint has to be refreshed once. A [baseline](usage.md#baseline) written by jactionlint does not use line numbers.

## What is stricter than actionlint

The `correctness` profile also has bug detectors that actionlint does not: `unsound-ternary` (`a && b || c` where `b` can be
falsy), `workflow-run-names` (a `workflow_run` trigger naming a workflow that does not exist), `local-action-checkout` (a local
action used before the repository is checked out), `action-syntax` (the metadata of composite actions) and `dependabot-syntax`
(`dependabot.yml`). A repository that passed actionlint can have findings from them; turn each one off with `rules: {<id>: off}`.
`template-injection` also covers sinks beyond scripts (container options, the prompt of AI agent actions), and in the `default`
profile the free text that is chosen from outside the workflow (`inputs.*` of type string, `client_payload`, release names, `ref_name`).
`workflow-input-type` differs from actionlint in one point: a quoted `"true"` or `'1'` passed to a reusable workflow is a string,
so it can be passed to an input of the type `string` (actionlint 1.7 reports it as a boolean or a number).
A call of an overloaded function (`contains`) that fits no signature is one finding, about the signature that fits best, where actionlint reports one finding per signature.
A context that is not available at a place (`runner` in a job-level `env` key, `inputs` in the `shell` of a composite step) is one finding; actionlint also reports the properties read from it.
`workflow-syntax` accepts `needs: []` and a filter without a value (`tags:`), which GitHub runs and actionlint 1.7 reports as an empty section.

The `default` profile adds the security and policy rules, such as `unpinned-uses`, `missing-permissions`, `missing-timeout`,
`excessive-permissions`, `concurrency-limits`, `artipacked`, `cache-poisoning`, `dangerous-triggers` and `use-trusted-publishing`. The `pedantic` profile adds the noisy and opinionated
ones. Adopt them one at a time: pick `correctness`, then raise the profile to `default` with a [baseline](usage.md#baseline) that
hides today's findings and fails only on new ones.

## hk

[hk](usage.md#hk) runs jactionlint per step. To keep what actionlint gave you, pass the profile in the command:

```pkl
["jactionlint"] {
    glob = List(".github/workflows/*.yml", ".github/workflows/*.yaml")
    batch = true
    diagnostic_format = "sarif"
    check = "jactionlint --profile correctness --format sarif {{files}}"
    check_diff = "hk util sarif-diff -- jactionlint --profile correctness --format sarif {{files}}"
    fix = "jactionlint --profile correctness --fix {{files}}"
}
```

A second step without the flag, run in CI, can then use the profile of the configuration file.

[actionlint]: https://github.com/rhysd/actionlint
