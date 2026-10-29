# Run jactionlint in CI

Run jactionlint in GitHub Actions with the action, mise, Docker or the download script, and show its findings as annotations or code scanning alerts. Each method comes with a workflow that has no finding under the default profile once you replace the placeholders.

<a id="use-jactionlint-on-github-actions"></a>
<a id="on-github-actions"></a>
## Choose a method

Every method runs the same jactionlint. They differ in how you pin the version, whether the download is verified and what else you get. Pick the row that fits your project.

| Method | Pin the version with | The download is verified by | Use it when |
| --- | --- | --- | --- |
| [The action](#the-action) | The commit of `jdx/jactionlint`. The action installs the release it was published with, unless you set `version`. | Nothing: it runs the [download script](#download-script), which does not verify the archive. | You want the shortest workflow, annotations or code scanning alerts. |
| [mise](#mise) | `mise.toml` and `mise.lock`. | The checksum and the provenance that `mise.lock` records. | The project already uses mise, or developers and CI must run the same verified version. |
| [Docker](#docker) | The tag of the image, or better its digest. | The digest, when you pin it. | You want shellcheck and pyflakes without installing them. |
| [Download script](#download-script) | The version argument of the script. | Nothing: the script does not verify the archive. | A runner has bash and curl and nothing else. |

Whatever the method, the step fails when jactionlint exits with a nonzero status. Findings of level `warn` and `info` do not count unless you pass `--strict-exit`, and [Exit status](./cli.md#exit-status) lists every cause.

jactionlint checks the shell scripts of `run` steps with shellcheck when it finds one on `PATH`. The `ubuntu-24.04` runner image includes [shellcheck](https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md). On other runners, install it yourself, for example with mise. Without it jactionlint prints a note and skips the scripts, as [Companion programs](./install.md#companion-programs) describes.

<a id="action"></a>
## The action

`jdx/jactionlint` downloads jactionlint, runs it on the repository and annotates the files with the findings. It runs on Linux, macOS and Windows runners, and its install step needs bash and curl. Create `.github/workflows/jactionlint.yaml`:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: jdx/jactionlint@a65ab044e916760429068fe5794b8306abdfcbb9 # v2.3.0
```

The default profile asks for `permissions`, a timeout, a concurrency group and a pin by commit on every action, so the workflow has all four. Pin `jdx/jactionlint` the same way, with the release in a comment. A release has only its exact tag, so there is no moving tag such as `v2` to follow. The pin also pins jactionlint, because the `version` input defaults to the release that the action was published with.

The commit in the example belongs to one release and does not move. Replace it with the commit of the newest release: `gh api repos/jdx/jactionlint/git/ref/tags/vX.Y.Z --jq .object.sha` prints it, with the tag of that release in place of `vX.Y.Z`.

Without `files`, the action checks the repository the way `jactionlint` does without operands: the workflow files, the composite actions, the Dependabot configuration and the Actions lock file, as [Repository mode](./usage.md#repository-mode) lists.

### Inputs

Every input is optional. Set them under `with:` of the `jdx/jactionlint` step.

| Input | Default | Effect |
| --- | --- | --- |
| `version` | The release the action was published with | The exact `X.Y.Z` version of jactionlint to download. It is ignored when `executable` is set. |
| `executable` | empty | The path of a jactionlint executable to use instead of downloading one, for example one that mise installed or an earlier step built. |
| `files` | empty | The files to check, separated by white space. They are not expanded: there is no globbing and there is no quote removal. |
| `profile` | empty | `correctness`, `default` or `pedantic`. Without it the `profile` of the config file applies, or `default`. |
| `config-file` | empty | The path of the config file, when it is not `.github/jactionlint.yaml`. |
| `min-severity` | empty | Hide findings less severe than this level: `info`, `warn` or `error`. |
| `strict-exit` | `false` | Fail for findings of level `warn` and `info` too, not only for level `error`. |
| `online` | empty | `true` runs the online checks and `false` never uses the network. `cache`, `strict` and `cache,strict` are the modes of `--online`. Empty leaves it to the config file. |
| `token` | `${{ github.token }}` | The GitHub API token of the online checks. The action does not pass it to jactionlint when `online` is `false`. |
| `annotations` | `true` | Annotate the files with the findings (`--format github`). It has no effect when `advanced-security` is `true`. |
| `advanced-security` | `false` | Upload the findings to code scanning as SARIF. The job needs `security-events: write`. It takes precedence over `annotations`. |
| `color` | `true` | Colorize the text output. It applies only when `annotations` and `advanced-security` are both `false`. |
| `args` | empty | Further options, for example `--ignore=unpinned-images`. The action splits the value at white space and does not remove quotes, so a value cannot contain a space and quotes stay literal. |
| `working-directory` | `.` | The directory to run jactionlint in. `files` and `config-file` are relative to it. |

For example, to check two files with the `correctness` profile and to fail for findings of every level, add these inputs:

```yaml
        with:
          files: .github/workflows/ci.yaml .github/workflows/release.yaml
          profile: correctness
          strict-exit: true
```

### Outputs

| Output | Value |
| --- | --- |
| `executable` | The path of the jactionlint executable that the action used: the `executable` input, or the one that it installed. |
| `sarif-file` | The path of the SARIF file. It is set only when `advanced-security` is `true` and jactionlint exits with status 0 or 1. |

### Online checks in the action

The online checks ask the GitHub API about the actions that a workflow uses. [Online checks](./online.md#online-checks) describes what they check and what happens when GitHub does not answer. In the action, turn them on with the `online` input:

```yaml
        with:
          online: true
```

The token is `github.token` unless you set `token`. The action passes it to jactionlint whenever `online` is not `false`, so a config file that enables the online checks needs no `online` input. The action passes the `online` input as `--online`, `--no-online` or `--online=MODE`, so use the input and not a `JACTIONLINT_ONLINE` variable that you set on the job. The input also takes the modes of `--online`, see [Turn on the online checks](./online.md#turn-on-the-online-checks), and [Cache the answers in CI](./online.md#cache-the-answers-in-ci) shows how to keep the cache between runs.

## mise

[mise](https://mise.jdx.dev) installs the version of jactionlint that your project records, so developers and CI run the same one. Record it once and commit the files:

```sh
mise use jactionlint
mise lock
```

`mise use jactionlint` adds the tool to `mise.toml`. `mise lock` writes `mise.lock` with the resolved version, a checksum for each platform and the provenance of the download. mise checks the checksum when it installs, so a `mise.lock` entry that does not match the archive fails the install. [Installation](./install.md#mise) shows how to pin an exact version in `mise.toml`.

The workflow installs the tools of `mise.toml` with [`jdx/mise-action`](https://github.com/jdx/mise-action) and runs jactionlint:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
      - run: jactionlint --format github
```

`--format github` annotates the files with the findings. Without it, jactionlint prints the text output, and [Problem matchers](#problem-matchers) can turn that into annotations. To check shell scripts on a runner without shellcheck, add it to the project with `mise use shellcheck`.

<a id="docker-image"></a>
## Docker

[Docker image](./install.md#docker-image) lists the contents, the user and the tags of the image `ghcr.io/jdx/jactionlint`. Two kinds of tag exist:

- `ghcr.io/jdx/jactionlint:latest`: the newest release, not the `main` branch. An option that only `main` has fails with `unknown option` in this image.
- `ghcr.io/jdx/jactionlint:2.4.0`: a specific release. <!-- x-release-please-version -->

### Run the image

To check a repository, mount its root directory and run in it:

```sh
docker run --rm --user "$(id -u):$(id -g)" --volume "$(pwd):/repo" --workdir /repo ghcr.io/jdx/jactionlint:latest
```

`--user` runs the container as you. Without it, the user `guest` of the image cannot read a checkout that only you can read, and it cannot write to files that belong to you: `--fix`, `--baseline-write` and `--init-config` then fail with a permission error and exit status 3.

To check one file, pass its content on stdin and give `-` as the file:

```sh
cat .github/workflows/ci.yaml | docker run --rm --interactive ghcr.io/jdx/jactionlint:latest -
```

The image has no Node.js, so the rule `github-script-syntax` is skipped with a note. It has no `gh` either. To run the online checks, pass the token in the environment and add `--online`:

```sh
docker run --rm --env GITHUB_TOKEN --user "$(id -u):$(id -g)" --volume "$(pwd):/repo" --workdir /repo ghcr.io/jdx/jactionlint:latest --online
```

### Run the image in a workflow

The default profile reports a tag in `docker run` with the rule `unpinned-images`, so the workflow names the image by its digest:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - name: Check the workflows
        run: >-
          docker run --rm --user "$(id -u):$(id -g)" --volume "${PWD}:/repo" --workdir /repo
          ghcr.io/jdx/jactionlint@sha256:<digest> --format github
```

Replace `<digest>` with the digest of the image that you tested: the rule `unpinned-images` reports a value that is not a valid digest. `docker buildx imagetools inspect ghcr.io/jdx/jactionlint:<tag>` prints it on the `Digest` line at the top, which is the digest of the multi-platform image.

A step can run the image as a container action instead of calling `docker run`. The rule `unpinned-uses` asks for a digest here too:

```yaml
      - uses: docker://ghcr.io/jdx/jactionlint@sha256:<digest>
        with:
          args: --format github
```

## Download script

Use the download script on a runner that has bash and curl and nothing else. The script downloads the release archive, extracts the executable and, inside GitHub Actions, writes the path of the executable to the step output `executable`. [Download script](./install.md#download-script) lists its arguments. Set `shell: bash` on the steps, because the default shell of Windows runners is `pwsh`:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - name: Download jactionlint
        id: get_jactionlint
        # jactionlint ignore=unverified-download
        run: bash <(curl -fsSL https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash)
        shell: bash
      - name: Check the workflows
        run: ${{ steps.get_jactionlint.outputs.executable }} --format github
        shell: bash
```

The rule `unverified-download` is in the default profile and reports a script that a shell runs without verification, so the step carries an ignore comment. A URL that names a commit passes without a comment, as [Download script](./install.md#download-script) shows. Neither form verifies the archive that the script downloads: use [mise](#mise) or the [Docker image](#docker) pinned by its digest when that matters, or verify a release archive as [Verify the download](./install.md#verify-the-download) describes.

## Annotations and code scanning

jactionlint can show its findings in GitHub in these ways. They differ in where the findings appear and in what the job needs.

| Way | How | Where the findings appear | The job needs |
| --- | --- | --- | --- |
| Annotations | `--format github`, the default of the action | On the files of the run and the pull request, one annotation for each finding, within the [limits that GitHub sets](https://github.com/actions/toolkit/blob/main/docs/problem-matchers.md#limitations) for a step and for a job. | Nothing beyond `contents: read`. |
| Code scanning | `--format sarif`, uploaded to GitHub | As code scanning alerts of the repository. | `security-events: write`. |
| Problem matchers | The text output, read by a matcher file | As the same annotations, from a step that does not use `--format github`. | A copy of the matcher file. |

`--format github` prints one workflow command for each finding: `::error` for level `error`, `::warning` for `warn` and `::notice` for `info`. [The `github` format](./output.md#github) shows the output and the escaping.

### Upload to code scanning with the action

Set `advanced-security: true` and give the job `security-events: write`. The action runs jactionlint with `--format=sarif`, so `annotations` and `color` have no effect, and then uploads the log with `github/codeql-action/upload-sarif` in the category `jactionlint`. Change the permissions and the inputs of the example in [The action](#the-action) to:

```yaml
    permissions:
      contents: read
      security-events: write
```

```yaml
        with:
          advanced-security: true
```

The upload happens even when jactionlint reports findings, and the job fails after it. The exit status of jactionlint decides what the action does:

| jactionlint exits with | Upload | `sarif-file` | Job |
| --- | --- | --- | --- |
| 0 | The log, which has no finding that fails the run | The path of the log | Passes |
| 1 | The log, which has findings that fail the run | The path of the log | Fails after the upload |
| 2 or 3 | None, the action removes the file | Empty | Fails with that status |

### Upload to code scanning without the action

Run jactionlint with `--format sarif` and upload the log with `github/codeql-action/upload-sarif`. Status 1 means findings and must not skip the upload, so the command accepts status 1 and fails for statuses 2 and 3. The workflow installs jactionlint with [mise](#mise), so commit a `mise.toml` that lists it, or replace the install step with another method:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
      - name: Write the SARIF log
        run: jactionlint --format sarif > jactionlint.sarif || [ "$?" -eq 1 ]
      - uses: github/codeql-action/upload-sarif@2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2 # v4.38.2
        with:
          sarif_file: jactionlint.sarif
          category: jactionlint
```

This job passes when jactionlint finds something, and the code scanning alerts carry the findings. [Output formats](./output.md#format-error-messages) describes the SARIF log.

### Problem matchers

A [problem matcher](https://github.com/actions/toolkit/blob/master/docs/problem-matchers.md) is a regular expression that GitHub applies to the output of a step to create annotations. Use it when a step runs jactionlint without `--format github`. The action and `--format github` do not need it.

Copy [`jactionlint-matcher.json`](https://raw.githubusercontent.com/jdx/jactionlint/main/.github/jactionlint-matcher.json) to `.github/jactionlint-matcher.json` in your repository. Then register the matcher with `::add-matcher::` before the step runs jactionlint. The workflow installs jactionlint with [mise](#mise), so commit a `mise.toml` that lists it, or replace the install step with another method:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  push:
    branches: [main]
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: jdx/mise-action@c2a87611a18de5b3828c5652fe268e992400cb5c # v4.3.0
      - name: Check the workflows
        run: |
          echo "::add-matcher::.github/jactionlint-matcher.json"
          jactionlint
```

The matcher reads the first line of each finding in the default text output, with or without color, and in the `oneline` and `gcc` formats. It does not read `--format github`, whose lines are annotations already. Copy the file again when the text output changes.

## Show only what a pull request changed

`--changed-since REV` shows only the findings on the lines that changed since `REV`, so that a pull request check reports what the pull request adds and not what the repository already has. It filters what is shown: jactionlint still checks every file and reads the whole call graph of the repository, so each finding that it shows is one that a full run reports.

The workflow below checks out the full history, because the option compares with the merge base of the revision and `HEAD`, and passes the option to the action:

```yaml
# .github/workflows/jactionlint.yaml
name: jactionlint
on:
  pull_request:
permissions: {}
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: jdx/jactionlint@a65ab044e916760429068fe5794b8306abdfcbb9 # v2.3.0
        with:
          args: --changed-since=origin/main
```

A release that does not have the option fails with `unknown option`, and `jactionlint --help` shows what the installed release supports. The action installs the release that it was published with: set its `version` input to a release that has the option, or its `executable` input to a build.

Run the option on your machine to see the effect. In this example, the workflow file is committed on `main` without the job `lint`, and the branch `feature` adds that job. The job `test` already has no `timeout-minutes`:

```yaml
name: CI
on:
  pull_request:
permissions:
  contents: read
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        with:
          persist-credentials: false
      - run: echo testing
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false
      - run: echo linting
```

```sh
jactionlint --changed-since main
```

```
.github/workflows/ci.yaml:17:3: "timeout-minutes" is not set at this job. Set it to avoid wasting runner minutes when the job hangs. Set "default-minutes" in the "missing-timeout" rule options of the configuration to turn on the --fix fix [missing-timeout]
   |
17 |   lint:
   |   ^~~~~
.github/workflows/ci.yaml:20:15: action "actions/checkout@v4" must be pinned to a full-length commit SHA like "{owner}/{repo}@{sha}" because the "unpinned-uses" rule is enabled [unpinned-uses]
   |
20 |       - uses: actions/checkout@v4
   |               ^~~~~~~~~~~~~~~~~~~
note: 1 finding(s) outside the changed lines since main are not shown (--changed-since)
```

The note on stderr counts the finding of the job `test`, which a full run reports. `--changed-scope files` shows every finding of a file that changed, for a team that wants the whole file clean once it touches it. The default is `lines`.

How the option behaves:

- **What changed** is what `git` reports between the merge base of `REV` and `HEAD` and the working tree: the commits of the branch, the staged and unstaged changes of tracked files, and every line of a file that is not tracked and not ignored. A moved file counts as new. A block that was only deleted marks the lines on both sides of the place, because removing a `permissions:` or a guard can cause a finding next to it.
- **The baseline and the exit status.** The baseline applies first, so the two combine: the baseline hides the findings that you accepted, and `--changed-since` hides the ones in code that you did not touch. The exit status is that of the findings that are shown, so a run whose findings are all hidden exits with status 0.
- **A finding somewhere else is not shown there.** When a pull request changes the events of a caller so that an unchanged line of a reusable workflow becomes a finding, that finding is at a line and in a file that did not change. Run the full check on the default branch too.
- **It fails when it cannot compare.** An unknown revision, a shallow clone that lacks the merge base, or a missing `git` exits with status 3 and says why, and never prints an empty report that looks clean. Give `actions/checkout` `fetch-depth: 0`, as the workflow above does.
- **Some options cannot be combined with it.** `--fix`, `--diff`, `--baseline-write`, `--migrate-ignores`, `--init-config`, `--migrate-config`, `--export-facts`, `--suggest-timeouts` and stdin exit with status 2, and so does `--changed-scope` without `--changed-since`. [Option combinations that fail](./cli.md#option-combinations-that-fail) lists the messages.
- **What it prints.** The `text` and `oneline` formats print the note to stderr. The SARIF log has `properties.changedSince` with `since`, `scope` and `hidden`, so that a consumer can tell it from a full log. `json` and `jsonl` have no place for run data and print no note. Without the option nothing changes in any format.

## Next steps

- [Run jactionlint from Git hooks](./hooks.md#pre-commit) to check before a commit or a push.
- [Run the online checks](./online.md#online-checks) with a token, and learn what a skipped lookup means.
- [Adopt jactionlint with a baseline](./baseline.md#baseline) when a repository has findings you cannot fix at once.
- [Choose a profile](./config.md#profiles) to make the checks stricter or looser.
- [Read the exit status](./cli.md#exit-status) that decides whether a step fails.
