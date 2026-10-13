#!/usr/bin/env bash
# Pin the workflow schemas of actions/runner that TestSchemaConformance compares the parser with.
#
#   scripts/update-schema-pin.bash            # pin the head of the default branch of actions/runner
#   scripts/update-schema-pin.bash <commit>   # pin a commit
#
# It downloads src/Sdk/WorkflowParser/workflow-v1.0.json and src/Runner.Worker/action_yaml.json at the commit into
# testdata/schema/runner/ and writes the commit to testdata/schema/runner/COMMIT. The test then tells what changed
# in the schemas and which tables of jactionlint have to follow (or which entry of the allow list to add).
# It needs the `gh` command, which is authenticated in a GitHub Actions workflow with GH_TOKEN.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

repo=actions/runner
dir=testdata/schema/runner

commit="${1:-}"
if [[ -z "$commit" ]]; then
    commit="$(gh api "repos/$repo/commits/HEAD" --jq .sha)"
fi
if [[ ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
    echo "not a full commit hash: $commit" >&2
    exit 1
fi

mkdir -p "$dir"
for f in src/Sdk/WorkflowParser/workflow-v1.0.json src/Runner.Worker/action_yaml.json; do
    tmp="$(mktemp)"
    gh api "repos/$repo/contents/$f?ref=$commit" -H "Accept: application/vnd.github.raw" >"$tmp"
    # Fail on an error page instead of pinning it
    if ! jq -e '.definitions' "$tmp" >/dev/null; then
        echo "$f at $commit is not a schema" >&2
        rm -f "$tmp"
        exit 1
    fi
    chmod 644 "$tmp"
    mv "$tmp" "$dir/$(basename "$f")"
done
echo "$commit" >"$dir/COMMIT"
echo "pinned $repo at $commit"
