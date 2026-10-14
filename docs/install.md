# Installation

Install jactionlint with mise, Homebrew, pip, a release archive, the download script, a Docker image or `go install`, and check that the installation works. jactionlint is a single executable that needs no other program: shellcheck, pyflakes and node are optional companions.

> [!NOTE]
> This site documents the development version (the `main` branch). A release can lack options and rules that these pages describe: `jactionlint --help` lists what the installed version supports, and an option that is newer than your version fails with `unknown option`. To get everything on this site, [build the `main` branch](#build-from-source).

## Choose a method

Each method installs the same program. They differ in how the download is verified and in how you update, so start with mise unless another row fits your machine better.

| Method | Use it when | The download is verified by | Update with |
| --- | --- | --- | --- |
| [mise](#mise) | You work on a laptop or in CI and want one command to install and update. | A checksum and the provenance that mise records in `mise.lock`. | `mise upgrade jactionlint` |
| [Homebrew cask](#homebrew) | You use Homebrew, on macOS or Linux. | The sha256 in the cask. | Homebrew. Each release updates the cask. |
| [PyPI](#pypi) | You manage your tools with pip, pipx or uv, for example in a Python project. | PyPI, over HTTPS. The checksums file and the attestation of a release do not cover the wheels. | `pip install --upgrade jactionlint`, `pipx upgrade jactionlint` or `uv tool upgrade jactionlint` |
| [Release archive](#prebuilt-binaries) | You want a file for any supported platform, installed by hand or by your own tooling. | The checksums file and the attestation, which you check. | Download the new archive. |
| [Download script](#download-script) | You need a one-command install in a script or on a throwaway CI runner. | Nothing: the script does not verify the archive. | Run the script again. |
| [Docker image](#docker-image) | You run jactionlint in a container and want shellcheck and pyflakes in it. | The image digest, when you pin it. | `docker pull` |
| [Go](#build-from-source) | You have Go, or you want the development version. | The Go checksum database. | Run `go install` again. |

In GitHub Actions, the `jdx/jactionlint` action installs jactionlint for you. [Run jactionlint in CI](./ci.md#choose-a-method) compares it with mise, Docker and the download script.

Package managers that carry a package named `actionlint`, such as Homebrew core, Chocolatey, Scoop and pacman, install the original [rhysd/actionlint](https://github.com/rhysd/actionlint), not this fork. Use one of the methods on this page instead.

## mise

[mise](https://mise.jdx.dev/) installs jactionlint from the releases of this repository. Install it for your user and check it:

```sh
mise use -g jactionlint
jactionlint --version
```

`mise use jactionlint` without `-g` adds jactionlint to the `mise.toml` of the current project. It writes `jactionlint = "latest"`, and each machine resolves `latest` when it installs, so two machines can end up with different versions. To give everyone who works on the project, and CI, the same version, pin it in one of two ways:

- Run `mise use jactionlint@<version>` to write an exact version into `mise.toml`. `mise ls-remote jactionlint` lists the versions that you can install.
- Run `mise lock` and commit the `mise.lock` that it writes. The file records the resolved version, a checksum for each platform and the provenance of the download. The [lockfile setting of mise](https://mise.jdx.dev/configuration/settings.html#lockfile) describes it.

`mise upgrade jactionlint` installs a newer version. It does not move past an exact version in `mise.toml`. mise can also hold back a release that is younger than its `minimum_release_age` setting, so a release that GitHub published a few hours ago may not be offered yet. The [settings of mise](https://mise.jdx.dev/configuration/settings.html) list the default of your mise version.

An older mise that lacks the registry entry for jactionlint does not know the short name. Use the GitHub backend, which is what the registry entry points to:

```sh
mise use -g github:jdx/jactionlint
```

The release workflow also signs a [packslip](https://packslip.dev) for the archives and uploads it as `packslip.sigstore.json`. To install through that bundle instead of the GitHub backend, run:

```sh
mise use -g packslip:jdx/jactionlint
```

To install with mise in a workflow, see [mise in CI](./ci.md#mise).

<a id="macos"></a>
## Homebrew

The repository provides a Homebrew cask, which each release updates automatically. Tap the repository, then install the cask:

```sh
brew tap jdx/jactionlint https://github.com/jdx/jactionlint
brew install --cask jdx/jactionlint/jactionlint
```

The cask downloads the release archive for macOS or Linux, on Intel or Arm, and installs the executable and the man page, so `man jactionlint` works. It also installs the `shellcheck` formula as a dependency. It does not install pyflakes, which jactionlint needs only for steps with `shell: python` (see [Companion programs](#companion-programs)).

The release workflow signs the macOS executables with a Developer ID certificate and has Apple notarize them, and it fails the release unless Apple accepts them. The cask and the macOS archives hold the same executables. Releases from before that step existed are not signed.

## PyPI

The release workflow publishes jactionlint to [PyPI](https://pypi.org/project/jactionlint/) as wheels, which are the Python packages that pip, pipx and uv install. A wheel holds the same program as a release archive, so use it when one of these tools already manages the programs of your machine or project:

```sh
pip install jactionlint
pipx install jactionlint
uv tool install jactionlint
uvx jactionlint --version
```

`pip install` adds jactionlint to the current Python environment. `pipx install` and `uv tool install` give it an isolated environment and put the `jactionlint` command on your `PATH`. `uvx` runs jactionlint from the cache of uv without installing it. To update, run `pip install --upgrade jactionlint`, `pipx upgrade jactionlint` or `uv tool upgrade jactionlint`.

What differs from the other methods:

- **Platforms.** There is a wheel for Linux, macOS and Windows, each on x86-64 and on Arm. The other platforms of [the release archives](#prebuilt-binaries), such as FreeBSD and 32-bit Linux, have none.
- **Python.** The wheel declares Python 3.10 or later. The `jactionlint` command that it installs is a small Python script that starts the executable inside the wheel, so Python stays installed. jactionlint itself does not use Python, except through the optional pyflakes (see [Companion programs](#companion-programs)).
- **Version.** The wheel has the version of the release, and `jactionlint --version` prints `installed from PyPI` on its second line (see [Check the installation](#check-the-installation)).
- **Verification.** A separate job of the release workflow builds the wheels from the tag, not from the release archives, and uploads them with PyPI trusted publishing. The checksums file and the attestation of [Verify the download](#verify-the-download) do not cover them, and the macOS executables are not signed with the Developer ID certificate of the archives.
- **Releases.** A release has wheels only when the workflow that made it had that job. Releases from before then are not on PyPI.

In a workflow, prefer the `jdx/jactionlint` action or mise: see [the action](./ci.md#the-action) and [mise in CI](./ci.md#mise). A `run:` step that installs from PyPI names the version, as in `pip install jactionlint==<version>` or `uvx jactionlint@<version>`. With the pedantic profile, jactionlint reports a call without it as [`unpinned-tools`](./rules.md#unpinned-tools).

## Prebuilt binaries

Each release on [the releases page](https://github.com/jdx/jactionlint/releases) has one archive per platform. The name of an archive is `jactionlint_<version>_<os>_<arch>.<format>`, where `<version>` is the release version without the leading `v`:

| Platform | `<os>_<arch>` | Format |
| --- | --- | --- |
| macOS | `darwin_amd64` (Intel), `darwin_arm64` (Apple silicon) | `tar.gz` |
| Linux | `linux_amd64`, `linux_arm64`, `linux_386`, `linux_armv6` (32-bit Arm) | `tar.gz` |
| Windows | `windows_amd64`, `windows_arm64`, `windows_386` | `zip` |
| FreeBSD | `freebsd_amd64`, `freebsd_386` | `tar.gz` |

A release also has `jactionlint_<version>_checksums.txt`, which [Verify the download](#verify-the-download) uses, and `packslip.sigstore.json`, which mise reads through its `packslip:` backend (see [mise](#mise)). An archive holds the `jactionlint` executable (`jactionlint.exe` on Windows), `README.md`, `LICENSE.txt`, `NOTICE.md`, `CONTRIBUTING.md`, the man page `man/jactionlint.1` and the `docs` directory with the Markdown sources of this site. The Linux and FreeBSD executables are statically linked: every executable is built with `CGO_ENABLED=0`, so none of them depends on the C library of the system.

To install on Linux x86-64 with the [`gh` command](https://docs.github.com/en/github-cli/github-cli/about-github-cli), download the archive and the checksums file of the latest release. Without a tag, `gh release download` takes the latest release.

```sh
gh release download --repo jdx/jactionlint --pattern '*_linux_amd64.tar.gz' --pattern '*_checksums.txt'
```

[Verify the download](#verify-the-download), then extract the executable. Naming it keeps the other members of the archive out of the current directory:

```sh
tar xzf jactionlint_*_linux_amd64.tar.gz jactionlint
./jactionlint --version
```

Move `jactionlint` into a directory on your `PATH`. On Windows, extract `jactionlint.exe` from the zip file instead. Without `gh`, download the archive from the releases page in a browser.

To install a release other than the latest, pass its tag as the last argument, for example `gh release download --repo jdx/jactionlint --pattern '*_linux_amd64.tar.gz' v<version>`. A tag is a release only when `gh release list --repo jdx/jactionlint` shows it. Older tags in the git history come from the original actionlint and have no archives, and a release can be published without archives.

### Verify the download

Run both checks before you extract the archive.

1. Compare the archive with the checksums file. The command prints `OK` for the archive that you downloaded and ignores the other lines of the file. The checksums file comes from the same release as the archive, so this check detects a damaged download and nothing more.

   ```sh
   sha256sum --check --ignore-missing jactionlint_*_checksums.txt
   ```

2. Verify the [artifact attestation](https://docs.github.com/en/actions/concepts/security/artifact-attestations) of the archive. The command succeeds only when a workflow of the repository `jdx/jactionlint` attested the archive. The release workflow creates this attestation for each release.

   ```sh
   gh attestation verify --repo jdx/jactionlint jactionlint_*_linux_amd64.tar.gz
   ```

## Download script

[`scripts/download-jactionlint.bash`](https://github.com/jdx/jactionlint/blob/main/scripts/download-jactionlint.bash) downloads the release archive for the platform it runs on, extracts the executable and prints its version. It needs bash and curl, plus `tar` on Linux, macOS and FreeBSD, or `unzip` on Windows, where you run it from a bash such as Git Bash.

The script picks the archive from `$OSTYPE` and `uname -m`. `x86_64` selects `amd64`, `aarch64` and `arm64` select `arm64`, names such as `i686` select `386`, and any other name that starts with `arm` selects `armv6`.

The command line is `bash download-jactionlint.bash [[VERSION] DIR]`. The table lists the two arguments and the two environment variables that change what the script downloads:

| Input | Default | Meaning |
| --- | --- | --- |
| `VERSION` | `latest` | An exact version such as `<major>.<minor>.<patch>` without a leading `v`, or `latest` (`LATEST` works too). `latest` is the version that is written into the script, which can be older than the newest release. Anything else exits with an error. |
| `DIR` | The current directory | The directory that receives `jactionlint` (`jactionlint.exe` on Windows). It must exist, or the script exits with an error. |
| `JACTIONLINT_REPO` | `jdx/jactionlint` | The GitHub repository whose releases the script downloads from. |
| `JACTIONLINT_NAME` | `jactionlint` | The name of the executable and the prefix of the archive name. |

Pass a version that has archives: for any other version curl downloads GitHub's not-found page and `tar` stops with `gzip: stdin: not in gzip format`. `bash download-jactionlint.bash --help` prints these inputs. Inside a GitHub Actions step the script also writes the path of the executable to the step output `executable`.

Run the script from a URL that names a commit, with the version that you want and a directory that exists:

```sh
bash <(curl -fsSL https://raw.githubusercontent.com/jdx/jactionlint/<commit-sha>/scripts/download-jactionlint.bash) <version> "$HOME/.local/bin"
```

Replace `<commit-sha>` with the full 40-character SHA of a commit of `jdx/jactionlint`, for example the commit that a release tag points to, and `<version>` with the version to install. The commit fixes the script that runs. The version fixes the archive that it downloads.

The release workflow updates the version in the script in a commit after it publishes the release. A saved copy of the script therefore installs the release that was the latest when you saved it, and the script at the commit of a release tag installs the release before that tag.

The same URL with `main` in place of the commit works, and so does one with a tag. Each runs whatever that branch or tag holds when you run it. jactionlint reports both forms in a workflow file as a finding of the rule `unverified-download`, which has the level error in the default profile, and only a URL with a full commit SHA passes. See [the check](./checks.md#check-unverified-download) and [the rule](./rules.md#unverified-download).

The script does not verify the archive that it downloads. Use [a release archive and its checks](#verify-the-download) when verification matters.

In a workflow, prefer the `jdx/jactionlint` action or mise: see [the action](./ci.md#the-action) and [mise in CI](./ci.md#mise).

## Docker image

The Docker image is published to the GitHub Container Registry for linux/amd64 and linux/arm64. Pull it and run it:

```sh
docker pull ghcr.io/jdx/jactionlint:latest
docker run --rm ghcr.io/jdx/jactionlint:latest --version
```

| Fact | Value |
| --- | --- |
| Image | `ghcr.io/jdx/jactionlint` |
| Tags | `latest`, the newest release, and `<version>`, one tag for each release, named by the release version without a leading `v`. The release workflow pushes these two tags. |
| Platforms | `linux/amd64` and `linux/arm64` |
| Base | Alpine Linux |
| Contains | `jactionlint`, `shellcheck` and `pyflakes` |
| Does not contain | `node` (the rule `github-script-syntax` is skipped with a note), `ruff`, `git` and `gh` |
| User | `guest`, uid 405, gid 100: the container does not run as root |
| Entrypoint | `jactionlint`, so the arguments follow the image name |

A tag can move to another image, and the default profile reports a tag in `docker run` as the rule `unpinned-images`. `<version>` names one release, but only the digest names one image. Pin it as `ghcr.io/jdx/jactionlint@sha256:<digest>`. [The Docker section of the CI page](./ci.md#docker) has the run examples: checking a mounted repository, reading stdin and writing files as the right user.

## Build from source

Building needs the Go version that the `go` line of [go.mod](https://github.com/jdx/jactionlint/blob/main/go.mod) names, or a newer one. Since Go 1.21 the `go` command downloads a newer toolchain itself when go.mod asks for one, unless `GOTOOLCHAIN=local` is set.

```sh
# The latest release
go install github.com/jdx/jactionlint/v2/cmd/jactionlint@latest

# The development version, the head of the main branch
go install github.com/jdx/jactionlint/v2/cmd/jactionlint@main
```

`go install` writes the executable to `$GOBIN`, or to `$GOPATH/bin` (`~/go/bin` by default) when `GOBIN` is not set. Put that directory on your `PATH`. Only `@main` has everything that this site documents. The module path ends in `/v2`, as [Migrating to v2](./v2-migration.md#module-path) explains.

In a clone of the repository, `go build ./cmd/jactionlint` writes the executable to the current directory. [CONTRIBUTING.md](https://github.com/jdx/jactionlint/blob/main/CONTRIBUTING.md) describes the development setup.

## Check the installation

Print the version of the installed executable:

```sh
jactionlint --version
```

The output has three lines. This is their shape: `<version>`, `<go version>` and `<os>/<arch>` stand for the values of your executable.

```
<version>
installed by downloading from release page
built with <go version> compiler for <os>/<arch>
```

The first line is the version and the third names the Go compiler and the platform of the executable. The second line tells you how the executable was installed, which shows where an unexpected version comes from:

| Installed with | First line | Second line |
| --- | --- | --- |
| mise, Homebrew cask, release archive, download script | The release version without a leading `v` | `installed by downloading from release page` |
| PyPI wheel (pip, pipx, uv) | The release version without a leading `v` | `installed from PyPI` |
| Docker image | The release version without a leading `v` | `installed by building from source` |
| `go install` with `@latest` | The release version with a leading `v` | `installed by building from source` |
| `go install` with `@main`, or a build in a clone | A pseudo-version of the commit | `installed by building from source` |

`jactionlint --help` lists the options of the installed version, and `jactionlint --completion=bash` prints a completion script, which [Shell completion](./usage.md#shell-completion) explains. The Homebrew cask installs the man page, and the release archive contains it.

### Companion programs

jactionlint runs without any other program, and three companions are optional. The Docker image contains `shellcheck` and `pyflakes`, and the Homebrew cask installs `shellcheck`. With every other method, install them yourself and put them on your `PATH`.

| Program | jactionlint uses it for | Without it |
| --- | --- | --- |
| `shellcheck` | The scripts of `run:` steps that use a shell | These scripts are not checked, and a note says so. |
| `pyflakes`, or `ruff` with `--ruff` | The scripts of steps with `shell: python` | These scripts are not checked, and a note says so. |
| `node` 20 or later | The rule `github-script-syntax`, which checks the script of `actions/github-script` | The rule does not run, and a note says so. |

A missing program does not change the exit status. jactionlint prints one note per program on stderr after the findings and skips the scripts that the program would check. [When shellcheck or pyflakes is missing](./usage.md#when-shellcheck-or-pyflakes-is-missing) shows the notes and how to silence them.

The mise registry has entries for `shellcheck`, `ruff` and `node`, so `mise use -g shellcheck` installs shellcheck. [External tools](./usage.md#external-tools) describes the options that choose the programs and how SARIF output reports them.

## Next steps

- [Run jactionlint on a repository](./usage.md#the-first-run) and read the first findings.
- [Choose a profile](./usage.md#choose-a-profile) that matches how strict you want the checks to be.
- [Run jactionlint in CI](./ci.md#choose-a-method) with the action, mise or Docker.
- [Run jactionlint from Git hooks](./hooks.md#pre-commit) with pre-commit or hk.
- [Move from actionlint](./actionlint.md#coming-from-actionlint) and see what changes.
