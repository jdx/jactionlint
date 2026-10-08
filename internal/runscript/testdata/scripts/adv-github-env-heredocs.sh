cat <<EOF >> $GITHUB_ENV
TITLE=${{ github.event.issue.title }}
OTHER=$(date)
EOF
cat <<-'EOF' >> "$GITHUB_OUTPUT"
	body=${{ steps.x.outputs.y }}
	EOF
cat >> $GITHUB_ENV <<EOT
A=1
EOT
tee -a $GITHUB_ENV <<< "B=${{ inputs.b }}"
cat <<EOF | tee -a $GITHUB_ENV
C=${{ matrix.c }}
EOF
