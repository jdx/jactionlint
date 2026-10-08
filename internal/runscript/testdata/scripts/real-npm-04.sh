if [ -z "$(git ls-files -- '*aube-lock.yaml')" ]; then
  echo "no aube-lock.yaml in this repo, nothing to do"
  echo "found=false" >> "$GITHUB_OUTPUT"
  exit 0
fi
echo "found=true" >> "$GITHUB_OUTPUT"
for d in $(git ls-files -- '*aube-lock.yaml' | xargs -n1 dirname); do
  echo "regenerating aube-lock.yaml in $d"
  (cd "$d" && mise x -- aube install --no-frozen-lockfile) || exit 1
done
