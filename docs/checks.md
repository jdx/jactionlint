All checks done by jactionlint
=============================

This document describes all checks done by [jactionlint](https://github.com/jdx/jactionlint) with example inputs, outputs, and playground links.

List of checks:

- [Unexpected keys](#check-unexpected-keys)
- [Missing required keys or key duplicates](#check-missing-required-duplicate-keys)
- [Unexpected empty mappings](#check-empty-mapping)
- [Unexpected mapping values](#check-mapping-values)
- [Syntax check for expression `${{ }}`](#check-syntax-expression)
- [Type checks for expression syntax in `${{ }}`](#check-type-check-expression)
- [Contexts and built-in functions](#check-contexts-and-builtin-func)
- [Contextual typing for `steps.<step_id>` objects](#check-contextual-step-object)
- [Contextual typing for `matrix` object](#check-contextual-matrix-object)
- [Contextual typing for `needs` object](#check-contextual-needs-object)
- [Strict type checks for comparison operators](#check-comparison-types)
- [shellcheck integration for `run:`](#check-shellcheck-integ)
- [pyflakes integration for `run:`](#check-pyflakes-integ)
- [Script injection by potentially untrusted inputs](#untrusted-inputs)
- [Job dependencies validation](#check-job-deps)
- [Parallel steps](#check-parallel-step-refs)
- [Timeout minutes of jobs](#check-timeout-minutes)
- [Workflow names of `workflow_run` event](#check-workflow-run-names)
- [Matrix values](#check-matrix-values)
- [Webhook events validation](#check-webhook-events)
- [Workflow dispatch event validation](#check-workflow-dispatch-events)
- [Glob filter pattern syntax validation](#check-glob-pattern)
- [CRON syntax and IANA timezone string at `on.schedule`](#check-cron-syntax-and-timezone)
- [Runner labels](#check-runner-labels)
- [Action format in `uses:`](#check-action-format)
- [Local action inputs validation at `with:`](#check-local-action-inputs)
- [Local action used before checkout](#check-local-action-checkout)
- [Popular action inputs validation at `with:`](#check-popular-action-inputs)
- [Outdated popular actions detection at `uses:`](#detect-outdated-popular-actions)
- [Shell name validation at `shell:`](#check-shell-names)
- [Run script policy (pedantic)](#check-run-policy)
- [Job ID and step ID uniqueness](#check-job-step-ids)
- [Hardcoded credentials](#check-hardcoded-credentials)
- [Dangerous writes to `GITHUB_ENV` and `GITHUB_PATH`](#check-github-env)
- [Packages installed by name](#check-adhoc-packages)
- [Tools installed without an exact version](#check-unpinned-tools)
- [Publishing with long-lived credentials](#check-use-trusted-publishing)
- [Superfluous actions](#check-superfluous-actions)
- [Installs without a lock file](#check-unlocked-install)
- [Environment variable names](#check-env-var-names)
- [Permissions](#permissions)
- [Reusable workflows](#check-reusable-workflows)
- [ID naming convention](#id-naming-convention)
- [Availability of contexts and special functions](#ctx-spfunc-availability)
- [Deprecated workflow commands](#check-deprecated-workflow-commands)
- [Constant conditions at `if:`](#if-cond-constant)
- [Action metadata syntax validation](#action-metadata-syntax)
- [Deprecated inputs usage](#deprecated-inputs-usage)
- [YAML anchors](#yaml-anchors)
- [Dependabot configuration syntax](#check-dependabot-syntax)
- [Dependabot cooldown](#check-dependabot-cooldown)
- [Dependabot insecure code execution](#check-dependabot-execution)
- [Dependabot updates of actions (pedantic)](#check-dependabot-missing-actions-update)
- [Pipelines that hide failures](#check-pipeline-without-pipefail)
- [Workflow and job names (pedantic)](#check-anonymous-definition)
- [Concurrency limits](#check-concurrency-limits)
- [Inherited secrets](#check-secrets-inherit)
- [Insecure workflow commands](#check-insecure-commands)
- [Unverified downloads](#check-unverified-download)
- [Host keys collected with ssh-keyscan](#check-insecure-ssh-keyscan)
- [Static credentials for actions/checkout](#check-checkout-static-credentials)
- [Insecure URL schemes](#check-insecure-url-scheme)
- [Dangerous triggers](#check-dangerous-triggers)
- [Self-hosted runners (pedantic)](#check-self-hosted-runner)
- [Unsound `contains()` on a string](#check-unsound-contains)
- [Overprovisioned secrets](#check-overprovisioned-secrets)
- [Unredacted secrets](#check-unredacted-secrets)
- [Secrets outside an environment (pedantic)](#check-secrets-outside-env)
- [Typosquatting of actions](#check-typosquat-uses)
- [Forbidden actions (opt-in)](#check-forbidden-uses)
- [Inputs, payloads, release names and branch names](#check-template-injection-inputs)
- [Expansions in scripts (pedantic)](#check-template-injection-expansion)
- [AI agent actions](#check-agentic-actions)
- [Bots trusted by `github.actor`](#check-bot-conditions)
- [Obfuscated paths and expressions](#check-obfuscation)
- [Misfeatures](#check-misfeature)
- [Impostor commits (online)](#check-impostor-commit)
- [Known vulnerable actions (online)](#check-known-vulnerable-actions)
- [Ref confusion (online)](#check-ref-confusion)
- [Stale action refs (online)](#check-stale-action-refs)
- [Archived repositories (online)](#check-archived-uses)
- [Version comments of pinned actions (online)](#check-ref-version-mismatch)
- [Concurrency that cancels unrelated pull requests](#check-concurrency-cancels-prs)
- [Concurrency that cancels a release](#check-concurrency-cancels-release)
- [Gate jobs that are skipped when a job fails](#check-gate-job-skipped-on-failure)
- [Untrusted code in privileged workflows](#check-untrusted-checkout)
- [Untrusted artifacts in workflow_run workflows](#check-untrusted-artifact)
- [Unused job outputs](#check-unused-job-output)
- [Unused workflow inputs (pedantic)](#check-unused-workflow-input)
- [Needs entries that do nothing (pedantic)](#check-unused-needs)
- [Duplicate triggers](#check-duplicate-triggers)
- [Failures hidden by continue-on-error (pedantic)](#check-continue-on-error)
- [Mutable runner labels (pedantic)](#check-mutable-runner-label)
- [Invisible characters](#check-invisible-characters)
- [Unsound prefix matches on names](#check-unsound-prefix-match)
- [Composite actions](#check-composite-actions)
- [Composite action syntax](#check-composite-action-syntax)

Note that jactionlint focuses on catching mistakes in workflow files. If you want some general code style checks, please consider
using a general YAML checker like [yamllint][].

<a id="check-unexpected-keys"></a>
## Unexpected keys

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
test.yaml:6:5: unexpected key "default" for "job" section. expected one of "cache-mode", "concurrency", "container", "continue-on-error", "defaults", "env", "environment", "if", "name", "needs", "outputs", "permissions", "runs-on", "secrets", "services", "snapshot", "steps", "strategy", "timeout-minutes", "uses", "with" [syntax-check]
  |
6 |     default:
  |     ^~~~~~~~
test.yaml:12:9: unexpected key "Shell" for step to run shell command. expected one of "background", "continue-on-error", "env", "id", "if", "name", "run", "shell", "timeout-minutes", "working-directory" [syntax-check]
   |
12 |         Shell: bash
   |         ^~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8jEEOAiEMRfdzin8Bwp5reAIYqqCEEtrGeHsDcVy1zXuv3AOGSTmenCQcgJLomsC0Lm5xS9bVXIuLbZTpHq39vG1eK/Dm+ar94XKddCrPT4AfUYtX9rnO7YnSkCtxuwedhVGoNf6/uq0zIEUp3wEAtPsxjA==)

[Workflow syntax][syntax-doc] defines what keys can be defined in which mapping object. When unknown key is defined, it makes
the workflow run fail.

jactionlint can detect unexpected keys while parsing workflow syntax and report them as an error.

Key names are basically case-sensitive (though some specific key names are case-insensitive). This check is useful to catch
case-sensitivity mistakes.

<a id="check-missing-required-duplicate-keys"></a>
## Missing required keys and key duplicates

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
test.yaml:3:3: "runs-on" section is missing in job "test" [syntax-check]
  |
3 |   test:
  |   ^~~~~
test.yaml:8:9: key "VERSION_NAME" is duplicated in "matrix" section. previously defined at line:7,col:9. note that this key is case insensitive [syntax-check]
  |
8 |         VERSION_NAME: [V1, V2]
  |         ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8zLEKwkAQhOE+TzFdmljEcjuLFBZGULgmSDhl8SLJbbjdHPr2Ihqr4eeDkUiYFw3FQ65KBWCs9llALXnj++tbwOQtDc+1gMxJB4l99BMTulxXyNvLn11zOu+Pbd/uDg2hc3UF92M1nnU92iAtkcC3ICgDj6OU7wEANm8osg==)

Some mappings must include specific keys. For example, job mappings must include `runs-on:` and `steps:`.

And duplicate keys are not allowed. In workflow syntax, comparing some keys is **case-insensitive**. For example, the job ID
`test` in lower case and the job ID `TEST` in upper case are not able to exist in the same workflow.

jactionlint checks these missing required keys and duplicate keys while parsing, and reports an error.

<a id="check-empty-mapping"></a>
## Unexpected empty mappings

Example input:

```yaml
on: push
jobs:
```

Output:

```
test.yaml:2:6: "jobs" section should not be empty. please remove this section if it's unnecessary [syntax-check]
  |
2 | jobs:
  |      ^
```

[Playground](https://jactionlint.jdx.dev/#eNrKz7NSKCgtzuDKyk8qtgIMACULBOo=)

Some mappings and sequences should not be empty. For example, `steps:` must include at least one step.

jactionlint checks such mappings and sequences are not empty while parsing, and reports the empty mappings and sequences as an
error.

<a id="check-mapping-values"></a>
## Unexpected mapping values

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
test.yaml:6:18: expecting a single ${{...}} expression or boolean literal "true" or "false", but found plain text node [syntax-check]
  |
6 |       fail-fast: off
  |                  ^~~
test.yaml:8:21: expected scalar node for integer value but found scalar node with "!!float" tag [syntax-check]
  |
8 |       max-parallel: 1.5
  |                     ^~~
test.yaml:13:26: expecting a single ${{...}} expression or float number literal, but found plain text node [syntax-check]
   |
13 |         timeout-minutes: two minutes
   |                          ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNo0zEEKAjEMheH9nOJdoDIKbnKbDKQ6kmlLk6DeXqp1Fd5P+GohtLD78qib0QK4mI8LmHd2ub1/C8i8a8psTqg5z3jwKzXurCpKOJ+u396jWBp0bFE8kvJgpyrN/mQanwRTkYbLus4M+H5IDU/HXsLFCP6smOMzAGy5NWY=)

Some mapping values are restricted to some constant strings. Several mapping values expect boolean value like `true` or
`false`. And some mapping values expect integer or floating number values.

jactionlint checks such constant strings are used properly while parsing and reports an error when an unexpected value is
specified.

<a id="check-syntax-expression"></a>
## Syntax check for expression `${{ }}`

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
test.yaml:7:24: got unexpected character '"' while lexing expression, expecting 'a'..'z', 'A'..'Z', '_', '0'..'9', ''', '}', '(', ')', '[', ']', '.', '!', '<', '>', '=', '&', '|', '*', ',', ' '. do you mean string literals? only single quotes are available for string delimiter [expression]
  |
7 |       - run: echo '${{ "hello" }}'
  |                        ^~~~~~~
test.yaml:9:26: got unexpected character '+' while lexing expression, expecting 'a'..'z', 'A'..'Z', '_', '0'..'9', ''', '}', '(', ')', '[', ']', '.', '!', '<', '>', '=', '&', '|', '*', ',', ' ' [expression]
  |
9 |       - run: echo '${{ 1 + 1 }}'
  |                          ^
test.yaml:11:65: unexpected end of input while parsing arguments of function call. expecting ",", ")" [expression]
   |
11 |       - run: echo "${{ toJson(hashFiles('**/lock', '**/cache/') }}"
   |                                                                 ^~~
test.yaml:13:38: unexpected end of input while parsing object property dereference like 'a.b' or array element dereference like 'a.*'. expecting "IDENT", "*" [expression]
   |
13 |       - run: echo '${{ github.event. }}'
   |                                      ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0zUGKwzAMheF9TvEwA85kJgnZ5gBd9BaJEXVaY4VK7ib47kXtOisJ/g8e5xl7kdjceZW5AZRE7QLPkqW3XtaStfRpsfZJorTLVwG9yRkUIsP/HAdcpJTYoVZ/Rib8YToBzoDyVTi3cZF42RJJ67tuTBwe/h/2hiVEGv0vanVnI7dNY1kHelHWwcbeAwDH30Qz)

jactionlint lexes and parses expression in `${{ }}` following [the expression syntax document][expr-doc]. It can detect
many syntax errors like invalid characters, missing parentheses, unexpected end of input, ...

<a id="check-type-check-expression"></a>
## Type checks for expression syntax in `${{ }}`

jactionlint checks types of expressions in `${{ }}` placeholders of templates. The following types are supported by the type
checker.

| Type          | Description                                                                                | Notation                 |
|---------------|--------------------------------------------------------------------------------------------|--------------------------|
| Any           | Any value like `any` type in TypeScript. Fallback type when a value can no longer be typed | `any`                    |
| Number        | Number value (integer or float)                                                            | `number`                 |
| Bool          | Boolean value                                                                              | `bool`                   |
| String        | String value                                                                               | `string`                 |
| Null          | Type of `null` value                                                                       | `null`                   |
| Array         | Array of specific type elements                                                            | `array<T>`               |
| Loose object  | Object which can contain any properties                                                    | `object`                 |
| Strict object | Object whose properties are strictly typed                                                 | `{prop1: T1, prop2: T2}` |
| Map object    | Object who has specific type values like `env` context                                     | `{string => T}`          |

Type check by jactionlint is stricter than GitHub Actions runtime.

- Only `any` and `number` are allowed to be converted to string implicitly
- Implicit conversion to `number` is not allowed
- Object, array, and null are not allowed to be evaluated at `${{ }}`

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
test.yaml:7:28: property access of object must be type of string but got "number" [expression]
  |
7 |       - run: echo '${{ env[0] }}'
  |                            ^~
test.yaml:9:24: property "os" is not defined in object type {id: string; network: string} [expression]
  |
9 |       - run: echo '${{ job.container.os }}'
  |                        ^~~~~~~~~~~~~~~~
test.yaml:11:24: receiver of object dereference "owner" must be type of object but got "string" [expression]
   |
11 |       - run: echo '${{ github.repository.owner }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~
test.yaml:13:20: object, array, and null values should not be evaluated in template with ${{ }} but evaluating the value of type {string => string} [expression]
   |
13 |       - run: echo '${{ env }}'
   |                    ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8yzEKwzAMheE9p3hDIVNMZ1+ldIiDqB2KZCwppYTcvbidm+kN//eEI6prHlZJGgfASK0v0Jx16t2Ts/n0nHv7JjWq+lPA1GUELVkwXvYdxNvtesdxjP/EKikswjYXphZEz+yjWPYUGlXRYtLeQV5M7exCvPX8GQCnCkLw)

Type checks for expression syntax in `${{ }}` are done by semantics checker. Note that actual type checks by GitHub Actions
runtime is loose.

Any object value can be assigned into string value as string `'Object'`. `echo '${{ env }}'` will be replaced with
`echo 'Object'`. And an array can also be converted into `'Array'` string. Such loose conversions are bugs in almost all cases.
jactionlint checks types more strictly. jactionlint checks values evaluated at `${{ }}` are not object (replaced with string
`'Object'`), array (replaced with string `'Array'`), nor null (replaced with string `''`). If you want to check a content of
object or array, use `toJSON()` function.

```
echo '${{ toJSON(github.event) }}'
```

There are two object types internally. One is an object which is strict for properties, which causes a type error when trying to
access unknown properties. And another is an object which is not strict for properties, which allows accessing unknown properties.
In the case, accessing unknown property is typed as `any`.

When the type check cannot be done statically, the type is deduced to `any` (e.g. return type of `toJSON()`).

As special case of `${{ }}`, it can be used for expanding object and array values.

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
test.yaml:19:14: type of expression at "env" must be object but found type string [expression]
   |
19 |         env: ${{ matrix.env_string }}
   |              ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNqckD8LgzAUxHc/xSGCU/oBAh10EDqluHUqKsE/tIkkL6VF8t1LULHi1inckbv3e08rjtHZLhp0bXkEkLQUXsCSqUi2n1kBz4pM/14VINXrbsn0qt08gCEthDjnWZke3evlJtJdga4H2dC+oBCCI8/KoxnyEQAYpywL7K52ihx7VIF7wZajXQtZ+Mkhm04jTgoh4t/pHMk0LXudNhp4/198vga8/w4AgQRZVA==)

In above example, environment variables mapping is expanded at `env:` section. jactionlint checks type of the expanded value.

<a id="check-contexts-and-builtin-func"></a>
## Contexts and built-in functions

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
      - run: echo "${{ contains(github.event.labels.*.name, 'enhancement') }}"
```

Output:

```
test.yaml:7:24: undefined variable "unknown_context". available variables are "env", "github", "inputs", "job", "matrix", "needs", "runner", "secrets", "steps", "strategy", "vars" [expression]
  |
7 |       - run: echo '${{ unknown_context }}'
  |                        ^~~~~~~~~~~~~~~
test.yaml:9:24: property "events" is not defined in object type {action: string; action_path: string; action_ref: string; action_repository: string; action_status: string; actor: string; actor_id: string; api_url: string; artifact_cache_size_limit: number; base_ref: string; env: string; event: object; event_name: string; event_path: string; graphql_url: string; head_ref: string; job: string; job_workflow_sha: string; output: string; path: string; ref: string; ref_name: string; ref_protected: bool; ref_type: string; repository: string; repository_id: string; repository_owner: string; repository_owner_id: string; repository_visibility: string; repositoryurl: string; retention_days: number; run_attempt: string; run_id: string; run_number: string; secret_source: string; server_url: string; sha: string; state: string; step_summary: string; token: string; triggering_actor: string; workflow: string; workflow_ref: string; workflow_sha: string; workspace: string} [expression]
  |
9 |       - run: echo '${{ github.events }}'
  |                        ^~~~~~~~~~~~~
test.yaml:11:24: undefined function "startWith". available functions are "always", "cancelled", "case", "contains", "endswith", "failure", "format", "fromjson", "hashfiles", "join", "startswith", "success", "tojson" [expression]
   |
11 |       - run: echo "${{ startWith('hello, world', 'lo,') }}"
   |                        ^~~~~~~~~~~~~~~~~
test.yaml:13:24: number of arguments is wrong. function "startsWith(string, string) -> bool" takes 2 parameters but 1 arguments are given [expression]
   |
13 |       - run: echo "${{ startsWith('hello, world') }}"
   |                        ^~~~~~~~~~~~~~~~~~
test.yaml:15:51: 2nd argument of function call is not assignable. "object" cannot be assigned to "string". called function type is "startsWith(string, string) -> bool" [expression]
   |
15 |       - run: echo "${{ startsWith('hello, world', github.event) }}"
   |                                                   ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqckEFKxjAQhfc9xVCEqKQ5QC/iUpI6mGo6UzoTK5TcXWJB/OFvF11l8b7v5TFMPcxZYvPBQfoGQFG0vgBLJulqnkMmzV3yNfuNRHGWnQLoKtkDDpHBPGwbZPokXul1YFL8VijFHKHvo8YcHH4hqRyAbQVF/aIvo8ZHEzEltrDykt6MBZPYmicopT115Y58zbI3q0876gX8SHJh9J/6/zOXfMAk7tmRn9CCQYqeBpyQdK/7GQBh6o/Y)

[Contexts][contexts-doc] and [built-in functions][funcs-doc] are strongly typed. Typos in property access of contexts and
function names can be checked. And invalid function calls like wrong number of arguments or type mismatch at parameter also
can be checked thanks to type checker.

The semantics checker can properly handle that

- some functions are overloaded (e.g. `contains(str, substr)` and `contains(array, item)`)
- some parameters are optional (e.g. `join(strings, sep)` and `join(strings)`)
- some parameters are repeatable (e.g. `hashFiles(file1, file2, ...)`)

Note that context names and function names are case-insensitive. For example, `toJSON` and `toJson` are the same function.

In addition, jactionlint performs special checks on some built-in functions.

- `format()`: Checks placeholders in the first parameter which represents the format string.
- `fromJSON()`: Checks the JSON string is valid and the return value is strongly typed.

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
test.yaml:6:18: property "mac" is not defined in object type {linux: string; win: string} [expression]
  |
6 |     runs-on: ${{ fromJSON('{"win":"windows-latest","linux":"ubuntul-latest"}')['mac'] }}
  |                  ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:24: format string "{0}{1}" does not contain placeholder {2}. remove argument which is unused in the format string [expression]
  |
9 |       - run: echo "${{ format('{0}{1}', 1, 2, 3) }}"
  |                        ^~~~~~~~~~~~~~~~
test.yaml:11:24: format string "{0}{1}{2}" contains placeholder {2} but only 2 arguments are given to format [expression]
   |
11 |       - run: echo "${{ format('{0}{1}{2}', 1, 2) }}"
   |                        ^~~~~~~~~~~~~~~~~~~
test.yaml:14:31: broken JSON string is passed to fromJSON() at offset 23: unexpected end of JSON input [expression]
   |
14 |         if: contains(fromJson('["main","release","dev"'), github.ref_name)
   |                               ^~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMj0FL9DAQhu/7K95v+CAtZMVdb/kJHvSgt0Uk7aY20k5KJnGFkP8ure5R8DJzmPeZhzewwZJl3O3eQydmByQnad1AzCz7NfC/FAwxzPdPjw+NKnTxTGad53CR/WRXhDRNnvMnGcpd5pSn66Gq9qRm26sX1Lo9luQW+XYA+9Vj4PoxgDZTiLNNjSq3tRyq0jhoHDXuWtRKf4PK8cr9Bj2PXuAFFrK43tsJXbTcj/9+soAfDPrAyXqWZmsvgRt1otl6Jk3RTc6KI01n90Gq1XjzaczdTXTDK9vZtV8DAMITaRA=)

GitHub Actions does not provide the syntax to create an array or object constant. It [is popular](https://github.com/search?q=fromJSON%28%27+lang%3Ayaml&type=code)
to create such constants via `fromJSON()`.

<a id="check-contextual-step-object"></a>
## Contextual typing for `steps.<step_id>` objects

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
test.yaml:10:24: property "get_value" is not defined in object type {} [expression]
   |
10 |       - run: echo '${{ steps.get_value.outputs.name }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:22:24: property "get_value" is not defined in object type {} [expression]
   |
22 |       - run: echo '${{ steps.get_value.outputs.name }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqskDFrhEAQhXt/xWMRrPQHLMQiTZIqKbQWNWs0mB1xZtKI/z3sxgQODo7jrprifd88eOQtFuUx+aSObQKIYwkXWNVzHnLt1IvmcxuyGJHKosK/HDAQWWTptoHFLVx8OGm+21ldcYCFb78c9j2LQoT+3Dz0WLh+pCtenHpmIHqItEFZwqRPL9Vz/di81tVbXZnDAKZ3i//Hd+w/6/Xk+1l5In9YJKNbL0572zg/AwAkVZJl)

Outputs of step can be accessed via `steps.<step_id>` objects. The `steps` context is dynamic:

- Accessing the outputs before running the step causes `null`
- Outputs of steps only in the job can be accessed. It cannot access steps across jobs

It is a common mistake to access the wrong step outputs since people often forget to fix placeholders on copying&pasting
steps. jactionlint can catch invalid accesses to step outputs and reports them as errors.

When the outputs are set by popular actions, the outputs object is more strictly typed.

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
test.yaml:8:23: property "cache" is not defined in object type {} [expression]
  |
8 |       - run: echo ${{ steps.cache.outputs.cache-hit }}
  |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:18:23: property "cache_hit" is not defined in object type {cache-hit: string} [expression]
   |
18 |       - run: echo ${{ steps.cache.outputs.cache_hit }}
   |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkjsFKxDAQhu99iv8grBbaXjzNyZOvIdk4OLElCc6MIsu+uyRd6lUwlzDzfT//lEyorjIM7+WsNADGau0HPjzr1AQ/ezafttBYR2pcdbeAqZkEjlJwd7nscI4hCs/FrbrdpkmS4Xo9Yq6shBAtlaxLV54+H28YSK+Evjw2X8mEjglY+Zt6pQSV57Sx3p/GcRnnrcT19PDb1V4NJoR5qSGu4Y31v9f/Mfayx34GAIltbNw=)

In the above example, [actions/cache][actions-cache] action sets `cache-hit` output so that the following steps can know
whether the cache was hit or not. At line 8, the cache action is not run yet. So `cache` property does not exist in the
`steps` context yet. On running the step whose ID is `cache`, `steps.cache` object is typed as
`{outputs: {cache-hit: any}, conclusion: string, outcome: string}`. At line 18, the expression has a typo in the output
name. jactionlint can check it because properties of `steps.cache.outputs` are typed.

This strict typing for outputs is also applied to local actions. Let's say we have the following local action.

```yaml
name: 'My action with output'
author: 'rhysd <https://rhysd.github.io>'
description: 'my action with outputs'

outputs:
  some_value:
    description: some value returned from this action

runs:
  using: 'node20'
  main: 'index.js'
```

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:8:23: property "my_action" is not defined in object type {} [expression]
  |
8 |       - run: echo ${{ steps.my_action.outputs.some_value }}
  |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:15:23: property "some-value" is not defined in object type {some_value: string} [expression]
   |
15 |       - run: echo ${{ steps.my_action.outputs.some-value }}
   |                       ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The 'My action with output' action defines one output `some_value`. The property is typed at `steps.my_action.outputs` object
so that jactionlint can check incorrect property accesses like a typo in the output name.

<a id="check-contextual-matrix-object"></a>
## Contextual typing for `matrix` object

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
test.yaml:19:24: property "platform" is not defined in object type {node: number; npm: string; os: string; package: {name: string; optional: bool}} [expression]
   |
19 |       - run: echo '${{ matrix.platform }}'
   |                        ^~~~~~~~~~~~~~~
test.yaml:21:24: property "dev" is not defined in object type {name: string; optional: bool} [expression]
   |
21 |       - run: echo '${{ matrix.package.dev }}'
   |                        ^~~~~~~~~~~~~~~~~~
test.yaml:34:24: property "os" is not defined in object type {} [expression]
   |
34 |       - run: echo '${{ matrix.os }}'
   |                        ^~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMUk1v6yAQvOdXzOFJJFJsKU+xIiG9XxK9A7FxQmsDYiFplfLfK+JQ58NVe0K7zCyzMxjNYQMdZi9mR3wGeEk+nQB5J7zcvw8V0Avv1FuuAEMc27AL2oeiE4m3xEnpxpzoWv//wmrTSI7tar3EqhrbVtSvYi/HmUABLXrJwVpj2E0fMNYro0XH4V2QU5SdcN9RWtHRyFG67kLz+O5F46q6m6Btz7Epq3J9abugqUie/Tmfr4aUhhDj1TFpKQ8tEphD1gcDdgO3nfCtcT1iZD9BB3vKRh6f0R83OgdmCuRRF3tCpTVxlI7Uwx6XiylGTgkT0pLziBHzbPW/CVC+Q4yLyZ217bMkKAJZWatWyWbUotrh+dpoL5SmeRZt+yXYpqzYYkgh/bu//D6su0/6+6QGAz8HAKxO5N4=)

Types of `matrix` context are contextually checked by the semantics checker. Type of matrix values in `matrix:` section
is deduced from element values of its array. When the matrix value is an array of objects, objects' properties are checked
strictly like `package.name` in above example.

When a type of the array elements is not persistent, the type of the matrix value falls back to `any`.

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
test.yaml:16:24: property "prepare" is not defined in object type {} [expression]
   |
16 |       - run: echo '${{ needs.prepare.outputs.prepared }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:3: job "build" reads "needs.some_job" but has no "if" with a status check function, so GitHub skips the job when a job it needs fails or is skipped, and a skipped job counts as passing for a required check. add "if: ${{ !cancelled() }}" (or "always()") to the job [gate-job-skipped-on-failure]
   |
17 |   build:
   |   ^~~~~~
test.yaml:26:24: property "foo" is not defined in object type {installed: string} [expression]
   |
26 |       - run: echo '${{ needs.install.outputs.foo }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:28:24: property "some_job" is not defined in object type {install: {outputs: {installed: string}; result: string}; prepare: {outputs: {prepared: string}; result: string}} [expression]
   |
28 |       - run: echo '${{ needs.some_job }}'
   |                        ^~~~~~~~~~~~~~
test.yaml:33:24: property "build" is not defined in object type {} [expression]
   |
33 |       - run: echo '${{ needs.build.outputs.built }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkkUFOxDAMRfdzir9A6ob2ALkKQqglhnRU4qh2xGLUuyNnMqmAESBmUamu7f/+dzk6pCzhcORJ3AGYo+i4LPYKcNaUVc5F65F36IZh6MrnNUfpTSZPOWrul1FJtLREKbXl3iYd6DkwuqoE4TfSMMdX00orpXGlq+jau51sKvZ8Rn8buzudEIm8DJU8VEOX2mPbbHPK8+LPpDLv8FDD3V9MP14LZHt6c5pC36PgfdaA3Xt10ry3H4htwxg9/hzzhwN9hbww/75klp+OPNVJ1kCr+/8dduFykebFKjXGxwDMHOsV)

Job dependencies can be defined at [`needs:`][needs-doc]. A job runs after all jobs defined in `needs:` are done.
Outputs from the jobs can be accessed only from jobs following them via [`needs` context][needs-context-doc].

jactionlint defines a type of `needs` variable contextually by looking at each job's `outputs:` section and `needs:` section.

<a id="check-comparison-types"></a>
## Strict type checks for comparison operators

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
test.yaml:13:17: "object" value cannot be compared to "string" value with "==" operator [expression]
   |
13 |         if: ${{ github.event == 'workflow_call' }}
   |                 ^~~~~~~~~~~~
test.yaml:16:17: "bool" value cannot be compared to "number" value with ">" operator [expression]
   |
16 |         if: ${{ inputs.timeout > 60 }}
   |                 ^~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsj81KwDAQhO99ihGEnFo8eQjUV5Gmbtto3C3NxiIl7y7pH4jednc+ZmaFbQWssnwMQdbXvguhHADPc9J4zID6T5Kk1wro90wWTiRQx1X1Lm5nleIJLYljLWyRXGJNdeiKtktRab6d60JaUD8JTImntwdzx/jB4nHbMHqdkmvoi1jRtjC/Ghvk/J/d2Ro+QkUQhMe/1sejzcW+4PkJOf8MAEdGU1w=)

Expressions in `${{ }}` placeholders support `==`, `!=`, `>`, `>=`, `<`, `<=` comparison operators. Arbitrary types of operands
can be compared. When different type values are compared, they are implicitly converted to numbers before the comparison. Please
see [the official document][operators-doc] to know the details of operators behavior.

However, comparisons between some types are actually meaningless:

- Objects and arrays are converted to `NaN`. Comparing an object or an array with other type is always evaluated to false.
- Comparing booleans, null, objects, and arrays with `>`, `>=`, `<`, `<=` makes no sense.

jactionlint checks operands of comparison operators and reports errors in these cases.

There are some additional surprising behaviors, but jactionlint allows them not to cause false positives as much as possible.

- `0 == null`, `'0' == null`, `false == null` are true since they are implicitly converted to `0 == 0`
- `'0' == false` and `0 == false` are true due to the same reason as above
- Objects and arrays are only considered equal when they are the same instance

<a id="check-shellcheck-integ"></a>
## [shellcheck][] integration for `run:`

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
test.yaml:6:9: shellcheck reported issue in this script: SC2086:info:1:6: Double quote to prevent globbing and word splitting [shellcheck]
  |
6 |       - run: echo $FOO
  |         ^~~~
test.yaml:14:9: shellcheck reported issue in this script: SC2086:info:1:6: Double quote to prevent globbing and word splitting [shellcheck]
   |
14 |       - run: echo $FOO
   |         ^~~~
```

<!-- Skip playground link -->

[shellcheck][] is a famous linter for ShellScript. jactionlint runs shellcheck for scripts at `run:` step in a workflow.
For installing shellcheck, see [the official installation document][shellcheck-install].

jactionlint detects which shell is used to run the scripts following [the documentation][shell-doc]. On Linux or macOS the
default shell is `bash`, and on Windows it is `pwsh`. Shell can be configured by `shell:` configuration at a workflow
level or job level. Each step can configure shell to run scripts by `shell:`.

In the above example output, `SC2086:info:1:6:` means that shellcheck reported SC2086 rule violation and the location is at
line 1, column 6. Note that the location is relative to the script of the `run:` section.
The reported source line is the line within the script when `run:` uses a literal block (`|`, `|-`, `|+`) and no `${{ }}`
in the script spans multiple lines. Otherwise, the position of `run:` is reported.

jactionlint remembers the default shell and checks what OS the job runs on. Only when the shell is `bash` or `sh`, jactionlint
applies shellcheck to scripts.

By default, jactionlint checks if `shellcheck` command exists in your system and uses it when it is found. The `-shellcheck`
option on running `jactionlint` command specifies the executable path of shellcheck. Setting empty string by `shellcheck=`
disables shellcheck integration explicitly.

Since both `${{ }}` expression syntax and ShellScript's variable access `$FOO` use `$`, the remaining `${{ }}` confuses
shellcheck. To avoid it, jactionlint replaces `${{ }}` with underscores. For example `echo '${{ matrix.os }}'` is replaced
with `echo '________________'`.

Some shellcheck rules conflict with the `${{ }}` expression syntax. To avoid errors due to the syntax, [SC1091][], [SC2050][],
[SC2194][], [SC2154][], [SC2157][], [SC2043][] are disabled.

When what shell is used cannot be determined statically, jactionlint assumes `shell: bash` optimistically. For example,

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

The 'Show file content' script is only run by `pwsh` due to `matrix.os == 'windows-latest'` guard. However, jactionlint does not
know that. It checks the script with shellcheck and it'd probably cause a false-positive (due to file separator). This kind of
false positives can be avoided by showing the shell name explicitly. It is also better in terms of maintenance of the workflow.

```yaml
- name: Show file content
  run: Get-Content -Path xxx\yyy.txt
  if: ${{ matrix.os == 'windows-latest' }}
  shell: pwsh
```

When you want to control shellcheck behavior, [`SHELLCHECK_OPTS` environment variable][shellcheck-env-var] is useful.

From command line:

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

<a id="check-pyflakes-integ"></a>
## [pyflakes][] integration for `run:`

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

Python script can be written in `run:` when `shell: python` is configured.

[pyflakes][] is a famous linter for Python. It is suitable for linting small code like scripts at `run:` since it focuses
on finding mistakes (not a code style issue) and tries to make false positives as minimal as possible. Install pyflakes
by `pip install pyflakes`.

jactionlint runs pyflakes for scripts at `run:` steps in a workflow and reports errors found by pyflakes. jactionlint detects
Python scripts in a workflow by checking `shell: python` at each step and `defaults:` configurations at workflows and jobs.

By default, jactionlint checks if `pyflakes` command exists in your system and uses it when found. The `-pyflakes` option
of `jactionlint` command allows to specify the executable path of pyflakes. Setting empty string by `pyflakes=` disables
pyflakes integration explicitly.

Since both `${{ }}` expression syntax is invalid as Python, remaining `${{ }}` might confuse pyflakes. To avoid it,
jactionlint replaces `${{ }}` with underscores. For example `print('${{ matrix.os }}')` is replaced with
`print('________________')`.

<a id="untrusted-inputs"></a>
## Script injection by potentially untrusted inputs

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
        run: if [ "${{ contains(github.event.pull_request.author.title, '[SKIP]') }}" = "true" ]; then echo "skip"; fi
```

Output:

```
test.yaml:10:24: "github.event.pull_request.title" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
10 |         run: echo '${{ github.event.pull_request.title }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:19:36: "github.event.head_commit.author.name" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
19 |           script: console.log('${{ github.event.head_commit.author.name }}')
   |                                    ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:22:31: object filter extracts potentially untrusted properties "github.event.comment.body", "github.event.discussion.body", "github.event.issue.body", "github.event.pull_request.body", "github.event.review.body", "github.event.review_comment.body". avoid using the value directly in inline scripts. instead, pass the value through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
22 |         run: echo '${{ toJSON(github.event.*.body) }}'
   |                               ^~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkUFrGzEQhe/+FY+lYKd0t8dShUIOLaUNJIHkFkKQ5YmlWqvZakYOJfi/F+0aE7cYclpm5+l7T0/J9mRwR6IzTgZDifEx0+9Sf8x+8VLMDFASrV8glyRtFZZlSVraaOtuXInSIJMKaDGBb3JIOlKxp0KDRtrLRqABOc+Yv3t5wTqoL8uOtpS0ex2mG49ht5sfHIqQGFingZN8FLWRLrafD+TnoN4cJiDTwK3yhpJBtRJymVS6u+vLb1fY7V5JR1Y75LYnEbsmg7dlw7MVuMhCqxMpJ0YrLodBL7afTqadFAaOk3CkLvJ68X9Dnuzq0XHfB+1sUc+5q73Xms7+eYnvpKhKSiqn2lf+eXt9tTiyeN8tefXn7Kj5ifiVIdyT+pDWY344T25TJ9mE4dgkPOEeTTVxnNSGJIvTfe6vMtb6AfP728sfNw/zmqHBFzSaCzV4OId6SlP8pjo253gKfwcAqP/kRg==)

Since `${{ }}` placeholders are evaluated and replaced directly by GitHub Actions runtime, you need to use them carefully in
inline scripts at `run:`. For example, if we have step as follows,

```yaml
- run: echo 'issue ${{github.event.issue.title}}'
```

an attacker can create a new issue with the title `'; malicious_command ...`, and the inline script will run
`echo 'issue'; malicious_command ...` in your workflow. The remediation of such script injection is passing potentially untrusted
inputs via environment variables. See [the official document][security-doc] for more details.

```yaml
- run: echo "issue ${TITLE}"
  env:
    TITLE: ${{github.event.issue.title}}
```

jactionlint recognizes the following inputs as potentially untrusted and checks your inline scripts at `run:`. When they are used
directly in a script, jactionlint will report it as an error.

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

Not only direct access to the untrusted properties, jactionlint also detects those properties indirectly accessed via
[object filter syntax][object-filter-syntax]. For example, `github.event.*.body` collects all `body` properties in child objects
of `github.event` as array. Those properties include untrusted inputs like `github.event.comment.body`,
`github.event.pull_request.body`, ...

```sh
# Echo list of github.event.comment.body, github.event.pull_request.body, ...
echo '${{ toJSON(github.event.*.body) }}'
```

Instead, you should store the JSON string in an environment variable:

```sh
- run: echo "${BODIES}"
  env:
    BODIES: '${{ toJSON(github.event.*.body) }}'
```

The following functions return a boolean value so it is not possible to inject anything as the result of the returned value.
jactionlint does not report an error even if untrusted inputs are passed to these function calls.

- `contains()`
- `startswith()`
- `endswith()`

At last, the popular action [actions/github-script][github-script] has the same issue in its `script` input. jactionlint also
checks the input.

### Untrusted inputs through objects, environment variables and action inputs

The rule `template-injection` does not stop at the properties above. It reports the same risk when it is less direct:

- every `${{ }}` of a script is checked, not only the first one;
- an object which holds untrusted properties, like `toJSON(github.event)`, is reported because the script gets the title and the
  body of the issue as a part of the JSON;
- an environment variable which is set from an untrusted input, like `env.TITLE` below, is reported when it is expanded with
  `${{ env.TITLE }}`. Reading it as a variable of the shell (`"$TITLE"`) is the fix, and it is what the environment variable is for;
- inputs of well-known actions which run their value as code are checked like `run:` and the `script` of github-script:
  `command` of nick-fields/retry, `inlineScript` of azure/cli and azure/powershell, `script` of appleboy/ssh-action,
  `run` and `options` of addnab/docker-run-action, `preCommands` and `postCommands` of cloudflare/wrangler-action, `cmd` of
  mikefarah/yq, the `command` of several SSH actions, and a few more. They are listed in `codeExecTable` of
  [injection_sinks.go](https://github.com/jdx/jactionlint/blob/main/injection_sinks.go), each with the place where its behavior can
  be checked;
- fields that GitHub passes to docker are checked too, see [below](#check-template-injection-sinks);
- the prompt, the arguments and the settings of [AI agent actions](#check-agentic-actions) are checked too, see below.

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
      # OK: The shell expands the variable
      - run: echo "$TITLE"
```

Output:

```
test.yaml:11:24: environment variable "env.TITLE" holds the potentially untrusted input "github.event.issue.title". expanding it with ${{ }} in an inline script is as dangerous as using the input directly. instead, read it as a variable of the shell [expression]
   |
11 |       - run: echo '${{ env.TITLE }}'
   |                        ^~~~~~~~~
test.yaml:14:21: "github.head_ref" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
14 |           echo '${{ github.head_ref }}'
   |                     ^~~~~~~~~~~~~~~
test.yaml:15:21: "github.event.comment.body" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
15 |           echo '${{ github.event.comment.body }}'
   |                     ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:17:31: "github.event" includes potentially untrusted properties such as "github.event.comment.body". avoid expanding it in inline scripts. instead, pass the properties you need through environment variables [expression]
   |
17 |       - run: echo '${{ toJSON(github.event) }}'
   |                               ^~~~~~~~~~~~~
test.yaml:21:29: "github.head_ref" is potentially untrusted. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
21 |           command: echo ${{ github.head_ref }}
   |                             ^~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpskL1OAzEQhHs/xSiKFCjOFHSuaChACArSR/ez4QyXNbpdH4rCvTvyxcqPODfWar/xzDiwgxeJtKnDbkesxhAPzgDrp/XLo8PycMCH1zZWlgZitRNt1WtHGEdjPkMliVcSTTfQR5YiPRyryBqLrky7aSVK33KkgCKRDlS3AavkQzzYyRbjuLqGfvOYzlmQg7VUNpuetheyWe5YIBe1VWj2/43OIg3P72+vN5fa2ys+CokD+/qr2HrqGrnrSfv9w3B/CvHjtXWnCUjeJTfZZr7CTJ7FcvqWxd8A3hV//w==)

`-fix` fixes the findings in `run:` scripts of bash and sh when it can show that the fix does the same: the `${{ }}` is in double
quotes or single quotes, and the script has no here-document, command substitution or comment around it. The expression is replaced
with a shell variable (the `GITHUB_*` and `RUNNER_*` variables which the runner sets already, like `$GITHUB_HEAD_REF`, or a new one
in the `env:` of the step):

```yaml
- run: echo "title is ${TITLE}"
  env:
    TITLE: ${{ github.event.issue.title }}
```

When the expression is not in quotes, quoting it changes how the shell splits words and expands globs, so the fix needs
`-fix=unsafe`. Scripts of other shells (PowerShell, cmd, Python) are not fixed.

<a id="check-template-injection-sinks"></a>
### Container options, Docker steps and AI agent inputs

`template-injection` also reports an attacker controlled `${{ }}` in the places where a runner or an action reads a string as a
command line or as the instructions of an agent, although they are not scripts:

| Place | Why it matters |
| --- | --- |
| `container.options` and `services.<id>.options` | docker reads them as command line flags, so text can add `--privileged`, `--volume` or `--entrypoint` ([zizmor#1128](https://github.com/zizmorcore/zizmor/issues/1128)) |
| `image`, `entrypoint`, `command` and `volumes` of `container` and of a service | the attacker chooses what runs, or which path of the runner is mounted |
| `args` and `entrypoint` of a `docker://` step | they are the command line of the container |
| `prompt`, `claude_args`, `settings` and the other inputs of [AI agent actions](#check-agentic-actions) | the agent reads the prompt as instructions, and the arguments and settings add tools, servers and hooks |

Only contexts that an attacker controls are reported here, unlike in a script: `matrix.node` in an `image` is how containers are
written. The environment variable is not a remedy for these places (the `env` context is not available in a container, and an agent
reads the environment as it reads the prompt): use a fixed value, or one from a fixed list, and for an agent see
[the advice below](#check-agentic-actions). `shell:` takes no expression, so it is not a sink.

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
test.yaml:9:27: "github.event.issue.title" is potentially untrusted and is expanded into the options of the container of the job. docker reads them as command line flags, so text from an attacker can add flags such as --privileged, --volume or --entrypoint. use a fixed value, or choose one from a fixed list such as a matrix or an input of type choice [expression]
  |
9 |       options: --user ${{ github.event.issue.title }}
  |                           ^~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:14:31: "github.event.issue.title" is potentially untrusted and is expanded into the "prompt" input of Claude Code Action. the agent reads the prompt as instructions, so text from an attacker can steer the agent, and quoting or escaping cannot prevent that. do not put event data in the prompt: let the agent read it with its own tools, and give the agent only the permissions, secrets and tools that the worst instruction could use [expression]
   |
14 |           prompt: Triage "${{ github.event.issue.title }}"
   |                               ^~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNq0UDluxDAM7P2KwSKtnKNUlUfkA7JMrBXYpCCSm2Lhvwc+ki5ItZ3EucgRjiiqTtp1nzJo7ABrJV1pewHNWcNG8sHZPMzJSG2HsrClwtQOJlCWTQaWkeLbyzmUakVYI0JwpYan+x3XYpMPPd2Ird/Teys2E9Z1V6lR1R/XAFfSiMQ2Nakl63Oek48UsowUUt7832+vJx34KjbF3x9QmyzVIj72s3D5Z4PLg3JtoqNpsC/D302c6Lp+DwDREIHh)

<a id="check-job-deps"></a>
## Job dependencies validation

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
test.yaml:3:3: cyclic dependencies in "needs" job configurations are detected. detected cycle is "prepare" -> "build" -> "install" -> "prepare" [job-needs]
  |
3 |   prepare:
  |   ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkjjEOwyAMRXdO8TcmLsBVqg7QWEoqZBC2719BvWTOZvn5v+/OGcPkDN9eJQdgTBpl0hoBJjok41Xtasd7r6axpJWyaqyWWlES3UiUhvyDQFqXGfQ5O6JLYwAuFi2t3f3OHzS4djXsZ+9+pw/8Wxp/AwC/J1vk)

Job dependencies can be defined at [`needs:`][needs-doc]. If cyclic dependencies exist, jobs never start to run. jactionlint
detects cyclic dependencies in `needs:` sections of jobs and reports it as an error.

jactionlint also detects undefined jobs and duplicate jobs in `needs:` section.

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
test.yaml:4:18: job ID "BAR" duplicates in "needs" section. note that job ID is case insensitive [job-needs]
  |
4 |     needs: [bar, BAR]
  |                  ^~~~
test.yaml:8:3: job "bar" needs job "unknown" which does not exist in this workflow [job-needs]
  |
8 |   bar:
  |   ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkjDsOAjEMRPucYrptyAXcwRFoEUUMRuEjexXb4vooS0VNNdLMvGdKWNN7eRg7FeBmNgNQkasTTtzGDof98by1I9XrhJJTI+urhXhsk4es/mWBOp8EuXTD0u9LAbiNX3PqU+2t/4k/AwB6DTh7)

<a id="check-parallel-step-refs"></a>
## Parallel steps

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
test.yaml:11:17: "serverr" is not the ID of a preceding background step. "wait" and "cancel" steps can only refer to an earlier step that has "background: true" [parallel-steps]
   |
11 |       - cancel: serverr
   |                 ^~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpczssNAjEMBND7VjG3nNKA26CCJGuxwOKs/KF+FD4R4mTJz6NxF8IRti3XXo0WwNl8TEBDLA+PGuKR9zLsReZ82PsKyJByZ8LJizqM9cH6IeCy0v9KQwjcto5kI5Km1NJuZ+0hK8E1eBb8RMYPlqa0Io33b4c+BwCanjvx)

<a id="check-timeout-minutes"></a>
## Timeout minutes of jobs

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

Output:
<!-- Skip update output -->
```
test.yaml:3:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs [timeout-check]
  |
3 |   no-timeout:
  |   ^~~~~~~~~~~
test.yaml:9:22: "timeout-minutes" is 120, which is greater than the maximum 60 minutes allowed by the configuration [timeout-check]
  |
9 |     timeout-minutes: 120
  |                      ^~~
```

<!-- Skip playground link -->

These are the rules `missing-timeout` (in the `default` profile) and `timeout-too-long` (only when `max` is set). The output above
is from the following `rules` section of the [configuration file](config.md). `missing-timeout` is on without it; the line is
there to show the default level:

```yaml
rules:
  missing-timeout: error
  timeout-too-long:
    max: 60
```

- `missing-timeout` reports jobs which do not set [`timeout-minutes`][timeout-minutes-doc]. Without it, a hanging job runs until
  the default timeout of 360 minutes. Jobs calling a reusable workflow are not checked since they do not support it.
- `timeout-too-long` reports jobs whose `timeout-minutes` is larger than the `max` option. It works without `missing-timeout`.
  Values given by expressions `${{ }}` are not checked.

`missing-timeout` is enabled by default because a job without a timeout can run for six hours (and be billed for it) when a step
hangs. Turn it off with `missing-timeout: off` in `rules`, or ignore one job with an
[ignore comment](usage.md#ignore-some-errors) above it.

#### Fixing missing timeouts

jactionlint has no built-in number of minutes, because the right limit depends on your jobs. The finding tells you to set
`timeout-minutes`, and `jactionlint -fix` adds it only when you configure the number with the `default-minutes` option:

```yaml
rules:
  missing-timeout:
    default-minutes: 15
```

Then `-fix` adds `timeout-minutes: 15` to every job which has none. It is a safe fix: a job which is cancelled after that time
is the only way it can change a workflow, and one which really needs longer sets its own value. The line goes right after
`runs-on:` (or after `name:`, or as the first key of the job when it has neither), with the indentation of the job's other keys
and the line endings of the file. Without `default-minutes` the finding has no fix.

When `timeout-too-long` has a `max` smaller than `default-minutes`, the fix uses `max` so the result stays clean. Jobs calling a
reusable workflow are skipped since they do not support `timeout-minutes`. A job written in the flow style (`job: {runs-on: ...}`)
or with a YAML anchor is still reported but has no fix: jactionlint does not guess where to put a key there.

[timeout-minutes-doc]: https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#jobsjob_idtimeout-minutes

<a id="check-workflow-run-names"></a>
## Workflow names of `workflow_run` event

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
<!-- Skip update output -->
```
test.yaml:4:17: workflow "Biuld" specified at "workflows" of "workflow_run" event is not found in the repository. a workflow is specified by its "name:" or its file path when it has no name [workflow-run]
  |
4 |     workflows: [Biuld]
  |                 ^~~~~
```

<!-- Skip playground link -->

This is the rule `workflow-run-names`. It is enabled by the default profile; turn it off with `rules: {workflow-run-names: off}`
in the [configuration file](config.md). Each name at `on.workflow_run.workflows` is compared (case-insensitively) with the `name:` of every workflow file in
`.github/workflows` (or its file path when it has no `name:`). The check is skipped for names with `${{ }}` or glob
characters, and entirely when a workflow file cannot be parsed or has a dynamic `name:`.

<a id="check-matrix-values"></a>
## Matrix values

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
test.yaml:6:28: duplicate value "14" is found in matrix "node". the same value is at line:6,col:24 [matrix]
  |
6 |         node: [10, 12, 14, 14]
  |                            ^~~
test.yaml:9:19: value "13" in "exclude" does not match in matrix "node" combinations. possible values are "10", "12", "14", "14" [matrix]
  |
9 |           - node: 13
  |                   ^~
test.yaml:12:13: "platform" in "exclude" section does not exist in matrix. available matrix configurations are "node", "os" [matrix]
   |
12 |             platform: ubuntu-latest
   |             ^~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpskMGOhCAQRO9+RR32KER298SvmD2gsuNMlDY0JE6M/z4h4jgmHggpqHrpanIaU+S+eFDDugCC5ZBugIM3wd6emwJGE/x93hXgqLMatapKqO8S6jedv/c3sUYdm+hCFINJ2BKjaYmzOpx2bofY2YMMiExXPx+PG/OEvIpUp8g0mPBPfrwK+uhYpA18LUuuJ4mxrrm/nXgfSiSzhm17gpTyNQBUwlNB)

[`matrix:`][matrix-doc] defines combinations of multiple values. Nested `include:` and `exclude:` can add/remove specific
combination of matrix values. jactionlint checks

- values in `exclude:` appear in `matrix:` (`include:` is processed after `exclude:`, so values added only by `include:` cannot be excluded)
- duplicate variations of matrix values

<a id="check-webhook-events"></a>
## Webhook events validation

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
test.yaml:4:5: unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows" [syntax-check]
  |
4 |     branch: foo
  |     ^~~~~~~
test.yaml:7:5: both "paths" and "paths-ignore" filters cannot be used for the same event "push". note: use '!' to negate patterns [events]
  |
7 |     paths-ignore: path/to/foo
  |     ^~~~~~~~~~~~~
test.yaml:10:12: invalid activity type "created" for "issues" Webhook event. available types are "assigned", "closed", "deleted", "demilestoned", "edited", "labeled", "locked", "milestoned", "opened", "pinned", "reopened", "transferred", "typed", "unassigned", "unlabeled", "unlocked", "unpinned", "untyped" [events]
   |
10 |     types: created
   |            ^~~~~~~
test.yaml:13:5: "tags" filter is not available for release event. it is only for push event [events]
   |
13 |     tags: v*.*.*
   |     ^~~~~
test.yaml:15:3: unknown Webhook event "pullreq". see https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#webhook-events for list of all Webhook event names [events]
   |
15 |   pullreq:
   |   ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpcjkGuwyAMRPecYtaRIHvfhuT7h1QIU2wq9fYVJJt2NbLf6NlSyAG1axoJbC2WPRH+ReZcoyWlGavJ+rX251Gk8S89VTvrpbN3ZSXsjaPxnwMaZ47KN42HEl5LWMIyv8i58ZOce8g2BcZqV7X1ol4KoW+9WPc5DjaRGtf7HOBHk8B7EoQQPgMAdVRCRg==)

At `on:`, Webhook events can be specified to trigger the workflow. [Webhook event documentation][webhook-doc] defines
which Webhook events are available and what types can be specified at `types:` for each event.

jactionlint validates the Webhook configurations:

- Webhook event name
- types for Webhook event
- filter names
- filter usages
  - `paths` and `paths-ignore`, `branches` and `branches-ignore`, `tags` and `tags-ignore` are exclusive. They can not
    be used for the same event.
  - Some filters are only available for specific events as explained in [the official document][specific-paths-doc]
    (see the following table).

| Filter name       | Events where the filter is available                                         |
|-------------------|------------------------------------------------------------------------------|
| `paths`           | `push`, `pull_request`, `pull_request_target`                                |
| `paths-ignore`    | `push`, `pull_request`, `pull_request_target`                                |
| `branches`        | `merge_group`, `push`, `pull_request`, `pull_request_target`, `workflow_run` |
| `branches-ignore` | `merge_group`, `push`, `pull_request`, `pull_request_target`, `workflow_run` |
| `tags`            | `push`                                                                       |
| `tags-ignore`     | `push`                                                                       |

The table of available Webhooks and their types are defined in [`all_webhooks.go`](https://github.com/jdx/jactionlint/blob/main/all_webhooks.go). It is generated
by [a script][generate-webhook-events] and kept to the latest by CI workflow triggered weekly.

<a id="check-workflow-dispatch-events"></a>
## Workflow dispatch event validation

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
test.yaml:6:15: input type of workflow_dispatch event must be one of "string", "number", "boolean", "choice", "environment" but got "text" [syntax-check]
  |
6 |         type: text
  |               ^~~~
test.yaml:8:7: input type of "kind" is "choice" but "options" is not set [events]
  |
8 |       kind:
  |       ^~~~~
test.yaml:16:18: default value "Chobi" of "name" input is not included in its options "\"Tama\", \"Mike\"" [events]
   |
16 |         default: Chobi
   |                  ^~~~~
test.yaml:22:18: type of "verbose" input is "boolean". its default value "yes" must be "true" or "false" [events]
   |
22 |         default: yes
   |                  ^~~
test.yaml:26:18: type of "age" input is "number" but its default value "teen" cannot be parsed as a float number: strconv.ParseFloat: parsing "teen": invalid syntax [events]
   |
26 |         default: teen
   |                  ^~~~
test.yaml:33:24: "inputs.massage" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
33 |       - run: echo "${{ inputs.massage }}"
   |                        ^~~~~~~~~~~~~~
test.yaml:33:24: property "massage" is not defined in object type {age: number; id: any; kind: string; message: string; name: string; verbose: bool} [expression]
   |
33 |       - run: echo "${{ inputs.massage }}"
   |                        ^~~~~~~~~~~~~~
test.yaml:35:28: property access of object must be type of string but got "bool" [expression]
   |
35 |       - run: echo "${{ env[inputs.verbose] }}"
   |                            ^~~~~~~~~~~~~~~
test.yaml:37:28: property access of object must be type of string but got "number" [expression]
   |
37 |       - run: echo "${{ env[inputs.age] }}"
   |                            ^~~~~~~~~~~
test.yaml:39:24: "github.event.inputs.massage" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
39 |       - run: echo "${{ github.event.inputs.massage }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:39:24: property "massage" is not defined in object type {age: string; id: string; kind: string; message: string; name: string; verbose: string} [expression]
   |
39 |       - run: echo "${{ github.event.inputs.massage }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkcFOwzAMhu97Cmvi2j5Ar5y5cUMIOa3XmrZ2VTsd07R3R+0CEwpM3KzPn538iUq1Azjq3B8GPb41bBN63a0QgGWKbtcagJuvCsBPE1Xg9OEJ9SxZu+6Ua0pQcKS7AoBOzip2swAKeMYRf4An7m8jDR0wDl7BY6eBEx7JDNvsNPOZpU1woTmoZU5QHQgl338iS/CXzRLHQHM+5ESy271r2DI5mV8n5yhWqFQQQxSPxYBrb2uZ0/T9AsVqVkB1p7B/OJ/Th5QjbgHhctn/ZZIsL8lOUV//qWN7X23ZuxhKWki8zC/0OQCbKqYK)

[`workflow_dispatch`][workflow-dispatch-event] is an event to trigger a workflow manually. The event can have parameters called
'inputs'. Each input has its name, description, default value, and [input type][workflow-dispatch-input-type-announce].

jactionlint checks several mistakes around `workflow_dispatch` configuration.

- Input type must be one of 'choice', 'string', 'number', 'boolean', 'environment'
- `options:` must be set for 'choice' input type
- The default value of 'choice' input must be included in options
- The default value of 'boolean' input must be `true` or `false`
- The default value of 'number' input must be parsed as a float number

In addition, `github.event.inputs` and `inputs` objects are typed based on the input definitions. Properties not defined in
`inputs:` will cause a type error thanks to a type checker.

For example,

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

`github.event.inputs` is typed as follows since all properties of it are strings unlike `inputs`:

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

<a id="check-glob-pattern"></a>
## Glob filter pattern syntax validation

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
test.yaml:6:10: character '^' is invalid for branch and tag names. ref name cannot contain spaces, ~, ^, :, [, ?, *. see `man git-check-ref-format` for more details. note that regular expression is unavailable. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [glob]
  |
6 |       - '^foo-'
  |          ^~~~~~
test.yaml:9:12: invalid glob pattern. unexpected character '+' while checking special character + (one or more). the preceding character must not be special character. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [glob]
  |
9 |       - 'v*+'
  |            ^~
test.yaml:11:14: invalid glob pattern. unexpected character '1' while checking character range in []. start of range '9' (57) is larger than end of range '1' (49). note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [glob]
   |
11 |       - 'v[9-1]'
   |              ^~~
test.yaml:14:9: '.' and '..' are not allowed in glob path. note: filter pattern syntax is explained at https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet [glob]
   |
14 |       - ./foo/bar.txt
   |         ^~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMjMGqAyEMRfd+xd0J76FDl/VXSgs6OJVSEtGk9PPL6MZVODmHyxQMULWX8wKpRdpL7pMAB/s4mJ0dLPG5ms/fv13odnWX+3zUKGUp/XYwbyk2L18x5sVpSMldZtSUumMK0KQk6t7xdEN1yXWZakoBeS8M7/1vADXfMEo=)

For filtering branches, tags and paths in Webhook events, [glob syntax][filter-pattern-doc] is available.
jactionlint validates glob patterns `branches:`, `branches-ignore:`, `tags:`, `tags-ignore:`, `paths:`, `paths-ignore:` in a
workflow. It checks:

- syntax errors like missing closing brackets for character range `[..]`
- invalid usage like `?` following `*`, invalid character range `[9-1]`, ...
- invalid character usage for Git ref names (branch name, tag name)
  - ref name cannot start/end with `/`
  - ref name cannot contain `[`, `:`, `\`, ...

Most common mistake I have ever seen here is a misunderstanding that regular expression is available for filtering.
This rule can catch the mistake so that users can notice their mistakes.

<a id="check-cron-syntax-and-timezone"></a>
## CRON syntax and IANA timezone string at `on.schedule`

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
test.yaml:4:13: invalid CRON format "0 */3 * *" in schedule event: expected exactly 5 fields, found 4: [0 */3 * *] [events]
  |
4 |     - cron: '0 */3 * *'
  |             ^~
test.yaml:6:13: scheduled job runs too frequently. it runs once per 60 seconds. the shortest interval is once every 5 minutes [events]
  |
6 |     - cron: '* */3 * * *'
  |             ^~
test.yaml:9:17: invalid timezone "Asia/Somewhere" in schedule event. it must be a valid IANA timezone name [events]
  |
9 |       timezone: 'Asia/Somewhere'
  |                 ^~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpkzU0KAjEMBeB9T/F2hUJbQdxk5xk8wUwNdGSmkaZF8PRSf0BxF94X3pNCBtCU+dxXHjfgkaoUgt3BxT0cnP3N3Sf/l3iA+xagLRvfpTDBHnWZ4kk2vmWubI25yKxkgMbaXtu1F/Wjqs+9tO7XadiTtPFV6d3qxyeBUxaEEB4DAMYnMuQ=)

To trigger a workflow in specific interval, [scheduled event][schedule-event-doc] can be defined in [POSIX CRON syntax][cron-syntax].

jactionlint checks the CRON syntax and frequency of running a job. [The official document][schedule-event-doc] says:

> The shortest interval you can run scheduled workflows is once every 5 minutes.

When the job is run more frequently than once every 5 minutes, jactionlint reports it as an error.

jactionlint also checks the `timezone` configuration [is a valid IANA timezone string][schedule-item-doc].

<a id="check-runner-labels"></a>
## Runner labels

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
test.yaml:10:13: label "linux-latest" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", "windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [runner-label]
   |
10 |           - linux-latest
   |             ^~~~~~~~~~~~
test.yaml:16:13: label "gpu" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", "windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [runner-label]
   |
16 |           - gpu
   |             ^~~
test.yaml:23:14: label "macos-10.13" is unknown. available labels are "windows-latest", "windows-latest-8-cores", "windows-2025", "windows-2025-vs2026", "windows-2022", "windows-11-arm", "windows-11-vs2026-arm", "ubuntu-slim", "ubuntu-latest", "ubuntu-latest-4-cores", "ubuntu-latest-8-cores", "ubuntu-latest-16-cores", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-22.04", "ubuntu-22.04-arm", "macos-latest", "macos-latest-xlarge", "macos-latest-large", "macos-26-intel", "macos-26-xlarge", "macos-26-large", "macos-26", "macos-15-intel", "macos-15-xlarge", "macos-15-large", "macos-15", "macos-14-xlarge", "macos-14-large", "macos-14", "xcode-27", "xcode-27-xlarge", "self-hosted", "x64", "arm", "arm64", "linux", "macos", "windows". if it is a custom label for self-hosted runner, set list of labels in jactionlint.yaml config file [runner-label]
   |
23 |     runs-on: macos-10.13
   |              ^~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEj8GqwyAQRfd+xV28ZZSXNmThr5QubGqTlESDo5AS8u9FNBSh0JVcZ+ZyjjUSS6CBPe2NJAO8Jh9fgLxTXvevlIBZeTeuRwJcMEa7TwY4ZtVZ4pOKLcVgGk1Yvw0upKcHHyx5fa/SWoW1ba7FlnJz2xQ//RJYpiAeLf62LSOKRIZ9zx56oQOTxwMJ3Q0WQgiWjU+yLEse9b+ozz873gMASPlVEA==)

GitHub Actions provides two kinds of job runners, [GitHub-hosted runner][gh-hosted-runner] and [self-hosted runner][self-hosted-runner].
Each runner has one or more labels. GitHub Actions runtime finds a proper runner based on label(s) specified at `runs-on:`
to run the job. So specifying proper labels at `runs-on:` is important.

jactionlint checks proper label is used at `runs-on:` configuration. Even if an expression is used in the section like
`runs-on: ${{ matrix.foo }}`, jactionlint parses the expression and resolves the possible values, then validates the values.

When you define some custom labels for your self-hosted runner, jactionlint does not know the labels. Please set the label
names in [`jactionlint.yaml` configuration file](config.md) to let jactionlint know them. A job that has the label `self-hosted`
runs on a runner of its owner, who chooses the other labels, so a label next to `self-hosted` is never reported as unknown (unless
`self-hosted-runner.strict-labels` is on). A label made of a known GitHub-hosted label and the size of a larger runner
(`ubuntu-latest-16-cores`, `ubuntu-24.04-xl`, `macos-14-xlarge`, `windows-2022-32cpu`) is accepted too; the names that an
organization gives to its larger runners are custom labels, set them in the configuration file.

In addition to checking label values, jactionlint checks combinations of labels. `runs-on:` section can be an array that contains
multiple labels. In this case, a runner which has all the labels will be selected. However, those labels combinations can have
conflicts.

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
test.yaml:4:30: label "windows-latest" conflicts with label "ubuntu-latest" defined at line:4,col:15. note: to run your job on each workers, use matrix [runner-label]
  |
4 |     runs-on: [ubuntu-latest, windows-latest]
  |                              ^~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNosi0EKgDAMBO99xT7A9gH9iniwWqgiSTEJ/b5EPS3LzDBldJMWTi6SA6BV1Be4jSQ6n60YqcVrdThhHLTzkP8vryxau3wdEL3NqFtjpJSeAQBe4h8M)

In most cases, this is a misunderstanding that a matrix combination can be specified at `runs-on:` directly. It should use
`matrix:` and expand it with `${{ }}` at `runs-on:` to run the workflow on multiple runners.

<a id="check-action-format"></a>
## Action format in `uses:`

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
test.yaml:7:15: specifying action "actions/checkout" in invalid format because ref is missing. available formats are "{owner}/{repo}@{ref}" or "{owner}/{repo}/{path}@{ref}" [action]
  |
7 |       - uses: actions/checkout
  |               ^~~~~~~~~~~~~~~~
test.yaml:9:15: specifying action "checkout@v2" in invalid format because owner is missing. available formats are "{owner}/{repo}@{ref}" or "{owner}/{repo}/{path}@{ref}" [action]
  |
9 |       - uses: checkout@v2
  |               ^~~~~~~~~~~
test.yaml:11:15: tag of Docker action should not be empty: "docker://image" [action]
   |
11 |       - uses: 'docker://image:'
   |               ^~~~~~~~~~~~~~~~~
test.yaml:13:15: specifying action ".github/my-actions/do-something" in invalid format because ref is missing. available formats are "{owner}/{repo}@{ref}" or "{owner}/{repo}/{path}@{ref}" [action]
   |
13 |       - uses: .github/my-actions/do-something
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpczbEOwyAMBNA9X+EtE0XqyNRfAWIBTbGj2K7Uv69olYXppHsnHVOAw6QuT04SFgBF0ZEAp5G44ZaM1NwrDvuRKB7yXwE4MEEJELM2JvG5Yt7ZdOKrfrzvk6wb5x3P4H3rsWBYJ7+VptWS7x93fWzshDtqbVS+AwCoTjwo)

Action needs to be specified in a format defined in [the document][action-uses-doc]. There are 4 types of actions:

- action hosted on GitHub: `owner/repo/path@ref`
- local action: `./path/to/my-action`
- self-repository action: `$/path/to/my-action`
- Docker action: `docker://image:tag`

jactionlint checks values at `uses:` sections follow one of these formats.

The self-repository form resolves against the repository running the workflow at the exact commit running it, so it
needs no checkout and takes no `@ref`. It is accepted everywhere the workspace-relative `./` form is, and jactionlint
resolves both to the same path in the repository.

Note that jactionlint does not report any error when a directory for a local action does not exist in the repository because it is
a common case where the action is managed in a separate repository and the action directory is cloned at running the workflow.
(See [#25][issue-25] and [#40][issue-40] for more details).

### Require pinning to a commit hash

By default, jactionlint accepts any ref (tag, branch, or SHA) for actions at `uses:`. Since tags and branches are mutable,
pinning to a full commit hash is recommended to mitigate supply chain attacks. This is the rule `unpinned-uses`. It is in the `default` profile. To set its level or turn it off, use [the configuration file](config.md):

```yaml
rules:
  unpinned-uses: error
```

When enabled, jactionlint reports the following at `uses:`:

- an action hosted on GitHub (`owner/repo@ref`, `owner/repo/path@ref`) whose ref is not a full-length 40-digit hexadecimal
  commit SHA (abbreviated SHAs, tags and branches are reported)
- a Docker action (`docker://image`) which is not pinned by digest, i.e. `docker://image@sha256:{64 hex digits}`
- a reusable workflow call (`owner/repo/.github/workflows/x.yml@ref`) whose ref is not a full-length commit SHA

Local actions and workflows (`./path`, `$/path`) are not reported because they always run at the commit of the workflow itself.
`uses:` values containing `${{ }}` expressions are skipped since they cannot be checked statically.

By default every action must be pinned to a full commit SHA. The `policies` option relaxes that for the repositories you trust,
like the `unpinned-uses` policies of [zizmor](https://docs.zizmor.sh/audits/#unpinned-uses):

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

- `hash-pin`: a full-length commit SHA is required (for Docker images, a digest). This is the policy of everything that no
  pattern matches.
- `ref-pin`: any tag, branch or commit SHA is accepted. For Docker images, a tag other than `latest` or a digest is required.
- `any`: nothing is required.

The patterns are `*` (everything), `owner/*`, `owner/repo` and `owner/repo/path` (the path and everything below it).
Owners and repositories are compared case-insensitively. When several patterns match, the most specific one wins whatever the order
they are written in: in the example, `actions/checkout` needs a commit SHA while the other `actions/*` actions only need a ref.
Reusable workflow calls follow the same patterns. Patterns describe repositories, so a Docker image follows the `*` pattern.
Invalid patterns and unknown policies are errors when the configuration is read.

<a id="check-local-action-checkout"></a>
### Local action used before checkout

A local action (`uses: ./.github/actions/foo`) is loaded from the workspace of the runner. When the repository has not been
checked out yet, the step fails at runtime with "Can't find 'action.yml'". This check reports the first local action of a job
which is not preceded by a checkout step in the same job. This is the rule `local-action-checkout`. It is enabled by the default
profile. jactionlint cannot know how your runner prepares the workspace, so turn the rule off when it does not apply to your
runners, in [the configuration file](config.md):

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

A job is considered to check out the repository when an earlier step in the same job

- uses an action whose name contains `checkout` (e.g. `actions/checkout`, or a wrapper such as
  `pytorch/pytorch/.github/actions/checkout-pytorch`; the owner, the repository and the path in the repository count), or
  uses a remote action with an input that only a checkout has (`fetch-depth`, `persist-credentials`, `submodules`,
  `sparse-checkout`, `sparse-checkout-cone-mode`, `lfs`, `fetch-tags`, `set-safe-directory`), or
- has a `run:` script with a line containing `git` and one of `clone`, `init`, `fetch`, `pull`, `checkout`, `worktree` or
  `submodule`, or `gh repo clone` / `gh pr checkout`.

Known limitations (use `ignore` in the configuration file when they matter):

- Whether the checkout step actually runs (`if:`) is not considered.
- A self-hosted runner may keep the workspace between jobs, so a job without a checkout step can work there.
- A step which generates the action directory without checking out the repository is reported.
- What a remote composite action does cannot be known without fetching it, so the name and the inputs are a heuristic: a wrapper
  with an unrelated name and none of the inputs above is not taken for a checkout and the next local action is reported.
- `uses: $/path` (self-repository syntax) is never reported since it does not need a checkout. `uses:` of reusable workflows and
  composite action files are not checked.
### Require `${{ }}` in `if:` conditions (pedantic)
<a id="check-require-expression-wrapping"></a>

GitHub Actions allows omitting `${{ }}` in `if:` conditions of jobs and steps. Some projects prefer to always write it so
that it is obvious an expression is used. This is the rule `require-expression-wrapping`. It is in the `pedantic` profile, so it is off unless you choose that profile. To turn it on alone, set it in [the configuration file](config.md):

```yaml
rules:
  require-expression-wrapping: error
```

When enabled, `if: github.ref == 'refs/heads/main'` is reported and `if: ${{ github.ref == 'refs/heads/main' }}` is accepted.
Other keys are not affected because they always need `${{ }}` to evaluate an expression.

### Falsy value in the `a && b || c` ternary idiom
<a id="check-falsy-ternary"></a>

`cond && b || c` is commonly used as a ternary operator, but it works only when `b` is truthy. Otherwise the result is always
`c`. This is the rule `unsound-ternary`. It is enabled by the default profile. To turn it off, set it in
[the configuration file](config.md):

```yaml
rules:
  unsound-ternary: off
```

jactionlint reports the idiom when `b` is a literal which is always falsy: `''`, `0`, `false` or `null`.
Non-literal values (e.g. `github.sha`) are not reported because whether they are falsy is unknown statically.

```yaml
env:
  # Always evaluated to 'staging-' regardless of the condition
  PREFIX: ${{ env.DEPLOY == 'true' && '' || 'staging-' }}
```

<a id="check-local-action-inputs"></a>
## Local action inputs validation at `with:`

My action definition at `.github/actions/my-action/action.yaml`:

```yaml
name: 'My action'
author: 'rhysd <https://rhysd.github.io>'
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

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:7:15: missing input "message" which is required by action "My action" defined at "./.github/actions/my-action". all required inputs are "message" [action]
  |
7 |       - uses: ./.github/actions/my-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:13:11: input "additions" is not defined in action "My action" defined at "./.github/actions/my-action". available inputs are "addition", "message", "name" [action]
   |
13 |           additions: foo, bar
   |           ^~~~~~~~~~
```

<!-- Skip playground link -->

When an action in the same repository is run in `uses:` of `step:` (with either the `./` or the `$/` form), jactionlint
reads the `action.yml` file in that action's directory and validates inputs at `with:` in the workflow are correct.
Missing required inputs and unexpected inputs can be detected.

<a id="check-popular-action-inputs"></a>
## Popular action inputs validation at `with:`

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
test.yaml:7:15: missing input "key" which is required by action "actions/cache@v4". all required inputs are "key", "path" [action]
  |
7 |       - uses: actions/cache@v4
  |               ^~~~~~~~~~~~~~~~
test.yaml:9:11: input "keys" is not defined in action "actions/cache@v4". available inputs are "enableCrossOsArchive", "fail-on-cache-miss", "key", "lookup-only", "path", "restore-keys", "save-always", "upload-chunk-size" [action]
  |
9 |           keys: |
  |           ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEjrHKwkAQhPs8xRQ/5DeQpLG6ysr32ByLFy/eHdlbRWLeXRIliI3VsvN9MBODQVJxRXGOnZgCyCx5ucCoQepF0E5D1nqgha1IMid5WUANFRYDsrmPQVpL1vHhun9j4NZnZ7YP8HwXg8dHAvxNExyJO/YDy39ZVW3VDNH6cod5/mmuld9qouwMmjaR9XRi2eaOGgwu5Pk5APtlRBU=)

jactionlint checks inputs of many popular actions such as `actions/checkout@v4`. It checks

- some input is required by the action but it is not set at `with:`
- input set at `with:` is not defined in the action (this commonly occurs by a typo)

this is done by checking `with:` section items with a small database collected at building `jactionlint` binary. jactionlint
can check popular actions without fetching any `action.yml` of the actions from the remote so that it can run efficiently.

Note that it only supports the case of specifying major versions like `actions/checkout@v4`. Fixing version of action like
`actions/checkout@v4.0.1` and using the HEAD of action like `actions/checkout@main` are not supported for now.

So far, jactionlint supports more than 100 popular actions The data set is embedded at [`popular_actions.go`](https://github.com/jdx/jactionlint/blob/main/popular_actions.go)
and were automatically collected by [a script][generate-popular-actions]. If you want more checks for other actions, please
make a request [as an issue][issue-form].

<a id="detect-outdated-popular-actions"></a>
## Outdated popular actions detection at `uses:`

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
test.yaml:8:15: the runner of "actions/checkout@v3" action is too old to run on GitHub Actions. update the action's version to fix this issue [action]
  |
8 |       - uses: actions/checkout@v3
  |               ^~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNokyjEOxCAMRNGeU8wF0BbbUe1VAFlik8hGGTvnj0iqX/xnWjCDI6XNGksCXOirwBnKvEC0UI981PWeRZfJVwEZQWFB7f435acP6buF/67vPQB0iR3O)

In addition to the checks for inputs of actions described in [the previous section](#check-popular-action-inputs), jactionlint
reports an error when a popular action is 'outdated'. An action is outdated when the runner used by the action is no longer
supported by GitHub Actions runtime. For example, `node12` is no longer available so any actions can not use `node12` runner.

Note that this check doesn't report that the action version is up-to-date. For example, even if you use `actions/checkout@v4` and
newer version `actions/checkout@v5` is available, jactionlint reports no error as long as `actions/checkout@v4` is not outdated.
If you want to keep actions used by your workflows up-to-date, consider to use [Dependabot][dependabot-doc].

<a id="check-shell-names"></a>
## Shell name validation at `shell:`

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
test.yaml:8:16: shell name "dash" is invalid. available names are "bash", "pwsh", "python", "sh" [shell-name]
  |
8 |         shell: dash
  |                ^~~~
test.yaml:11:16: shell name "powershell" is invalid on macOS or Linux. available names are "bash", "pwsh", "python", "sh" [shell-name]
   |
11 |         shell: powershell
   |                ^~~~~~~~~~
test.yaml:17:16: shell name "fish" is invalid. available names are "bash", "pwsh", "python", "sh" [shell-name]
   |
17 |         shell: fish
   |                ^~~~
test.yaml:27:16: shell name "sh" is invalid on Windows. available names are "bash", "cmd", "powershell", "pwsh", "python" [shell-name]
   |
27 |         shell: sh
   |                ^~
```

[Playground](https://jactionlint.jdx.dev/#eNqkkMHKgzAQhO8+xdw8Cf85bxN1Jf6s2eBmsVD67mWtlOKp2Nsk85F8jOSAYpqaf+k1NADP2W4egNWydg5Yb7lax7GS1r3SSkVfFNA5GUBDErSJmKU9GkD9HDBGTd/TRTZa99wASxxOOkscRD9tRpqicX0L+QfnN6dZ0yX1ttDKuP89vNnmPMqmJ6Hj9peBrs3zHAA+f36k)

Available shells for runners are defined in [the documentation][shell-doc]. jactionlint checks shell names at `shell:`
configuration are properly using the available shells.

<a id="check-run-policy"></a>
## Run script policy (pedantic)

These are the rules `require-shell` and `max-run-lines`. They are in the `pedantic` profile, so they are off unless you choose that profile. Enable
them alone in [the config file](config.md).

```yaml
rules:
  require-shell: error
  max-run-lines:
    max: 3
```

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:7:9: shell is not set explicitly. set "shell:" at the step or "defaults.run.shell" because "require-shell" is enabled [run-policy]
  |
7 |       - run: echo hello
  |         ^~~~
test.yaml:12:14: script in "run:" has 4 lines but at most 3 lines are allowed because "max-run-lines" is set. consider moving it to a script file or an action [run-policy]
   |
12 |       - run: |
   |              ^
```

<!-- Skip playground link -->

`require-shell` accepts `shell:` of the step, `defaults.run.shell` of the job, and `defaults.run.shell` of the workflow. When
the shell is omitted, GitHub Actions runs `bash -e {0}` while `shell: bash` runs `bash --noprofile --norc -eo pipefail {0}`
so the behavior differs (for example, a failure in the middle of a pipe is not detected). Note that composite actions always
need `shell:` so they are not affected.

`max-run-lines` counts non-blank lines in a `run:` script. Comment lines are counted. Its default `max` is 100 lines when the rule
is enabled by a profile.

<a id="check-job-step-ids"></a>
## Job ID and step ID uniqueness

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
test.yaml:10:13: step ID "STEP_ID" duplicates. previously defined at line:7,col:13. step ID must be unique within a job. note that step ID is case insensitive [id]
   |
10 |         id: STEP_ID
   |             ^~~~~~~
test.yaml:12:3: key "TEST" is duplicated in "jobs" section. previously defined at line:3,col:3. note that this key is case insensitive [syntax-check]
   |
12 |   TEST:
   |   ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNrKz7NSKCgtzuDKyk8qtuJSUChJLS4B0QoKRaV5xbog+dKk0rySUt2cRJAcWKq4JLWgGKJKQUEXpNJKITU5I19BPSM1JydfHSqjoJCZYgVWHJ+Zgk11UmUqqtrgENeAeE8XLgWFENfgEJq4AzAAioFDag==)

Job IDs and step IDs in each jobs must be unique. IDs are compared in case-insensitive. jactionlint checks all job IDs
and step IDs, and reports errors when some IDs duplicate.

<a id="check-hardcoded-credentials"></a>
## Hardcoded credentials

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
test.yaml:10:19: "password" section in "container" section should be specified via secrets. do not put password value directly [credentials]
   |
10 |         password: pass
   |                   ^~~~
test.yaml:17:21: "password" section in "redis" service should be specified via secrets. do not put password value directly [credentials]
   |
17 |           password: pass
   |                     ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0kLFuxSAMRff3Fd6Y0rfzNzxy1VCBjWxo+vkVNKVLOmGd48tFCHuq3Y7Hh7zMP4garI2TSDvbNnx/dW59y2G4qaJwC4mhP5tEqYR3eHL4CqVmvEUpTzkZ+pzGXWtRsYNbCtl+k0TdoBwK/JwWrsHsFN39nCY26GeKWFnFnuzvousRky54W/lv6X1tQ13hbfyLJ8RDyB3IWdz3AFJLXcM=)

[Credentials for container][credentials-doc] can be put in `container:` configuration. Password should be put in secrets
and the value should be expanded with `${{ }}` syntax at `password:`. jactionlint checks hardcoded credentials, and reports
them as an error.

<a id="check-github-env"></a>
## Dangerous writes to `GITHUB_ENV` and `GITHUB_PATH`

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
test.yaml:8:48: a value that is not a literal is written to $GITHUB_ENV in a workflow triggered by "pull_request_target", which runs with secrets and a write token for events that may come from a fork. an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps. write only literal values and values computed from trusted sources, or pass state with $GITHUB_OUTPUT [github-env]
  |
8 |       - run: echo "VERSION=$(cat version.txt)" >> "$GITHUB_ENV"
  |                                                ^~
test.yaml:9:34: untrusted input from the variable TITLE (github.event.pull_request.title) is written to $GITHUB_ENV. an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps. do not write input that an outsider controls to $GITHUB_ENV; validate it first or pass it to the next step with $GITHUB_OUTPUT [github-env]
  |
9 |       - run: echo "TITLE=$TITLE" >> "$GITHUB_ENV"
  |                                  ^~
```

[Playground](https://jactionlint.jdx.dev/#eNp0z8FKw0AQBuB7n+In5KCH5AEW2oMQNCAVNPYiEpJ2aCPL7LozE5TSd5ckIB7saZj5v/8wgd0KiOZ9m+jTSLTVLh1JpzOg35HE4S1EYjq8rz5CL1PS2+APC0nGUgR2sN5YrfCdkugciVKURQHFJB1ofwrIdtXzS/20Xec3+04xUpIhcKlfepths0GW39fNw+tdW2132X/9pm4eq3U+j6sNgHh0vwswc4f8fMZx0JP1JY3EWv59v9RBPeFy+RkA2itTFQ==)

What a step writes to the file `$GITHUB_ENV` becomes the environment of every later step of the job, and a directory written to
`$GITHUB_PATH` is searched for executables first. If an attacker controls what is written, they can run code in the next
steps: a value with a newline sets another variable, and `LD_PRELOAD` or `NODE_OPTIONS` load code of their choice, while a
directory in front of the `PATH` can shadow a program such as `ssh`. See
[Keeping your GitHub Actions and workflows secure: preventing pwn requests](https://securitylab.github.com/resources/github-actions-preventing-pwn-requests/).

The rule `github-env` (in the `default` profile) reports two kinds of writes:

- A write of something that is not a literal in a workflow started by `pull_request_target` or
  `workflow_run`. These run with secrets and a write token while the event may come from a fork, so anything computed from the
  checked out code, its artifacts or the event is suspect. Values that the workflow author or GitHub decide are accepted:
  literals, the `HOME` and `RUNNER_*` variables, `github.sha`, `github.run_id`, `github.event.pull_request.head.sha`, the
  `runner`, `matrix`, `vars` and `secrets` contexts, command substitutions of `mktemp`, `date`, `pwd`, `uname` and the like with
  such arguments, and variables of `env:` or of the script that are set to such values.
- A write of input that an outsider controls, whatever the trigger: an expression such
  as `github.event.issue.title` or `github.head_ref`, or an environment variable that was set to one (as `TITLE` is in the
  example above; a template injection check does not see that one).
  In the metadata of a composite action, `inputs.*` counts as such an input (also through `env:`): the caller chooses it, and a
  workflow can pass it the title of an issue. This holds without a calling workflow, the action may be used by other repositories.

`echo "VERSION=1.0" >> "$GITHUB_ENV"` and `echo "$HOME/.cargo/bin" >> "$GITHUB_PATH"` are fine. Use `$GITHUB_OUTPUT` to pass
state between steps (`echo "version=$(cat version.txt)" >> "$GITHUB_OUTPUT"` is not reported) and validate or avoid the value
otherwise. The rule understands `>>` and `>`, `tee`, groups (`{ ...; } >> "$GITHUB_ENV"`), here documents and a file name held
in another variable. Scripts of `bash` and `sh` are parsed; for `pwsh`, `powershell` and `cmd` the rules look at the lines that
mention `$env:GITHUB_ENV` or `%GITHUB_ENV%` together with a redirection or `Out-File`, `Add-Content`, `Set-Content` and
`Tee-Object`.

To turn the rule off, put `github-env: off` in the `rules` section of the [configuration file](config.md) or write
`# jactionlint ignore=github-env` after a reason.

<a id="check-adhoc-packages"></a>
## Packages installed by name

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
  |              ^~~
test.yaml:7:14: command "gem install" installs a package outside of a lock file: its version and its dependencies are resolved anew on every run. add the package to a Gemfile and commit the Gemfile.lock and install with `bundle install` [adhoc-packages]
  |
7 |       - run: gem install rake
  |              ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNpUyjEOAjEMRNF+TzEXyGpbUnGVrGRBwDhRxr4/MhSI6hf/DauYwfv2GCfrBrjQs8AKY8kfZ5hH0Zbvs+gy+VVASVlh84Vu9KYKoXbz62U/9uNf3eSnVnvKewDexCcZ)

The rule `adhoc-packages` (in the `default` profile) reports a `run:` script that installs a package by name with `npm`, `yarn`,
`pnpm`, `bun`, `gem` or `bundle add`. Such a package is usually not pinned, so the newest release (and so a compromised one)
is picked up. Even with `eslint@9.0.0` the dependencies of the package are resolved anew on every run. Commands like
`yarn add` and `bundle add` change the lock file of the run instead of using it.

Add the package to a manifest that produces a lock file (`package.json`, a `Gemfile`), commit the lock file and install with
a command that follows it: `npm ci`, `yarn install --immutable`, `pnpm install --frozen-lockfile`, `bun ci` or
`bundle install`. Installing from a manifest (`npm install`, `npm ci`), from the checkout (`npm install .`) and from a gem file
(`gem install ./pkg.gem`) is not reported. See [unlocked installs](#check-unlocked-install) for the lock file.

Installing a tool for the workflow itself, such as `pip install` and `cargo install`, is covered by
[unpinned tools](#check-unpinned-tools). The rule is the audit `adhoc-packages` of zizmor, which does not look at `pip`.

<a id="check-unpinned-tools"></a>
## Tools installed without an exact version

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

Pinning an action to a commit does not pin the tool that the action downloads. Some actions install the newest release of their
tool unless they are told which one to use. The rule `unpinned-tools` (in the `default` profile) reports a step that uses one
of these actions (`aquasecurity/setup-trivy`, `1password/load-secrets-action`, `extractions/setup-just` and
`extractions/setup-crate`, the ones zizmor knows) and does not set the input that selects the version, or sets it to
`latest` (`*` for the last two). Set the input to an exact version. A value that is an expression is not judged.

With the option `pedantic` (on under `profile: pedantic`, or `rules: {unpinned-tools: {level: warn, pedantic: true}}`) the
rule reports more:

Example input:

```yaml
on: push
jobs:
  tools:
    runs-on: ubuntu-latest
    steps:
      - uses: hashicorp/setup-terraform@b9cd54a3c349d3f38e8881555d616ced269862dd # v3.1.2
      - run: pip install requests black==24.3.0
      - run: go install golang.org/x/tools/cmd/stringer@latest
```

Output:
<!-- Skip update output -->
```
test.yaml:6:15: warning: action "hashicorp/setup-terraform@b9cd54a3c349d3f38e8881555d616ced269862dd" installs the newest version of its tool because the input "terraform_version" is not set. set "terraform_version" to an exact version [unpinned-tools]
  |
6 |       - uses: hashicorp/setup-terraform@b9cd54a3c349d3f38e8881555d616ced269862dd # v3.1.2
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:7:14: warning: tool "requests" is installed without an exact version, so every run may fetch a different release. pin it with `==`, for example `tool==1.2.3` [unpinned-tools]
  |
7 |       - run: pip install requests black==24.3.0
  |              ^~~
test.yaml:8:14: warning: tool "golang.org/x/tools/cmd/stringer@latest" is installed without an exact version, so every run may fetch a different release. pin it with a version, for example `tool@v1.2.3` [unpinned-tools]
  |
8 |       - run: go install golang.org/x/tools/cmd/stringer@latest
  |              ^~
```

<!-- Skip playground link -->

- further actions that use the newest version of their tool by default: `hashicorp/setup-terraform`, `azure/setup-kubectl`
  and `azure/setup-helm` (the table is `floatingToolActions` in `rule_unpinned_tools.go`, with the source of each entry);
- `run:` scripts that install or run a tool without an exact version: `pip install`, `pipx install` and `pipx run`,
  `uv tool install`, `uv pip install` and `uvx`, `cargo install` and `cargo binstall`, `go install`, `npm install -g`,
  `npx --yes`, `pnpm dlx` and `yarn dlx`. One finding names all the tools of a command. Pin them with `==`, `--version`,
  `@v1.2.3` or `@1.2.3` according to the tool. Packages from a path, requirements files (`pip install -r`) and `npx tsc`,
  which runs the program of the project, are not reported. There is no fix since the version to use is the decision of the
  author.

Installs of the packages of a project are the business of the lock file, see [unlocked installs](#check-unlocked-install).
zizmor 1.30.1 only has the first rule, and only for the four actions above.

<a id="check-use-trusted-publishing"></a>
## Publishing with long-lived credentials

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
  |              ^~~~~
test.yaml:13:21: action "pypa/gh-action-pypi-publish@76f52bc884231f62b9a034ebfe128415bbaabdfc" publishes to PyPI but is given a password (input "password") instead of using trusted publishing. prefer trusted publishing with pypa/gh-action-pypi-publish and the permission "id-token: write" [use-trusted-publishing]
   |
13 |           password: ${{ secrets.PYPI_TOKEN }}
   |                     ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8jsFKw0AQQO/9igE9CduaNK1xTwr2UIQ22EIRkbKbTMxK2F0ysw2h9N8lNgRP3mbmPZjnrJwANFijIuxHAO48koQPH3RtqMLic/LtNPVwOF29JlgSzkoIOlgOolaMxL+IGD1dLQDRmxK4NRYh+NqpAgpDPLsbBAC0JzkuAPvDerM6Zs+73WH79iLh9nwGwrxBpmn2nq2P++3ragOXy/ghUJ/sO69mX5VQORtnhe+8EUPy08OyXMQ6T9MknkflMtaP6n6eoC4xitMkWmitlC7KHG7gFE2jeJqMQa3h6m+eV0Sta4p/wn4GAGw+ZHU=)

PyPI, crates.io, RubyGems, npm and NuGet support [trusted publishing](https://docs.pypi.org/trusted-publishers/): the registry
trusts the OIDC token that GitHub issues for the workflow run, so no API token has to be stored as a secret, and a stolen
token cannot publish. The rule `use-trusted-publishing` (in the `default` profile) reports

- a `run:` script that publishes to the public registry: `twine upload`, `uv publish`, `poetry|flit|hatch|pdm publish`,
  `cargo publish`, `npm|pnpm|yarn|bun publish`, `gem push`, `dotnet nuget push` and `nuget push`, also behind `sudo`,
  `uvx`, `pipx run`, `uv run`, `bundle exec` and `python -m`. The message names the long-lived credential when the step,
  its job or the workflow sets one of the usual environment variables (`TWINE_PASSWORD`, `NODE_AUTH_TOKEN`,
  `CARGO_REGISTRY_TOKEN`, `GEM_HOST_API_KEY`, ...);
- the actions `pypa/gh-action-pypi-publish` with `password`, `rubygems/configure-rubygems-credentials` with `api-token`,
  `rubygems/release-gem` with `setup-trusted-publisher: false` and `actions/setup-node` with an npm registry and
  `always-auth: true`.

A job that grants `id-token: write` (itself or through the workflow) is accepted, since it can use trusted publishing, and so
are dry runs and publishing to a registry of your own (`--registry`, `--repository-url`). Configure the trusted publisher at the
registry once, then use `pypa/gh-action-pypi-publish`, `rubygems/release-gem`, `rust-lang/crates-io-auth-action`, `NuGet/login`
or `npm publish` with that permission.

`id-token: write` does not silence the rule by itself, because jobs ask for it to sign in to a cloud provider or to publish with
provenance: a publish command that is given a long-lived credential (an environment variable such as `NODE_AUTH_TOKEN` or
`CARGO_REGISTRY_TOKEN`, or `--token`) is reported even when the job can request the OIDC token. Scripts of `shell: pwsh` (the default
shell of the Windows runners) and `powershell` are searched for the publish commands line by line.

A credential variable that does not hold a long-lived credential is not one: `NODE_AUTH_TOKEN: ''` blanks the placeholder token
that `actions/setup-node` writes, so that npm falls back to the OIDC token (the documented way to publish to npm with provenance),
and a variable set from the output of `rust-lang/crates-io-auth-action` or `NuGet/login` of the same job (for example
`CARGO_REGISTRY_TOKEN: ${{ steps.auth.outputs.token }}`) holds a token that is valid for minutes. Both are trusted publishing.

<a id="check-superfluous-actions"></a>
## Superfluous actions

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

Some actions only run a command that the runner image already has. The action is a dependency that can be compromised, with
nothing gained. The rule `superfluous-actions` (in the `default` profile) reports the ones with a simple replacement: the release
actions (`softprops/action-gh-release`, `ncipollo/release-action`, `elgohr/Github-Release-Action`,
`svenstaro/upload-release-action` and the archived `actions/create-release` and `actions/upload-release-asset`), which
`gh release` replaces, `dacbd/create-issue-action`, `actions-ecosystem/action-add-labels` and `action-remove-labels` (`gh issue`,
`gh pr`), `addnab/docker-run-action` (`docker run`) and `sergeysova/jq-action` (`jq`).

With the option `pedantic` (on under the `pedantic` profile) the rule reports the ones whose replacement takes several commands or
misses a feature: `peter-evans/create-pull-request`, `peter-evans/create-or-update-comment`, `dtolnay/rust-toolchain`,
`stefanzweifel/git-auto-commit-action` and `EndBug/add-and-commit`. Each entry of the table `superfluousActions` in
`rule_superfluous_actions.go` names its source (the audit of zizmor, or the README of the archived action).

Nothing is reported for a job that runs on a `self-hosted` runner, because the tools of the GitHub-hosted images are not there and the
action is what installs them.

<a id="check-unlocked-install"></a>
## Installs without a lock file

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
  |              ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNoky90JwCAMxPF3p7gFsoDbaJF+IFG8BDp+SX0K4fe/oRnTeaVnVOYEWKPFBZYrJdyrq7n0EvYTrU3uCpAoM46yzoFbaaX3/Ym2NzbfAESiIIE=)

An install that does not use a lock file resolves the dependencies anew on every run, so a new release of any transitive
dependency, a compromised one included, reaches the workflow. The rule `unlocked-install` (in the `default` profile) reports
`cargo install` without `--locked` (or `--frozen`): the crate was published with a `Cargo.lock`, and `--locked` builds with it.

The rule is fixable. `jactionlint -fix=unsafe` inserts `--locked`. The fix is **unsafe** because the build can fail where it
succeeded, for example when the lock file of the crate is out of date. It is only offered when the position of `install` in the
file is known for sure.

The rule also reports installs from a manifest that ignore the lock file of the repository. They are reported when the lock file
exists, because only then it is being ignored: the root of the repository (or the `working-directory` of the step) has the file of
the tool, `package-lock.json` (or `npm-shrinkwrap.json`) for npm, `yarn.lock` for Yarn or `pnpm-lock.yaml` for pnpm.

- `npm install` without a package: use `npm ci`, which fails when `package-lock.json` is out of date;
- `yarn` and `yarn install` without `--immutable` (`--frozen-lockfile` in Yarn 1) and `pnpm install --no-frozen-lockfile`. pnpm freezes
  the lock file in CI by default, so plain `pnpm install` is not reported. Yarn 2+ does so too, so for Yarn the flag makes it
  explicit.

With the option `pedantic` (on under the `pedantic` profile) the rule reports these installs without a lock file in the repository
as well (a lock file may be created in the job, or not exist yet), and two more kinds of installs: `bun install` without
`--frozen-lockfile` (or use `bun ci`), and `pip install -r` (also `uv pip install -r`) without `--require-hashes` or a constraints
file `-c`.

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: npm install
      - run: pip install -r requirements.txt
```

Output:
<!-- Skip update output -->
```
test.yaml:6:14: "npm install" resolves the dependencies again and may update the package-lock.json instead of failing when it is out of date. use `npm ci` [unlocked-install]
  |
6 |       - run: npm install
  |              ^~~
test.yaml:7:14: "pip install -r requirements.txt" installs a requirements file without hashes or constraints, so the transitive dependencies are resolved anew on every run. use a lock file with hashes (`pip-compile --generate-hashes`) and pass --require-hashes, or pin them with -c [unlocked-install]
  |
7 |       - run: pip install -r requirements.txt
  |              ^~~
```

<!-- Skip playground link -->

This is the output with the option `pedantic`, or with a `package-lock.json` in the repository for the first finding.

Installing named packages is covered by [adhoc packages](#check-adhoc-packages) and [unpinned tools](#check-unpinned-tools).
These rules have no equivalent in zizmor.

<a id="check-env-var-names"></a>
## Environment variable names

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
test.yaml:6:7: environment variable name "FOO=BAR" is invalid. '&', '=' and spaces should not be contained [env-var]
  |
6 |       FOO=BAR: foo
  |       ^~~~~~~~
test.yaml:7:7: environment variable name "FOO BAR" is invalid. '&', '=' and spaces should not be contained [env-var]
  |
7 |       FOO BAR: foo
  |       ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNrKz7NSKCgtzuDKyk8qtuJSUChJLS4B0QoKRaV5xbog+dKk0rySUt2cRJAcWCo1rwyiRkHBzd/f1skxyEohLT8fIaSAIlRcklpQDNOgCzLYSiE1OSNfQT0jNScnXx0wAPhYJMc=)

`=` must not be included in environment variable names. And `&` and spaces should not be included in them. In almost all
cases they are mistakes, and they may cause some issues on using them in shell since they have special meaning in shell syntax.

jactionlint checks environment variable names are correct in `env:` configuration.

<a id="permissions"></a>
<a id="check-permissions"></a>
## Permissions

Example input:

```yaml
on: push

# ERROR: Available values for whole permissions are "write-all", "read-all" or "none"
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
test.yaml:4:14: "write" is invalid for permission for all the scopes. available values are "read-all", "write-all" or {} [permissions]
  |
4 | permissions: write
  |              ^~~~~
test.yaml:11:7: unknown permission scope "check". all available permission scopes are "actions", "artifact-metadata", "attestations", "checks", "code-quality", "contents", "copilot-requests", "deployments", "discussions", "id-token", "issues", "models", "packages", "pages", "pull-requests", "repository-projects", "security-events", "statuses", "vulnerability-alerts" [permissions]
   |
11 |       check: write
   |       ^~~~~~
test.yaml:13:15: "readable" is invalid as permission of scope "issues". available values are "read", "write", "none" [permissions]
   |
13 |       issues: readable
   |               ^~~~~~~~
test.yaml:15:15: "write" is invalid as permission of scope "models". available values are "read", "none" [permissions]
   |
15 |       models: write
   |               ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMjdENwyAMBf+Z4i3AAmwDxBK0BCMeVtevSJUqX5bu7LP2gGEszg2ZZyWrdgZ8Zl3i3EsTgwOWcO0JTOv0+8iS9WW+xe0u9QxcAMhF8vuu/VAlTRgwJR4xtRufekjjc5VLxj/k9+MAyUVRpDX9DgAfnji8)

Permissions of `GITHUB_TOKEN` token can be configured at workflow-level or job-level by [`permissions:` section][perm-config-doc].
Each permission scopes have their access levels. The default levels and available levels are described in
[the document][permissions-doc].

jactionlint checks permission scopes and access levels in a workflow are correct.

### Require explicit permissions

When `permissions:` is not set, `GITHUB_TOKEN` gets the default permissions of the repository or organization, which may be
read-write. Setting permissions explicitly follows the principle of least privilege. This is the rule `missing-permissions`. It is in the `default` profile. To set its level or turn it off, use [the configuration file](config.md):

```yaml
rules:
  missing-permissions: error
```

When enabled, jactionlint reports every job that is not covered by `permissions:`, i.e. the workflow has no top-level
`permissions:` and the job has no `permissions:` of its own. Either of them is enough, and `permissions: {}` (no permissions)
counts as explicit. Jobs which call reusable workflows (`uses:`) are checked in the same way because the caller limits the
permissions of the callee. The error is reported at the job so that it is easy to see which jobs need a fix.

#### Fixing missing permissions

`jactionlint -fix` adds this to the workflow, right after the `on:` block:

```yaml
permissions:
  contents: read
```

`contents: read` takes every other permission away from `GITHUB_TOKEN`, so it breaks a job which comments on a pull request,
pushes a commit, publishes a package or a release and so on. jactionlint cannot see inside the actions and scripts a job runs, so
the fix is only **safe** (applied by `-fix`) when the workflow gives no sign that the token is needed: every action of the jobs
which have no `permissions:` is a well-known one that only reads the repository (`actions/checkout`, `actions/setup-*`,
`actions/cache`, `actions/upload-artifact`, `jdx/mise-action` and a few more), the jobs do not call reusable workflows, and
the text of the workflow does not mention `GITHUB_TOKEN`, `github.token`, `GH_TOKEN`, the `gh` command, `git push` or
`api.github.com`. In every other case the fix is **unsafe** and `-fix=unsafe` applies it; review the result. A script file that the
workflow runs and that pushes with the credentials left by `actions/checkout` is not visible, so check such a workflow before
accepting the safe fix.

<a id="check-excessive-permissions"></a>
## Excessive permissions

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

Output:
<!-- Skip update output -->
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

The rule `excessive-permissions` reports write access that the `GITHUB_TOKEN` gets without a narrow need. It is in the `default` profile. The output above is from the following `rules` section of [the configuration
file](config.md):

```yaml
rules:
  excessive-permissions: error
```

It reports:

- a write scope in the `permissions:` of the workflow. Every job inherits it, including the jobs which only build or test.
  The message says what the scope allows (`contents: write` pushes commits and tags, `id-token: write` requests OIDC tokens,
  `packages: write` publishes packages, and so on). When the workflow runs on `pull_request_target`, `workflow_run` or
  `issue_comment`, which people without write access can trigger, the message says so.
- `write-all` and `read-all`, at the workflow level or on a job. They grant a level to every scope, including scopes which the
  job never uses.

Write scopes on a job are not reported: that is where the access should be granted. The best practice is `permissions: {}` at
the workflow level and the scopes a job needs on that job. Read scopes and `none` are not reported either.

A job which has no `permissions:` in a workflow which has none is the business of the [`missing-permissions`](#permissions)
rule, which reports the jobs the default permissions of the repository apply to. To also have a workflow without a top-level
`permissions:` reported when its jobs set their own, turn on `require-workflow-permissions`:

```yaml
rules:
  excessive-permissions:
    level: warn
    require-workflow-permissions: true
```

[zizmor](https://docs.zizmor.sh/audits/#excessive-permissions) reports both cases as `excessive-permissions`.

To silence one finding, put `# jactionlint ignore=excessive-permissions` above the scope, or list the rule in `paths.*.ignore`
of the configuration for workflows which need it (a release workflow with a single job, for example).

<a id="check-undocumented-permissions"></a>
## Undocumented permissions (pedantic)

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

Output:
<!-- Skip update output -->
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

The rule `undocumented-permissions` requires a comment which explains why a permission scope above `read` is granted. Permissions
are easy to add and never revisited, so a sentence next to each one keeps the list honest and makes a review of the workflow
quick. It is in the `pedantic` profile because it is a style check. The output above is from the
following `rules` section of [the configuration file](config.md):

```yaml
rules:
  undocumented-permissions: error
```

A scope counts as documented when a comment is at the end of its line (`issues: write # close stale issues`) or when comment
lines are directly above it, with no blank line in between. The comment above `permissions:` does not document the scopes below
it. Comments which only configure a tool, such as `# jactionlint ignore=...`, do not explain anything. Both the workflow and job
`permissions:` are checked; `write-all` and `read-all` are the business of `excessive-permissions`.

With the `include-read` option the scopes granted with `read` need a comment too, except `contents: read`, which every job needs
to check out the repository:

```yaml
rules:
  undocumented-permissions:
    level: warn
    include-read: true
```

[zizmor](https://docs.zizmor.sh/audits/#undocumented-permissions) behaves like `include-read: true`, and counts only a comment at the end of the line.

<a id="check-unpinned-images"></a>
## Unpinned container images

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

Output:
<!-- Skip update output -->
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

The rule `unpinned-images` reports `container:` and `services:` images which are not pinned by a digest. A tag can be moved to
another image by whoever controls the registry repository, and an image without a tag is `latest`, so what runs in your job can
change without any change to the workflow. It is in the `default` profile. The output above is
from the following `rules` section of [the configuration file](config.md):

```yaml
rules:
  unpinned-images: error
```

Images are pinned with their content digest: `postgres:16@sha256:{64 hex digits}`. Keep the tag next to the digest so that
Dependabot and Renovate can update both. `docker inspect postgres:16 --format='{{index .RepoDigests 0}}'` prints the digest of
an image you have pulled.

An image without a tag and an image with the tag `latest` are reported with a stronger message. To report only those two and
accept tags (which is what zizmor does by default), turn off the `require-digest` option:

```yaml
rules:
  unpinned-images:
    level: warn
    require-digest: false
```

Images written with an expression (`image: ${{ vars.IMAGE }}`) are skipped since they cannot be checked statically. Docker
images used as actions (`uses: docker://alpine:3.20`) are checked by the [`unpinned-uses`](#check-action-format) rule.

<a id="check-self-repository"></a>
## Self-repository syntax (pedantic)

Example input:

```yaml
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

Output:
<!-- Skip update output -->
```
test.yaml:11:15: "./.github/actions/setup" is looked up in the workspace at run time, where an earlier step can replace it. use the self-repository syntax "$/.github/actions/setup" which always refers to the commit running the workflow [self-repository]
   |
11 |       - uses: ./.github/actions/setup
   |               ^~~~~~~~~~~~~~~~~~~~~~~
test.yaml:16:11: "./.github/workflows/reusable.yml" is looked up in the workspace at run time, where an earlier step can replace it. use the self-repository syntax "$/.github/workflows/reusable.yml" which always refers to the commit running the workflow [self-repository]
   |
16 |     uses: ./.github/workflows/reusable.yml
   |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The rule `self-repository` reports `uses: ./path` and asks for `uses: $/path`. Both name an action or reusable workflow of the
repository that runs the workflow. `./path` is looked up in the workspace of the runner, so any earlier step can change what it
runs (a checkout of another ref or repository, or a script writing an `action.yml`), and a policy cannot tell it from an
arbitrary directory. `$/path` always resolves to the commit that runs the workflow. It is in the `pedantic` profile and reported as `info`, because the syntax is new and some GitHub Enterprise Server versions do not understand
it. The output above is from the following `rules` section of [the configuration file](config.md):

```yaml
rules:
  self-repository: error
```

The rule has an unsafe fix, so `jactionlint -fix=unsafe` rewrites `./` to `$/`. It is unsafe because the two forms are not
the same when a step replaces the workspace content, which is exactly the case the rule is about.

`uses: $/path` needs no checkout step, and the [`local-action-checkout`](#check-local-action-checkout) rule does not report it.

<a id="check-github-app"></a>
## GitHub App tokens

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
<!-- Skip update output -->
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

The rule `github-app` reports uses of [`actions/create-github-app-token`][create-github-app-token] which issue a token that is
more powerful or lives longer than the job needs. An installation token is not a problem by itself, but the defaults of the
action are generous. It is in the `default` profile. The output above is from the following
`rules` section of [the configuration file](config.md):

```yaml
rules:
  github-app: error
```

It reports:

- no `permission-*` input (such as `permission-contents: read`), which gives the token every permission the app has been
  granted on the installation
- `owner:` without `repositories:`, which gives the token access to every repository of the owner where the app is installed
- `skip-token-revoke: true`, which keeps the token valid after the job instead of revoking it in the post step

Only `actions/create-github-app-token` is checked. Other actions which issue app tokens are not known to the rule.

[create-github-app-token]: https://github.com/actions/create-github-app-token

`owner` without `repositories` is not reported when every `permission-*` input is an organization permission (`permission-members`,
`permission-organization-*` and the like), because such a token has no repository to list.

<a id="check-artipacked"></a>
## Persisted checkout credentials

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

Output:
<!-- Skip update output -->
```
test.yaml:8:15: actions/checkout leaves the GITHUB_TOKEN in the git config of the workspace, where a later step such as an artifact upload can publish it. set "persist-credentials: false" under "with:" unless a later step needs to push [artipacked]
  |
8 |       - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The rule `artipacked` reports `actions/checkout` steps which do not set `persist-credentials`. The checkout action stores the
`GITHUB_TOKEN` in the git config of the workspace so that later `git` commands can push. Any later step can read it, and a step
that copies the workspace, such as `actions/upload-artifact` with `path: .`, publishes it in the artifact (the
[ArtiPACKED](https://unit42.paloaltonetworks.com/github-repo-artifacts-leak-tokens/) attack). It is in the `default` profile. The output above is from the following `rules` section of [the configuration file](config.md):

```yaml
rules:
  artipacked: error
```

Set `persist-credentials: false` unless a later step needs to push. An explicit `persist-credentials: true` says that the
credential is needed, so it is not reported, and neither is a value given by an expression.

The rule has a fix: `jactionlint -fix` adds `persist-credentials: false` under `with:` of the step, and creates `with:` when
the step has none. The fix is applied by `-fix` only when no later step of the job looks like it needs the credential. When a
later `run:` script has a `git` command that talks to a remote (`push`, `pull`, `fetch`, `clone`, `remote`, `submodule`, `lfs`,
`ls-remote`, `commit` or `tag`) or a later step uses an action known to push (such as `stefanzweifel/git-auto-commit-action` or
`peter-evans/create-pull-request`), the fix is unsafe and needs `-fix=unsafe`. A script which pushes without a visible git
command cannot be detected, so check the workflow after applying the fix. A step written in flow style (`- {uses: ...}`) is
reported without a fix.

A checkout is not reported when a later step of the job pushes with the credential it left (a `git push` in a script or an action
such as `stefanzweifel/git-auto-commit-action`) and no step uploads the workspace (`path: .`, `..` or `${{ github.workspace }}`),
because the credential is what the push needs and nothing publishes it. `actions/checkout@v1` is not reported either: it has no
`persist-credentials` input to set.

<a id="check-cache-poisoning"></a>
## Cache poisoning

Example input:

```yaml
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

Output:
<!-- Skip update output -->
```
test.yaml:13:15: swatinem/rust-cache restores a cache although the workflow runs on pushed tags, so a poisoned cache entry can end up in the published artifacts. remove this step or set "lookup-only: true", or set "cache-mode: none" on the job [cache-poisoning]
   |
13 |       - uses: Swatinem/rust-cache@6323deb102c322ba6fcbdcafc7e3dddab59af2b6 # v2
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The rule `cache-poisoning` reports caches which an attacker can use to get code into a release. Whoever can write a GitHub Actions
cache entry can make a later workflow restore it, so a job that publishes artifacts should not read caches at all. It is in the `default` profile. The output above is from the following `rules` section of [the
configuration file](config.md):

```yaml
rules:
  cache-poisoning: error
```

A job is a release job when the workflow runs on the `release` event or on pushed tags (`push` with `tags:`), or when the job
uses a publishing action (such as `pypa/gh-action-pypi-publish`, `softprops/action-gh-release` or `goreleaser/goreleaser-action`)
or runs a publishing command (`cargo publish`, `npm publish`, `twine upload`, `gh release create`, `docker push`, ...). In a
release job the rule reports the steps which restore a cache:

| Action | Restores a cache unless |
| --- | --- |
| `actions/cache`, `actions/cache/restore` | `lookup-only: true` |
| `actions/setup-node`, `setup-python`, `setup-java`, `setup-dotnet` | the `cache` input is missing or `false` (`package-manager-cache: false` only turns off the automatic cache of `setup-node`, not an explicit `cache`) |
| `actions/setup-node` v5 and later | `package-manager-cache: false`, or `package.json` of the repository does not name a package manager. v5 caches on its own when `package.json` has `packageManager` (or `devEngines.packageManager`), v6 only when it names `npm`. Without a repository to read (a workflow linted on its own, a composite action) the answer is "caches" |
| `docker/setup-buildx-action` v3 and later | `cache-binary: false` (it caches the buildx binary) |
| `actions/setup-go` | `cache: false` (before `v4` the cache is opt-in) |
| `ruby/setup-ruby` | `bundler-cache` is missing or `false` |
| `astral-sh/setup-uv` | `enable-cache: false` |
| `Swatinem/rust-cache` | `lookup-only: true` |
| `jdx/mise-action` | `cache: false` |
| `gradle/actions/setup-gradle` | `cache-disabled: true` |
| `docker/build-push-action` | there is no `cache-from` with `type=gha` |
| `hendrikmuhs/ccache-action`, `DeterminateSystems/magic-nix-cache-action` | never: remove the step |

A `tags:` filter that lets no tag through is no tag trigger: `tags: ['!**']`, a list of negative patterns only (GitHub requires one
positive pattern) and a list that ends with `!**`. A workflow that runs on pushed tags only to start checks is not a release
either: when the workflow or the job sets `permissions:` and grants nothing but read access (`read-all`, `{}`, `contents: read`),
and the job has no `environment:` and no publishing command or action, the tag does not make it a release job. This is a heuristic:
without `permissions:` the token is whatever the repository sets, so the job may publish, and a job can still publish with a secret
(then the command or the action of the job says so). The `release` event always counts.

The list is not exhaustive: it has the actions whose caching behavior is known. A step is not reported when its `if:` looks at
`github.event_name` or `github.ref`, or an input is an expression which does, since that is how caching is limited to
non-release runs (`enable-cache: ${{ !startsWith(github.ref, 'refs/tags/') }}`).

`cache-mode: none` on the workflow or on the job switches the cache off for the runner and suppresses these findings.

The rule also reports `cache-mode: write` and `cache-mode: write-only` in a workflow which runs on `pull_request_target`,
`workflow_run` or `issue_comment`: code of untrusted people writes the entries which privileged workflows restore later.

Differences from [zizmor](https://docs.zizmor.sh/audits/#cache-poisoning): zizmor reports a release workflow once for the trigger and once
for each step. jactionlint reports the steps (and the job-level publishing detection is jactionlint only).

`astral-sh/setup-uv` from v10 on, with `enable-cache` unset or `auto`, does not restore a cache on the events that are open to cache
poisoning, so it is not reported. The version is the tag of the `uses:`, or the version in the comment after a pinned commit
(`# v10.0.0`).

<a id="check-reusable-workflows"></a>
## Reusable workflows

[Reusable workflows][reusable-workflow-doc] is a feature to call a workflow from another workflow.

jactionlint does several checks for both workflow calls (caller) and reusable workflows (callee):

- syntax of workflow calls and reusable workflows
- type checks for inputs (respecting `type:` field of each input) in both workflow calls and reusable workflows
- type checks for `inputs`, `outputs` and `secrets` context objects in reusable workflows
- optional/required/undefined inputs and secrets at `uses:` in workflow calls
- type checks for `outputs` objects used by downstream jobs of workflow calls

These checks are described in this section.

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
test.yaml:15:18: input of workflow_call event "port" is typed as number but its default value ":1234" cannot be parsed as a float number: strconv.ParseFloat: parsing ":1234": invalid syntax [events]
   |
15 |         default: ':1234'
   |                  ^~~~~~~
test.yaml:20:15: invalid value "object" for input type of workflow_call event. it must be one of "boolean", "number", or "string" [syntax-check]
   |
20 |         type: object
   |               ^~~~~~
test.yaml:25:18: input "path" of workflow_call event has the default value "", but it is also required. if an input is marked as required, its default value will never be used [events]
   |
25 |         default: ''
   |                  ^~
test.yaml:31:24: "inputs.scheme" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
31 |       - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
   |                        ^~~~~~~~~~~~~
test.yaml:31:47: "inputs.host" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
31 |       - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
   |                                               ^~~~~~~~~~~
test.yaml:31:84: "inputs.path" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
31 |       - run: echo "${{ inputs.scheme }}://${{ inputs.host }}:${{ inputs.port }}${{ inputs.path }}"
   |                                                                                    ^~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8kctu8yAQhff/U4yiX8oqiXpZ8Qxd9KKuK4zHxSlmyDAojSLevcJ2IsuNu4NvZg6HOeTVP4Aj8Vfj6PhhtHMFALQ+JInDGSAaix1ebgA1RsNtkJa8gre+CNTA++vTpKXRyYkCKxLiFcspoIIo3PrPEVqKon7P4bfugsOtoe6v6UAsC8aeiWXR1lrd3T88rmfSPnUV8ggPCfm0oP1SanPxQYOqPRq52NNil+xpsXMFxkNqGWsFwglvuF7f3sWeqj6smlTfwMnHTXkkVclL2jgtGAdPUTBcg92UTgVoLMHq//k85r4dAoec1W43wSWqAieo7B9ynpLyr5xXPwMAtTmuwA==)

Unlike inputs of action, inputs of a workflow must specify their types. jactionlint validates input types and checks the default
values are correctly typed. For more details, see [the official document][create-reusable-workflow-doc].

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
test.yaml:6:5: when a reusable workflow is called with "uses", "runs-on" is not available. only following keys are allowed: "name", "uses", "with", "secrets", "needs", "if", "permissions", and "cache-mode" in job "job1" [syntax-check]
  |
6 |     runs-on: ubuntu-latest
  |     ^~~~~~~~
test.yaml:9:11: reusable workflow call "./.github/workflows/ci.yml@main" at "uses" is not following the format "owner/repo/path/to/workflow.yml@ref" nor "./path/to/workflow.yml" nor "$/path/to/workflow.yml". see https://docs.github.com/en/actions/learn-github-actions/reusing-workflows for more details [workflow-call]
  |
9 |     uses: ./.github/workflows/ci.yml@main
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:5: "with" is only available for a reusable workflow call with "uses" but "uses" is not found in job "job3" [syntax-check]
   |
12 |     with:
   |     ^~~~~
test.yaml:19:11: could not read reusable workflow file for "./.github/workflows/not-existing.yml": open /path/to/repo/.github/workflows/not-existing.yml: no such file or directory [workflow-call]
   |
19 |     uses: ./.github/workflows/not-existing.yml
   |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqEzjFywyAQBdBep9gLIEZJKqpcBTQrgYJYhl1CcnsPYuxxZVdQ/P/2UzKQK/vpIMdmAjjILf0FqIxsgFrCogtm0tmK10K6UfnZIrX5/4zfv8sVLjWx6lh1NUlV0QqyDO/j2Zv1vAfx1T0Y1mu4qNOGNBqfo9GC+PED2IgMOFteXwNgwcz3kupJA7h6Ao8x0uC/3g1KJAr/AktIe592GwBtJVzI)

When calling an external workflow, [only specific keys are available][reusable-workflow-call-keys] at job configuration.
For example, `secrets:` is not available when running steps in a normal job. And `runs-on:` is not available when calling
a reusable workflow since the called workflow determines which OS is used. jactionlint checks such keys are used correctly
to call a reusable workflow or to run steps in a normal job.

And the workflow syntax at `uses:` must follow one of the formats `owner/repo/path/to/workflow.yml@ref`,
`./path/to/workflow.yml`, or `$/path/to/workflow.yml` as described in
[the official document][create-reusable-workflow-doc]. jactionlint checks if the value follows the format.

jactionlint also validates the called workflow file is actually existing when it is in the same repository (starting
with `./` or `$/`). jactionlint reports an error when it does not exist.

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
test.yaml:20:23: "inputs.uri" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
20 |         run: curl ${{ inputs.uri }} -d ${{ inputs.lucky_number }}
   |                       ^~~~~~~~~~
test.yaml:20:23: property "uri" is not defined in object type {lucky_number: number; url: string} [expression]
   |
20 |         run: curl ${{ inputs.uri }} -d ${{ inputs.lucky_number }}
   |                       ^~~~~~~~~~
test.yaml:23:22: property "credentials" is not defined in object type {actions_runner_debug: string; actions_step_debug: string; credential: string; github_token: string} [expression]
   |
23 |           TOKEN: ${{ secrets.credentials }}
   |                      ^~~~~~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8UL1q80AQ7PUUU3ygSuIjuLo+VUIC+amNdNqEi897Yu82Rph792CdIpkU7pbZ+dnZwKYCTkEOHz6c9rbz/gIAjkdNscyAiv8dgYGiFTcmF9ignoIK3l8e63WfppEMYhLHnwvo1R6mPeuxJ7lpNBNRiH8dCzqDkazQdp4VGoiT625fudHqqvoK/axPFFNRiXJsLmztlZM2d7v2/67EJRrXsAbcHcnglXjA0KVujRRlA6vi8e98Xj7YqjjkjGa4Bq//gZxXB+LvrQHw9vxw/2Rm4dK43TpE5PwzAKvehKY=)

Inputs of reusable workflow calls are set to `inputs.*` properties following the definitions at `on.workflow_call.inputs`.
And in a job of a reusable workflow, `secrets.*` are passed from caller of the workflow so it is set following the definitions at
`on.workflow_call.secrets`. See [the official document][create-reusable-workflow-doc] for more details.

jactionlint contextually defines types of `inputs` and `secrets` contexts looking at `workflow_call` event. Keys of `inputs` only
allow keys at `on.workflow_call.inputs` and their values are typed based on `on.workflow_call.inputs.<input_name>.type`. Type of
`secrets` is also strictly typed following `on.workflow_call.secrets`.

[From May 3, 2022][inherit-secrets-announce], GitHub Actions allows inheriting secrets by calling reusable workflows. The caller
declares to inherit all secrets.

```yaml
jobs:
  pass-secrets-to-workflow:
    uses: ./.github/workflows/called-workflow.yml
    secrets: inherit
```

This means that jactionlint cannot know whether the workflow inherits secrets or not when checking a reusable workflow.
To solve this issue, jactionlint assumes that

- when `secrets:` is omitted in a reusable workflow, the workflow inherits secrets from a caller
- when `secrets:` exists in a reusable workflow, the workflow inherits no other secret

Following the assumptions,

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

this workflow causes no error. And

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

this workflow causes 'no such secret' error at `secrets.FOO`.

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
test.yaml:7:20: property "imagetag" is not defined in object type {image_tag: string} [expression]
  |
7 |         value: ${{ jobs.gen-image-version.outputs.imagetag }}
  |                    ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:7: output "image_tag" of job "gen-image-version" is never used: no other job reads "needs.gen-image-version.outputs.image_tag" and no output of the workflow uses it. remove it [unused-job-output]
   |
12 |       image_tag: "${{ steps.get_tag.outputs.tag }}"
   |       ^~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp0j8FuwyAQRO/5ipHVK/TOuf9hEWdLaShY7JIcovx7tcZGqqocGT3mzZbsTsC91OtnKvd58SlpAJQmaxPuDyD++EDmRpVjyUcIXIiXGlfRENNHWa5UO4udnQZ786mRw9vjge9yZhsomz+1dnfaLRUf8HyeFFXfP7qPqC2zUXk7tyzNJC/E8vKCWXxwmHQDC606QjQb6m7tozfi+G5U5WDfOzmPOstf48R4cdgbfwcA2ORuYw==)

Outputs of a reusable workflow can be defined at `on.workflow_call.outputs` as described in [the document][reusable-workflow-outputs].
The `jobs` context is available to define an output value to refer the outputs of jobs in the workflow. jactionlint checks
the context is used correctly.

### Check inputs and secrets in workflow call

Example reusable workflow:

```yaml
# .github/workflows/reusable.yaml
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
      - run: echo '${{ outputs.required_input }}'
```

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:6:11: input "name" is required by "./.github/workflows/reusable.yaml" reusable workflow [workflow-call]
  |
6 |     uses: ./.github/workflows/reusable.yaml
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:6:11: secret "password" is required by "./.github/workflows/reusable.yaml" reusable workflow [workflow-call]
  |
6 |     uses: ./.github/workflows/reusable.yaml
  |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:7: input "user" is not defined in "./.github/workflows/reusable.yaml" reusable workflow. defined inputs are "id", "message", "name" [workflow-call]
  |
9 |       user: rhysd
  |       ^~~~~
test.yaml:13:7: secret "credentials" is not defined in "./.github/workflows/reusable.yaml" reusable workflow. defined secret is "password" [workflow-call]
   |
13 |       credentials: my-token
   |       ^~~~~~~~~~~~
test.yaml:22:11: input "id" is typed as number by reusable workflow "./.github/workflows/reusable.yaml". bool value cannot be assigned [expression]
   |
22 |       id: true
   |           ^~~~
test.yaml:24:16: input "message" is typed as string by reusable workflow "./.github/workflows/reusable.yaml". null value cannot be assigned [expression]
   |
24 |       message: null
   |                ^~~~
```

<!-- Skip playground link -->

Reusable workflows can define required/optional inputs and secrets. When they are missing or some undefined input is used in a
workflow call, jactionlint reports an error.

And reusable workflows must define types of their inputs by `type:` field. Workflow calls pass constants (`input: 42`) or
expressions (`inputs: ${{ ... }}`) to the inputs or secrets. jactionlint checks types of values passed to inputs in workflow call.
When a type of input doesn't match to its definition, jactionlint reports an error.

Note that this check only works with a reusable workflow in the same repository (it starts with `./` or `$/`).

### Check outputs of workflow call in downstream jobs

Example reusable workflow:

```yaml
# .github/workflows/get-build-info.yaml
on:
  workflow_call:
    outputs:
      version:
        value: ${{ outputs.version }}
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

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:13:24: property "tag" is not defined in object type {version: string} [expression]
   |
13 |       - run: echo '${{ needs.get_build_info.outputs.tag }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

Outputs of workflow call are set to the job's outputs object. They can be accessed by downstream jobs specified with `needs:`.
What outputs are set is defined in the reusable workflow. jactionlint types outputs objects from workflow calls and check the
object types in downstream jobs.

In the above example, `get-build-info.yaml` has one output `version`. jactionlint types the outputs object of workflow call job
as `{version: string}`. In the downstream job, jactionlint can report an error at undefined key `tag` in the object.

Note that this check only works with a reusable workflow in the same repository (starting with `./` or `$/`).

### Check caller/callee permissions in workflow call

Example reusable workflow:

```yaml
# .github/workflows/reusable.yaml
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

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:10:11: nested job "snapshot" of "./.github/workflows/reusable.yaml" requires "pull-requests: write" but the calling job grants "pull-requests: none" [workflow-call]
   |
10 |     uses: ./.github/workflows/reusable.yaml
   |           ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

GitHub validates `permissions:` at workflow load time. Every scope a job in the called workflow declares must also be
granted by the calling job, otherwise the run fails with `startup_failure` and no jobs run, so any `if: failure()`
notification job cannot fire. jactionlint compares each called job's effective `permissions:` (its own or, when absent,
the workflow-level block) against the caller's effective grant (the calling job's `permissions:` or, when absent, the
workflow-level block) and reports each missing scope.

The check ignores `if:` on called jobs because GitHub evaluates permissions before any condition runs.

When the caller has no `permissions:` block at the workflow level and none on the calling job, the token gets the default of the
repository (Settings, Actions, General, "Workflow permissions"), which jactionlint cannot read from a workflow file. It does not
guess: only a scope that no default token has is reported (`id-token`, which always needs an explicit opt-in, and any other scope
that is missing even from a read-write token), and the message says so. Set
[`assume-default-permissions`](./config.md) to `restricted` when the repository uses the restricted token (only `contents: read`
and `packages: read` are granted), and every other scope the called workflow needs is reported. `permissive` assumes read and
write on every scope but `id-token`, which is also what the check does when the option is not set.

When the caller workflow is itself a reusable workflow (`on.workflow_call`) without any `permissions:` block, the check is
skipped: such a workflow inherits the token permissions of its own caller, which jactionlint cannot see.

Note that this check only works with local reusable workflows (starting with `./` or `$/`).

<a id="id-naming-convention"></a>
## ID naming convention

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
test.yaml:5:3: invalid job ID "foo-v1.2.3". job ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [id]
  |
5 |   foo-v1.2.3:
  |   ^~~~~~~~~~~
test.yaml:10:13: invalid step ID "echo for test". step ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [id]
   |
10 |         id: echo for test
   |             ^~~~
test.yaml:12:3: invalid job ID "-hello-world-". job ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [id]
   |
12 |   -hello-world-:
   |   ^~~~~~~~~~~~~~
test.yaml:17:3: invalid job ID "2d-game". job ID must start with a letter or _ and contain only alphanumeric characters, -, or _ [id]
   |
17 |   2d-game:
   |   ^~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqkzTEOgzAMheGdU7yNyUilG3OXHgOKaUBpHooTuH6VNjdgtPzp/QwD9myuaTZONjTAQspx6/ruXi4g5mBSWJ5ySFn8mNTS72VJd/srQIocoC9HtBsnPB841+RwaLSVoa0OWOfKFkbUMXHqPeVk9LNcCJO7lVI/y3v86NWl7wCDIlSH)

IDs must start with a letter or `_` and contain only alphanumeric characters, `-` or `_`. jactionlint checks the naming
convention, and reports invalid IDs as errors.

<a id="ctx-spfunc-availability"></a>
## Availability of contexts and special functions

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
test.yaml:6:16: context "env" is not allowed here. no context is available here. see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [expression]
  |
6 |     shell: ${{ env.SHELL }}
  |                ^~~~~~~~~
test.yaml:16:17: context "runner" is not allowed here. available contexts are "github", "inputs", "needs", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [expression]
   |
16 |           - ${{ runner.temp }}
   |                 ^~~~~~~~~~~
test.yaml:24:16: context "env" is not allowed here. available contexts are "github", "inputs", "matrix", "needs", "secrets", "strategy", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [expression]
   |
24 |       FOO: ${{ env.BAR }}
   |                ^~~~~~~
test.yaml:30:20: context "env" is not allowed here. no context is available here. see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [expression]
   |
30 |         shell: ${{ env.SHELL}}
   |                    ^~~~~~~~~~~
test.yaml:32:33: calling function "success" is not allowed here. "success" is only available in "jobs.<job_id>.if", "jobs.<job_id>.steps.if". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [expression]
   |
32 |         run: echo 'Success? ${{ success() }}'
   |                                 ^~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8j01q60AQhPdzilo88MtCOoA2IYGELAyG+ASS3LKUjGdE/8QxRncPI1uWCDiroaa+7qqOoUBv0jq3o6Y0r1I4gC2kB5CWvC/w73wGha98+/ayXmMYnPuI1UgqiV5R5VJpf7oo4FAqd9+TAnYdU62RT/MXkI2r9522VuXHyJ+Nj8cU8BthC4E4Vzr0k80WJEv9rbKglvkylRmt5S1XdE69dxOA9DGBr5vNDD0/vU+IKPW3xdly4q+pe8ELO5UE1W3Eamt1TSKPIysX8f8Bw7C60V1TzM7PAHL9fcg=)

Some contexts are only available in some places. For example, `env` context is not available at `jobs.<job_id>.env`, but it is
available at `jobs.<job_id>.steps.env`.

Similarly, some status functions are special since they limit where they can be called. For example, `success()`, `failure()`,
`always()`, and `cancelled()` are only available at `if:` section. At the time of writing this document, the following functions
are special.

- `hashFiles()`
- `always()`
- `success()`
- `failure()`
- `cancelled()`

[The official contexts document][availability-doc] describes which contexts and special functions are available at which workflow
keys.

jactionlint checks if these contexts and special functions are used correctly. It reports an error when it finds that some context
or special function is not available in your workflow.

<a id="check-deprecated-workflow-commands"></a>
## Check deprecated workflow commands

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

GitHub deprecated the following workflow commands.

- [`set-output`][deprecate-set-output-save-state]
- [`save-state`][deprecate-set-output-save-state]
- [`set-env`][deprecate-set-env-add-path]
- [`add-path`][deprecate-set-env-add-path]

jactionlint detects these commands are used in `run:` and reports them as errors suggesting alternatives. See
[the official document][workflow-commands-doc] for the comprehensive list of workflow commands to know the usage.

<a id="if-cond-constant"></a>
## Constant conditions at `if:`

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
        # ERROR: It is always evaluated to true
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
test.yaml:9:13: constant expression "false" in condition. remove the if: section [if-cond]
  |
9 |         if: false
  |             ^~~~~
test.yaml:19:13: if: condition "${{ github.event_name == 'push' }}\n" is always evaluated to true because extra characters are around ${{ }} [if-cond]
   |
19 |         if: |
   |             ^
test.yaml:23:13: if: condition "${{ github.event_name == 'push' }} " is always evaluated to true because extra characters are around ${{ }} [if-cond]
   |
23 |         if: "${{ github.event_name == 'push' }} "
   |             ^~~~
test.yaml:29:13: if: condition "${{ github.event_name == 'push' }} && ${{ github.ref_name == 'main' }}" is always evaluated to true because extra characters are around ${{ }} [if-cond]
   |
29 |         if: ${{ github.event_name == 'push' }} && ${{ github.ref_name == 'main' }}
   |             ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNq0zz1OxDAQBeA+p3hYyK7CASxtw484ATVyYEKM1vZqZ0yz+O7Iy18iohBAVFH03nwzTtFil3lomsfUsW0AIZb6BfY5clsLuctRcrt1NTtGLLTj1xbQ1qYF3Q0J6vLq/Ob6BKeHAx68DLk7oyeKchtdIJSi3mYA31v0bss0o5iLFIIXeD4eR/dmMjaPbzYwtW1Qys/N548/LNl/g//jcPU9CrWGhSQE5+OUX5K1fo/31H+GY+QXG1e8R+tx6+tylPIyANrl1qA=)

The conditions with characters around `${{ }}` (a trailing newline of a block scalar included) are reported by the rule
`if-always-true`. It is the audit `unsound-condition` of zizmor.

jactionlint reports constant conditions at `if:` like `if: true` as error because they are usually leftover debug code like
`#if 0` in C. `if: true` should be removed because it doesn't affect the workflow behavior. `if: false` should be replaced with
commenting out because it is more obvious (or simply remove the step or job if not needed).

In addition, evaluation of `${{ }}` at `if:` condition is tricky. When the expression in `${{ }}` is evaluated to boolean value
and there is no extra characters around the `${{ }}`, the condition is evaluated to the boolean value. Otherwise the condition is
treated as string hence it is **always** evaluated to `true`.

It means that multi-line string must not be used at `if:` condition (`if: |`) because the condition is always evaluated to true.
Multi-line string inserts newline character at end of each line.

```yaml
if: |
  ${{ false }}
```

is equivalent to

```yaml
if: "${{ false }}\n"
```

Unlike using `${{ }}`, putting an expression directly ignores white spaces around it. It's the reason why the following `if:`
condition works as intended.

```yaml
if: |
  false
```

jactionlint also checks extra characters around `${{ }}` in `if:` which unexpectedly make the conditions true.

<a id="action-metadata-syntax"></a>
## Action metadata syntax validation

Example action metadata:

```yaml
# .github/actions/my-invalid-action/action.yml

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
  # ERROR: 'env' configuration is only allowed for Docker actions
  env:
    SOME_VAR: SOME_VALUE
```

Example input:

```yaml
on: push

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # jactionlint checks an action when it is actually used in a workflow
      - uses: ./.github/actions/my-invalid-action
```

Output:
<!-- Skip update output -->

```
test.yaml:8:15: description is required in metadata of "My action" action at "/Users/jdx/src/github.com/jdx/jactionlint/.github/actions/my-invalid-action/action.yml" [action]
  |
8 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:15: incorrect icon name "dog" at branding.icon in metadata of "My action" action at "/Users/jdx/src/github.com/jdx/jactionlint/.github/actions/my-invalid-action/action.yml". see the official document to know the exhaustive list of supported icons: https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#brandingicon [action]
  |
8 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:15: incorrect color "gray-white" at branding.icon in metadata of "My action" action at "/Users/jdx/src/github.com/jdx/jactionlint/.github/actions/my-invalid-action/action.yml". see the official document to know the exhaustive list of supported colors: https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#brandingcolor [action]
  |
8 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:15: invalid runner name "node16" at runs.using in "My action" action defined at "/Users/jdx/src/github.com/jdx/jactionlint/.github/actions/my-invalid-action". valid runners are "composite", "docker", "node20", and "node24". see https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions#runs [action]
  |
8 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:15: file "this-file-does-not-exist.js" does not exist in "/Users/jdx/src/github.com/jdx/jactionlint/.github/actions/my-invalid-action". it is specified at "main" key in "runs" section in "My action" action [action]
  |
8 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:8:15: "env" is not allowed in "runs" section because "My action" is a JavaScript action. the action is defined at "/Users/jdx/src/github.com/jdx/jactionlint/.github/actions/my-invalid-action" [action]
  |
8 |       - uses: ./.github/actions/my-invalid-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

All actions require a metadata file `action.yml` or `action.yaml`. The syntax is defined in [the official document][action-metadata-doc].

jactionlint checks metadata files used in workflows and reports errors when they are not following the syntax.

- `name:`, `description:`, `runs:` sections are required
- Runner name at `using:` is one of `composite`, `docker`, `node20`
- Keys under `runs:` section are correct. Required/Valid keys are different depending on the type of action; Docker action or
  Composite action or JavaScript action (e.g. `image:` is required for Docker action).
- Files specified in some keys under `runs` are existing. For example, JavaScript action defines a script file path for
  entrypoint at `main:`.
- Icon name at `icon:` in `branding:` section is correct. Supported icon names are listed in
  [the official document][branding-icons-doc].
- Icon color at `color:` in `branding:` section is correct. Supported icon colors are white, yellow, blue, green, orange, red,
  purple, or gray-dark.

jactionlint checks action metadata files which are used by workflows. Currently, it is not supported to specify `action.yml`
directly via command line arguments.

Note that `steps` in Composite action's metadata is not checked at this point. It will be supported in the future.

<a id="deprecated-inputs-usage"></a>
## Deprecated inputs usage

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
test.yaml:9:11: avoid using deprecated input "fail_on_error" in action "reviewdog/action-actionlint@v1": Deprecated, use `fail_level` instead [action]
  |
9 |           fail_on_error: true
  |           ^~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8yksKwkAQhOF9TlEXGMTtrLxJmGhrWobu0I/k+jJGXBXF96tUbOnr9NbF6wQEeYwFLMXL8FxSIktvw77kQZufFVCQTl5htDMdD31d2j1YpZzTWeK2X38xcHCs9f+AZ+M+q8xkplYRlvQZAIfnLew=)

Action inputs can be deprecated by setting [`deprecationMessage`][dep-msg]. When deprecated inputs are used in a
workflow, jactionlint reports the usage as error.

jactionlint also checks local actions. In addition to the usage of deprecated inputs, it checks the input definitions in
the action metadata `action.yml` or `action.yaml`.

Example action metadata:

```yaml
# .github/actions/my-action/action.yml

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
  ...
```

Example input:

```yaml
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

Output:
<!-- Skip update output -->

```
test.yaml:6:15: input "empty-message" is deprecated but "deprecationMessage" is empty in metadata of "My action" action at "/path/to/.github/actions/my-action/action.yaml" [action]
  |
6 |       - uses: ./.github/actions/my-action
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:9:11: avoid using deprecated input "old-input" in action "My action" defined at "./.github/actions/my-action": This input is deprecated. Use new-input instead [action]
  |
9 |           old-input: some value
  |           ^~~~~~~~~~
```

<!-- Skip playground link -->

Note that the usage of deprecated inputs marked as 'required' are not reported as error because it is not possible to avoid using
them.

<a id="yaml-anchors"></a>
## YAML anchors

GitHub Actions [supports][anochor-support-announce] YAML [anchor and alias nodes][yaml-anchor-spec]. jactionlint checks them in
workflows.

jactionlint detects errors under YAML anchors. When an alias node references an erroneous anchor, jactionlint checks them as if the
alias node is replaced with the anchor node. This means that one anchor node may be checked multiple times and jactionlint may
report multiple similar errors at the same source location.

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
test.yaml:11:21: "password" section in "nginx" service should be specified via secrets. do not put password value directly [credentials]
   |
11 |           password: P@ssw0rd
   |                     ^~~~~~~~
test.yaml:11:21: "password" section in "redis" service should be specified via secrets. do not put password value directly [credentials]
   |
11 |           password: P@ssw0rd
   |                     ^~~~~~~~
test.yaml:13:11: unexpected key "email" for "credentials" section. expected one of "password", "username" [syntax-check]
   |
13 |           email: me@example.com
   |           ^~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNp8kM1KxDAUhfd9irNyIaS6zmoewTeQTHNpI703JSdxxreXMLUUBVfhOz/kcLN5bI3LMHzkK/0AVGHtL0Apn2kSPgiwOdn9B4CkYRa/q2vovcObikSxmsJKj6cTHQmgUYoFFQ/9ch1cp1NgC+Qtl+jxdiFvryWeTNGQVg+Vi9yDbquMU9bdLxIT/wx9qP8Nff49tDSj6ydq12a1uVOZVbbjD9eTHuNLzO/MKnVJNo9cvgcAl3xngQ==)

jactionlint also checks usage of anchors and aliases. In the following example jactionlint reports recursive aliases and unused
anchors as error.

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
test.yaml:18:14: anchor "credentials" is defined but not used [syntax-check]
   |
18 |         env: &credentials
   |              ^~~~~~~~~~~~
test.yaml:18:14: expecting a single ${{...}} expression or mapping value for "env" section, but found plain text node [syntax-check]
   |
18 |         env: &credentials
   |              ^~~~~~~~~~~~
test.yaml:22:14: "env" section is alias node but mapping node is expected [syntax-check]
   |
22 |         env: *recursive
   |              ^~~~~~~~~~
test.yaml:22:14: recursive alias "recursive" is found. anchor was declared at line:19, column:9 [syntax-check]
   |
22 |         env: *recursive
   |              ^~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsj8FqwzAQRO/+ijkUHwp27/qZothLomKvxI7kFEL+vYjGwiE5mRm/p92N6pAKL133E090HZCFuX4Bim1hEv4nQM9Bf/cAhNWfxT3axVev/ZtMZtEc/EKH/pAaARSKqV/F4eN2A2UyyRxri/v9wCVPXqPNz9ze7qwV5VCvKaeiuQyHhZgltSOGSjqMX3O86hL9PPLSholuDp+v6zappLdK/07pTaZiDJs0+PEK4yrfnCyk/Dq9WX8DAAHceU8=)

jactionlint checks dangling aliases as syntax error. Note that the error position is currently incorrect as the below output
indicates. This issue is due to go-yaml library and the [fix](https://github.com/yaml/go-yaml/pull/191) will be included at the
next release of the library.

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
test.yaml:9:14: could not parse as YAML: unknown anchor 'credentials' referenced [syntax-check]
  |
9 |         env: *credentials
  |              ^~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNosyjEOwjAMheE9p3gzUsqe26TEUkGRXeXZcH1k6PQP/2facAaPUl62sxXAhZ4FVihrgthDPers+X6LLif/CqgpG7b7sI9O62PjcS1A9N1weywZov7sk98BAKp1Iic=)

<a id="check-dependabot-syntax"></a>
## Dependabot configuration syntax

jactionlint checks the Dependabot configuration file `.github/dependabot.yml` (or `.github/dependabot.yaml`) of the repository
together with the workflows. It is checked when the repository is linted without file arguments, and when the file is given
explicitly (`jactionlint .github/dependabot.yml`) or with `-stdin-filename .github/dependabot.yml`. The file is recognized by its
path only. The rules for workflows are not applied to it, and `.github/workflows/dependabot.yml` is still a workflow.

The syntax is [the version 2 of the options reference][dependabot-options-doc]. Like for workflows, jactionlint reports unknown
keys, missing required keys, values of wrong types and values which are not one of the accepted ones (package ecosystems,
schedule intervals and days, update types, registry types, etc.). It also reports `registries` and `multi-ecosystem-group` names
which are not defined, and `updates` items for the same package ecosystem, directory and target branch. All of them are reported with
the rule ID `dependabot-syntax`, which can be turned off, ignored by `paths:` in [the configuration](config.md) or with
[ignore comments](usage.md) like the other rules.

Registries accept more keys than the common ones (`url`, `username`, `password`, `key`, `token`, `replaces-base`) depending on
their types, and new ones are added over time (e.g. for OIDC). jactionlint accepts any scalar value for them.

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
.github/dependabot.yml:10:7: unexpected key "dayy" for "schedule" section. expected one of "cronjob", "day", "interval", "time", "timezone" [syntax-check]
   |
10 |       dayy: monday
   |       ^~~~~
.github/dependabot.yml:12:5: unexpected key "label" for "updates" section. expected one of "allow", "assignees", "commit-message", "cooldown", "directories", "directory", "exclude-paths", "groups", "ignore", "insecure-external-code-execution", "labels", "milestone", "multi-ecosystem-group", "open-pull-requests-limit", "package-ecosystem", "patterns", "pull-request-branch-name", "rebase-strategy", "registries", "reviewers", "schedule", "target-branch", "vendor", "versioning-strategy" [syntax-check]
   |
12 |     label: [dependencies]
   |     ^~~~~~
.github/dependabot.yml:13:5: "cooldown" is not set in this update, so Dependabot applies its implicit cooldown of 3 days. set "cooldown.default-days" to at least 7 to avoid updating to a version right after its release [dependabot-cooldown]
   |
13 |   - package-ecosystem: npm
   |     ^~~~~~~~~~~~~~~~~~
.github/dependabot.yml:17:17: schedule interval "hourly" is invalid. expected one of "daily", "weekly", "monthly", "quarterly", "semiannually", "yearly", "cron" [syntax-check]
   |
17 |       interval: hourly
   |                 ^~~~~~
.github/dependabot.yml:19:5: "cooldown" is not set in this update, so Dependabot applies its implicit cooldown of 3 days. set "cooldown.default-days" to at least 7 to avoid updating to a version right after its release [dependabot-cooldown]
   |
19 |   - package-ecosystem: cargo
   |     ^~~~~~~~~~~~~~~~~~
.github/dependabot.yml:19:5: "schedule" key is missing in "updates" item [syntax-check]
   |
19 |   - package-ecosystem: cargo
   |     ^~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

<a id="check-dependabot-cooldown"></a>
## Dependabot cooldown

A cooldown makes Dependabot wait until a new version is some days old before it proposes the update. It limits the damage of
a compromised release, which is usually taken down within days, and of releases which turn out to be broken. Without
`cooldown.default-days` Dependabot applies an implicit cooldown of 3 days. This check reports an update of `dependabot.yml`
whose `cooldown.default-days` (explicit or implicit) is less than the minimum, which is 7 days. The finding is at the update
when it has no `cooldown`, at `cooldown` when it has no `default-days`, and at the value otherwise. The check is the
equivalent of the `dependabot-cooldown` audit of zizmor.

The other keys of `cooldown` (`semver-major-days`, `include` and so on) are not checked.

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
.github/dependabot.yml:4:5: "cooldown" is not set in this update, so Dependabot applies its implicit cooldown of 3 days. set "cooldown.default-days" to at least 7 to avoid updating to a version right after its release [dependabot-cooldown]
  |
4 |   - package-ecosystem: github-actions
  |     ^~~~~~~~~~~~~~~~~~
.github/dependabot.yml:14:21: "cooldown.default-days" is 2, which is less than the minimum 7 days. set it to at least 7 [dependabot-cooldown]
   |
14 |       default-days: 2
   |                     ^
```

<!-- Skip playground link -->

The rule is `dependabot-cooldown`. It is in the `default` profile. Change the minimum with the `days` option and let
`-fix` write the cooldown by setting `default-days`:

```yaml
rules:
  dependabot-cooldown:
    days: 14
    default-days: 14
```

jactionlint never makes up the number of days: without the `default-days` option the findings have no fix. With it,
`-fix` adds a `cooldown` section, adds `default-days` to a `cooldown` section without it, or raises a smaller value. The fix
is offered only when the number is at least `days`, and not when the update is written in flow style (`{...}`). To turn the
check off, set `dependabot-cooldown: off`.

<a id="check-dependabot-execution"></a>
## Dependabot insecure code execution

Some package managers run code of the dependencies (build scripts, `setup.py`, plugins) while they resolve versions.
Dependabot does not run it unless an update sets `insecure-external-code-execution: allow`. In an automated job the code is
run without a human looking at it first, and it may find the credentials Dependabot has for private registries. This check
reports every update which allows it. It is the equivalent of the `dependabot-execution` audit of zizmor.

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

The rule is `dependabot-execution`. It is enabled by default as an error. Remove the key or set it to `deny` (the default).
`-fix=unsafe` sets the value to `deny`. The fix is unsafe because updates of dependencies which need the code to run stop
working. If you need `allow` for a registry, ignore the finding for that file with `paths:` in [the configuration](config.md)
or with an [ignore comment](usage.md).

<a id="check-dependabot-missing-actions-update"></a>
## Dependabot updates of actions (pedantic)

A repository which uses Dependabot for its dependencies often forgets that the actions in its workflows are dependencies too:
they are never updated, or updated by hand when someone notices. This check reports a `dependabot.yml` which has no update with
`package-ecosystem: github-actions` while `.github/workflows` has a workflow using an action or a reusable workflow (not a
local path or a `docker://` image). It is a policy of jactionlint; zizmor has no such audit.

The check does nothing when the repository has a Renovate configuration (`renovate.json`, `.github/renovate.json`,
`.renovaterc` and so on), since another tool may update the actions. A repository which does not have `dependabot.yml` at all
is not reported: there is no file to report. There is no automatic fix because the update needs a schedule that only you can
choose.

Example input:

```yaml
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

Output:
<!-- Skip update output -->
```
.github/dependabot.yml:1:1: this repository uses actions in its workflows but no update has the "github-actions" package ecosystem, so Dependabot never updates them. add an update with "package-ecosystem: github-actions" and "directory: /" [dependabot-missing-actions-update]
  |
1 | version: 2
  | ^~~~~~~~~~
```

<!-- Skip playground link -->

The rule is `dependabot-missing-actions-update`. It is in the `pedantic` profile as a warning, or enabled explicitly with
`rules: {dependabot-missing-actions-update: warn}`.

<a id="check-pipeline-without-pipefail"></a>
## Pipelines that hide failures

The default shell of a `run:` step on Linux and macOS runs `bash -e {0}`, and `shell: sh` runs `sh -e {0}`. Neither enables
`pipefail`, so the exit status of `cmd1 | cmd2` is the one of `cmd2` alone. When `cmd1` fails, the step still succeeds
and the broken output is silently passed on. An explicit `shell: bash` runs `bash --noprofile --norc -eo pipefail {0}`, which
does not have this problem. This is the rule `pipeline-without-pipefail`. It is enabled by the default profile. To turn it
off, set it in [the configuration file](config.md):

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

jactionlint reports a pipeline when all of the following hold:

- The shell has no pipefail: the default shell on a runner which is known to be Linux or macOS (the default shell of Windows
  is `pwsh`, so jobs on Windows runners, and jobs whose runner is not known from `runs-on:`, are skipped), `shell: sh`, or a
  custom `bash`/`sh` template with errexit and without `pipefail`. The shell of the step, of `defaults.run.shell` of the job and
  of the workflow are considered.
- The script does not turn it on before the pipeline with `set -o pipefail`, `set -eo pipefail`, `set -euxo pipefail`,
  `set -o errexit -o pipefail` or `SHELLOPTS=pipefail` (and does not turn it off again with `set +o pipefail`).
- A stage in front of the last one is a command whose failure matters. `echo`, `printf`, `true`, `yes`, `cat` of a here
  document or here string, and similar commands which cannot meaningfully fail do not count. Filters such as `sed`, `awk`,
  `sort` and `tail` do not count in the middle of a pipeline either, because a failure of what feeds them is reported at that
  command. `grep`, `rg` and `diff` never count: their non-zero status means "no match" or "files differ", and `pipefail` would
  turn that expected answer into a failed step.
- The script does not handle the status of the pipeline itself: it is not negated with `!`, not the condition of `if`, `elif`,
  `while` or `until`, and not an operand of `&&` or `||` other than the last one of a list (`cmd | grep x || true`). A command
  followed by `|| true` in the first stage (`{ git show || true; cat note; } | sort`) is not blamed either.
- No later stage stops reading early. `head`, `grep -q`, `grep -m`, `read`, `sed ... q` and `awk ... exit` end the pipeline
  before the producer is done, which kills it with SIGPIPE. With `pipefail` such a pipeline fails although nothing went wrong,
  so such pipelines are not reported. That holds for a pipeline in the body of a loop as well
  (`for s in a b; do cmd "$s" | grep -Fq x; done`).
- The pipeline is not inside a command substitution in the argument of a command (`echo "hash=$(sha256sum f | cut -d' ' -f1)"`):
  the status of the substitution is not the status of anything, so `pipefail` would change nothing. The value of an assignment
  (`hash=$(sha256sum f | cut -d' ' -f1)`) is the status of the assignment, and that pipeline is reported.

Add `set -o pipefail` before the pipeline or use `shell: bash`. `shell: sh` has no `pipefail` in dash (the `sh` of Ubuntu),
so use `shell: bash` there. Be aware that turning `pipefail` on makes hidden failures fail the step, and that `grep` without a
match exits with 1, so check pipelines like `cmd | grep pattern | wc -l` when you enable it.

`-fix=unsafe` inserts `set -o pipefail` as the first line of a `run: |` script (default shell and custom bash templates only).
The fix is unsafe because failures which used to be ignored now fail the step. There is no fix for a script on a single line,
for `shell: sh` and for shells which are not bash.

### Known limits of this rule

This is the most intricate rule that is on by default, and it decides from the words of a script without running it. It is wrong in
these ways, and a finding that is one of them can be silenced with `# jactionlint ignore=pipeline-without-pipefail`, or the rule can
be turned off with `rules: {pipeline-without-pipefail: off}`:

- It cannot tell whether the first stage fails in practice. A producer that never fails (a command of your own) is reported like
  `curl`, unless it is in the list of commands that cannot fail.
- Functions, aliases and sourced files are not followed: `pipefail` set in a script that is `source`d, or a pipeline inside a shell
  function that is called with `set -o pipefail` already on, is judged by the lines in front of it.
- A shell that is chosen with an expression, a runner chosen with an expression, and a script with a `${{ }}` in the shell
  syntax that makes it unparsable are skipped: no finding is made on a guess.
- A program that exits early without being one of the known commands (`mysql -e`, a pager, your own reader) can get SIGPIPE
  under `pipefail`, so a fix may turn a pipeline that worked into a failing one. That is why the fix is unsafe.
- `grep`, `rg` and `diff` as the first stage are never blamed, although a failure of `grep` that is an error (status 2) is hidden too.

There is no `conservative` mode: it would have to guess which of these a finding is, and the guesses are the ones above.


<a id="check-anonymous-definition"></a>
## Workflow and job names (pedantic)

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: make
```

Output:
<!-- Skip update output -->
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

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  anonymous-definition: warn
```

The rule `anonymous-definition` (in the `pedantic` profile) reports workflows without a top-level `name:` and jobs without a
`name:`. GitHub shows the file path of a workflow without a name, and the ID of a job without one, in the Actions UI, in the
checks of a pull request and in notifications. Names make it clear which definition is running, and a name can be changed
without breaking the `needs:` of other jobs, which refer to the job ID. Jobs that call a reusable workflow are not reported
since GitHub shows them as `caller / called job`.

The rule is fixable. `jactionlint -fix` adds `name: <file name without the extension>` to the workflow and `name: <job ID>` to
a job, which shows the job exactly as it was shown before. Both fixes are safe. A workflow read from stdin gets the ID of its
only job; with several jobs it has no fix.

To turn the rule off for a project, set `anonymous-definition: off` in `rules`. Single findings can be ignored with
`# jactionlint ignore=anonymous-definition`.

`copilot-setup-steps.yml` is not reported, see [concurrency limits](#check-concurrency-limits).

<a id="check-concurrency-limits"></a>
## Concurrency limits

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

Output:
<!-- Skip update output -->
```
test.yaml:1:1: workflow has no "concurrency:", so every run of it executes at the same time even when a newer run supersedes the older ones. add a top-level "concurrency:" with a "group:" and "cancel-in-progress: true". use a group per pull request such as "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}", so that a new push cancels only the older runs of the same pull request and not the runs of the others [concurrency-limits]
  |
1 | on:
  | ^~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  concurrency-limits: error
```

The rule `concurrency-limits` (in the `default` profile) reports a workflow without a `concurrency:` setting, and a workflow whose
`concurrency:` is written as a plain group name (`concurrency: my-group`), which cannot cancel anything. By default GitHub runs
every instance of a workflow at the same time even when a new run supersedes the old ones. That wastes runner minutes (an
attacker can use it to burn the billed minutes of a repository) and can make workflows race when they look for artifacts by
workflow and job names.

Add a top-level `concurrency:` with a group and `cancel-in-progress: true` (or an expression which is true in most cases):

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

For a workflow that pull requests start, the group above is per pull request: a new push cancels the older runs of the same
pull request and nothing else. A group that is the same for every pull request would cancel the runs of unrelated ones, which
[`concurrency-cancels-prs`](#check-concurrency-cancels-prs) reports.

A workflow that releases or deploys is told the opposite: when it runs on the `release` event or on pushed tags, or a job has an
`environment:`, publishes a package or deploys, cancelling a run that is half done leaves a half done release
([`concurrency-cancels-release`](#check-concurrency-cancels-release) reports exactly that). The advice is then `cancel-in-progress: false`,
so that a new run waits for the running one, there is no fix for it, and the plain group name (`concurrency: release`), which
queues runs and cancels none, is accepted. The two rules never contradict each other: the block that one recommends is not reported
by the other.

Whether to cancel is a choice, so a `concurrency:` mapping that does not cancel (to serialize releases, for example) is
accepted, and so is a `queue:`. The rule skips workflows that only run through `workflow_call` (the caller decides) and workflows where every job sets its own
`concurrency:`.

`jactionlint -fix` adds the block above after `on:` when it is safe to do so, that is, only for a workflow whose triggers are all
events of a pull request (`pull_request`, `pull_request_target`, `pull_request_review` and `pull_request_review_comment`), in which
no job has an `environment:` or publishes or deploys anything, no job has a `concurrency:` of its own, and the file has no
anchors or aliases. It never adds the block to a workflow that a push, a tag, a release or a manual run starts, because cancelling
those runs is a decision about the workflow: use the group of your choice there.

The workflow `.github/workflows/copilot-setup-steps.yml` is not reported: GitHub runs it when the Copilot coding agent starts, from a
job with a fixed ID, and a concurrency group has no use there. A caller workflow whose jobs all call reusable workflows is reported
like any other, because a `concurrency:` in the called workflow can deadlock with the one of its caller, so the caller is where the
limit belongs.

<a id="check-secrets-inherit"></a>
## Inherited secrets

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

The rule `secrets-inherit` (in the `default` profile) reports a job that calls a reusable workflow with
`secrets: inherit`. The called workflow receives every secret that the caller can see, so it breaks the principle of least
privilege and makes it impossible to tell from the caller which secrets the called workflow gets.

Pass the secrets the called workflow needs one by one:

```yaml
jobs:
  deploy:
    uses: octo-org/octo-repo/.github/workflows/deploy.yaml@main
    secrets:
      deploy-token: ${{ secrets.DEPLOY_TOKEN }}
```

There is no automatic fix because jactionlint cannot know which secrets the called workflow uses. To accept the inheritance for
a trusted workflow of your own organization, ignore the finding with `# jactionlint ignore=secrets-inherit` or turn the rule
off with `secrets-inherit: off` in `rules`.

<a id="check-insecure-commands"></a>
## Insecure workflow commands

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

The rule `insecure-commands` (in the `default` profile) reports a workflow, job or step that sets the environment variable
`ACTIONS_ALLOW_UNSECURE_COMMANDS` to `true` (or `1`). GitHub [deprecated the workflow commands][deprecate-set-env-add-path]
`::set-env` and `::add-path` in 2020 because anything that can print a line to the log of a step can use them to inject
environment variables, and so to run code. This variable opts back in to them. The [deprecated workflow commands](#check-deprecated-workflow-commands)
check reports the commands themselves.

Write to the environment files instead (`$GITHUB_ENV`, `$GITHUB_PATH`) and remove the variable.

The rule is fixable but the fix is **unsafe**: `jactionlint -fix=unsafe` removes the variable (and the `env:` mapping when it
was the only entry). A step that still prints `::set-env` or `::add-path` stops working after that, so replace the commands first.

<a id="check-unverified-download"></a>
## Unverified downloads

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
test.yaml:7:14: the script downloaded from "https://example.com/install.sh" is run by "sh" without being verified, so whoever controls that server or the connection to it controls this job. if it installs a tool, install the tool with mise instead (jdx/mise-action pinned by SHA, or "mise use" with a committed mise.lock, which records the version and checksum of each tool). otherwise download the file, check its checksum or signature (sha256sum -c, gpg --verify, cosign verify-blob or gh attestation verify) before running it, or use the package of the vendor [unverified-download]
  |
7 |         run: curl -fsSL https://example.com/install.sh | sh
  |              ^~~~
test.yaml:9:11: the file "tool" downloaded from "https://example.com/dl/tool" is made executable without a checksum or signature check in this script, so whatever the server (or anyone on the connection) sends is trusted. if it installs a tool, install the tool with mise instead (jdx/mise-action pinned by SHA, or "mise use" with a committed mise.lock, which records the version and checksum of each tool). otherwise download the file, check its checksum or signature (sha256sum -c, gpg --verify, cosign verify-blob or gh attestation verify) before running it, or use the package of the vendor [unverified-download]
  |
9 |           curl -fsSLo tool https://example.com/dl/tool
  |           ^~~~
```

[Playground](https://jactionlint.jdx.dev/#eNpsjDEOgzAMRXdO8fcqZM8ZuvUEAVKFysQRtisGDl8BFUJVN8v/vccloJrk5sWdhAaQpFa3A5itiNsA66yoOYqaRPdJNFU5KMBBciIK6KLk72+3A3qbCe4pjzuyapXgfVriVCm1PU9+LKKRqJWMFafrDnc9U7h0GMpMf2sD+W27anniAbcFP/92B+HcO80ycvkMAGLeTWk=)

The rule `unverified-download` (in the `default` profile) reports a `run:` script that runs what it has downloaded without checking
it first. Whoever controls the server, or the connection to it, then controls the job, with its secrets and its token. It reports

- a download piped into a shell or an interpreter: `curl ... | sh`, `wget -qO- ... | sudo bash`, `bash <(curl ...)`,
  `sh -c "$(curl ...)"`, `eval "$(curl ...)"` and `curl ... | python3 -`;
- a downloaded file that the same script makes executable (`chmod +x`, `install -m 755`), runs, sources or installs with `dpkg`,
  `rpm` or `apt`, when no checksum or signature check (`sha256sum`, `shasum`, `gpg --verify`, `cosign verify-blob`,
  `gh attestation verify`, `minisign`, ...) comes before the use. A file that is moved with `mv`, `cp` or `install` is followed;
- a download made with TLS certificate verification turned off (`curl -k`, `curl --insecure`, `wget --no-check-certificate`).

A pipe into something that only reads the data (`curl ... | tar xz`, `| jq`, `| python3 -c '...'`) is not reported, and neither is a
URL for a full commit on GitHub, GitLab, Codeberg or Bitbucket (`https://raw.githubusercontent.com/<owner>/<repo>/<40 hex digits>/install.sh`),
because the URL fixes the content. A file of this repository at the commit that runs the workflow is the same
(`https://github.com/${GITHUB_REPOSITORY}/raw/${GITHUB_SHA}/tools/release.sh`, or with `${{ github.repository }}` and `${{ github.sha }}`): the
repository checked it in itself. A host that is not on the internet (`localhost`, a private address) is not reported either. A
script is analyzed when its shell is `bash` or `sh`; the default shell of Windows runners is `pwsh`, which is not analyzed.

If the download installs a tool, install the tool with [mise](https://mise.jdx.dev) instead: use `jdx/mise-action` pinned by SHA,
or `mise use` with a committed `mise.lock`, which records the version and the checksum of each tool. Otherwise download the
installer to a file, check its checksum or signature and run the file, or use the package of the vendor.

To accept a download, list the host or a URL prefix in `allow` (an entry with `://` is a prefix of the URL, anything else is a host
name), or ignore one finding with `# jactionlint ignore=unverified-download`:

```yaml
rules:
  unverified-download:
    allow: [get.example.com, "https://example.org/install/"]
```

There is no fix, because whether a download is acceptable is a decision of its author. zizmor 1.30.1 has no equivalent audit; the
idea is [zizmorcore/zizmor#711](https://github.com/zizmorcore/zizmor/issues/711). Downloads of archives that are extracted and then
run, `npx`/`go install` of a remote script, and a verification that happens in a later step are not tracked.

<a id="check-insecure-ssh-keyscan"></a>
## Host keys collected with ssh-keyscan

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

The rule `insecure-ssh-keyscan` (in the `default` profile) reports `ssh-keyscan` output that a `run:` script writes to a `known_hosts`
file: `>>` and `>` redirections, `tee`, a command substitution and a group. `ssh-keyscan` asks the server for its key and trusts
the answer (trust on first use), so anyone who can intercept the connection of the runner becomes the server for every later `ssh`, `scp` and `rsync` of the job.

Keep the verified host key in a repository variable or secret and write that to the file instead:

```yaml
- run: echo "${{ secrets.KNOWN_HOSTS }}" >> ~/.ssh/known_hosts
```

A script that shows the fingerprints with
`ssh-keygen -l`, to compare them with known ones, is not reported. The rule does not look at `ssh -o StrictHostKeyChecking=no`.

There is no fix. Turn the rule off with `insecure-ssh-keyscan: off` in `rules` or ignore one finding with
`# jactionlint ignore=insecure-ssh-keyscan`. zizmor 1.30.1 has no equivalent audit; the idea is
[zizmorcore/zizmor#2012](https://github.com/zizmorcore/zizmor/issues/2012).

<a id="check-checkout-static-credentials"></a>
## Static credentials for actions/checkout

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

The rule `checkout-static-credentials` (in the `default` profile) reports an `actions/checkout` step that is given a credential that
does not expire: the `ssh-key` input, or a `token` written in the workflow. The default `GITHUB_TOKEN` lasts as long as the job and
is limited to the repository, and the token of a GitHub App lasts an hour at most. An SSH key or a personal access token stays valid
until somebody rotates it, usually reaches more than the job needs, and `actions/checkout` writes it to the git config of the
workspace unless `persist-credentials: false` is set, where every later step can read it.

Use the default token for the repository running the workflow. To reach other repositories or to push something that has to start
other workflows, create a short-lived token with [actions/create-github-app-token](https://github.com/actions/create-github-app-token),
or use a fine-grained personal access token that is limited to the repositories it needs and kept in a secret of a protected
environment (one with required reviewers or a branch restriction).

A `token` that comes from a secret is a personal access token in most cases, and workflows that release or push often have to use one,
so it is a pedantic check: the option `secret-tokens` is on under the `pedantic` profile and off under `default` (set it to
`true` or `false` to decide). The `ssh-key` input and a literal `token` are always reported. Secrets named in `allow` (case-insensitive) are never reported. A `token`
that is `secrets.GITHUB_TOKEN`, `github.token`, the output of a step, an input of a reusable workflow, or has a fallback to one
of these, is not reported because the credential behind it is not known to be static.

```yaml
rules:
  checkout-static-credentials:
    secret-tokens: true
    allow: [DEPLOY_KEY]
```

There is no fix. Turn the rule off with `checkout-static-credentials: off` in `rules` or ignore one finding with
`# jactionlint ignore=checkout-static-credentials`. zizmor 1.30.1 has no equivalent audit; the idea is
[zizmorcore/zizmor#1118](https://github.com/zizmorcore/zizmor/issues/1118).

<a id="check-insecure-url-scheme"></a>
## Insecure URL schemes

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

The rule `insecure-url-scheme` (in the `default` profile) reports a location that is fetched over `http://`, `ftp://` or `git://`.
These schemes send the request and the answer in the clear and do not prove who the server is, so anyone on the path can read or
replace what the job downloads. The rule looks at

- the URLs given to `curl`, `wget`, `git` (`clone`, `fetch`, `pull`, `push`, `remote`, `submodule`, `ls-remote`), `pip`, `uv`, `npm`,
  `pnpm`, `yarn`, `bun`, `cargo`, `gem`, `go` and a few other download commands in `run:` scripts, including the values of options
  such as `--index-url` and `--registry`;
- inputs of actions in `with:` whose whole value is such a URL.

The message names the same URL with `https://`. There is no fix, because the host may not serve the same content over HTTPS.

URLs of hosts that are not on the internet are not reported: `localhost`, loopback, private and link-local addresses, names without
a dot (the services of a job), and `.local`, `.internal`, `.svc`, `.lan` and `.test` names. A host that is a variable or an expression
is not reported either, nor are proxies (`curl -x`), headers, request data and text printed by `echo`. An input whose name contains
`timestamp` (`timestamp-rfc3161`) is not reported: the address of an RFC 3161 timestamp authority is `http` by design, because what it
returns is signed. This rule is not the audit of
the same name in zizmor 1.30.1, which checks the `repo:` URLs of `.pre-commit-config.yaml` and is not covered here.

Turn the rule off with `insecure-url-scheme: off` in `rules` or ignore one finding with `# jactionlint ignore=insecure-url-scheme`.

<a id="check-dangerous-triggers"></a>
## Dangerous triggers

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

Output:
<!-- Skip update output -->
```
test.yaml:2:3: warning: trigger "pull_request_target" is dangerous: it runs in the context of the base repository with a write token and secrets even when the pull request comes from a fork. use "pull_request" unless write access is required, and never check out or run code of the pull request [dangerous-triggers]
  |
2 |   pull_request_target:
  |   ^~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  dangerous-triggers: warn
```

The rule `dangerous-triggers` (in the `default` profile) reports the triggers `pull_request_target`, `workflow_run` and
`issue_comment`. They run with the secrets and the write token of the repository (in the context of the default branch) while
a fork, or any commenter, controls part of the event. Making them safe is hard: it is not enough to avoid checking out the
pull request, because arguments, environment variables and files that come from the event can still lead to code execution. See
[Keeping your GitHub Actions and workflows secure: preventing pwn requests](https://securitylab.github.com/resources/github-actions-preventing-pwn-requests/).

Prefer `pull_request` unless the workflow really needs write access (to leave a comment, for example), `workflow_call` instead
of `workflow_run`, and a label added by a maintainer instead of a comment as the trigger. A workflow whose steps are only
`actions/labeler` is accepted for `pull_request_target`, since it never runs code from the pull request.

The rule only looks at the trigger, so a workflow that uses one of them safely is reported as well. Ignore it with
`# jactionlint ignore=dangerous-triggers` and a reason, as in `zizmor: ignore[dangerous-triggers]`. zizmor 1.30.1 does not report
`issue_comment` yet.

<a id="check-self-hosted-runner"></a>
## Self-hosted runners (pedantic)

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: [self-hosted, linux]
    steps:
      - run: make
```

Output:
<!-- Skip update output -->
```
test.yaml:4:15: info: job runs on a self-hosted runner (label "self-hosted"). self-hosted runners are hard to secure and should not run workflows of untrusted pull requests of a public repository [self-hosted-runner]
  |
4 |     runs-on: [self-hosted, linux]
  |               ^~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  self-hosted-runner: info
```

The rule `self-hosted-runner` (in the `pedantic` profile, at the `info` level) points out jobs that have the `self-hosted` label in
`runs-on:`, also when it comes from a literal value of the matrix. [Self-hosted runners][self-hosted-runner-security] are hard
to secure: unless they are ephemeral, a job can leave something behind for the next one, and GitHub does not recommend them for
public repositories, where any pull request can run code on them. The rule is informational because it cannot see how a runner
is set up. Runner labels of third party providers (without the `self-hosted` label) are not reported, and neither are labels
that come from an expression it cannot resolve.

If you must use self-hosted runners in a public repository, require approval for the workflows of outside contributors and use
ephemeral (just-in-time) runners.

[self-hosted-runner-security]: https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/about-self-hosted-runners#self-hosted-runner-security

<a id="check-unsound-contains"></a>
## Unsound contains() on a string

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

The rule `unsound-contains` (in the `default` profile) reports a condition in `if:` that calls `contains()` with a
string literal as the first argument and something else as the second. People write it to test that a value is one of a list
of words, but `contains()` on strings tests for a substring, so `contains('refs/heads/main refs/heads/develop', github.ref)` is
also true for a branch named `mai` or `heads/dev`. When the condition guards a deployment or a privileged step, that is a
bypass.

Give `contains()` an array, or compare with each value:

```yaml
if: contains(fromJSON('["refs/heads/main", "refs/heads/develop"]'), github.ref)
# or
if: github.ref == 'refs/heads/main' || github.ref == 'refs/heads/develop'
```

Calls where the first argument is not a literal (`contains(github.event.head_commit.message, 'skip ci')`) are not reported.
There is no automatic fix.

<a id="check-overprovisioned-secrets"></a>
## Overprovisioned secrets

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

Output:
<!-- Skip update output -->
```
test.yaml:8:31: warning: the whole "secrets" context is used, which exposes every secret to the runner even if only one is needed. reference each secret by its name, e.g. "secrets.NAME" [overprovisioned-secrets]
  |
8 |           SECRETS: ${{ toJSON(secrets) }}
  |                               ^~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  overprovisioned-secrets: warn
```

The rule `overprovisioned-secrets` (in the `default` profile) reports expressions that use the whole `secrets` context:
`toJSON(secrets)`, `secrets[<computed name>]`, `secrets.*` or a bare `secrets` passed to a function. GitHub passes to the
runner only the secrets that a job refers to by name. An expression like the above makes it pass all of them, even if the
step needs one.

Refer to each secret by its name instead:

```yaml
env:
  SECRET_ONE: ${{ secrets.SECRET_ONE }}
  SECRET_TWO: ${{ secrets.SECRET_TWO }}
```

`secrets.NAME` and `secrets['NAME']` are fine. There is no automatic fix.

<a id="check-unredacted-secrets"></a>
## Unredacted secrets

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

Output:
<!-- Skip update output -->
```
test.yaml:8:25: warning: secret "CREDENTIALS" is parsed with fromJSON(), and the runner does not redact the fields of a parsed secret from the logs. store each field in its own secret and reference it by name [unredacted-secrets]
  |
8 |           PASSWORD: ${{ fromJSON(secrets.CREDENTIALS).password }}
  |                         ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  unredacted-secrets: warn
```

The rule `unredacted-secrets` (in the `default` profile) reports a field that is extracted from a secret parsed with `fromJSON()`,
as in `fromJSON(secrets.CREDENTIALS).password`. The runner redacts the exact value of each secret from the logs, but it does
not know that a field of a JSON document is a secret too, so the extracted value can show up in the logs in clear.

Store each field in its own secret (`secrets.CREDENTIALS_PASSWORD`). Passing the whole parsed secret on (`fromJSON(secrets.X)`
without a property access) is not reported. There is no automatic fix.

<a id="check-secrets-outside-env"></a>
## Secrets outside an environment (pedantic)

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

Output:
<!-- Skip update output -->
```
test.yaml:8:24: warning: secret "API_KEY" is used by a job which has no "environment:", so it is a repository or organization secret exposed to every job. move it to an environment with protection rules and set "environment:" at the job [secrets-outside-env]
  |
8 |           API_KEY: ${{ secrets.API_KEY }}
  |                        ^~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  secrets-outside-env: warn
```

The rule `secrets-outside-env` (in the `pedantic` profile) reports a use of a secret in a job that has no `environment:`. Secrets of a
repository or an organization are exposed to every job that asks for them. The secrets of an
[environment](https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-deployments/managing-environments-for-deployment)
are only available to jobs that meet its protection rules (required reviewers, branch restrictions), which limits the damage of
a compromised workflow.

Move the secret to an environment, remove it from the repository secrets, and set `environment:` at the job. jactionlint
cannot see where a secret is stored, so it assumes that the secrets of a job with an `environment:` are environment secrets.

`GITHUB_TOKEN` is never reported because its permissions are set by the workflow. Workflows that run through `workflow_call`
are skipped: environment secrets do not reach a reusable workflow unless the caller inherits all secrets. Secrets that are fine
outside of an environment can be listed in the `allow` option (names are case-insensitive):

```yaml
rules:
  secrets-outside-env:
    level: warn
    allow: [CI_COVERAGE_TOKEN]
```

The line of a secret in a folded block scalar (`>-`) is the line where the block starts. There is no automatic fix.

<a id="check-typosquat-uses"></a>
## Typosquatting of actions

Example input:

```yaml
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: action/checkout@v4
```

Output:
<!-- Skip update output -->
```
test.yaml:6:15: warning: "action/checkout" looks like a typo of the popular action "actions/checkout", which belongs to another account. a typosquatted account can serve malicious code. check the name; to accept it as is, add "action/checkout" to the "allow" option of the "typosquat-uses" rule [typosquat-uses]
  |
6 |       - uses: action/checkout@v4
  |               ^~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  typosquat-uses: warn
```

The rule `typosquat-uses` (in the `default` profile) reports an action whose `owner/repo` is one typo away from a popular action
of another owner, like `action/checkout` instead of `actions/checkout` or `dokcer/login-action` instead of
`docker/login-action`. An attacker can register the misspelled account and serve malicious code to everyone who copied the typo.
One typo means a character added, removed, replaced or two neighboring characters swapped. The popular actions are the ones
in the data set behind [the popular action inputs check](#check-popular-action-inputs).

A near miss in the repository name under the right owner (`actions/chekout`) is not reported: nobody else can create it, and
it fails to resolve. An action that is itself in the data set is never reported.

If the name is intentional (a fork or a renamed action), add its `owner/repo` to the `allow` option:

```yaml
rules:
  typosquat-uses:
    level: warn
    allow: [acttons/setup-node]
```

The check compares names only. It does not ask GitHub whether the repository exists. There is no automatic fix because
jactionlint cannot be sure which name was meant.

<a id="check-forbidden-uses"></a>
## Forbidden actions (opt-in)

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

Output:
<!-- Skip update output -->
```
test.yaml:7:15: action "tj-actions/changed-files@v45" is not allowed because it matches no pattern in the "allow" list of the "forbidden-uses" rule ("actions/*", "github/codeql-action/*"). use an allowed one or add a pattern to the configuration [forbidden-uses]
  |
7 |       - uses: tj-actions/changed-files@v45
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from this `rules` section of the [configuration file](config.md):

```yaml
rules:
  forbidden-uses:
    allow:
      - actions/*
      - github/codeql-action/*
```

The rule `forbidden-uses` restricts which actions and reusable workflows a repository may use. It does nothing unless the `allow`
or the `deny` option is set. With `allow`, only the matching ones are accepted. With `deny`, the matching ones are rejected. It is
the inverse of [`required-actions`](config.md), which demands that some action is used.

Both options are lists of patterns. A pattern is `owner/repo[/path]`, optionally followed by `@ref`; names are
case-insensitive and refs are case-sensitive:

| Pattern | Matches |
| --- | --- |
| `*` | every action and reusable workflow |
| `actions/*` | every action of the owner, including those in sub-directories |
| `github/codeql-action/*` | the repository and everything in it, e.g. `github/codeql-action/init` |
| `actions/cache` | only the root of the repository: not `actions/cache/save` |
| `actions/cache/save` | only that sub-directory |
| `octo-org/shared/.github/workflows/ci.yaml@v1` | only that workflow at that ref |
| `octo-org/*@v1` | anything of the owner at `v1` |
| `actions/setup-*` | any other `*` matches any characters |

Local actions (`./path`, `$/path`) and Docker images (`docker://`) are never reported, and neither are values that contain an
expression. A deny pattern wins over an allow pattern. When both lists are set, a reference must match `allow` and must not match
`deny`.

```yaml
rules:
  forbidden-uses:
    deny:
      - tj-actions/changed-files
```

The rule is for policy, so its default level is `error`. There is no automatic fix.


<a id="check-template-injection-inputs"></a>
## Inputs, payloads, release names and branch names

Some values are not on the list of attacker controlled properties, but the person who chooses them is not the author of the
workflow either, and GitHub does not validate them. `template-injection` reports them in the `default` profile (not in
`correctness`, which is what actionlint reports), because a `${{ }}` of them in a script runs whatever the sender typed:

- `inputs.*` and `github.event.inputs.*` of a `workflow_dispatch` workflow (anyone with write access can type anything), of a
  reusable workflow (whatever the caller passes, which may be an attacker controlled value of the caller) and of a composite
  action. An input of the type `boolean`, `number`, `choice` or `environment` is a fixed vocabulary and is not reported;
- `github.event.client_payload.*`, which the sender of a `repository_dispatch` event chooses;
- `github.event.release.tag_name`, `name`, `body` and `target_commitish`, and the names of the release assets;
- `github.ref_name`, `github.base_ref` and `github.event.pull_request.base.ref`: a branch or tag name can hold shell syntax such
  as `a$(cmd)`. Creating a branch takes write access, which is why zizmor reports them as well.

`github.actor`, SHAs, numbers and the IDs of events are not reported: GitHub restricts their characters. `steps.*.outputs.*`,
`needs.*.outputs.*`, `matrix.*` and `env.*` are values of the workflow itself, which the pedantic option of the
[expansions](#check-template-injection-expansion) covers. The fix is the same as for the properties above.

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
test.yaml:15:24: "inputs.title" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
15 |       - run: echo "${{ inputs.title }}"
   |                        ^~~~~~~~~~~~
test.yaml:17:24: "github.event.client_payload.branch" is the payload of a repository_dispatch event, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
17 |       - run: echo "${{ github.event.client_payload.branch }}"
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:19:24: "github.ref_name" is a branch or tag name, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
19 |       - run: echo "${{ github.ref_name }}"
   |                        ^~~~~~~~~~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNqMkLFu8zAMhHc/xSH4V/sBtP9DgY7ZDclmYrUqKVBUAiPIuxd2nLYomqKbxDviPp6wa4Cz6OshybkfY8nehmkZApFztXJ7AxYt0f0D2JzJoZhGPm7DUedWK3/3BJFEnhtAKUuJJjp/SWpeJKwhRsVuu1q5tMIONVS22ia/aKtUjPIHUrs4HWiYBLt/l8tG3K2ouF53j3zHaFMNHZ2IrRtSJLY++zmJH7ugnofpD9tKh579269BG9DWzEPn/mn//P8uAcSnzxKBVXX44cD3AQAr74WJ)

<a id="check-template-injection-expansion"></a>
## Expansions in scripts (pedantic)

The rule `template-injection` reports the contexts that an attacker controls. Every other `${{ }}` in a `run:` script, in
the `script` of github-script and in the inputs listed above is also a risk: the value is pasted into the source of the script
before it runs, so a value with quotes, `$(...)` or a newline changes the script. The option `pedantic` of the rule reports them
too, in two kinds:

- Values that are free text: `steps.*.outputs.*`, `needs.*.outputs.*`, `matrix.*` with values from `fromJSON` and `env.*` set
  from an expression. (Free text inputs and `github.ref_name` are reported by the default profile, see
  [above](#check-template-injection-inputs).)
- Values that an attacker cannot control: `github.repository`, `github.sha`, `runner.*`, `secrets.*`, `vars.*`, boolean, number
  and choice inputs, a matrix whose values are written in the workflow, enumerations like `needs.*.result`, and expressions
  which only test a context. This is the "everything is a code smell" view of the pedantic persona of zizmor: a value that cannot
  be attacked still breaks when somebody later changes it to something that can.

The option is on under the `pedantic` profile and off otherwise, and you can set it for the rule alone. Both kinds fix as the
previous section describes. They are the findings that the retired rules `template-injection-expansion` and
`template-injection-trusted` reported: an ignore of one of those IDs still works for its kind and prints a deprecation warning.

Example input:

```yaml
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

Output:

The output is with the option `pedantic` of `template-injection` on.

<!-- Skip update output -->
```
test.yaml:10:32: "steps.version.outputs.tag" is expanded with ${{ }} into an inline script, so a value with shell syntax changes what the script does. instead, pass it through an environment variable and read it as a variable of the shell [expression]
   |
10 |       - run: ./release.sh '${{ steps.version.outputs.tag }}'
   |                                ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:16:33: "github.repository" is expanded with ${{ }} into an inline script. its value is not controlled by an attacker, but an expansion in a script is easy to get wrong when the script changes. instead, read it as a variable of the shell [expression]
   |
16 |       - run: echo "Building ${{ github.repository }}"
   |                                 ^~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

To enable them, use the `pedantic` profile or set the option in [the configuration file](config.md):

```yaml
rules:
  template-injection:
    pedantic: true
```

What is considered free text is a decision about the value, not the syntax. A boolean input, a matrix of literals and the `result` of
a job cannot hold anything but a few words, and a `string` input can hold anything. When you know that a value is safe, set the
option to `false` or ignore the line, for example with `# jactionlint ignore=template-injection`.

<a id="check-agentic-actions"></a>
## AI agent actions

An AI agent action such as Claude Code Action, Gemini CLI or Codex reads text and then acts with the tools it was given, using the
token and the secrets of the job. When an outsider wrote the text (an issue, a comment, the title of a pull request, the files of a
pull request), the outsider writes the instructions of the agent. This is the AI form of [script injection](#untrusted-inputs), and
it is worse in one respect: there is no escaping that makes text safe to read for an agent, so the remedy is to limit what a
hijacked agent can do. Public attacks include "PromptPwnd" (an issue body that made an agent edit an issue with the token) and
"Trusting Claude with a Knife" (a pull request that changed what the agent runs). zizmor has no audit for this yet
([zizmor#1605](https://github.com/zizmorcore/zizmor/issues/1605)).

The rule `agentic-actions` is enabled by the default profile and reports at error level. It looks at the steps that run an agent
action it knows (the list is below) in a workflow that outsiders can reach, that is, one with an `issues`, `issue_comment`,
`pull_request_target`, `pull_request_review`, `pull_request_review_comment`, `discussion` or `discussion_comment` trigger.
`pull_request` is not on the list: a workflow that a pull request from a fork starts has a read-only token and no secrets.
`workflow_run` is not either, because it is as safe as the workflow it follows, which the rule cannot see; it is checked only for an
open gate and for a checkout of the code that the upstream run built.
The rule reports:

- **An open gate.** Claude Code Action, Codex and Droid run only for users with write access, but an input switches that off:
  `allowed_non_write_users: '*'`, `allowed_bots: '*'` or `allow-users: '*'`. A list of named users is accepted.
- **No check of the user.** The agent actions that do not check who started them (Gemini CLI, AI inference, OpenHands, Oz, PR-Agent
  and others) run for everybody who can cause the trigger. The rule accepts a job that is restricted by an `if:` on
  `author_association`, the actor, the sender or a label (also in a job it `needs`), by an `environment:` (reviewers can be required), or
  by an earlier step that checks the permission of the actor. This is a heuristic: it cannot tell a correct condition from a
  condition that mentions the actor.
- **Settings that turn the safeguards off.** `--dangerously-skip-permissions`, `--permission-mode bypassPermissions` and allowed
  tools that give a shell (`Bash`, `Bash(*)`, `Bash(python:*)`, `Bash(curl:*)`) or any URL (`WebFetch`) in `claude_args` and `settings`
  of Claude Code; `safety-strategy: unsafe` (except on Windows, where Codex needs it), `sandbox: danger-full-access` and the
  matching `codex-args` of Codex; `tools.allowed` with `run_shell_command` of the Gemini CLI; `shell(bash:*)` in `copilot-allow-tools`
  of AI inference; `--skip-permissions-unsafe` of Droid. Exact commands like `Bash(git diff:*)` are not reported. With the option
  `any-trigger` these are reported in every workflow.
- **Code of a pull request in the workspace.** On `pull_request_target`, `issue_comment`, `workflow_run` and the review triggers, an
  agent that runs after a checkout of the pull request reads `CLAUDE.md`, `AGENTS.md`, `.claude/settings.json`, `.mcp.json`
  and the like from it, so the pull request configures the agent (hooks and servers run commands with the secrets of the job). Check
  out the base branch in the workspace and the pull request in a subdirectory with `path:`, as
  [the documentation of Claude Code Action](https://github.com/anthropics/claude-code-action/blob/main/docs/security.md) says.
- **Untrusted data read from the environment.** A prompt that tells the agent to read `$TITLE` when `TITLE` is set from
  `github.event.issue.title`. (`${{ env.TITLE }}` in a prompt is reported by `template-injection`.)
- **A secret in the environment of the agent**, when outsiders can reach the agent by one of the reports above: the shell of the agent
  can print its environment. `GITHUB_TOKEN` is not reported.

The first two reports are skipped when a steered agent can do little: for a job whose `permissions:` are set explicitly and grant no
write access except to `issues`, `pull-requests` and `discussions` (the setup the actions document for triage and labeling), and for an
agent that is limited to a list of exact tools (`--allowedTools "Bash(gh issue edit:*)"` of Claude Code, `tools.core` of the Gemini CLI) or
to a read-only sandbox (Codex). These are the shape of the architecture the vendors advise: an agent with a read-only token and a
sandbox, and a second job that acts on its validated output. The agent still holds its API key, so keep every other secret away from it.
The message names the scopes that the token can write when the workflow or the job sets `permissions:`.

The known actions, their inputs and where each was checked (an action that is not in the table is not checked; the list changes quickly):

| Action | Prompt inputs | Arguments and settings | Checks the user itself |
| --- | --- | --- | --- |
| `anthropics/claude-code-action` | `prompt` (v0: `direct_prompt`, `override_prompt`, `custom_instructions`) | `claude_args`, `settings` (v0: `allowed_tools`, `disallowed_tools`, `mcp_config`, `claude_env`) | yes |
| `anthropics/claude-code-base-action` | `prompt` | `claude_args`, `settings` | no |
| `anthropics/claude-code-security-review` | | | no |
| `google-github-actions/run-gemini-cli` | `prompt` | `settings`, `extensions` | no |
| `google-gemini/gemini-cli-action` | `prompt` | `settings_json` | no |
| `openai/codex-action` | `prompt` | `codex-args` | yes |
| `actions/ai-inference` | `prompt`, `system-prompt` | `copilot-allow-tools` | no |
| `factory-ai/droid-action` | | `droid_args`, `settings` | yes |
| `sst/opencode/github`, `anomalyco/opencode/github` | `prompt` | | yes |
| `openhands/openhands-github-action` | `prompt` | | no |
| `warpdotdev/oz-agent-action` | `prompt` | `mcp` | no |
| `qodo-ai/pr-agent` | `artifact_instructions` | | no |

What the rule cannot know, so a clean result is not a proof of safety:

- What the prompt tells the agent to read. A prompt that says "triage the newest issue" and gives the agent `gh issue view` exposes it to
  the text of the issue without any `${{ }}`. Limit the tools and the token.
- Files of the repository that configure the agent (`.claude/settings.json`, `.gemini/settings.json`, `.mcp.json`). Only the inputs
  of the workflow are read, and an input that is an expression is skipped.
- A gate in another workflow. A workflow with only `workflow_call` has no trigger of its own, so it is not reported.
- Whether the agent action of a version really has an input: the rule reads names, not versions.

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

To restrict an agent, allow the exact commands it needs (`--allowedTools "Bash(gh issue view:*)"`), give the job `permissions:` with the
least access (`permissions: {}` plus the one scope), keep the key of the agent the only secret in the job, and gate the job with an
`if:` on `github.event.comment.author_association`. To turn the rule off for a step, ignore the line (`# jactionlint ignore=agentic-actions`)
or set `rules: {agentic-actions: off}`; `rules: {agentic-actions: {any-trigger: true}}` also reports the unsafe settings in workflows that
outsiders cannot trigger.

<a id="check-bot-conditions"></a>
## Bots trusted by `github.actor`

Workflows trust bots like Dependabot with a condition such as `github.actor == 'dependabot[bot]'`. But `github.actor` is the
account of the last event, not the author of the pull request. An attacker can build a pull request whose last commit event is
made by the bot while the rest of the branch is theirs, and the condition passes. This is the rule `bot-conditions`. It is in
the `default` profile. It reports `github.actor`, `github.triggering_actor`, `github.actor_id` and `github.event.sender.*`
compared with a bot account (a name with `[bot]`, or the ID of Dependabot, Renovate and github-actions) with `==`, `contains()`,
`startsWith()` and `endsWith()`. A negative check like `github.actor != 'dependabot[bot]'` is not reported since it only skips what
the condition guards.

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

<!-- Skip update output -->
```
test.yaml:7:9: warning: "github.actor" holds the account of the last event and not the author of the change, so it can be spoofed and comparing it with the bot "dependabot[bot]" does not prove that the bot made the change. check the author of the pull request instead, e.g. "github.event.pull_request.user.login" [bot-conditions]
  |
7 |     if: github.actor == 'dependabot[bot]'
  |         ^~~~~~~~~~~~
```

<!-- Skip playground link -->

Use `github.event.pull_request.user.login` (or `.id`), the author of the pull request. When the workflow runs on `pull_request` or
`pull_request_target` only, `-fix=unsafe` makes the change. It is unsafe because the condition is true for a pull request of the bot
that somebody else pushed to, where it used to be false. The GitHub documentation also recommends not auto-merging from
`pull_request_target`.

A condition that also requires the author of the pull request to be the bot, such as
`github.actor == 'dependabot[bot]' && github.event.pull_request.user.login == 'dependabot[bot]'`, is not reported: the author is
what proves that the bot made the change, and the actor next to it only narrows the condition.

<a id="check-obfuscation"></a>
## Obfuscated paths and expressions

Some constructs work but hide what they do from the people and the tools reading the workflow. This is the rule `obfuscation`. It is
in the `default` profile and reports:

- a path at `uses:` with empty, `.` or `..` segments, like `actions/checkout/./sub` or `./.github/actions/../actions/x`. A leading
  `../` of a local path is not reported because it refers to a repository checked out next to the workspace;
- `format()` with literal arguments only, and any other expression which is a constant, outside of `if:` (constant conditions are
  reported by `constant-condition`);
- `fromJSON(toJSON(x))`, unless `x` is a property of a context (`fromJSON(toJSON(matrix.container))` is a known way to turn the
  value into an object);
- an index which is computed, like `vars[format('NAME_{0}', github.job)]`. It hides which property is read.

Example input:

```yaml
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

Output:

<!-- Skip update output -->
```
test.yaml:7:15: warning: path of "actions/checkout/./sub@v4" has redundant or empty segments ("//", "." or ".."). write it as "actions/checkout/sub@v4" [obfuscation]
  |
7 |       - uses: actions/checkout/./sub@v4
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:10:27: warning: format() is called with literal arguments only so its result is the constant "octocat/hello-world". write the string itself [expression]
   |
10 |           repository: ${{ format('{0}/{1}', 'octocat', 'hello-world') }}
   |                           ^~~~~~~~~~~~~~~~~
test.yaml:11:24: warning: fromJSON(toJSON(...)) returns its argument unchanged. remove both calls [expression]
   |
11 |       - run: echo '${{ fromJSON(toJSON('[1]'))[0] }}'
   |                        ^~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:12:29: warning: the index is computed, which hides which property is read from tools that look for it. use a literal property name or a matrix to select the value [expression]
   |
12 |       - run: echo '${{ vars[format('NAME_{0}', github.job)] }}'
   |                             ^~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

`-fix=unsafe` rewrites the paths of `uses:`. It is unsafe because the plain path is expected, but not guaranteed, to name the same
action.

<a id="check-misfeature"></a>
## Misfeatures

Some features of GitHub Actions are best avoided. The rule `misfeature` (in the `default` profile) reports

- the `pip-install` input of actions/setup-python. It installs packages into the global Python environment, which is hard to audit
  and can break the resolution of dependencies. Create a virtual environment in a `run:` step instead;
- the Windows `cmd` shell (`shell: cmd`). It has no formal grammar, so a script cannot be analyzed reliably, and it has not been the
  default shell of Windows runners since 2019.

With the option `pedantic` (on under the `pedantic` profile) the rule also reports a shell that GitHub does not document (`bash`, `pwsh`,
`powershell`, `python`, `sh` and `cmd`), like `shell: perl {0}`. Such a shell may not exist on every runner and its scripts cannot be
analyzed. The shell names which GitHub does not accept at all are reported by the correctness check [shell names](#check-shell-names).

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

<!-- Skip update output -->
```
test.yaml:9:11: warning: "pip-install" of actions/setup-python installs packages into the global Python environment, which is hard to audit and can break the resolution of dependencies. create a virtual environment and install the packages in a "run:" step instead [misfeature]
  |
9 |           pip-install: .[dev]
  |           ^~~~~~~~~~~~
test.yaml:11:16: warning: shell "cmd" is the Windows cmd shell, which has no formal grammar so scripts cannot be analyzed reliably, and it has not been the default shell of Windows runners since 2019. use "pwsh", "bash" or another shell instead [misfeature]
   |
11 |         shell: cmd
   |                ^~~
```

<!-- Skip playground link -->

<a id="check-impostor-commit"></a>
## Impostor commits (online)

The checks in this section and the five that follow query the GitHub API, so they run only when you ask for it with
`jactionlint -online` or `online: true` in [the configuration](config.md#online-options). Nothing in jactionlint uses the network
otherwise. How the token, the cache and the rate limit work is in [the usage document](usage.md#online-checks). The online
checks are not available in the playground, so their examples have no playground link.

GitHub stores a repository and all its forks as one network of commits. A commit which exists only in a fork (or only in an
unmerged pull request) can therefore be written as `owner/repo@<sha>` of the parent repository, and a workflow pinned this way
looks as safe as any other hash-pinned action while it runs code that was never part of the repository. This is the rule
`impostor-commit`, the same as the [zizmor audit][zizmor-impostor-commit] of that name.

The rule accepts a commit when a tag of the repository points to it, or it is an ancestor of the head of the default branch or of
one of the other branches. The `max-branches` option (default `1000`) bounds how many branches are compared. With a token the
branches are compared 100 at a time in one GraphQL request; without one each branch costs a request and at most 100 are
compared. When the repository has more branches than that and none has the commit, the rule says nothing: it reports only what it
could verify. A commit that is in the history of a tag only (for example of a deleted release branch) is reported.

Example input:

```yaml
# requires -online
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

The same commit is also reported by `stale-action-refs`, because no tag points to it. To accept a finding, ignore it by rule ID
(`jactionlint -ignore impostor-commit`) or lower its level in the configuration. The fix is to pin a commit of the action's own
history, preferably the one of a release tag.

<a id="check-known-vulnerable-actions"></a>
## Known vulnerable actions (online)

GitHub publishes security advisories for actions, such as the leak of secrets through `tj-actions/changed-files`. The rule
`known-vulnerable-actions` looks the advisories of the GitHub Actions ecosystem up for every action and reusable workflow, works
out which version the workflow runs, and reports it when an advisory covers that version. This is the same as the
[zizmor audit][zizmor-known-vulnerable-actions] of that name.

The version comes from the ref: a version tag such as `v45.0.2` is the version. For `v45`, or a commit SHA, the rule takes the most
specific version tag on the same commit. A branch has no version and is never reported. The finding names the first patched
version when there is one.

Example input:

```yaml
# requires -online
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

Do not accept a finding unless you know the vulnerability does not apply to how you use the action. The `allow` option lists the
advisory IDs to ignore, and the level of the rule is set like any other:

```yaml
rules:
  known-vulnerable-actions:
    level: error
    allow:
      - GHSA-mrrh-fwg8-r2c3
```

<a id="check-ref-confusion"></a>
## Ref confusion (online)

`uses: owner/repo@v1` does not say whether `v1` is a branch or a tag. When the repository has both, anyone who can push the second
one can change what every workflow using the first runs, with no change to those workflows. The rule `ref-confusion` reports
an action whose ref is both a branch and a tag of its repository. It is the same as the [zizmor audit][zizmor-ref-confusion].
The fix is to pin the action to a full-length commit SHA.

Example input:

```yaml
# requires -online
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

Refs which are commit SHAs are never ambiguous and are not looked up.

An abbreviated commit SHA (7 to 39 hexadecimal digits) is read as a name by GitHub, which prefers a branch or a tag of that name to the
commit, so it is reported when the repository has one. A full-length SHA is read as the commit and is not looked up.

<a id="check-stale-action-refs"></a>
## Stale action refs (online)

An action pinned to a commit SHA that no tag points to runs a snapshot between two releases. It may contain bugs, or fixes of
vulnerabilities, which were never documented because changelogs describe releases. The rule `stale-action-refs` reports hash-pinned
actions whose commit is not the commit of any tag. It is the same as the [zizmor audit][zizmor-stale-action-refs] and, like it, is
informational: some repositories release from a rolling branch, where the finding does not apply.

Example input:

```yaml
# requires -online
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

When the repository has more tags than jactionlint reads (1000), the rule reports nothing for a commit it did not find.

<a id="check-archived-uses"></a>
## Archived repositories (online)

An archived repository is read-only: nobody fixes the vulnerabilities of the action, or of the dependencies bundled with it. The
rule `archived-uses` reports actions and reusable workflows which live in an archived repository. It is the same as the
[zizmor audit][zizmor-archived-uses]. Replace the action with a maintained one, or with the commands it wraps in a `run:` step (many
actions are thin wrappers around the `gh` CLI, which is on the GitHub-hosted runners).

Example input:

```yaml
# requires -online
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

<a id="check-ref-version-mismatch"></a>
## Version comments of pinned actions (online)

Dependabot and Renovate keep a hash-pinned action up to date by reading the version in the comment after it
(`uses: actions/checkout@<sha> # v4.2.2`). When the commit is changed by hand and the comment is not, the comment lies, and the tools
may skip it. The rule `ref-version-mismatch` reads the version of the comment and reports it when the commit is not the one of that tag.
It is the same as the [zizmor audit][zizmor-ref-version-mismatch], except that it does not report a pinned action
without a version comment.

A comment gives a version when it starts with one (`v4.2.2`, `4.2.2`, `v4`, `tag=v4.2.2`), as the bots write it; other
comments are left alone. The comment matches
when a tag with that version points to the pinned commit; the `v` prefix does not matter. A less specific
version is not enough: `# v4` on the commit of `v4.2.2` is reported when the tag `v4` points to another commit, because the
comment no longer describes what is pinned.

Example input:

```yaml
# requires -online
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
test.yaml:10:15: warning: the version comment "# v3.0.0" does not match the commit pinned in action "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683": tag "v3.0.0" points to commit a12a3943b4bd, but the pinned commit is tagged "v4.2.2". update the comment, or pin the commit of the version you mean [ref-version-mismatch]
   |
10 |       - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v3.0.0
   |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
```

<!-- Skip playground link -->

The `-online -fix` option can pin tags to commits and add this comment for you, see [the usage document](usage.md#online-checks).

<a id="check-invisible-characters"></a>
## Invisible characters

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
  |                               ^~~
test.yaml:7:36: invisible character U+202E RIGHT-TO-LEFT OVERRIDE in a comment: it changes the order in which the text around it is displayed, so the code can run differently from how it reads. remove it [invisible-characters]
  |
7 |       - run: echo "tests passed" # ‮success
  |                                    ^~~~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo8zDsOwjAQhOE+pxiF2qKhcsVVnGUl85DXyuxSp+cuHConQQaJaorv11jL6ME63WxhngBX+lhgjcY0PJZoHulRhn2Jrp2/CkgIKjOK+NUaj1JV7ha+b6/z8/SP1mgZKtUwjx+iF1IvMw7YtzdDRMnPAKsALro=)

The rule `invisible-characters` (in the `default` profile, as an error) reports characters which are displayed as nothing, or which
change how the text around them is displayed, in a workflow or a Dependabot configuration. The example above has a zero width
space in the `uses:` reference and a right-to-left override in the comment. Neither is visible in an editor, and GitHub does not
show them in the diff view of a pull request either, so a change that adds one looks like a change of nothing, or of a comment. They
can make a value differ from what a reviewer reads (an action, a branch, a condition) or reorder the text so that code reads
differently from how it runs ([Trojan Source][trojan-source]).

The rule reports these characters, wherever they are in the file (code, a key, a string or a comment, and also in a file that does
not parse):

- bidirectional controls: embeddings, overrides and isolates (`U+202A` to `U+202E`, `U+2066` to `U+2069`), and the marks
  `U+061C`, `U+200E` and `U+200F` unless they are next to right-to-left text
- zero width characters: `U+200B`, `U+2060`, `U+180E`, the invisible operators `U+2061` to `U+2064`, the soft hyphen `U+00AD`, the
  deprecated format characters `U+206A` to `U+206F`, `U+FEFF` anywhere except at the start of the file, and the joiners
  `U+200C` and `U+200D` where they are not part of correct spelling
- tag characters (`U+E0000` block), which can smuggle text, and variation selectors that do not follow a character they can modify
- fillers that are drawn as blanks (`U+115F`, `U+1160`, `U+3164`, `U+FFA0`), line and paragraph separators (`U+2028`, `U+2029`) and
  control characters other than tab, line feed and carriage return (such as the escape character)

Letters, combining marks and symbols of any language are never reported, so non-English text is fine. These legitimate uses are
not reported either: the byte order mark at the start of a file, emoji sequences (a zero width joiner or a variation selector
between emoji, keycaps, flags made from tag characters), the zero width (non-)joiner in scripts which are spelled with it (Persian,
Devanagari and other Indic scripts, and others), the variation selectors of CJK ideographs and Mongolian, and direction marks
next to Arabic or Hebrew letters.

One finding is reported for a run of adjacent characters. The message says where the character is (a `run:` script, an
expression, a `uses:` reference, a comment, a value or a key). Every finding is an error, also in a comment: a comment is how the
change is made to look harmless.

`-fix` removes the characters of the finding. This is a safe fix: what is left is what the reader of the file already saw. If the
character is meant to be in a string, write it as an escape in a double quoted YAML string (`"\u200b"`) or in the shell
(`$'\u200b'`), where it is visible in the source. To silence a finding, put `# jactionlint ignore=invisible-characters` on the line above it,
or turn the rule off with `rules: invisible-characters: off`.

zizmor has no audit for this (the request is [zizmor#914][zizmor-914]). `action.yml` files are not checked yet, because jactionlint
does not lint them as files.

<a id="check-unsound-prefix-match"></a>
## Unsound prefix matches on names

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

The rule `unsound-prefix-match` (in the `default` profile, as an error) reports `startsWith()`, `endsWith()` and `contains()` calls
which test an account, an organization or a repository by a part of its name. Names are chosen by whoever registers them, so
`startsWith(github.actor, 'jdx')` is also true for `jdx-evil`, and `endsWith(github.repository, '/mise')` is true for the fork of
`mise` by every owner. When the condition decides whether a privileged step runs, this is a bypass of the check.

Compare the whole name, or test a list of whole names:

```yaml
if: github.actor == 'jdx'
# or
if: contains(fromJSON('["jdx", "jdy"]'), github.actor)
```

The properties which are checked are the names of the actor, the sender, the repository owner and the authors of pull requests,
issues and comments (`github.actor`, `github.triggering_actor`, `github.repository_owner`, `github.event.sender.login`,
`github.event.pull_request.user.login`, and similar ones), and the full name of a repository (`github.repository`,
`github.event.repository.full_name`, `github.event.pull_request.head.repo.full_name`). The literal must be the second argument; a
literal first argument of `contains()` is the business of [`unsound-contains`](#check-unsound-contains).

Not reported:

- a prefix which ends with a slash on a repository name, as in `startsWith(github.repository, 'jdx/')`, or which has a slash in it
  (`jdx/mise`): the owner is complete, so only the owner can create the repository
- a name ending with `[bot]`: GitHub appends it, so `startsWith(github.actor, 'dependabot[bot]')` is a whole name. The comparison of
  `github.actor` with a bot in a condition is reported by [`bot-conditions`](#check-bot-conditions) when that rule is on, so it is not
  reported twice
- a negated test (`!startsWith(github.actor, 'bot-')`): it only excludes names, nobody gets in through it
- an expression where the same property is also compared exactly in an `&&` chain
  (`github.actor == 'jdx' && startsWith(github.actor, 'j')`) or tested against a list of whole names, because the exact test decides.
  An exact test in a different `if:` (a job and a step) is not understood
- values that are not literals, such as `startsWith(github.actor, inputs.user)`
- an expression outside a condition (`env:`, `with:`, `runs-on:`), unless it also refers to a secret, `github.token` or a
  `self-hosted` runner: `GOOS: ${{ contains(github.repository, 'windows_exporter') && 'windows' || '' }}` selects a setting, not a
  credential, but `token: ${{ startsWith(github.actor, 'jdx') && secrets.TOKEN }}` decides who gets one

The names of branches and tags (`github.ref`, `github.head_ref`, ...) are not checked by default, since `startsWith(github.ref,
'refs/tags/v')` is usually a pattern someone means. Set the option `refs` to check them as well, except for prefixes ending with a
slash:

```yaml
rules:
  unsound-prefix-match:
    level: error
    refs: true
```

There is no automatic fix, because the right comparison depends on what you mean to trust. zizmor has no audit for this yet (the
request is [zizmor#1533][zizmor-1533]).

[trojan-source]: https://trojansource.codes/
[zizmor-914]: https://github.com/zizmorcore/zizmor/issues/914
[zizmor-1533]: https://github.com/zizmorcore/zizmor/issues/1533

[zizmor-impostor-commit]: https://docs.zizmor.sh/audits/#impostor-commit
[zizmor-known-vulnerable-actions]: https://docs.zizmor.sh/audits/#known-vulnerable-actions
[zizmor-ref-confusion]: https://docs.zizmor.sh/audits/#ref-confusion
[zizmor-stale-action-refs]: https://docs.zizmor.sh/audits/#stale-action-refs
[zizmor-archived-uses]: https://docs.zizmor.sh/audits/#archived-uses
[zizmor-ref-version-mismatch]: https://docs.zizmor.sh/audits/#ref-version-mismatch

---

[Installation](install.md) | [Usage](usage.md) | [Configuration](config.md) | [Go API](api.md) | [References](reference.md)

<a id="check-concurrency-cancels-prs"></a>
## Concurrency that cancels unrelated pull requests

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

The rule `concurrency-cancels-prs` (in the `default` profile) reports a `concurrency` block (of the workflow or
of a job) of a workflow that is triggered by `pull_request`, `pull_request_target`, `pull_request_review` or
`pull_request_review_comment`, when `cancel-in-progress` is on and the `group` has nothing that differs between pull requests.
All pull requests share the group, so a push to one of them cancels the run of another one that has nothing to do with it.

Add the pull request to the group:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.head_ref || github.run_id }}
  cancel-in-progress: true
```

What the rule judges:

- A group is fine when it reads something that differs between pull requests for the event: `github.head_ref`,
  `github.event.pull_request.number`, `github.event.number`, the head of the pull request, `github.run_id` and so on. For
  `pull_request` it also accepts `github.ref`, which is `refs/pull/<number>/merge`. It does **not** accept `github.ref` and
  `github.sha` for `pull_request_target`, because they are the base branch there, and it does not accept `github.head_ref` for
  `pull_request_review`, where it is empty.
- `cancel-in-progress` counts as on when it is `true` or an expression that is true for the pull request event as far as the
  rule can tell from `github.event_name` and `github.ref`. The idiom `cancel-in-progress: ${{ github.event_name == 'pull_request' }}`
  is on for pull requests, so it is reported when the group is shared. An expression that depends on something else (a
  variable, an input) is not reported.
- A group that reads `env`, `vars`, `inputs`, `needs`, `steps` or `secrets` is not reported, because the rule cannot see its value.

There is no automatic fix: which value distinguishes the runs is up to you. To turn the rule off use `rules: {concurrency-cancels-prs: off}`
in the [configuration file](config.md) or an ignore comment on the `group:` line (`# jactionlint ignore=concurrency-cancels-prs`).

<a id="check-concurrency-cancels-release"></a>
## Concurrency that cancels a release

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
   |              ^~~~~
```

[Playground](https://jactionlint.jdx.dev/#eNo0zD2qwzAQxPFepxhcPtAFdJVHCnlZFAexK/YjkNsHx7iaYv78VFoBVvrzXCD68Ib/7f23PQqpUJqx0Od8h2muBuPJ3bkA1IV41kPqMh3G7g1hyeWlu1/uPo+bthSvKg25p0TW2YM9fpcHL78qoJ5lA3UbegvfAQA6LzT4)

The rule `concurrency-cancels-release` (in the `default` profile) reports `cancel-in-progress` that cancels a
release or a deployment which is still running: a new push, tag or manual run must not kill an in-flight release. Cancelling
the run of a publish half way can leave a tag without its packages, a registry with some of the files or a deployment half
rolled out.

A `concurrency` block of the workflow or of a job is reported when `cancel-in-progress` is on for a trigger other than a pull
request and one of these is true:

- The workflow runs for a tag push or a `release` event, and the group does not name the ref. This is the case where the next
  tag cancels the release of the previous one.
- A job under the block publishes or deploys. That is a job with an `environment:`, a step that uses a release or deploy
  action (for example `softprops/action-gh-release`, `pypa/gh-action-pypi-publish`, `actions/deploy-pages`,
  `cloudflare/wrangler-action`, `goreleaser/goreleaser-action` with `release` in `args` and without `--snapshot`), a
  `docker/build-push-action` with `push` on, or a `run:` step with a command such as `npm publish`, `cargo publish`,
  `twine upload`, `gh release create`, `docker push`, `wrangler deploy` or `kubectl apply`. A job or step whose `if:` is false
  for the trigger is ignored (`if: startsWith(github.ref, 'refs/tags/')` does not count for a push to a branch). `--dry-run`
  commands do not count.

What is **not** reported: the mixed idiom `cancel-in-progress: ${{ github.event_name == 'pull_request' }}` (false for pushes
and tags) and other expressions that are false for the trigger; an expression that depends on something the rule cannot see
(an input, a variable); a group that names the release (`inputs.*`, `github.event.release.*`) and, for tag pushes, releases
and manual runs, a group that names the ref, because then only a second run for the same ref replaces the first one; and
workflows that only run for pull requests. Names of workflows and jobs are not taken as a signal.

The fix sets a literal `cancel-in-progress: true` to `false`. It is **unsafe** (`-fix=unsafe`) because the runs of a group queue
instead of replacing each other, which changes when and how often the workflow runs. Expressions are not fixed.

What to write for a release:

```yaml
concurrency:
  group: release-${{ github.ref }} # one group per tag or branch, so that unrelated releases do not wait for each other
  cancel-in-progress: false # or leave it out: false is the default
  queue: max # when no release may be skipped
```

`cancel-in-progress: false` lets the running release finish. Without `queue`, GitHub keeps one pending run per group and a newer
pending run replaces the older pending one, which skips that release. `queue: max` keeps the pending runs in a queue instead, so
every release runs. It cannot be combined with `cancel-in-progress: true`, and jactionlint accepts it. A group per tag or ref
is what keeps the releases of different tags from queueing behind each other.

Ignore the rule with `# jactionlint ignore=concurrency-cancels-release` on the line or turn it off with `rules: {concurrency-cancels-release: off}`.

<a id="check-gate-job-skipped-on-failure"></a>
## Gate jobs that are skipped when a job fails

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

The rule `gate-job-skipped-on-failure` (in the `default` profile, as an error) reports a job that has `needs` and reads the
result of the jobs it needs, but whose own `if:` has no status check function. GitHub adds an implicit `success()` to such a
job, so it is **skipped** when a needed job fails, is cancelled or is skipped. The check never sees a failure, and a skipped job
counts as passing for a required status check, so the failure goes through the gate.

Add a status check function that lets the job run, and check the results yourself:

```yaml
final:
  needs: [build, test]
  if: ${{ !cancelled() }}
  runs-on: ubuntu-latest
  steps:
    - run: test "${{ contains(needs.*.result, 'failure') }}" = false
```

The rule looks at the whole job: its `if:`, the steps, `env`, `with`, `outputs` and the rest. It reports a read of
`needs.<job>.result` or `needs.<job>.outcome`, of `needs.*.result`, and of the whole `needs` context (for example `toJSON(needs)`).
`always()`, `cancelled()` and `failure()` (also negated, as in `!cancelled()`) in the job's `if:` satisfy the rule; an explicit
`success()` does not, because it is what GitHub adds anyway.

What is **not** reported:

- A read that only compares the result with `'success'` using `==` (`if: needs.build.result == 'success'`). It is redundant in
  a job that is skipped unless its needs succeeded, but it does not expect to see a failure.
- Jobs that need a job with `continue-on-error`, because the result of that job is not what it seems.
- Reads of `needs.<job>.outputs.*`.
- A workflow where an expression does not parse (the syntax error is reported by `expression`).

There is no automatic fix: whether `always()` or `!cancelled()` is right depends on whether the gate should run for a cancelled
workflow.

<a id="check-untrusted-checkout"></a>
## Untrusted code in privileged workflows

Example input:

```yaml
on: pull_request_target
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm ci && npm test
```

Output:

```
test.yaml:8:16: this step checks out code from a pull request (github.event.pull_request.head.sha) in a "pull_request_target" workflow and "npm" runs it afterwards. the workflow has a write token and secrets, so whoever controls that code can use them. run untrusted code in a "pull_request" workflow without secrets, or check out the base branch and only read the pull request as data [untrusted-checkout]
  |
8 |           ref: ${{ github.event.pull_request.head.sha }}
  |                ^~~
```

[Playground](https://jactionlint.jdx.dev/#eNpMjjHOwjAMRvee4ht+dWv+hSkTN6nSYJpASUpsl6Hq3VEKqphs6z3JLyeLWaepL/RUYunFlZGkueWBbQMIsdQJFE3cVV0HTaLd5CrbEQvN/LGADsrEFs5LzIn/fSB/zyrn5fQ1gFeUYI8LKHS1+FtXjFGCDoYWSmJ+u0wgdzEcHLbteFQ0WaT5AR/RtvtWm94DACIeQkk=)

The rule `untrusted-checkout` (in the `default` profile, as an error) reports a `pull_request_target` or `workflow_run`
workflow that checks out the code of a pull request and then runs it. These events run in the context of the base repository,
with a token that can write and with the secrets, while the code is what the author of the pull request wrote. A build script,
a test, a `package.json` hook or a local action is enough to take over the token.

Run untrusted code in a `pull_request` workflow, which has no secrets for pull requests from forks. If a privileged workflow
has to look at the pull request, check out the base branch and read the pull request as data.

A step is reported at its checkout when it puts untrusted code in the workspace and a later step in the same job runs it:

- `actions/checkout` with a `ref` or `repository` that names the head of a pull request or of the triggering run
  (`github.event.pull_request.head.*`, `github.head_ref`, `github.event.pull_request.merge_commit_sha`,
  `github.event.workflow_run.head_sha`, `head_branch`, `head_repository`, ...) or `refs/pull/...`.
- `gh pr checkout`, and `git checkout`, `fetch`, `switch`, `merge`, `pull`, `clone` ... with such a reference in the arguments or
  in an environment variable they use.
- "Runs it" is a later step with a `run:` command that can execute workspace code (a script, `npm`, `cargo`, `make`, an
  interpreter, ...), a local action (`uses: ./...`), or one of a few actions that build the workspace. Commands that only read or
  move files (`cat`, `git diff`, `grep`, `jq`, `tar`, `gh`, ...) do not count. A checkout with `path:` only counts when a later
  step works in that directory (`working-directory`, `cd`, or an argument below it).

What is **not** reported: a checkout of the base (no `ref`, or `github.event.pull_request.base.*`), a checkout that nothing runs,
a job with an `environment:` (its reviewers decide), and a job or step whose `if:` reads who or what started the workflow:
`github.actor`, the author or labels of the pull request, `github.event.pull_request.head.repo`, `github.event.workflow_run.event`,
`needs` or `steps`. A `workflow_run` workflow is not reported when every workflow in `workflows:` is in the repository and only
runs for `push`, `schedule`, `workflow_dispatch`, `release`, `merge_group` or `repository_dispatch`. The rule cannot tell that
a `labeled` pull request was reviewed or that an earlier step vouched for the code in a way other than a condition on `needs` or
`steps`. Code that other steps fetch from an output (`ref: ${{ steps.x.outputs.sha }}`) is not followed.

There is no automatic fix.

<a id="check-untrusted-artifact"></a>
## Untrusted artifacts in workflow_run workflows

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
test.yaml:3:17: workflow "PR checks" specified at "workflows" of "workflow_run" event is not found in the repository. a workflow is specified by its "name:" or its file path when it has no name [workflow-run]
  |
3 |     workflows: [PR checks]
  |                 ^~
test.yaml:9:15: this step downloads an artifact of the run that triggered the workflow, which ran the code of a pull request, and the step at line 15 writes its content to $GITHUB_ENV without validating its content first. the artifact is whatever the pull request wanted, and this workflow has a write token and secrets. match the content against a strict pattern (for example digits only) before you use it, and never run or extract it [untrusted-artifact]
  |
9 |       - uses: actions/download-artifact@v4
  |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
test.yaml:15:45: a value that is not a literal is written to $GITHUB_ENV in a workflow triggered by "workflow_run", which runs with secrets and a write token for events that may come from a fork. an attacker who controls the value can set LD_PRELOAD or NODE_OPTIONS (a newline adds another variable) and run code in the next steps. write only literal values and values computed from trusted sources, or pass state with $GITHUB_OUTPUT [github-env]
   |
15 |       - run: echo "number=$(cat pr/number)" >> "$GITHUB_ENV"
   |                                             ^~
```

<!-- Skip playground link -->

The rule `untrusted-artifact` (in the `default` profile, as an error) reports a `workflow_run` workflow that downloads an artifact
of the run that triggered it and then runs it, extracts it or writes its content to `$GITHUB_ENV`, `$GITHUB_PATH` or
`$GITHUB_OUTPUT` without checking it. The upstream workflow ran the code of a pull request, so the artifact is whatever the pull
request wanted. The `workflow_run` workflow has a write token and the secrets. A file name or a value with a newline in the
environment file is enough to inject a variable such as `LD_PRELOAD` or `NODE_OPTIONS`, and an archive can write outside its
directory.

Check the content before you use it, for example match a pull request number against `^[0-9]+$`:

```yaml
- run: |
    NUMBER=$(cat pr/number)
    [[ "$NUMBER" =~ ^[0-9]+$ ]] || exit 1
    echo "number=$NUMBER" >> "$GITHUB_OUTPUT"
```

The download is `actions/download-artifact` with a `run-id` that reads `github.event.workflow_run`, any use of
`dawidd6/action-download-artifact`, `gh run download`, or an `actions/github-script` that calls `listWorkflowRunArtifacts` and
`downloadArtifact`. The use is a later step of the same job with a `run:` that

- runs something below the artifact directory (`path:`, or the artifact `name` when there is no `path`), including `cd` into it
  and `working-directory:`;
- extracts an archive (`unzip`, `tar -x`, `7z x`, ...) from there, or any archive after a download with `github-script`;
- writes data read from there (`cat`, `jq`, `$(...)`, `read ... < file`) to `$GITHUB_ENV`, `$GITHUB_PATH` or `$GITHUB_OUTPUT`.

A step that validates the data stops the check from there on: a regular expression match (`=~`, `grep -E`), a numeric test
(`-eq`), a checksum or signature verification (`sha256sum -c`, `gh attestation verify`, `cosign verify`, ...). This is
deliberately generous.

What is **not** reported: artifacts of the current run, a download whose files cannot be linked to a later command (an artifact
without `path` and `name` lands in the root of the workspace under names the rule does not know), jobs with an `environment:`,
jobs and steps with the same guards as `untrusted-checkout`, and workflows that wait only for workflows that run for `push`,
`schedule` and other events that only people with write access cause.

There is no automatic fix.

<a id="check-unused-job-output"></a>
## Unused job outputs

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

The rule `unused-job-output` (in the `default` profile) reports an entry of `jobs.<id>.outputs` that nothing reads:
no job reads `needs.<id>.outputs.<name>` and no output of a reusable workflow (`on.workflow_call.outputs`) uses
`jobs.<id>.outputs.<name>`. The output is dead code, and it makes a reader look for a consumer that does not exist.

The rule reads every expression of the workflow. A read of a whole object counts as a read of everything in it:
`toJSON(needs.build.outputs)`, `toJSON(needs)`, `needs[matrix.job].outputs.x`. When an expression does not parse, nothing is
reported. Outputs can only be read inside the workflow, so an output of a reusable workflow that its callers use is a
`workflow_call` output, not a job output.

There is no automatic fix because removing an output means removing a block of YAML and finding its step.

<a id="check-unused-workflow-input"></a>
## Unused workflow inputs (pedantic)

Example input:

```yaml
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

Output:
<!-- Skip update output -->
```
test.yaml:6:7: warning: input "dry-run" of "workflow_dispatch" is never used: no expression reads "inputs.dry-run". remove it or use it [unused-workflow-input]
  |
6 |       dry-run:
  |       ^~~~~~~~
```

<!-- Skip playground link -->

The output above is from the following `rules` section of the [configuration file](config.md):

```yaml
rules:
  unused-workflow-input: warn
```

The rule `unused-workflow-input` (in the `pedantic` profile, as a warning) reports an input of `workflow_dispatch` or
`workflow_call` that no expression of the workflow reads. A manual run asks for a value that does nothing, or a caller passes
a value that is dropped.

An input is read as `inputs.<name>`; for `workflow_dispatch` also as `github.event.inputs.<name>`. A read of the whole context
(`toJSON(inputs)`, `inputs[matrix.name]`) counts as a read of every input, and when a script reads `$GITHUB_EVENT_PATH` the inputs
of `workflow_dispatch` are not reported because the script can read them from the payload. A workflow where an expression does not
parse is skipped.

The rule is not in the `default` profile because an input can exist for someone else: GitHub refuses a manual run or a
`workflow_call` that passes an input the workflow does not declare, so a tool that dispatches the workflow with an input of its own
(for example the `distinct_id` that `codex-/return-dispatch` passes) or a caller in another repository needs it even when the
workflow never reads it. Silence such an input with `# jactionlint ignore=unused-workflow-input` on its line. There is no
automatic fix because removing an input of `workflow_call` breaks the callers that still pass it.

<a id="check-unused-needs"></a>
## Needs entries that do nothing (pedantic)

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

Output:
<!-- Skip update output -->
```
test.yaml:13:13: info: job "publish" needs "build" but never reads its outputs or result, and it already needs "test" which waits for "build". this entry changes nothing and can be removed [unused-needs]
   |
13 |     needs: [build, test]
   |             ^~~~~~
```

<!-- Skip playground link -->

The output above is from the following `rules` section of the [configuration file](config.md):

```yaml
rules:
  unused-needs: info
```

The rule `unused-needs` (in the `pedantic` profile, as info) reports an entry of `needs` that has no effect: the job never reads
the outputs or the result of the needed job, and another job it needs already depends on it, so the entry changes neither the order
of the jobs nor whether the job runs.

An entry is reported only when this can be shown. The other needed job must wait for the job directly or through other jobs, and
none of the jobs on the way may have a status check function in its `if:` (`always()`, `!cancelled()`, `failure()`, ...), because
such a job runs even when its needs failed, and then the entry changes the result. An entry that is only there for the order
(`needs: [a, b]` with unrelated jobs) is **not** reported: the rule cannot tell it from a forgotten one.

There is no automatic fix.

<a id="check-duplicate-triggers"></a>
## Duplicate triggers

Example input:

```yaml
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - run: echo test
```

Output:
<!-- Skip update output -->
```
test.yaml:1:6: warning: "push" has no branch filter and "pull_request" is used too, so a commit pushed to a branch of this repository that has a pull request runs the workflow twice. limit "push" to the branches that need it, for example the default branch [duplicate-triggers]
  |
1 | on: [push, pull_request]
  |      ^~~~~
```

<!-- Skip playground link -->

The output above is from the following `rules` section of the [configuration file](config.md):

```yaml
rules:
  duplicate-triggers: warn
```

The rule `duplicate-triggers` (in the `default` profile) reports a workflow that is triggered by `push` and by
`pull_request` when `push` has no branch filter. A commit pushed to a branch of the repository that has a pull request starts the
workflow twice, once for each event, and both runs say the same thing.

Limit `push` to the branches that need it:

```yaml
on:
  push:
    branches: [main]
  pull_request:
```

`push` counts as unfiltered when it has no `branches` (or only `**`), also with `branches-ignore`, `paths` or `tags-ignore`. A
`push` with only `tags` does not run for branches. A `pull_request` with only `types` that do not carry new commits (for example
`closed`) is not a duplicate. The rule does not report a workflow where every job has an `if:` on `github.event_name` or on
`github.event.pull_request.head.repo`, or where the group of the workflow `concurrency` has `github.head_ref` and
`github.ref_name` and cancels runs, which are the two common ways to run once.

There is no automatic fix because the right branches are not known.

<a id="check-continue-on-error"></a>
## Failures hidden by continue-on-error (pedantic)

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

Output:
<!-- Skip update output -->
```
test.yaml:5:24: info: "continue-on-error: true" makes the workflow pass even when job "lint" fails, which hides failures. remove it, or limit it to what is allowed to fail, for example with an expression on a matrix entry [continue-on-error]
  |
5 |     continue-on-error: true
  |                        ^~~~
```

<!-- Skip playground link -->

The output above is from the following `rules` section of the [configuration file](config.md):

```yaml
rules:
  continue-on-error: info
```

The rule `continue-on-error` (in the `pedantic` profile, as info) reports a job with a literal `continue-on-error: true`. The workflow
succeeds when the job fails, so nobody sees the failure unless they open the job. Some jobs are meant to be advisory; for those
the finding is a reminder to keep it deliberate.

An expression (`continue-on-error: ${{ matrix.experimental }}`) is not reported, because it is the usual way to allow the
failure of some matrix entries only. Steps with `continue-on-error: true` are reported only when you set the `steps` option:

```yaml
rules:
  continue-on-error:
    steps: true
```

There is no automatic fix: removing the line turns an advisory job into a blocking one.

<a id="check-mutable-runner-label"></a>
## Mutable runner labels (pedantic)

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
```

Output:
<!-- Skip update output -->
```
test.yaml:4:14: warning: runner label "ubuntu-latest" is an alias that GitHub moves to newer images, so the job can break without a change in this repository. use a fixed label such as "ubuntu-24.04", which is the same image today [mutable-runner-label]
  |
4 |     runs-on: ubuntu-latest
  |              ^~~~~~~~~~~~~
```

<!-- Skip playground link -->

The output above is from the following `rules` section of the [configuration file](config.md):

```yaml
rules:
  mutable-runner-label: warn
```

The rule `mutable-runner-label` (in the `pedantic` profile, as a warning) reports the labels of GitHub-hosted runners that GitHub
moves to a newer image over time: `ubuntu-latest`, `windows-latest`, `macos-latest` and their sized variants. A job on
such a label can start to fail on the day GitHub switches the image, without any change in the repository. The message names the fixed label that the
alias is today, according to the label table of jactionlint.

The rule reads `runs-on` and the values of `matrix.<key>` that `runs-on: ${{ matrix.<key> }}` selects. Jobs with `self-hosted` among
the labels and labels given by other expressions are not reported.

The fix is only offered when you decide the replacement, because jactionlint does not know which version you want. Configure it
with the `pin` option; the finding of a label with an entry then has a fix that writes the fixed label in its place:

```yaml
rules:
  mutable-runner-label:
    pin:
      ubuntu-latest: ubuntu-24.04
      macos-latest: macos-15
```

A label in a matrix is reported but never fixed, because the same value may be compared in an expression of the job.

[yamllint]: https://github.com/adrienverge/yamllint
[issue-form]: https://github.com/jdx/jactionlint/issues/new
[syntax-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions
[filter-pattern-doc]: https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#filter-pattern-cheat-sheet
[shellcheck]: https://github.com/koalaman/shellcheck
[shellcheck-install]: https://github.com/koalaman/shellcheck#installing
[SC1091]: https://github.com/koalaman/shellcheck/wiki/SC1091
[SC2050]: https://github.com/koalaman/shellcheck/wiki/SC2050
[SC2194]: https://github.com/koalaman/shellcheck/wiki/SC2194
[SC2154]: https://github.com/koalaman/shellcheck/wiki/SC2154
[SC2157]: https://github.com/koalaman/shellcheck/wiki/SC2157
[SC2043]: https://github.com/koalaman/shellcheck/wiki/SC2043
[shellcheck-env-var]: https://github.com/koalaman/shellcheck/wiki/Integration#environment-variables
[pyflakes]: https://github.com/PyCQA/pyflakes
[expr-doc]: https://docs.github.com/en/actions/learn-github-actions/expressions
[contexts-doc]: https://docs.github.com/en/actions/learn-github-actions/contexts
[funcs-doc]: https://docs.github.com/en/actions/learn-github-actions/expressions#functions
[needs-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idneeds
[needs-context-doc]: https://docs.github.com/en/actions/learn-github-actions/contexts#needs-context
[shell-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#using-a-specific-shell
[matrix-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idstrategymatrix
[webhook-doc]: https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#webhook-events
[schedule-event-doc]: https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#scheduled-events
[cron-syntax]: https://pubs.opengroup.org/onlinepubs/9699919799/utilities/crontab.html#tag_20_25_07
[schedule-item-doc]: https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onschedule
[gh-hosted-runner]: https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners
[self-hosted-runner]: https://docs.github.com/en/actions/hosting-your-own-runners/about-self-hosted-runners
[action-uses-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idstepsuses
[dependabot-doc]: https://docs.github.com/en/code-security/dependabot/working-with-dependabot/keeping-your-actions-up-to-date-with-dependabot
<a id="check-composite-actions"></a>
## Composite actions

jactionlint checks the metadata of actions (`action.yml` or `action.yaml`) together with the workflows. When the repository is
linted without file arguments it checks the action in the root of the repository, every action under `.github/actions`, and every
directory which a local `uses: ./path` of a workflow or of another action refers to. An action is also checked when its file is given
explicitly (`jactionlint .github/actions/setup/action.yml`, which is what the [hk][hk] step does for `.github/actions/**/action.y*ml`)
or with `-stdin-filename`. The file is recognized by its name: `action.yml` and `action.yaml` are actions anywhere except under
`.github/workflows`. Output formats, [ignore comments](usage.md#ignore-some-errors), `paths:` in [the configuration](config.md),
SARIF and `-fix` work for them like for workflows.

The `steps` of a composite action (`runs.using: composite`) are checked with the same rules as the steps of a workflow job, and
the rule IDs are the same. Actions that run JavaScript (`node20`, `node24`, ...) or a container (`docker`) have no steps: only their
metadata is checked (the syntax, and the `docker://` image of a Docker action by `unpinned-images`).

What differs from a workflow:

- Every `run:` step must have `shell:` because a composite action has no default shell. A missing one is an error of
  `action-syntax`, so the opt-in `require-shell` is not used for actions.
- The `inputs` context has the inputs declared in the `inputs:` section (all of them are strings), and `outputs.<id>.value` can read
  the `steps` context. The `secrets` context is not available: [secrets must be passed as an input][contexts-doc].
- There is no `on:`, `permissions:`, `runs-on:`, `needs:`, `timeout-minutes:` or `concurrency:`: they belong to the workflow that calls
  the action, so the rules about them are not applied to actions.

**Caller-aware rules.** Some findings depend on how the action is run, which only the calling workflow knows. For every action
jactionlint finds the local workflows which call it with `uses: ./path`, also through other local actions and reusable workflows
(`uses: ./.github/workflows/build.yml`; a reusable workflow runs in the context of the workflow that calls it). Cycles, like an action
that calls itself, are followed once. The context of an action is the most dangerous one among its callers: an action called from
a `pull_request` workflow and from a `pull_request_target` workflow is judged as if it only ran in the second one. The message names
the workflow, for example `.github/workflows/release.yml runs on the release event and calls this action`.

- `cache-poisoning` reports a restored cache when a calling workflow runs on `release` or on pushed tags.
- `bot-conditions` offers its fix (which needs a pull request event) only when every caller runs on a pull request event.
- `github-env`, `untrusted-checkout`, `untrusted-artifact` and `agentic-actions` look at the events of the callers like they look at the events of a workflow: `github-env` and `untrusted-checkout` need a caller that runs on `pull_request_target` (or `workflow_run`), `untrusted-artifact` one that runs on `workflow_run`, and `agentic-actions` one that outsiders can steer.
- `template-injection` adds the calling workflow to the message when it
  runs on a trigger that an outsider controls (`pull_request_target`, `workflow_run`, `issue_comment`, `issues`, comments and reviews).

When no local workflow calls the action (it is published for other repositories, or used by a workflow that is not in the repository)
the rules do not assume a trigger and judge the action by its own steps: the caller-dependent findings above are not reported, and
`bot-conditions` has no fix. Findings about the inputs (for example that `inputs.title` is expanded into a script) are reported either
way because any caller can pass attacker-controlled text. jactionlint does not look at what the callers pass in `with:`.

How the rules treat actions:

- **Applies** to the steps (and the metadata) of an action: `action-syntax`, `adhoc-packages`, `archived-uses`, `artipacked`, `checkout-static-credentials`, `constant-condition`, `context-availability`, `deprecated-action-input`, `deprecated-commands`, `duplicate-key`, `duplicate-step-id`, `expired-ignore`, `expression-syntax`, `expression-type`, `forbidden-uses`, `github-app`, `if-always-true`, `impostor-commit`, `insecure-commands`, `insecure-ssh-keyscan`, `insecure-url-scheme`, `invalid-env-var-name`, `invalid-function-call`, `invalid-id`, `invalid-ignore-comment`, `invalid-local-action`, `invalid-parallel-step`, `invalid-shell-name`, `invalid-uses`, `invisible-characters`, `known-vulnerable-actions`, `max-run-lines`, `merge-key`, `misfeature`, `missing-action-input`, `obfuscation`, `outdated-action-runner`, `pipeline-without-pipefail`, `pyflakes`, `recursive-alias`, `ref-confusion`, `ref-version-mismatch`, `require-expression-wrapping`, `self-repository`, `shellcheck`, `stale-action-refs`, `superfluous-actions`, `template-injection`, `typosquat-uses`, `undefined-function`, `undefined-property`, `unknown-action-input`, `unlocked-install`, `unpinned-images`, `unpinned-tools`, `unpinned-uses`, `unsound-contains`, `unsound-prefix-match`, `unsound-ternary`, `unused-anchor`, `unused-ignore`, `unverified-download`, `use-trusted-publishing`, `yaml-syntax`.
- **Caller-dependent**: `agentic-actions`, `bot-conditions`, `cache-poisoning`, `github-env` (its findings about untrusted input do not depend on the caller), `untrusted-artifact` and `untrusted-checkout`.
- **Not applicable** because an action does not have what the rule checks, or because only the calling job can decide (`unused-baseline-entry` is about the baseline file, not about a workflow or an action): `anonymous-definition`, `concurrency-cancels-prs`, `concurrency-cancels-release`, `concurrency-limits`, `conflicting-runner-labels`, `continue-on-error`, `cron-too-frequent`, `cyclic-job-needs`, `dangerous-triggers`, `dependabot-cooldown`, `dependabot-execution`, `dependabot-missing-actions-update`, `dependabot-syntax`, `duplicate-job-id`, `duplicate-job-needs`, `duplicate-triggers`, `excessive-permissions`, `gate-job-skipped-on-failure`, `hardcoded-container-credentials`, `invalid-activity-type`, `invalid-cron`, `invalid-event-config`, `invalid-event-filter`, `invalid-glob`, `invalid-label-pattern`, `invalid-local-workflow`, `invalid-permissions`, `invalid-timezone`, `invalid-workflow-call`, `invalid-workflow-call-input`, `invalid-workflow-dispatch-input`, `local-action-checkout`, `matrix-duplicate-value`, `matrix-invalid-exclude`, `missing-permissions`, `missing-timeout`, `missing-workflow-input`, `missing-workflow-secret`, `mutable-runner-label`, `overprovisioned-secrets`, `require-shell`, `required-actions`, `secrets-inherit`, `secrets-outside-env`, `self-hosted-runner`, `timeout-too-long`, `undefined-job-needs`, `undocumented-permissions`, `unknown-event`, `unknown-runner-label`, `unknown-workflow-input`, `unknown-workflow-secret`, `unredacted-secrets`, `unused-baseline-entry`, `unused-job-output`, `unused-needs`, `unused-workflow-input`, `workflow-call-permissions`, `workflow-input-type`, `workflow-run-names`, `workflow-syntax`.

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
.github/actions/example/action.yml:11:30: context "secrets" is not allowed in a composite action because secrets are not passed to it. declare an input and let the workflow pass the secret with "with:". available contexts are "env", "github", "inputs", "job", "matrix", "needs", "runner", "steps", "strategy", "vars". see https://docs.github.com/en/actions/learn-github-actions/contexts#context-availability for more details [expression]
   |
11 |     - run: ./publish.sh "${{ secrets.PUBLISH_TOKEN }}"
   |                              ^~~~~~~~~~~~~~~~~~~~~
.github/actions/example/action.yml:14:28: "inputs.tokan" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
14 |     - run: ./build.sh "${{ inputs.tokan }}"
   |                            ^~~~~~~~~~~~
.github/actions/example/action.yml:14:28: property "tokan" is not defined in object type {token: string} [expression]
   |
14 |     - run: ./build.sh "${{ inputs.tokan }}"
   |                            ^~~~~~~~~~~~
```

<!-- Skip playground link -->

<a id="check-composite-action-syntax"></a>
## Composite action syntax

The syntax of the metadata file is checked like the syntax of workflows: unknown keys (with a suggestion), duplicate keys, missing
`runs.using`, `runs.main` of a JavaScript action, `runs.image` of a Docker action and `runs.steps` of a composite action, keys which
belong to another kind of action (`steps` in a `node24` action), values of the wrong type, and `run:` steps of a composite action
without `shell:`. They are reported with the rule ID `action-syntax`. `runs.using` accepts `composite`, `docker` and `node` followed by
a version number.

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
.github/actions/example/action.yml:6:5: unexpected key "descriptions" for input "who". expected one of "default", "deprecationMessage", "description", "required" [syntax-check]
  |
6 |     descriptions: Who to greet
  |     ^~~~~~~~~~~~~
.github/actions/example/action.yml:11:7: "shell" is required for a "run" step of a composite action, which has no default shell. for example, add "shell: bash" [syntax-check]
   |
11 |     - run: echo "hello ${{ inputs.who }}"
   |       ^~~~
.github/actions/example/action.yml:11:28: "inputs.who" is an input chosen by whoever runs this, which is not validated and can hold shell syntax. avoid using it directly in inline scripts. instead, pass it through an environment variable. see https://docs.github.com/en/actions/reference/security/secure-use#good-practices-for-mitigating-script-injection-attacks for more details [expression]
   |
11 |     - run: echo "hello ${{ inputs.who }}"
   |                            ^~~~~~~~~~
.github/actions/example/action.yml:14:7: unexpected key "shell" for step to execute action. expected one of "background", "continue-on-error", "env", "id", "if", "name", "timeout-minutes", "uses", "with" [syntax-check]
   |
14 |       shell: bash
   |       ^~~~~~
.github/actions/example/action.yml:16:3: "main" is not available in "runs" section of the composite action. it is for JavaScript and Docker actions [syntax-check]
   |
16 |   main: dist/index.js
   |   ^~~~~
```

<!-- Skip playground link -->

[credentials-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#jobsjob_idcontainercredentials
[actions-cache]: https://github.com/actions/cache
[permissions-doc]: https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions
[perm-config-doc]: https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions#permissions
[generate-webhook-events]: https://github.com/jdx/jactionlint/tree/main/scripts/generate-webhook-events
[generate-popular-actions]: https://github.com/jdx/jactionlint/tree/main/scripts/generate-popular-actions
[issue-25]: https://github.com/jdx/jactionlint/issues/25
[issue-40]: https://github.com/jdx/jactionlint/issues/40
[security-doc]: https://docs.github.com/en/actions/reference/security/secure-use
[reusable-workflow-doc]: https://docs.github.com/en/actions/learn-github-actions/reusing-workflows
[create-reusable-workflow-doc]: https://docs.github.com/en/actions/learn-github-actions/reusing-workflows#creating-a-reusable-workflow
[reusable-workflow-call-keys]: https://docs.github.com/en/actions/learn-github-actions/reusing-workflows#supported-keywords-for-jobs-that-call-a-reusable-workflow
[object-filter-syntax]: https://docs.github.com/en/actions/learn-github-actions/expressions#object-filters
[github-script]: https://github.com/actions/github-script
[workflow-dispatch-event]: https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_dispatch
[workflow-dispatch-input-type-announce]: https://github.blog/changelog/2021-11-10-github-actions-input-types-for-manual-workflows/
[reusable-workflow-outputs]: https://docs.github.com/en/actions/using-workflows/reusing-workflows#using-outputs-from-a-reusable-workflow
[inherit-secrets-announce]: https://github.blog/changelog/2022-05-03-github-actions-simplify-using-secrets-with-reusable-workflows/
[specific-paths-doc]: https://docs.github.com/en/actions/using-workflows/triggering-a-workflow#using-filters-to-target-specific-paths-for-pull-request-or-push-events
[availability-doc]: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/accessing-contextual-information-about-workflow-runs#context-availability
[deprecate-set-output-save-state]: https://github.blog/changelog/2022-10-11-github-actions-deprecating-save-state-and-set-output-commands/
[deprecate-set-env-add-path]: https://github.blog/changelog/2020-10-01-github-actions-deprecating-set-env-and-add-path-commands/
[workflow-commands-doc]: https://docs.github.com/en/actions/using-workflows/workflow-commands-for-github-actions
[action-metadata-doc]: https://docs.github.com/en/actions/creating-actions/metadata-syntax-for-github-actions
[branding-icons-doc]: https://github.com/github/docs/blob/main/content/actions/creating-actions/metadata-syntax-for-github-actions.md#exhaustive-list-of-all-currently-supported-icons
[operators-doc]: https://docs.github.com/en/actions/learn-github-actions/expressions#operators
[dep-msg]: https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax#inputsinput_iddeprecationmessage
[anochor-support-announce]: https://github.blog/changelog/2025-09-18-actions-yaml-anchors-and-non-public-workflow-templates/
[yaml-anchor-spec]: https://yaml.org/spec/1.2.2/#71-alias-nodes
[dependabot-options-doc]: https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference
[hk]: https://hk.jdx.dev
