#!/bin/bash
# Build the jactionlint WebAssembly module and the Go runtime shim used by the docs site playground.
# Outputs: docs/public/playground/{main.wasm,wasm_exec.js} (gitignored build outputs).

set -e -o pipefail

if [ ! -d .git ] && [ ! -f .git ]; then
    echo 'This script must be run from root of repository: bash ./scripts/build-wasm.bash' 1>&2
    exit 1
fi

out=./docs/public/playground
mkdir -p "$out"

echo "Building ${out}/main.wasm"
GOOS=js GOARCH=wasm go build -o "${out}/main.wasm" ./playground

goroot="$(go env GOROOT)"
wasm_exec="${goroot}/lib/wasm/wasm_exec.js"
if [ ! -f "${wasm_exec}" ]; then
    wasm_exec="${goroot}/misc/wasm/wasm_exec.js" # Go 1.23 or earlier
fi
if [ ! -f "${wasm_exec}" ]; then
    echo "wasm_exec.js was not found in ${goroot}" 1>&2
    exit 1
fi
echo "Copying ${wasm_exec} to ${out}/wasm_exec.js"
cp "${wasm_exec}" "${out}/wasm_exec.js"
