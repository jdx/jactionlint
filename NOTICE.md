# Acknowledgements

jactionlint stands on the work of other projects. Thank you to their authors and contributors.

## Forked from

- [rhysd/actionlint](https://github.com/rhysd/actionlint) by rhysd (MIT). jactionlint is a fork of actionlint: the
  parser, the expression type checker, the workflow syntax checks and many tests come from it. Its copyright
  notice is kept in [LICENSE.txt](LICENSE.txt).

## Checks informed by

- [zizmorcore/zizmor](https://github.com/zizmorcore/zizmor): the security audits of jactionlint follow its audits, and
  jactionlint reads its `zizmor: ignore` comments. [jactionlint and zizmor](docs/zizmor-parity.md) lists them one
  by one.
- [TWiStErRob/net.twisterrob.ghlint](https://github.com/TWiStErRob/net.twisterrob.ghlint): several policy and
  correctness checks started as issues of ghlint, which are referenced as `ghlint#NNN` in the code and
  [the list of checks](docs/checks.md).

## Data and specifications

- [octokit/webhooks](https://github.com/octokit/webhooks) and
  [github/rest-api-description](https://github.com/github/rest-api-description) (MIT): the payloads of
  `github.event` are generated from their schemas.
- [actions/languageservices](https://github.com/actions/languageservices) and
  [actions/runner-images](https://github.com/actions/runner-images): the workflow schema, context availability and the
  runner labels.

## Ported from

- [pyflakes](https://github.com/PyCQA/pyflakes): the checks of Python scripts in `run:` are ported from it.
