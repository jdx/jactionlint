sudo apt-get update -q
sudo apt-get install -y -q ffmpeg
(cd docs && aube install && aube exec playwright-core install --with-deps chromium-headless-shell)
echo "SHOWREEL_REQUIRE_CHROMIUM=1" >> "$GITHUB_ENV"
mise run render:showreel
