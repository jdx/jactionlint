sudo apt-get update
sudo apt-get install -y libssl-dev pkg-config
apt-get install -y --no-install-recommends -o Dpkg::Options::=--force-confnew curl=7.88.1-10 git
apt install ./local.deb
sudo apt install -t bookworm-backports foo
apt-get remove -y foo
brew install jq
brew install --cask docker
brew install python@3.12 node
brew install ./local.rb
brew update && brew upgrade
uv pip install --system requests==2.0 flask
uv tool install ruff==0.4.0
uv tool install --from "git+https://github.com/o/r@v1" tool
uvx ruff check .
uv add pytest
uv sync --locked
pipx install black
pipx install "black==24.1.0"
pipx run --spec ruff==0.1.0 ruff check
pipx install --python python3.12 poetry
