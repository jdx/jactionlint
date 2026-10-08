git add -- '*aube-lock.yaml'
while IFS= read -r path; do
  if [ -n "$path" ]; then
    git add -- "$path"
  fi
done <<< "$POST_UPDATE_PATHS"
if git diff --cached --quiet; then
  echo "dependency artifacts already up to date"
  exit 0
fi
git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
# The paths this job changed, so a rejected push can ask whether
# they are already on the branch.
changed_paths="$(git diff --cached --name-only)"
# The commit this job builds on. Read before committing: the
# checkout is a depth-1 shallow clone, where HEAD^ does not resolve
# even once a commit sits on top of it.
base="$(git rev-parse HEAD)"
git commit -m 'chore(deps): regenerate dependency artifacts'
# Renovate force-pushes the branch while this job runs, so a plain
# push loses the fix to a non-fast-forward rejection (seen on
# jdx/hk). Recover by looking at why the branch moved.
push_url="https://x-access-token:${PUSH_TOKEN}@github.com/${REPO}.git"
# Loops until it pushes or gives up: a recovery must always be
# followed by another push, never left rebased-but-unpushed.
retries=0
recoveries=0
while true; do
  # The lease pins the push to the tip we built on, so it is
  # rejected rather than silently recreating a branch Renovate
  # deleted or clobbering a newer one.
  if git push --force-with-lease="refs/heads/${BRANCH}:${base}" "$push_url" "HEAD:${BRANCH}"; then
    exit 0
  fi
  # Never retry a push without knowing the branch's current state:
  # the fetch also fails when Renovate has deleted the branch, and
  # a blind retry would recreate it.
  if ! git fetch --quiet "$push_url" "$BRANCH"; then
    echo "::error::could not fetch ${BRANCH} to see why the push was rejected; not retrying (the branch may have been deleted)"
    exit 1
  fi
  # --verify --quiet prints nothing on failure; plain rev-parse
  # would echo the literal "FETCH_HEAD" and be mistaken for a sha.
  remote="$(git rev-parse --verify --quiet "FETCH_HEAD^{commit}" || true)"
  if [ -z "$remote" ]; then
    echo "::error::fetched ${BRANCH} but could not resolve its tip"
    exit 1
  fi
  if [ "$remote" = "$base" ]; then
    # The branch is where we left it, so the push itself failed.
    retries=$((retries + 1))
    if [ "$retries" -gt 3 ]; then
      echo "::error::could not push regenerated dependency artifacts to ${BRANCH} after ${retries} attempts"
      exit 1
    fi
    echo "push failed but ${BRANCH} has not moved; retrying (attempt ${retries})"
    sleep "$((retries * 5))"
    continue
  fi
  # Who moved the branch decides whether anything will regenerate
  # these artifacts: only a Renovate push starts a replacement run
  # (the job's sender gate), and a github-actions[bot] tip is
  # another run of this workflow that has already done the work.
  tip_author="$(git log -1 --format='%an <%ae>' "$remote" 2>/dev/null)"
  case "$tip_author" in
    *'renovate[bot]'*) moved_by=renovate ;;
    *'github-actions[bot]'*) moved_by=sibling ;;
    *) moved_by=foreign ;;
  esac
  # Nothing to do if those artifacts are already on the branch --
  # a sibling run regenerating the same config lands the same bytes.
  if [ -n "$changed_paths" ]; then
    already_there=true
    while IFS= read -r changed; do
      [ -n "$changed" ] || continue
      if ! git diff --quiet "$remote" HEAD -- "$changed"; then
        already_there=false
        break
      fi
    done <<< "$changed_paths"
    if [ "$already_there" = true ]; then
      echo "::notice::${BRANCH} already carries these artifacts (pushed by ${tip_author}); nothing to do"
      exit 0
    fi
  fi
  # 0 = unchanged, 1 = changed, anything else = diff failed.
  config_changed=0
  git diff --quiet "$base" "$remote" -- '*package.json' || config_changed=$?
  if [ "$config_changed" -gt 1 ]; then
    retries=$((retries + 1))
    if [ "$retries" -gt 3 ]; then
      echo "::error::could not compare ${base} with ${remote} after ${retries} attempts"
      exit 1
    fi
    echo "could not compare ${base} with ${remote}; retrying (attempt ${retries})"
    sleep "$((retries * 5))"
    continue
  fi
  if [ "$config_changed" -eq 1 ]; then
    # A newer push changed the inputs these artifacts are generated
    # from, so what we regenerated is already out of date. That push
    # started its own run of this workflow, which regenerates from
    # the new state; pushing ours would only overwrite it with stale
    # content.
    if [ "$moved_by" = renovate ]; then
      echo "::notice::${BRANCH} moved and its dependency config changed; the run for that push regenerates these artifacts, so this one stops here"
      exit 0
    fi
    echo "::error::${BRANCH} moved and its dependency config changed, but not by renovate[bot] (tip: ${tip_author}), so no run will regenerate these artifacts; rerun this workflow once the branch settles"
    exit 1
  fi
  # The branch moved for unrelated reasons, so our artifacts still
  # describe the current config: replay them on the new tip.
  echo "${BRANCH} moved without touching the dependency config; rebasing onto ${remote}"
  # --onto replays only the commit made above. A plain rebase would
  # also replay everything under $base that a force-push dropped,
  # restoring content Renovate deliberately removed.
  if ! git rebase --onto "$remote" "$base"; then
    git rebase --abort || true
    if [ "$moved_by" != foreign ]; then
      echo "::notice::rebase onto ${remote} conflicted; ${tip_author} already regenerated these artifacts, so this run stops here"
      exit 0
    fi
    echo "::error::rebase onto ${remote} conflicted and ${BRANCH} was moved by ${tip_author}, so no run will regenerate these artifacts"
    exit 1
  fi
  recoveries=$((recoveries + 1))
  if [ "$recoveries" -gt 3 ]; then
    echo "::error::${BRANCH} moved again after ${recoveries} rebases; giving up rather than fighting it"
    exit 1
  fi
  base="$remote"
done
