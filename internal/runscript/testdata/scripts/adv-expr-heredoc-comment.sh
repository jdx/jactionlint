# comment with ${{ secrets.TOKEN }}
cat <<EOF
${{ github.event.comment.body }}
EOF
echo x # ${{ inputs.y }}
