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

# The token goes to curl as a header on stdin, not on the command line. A mock curl records its arguments and stdin,
# and answers 404, so the script stops before it extracts anything.
mkdir ./mockbin
cat > ./mockbin/curl <<'EOF'
#!/bin/bash
printf '%s\n' "$@" > "$MOCK_CURL_DIR/args"
cat > "$MOCK_CURL_DIR/stdin"
printf '404'
exit 22
EOF
chmod +x ./mockbin/curl
MOCK_CURL_DIR="$(pwd)/mockbin"
export MOCK_CURL_DIR

out="$(PATH="$MOCK_CURL_DIR:$PATH" JACTIONLINT_TOKEN='test-token' bash "$script" '9.9.9' </dev/null 2>&1)"
if [[ "$out" != *'jactionlint v9.9.9 was not found'* ]]; then
    echo "FAIL: Download with a mock curl did not cause the not found message: '${out}'" >&2
    ((fails++))
fi
if [[ "$(cat ./mockbin/stdin)" != 'Authorization: Bearer test-token' ]]; then
    echo "FAIL: curl did not get the token header on stdin: '$(cat ./mockbin/stdin)'" >&2
    ((fails++))
fi
if ! grep -qx -- '@-' ./mockbin/args || grep -q 'test-token' ./mockbin/args; then
    echo "FAIL: curl did not read the header from stdin, or got the token as an argument: '$(cat ./mockbin/args)'" >&2
    ((fails++))
fi

out="$(PATH="$MOCK_CURL_DIR:$PATH" JACTIONLINT_TOKEN='' bash "$script" '9.9.9' </dev/null 2>&1)"
if grep -qx -- '-H' ./mockbin/args || [[ -s ./mockbin/stdin ]]; then
    echo "FAIL: curl got a header without a token: '$(cat ./mockbin/args)' '$(cat ./mockbin/stdin)'" >&2
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
