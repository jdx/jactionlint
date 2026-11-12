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

# Tests with a mock curl. Each call of the mock records its arguments in args.N and its stdin in stdin.N, where N counts
# the calls. Line N of the file answers gives the exit code and the stdout of call N.
mkdir ./mockbin
cat > ./mockbin/curl <<'MOCK'
#!/bin/bash
n=1
while [[ -e "$MOCK_CURL_DIR/args.$n" ]]; do
    n=$((n + 1))
done
printf '%s\n' "$@" > "$MOCK_CURL_DIR/args.$n"
cat > "$MOCK_CURL_DIR/stdin.$n"
read -r code answer < <(sed -n "${n}p" "$MOCK_CURL_DIR/answers")
printf '%s' "$answer"
exit "${code:-1}"
MOCK
chmod +x ./mockbin/curl
MOCK_CURL_DIR="$(pwd)/mockbin"
export MOCK_CURL_DIR

# Usage: run_mock TOKEN ANSWER... Runs the download script with the mock curl and puts its output in $out.
function run_mock() {
    local token="$1"
    shift
    rm -f ./mockbin/args.* ./mockbin/stdin.*
    printf '%s\n' "$@" > ./mockbin/answers
    out="$(PATH="$MOCK_CURL_DIR:$PATH" JACTIONLINT_TOKEN="$token" bash "$script" '9.9.9' </dev/null 2>&1)" || true
}

# Usage: expect_calls N. Fails when the mock curl did not get N calls.
function expect_calls() {
    local calls
    calls="$(find ./mockbin -name 'args.*' | wc -l | tr -d ' ')"
    if [[ "$calls" != "$1" ]]; then
        echo "FAIL: curl got ${calls} calls, not $1: '${out}'" >&2
        ((fails++))
        return 1
    fi
}

# With a token, curl reads the header from a config file on stdin (-K -), not from the command line, and does not
# follow redirects.
run_mock 'test-token' '22 404'
if [[ "$out" != *'jactionlint v9.9.9 was not found'* ]]; then
    echo "FAIL: Download with a token and a 404 answer did not cause the not found message: '${out}'" >&2
    ((fails++))
fi
if expect_calls 1; then
    if [[ "$(cat ./mockbin/stdin.1)" != 'header = "Authorization: Bearer test-token"' ]]; then
        echo "FAIL: curl did not get the token header on stdin: '$(cat ./mockbin/stdin.1)'" >&2
        ((fails++))
    fi
    if ! grep -qx -- '-K' ./mockbin/args.1 || grep -q 'test-token' ./mockbin/args.1 || grep -qx -- '-fL' ./mockbin/args.1; then
        echo "FAIL: curl did not read the config from stdin, got the token as an argument, or follows redirects: '$(cat ./mockbin/args.1)'" >&2
        ((fails++))
    fi
fi

# The config file syntax needs a backslash before each backslash and double quote of the token.
run_mock 'a\b"c' '22 404'
if expect_calls 1 && [[ "$(cat ./mockbin/stdin.1)" != 'header = "Authorization: Bearer a\\b\"c"' ]]; then
    echo "FAIL: curl did not get the escaped token on stdin: '$(cat ./mockbin/stdin.1)'" >&2
    ((fails++))
fi

# With a token, the script follows a redirect with a second curl call, which does not get the token.
run_mock 'test-token' '0 302 https://assets.example.com/archive' '22 404'
if expect_calls 2; then
    if ! grep -qx 'https://assets.example.com/archive' ./mockbin/args.2 || grep -qx -- '-K' ./mockbin/args.2 ||
        [[ -s ./mockbin/stdin.2 ]]; then
        echo "FAIL: curl did not download the redirect URL without the token: '$(cat ./mockbin/args.2)' '$(cat ./mockbin/stdin.2)'" >&2
        ((fails++))
    fi
    if [[ "$out" != *'jactionlint v9.9.9 was not found'* ]]; then
        echo "FAIL: A 404 answer after the redirect did not cause the not found message: '${out}'" >&2
        ((fails++))
    fi
fi

# Without a token, curl gets no header and follows redirects.
run_mock '' '22 404'
if expect_calls 1; then
    if grep -qx -- '-K' ./mockbin/args.1 || grep -qx -- '-H' ./mockbin/args.1 || [[ -s ./mockbin/stdin.1 ]] ||
        ! grep -qx -- '-fL' ./mockbin/args.1; then
        echo "FAIL: curl without a token got a header or does not follow redirects: '$(cat ./mockbin/args.1)' '$(cat ./mockbin/stdin.1)'" >&2
        ((fails++))
    fi
fi

# When curl gets no HTTP response, it writes 000. The message must not give 000 as an HTTP status.
for token in '' 'test-token'; do
    run_mock "$token" '7 000'
    if [[ "$out" != *'Could not download jactionlint v9.9.9 from '* || "$out" == *'HTTP status 000'* ]]; then
        echo "FAIL: A download without an HTTP response (token '${token}') did not cause the correct message: '${out}'" >&2
        ((fails++))
    fi
done

# Other HTTP errors give the status.
run_mock '' '22 503'
if [[ "$out" != *'(HTTP status 503)'* ]]; then
    echo "FAIL: A 503 answer did not cause a message with the HTTP status: '${out}'" >&2
    ((fails++))
fi

rm -rf ./mockbin
unset MOCK_CURL_DIR

set -e
if [[ "$fails" != "0" ]]; then
    echo "${fails} error cases failed. Check the above log" >&2
    exit 1
fi

echo 'SUCCESS'
