for crate in ${{ matrix.crates }}; do
  cargo +${{ matrix.version }} check --locked -p "$crate" --all-features
done
