curl -fsSL https://mise.run | sh
echo "$HOME/.local/bin" >> "$GITHUB_PATH"
~/.local/bin/mise --version
