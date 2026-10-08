# Sign aube only — aubr/aubx become symlinks to aube in the
# archive step below and share its signature. Gatekeeper
# validates the underlying file's signature; the invocation
# name only matters for argv[0] dispatch inside the binary.
codesign --sign "$IDENTITY" --options runtime --timestamp \
  --prefix dev.jdx. "$BIN_DIR/aube"
