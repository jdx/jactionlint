mkdir npm-smoke
cd npm-smoke
npm init --yes >/dev/null
npm install --force --ignore-scripts --no-package-lock ../packages/*.tgz
node -e "const ffi=require('@jdxcode/aube-ffi'); require('fs').accessSync(ffi.libraryPath); require('fs').accessSync(ffi.headerPath)"
cp "$GITHUB_WORKSPACE/crates/aube-ffi/npm/typecheck.ts" .
npx --yes --package=typescript@5.9.3 tsc --strict --noEmit \
  --lib ES2022,DOM --module NodeNext --moduleResolution NodeNext typecheck.ts
