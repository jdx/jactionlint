#!/bin/bash

set -o pipefail
set -e

if [ ! -d .git ]; then
    echo 'This script must be run from root of repository' >&2
    exit 1
fi

set -x

# Releases of this repository have its own default version ("No arguments", which the release workflow bumps on
# every release) and "latest". The old versions below only exist in the releases of the original repository, and
# so do not exist here. The script works the same way for any repository.
use_own_releases() {
    export JACTIONLINT_REPO=jdx/jactionlint
    export JACTIONLINT_NAME=jactionlint
    bin=jactionlint
}
use_original_releases() {
    export JACTIONLINT_REPO=rhysd/actionlint
    # The executable and the archives of the original releases are named "actionlint"
    export JACTIONLINT_NAME=actionlint
    bin=actionlint
}

script="$(pwd)/scripts/download-jactionlint.bash"
temp_dir="$(mktemp -d)"
trap 'popd && rm -rf $temp_dir' EXIT
pushd "$temp_dir"

# Normal cases
set -e

# No arguments
use_own_releases
out="$(bash "$script")"
if [ -n "$GITHUB_ACTION" ]; then
    if [[ "$out" != *"executable="* ]]; then
        echo "'executable' step output is not set: '${out}'" >&2
    fi
fi
out="$(./"${bin}" --version)"
if [[ "$out" != *'installed by downloading from release page'* ]]; then
    echo "Output from ./${bin} --version is unexpected: '${out}'" >&2
    exit 1
fi
rm -f ./"${bin}"

# Specify only version
use_original_releases
bash "$script" '1.6.12'
out="$(./"${bin}" --version | head -n 1)"
if [[ "$out" != '1.6.12' ]]; then
    echo "Unexpected version: '${out}'" 1>&2
    exit 1
fi
rm -f ./"${bin}"

# Specify only a download directory
use_own_releases
mkdir ./test1
bash "$script" latest ./test1
out="$(./test1/"${bin}" --version)"
if [[ "$out" != *'installed by downloading from release page'* ]]; then
    echo "Output from ./${bin} --version is unexpected: '${out}'" >&2
    exit 1
fi
rm -rf ./test1

# Specify both version and a download directory
use_original_releases
mkdir ./test2
bash "$script" '1.6.12' ./test2
out="$(./test2/"${bin}" --version | head -n 1)"
if [[ "$out" != '1.6.12' ]]; then
    echo "Unexpected version: '${out}'" 1>&2
    exit 1
fi
rm -rf ./test2

# Download with a token. This uses the real curl, so on Windows it checks that the native curl.exe of Git Bash can read
# the header. CI gives GITHUB_TOKEN. The trace is off while the token is expanded, so that the log does not show it.
set +x
if [ -n "${GITHUB_TOKEN:-}" ]; then
    use_own_releases
    mkdir ./test3
    JACTIONLINT_TOKEN="$GITHUB_TOKEN" bash "$script" latest ./test3
    set -x
    out="$(./test3/"${bin}" --version)"
    if [[ "$out" != *'installed by downloading from release page'* ]]; then
        echo "Output from ./${bin} --version is unexpected: '${out}'" >&2
        exit 1
    fi
    rm -rf ./test3
fi
set -x

# Error cases
use_own_releases
set +e

fails=0
if bash "$script" 'v1.6.12'; then
    echo "FAIL: Invalid version at the first argument did not cause any error" >&2
    ((fails++))
fi
if bash "$script" './this/dir/does/not/exist'; then
    echo "FAIL: Directory which does not exist at the first argument did not cause any error" >&2
    ((fails++))
fi
if bash "$script" '999999999999999999.9.9'; then
    echo "FAIL: Unknown version at the first argument did not cause any error" >&2
    ((fails++))
fi

# A version without an archive must be reported as such, naming the version and the URL, and leave nothing behind.
# The script keeps the download in a temporary directory: in the current directory on Windows, in $TMPDIR elsewhere.
mkdir ./tmpdir
out="$(TMPDIR="$(pwd)/tmpdir" bash "$script" '9.9.9' 2>&1)"
status=$?
if [[ "$status" != "1" ]]; then
    echo "FAIL: Version without an archive exited with ${status}, not 1: '${out}'" >&2
    ((fails++))
fi
if [[ "$out" != *'jactionlint v9.9.9 was not found'* || "$out" != *'/releases/download/v9.9.9/jactionlint_9.9.9_'* || "$out" == *gzip* ]]; then
    echo "FAIL: Version without an archive did not cause the not found message: '${out}'" >&2
    ((fails++))
fi
if [[ -n "$(ls -A ./tmpdir)" ]]; then
    echo "FAIL: Version without an archive left files in TMPDIR: '$(ls -A ./tmpdir)'" >&2
    ((fails++))
fi
rmdir ./tmpdir
if [[ -n "$(ls -A)" ]]; then
    echo "FAIL: Version without an archive left files behind: '$(ls -A)'" >&2
    ((fails++))
fi

