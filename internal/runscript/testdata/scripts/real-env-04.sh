platform="${{ matrix.platform.platform }}"
echo "PLATFORM_PAIR=${platform//\//-}" >> "$GITHUB_ENV"
