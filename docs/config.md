# Configuration

Choose a profile, set rule levels and options, describe your runners and accept findings in `.github/jactionlint.yaml`. This page shows where jactionlint reads the config file and what every key does. jactionlint works without one.

## Configuration file

A config file is a YAML file at `.github/jactionlint.yaml`. Every key is optional, an empty file is valid and an unknown key is rejected. This example chooses a profile, changes the level and an option of some rules, declares runner labels and variables, silences a rule in some files and accepts one finding:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/jdx/jactionlint/main/schema/jactionlint-config.json
profile: default

rules:
  missing-timeout: warn       # lower the level of a rule the profile enables
  require-shell: error        # turn on a rule the profile does not enable
  max-run-lines:              # a rule with options takes a mapping
    level: warn
    max: 40

self-hosted-runner:
  labels: [linux.2xlarge, linux-multi-gpu]

config-variables: [DEFAULT_RUNNER, ENVIRONMENT_STAGE]

paths:
  .github/workflows/legacy-*.yaml:
    ignore:
      - missing-permissions

ignores:
  - rule: unpinned-uses
    uses: actions/checkout
    file: .github/workflows/release.yaml
    reason: pinned by an organization ruleset
```

The first line is a comment that gives editors the [JSON Schema](#json-schema) of the file, so they complete keys, rule IDs and rule options. Find what you want to do in this table:

| You want to | Set | Details |
| --- | --- | --- |
| Choose which rules run | `profile` | [Profiles](#profiles) |
| Change the level of a rule, turn it off or set its options | `rules` | [Rule levels and options](#rule-levels-and-options) |
| Silence rules in some files | `paths` | [Ignore findings in some files](#ignore-findings-in-some-files) |
| Accept a finding with a reason and an expiry date, in a way that survives edits of the workflow | `ignores` | [Durable ignores](#durable-ignores) |
| Describe your runners, variables and secrets, or a Forgejo or Gitea platform | `self-hosted-runner`, `config-variables`, `config-secrets`, `platform` | [Key reference](#key-reference) |
| Turn on the checks that query the GitHub API | `online`, `online-options` | [Online options](#online-options) |
| Accept the current findings and fix them step by step | `baseline` | [Baseline and fixes](#baseline-and-fixes) |
| Share settings between repositories | `extends`, or a user-global file | [Extending config files](#extending-config-files), [Where jactionlint reads the config file](#where-jactionlint-reads-the-config-file) |
| Replace old keys | `--migrate-config` | [Deprecated keys](#deprecated-keys) |

### Generate the initial configuration

`jactionlint --init-config` writes `.github/jactionlint.yaml` in the repository that holds the current directory and prints the path of the new file:

```sh
jactionlint --init-config
```

The file starts with the schema comment, explains the main keys in comments and sets nothing except empty `self-hosted-runner.labels`, `config-variables: null`, `config-secrets: null` and empty `paths` and `rules`. The command does not overwrite a file: when a config file under any name of the [repository config file](#where-jactionlint-reads-the-config-file) exists in `.github`, it prints `config file already exists at "<path>"` and exits with status 3.

The command needs a [project](#where-jactionlint-reads-the-config-file): a repository that has `.github/workflows`, `.github/actions` or an `action.yml` at its root. It also needs a `.github` directory and does not create one, so a project whose only marker is a root `action.yml` needs `.github` first. Without a project or without `.github`, the command exits with status 3.

To rewrite a file that uses [deprecated keys](#deprecated-keys), run `jactionlint --migrate-config`.

<a id="configuration-file-location-and-priority"></a>
## Where jactionlint reads the config file

jactionlint uses one config file per project and never merges these files. It uses the first of these that exists:

1. The file of `--config-file` (`-c`). It replaces both files below, and its `extends` entries still apply.
2. The repository config file: the first that exists of `.github/jactionlint.yaml`, `.github/jactionlint.yml`, `.github/actionlint.yaml` and `.github/actionlint.yml`.
3. The user-global config file: the first that exists of `jactionlint/jactionlint.yaml`, `jactionlint/jactionlint.yml`, `actionlint/actionlint.yaml` and `actionlint/actionlint.yml` in your user config directory.

The user config directory is `$XDG_CONFIG_HOME` when that variable holds an absolute path. Otherwise it is `$HOME/.config`, on every platform (`%USERPROFILE%\.config` on Windows), so the usual user-global config file is `$HOME/.config/jactionlint/jactionlint.yaml`. A relative `$XDG_CONFIG_HOME` is ignored, as the [XDG Base Directory specification](https://specifications.freedesktop.org/basedir-spec/latest/) requires.

The project of a linted file is the nearest parent directory that is a repository (it has `.git` or `.jj`) and has `.github/workflows`, `.github/actions` or an `action.yml` or `action.yaml` at its root. A monorepo has one config file at its root. A nested repository or submodule with its own workflows uses its own config file.

The user-global file applies to a project that has no repository config file, and to input that jactionlint reads from stdin, which belongs to a project only when `--stdin-filename` names an existing file in it. Use the user-global file for personal or organization defaults that many repositories share, or for a CI base image that needs a config without a file in every checkout. A repository config file always wins over the user-global file. To build on a shared file instead of replacing it, list it in [`extends`](#extending-config-files).

`--profile` applies on top of whichever file is used.

The names `actionlint.yaml` and `actionlint.yml` are the ones that actionlint reads, so an existing actionlint config keeps working. A file with such a name that sets no `profile` runs under the `default` profile, which has more rules than actionlint, and jactionlint prints a `note:` line that says so. [A config file written for actionlint](./actionlint.md#a-config-file-written-for-actionlint) explains the differences.

## Profiles

A profile is a named set of [rules](./rules.md) that run together. `profile` selects one, and each profile includes the rules of the one before it. When `profile` is omitted, the `default` profile runs. `--profile NAME` on the command line overrides the key for one run.

| Profile | Enables |
| --- | --- |
| `correctness` | What actionlint checks (syntax, expressions, actions, `workflow_call`, permissions, credentials, events, globs, runner labels, shell names, IDs, `needs`, matrix, `if:`, shellcheck, pyflakes) and the bug detectors of jactionlint, such as `unsound-ternary`, `workflow-run-names`, `local-action-checkout`, `action-syntax` and `dependabot-syntax`. |
| `default` | `correctness` plus the security and policy rules that are worth failing a build on, for example for pinned actions and images, permissions, timeouts, concurrency, dangerous triggers, artifact and cache poisoning, unverified downloads and trusted publishing. |
| `pedantic` | `default` plus the noisy and opinionated rules, such as `require-shell`, `max-run-lines`, `anonymous-definition`, `self-hosted-runner` and `unused-needs`, and the noisier findings of the rules that have the option `pedantic`. |

The `correctness` profile has almost no security or policy rules. These are the exceptions:

- `template-injection` (untrusted input in scripts, in `container:` options and in the prompts of AI agent actions) and `hardcoded-container-credentials`, which actionlint checks too.
- `expired-ignore` and `unused-baseline-entry`, which check your ignores and your baseline.

The profile of each rule is in the Profile column of [the rules page](./rules.md), in `jactionlint --list-rules` and in `jactionlint --explain RULE`. [Choose a profile](./usage.md#choose-a-profile) covers the first run on a repository, and the [policy for jactionlint's features](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md#policy-for-jactionlints-features) says how a rule gets into a profile.

These cases sit outside the plain rule sets of the profiles:

- The opt-in rules belong to no profile and run only when the config file turns them on. They report nothing until you give them their setting: `forbidden-uses` needs `allow`, `deny`, `images-allow` or `images-deny`, `required-actions` needs the `required-actions` list, and `timeout-too-long` needs its option `max`. Writing the setting turns the rule on at its default level: the option in the `rules` entry of the rule, or, for `required-actions`, the top-level list.
- The online rules do not follow a profile either. They run at their own level when the [online checks](./online.md#online-checks) are on.
- The option `pedantic` turns the noisier findings of a rule on or off. Unset, it is true under the `pedantic` profile and false otherwise, so `rules: {template-injection: {pedantic: true}}` reports the noisier findings of that rule under the `default` profile, and `pedantic: false` turns them off under the `pedantic` profile. [The rules page](./rules.md#rules) shows the option in the rules that have it.

`profile: strict` and `profile: all` in a config file are read as `pedantic` with a deprecation warning, and `--migrate-config` rewrites them (see [Deprecated keys](#deprecated-keys)). The command-line option does not accept them: `jactionlint --profile strict` exits with status 2.

<a id="rules"></a>
## Rule levels and options

`rules` sets the level and the options of each rule by its [rule ID](./rules.md). A level is one of these values:

| Value | Effect |
| --- | --- |
| `error` | jactionlint prints the finding and exits with status 1. |
| `warn` | jactionlint prints the finding with a `warning:` prefix. The exit status does not change unless you pass `--strict-exit`. `warning` is accepted as a spelling of `warn`. |
| `info` | The same as `warn`, with an `info:` prefix. |
| `off` or `false` | The rule does not run. |
| `true` | The rule runs at its default level. |

A rule that you do not list follows the profile: it runs at its default level when the profile includes it, and it is off otherwise. The default level belongs to the rule and is `error`, `warn` or `info`: see the Default level column of [the rules page](./rules.md). To run one rule once, whatever the profile says, use `--select RULE` ([Run one rule](./usage.md#run-one-rule-or-run-without-the-ignores)).

A rule with options takes a mapping with the optional key `level` and the options. A mapping without `level` turns the rule on at its default level, also when the profile does not include the rule:

```yaml
rules:
  missing-timeout: warn        # a level
  require-shell: off           # off, although the pedantic profile enables it
  max-run-lines:               # a level and an option
    level: warn
    max: 80
  timeout-too-long:            # options only: the rule runs at its default level
    max: 60
  unpinned-uses:               # an option that maps patterns to policies
    policies:
      actions/*: ref-pin
      my-org/*: any
```

[The rules page](./rules.md#rules) lists the options of each rule with their types and defaults, and `jactionlint --explain RULE` prints them under `Options (in "rules")`. An option has one of these types:

- `int`: a non-negative integer.
- `number`: a non-negative number. Decimals are accepted.
- `bool`: `true` or `false`.
- `strings`: a list of strings.
- `string-map`: a mapping from strings to strings.

jactionlint rejects an option that a rule does not have, and an unknown rule ID. The message suggests the closest valid name when there is one (see [When the config file is rejected](#when-the-config-file-is-rejected)).

<a id="ignoring-errors-by-rule-id"></a>
## Ignore findings in some files

`paths` maps a glob to the findings that jactionlint ignores in the files the glob matches. The glob is matched to the path relative to the repository root, with `/` as the separator, and it supports `**` and braces such as `*.{yml,yaml}` ([doublestar](https://github.com/bmatcuk/doublestar) syntax). The only key of an entry is `ignore`, a list of patterns:

```yaml
paths:
  .github/workflows/**/*.{yml,yaml}:
    ignore:
      - unpinned-uses
      - 'shellcheck reported issue in this script: SC2086:.+'
