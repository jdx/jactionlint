# Online checks

Turn on the online checks that ask GitHub about your actions, and learn what they send, what they cost and what happens when GitHub does not answer. They are off by default: without them jactionlint never uses the network.

## Quick start

The steps below show the online checks on one workflow. A token is optional but raises the rate limit.

1. Give jactionlint a token. Without one GitHub allows 60 requests an hour. jactionlint reads `GITHUB_TOKEN` or `GH_TOKEN` and, when `gh` is installed, asks `gh auth token`, so this step is only needed when neither is available.

   ```sh
   export GITHUB_TOKEN=$(gh auth token)
   ```

2. Save this workflow as `.github/workflows/ci.yaml` in a Git repository. It has no finding without `--online`, but the version comment names a release that the pinned commit is not.

   ```yaml
   name: ci
   on: [push]
   permissions: {}
   concurrency:
     group: ci-${{ github.ref }}
     cancel-in-progress: true
   jobs:
     build:
       runs-on: ubuntu-latest
       timeout-minutes: 10
       steps:
         - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.1.0
           with:
             persist-credentials: false
         - run: echo ok
   ```

3. Run the checks.

   ```sh
   jactionlint --online
   ```

   The default profile gives this output:

   ```
   .github/workflows/ci.yaml:12:15: warning: the version comment "# v4.1.0" does not match the commit pinned in action "actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683": tag "v4.1.0" points to commit 8ade135a41bc, but the pinned commit is tagged "v4.2.2". either the comment is stale or the SHA is wrong, so check which one is meant: update the comment, or pin the commit of the version you mean [ref-version-mismatch]
      |
   12 |       - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.1.0
      |               ^~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
   ```

   The rule `ref-version-mismatch` compared the comment with the tags of `actions/checkout`. The finding has level `warn`, so the exit status stays 0.

