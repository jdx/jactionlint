#!/bin/bash

set -o pipefail
set -e

if [ ! -d .git ]; then
    echo 'This script must be run from root of repository' >&2
    exit 1
fi

set -x

# This repository has not published releases of its own that include the old versions below, so the
# script is tested against the releases of the original repository. The script works the same way for
# any repository.
export JACTIONLINT_REPO=rhysd/actionlint
# The executable and the archives of the original releases are named "actionlint"
export JACTIONLINT_NAME=actionlint
bin=actionlint

script="$(pwd)/scripts/download-jactionlint.bash"
temp_dir="$(mktemp -d)"
trap 'popd && rm -rf $temp_dir' EXIT
pushd "$temp_dir"

# Normal cases
set -e

# No arguments
out="$(bash "$script")"
if [ -n "$GITHUB_ACTION" ]; then
    if [[ "$out" != *"executable="* ]]; then
        echo "'executable' step output is not set: '${out}'" >&2
    fi
fi
out="$(./"${bin}" -version)"
if [[ "$out" != *'installed by downloading from release page'* ]]; then
    echo "Output from ./${bin} -version is unexpected: '${out}'" >&2
    exit 1
fi
rm -f ./"${bin}"

# Specify only version
bash "$script" '1.6.12'
out="$(./"${bin}" -version | head -n 1)"
if [[ "$out" != '1.6.12' ]]; then
    echo "Unexpected version: '${out}'" 1>&2
    exit 1
fi
rm -f ./"${bin}"

# Specify only a download directory
mkdir ./test1
bash "$script" latest ./test1
out="$(./test1/"${bin}" -version)"
if [[ "$out" != *'installed by downloading from release page'* ]]; then
    echo "Output from ./${bin} -version is unexpected: '${out}'" >&2
    exit 1
fi
rm -rf ./test1

# Specify both version and a download directory
mkdir ./test2
bash "$script" '1.6.12' ./test2
out="$(./test2/"${bin}" -version | head -n 1)"
if [[ "$out" != '1.6.12' ]]; then
    echo "Unexpected version: '${out}'" 1>&2
    exit 1
fi
rm -rf ./test2

# Error cases
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

set -e
if [[ "$fails" != "0" ]]; then
    echo "${fails} error cases failed. Check the above log" >&2
    exit 1
fi

echo 'SUCCESS'
