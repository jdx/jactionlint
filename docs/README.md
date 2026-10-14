# Documentation index

This directory holds the pages of the jactionlint documentation. The site [jactionlint.jdx.dev](https://jactionlint.jdx.dev) is the canonical rendering, and the release archives ship the same files in `docs/`.

| Page | Read it when |
| ---- | ------------ |
| [Installation](install.md) | you choose how to install jactionlint (mise, Homebrew, a release archive, the download script, Docker or Go) and check that it works. |
| [Run jactionlint](usage.md) | you run jactionlint on a repository, files or stdin, choose a profile, explain a rule or set up shellcheck, pyflakes and completion. |
| [Ignore findings](ignore.md) | you accept a finding and silence it with a comment, an option, the config file or a zizmor comment. |
| [Adopt new checks with a baseline](baseline.md) | you adopt a stricter profile before everything is fixed: record a baseline so that only new findings fail. |
| [Fix findings automatically](fix.md) | you preview and apply the automatic fixes and want to know why a finding is not fixed. |
| [Run jactionlint in CI](ci.md) | you run jactionlint on GitHub Actions with the action, mise, Docker or the download script and show findings as annotations or code scanning alerts. |
| [Git hooks and editors](hooks.md) | you run jactionlint from hk or pre-commit, or connect an editor or another tool. |
| [Online checks](online.md) | you turn on the checks that ask GitHub about the actions a workflow uses and want to know what they send and what happens when GitHub does not answer. |
| [Configuration](config.md) | you create `.github/jactionlint.yaml` or look up a key, a profile or the level of a rule. |
| [Command line](cli.md) | you look up an option, an environment variable, a program, a network host or the exit status. |
| [Output formats](output.md) | you choose an output format for annotations, code scanning or scripts, or look up the fields of a finding. |
| [Checks](checks.md) | you hold a finding or a rule ID and want to know what is checked, with an example and the fix. |
| [Rules](rules.md) | you look up the group, default level, profile, fix and options of a rule. |
| [Go API](api.md) | you use jactionlint from a Go program: lint, read and fix findings, add a rule of your own. |
| [References](reference.md) | you look up the GitHub documentation, the specifications and the tools behind the checks. |
| [Coming from actionlint](actionlint.md) | you switch a repository from actionlint and keep the same findings with the `correctness` profile. |
| [Migrating to v2](v2-migration.md) | you upgrade a repository from jactionlint 1.x to v2: what breaks and how to fix it. |
| [jactionlint and zizmor](zizmor-parity.md) | you run zizmor and want to run jactionlint beside it or in its place. |
| [zizmor comparison measurements](zizmor-measurements.md) | you decide whether a rule is ready for the default profile and want the dated measurements. |
| [Playground](https://jactionlint.jdx.dev/playground) | you check a workflow in the browser without installing jactionlint. |
