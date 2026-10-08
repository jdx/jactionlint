cd /tmp/packages

# Configure dput
cat > ~/.dput.cf << EOF
[mise-ppa]
fqdn = ppa.launchpad.net
method = ftp
incoming = ~${PPA_NAME#ppa:}/ubuntu/
login = anonymous
allow_unsigned_uploads = 0
EOF

# Upload each changes file
for changes_file in *.changes; do
  echo "Uploading $changes_file to PPA..."
  dput mise-ppa "$changes_file"
done
