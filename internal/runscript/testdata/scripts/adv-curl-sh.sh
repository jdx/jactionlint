curl -fsSL https://example.com/install.sh | sh
curl -sSf https://sh.rustup.rs | sh -s -- -y
wget -qO- https://example.com/install.sh | sudo bash
curl --proto '=https' --tlsv1.2 -sSf -H "Accept: x" -o /dev/null https://a.example | bash -s
bash <(curl -fsSL https://example.com/install.sh)
sh -c "$(curl -fsSL https://example.com/install.sh)"
eval "$(curl -fsSL https://example.com/env.sh)"
source <(curl -s https://example.com/env.sh)
curl -fsSL https://example.com/install.sh -o install.sh && sh install.sh
curl -fsSL https://example.com/data.json | jq .
curl -fsSL https://example.com/x.sh | bash script.sh
curl -fsSL http://insecure.example.com/x.sh | sh
curl -fsSL ${{ inputs.url }} | sh
curl -fsSL https://example.com/a | tee out | sh