```

A pattern is a rule ID or a regular expression on the message of the finding: [Write a pattern](./ignore.md#write-a-pattern) describes the syntax, and [Ignore findings](./ignore.md#ignore-some-errors) helps you choose between `paths`, ignore comments and the other ways to ignore a finding. `--ignore` takes the same patterns for every file of one run. jactionlint rejects an invalid glob or regular expression, and `paths` cannot change the level of a rule for some files.

## Durable ignores

An ignore comment sits next to the line it covers, so a tool that rewrites the line can overwrite the comment. When Renovate or Dependabot bumps `uses: actions/checkout@<sha> # v4.1.0`, it replaces the trailing comment with the new version, and the ignore is gone. An entry of `ignores` does not have this problem: it names the rule and describes the place by what the workflow says, not by its position.

```yaml
ignores:
  - rule: unpinned-uses            # a rule ID, or a list of IDs: [unpinned-uses, artipacked]
    uses: actions/checkout         # the action, whatever its ref is
    file: .github/workflows/*.yaml # optional glob
    job: release                   # optional job ID
    step: checkout                 # optional step id or name
    reason: pinned by an organization ruleset
    expires: "2030-12-31"
```

| Key | Type | Meaning |
| --- | --- | --- |
| `rule` | string or list of strings | Required. A [rule ID](./rules.md) or a list of them. jactionlint rejects an unknown ID. |
| `file` | glob | Matches the path relative to the project root, and the path as printed. `**` crosses directories, and the separator is always `/`. |
| `job` | string | The ID of the job (its key under `jobs`), without regard to case. Matches findings anywhere in the job, including its steps. |
| `step` | string | The `id` or the `name` of the step, compared as written. Matches findings anywhere in the step. |
| `uses` | string | The `uses:` value of the step, or of the job when it calls a reusable workflow. See [Forms of `uses`](#forms-of-uses). |
| `reason` | string | Why the finding is accepted. It is free text for the readers of the file, and it appears in the messages about the entry. |
| `expires` | string `YYYY-MM-DD` | The entry suppresses through the end of that day (UTC) and stops after it. Write the date in quotes. |

An entry needs at least one of `file`, `uses`, `job` and `step`. To turn a rule off everywhere, use [`rules`](#rule-levels-and-options). Every key that an entry sets must match, and entries are not combined: two entries are two independent reasons to ignore.

### Forms of `uses`

`uses` takes one of three forms:

- A name pattern like the patterns of [`forbidden-uses`](./checks.md#check-forbidden-uses): `actions/checkout` (the root action of the repository), `actions/*` (every action of the owner, including the ones in directories), `owner/repo/*` and `owner/repo/sub`. Names are case-insensitive. Without `@ref`, any ref matches, so the entry keeps working when a tool changes the tag or the SHA. A `@ref` must match the ref of the step, and it accepts `*` wildcards: `actions/checkout@v4` matches only that ref, and `actions/checkout@v*` matches every tag that starts with `v`. Refs are case-sensitive.
- A glob with `*` for values that are not repository references: `docker://alpine*` and `./.github/actions/*`. A local path can be written `./path` or `$/path`, and the entry matches both spellings.
- A regular expression between slashes, matched to the whole `uses:` value and not anchored unless you write `^` or `$`: `/^actions\/(checkout|cache)@/`. The syntax is [RE2](https://golang.org/s/re2syntax).

### Which job and step a finding belongs to

jactionlint finds the job and the step whose YAML block contains the line of the finding, the same blocks that an [ignore comment](./ignore.md#ignore-some-errors) covers:

- A finding on the `uses:` line of a step belongs to that step and its job.
- A finding on the `jobs.<id>` line or on the `runs-on` of a job belongs to the job and to no step.
- A finding about the workflow as a whole (`permissions`, `on`) belongs to neither, so only `rule` and `file` can match it.
- When the structure is ambiguous, for example two steps in one flow-style line (`steps: [{uses: a}, {uses: b}]`), the step is unknown, and an entry that asks for `step` or `uses` does not match. An ignore never suppresses on a guess.
- The steps of a `parallel:` group are steps like the others. An entry finds them by their `uses` and by their `id` or `name`.

### Expiry and unused entries

- On the day after `expires`, the entry stops suppressing: the findings it hid come back. The entry itself is reported as [`expired-ignore`](./rules.md#expired-ignore) (level `error`, in the `correctness` profile) at its position in the config file, with its `reason`. Fix the findings or renew the date.
- During the last 14 days, the entry still works and is reported as `expired-ignore` with level `info`, so you are warned before the findings come back. An entry that is about to expire and suppresses nothing is also reported as `unused-ignore`.
- An entry that suppressed nothing is reported as [`unused-ignore`](./rules.md#unused-ignore) at its position in the config file. This is the rule that also reports unused ignore comments, so it is in the `pedantic` profile.
- A pre-commit hook lints only the changed files, so an entry is called unused only when every file it could apply to was linted in the run. A run that lints a single file never reports an entry that also applies to others.
- An entry for a rule that did not run is not unused: the rule is off, it is an online rule and the online checks are off, or it is `shellcheck` or `pyflakes` without its command.
- `--ignore expired-ignore` hides the findings about entries like any other finding, and `--no-ignores` applies no entry, so it reports none.

Entries of the files listed in `extends` are added to the entries of the file that extends them, extended files first. A finding about an entry is reported in the file that contains it.

## Key reference

Every top-level key of the config file is in this table with its type, its default, the command-line option that does the same and its effect. A key with sub-keys has a table of its own in the group after this table. The old keys that `rules` replaced are in [Deprecated keys](#deprecated-keys).

| Key | Type | Default | Command-line equivalent | Effect |
| --- | --- | --- | --- | --- |
| `profile` | `correctness`, `default` or `pedantic` | `default` | `--profile` | Chooses which rules run. See [Profiles](#profiles). |
| `extends` | list of paths | none | none | Config files to inherit from. See [Extending config files](#extending-config-files). |
| `rules` | mapping from rule ID to a level or a mapping | the profile decides | none | Sets the level and the options of each rule. See [Rule levels and options](#rule-levels-and-options). |
| `paths` | mapping from a glob to `{ignore: [...]}` | none | `--ignore`, for every file | Ignores rule IDs or messages in the files that match a glob. See [Ignore findings in some files](#ignore-findings-in-some-files). |
| `ignores` | list of mappings | none | none | Accepts findings by rule and place, with a reason and an expiry date. See [Durable ignores](#durable-ignores). |
| `self-hosted-runner` | mapping | none | none | Describes your runner labels. See [Runners](#runners). |
| `config-variables` | list of strings, or `null` | `null` (no check) | none | The names that the `vars` context may use. See [Variables and secrets](#variables-and-secrets). |
| `config-secrets` | list of strings, or `null` | `null` (no check) | none | The names that the `secrets` context may use. See [Variables and secrets](#variables-and-secrets). |
| `required-actions` | list of mappings | empty (no check) | none | Actions that every workflow must use. See [Required actions](#required-actions). |
| `assume-default-permissions` | `restricted` or `permissive` | unset | none | The token permissions to assume for a caller that sets none. See [Permissions](#permissions). |
| `platform` | `github`, `forgejo` or `gitea` | `github` | none | The system that runs the workflows. See [Platform](#platform). |
| `generated-files` | `report`, `skip-policy` or `skip` | `skip-policy` | none | What to do with files that a tool wrote. See [Generated files](#generated-files). |
| `shellcheck` | mapping | none | `--shellcheck-rcfile`, for `rcfile` | How shellcheck runs. See [Options for shellcheck](#options-for-shellcheck). |
| `online` | boolean | `false` | `--online`, `--no-online`, `JACTIONLINT_ONLINE` | Turns on the online checks. See [Online options](#online-options). |
| `online-options` | mapping | see the table | `--online=MODE` and the `--online-*` options | Tunes the online checks. See [Online options](#online-options). |
| `baseline` | `auto`, a boolean or a path | none | `--baseline`, `--no-baseline` | Hides the findings recorded in a baseline file. See [Baseline and fixes](#baseline-and-fixes). |
| `fix` | mapping | none | `--fix-rules`, for `rules` | Limits the fixes. See [Baseline and fixes](#baseline-and-fixes). |

[Command line](./cli.md#command-line-options) describes each option. When a setting has a command-line option, the option wins over the config file.

### Runners

`self-hosted-runner` tells jactionlint which labels your runners have, so that `runs-on` does not report them as unknown.

| Key | Type | Default | Effect |
| --- | --- | --- | --- |
| `self-hosted-runner.labels` | list of glob patterns | empty | The labels of your self-hosted runners. The syntax is the one of [`path.Match`](https://pkg.go.dev/path#Match). The [runner label check](./checks.md#check-runner-labels) accepts them, and the [`self-hosted-runner` rule](./checks.md#check-self-hosted-runner) treats a job that uses one as running on a self-hosted runner. |
| `self-hosted-runner.strict-labels` | boolean | `false` | When true, `runs-on` accepts only the labels listed in `labels`. |
| `self-hosted-runner.providers` | list of provider names | all providers | The hosted-runner providers whose labels `runs-on` accepts without a listing in `labels`. |

With `strict-labels: true`, the built-in labels are reported unless you list them too. These are the GitHub-hosted runner labels such as `ubuntu-latest` and the preset self-hosted labels such as `self-hosted`, `linux` and `x64`. Use it when every job must run on your own runners, or when your runners (for example the runner sets of Actions Runner Controller) do not carry the `self-hosted` label. The checks for conflicting labels still apply to listed built-in labels.

The labels of a hosted-runner service are not GitHub labels, so without `providers` each of them would be an unknown label. When the key is unset, jactionlint accepts the labels of all providers in the table. A list accepts only the providers in it, and an empty list accepts none. Names are case-insensitive.

A service that is not in the table goes in `labels`. `providers` has no effect when `strict-labels` is true.

| Provider | Labels it accepts |
| --- | --- |
| `blacksmith` | `blacksmith-*` |
| `depot` | `depot-ubuntu-*`, `depot-windows-*`, `depot-macos-*` |
| `namespace` | `namespace-profile-*`, `nscloud-*`, `namespace-features:*` |
| `runson` | `runs-on=...`, `runs-on/...`, `runner=...`, `family=...`, `image=...` and the other label keys of RunsOn |
| `ubicloud` | `ubicloud`, `ubicloud-arm`, `ubicloud-standard-*`, `ubicloud-premium-*` |
| `warpbuild` | `warp-*` |

### Variables and secrets

`config-variables` and `config-secrets` list the names that exist in your repository or organization, so that jactionlint can report a name that does not.

| Key | Type | Default | Effect |
| --- | --- | --- | --- |
| `config-variables` | list of strings, or `null` | `null` | With a list, jactionlint checks the `vars` context against it and reports a name that is not listed. An empty list allows no variable. `null`, or no key, turns the check off. See [GitHub's documentation of variables](https://docs.github.com/en/actions/learn-github-actions/variables). |
| `config-secrets` | list of strings, or `null` | `null` | The same for the `secrets` context. `GITHUB_TOKEN` is always allowed. The check applies only when the workflow does not declare its secrets itself (for example `secrets:` under `on.workflow_call`), because declared secrets are checked against their declaration. See [GitHub's documentation of secrets](https://docs.github.com/en/actions/security-guides/using-secrets-in-github-actions). |

Both checks report the rule `undefined-property`, which also reports other undefined properties of contexts. To switch the two checks off, remove the keys or set them to `null`.

### Required actions

`required-actions` lists the actions or reusable workflows that every workflow must use. The `required-actions` rule is [opt-in](#profiles): it runs when the list is not empty. A missing action is reported once per workflow, at the position of its first job.

```yaml
required-actions:
  - action: actions/checkout
  - action: github/codeql-action/init
    version: v3
```

| Key | Type | Default | Effect |
| --- | --- | --- | --- |
| `required-actions[].action` | string | required | The name of the action, such as `actions/checkout`, without `@version`. A sub-path is part of the name (`github/codeql-action/init`). The name is case-insensitive, and reusable workflow calls (`jobs.<id>.uses`) are matched too. A value with `@`, a local path (`./...` or `$/...`), a Docker image (`docker://...`) or a name without `/` is rejected. |
| `required-actions[].version` | string | any version | The ref (tag, branch or commit SHA), compared exactly. The requirement is satisfied when at least one use of the action has this ref. |

Local actions and Docker images never match. Only workflow files are checked. The steps of composite action files (`action.yml`) are not ([composite actions](./checks.md#check-composite-actions)).

### Permissions

`assume-default-permissions` tells the check for reusable workflow calls which token permissions to assume for a caller that has no `permissions:` block at the workflow level and none on the calling job. The permissions of such a caller come from the repository setting (Settings, Actions, General, Workflow permissions), which jactionlint cannot read from a workflow file.

| Value | Effect |
| --- | --- |
| unset | jactionlint reports only a scope that no default token has, such as `id-token`, which always needs an explicit opt-in. The findings are the ones of `permissive`, and the message says that no default token has the permissions. |
| `restricted` | Assumes GitHub's restricted default token (`contents: read` and `packages: read`, every other scope `none`). jactionlint reports every other scope that the called workflow needs. |
| `permissive` | Assumes the permissive default (write on every scope). `id-token` is still treated as `none`. |

The key affects only callers that have no `permissions:` block anywhere. When a caller declares any block, even `permissions: {}`, the check runs against that block, because GitHub treats a scope that an explicit block omits as `none`. The rule is `workflow-call-permissions`; see [Reusable workflows](./checks.md#check-reusable-workflows).

### Platform

`platform` names the system that runs the workflows: `github` (the default), `forgejo` or `gitea`. jactionlint treats `forgejo` and `gitea` alike. On both, a step can run an action from any host by the URL of its repository, as in `uses: https://code.forgejo.org/actions/checkout@v4` ([actionlint#698](https://github.com/rhysd/actionlint/issues/698)). On GitHub that is a malformed `uses:` and stays a finding, so the switch is off unless you set it. With `forgejo` or `gitea`, jactionlint treats such a value like this:

- It checks the form `https://{host}/{owner}/{repo}[/{path}]@{ref}`. A value without a host, an owner, a repository or a ref is reported as `invalid-uses`.
- It applies the pinning policy of [`unpinned-uses`](./rules.md#unpinned-uses) to `{owner}/{repo}`, whatever the host is.
- [`forbidden-uses`](./rules.md#forbidden-uses) and [`required-actions`](./rules.md#required-actions) read it as `{owner}/{repo}[/{path}]@{ref}`, too. These rules look at the repository, as `unpinned-uses` does, and not at the host. So `allow: ['actions/*']` allows `https://code.forgejo.org/actions/checkout@v4`, and that value satisfies `required-actions: [{action: actions/checkout}]`.
- It does not look the action up in the data about GitHub actions (inputs, outdated runtimes), and the online checks do not apply to it: an action at another host is another action. The other rules that read a `uses:` value (typosquats, popular actions, cache and artifact checks) do not know the value.

A reusable workflow call (`jobs.<id>.uses`) is not covered.

### Generated files

`generated-files` tells jactionlint what to do with a workflow file whose first comment says that a tool wrote it, such as `# This file was automatically generated by gh-aw. DO NOT EDIT.`, `# generated by praktika` or `# Generated by projen`. The author of such a file fixes the generator, not the file.

| Value | Effect |
| --- | --- |
| `skip-policy` (default) | The rules of the `policy` and `style` groups (`missing-timeout`, `unused-job-output`, `anonymous-definition`, `unpinned-uses` and so on) do not report in the file. The `correctness` and `security` rules still do. |
| `report` | The file is treated like any other file. |
| `skip` | Nothing is reported in the file. |

Only the comment block at the top of the file counts, within its first 40 lines. The comment must be a generator marker: it starts with a wording such as `automatically generated`, `generated by` or `managed by modulesync`, or it is only `DO NOT EDIT`. A sentence such as `# do not edit the matrix by hand` or `# generated by hand` is not a marker. `unused-ignore` does not flag an ignore comment or an `ignores` entry for a rule that is not reported in such a file.

<a id="shellcheck"></a>
### Options for shellcheck

`shellcheck` tunes how jactionlint runs shellcheck for the scripts of `run:` steps ([shellcheck integration](./checks.md#check-shellcheck-integ)). By default jactionlint passes `--norc`, so a `.shellcheckrc` in the working directory cannot change the results. Use the keys when you want an rc file or more arguments in every run, for example where you cannot set `SHELLCHECK_OPTS`, which shellcheck still reads (pre-commit.ci is one such place). The option `--shellcheck-rcfile` overrides `rcfile`.

| Key | Type | Default | Effect |
| --- | --- | --- | --- |
| `shellcheck.rcfile` | path | none | A shellcheckrc file to read instead of `--norc`. A relative path is resolved from the repository root, and the file must exist, or the run exits with status 3. A repository config file can name only a file inside the repository. The user-global file and `--config-file` can name any file. The settings of the rc file (`disable=SC2086`, `enable=...`, `source-path=...`) apply. |
| `shellcheck.args` | list of strings | empty | More arguments for shellcheck, such as `--exclude=SC2129`, `--severity=warning` or `-eSC2129`. Write one option per item and attach its value with `=`: an item is never a file name. |
| `shellcheck.enable` | list of strings | empty | Optional checks of shellcheck to turn on, like `--enable=NAME,NAME`. `all` turns on every optional check. A name is lowercase letters, digits and dashes. |

jactionlint sets the output format, the shell of each script, `-x` and the script itself. So `args` rejects `--format`, `--color`, `--shell`, `--norc`, `--rcfile`, `--version` and `--wiki-link-count`, and the short forms `-f`, `-C`, `-s`, `-V` and `-W`.

jactionlint always passes `-e SC1091,SC2194,SC2050,SC2153,SC2154,SC2157,SC2043`, the codes that give false findings in workflow scripts: a file that exists only in CI, a variable that `env:` defines, and the placeholder that replaces `${{ }}`. An `--exclude` in `args` adds to them. An `--include` in `args` makes shellcheck report only the codes it names, and that includes a code from the built-in list, which it brings back.

```yaml
shellcheck:
  rcfile: .github/shellcheckrc
  enable: [avoid-nullary-conditions, require-variable-braces]
```

### Online options

`online` turns on the [online checks](./online.md#online-checks), which query the GitHub API, for the files that the config file applies to. The default is `false`: nothing uses the network. `online-options` tunes them, and every sub-key is optional. The keys apply to the whole run and come from the configuration of the first file for which the online checks run.

A setting has this priority: a command-line option, then the environment variable `JACTIONLINT_ONLINE` (`1`, a mode or `0`: see [the accepted values](./cli.md#variables-that-jactionlint-reads)), then the config file. `--no-online` and `JACTIONLINT_ONLINE=0` turn the checks off whatever the file says. A `mode` of `cache`, `strict` or `cache,strict` also turns the online checks on, unless the file says `online: false` explicitly: an explicit `false` wins over the mode.

| Key | Type | Default | Command-line equivalent | Effect |
| --- | --- | --- | --- | --- |
| `online-options.mode` | `cache`, `strict` or `cache,strict` | none (plain online mode) | `--online=MODE` | `cache` never uses the network and answers from the cache. `strict` makes a skipped lookup exit with status 3. A config file accepts exactly these three spellings, while `--online` also accepts other orders. |
| `online-options.api-url` | URL | `$GITHUB_API_URL`, else derived from `$GITHUB_SERVER_URL` or `$GH_HOST`, else `https://api.github.com` | `--online-api-url` | The REST API of a GitHub Enterprise Server, such as `https://ghe.example.com/api/v3`. The URL must not contain credentials. jactionlint sends no token to a host that the config file of a repository names here. It sends one to a host that the environment, `--online-api-url`, the user-global file or `--config-file` names. |
| `online-options.token-env` | name | none | `--online-token-env` | The environment variable that holds the token. It is read before `GITHUB_TOKEN` and `GH_TOKEN`. |
| `online-options.token-file` | path | none | `--online-token-file` | The file that holds the token. It is read after `token-env`. |
| `online-options.github-com-fallback` | boolean | `false` | `--online-github-com-fallback` | On a GitHub Enterprise Server, also asks `api.github.com` about the repositories that the Server does not have. The names of those repositories are sent to github.com: list internal ones in `deny`. |
| `online-options.github-com-token-env` | name | `GITHUB_COM_TOKEN` | `--online-github-com-token-env` | The environment variable that holds the token for `api.github.com` in the fallback. jactionlint reads the variable that you name first, then `github-com-token-file`, then `GITHUB_COM_TOKEN`. |
| `online-options.github-com-token-file` | path | none | `--online-github-com-token-file` | The file that holds the token for `api.github.com` in the fallback. It is read after `github-com-token-env`. |
| `online-options.allow` | list of `owner/repo` patterns | empty (all repositories) | `--online-allow` | The repositories that may be looked up. `*` is a wildcard within a part (`mycorp/*`, `*/setup-*`), and case does not matter. |
| `online-options.deny` | list of `owner/repo` patterns | empty | `--online-deny` | The repositories that may not be looked up, such as internal actions that GitHub cannot answer for. `deny` wins over `allow`. |
| `online-options.cache-ttl` | duration | `1h` | `--online-cache-ttl` | How long a cached answer is used without asking GitHub again. `0s` revalidates every answer. |
| `online-options.max-rate-limit-wait` | duration | `30s` | `--online-max-wait` | The longest jactionlint waits for a rate limit to reset. `0s` never waits. |
| `online-options.retries` | integer from 0 to 10 | `2` | none | How often a request is repeated after a server error or a secondary rate limit. |
| `online-options.concurrency` | integer from 0 to 32 | `6` | none | How many requests are in flight at most. `0` selects the default. |
| `online-options.gh-cli` | boolean | `true` | none | Asks `gh auth token` for a token when no variable or file has one. |

A duration is written in Go syntax, such as `90s`, `15m` or `1h`. [Online checks](./online.md#online-checks) covers the token order, the cache and the behavior with a GitHub Enterprise Server.

```yaml
online: true
online-options:
  deny: ["mycorp/*"]   # internal actions GitHub cannot answer for
  max-rate-limit-wait: 10s
```

### Baseline and fixes

| Key | Type | Default | Command-line equivalent | Effect |
| --- | --- | --- | --- | --- |
| `baseline` | `auto`, a boolean or a path | none (no baseline) | `--baseline[=FILE]`, `--no-baseline` | Hides the findings recorded in a [baseline file](./baseline.md#baseline), so that a repository adopts the checks step by step. `auto` or `true` applies `.github/jactionlint-baseline.json` when that file exists. `false`, or no key, applies no baseline. Any other value is the path of the baseline file relative to the repository root, and the file must exist, or the run exits with status 3. `jactionlint --baseline-write` writes the file. |
| `fix.rules` | list of rule IDs | empty (every fix) | `--fix-rules` | `--fix` and `--diff` apply only the fixes of these rules. jactionlint rejects an unknown ID. See [Fix findings automatically](./fix.md#fix-errors-automatically). |

## Extending config files

`extends` lists config files to inherit from. It makes an organization-wide configuration possible: keep a shared file in a repository or a submodule, and extend it from every project.

```yaml
extends:
  - ../shared/jactionlint.yaml
  - /etc/jactionlint/org.yaml
profile: pedantic
rules:
  require-shell: off
```

- A relative path is resolved from the directory of the file that lists it, not from the working directory. An absolute path is accepted.
- Later files win over earlier ones, and the file itself wins over all of them. A key that a later file does not write keeps the value of the earlier files.
- A listed file can extend other files, up to ten levels deep. jactionlint rejects a cycle and a deeper chain.
- With `--config-file`, a listed file that does not exist stops the run with exit status 3. Without that option, jactionlint treats a repository or user-global config file that lists a missing file as if it did not exist: none of its keys apply, jactionlint falls back to the next file of [the lookup order](#where-jactionlint-reads-the-config-file) or to the defaults, and it prints no message. Run `jactionlint --config-file .github/jactionlint.yaml` once after you add an entry: it exits with status 3 and names the missing file.

The keys merge as follows when more than one file writes them:

| Key | Merge |
| --- | --- |
| `profile`, `online`, `baseline`, `platform`, `generated-files`, `assume-default-permissions`, `config-variables`, `config-secrets`, `required-actions` | The later value replaces the earlier one, an empty list included. |
| `rules` | Merged by rule ID. The entry of the later file replaces the whole entry of the earlier file, so its level and its options replace the earlier ones. |
| `paths` | Merged by glob. The entry of the later file replaces the entry with the same glob. |
| `self-hosted-runner`, `shellcheck`, `fix`, `online-options` | Merged by sub-key (`labels`, `strict-labels` and `providers`; `rcfile`, `args` and `enable`; `rules`; each key of `online-options`). A sub-key that a later file writes replaces the earlier value, a list included. |
| `ignores` | Concatenated, the extended files first. |
| `extends` | Not inherited. |

## Deprecated keys

The keys in the table below, which the first config files used, and the profile names `strict` and `all` are replaced by `profile` and `rules`. They still work: jactionlint translates them and prints one deprecation warning on stderr for each deprecated key. With `--format sarif`, the warnings are in `toolConfigurationNotifications` of the log, and stderr stays empty. The messages show the path of the config file as you gave it to `--config-file`, and the absolute path of a file that jactionlint finds on its own. For example, `jactionlint --config-file .github/jactionlint.yaml` prints this warning for `require-commit-hash`:

```
warning: config file ".github/jactionlint.yaml": "require-commit-hash" is deprecated and will be removed in a future version. use "rules: {unpinned-uses: error}" instead. run "jactionlint --migrate-config" to rewrite the file
```

| Deprecated key | Replacement |
| --- | --- |
| `profile: strict`, `profile: all` | `profile: pedantic` |
| `require-commit-hash: true` | `rules: {unpinned-uses: error}` |
| `require-permissions: true` | `rules: {missing-permissions: error}` |
| `require-checkout-before-local-action: true` | `rules: {local-action-checkout: error}` (the `correctness` profile enables it) |
| `require-expression-wrapping: true` | `rules: {require-expression-wrapping: error}` |
| `check-falsy-ternary: true` | `rules: {unsound-ternary: error}` (the `correctness` profile enables it) |
| `check-workflow-run-names: true` | `rules: {workflow-run-names: error}` (the `correctness` profile enables it) |
| `require-shell: true` | `rules: {require-shell: error}` |
| `max-run-lines: N` | `rules: {max-run-lines: {level: error, max: N}}` |
| `timeout-minutes: {required: true}` | `rules: {missing-timeout: error}` |
| `timeout-minutes: {required: false}` | `rules: {missing-timeout: off}` |
| `timeout-minutes: {max: N}` | `rules: {timeout-too-long: {level: error, max: N}}` (`missing-timeout` is left to the profile) |

Setting a boolean key to `false`, or `max-run-lines: 0`, turns the rule off. That is how an old file disables `local-action-checkout`, `unsound-ternary` and `workflow-run-names`, which the `correctness` profile enables. A rule that you write in `rules` wins over the deprecated key.

`jactionlint --migrate-config` rewrites the deprecated keys of the repository config file, or of the file of `--config-file`, in place. It keeps the comments and the other keys, removes blank lines and adds the new rules to `rules`, which it creates at the position of the first deprecated key when the file has none. A rule with options is written as a one-line mapping, and a rule that is already in `rules` keeps its entry.

The command prints what it did and exits with status 0. It does not rewrite the files that `extends` lists: run it with `--config-file` on each of them. Without a config file it exits with status 3. The example passes the file with `--config-file`, and a file that jactionlint finds on its own is shown with its absolute path in the message. Before:

```yaml
profile: strict   # the old name of pedantic

# Pin every action.
require-commit-hash: true

max-run-lines: 40

timeout-minutes:
  required: true
  max: 30
```

Run the command:

```sh
jactionlint --migrate-config --config-file .github/jactionlint.yaml
```

It prints:

```
Config file ".github/jactionlint.yaml" was migrated. Replaced the deprecated keys with "rules": require-commit-hash, max-run-lines, timeout-minutes, profile
```

After:

```yaml
profile: pedantic # the old name of pedantic
# Pin every action.
rules:
  unpinned-uses: error
  max-run-lines: {level: error, max: 40}
  missing-timeout: error
  timeout-too-long: {level: error, max: 30}
```

A second run changes nothing and prints:

```
Config file ".github/jactionlint.yaml" has no deprecated key. Nothing was changed
```

Rule IDs that were merged into another rule are retired, and [the rules page](./rules.md#retired-rule-ids) lists them. `rules`, `ignores` and `fix.rules` refuse a retired ID with a message that names the new place. `--ignore`, `paths` and ignore comments still accept it with a deprecation warning, and the ignore covers only the findings that the old rule reported.

## JSON Schema

The config file has a [JSON Schema](https://json-schema.org/) (draft-07), generated from the rule registry. The rule IDs are the only keys allowed under `rules` and an enum in `fix.rules` and `ignores`, each rule lists its own options, and the other keys are described. Use it for completion and for checking the file in an editor or in CI:

- In the file, add the comment that `--init-config` writes at the top. Editors that use [yaml-language-server](https://github.com/redhat-developer/yaml-language-server) (VS Code with the YAML extension, Neovim, Helix and others) then complete and check the keys, the rule IDs and the options of each rule:

  ```yaml
  # yaml-language-server: $schema=https://raw.githubusercontent.com/jdx/jactionlint/main/schema/jactionlint-config.json
  ```

- Print the schema of the installed version, which always matches the rules of that binary: `jactionlint --print-config-schema > jactionlint-config.json`.
- In CI, validate the file with any JSON Schema validator, for example `check-jsonschema --schemafile jactionlint-config.json .github/jactionlint.yaml`.

The committed copy is [`schema/jactionlint-config.json`](https://github.com/jdx/jactionlint/blob/main/schema/jactionlint-config.json). The parser of jactionlint is the authority: where the schema and the parser differ, the parser is right, and the difference is a bug of the schema. The findings of `--format json` have a schema too: see [JSON schema of the output](./output.md#json-schema-of-the-output).

## When the config file is rejected

jactionlint checks the whole config file before it lints anything. A file that cannot be read or parsed stops the run with [exit status 3](./cli.md#exit-status), and nothing is linted. Deprecation warnings do not stop the run. Parsing is strict at every level: an unknown key is rejected in the top-level mapping, in `self-hosted-runner`, `online-options`, `shellcheck`, `fix`, `paths`, `ignores`, `required-actions` and in the options of a rule. A message about the content of the file starts with `could not parse config file` and the path of the file, and ends with the cause. The causes read like these:

| Cause | The message ends with |
| --- | --- |
| A misspelled key | `unknown key "self-hosted-runnr" in the configuration at line 1, column 1. did you mean "self-hosted-runner"?` |
| An option that the rule does not have | `unknown key "max" in the options of rule "unpinned-uses" at line 3, column 5. available keys are "actions-lock", "level", "policies", "short-pin-comment"` |
| An unknown rule ID | `unknown rule ID "unpinned-use" in "rules" at line 2, column 3. did you mean "unpinned-uses"? see https://jactionlint.jdx.dev/rules for all rule IDs` |
| A level that does not exist | `yaml: construct errors: line 2: invalid severity "fatal". available values are "off", "info", "warn" and "error" at line 2, column 20` |
| A value of the wrong type | `invalid value 1.5 for option "max" of rule "max-run-lines" in "rules": it must be a non-negative integer` |
| A value outside the allowed set | `invalid value "bitbucket" for "platform". available values are "github", "forgejo" and "gitea"` |

A file that the config names must exist too. The run exits with status 3 for a missing `baseline` file, a missing `shellcheck.rcfile`, and for a missing `extends` file when you pass `--config-file` (see [Extending config files](#extending-config-files)). `--init-config` and `--migrate-config` exit with status 3 when they cannot do their work.

## Next steps

- [Ignore findings](./ignore.md#ignore-some-errors): ignore comments, `--ignore` and how to choose between the ways to ignore a finding.
- [Adopt the checks step by step](./baseline.md#baseline): record today's findings in a baseline.
- [Online checks](./online.md#online-checks): tokens, the cache and GitHub Enterprise Server.
- [Rules](./rules.md): every rule with its group, level, profile and options.
- [Command line](./cli.md#command-line-options): every option, and the exit status.
