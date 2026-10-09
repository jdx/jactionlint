cargo build --locked --release -p mbx
echo "MBX_BENCH_BINARY=$PWD/target/release/mbx" >> "$GITHUB_ENV"
