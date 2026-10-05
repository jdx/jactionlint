#!/bin/bash

set -e -o pipefail

if [ ! -d .git ]; then
    echo 'This script must be run from root of repository: bash ./scripts/build-pages.bash' 1>&2
    exit 1
fi

dist=./playground-dist

echo 'Installing dependencies and building wasm'
(cd ./playground && make clean && make build)

echo "Creating ${dist}"
rm -rf "$dist"
mkdir "$dist"

files=(
    index.html
    index.js
    index.js.map
    index.ts
    lib
    main.wasm
    style.css
)

echo "Copying built assets from ./playground to ${dist}: " "${files[@]}"
for f in "${files[@]}"; do
    cp -R "./playground/${f}" "${dist}/${f}"
done

echo "Applying wasm-opt to ${dist}/main.wasm"
wasm-opt -O -o "${dist}/opt.wasm" "${dist}/main.wasm" --enable-bulk-memory --enable-nontrapping-float-to-int --enable-sign-ext
mv "${dist}/opt.wasm" "${dist}/main.wasm"

echo "Done. The site is in ${dist}"
