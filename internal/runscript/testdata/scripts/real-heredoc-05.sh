set -euo pipefail
VERSION="0.0.0-smoke.${RUN_ID}.${RUN_ATTEMPT}"
STAGE="$RUNNER_TEMP/aube-publish-smoke"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R npm/smoke-package/. "$STAGE/"
node - "$STAGE/package.json" "$VERSION" <<'NODE'
const fs = require('node:fs');
const [path, version] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(path, 'utf8'));
pkg.version = version;
fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + '\n');
NODE
echo "version=$VERSION" >> "$GITHUB_OUTPUT"
echo "stage=$STAGE" >> "$GITHUB_OUTPUT"
