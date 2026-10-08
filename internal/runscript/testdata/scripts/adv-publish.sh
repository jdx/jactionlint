twine upload dist/*
python -m twine upload --repository testpypi dist/*
twine check dist/*
cargo publish
cargo publish --dry-run
npm publish --provenance --access public
npm publish --dry-run
pnpm publish --no-git-checks
yarn npm publish
gem push foo.gem
uv publish
poetry publish --build
gh release create v1.0.0 dist/* --title "v1" --notes-file notes.md
gh release upload v1.0.0 dist/*
gh release view v1
gh pr create --title "x" --body "y"
gh api repos/o/r/issues -f title=x
