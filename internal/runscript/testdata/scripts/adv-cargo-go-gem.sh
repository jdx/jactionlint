cargo install ripgrep
cargo install --locked ripgrep --version 14.1.0
cargo install cargo-nextest --locked
cargo install --git https://github.com/o/r --rev 0123456789abcdef0123456789abcdef01234567
cargo install --git https://github.com/o/r --branch main tool
cargo install --path . --force
cargo install cargo-edit@0.12.2
cargo install --version "=1.2.3" foo
cargo binstall -y cargo-deny@0.14.0 cargo-audit
cargo-binstall --no-confirm just
cargo --version && rustc --version
go install golang.org/x/tools/cmd/goimports@latest
go install honnef.co/go/tools/cmd/staticcheck@v0.5.1
go install github.com/o/r/cmd/x@0123456789ab
go install github.com/o/r@master
go install ./cmd/foo ./...
go get -u github.com/o/r
go version
gem install bundler
gem install bundler -v 2.5.3
gem install rake --version '~> 13.0'
gem install foo:1.2.3 bar ./local.gem
gem push pkg-1.0.gem
