echo "tag=v$(jq -r .version package.json)" >> "$GITHUB_OUTPUT"