4. Check what the run did. `--verbose` prints which source supplied the token (`verbose: online: using the token from $GITHUB_TOKEN for api.github.com`, never the token itself) and one line for every skipped lookup. `--online-report=FILE` writes a summary of the run, see [Prove that the checks ran](#prove-that-the-checks-ran).

To keep the checks on, set `online: true` in the [config file](./config.md#online-options) or `JACTIONLINT_ONLINE=1` in your CI job, as [the next section](#turn-on-the-online-checks) explains.

## Turn on the online checks

The command line, the environment, the config file and the action turn the checks on or off. The command line wins over the environment, and the environment wins over the config file. The action turns its input into command-line options.

| Where | Turn on | Turn off |
| --- | --- | --- |
| Command line | `--online`, `--online=MODE` | `--no-online` |
| Environment | `JACTIONLINT_ONLINE=1`, `true`, `on` or a mode | `JACTIONLINT_ONLINE=0`, `false`, `off` or `no` |
| Config file | `online: true`, or a `mode` under `online-options` | `online: false` |
| [The action](./ci.md#the-action) | the input `online: true`, or a mode | the input `online: false` |

The rules of precedence:

- `--online` and `--no-online` exit with status 2 when you give both.
- `JACTIONLINT_ONLINE` is read only when neither flag is given. An empty variable counts as unset, the values are case-insensitive, and any other value exits with status 2.
- `--no-online` and `JACTIONLINT_ONLINE=0` turn the checks off even when the config file turns them on.
- A `mode` under `online-options` turns the checks on by itself. An explicit `online: false` in the same file wins over it, and `--online` still turns them on.
- A bare `--online` replaces the `mode` of the config file with the plain online mode. `--online=MODE` replaces it with `MODE`.

A mode says how the run uses the network and what it does with a lookup that could not be answered:

| Mode | Network | A skipped lookup |
| --- | --- | --- |
| none (plain) | Asks GitHub, reads the cache first | One warning per kind of failure. The exit status does not change. |
| `strict` | Like plain | Exit status 3 |
| `cache` | Never. Answers come from the disk cache whatever their age. | One warning for the lookups that have no cached answer. The exit status does not change. |
| `cache,strict` | Never | Exit status 3 |

Write `--online=cache,strict` (or `--online=strict,cache`) to combine the two modes. A comma, a `+` or a space separates the words. The same words work in `JACTIONLINT_ONLINE` and in the `online` input of the action. `online-options.mode` takes exactly `cache`, `strict` or `cache,strict`, in this spelling.

To use the checks in CI only, set the variable in the CI job: a hook on a laptop does not have it, so it stays offline. The runner does not export the job token by itself, so hand it over:

```yaml
- run: jactionlint
  env:
    JACTIONLINT_ONLINE: 1
    GITHUB_TOKEN: ${{ github.token }}
```

In the action, use the `online` input and not a `JACTIONLINT_ONLINE` variable on the job: the action passes the input as `--online`, `--no-online` or `--online=MODE` and hands over its `token` input itself. [Online checks in the action](./ci.md#online-checks-in-the-action) has the details.

To run the online checks from a Git hook, see [Add the online checks](./hooks.md#add-the-online-checks).

## What the online rules check

The online rules run only with the online checks, whatever the profile: they belong to no profile, and `--profile correctness --online` runs them too. Each rule has its own default level, and `rules` sets another level or `off` as for any rule. `jactionlint --list-rules` is the list of these rules: it shows `online` in the `PROFILE` column, and `--list-rules --format json` marks them with `"online": true`. The [rules page](./rules.md) shows `only with --online` in the Profile column.

| Rule | Default level | Reports | Asks GitHub for |
| --- | --- | --- | --- |
| [`actions-lock-drift`](./checks.md#check-actions-lock) | error | A pin of the Actions lock file that no longer matches GitHub: the repository was replaced or changed owner, a version tag moved, or the commit is not in the repository's history | The repository, the tag and the commit's history |
| [`archived-uses`](./checks.md#check-archived-uses) | warn | An action or reusable workflow in an archived repository | The repository |
| [`impostor-commit`](./checks.md#check-impostor-commit) | error | A hash-pinned action whose commit is on no branch or tag of its repository, so it can come from a fork | The tags and branches, compared with the commit |
| [`known-vulnerable-actions`](./checks.md#check-known-vulnerable-actions) | error | An action version covered by a GitHub security advisory | The advisories and the tags of the action |
| [`ref-confusion`](./checks.md#check-ref-confusion) | warn | A ref that is both a branch and a tag | The tag and the branch of that name |
| [`ref-version-mismatch`](./checks.md#check-ref-version-mismatch) | warn | A `# v1.2.3` comment that does not match the pinned commit | The tags of the repository |
| [`repojacking`](./checks.md#check-repojacking) | warn | An action or reusable workflow whose owner does not exist, or whose repository moved to another owner and left the old name free, so anyone can register the name | The repository and its owner |
| [`stale-action-refs`](./checks.md#check-stale-action-refs) | info | A pinned commit that no tag points to | The tags of the repository |
| [`unpinnable-action`](./checks.md#check-unpinnable-action) | warn | A hash-pinned action that itself pulls in an action or a Docker image by a tag or a branch. It runs only with the rule option `pedantic`, which is on under `profile: pedantic`. | The `action.yml` of the pinned action at its commit, and of the commit-pinned actions that its steps use |

The levels are the defaults. A rule whose lookup fails reports nothing for that action, see [When GitHub does not answer](#when-github-does-not-answer).

Other rules run without `--online` and gain coverage with it:

| Rule | What `--online` adds |
| --- | --- |
| `missing-action-input`, `unknown-action-input`, `deprecated-action-input` | The inputs of every action that is not in the built-in data set are checked against its `action.yml` at the pinned ref, see [inputs and outputs of any action](./checks.md#check-remote-action-metadata). |
| `undefined-property` | `steps.<id>.outputs.<name>` of such a composite action is checked against its declared outputs. `needs.<id>.outputs.<name>` of a job that calls a reusable workflow of another repository is checked against the outputs it declares. |
| `missing-workflow-input`, `unknown-workflow-input`, `workflow-input-type`, `missing-workflow-secret`, `unknown-workflow-secret` | A call of a reusable workflow of another repository (`owner/repo/.github/workflows/x.yml@ref`) is checked against the workflow file at that ref, as a call of a local workflow is, see [reusable workflows of other repositories](./checks.md#check-remote-reusable-workflows). |
| `unpinned-uses` | With the policy `immutable-release`, the tag of an immutable GitHub release is accepted like a commit SHA, see [tags of immutable releases](./checks.md#check-immutable-release). |
| `unpinned-images`, `unpinned-uses` | The digest of an image tag is looked up so that the finding carries a fix, see [pin container images](#pin-container-images-to-digests-with-online-fix-unsafe). |
| `untrusted-checkout`, `untrusted-artifact`, `agentic-actions` | The `environment:` of a job counts as a gate only when it has required reviewers. See below. |

The rules `untrusted-checkout`, `untrusted-artifact` and `agentic-actions` trust the `environment:` of a job ("its reviewers decide"). With the online checks they look the environment up with `GET /repos/{owner}/{repo}/environments/{name}`. The repository comes from `GITHUB_REPOSITORY`, or else from the `origin` remote. They report the job when the environment has no required reviewers or does not exist, see [environments without reviewers](./checks.md#check-untrusted-checkout-environment).

A lookup that fails leaves the environment trusted and counts as skipped. In a private repository the token needs read access to environments (the `actions: read` permission of a job token). Without it GitHub refuses the request with a 403 or a 404, so check `--verbose` when an expected finding is missing.

## Pin with the online fixes

With `--online`, `--fix` can write the pins that `unpinned-uses` and `unpinned-images` ask for. The list of every fixable rule and the meaning of safe and unsafe fixes are on [the fix page](./fix.md#fix-errors-automatically).

### Pin tags to commits with `--online --fix`

`--fix` replaces the tag of an action or a reusable workflow with the commit it points to and names the version in a comment. A moving tag such as `v4` becomes `@<commit> # v4.x.y`. This example uses the exact tag `v4.2.2`, which does not move:

```yaml
name: ci
on: [push]
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4.2.2
        with:
          persist-credentials: false
```

```sh
jactionlint --online --diff
```

```
--- a/.github/workflows/ci.yaml
+++ b/.github/workflows/ci.yaml
@@ -9,6 +9,6 @@
     runs-on: ubuntu-latest
     timeout-minutes: 10
     steps:
-      - uses: actions/checkout@v4.2.2
+      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
         with:
           persist-credentials: false
Would fix 1 problem(s) in 1 file(s)
  unpinned-uses: 1
```

Replace `--diff` with `--fix` to write the change. Plain `--fix` applies it, because the fix is safe: the step runs the commit that the tag names now. The fix is offered only when the ref is the name of a tag (annotated tags are followed to the commit) and no branch has the same name. A branch, an unknown tag, an abbreviated SHA, a Docker image, or a line whose comment does not name the ref is left to you. So is an action in a workflow that the [Actions lock file](./checks.md#check-actions-lock) lists, because a SHA is another pin than the one of the lock file.

The comment names the most specific version tag that points to the commit (`# v4.2.2`, not `# v4`), because the moving tag `v4` points somewhere else after the next release. The tag chosen is always a refinement of the one written: `v4` can become `v4.2.2`, never `v5.0.0`. A tag that is not a version (`release`), and a commit with two versions of the same specificity (`v2.0.1` and `v2.0.2`), are named as written. A comment that is already there is not rewritten, and the tag is not refined when the repository has more tags than jactionlint read.

To keep the tag as written (`# v4`), set `short-pin-comment`:

```yaml
rules:
  unpinned-uses:
    short-pin-comment: true
```

`ref-version-mismatch` has an unsafe fix that rewrites the version in the comment to the tag of the pinned commit. Plain `--fix` leaves it, because a mismatch can also mean that the SHA is the wrong part, see [the check](./checks.md#check-ref-version-mismatch). Apply it after you have read the findings:

```sh
jactionlint --online --fix=unsafe
```

### Pin container images to digests with `--online --fix=unsafe`

The images of `container:`, `services:`, `uses: docker://` and `runs.image: docker://` (the findings of `unpinned-images` and of `unpinned-uses`) get the digest that their tag has in the registry. This workflow has one unpinned image:

```yaml
name: ci
on: [push]
permissions: {}
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    container: postgres:16
    steps:
      - run: pg_isready
```

Plain `--online --diff` reports the finding and changes nothing. The fix is unsafe, so it needs `--fix=unsafe`: a tag moves, and the digest pins the image that the tag names on the day you run it. The digest in the diff below is the one that `postgres:16` had when this example was run: your run prints the current one.

```sh
jactionlint --online --diff --fix=unsafe
```

```
--- a/.github/workflows/ci.yaml
+++ b/.github/workflows/ci.yaml
@@ -8,6 +8,6 @@
   build:
     runs-on: ubuntu-latest
     timeout-minutes: 10
-    container: postgres:16
+    container: postgres:16@sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d
     steps:
       - run: pg_isready
Would fix 1 problem(s) in 1 file(s)
  unpinned-images: 1
```

Docker Hub and ghcr.io are asked. Another registry is not: the finding then has no fix, and its `no_fix` reason in the JSON and SARIF formats says so, see [why a finding was not fixed](./fix.md#why-a-finding-was-not-fixed). Every image and tag is asked once per run, with a timeout, and `--online-allow` and `--online-deny` apply (an image is matched as `owner/image`). The lookup happens in every `--online` run that meets an unpinned image, because the fix is part of the finding, and a failed lookup counts as skipped. See [the check](./checks.md#check-unpinned-images).

## Token and privacy

The online checks send the names of what your workflows use to a few hosts, and a token to one of them. This section lists the hosts, the order in which jactionlint looks for a token and the cases in which it refuses to send one.

### Hosts and data

A request names a repository, a ref, a commit or a file path of something that one of your workflows uses: an action, a reusable workflow, an image or an environment. jactionlint sends no workflow content. Every request is a `GET` or `HEAD`, except the GraphQL query that `impostor-commit` uses to compare branches. The requests to GitHub carry the user agent `jactionlint/` and the version.

| Host | Asked for | Token |
| --- | --- | --- |
| The API host: `api.github.com`, or your GitHub Enterprise Server | Repositories, tags, branches, commits, advisories, files such as `action.yml` and reusable workflows, owners, releases and environments, over REST and GraphQL | Yes |
| `api.github.com`, with the [github.com fallback](#github-enterprise-server) of a Server | The repositories that the Server does not have, and the advisories | Its own token (`GITHUB_COM_TOKEN`), never the one of the Server |
| `auth.docker.io` and `registry-1.docker.io` (Docker Hub) | The digest of an image tag | No |
| `ghcr.io` | The digest of an image tag | Only when ghcr.io refuses the anonymous request, and then only the token for `api.github.com` |

With `--online=cache` no host is asked.

### Find the token

jactionlint uses the first of these:

1. The variable named by `--online-token-env`.
2. The file named by `--online-token-file`. It may hold at most 4096 bytes and must hold exactly one token.
3. `GITHUB_TOKEN`, then `GH_TOKEN`. For a GitHub Enterprise Server, `GITHUB_ENTERPRISE_TOKEN` and `GH_ENTERPRISE_TOKEN` come first.
4. The output of `gh auth token --hostname HOST`, when `gh` is installed. jactionlint gives it five seconds. Set `gh-cli: false` under [`online-options`](./config.md#online-options) to never run it.

Without a token the requests are unauthenticated, which works but is limited to 60 an hour, and jactionlint says so once. In the Docker image `gh` is not installed: pass the token with `-e GITHUB_TOKEN`, see [Docker](./ci.md#docker).

A token that GitHub rejects (401) is dropped with a warning, and the run goes on without it. A token that cannot read a repository (403) is tried again without it for that repository.

### Where the token goes

The token goes to the API host only, over https. Plain http is accepted only for `localhost` or a loopback address such as `127.0.0.1`. For any other http host jactionlint prints a notice and sends no token. A redirect of a request that carries the token to another host is refused. The token is replaced by `[redacted]` in `--debug` output, warnings and errors, even when a server echoes it.

The one exception is ghcr.io, the registry of github.com. It gets the token for `api.github.com` only after it refused an anonymous request for an image, and never the token of a Server.

### A repository cannot choose the host

A pull request could otherwise make jactionlint send your token to any server, so a host named by the `online-options.api-url` of the `.github/jactionlint.yaml` of a repository gets no token. jactionlint prints a warning and checks without one. To use such a host, name it yourself: with `--online-api-url`, in your user-global config file, or in a config file that you pass with `--config-file`. The environment variable `GITHUB_API_URL` does not replace the `api-url` of a repository config file, only `--online-api-url` does.

## When GitHub does not answer

A lookup that fails is skipped. The checks that needed it report nothing for that action, every other lookup goes on, and the exit status does not change. jactionlint counts each skipped lookup and prints one warning for each kind of failure. `--verbose` lists every skipped lookup.

| Kind | Cause | What to do |
| --- | --- | --- |
| `not found` | GitHub answers 404: the repository is private, deleted or not visible to the token | Put private actions under `--online-deny`, or use a token that can read them. |
| `forbidden` | A 403 that is not a rate limit: the token cannot read the repository. jactionlint tries once without the token. | Use a token with access, or deny the repository. |
| `unauthorized` | A 401 that remains after the token was dropped | Fix the token, or unset it to check public repositories only. |
| `rate limit` | The quota is used up and resets later than `--online-max-wait` | Set a token, or wait for the reset time that the warning names. |
| `server error`, `timeout` | A 5xx answer, or no answer within 20 seconds, after the retries | Run again later. |
| `network` | The host does not resolve or refuses the connection | Check the network and `--online-api-url`, or use `--online=cache`. |
| `not cached` | `--online=cache` found nothing in the cache | Run once with `--online` to fill the cache. |
| `other` | Any other status | `--verbose` shows it. |

jactionlint stops asking when asking again cannot work, and counts the rest of the lookups as skipped without a request. This happens after:

- a rate limit, until it resets (cached answers are still used);
- a 401 that remains without the token;
- a DNS failure or a refused connection;
- three failed lookups in a row (timeouts, server errors or other failures). A 404 or a 403 for one repository never counts towards the three.

Ctrl-C stops the lookups and prints a warning that the findings which need GitHub are missing.

The warnings go to stderr as lines that start with `warning:`. With `--format sarif`, the warnings are not printed: they are in `runs[].invocations[].toolConfigurationNotifications` of the log on stdout, see [Streams and color](./output.md#streams-and-color).

### Fail when a lookup is skipped

`--online=strict` turns a skipped lookup into exit status 3, for a pipeline where a check that could not run must not pass silently. The status 3 replaces a status 1 when the run has both findings and skipped lookups. With no token and nothing listening on the API URL, the workflow of the quick start gives this on stderr. The command removes the token variables from the environment: the first line appears only when jactionlint finds no token.

```sh
env -u GITHUB_TOKEN -u GH_TOKEN jactionlint --online=strict --online-api-url=http://127.0.0.1:9
```

```
warning: no token found (GITHUB_TOKEN, GH_TOKEN, --online-token-file or gh auth login) so the online checks use unauthenticated requests, which GitHub limits to 60 per hour. set a token to raise the limit
warning: online: could not reach the GitHub API: Get "http://127.0.0.1:9/repos/actions/checkout": dial tcp 127.0.0.1:9: connect: connection refused. the lookup was skipped; --online=cache works from the cache without the network. --verbose lists every skipped lookup.
online=strict: 3 GitHub lookups were skipped, so the online checks are incomplete
```

`--online=cache,strict` fails the same way for a lookup that has no cached answer. The other exit statuses are in [the exit status table](./cli.md#exit-status).

### Skip private or internal actions

`--online-deny 'mycorp/*'` never asks about the matching repositories, and `--online-allow 'actions/*'` asks only about the matching ones. Both are repeatable, and `deny` and `allow` under `online-options` do the same. A pattern is `owner/repo` with `*` as the only wildcard, within one part, and case does not matter. `deny` wins over `allow`. These lookups are left out on purpose: they are neither failures nor warnings, and `excludedRepositories` of the report counts them.

A token reads only what its owner can read. An action in a private repository that the token cannot see answers 404 and is skipped, so list it under `deny` or use a token that can read it.

## Prove that the checks ran

A run without findings could mean "checked, nothing found" or "not checked". The coverage of the run tells them apart. `--format json` is a bare array and `jsonl` has one finding a line, so neither has a place for run-level data: `--online-report=FILE` writes the coverage to a file for any `--format`. With `--format sarif` the log carries the same object in `runs[0].properties.onlineCoverage`, which is absent when the online checks were off. The coverage is evidence about the run, not a trust guarantee: a resolved ref or a timestamp does not prove that a tag was never repointed.

```sh
jactionlint --online --online-report=report.json
```

With a cold cache, the quick-start workflow writes this report:

```json
{
  "version": 1,
  "status": "complete",
  "mode": "online",
  "skippedLookups": 0,
  "excludedRepositories": 0,
  "evidence": "network",
  "answers": {
    "network": 3,
    "cache": 0,
    "stale": 0,
    "cacheTtlSeconds": 3600
  }
}
```

`status` has three values:

| `status` | Meaning | What a consumer does |
| --- | --- | --- |
| `complete` | The checks ran and every lookup was answered, apart from repositories excluded on purpose | Trust the findings, or their absence. |
| `incomplete` | `skippedLookups` is not zero. `skippedBy` says why, and the checks that needed those lookups reported nothing. | Treat the result as partial. |
| `disabled` | The online checks were off. Only the report file has this value. | The checks did not run. |

A run with the checks off writes this:

```sh
jactionlint --online-report=report.json
```

```json
{
  "version": 1,
  "status": "disabled",
  "skippedLookups": 0,
  "excludedRepositories": 0
}
```

The fields of the report, and of `onlineCoverage` (which has no `version`):

| Field | Meaning |
| --- | --- |
| `version` | The version of the report format, only in the report file. It changes when a field changes meaning. |
| `mode` | `online`, `cache`, `strict` or `cache,strict`. It is absent when `status` is `disabled`. |
| `skippedLookups` | How many lookups failed. Lookups left out by `allow` and `deny` are not counted. |
| `excludedRepositories` | How many repositories `allow` and `deny` left out on purpose. It does not make a run incomplete. |
| `evidence` | `network` when the run could use the network, `cache` in the cache mode, where answers can be older than the TTL. |
| `skippedBy` | The skipped lookups by kind (`rate limit`, `unauthorized`, `forbidden`, `not found`, `server error`, `timeout`, `network`, `not cached`, `other`). It is absent when nothing was skipped. |
| `answers.network` | Answers that GitHub gave in this run, including a `304` that confirmed a cached answer. |
| `answers.cache` | Answers read from the disk cache without asking, at most `cacheTtlSeconds` old, or of any age with `--online=cache`. |
| `answers.stale` | Expired cached answers used because the rate limit left no other. |
| `answers.immutable` | Cached answers about a file at a full commit SHA, which cannot change, used whatever their age. They are not in `cache`. It is absent when there were none. |
| `answers.oldestFetched` | The oldest time (RFC 3339, UTC) at which a cached answer that was used was received. It is absent when no cached answer was used. |
| `answers.cacheTtlSeconds` | How long a cached answer is used without asking again, rounded up to whole seconds. It is absent in the cache mode and with a TTL of 0. |

`oldestFetched` is the clock of the machine that runs jactionlint, at the moment it last received the answer from GitHub or confirmed it with a `304`. It is not a date from GitHub and says nothing about when a tag or branch last changed. `answers` is absent when a Go program supplies its own GitHub client that does not track answers: that means unknown, not fresh. `--online=strict` is unchanged by cached or stale answers: it fails on skipped lookups only.

To fail a CI job when the checks did not complete, test the report with `jq`:

```sh
jactionlint --online --online-report=report.json
jq -e '.status == "complete"' report.json
```

`jq -e` exits with 1 when the expression is false.

## Cache and limits

jactionlint keeps the answers of GitHub on disk, so a second run, a parallel run and a run in CI with a restored cache ask GitHub for less. This section covers where the cache is, how long an answer lives, how to cache it in CI, and the limits of rate and size.

### The cache

- **Location.** `$XDG_CACHE_HOME/jactionlint` when `XDG_CACHE_HOME` is an absolute path, else `~/.cache/jactionlint`, also on macOS and Windows. A relative `XDG_CACHE_HOME` is ignored. No option chooses another directory.
- **Size.** At most 32 MiB. When the directory grows past that, the oldest files are removed down to three quarters of the limit. An answer larger than 4 MiB is used and not kept.
- **Sharing.** Parallel processes (hk runs one jactionlint per batch of files) share the directory safely. Answers are keyed by API host and token.
- **Time to live.** An answer is used for an hour (`--online-cache-ttl`, `cache-ttl`). After that jactionlint asks again with the ETag of the answer. When nothing changed, GitHub answers `304`, which costs no rate limit for a request with a token. `--online-cache-ttl=0` revalidates every answer.
- **Immutable answers.** The `action.yml` of an action, or a reusable workflow, pinned to a full commit SHA never changes, so it is used from the cache whatever its age. One pinned to a tag or a branch follows the TTL.
- **Tokens.** A public answer fetched without a token also serves a run with one. An answer fetched with a token is stored for that token, and also for every other token when the repository is known to be public, so the `GITHUB_TOKEN` of a CI job, which changes every run, does not empty a restored cache. jactionlint confirms that a repository is public with one request per repository and run. For a private or internal repository the answer serves only the token that fetched it, and an answer that says "not found" is never shared.

### Use the cache offline

`--online=cache` never uses the network. It answers from the cache whatever the age of the answers, and a lookup with nothing cached is skipped with one warning. Run once with `--online` to fill the cache, then use `--online=cache` on a laptop without a connection or in a sandboxed CI job that restores the cache directory. Add `strict` to fail on a lookup that has no cached answer.

### Cache the answers in CI

Restore the directory before the run and save it afterwards. A cache key that GitHub has stored cannot be written again, so the key holds the run ID and `restore-keys` finds the newest earlier one. The save step runs with `!cancelled()` so that a run with findings, which fails the job, still saves what it learned. The action downloads jactionlint and hands it the job token, see [the CI page](./ci.md#the-action) for its other inputs.

```yaml
name: lint-workflows
on:
  pull_request:
permissions:
  contents: read
concurrency:
  group: lint-workflows-${{ github.ref }}
  cancel-in-progress: true
jobs:
  jactionlint:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803 # v6.1.0
        with:
          persist-credentials: false
      - uses: actions/cache/restore@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0
        with:
          path: ~/.cache/jactionlint
          key: jactionlint-online-${{ github.run_id }}-${{ github.run_attempt }}
          restore-keys: jactionlint-online-
      - uses: jdx/jactionlint@a0bb9f012b6b5760c17c9ccd040001d08002a10e # v2.1.0
        with:
          online: true
      - uses: actions/cache/save@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 # v6.1.0
        if: ${{ !cancelled() }}
        with:
          path: ~/.cache/jactionlint
          key: jactionlint-online-${{ github.run_id }}-${{ github.run_attempt }}
```

This workflow has no finding with the default profile, with or without `--online`.

### Rate limits and retries

jactionlint reads `X-RateLimit-*` and `Retry-After`. A limit that resets within 30 seconds (`--online-max-wait`) is waited for. A limit that resets later skips the remaining lookups with a message that names the reset time, and answers in the cache are still used, expired ones too.

Server errors (500, 502, 503, 504), a secondary rate limit and a request that timed out once are repeated with exponential backoff and jitter, starting at half a second and capped at eight. The number of repeats is `retries` (default 2).

### How many requests a run makes

Every repository, tag and commit is asked for once per run however often it appears, and the checks share what they learn. The number of requests grows with the number of different actions, not with the number of steps. These requests come on top of the lookups of the other online rules:

- An action that is not in the built-in data set costs one request for its metadata for each distinct action, path and ref, and two when its file is `action.yaml` instead of `action.yml`.
- A reusable workflow of another repository costs one request for each distinct workflow, path and ref.
- With the `pedantic` option, `unpinnable-action` costs one request for every action pinned to a commit, and one for each commit-pinned action that the composite steps of those actions use. It reads two levels.

An answer in the cache costs no request. Without a token GitHub allows 60 requests an hour, so set one when a repository uses many actions.

### Limits

| Limit | Value |
| --- | --- |
| Disk cache | 32 MiB, the oldest files first removed |
| One cached answer | 4 MiB |
| One answer read | 16 MiB |
| One request | 20 seconds |
| Requests in flight | 6, up to 32 (`concurrency`) |
| Retries | 2, up to 10 (`retries`) |
| Longest wait for a rate limit | 30 seconds (`--online-max-wait`) |
| Pages read from a list of tags or branches | 10 |
| Branches that `impostor-commit` compares | `max-branches` (default 1000), and at most 100 without a token |
| Token file | 4096 bytes |
| `gh auth token` | 5 seconds |

## GitHub Enterprise Server

Point jactionlint at your Server with the API URL, and use the github.com fallback when your Server runs actions that GitHub Connect syncs from github.com. In a job on a Server, GitHub sets `GITHUB_API_URL`, so no setting is needed there.

### The API URL

The API is the first of these that is set:

1. `--online-api-url`.
2. `api-url` under `online-options` in the config file. A repository config file gets no token for it, see [A repository cannot choose the host](#a-repository-cannot-choose-the-host).
3. `$GITHUB_API_URL`.
4. The host of `$GITHUB_SERVER_URL`, else `$GH_HOST`: `https://HOST/api/v3`, or `https://api.NAME.ghe.com` for a `NAME.ghe.com` host with data residency.
5. `https://api.github.com`.

The URL must be http or https, without credentials. Answers are cached per host, and the Enterprise token variables come first, see [Find the token](#find-the-token). The branch scan of `impostor-commit` uses the GraphQL API next to the REST URL (`/api/graphql` on a Server). `GITHUB_GRAPHQL_URL` replaces it and is ignored with a notice when it is not on the host of the REST API.

### Actions from github.com

With GitHub Connect, a Server runs `uses: actions/checkout@v4` from github.com. The API of the Server answers 404 for that repository, so every check that needs it is skipped. `--online-github-com-fallback` (or `github-com-fallback: true` under [`online-options`](./config.md#online-options)) asks the Server first and, only for a repository that the Server answers 404 for, `api.github.com`. The host that has the repository answers every later question about it, and the Server wins when both have it, as it does when it runs the action. The fallback has no effect, and prints a warning, when the API already is `api.github.com`.

The fallback is off by default, because the names of the repositories that the Server does not have are sent to github.com. List the internal ones under `--online-deny` before you turn it on: they are never looked up. This example belongs in your user-global config file or in a file that you pass with `--config-file`:

```yaml
online-options:
  api-url: https://ghe.example.com/api/v3
  github-com-fallback: true
  deny:
    - my-org/*
```

How the fallback treats tokens and failures:

- **Tokens.** The token for `api.github.com` is the first of the variable of `--online-github-com-token-env`, the file of `--online-github-com-token-file`, `$GITHUB_COM_TOKEN` and `gh auth token --hostname github.com`. jactionlint runs `gh` without `GH_TOKEN` and `GITHUB_TOKEN` for it, because `gh` would print them. The token of the Server is never sent to github.com, and the token for github.com is never sent to the Server. Without a token the requests to github.com are unauthenticated (60 an hour), and a notice says so.
- **Registries.** Without the fallback, a Server gives ghcr.io no token. With the fallback on, the token that ghcr.io may receive is the one for github.com.
- **Advisories.** `known-vulnerable-actions` gets its advisories from github.com, where the database of advisories for actions is.
- **Failures.** A failure of github.com is counted and warned about like any other, but does not stop the lookups on the Server. After three timeouts or server errors in a row, a rate limit or a rejected token, github.com is not asked again in the run. The warning of a rate limit of github.com names `GITHUB_COM_TOKEN`.
- **Cost.** Every repository that the Server does not have costs one more request (a 404, cached like any answer, so the next runs do not repeat it). A repository that the Server has costs nothing more. `--online-report` adds up both hosts.

## Options and keys

Every online option has a key in the config file, except `--online-report`. The keys `retries`, `concurrency` and `gh-cli` have no option. An option wins over its key. The keys apply to the whole run: when one run checks files of several projects, the config file of the first file for which the online checks run decides for all of them.

- [The online options](./cli.md#github-api-online-checks) are listed with their defaults.
- [The mapping table](./cli.md#flag-environment-variable-and-configuration-key) pairs each option with its key and its environment variable. Two names differ: the key of `--online-max-wait` is `max-rate-limit-wait`, and the mode of `--online` is the key `mode`.
- [The online keys](./config.md#online-options) give each key with its type, default and effect.

## Next steps

- [Online options in the config file](./config.md#online-options)
- [Run jactionlint in CI with the action](./ci.md#the-action)
- [Fix findings automatically](./fix.md#fix-errors-automatically)
- [Exit status](./cli.md#exit-status)
- [Output formats and SARIF](./output.md#format-error-messages)