# The architecture is detected from "uname -m". A version without an archive stops the script right after the
# detection, so this works on any OS and does not run a downloaded binary.
for pair in x86_64:amd64 amd64:amd64 i386:386 i686:386 aarch64:arm64 arm64:arm64 armv7l:armv6; do
    machine="${pair%%:*}"
    arch="${pair##*:}"
    # shellcheck disable=SC2329 # called by the script that runs below, through the exported function
    uname() {
        if [[ "$1" == '-m' ]]; then echo "$machine"; else command uname "$@"; fi
    }
    export machine
    export -f uname
    out="$(bash "$script" '9.9.9' 2>&1)"
    unset -f uname
    if [[ "$out" != *" arch=${arch}"$'\n'* ]]; then
        echo "FAIL: 'uname -m' printing '${machine}' did not select arch=${arch}: '${out}'" >&2
        ((fails++))
    fi
done

# Tests with a mock curl, an exported function like the mock uname above. Call N of the mock writes its arguments to
# args.N and its stdin to stdin.N. Line N of the file answers gives the exit code and the stdout of call N.
mock_dir="$(pwd)/mock"
mkdir "$mock_dir"
export mock_dir
# shellcheck disable=SC2329 # called by the script that runs below, through the exported function
curl() {
    local n=1 i code answer
    while [[ -e "$mock_dir/args.$n" ]]; do
        n=$((n + 1))
    done
    printf '%s\n' "$@" > "$mock_dir/args.$n"
    cat > "$mock_dir/stdin.$n"
    {
        for ((i = 1; i < n; i++)); do
            read -r _
        done
        read -r code answer
    } < "$mock_dir/answers"
    printf '%s' "$answer"
    return "${code:-1}"
}
export -f curl

# Usage: run_mock TOKEN ANSWER... Runs the download script with the mock curl and puts its output in $out.
function run_mock() {
    local token="$1"
    shift
    rm -f "$mock_dir"/args.* "$mock_dir"/stdin.*
    printf '%s\n' "$@" > "$mock_dir/answers"
    out="$(JACTIONLINT_TOKEN="$token" bash "$script" '9.9.9' </dev/null 2>&1)" || true
}

# Usage: expect_calls N. Fails when the mock curl did not get exactly N calls.
function expect_calls() {
    if [[ ! -e "$mock_dir/args.$1" || -e "$mock_dir/args.$(($1 + 1))" ]]; then
        echo "FAIL: curl did not get $1 calls: '${out}'" >&2
        ((fails++))
        return 1
    fi
}

# Usage: has_arg N ARG. Succeeds when call N of the mock curl got the argument ARG.
function has_arg() {
    grep -qx -- "$2" "$mock_dir/args.$1"
}

# curl reads the token header from a config file on stdin (-K -), not from the command line. The first request does
# not follow redirects.
run_mock 'test-token' '22 404'
if [[ "$out" != *'jactionlint v9.9.9 was not found'* ]]; then
    echo "FAIL: Download with a token and a 404 answer did not cause the not found message: '${out}'" >&2
    ((fails++))
fi
if expect_calls 1; then
    if [[ "$(< "$mock_dir/stdin.1")" != 'header = "Authorization: Bearer test-token"' ]]; then
        echo "FAIL: curl did not get the token header on stdin: '$(< "$mock_dir/stdin.1")'" >&2
        ((fails++))
    fi
    if ! has_arg 1 '-K' || has_arg 1 '-fL' || grep -q 'test-token' "$mock_dir/args.1"; then
        echo "FAIL: curl did not read the config from stdin, follows redirects, or got the token as an argument: '$(< "$mock_dir/args.1")'" >&2
        ((fails++))
    fi
fi

# The config file syntax needs a backslash before each backslash and double quote of the token.
run_mock 'a\b"c' '22 404'
if expect_calls 1 && [[ "$(< "$mock_dir/stdin.1")" != 'header = "Authorization: Bearer a\\b\"c"' ]]; then
    echo "FAIL: curl did not get the escaped token on stdin: '$(< "$mock_dir/stdin.1")'" >&2
    ((fails++))
fi

# Without a token, the config on stdin is empty.
run_mock '' '22 404'
if expect_calls 1 && [[ -n "$(< "$mock_dir/stdin.1")" ]]; then
    echo "FAIL: curl without a token got a config: '$(< "$mock_dir/stdin.1")'" >&2
    ((fails++))
fi

# The script follows a redirect with a second curl call, which does not get the token.
run_mock 'test-token' '0 302 https://assets.example.com/archive' '22 404'
if expect_calls 2; then
    if ! has_arg 2 'https://assets.example.com/archive' || has_arg 2 '-K' || [[ -s "$mock_dir/stdin.2" ]]; then
        echo "FAIL: curl did not download the redirect URL without the token: '$(< "$mock_dir/args.2")' '$(< "$mock_dir/stdin.2")'" >&2
        ((fails++))
    fi
    if [[ "$out" != *'jactionlint v9.9.9 was not found'* ]]; then
        echo "FAIL: A 404 answer after the redirect did not cause the not found message: '${out}'" >&2
        ((fails++))
    fi
fi

# Other errors cause a general message.
run_mock '' '7 000'
if [[ "$out" != *'Could not download jactionlint v9.9.9 from '* ]]; then
    echo "FAIL: A download without an HTTP response did not cause the general message: '${out}'" >&2
    ((fails++))
fi

unset -f curl
rm -rf "$mock_dir"

set -e
if [[ "$fails" != "0" ]]; then
    echo "${fails} error cases failed. Check the above log" >&2
    exit 1
fi

echo 'SUCCESS'
