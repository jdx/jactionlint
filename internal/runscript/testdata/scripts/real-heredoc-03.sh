KEY_ID=$(gpg --list-secret-keys --with-colons | grep '^sec:' | cut -d: -f5 | head -1)
if [ -z "$KEY_ID" ]; then
  echo "Error: No GPG key found"
  exit 1
fi
echo "Using GPG key: $KEY_ID"
echo "DEBSIGN_KEYID=$KEY_ID" >> "$GITHUB_ENV"

cat > ~/.devscripts << EOF
DEBSIGN_KEYID=$KEY_ID
DEBUILD_DPKG_BUILDPACKAGE_OPTS="-i -I -S -sa"
DEBUILD_LINTIAN_OPTS="-i -I --show-overrides --profile ubuntu"
DEBSIGN_PROGRAM=gpg
EOF
