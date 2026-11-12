#!/bin/bash

set -e -o pipefail

# Default value is updated by the release workflow after each release
version="2.5.0"
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
    echo 'JACTIONLINT_TOKEN:' >&2
    echo '  Environment variable. Optional GitHub token. When it is set, the script sends' >&2
    echo '  it with the download request to github.com, but not to the host that GitHub' >&2
    echo '  redirects to. Then the higher rate limit for authenticated requests applies.' >&2
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
token="${JACTIONLINT_TOKEN:-}"

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

# Download to a file before extracting it. A version without an archive makes GitHub answer with a short "Not Found"
# body, which tar and unzip would report as a broken archive.
if [[ "$os" == "windows" ]]; then
    tempdir="$(mktemp -d "${name}.XXXXXXXXXXXXXXXX")"
else
    # Give mktemp a template: not every mktemp (e.g. the one of FreeBSD) works without one
    tempdir="$(mktemp -d "${TMPDIR:-/tmp}/${name}.XXXXXXXXXXXXXXXX")"
fi
trap 'rm -rf "$tempdir"' EXIT
archive="$tempdir/${file}"

function not_found() {
    echo >&2
    echo "jactionlint v${version} was not found: there is no ${url}" >&2
    echo "Check that the release v${version} exists and has the archive ${file} at https://github.com/${repo}/releases" >&2
    exit 1
}

echo "Downloading ${url} with curl"

# Stop with a message for the HTTP status in $status. curl writes 000 when it gets no HTTP response, for example when
# it cannot connect. Then curl already wrote the error, and the message does not give a status.
function download_failed() {
    if [[ "$status" == "404" ]]; then
        not_found
    fi
    echo >&2
    if [[ -z "$status" || "$status" == "000" ]]; then
        echo "Could not download jactionlint v${version} from ${url}" >&2
    else
        echo "Could not download jactionlint v${version} from ${url} (HTTP status ${status})" >&2
    fi
    exit 1
}

# -f makes curl fail on an HTTP error and print no body. The -w output is the only output on stdout.
if [ -n "$token" ]; then
    # When JACTIONLINT_TOKEN is set, send the token only to github.com. This works with all curl versions:
    # - curl reads the header from a config file on stdin (-K -), which keeps the token out of the curl command line.
    #   printf is a shell builtin, so the token is not on its command line either. Do not use -H @-, which needs curl
    #   7.55.0. Do not use a process substitution such as -K <(...): the native curl.exe of Git Bash on Windows cannot
    #   open the /dev/fd path it gives.
    # - curl does not follow the redirect to the host of the release asset (no -L): before curl 7.58.0, -L sends a
    #   custom Authorization header to the host of a redirect. The script downloads the redirect URL without the token.
    # The config file syntax needs a backslash before each backslash and double quote in the value.
    escaped_token="${token//\\/\\\\}"
    escaped_token="${escaped_token//\"/\\\"}"
    response="$(printf 'header = "Authorization: Bearer %s"\n' "$escaped_token" |
        curl -f -K - -o "$archive" -w '%{http_code} %{redirect_url}' "$url")" || {
        status="${response%% *}"
        download_failed
    }
    status="${response%% *}"
    location="${response#* }"
    if [[ "$status" == 3* ]]; then
        if [[ -z "$location" ]]; then
            download_failed
        fi
        status="$(curl -fL -o "$archive" -w '%{http_code}' "$location")" || download_failed
    fi
else
    status="$(curl -fL -o "$archive" -w '%{http_code}' "$url")" || download_failed
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
