case "$TASK" in
  legacy-drop-allow-cutover-gaps) args=(--mode=drop --allow-cutover-gaps) ;;
  *) args=(--mode="${TASK#legacy-}") ;;
esac
node scripts/retire-legacy-downloads-direct.js "${args[@]}"
