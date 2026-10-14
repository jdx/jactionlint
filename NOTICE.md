# Acknowledgements

jactionlint builds on the work of the projects below. This file says what each project contributed, which data jactionlint reads from it and which programs jactionlint runs. Thank you to their authors and contributors.

## Forked from

- [rhysd/actionlint](https://github.com/rhysd/actionlint) by rhysd (MIT). jactionlint is a fork of actionlint: the parser, the expression type checker, the workflow syntax checks and many tests come from it, and jactionlint merges pull requests of upstream. The copyright notice of actionlint is kept in [LICENSE.txt](LICENSE.txt).

## Checks informed by

- [zizmorcore/zizmor](https://github.com/zizmorcore/zizmor): the security rules of jactionlint follow its audits, and jactionlint reads its `# zizmor: ignore[...]` comments. [jactionlint and zizmor](docs/zizmor-parity.md) lists the rules one by one.
- [TWiStErRob/net.twisterrob.ghlint](https://github.com/TWiStErRob/net.twisterrob.ghlint): the checks `malformed-expression-delimiters`, `environment-file-redirect` and `gh-without-token`, and the option `pedantic` of `untrusted-artifact`, started as issues or checks of ghlint. The commit history of each names the ghlint issue (`ghlint#NNN`) or the ghlint check.
- [boostsecurityio/poutine](https://github.com/boostsecurityio/poutine): the rule `unpinnable-action` is its "unpinnable action" check, and the rules about the checkout of untrusted code take part of their lists of actions and commands from it. The comments in the Go source name the origin.
- [github/codeql](https://github.com/github/codeql): the rules about the checkout of untrusted code take part of their lists of actions and commands from its models and judge a reusable workflow by its caller as CodeQL does, and the rule `argument-injection` leaves out the names of branches, labels and accounts as its models do. The comments in the Go source name the origin.
- [suzuki-shunsuke/ghalint](https://github.com/suzuki-shunsuke/ghalint): the `accept-step-timeouts` option of `missing-timeout` is the exception of its policy 012.

## Data and specifications

- [octokit/webhooks](https://github.com/octokit/webhooks) and [github/rest-api-description](https://github.com/github/rest-api-description) (MIT): the payloads of `github.event` are generated from their schemas.
- [actions/languageservices](https://github.com/actions/languageservices): its workflow and action schemas are the reference for the events that GitHub still lists and for the contexts that the keys of an action allow.
- [actions/runner](https://github.com/actions/runner): its workflow and action schemas are pinned in `testdata/schema/runner`, and the tests compare the parser of jactionlint with them.
- [github/docs](https://github.com/github/docs): the table of context availability on its contexts page is the source of the contexts and functions that jactionlint allows at each workflow key.
- [actions/runner-images](https://github.com/actions/runner-images): the runner labels and the images behind the `-latest` labels.
- The `action.yml` files of popular actions: their inputs and outputs are read from the repository of each action and kept in `popular_actions.go`.

## Programs it runs

jactionlint runs these programs and does not contain their code. The executable bundles none of them.

- [koalaman/shellcheck](https://github.com/koalaman/shellcheck) checks the shell scripts of `run:`. The Docker image includes it, and the Homebrew cask depends on it.
- [PyCQA/pyflakes](https://github.com/PyCQA/pyflakes) checks the Python scripts of `run:`. The Docker image includes it.
- [astral-sh/ruff](https://github.com/astral-sh/ruff), optional, checks the Python scripts with its pyflakes rules when you pass `--ruff`.
- [Node.js](https://nodejs.org/), optional, checks the JavaScript syntax of the `script` input of actions/github-script with `node --check`.
- [wasilibs/go-shellcheck](https://github.com/wasilibs/go-shellcheck) is the build of shellcheck that the `jactionlint-shellcheck` pre-commit hook installs, so that the hook needs no system shellcheck.

[Programs and network](docs/cli.md#programs-and-network) in the command line reference lists every program that jactionlint starts.

## Go modules

The Go modules that jactionlint builds on are listed in [go.mod](go.mod).
