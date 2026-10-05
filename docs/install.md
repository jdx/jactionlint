Installation
============

This document describes how to install [jactionlint](../docs) from jactionlint, the fork in
[jdx/jactionlint][repo].

> [!NOTE]
> Package managers such as Homebrew (core formula), Chocolatey, Scoop, winget, pacman, Nix, apt and the asdf plugin
> distribute the original [rhysd/actionlint][upstream], not this fork. Use one of the methods below to install this fork.

## mise

[mise][mise] can install the binaries from the GitHub releases of this repository:

```sh
mise use -g github:jdx/jactionlint@latest
jactionlint -version
```

## macOS

### Homebrew

This repository provides a Homebrew cask, which is automatically updated on new releases. Tap the repository and install
the `jactionlint` package with `--cask` option.

```sh
brew tap jdx/jactionlint https://github.com/jdx/jactionlint
brew install --cask jdx/jactionlint/jactionlint
```

> [!WARNING]
> Since the `jactionlint` executable is unsigned, macOS displays a warning and tries to move it to the Trash. To allow it to run,
> go to 'Settings -> Privacy & Security' and grant the permission.

## Prebuilt binaries

Download an archive file from [the releases page][releases] for your platform, unarchive it and put the executable file to a
directory in `$PATH`.

Prebuilt binaries are built at each releases by CI for the following OS and arch:

- macOS (x86_64, arm64)
- Linux (i386, x86_64, arm32, arm64)
- Windows (i386, x86_64, arm64)
- FreeBSD (i386, x86_64)

Note that the following targets are not tested since GitHub Actions doesn't support them:

- Linux i386, arm32
- Windows i386
- FreeBSD i386, x86_64

To install these binaries [`gh`][gh] command is useful. The following command is an example for x86_64 Linux.

```sh
gh release download --repo jdx/jactionlint --pattern '*_linux_amd64.tar.gz' v1.7.12
tar xf jactionlint_1.7.12_linux_amd64.tar.gz
./jactionlint -version
```

Optionally you can verify the [attestation][attestations] of the downloaded artifact. This is highly recommended in terms of
security.

```sh
gh attestation verify -R jdx/jactionlint jactionlint_1.7.12_linux_amd64.tar.gz
```

<a id="download-script"></a>
## Download script

To install `jactionlint` executable with one command, [the download script](../scripts/download-jactionlint.bash) is available.
It downloads the latest version of jactionlint (`jactionlint.exe` on Windows and `jactionlint` on other OSes) to the current
directory automatically. This is a recommended way if you install jactionlint in some shell script.

```sh
bash <(curl https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash)
```

When you need to install specific version of jactionlint, please give the version to the 1st command line argument. The following
example installs v1.6.17.

```sh
bash <(curl https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash) 1.6.17
```

This script downloads `jactionlint` (or `jactionlint.exe` on Windows) binary to the current working directory. When you need to put
the downloaded binary to some other directory, please give the directory path to the 2nd command line argument. The following
example installs the latest version to `/usr/bin`.

```sh
bash <(curl https://raw.githubusercontent.com/jdx/jactionlint/main/scripts/download-jactionlint.bash) latest /usr/bin
```

For the usage of jactionlint on GitHub Actions, see [the usage document](usage.md#on-github-actions).

## Docker image

The image is published to the GitHub Container Registry as `ghcr.io/jdx/jactionlint`. See
[the usage document](./usage.md#docker) to know how to use it.

## Build from source

Recent [Go][] toolchain is necessary to build jactionlint from source. Last two major versions of Go are supported.

```sh
# Install the latest stable version
go install github.com/jdx/jactionlint/cmd/jactionlint@latest

# Install the head of the main branch
go install github.com/jdx/jactionlint/cmd/jactionlint@main
```

---

[Checks](checks.md) | [Usage](usage.md) | [Configuration](config.md) | [Go API](api.md) | [References](reference.md)

[repo]: https://github.com/jdx/jactionlint
[upstream]: https://github.com/rhysd/actionlint
[releases]: https://github.com/jdx/jactionlint/releases
[gh]: https://docs.github.com/en/github-cli/github-cli/about-github-cli
[attestations]: https://docs.github.com/en/actions/concepts/security/artifact-attestations
[Go]: https://golang.org/
[mise]: https://mise.jdx.dev/
