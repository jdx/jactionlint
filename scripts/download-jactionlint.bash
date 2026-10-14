#!/bin/bash

set -e -o pipefail

# Default value is updated by the release workflow after each release
version="2.1.0"
default_version="$version"

function usage() {
    echo 'USAGE:' >&2
    echo '  bash download-jactionlint.bash [[VERSION] DIR]' >&2
    echo >&2
    echo 'This script downloads jactionlint binary from the following release page. curl' >&2
    echo 'command is required as dependency' >&2
    echo 'https://github.com/jdx/jactionlint/releases' >&2
    echo >&2
    echo 'JACTIONLINT_REPO:' >&2
    echo '  Environment variable. GitHub repository to download from. The default is' >&2
    echo '  "jdx/jactionlint".' >&2
    echo >&2
    echo 'JACTIONLINT_NAME:' >&2
    echo '  Environment variable. Name of the executable and the prefix of the archive file' >&2
    echo '  name in the releases. The default is "jactionlint".' >&2
    echo >&2
    echo 'DIR:' >&2
    echo '  Directory to put the downloaded binary (e.g. /path/to/dir). When this value is' >&2
    echo '  omitted, the binary will be put in the current directory.' >&2
    echo >&2
    echo 'VERSION:' >&2
    echo '   Version of jactionlint to download. Version must be a specific version' >&2
    echo '   "{major}.{minor}.{patch}" such as "2.0.2" or "latest". When "latest" is' >&2
    echo "   specified or this argument is omitted, the default version of this script" >&2
    echo "   (${default_version}) will be selected. The release workflow updates the default after" >&2
    echo '   each release, so it is the latest release when this script was last updated.' >&2
    echo '   An older copy of this script selects an older release.' >&2
    echo >&2
    echo 'EXAMPLE:' >&2
    echo '  - Download the default version to the current directory' >&2
    echo >&2
    echo '      $ bash download-jactionlint.bash' >&2
    echo >&2
    echo '  - Download the default version to /usr/bin' >&2
    echo >&2
    echo '      $ bash download-jactionlint.bash latest /usr/bin' >&2
    echo >&2
    echo '  - Download version 2.0.2 to the current directory' >&2
    echo >&2
    echo '      $ bash download-jactionlint.bash 2.0.2' >&2
    echo >&2
    echo '  - Download version 2.0.2 to /usr/bin' >&2
    echo >&2
    echo '      $ bash download-jactionlint.bash 2.0.2 /usr/bin' >&2
}

if [[ "$1" == "-h" || "$1" == "--help" ]]; then
    usage
    exit
fi

repo="${JACTIONLINT_REPO:-jdx/jactionlint}"
name="${JACTIONLINT_NAME:-jactionlint}"

if [ -n "$1" ]; then
    if [[ "$1" != 'latest' && "$1" != 'LATEST' ]]; then
        if [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            version="$1"
        else
            echo "Given version '$1' does not match to regex '^[0-9]+\.[0-9]+\.[0-9]+$' nor equal to 'latest'" >&2
            echo >&2
            usage
            exit 1
        fi
    fi
fi

target_dir="$(pwd)"
if [ -n "$2" ]; then
    if [ -d "$2" ]; then
        target_dir="${2%/}"
    else
        echo "Directory '$2' does not exist" >&2
        echo >&2
        usage
        exit 1
    fi
fi

echo "Start downloading jactionlint v${version} to ${target_dir}"

case "$OSTYPE" in
    linux-*)
        os=linux
        ext=tar.gz
    ;;
    darwin*)
        os=darwin
        ext=tar.gz
    ;;
    freebsd*)
        os=freebsd
        ext=tar.gz
    ;;
    msys|cygwin|win32)
        os=windows
        ext=zip
    ;;
    *)
        echo "OS '${OSTYPE}' is not supported. Note: If you're using Windows, please ensure bash is used to run this script" >&2
        exit 1
    ;;
esac


machine="$(uname -m)"
case "$machine" in
    x86_64|amd64) arch=amd64 ;; # FreeBSD prints amd64
    i?86) arch=386 ;;
    aarch64|arm64) arch=arm64 ;;
    arm*) arch=armv6 ;;
    *)
        echo "Could not determine arch from machine hardware name '${machine}'" >&2
        exit 1
    ;;
esac

echo "Detected OS=${os} ext=${ext} arch=${arch}"

# https://github.com/jdx/jactionlint/releases/download/v1.0.0/jactionlint_1.0.0_linux_386.tar.gz
file="${name}_${version}_${os}_${arch}.${ext}"
url="https://github.com/${repo}/releases/download/v${version}/${file}"

echo "Downloading ${url} with curl"

# Download to a file before extracting it. A version without an archive makes GitHub answer with a short "Not Found"
# body, which tar and unzip would report as a broken archive.
if [[ "$os" == "windows" ]]; then
    tempdir="$(mktemp -d "${name}.XXXXXXXXXXXXXXXX")"
else
    tempdir="$(mktemp -d)"
fi
trap 'rm -rf "$tempdir"' EXIT
archive="$tempdir/${file}"

# -f makes curl fail on an HTTP error and print no body. The HTTP status is the only output on stdout.
if ! status="$(curl -fL -o "$archive" -w '%{http_code}' "${url}")"; then
    echo >&2
    if [[ "$status" == "404" ]]; then
        echo "jactionlint v${version} was not found: there is no ${url}" >&2
        echo "Check that the release v${version} exists and has the archive ${file} at https://github.com/${repo}/releases" >&2
    else
        echo "Could not download jactionlint v${version} from ${url}" >&2
    fi
    exit 1
fi

if [[ "$os" == "windows" ]]; then
    unzip "$archive" "${name}.exe" -d "$target_dir"
    exe="$target_dir/${name}.exe"
else
    tar xvzf "$archive" -C "$target_dir" "$name"
    exe="$target_dir/${name}"
fi

echo "Downloaded and unarchived executable: ${exe}"

echo "Done: $("${exe}" --version)"

if [ -n "$GITHUB_ACTION" ]; then
    # On GitHub Actions, set executable path to output
    if [ -n "${GITHUB_OUTPUT}" ]; then
        echo "executable=${exe}" >> "$GITHUB_OUTPUT"
    else
        # GitHub Enterprise instance may not introduce the new set-output command yet (see #240)
        echo "::set-output name=executable::${exe}"
    fi
fi
