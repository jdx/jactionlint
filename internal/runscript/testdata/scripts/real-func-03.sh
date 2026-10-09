version=${RELEASE_TAG#v}; tag=latest; [[ "$version" == *-* ]] && tag=next
publish_package() {
  local package=$1 name
  name=$(tar -xOf "$package" package/package.json | jq -r .name)
  if npm view "${name}@${version}" version >/dev/null 2>&1; then
    echo "${name}@${version} is already published"
    return
  fi
  npm publish "$package" --access public --provenance --tag "$tag"
}
# Relative paths need the ./ prefix or npm parses them as
# hosted-git owner/repo shorthands instead of local tarballs.
for package in ./packages/*-darwin-*.tgz ./packages/*-linux-*.tgz ./packages/*-win32-*.tgz; do publish_package "$package"; done
publish_package "./packages/jdxcode-aube-ffi-${version}.tgz"
